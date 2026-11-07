"use strict";

const fs = require("node:fs");
const fsp = fs.promises;
const path = require("node:path");
const os = require("node:os");
const { keyOf, buildTargets, TIER, distance } = require("./targets.cjs");
const catalog = require("./catalog.cjs");
const { TradeLedger } = require("./trade.cjs");
const { CarrierTracker } = require("./carrier.cjs");
const { MiningPlanner } = require("./mining-planner.cjs");
const { EngineeringPlanner } = require("./engineering.cjs");
const { PilotTracker, normalizeObjective } = require("./pilot.cjs");
const russianCommodityNames = require("./commodity-ru.json");

const DEFAULTS = {
  minTier: "medium", method: "all", radiusLy: 50, maxArrivalLs: 10000, minLaserPercent: 15,
  hideDepleted: true, overlayEnabled: true, hideUnfocused: true,
  activity: "flight",
};
const finite = (value, fallback = null) => Number.isFinite(Number(value)) && value != null ? Number(value) : fallback;
const nowIso = () => new Date().toISOString();
const later = () => new Promise((resolve) => setImmediate(resolve));
const waitForPoll = () => new Promise((resolve) => setTimeout(resolve, 25));
const resourceFor = (name) => catalog.getResource?.(name) ?? catalog.resources.find((resource) => resource.key === keyOf(name));

function readJson(filename, fallback) {
  try { return JSON.parse(fs.readFileSync(filename, "utf8")); } catch { return fallback; }
}

function writeJson(filename, value) {
  fs.mkdirSync(path.dirname(filename), { recursive: true });
  const temporary = filename + ".tmp";
  fs.writeFileSync(temporary, JSON.stringify(value), "utf8");
  fs.renameSync(temporary, filename);
}

function displayName(name, localized) {
  return resourceFor(name)?.name ?? localized ?? String(name ?? "");
}

function defaultJournalDir() {
  return path.join(os.homedir(), "Saved Games", "Frontier Developments", "Elite Dangerous");
}

function mergeRemoteRecords(previous, incoming) {
  const bodies = new Map((previous?.bodies ?? []).map((body) => [String(body.name).toLowerCase(), body]));
  for (const body of incoming.bodies ?? []) {
    const key = String(body.name).toLowerCase();
    const old = bodies.get(key);
    const rings = new Map((old?.rings ?? []).map((ring) => [ring.name, ring]));
    for (const ring of body.rings ?? []) {
      const prior = rings.get(ring.name);
      const signals = new Map((prior?.signals ?? []).map((signal) => [keyOf(signal.key ?? signal.Type ?? signal.name), signal]));
      for (const signal of ring.signals ?? []) signals.set(keyOf(signal.key ?? signal.Type ?? signal.name), signal);
      rings.set(ring.name, { ...prior, ...ring, signals: [...signals.values()] });
    }
    const fields = Object.fromEntries(Object.entries(body).filter(([, value]) => value != null && value !== ""));
    bodies.set(key, { ...old, ...fields, rings: [...rings.values()],
      materials: body.materials?.length ? body.materials : old?.materials ?? [] });
  }
  return { ...previous, ...incoming, address: incoming.address ?? previous?.address ?? null,
    coordinates: incoming.coordinates ?? previous?.coordinates ?? null, bodies: [...bodies.values()] };
}

class MiningEngine {
  constructor({ dataDir, journalDir }) {
    this.dataDir = dataDir;
    this.configFile = path.join(dataDir, "settings.json");
    this.cacheFile = path.join(dataDir, "online-cache.json");
    const saved = readJson(this.configFile, {});
    this.preferences = { ...DEFAULTS, ...saved.settings };
    this.journalDir = journalDir || saved.journalDir || defaultJournalDir();
    this.bookmarks = Array.isArray(saved.bookmarks) ? saved.bookmarks : [];
    this.manualPrices = saved.manualPrices && typeof saved.manualPrices === "object" ? saved.manualPrices : {};
    this.statsSince = saved.statsSince || null;
    this.carrierTracker = new CarrierTracker(saved.carrier ?? {});
    this.tradeLedger = new TradeLedger({ russianNames: russianCommodityNames });
    this.miningPlanner = new MiningPlanner(saved.mining ?? {});
    this.engineeringPlanner = new EngineeringPlanner(saved.engineering ?? {});
    this.pilotTracker = new PilotTracker();
    this.objectives = [];
    for (const item of Array.isArray(saved.objectives) ? saved.objectives.slice(0, 200) : []) {
      try { this.objectives.push(normalizeObjective(item, item)); } catch {}
    }
    this.tradePlan = saved.tradePlan ?? null;
    this.tradingState = { running: false, results: [], options: {}, status: "", error: null,
      updatedAt: null, destination: saved.tradingDestination ?? null };
    this.remoteCache = readJson(this.cacheFile, []);
    this.listeners = new Set();
    this.journalObservers = new Set();
    this.sideObservers = new Set();
    this.positions = new Map();
    this.fileCommanders = new Map();
    this.sideContents = new Map();
    this.activeFid = null;
    this.busy = false;
    this.closed = false;
    this.rescanRequested = true;
    this.revision = 0;
    this.lastTimedEmit = 0;
    this.noticeList = [];
    this.searchState = { running: false, system: "", radiusLy: 50, checked: 0, total: 0, status: "", error: null, finishedAt: null };
    this.resetReplayState();
    this.timer = setInterval(() => void this.poll(), 750);
    this.timer.unref?.();
    void this.poll();
  }

  resetReplayState() {
    this.carrierTracker?.reset();
    this.tradeLedger?.reset();
    this.miningPlanner?.reset();
    this.engineeringPlanner?.reset();
    this.pilotTracker?.reset();
    this.world = new Map();
    this.signals = new Map();
    this.prices = new Map();
    this.rawInventory = new Map();
    this.statsRows = new Map();
    this.daily = new Map();
    this.recent = [];
    this.statsTotal = { refinedTons: 0, materialUnits: 0, soldTons: 0, revenue: 0, miningRevenue: 0,
      prospected: 0, accepted: 0, rejected: 0, asteroidsCracked: 0 };
    this.firstStatsEvent = null;
    this.cargoUpdatedAt = null;
    this.cargoSnapshots = new Map();
    this.cargoSync = new Map();
    this.locationUpdatedAt = null;
    this.game = { version: "", build: "", live: false, isBeta: false };
    this.journalTimes = new Map();
    this.localSignalKeys = new Set();
    this.commander = { name: "", fid: this.activeFid ?? "", system: "", systemAddress: null, coordinates: null,
      body: "", ship: "", vehicle: "unknown", docked: false, cargoCapacity: null, marketId: null, station: "" };
    this.connection = { journalDir: this.journalDir, scanning: false, filesRead: 0, filesTotal: 0,
      eventCount: 0, lastEvent: null, lastUpdate: null, error: null, gameRunning: null };
    this.prospect = null;
    this.surface = { site: "", body: "", system: "", lastRefined: [], timestamp: null };
    this.market = { station: "", system: "", timestamp: null, pricesCount: 0, error: null };
  }

