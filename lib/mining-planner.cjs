"use strict";

const catalog = require("./catalog.cjs");

const sources = [
  { title: "EDDiscovery — Mining Overlay Panel", url: "https://github.com/EDDiscovery/EDDiscovery/wiki/4.-Mining-Overlay-Panel" },
  { title: "EDMC Mining Analytics", url: "https://github.com/SweetJonnySauce/EDMC-Mining-Analytics" },
  { title: "Frontier — Player Journal Manual v37", url: "https://hosting.zaonce.net/community/journal/v37/Journal_Manual_v37.pdf" },
  { title: "EDCD — Frontier module identifiers", url: "https://github.com/EDCD/FDevIDs/blob/master/outfitting.csv" },
  { title: "EDTools — Miner’s Tool", url: "https://edtools.cc/miner" },
];
const DEFAULT_PLAN = Object.freeze({ resourceKey: "platinum", method: "laser", mode: "mining", targetTons: null,
  sellPrice: null, purchasePrice: null, tripMinutes: 20, operatingCost: 0, targetCredits: null, tonsPerHour: null });
const METHODS = new Set(["laser", "core", "subsurface", "surface", "surface-planet", "raw"]);
const TIME_BOUNDARIES = new Map([["Docked", "Посадка на станцию"], ["FSDJump", "Переход в другую систему"],
  ["CarrierJump", "Прыжок флотоносца"], ["SupercruiseEntry", "Выход в суперкруиз"],
  ["Shutdown", "Завершение игры"], ["Died", "Потеря корабля"], ["LoadGame", "Новый вход в игру"]]);
const n = (value, fallback = null) => value != null && value !== "" && Number.isFinite(Number(value)) ? Number(value) : fallback;
const positive = (value, fallback = null) => { const result = n(value, fallback); return result == null ? null : Math.max(0, result); };
const stamp = (value) => { const result = Date.parse(value); return Number.isFinite(result) ? result : null; };
const iso = (value) => { const parsed = stamp(value); return parsed == null ? null : new Date(parsed).toISOString(); };
const keyOf = (value) => catalog.getResource(value)?.key ?? catalog.normalize(value);
const resourceName = (value, localized) => catalog.getResource(value)?.name ?? localized ?? String(value ?? "");
const isSRV = (value) => /srv|scarab|scorpion|rhino|nomad/i.test(String(value));
const clone = (value) => JSON.parse(JSON.stringify(value));
const cargoCapacity = (commander, loadout = null) => isSRV(commander.vehicle)
  ? positive(commander.srvCargoCapacity) : positive(commander.cargoCapacity ?? loadout?.cargoCapacity);

function classifyModule(module) {
  const item = String(module.Item ?? module.item ?? "").toLowerCase();
  const tags = [];
  if (/^hpt_(mininglaser_|miningtoolv2_)/.test(item)) tags.push("laser");
  if (/^hpt_mining_seismchrgwarhd_/.test(item)) tags.push("core");
  if (/^hpt_mining_subsurfdispmisle_/.test(item)) tags.push("subsurface");
  if (/^hpt_mining_abrblstr_/.test(item)) tags.push("abrasion");
  if (/^hpt_mrascanner_/.test(item)) tags.push("pulse");
  if (/^int_refinery_/.test(item)) tags.push("refinery");
  if (/^int_dronecontrol_collection_/.test(item)) tags.push("collector");
  if (/^int_dronecontrol_prospector_/.test(item)) tags.push("prospector");
  if (/^int_multidronecontrol_(miningv?2?|universal)_/.test(item)) tags.push("collector", "prospector");
  if (/^int_multidronecontrol_operations_/.test(item)) tags.push("collector");
  if (/^int_detailedsurfacescanner_/.test(item)) tags.push("dss");
  if (/^int_(mkii)?largebuggybay_/.test(item)) tags.push("large-srv-bay");
  if (/^int_(buggybay|largebuggybay|mkiilargebuggybay)_/.test(item)) tags.push("srv-bay");
  if (/^int_shieldgenerator_/.test(item)) tags.push("shield");
  return { slot: module.Slot ?? "", item, tags, enabled: module.On !== false,
    health: n(module.Health), ammo: module.AmmoInClip != null || module.AmmoInHopper != null
      ? positive(module.AmmoInClip, 0) + positive(module.AmmoInHopper, 0) : null };
}

