"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { setTimeout: delay } = require("node:timers/promises");

const API = "https://spansh.co.uk/api";
const CAPACITY = 25000;
const TANK_CAPACITY = 1000;
const SOURCES = [
  { title: "Spansh · Fleet Carrier Router", url: "https://spansh.co.uk/fleet-carrier" },
  { title: "Frontier · Fleet Carriers Update", url: "https://steamcommunity.com/games/elitedangerous/announcements/detail/2248932323933514462" },
];
const DEFAULT_OPTIONS = {
  sourceSystem: "", destinationSystem: "", waypoints: [], refuelSystems: [],
  calculateStartingFuel: true, usedCapacityTons: 0, tankTons: 1000, cargoTritiumTons: 0,
  reserveTritiumTons: 0, departureTime: null, jumpMinutes: 15, cooldownMinutes: 5,
  overheadMinutes: 1, maxQueueMinutes: 0, miningTonsPerHour: 150,
  refuelMinutes: 0, refuelMethod: "mine", tritiumPrice: 0,
};

const finite = (value) => value !== "" && value != null && Number.isFinite(Number(value)) ? Number(value) : null;
const systemKey = (value) => String(value ?? "").trim().toLowerCase();
const coordinatesOf = (value) => {
  const coordinates = Array.isArray(value) ? value : [value?.x, value?.y, value?.z];
  return coordinates.length === 3 && coordinates.every((v) => finite(v) != null) ? coordinates.map(Number) : null;
};
const iso = (milliseconds) => new Date(milliseconds).toISOString();
const sum = (items, field) => items.reduce((total, item) => total + (finite(item[field]) ?? 0), 0);
const distance = (from, to) => Math.hypot(...from.map((value, index) => value - to[index]));
const safeIndex = (value, length) => Number.isInteger(Number(value)) && Number(value) >= 0 && Number(value) < length ? Number(value) : null;

function emptyCarrierRouteState() {
  return {
    running: false, status: "idle", error: null, options: { ...DEFAULT_OPTIONS, waypoints: [], refuelSystems: [] },
    jobId: null, source: "Spansh", sourceUrl: SOURCES[0].url, updatedAt: null,
    progress: { stage: "idle", label: "", completedJumps: 0, totalJumps: 0, nextIndex: null },
    route: null, summary: null, warnings: [], assumptions: [], sources: SOURCES,
  };
}

function namesList(value, label) {
  const values = Array.isArray(value) ? value : typeof value === "string" ? value.split(/[\r\n;]+/) : [];
  if (values.length > 100) throw new Error(`${label}: допускается до 100 систем.`);
  return values.map((name) => String(name).trim()).filter(Boolean).map((name) => {
    if (name.length > 150) throw new Error(`${label}: слишком длинное название системы.`);
    return name;
  });
}