  subscribe(callback) {
    this.listeners.add(callback);
    return () => this.listeners.delete(callback);
  }

  observeJournal(callback) {
    this.journalObservers.add(callback);
    return () => this.journalObservers.delete(callback);
  }

  observeSide(callback) {
    this.sideObservers.add(callback);
    return () => this.sideObservers.delete(callback);
  }

  emit() {
    this.revision += 1;
    this.cachedState = null;
    if (this.emitTimer || this.closed) return;
    this.emitTimer = setTimeout(() => {
      this.emitTimer = null;
      const state = this.getState();
      for (const callback of this.listeners) {
        try { callback(state); } catch {}
      }
    }, 120);
    this.emitTimer.unref?.();
  }

  savePreferences() {
    writeJson(this.configFile, { settings: this.preferences, journalDir: this.journalDir,
      bookmarks: this.bookmarks, manualPrices: this.manualPrices, statsSince: this.statsSince,
      carrier: this.carrierTracker.getConfiguration(), tradePlan: this.tradePlan,
      mining: this.miningPlanner.exportSettings(),
      engineering: this.engineeringPlanner.exportSettings(),
      objectives: this.objectives,
      tradingDestination: this.tradingState.destination ?? null });
  }

  notice(text, level = "info") {
    this.noticeList.unshift({ id: String(Date.now()) + ":" + this.noticeList.length, timestamp: nowIso(), text, level });
    this.noticeList = this.noticeList.slice(0, 20);
    this.emit();
  }

  settings(update) {
    const next = { ...this.preferences };
    if (["low", "medium", "high"].includes(update.minTier)) next.minTier = update.minTier;
    if (["all", "laser", "core", "subsurface", "surface", "surface-planet", "raw"].includes(update.method)) next.method = update.method;
    if (["flight", "asteroid", "surface", "raw", "trade", "carrier", "engineering"].includes(update.activity)) next.activity = update.activity;
    for (const [field, min, max] of [["radiusLy", 5, 100], ["maxArrivalLs", 100, 1000000], ["minLaserPercent", 1, 100]]) {
      const value = finite(update[field]);
      if (value != null) next[field] = Math.max(min, Math.min(max, value));
    }
    for (const field of ["hideDepleted", "overlayEnabled", "hideUnfocused"]) {
      if (typeof update[field] === "boolean") next[field] = update[field];
    }
    this.preferences = next;
    this.savePreferences();
    if (this.prospect) this.prospect = this.assessProspect(this.prospect);
    this.emit();
  }

  async setDirectory(folder) {
    const resolved = path.resolve(String(folder ?? ""));
    const stat = await fsp.stat(resolved).catch(() => null);
    if (!stat?.isDirectory()) throw new Error("Папка журнала не найдена.");
    while (this.busy && !this.closed) await waitForPoll();
    if (this.closed) throw new Error("Приложение завершает работу.");
    this.journalDir = resolved;
    this.fileCommanders.clear();
    this.connection.journalDir = resolved;
    this.savePreferences();
    this.rescanRequested = true;
    void this.poll();
  }

  getSystem(address, name, coordinates) {
    const key = address != null ? String(address) : "name:" + String(name).toLowerCase();
    let system = this.world.get(key);
    if (!system && name) system = [...this.world.values()].find((entry) => entry.name.toLowerCase() === String(name).toLowerCase());
    if (!system) {
      system = { key, address: address ?? null, name: name || this.commander.system || "Неизвестная система",
        coordinates: coordinates ?? null, bodies: new Map(), timestamp: nowIso() };
      this.world.set(key, system);
    }
    if (name) system.name = name;
    if (address != null) system.address = address;
    if (Array.isArray(coordinates) && coordinates.every(Number.isFinite)) system.coordinates = coordinates;
    return system;
  }

  eventSystem(event) {
    return this.getSystem(event.SystemAddress ?? this.commander.systemAddress,
      event.StarSystem ?? event.SystemName ?? this.commander.system, event.StarPos);
  }

  getBody(system, id, name) {
    const key = id != null ? String(id) : "name:" + name;
    let body = system.bodies.get(key);
    if (!body && name) body = [...system.bodies.values()].find((entry) => entry.name.toLowerCase() === String(name).toLowerCase());
    if (!body) {
      body = { id: id ?? key, name: name || system.name, rings: [], materials: [], source: "journal" };
      system.bodies.set(key, body);
    }
    if (name) body.name = name;
    return body;
  }

  addScan(event, source = "journal") {
    const system = this.eventSystem(event);
    const body = this.getBody(system, event.BodyID, event.BodyName);
    body.timestamp = event.timestamp ?? nowIso();
    body.source = source;
    body.remoteOnly = source !== "journal";
    if (typeof event.Landable === "boolean") body.landable = event.Landable;
    if (event.PlanetClass) body.planetClass = event.PlanetClass;
    if (finite(event.SurfaceGravity) != null) body.gravity = event.SurfaceGravity / 9.80665;
    if (finite(event.DistanceFromArrivalLS) != null) body.distanceLs = event.DistanceFromArrivalLS;
    if (event.ReserveLevel) body.reserve = event.ReserveLevel;
    if (event.Materials) {
      const materials = Array.isArray(event.Materials) ? event.Materials
        : Object.entries(event.Materials).map(([Name, Percent]) => ({ Name, Percent }));
      body.materials = materials.map((entry) => ({ key: keyOf(entry.Name), percent: finite(entry.Percent, 0) }));
    }
    if (Array.isArray(event.Rings)) {
      body.rings = event.Rings.filter((ring) => ring.Name).map((ring) => ({
        name: ring.Name, type: ring.RingClass ?? "", inner: ring.InnerRad, outer: ring.OuterRad,
      }));
    }
    return body;
  }