function loadoutPreflight(loadout, method, commander = {}, limpets = null) {
  const selected = METHODS.has(method) ? method : "laser";
  const modules = loadout?.modules ?? [];
  const known = Boolean(loadout?.known);
  const checks = [];
  const required = selected === "raw" ? [["srv-bay", "Ангар для ТРП"]]
    : selected === "surface-planet" ? [["large-srv-bay", "Ангар для Rhino"]]
      : [["refinery", "Очиститель"], [selected === "surface" ? "abrasion" : selected, catalog.methods[selected]],
        ...(selected === "core" ? [["abrasion", "Абразивный бластер"]] : [])];
  const recommended = ["raw", "surface-planet"].includes(selected) ? []
    : [["collector", "Контроллер дронов-сборщиков"], ["prospector", "Контроллер дронов-разведчиков"],
      ["dss", "Подробный сканер поверхности"], ...(["core", "subsurface", "surface"].includes(selected) ? [["pulse", "Импульсный волновой анализатор"]] : [])];
  const testModule = ([key, name], required) => {
    const found = modules.filter((module) => module.tags.includes(key));
    const usable = found.filter((module) => module.enabled && module.health !== 0);
    const status = !known ? "unknown" : !found.length ? "missing" : !usable.length ? "disabled"
      : usable.every((module) => module.ammo === 0) ? "empty" : "ok";
    checks.push({ key, name, required, status, count: found.length, usableCount: usable.length });
  };
  required.forEach((entry) => testModule(entry, true));
  recommended.forEach((entry) => testModule(entry, false));
  const capacity = cargoCapacity(commander, loadout);
  const cargoTons = positive(commander.cargoTons);
  if (selected !== "raw") checks.push({ key: "cargo", name: "Место в грузовом трюме", required: true,
    status: capacity == null || cargoTons == null ? "unknown" : capacity > cargoTons ? "ok" : "full",
    freeTons: capacity != null && cargoTons != null ? Math.max(0, capacity - cargoTons) : null });
  if (!["raw", "surface-planet"].includes(selected)) checks.push({ key: "limpets", name: "Дроны в трюме", required: false,
    status: limpets == null ? "unknown" : limpets > 0 ? "ok" : "empty", count: limpets });
  const unavailable = checks.some((check) => check.required && ["missing", "disabled", "empty", "full"].includes(check.status));
  const unknown = checks.some((check) => check.required && check.status === "unknown");
  return { method: selected, known, ship: loadout?.ship ?? commander.ship ?? "", timestamp: loadout?.timestamp ?? null,
    status: unavailable ? "incomplete" : unknown ? "unknown" : "ready", checks,
    help: selected === "surface-planet" ? "Нужен именно Rhino. Наличие ангара не подтверждает тип хранящегося в нём ТРП."
      : selected === "raw" ? "Материалы идут в отдельный запас. Состояние ТРП и остаток топлива проверяйте в игре."
        : "Проверяются установленные модули из последнего Loadout. Журнал не показывает распределение энергии и текущее число активных дронов." };
}

class MiningPlanner {
  constructor(saved = {}) {
    this.options = { ...DEFAULT_PLAN };
    this.resetAt = iso(saved.resetAt);
    this.configure(saved.options ?? saved.plan ?? saved);
    this.reset();
  }

  reset() {
    this.current = null;
    this.history = [];
    this.loadout = null;
    this.sequence = 0;
    this.lastEventAt = null;
    this.limpets = null;
    this.lastLimpetsAt = null;
    this.gameRunning = false;
    this.resetApplied = false;
  }

