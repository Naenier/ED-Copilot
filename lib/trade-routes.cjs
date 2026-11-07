"use strict";

const { MarketSearch, emptyTradingState } = require("./markets.cjs");
const { keyOf, distance } = require("./targets.cjs");
const finite = (x, fallback) => x !== "" && x != null && Number.isFinite(Number(x)) ? Number(x) : fallback;
const DEFAULT_GOODS = ["gold", "silver", "agronomictreatment", "bertrandite", "indite", "bauxite"];

function optionsFor(input = {}, commander = {}) {
  const keys = Array.isArray(input.resourceKeys) ? input.resourceKeys : input.resourceKey ? [input.resourceKey] : DEFAULT_GOODS;
  return { system: String(input.system || commander.system || "").trim().slice(0, 150),
    resourceKeys: [...new Set(keys.map(keyOf).filter(Boolean))].slice(0, 6),
    cargoCapacity: Math.floor(Math.max(1, Math.min(25000, finite(input.cargoCapacity, commander.cargoCapacity || 128)))),
    credits: Math.max(0, finite(input.credits, 10000000)), radiusLy: Math.max(5, Math.min(5000, finite(input.radiusLy, 100))),
    maxLegLy: Math.max(1, Math.min(5000, finite(input.maxLegLy, 100))),
    maxAgeHours: Math.max(1, Math.min(720, finite(input.maxAgeHours, 24))),
    carrierMaxAgeHours: Math.max(0.25, Math.min(168, finite(input.carrierMaxAgeHours, 6))),
    minutesPerLeg: Math.max(1, Math.min(1440, finite(input.minutesPerLeg, 15))),
    operatingCostPerLeg: Math.max(0, finite(input.operatingCostPerLeg, 0)),
    demandRatio: Math.max(1, Math.min(20, finite(input.demandRatio, 4))),
    maxArrivalLs: Math.max(0, Math.min(1e7, finite(input.maxArrivalLs, 10000))),
    minPad: ["L", "M", "S", "any"].includes(input.minPad) ? input.minPad : "L",
    includeCarriers: input.includeCarriers === true, includePlanetary: input.includePlanetary !== false,
    roundTrip: input.roundTrip === true };
}

function pairLeg(buy, sell, key, options, budget = options.credits) {
  if (buy.id === sell.id || buy.own || sell.own || (!options.includePlanetary && (buy.planetary || sell.planetary)) ||
    (!options.includeCarriers && (buy.carrier || sell.carrier))) return null;
  const legLy = distance(buy.coordinates, sell.coordinates);
  if (!Number.isFinite(legLy) || legLy > options.maxLegLy) return null;
  const source = buy.quotes.find((q) => q.key === key);
  const target = sell.quotes.find((q) => q.key === key);
  if (!(source?.price > 0) || !(target?.price > source.price)) return null;
  const safeDemand = sell.carrier ? target.demand : Math.floor(target.demand / options.demandRatio);
  // Keep the entered operating allowance available after paying for the cargo.
  const quantity = Math.floor(Math.min(options.cargoCapacity, source.supply, safeDemand,
    (budget - options.operatingCostPerLeg) / source.price));
  if (!(quantity > 0)) return null;
  const purchaseCost = quantity * source.price;
  const revenue = quantity * target.price;
  const grossMargin = revenue - purchaseCost;
  const netEstimate = grossMargin - options.operatingCostPerLeg;
  if (!(netEstimate > 0)) return null;
  return { key, name: source.name, buy, sell, quantity, buyPrice: source.price, sellPrice: target.price,
    supply: source.supply, demand: target.demand, purchaseCost, revenue, grossMargin, netEstimate,
    profitPerTonne: target.price - source.price, distanceLy: legLy,
    minutes: options.minutesPerLeg, profitPerHour: netEstimate * 60 / options.minutesPerLeg };
}

