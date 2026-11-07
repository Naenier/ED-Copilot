"use strict";

// Public native OAuth client: register a dedicated Client ID with Frontier.
// Protocol references: https://github.com/EDCD/FDevIDs/tree/master/Frontier%20API
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");

const AUTH = "https://auth.frontierstore.net";
const CAPI = "https://companion.orerve.net";
const REGISTER = "https://auth.frontierstore.net/client/signup";
// A native PKCE client returns through loopback HTTP, as EDMC does. This is
// separate from the HTTPS-only endpoint list shown for the registered client.
// There is no TLS listener here: changing this to https would break the return.
const DEFAULT_REDIRECT = "http://localhost:7122/api/frontier/callback";
const MAX_JSON_BYTES = 16 * 1024 * 1024;

class FrontierError extends Error {
  constructor(message, code = "FRONTIER_ERROR", status = 400) {
    super(message); this.name = "FrontierError"; this.code = code; this.status = status;
  }
}
function error(message, code, status) { return new FrontierError(message, code, status); }
function object(value) { return value && typeof value === "object" && !Array.isArray(value) ? value : {}; }
function array(value) { return Array.isArray(value) ? value : Object.values(object(value)); }
function number(value) { return Number.isFinite(Number(value)) && value !== null && value !== "" ? Number(value) : null; }
function text(value, length = 180) { return typeof value === "string" ? value.slice(0, length) : ""; }
function atomicWrite(file, value) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  const temporary = file + "." + crypto.randomBytes(8).toString("hex") + ".tmp";
  try { fs.writeFileSync(temporary, value, { mode: 0o600 }); fs.renameSync(temporary, file); }
  finally { try { fs.unlinkSync(temporary); } catch {} }
}
function redirectAddress(value) {
  let url;
  try { url = new URL(value); } catch { throw error("Некорректный адрес возврата Frontier.", "REDIRECT_INVALID"); }
  if (url.protocol !== "http:" || !["localhost", "127.0.0.1", "[::1]"].includes(url.hostname) ||
      url.username || url.password || url.search || url.hash || !url.port) {
    throw error("Для Frontier требуется адрес возврата HTTP на localhost с портом.", "REDIRECT_INVALID");
  }
  return url.href;
}
function carrierName(value) {
  const raw = text(value, 512);
  if (raw && raw.length % 2 === 0 && /^[0-9a-f]+$/i.test(raw)) {
    return Buffer.from(raw, "hex").toString("utf8").replace(/[\u0000-\u001f]/g, "").slice(0, 180);
  }
  return raw.slice(0, 180);
}
function carrierCargo(value) {
  // CAPI can return one record per tonne. Group like cargo without mixing legal
  // and stolen stock or mission cargo, so large holds remain readable.
  const groups = new Map();
  for (const row of array(value)) {
    const name = text(row.commodity), count = number(row.qty) || 0;
    if (!name || count <= 0) continue;
    const stolen = row.stolen === true, mission = row.mission === true;
    const key = JSON.stringify([name.toLowerCase(), stolen, mission]);
    if (!groups.has(key)) groups.set(key, { name, localized: text(row.locName), count: 0, stolen, mission });
    groups.get(key).count += count;
  }
  return [...groups.values()];
}
function summarise(profile, carrier, previous = {}) {
  const cmdr = object(profile?.commander), ship = object(profile?.ship);
  const result = { ...previous };
  if (profile) result.profile = {
    commander: text(cmdr.name), credits: number(cmdr.credits), debt: number(cmdr.debt),
    docked: cmdr.docked === true, system: text(profile.lastSystem?.name),
    station: text(profile.lastStarport?.name), marketId: number(profile.lastStarport?.id),
    ship: text(ship.shipName || ship.name), shipId: text(ship.shipID),
    cargoCount: array(ship.cargo).reduce((sum, row) => sum + (number(row.qty ?? row.quantity) || 0), 0)
  };
  if (carrier === null) result.carrier = { owned: false };
  else if (carrier) {
    const names = object(carrier.name), capacity = object(carrier.capacity), finance = object(carrier.finance);
    const orders = object(carrier.orders?.commodities);
    result.carrier = {
      owned: true, callsign: text(names.callsign),
      name: carrierName(names.filteredVanityName || names.vanityName || names.callsign),
      system: text(typeof carrier.currentStarSystem === "string" ? carrier.currentStarSystem : carrier.currentStarSystem?.name),
      fuel: number(carrier.fuel), balance: number(finance.bankBalance ?? carrier.balance),
      reservedBalance: number(finance.bankReservedBalance), maintenance: number(finance.maintenance),
      coreCost: number(finance.coreCost), servicesCost: number(finance.servicesCost),
      jumpCost: number(finance.jumpsCost), state: text(carrier.state),
      dockingAccess: text(carrier.dockingAccess), notoriousAccess: carrier.notoriousAccess === true,
      freeSpace: number(capacity.freeSpace), crewSpace: number(capacity.crew),
      usedCapacity: number(capacity.freeSpace) === null ? null : 25000 - Number(capacity.freeSpace),
      shipPackSpace: number(capacity.shipPacks), modulePackSpace: number(capacity.modulePacks),
      cargoSpace: (number(capacity.cargoForSale) || 0) + (number(capacity.cargoNotForSale) || 0),
      reservedSpace: number(capacity.cargoSpaceReserved),
      cargo: carrierCargo(carrier.cargo),
      sales: array(orders.sales).map((row) => ({ name: text(row.name), count: number(row.stock),
        price: number(row.price), blackmarket: row.blackmarket === true })),
      purchases: array(orders.purchases).map((row) => ({ name: text(row.name), total: number(row.total),
        outstanding: number(row.outstanding), price: number(row.price) })),
      services: Object.entries(object(carrier.market?.services)).map(([name, status]) => ({ name: text(name), status: text(status) })),
      currentJump: text(carrier.itinerary?.currentJump), totalDistance: number(carrier.itinerary?.totalDistanceJumpedLY)
    };
  }
  return result;
}