  configure(update = {}) {
    if (!update || typeof update !== "object") return this.exportSettings();
    if (update.resourceKey != null) this.options.resourceKey = keyOf(update.resourceKey).slice(0, 100);
    if (METHODS.has(update.method)) this.options.method = update.method;
    if (["mining", "trade"].includes(update.mode)) this.options.mode = update.mode;
    for (const field of ["targetTons", "sellPrice", "purchasePrice", "tripMinutes", "operatingCost", "targetCredits", "tonsPerHour"]) {
      if (Object.hasOwn(update, field)) this.options[field] = positive(update[field]);
    }
    if (Object.hasOwn(update, "buyPrice") && !Object.hasOwn(update, "purchasePrice")) this.options.purchasePrice = positive(update.buyPrice);
    return this.exportSettings();
  }

  exportSettings() { return { options: { ...this.options }, resetAt: this.resetAt }; }

  resetRun(timestamp = new Date().toISOString()) {
    const at = iso(timestamp);
    if (!at) throw new Error("Некорректная дата начала новой добычи.");
    this.finish(at, "Начата новая сессия вручную");
    this.resetAt = at;
    this.resetApplied = true;
    return this.exportSettings();
  }

  begin(event, commander) {
    if (this.current) return this.current;
    this.current = { id: String(++this.sequence) + ":" + event.timestamp, startedAt: iso(event.timestamp),
      lastEventAt: iso(event.timestamp), lastMiningAt: iso(event.timestamp), endedAt: null, closeReason: null,
      system: commander.system ?? "", body: commander.body ?? "", ship: commander.ship ?? "",
      vehicle: commander.vehicle ?? "unknown", refinedTons: 0, materialUnits: 0,
      prospectScans: 0, prospectors: 0, collectors: 0, asteroidsCracked: 0, yields: {}, composition: {},
      rawContent: { high: 0, medium: 0, low: 0, unknown: 0 } };
    return this.current;
  }

  finish(timestamp, reason) {
    if (!this.current) return;
    this.current.endedAt = iso(timestamp) ?? this.current.lastEventAt;
    this.current.lastEventAt = this.current.endedAt;
    this.current.closeReason = reason;
    if (this.current.refinedTons || this.current.materialUnits || this.current.prospectScans || this.current.asteroidsCracked) {
      this.history.unshift(this.current);
      this.history = this.history.slice(0, 40);
    }
    this.current = null;
  }

