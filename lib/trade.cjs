"use strict";

const catalog = require("./catalog.cjs");
const commodityNames = require("./commodity-names.json");
const { keyOf } = require("./targets.cjs");

const amount = (value, fallback = 0) => value != null && Number.isFinite(Number(value)) ? Math.max(0, Number(value)) : fallback;
const vesselKey = (vessel) => /srv|scarab|scorpion|rhino|nomad/i.test(String(vessel)) ? "srv" : "ship";

class TradeLedger {
  constructor({ russianNames = {} } = {}) {
    this.russianNames = russianNames;
    this.reset();
  }

  reset() {
    this.goods = new Map();
    for (const [key, english] of Object.entries(commodityNames)) {
      this.goods.set(key, { key, name: this.russianNames[key] || english[0] || key,
        english: english[0] || key, kind: "commodity" });
    }
    for (const resource of catalog.resources.filter((entry) => entry.kind === "commodity")) this.goods.set(resource.key, { ...resource });
    this.holds = new Map([["ship", new Map()], ["srv", new Map()]]);
    this.warehouses = new Map();
    this.activeVessel = "ship";
    this.totals = { boughtTons: 0, soldTons: 0, purchaseCost: 0, revenue: 0, knownCost: 0,
      knownRevenue: 0, grossMargin: 0, unknownCostTons: 0, miningRevenue: 0, internalTons: 0, operatingCosts: 0 };
    this.rows = new Map();
    this.recent = [];
    this.transactions = [];
    this.daily = new Map();
  }

  good(raw, localized) {
    const resource = catalog.getResource(raw);
    const key = resource?.key ?? keyOf(raw);
    if (!key) return null;
    const previous = this.goods.get(key);
    const english = resource?.english ?? commodityNames[key]?.[0] ?? previous?.english ?? String(raw);
    const name = resource?.name ?? localized ?? this.russianNames[key] ?? previous?.name ?? english;
    const item = { ...previous, key, name, english, kind: "commodity" };
    this.goods.set(key, item);
    return item;
  }

  store(vessel) {
    const key = vesselKey(vessel);
    if (!this.holds.has(key)) this.holds.set(key, new Map());
    return this.holds.get(key);
  }

  warehouse(carrierId) {
    const id = String(carrierId || "unknown");
    if (!this.warehouses.has(id)) this.warehouses.set(id, new Map());
    return this.warehouses.get(id);
  }

  add(store, key, lots) {
    const existing = store.get(key) ?? [];
    for (const lot of lots) if (lot.count > 0) existing.push({ ...lot });
    store.set(key, existing);
  }

  take(store, key, requested) {
    const lots = store.get(key) ?? [];
    const taken = [];
    let remaining = requested;
    while (remaining > 0 && lots.length) {
      const lot = lots[0];
      const count = Math.min(remaining, lot.count);
      taken.push({ ...lot, count });
      lot.count -= count;
      remaining -= count;
      if (lot.count <= 0) lots.shift();
    }
    if (remaining > 0) taken.push({ count: remaining, unitCost: null, origin: "unknown" });
    store.set(key, lots);
    return taken;
  }

  reconcile(event, context) {
    const vessel = event.Vessel ? vesselKey(event.Vessel) : vesselKey(context?.commander?.vehicle);
    this.activeVessel = vessel;
    const store = this.store(vessel);
    const actual = new Map();
    for (const entry of event.Inventory ?? []) {
      const item = this.good(entry.Name, entry.Name_Localised);
      if (item) actual.set(item.key, amount(entry.Count));
    }
    for (const key of new Set([...store.keys(), ...actual.keys()])) {
      const current = (store.get(key) ?? []).reduce((sum, lot) => sum + lot.count, 0);
      const expected = actual.get(key) ?? 0;
      if (expected > current) this.add(store, key, [{ count: expected - current, unitCost: null, origin: "unknown" }]);
      if (expected < current) this.take(store, key, current - expected);
    }
  }

  market(event) {
    for (const entry of event.Items ?? []) this.good(entry.Name, entry.Name_Localised);
  }

