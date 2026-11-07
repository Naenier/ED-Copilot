"use strict";

const crypto = require("node:crypto");
const finite = (value) => value != null && value !== "" && Number.isFinite(Number(value)) ? Number(value) : null;
const clean = (value, maximum = 150) => typeof value === "string" ? value.trim().slice(0, maximum) : "";
const time = (value) => Number.isFinite(Date.parse(value)) ? Date.parse(value) : null;
const same = (a, b) => String(a || "").toLowerCase() === String(b || "").toLowerCase();
const KINDS = new Set(["travel", "trade", "mining", "engineering", "exploration", "combat", "mission", "carrier", "odyssey", "other"]);
const LABELS = {
  LoadGame: "Начало сеанса", FSDJump: "Гиперпрыжок", CarrierJump: "Перелёт на флотоносце", Docked: "Стыковка", Undocked: "Отстыковка",
  Touchdown: "Посадка на поверхность", Liftoff: "Взлёт с поверхности", Disembark: "Выход пешком", Embark: "Посадка на борт", LaunchSRV: "Выезд на ТРП", DockSRV: "Возвращение ТРП",
  MissionAccepted: "Принято поручение", MissionCompleted: "Поручение выполнено", MissionFailed: "Поручение провалено", MissionAbandoned: "Отказ от поручения", MissionRedirected: "Новое назначение поручения",
  MarketBuy: "Покупка товара", MarketSell: "Продажа товара", MaterialTrade: "Обмен материалов", EngineerCraft: "Инженерное улучшение", Died: "Потеря корабля", Resurrect: "Восстановление",
  FSSDiscoveryScan: "Обзор системы", SAAScanComplete: "Картографирование тела", ScanOrganic: "Исследование органики", SellOrganicData: "Продажа биологических данных",
  SellExplorationData: "Продажа исследовательских данных", MultiSellExplorationData: "Продажа исследовательских данных", Bounty: "Награда за цель", FactionKillBond: "Боевой ваучер", Shutdown: "Завершение игры",
};

function normalizeObjective(input = {}, existing = null) {
  const title = clean(input.title ?? existing?.title, 120);
  if (!title) throw new Error("Укажите название цели.");
  const kind = input.kind ?? existing?.kind ?? "travel";
  if (!KINDS.has(kind)) throw new Error("Выберите тип цели.");
  return { id: typeof existing?.id === "string" ? existing.id.slice(0, 100) : crypto.randomUUID(), title, kind,
    system: clean(input.system ?? existing?.system), station: clean(input.station ?? existing?.station), body: clean(input.body ?? existing?.body),
    notes: clean(input.notes ?? existing?.notes, 1200), priority: (input.priority ?? existing?.priority) === "high" ? "high" : "normal",
    status: existing?.status === "completed" ? "completed" : "active", createdAt: time(existing?.createdAt) != null ? existing.createdAt : new Date().toISOString(),
    completedAt: existing?.completedAt || null };
}

function missionName(event, previous) {
  return clean(event.LocalisedName || event.Name_Localised || previous?.name, 240) || "Поручение №" + event.MissionID;
}