function normalizeRouteOptions(input = {}, snapshot = {}) {
  const carrier = snapshot.carrier ?? {};
  const cargoTritium = finite(carrier.fuel?.reserveTons);
  const used = finite(carrier.space?.capacityTons) != null && finite(carrier.space?.freeTons) != null
    ? carrier.space.capacityTons - carrier.space.freeTons : null;
  const options = { ...DEFAULT_OPTIONS,
    sourceSystem: carrier.location?.system || snapshot.commander?.system || "",
    usedCapacityTons: used ?? 0, tankTons: finite(carrier.fuel?.tankTons) ?? 1000,
    cargoTritiumTons: cargoTritium ?? 0,
    reserveTritiumTons: finite(carrier.planning?.reserveTritium) ?? 0,
  };
  for (const key of ["sourceSystem", "destinationSystem"]) {
    if (input[key] != null) options[key] = String(input[key]).trim();
    if (options[key].length > 150) throw new Error("Слишком длинное название системы.");
  }
  options.waypoints = namesList(input.waypoints, "Промежуточные системы");
  options.refuelSystems = namesList(input.refuelSystems, "Дозаправки");
  if (typeof input.calculateStartingFuel === "boolean") options.calculateStartingFuel = input.calculateStartingFuel;
  for (const [key, label, min, max, integer] of [
    ["usedCapacityTons", "Занятая вместимость", 0, CAPACITY, true],
    ["tankTons", "Тритий в баке", 0, TANK_CAPACITY, true],
    ["cargoTritiumTons", "Тритий на складе", 0, CAPACITY, true],
    ["reserveTritiumTons", "Запас по прибытии", 0, CAPACITY + TANK_CAPACITY, true],
    ["jumpMinutes", "Подготовка прыжка", 1, 240, false],
    ["cooldownMinutes", "Перезарядка", 0, 120, false],
    ["overheadMinutes", "Дополнительное время на прыжок", 0, 240, false],
    ["maxQueueMinutes", "Дополнительная очередь на прыжок", 0, 240, false],
    ["miningTonsPerHour", "Скорость добычи трития", 1, 10000, false],
    ["refuelMinutes", "Остановка на дозаправку", 0, 10080, false],
    ["tritiumPrice", "Цена трития", 0, 100000000, false],
  ]) {
    if (input[key] != null && input[key] !== "") {
      const value = finite(input[key]);
      if (value == null || value < min || value > max || (integer && !Number.isInteger(value))) {
        throw new Error(`${label}: введите ${integer ? "целое " : ""}число от ${min} до ${max}.`);
      }
      options[key] = value;
    }
  }
  if (input.departureTime) {
    const timestamp = Date.parse(input.departureTime);
    if (!Number.isFinite(timestamp)) throw new Error("Некорректное время отправления.");
    options.departureTime = iso(timestamp);
  }
  if (input.refuelMethod != null) {
    if (!["mine", "buy", "manual"].includes(input.refuelMethod)) throw new Error("Неизвестный способ пополнения трития.");
    options.refuelMethod = input.refuelMethod;
  }
  if (!options.sourceSystem) throw new Error("Укажите исходную систему флотоносца.");
  if (!options.destinationSystem) throw new Error("Укажите конечную систему.");
  if (options.usedCapacityTons < options.cargoTritiumTons) {
    throw new Error("Складской тритий входит в занятую вместимость и не может превышать её.");
  }
  const destinations = [...options.waypoints, options.destinationSystem];
  let previous = options.sourceSystem;
  for (const name of destinations) {
    if (systemKey(name) === systemKey(previous)) throw new Error("Соседние точки маршрута должны отличаться.");
    previous = name;
  }
  for (const name of options.refuelSystems) {
    if (!destinations.some((destination) => systemKey(destination) === systemKey(name))) {
      throw new Error(`Точка дозаправки «${name}» должна быть промежуточной или конечной системой.`);
    }
  }
  return options;
}

// Preserve 64-bit system identifiers on all supported Electron/Node versions.
// Native JSON.parse would round identifiers greater than Number.MAX_SAFE_INTEGER.
function parseSpanshJson(text) {
  return JSON.parse(text.replace(/"(?:\\.|[^"\\])*"|-?\d+(?:\.\d+)?(?:e[+-]?\d+)?/gi, (token) => {
    if (token.startsWith('"') || /[.e]/i.test(token)) return token;
    return Number.isSafeInteger(Number(token)) ? token : JSON.stringify(token);
  }));
}

function buildRoutePayload(options, systems, overrides = {}) {
  const calculate = overrides.calculateStartingFuel ?? options.calculateStartingFuel;
  const payload = {
    source: systems[0].id64, destinations: systems.slice(1).map((system) => system.id64),
    mass: CAPACITY, capacity: CAPACITY,
    capacity_used: overrides.usedCapacityTons ?? (calculate ? options.usedCapacityTons - options.cargoTritiumTons : options.usedCapacityTons),
    calculate_starting_fuel: calculate ? 1 : 0,
  };
  if (calculate) {
    payload.refuel_destinations = systems.slice(1).filter((system) => options.refuelSystems.some((name) => systemKey(name) === systemKey(system.name))).map((system) => system.id64);
  } else {
    payload.fuel_loaded = overrides.tankTons ?? options.tankTons;
    payload.tritium_stored = overrides.cargoTritiumTons ?? options.cargoTritiumTons;
  }
  return payload;
}

function payloadForm(payload) {
  const form = new URLSearchParams();
  for (const [key, value] of Object.entries(payload)) {
    for (const item of Array.isArray(value) ? value : [value]) form.append(key, String(item));
  }
  return form;
}