  apply(event, context = {}) {
    if (!event || typeof event !== "object") return;
    const at = iso(event.timestamp);
    if (!at) return;
    const type = event.event;
    const commander = context.commander ?? {};
    // Replay the persisted manual boundary as well, so earlier completed runs remain in history.
    if (!this.resetApplied && this.resetAt && stamp(at) >= stamp(this.resetAt)) {
      this.finish(this.resetAt, "Начата новая сессия вручную");
      this.resetApplied = true;
    }
    this.lastEventAt = at;
    this.gameRunning = type !== "Shutdown";
    if (type === "Loadout" && Array.isArray(event.Modules)) this.loadout = { known: true, timestamp: at,
      ship: event.Ship_Localised ?? event.Ship ?? commander.ship ?? "", shipId: event.ShipID ?? null,
      cargoCapacity: positive(event.CargoCapacity), modules: event.Modules.map(classifyModule) };
    // An outfitting change invalidates the complete snapshot until the next Loadout.
    if (["ModuleBuy", "ModuleSell", "ModuleStore", "ModuleRetrieve", "ModuleSwap", "MassModuleStore", "ShipyardSwap", "ShipyardBuy", "ShipyardNew"].includes(type)) {
      if (this.loadout) this.loadout.known = false;
    }
    if (type === "LoadGame" && this.loadout && event.ShipID != null && this.loadout.shipId !== event.ShipID) this.loadout.known = false;
    if (type === "Cargo" && Array.isArray(event.Inventory) && !isSRV(event.Vessel)) {
      const found = event.Inventory.find((item) => ["drones", "limpet", "limpets"].includes(keyOf(item.Name ?? item.Type)));
      this.limpets = positive(found?.Count, 0);
      this.lastLimpetsAt = at;
    }
    // Cargo.json is authoritative. These deltas cover the interval between snapshots.
    if (["BuyDrones", "SellDrones", "LaunchDrone"].includes(type) && this.limpets != null) {
      const delta = type === "BuyDrones" ? positive(event.Count, 0) : type === "SellDrones" ? -positive(event.Count, 0) : -1;
      this.limpets = Math.max(0, this.limpets + delta);
      this.lastLimpetsAt = at;
    }
    if (this.current && stamp(at) >= stamp(this.current.lastEventAt)) this.current.lastEventAt = at;
    if (TIME_BOUNDARIES.has(type)) { this.finish(at, TIME_BOUNDARIES.get(type)); return; }
    if (context.countInPeriod === false) return;
    const rawCollected = type === "MaterialCollected" && (catalog.getResource(event.Name)?.kind === "raw" || String(event.Category).toLowerCase() === "raw");
    const miningDrone = type === "LaunchDrone" && ["Collection", "Prospector"].includes(event.Type);
    if (!["ProspectedAsteroid", "MiningRefined", "AsteroidCracked"].includes(type) && !rawCollected && !miningDrone) return;
    const run = this.begin(event, commander);
    run.lastMiningAt = at;
    if (type === "LaunchDrone") run[event.Type === "Prospector" ? "prospectors" : "collectors"]++;
    if (type === "AsteroidCracked") run.asteroidsCracked++;
    if (type === "MiningRefined" || rawCollected) {
      const raw = rawCollected ? event.Name : event.Type;
      const key = keyOf(raw);
      if (!key) return;
      const count = positive(event.Count, 1);
      const row = run.yields[key] ??= { key, name: resourceName(raw, event.Name_Localised ?? event.Type_Localised), refinedTons: 0, materialUnits: 0 };
      row[rawCollected ? "materialUnits" : "refinedTons"] += count;
      run[rawCollected ? "materialUnits" : "refinedTons"] += count;
    }
    if (type === "ProspectedAsteroid") {
      run.prospectScans++;
      const content = String(event.Content ?? "").toLowerCase();
      const level = ["high", "medium", "low"].find((value) => content.includes(value)) ?? "unknown";
      run.rawContent[level]++;
      const seen = new Set();
      const motherlode = keyOf(event.MotherlodeMaterial);
      const entries = [...(event.Materials ?? []), ...(motherlode ? [{ Name: event.MotherlodeMaterial, Proportion: null }] : [])];
      for (const entry of entries) {
        const key = keyOf(entry.Name);
        if (!key || seen.has(key)) continue;
        seen.add(key);
        const row = run.composition[key] ??= { key, name: resourceName(entry.Name, entry.Name_Localised),
          scans: 0, samples: 0, sumPercent: 0, minPercent: null, maxPercent: null, motherlodes: 0,
          above: { 10: 0, 15: 0, 20: 0, 30: 0, 40: 0 } };
        row.scans++;
        if (key === motherlode) row.motherlodes++;
        const percent = n(entry.Proportion);
        if (percent != null && percent >= 0 && percent <= 100) {
          row.samples++;
          row.sumPercent += percent;
          row.minPercent = row.minPercent == null ? percent : Math.min(percent, row.minPercent);
          row.maxPercent = row.maxPercent == null ? percent : Math.max(percent, row.maxPercent);
          for (const threshold of Object.keys(row.above)) if (percent >= Number(threshold)) row.above[threshold]++;
        }
      }
    }
  }

