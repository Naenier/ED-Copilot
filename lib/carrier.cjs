"use strict";

const catalog = require("./catalog.cjs");

const SOURCE = "https://hosting.zaonce.net/community/journal/v37/Journal_Manual_v37.pdf";
const SERVICES = {
  captain: "Управление", refuel: "Заправка кораблей", repair: "Ремонт", rearm: "Боеприпасы и дроны",
  commodities: "Товарный рынок", carrierfuel: "Тритиевое хранилище", blackmarket: "Чёрный рынок",
  voucherredemption: "Служба погашения", shipyard: "Верфь", outfitting: "Снаряжение",
  exploration: "Universal Cartographics", vistagenomics: "Vista Genomics", bartender: "Бар",
  pioneersupplies: "Снабжение «Пионер»",
};
const ACCESS = { all: "Все", none: "Только владелец", friends: "Друзья", squadron: "Эскадрилья", squadronfriends: "Друзья и эскадрилья" };
const OWNER_EVENTS = new Set([
  "CarrierBuy", "CarrierStats", "CarrierLocation", "CarrierJumpRequest", "CarrierJumpCancelled",
  "CarrierFinance", "CarrierBankTransfer", "CarrierCrewServices", "CarrierTradeOrder", "CarrierNameChanged",
  "CarrierDockingPermission", "CarrierDecommission", "CarrierCancelDecommission", "CarrierShipPack", "CarrierModulePack",
]);
const numeric = (value) => value != null && value !== "" && Number.isFinite(Number(value)) ? Number(value) : null;
const identifier = (value) => value != null && String(value).trim() ? String(value) : null;
const timestampOf = (event) => typeof event.timestamp === "string" ? event.timestamp : null;
const normalized = (value) => catalog.normalize(value);
const finiteDate = (value) => Number.isFinite(Date.parse(value)) ? Date.parse(value) : null;
const ownType = (event) => event.CarrierType == null || event.CarrierType === "FleetCarrier";
const emptyFuel = () => ({ tankTons: null, tankCapacityTons: 1000, depositedTons: 0,
  jumpRangeCurrent: null, jumpRangeMax: null, updatedAt: null, needsRefresh: false });
const emptySpace = () => ({ capacityTons: null, cargoTons: null, reservedTons: null, crewTons: null,
  shipPacksTons: null, modulePacksTons: null, freeTons: null, updatedAt: null, estimated: false });
const emptyFinance = () => ({ balance: null, reserveBalance: null, availableBalance: null,
  reservePercent: null, taxRates: {}, updatedAt: null });

function normalizePlanning(value = {}) {
  const cost = numeric(value.weeklyUpkeep);
  return {
    weeklyUpkeep: cost != null && cost > 0 ? Math.min(10000000000, cost) : null,
    reserveTritium: Math.min(25000, Math.max(0, numeric(value.reserveTritium) ?? 0)),
    targetTankTons: Math.min(1000, Math.max(0, numeric(value.targetTankTons) ?? 1000)),
  };
}

function resourceName(key, localized) {
  return catalog.getResource(key)?.name ?? localized ?? String(key ?? "");
}

function newCarrier(id) {
  return { id, name: "", callsign: "", updatedAt: null, statsAt: null, location: null,
    dockingAccess: null, allowNotorious: null, pendingDecommission: false, scrapTime: null,
    fuel: emptyFuel(), space: emptySpace(), finance: emptyFinance(), services: new Map(),
    orders: new Map(), items: new Map(), movements: [], transactions: [], jump: { pending: null, lastArrival: null },
  };
}

class CarrierTracker {
  constructor(configuration = {}) {
    this.planning = normalizePlanning(configuration.planning);
    this.stockBaseline = configuration.stockBaseline && typeof configuration.stockBaseline === "object"
      ? configuration.stockBaseline : null;
    this.reset();
  }

  reset() {
    this.carriers = new Map();
    this.ownerId = null;
    this.dockedCarrierId = null;
    this.dockedStation = null;
    this.otherCarrierEvents = 0;
  }

  getConfiguration() {
    return { planning: { ...this.planning }, stockBaseline: this.stockBaseline };
  }

  updatePlanning(patch = {}) {
    this.planning = normalizePlanning({ ...this.planning, ...patch });
    return { ...this.planning };
  }