function routeAssumptions(options) {
  return [
    "Системы, координаты и расход каждого прыжка рассчитаны Spansh для личного флотоносца: базовая масса 25 000 т, вместимость 25 000 т, бак до 1 000 т, дальность до 500 св. лет.",
    options.calculateStartingFuel
      ? "Подбирается новая стартовая загрузка трития. Занятая вместимость без текущего складского трития сохраняется; новый запас учтён Spansh при расчёте массы и расхода."
      : "Используются введённые текущие запасы. Занятая вместимость включает складской тритий; бак учитывается отдельно.",
    "Остатки в строке — после прибытия, условного пополнения и перелива трития со склада в бак. Перелив и пополнение нужно выполнять в игре вручную.",
    `Расписание — оценка: подготовка ${options.jumpMinutes} мин, перезарядка ${options.cooldownMinutes} мин между прыжками, прочее ${options.overheadMinutes} мин на прыжок. Фактическая очередь и задержки игры могут быть больше выбранного диапазона.`,
    options.refuelMethod === "mine"
      ? `Время добычи по дороге рассчитано при ${options.miningTonsPerHour} т/ч. Ледяное кольцо не гарантирует удобную или богатую точку трития; начальная загрузка в это время не входит.`
      : "Время приобретения и доставки трития по дороге задаётся вручную на остановку; наличие топлива, актуальные цены и доступ проверяются отдельно.",
    "Разрешение на посещение системы, свободная орбита и доступность топлива не гарантируются маршрутом. Изменение груза или услуг требует перерасчёта.",
  ];
}

