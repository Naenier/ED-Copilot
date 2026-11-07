"use strict";

const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const inara = require("./inara.cjs");
const eddn = require("./eddn.cjs");
const MAX_QUEUE = 1000, MAX_QUEUE_BYTES = 8 * 1024 * 1024, RETRY_MIN = 60000;
const clean = (value, max = 180) => typeof value === "string" ? value.replace(/[\u0000-\u001f]/g, " ").slice(0, max) : "";
const obj = (value) => value && typeof value === "object" && !Array.isArray(value) ? value : {};
const number = (value) => typeof value === "number" && Number.isFinite(value) ? value : null;
const iso = (now) => new Date(now).toISOString();
const identity = (commander = {}) => ({ name: clean(commander.name), fid: clean(commander.fid, 40) });
const same = (left, right) => Boolean(left?.fid && right?.fid && left.fid === right.fid && left.name && left.name === right.name);
const digest = (value) => crypto.createHash("sha256").update(JSON.stringify(value)).digest("hex");
function emptyService() { return { sent: 0, failed: 0, lastSuccess: null, lastError: "", setupRequired: false, nextRetryAt: null, retryAt: 0 }; }
class IntegrationError extends Error {
  constructor(message, code = "INTEGRATION_ERROR", status = 400) { super(message); this.name = "IntegrationError"; this.code = code; this.status = status; }
}