  setStockBaseline(value) {
    const carrierId = identifier(value?.carrierId ?? this.ownerId);
    if (!carrierId || carrierId !== this.ownerId) throw new Error("Сначала откройте управление личным флотоносцем в игре.");
    if (!Array.isArray(value.items)) throw new Error("Нужен список ресурсов склада.");
    const items = new Map();
    for (const entry of value.items.slice(0, 512)) {
      const key = catalog.getResource(entry.key ?? entry.name)?.key ?? normalized(entry.key ?? entry.name);
      const count = numeric(entry.count);
      if (!key || count == null || count < 0 || count > 25000) throw new Error("Некорректное количество ресурса на складе.");
      items.set(key, { key, name: resourceName(key, entry.name), count });
    }
    const time = value.timestamp ?? new Date().toISOString();
    if (finiteDate(time) == null) throw new Error("Некорректное время снимка склада.");
    this.stockBaseline = { carrierId, timestamp: new Date(time).toISOString(), items: [...items.values()] };
    return this.stockBaseline;
  }

  isOwnDockedMarket(marketId) {
    const candidate = identifier(marketId) ?? this.dockedCarrierId;
    return Boolean(this.ownerId && candidate === this.ownerId);
  }

  current() {
    return this.ownerId ? this.carriers.get(this.ownerId) ?? null : null;
  }

  confirm(event) {
    if (!ownType(event)) {
      this.otherCarrierEvents += 1;
      return null;
    }
    const id = identifier(event.CarrierID);
    if (!id) return null;
    let carrier = this.carriers.get(id);
    if (!carrier) {
      carrier = newCarrier(id);
      this.carriers.set(id, carrier);
    }
    this.ownerId = id;
    return carrier;
  }

  addTransaction(carrier, event, kind, label, amount = null) {
    carrier.transactions.unshift({ timestamp: timestampOf(event), kind, label,
      amount: numeric(amount), balance: carrier.finance.balance });
    carrier.transactions = carrier.transactions.slice(0, 100);
  }

  applyFinance(carrier, values, timestamp) {
    for (const [source, field] of [["CarrierBalance", "balance"], ["ReserveBalance", "reserveBalance"],
      ["AvailableBalance", "availableBalance"], ["ReservePercent", "reservePercent"]]) {
      if (numeric(values[source]) != null) carrier.finance[field] = Number(values[source]);
    }
    if (numeric(values.TaxRate) != null) carrier.finance.taxRates.all = Number(values.TaxRate);
    for (const [key, value] of Object.entries(values)) {
      if (key.startsWith("TaxRate_") && numeric(value) != null) carrier.finance.taxRates[key.slice(8)] = Number(value);
    }
    carrier.finance.updatedAt = timestamp;
  }

  moveStock(carrier, key, label, delta, timestamp, kind) {
    if (!key || !Number.isFinite(delta) || delta === 0) return;
    let item = carrier.items.get(key);
    if (!item) {
      item = { key, name: resourceName(key, label), count: 0, netTransferred: 0,
        basis: "По записанным перемещениям", snapshotAt: null, updatedAt: timestamp };
      carrier.items.set(key, item);
    }
    item.count = Math.max(0, item.count + delta);
    item.netTransferred += delta;
    item.updatedAt = timestamp;
    carrier.movements.push({ key, name: item.name, delta, timestamp, kind });
    // Transfer history is also used for a manual inventory baseline, so keep the complete replay.
    if (carrier.space.cargoTons != null) {
      carrier.space.cargoTons = Math.max(0, carrier.space.cargoTons + delta);
      if (carrier.space.freeTons != null) carrier.space.freeTons = Math.max(0, carrier.space.freeTons - delta);
      carrier.space.updatedAt = timestamp;
      carrier.space.estimated = true;
    }
  }