function normalizeProviderRoute(response, options, metadata = {}) {
  const raw = response?.result;
  if (response?.status !== "ok" || !raw || !Array.isArray(raw.jumps) || raw.jumps.length < 2) {
    throw new Error("Spansh не вернул полный маршрут флотоносца.");
  }
  const stops = raw.jumps.map((jump, index) => {
    const coordinates = coordinatesOf([jump.x, jump.y, jump.z]);
    if (!coordinates || !jump.name || !/^\d+$/.test(String(jump.id64 ?? ""))) throw new Error("В маршруте Spansh отсутствуют координаты или идентификатор системы.");
    const fields = ["fuel_used", "fuel_in_tank", "tritium_in_market", "restock_amount"];
    if (fields.some((field) => finite(jump[field]) == null || Number(jump[field]) < 0)) throw new Error("Spansh вернул неполные данные о тритии.");
    if (Number(jump.fuel_in_tank) > TANK_CAPACITY || Number(jump.tritium_in_market) > CAPACITY) throw new Error("Spansh вернул невозможный запас трития.");
    return {
      index, name: jump.name, id64: String(jump.id64), coordinates,
      distanceLy: finite(jump.distance) ?? 0, remainingLy: finite(jump.distance_to_destination),
      fuelUsedTons: Number(jump.fuel_used), tankTons: Number(jump.fuel_in_tank),
      cargoTritiumTons: Number(jump.tritium_in_market), restockTons: Number(jump.restock_amount),
      mustRestock: Boolean(jump.must_restock), hasIcyRing: Boolean(jump.has_icy_ring),
      isPristine: Boolean(jump.is_system_pristine), isDestination: Boolean(jump.is_desired_destination),
      visited: index === 0, visitedAt: null, confirmation: index === 0 ? "source" : null,
    };
  });
  for (let index = 1; index < stops.length; index += 1) {
    const measured = distance(stops[index - 1].coordinates, stops[index].coordinates);
    if (measured > 500.01 || Math.abs(measured - stops[index].distanceLy) > 0.05) {
      throw new Error("Проверка маршрута Spansh не пройдена: длина прыжка не соответствует координатам или превышает 500 св. лет.");
    }
    const expectedFuel = stops[index - 1].tankTons + stops[index - 1].cargoTritiumTons
      - stops[index].fuelUsedTons + stops[index].restockTons;
    if (Math.abs(expectedFuel - stops[index].tankTons - stops[index].cargoTritiumTons) > 0.01) {
      throw new Error("Проверка маршрута Spansh не пройдена: баланс трития между прыжками не сходится.");
    }
  }
  const totalJumps = stops.length - 1;
  const first = stops[0];
  const last = stops.at(-1);
  const restockTons = sum(stops.slice(1), "restockTons");
  const restockStops = stops.slice(1).filter((stop) => stop.mustRestock || stop.restockTons > 0);
  const miningMinutes = options.refuelMethod === "mine" ? restockTons / options.miningTonsPerHour * 60 : 0;
  const refuelMinutes = restockStops.length * options.refuelMinutes;
  // No final cooldown is needed to arrive. It is shown separately as readyAgain.
  const travelMinutes = totalJumps * (options.jumpMinutes + options.overheadMinutes) + Math.max(0, totalJumps - 1) * options.cooldownMinutes;
  const totalMinutes = travelMinutes + miningMinutes + refuelMinutes;
  const maxMinutes = totalMinutes + totalJumps * options.maxQueueMinutes;
  const departureMs = Date.parse(options.departureTime ?? metadata.createdAt ?? new Date().toISOString());
  let elapsed = 0;
  let maximumElapsed = 0;
  first.estimatedArrival = iso(departureMs);
  first.estimatedArrivalLatest = first.estimatedArrival;
  first.tankBeforeTopupTons = finite(raw.fuel_loaded);
  first.transferTons = first.restockTons > 0 ? null : Math.max(0, first.tankTons - (finite(raw.fuel_loaded) ?? 0));
  for (let index = 0; index < stops.length; index += 1) {
    const stop = stops[index];
    if (index > 0) {
      stop.tankBeforeTopupTons = stops[index - 1].tankTons - stop.fuelUsedTons;
      if (stop.tankBeforeTopupTons < -0.01) throw new Error("Spansh вернул прыжок без достаточного топлива в баке.");
      stop.transferTons = stop.restockTons > 0 ? null : Math.max(0, stop.tankTons - stop.tankBeforeTopupTons);
    }
    const stopMinutes = index === 0 ? 0 : (options.refuelMethod === "mine" ? stop.restockTons / options.miningTonsPerHour * 60 : 0)
      + (stop.mustRestock || stop.restockTons > 0 ? options.refuelMinutes : 0);
    stop.refuelMinutes = stopMinutes;
    stop.elapsedMinutes = elapsed;
    stop.estimatedReady = iso(departureMs + (elapsed + stopMinutes) * 60000);
    if (index < stops.length - 1) {
      const cooldown = index === 0 ? 0 : options.cooldownMinutes;
      stop.estimatedDeparture = iso(departureMs + (elapsed + stopMinutes + cooldown + options.jumpMinutes) * 60000);
      elapsed += stopMinutes + cooldown + options.jumpMinutes + options.overheadMinutes;
      maximumElapsed += stopMinutes + cooldown + options.jumpMinutes + options.overheadMinutes + options.maxQueueMinutes;
      stops[index + 1].estimatedArrival = iso(departureMs + elapsed * 60000);
      stops[index + 1].estimatedArrivalLatest = iso(departureMs + maximumElapsed * 60000);
    } else stop.estimatedDeparture = null;
  }
  const summary = {
    totalJumps, completedJumps: 0, remainingJumps: totalJumps,
    distanceLy: sum(stops, "distanceLy"), directDistanceLy: distance(first.coordinates, last.coordinates),
    fuelTons: sum(stops, "fuelUsedTons"), startingTritiumTons: first.tankTons + first.cargoTritiumTons,
    startingTankTons: first.tankTons, startingCargoTritiumTons: first.cargoTritiumTons,
    initialLoadTons: options.calculateStartingFuel ? first.tankTons + first.cargoTritiumTons : first.restockTons,
    restockTons, acquireTritiumTons: options.calculateStartingFuel
      ? first.tankTons + first.cargoTritiumTons + restockTons : sum(stops, "restockTons"),
    additionalTritiumNeededTons: Math.max(0, first.tankTons + first.cargoTritiumTons - options.tankTons - options.cargoTritiumTons) + restockTons,
    restockStops: restockStops.length, arrivalTankTons: last.tankTons,
    arrivalCargoTritiumTons: last.cargoTritiumTons, arrivalTritiumTons: last.tankTons + last.cargoTritiumTons,
    reserveTritiumTons: options.reserveTritiumTons,
    reserveShortfallTons: Math.max(0, options.reserveTritiumTons - last.tankTons - last.cargoTritiumTons),
    miningMinutes, refuelMinutes, travelMinutes, totalMinutes,
    timeRange: { minMinutes: totalMinutes, maxMinutes, bounded: false },
    departureTime: iso(departureMs), estimatedArrival: last.estimatedArrival,
    estimatedArrivalLatest: last.estimatedArrivalLatest,
    estimatedComplete: iso(departureMs + totalMinutes * 60000),
    readyAgain: iso(Date.parse(last.estimatedArrival) + options.cooldownMinutes * 60000),
    fuelCost: options.tritiumPrice > 0 ? sum(stops, "fuelUsedTons") * options.tritiumPrice : null,
    acquireCost: options.tritiumPrice > 0 ? (options.calculateStartingFuel
      ? first.tankTons + first.cargoTritiumTons + restockTons : sum(stops, "restockTons")) * options.tritiumPrice : null,
    minimumStartingTritiumTons: metadata.minimumStartingTritiumTons ?? null,
  };
  const warnings = [];
  if (summary.reserveShortfallTons > 0) warnings.push(`По прибытии не хватает ${summary.reserveShortfallTons} т до выбранного резерва. Требуется изменить стартовую загрузку или план дозаправок.`);
  if (restockTons > 0) warnings.push(`Spansh требует дополнительно получить по дороге ${restockTons} т трития. Эти остановки не подтверждают наличие продавца или места добычи.`);
  if (options.refuelMethod === "mine" && restockStops.some((stop) => !stop.hasIcyRing)) {
    warnings.push("Часть остановок пополнения Spansh не имеет известных ледяных колец. Добыча в этой системе не подтверждена; заранее выберите другой источник топлива и пересчитайте маршрут.");
  }
  if (options.calculateStartingFuel && summary.arrivalTritiumTons === 0) warnings.push("Минимальная загрузка Spansh оставляет нулевой тритий по прибытии. Задайте резерв для дальнейшего перелёта.");
  if (options.refuelMethod !== "mine" && restockStops.length && options.refuelMinutes === 0) warnings.push("Время приобретения и доставки трития не задано; оценка времени не включает эти операции.");
  return {
    route: {
      stops, activeIndex: 1, createdAt: metadata.createdAt ?? new Date().toISOString(),
      options: { ...options },
      sourceSystem: first.name, destinationSystem: last.name, jobId: response.job,
      sourceUrl: `https://spansh.co.uk/fleet-carrier/results/${encodeURIComponent(response.job)}`,
      providerParameters: response.parameters ?? null,
      providerResult: raw,
      mass: { baseTons: CAPACITY, capacityTons: CAPACITY,
        usedCapacityTons: raw.capacity_used, tankCapacityTons: TANK_CAPACITY,
        startingTotalMassTons: CAPACITY + Number(raw.capacity_used) - Number(raw.tritium_stored ?? 0) + summary.startingTritiumTons },
    }, summary, warnings, assumptions: routeAssumptions(options),
  };
}