class IntegrationManager {
  constructor({ dataDir, secureStorage = {}, encrypt, decrypt, getState = () => ({}), onUpdate = () => {},
    now = Date.now, transports = {}, autoDelayMs = 20000, bridgeUrl = "http://127.0.0.1:7122/api/integrations/edmc" } = {}) {
    if (!dataDir) throw new IntegrationError("Не задан каталог профиля интеграций.", "DATA_DIR_REQUIRED");
    this.file = path.join(dataDir, "integration-state.bin");
    this.encrypt = encrypt || secureStorage.encrypt; this.decrypt = decrypt || secureStorage.decrypt;
    this.getState = getState; this.onUpdate = onUpdate; this.now = now; this.transports = transports;
    this.autoDelayMs = Math.max(100, autoDelayMs); this.bridgeUrl = bridgeUrl;
    this.config = { publisher: "off", inaraEnabled: false, eddnEnabled: false, inaraAppRegistered: false, inaraApiKey: "", boundCommander: null };
    this.queue = []; this.delivered = []; this.cursors = {}; this.bridgeToken = ""; this.storageError = "";
    this.services = { inara: emptyService(), eddn: emptyService() };
    this.edmc = { lastSeen: null, commander: "", version: "", status: "Нет подключения", event: "", liveGalaxy: null };
    this.current = { commander: {}, game: {}, connection: {}, app: {} }; this.game = {}; this.shipType = ""; this.shipId = null; this.loan = null;
    this.closed = false; this.busy = false; this.lastSync = null; this.epoch = 0; this.controller = null; this.enabledAt = 0; this.readingState = false;
    this._load();
    this._schedule();
  }
  storageAvailable() { return typeof this.encrypt === "function" && typeof this.decrypt === "function"; }
  _load() {
    if (!fs.existsSync(this.file)) return;
    if (!this.storageAvailable()) { this.storageError = "Защищённое хранилище Windows недоступно. Синхронизация отключена."; return; }
    try {
      const saved = JSON.parse(this.decrypt(fs.readFileSync(this.file)).toString());
      if (saved.version !== 1 || !["off", "copilot", "edmc"].includes(saved.config?.publisher)) throw new Error();
      this.config = { ...this.config, ...saved.config }; this.bridgeToken = clean(saved.bridgeToken, 512);
      this.queue = (Array.isArray(saved.queue) ? saved.queue : []).filter((row) => ["inara", "eddn"].includes(row?.service) && row.identity?.fid && row.payload && row.key).slice(-MAX_QUEUE);
      this.delivered = (Array.isArray(saved.delivered) ? saved.delivered : []).filter((value) => typeof value === "string").slice(-4000);
      this.cursors = obj(saved.cursors); this.enabledAt = number(saved.enabledAt) || 0;
      for (const service of ["inara", "eddn"]) this.services[service] = { ...emptyService(), ...obj(saved.services?.[service]) };
    } catch { this.storageError = "Не удалось открыть защищённые данные интеграций. Используйте тот же профиль Windows."; }
  }
  _persist() {
    if (this.closed || !this.storageAvailable() || this.storageError) return;
    const saved = { version: 1, config: this.config, bridgeToken: this.bridgeToken, queue: this.queue,
      delivered: this.delivered, cursors: this.cursors, services: this.services, enabledAt: this.enabledAt };
    try {
      const buffer = this.encrypt(JSON.stringify(saved));
      if (!Buffer.isBuffer(buffer) || !buffer.length) throw new Error();
      fs.mkdirSync(path.dirname(this.file), { recursive: true });
      const temporary = this.file + "." + crypto.randomBytes(6).toString("hex") + ".tmp";
      try { fs.writeFileSync(temporary, buffer, { mode: 0o600 }); fs.renameSync(temporary, this.file); }
      finally { try { fs.unlinkSync(temporary); } catch {} }
    } catch { this.storageError = "Не удалось сохранить защищённую очередь. Отправка приостановлена."; }
  }
  _saveSoon() {
    if (this.saveTimer || this.closed) return;
    this.saveTimer = setTimeout(() => { this.saveTimer = null; this._persist(); }, 500); this.saveTimer.unref?.();
  }
  _notify() { try { this.onUpdate(this.status()); } catch {} }
  _cache(context, merge = true) {
    const previous = identity(this.current.commander), wasLive = inara.validateLive(this._metadata()).ok;
    if (!merge) this.current = { ...context, commander: { ...obj(context.commander) }, game: { ...obj(context.game) },
      connection: { ...obj(context.connection) }, app: { ...obj(context.app) } };
    else for (const field of ["commander", "game", "connection", "app"]) {
      if (context[field] && typeof context[field] === "object") this.current[field] = { ...this.current[field], ...context[field] };
    }
    const next = identity(this.current.commander);
    if (previous.fid && next.fid && !same(previous, next) || wasLive && !inara.validateLive(this._metadata()).ok) {
      this.epoch += 1; this.controller?.abort();
    }
    if (this.queue.length && this._bound() && this.current.connection.scanning !== true) this._schedule();
  }
  _refresh() {
    if (this.readingState) return this.current;
    this.readingState = true;
    try {
      const state = this.getState() || {};
      this._cache(state, false);
    } catch {} finally { this.readingState = false; }
    return this.current;
  }
  _metadata() {
    const game = { ...this.game, ...this.current.game };
    return { gameVersion: game.version || game.gameversion || game.gameVersion || "", gameBuild: game.build || game.gamebuild || game.gameBuild || "",
      isBeta: game.isBeta === true, live: game.live, legacy: game.legacy, multicrew: game.multicrew,
      horizons: game.horizons, odyssey: game.odyssey };
  }
  _bound() { return same(identity(this.current.commander), this.config.boundCommander); }
  _eligible(service) {
    return !this.closed && !this.storageError && this.storageAvailable() && this.config.publisher === "copilot" && this.current.connection.scanning !== true && this._bound() && inara.validateLive(this._metadata()).ok &&
      (service === "inara" ? this.config.inaraEnabled && this.config.inaraAppRegistered && Boolean(this.config.inaraApiKey) : this.config.eddnEnabled);
  }
  _inaraContext() {
    const commander = this.current.commander;
    return { ...this._metadata(), now: this.now(), commander, state: this.current, shipType: commander.shipType || this.shipType, shipId: commander.shipId ?? this.shipId,
      loan: this.current.game.loan ?? this.loan, cargoUpdatedAt: commander.cargoUpdatedAt, cargoIsShip: String(commander.cargoVessel).toLowerCase() === "ship",
      locationUpdatedAt: commander.locationUpdatedAt };
  }
  _eddnContext() {
    const commander = this.current.commander, metadata = this._metadata();
    return { commanderName: commander.name, softwareVersion: this.current.app.version || "1.4.1", ...metadata,
      liveGalaxy: inara.validateLive(metadata).ok, systemName: commander.system, systemAddress: commander.systemAddress,
      starPos: commander.coordinates, stationName: commander.station, marketId: commander.marketId, docked: commander.docked,
      gameRunning: this.current.connection.gameRunning };
  }
  status(context) {
    // Must stay independent of getState(): engine includes this status in its own state.
    if (context && typeof context === "object") this._cache(context);
    const inaraSetup = this.config.inaraEnabled && (!this.config.inaraAppRegistered || !this.config.inaraApiKey);
    const service = (name) => {
      const rows = this.queue.filter((row) => row.service === name);
      const times = rows.map((row) => Math.max(row.nextAttemptAt || 0, this.services[name].retryAt || 0)).filter((time) => time > this.now());
      return { enabled: name === "inara" ? this.config.inaraEnabled : this.config.eddnEnabled,
        ...this.services[name], queued: rows.length, nextRetryAt: times.length ? iso(Math.min(...times)) : null };
    };
    return { publisher: this.config.publisher, storageAvailable: this.storageAvailable(), storageError: this.storageError || null,
      commander: identity(this.current.commander), boundCommander: this.config.boundCommander ? { ...this.config.boundCommander } : null,
      identityMatches: this._bound(), live: inara.validateLive(this._metadata()),
      inara: { ...service("inara"), keySet: Boolean(this.config.inaraApiKey), appRegistered: this.config.inaraAppRegistered,
        setupRequired: Boolean(inaraSetup || this.services.inara.setupRequired) }, eddn: service("eddn"),
      edmc: { ...this.edmc, paired: Boolean(this.bridgeToken), connected: Boolean(this.edmc.lastSeen && this.now() - Date.parse(this.edmc.lastSeen) < 60000) },
      queue: { total: this.queue.length }, busy: this.busy, lastSync: this.lastSync };
  }
  configure(body = {}) {
    if (this.closed) throw new IntegrationError("Интеграции закрыты.", "CLOSED", 409);
    this._refresh();
    const nested = obj(body.inara), eddnConfig = obj(body.eddn);
    const next = { ...this.config };
    if (body.publisher !== undefined) {
      if (!["off", "copilot", "edmc"].includes(body.publisher)) throw new IntegrationError("Выберите источник отправки: ED Copilot, EDMC или выключено.", "PUBLISHER_INVALID");
      next.publisher = body.publisher;
    }
    for (const [field, value] of [["inaraEnabled", body.inaraEnabled ?? nested.enabled], ["eddnEnabled", body.eddnEnabled ?? eddnConfig.enabled],
      ["inaraAppRegistered", body.inaraAppRegistered ?? nested.appRegistered]]) {
      if (value !== undefined) { if (typeof value !== "boolean") throw new IntegrationError("Некорректная настройка интеграции.", "SETTING_INVALID"); next[field] = value; }
    }
    const apiKey = body.inaraApiKey ?? nested.apiKey;
    if (apiKey !== undefined) {
      if (typeof apiKey !== "string" || apiKey.length > 2048 || /[\u0000\r\n]/.test(apiKey)) throw new IntegrationError("Некорректный API-ключ Inara.", "KEY_INVALID");
      if (apiKey.trim()) next.inaraApiKey = apiKey.trim();
    }
    if (body.clearInaraKey === true) next.inaraApiKey = "";
    const active = next.publisher !== "off";
    if ((active || next.inaraApiKey) && (!this.storageAvailable() || this.storageError)) {
      throw new IntegrationError(this.storageError || "Для интеграций требуется защищённое хранилище Windows.", "STORAGE_UNAVAILABLE", 409);
    }
    const current = identity(this.current.commander);
    if (active && (!current.name || !current.fid)) throw new IntegrationError("Откройте игру: сначала нужно прочитать имя и FID командира.", "COMMANDER_REQUIRED", 409);
    if (active && next.boundCommander && !same(current, next.boundCommander) && body.rebindCommander !== true) {
      throw new IntegrationError("Настройки привязаны к другому командиру. Отправка приостановлена.", "COMMANDER_MISMATCH", 409);
    }
    if (active && (!next.boundCommander || body.rebindCommander === true)) next.boundCommander = current;
    const changedPublisher = next.publisher !== this.config.publisher;
    if (changedPublisher || body.rebindCommander === true) this.queue = [];
    else this.queue = this.queue.filter((row) => row.service === "inara" ? next.inaraEnabled : next.eddnEnabled);
    const enabling = !this.config.inaraEnabled && next.inaraEnabled || !this.config.eddnEnabled && next.eddnEnabled;
    if (apiKey !== undefined || body.inaraAppRegistered !== undefined || nested.appRegistered !== undefined) {
      for (const row of this.queue) if (row.service === "inara") { row.nextAttemptAt = 0; row.attempts = 0; }
      this.services.inara.retryAt = 0;
    }
    this.config = next; this.epoch += 1; this.controller?.abort();
    if (changedPublisher || enabling) this.enabledAt = this.now();
    this.services.inara.setupRequired = false;
    if (apiKey !== undefined || body.inaraAppRegistered !== undefined || nested.appRegistered !== undefined) this.services.inara.lastError = "";
    clearTimeout(this.timer); this.timer = null;
    this._persist(); this._notify(); this._schedule(); return this.status();
  }
  _remember(key) { this.delivered.push(key); if (this.delivered.length > 4000) this.delivered.splice(0, this.delivered.length - 4000); }
  _enqueue(service, payload) {
    if (!this._eligible(service)) return false;
    const active = identity(this.current.commander);
    const timestamp = service === "inara" ? payload.eventTimestamp : payload.message?.timestamp;
    const time = Date.parse(timestamp), maxAge = service === "inara" ? inara.MAX_AGE_MS : 600000;
    if (!Number.isFinite(time) || time > this.now() + 300000 || time < this.now() - maxAge) return false;
    const key = service + ":" + (service === "inara" ? inara.eventKey(payload, { frontierId: active.fid }) : digest({ fid: active.fid, payload }));
    if (this.delivered.includes(key) || this.queue.some((row) => row.key === key)) return false;
    const replace = service === "inara" ? inara.snapshotKey(payload) : payload.$schemaRef + ":" + payload.message?.marketId;
    if (replace && !(service === "eddn" && payload.$schemaRef?.includes("/journal/"))) {
      if (this.queue.some((row) => row.service === service && row.identity.fid === active.fid && row.replace === replace && Date.parse(row.timestamp) > time)) return false;
      this.queue = this.queue.filter((row) => !(row.service === service && row.identity.fid === active.fid && row.replace === replace &&
        Date.parse(row.timestamp) <= time));
    }
    this.queue.push({ key, service, identity: active, payload, replace, timestamp, attempts: 0, nextAttemptAt: 0, enqueuedAt: this.now() });
    let bytes = Buffer.byteLength(JSON.stringify(this.queue));
    while (this.queue.length > MAX_QUEUE || bytes > MAX_QUEUE_BYTES) {
      const removed = this.queue.shift(); bytes = Buffer.byteLength(JSON.stringify(this.queue));
      this.services[removed.service].failed += 1; this.services[removed.service].lastError = "Очередь переполнена: старые неподтверждённые наблюдения пропущены.";
    }
    this._saveSoon(); this._schedule(); return true;
  }
  observeJournal(event, metadata = {}) {
    if (this.closed || !event || typeof event.event !== "string") return;
    const type = event.event;
    if (type === "Fileheader") this.game = { version: event.gameversion || "", build: event.build || "", isBeta: /beta|alpha/i.test(event.gameversion || ""),
      live: /^([4-9]|[1-9]\d+)\./.test(event.gameversion || "") };
    if (type === "LoadGame") {
      if (typeof event.Horizons === "boolean" || typeof event.horizons === "boolean") this.game.horizons = event.Horizons ?? event.horizons;
      if (typeof event.Odyssey === "boolean" || typeof event.odyssey === "boolean") this.game.odyssey = event.Odyssey ?? event.odyssey;
      this.loan = Number.isSafeInteger(event.Loan) && event.Loan >= 0 ? event.Loan : 0;
    }
    if (type === "Loadout" && typeof event.Ship === "string" && Number.isSafeInteger(event.ShipID)) { this.shipType = event.Ship; this.shipId = event.ShipID; }
    if (["JoinACrew", "CrewJoin"].includes(type)) this.game.multicrew = true;
    if (["QuitACrew", "CrewQuit", "LoadGame"].includes(type)) this.game.multicrew = false;
    const file = clean(metadata.file, 2048), offset = metadata.offset;
    if (!file || !Number.isSafeInteger(offset) || offset < 0) return;
    const previous = number(this.cursors[file]);
    this.cursors[file] = Math.max(previous ?? -1, offset); this._saveSoon();
    if (metadata.replay || previous !== null && offset <= previous || Date.parse(event.timestamp) < this.enabledAt - 5000) return;
    this._refresh();
    if (this._eligible("inara")) for (const payload of inara.journalEvents(event, { ...this._inaraContext(), suppressDock: this.suppressDock })) this._enqueue("inara", payload);
    if (type === "Location") this.suppressDock = event.Docked === true;
    else if (type === "Docked") this.suppressDock = false;
    else if (["FSDJump", "Undocked"].includes(type)) this.suppressDock = false;
    if (type === "CarrierJump") this.suppressDock = true;
    if (this._eligible("eddn")) for (const payload of eddn.buildJournalMessages(event, this._eddnContext())) this._enqueue("eddn", payload);
    if (["Docked", "FSDJump", "Shutdown"].includes(type)) this._snapshot();
    this._notify();
  }
  observeSide(snapshot, metadata = {}) {
    if (this.closed || metadata.replay || !snapshot) return;
    this._refresh();
    // Side files can be reread after enabling an integration. That is not a new public observation.
    const at = Date.parse(snapshot.timestamp);
    if (!Number.isFinite(at) || at < this.enabledAt - 5000) return;
    if (this._eligible("inara")) for (const payload of inara.journalEvents(snapshot, this._inaraContext())) this._enqueue("inara", payload);
    if (this._eligible("eddn")) for (const payload of eddn.buildStationMessages(snapshot, this._eddnContext())) this._enqueue("eddn", payload);
    this._notify();
  }
  observeCapi(result) {
    if (this.closed || result?.metadata?.source !== "Frontier CAPI") return;
    this._refresh();
    const profile = result.data?.profile, commander = profile?.commander;
    if (!commander || commander.name !== this.current.commander.name || !this._bound()) return;
    const at = result.metadata.observedAt?.profile;
    if (Number.isSafeInteger(commander.debt) && commander.debt >= 0) this.loan = commander.debt;
    if (this._eligible("inara") && Number.isSafeInteger(commander.credits) && commander.credits >= 0 && Number.isSafeInteger(this.loan)) {
      this._enqueue("inara", { eventName: "setCommanderCredits", eventTimestamp: at,
        eventData: { commanderCredits: commander.credits, commanderLoan: this.loan } });
    }
    if (this._eligible("eddn")) for (const payload of eddn.buildStationMessages({ source: "capi", profile, market: result.data.market,
      shipyard: result.data.shipyard, observedAt: result.metadata.observedAt }, this._eddnContext())) this._enqueue("eddn", payload);
    this._notify();
  }
  _snapshot() {
    if (!this._eligible("inara")) return;
    for (const payload of inara.buildSnapshotEvents(this.current, this._inaraContext())) this._enqueue("inara", payload);
  }
  _schedule() {
    if (this.timer || this.closed || this.busy || !this.queue.length || this.config.publisher !== "copilot") return;
    const due = Math.min(...this.queue.map((row) => Math.max(row.nextAttemptAt || this.now(), this.services[row.service].retryAt || 0)));
    const delay = Math.max(this.autoDelayMs, due - this.now());
    this.timer = setTimeout(() => { this.timer = null; void this._drain().catch(() => {}); }, Math.min(delay, 86400000)); this.timer.unref?.();
  }
  _retry(row, message, retryAfterMs = 0) {
    row.attempts += 1;
    row.nextAttemptAt = this.now() + Math.max(RETRY_MIN, retryAfterMs, Math.min(3600000, RETRY_MIN * 2 ** Math.min(row.attempts - 1, 6)));
    this.services[row.service].retryAt = Math.max(this.services[row.service].retryAt || 0, row.nextAttemptAt);
    this.services[row.service].lastError = clean(message, 500) || "Отправка не подтверждена. Повторим позже.";
  }
  _finish(row, ok, message = "") {
    this.queue = this.queue.filter((item) => item.key !== row.key); this._remember(row.key);
    const service = this.services[row.service];
    if (ok) { service.sent += 1; service.lastSuccess = iso(this.now()); if (!this.queue.some((item) => item.service === row.service && item.attempts > 0)) service.lastError = ""; }
    else { service.failed += 1; service.lastError = clean(message, 500) || "Сервис отклонил событие."; }
  }
  async sync() {
    if (this.closed) throw new IntegrationError("Интеграции закрыты.", "CLOSED", 409);
    this._refresh(); this._snapshot(); return this._drain();
  }
  async _drain() {
    if (this.closed || this.busy) return this.status();
    this._refresh();
    if (!this._eligible("inara") && !this._eligible("eddn")) { this._notify(); return this.status(); }
    this.busy = true; const epoch = this.epoch; this.controller = new AbortController(); this._notify();
    // Persist before attempting upload so a process crash cannot lose unconfirmed observations.
    this._persist();
    try {
      if (this.storageError) return this.status();
      for (const row of this.queue.slice()) {
        const age = this.now() - Date.parse(row.timestamp), maxAge = row.service === "inara" ? inara.MAX_AGE_MS : 600000;
        if (!Number.isFinite(age) || age > maxAge) this._finish(row, false, "Наблюдение устарело и не отправлено.");
      }
      if (this._eligible("inara") && (this.services.inara.retryAt || 0) <= this.now()) {
        const batch = []; let bytes = 0;
        for (const row of this.queue.filter((item) => item.service === "inara" && same(item.identity, this.config.boundCommander) && item.nextAttemptAt <= this.now())) {
          const size = Buffer.byteLength(JSON.stringify(row.payload));
          if (batch.length && (batch.length >= 50 || bytes + size > 768000)) break;
          batch.push(row); bytes += size;
        }
        if (batch.length) {
          try {
            const send = this.transports.sendInara || inara.sendBatch;
            const response = await send({ commanderName: this.config.boundCommander.name, frontierId: this.config.boundCommander.fid,
              apiKey: this.config.inaraApiKey, events: batch.map((row) => row.payload), appVersion: this.current.app.version || "1.4.1",
              signal: this.controller.signal, fetchImpl: this.transports.inaraFetch || this.transports.fetch, now: this.now(), isBeingDeveloped: true });
            this._refresh();
            if (this.closed || epoch !== this.epoch) return this.status();
            this.services.inara.setupRequired = response.setupRequired === true;
            for (let i = 0; i < batch.length; i++) {
              const outcome = response.results?.[i];
              if (outcome?.ok) this._finish(batch[i], true);
              else if (response.setupRequired) this._retry(batch[i], response.message || outcome?.message, 3600000);
              else if (!outcome || outcome.retryable) this._retry(batch[i], outcome?.message || response.message);
              else this._finish(batch[i], false, outcome.message || response.message);
            }
          } catch (problem) {
            if (this.closed || epoch !== this.epoch) return this.status();
            for (const row of batch) {
              if (problem instanceof inara.InaraError && !problem.retryable) this._finish(row, false, problem.message);
              else this._retry(row, problem instanceof inara.InaraError ? problem.message : "Не удалось связаться с Inara.", problem.retryAfterMs);
            }
          }
        }
      }
      if (this._eligible("eddn") && epoch === this.epoch && (this.services.eddn.retryAt || 0) <= this.now()) {
        const batch = this.queue.filter((row) => row.service === "eddn" && same(row.identity, this.config.boundCommander) && row.nextAttemptAt <= this.now()).slice(0, 10);
        for (const row of batch) {
          this._refresh();
          if (this.closed || epoch !== this.epoch || !this._eligible("eddn") || (this.services.eddn.retryAt || 0) > this.now() || !same(row.identity, identity(this.current.commander))) break;
          const send = this.transports.sendEddn || eddn.sendEddn;
          let result;
          try { result = await send(row.payload, { transport: this.transports.eddnTransport, signal: this.controller.signal, timeoutMs: 20000 }); }
          catch { result = { ok: false, retryable: true, error: "Не удалось связаться с EDDN." }; }
          this._refresh();
          if (this.closed || epoch !== this.epoch) break;
          if (result.ok) this._finish(row, true);
          else if (result.retryable) this._retry(row, result.error, result.retryAfterMs);
          else this._finish(row, false, result.error);
        }
      }
      this.lastSync = iso(this.now());
    } finally {
      this.busy = false; this.controller = null;
      for (const name of ["inara", "eddn"]) {
        const times = this.queue.filter((row) => row.service === name).map((row) => row.nextAttemptAt).filter((value) => value > this.now());
        this.services[name].nextRetryAt = times.length ? iso(Math.min(...times)) : null;
      }
      this._persist(); this._notify(); this._schedule();
    }
    return this.status();
  }
  pair() {
    if (this.closed) throw new IntegrationError("Интеграции закрыты.", "CLOSED", 409);
    if (!this.storageAvailable() || this.storageError) throw new IntegrationError("Для токена EDMC требуется защищённое хранилище Windows.", "STORAGE_UNAVAILABLE", 409);
    this.bridgeToken = crypto.randomBytes(32).toString("base64url"); this._persist();
    if (this.storageError) throw new IntegrationError(this.storageError, "STORAGE_UNAVAILABLE", 409);
    this._notify(); return { token: this.bridgeToken, url: this.bridgeUrl };
  }
  acceptEdmc(body, token) {
    if (this.closed) throw new IntegrationError("Интеграции закрыты.", "CLOSED", 409);
    const expected = Buffer.from(this.bridgeToken), supplied = Buffer.from(typeof token === "string" && token.length <= 512 ? token : "");
    if (!expected.length || expected.length !== supplied.length || !crypto.timingSafeEqual(expected, supplied)) {
      throw new IntegrationError("Неверный токен локального моста EDMC.", "BRIDGE_UNAUTHORIZED", 401);
    }
    if (this.config.publisher !== "edmc") throw new IntegrationError("В настройках выберите EDMC как источник отправки.", "BRIDGE_INACTIVE", 409);
    if (!body || !["heartbeat", "journal", "capi"].includes(body.type) || typeof body.commander !== "string" || body.commander.length > 180) {
      throw new IntegrationError("Некорректное сообщение EDMC.", "BRIDGE_INVALID");
    }
    this.edmc = { lastSeen: iso(this.now()), commander: clean(body.commander), version: clean(body.pluginVersion, 40),
      event: clean(body.entry?.event, 80), liveGalaxy: typeof body.liveGalaxy === "boolean" ? body.liveGalaxy : null,
      status: body.commander && body.commander !== this.config.boundCommander?.name ? "EDMC передаёт другого командира" : body.isBeta ? "Бета-версия: отправка отключена в EDMC" : "Мост подключён; отправкой управляет EDMC" };
    // EDMC remains the only publisher in this mode. Do not replay its telemetry into Inara or EDDN.
    this._notify(); return { ok: true, publisher: "edmc", receivedAt: this.edmc.lastSeen };
  }
  close() {
    if (this.closed) return;
    this.controller?.abort(); this.epoch += 1;
    clearTimeout(this.timer); clearTimeout(this.saveTimer); this.timer = null; this.saveTimer = null;
    this._persist(); this.closed = true;
  }
}

module.exports = { IntegrationManager, Integrations: IntegrationManager, IntegrationError, MAX_QUEUE, MAX_QUEUE_BYTES };