  runSnapshot(run, context = {}) {
    if (!run) return null;
    const live = !run.endedAt && context.gameRunning === true;
    const now = live ? stamp(context.now ?? new Date().toISOString()) : null;
    // Never count an unattended app after the game closes. Parent supplies gameRunning.
    const end = Math.max(stamp(run.lastEventAt), now == null ? 0 : Math.min(now, stamp(run.lastEventAt) + 300000));
    const elapsedSeconds = Math.max(0, end - stamp(run.startedAt)) / 1000;
    const rate = elapsedSeconds >= 60 ? run.refinedTons * 3600 / elapsedSeconds : null;
    return { ...clone(run), elapsedSeconds, timeBasis: "elapsed-operation", tonsPerHour: rate,
      clockCapped: now != null && now > stamp(run.lastEventAt) + 300000,
      refinementsPerMinute: elapsedSeconds >= 60 ? run.refinedTons * 60 / elapsedSeconds : null,
      tonsPerProspector: run.prospectors > 0 ? run.refinedTons / run.prospectors : null,
      yields: Object.values(run.yields).map((row) => ({ ...row,
        tonsPerHour: elapsedSeconds >= 60 ? row.refinedTons * 3600 / elapsedSeconds : null }))
        .sort((a, b) => b.refinedTons - a.refinedTons || b.materialUnits - a.materialUnits),
      composition: Object.values(run.composition).map((row) => ({ ...clone(row),
        occurrencePercent: run.prospectScans > 0 ? row.scans * 100 / run.prospectScans : null,
        averagePercent: row.samples ? row.sumPercent / row.samples : null,
        above: Object.entries(row.above).map(([threshold, scans]) => ({ threshold: Number(threshold), scans,
          percentOfAllScans: run.prospectScans ? scans * 100 / run.prospectScans : null })) }))
        .sort((a, b) => b.scans - a.scans),
      help: "Время от первого действия добычи до конца операции, включая паузы внутри неё. Сканирования повторно выбранного астероида могут повторяться: журнал не содержит его уникального ID. Content описывает сырьевые материалы, а не стоимость товарной руды." };
  }

  snapshot(context = {}) {
    const commander = context.commander ?? {};
    const current = this.runSnapshot(this.current, context);
    const history = this.history.map((run) => this.runSnapshot(run));
    const cargo = commander.cargo ?? [];
    const cargoLimpets = cargo.find((item) => ["drones", "limpet", "limpets"].includes(keyOf(item.key ?? item.Name ?? item.name)));
    const limpets = this.limpets ?? (cargoLimpets ? positive(cargoLimpets.count ?? cargoLimpets.Count, 0) : null);
    const capacity = cargoCapacity(commander, this.loadout);
    const cargoTons = positive(commander.cargoTons);
    const freeTons = capacity != null && cargoTons != null ? Math.max(0, capacity - cargoTons) : null;
    // The dedicated preparation form selects this independently of the target-list filter.
    const method = this.options.method;
    const bestRate = current?.tonsPerHour > 0 ? current.tonsPerHour : null;
    const fillMinutes = bestRate > 0 && freeTons != null ? freeTons / bestRate * 60 : null;
    const latest = current ?? history[0] ?? null;
    return { options: { ...this.options }, resetAt: this.resetAt, current, history,
      cargo: { capacity, tons: cargoTons, freeTons, fullPercent: capacity > 0 && cargoTons != null ? cargoTons / capacity * 100 : null,
        limpets, limpetsVessel: "ship", limpetsAt: this.lastLimpetsAt, minutesToFull: fillMinutes },
      prospecting: latest ? { runId: latest.id, prospectScans: latest.prospectScans, rawContent: latest.rawContent,
        composition: latest.composition } : null,
      preflight: loadoutPreflight(this.loadout, method, commander, limpets),
      plan: estimatePlan(this.options, { ...context, current, history, freeTons }), sources };
  }
}