function marketSnapshot(syncResult) {
  // /market is the last docked market. Bind it to the same verified docked
  // profile instead of attaching an old station's prices to the current system.
  const status = object(syncResult?.status), profile = object(syncResult?.data?.profile);
  const market = object(syncResult?.data?.market), commander = object(profile.commander);
  const marketId = number(market.id), profileId = number(profile.lastStarport?.id);
  const observedAt = syncResult?.metadata?.observedAt?.market;
  if (status.connected !== true || status.commanderMismatch || commander.docked !== true ||
    !text(commander.name) || text(commander.name).toLowerCase() !== text(status.commander).toLowerCase() ||
    !Number.isSafeInteger(marketId) || marketId <= 0 || marketId !== profileId ||
    !text(market.name) || !text(profile.lastSystem?.name) || !Array.isArray(market.commodities) ||
    syncResult?.metadata?.source !== "Frontier CAPI" || typeof observedAt !== "string" || !Number.isFinite(Date.parse(observedAt))) return null;
  const Items = [], seen = new Set();
  for (const candidate of market.commodities) {
    const row = object(candidate), name = text(row.name).trim().toLowerCase();
    const fields = [row.buyPrice, row.sellPrice, row.stock, row.demand];
    if (!name || seen.has(name) || row.legality ||
      !fields.every((value) => typeof value === "number" && Number.isFinite(value) && value >= 0) ||
      !Number.isSafeInteger(row.stock) || !Number.isSafeInteger(row.demand) ||
      ![0, 1, 2, 3].includes(row.stockBracket) || ![0, 1, 2, 3].includes(row.demandBracket)) continue;
    const stock = row.stockBracket === 0 ? 0 : row.stock;
    const demand = row.demandBracket === 0 ? 0 : row.demand;
    if ((!stock || !row.buyPrice) && (!demand || !row.sellPrice)) continue;
    seen.add(name);
    Items.push({ Name: name, Name_Localised: text(row.locName), BuyPrice: row.buyPrice,
      SellPrice: row.sellPrice, Stock: stock, Demand: demand });
  }
  if (!Items.length) return null;
  return { event: "Market", timestamp: new Date(observedAt).toISOString(), MarketID: marketId,
    StationName: text(market.name), StarSystem: text(profile.lastSystem.name), Items, Source: "Frontier CAPI" };
}

