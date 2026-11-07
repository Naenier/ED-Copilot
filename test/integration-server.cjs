"use strict";
const fs = require("node:fs");
const path = require("node:path");
const os = require("node:os");
const crypto = require("node:crypto");
const assert = require("node:assert/strict");
const { setTimeout: delay } = require("node:timers/promises");
const { startServer } = require("../lib/server.cjs");
const root = fs.mkdtempSync(path.join(os.tmpdir(), "copilot-server-test-"));
const journalDir = path.join(root, "journals"), dataDir = path.join(root, "profile");
fs.mkdirSync(journalDir); fs.mkdirSync(dataDir);
const stamp = () => new Date().toISOString();
const make = (event, fields = {}) => ({ timestamp: stamp(), event, ...fields });
const journal = path.join(journalDir, "Journal.2026-11-07T180000.01.log");
const events = [make("Fileheader", { gameversion: "4.4.2", build: "test" }),
  make("Commander", { Name: "Тестовый пилот", FID: "FSYNCTEST" }),
  make("LoadGame", { Commander: "Тестовый пилот", FID: "FSYNCTEST", Ship: "python", ShipID: 1, Horizons: true, Odyssey: true, Loan: 0, Credits: 1234 }),
  make("Location", { StarSystem: "Sol", StarPos: [0, 0, 0], SystemAddress: 123, Docked: true, StationName: "Galileo", MarketID: 42 }),
  make("Cargo", { Vessel: "Ship", Inventory: [] })];
fs.writeFileSync(journal, events.map(JSON.stringify).join("\n") + "\n");
const key = crypto.randomBytes(32);
const secureStorage = {};
secureStorage.encrypt = text => {
  const iv = crypto.randomBytes(12), cipher = crypto.createCipheriv("aes-256-gcm", key, iv);
  const data = Buffer.concat([cipher.update(text, "utf8"), cipher.final()]);
  return Buffer.concat([iv, cipher.getAuthTag(), data]);
};
secureStorage.decrypt = bytes => {
  const cipher = crypto.createDecipheriv("aes-256-gcm", key, bytes.subarray(0, 12));
  cipher.setAuthTag(bytes.subarray(12, 28));
  return Buffer.concat([cipher.update(bytes.subarray(28)), cipher.final()]).toString("utf8");
};
async function until(get, label) {
  for (let i = 0; i < 200; i++) { if (get()) return; await delay(30); }
  throw new Error("Timeout: " + label);
}
async function run() {
  const server = await startServer({ dataDir, journalDir, port: 0, secureStorage });
  let submitted = 0;
  server.integrations.transports = { async sendInara(options) { submitted += options.events.length; return { results: options.events.map(() => ({ ok: true })) }; } };
  const request = async (route, body, headers = {}) => {
    const response = await fetch(server.url + route, body === undefined ? { headers } : {
      method: "POST", headers: { "Content-Type": "application/json", ...headers }, body: JSON.stringify(body)
    });
    return { status: response.status, body: await response.json() };
  };
  try {
    await until(() => !server.engine.busy && server.engine.connection.filesRead === 1, "replay");
    let state = server.engine.getState();
    assert.equal(state.game.live, true);
    assert.equal(state.commander.cargoVessel, "ship");
    assert(state.commander.cargoUpdatedAt, "authoritative empty cargo is known");
    assert.equal(state.integrations.enabled, false);
    assert.equal(state.integrations.queued, 0, "Startup history never uploads");
    const configured = await request("/api/integrations/configure", { enabled: true, apiKey: "mock-private-key" });
    assert.equal(configured.status, 200);
    assert.equal(server.integrations.status().apiKeySet, true);
    assert.equal(server.integrations.status().identityMatches, true);
    assert.equal((await request("/api/integrations/configure", { enabled: true }, { Origin: "https://foreign.invalid" })).status, 403);
    assert.equal((await request("/api/frontier/configure", {})).status, 404, "Account registration is not a user-facing configuration endpoint");
    const login = await request("/api/frontier/login", {});
    assert.equal(login.status, 200);
    assert.equal(new URL(login.body.url).searchParams.get("redirect_uri"), "http://localhost:" + new URL(server.url).port + "/api/frontier/callback");
    assert.equal(new URL(login.body.url).searchParams.get("client_id"), "0a4c1d43-e433-411e-bcf5-54527fe5a34f");
    await request("/api/frontier/logout", {});
    const seen = [];
    server.engine.observeJournal((event, meta) => seen.push({ event, meta }));
    const before = fs.statSync(journal).size;
    const line1 = JSON.stringify(make("Music", { MusicTrack: "Тест кириллицы" })) + "\n";
    const line2 = JSON.stringify(make("Music", { MusicTrack: "Вторая запись" })) + "\n";
    fs.appendFileSync(journal, line1 + line2);
    await server.engine.poll();
    assert.equal(seen.length, 2);
    assert.equal(seen[0].meta.offset, before);
    assert.equal(seen[1].meta.offset, before + Buffer.byteLength(line1));
    assert.equal(seen[0].meta.replay, false);
    server.engine.rescanRequested = true; await server.engine.poll();
    assert(seen.slice(2).every(row => row.meta.replay === true));
    state = (await request("/api/state")).body;
    assert.equal(state.integrations.queued, 0, "Replay and unrelated music events create no uploads");
    const missionLine = JSON.stringify(make("MissionAccepted", { MissionID: 77, Name: "Mission_Delivery" })) + "\n";
    fs.appendFileSync(journal, missionLine); await server.engine.poll();
    assert.equal(server.integrations.status().queued, 1);
    assert.equal((await request("/api/integrations/sync", {})).status, 200);
    assert(submitted > 0); assert.equal(server.integrations.status().queued, 0);
    state = (await request("/api/state")).body;
    const serialized = JSON.stringify(state);
    assert(!serialized.includes("mock-private-key"));
    for (const field of ["clientId", "registrationUrl", "redirectUri"]) assert.equal(Object.hasOwn(state.frontier, field), false);
    const stored = fs.readFileSync(path.join(dataDir, "integration-state.bin"));
    assert(!stored.includes(Buffer.from("mock-private-key")));
    assert.equal((await request("/api/integrations/configure", { enabled: false })).status, 200);
    assert.equal(server.integrations.status().enabled, false);
    assert.equal(server.integrations.status().apiKeySet, true);
    console.log("Integration server: Inara-only settings, protected key, synthetic delivery, default Frontier sign-in, Unicode journal offsets and replay boundary passed.");
  } finally { await server.close(); }
}
run().catch(error => { console.error(error); process.exitCode = 1; }).finally(() => {
  if (path.dirname(root) === path.resolve(os.tmpdir()) && path.basename(root).startsWith("copilot-server-test-")) {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

