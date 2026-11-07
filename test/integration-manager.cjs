"use strict";
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { IntegrationManager, IntegrationError } = require("../lib/integrations.cjs");
const { InaraError, eventKey } = require("../lib/inara.cjs");
const temporary = fs.mkdtempSync(path.join(__dirname, "copilot-inara-test-"));
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
let clock = Date.parse("2026-11-07T12:00:00Z"), offset = 0, manager, state, getCalls = 0, sendMode = "ok";
const at = () => new Date(clock).toISOString();
const baseState = () => ({ commander: { name: "Synthetic Commander", fid: "F000000", system: "Test System",
  shipType: "anaconda", shipId: 7, cargo: [], cargoVessel: "ship" },
  game: { version: "4.1.0", live: true, loan: 0 }, connection: {}, app: { version: "1.8.0" }, pilot: {} });
state = baseState();
const getState = () => {
  getCalls++;
  return { ...state, integrations: manager?.status({ commander: state.commander, game: state.game, connection: state.connection }) };
};
const observe = (id, meta = {}, timestamp = at()) => manager.observeJournal({
  event: "MissionAccepted", timestamp, MissionID: id, Name: "Mission_Delivery"
}, { file: "synthetic.journal", offset: ++offset, ...meta });
const calls = [];
const accepted = options => ({ ok: true, results: options.events.map(() => ({ ok: true })) });
const transports = { async sendInara(options) {
  calls.push(options);
  if (sendMode === "network") throw new InaraError("Синтетический сбой", "NETWORK", true, 60000);
  if (sendMode === "partial") return { results: options.events.map((_x, i) => ({ ok: i === 0, retryable: i > 0, message: "Повтор" })) };
  if (sendMode === "provider") return { setupRequired: true, message: "Синтетический отказ сервиса", results: options.events.map(() => ({ ok: false })) };
  return accepted(options);
} };
const create = (dir = "main", extras = {}) => new IntegrationManager({
  dataDir: path.join(temporary, dir), secureStorage, getState, now: () => clock,
  transports, autoDelayMs: 3600000, ...extras
});
const settings = { enabled: true, apiKey: "synthetic-only-api-key" };

