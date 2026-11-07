"use strict";
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { IntegrationManager, IntegrationError } = require("../lib/integrations.cjs");
const { InaraError } = require("../lib/inara.cjs");
const temporary = fs.mkdtempSync(path.join(__dirname, "copilot-integrations-test-"));
const encryptionKey = crypto.randomBytes(32);
const secureStorage = {
  encrypt(value) {
    const nonce = crypto.randomBytes(12), cipher = crypto.createCipheriv("aes-256-gcm", encryptionKey, nonce);
    const encrypted = Buffer.concat([cipher.update(value, "utf8"), cipher.final()]);
    return Buffer.concat([nonce, cipher.getAuthTag(), encrypted]);
  },
  decrypt(value) {
    const cipher = crypto.createDecipheriv("aes-256-gcm", encryptionKey, value.subarray(0, 12));
    cipher.setAuthTag(value.subarray(12, 28));
    return Buffer.concat([cipher.update(value.subarray(28)), cipher.final()]).toString("utf8");
  }
};
let clock = Date.parse("2026-11-07T12:00:00Z"), offset = 0;
const at = () => new Date(clock).toISOString();
const baseState = () => ({ commander: { name: "Synthetic Commander", fid: "F000000", system: "Test System", systemAddress: 123,
  coordinates: [1, 2, 3], vehicle: "ship", station: "Test Port", marketId: 456, docked: true, shipType: "anaconda", shipId: 7,
  cargo: [], cargoVessel: "ship" }, game: { version: "4.1.0.1234", build: "r123 ", live: true, loan: 0, horizons: true, odyssey: true },
  connection: { gameRunning: true }, app: { version: "1.4.1" }, pilot: {} });
let state = baseState(), manager, getCalls = 0;
const getState = () => { getCalls++; return { ...state, integrations: manager?.status({ commander: state.commander, game: state.game }) }; };
const observe = (type, data = {}, meta = {}) => manager.observeJournal({ event: type, timestamp: at(), ...data }, { file: "synthetic.journal", offset: ++offset, replay: false, ...meta });
const accepted = (options) => ({ ok: true, headerStatus: 200, setupRequired: false, results: options.events.map((_x, index) => ({ index, ok: true, retryable: false, status: 200 })) });
let inaraCalls = [], eddnCalls = [], sendMode = "ok";
const transports = {
  async sendInara(options) {
    inaraCalls.push(options);
    if (sendMode === "network") throw new InaraError("Синтетический сбой", "NETWORK", true, 60000);
    if (sendMode === "setup") return { ok: false, setupRequired: true, message: "Синтетический отказ допуска", results: options.events.map(() => ({ ok: false, retryable: false, status: 400 })) };
    if (sendMode === "partial") return { ok: false, results: options.events.map((_x, i) => ({ ok: i === 0, retryable: i > 0, status: i === 0 ? 200 : 500 })) };
    return accepted(options);
  },
  async sendEddn(payload) { eddnCalls.push(payload); return { ok: true, status: 200, retryable: false }; }
};
const create = (dir = "main", extras = {}) => new IntegrationManager({ dataDir: path.join(temporary, dir), secureStorage, getState,
  now: () => clock, transports, autoDelayMs: 3600000, ...extras });
const settings = { publisher: "copilot", inaraEnabled: true, eddnEnabled: true, inaraApiKey: "synthetic-only-api-key", inaraAppRegistered: true };

