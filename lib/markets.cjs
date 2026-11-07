"use strict";

const catalog = require("./catalog.cjs");
const commodityNames = require("./commodity-names.json");
const { distance, keyOf } = require("./targets.cjs");
const { setTimeout: delay } = require("node:timers/promises");

const SEARCH_URL = "https://spansh.co.uk/api/stations/search";
const PAGE_SIZE = 60;
const MAX_PAGES = 2;
const MAX_COMMODITIES = 6;
const finite = (value) => value != null && Number.isFinite(Number(value)) ? Number(value) : null;

const DEFAULT_OPTIONS = {
  mode: "sell", system: "", resourceKey: "", useCargo: true, useCarrierCargo: false,
  radiusLy: 300, maxAgeHours: 24, carrierMaxAgeHours: 6, minDemand: 1, minSupply: 1,
  includeCarriers: true, includeOwn: false, minPad: "L", maxArrivalLs: 10000, quantity: 1, demandRatio: 4,
};

function emptyTradingState() {
  return { running: false, options: { ...DEFAULT_OPTIONS }, results: [], basket: [],
    progress: { checked: 0, total: 0, stationsSeen: 0, resultsCount: 0, label: "" },
    error: null, updatedAt: null, source: "Spansh", destination: null, selectedId: null,
    coverageLimited: false, skippedGoods: [], reference: null, status: "" };
}

function normalizedOptions(input = {}) {
  const result = { ...DEFAULT_OPTIONS };
  result.mode = input.mode === "buy" ? "buy" : "sell";
  result.system = String(input.system ?? "").trim().slice(0, 150);
  result.resourceKey = keyOf(input.resourceKey);
  for (const key of ["includeCarriers", "includeOwn", "useCargo", "useCarrierCargo"]) {
    if (typeof input[key] === "boolean") result[key] = input[key];
  }
  for (const [key, low, high] of [["radiusLy", 5, 5000], ["maxAgeHours", 1, 720],
    ["carrierMaxAgeHours", 0.25, 168], ["minDemand", 1, 1000000000], ["minSupply", 1, 1000000000],
    ["maxArrivalLs", 0, 10000000], ["quantity", 1, 1000000], ["demandRatio", 1, 20]]) {
    const value = finite(input[key]);
    if (value != null) result[key] = Math.max(low, Math.min(high, value));
  }
  result.quantity = Math.floor(result.quantity);
  if (["L", "M", "S", "any"].includes(input.minPad)) result.minPad = input.minPad;
  return result;
}

function goodsIndex(state) {
  const goods = new Map(catalog.resources.filter((resource) => resource.kind !== "raw").map((resource) => [resource.key, resource]));
  for (const entry of state.goods ?? []) {
    if (entry?.key && entry.kind !== "raw") goods.set(keyOf(entry.key), { ...goods.get(keyOf(entry.key)), ...entry });
  }
  return goods;
}

function buildBasket(state, options) {
  const goods = goodsIndex(state);
  const cargo = options.useCarrierCargo
    ? (state.carrier?.warehouse?.items ?? state.carrier?.cargo ?? state.carrier?.inventory ?? []) : (state.commander?.cargo ?? []);
  if (options.mode === "buy" || options.resourceKey || !options.useCargo) {
    const key = options.resourceKey || "tritium";
    const resource = goods.get(key) ?? catalog.getResource(key);
    if (resource?.kind === "raw") throw new Error("Сырьевые материалы не продаются на товарном рынке.");
    const carried = cargo.find((entry) => keyOf(entry.key) === key)?.count ?? 0;
    const quantity = options.mode === "sell" && options.useCargo && carried > 0 ? carried : options.quantity;
    return { basket: [{ key, name: resource?.name || key, english: resource?.english || key,
      marketName: resource?.english || commodityNames[key]?.[0] || key, quantity }], skippedGoods: [] };
  }
  const items = cargo.filter((entry) => entry.count > 0 && entry.key && keyOf(entry.key) !== "drones")
    .filter((entry) => catalog.getResource(entry.key)?.kind !== "raw")
    .map((entry) => {
      const resource = goods.get(keyOf(entry.key)) ?? catalog.getResource(entry.key);
      return { key: resource?.key ?? keyOf(entry.key), name: resource?.name || entry.name || entry.key,
        english: resource?.english || entry.english || entry.key,
        marketName: resource?.english || commodityNames[resource?.key ?? keyOf(entry.key)]?.[0] || entry.english || entry.key,
        quantity: Math.floor(entry.count) };
    }).sort((a, b) => b.quantity - a.quantity);
  if (!items.length) throw new Error("Нет товарного груза для продажи. Выберите ресурс вручную.");
  return { basket: items.slice(0, MAX_COMMODITIES), skippedGoods: items.slice(MAX_COMMODITIES) };
}

