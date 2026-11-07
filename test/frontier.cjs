"use strict";
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { FrontierClient, FrontierError, summarise, marketSnapshot, DEFAULT_REDIRECT } = require("../lib/frontier.cjs");

const temporary = fs.mkdtempSync(path.join(__dirname, "frontier-test-"));
const key = crypto.randomBytes(32);
const encrypt = (value) => {
  const iv = crypto.randomBytes(12), cipher = crypto.createCipheriv("aes-256-gcm", key, iv);
  const encrypted = Buffer.concat([cipher.update(value, "utf8"), cipher.final()]);
  return Buffer.concat([iv, cipher.getAuthTag(), encrypted]);
};
const decrypt = (value) => {
  const cipher = crypto.createDecipheriv("aes-256-gcm", key, value.subarray(0, 12));
  cipher.setAuthTag(value.subarray(12, 28));
  return Buffer.concat([cipher.update(value.subarray(28)), cipher.final()]).toString("utf8");
};
const reply = (data, status = 200) => ({ ok: status >= 200 && status < 300, status,
  headers: new Headers(), text: async () => JSON.stringify(data) });
const rejected = (promise, code) => assert.rejects(promise, (e) => e instanceof FrontierError && e.code === code);

async function main() {
  let now = 1700000000000, refreshCount = 0, generation = 1, expected = { fid: "F123", name: "Test Pilot" };
  let callbackBody, transient401 = false, badStation = false, decodeFid = 123, profileCalls = 0, carrierCalls = 0;
  const calls = [];
  const fetch = async (url, options) => {
    calls.push({ url, options });
    const parsed = new URL(url), route = parsed.pathname;
    if (route === "/token") {
      const body = new URLSearchParams(options.body);
      if (body.get("grant_type") === "authorization_code") {
        callbackBody = body;
        return reply({ access_token: "access-secret-1", refresh_token: "refresh-secret-1", expires_in: 3600, token_type: "Bearer" });
      }
      refreshCount++; generation++;
      await new Promise((resolve) => setImmediate(resolve));
      return reply({ access_token: `access-secret-${generation}`, refresh_token: `refresh-secret-${generation}`, expires_in: 3600 });
    }
    if (route === "/decode") return reply({ usr: { customer_id: decodeFid } });
    if (transient401 && options.headers.Authorization === "Bearer access-secret-1") return reply({ access_token: "must-not-surface" }, 401);
    if (route === "/profile") { profileCalls++; return reply({ commander: { name: "Test Pilot", credits: 123456, docked: true },
      lastSystem: { name: "Sol" }, lastStarport: { id: 100, name: "Galileo" }, ship: { shipName: "Miner" } }); }
    if (route === "/market" || route === "/shipyard") return reply({ id: badStation ? 999 : 100, name: "Galileo", commodities: [] });
    if (route === "/fleetcarrier") { carrierCalls++; return reply({ name: { callsign: "ABC-123", vanityName: Buffer.from("Шахтёр").toString("hex") },
      currentStarSystem: "Sol", fuel: 700, balance: 800000000, finance: { bankReservedBalance: 40000000, maintenance: 6000000 },
      capacity: { freeSpace: 19000, cargoForSale: 1000, cargoNotForSale: 4000 },
      orders: { commodities: { purchases: [{ name: "Tritium", total: 200, outstanding: 150, price: 50000 }] } } }); }
    throw new Error("Unexpected URL");
  };
  const options = { dataDir: temporary, fetch, encrypt, decrypt, now: () => now, getCommander: () => expected };
  const client = new FrontierClient(options);
  assert.equal(client.status().configured, false);
  assert.equal(DEFAULT_REDIRECT, "http://localhost:7122/api/frontier/callback");
  assert.equal(client.status().redirectUri, DEFAULT_REDIRECT);
  assert.equal(client.status().authorizationFlow, "native-pkce");
  assert.equal(client.status().redirectTransport, "loopback-http");
  assert.throws(() => new FrontierClient({ ...options, redirectUri: "https://localhost:7122/api/frontier/callback" }), (e) => e.code === "REDIRECT_INVALID",
    "The native listener is HTTP; a fake HTTPS callback would fail after login");
  assert.throws(() => new FrontierClient({ ...options, redirectUri: "http://external.invalid:7122/api/frontier/callback" }), (e) => e.code === "REDIRECT_INVALID");
  assert.throws(() => new FrontierClient({ ...options, redirectUri: "http://localhost:7122/api/frontier/callback?code=old" }), (e) => e.code === "REDIRECT_INVALID");
  const alternate = new FrontierClient({ ...options, redirectUri: "http://localhost:7133/api/frontier/callback" });
  assert.equal(alternate.status().redirectUri, "http://localhost:7133/api/frontier/callback");
  assert.throws(() => client.startLogin(), (e) => e.code === "CLIENT_REQUIRED");
  client.configure({ clientId: "edmining-personal-test-client" });
  const insecure = new FrontierClient({ dataDir: path.join(temporary, "insecure") });
  insecure.configure({ clientId: "test-client" });
  assert.throws(() => insecure.startLogin(), (e) => e.code === "SECURE_STORAGE_REQUIRED");

  const auth = new URL(client.startLogin().url);
  assert.equal(auth.origin, "https://auth.frontierstore.net");
  assert.equal(auth.pathname, "/auth");
  assert.equal(auth.searchParams.get("scope"), "auth capi");
  assert.equal(auth.searchParams.get("code_challenge_method"), "S256");
  assert.equal(auth.searchParams.get("redirect_uri"), "http://localhost:7122/api/frontier/callback");
  assert.ok(auth.searchParams.get("state").length >= 32);
  await rejected(client.callback({ state: "wrong", code: "code-secret" }), "STATE_INVALID");
  await client.callback({ state: auth.searchParams.get("state"), code: "code-secret" });
  assert.equal(callbackBody.get("code"), "code-secret");
  assert.equal(callbackBody.get("redirect_uri"), auth.searchParams.get("redirect_uri"));
  assert.equal(crypto.createHash("sha256").update(callbackBody.get("code_verifier")).digest("base64url"), auth.searchParams.get("code_challenge"));
  await rejected(client.callback({ state: auth.searchParams.get("state"), code: "code-secret" }), "STATE_INVALID");
  assert.equal(client.status().connected, true);
  assert.equal(client.status().fid, "F123");
  assert.ok(!JSON.stringify(client.status()).includes("secret"));
  assert.ok(!fs.readFileSync(client.tokenFile).toString("utf8").includes("secret"));
  const restored = new FrontierClient(options);
  assert.equal(restored.status().connected, true);
  assert.equal(restored.status().fid, "F123");

  // Concurrent endpoint 401s cause exactly one rotating refresh, never token disclosure.
  transient401 = true;
  await Promise.all([client.capi("profile"), client.capi("market"), client.capi("shipyard")]);
  assert.equal(refreshCount, 1);
  assert.equal(JSON.parse(decrypt(fs.readFileSync(client.tokenFile))).refreshToken, "refresh-secret-2");
  transient401 = false;
  const result = await client.sync();
  assert.equal(result.status.summary.carrier.name, "Шахтёр");
  assert.equal(result.status.summary.carrier.fuel, 700);
  assert.equal(result.status.summary.carrier.usedCapacity, 6000);
  assert.equal(result.status.summary.profile.credits, 123456);
  assert.equal(result.status.summary.carrier.purchases[0].outstanding, 150);
  assert.equal(result.data.market.id, 100);
  assert.equal(result.metadata.source, "Frontier CAPI");
  assert.equal(result.metadata.observedAt.market, new Date(now).toISOString());
  const validCommodity = { name: "Gold", locName: "Золото", buyPrice: 100, sellPrice: 90, stock: 50, demand: 70,
    stockBracket: 2, demandBracket: 2, legality: "" };
  const marketResult = { ...result, data: { ...result.data,
    market: { ...result.data.market, updatedAt: "2000-01-01T00:00:00Z", commodities: [validCommodity,
      { ...validCommodity, name: "Silver", demandBracket: 0 },
      { ...validCommodity, name: "Tritium", stockBracket: 0 },
      { ...validCommodity, name: "BadNumeric", sellPrice: "999999" },
      { ...validCommodity, name: "Illegal", legality: "illegal" },
      { ...validCommodity, name: "NegativeStock", stock: -1 },
      { ...validCommodity, name: "UnknownBracket", stockBracket: 99 },
      { ...validCommodity, name: "Unavailable", stockBracket: 0, demandBracket: 0 },
      { ...validCommodity, sellPrice: 999999 }, null
    ] } } };
  const localMarket = marketSnapshot(marketResult);
  assert.equal(localMarket.Source, "Frontier CAPI");
  assert.equal(localMarket.timestamp, result.metadata.observedAt.market, "Observation time comes from the actual fetch, not arbitrary station fields");
  assert.equal(localMarket.MarketID, 100);
  assert.equal(localMarket.StarSystem, "Sol");
  assert.equal(localMarket.StationName, "Galileo");
  assert.equal(localMarket.Items.length, 3);
  assert.deepEqual(localMarket.Items[0], { Name: "gold", Name_Localised: "Золото", BuyPrice: 100, SellPrice: 90, Stock: 50, Demand: 70 });
  assert.equal(localMarket.Items[1].Demand, 0);
  assert.equal(localMarket.Items[2].Stock, 0);
  assert.equal(marketSnapshot({ ...marketResult, status: { ...result.status, commanderMismatch: true } }), null);
  assert.equal(marketSnapshot({ ...marketResult, status: { ...result.status, commander: "Wrong commander" } }), null);
  assert.equal(marketSnapshot({ ...marketResult, metadata: {} }), null);
  assert.equal(marketSnapshot({ ...marketResult, data: { ...marketResult.data,
    market: { ...marketResult.data.market, id: 999 } } }), null);
  assert.equal(marketSnapshot({ ...marketResult, data: { ...marketResult.data,
    profile: { ...result.data.profile, commander: { ...result.data.profile.commander, docked: false } } } }), null);
  const counted = [profileCalls, carrierCalls];
  const cached = await client.sync();
  assert.deepEqual([profileCalls, carrierCalls], counted);
  assert.equal(marketSnapshot(cached), null, "Cooldown cannot relabel previous prices as a fresh observation");
  now += 61000; badStation = true;
  const inconsistent = await client.sync();
  assert.ok(inconsistent.errors.market.includes("другой станции"));
  assert.equal(inconsistent.data.market, undefined);
  assert.equal(carrierCalls, counted[1]);
  assert.ok(calls.filter((c) => new URL(c.url).host === "companion.orerve.net").every((c) => new URL(c.url).searchParams.get("language") === "ru"));

  // Switching the in-game commander hides cached data and blocks server queries.
  expected = { fid: "F999", name: "Other" };
  assert.equal(client.status().commanderMismatch, true);
  assert.deepEqual(client.status().summary, {});
  await rejected(client.sync(), "COMMANDER_MISMATCH");
  expected = { fid: "F123", name: "Test Pilot" };
  const denied = new URL(client.startLogin().url);
  await rejected(client.callback({ state: denied.searchParams.get("state"), error: "denied refresh-secret" }), "ACCESS_DENIED");
  assert.ok(!client.status().error.includes("secret"));
  const wrongAccount = new URL(client.startLogin().url); decodeFid = 999;
  await rejected(client.callback({ state: wrongAccount.searchParams.get("state"), code: "code" }), "COMMANDER_MISMATCH");
  assert.equal(client.status().fid, "F123");
  decodeFid = 123;
  const expired = new URL(client.startLogin().url); now += 600001;
  await rejected(client.callback({ state: expired.searchParams.get("state"), code: "code" }), "STATE_INVALID");
  client.logout();
  assert.equal(client.status().connected, false);
  assert.equal(fs.existsSync(client.tokenFile), false);
  assert.equal(new FrontierClient(options).status().connected, false);

  // An in-flight refresh must not recreate credentials after logout.
  const again = new URL(client.startLogin().url);
  await client.callback({ state: again.searchParams.get("state"), code: "code" });
  const pendingRefresh = client.refresh();
  client.logout();
  await rejected(pendingRefresh, "CANCELLED");
  assert.equal(fs.existsSync(client.tokenFile), false);
  assert.deepEqual(summarise(undefined, null).carrier, { owned: false });
  const grouped = summarise(undefined, { name: { callsign: "ABC-123" }, cargo: [
    { commodity: "Tritium", qty: 1 }, { commodity: "Tritium", qty: 1 },
    { commodity: "Tritium", qty: 1, stolen: true }, { commodity: "Tritium", qty: 1, mission: true }
  ] }).carrier.cargo;
  assert.equal(grouped.length, 3);
  assert.equal(grouped[0].count, 2);
  assert.equal(grouped[1].stolen, true);
  assert.equal(grouped[2].mission, true);

  // Provider error bodies are never forwarded to the local API.
  const errorClient = new FrontierClient({ ...options, dataDir: path.join(temporary, "errors"),
    fetch: async () => reply({ error: "token-supersecret" }, 400) });
  errorClient.configure({ clientId: "test-client" });
  const failing = new URL(errorClient.startLogin().url);
  await assert.rejects(errorClient.callback({ state: failing.searchParams.get("state"), code: "code" }), (e) => !e.message.includes("supersecret"));
  assert.ok(!JSON.stringify(errorClient.status()).includes("supersecret"));
  console.log("Frontier: native localhost callback and mocked PKCE roundtrip, state/replay/expiry, encrypted persistence, identity, rotating concurrent refresh, CAPI station consistency/cooldowns, logout races: OK");
}
main().then(() => {
  // Resolve and verify the generated test directory before recursive removal.
  if (path.dirname(path.resolve(temporary)) !== path.resolve(__dirname) || !path.basename(temporary).startsWith("frontier-test-")) throw new Error("Unexpected test directory");
  fs.rmSync(temporary, { recursive: true });
}).catch((problem) => { console.error(problem); process.exitCode = 1; });