  record(event, transaction, countInPeriod) {
    if (!countInPeriod) return;
    const date = String(event.timestamp ?? "").slice(0, 10);
    let day = this.daily.get(date);
    if (!day) {
      day = { date, purchaseCost: 0, revenue: 0, grossMargin: 0, minedTons: 0 };
      this.daily.set(date, day);
    }
    const { kind, internal, count, value } = transaction;
    if (internal) this.totals.internalTons += count;
    else if (kind === "buy") {
      this.totals.boughtTons += count;
      this.totals.purchaseCost += value;
      day.purchaseCost += value;
    } else if (kind === "sell") {
      this.totals.soldTons += count;
      this.totals.revenue += value;
      this.totals.knownCost += transaction.knownCost;
      this.totals.knownRevenue += transaction.knownRevenue;
      this.totals.grossMargin += transaction.margin;
      this.totals.unknownCostTons += transaction.unknownTons;
      this.totals.miningRevenue += transaction.minedRevenue;
      day.revenue += value;
      day.grossMargin += transaction.margin;
    } else if (kind === "cost") this.totals.operatingCosts += value;
    if (transaction.key) {
      let row = this.rows.get(transaction.key);
      if (!row) {
        row = { key: transaction.key, name: transaction.name, boughtTons: 0, soldTons: 0, revenue: 0,
          purchaseCost: 0, grossMargin: 0, unknownCostTons: 0 };
        this.rows.set(row.key, row);
      }
      if (!internal && kind === "buy") { row.boughtTons += count; row.purchaseCost += value; }
      if (!internal && kind === "sell") {
        row.soldTons += count; row.revenue += value; row.grossMargin += transaction.margin; row.unknownCostTons += transaction.unknownTons;
      }
    }
    const record = { timestamp: event.timestamp, ...transaction };
    this.transactions.push(record);
    this.recent.unshift(record);
    this.recent = this.recent.slice(0, 200);
  }