function estimatePlan(options = {}, context = {}) {
  const plan = { ...DEFAULT_PLAN, ...options };
  const resource = (context.resources ?? []).find((row) => keyOf(row.key) === keyOf(plan.resourceKey)) ?? catalog.getResource(plan.resourceKey);
  const key = resource?.key ?? keyOf(plan.resourceKey);
  const cargo = (context.commander?.cargo ?? []).find((row) => keyOf(row.key ?? row.Name) === key);
  const aboardTons = positive(cargo?.count ?? cargo?.Count, 0);
  const capacity = cargoCapacity(context.commander ?? {});
  const targetTons = positive(plan.targetTons, capacity);
  const remainingTons = targetTons != null ? Math.max(0, targetTons - aboardTons) : null;
  const sellPrice = resource?.kind === "raw" ? null : positive(plan.sellPrice, positive(resource?.price));
  const purchasePrice = plan.mode === "mining" ? positive(plan.purchasePrice, 0) : positive(plan.purchasePrice);
  const unitMargin = sellPrice != null && purchasePrice != null ? sellPrice - purchasePrice : null;
  const observedRun = [context.current, ...(context.history ?? [])].find((run) => run?.elapsedSeconds >= 60
    && run.yields?.some((row) => row.key === key && row.tonsPerHour > 0));
  const observedRate = observedRun?.yields.find((row) => row.key === key)?.tonsPerHour ?? null;
  const tonsPerHour = positive(plan.tonsPerHour) > 0 ? positive(plan.tonsPerHour) : observedRate;
  const miningMinutes = plan.mode === "trade" ? 0 : remainingTons === 0 ? 0 : tonsPerHour > 0 && remainingTons != null ? remainingTons / tonsPerHour * 60 : null;
  const trips = targetTons === 0 ? 0 : capacity > 0 && targetTons != null ? Math.ceil(targetTons / capacity) : null;
  const travelMinutes = trips != null && plan.tripMinutes != null ? trips * positive(plan.tripMinutes, 0) : null;
  const totalMinutes = miningMinutes != null && travelMinutes != null ? miningMinutes + travelMinutes : null;
  const revenue = targetTons != null && sellPrice != null ? targetTons * sellPrice : null;
  const acquisitionCost = targetTons != null && purchasePrice != null ? targetTons * purchasePrice : null;
  const operatingCost = trips != null && plan.operatingCost != null ? trips * positive(plan.operatingCost, 0) : null;
  const profit = revenue != null && acquisitionCost != null && operatingCost != null ? revenue - acquisitionCost - operatingCost : null;
  const priceAt = plan.sellPrice != null ? null : resource?.priceAt ?? null;
  const quoteAgeHours = stamp(priceAt) != null ? Math.max(0, (stamp(context.now ?? new Date().toISOString()) - stamp(priceAt)) / 3600000) : null;
  const warnings = [];
  if (resource?.kind === "raw") warnings.push("Сырьевые материалы не продаются на товарном рынке; кредитный план для них не применим.");
  if (sellPrice == null) warnings.push("Цена продажи неизвестна. Укажите её или найдите свежий рынок.");
  if (plan.mode === "trade" && purchasePrice == null) warnings.push("Для торговой прибыли нужна цена закупки.");
  if (plan.mode === "mining" && miningMinutes == null) warnings.push("Скорость добычи ещё не измерена. Укажите т/ч или накопите минимум минуту статистики этого ресурса.");
  if (quoteAgeHours == null && plan.sellPrice == null && sellPrice != null) warnings.push("У используемой цены нет даты наблюдения.");
  if (quoteAgeHours > 24) warnings.push("Наблюдавшаяся цена старше суток. Проверьте её перед вылетом.");
  if (capacity == null || capacity === 0) warnings.push("Неизвестна вместимость грузового трюма. Число рейсов не рассчитано.");
  if (unitMargin != null && unitMargin <= 0) warnings.push("Выручка с тонны не покрывает указанную цену закупки.");
  return { key, name: resource?.name ?? key, mode: plan.mode, targetTons, aboardTons, remainingTons,
    sellPrice, purchasePrice, unitMargin, revenue, acquisitionCost, operatingCost, profit,
    trips, tripMinutes: positive(plan.tripMinutes), travelMinutes, miningMinutes, totalMinutes, tonsPerHour,
    rateSource: plan.tonsPerHour > 0 ? "manual" : observedRate > 0 ? "journal" : null,
    rateRunId: observedRun?.id ?? null, priceSource: plan.sellPrice != null ? "Цена пользователя" : resource?.priceSource ?? null,
    priceAt, quoteAgeHours, estimatedProfitPerHour: totalMinutes > 0 && profit != null ? profit * 60 / totalMinutes : null,
    tonsForCreditTarget: positive(plan.targetCredits) > 0 && unitMargin > 0 ? plan.targetCredits / unitMargin : null,
    applicable: resource?.kind !== "raw", warnings,
    help: "Прогноз по выбранному ресурсу. Цена и скорость могут измениться. Время рейса и расходы на рейс задаются вручную; спрос, скидка за крупный груз, бонусы и стоимость трития автоматически не включены. Тоннаж для денежной цели рассчитан до расходов на рейсы." };
}

module.exports = { MiningPlanner, loadoutPreflight, estimatePlan, classifyModule, DEFAULT_PLAN, sources };