  apply(event, context = {}) {
    if (!event || typeof event.event !== "string") return;
    const type = event.event;
    const timestamp = timestampOf(event);
    if (type === "Docked" || type === "Location") {
      const marketId = identifier(event.MarketID);
      this.dockedCarrierId = /carrier/i.test(String(event.StationType)) || marketId === this.ownerId ? marketId : null;
      this.dockedStation = event.StationName ?? null;
    } else if (type === "Undocked") {
      this.dockedCarrierId = null;
      this.dockedStation = null;
    }

    let carrier = OWNER_EVENTS.has(type) ? this.confirm(event) : this.current();
    if (!carrier) return;
    if (OWNER_EVENTS.has(type) && !ownType(event)) return;
    const explicitId = identifier(event.CarrierID);
    if (explicitId && explicitId !== carrier.id) return;

    if (type === "CarrierBuy") {
      carrier.callsign = event.Callsign ?? carrier.callsign;
      carrier.name = event.Name ?? carrier.name;
      if (event.Location) carrier.location = { system: event.Location, systemAddress: event.SystemAddress ?? null,
        bodyId: null, coordinates: null, updatedAt: timestamp };
      this.addTransaction(carrier, event, "purchase", "Покупка флотоносца", -Number(event.Price || 0));
    }
    if (type === "CarrierStats") {
      carrier.callsign = event.Callsign ?? carrier.callsign;
      carrier.name = event.Name ?? carrier.name;
      carrier.statsAt = timestamp;
      carrier.dockingAccess = event.DockingAccess ?? carrier.dockingAccess;
      if (typeof event.AllowNotorious === "boolean") carrier.allowNotorious = event.AllowNotorious;
      if (typeof event.PendingDecommission === "boolean") carrier.pendingDecommission = event.PendingDecommission;
      if (numeric(event.FuelLevel) != null) carrier.fuel.tankTons = Number(event.FuelLevel);
      if (numeric(event.JumpRangeCurr) != null) carrier.fuel.jumpRangeCurrent = Number(event.JumpRangeCurr);
      if (numeric(event.JumpRangeMax) != null) carrier.fuel.jumpRangeMax = Number(event.JumpRangeMax);
      carrier.fuel.updatedAt = timestamp;
      carrier.fuel.needsRefresh = false;
      for (const [source, field] of [["TotalCapacity", "capacityTons"], ["Cargo", "cargoTons"],
        ["CargoSpaceReserved", "reservedTons"], ["Crew", "crewTons"], ["ShipPacks", "shipPacksTons"],
        ["ModulePacks", "modulePacksTons"], ["FreeSpace", "freeTons"]]) {
        if (numeric(event.SpaceUsage?.[source]) != null) carrier.space[field] = Number(event.SpaceUsage[source]);
      }
      carrier.space.updatedAt = timestamp;
      carrier.space.estimated = false;
      if (event.Finance) this.applyFinance(carrier, event.Finance, timestamp);
      if (Array.isArray(event.Crew)) {
        carrier.services.clear();
        for (const service of event.Crew) {
          if (!service.CrewRole) continue;
          const key = normalized(service.CrewRole);
          carrier.services.set(key, { key, name: SERVICES[key] ?? service.CrewRole,
            installed: service.Activated === true, enabled: service.Enabled === true,
            crewName: service.CrewName ?? "", updatedAt: timestamp });
        }
      }
    }
    if (type === "CarrierLocation") {
      carrier.location = { system: event.StarSystem ?? "", systemAddress: event.SystemAddress ?? null,
        bodyId: event.BodyID ?? null, coordinates: event.StarPos ?? null, updatedAt: timestamp };
      const pending = carrier.jump.pending;
      const afterDeparture = pending?.departureMs != null && finiteDate(timestamp) >= pending.departureMs;
      const arrivedAtBody = pending?.bodyId == null || event.BodyID == null || pending.bodyId === event.BodyID;
      if (pending && afterDeparture && arrivedAtBody && event.StarSystem === pending.system) {
        carrier.jump.lastArrival = timestamp;
        carrier.jump.pending = null;
        carrier.fuel.needsRefresh = true;
      }
    }
    if (type === "CarrierJumpRequest") {
      const departureTime = event.DepartureTime ?? null;
      carrier.jump.pending = { system: event.SystemName ?? "", body: event.Body ?? null,
        bodyId: event.BodyID ?? null,
        systemAddress: event.SystemAddress ?? event.SystemID ?? null,
        departureTime, departureMs: finiteDate(departureTime), requestedAt: timestamp };
    }
    if (type === "CarrierJumpCancelled") carrier.jump.pending = null;
    if (type === "CarrierJump" && identifier(event.MarketID ?? event.CarrierID) === carrier.id) {
      carrier.location = { system: event.StarSystem ?? "", systemAddress: event.SystemAddress ?? null,
        bodyId: event.BodyID ?? null, coordinates: event.StarPos ?? null, updatedAt: timestamp };
      carrier.jump.pending = null;
      carrier.jump.lastArrival = timestamp;
      carrier.fuel.needsRefresh = true;
    }
    if (["Location", "FSDJump", "CarrierJump"].includes(type) && carrier.location?.system === event.StarSystem && Array.isArray(event.StarPos)) {
      carrier.location.coordinates = event.StarPos;
    }
    if (type === "CarrierDepositFuel" && explicitId === carrier.id) {
      if (numeric(event.Total) != null) carrier.fuel.tankTons = Number(event.Total);
      carrier.fuel.depositedTons += Math.max(0, numeric(event.Amount) ?? 0);
      carrier.fuel.updatedAt = timestamp;
      carrier.fuel.needsRefresh = false;
      this.addTransaction(carrier, event, "fuel", "Заправлено трития: " + (numeric(event.Amount) ?? 0) + " т");
    }
    if (type === "CarrierFinance") this.applyFinance(carrier, event, timestamp);
    if (type === "CarrierBankTransfer") {
      this.applyFinance(carrier, event, timestamp);
      const deposit = numeric(event.Deposit);
      const withdraw = numeric(event.Withdraw);
      this.addTransaction(carrier, event, "bank", deposit != null ? "Пополнение счёта" : "Снятие со счёта",
        deposit != null ? deposit : withdraw != null ? -withdraw : null);
    }
    if (type === "CarrierNameChanged") {
      carrier.name = event.Name ?? carrier.name;
      carrier.callsign = event.Callsign ?? carrier.callsign;
    }
    if (type === "CarrierDockingPermission") {
      carrier.dockingAccess = event.DockingAccess ?? carrier.dockingAccess;
      if (typeof event.AllowNotorious === "boolean") carrier.allowNotorious = event.AllowNotorious;
    }
    if (type === "CarrierCrewServices" && event.CrewRole) {
      const key = normalized(event.CrewRole);
      const service = carrier.services.get(key) ?? { key, name: SERVICES[key] ?? event.CrewRole, installed: false, enabled: false };
      const operation = String(event.Operation).toLowerCase();
      if (operation === "activate") { service.installed = true; service.enabled = true; }
      if (operation === "deactivate") { service.installed = false; service.enabled = false; }
      if (operation === "pause") service.enabled = false;
      if (operation === "resume") { service.installed = true; service.enabled = true; }
      service.crewName = event.CrewName ?? service.crewName ?? "";
      service.updatedAt = timestamp;
      carrier.services.set(key, service);
      this.addTransaction(carrier, event, "service", "Услуга: " + service.name + " · " + ({
        activate: "установлена", deactivate: "снята", pause: "приостановлена", resume: "возобновлена", replace: "замена экипажа",
      }[operation] ?? event.Operation));
    }
    if (type === "CarrierTradeOrder" && event.Commodity) {
      const resource = catalog.getResource(event.Commodity);
      const key = resource?.key ?? normalized(event.Commodity);
      const orderKey = key + ":" + (event.BlackMarket === true ? "black" : "legal");
      if (event.CancelTrade === true) carrier.orders.delete(orderKey);
      else {
        const side = numeric(event.PurchaseOrder) != null ? "buy" : numeric(event.SaleOrder) != null ? "sell" : null;
        if (side) {
          carrier.orders.set(orderKey, { key, name: resourceName(key, event.Commodity_Localised),
            side, price: numeric(event.Price), quantity: numeric(side === "buy" ? event.PurchaseOrder : event.SaleOrder),
            blackMarket: event.BlackMarket === true, timestamp });
          if (side === "sell") {
            const old = carrier.items.get(key);
            carrier.items.set(key, { key, name: resourceName(key, event.Commodity_Localised),
              count: Math.max(0, Number(event.SaleOrder)), netTransferred: old?.netTransferred ?? 0,
              basis: "Количество в заявке продажи", snapshotAt: timestamp, updatedAt: timestamp });
          }
        }
      }
    }
    if (type === "CargoTransfer" && this.isOwnDockedMarket()) {
      for (const item of event.Transfers ?? []) {
        const resource = catalog.getResource(item.Type);
        const key = resource?.key ?? normalized(item.Type);
        const count = numeric(item.Count);
        const direction = String(item.Direction).toLowerCase();
        if (count == null || count <= 0 || !["tocarrier", "toship"].includes(direction)) continue;
        const delta = direction === "tocarrier" ? count : -count;
        this.moveStock(carrier, key, item.Type_Localised, delta, timestamp, "transfer");
        this.addTransaction(carrier, event, "cargo", resourceName(key, item.Type_Localised) +
          (delta > 0 ? " на склад: " : " на корабль: ") + count + " т");
      }
    }
    if (["MarketBuy", "MarketSell"].includes(type) && this.isOwnDockedMarket(event.MarketID)) {
      const resource = catalog.getResource(event.Type);
      const key = resource?.key ?? normalized(event.Type);
      const count = numeric(event.Count);
      if (count != null && count > 0) {
        const delta = type === "MarketSell" ? count : -count;
        this.moveStock(carrier, key, event.Type_Localised, delta, timestamp, "owner-market");
        this.addTransaction(carrier, event, "owner-market", resourceName(key, event.Type_Localised) +
          " · собственный рынок: " + count + " т");
      }
    }
    if (type === "CarrierDecommission") {
      carrier.pendingDecommission = true;
      const scrap = numeric(event.ScrapTime);
      carrier.scrapTime = scrap != null ? new Date(scrap * 1000).toISOString() : event.ScrapTime ?? null;
    }
    if (type === "CarrierCancelDecommission") {
      carrier.pendingDecommission = false;
      carrier.scrapTime = null;
    }
    if (type === "CarrierShipPack" || type === "CarrierModulePack") {
      const cost = numeric(event.Cost);
      const refund = numeric(event.Refund);
      this.addTransaction(carrier, event, "pack", (type === "CarrierShipPack" ? "Комплект кораблей" : "Комплект модулей") +
        " · " + String(event.PackTheme ?? ""), cost != null ? -cost : refund);
    }
    if (type.startsWith("Carrier") && (explicitId === carrier.id || type === "CarrierJump")) carrier.updatedAt = timestamp;
    if (context.commander?.system === carrier.location?.system && Array.isArray(context.commander.coordinates)) {
      carrier.location.coordinates ??= context.commander.coordinates;
    }
  }