  apply(event, context = {}) {
    const type = event.event;
    const countInPeriod = context.countInPeriod !== false;
    const vessel = vesselKey(context.commander?.vehicle);
    const store = this.store(vessel);
    const carrierStore = this.warehouse(context.carrierId);
    const internal = context.ownMarket === true;
    const result = { internal: false, minedRevenue: 0 };
    if (type === "Cargo" && Array.isArray(event.Inventory)) this.reconcile(event, context);
    if (type === "Market" && Array.isArray(event.Items)) this.market(event);
    if (type === "MiningRefined") {
      const good = this.good(event.Type, event.Type_Localised);
      if (good) this.add(store, good.key, [{ count: amount(event.Count, 1), unitCost: 0, origin: "mined" }]);
    }
    if (type === "CollectCargo") {
      const good = this.good(event.Type, event.Type_Localised);
      if (good) this.add(store, good.key, [{ count: amount(event.Count, 1), unitCost: null, origin: "collected" }]);
    }
    if (type === "EjectCargo") {
      const good = this.good(event.Type, event.Type_Localised);
      if (good) this.take(store, good.key, amount(event.Count, 1));
    }
    if (type === "CarrierDepositFuel") {
      const count = amount(event.Amount);
      this.take(store, "tritium", count);
      this.record(event, { kind: "transfer", key: "tritium", name: "Тритий", count, value: 0, internal: true,
        system: context.commander?.system ?? "", station: context.station ?? "", direction: "carrier-fuel" }, countInPeriod);
      result.internal = true;
    }
    if (type === "CargoTransfer") {
      for (const transfer of event.Transfers ?? []) {
        const good = this.good(transfer.Type, transfer.Type_Localised);
        const count = amount(transfer.Count);
        if (!good || count <= 0) continue;
        const toCarrier = String(transfer.Direction).toLowerCase() === "tocarrier";
        const toShip = String(transfer.Direction).toLowerCase() === "toship";
        if (!toCarrier && !toShip) continue;
        const lots = this.take(toCarrier ? store : carrierStore, good.key, count);
        this.add(toCarrier ? carrierStore : store, good.key, lots);
        this.record(event, { kind: "transfer", key: good.key, name: good.name, count, value: 0, internal: true,
          system: context.commander?.system ?? "", station: context.station ?? "", direction: toCarrier ? "tocarrier" : "toship" }, countInPeriod);
      }
      result.internal = true;
    }
    if (type === "MarketBuy" || type === "MarketSell") {
      const good = this.good(event.Type, event.Type_Localised);
      const count = amount(event.Count);
      if (!good || count <= 0) return result;
      const value = amount(type === "MarketBuy" ? event.TotalCost : event.TotalSale,
        amount(type === "MarketBuy" ? event.BuyPrice : event.SellPrice) * count);
      const transaction = { kind: type === "MarketBuy" ? "buy" : "sell", key: good.key, name: good.name,
        count, value, internal, system: context.commander?.system ?? "", station: context.station ?? "",
        knownCost: 0, knownRevenue: 0, margin: 0, unknownTons: 0, minedRevenue: 0 };
      if (type === "MarketBuy") {
        const lots = internal ? this.take(carrierStore, good.key, count)
          : [{ count, unitCost: value / count, origin: "bought" }];
        this.add(store, good.key, lots);
      } else {
        const lots = this.take(store, good.key, count);
        if (internal) this.add(carrierStore, good.key, lots);
        else for (const lot of lots) {
          const saleValue = value * lot.count / count;
          let unitCost = lot.unitCost;
          if (unitCost == null && amount(event.AvgPricePaid) > 0) unitCost = Number(event.AvgPricePaid);
          if (unitCost != null) {
            transaction.knownCost += unitCost * lot.count;
            transaction.knownRevenue += saleValue;
          } else transaction.unknownTons += lot.count;
          if (lot.origin === "mined") transaction.minedRevenue += saleValue;
        }
        transaction.margin = transaction.knownRevenue - transaction.knownCost;
      }
      this.record(event, transaction, countInPeriod);
      result.internal = internal;
      result.minedRevenue = transaction.minedRevenue;
    }
    const expenses = new Set(["RefuelAll", "RefuelPartial", "Repair", "RepairAll", "RestockVehicle", "Resurrect", "BuyAmmo"]);
    if (expenses.has(type) && amount(event.Cost) > 0) {
      this.record(event, { kind: "cost", name: { RefuelAll: "Заправка", RefuelPartial: "Заправка", Repair: "Ремонт",
        RepairAll: "Ремонт", RestockVehicle: "Пополнение техники", Resurrect: "Страховка", BuyAmmo: "Боеприпасы" }[type],
      count: 0, value: amount(event.Cost), cost: amount(event.Cost), internal: false,
      system: context.commander?.system ?? "", station: context.station ?? "" }, countInPeriod);
    }
    if (type === "Died") this.holds = new Map([["ship", new Map()], ["srv", new Map()]]);
    return result;
  }

  snapshot(commander = {}) {
    const inventory = [];
    const vessel = ["ship", "srv"].includes(commander.vehicle) ? commander.vehicle : this.activeVessel;
    for (const [key, lots] of this.store(vessel)) {
      const count = lots.reduce((sum, lot) => sum + lot.count, 0);
      if (!count) continue;
      const known = lots.filter((lot) => lot.unitCost != null);
      const knownCostTons = known.reduce((sum, lot) => sum + lot.count, 0);
      const purchaseCost = known.reduce((sum, lot) => sum + lot.count * lot.unitCost, 0);
      inventory.push({ key, name: this.goods.get(key)?.name ?? key, count, knownCostTons,
        unknownCostTons: count - knownCostTons, purchaseCost, unitCost: knownCostTons === count ? purchaseCost / count : null,
        minedTons: lots.filter((lot) => lot.origin === "mined").reduce((sum, lot) => sum + lot.count, 0) });
    }
    return { totals: { ...this.totals }, inventory: inventory.sort((a, b) => b.count - a.count),
      goods: [...this.goods.values()], byResource: [...this.rows.values()],
      recent: this.recent, daily: [...this.daily.values()].sort((a, b) => b.date.localeCompare(a.date)),
      help: "Валовая маржа учитывает известную себестоимость проданных партий. Неизвестная себестоимость показана отдельно. Переводы на собственный флотоносец не считаются внешней выручкой.",
    };
  }
}

module.exports = { TradeLedger, vesselKey };