  resourceRow(key, name) {
    let row = this.statsRows.get(key);
    if (!row) {
      row = { key, name, refinedTons: 0, materialUnits: 0, soldTons: 0, revenue: 0 };
      this.statsRows.set(key, row);
    }
    return row;
  }

  statistical(event) {
    return !this.statsSince || String(event.timestamp ?? "") >= this.statsSince;
  }

  record(event, kind, resource, count = 1, value = 0) {
    if (!this.statistical(event)) return;
    const timestamp = event.timestamp || nowIso();
    this.firstStatsEvent ??= timestamp;
    const date = timestamp.slice(0, 10);
    let day = this.daily.get(date);
    if (!day) {
      day = { date, refinedTons: 0, materialUnits: 0, prospected: 0, revenue: 0 };
      this.daily.set(date, day);
    }
    const key = resource?.key ?? kind;
    const name = resource?.name ?? ({ prospect: "Исследован астероид", cracked: "Расколото ядро" }[kind] ?? kind);
    const row = resource ? this.resourceRow(key, name) : null;
    if (kind === "refined") {
      this.statsTotal.refinedTons += count;
      day.refinedTons += count;
      row.refinedTons += count;
    } else if (kind === "material") {
      this.statsTotal.materialUnits += count;
      day.materialUnits += count;
      row.materialUnits += count;
    } else if (kind === "sale") {
      this.statsTotal.revenue += value;
      this.statsTotal.soldTons += count;
      day.revenue += value;
      row.soldTons += count;
      row.revenue += value;
    } else if (kind === "prospect") {
      this.statsTotal.prospected += 1;
      if (this.prospect?.decision === "mine") this.statsTotal.accepted += 1;
      if (this.prospect?.decision === "skip") this.statsTotal.rejected += 1;
      day.prospected += 1;
    } else if (kind === "cracked") this.statsTotal.asteroidsCracked += 1;
    this.recent.unshift({ timestamp, kind, key, name, count, value, system: this.commander.system });
    this.recent = this.recent.slice(0, 120);
  }

  assessProspect(prospect) {
    const priced = prospect.materials.map((entry) => {
      const resource = resourceFor(entry.key);
      return { ...entry, name: resource?.name ?? entry.name, tier: resource?.tier ?? "unknown",
        price: this.referencePrice(entry.key)?.price ?? null };
    }).sort((a, b) => TIER[b.tier] - TIER[a.tier] || b.percent - a.percent);
    const core = resourceFor(prospect.motherlode);
    const threshold = TIER[this.preferences.minTier];
    const method = this.preferences.method;
    const allowCore = method === "all" || method === "core";
    const allowLaser = method === "all" || method === "laser";
    const laser = priced.filter((entry) => TIER[entry.tier] >= threshold && entry.percent >= this.preferences.minLaserPercent
      && (resourceFor(entry.key)?.methods ?? []).includes("laser"));
    const reasons = [];
    let decision = "skip";
    if (allowCore && core && TIER[core.tier] >= threshold && !prospect.coreCracked) {
      decision = "mine";
      reasons.push("Ядро: " + core.name + ". Нужен комплект для подрыва.");
    }
    if (allowLaser && laser.length && prospect.remaining !== 0) {
      decision = "mine";
      reasons.push("Подходящий лазерный состав: " + laser.map((entry) => entry.name + " " + entry.percent + "%").join(", "));
    }
    if (prospect.remaining === 0 && decision !== "mine") {
      decision = "skip";
      reasons.unshift("Астероид выработан.");
    } else if (decision === "skip") {
      reasons.push("Нет ресурсов выбранного приоритета с нужной долей. Порог для лазеров — " + this.preferences.minLaserPercent + "%.");
    }
    if (!priced.length && !core) {
      decision = "unknown";
      reasons.splice(0, reasons.length, "Недостаточно данных о составе.");
    }
    if (!allowLaser && !allowCore) {
      decision = "unknown";
      reasons.splice(0, reasons.length, "Доля веществ разведчика не подтверждает отдельные залежи для выбранного метода. Нужна проверка астероида подходящим сканером.");
    } else if (prospect.coreCracked && allowCore && !allowLaser) {
      decision = "unknown";
      reasons.splice(0, reasons.length, "После последней разведки зарегистрирован раскол астероида. Проверьте, осталось ли подходящее ядро.");
    }
    const known = priced.filter((entry) => entry.price != null && (resourceFor(entry.key)?.methods ?? []).includes("laser"));
    return { ...prospect, materials: priced, decision, reasons,
      estimatedPerTonne: known.length ? Math.round(known.reduce((sum, entry) => sum + entry.price * entry.percent / 100, 0)) : null };
  }