function csvValue(value) {
  const text = String(value ?? "");
  // Spreadsheet programs must not execute a system name as a formula.
  const safe = /^[=+@\t\r]/.test(text) ? "'" + text : text;
  return '"' + safe.replace(/"/g, '""') + '"';
}

function routeCsv(state) {
  if (!state?.route?.stops) throw new Error("Сначала рассчитайте маршрут.");
  const rows = [["№", "Система", "ID64", "X", "Y", "Z", "Прыжок, св. лет", "Осталось, св. лет", "Расход, т", "Бак после пополнения, т", "Складской тритий, т", "Получить тритий, т", "Ледяные кольца", "Расчётное прибытие UTC", "Посещено"]];
  for (const stop of state.route.stops) rows.push([stop.index, stop.name, stop.id64, ...stop.coordinates,
    stop.distanceLy, stop.remainingLy, stop.fuelUsedTons, stop.tankTons, stop.cargoTritiumTons,
    stop.restockTons, stop.hasIcyRing ? "Да" : "Нет", stop.estimatedArrival, stop.visited ? "Да" : "Нет"]);
  return "\uFEFF" + rows.map((row) => row.map(csvValue).join(";")).join("\r\n");
}

class FleetRoutePlanner {
  constructor(engine, dependencies = {}) {
    this.engine = engine;
    this.fetch = dependencies.fetch ?? globalThis.fetch;
    this.pollMs = dependencies.pollMs ?? 1500;
    this.timeoutMs = dependencies.timeoutMs ?? 10 * 60000;
    this.controller = null;
    this.disposed = false;
    this.file = engine.dataDir ? path.join(engine.dataDir, "carrier-route.json") : null;
    let restored = null;
    if (this.file) {
      try {
        const saved = JSON.parse(fs.readFileSync(this.file, "utf8"));
        if (saved.version === 1 && saved.state?.options) restored = saved.state;
      } catch { /* A missing/corrupt saved route must not prevent app startup. */ }
    }
    engine.carrierRouteState = restored ? { ...emptyCarrierRouteState(), ...restored, running: false,
      status: restored.route?.stops?.length ? "ready" : "idle", error: null,
      progress: { ...restored.progress, stage: restored.route?.stops?.length ? "ready" : "idle",
        label: restored.route?.stops?.length ? "Сохранённый маршрут" : "" } } : engine.carrierRouteState ?? emptyCarrierRouteState();
  }

  save() {
    if (!this.file || this.disposed) return;
    const state = this.engine.carrierRouteState;
    try {
      fs.mkdirSync(path.dirname(this.file), { recursive: true });
      delete state.persistenceError;
      fs.writeFileSync(this.file + ".tmp", JSON.stringify({ version: 1, state: { ...state, running: false } }, null, 2));
      fs.renameSync(this.file + ".tmp", this.file);
    } catch (error) {
      state.persistenceError = "Не удалось сохранить маршрут: " + error.message;
    }
  }

  publish() { if (!this.disposed) this.engine.emit(); }

  async json(url, init, signal) {
    signal?.throwIfAborted();
    let response;
    try {
      response = await this.fetch(url, { ...init, signal: AbortSignal.any([signal, AbortSignal.timeout(45000)].filter(Boolean)),
        headers: { Accept: "application/json", "User-Agent": "ED-Copilot/1.2 FleetCarrierRoute", ...init?.headers } });
    } catch (error) {
      if (signal?.aborted) throw signal.reason;
      throw new Error(`Не удалось связаться со Spansh: ${error.message}`);
    }
    const text = await response.text();
    if (!response.ok) throw new Error(`Spansh: HTTP ${response.status}. ${response.status === 429 ? "Слишком много запросов; повторите позже." : "Повторите запрос позже."}`);
    try { return parseSpanshJson(text); }
    catch { throw new Error("Spansh вернул ответ в неизвестном формате."); }
  }

  async resolveSystem(name, signal) {
    const response = await this.json(`${API}/search/systems?${new URLSearchParams({ q: name })}`, {}, signal);
    const system = response.results?.find((entry) => systemKey(entry.name) === systemKey(name));
    if (!system || !/^\d+$/.test(String(system.id64 ?? "")) || !coordinatesOf(system)) {
      throw new Error(`Система «${name}» не найдена в Spansh. Проверьте точное название.`);
    }
    return { name: system.name, id64: String(system.id64), coordinates: coordinatesOf(system) };
  }

  async providerJob(payload, state, signal) {
    const response = await this.json(`${API}/fleetcarrier/route`, { method: "POST", body: payloadForm(payload) }, signal);
    if (response.status === "ok") return response;
    if (response.status === "error") throw new Error("Spansh: " + String(response.error || "не удалось построить маршрут"));
    if (response.status !== "queued" || !/^[a-f0-9-]{20,50}$/i.test(response.job ?? "")) throw new Error("Spansh не принял задание маршрутизации.");
    state.jobId = response.job;
    state.sourceUrl = `https://spansh.co.uk/fleet-carrier/results/${encodeURIComponent(response.job)}`;
    state.status = "queued";
    state.progress.label = "Маршрут ожидает расчёта на Spansh…";
    this.publish();
    const deadline = Date.now() + this.timeoutMs;
    while (Date.now() < deadline) {
      await delay(this.pollMs, null, { signal });
      const result = await this.json(`${API}/results/${encodeURIComponent(response.job)}`, {}, signal);
      if (result.status === "ok") return result;
      if (["error", "failed", "not_found"].includes(result.status)) throw new Error("Spansh: " + String(result.error ?? result.message ?? "маршрут не найден"));
      if (!["queued", "running", "processing"].includes(result.status)) throw new Error("Spansh вернул неизвестный статус задания.");
      state.status = result.status === "queued" ? "queued" : "running";
      state.progress.label = result.status === "queued" ? "Очередь маршрутизации Spansh…" : "Spansh рассчитывает маршрут и тритий…";
      this.publish();
    }
    throw new Error("Spansh не завершил расчёт за 10 минут. Попробуйте позже; сохранённый маршрут доступен.");
  }

  async start(input = {}) {
    if (this.disposed) throw new Error("Планировщик остановлен.");
    if (this.controller) throw new Error("Расчёт маршрута уже идёт. Дождитесь завершения или отмените его.");
    const snapshot = this.engine.getState();
    const options = normalizeRouteOptions(input, snapshot);
    const previous = this.engine.carrierRouteState ?? emptyCarrierRouteState();
    const controller = new AbortController();
    this.controller = controller;
    this.engine.carrierRouteState = {
      ...emptyCarrierRouteState(), route: previous.route, summary: previous.summary,
      options, running: true, status: "resolving", warnings: [],
      progress: { ...previous.progress, stage: "resolving", label: "Проверка названий систем в Spansh…" },
    };
    this.publish();
    this.pending = this.run(options, snapshot, controller.signal).finally(() => {
      if (this.controller === controller) this.controller = null;
      this.engine.carrierRouteState.running = false;
      this.engine.carrierRouteState.updatedAt = new Date().toISOString();
      this.save();
      this.publish();
    });
    return this.engine.carrierRouteState;
  }

  async run(options, snapshot, signal) {
    const state = this.engine.carrierRouteState;
    const createdAt = new Date().toISOString();
    try {
      const systems = [];
      const cache = new Map();
      for (const name of [options.sourceSystem, ...options.waypoints, options.destinationSystem]) {
        const key = systemKey(name);
        if (!cache.has(key)) cache.set(key, await this.resolveSystem(name, signal));
        systems.push(cache.get(key));
      }
      state.progress.stage = "routing";
      let response = await this.providerJob(buildRoutePayload(options, systems), state, signal);
      let result = normalizeProviderRoute(response, options, { createdAt });
      const minimum = options.calculateStartingFuel ? result.summary.startingTritiumTons : null;
      if (options.calculateStartingFuel && options.reserveTritiumTons > 0 && options.refuelSystems.length === 0) {
        let loaded = minimum + options.reserveTritiumTons;
        const baseUsed = options.usedCapacityTons - options.cargoTritiumTons;
        for (let attempt = 0; attempt < 8; attempt += 1) {
          const tank = Math.min(TANK_CAPACITY, loaded);
          const hold = Math.max(0, loaded - tank);
          if (baseUsed + hold > CAPACITY) throw new Error("Для выбранного маршрута и резерва недостаточно вместимости. Добавьте промежуточную систему дозаправки.");
          state.progress.label = "Уточнение массы и расхода с резервом по прибытии…";
          this.publish();
          response = await this.providerJob(buildRoutePayload(options, systems, { calculateStartingFuel: false,
            usedCapacityTons: baseUsed + hold, tankTons: tank, cargoTritiumTons: hold }), state, signal);
          result = normalizeProviderRoute(response, options, { createdAt, minimumStartingTritiumTons: minimum });
          const deficit = result.summary.reserveShortfallTons + result.summary.restockTons;
          if (deficit === 0) break;
          loaded += Math.ceil(deficit);
          if (attempt === 7) throw new Error("Spansh не смог подтвердить резерв без дозаправки. Уменьшите резерв или добавьте точку пополнения.");
        }
      } else if (options.calculateStartingFuel && options.reserveTritiumTons > 0 && options.refuelSystems.length > 0) {
        result.warnings.push("При выбранных точках дозаправки резерв показан как цель; Spansh рассчитывает минимальное топливо между ними. Проверьте остаток и дефицит по прибытии.");
      }
      if (systems[0].id64 !== result.route.stops[0].id64 || systems.at(-1).id64 !== result.route.stops.at(-1).id64) {
        throw new Error("Начало или конец ответа Spansh не соответствует запрошенному маршруту.");
      }
      let previousIndex = 0;
      for (const system of systems.slice(1)) {
        const index = result.route.stops.findIndex((stop, stopIndex) => stopIndex > previousIndex && stop.id64 === system.id64);
        if (index < 0) throw new Error("Ответ Spansh не сохранил порядок промежуточных систем.");
        previousIndex = index;
      }
      if (snapshot.carrier?.fuel?.needsRefresh) result.warnings.push("Запасы из журнала могли измениться после прыжка. Обновите управление флотоносцем в игре и пересчитайте маршрут.");
      if (snapshot.carrier?.owned && finite(snapshot.carrier.fuel?.reserveTons) == null) {
        result.warnings.push("Полный состав склада неизвестен. Подтвердите складской тритий вручную.");
      }
      Object.assign(state, result, { status: "ready", error: null, jobId: response.job,
        sourceUrl: result.route.sourceUrl, updatedAt: new Date().toISOString(),
        progress: { stage: "ready", label: "Маршрут рассчитан Spansh", completedJumps: 0,
          totalJumps: result.summary.totalJumps, nextIndex: 1 } });
    } catch (error) {
      state.status = signal.aborted ? "cancelled" : "error";
      state.error = signal.aborted ? null : error.message;
      state.progress.stage = state.status;
      state.progress.label = signal.aborted ? "Расчёт отменён" : "Не удалось рассчитать маршрут";
    }
  }

  cancel() { this.controller?.abort(); return this.engine.carrierRouteState; }

  selectStop(index) {
    const state = this.engine.carrierRouteState;
    const selected = safeIndex(index, state.route?.stops?.length ?? 0);
    if (selected == null) throw new Error("Неизвестная точка маршрута.");
    state.route.activeIndex = selected;
    this.save(); this.publish();
    return state;
  }

  markVisited(index, confirmation = "manual", timestamp = new Date().toISOString()) {
    const state = this.engine.carrierRouteState;
    const selected = safeIndex(index, state.route?.stops?.length ?? 0);
    if (selected == null) throw new Error("Неизвестная точка маршрута.");
    if (!Number.isFinite(Date.parse(timestamp))) throw new Error("Некорректное время посещения.");
    for (const stop of state.route.stops) {
      if (stop.index <= selected) Object.assign(stop, { visited: true,
        visitedAt: stop.visitedAt ?? timestamp, confirmation: stop.confirmation ?? (stop.index === selected ? confirmation : "progress") });
    }
    const next = state.route.stops.find((stop) => !stop.visited);
    state.route.activeIndex = next?.index ?? null;
    const completed = state.route.stops.filter((stop) => stop.index > 0 && stop.visited).length;
    Object.assign(state.progress, { completedJumps: completed, totalJumps: state.route.stops.length - 1, nextIndex: next?.index ?? null });
    Object.assign(state.summary, { completedJumps: completed, remainingJumps: state.summary.totalJumps - completed });
    this.save(); this.publish();
    return state;
  }

  observeCarrier(carrier) {
    const state = this.engine.carrierRouteState;
    const location = carrier?.location;
    if (state.running || !carrier?.owned || !location || !state.route?.stops?.length) return false;
    const timestamp = location.updatedAt;
    if (!timestamp || Date.parse(timestamp) < Date.parse(state.route.createdAt)) return false;
    // Only the next point may be automatically confirmed. A ship's own FSDJump
    // and old journal replays cannot complete an arbitrary later carrier stop.
    const next = state.route.stops.find((stop) => !stop.visited);
    if (!next || (location.systemAddress != null ? String(location.systemAddress) !== next.id64 : systemKey(location.system) !== systemKey(next.name))) return false;
    this.markVisited(next.index, "carrier-journal", timestamp);
    return true;
  }

  exportCsv() { return routeCsv(this.engine.carrierRouteState); }

  dispose() { this.save(); this.disposed = true; this.controller?.abort(); }
}

module.exports = {
  FleetRoutePlanner, emptyCarrierRouteState, normalizeRouteOptions,
  buildRoutePayload, payloadForm, normalizeProviderRoute, parseSpanshJson, routeCsv,
};