class PilotTracker {
  constructor() { this.reset(); }
  reset() {
    this.credits = null; this.creditsAt = null; this.creditsSource = "";
    this.fuel = { mainTons: null, mainCapacityTons: null, updatedAt: null };
    this.missions = new Map(); this.recent = []; this.navRoute = []; this.navTarget = null;
    this.routeAt = null; this.routeClearedAt = null; this.routeExpected = false; this.system = ""; this.station = "";
    this.session = { since: null, jumps: 0, distanceLy: 0 }; this.legalState = "";
  }
  setCredits(value, at, source) {
    const number = finite(value);
    if (number != null && number >= 0 && (!this.creditsAt || time(at) >= time(this.creditsAt))) {
      this.credits = number; this.creditsAt = at || null; this.creditsSource = source;
    }
  }
  setFuel(value, at) {
    const number = finite(value);
    if (number != null && number >= 0 && (!this.fuel.updatedAt || time(at) >= time(this.fuel.updatedAt))) {
      this.fuel.mainTons = number; this.fuel.updatedAt = at || null;
    }
  }
  status(event) {
    this.setCredits(event.Balance, event.timestamp, "Status.json");
    this.setFuel(event.Fuel?.FuelMain, event.timestamp);
    if (typeof event.LegalState === "string") this.legalState = event.LegalState;
  }
  route(event, commander = {}) {
    if (!Array.isArray(event.Route)) return;
    if (this.routeClearedAt && (!event.timestamp || time(event.timestamp) < time(this.routeClearedAt) ||
        time(event.timestamp) === time(this.routeClearedAt) && !this.routeExpected)) return;
    if (this.routeAt && event.timestamp && time(event.timestamp) < time(this.routeAt)) return;
    this.routeAt = event.timestamp || null;
    this.navRoute = event.Route.slice(0, 1000).filter((row) => typeof row.StarSystem === "string").map((row) => ({
      system: clean(row.StarSystem), systemAddress: row.SystemAddress ?? null,
      coordinates: Array.isArray(row.StarPos) && row.StarPos.length === 3 ? row.StarPos : null,
      starClass: clean(row.StarClass, 30), visited: false,
    }));
    const index = this.navRoute.findIndex((row) => same(row.system, commander.system || this.system));
    if (index >= 0) for (let i = 0; i <= index; i++) this.navRoute[i].visited = true;
    this.navTarget = this.navRoute.find((row) => !row.visited) ? {
      system: this.navRoute.find((row) => !row.visited).system,
      remainingJumps: this.navRoute.filter((row) => !row.visited).length,
    } : null;
  }
  apply(event, context = {}) {
    if (!event || typeof event.event !== "string") return;
    const type = event.event, at = event.timestamp || null;
    if (type === "LoadGame") {
      this.session = { since: at, jumps: 0, distanceLy: 0 };
      this.setCredits(event.Credits, at, "Журнал: начало сеанса");
      this.setFuel(event.FuelLevel, at);
      const capacity = finite(event.FuelCapacity);
      if (capacity != null) this.fuel.mainCapacityTons = capacity;
    }
    if (type === "Loadout") {
      const capacity = finite(event.FuelCapacity?.Main ?? event.FuelCapacity);
      if (capacity != null) this.fuel.mainCapacityTons = capacity;
    }
    if (["Location", "FSDJump", "CarrierJump"].includes(type)) {
      this.system = clean(event.StarSystem) || this.system;
      this.station = type === "Location" && event.Docked ? clean(event.StationName) : "";
      this.setFuel(event.FuelLevel, at);
      if (type === "FSDJump") {
        this.session.jumps += 1;
        this.session.distanceLy += Math.max(0, finite(event.JumpDist) || 0);
        const next = this.navRoute.findIndex((row) => !row.visited);
        if (next >= 0 && same(this.navRoute[next].system, this.system)) this.navRoute[next].visited = true;
        const remaining = this.navRoute.filter((row) => !row.visited);
        this.navTarget = remaining.length ? { system: remaining[0].system, remainingJumps: remaining.length } : null;
      }
    }
    if (type === "Docked") this.station = clean(event.StationName);
    if (type === "Undocked") this.station = "";
    if (type === "FuelScoop") this.setFuel(event.Total, at);
    if (type === "Status") this.status(event);
    if (type === "NavRoute") { this.routeExpected = true; this.route(event, context.commander); }
    if (type === "NavRouteClear") { this.navRoute = []; this.navTarget = null; this.routeAt = at; this.routeClearedAt = at; this.routeExpected = false; }
    if (type === "FSDTarget") this.navTarget = { system: clean(event.Name), systemAddress: event.SystemAddress ?? null,
      remainingJumps: finite(event.RemainingJumpsInRoute), starClass: clean(event.StarClass, 30) };
    const id = event.MissionID != null ? String(event.MissionID) : null;
    if (type === "MissionAccepted" && id) {
      const previous = this.missions.get(id);
      this.missions.set(id, { id, name: missionName(event, previous), status: "active", acceptedAt: at,
        destinationSystem: clean(event.NewDestinationSystem || event.DestinationSystem),
        destinationStation: clean(event.NewDestinationStation || event.DestinationStation || event.DestinationSettlement),
        expiry: time(event.Expiry) != null ? event.Expiry : null, reward: finite(event.Reward), faction: clean(event.Faction),
        cargo: clean(event.Commodity_Localised), count: finite(event.Count), updatedAt: at });
    }
    if (type === "MissionRedirected" && id) {
      const mission = this.missions.get(id) || { id, name: missionName(event), status: "active" };
      Object.assign(mission, { destinationSystem: clean(event.NewDestinationSystem), destinationStation: clean(event.NewDestinationStation), updatedAt: at });
      this.missions.set(id, mission);
    }
    const statuses = { MissionCompleted: "completed", MissionFailed: "failed", MissionAbandoned: "abandoned" };
    if (statuses[type] && id) {
      const mission = this.missions.get(id) || { id, name: missionName(event) };
      Object.assign(mission, { status: statuses[type], updatedAt: at }); this.missions.set(id, mission);
    }
    if (type === "Missions" && Array.isArray(event.Active)) {
      const present = new Set([...event.Active, ...(event.Failed || []), ...(event.Complete || [])].map((row) => String(row.MissionID)));
      for (const mission of this.missions.values()) if (["active", "ready"].includes(mission.status) && !present.has(mission.id)) mission.status = "untracked";
      for (const [field, status] of [["Active", "active"], ["Failed", "failed"], ["Complete", "ready"]]) {
        for (const row of event[field] || []) {
          const key = String(row.MissionID), previous = this.missions.get(key);
          const expiry = finite(row.Expires) != null && time(at) != null ? new Date(time(at) + Math.max(0, row.Expires) * 1000).toISOString() : previous?.expiry || null;
          this.missions.set(key, { ...previous, id: key, name: missionName(row, previous), status, expiry, updatedAt: at });
        }
      }
    }
    if (LABELS[type]) {
      const detail = type.startsWith("Mission") ? missionName(event, id ? this.missions.get(id) : null)
        : type === "FSDJump" ? (finite(event.JumpDist) ?? "?") + " св. г."
          : clean(event.Type_Localised || event.BodyName || event.Body || event.ShipName);
      this.recent.unshift({ timestamp: at, event: type, label: LABELS[type], system: this.system || context.commander?.system || "", station: this.station, detail });
      this.recent = this.recent.slice(0, 40);
    }
  }
  snapshot() {
    const missions = [...this.missions.values()].filter((row) => ["active", "ready"].includes(row.status)).map((row) => ({ ...row,
      expired: row.expiry ? Date.parse(row.expiry) < Date.now() : false })).sort((a, b) => (time(a.expiry) ?? Infinity) - (time(b.expiry) ?? Infinity));
    return { credits: this.credits, creditsAt: this.creditsAt, creditsSource: this.creditsSource, fuel: { ...this.fuel },
      missions, navRoute: this.navRoute.map((row) => ({ ...row })), navTarget: this.navTarget ? { ...this.navTarget } : null,
      routeAt: this.routeAt, recent: this.recent, session: { ...this.session }, legalState: this.legalState };
  }
}

module.exports = { PilotTracker, normalizeObjective, KINDS };