function stationPad(station) {
  if (station.has_large_pad === true || Number(station.large_pads) > 0) return "L";
  if (station.has_medium_pad === true || Number(station.medium_pads) > 0) return "M";
  if (station.has_small_pad === true || Number(station.small_pads) > 0) return "S";
  return "";
}

function quoteForCommodity(raw, requested, mode, timestamp, nowMs) {
  const normalized = keyOf(raw.symbol ?? raw.commodity ?? raw.name);
  const resource = catalog.getResource(normalized);
  if (normalized !== requested.key && resource?.key !== requested.key && normalized !== keyOf(requested.english)) return null;
  // Spansh's live market payload uses the commander perspective, as Market.json does.
  // demand > 0 / sell_price is a buyer. supply > 0 / buy_price is a seller.
  const price = finite(mode === "sell" ? raw.sell_price : raw.buy_price);
  const demand = finite(raw.demand);
  const supply = finite(raw.supply);
  const available = mode === "sell" ? demand : supply;
  if (!(price > 0) || !(available > 0)) return null;
  const quantity = Math.min(requested.quantity, Math.floor(available));
  return { key: requested.key, name: requested.name, price, demand, supply,
    requestedQuantity: requested.quantity, quantity, subtotal: quantity * price,
    updatedAt: timestamp, ageHours: Math.max(0, (nowMs - Date.parse(timestamp)) / 3600000) };
}

function stationResult(station, basket, options, nowMs = Date.now(), reference = null) {
  const timestamp = typeof station.market_updated_at === "string" ? station.market_updated_at : null;
  const updated = Date.parse(timestamp);
  if (!Number.isFinite(updated) || updated > nowMs + 10 * 60000) return null;
  const carrier = /carrier/i.test(String(station.type ?? ""));
  if (carrier && !options.includeCarriers) return null;
  const ageHours = Math.max(0, (nowMs - updated) / 3600000);
  if (ageHours > (carrier ? options.carrierMaxAgeHours : options.maxAgeHours)) return null;
  const coords = [station.system_x, station.system_y, station.system_z].map(finite);
  const validCoords = coords.every((coordinate) => coordinate != null) ? coords : null;
  const distanceLy = validCoords && reference ? distance(reference, validCoords) : finite(station.distance);
  if (distanceLy == null || distanceLy > options.radiusLy) return null;
  const distanceLs = finite(station.distance_to_arrival);
  if (options.maxArrivalLs > 0 && distanceLs != null && distanceLs > options.maxArrivalLs) return null;
  const pad = stationPad(station);
  const padRank = { "": 0, S: 1, M: 2, L: 3, any: 0 };
  if (padRank[pad] < padRank[options.minPad]) return null;
  const market = Array.isArray(station.market) ? station.market : [];
  const quotes = basket.map((requested) => {
    for (const raw of market) {
      const quote = quoteForCommodity(raw, requested, options.mode, timestamp, nowMs);
      if (!quote) continue;
      if (options.mode === "sell" && quote.demand < options.minDemand) return null;
      if (options.mode === "buy" && quote.supply < options.minSupply) return null;
      quote.bulkTaxRisk = options.mode === "sell" && !carrier && quote.demand < quote.requestedQuantity * options.demandRatio;
      return quote;
    }
    return null;
  }).filter(Boolean);
  if (!quotes.length) return null;
  const requestedTons = basket.reduce((total, entry) => total + entry.quantity, 0);
  const coveredTons = quotes.reduce((total, entry) => total + entry.quantity, 0);
  const estimatedTotal = quotes.reduce((total, entry) => total + entry.subtotal, 0);
  const marketId = String(station.market_id ?? station.id ?? "");
  const warnings = [];
  if (quotes.some((quote) => quote.bulkTaxRisk)) warnings.push("Спрос ниже выбранного запаса " + options.demandRatio + "× объёма груза. Цена при продаже на станции может снизиться.");
  if (carrier) warnings.push("Авианосец мог переместиться или изменить заказ после обновления рынка.");
  if (coveredTons < requestedTons) warnings.push("Известный спрос или запас покрывает только часть выбранного груза.");
  if (!pad) warnings.push("Размер посадочной площадки не подтверждён.");
  if (distanceLs == null) warnings.push("Расстояние от звезды прибытия неизвестно.");
  return {
    id: marketId ? "market:" + marketId : "station:" + station.system_name + ":" + station.name,
    marketId, station: String(station.name ?? ""), system: String(station.system_name ?? ""),
    systemAddress: station.system_id64 ?? null, type: String(station.type ?? ""), carrier, pad,
    planetary: station.is_planetary === true, coordinates: validCoords, distanceLy, distanceLs,
    updatedAt: timestamp, sourceUpdatedAt: timestamp, ageHours, quotes, estimatedTotal, requestedTons,
    coveredTons, missingTons: Math.max(0, requestedTons - coveredTons), coverageComplete: coveredTons >= requestedTons,
    demandSafe: !quotes.some((quote) => quote.bulkTaxRisk), bulkTaxRisk: quotes.some((quote) => quote.bulkTaxRisk),
    averagePrice: coveredTons > 0 ? estimatedTotal / coveredTons : null,
    warnings, source: "Spansh", sourceUrl: marketId ? "https://spansh.co.uk/station/" + encodeURIComponent(marketId) : "https://spansh.co.uk/stations",
  };
}