  apply(event) {
    if (!event || typeof event.event !== "string") return;
    this.connection.eventCount += 1;
    this.connection.lastEvent = event.timestamp ?? this.connection.lastEvent;
    this.connection.lastUpdate = nowIso();
    const type = event.event;
    if (type === "Fileheader") {
      const version = String(event.gameversion || "");
      this.game = { version, build: String(event.build || ""),
        isBeta: /beta|test/i.test(version), live: /^4\./.test(version) && !/beta|test/i.test(version) };
    }
    if (type === "LoadGame") {
      for (const key of ["Horizons", "Odyssey"]) if (typeof event[key] === "boolean") this.game[key.toLowerCase()] = event[key];
      if (event.Beta === true) { this.game.isBeta = true; this.game.live = false; }
      this.game.loan = Number.isFinite(event.Loan) ? event.Loan : 0;
    }
    if (["JoinACrew", "CrewJoin"].includes(type)) this.game.multicrew = true;
    if (["QuitACrew", "CrewQuit", "LoadGame"].includes(type)) this.game.multicrew = false;
    if (["Commander", "LoadGame"].includes(type)) {
      if (event.FID && this.activeFid && this.activeFid !== event.FID) {
        this.activeFid = event.FID;
        this.rescanRequested = true;
        return;
      }
      this.commander.name = event.Name ?? event.Commander ?? this.commander.name;
      this.commander.fid = event.FID ?? this.commander.fid;
    }
    this.carrierTracker.apply(event, { commander: this.commander });
    this.miningPlanner.apply(event, { commander: this.commander, countInPeriod: this.statistical(event) });
    this.engineeringPlanner.apply(event, { commander: this.commander });
    this.pilotTracker.apply(event, { commander: this.commander });
    if (["NavRoute", "NavRouteClear"].includes(type)) this.sideContents.delete("NavRoute.json");
    const tradeResult = this.tradeLedger.apply(event, { commander: this.commander,
      carrierId: event.CarrierID ?? (type === "CargoTransfer" ? this.carrierTracker.dockedCarrierId : this.carrierTracker.ownerId),
      ownMarket: this.carrierTracker.isOwnDockedMarket(event.MarketID ?? this.commander.marketId),
      station: this.commander.station, countInPeriod: this.statistical(event) });
    if (["MarketBuy", "MarketSell", "MiningRefined", "Cargo", "CollectCargo", "EjectCargo", "CargoTransfer", "CarrierDepositFuel"].includes(type)) {
      this.cargoUpdatedAt = event.timestamp ?? this.cargoUpdatedAt;
      if (type === "CargoTransfer") this.cargoSync.clear();
      else if (type !== "Cargo") this.cargoSync.delete(this.commander.vehicle === "srv" ? "srv" : "ship");
    }
    if (["Location", "FSDJump", "CarrierJump"].includes(type)) {
      this.locationUpdatedAt = event.timestamp || null;
      const previous = this.commander.systemAddress;
      this.commander.system = event.StarSystem ?? this.commander.system;
      this.commander.systemAddress = event.SystemAddress ?? this.commander.systemAddress;
      this.commander.coordinates = event.StarPos ?? this.commander.coordinates;
      if (typeof event.Docked === "boolean") {
        this.commander.docked = event.Docked;
        this.commander.station = event.Docked ? event.StationName ?? "" : "";
        this.commander.marketId = event.Docked ? event.MarketID ?? null : null;
      } else if (type === "FSDJump") {
        this.commander.docked = false; this.commander.station = ""; this.commander.marketId = null;
      }
      if (previous !== this.commander.systemAddress) {
        this.prospect = null;
        this.commander.body = "";
        this.surface = { site: "", body: "", system: this.commander.system, lastRefined: [], timestamp: event.timestamp };
      }
      if (event.BodyType === "Planet") this.commander.body = event.Body ?? "";
      this.eventSystem(event);
    }
    if (["LoadGame", "Loadout"].includes(type)) {
      this.commander.ship = event.Ship_Localised ?? event.Ship ?? this.commander.ship;
      this.commander.shipType = event.Ship ?? this.commander.shipType;
      this.commander.shipId = event.ShipID ?? this.commander.shipId;
      if (type === "LoadGame") this.commander.vehicle = /srv|buggy/i.test(event.Ship || "") ? "srv" : /onfoot/i.test(event.Ship || "") ? "foot" : "ship";
      this.commander.cargoCapacity = finite(event.CargoCapacity, this.commander.cargoCapacity);
    }
    if (["ApproachBody", "Touchdown", "SupercruiseExit", "ApproachSettlement"].includes(type)) {
      this.commander.body = event.Body ?? event.BodyName ?? this.commander.body;
      if (/PlanetaryMiningLocation/.test(event.NearestDestination ?? "")) {
        const site = /#index=(\d+)/.exec(event.NearestDestination)?.[1];
        this.surface = { site: site ? "Участок " + site : "Участок планетарной добычи",
          body: this.commander.body, system: this.commander.system, lastRefined: [], timestamp: event.timestamp };
      }
    }
    if (type === "Docked") {
      this.locationUpdatedAt = event.timestamp || this.locationUpdatedAt;
      this.commander.docked = true;
      this.commander.marketId = event.MarketID ?? null;
      this.commander.station = event.StationName ?? "";
    }
    if (type === "Undocked") { this.commander.docked = false; this.commander.marketId = null; this.commander.station = ""; }
    if (type === "Disembark") this.commander.vehicle = "foot";
    if (type === "Embark" || type === "LaunchSRV") this.commander.vehicle = event.SRV || type === "LaunchSRV" ? "srv" : "ship";
    if (type === "DockSRV") this.commander.vehicle = "ship";
    if (type === "Scan") this.addScan(event);
    if (["SAASignalsFound", "FSSBodySignals"].includes(type)) {
      const system = this.eventSystem(event);
      if (event.BodyName && Array.isArray(event.Signals)) {
        const signalKey = system.key + ":" + event.BodyName;
        this.signals.set(signalKey, event.Signals);
        this.localSignalKeys.add(signalKey);
        if (!/Ring$/.test(event.BodyName)) this.getBody(system, event.BodyID, event.BodyName).timestamp = event.timestamp;
      }
    }
    if (type === "ProspectedAsteroid") {
      const materials = (event.Materials ?? []).map((entry) => {
        const resource = resourceFor(entry.Name);
        return { key: resource?.key ?? keyOf(entry.Name), name: displayName(entry.Name, entry.Name_Localised),
          percent: finite(entry.Proportion, 0), tier: resource?.tier ?? "unknown" };
      });
      this.prospect = this.assessProspect({ timestamp: event.timestamp, system: this.commander.system,
        body: this.commander.body, remaining: finite(event.Remaining), content: event.Content_Localised ?? event.Content ?? "",
        motherlode: resourceFor(event.MotherlodeMaterial)?.key ?? keyOf(event.MotherlodeMaterial), materials });
      this.record(event, "prospect");
    }
    if (type === "AsteroidCracked") {
      this.record(event, "cracked");
      if (this.prospect) this.prospect = this.assessProspect({ ...this.prospect, coreCracked: true });
    }
    if (type === "MiningRefined") {
      const resource = resourceFor(event.Type);
      const key = resource?.key ?? keyOf(event.Type);
      const name = displayName(event.Type, event.Type_Localised);
      const count = Math.max(0, finite(event.Count, 1));
      this.record(event, "refined", resource ?? { key, name }, count);
      if (this.commander.vehicle === "srv") {
        let item = this.surface.lastRefined.find((entry) => entry.key === key);
        if (!item) {
          item = { key, name, count: 0 };
          this.surface.lastRefined.push(item);
        }
        item.count += count;
        this.surface.body = this.commander.body;
        this.surface.system = this.commander.system;
        this.surface.timestamp = event.timestamp;
        if (this.commander.system && this.commander.body) {
          const body = this.getBody(this.eventSystem(event), null, this.commander.body);
          body.surfaceFinds ??= [];
          const site = this.surface.site || "Участок не определён";
          let finding = body.surfaceFinds.find((entry) => entry.site === site);
          if (!finding) {
            finding = { site, resources: [], timestamp: event.timestamp };
            body.surfaceFinds.push(finding);
          }
          let product = finding.resources.find((entry) => entry.key === key);
          if (!product) {
            product = { key, name, count: 0 };
            finding.resources.push(product);
          }
          product.count += count;
          finding.timestamp = event.timestamp;
        }
      }
    }
    if (type === "MaterialCollected") {
      const resource = resourceFor(event.Name);
      if (resource?.kind === "raw" || String(event.Category).toLowerCase() === "raw") {
        const key = resource?.key ?? keyOf(event.Name);
        const name = displayName(event.Name, event.Name_Localised);
        const count = Math.max(0, finite(event.Count, 1));
        this.rawInventory.set(key, { key, name, grade: resource?.grade ?? 1,
          count: (this.rawInventory.get(key)?.count ?? 0) + count });
        this.record(event, "material", resource ?? { key, name }, count);
      }
    }
    if (type === "Materials" && Array.isArray(event.Raw)) {
      this.rawInventory.clear();
      for (const item of event.Raw) {
        const resource = resourceFor(item.Name);
        const key = resource?.key ?? keyOf(item.Name);
        this.rawInventory.set(key, { key, name: displayName(item.Name, item.Name_Localised),
          count: item.Count ?? 0, grade: resource?.grade ?? 1 });
      }
    }
    if (type === "Cargo" && Array.isArray(event.Inventory)) this.applyCargo(event);
    if (["MarketBuy", "CollectCargo", "EjectCargo", "MarketSell"].includes(type)) {
      const resource = resourceFor(event.Type);
      const key = resource?.key ?? keyOf(event.Type);
      const count = finite(event.Count, 1);
      if (type === "MarketSell" && resource?.kind === "commodity" && !tradeResult.internal) {
        const value = finite(event.TotalSale, finite(event.SellPrice, 0) * count);
        if (this.statistical(event)) this.statsTotal.miningRevenue += tradeResult.minedRevenue;
        this.record(event, "sale", resource, count, value);
      }
    }
    if (type === "Shutdown") this.connection.gameRunning = false;
    else this.connection.gameRunning = true;
  }