async function main() {
  const noStorage = create("no-storage", { secureStorage: {} });
  assert.throws(() => noStorage.configure(settings), e => e instanceof IntegrationError && e.code === "STORAGE_UNAVAILABLE");
  assert.equal(fs.existsSync(noStorage.file), false); noStorage.close();

  // A previous encrypted state is upgraded without losing this service's private data.
  const legacyDir = path.join(temporary, "migration"); fs.mkdirSync(legacyDir);
  const payload = { eventName: "addCommanderMission", eventTimestamp: at(), eventData: { missionGameID: 900, missionName: "Mission_Delivery" } };
  const legacyRow = { service: "inara", identity: { name: state.commander.name, fid: state.commander.fid }, payload,
    key: "inara:" + eventKey(payload, { frontierId: state.commander.fid }), timestamp: at(), attempts: 0, nextAttemptAt: 0 };
  fs.writeFileSync(path.join(legacyDir, "integration-state.bin"), secureStorage.encrypt(JSON.stringify({
    version: 1, config: { publisher: "copilot", inaraEnabled: true, inaraApiKey: settings.apiKey, boundCommander: legacyRow.identity },
    queue: [legacyRow, { ...legacyRow, service: "discarded", key: "discarded:event" }],
    cursors: { "previous.journal": 12345 }, delivered: ["inara:previous", "discarded:previous"],
    services: { inara: { sent: 7, failed: 1, lastSuccess: at() } }, enabledAt: clock - 1000, unusedLegacyData: "discard me"
  })));
  const migration = create("migration");
  assert.equal(migration.config.apiKey, settings.apiKey);
  assert.equal(migration.status().enabled, true);
  assert.equal(migration.status().apiKeySet, true);
  assert.equal(migration.queue.length, 1);
  assert.equal(migration.cursors["previous.journal"], 12345);
  assert.equal(migration.status().sent, 7);
  assert.deepEqual(migration.config.boundCommander, legacyRow.identity);
  const upgraded = JSON.parse(secureStorage.decrypt(fs.readFileSync(migration.file)));
  assert.equal(upgraded.version, 2);
  assert.deepEqual(Object.keys(upgraded.config).sort(), ["apiKey", "boundCommander", "enabled"]);
  assert.equal(upgraded.unusedLegacyData, undefined);
  assert.deepEqual(upgraded.delivered, ["inara:previous"]);
  migration.status(state); await migration.sync();
  assert.equal(migration.status().queued, 0);
  migration.close(); calls.length = 0;

  manager = create();
  const beforeStatus = getCalls; manager.status(state);
  assert.equal(getCalls, beforeStatus, "Status must not recursively build the full engine state");
  manager.configure(settings);
  observe(1, { replay: true });
  observe(2, {}, "2026-11-01T00:00:00Z");
  assert.equal(manager.status().queued, 0);
  const beforeReplay = getCalls;
  for (let i = 0; i < 100; i++) observe(1000 + i, { replay: true });
  assert.equal(getCalls, beforeReplay, "Historical replay never fetches a full renderer state per record");

  observe(3);
  const duplicate = { event: "MissionAccepted", timestamp: at(), MissionID: 3, Name: "Mission_Delivery" };
  manager.observeJournal(duplicate, { file: "synthetic.journal", offset });
  assert.equal(manager.status().queued, 1);
  await manager.sync();
  assert.equal(calls.length, 1); assert.equal(manager.status().queued, 0);
  assert.equal(calls[0].apiKey, settings.apiKey);
  assert.equal(calls[0].events[0].eventName, "addCommanderMission");
  manager.observeJournal(duplicate, { file: "another.journal", offset: 1 });
  assert.equal(manager.status().queued, 0, "Acknowledged events deduplicate across journal files");
  assert(!fs.readFileSync(manager.file).includes(Buffer.from(settings.apiKey)));
  assert(!JSON.stringify(manager.status()).includes(settings.apiKey));

  sendMode = "partial"; observe(4); observe(5); await manager.sync();
  assert.equal(manager.status().queued, 1);
  assert(Date.parse(manager.status().nextRetryAt) >= clock + 60000);
  const count = calls.length; observe(6); await manager.sync();
  assert.equal(calls.length, count, "New events and manual sync respect the service-wide retry pause");
  manager.close(); manager = create(); manager.status(state);
  assert(manager.timer, "Restored queue schedules once current commander/Live context is known");
  assert.equal(manager.status().queued, 2);
  assert.equal(manager.status().apiKeySet, true);
  clock += 61000; sendMode = "ok"; await manager.sync();
  assert.equal(manager.status().queued, 0);

  sendMode = "provider"; observe(7); await manager.sync();
  assert.equal(manager.status().queued, 1, "Provider refusal preserves pending events");
  assert(manager.status().lastError.includes("Синтетический"));
  manager.configure({ apiKey: "synthetic-replacement" }); sendMode = "ok"; await manager.sync();
  assert.equal(manager.status().queued, 0, "Replacing a key clears retry cooldown");

  observe(8); manager.configure({ enabled: false });
  assert.equal(manager.status().queued, 1, "Disabling retains the protected queue");
  const stopped = calls.length; observe(9); await manager.sync(); assert.equal(calls.length, stopped);
  manager.configure({ enabled: true, apiKey: "" });
  assert.equal(manager.config.apiKey, "synthetic-replacement", "Empty field preserves the existing key");
  await manager.sync(); assert.equal(manager.status().queued, 0);

  state.game = { version: "3.8.0", live: false }; observe(10);
  state.game = { version: "4.1 Beta", live: true, isBeta: true }; observe(11);
  assert.equal(manager.status().queued, 0, "Legacy and beta data are never enqueued");
  state = baseState(); observe(12);
  state = { ...state, commander: { name: "Other", fid: "FOTHER" } }; manager.status(state);
  const beforeMismatch = calls.length; await manager.sync();
  assert.equal(calls.length, beforeMismatch);
  assert.equal(manager.status().identityMatches, false);
  assert.throws(() => manager.configure({ enabled: true }), e => e.code === "COMMANDER_MISMATCH");
  state = baseState(); manager.status(state); await manager.sync();

  manager.observeCapi({ metadata: { source: "Frontier CAPI", observedAt: { profile: at() } },
    data: { profile: { commander: { name: state.commander.name, credits: 5000, debt: 20 } } } });
  await manager.sync();
  assert.equal(calls.at(-1).events[0].eventData.commanderLoan, 20);

  // A late acknowledgement must not delete an event after the commander changed.
  observe(13);
  manager.transports = { async sendInara(options) {
    state = { ...state, commander: { name: "Other", fid: "FOTHER" } };
    manager.status(state); assert.equal(options.signal.aborted, true);
    return accepted(options);
  } };
  await manager.sync(); assert.equal(manager.status().queued, 1);
  state = baseState(); manager.status(state);
  let aborted = false;
  manager.transports = { sendInara: options => new Promise((_resolve, reject) => options.signal.addEventListener("abort", () => {
    aborted = true; reject(new InaraError("Отменено", "ABORTED", false));
  }, { once: true })) };
  const draining = manager.sync(); await Promise.resolve();
  manager.configure({ enabled: false }); await draining;
  assert.equal(aborted, true); assert.equal(manager.status().queued, 1);
  manager.close();

  const locked = create("main", { secureStorage: { encrypt: secureStorage.encrypt, decrypt() { throw Error("locked"); } } });
  assert(locked.status().storageError); assert.equal(locked.status().enabled, false); locked.close();
  console.log("Inara manager: protected v1 migration, private key, replay/dedupe, commander/Live gates, partial acknowledgements, backoff, pause/restart and cancellation passed (mock transports).");
}
main().catch(error => { console.error(error); process.exitCode = 1; }).finally(() => {
  manager?.close();
  assert(path.dirname(temporary) === path.resolve(__dirname) && path.basename(temporary).startsWith("copilot-inara-test-"));
  fs.rmSync(temporary, { recursive: true, force: true });
});