function buildRoutes(markets, options) {
  const legs = [];
  const returns = new Map();
  for (const [key, sides] of Object.entries(markets)) {
    for (const buy of sides.buy || []) for (const sell of sides.sell || []) {
      const leg = pairLeg(buy, sell, key, options);
      if (leg) legs.push(leg);
      // A return purchase can be funded by the outward sale, even when its
      // cheapest tonne is initially unaffordable. Do not filter it by initial cash.
      if (options.roundTrip && pairLeg(buy, sell, key, options, Infinity)) {
        const pair = buy.id + "|" + sell.id;
        if (!returns.has(pair)) returns.set(pair, []);
        returns.get(pair).push({ buy, sell, key });
      }
    }
  }
  let rows;
  if (options.roundTrip) {
    // Both legs must close at the exact same markets, not merely in the same systems.
    rows = [];
    for (const outward of legs) for (const candidate of returns.get(outward.sell.id + "|" + outward.buy.id) || []) {
      const inward = pairLeg(candidate.buy, candidate.sell, candidate.key, options, options.credits + outward.netEstimate);
      if (!inward) continue;
      rows.push({ id: outward.buy.id + "|" + outward.sell.id + "|" + outward.key + "|" + inward.key,
        legs: [outward, inward], netEstimate: outward.netEstimate + inward.netEstimate,
        grossMargin: outward.grossMargin + inward.grossMargin, minutes: options.minutesPerLeg * 2,
        profitPerHour: (outward.netEstimate + inward.netEstimate) * 60 / (options.minutesPerLeg * 2) });
    }
  } else rows = legs.map((leg) => ({ id: leg.buy.id + "|" + leg.sell.id + "|" + leg.key,
    legs: [leg], netEstimate: leg.netEstimate, grossMargin: leg.grossMargin,
    minutes: leg.minutes, profitPerHour: leg.profitPerHour }));
  return rows.sort((a, b) => b.profitPerHour - a.profitPerHour || b.netEstimate - a.netEstimate).slice(0, 60);
}

class TradeRouteFinder {
  constructor(engine) { this.engine = engine; this.cancelled = false; this.current = null; }
  cancel() { this.cancelled = true; this.current?.cancel(); }
  async start(input) {
    if (this.engine.tradeRoutesState?.running) throw new Error("Поиск торговых рейсов уже выполняется.");
    const snapshot = this.engine.getState();
    const options = optionsFor(input, snapshot.commander);
    if (!options.system || !options.resourceKeys.length) throw new Error("Укажите исходную систему и товары.");
    this.cancelled = false;
    const state = this.engine.tradeRoutesState = { running: true, options, results: [], checked: 0,
      total: options.resourceKeys.length * 2, error: null, status: "Сравниваем закупку и продажу…", updatedAt: null,
      sources: [{ name: "Inara · торговые маршруты", url: "https://inara.cz/elite/market-traderoutes/" },
        { name: "Spansh · рыночные данные", url: "https://spansh.co.uk/stations" }],
      assumptions: "Спрос ограничивает объём с выбранным запасом. Доход в час рассчитан по введённому времени рейса; путь до первой станции и обратный перегон без груза в него не входят. Цены сообщества могут измениться.",
      coverageLimited: false };
    this.engine.emit();
    void this.run(snapshot, state);
    return state;
  }
  async run(snapshot, state) {
    const markets = {};
    const errors = [];
    try {
      for (const key of state.options.resourceKeys) {
        markets[key] = {};
        for (const mode of ["buy", "sell"]) {
          if (this.cancelled) throw new Error("cancelled");
          const proxy = { world: this.engine.world, tradingState: emptyTradingState(), getState: () => snapshot,
            emit: () => { state.status = proxy.tradingState.progress?.label || "Сравниваем рынки…"; this.engine.emit(); } };
          const search = this.current = new MarketSearch(proxy);
          await search.start({ ...state.options, mode, resourceKey: key, quantity: state.options.cargoCapacity, useCargo: false });
          await new Promise((resolve) => {
            const previous = proxy.emit;
            proxy.emit = () => { previous(); if (!proxy.tradingState.running) resolve(); };
            if (!proxy.tradingState.running) resolve();
          });
          this.current = null;
          markets[key][mode] = proxy.tradingState.results;
          if (proxy.tradingState.error) errors.push(proxy.tradingState.error);
          state.coverageLimited ||= proxy.tradingState.coverageLimited;
          state.checked += 1;
          state.results = buildRoutes(markets, state.options);
          this.engine.emit();
        }
      }
      state.status = state.results.length ? "Торговые рейсы рассчитаны по доступным котировкам." : "Прибыльных рейсов с этими ограничениями не найдено.";
    } catch (error) { state.status = this.cancelled ? "Поиск отменён." : "Поиск не завершён."; if (!this.cancelled) errors.push(error.message); }
    finally {
      state.error = errors.slice(0, 3).join(" · ") || null; state.running = false;
      state.updatedAt = new Date().toISOString(); this.engine.emit();
    }
  }
}
module.exports = { TradeRouteFinder, optionsFor, pairLeg, buildRoutes };
