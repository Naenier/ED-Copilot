"use strict";

const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const inara = require("./inara.cjs");
const MAX_QUEUE = 1000, MAX_QUEUE_BYTES = 8 * 1024 * 1024, RETRY_MIN = 60000;
const clean = (value, max = 180) => typeof value === "string" ? value.replace(/[\u0000-\u001f]/g, " ").slice(0, max) : "";
const obj = (value) => value && typeof value === "object" && !Array.isArray(value) ? value : {};
const number = (value) => typeof value === "number" && Number.isFinite(value) ? value : null;
const iso = (now) => new Date(now).toISOString();
const identity = (commander = {}) => ({ name: clean(commander.name), fid: clean(commander.fid, 40) });
const same = (left, right) => Boolean(left?.fid && right?.fid && left.fid === right.fid && left.name && left.name === right.name);
function emptyService() { return { sent: 0, failed: 0, lastSuccess: null, lastError: "", retryAt: 0 }; }
class IntegrationError extends Error {
  constructor(message, code = "INTEGRATION_ERROR", status = 400) { super(message); this.name = "IntegrationError"; this.code = code; this.status = status; }
}

class IntegrationManager {
  constructor({ dataDir, secureStorage = {}, encrypt, decrypt, getState = () => ({}), onUpdate = () => {},
    now = Date.now, transports = {}, autoDelayMs = 20000 } = {}) {
    if (!dataDir) throw new IntegrationError("Не задан каталог профиля интеграций.", "DATA_DIR_REQUIRED");
    this.file = path.join(dataDir, "integration-state.bin");
    this.encrypt = encrypt || secureStorage.encrypt; this.decrypt = decrypt || secureStorage.decrypt;
    this.getState = getState; this.onUpdate = onUpdate; this.now = now; this.transports = transports;
    this.autoDelayMs = Math.max(100, autoDelayMs);
    this.config = { enabled: false, apiKey: "", boundCommander: null };
    this.queue = []; this.delivered = []; this.cursors = {}; this.storageError = "";
    this.service = emptyService();
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
      if (![1, 2].includes(saved.version) || !saved.config || typeof saved.config !== "object") throw new Error();
      const previous = saved.config, legacy = saved.version === 1;
      const apiKey = legacy ? previous.inaraApiKey : previous.apiKey;
      this.config = {
        enabled: legacy ? previous.publisher === "copilot" && previous.inaraEnabled === true : previous.enabled === true,
        apiKey: typeof apiKey === "string" && apiKey.length <= 2048 && !/[\u0000\r\n]/.test(apiKey) ? apiKey : "",
        boundCommander: previous.boundCommander?.fid && previous.boundCommander?.name ? identity(previous.boundCommander) : null
      };
      this.queue = (Array.isArray(saved.queue) ? saved.queue : []).filter((row) => row?.service === "inara" && row.identity?.fid && row.payload && row.key).slice(-MAX_QUEUE);
      this.delivered = (Array.isArray(saved.delivered) ? saved.delivered : []).filter((value) => typeof value === "string" && value.startsWith("inara:")).slice(-4000);
      this.cursors = obj(saved.cursors); this.enabledAt = number(saved.enabledAt) || 0;
      const storedService = obj(legacy ? saved.services?.inara : saved.service);
      for (const key of Object.keys(this.service)) if (storedService[key] !== undefined) this.service[key] = storedService[key];
      if (legacy) this._persist();
    } catch { this.storageError = "Не удалось открыть защищённые данные интеграций. Используйте тот же профиль Windows."; }
  }
  _persist() {
    if (this.closed || !this.storageAvailable() || this.storageError) return;
    const saved = { version: 2, config: this.config, queue: this.queue,
      delivered: this.delivered, cursors: this.cursors, service: this.service, enabledAt: this.enabledAt };
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
  _eligible() {
    return !this.closed && !this.storageError && this.storageAvailable() && this.config.enabled && Boolean(this.config.apiKey) &&
      this.current.connection.scanning !== true && this._bound() && inara.validateLive(this._metadata()).ok;
  }
  _inaraContext() {
    const commander = this.current.commander;
    return { ...this._metadata(), now: this.now(), commander, state: this.current, shipType: commander.shipType || this.shipType, shipId: commander.shipId ?? this.shipId,
      loan: this.current.game.loan ?? this.loan, cargoUpdatedAt: commander.cargoUpdatedAt, cargoIsShip: String(commander.cargoVessel).toLowerCase() === "ship",
      locationUpdatedAt: commander.locationUpdatedAt };
  }
  status(context) {
    // Must stay independent of getState(): engine includes this status in its own state.
    if (context && typeof context === "object") this._cache(context);
    const live = inara.validateLive(this._metadata());
    const times = this.queue.map((row) => Math.max(row.nextAttemptAt || 0, this.service.retryAt || 0)).filter((time) => time > this.now());
    const blockedReason = !this.config.enabled ? "Отправка выключена" : this.storageError ||
      (!this.storageAvailable() ? "Для отправки запустите настольное приложение" : !this.config.apiKey ? "Сохраните личный API-ключ Inara" :
        !this.config.boundCommander || !this.current.commander.fid ? "Ожидаем командира из журнала игры" : !this._bound() ? "В игре выбран другой командир" :
          this.current.connection.scanning === true ? "Читаем журналы игры" : !live.ok ? live.reason : "");
    return { enabled: this.config.enabled, apiKeySet: Boolean(this.config.apiKey), storageAvailable: this.storageAvailable(), storageError: this.storageError || null,
      commander: identity(this.current.commander), boundCommander: this.config.boundCommander ? { ...this.config.boundCommander } : null,
      identityMatches: this._bound(), live, blockedReason, ...this.service,
      queued: this.queue.length, nextRetryAt: times.length ? iso(Math.min(...times)) : null, busy: this.busy, lastSync: this.lastSync };
  }
  configure(body = {}) {
    if (this.closed) throw new IntegrationError("Отправка в Inara закрыта.", "CLOSED", 409);
    this._refresh(); const next = { ...this.config };
    if (body.enabled !== undefined) {
      if (typeof body.enabled !== "boolean") throw new IntegrationError("Некорректная настройка отправки.", "SETTING_INVALID");
      next.enabled = body.enabled;
    }
    if (body.apiKey !== undefined) {
      if (typeof body.apiKey !== "string" || body.apiKey.length > 2048 || /[\u0000\r\n]/.test(body.apiKey)) throw new IntegrationError("Некорректный API-ключ Inara.", "KEY_INVALID");
      if (body.apiKey.trim()) next.apiKey = body.apiKey.trim();
    }
    if (body.clearApiKey === true) next.apiKey = "";
    if ((next.enabled || next.apiKey) && (!this.storageAvailable() || this.storageError)) {
      throw new IntegrationError(this.storageError || "Для Inara требуется защищённое хранилище Windows.", "STORAGE_UNAVAILABLE", 409);
    }
    const current = identity(this.current.commander);
    if (next.enabled && (!current.name || !current.fid)) throw new IntegrationError("Откройте игру: сначала нужно прочитать имя командира.", "COMMANDER_REQUIRED", 409);
    if (next.enabled && next.boundCommander && !same(current, next.boundCommander) && body.rebindCommander !== true) {
      throw new IntegrationError("Ключ привязан к другому командиру. Отправка приостановлена.", "COMMANDER_MISMATCH", 409);
    }
    if (next.enabled && (!next.boundCommander || body.rebindCommander === true)) next.boundCommander = current;
    if (body.rebindCommander === true) this.queue = [];
    if (next.apiKey !== this.config.apiKey) {
      for (const row of this.queue) { row.nextAttemptAt = 0; row.attempts = 0; }
      this.service.retryAt = 0; this.service.lastError = "";
    }
    if (!this.config.enabled && next.enabled) this.enabledAt = this.now();
    this.config = next; this.epoch += 1; this.controller?.abort();
    clearTimeout(this.timer); this.timer = null;
    this._persist(); this._notify(); this._schedule(); return this.status();
  }
  _remember(key) { this.delivered.push(key); if (this.delivered.length > 4000) this.delivered.splice(0, this.delivered.length - 4000); }
  _enqueue(payload) {
    if (!this._eligible()) return false;
    const active = identity(this.current.commander);
    const timestamp = payload.eventTimestamp;
    const time = Date.parse(timestamp), maxAge = inara.MAX_AGE_MS;
    if (!Number.isFinite(time) || time > this.now() + 300000 || time < this.now() - maxAge) return false;
    const key = "inara:" + inara.eventKey(payload, { frontierId: active.fid });
    if (this.delivered.includes(key) || this.queue.some((row) => row.key === key)) return false;
    const replace = inara.snapshotKey(payload);
    if (replace) {
      if (this.queue.some((row) => row.identity.fid === active.fid && row.replace === replace && Date.parse(row.timestamp) > time)) return false;
      this.queue = this.queue.filter((row) => !(row.identity.fid === active.fid && row.replace === replace &&
        Date.parse(row.timestamp) <= time));
    }
    this.queue.push({ key, service: "inara", identity: active, payload, replace, timestamp, attempts: 0, nextAttemptAt: 0, enqueuedAt: this.now() });
    let bytes = Buffer.byteLength(JSON.stringify(this.queue));
    while (this.queue.length > MAX_QUEUE || bytes > MAX_QUEUE_BYTES) {
      this.queue.shift(); bytes = Buffer.byteLength(JSON.stringify(this.queue));
      this.service.failed += 1; this.service.lastError = "Очередь переполнена: старые неподтверждённые наблюдения пропущены.";
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
    if (this._eligible()) for (const payload of inara.journalEvents(event, { ...this._inaraContext(), suppressDock: this.suppressDock })) this._enqueue(payload);
    if (type === "Location") this.suppressDock = event.Docked === true;
    else if (type === "Docked") this.suppressDock = false;
    else if (["FSDJump", "Undocked"].includes(type)) this.suppressDock = false;
    if (type === "CarrierJump") this.suppressDock = true;
    if (["Docked", "FSDJump", "Shutdown"].includes(type)) this._snapshot();
    this._notify();
  }
  observeSide(snapshot, metadata = {}) {
    if (this.closed || metadata.replay || !snapshot) return;
    this._refresh();
    // Side files can be reread after enabling an integration. That is not a new public observation.
    const at = Date.parse(snapshot.timestamp);
    if (!Number.isFinite(at) || at < this.enabledAt - 5000) return;
    if (this._eligible()) for (const payload of inara.journalEvents(snapshot, this._inaraContext())) this._enqueue(payload);
    this._notify();
  }
  observeCapi(result) {
    if (this.closed || result?.metadata?.source !== "Frontier CAPI") return;
    this._refresh();
    const profile = result.data?.profile, commander = profile?.commander;
    if (!commander || commander.name !== this.current.commander.name || !this._bound()) return;
    const at = result.metadata.observedAt?.profile;
    if (Number.isSafeInteger(commander.debt) && commander.debt >= 0) this.loan = commander.debt;
    if (this._eligible() && Number.isSafeInteger(commander.credits) && commander.credits >= 0 && Number.isSafeInteger(this.loan)) {
      this._enqueue({ eventName: "setCommanderCredits", eventTimestamp: at,
        eventData: { commanderCredits: commander.credits, commanderLoan: this.loan } });
    }
    this._notify();
  }
  _snapshot() {
    if (!this._eligible()) return;
    for (const payload of inara.buildSnapshotEvents(this.current, this._inaraContext())) this._enqueue(payload);
  }
  _schedule() {
    if (this.timer || this.closed || this.busy || !this.queue.length || !this._eligible()) return;
    const due = Math.min(...this.queue.map((row) => Math.max(row.nextAttemptAt || this.now(), this.service.retryAt || 0)));
    const delay = Math.max(this.autoDelayMs, due - this.now());
    this.timer = setTimeout(() => { this.timer = null; void this._drain().catch(() => {}); }, Math.min(delay, 86400000)); this.timer.unref?.();
  }
  _retry(row, message, retryAfterMs = 0) {
    row.attempts = (row.attempts || 0) + 1;
    row.nextAttemptAt = this.now() + Math.max(RETRY_MIN, retryAfterMs, Math.min(3600000, RETRY_MIN * 2 ** Math.min(row.attempts - 1, 6)));
    this.service.retryAt = Math.max(this.service.retryAt || 0, row.nextAttemptAt);
    this.service.lastError = clean(message, 500) || "Отправка не подтверждена. Повторим позже.";
  }
  _finish(row, ok, message = "") {
    this.queue = this.queue.filter((item) => item.key !== row.key); this._remember(row.key);
    const service = this.service;
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
    if (!this._eligible()) { this._notify(); return this.status(); }
    this.busy = true; const epoch = this.epoch; this.controller = new AbortController(); this._notify();
    // Persist before attempting upload so a process crash cannot lose unconfirmed observations.
    this._persist();
    try {
      if (this.storageError) return this.status();
      for (const row of this.queue.slice()) {
        const age = this.now() - Date.parse(row.timestamp), maxAge = inara.MAX_AGE_MS;
        if (!Number.isFinite(age) || age > maxAge) this._finish(row, false, "Наблюдение устарело и не отправлено.");
      }
      if (this._eligible() && (this.service.retryAt || 0) <= this.now()) {
        const batch = []; let bytes = 0;
        for (const row of this.queue.filter((item) => same(item.identity, this.config.boundCommander) && (item.nextAttemptAt || 0) <= this.now())) {
          const size = Buffer.byteLength(JSON.stringify(row.payload));
          if (batch.length && (batch.length >= 50 || bytes + size > 768000)) break;
          batch.push(row); bytes += size;
        }
        if (batch.length) {
          try {
            const send = this.transports.sendInara || inara.sendBatch;
            const response = await send({ commanderName: this.config.boundCommander.name, frontierId: this.config.boundCommander.fid,
              apiKey: this.config.apiKey, events: batch.map((row) => row.payload), appVersion: this.current.app.version || "1.8.0",
              signal: this.controller.signal, fetchImpl: this.transports.inaraFetch || this.transports.fetch, now: this.now(), isBeingDeveloped: true });
            this._refresh();
            if (this.closed || epoch !== this.epoch || !this._eligible()) return this.status();
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
              else this._retry(row, problem instanceof inara.InaraError ? problem.message : "Не удалось связаться с Inara.", problem?.retryAfterMs);
            }
          }
        }
      }
      this.lastSync = iso(this.now());
    } finally {
      this.busy = false; this.controller = null;
      this._persist(); this._notify(); this._schedule();
    }
    return this.status();
  }
  close() {
    if (this.closed) return;
    this.controller?.abort(); this.epoch += 1;
    clearTimeout(this.timer); clearTimeout(this.saveTimer); this.timer = null; this.saveTimer = null;
    this._persist(); this.closed = true;
  }
}

module.exports = { IntegrationManager, Integrations: IntegrationManager, IntegrationError, MAX_QUEUE, MAX_QUEUE_BYTES };