  warehouseFor(carrier) {
    if (!carrier) return { items: [], knownTons: 0, totalReportedTons: null, complete: false,
      sourceLabel: "Склад ещё не прочитан", updatedAt: null };
    let items = [...carrier.items.values()].map((item) => ({ ...item }));
    const baseline = this.stockBaseline?.carrierId === carrier.id ? this.stockBaseline : null;
    if (baseline) {
      const byKey = new Map(baseline.items.map((item) => [item.key, { ...item, netTransferred: 0,
        basis: "Ручной снимок склада", snapshotAt: baseline.timestamp, updatedAt: baseline.timestamp }]));
      for (const movement of carrier.movements) {
        if (String(movement.timestamp ?? "") <= baseline.timestamp) continue;
        const item = byKey.get(movement.key) ?? { key: movement.key, name: movement.name, count: 0,
          netTransferred: 0, basis: "После ручного снимка", snapshotAt: baseline.timestamp };
        item.count = Math.max(0, item.count + movement.delta);
        item.netTransferred += movement.delta;
        item.updatedAt = movement.timestamp;
        byKey.set(item.key, item);
      }
      for (const item of items) {
        if (item.snapshotAt && item.snapshotAt > baseline.timestamp) byKey.set(item.key, item);
      }
      items = [...byKey.values()];
    }
    items = items.filter((item) => item.count > 0 || item.netTransferred !== 0)
      .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name, "ru"));
    return {
      items, knownTons: items.reduce((sum, item) => sum + item.count, 0),
      totalReportedTons: carrier.space.cargoTons, complete: Boolean(baseline),
      sourceLabel: baseline ? "Ручной снимок и последующие записанные операции" : "Заявки продажи и записанные перемещения",
      updatedAt: items.map((item) => item.updatedAt).filter(Boolean).sort().at(-1) ?? carrier.space.updatedAt,
      manualAt: baseline?.timestamp ?? null,
      caveat: "Сделки других командиров и начальный состав склада могут отсутствовать в вашем журнале. Общая загрузка и состав показаны по последним известным данным.",
    };
  }

  getState(nowMs = Date.now()) {
    const carrier = this.current();
    const warehouse = this.warehouseFor(carrier);
    const fuel = { ...(carrier?.fuel ?? emptyFuel()),
      reserveTons: warehouse.items.find((item) => item.key === "tritium")?.count ?? (warehouse.complete ? 0 : null) };
    const finance = { ...(carrier?.finance ?? emptyFinance()), weeklyUpkeep: this.planning.weeklyUpkeep,
      upkeepSource: this.planning.weeklyUpkeep != null ? "manual" : null,
      runwayWeeks: carrier?.finance.balance != null && this.planning.weeklyUpkeep > 0
        ? Math.max(0, carrier.finance.balance / this.planning.weeklyUpkeep) : null };
    const pending = carrier?.jump.pending ?? null;
    const arrival = finiteDate(carrier?.jump.lastArrival);
    const cooldownUntilMs = arrival != null ? arrival + 5 * 60000 : null;
    const phase = pending ? pending.departureMs != null && nowMs >= pending.departureMs ? "departure-passed" : "countdown"
      : cooldownUntilMs != null && nowMs < cooldownUntilMs ? "cooldown" : "idle";
    const advisories = [];
    if (carrier && !carrier.statsAt) advisories.push({ level: "info", text: "Откройте управление флотоносцем в игре, чтобы получить баланс, тритий, услуги и вместимость." });
    if (fuel.needsRefresh) advisories.push({ level: "info", text: "После прыжка обновите управление флотоносцем: запас топлива мог измениться." });
    if (fuel.tankTons != null && fuel.tankTons < 100) advisories.push({ level: "warn", text: "В тритиевом баке меньше 100 т. Проверьте заправку перед следующим прыжком." });
    if (finance.runwayWeeks != null && finance.runwayWeeks < 4) advisories.push({ level: "warn", text: "Баланса хватит менее чем на четыре недели по введённым расходам." });
    if (carrier?.pendingDecommission) advisories.push({ level: "warn", text: "В журнале есть запрос на списание флотоносца." });
    if (phase === "departure-passed") advisories.push({ level: "info", text: "Время отправления прошло. Фактическое прибытие ещё не подтверждено журналом." });
    return {
      owned: Boolean(carrier), id: carrier?.id ?? null, name: carrier?.name ?? "", callsign: carrier?.callsign ?? "",
      updatedAt: carrier?.updatedAt ?? null, statsAt: carrier?.statsAt ?? null, location: carrier?.location ?? null,
      dockingAccess: carrier?.dockingAccess ?? null, dockingAccessLabel: ACCESS[carrier?.dockingAccess] ?? carrier?.dockingAccess ?? "Неизвестно",
      allowNotorious: carrier?.allowNotorious ?? null, pendingDecommission: carrier?.pendingDecommission ?? false,
      scrapTime: carrier?.scrapTime ?? null, dockedHere: this.isOwnDockedMarket(), finance, fuel,
      space: carrier?.space ?? emptySpace(), services: [...(carrier?.services.values() ?? [])], warehouse,
      orders: [...(carrier?.orders.values() ?? [])].sort((a, b) => String(b.timestamp ?? "").localeCompare(String(a.timestamp ?? ""))),
      jump: { pending, lastArrival: carrier?.jump.lastArrival ? { timestamp: carrier.jump.lastArrival,
        system: carrier.location?.system ?? "", bodyId: carrier.location?.bodyId ?? null } : null,
        cooldownUntilMs, cooldownEstimated: true,
        phase, secondsToDeparture: pending?.departureMs != null ? Math.max(0, (pending.departureMs - nowMs) / 1000) : null,
        arrivalConfirmed: !pending && Boolean(carrier?.location) },
      transactions: carrier?.transactions ?? [], advisories,
      planning: { ...this.planning,
        tritiumNeededTank: fuel.tankTons != null ? Math.max(0, this.planning.targetTankTons - fuel.tankTons) : null,
        tritiumNeededReserve: fuel.reserveTons != null ? Math.max(0, this.planning.reserveTritium - fuel.reserveTons) : null },
      onboarding: [
        "После покупки откройте управление личным флотоносцем в игре.",
        "Журнал передаст имя, позывной, баланс, тритий, услуги и общую загрузку.",
        "Перемещения груза и заявки рынка дополнят известный состав склада.",
        "Введите недельные расходы из панели флотоносца для расчёта финансового запаса.",
      ],
      sources: [{ title: "Frontier Player Journal · Fleet Carriers", url: SOURCE }],
    };
  }
}

module.exports = { CarrierTracker, SERVICES, ACCESS };