  applyCargo(event) {
    if (!Array.isArray(event.Inventory)) return;
    const vessel = /srv|buggy/i.test(event.Vessel || this.commander.vehicle) ? "srv" : "ship";
    this.cargoSnapshots.set(vessel, event.timestamp || null);
    this.cargoSync.set(vessel, { timestamp: event.timestamp || null, inventory: event.Inventory.map(row => ({ ...row })) });
    this.cargoUpdatedAt = event.timestamp || this.cargoUpdatedAt;
    this.tradeLedger.reconcile(event, { commander: this.commander });
  }

  applyMarket(event) {
    if (!Array.isArray(event.Items)) return;
    this.tradeLedger.market(event);
    for (const item of event.Items) {
      const resource = this.tradeLedger.good(item.Name, item.Name_Localised);
      if (!resource || !(item.SellPrice > 0)) continue;
      this.prices.set(resource.key, { price: item.SellPrice, priceSource: (event.Source ? event.Source + " · " : "") + (event.StationName || "Последний рынок"),
        priceAt: event.timestamp ?? nowIso(), demand: item.Demand ?? null });
    }
    this.market = { station: event.StationName ?? "", system: event.StarSystem ?? this.commander.system, source: event.Source || "Журнал игры",
      timestamp: event.timestamp ?? null, pricesCount: this.prices.size, error: null };
  }

  referencePrice(key) {
    const override = this.manualPrices[key];
    return override ? { price: override.price, priceAt: override.timestamp, priceSource: "Введено вручную" } : this.prices.get(key) ?? null;
  }

  async readSideFiles({ replay = false } = {}) {
    for (const filename of ["Status.json", "Cargo.json", "Market.json", "NavRoute.json", "Outfitting.json", "Shipyard.json", "ShipLocker.json"]) {
      const raw = await fsp.readFile(path.join(this.journalDir, filename), "utf8").catch(() => null);
      if (!raw || this.sideContents.get(filename) === raw) continue;
      let data;
      try { data = JSON.parse(raw); } catch { continue; }
      this.sideContents.set(filename, raw);
      if (filename === "Cargo.json" && (!this.cargoUpdatedAt || !data.timestamp || data.timestamp >= this.cargoUpdatedAt)) {
        this.applyCargo(data);
        this.miningPlanner.apply({ ...data, event: "Cargo" }, { commander: this.commander });
      }
      if (filename === "Market.json") this.applyMarket(data);
      if (filename === "NavRoute.json") this.pilotTracker.route(data, this.commander);
      if (filename === "Status.json") {
        this.pilotTracker.status(data);
        const flags = Number(data.Flags);
        const flags2 = Number(data.Flags2) || 0;
        if (Number.isSafeInteger(flags)) {
          this.commander.vehicle = flags2 & 1 ? "foot" : flags & 0x04000000 ? "srv"
            : (flags & 0x03000000) || (flags2 & (2 | 4 | 4096 | 131072 | 262144)) ? "ship" : this.commander.vehicle;
          this.commander.docked = Boolean(flags & 1);
        }
        if (typeof data.BodyName === "string") this.commander.body = data.BodyName;
        if (/PlanetaryMiningLocation/.test(data.Destination?.Name ?? "")) {
          const index = /#index=(\d+)/.exec(data.Destination.Name)?.[1];
          const site = index ? "Участок " + index : "Участок планетарной добычи";
          if (this.surface.site !== site || this.surface.body !== this.commander.body || this.surface.system !== this.commander.system) this.surface.lastRefined = [];
          this.surface.site = site;
          this.surface.body = this.commander.body;
          this.surface.system = this.commander.system;
        } else if (this.surface.body !== this.commander.body || this.surface.system !== this.commander.system) {
          this.surface = { site: "", body: this.commander.body, system: this.commander.system, lastRefined: [], timestamp: data.timestamp };
        }
      }
      this.emit();
      for (const callback of this.sideObservers) {
        try { callback({ ...data, event: data.event || filename.replace(".json", "") }, { replay }); } catch {}
      }
    }
  }

