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
    assert.equal(state.integrations.publisher, "off");
    assert.equal(state.integrations.queue.total, 0, "startup history never uploads");
    const configured = await request("/api/integrations/configure", { publisher: "edmc", inaraApiKey: "mock-private-key" });
    assert.equal(configured.status, 200);
    const pair = await request("/api/integrations/pair", {});
    assert.equal(pair.status, 200); assert(pair.body.token.length >= 32);
    assert.equal(pair.body.url, server.url + "/api/integrations/edmc");
    const token = pair.body.token;
    const headers = { "X-EDCopilot-Token": token };
    const heartbeat = { type: "heartbeat", commander: "Тестовый пилот", isBeta: false, pluginVersion: "1.0.0" };
    assert.equal((await request("/api/integrations/edmc", heartbeat)).status, 401);
    assert.equal((await request("/api/integrations/edmc", heartbeat, { ...headers, Origin: server.url })).status, 403);
    assert.equal((await request("/api/integrations/edmc", heartbeat, { ...headers, Origin: "https://foreign.invalid" })).status, 403);
    assert.equal((await request("/api/integrations/edmc", heartbeat, headers)).status, 200);
    const publicMarket = { ...heartbeat, type: "capi", observedAt: stamp(), data: {
      commander: { name: "Тестовый пилот", docked: true }, lastSystem: { name: "Sol" },
      lastStarport: { id: 42, name: "Galileo", commodities: [{ name: "gold", buyPrice: 40000, sellPrice: 50000,
        stock: 20, demand: 2000, stockBracket: 2, demandBracket: 3 }] }
    } };
    assert.equal((await request("/api/integrations/edmc", publicMarket, headers)).status, 200);
    assert.equal(server.engine.market.source, "EDMC · Frontier CAPI");
    assert.equal(server.engine.prices.get("gold").price, 50000);
    const wrongMarket = structuredClone(publicMarket); wrongMarket.data.lastStarport.id = 999;
    wrongMarket.data.lastStarport.commodities[0].sellPrice = 999999;
    await request("/api/integrations/edmc", wrongMarket, headers);
    assert.equal(server.engine.prices.get("gold").price, 50000, "different station rejected");
    const stale = structuredClone(publicMarket); stale.observedAt = new Date(Date.now() - 600000).toISOString();
    stale.data.lastStarport.commodities[0].sellPrice = 999999;
    await request("/api/integrations/edmc", stale, headers);
    assert.equal(server.engine.prices.get("gold").price, 50000, "stale EDMC market rejected");
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
    assert.equal(state.integrations.queue.total, 0, "EDMC mode never republishes");
    const serialized = JSON.stringify(state);
    assert(!serialized.includes(token) && !serialized.includes("mock-private-key"));
    const stored = fs.readFileSync(path.join(dataDir, "integration-state.bin"));
    assert(!stored.includes(Buffer.from("mock-private-key")) && !stored.includes(Buffer.from(token)));
    const next = await request("/api/integrations/pair", {});
    assert.notEqual(next.body.token, token);
    assert.equal((await request("/api/integrations/edmc", heartbeat, headers)).status, 401);
    console.log("Integration server: loopback pairing, encryption, privacy, EDMC market validation, Unicode cursors and replay boundary passed.");
  } finally { await server.close(); }
}
run().catch(error => { console.error(error); process.exitCode = 1; }).finally(() => {
  if (path.dirname(root) === path.resolve(os.tmpdir()) && path.basename(root).startsWith("copilot-server-test-")) {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