async function main() {
  const noStorage = new IntegrationManager({ dataDir: path.join(temporary, "no-storage"), getState, now: () => clock });
  assert.throws(() => noStorage.configure(settings), (e) => e instanceof IntegrationError && e.code === "STORAGE_UNAVAILABLE");
  assert.equal(fs.existsSync(noStorage.file), false); noStorage.close();
  manager = create();
  const callsBeforeStatus = getCalls; manager.status({ commander: state.commander, game: state.game });
  assert.equal(getCalls, callsBeforeStatus, "Status must not call getState and recurse");
  observe("MissionAccepted", { MissionID: 1, Name: "Mission_Delivery", timestamp: "2026-11-06T00:00:00Z" }, { replay: true });
  manager.configure(settings);
  assert.equal(manager.status().identityMatches, true);
  observe("MissionAccepted", { MissionID: 2, Name: "Mission_Delivery", timestamp: "2026-11-06T00:00:00Z" });
  assert.equal(manager.status().queue.total, 0, "An unseen old file must not upload old history after opting in");
  const callsBeforeReplay = getCalls;
  observe("MissionAccepted", { MissionID: 3, Name: "Mission_Delivery" }, { replay: true });
  assert.equal(getCalls, callsBeforeReplay, "Replaying historical journals must not build the full renderer state per record");
  assert.equal(manager.status().queue.total, 0, "Rescan/replay only establishes cursor boundaries");
  const missionOffset = ++offset;
  const mission = { event: "MissionAccepted", timestamp: at(), MissionID: 4, Name: "Mission_Delivery" };
  manager.observeJournal(mission, { file: "synthetic.journal", offset: missionOffset });
  manager.observeJournal(mission, { file: "synthetic.journal", offset: missionOffset });
  assert.equal(manager.status().inara.queued, 1, "Same cursor must not enqueue twice");
  observe("Scan", { SystemAddress: 123, BodyName: "Test System A1", BodyID: 1, Landable: true, PlanetClass: "Rocky body", FuelLevel: 32, Credits: 888 });
  assert.equal(manager.status().eddn.queued, 1);
  await manager.sync();
  assert.equal(manager.status().queue.total, 0);
  assert.equal(inaraCalls.length, 1); assert.equal(eddnCalls.length, 1);
  assert.equal(eddnCalls[0].message.Credits, undefined); assert.equal(eddnCalls[0].message.FuelLevel, undefined);
  assert.equal(inaraCalls[0].commanderName, state.commander.name);
  assert.equal(inaraCalls[0].events[0].eventName, "addCommanderMission");
  manager.observeJournal(mission, { file: "another-file.journal", offset: 1 });
  assert.equal(manager.status().queue.total, 0, "Acknowledged events are deduplicated across files");
  const fileBytes = fs.readFileSync(manager.file);
  assert.ok(!fileBytes.includes(Buffer.from(settings.inaraApiKey)));
  assert.ok(!fileBytes.includes(Buffer.from("Synthetic Commander")));
  assert.ok(!JSON.stringify(manager.status()).includes(settings.inaraApiKey));

  sendMode = "partial";
  observe("MissionAccepted", { MissionID: 5, Name: "Mission_Delivery" });
  observe("MissionAccepted", { MissionID: 6, Name: "Mission_Delivery" });
  await manager.sync();
  assert.equal(manager.status().inara.queued, 1, "Only the unacknowledged event remains pending");
  assert.ok(Date.parse(manager.status().inara.nextRetryAt) >= clock + 60000);
  const firstRetryCount = inaraCalls.length;
  await manager.sync(); assert.equal(inaraCalls.length, firstRetryCount, "Manual sync must respect server backoff");
  observe("MissionAccepted", { MissionID: 53, Name: "Mission_Delivery" });
  await manager.sync(); assert.equal(inaraCalls.length, firstRetryCount, "New observations cannot bypass a service retry cooldown");
  const pendingId = manager.queue[0].payload.eventData.missionGameID;
  manager.close();
  manager = create();
  assert.ok(manager.timer, "Persisted pending delivery is scheduled automatically on restart");
  assert.equal(manager.status().inara.keySet, true);
  assert.equal(manager.status().inara.queued, 2, "Encrypted pending queue survives a restart");
  clock += 61000; sendMode = "ok";
  await manager.sync();
  assert.equal(manager.status().queue.total, 0);
  assert.deepEqual(inaraCalls.at(-1).events.map((x) => x.eventData.missionGameID), [pendingId, 53]);
  manager.observeJournal(mission, { file: "synthetic.journal", offset: missionOffset });
  assert.equal(manager.status().queue.total, 0, "Persisted cursor survives restart");

  sendMode = "setup";
  observe("MissionAccepted", { MissionID: 7, Name: "Mission_Delivery" });
  await manager.sync();
  assert.equal(manager.status().inara.setupRequired, true);
  assert.equal(manager.status().inara.queued, 1, "App/key setup failure preserves pending events");
  manager.configure({ inaraApiKey: "synthetic-corrected-key" });
  sendMode = "ok"; await manager.sync();
  assert.equal(manager.status().inara.queued, 0, "Fixing the key permits an immediate retry");
  assert.equal(manager.status().inara.setupRequired, false);

  observe("MissionAccepted", { MissionID: 8, Name: "Mission_Delivery" });
  state = { ...state, commander: { ...state.commander, name: "Another Commander", fid: "F000999" } };
  manager.status({ commander: state.commander, game: state.game });
  assert.equal(manager.status().identityMatches, false);
  const identityCalls = inaraCalls.length; await manager.sync();
  assert.equal(inaraCalls.length, identityCalls, "Never flush a queue using another commander's identity");
  assert.throws(() => manager.configure({ publisher: "copilot" }), (e) => e.code === "COMMANDER_MISMATCH");
  state = baseState(); manager.status({ commander: state.commander, game: state.game });
  manager.configure({ publisher: "edmc" });
  assert.equal(manager.status().queue.total, 0, "Switching publisher discards pending Copilot uploads");
  const pairing = manager.pair();
  assert.ok(pairing.token.length >= 32); assert.match(pairing.url, /^http:\/\/127\.0\.0\.1:7122\//);
  assert.ok(!JSON.stringify(manager.status()).includes(pairing.token));
  assert.throws(() => manager.acceptEdmc({ type: "heartbeat", commander: state.commander.name }, "wrong-token"), (e) => e.status === 401);
  assert.equal(manager.acceptEdmc({ type: "journal", commander: state.commander.name, pluginVersion: "1.0.0", liveGalaxy: true,
    entry: { event: "Docked" } }, pairing.token).ok, true);
  assert.equal(manager.status().edmc.connected, true);
  observe("Scan", { SystemAddress: 123, BodyName: "Test System A2", BodyID: 2 });
  observe("MissionAccepted", { MissionID: 9, Name: "Mission_Delivery" });
  assert.equal(manager.status().queue.total, 0, "EDMC mode cannot also publish from Copilot");
  const rotated = manager.pair(); assert.notEqual(rotated.token, pairing.token);
  assert.throws(() => manager.acceptEdmc({ type: "heartbeat", commander: state.commander.name }, pairing.token), (e) => e.code === "BRIDGE_UNAUTHORIZED");

  manager.configure({ publisher: "copilot", inaraAppRegistered: false });
  observe("MissionAccepted", { MissionID: 10, Name: "Mission_Delivery" });
  assert.equal(manager.status().inara.queued, 0); assert.equal(manager.status().inara.setupRequired, true);
  manager.configure({ inaraAppRegistered: true });
  state.game = { version: "3.8.0", live: false };
  observe("MissionAccepted", { MissionID: 11, Name: "Mission_Delivery" });
  observe("Scan", { SystemAddress: 123, BodyName: "Test System A3", BodyID: 3 });
  assert.equal(manager.status().queue.total, 0, "Legacy galaxy never uploads");
  state.game = { version: "4.1.0 Beta", live: true, isBeta: true };
  observe("MissionAccepted", { MissionID: 12, Name: "Mission_Delivery" });
  assert.equal(manager.status().queue.total, 0);
  state = baseState();
  manager.configure({ publisher: "off" });
  observe("MissionAccepted", { MissionID: 13, Name: "Mission_Delivery" });
  const offOffset = offset;
  manager.configure({ publisher: "copilot" });
  manager.observeJournal({ event: "MissionAccepted", timestamp: at(), MissionID: 13, Name: "Mission_Delivery" }, { file: "synthetic.journal", offset: offOffset });
  assert.equal(manager.status().queue.total, 0, "Events recorded while disabled are not replayed on enabling");

  const oldMarket = { event: "Market", timestamp: new Date(clock - 10000).toISOString(), StarSystem: state.commander.system,
    StationName: state.commander.station, MarketID: state.commander.marketId, Items: [] };
  manager.observeSide(oldMarket); assert.equal(manager.status().queue.total, 0, "Old station side file is not a new observation");
  manager.observeSide({ ...oldMarket, timestamp: at() });
  assert.equal(manager.status().eddn.queued, 1);
  manager.observeSide({ ...oldMarket, timestamp: at() }, { replay: true }); assert.equal(manager.status().eddn.queued, 1);
  await manager.sync();
  const capiProfile = { commander: { name: state.commander.name, docked: true, credits: 5000, debt: 20 },
    lastSystem: { name: state.commander.system }, lastStarport: { name: state.commander.station, id: state.commander.marketId } };
  manager.observeCapi({ metadata: { source: "Frontier CAPI", observedAt: { profile: at() } }, data: { profile: capiProfile } });
  assert.equal(manager.status().inara.queued, 1);
  await manager.sync();
  assert.equal(inaraCalls.at(-1).events[0].eventData.commanderLoan, 20);
  manager.observeCapi({ metadata: { source: "Frontier CAPI", observedAt: { profile: at() } }, data: { profile: { ...capiProfile, commander: { name: "Wrong", credits: 1000, debt: 0 } } } });
  assert.equal(manager.status().inara.queued, 0);

  manager.configure({ inaraEnabled: false });
  let raceCalls = 0;
  manager.transports = { ...transports, async sendEddn(_payload, options) {
    raceCalls++;
    state = { ...state, commander: { ...state.commander, fid: "F000999", name: "Another Commander" } };
    manager.status({ commander: state.commander });
    assert.equal(options.signal.aborted, true, "Switching commander aborts the outstanding upload");
    return { ok: true, status: 200 };
  } };
  observe("Scan", { SystemAddress: 123, BodyName: "Test System A4", BodyID: 4 });
  observe("Scan", { SystemAddress: 123, BodyName: "Test System A5", BodyID: 5 });
  await manager.sync();
  assert.equal(raceCalls, 1, "No second message may leave after commander changes mid-batch");
  assert.equal(manager.status().eddn.queued, 2, "Late response cannot acknowledge a paused commander's batch");
  state = baseState(); manager.configure({ publisher: "off" });
  manager.configure({ publisher: "copilot", inaraEnabled: true });

  // A publisher switch cancels a pending request and cannot later acknowledge it under the new mode.
  let aborted = false;
  manager.transports = { ...transports, sendInara: (options) => new Promise((_resolve, reject) => options.signal.addEventListener("abort", () => {
    aborted = true; reject(new InaraError("Отменено", "ABORTED", false));
  }, { once: true })) };
  observe("MissionAccepted", { MissionID: 14, Name: "Mission_Delivery" });
  const draining = manager.sync(); await Promise.resolve();
  manager.configure({ publisher: "off" }); await draining;
  assert.equal(aborted, true); assert.equal(manager.status().publisher, "off"); assert.equal(manager.status().queue.total, 0);
  manager.close();

  const locked = new IntegrationManager({ dataDir: path.join(temporary, "main"), secureStorage: { encrypt: secureStorage.encrypt, decrypt: () => { throw new Error(); } }, getState });
  assert.ok(locked.status().storageError); assert.equal(locked.status().publisher, "off"); locked.close();
  console.log("Integration manager tests passed: encrypted queue/cursors, Live gate, identity binding, replay/dedupe, backoff, publisher exclusivity and bridge auth (mocked network only).");
}
main().catch((error) => { console.error(error); process.exitCode = 1; }).finally(() => {
  manager?.close();
  assert.ok(path.resolve(temporary).startsWith(path.resolve(__dirname) + path.sep));
  fs.rmSync(temporary, { recursive: true, force: true });
});