  async discoverFiles() {
    const entries = await fsp.readdir(this.journalDir).catch(() => null);
    if (!entries) throw new Error("Папка игровых журналов недоступна. Выберите её в настройках.");
    const filenames = entries.filter((name) => /^Journal.*\.log$/i.test(name)).sort();
    for (const name of filenames) {
      if (this.fileCommanders.get(name)) continue;
      const handle = await fsp.open(path.join(this.journalDir, name), "r");
      try {
        const buffer = Buffer.alloc(32768);
        const { bytesRead } = await handle.read(buffer, 0, buffer.length, 0);
        const lines = buffer.subarray(0, bytesRead).toString("utf8").split("\n");
        let fid = null;
        for (const line of lines) {
          let event;
          try { event = JSON.parse(line.replace(/^\uFEFF/, "")); } catch { continue; }
          if (["Commander", "LoadGame"].includes(event.event) && event.FID) { fid = event.FID; break; }
        }
        this.fileCommanders.set(name, fid);
      } finally { await handle.close(); }
    }
    return filenames;
  }

  async readJournal(name) {
    const filepath = path.join(this.journalDir, name);
    const handle = await fsp.open(filepath, "r");
    try {
      const stat = await handle.stat();
      let position = this.positions.get(name) ?? 0;
      this.positions.set(name, position);
      if (position > stat.size) {
        this.fileCommanders.delete(name);
        this.rescanRequested = true;
        return;
      }
      while (position < stat.size) {
        let length = Math.min(131072, stat.size - position);
        let buffer, bytesRead, end;
        do {
          buffer = Buffer.alloc(length);
          ({ bytesRead } = await handle.read(buffer, 0, buffer.length, position));
          end = buffer.lastIndexOf(10, bytesRead - 1);
          length = Math.min(length * 2, stat.size - position);
        } while (bytesRead === buffer.length && end < 0 && bytesRead < stat.size - position);
        if (!bytesRead) break;
        const consumed = end < 0 ? bytesRead : end + 1;
        const lines = buffer.subarray(0, consumed).toString("utf8").split("\n");
        let incomplete = false;
        let lineOffset = position;
        for (const line of lines) {
          const offset = lineOffset;
          lineOffset += Buffer.byteLength(line, "utf8") + 1;
          if (!line.trim()) continue;
          let event;
          try { event = JSON.parse(line.replace(/^\uFEFF/, "")); }
          catch {
            if (end < 0) incomplete = true;
            continue;
          }
          const timestamp = Date.parse(event.timestamp);
          if (Number.isFinite(timestamp)) {
            const interval = this.journalTimes.get(name) ?? { start: timestamp, end: timestamp };
            interval.start = Math.min(interval.start, timestamp);
            interval.end = Math.max(interval.end, timestamp);
            this.journalTimes.set(name, interval);
          }
          this.apply(event);
          this.cachedState = null;
          for (const callback of this.journalObservers) {
            try { callback(event, { file: name, offset, replay: this.replaying === true }); } catch {}
          }
        }
        if (incomplete) break;
        position += consumed;
        this.positions.set(name, position);
        if (this.replaying) await later();
      }
    } catch (error) {
      this.rescanRequested = true;
      throw error;
    } finally { await handle.close(); }
  }

  async replay(filenames) {
    this.rescanRequested = false;
    const fids = filenames.map((file) => this.fileCommanders.get(file)).filter(Boolean);
    this.activeFid = fids.at(-1) ?? null;
    this.resetReplayState();
    this.positions.clear();
    this.sideContents.clear();
    this.replaying = true;
    this.connection.scanning = true;
    const included = filenames.filter((file) => !this.fileCommanders.get(file) || this.fileCommanders.get(file) === this.activeFid);
    this.connection.filesTotal = included.length;
    this.emit();
    try {
      for (const name of included) {
        if (this.closed) return;
        await this.readJournal(name);
        this.connection.filesRead += 1;
        this.emit();
      }
      for (const entry of this.remoteCache) this.mergeRemoteSystem(entry, false);
    } finally {
      this.replaying = false;
      this.connection.scanning = false;
      this.emit();
    }
  }

  async poll() {
    if (this.closed || this.busy) return;
    this.busy = true;
    try {
      const filenames = await this.discoverFiles();
      const latestFid = filenames.map((file) => this.fileCommanders.get(file)).filter(Boolean).at(-1);
      if (latestFid && latestFid !== this.activeFid) this.rescanRequested = true;
      const known = [...this.positions.keys()].sort();
      if (known.some((file) => !filenames.includes(file)) ||
          filenames.some((file) => !this.positions.has(file) && file < known.at(-1))) this.rescanRequested = true;
      const replay = this.rescanRequested;
      if (replay) await this.replay(filenames);
      else {
        const previousCount = this.connection.eventCount;
        for (const name of filenames) {
          if (this.fileCommanders.get(name) && this.fileCommanders.get(name) !== this.activeFid) continue;
          await this.readJournal(name);
        }
        if (this.connection.eventCount !== previousCount) this.emit();
      }
      await this.readSideFiles({ replay });
      if ((this.carrierTracker.ownerId || this.tradingState.destination || this.tradingState.results.length || this.miningPlanner.current)
        && Date.now() - this.lastTimedEmit > 10000) {
        this.lastTimedEmit = Date.now();
        this.emit();
      }
      this.connection.error = null;
    } catch (error) {
      this.connection.error = error.message;
      this.emit();
    } finally { this.busy = false; }
  }