function rankResults(rows, mode) {
  return [...rows].sort((a, b) => Number(b.coverageComplete) - Number(a.coverageComplete)
    || (mode === "sell" ? Number(b.demandSafe) - Number(a.demandSafe) : 0)
    || (mode === "sell" ? b.estimatedTotal - a.estimatedTotal
      : (a.estimatedTotal / a.coveredTons) - (b.estimatedTotal / b.coveredTons))
    || b.coveredTons - a.coveredTons || a.distanceLy - b.distanceLy || a.ageHours - b.ageHours);
}

function searchPayload(resource, options, reference, page, nowMs) {
  const availability = options.mode === "sell" ? "demand" : "supply";
  const minimum = options.mode === "sell" ? options.minDemand : options.minSupply;
  const since = new Date(nowMs - Math.max(options.maxAgeHours, options.carrierMaxAgeHours) * 3600000).toISOString();
  const until = new Date(nowMs + 10 * 60000).toISOString();
  const sortField = options.mode === "sell" ? "market_sell_price" : "market_buy_price";
  const filters = {
      has_market: { value: true },
      market: [{ name: resource.marketName || resource.english, [availability]: { value: [String(minimum), "999999999999"], comparison: "<=>" } }],
      market_updated_at: { value: [since, until], comparison: "<=>" },
      distance: { min: 0, max: options.radiusLy },
  };
  if (options.minPad === "L") filters.has_large_pad = { value: true };
  if (options.maxArrivalLs > 0) filters.distance_to_arrival = { value: [0, options.maxArrivalLs], comparison: "<=>" };
  return {
    filters,
    sort: [{ [sortField]: [{ name: resource.marketName || resource.english, direction: options.mode === "sell" ? "desc" : "asc" }] },
      { distance: { direction: "asc" } }],
    size: PAGE_SIZE, page,
    reference_coords: { x: reference[0], y: reference[1], z: reference[2] },
  };
}

async function fetchJson(url, options, signal) {
  for (let attempt = 0; attempt < 2; attempt += 1) {
    const timeout = AbortSignal.timeout(20000);
    const response = await fetch(url, { ...options, signal: AbortSignal.any([signal, timeout]),
      headers: { Accept: "application/json", "User-Agent": "ED-Copilot/1.1 (local mining and trading companion)", ...options?.headers } });
    if ((response.status === 429 || response.status === 503) && attempt === 0) {
      await response.body?.cancel();
      await delay(2000, undefined, { signal });
      continue;
    }
    if (!response.ok) throw new Error("Сервис рынка ответил HTTP " + response.status + ".");
    const text = await response.text();
    if (text.length > 16000000) throw new Error("Ответ рынка слишком большой. Уменьшите радиус поиска.");
    return JSON.parse(text);
  }
}

async function resolveReference(engine, snapshot, options, signal) {
  const carrier = snapshot.carrier ?? {};
  const defaultSystem = options.useCarrierCargo ? carrier.system ?? carrier.location?.system : snapshot.commander?.system;
  const system = options.system || defaultSystem;
  if (!system) throw new Error("Укажите систему, от которой считать расстояния.");
  const normalized = String(system).toLowerCase();
  const known = [...(engine.world?.values() ?? [])].find((entry) => entry.name?.toLowerCase() === normalized && entry.coordinates);
  if (known) return { system: known.name, coordinates: [...known.coordinates], systemAddress: known.address ?? null };
  if (snapshot.commander?.system?.toLowerCase() === normalized && Array.isArray(snapshot.commander.coordinates)) {
    return { system: snapshot.commander.system, coordinates: [...snapshot.commander.coordinates],
      systemAddress: snapshot.commander.systemAddress ?? null };
  }
  const payload = await fetchJson("https://www.edsm.net/api-v1/system?" + new URLSearchParams({
    systemName: String(system), showCoordinates: "1", showId: "1",
  }), {}, signal);
  const coords = [payload.coords?.x, payload.coords?.y, payload.coords?.z].map(finite);
  if (!coords.every((value) => value != null)) throw new Error("Координаты системы не найдены. Проверьте название.");
  return { system: payload.name || String(system), coordinates: coords, systemAddress: payload.id64 ?? null };
}