class FrontierClient {
  constructor({ dataDir, redirectUri = DEFAULT_REDIRECT, fetch: request = globalThis.fetch,
    encrypt, decrypt, getCommander = () => ({}), onUpdate = () => {}, now = Date.now } = {}) {
    if (!dataDir) throw new Error("Frontier dataDir is required");
    this.configFile = path.join(dataDir, "frontier-settings.json");
    this.tokenFile = path.join(dataDir, "frontier-tokens.enc");
    this.redirectUri = redirectAddress(redirectUri);
    this.request = request; this.encrypt = encrypt; this.decrypt = decrypt;
    this.getCommander = getCommander; this.onUpdate = onUpdate; this.now = now;
    this.config = { clientId: "" }; this.tokens = null; this.pending = null;
    this.identity = null; this.summary = {}; this.error = "";
    this.lastSync = null; this.lastCarrierSync = null; this.refreshing = null; this.syncing = null;
    this.epoch = 0; this.endpointTimes = {};
    try {
      const saved = JSON.parse(fs.readFileSync(this.configFile, "utf8"));
      if (typeof saved.clientId === "string" && /^[A-Za-z0-9._-]{8,160}$/.test(saved.clientId)) this.config.clientId = saved.clientId;
    } catch {}
    if (this.storageAvailable() && this.config.clientId && fs.existsSync(this.tokenFile)) {
      try {
        const saved = JSON.parse(this.decrypt(fs.readFileSync(this.tokenFile)));
        if (saved.clientId === this.config.clientId && typeof saved.refreshToken === "string") {
          this.tokens = saved; this.identity = object(saved.identity);
        }
      } catch { this.error = "Не удалось открыть сохранённый вход Frontier. Войдите заново."; }
    }
  }
  storageAvailable() { return typeof this.encrypt === "function" && typeof this.decrypt === "function"; }
  status() {
    let commanderMismatch = false;
    try { if (this.identity) this.assertCommander(this.identity); } catch { commanderMismatch = true; }
    return { configured: !!this.config.clientId, clientId: this.config.clientId, redirectUri: this.redirectUri,
      authorizationFlow: "native-pkce", redirectTransport: "loopback-http",
      registrationUrl: REGISTER, storageAvailable: this.storageAvailable(), connected: !!this.tokens,
      pending: !!this.pending && this.pending.expires > this.now(), syncing: !!this.syncing,
      commander: this.identity?.name || "", fid: this.identity?.fid || "", lastSync: this.lastSync,
      lastCarrierSync: this.lastCarrierSync, nextSyncAt: this.endpointTimes.profile ? this.endpointTimes.profile + 60000 : null,
      nextCarrierSyncAt: this.endpointTimes.carrier ? this.endpointTimes.carrier + 900000 : null,
      commanderMismatch, error: this.error, summary: commanderMismatch ? {} : structuredClone(this.summary) };
  }
  emit() { this.onUpdate(this.status()); }
  configure({ clientId } = {}) {
    const id = String(clientId || "").trim();
    if (id && !/^[A-Za-z0-9._-]{8,160}$/.test(id)) throw error("Некорректный Client ID Frontier.", "CLIENT_INVALID");
    if (id !== this.config.clientId) this.logout();
    this.config = { clientId: id };
    atomicWrite(this.configFile, JSON.stringify(this.config, null, 2));
    this.error = ""; this.emit(); return this.status();
  }
  startLogin() {
    if (!this.config.clientId) throw error("Укажите собственный Client ID, зарегистрированный для ED Copilot в Frontier Developer Zone.", "CLIENT_REQUIRED");
    if (!this.storageAvailable()) throw error("Защищённое хранилище Windows недоступно. Вход Frontier доступен в настольном приложении.", "SECURE_STORAGE_REQUIRED");
    this.epoch += 1;
    const verifier = crypto.randomBytes(48).toString("base64url"), state = crypto.randomBytes(32).toString("base64url");
    const expected = object(this.getCommander());
    this.pending = { verifier, state, expires: this.now() + 600000,
      expectedFid: text(expected.fid), expectedName: text(expected.name), epoch: this.epoch };
    const url = new URL(AUTH + "/auth");
    url.search = new URLSearchParams({ response_type: "code", audience: "frontier,steam,epic", scope: "auth capi",
      client_id: this.config.clientId, redirect_uri: this.redirectUri, state,
      code_challenge: crypto.createHash("sha256").update(verifier).digest("base64url"), code_challenge_method: "S256" }).toString();
    this.error = ""; this.emit(); return { url: url.href, status: this.status() };
  }
  async jsonRequest(url, options = {}, timeout = 30000) {
    let response;
    try { response = await this.request(url, { ...options, redirect: "error", signal: AbortSignal.timeout(timeout) }); }
    catch { throw error("Frontier не отвечает. Повторите попытку позднее.", "NETWORK", 502); }
    if (response.status === 204) return { response, data: null };
    if (!response.ok) {
      // Do not include raw provider responses or request bodies: these may carry credentials.
      const message = response.status === 401 ? "Срок входа Frontier истёк. Войдите заново." :
        response.status === 429 ? "Frontier ограничил частоту запросов. Повторите попытку позднее." :
          `Frontier вернул ошибку HTTP ${response.status}.`;
      throw error(message, response.status === 401 ? "UNAUTHORIZED" : response.status === 429 ? "RATE_LIMITED" : "HTTP_ERROR", response.status);
    }
    let data;
    try {
      if (Number(response.headers?.get("content-length")) > MAX_JSON_BYTES) throw new Error();
      const raw = await response.text();
      if (Buffer.byteLength(raw) > MAX_JSON_BYTES) throw new Error();
      data = JSON.parse(raw);
      if (!data || typeof data !== "object" || Array.isArray(data)) throw new Error();
    } catch { throw error("Некорректный ответ Frontier.", "INVALID_RESPONSE", 502); }
    return { response, data };
  }
  async tokenRequest(fields) {
    const { data } = await this.jsonRequest(AUTH + "/token", { method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json" },
      body: new URLSearchParams({ ...fields, client_id: this.config.clientId }).toString() });
    if (!data || typeof data.access_token !== "string" || !data.access_token ||
      (data.token_type && String(data.token_type).toLowerCase() !== "bearer")) {
      throw error("Frontier не выдал токен доступа.", "TOKEN_INVALID", 502);
    }
    return { accessToken: data.access_token, refreshToken: typeof data.refresh_token === "string" ? data.refresh_token :
      (fields.grant_type === "refresh_token" ? fields.refresh_token : undefined),
      expires: this.now() + Math.max(1, number(data.expires_in) || 3600) * 1000, clientId: this.config.clientId };
  }
  async decode(tokens) {
    const { data } = await this.jsonRequest(AUTH + "/decode", { headers: { Authorization: "Bearer " + tokens.accessToken, Accept: "application/json" } });
    const customer = data?.usr?.customer_id;
    if (customer === undefined || customer === null || !/^\d+$/.test(String(customer))) {
      throw error("Frontier не подтвердил владельца аккаунта.", "IDENTITY_INVALID", 502);
    }
    return { fid: "F" + customer, name: "" };
  }
  assertCommander(identity, expected = this.getCommander()) {
    const current = object(expected);
    if ((current.fid && identity.fid !== current.fid) ||
      (!current.fid && current.name && identity.name && current.name.toLowerCase() !== identity.name.toLowerCase())) {
      throw error("Аккаунт Frontier не совпадает с командиром в журнале игры. Войдите в нужный аккаунт.", "COMMANDER_MISMATCH");
    }
  }
  persist(tokens, identity = this.identity) {
    if (!tokens.refreshToken) throw error("Frontier не выдал токен продолжения входа. Авторизуйтесь повторно.", "REFRESH_REQUIRED");
    try {
      const encrypted = this.encrypt(JSON.stringify({ ...tokens, identity }));
      if (!Buffer.isBuffer(encrypted) || !encrypted.length) throw new Error();
      atomicWrite(this.tokenFile, encrypted);
    } catch { throw error("Не удалось безопасно сохранить вход Frontier.", "SECURE_STORAGE_FAILED", 500); }
  }
  async callback(parameters) {
    const params = parameters instanceof URLSearchParams ? parameters : new URLSearchParams(parameters);
    const pending = this.pending;
    if (!pending || pending.expires <= this.now() || params.getAll("state").length !== 1 || params.get("state") !== pending.state) {
      throw error("Ответ входа Frontier устарел или не относится к текущему запросу.", "STATE_INVALID");
    }
    this.pending = null;
    try {
      if (params.has("error")) throw error("Вход Frontier отменён или не разрешён.", "ACCESS_DENIED");
      if (params.getAll("code").length !== 1 || !params.get("code") || params.get("code").length > 8192) throw error("Frontier не вернул код авторизации.", "CODE_REQUIRED");
      const tokens = await this.tokenRequest({ grant_type: "authorization_code", code: params.get("code"),
        code_verifier: pending.verifier, redirect_uri: this.redirectUri });
      const identity = await this.decode(tokens);
      this.assertCommander(identity, { fid: pending.expectedFid, name: pending.expectedName });
      this.assertCommander(identity);
      if (pending.epoch !== this.epoch) throw error("Вход Frontier отменён.", "CANCELLED");
      this.persist(tokens, identity); this.tokens = tokens; this.identity = identity;
      this.summary = {}; this.endpointTimes = {}; this.lastSync = null; this.lastCarrierSync = null;
      this.error = ""; this.emit(); return this.status();
    } catch (problem) { this.error = problem instanceof FrontierError ? problem.message : "Не удалось выполнить вход Frontier."; this.emit(); throw problem; }
  }
  async refresh() {
    if (this.refreshing) return this.refreshing;
    if (!this.tokens?.refreshToken) throw error("Сначала войдите в Frontier.", "LOGIN_REQUIRED", 401);
    const epoch = this.epoch;
    this.refreshing = (async () => {
      try {
        const tokens = await this.tokenRequest({ grant_type: "refresh_token", refresh_token: this.tokens.refreshToken });
        if (epoch !== this.epoch) throw error("Вход Frontier отменён.", "CANCELLED");
        this.persist(tokens); this.tokens = tokens; this.error = "";
        return true;
      } catch (problem) {
        if (epoch === this.epoch) {
          if (problem.status === 400 || problem.status === 401) { this.tokens = null; try { fs.unlinkSync(this.tokenFile); } catch {} }
          this.error = problem instanceof FrontierError ? problem.message : "Не удалось продлить вход Frontier.";
        }
        throw problem;
      } finally { this.refreshing = null; this.emit(); }
    })();
    return this.refreshing;
  }
  async capi(endpoint) {
    if (!this.tokens) throw error("Сначала войдите в Frontier.", "LOGIN_REQUIRED", 401);
    this.assertCommander(this.identity || {});
    if (this.tokens.expires < this.now() + 60000) await this.refresh();
    const token = this.tokens.accessToken;
    const query = () => this.jsonRequest(CAPI + "/" + endpoint + "?language=ru", {
      headers: { Authorization: "Bearer " + this.tokens.accessToken, Accept: "application/json", "User-Agent": "EDCopilot/1.2" }
    }, endpoint === "fleetcarrier" ? 60000 : 15000);
    try { return (await query()).data; }
    catch (problem) {
      if (problem.code !== "UNAUTHORIZED") throw problem;
      if (this.tokens?.accessToken === token) await this.refresh();
      if (!this.tokens) throw error("Сначала войдите в Frontier.", "LOGIN_REQUIRED", 401);
      return (await query()).data;
    }
  }
  async sync({ carrier = true, shipyard = true } = {}) {
    if (this.syncing) return this.syncing;
    if (!this.tokens) throw error("Сначала войдите в Frontier.", "LOGIN_REQUIRED", 401);
    this.assertCommander(this.identity || {});
    const epoch = this.epoch;
    this.syncing = this.performSync({ carrier, shipyard, epoch }).catch((problem) => {
      if (epoch === this.epoch) this.error = problem instanceof FrontierError ? problem.message : "Не удалось обновить данные Frontier.";
      throw problem;
    }).finally(() => { this.syncing = null; this.emit(); });
    this.emit(); return this.syncing;
  }
  async performSync({ carrier, shipyard, epoch }) {
    const data = {}, errors = {}, current = this.now();
    const metadata = { source: "Frontier CAPI", observedAt: {} };
    if (!this.endpointTimes.profile || current >= this.endpointTimes.profile + 60000) {
      this.endpointTimes.profile = current;
      const profile = await this.capi("profile");
      metadata.observedAt.profile = new Date(this.now()).toISOString();
      if (!profile?.commander?.name) throw error("Frontier не вернул профиль командира.", "PROFILE_INVALID", 502);
      const identity = { ...this.identity, name: text(profile.commander.name) };
      this.assertCommander(identity);
      if (epoch !== this.epoch) throw error("Синхронизация Frontier отменена.", "CANCELLED");
      this.identity = identity; data.profile = profile;
      this.summary = summarise(profile, undefined, this.summary);
      this.lastSync = new Date(this.now()).toISOString();
      // Station endpoints are the last docked station, not a galaxy-wide market.
      if (profile.commander.docked === true) {
        const stationId = profile.lastStarport?.id;
        for (const endpoint of shipyard ? ["market", "shipyard"] : ["market"]) {
          try {
            const station = await this.capi(endpoint);
            const id = station?.id ?? station?.lastStarport?.id;
            if (stationId && String(stationId) !== String(id)) throw error("Данные Frontier относятся к другой станции. Обновите после синхронизации игры.", "STATION_MISMATCH");
            data[endpoint] = station;
            metadata.observedAt[endpoint] = new Date(this.now()).toISOString();
          } catch (problem) { errors[endpoint] = problem instanceof FrontierError ? problem.message : "Ошибка Frontier."; }
        }
      }
    }
    if (carrier && (!this.endpointTimes.carrier || current >= this.endpointTimes.carrier + 900000)) {
      this.endpointTimes.carrier = current;
      try {
        data.carrier = await this.capi("fleetcarrier");
        metadata.observedAt.carrier = new Date(this.now()).toISOString();
        if (data.carrier !== null && !data.carrier?.name?.callsign) throw error("Frontier не вернул данные флотоносца.", "CARRIER_INVALID", 502);
        if (epoch !== this.epoch) throw error("Синхронизация Frontier отменена.", "CANCELLED");
        this.summary = summarise(undefined, data.carrier, this.summary);
        this.lastCarrierSync = new Date(this.now()).toISOString();
      } catch (problem) { delete data.carrier; errors.carrier = problem instanceof FrontierError ? problem.message : "Ошибка Frontier."; }
    }
    if (epoch !== this.epoch) throw error("Синхронизация Frontier отменена.", "CANCELLED");
    this.error = Object.values(errors).join(" ");
    return { status: this.status(), data, errors, metadata };
  }
  logout() {
    this.epoch += 1; this.tokens = null; this.pending = null; this.identity = null;
    this.summary = {}; this.error = ""; this.lastSync = null; this.lastCarrierSync = null; this.endpointTimes = {};
    try { fs.unlinkSync(this.tokenFile); } catch (problem) { if (problem.code !== "ENOENT") throw error("Не удалось удалить сохранённый вход Frontier.", "LOGOUT_FAILED", 500); }
    this.emit(); return this.status();
  }
}

module.exports = { FrontierClient, FrontierError, summarise, marketSnapshot, DEFAULT_REDIRECT, REGISTER };