  mergeRemoteSystem(entry, persist = true) {
    const previous = this.remoteCache.find((record) => record.name.toLowerCase() === entry.name.toLowerCase());
    entry = mergeRemoteRecords(previous, entry);
    const system = this.getSystem(entry.address, entry.name, entry.coordinates);
    for (const remote of entry.bodies ?? []) {
      const body = this.getBody(system, remote.id, remote.name);
      if (body.source === "journal" && body.timestamp && !body.remoteOnly) {
        // Community data supplements an existing local scan without replacing its newer fields.
        if (!body.rings.length) body.rings = remote.rings ?? [];
        if (!body.materials.length) body.materials = remote.materials ?? [];
        for (const field of ["landable", "gravity", "distanceLs", "reserve", "planetClass"]) {
          if (body[field] == null && remote[field] != null) body[field] = remote[field];
        }
      } else Object.assign(body, structuredClone(remote), { id: body.id, source: remote.source ?? "edsm", remoteOnly: true });
      const bodySignalKey = system.key + ":" + body.name;
      if (remote.signals?.length && !this.localSignalKeys.has(bodySignalKey)) this.signals.set(bodySignalKey, remote.signals);
      for (const ring of remote.rings ?? []) {
        const key = system.key + ":" + ring.name;
        if (ring.signals?.length && !this.localSignalKeys.has(key)) {
          const combined = new Map((this.signals.get(key) ?? []).map((signal) => [keyOf(signal.key ?? signal.Type ?? signal.name), signal]));
          for (const signal of ring.signals) combined.set(keyOf(signal.key ?? signal.Type ?? signal.name), signal);
          this.signals.set(key, [...combined.values()]);
        }
      }
    }
    if (persist) {
      this.remoteCache = this.remoteCache.filter((record) => record.name.toLowerCase() !== entry.name.toLowerCase());
      this.remoteCache.push(entry);
      this.remoteCache = this.remoteCache.slice(-300);
      writeJson(this.cacheFile, this.remoteCache);
      this.emit();
    }
  }

  getState() {
    if (this.cachedState) return this.cachedState;
    const resources = catalog.resources.map((resource) => ({ ...resource,
      price: this.referencePrice(resource.key)?.price ?? null,
      priceSource: this.referencePrice(resource.key)?.priceSource ?? "",
      priceAt: this.referencePrice(resource.key)?.priceAt ?? null }));
    const targets = buildTargets(this.world, this.signals, resources, this.commander, this.preferences);
    const ledger = this.tradeLedger.snapshot(this.commander);
    const goods = ledger.goods.map((good) => ({ ...good,
      price: this.referencePrice(good.key)?.price ?? null,
      priceSource: this.referencePrice(good.key)?.priceSource ?? "",
      priceAt: this.referencePrice(good.key)?.priceAt ?? null }));
    const cargo = ledger.inventory.map(({ key, name, count }) => ({ key, name, count }));
    const engineering = this.engineeringPlanner.snapshot();
    const materials = engineering.shopping.knownCategories.raw
      ? engineering.materials.filter((item) => item.category === "raw" && item.owned > 0)
        .map((item) => ({ key: item.key, name: item.name, grade: item.grade, count: item.owned }))
        .sort((a, b) => b.grade - a.grade || b.count - a.count)
      : [...this.rawInventory.values()].sort((a, b) => b.grade - a.grade || b.count - a.count);
    const since = this.statsSince ?? this.firstStatsEvent ?? nowIso();
    const startTime = new Date(since).getTime();
    const intervals = [...this.journalTimes.values()].map((interval) => ({
      start: Math.max(startTime, interval.start), end: interval.end,
    })).filter((interval) => interval.end > interval.start).sort((a, b) => a.start - b.start);
    let elapsedMs = 0;
    let lastEnd = -Infinity;
    for (const interval of intervals) {
      elapsedMs += Math.max(0, interval.end - Math.max(interval.start, lastEnd));
      lastEnd = Math.max(lastEnd, interval.end);
    }
    const elapsedSeconds = elapsedMs / 1000;
    this.cachedState = {
      revision: this.revision, app: { name: "ED Copilot", version: "1.5.0", profileDir: this.dataDir,
        sources: catalog.sources, qualityGuidance: catalog.qualityGuidance },
      connection: { ...this.connection }, settings: { ...this.preferences },
      commander: { ...this.commander, cargo, cargoTons: cargo.reduce((sum, item) => sum + item.count, 0), materials,
        cargoUpdatedAt: this.cargoSnapshots.has(this.commander.vehicle) ? this.cargoUpdatedAt : null,
        cargoForSync: this.cargoSync.get(this.commander.vehicle)?.inventory,
        cargoForSyncAt: this.cargoSync.get(this.commander.vehicle)?.timestamp,
        cargoVessel: this.commander.vehicle, locationUpdatedAt: this.locationUpdatedAt },
      game: { ...this.game },
      system: { name: this.commander.system, bodiesScanned: [...this.world.values()].find((system) => system.name === this.commander.system)?.bodies.size ?? 0,
        rings: targets.filter((target) => target.system === this.commander.system && target.kind === "ring").length,
        surfaceTargets: targets.filter((target) => target.system === this.commander.system && target.kind === "surface").length },
      targets, prospect: this.prospect, surface: this.surface, resources,
      goods, tradeLedger: ledger, carrier: this.carrierTracker.getState(),
      mining: this.miningPlanner.snapshot({ commander: { ...this.commander, cargo, cargoTons: cargo.reduce((sum, item) => sum + item.count, 0), materials }, resources, settings: this.preferences, gameRunning: this.connection.gameRunning, now: nowIso() }),
      engineering,
      pilot: this.pilotTracker.snapshot(), objectives: this.objectives.map((item) => ({ ...item })),
      carrierRoute: this.carrierRouteState ?? null, frontier: this.frontierStatus?.() ?? this.frontierState ?? null,
      integrations: this.integrationsStatus?.({ commander: this.commander, game: this.game, connection: this.connection }) ?? null,
      tradeRoutes: this.tradeRoutesState ?? null,
      trading: { ...this.tradingState, results: this.tradingState.results.map((row) => this.quoteState(row)),
        destination: this.destinationState(cargo) }, tradePlan: this.tradePlan,
      stats: { since, total: { ...this.statsTotal }, byResource: [...this.statsRows.values()].sort((a, b) => b.refinedTons - a.refinedTons),
        sessions: [...this.daily.values()].sort((a, b) => b.date.localeCompare(a.date)),
        recent: this.recent, elapsedSeconds, timeBasis: "journal-sessions",
        revenuePerHour: elapsedSeconds >= 60 ? this.statsTotal.miningRevenue / elapsedSeconds * 3600 : null },
      bookmarks: this.bookmarks.map((entry) => ({ ...entry, target: targets.find((target) => target.id === entry.targetId) ?? null })),
      market: this.market, search: this.searchState, notices: this.noticeList,
      methods: catalog.methods, tierLabels: { ...catalog.tierLabels, unknown: "Нужна оценка" },
    };
    return this.cachedState;
  }