class MarketSearch {
  constructor(engine) {
    this.engine = engine;
    this.controller = null;
  }

  cancel() { this.controller?.abort(); }

  async start(input = {}) {
    if (this.controller) throw new Error("Поиск покупателей уже идёт. Дождитесь завершения или отмените его.");
    const snapshot = this.engine.getState();
    const options = normalizedOptions(input);
    const { basket, skippedGoods } = buildBasket(snapshot, options);
    const previous = this.engine.tradingState ?? emptyTradingState();
    const controller = new AbortController();
    this.controller = controller;
    this.engine.tradingState = {
      ...emptyTradingState(), options, basket, skippedGoods, running: true,
      destination: previous.destination ?? null, selectedId: previous.selectedId ?? null,
      progress: { checked: 0, total: basket.length, stationsSeen: 0, resultsCount: 0, label: "Определение исходной системы…" },
      status: options.mode === "sell" ? "Поиск покупателей груза…" : "Поиск продавцов…",
      coverageLimited: skippedGoods.length > 0,
    };
    this.engine.emit();
    void this.run(snapshot, options, basket, controller.signal).finally(() => {
      if (this.controller === controller) this.controller = null;
      this.engine.tradingState.running = false;
      this.engine.tradingState.updatedAt = new Date().toISOString();
      this.engine.emit();
    });
    return this.engine.tradingState;
  }

  async run(snapshot, options, basket, signal) {
    const engine = this.engine;
    const state = engine.tradingState;
    const nowMs = Date.now();
    const results = new Map();
    const errors = [];
    try {
      state.reference = await resolveReference(engine, snapshot, options, signal);
      options.system = state.reference.system;
      for (const resource of basket) {
        signal.throwIfAborted();
        state.progress.label = (options.mode === "sell" ? "Покупатели: " : "Продавцы: ") + resource.name;
        engine.emit();
        let scanned = 0;
        let total = 0;
        for (let page = 0; page < MAX_PAGES; page += 1) {
          signal.throwIfAborted();
          try {
            const payload = await fetchJson(SEARCH_URL, { method: "POST", headers: { "Content-Type": "application/json" },
              body: JSON.stringify(searchPayload(resource, options, state.reference.coordinates, page, nowMs)) }, signal);
            const stations = Array.isArray(payload.results) ? payload.results : [];
            total = Number(payload.count) || stations.length;
            scanned += stations.length;
            state.progress.stationsSeen += stations.length;
            for (const station of stations) {
              const marketId = String(station.market_id ?? station.id ?? "");
              const ownCarrier = snapshot.carrier?.id != null && marketId === String(snapshot.carrier.id);
              if (ownCarrier && !options.includeOwn) continue;
              const result = stationResult(station, basket, options, nowMs, state.reference.coordinates);
              if (!result) continue;
              result.own = ownCarrier;
              if (ownCarrier) result.warnings.push("Это ваш авианосец. Перемещение груза на него учитывается отдельно от внешней продажи.");
              const previous = results.get(result.id);
              if (!previous || Date.parse(result.updatedAt) >= Date.parse(previous.updatedAt)) results.set(result.id, result);
            }
            state.results = rankResults(results.values(), options.mode).slice(0, 120);
            state.progress.resultsCount = state.results.length;
            engine.emit();
            if (stations.length < PAGE_SIZE || scanned >= total) break;
            await delay(350, undefined, { signal });
          } catch (error) {
            if (signal.aborted) throw error;
            errors.push(resource.name + ": " + error.message);
            break;
          }
        }
        if (scanned < total) state.coverageLimited = true;
        state.progress.checked += 1;
        engine.emit();
        await delay(350, undefined, { signal });
      }
      state.status = state.results.length ? "Рынки сравнены по последним переданным котировкам." : "Подходящих рынков по выбранным условиям не найдено.";
      if (state.coverageLimited) state.status += " Охват ограничен первыми 120 рынками на товар и шестью товарами за запрос.";
      if (errors.length) state.error = errors.slice(0, 3).join(" · ");
    } catch (error) {
      state.status = signal.aborted ? "Поиск отменён. Полученные котировки сохранены." : "Поиск рынка не завершён.";
      state.error = signal.aborted ? null : error.message;
    }
  }
}

module.exports = {
  MarketSearch, emptyTradingState, normalizedOptions, buildBasket, quoteForCommodity, stationResult, rankResults, searchPayload,
};