  bookmark(action, id) {
    if (action === "remove") this.bookmarks = this.bookmarks.filter((entry) => entry.id !== id);
    else if (action === "add") {
      const target = this.getState().targets.find((entry) => entry.id === id);
      if (!target) throw new Error("Цель больше не найдена. Обновите список.");
      if (!this.bookmarks.some((entry) => entry.targetId === id)) {
        this.bookmarks.push({ id: "b:" + Date.now(), targetId: id, label: target.ring || target.body,
          system: target.system, body: target.body, ring: target.ring, kind: target.kind, addedAt: nowIso() });
      }
    } else throw new Error("Неизвестное действие.");
    this.savePreferences();
    this.emit();
  }

  resetSession() {
    this.statsSince = nowIso();
    this.savePreferences();
    this.rescanRequested = true;
    void this.poll();
  }

  setPrices(entries) {
    if (!Array.isArray(entries)) throw new Error("Нужен список цен.");
    for (const entry of entries) {
      const resource = resourceFor(entry.key) ?? this.tradeLedger.goods.get(keyOf(entry.key));
      if (!resource || resource.kind !== "commodity") continue;
      if (entry.price == null || entry.price === "") delete this.manualPrices[resource.key];
      else {
        const price = Number(entry.price);
        if (!Number.isFinite(price) || price <= 0 || price > 1000000000) throw new Error("Цена должна быть положительным числом.");
        this.manualPrices[resource.key] = { price, timestamp: nowIso() };
      }
    }
    this.savePreferences();
    if (this.prospect) this.prospect = this.assessProspect(this.prospect);
    this.emit();
  }

  destinationState(cargo) {
    const destination = this.tradingState.destination;
    if (!destination) return null;
    const manifest = cargo.map((item) => item.key + ":" + item.count).sort().join("|");
    return { ...this.quoteState(destination),
      distanceLy: distance(this.commander.coordinates, destination.coordinates) ?? destination.distanceLy,
      cargoChanged: destination.cargoManifest != null && manifest !== destination.cargoManifest };
  }

  quoteState(row) {
    const timestamp = Date.parse(row.updatedAt);
    const ageHours = Number.isFinite(timestamp) ? Math.max(0, (Date.now() - timestamp) / 3600000) : null;
    const options = this.tradingState.options ?? {};
    const maximum = row.carrier ? options.carrierMaxAgeHours ?? 6 : options.maxAgeHours ?? 24;
    return { ...row, ageHours, expired: ageHours == null || ageHours > maximum,
      quotes: (row.quotes ?? []).map((quote) => ({ ...quote,
        ageHours: Number.isFinite(Date.parse(quote.updatedAt)) ? Math.max(0, (Date.now() - Date.parse(quote.updatedAt)) / 3600000) : ageHours })) };
  }

  setTradingDestination(destination) {
    if (destination == null) this.tradingState.destination = null;
    else {
      if (typeof destination.system !== "string" || typeof destination.station !== "string"
        || destination.system.length > 150 || destination.station.length > 150) throw new Error("Укажите систему и станцию.");
      const cargoManifest = this.getState().commander.cargo.map((item) => item.key + ":" + item.count).sort().join("|");
      this.tradingState.destination = { ...destination, cargoManifest, selectedAt: nowIso() };
    }
    this.savePreferences();
    this.emit();
  }

  setTradePlan(input) {
    const good = this.tradeLedger.goods.get(keyOf(input.key)) ?? resourceFor(input.key);
    const quantity = Number(input.quantity);
    const buyPrice = Number(input.buyPrice);
    const sellPrice = Number(input.sellPrice);
    if (!good || ![quantity, buyPrice, sellPrice].every(Number.isFinite) || quantity <= 0
      || quantity > 25000 || buyPrice < 0 || sellPrice < 0 || buyPrice > 1e9 || sellPrice > 1e9) {
      throw new Error("Проверьте товар, тоннаж и цены в плане.");
    }
    this.tradePlan = { key: good.key, name: good.name, quantity, buyPrice, sellPrice,
      purchaseCost: quantity * buyPrice, revenue: quantity * sellPrice, grossMargin: quantity * (sellPrice - buyPrice),
      source: String(input.source ?? "").slice(0, 200), destination: String(input.destination ?? "").slice(0, 200), updatedAt: nowIso() };
    this.savePreferences();
    this.emit();
  }

  setCarrierPlanning(input) {
    this.carrierTracker.updatePlanning(input);
    this.savePreferences();
    this.emit();
  }

  setMiningPlan(input) {
    this.miningPlanner.configure(input);
    this.savePreferences();
    this.emit();
  }

  updateEngineering(input) {
    switch (input.action) {
      case "add": this.engineeringPlanner.add(input); break;
      case "update": this.engineeringPlanner.update(input); break;
      case "remove": this.engineeringPlanner.remove(input.id); break;
      case "configure": this.engineeringPlanner.configure(input); break;
      case "engineer": this.engineeringPlanner.setEngineer(input); break;
      case "import": this.engineeringPlanner.importPlan(input.plan); break;
      default: throw new Error("Неизвестное действие инженерного планировщика.");
    }
    this.savePreferences();
    this.emit();
  }

  updateObjective(input) {
    if (input.action === "add") {
      if (this.objectives.length >= 200) throw new Error("В плане допускается до 200 целей. Уберите ненужные записи.");
      this.objectives.push(normalizeObjective(input));
    } else {
      const index = this.objectives.findIndex((row) => row.id === input.id);
      if (index < 0) throw new Error("Цель не найдена.");
      if (input.action === "remove") this.objectives.splice(index, 1);
      else if (input.action === "update") this.objectives[index] = normalizeObjective(input, this.objectives[index]);
      else if (input.action === "status" && ["active", "completed"].includes(input.status)) {
        this.objectives[index].status = input.status;
        this.objectives[index].completedAt = input.status === "completed" ? nowIso() : null;
      } else throw new Error("Неизвестное действие с целью.");
    }
    this.savePreferences(); this.emit();
  }

  resetMiningRun() {
    this.miningPlanner.resetRun(nowIso());
    this.savePreferences();
    this.rescanRequested = true;
    void this.poll();
  }

  setCarrierStock(input) {
    this.carrierTracker.setStockBaseline(input);
    this.savePreferences();
    this.rescanRequested = true;
    void this.poll();
  }

  close() {
    this.closed = true;
    clearInterval(this.timer);
    clearTimeout(this.emitTimer);
    this.listeners.clear();
  }
}

module.exports = { MiningEngine, defaultJournalDir };
