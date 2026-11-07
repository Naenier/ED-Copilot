"use strict";

// Personal-account uploads only. Protocol: https://inara.cz/elite/inara-api-docs/
// Persistence, commander partitioning and live/replay boundaries belong to Integrations.
const crypto = require("node:crypto");
const ENDPOINT = "https://inara.cz/inapi/v1/";
const APP_NAME = "ED Copilot";
const MAX_AGE_MS = 30 * 86400000;
const MAX_RESPONSE_BYTES = 2 * 1024 * 1024;
const CATEGORIES = ["raw", "manufactured", "encoded"];
const RANKS = { Combat: "combat", Trade: "trade", Explore: "explore", CQC: "cqc", Soldier: "soldier",
  Exobiologist: "exobiologist", Federation: "federation", Empire: "empire" };
const SUPPORTED_EVENTS = new Set([
  "setCommanderCredits", "setCommanderGameStatistics", "setCommanderRankEngineer", "setCommanderRankPilot",
  "setCommanderRankPower", "setCommanderReputationMajorFaction", "setCommanderReputationMinorFaction",
  "setCommanderInventoryCargo", "setCommanderInventoryMaterials", "resetCommanderInventory", "setCommanderInventory",
  "setCommanderStorageModules", "setCommanderShip", "setCommanderShipLoadout", "addCommanderShip", "delCommanderShip",
  "setCommanderShipTransfer", "setCommanderSuitLoadout", "updateCommanderSuitLoadout", "delCommanderSuitLoadout",
  "addCommanderTravelCarrierJump", "addCommanderTravelDock", "addCommanderTravelFSDJump", "addCommanderTravelLand",
  "setCommanderTravelLocation", "addCommanderMission", "setCommanderMissionAbandoned", "setCommanderMissionCompleted",
  "setCommanderMissionFailed", "addCommanderPermit"
]);

class InaraError extends Error {
  constructor(message, code = "INARA_ERROR", retryable = false, retryAfterMs = 0) {
    super(message); this.name = "InaraError"; this.code = code;
    this.retryable = retryable; this.retryAfterMs = retryAfterMs;
  }
}
const obj = (value) => value && typeof value === "object" && !Array.isArray(value) ? value : {};
const str = (value) => typeof value === "string" ? value.trim() : "";
const number = (value) => typeof value === "number" && Number.isFinite(value) ? value : null;
const integer = (value) => Number.isSafeInteger(value) && value >= 0 ? value : null;
const nowValue = (value) => typeof value === "function" ? value() : value ?? Date.now();
function validTime(value, now = Date.now()) {
  const at = typeof value === "string" ? Date.parse(value) : NaN;
  return Number.isFinite(at) && at <= now + 300000 && at >= now - MAX_AGE_MS;
}
function cleanData(source, mapping) {
  const out = {};
  for (const [target, field] of Object.entries(mapping)) {
    const value = source[field];
    if (typeof value === "string" && value.trim() || typeof value === "boolean" || number(value) !== null) out[target] = value;
  }
  return out;
}
function canonical(value) {
  if (Array.isArray(value)) return "[" + value.map(canonical).join(",") + "]";
  if (value && typeof value === "object") return "{" + Object.keys(value).sort().filter((key) => value[key] !== undefined)
    .map((key) => JSON.stringify(key) + ":" + canonical(value[key])).join(",") + "}";
  return JSON.stringify(value);
}
function eventKey(event, identity = {}) {
  return crypto.createHash("sha256").update(canonical({ commander: identity.frontierId || identity.fid || identity.commanderName || "",
    eventName: event.eventName, eventTimestamp: event.eventTimestamp, eventData: event.eventData })).digest("hex");
}
function snapshotKey(event) {
  // Only complete, replaceable snapshots are coalesced. Mission/travel history must keep every entry.
  if (["setCommanderInventoryCargo", "setCommanderInventoryMaterials", "setCommanderCredits", "setCommanderGameStatistics",
    "setCommanderStorageModules", "setCommanderTravelLocation"].includes(event.eventName)) return event.eventName;
  if (event.eventName === "setCommanderShipLoadout") {
    return event.eventName + ":" + event.eventData.shipType + ":" + event.eventData.shipGameID;
  }
  return null;
}

function validateLive(metadata = {}) {
  const version = str(metadata.gameVersion || metadata.gameversion || metadata.GameVersion).toLowerCase();
  if (metadata.isBeta === true || metadata.beta === true || /beta|alpha|test/.test(version)) {
    return { ok: false, reason: "Inara не принимает данные бета-версий Elite Dangerous." };
  }
  if (metadata.legacy === true || metadata.isLegacy === true || metadata.live === false || /legacy/.test(version) || /^3\./.test(version)) {
    return { ok: false, reason: "Inara принимает только Live-галактику: Odyssey / Horizons 4.0." };
  }
  if (metadata.multicrew === true || metadata.inMulticrew === true) {
    return { ok: false, reason: "Синхронизация личного имущества недоступна в чужом экипаже." };
  }
  if (metadata.live === true || /^([4-9]|[1-9]\d+)\./.test(version)) return { ok: true, reason: "" };
  return { ok: false, reason: "Нужен журнал сеанса Live-версии игры; версия галактики пока неизвестна." };
}

function inventory(rows, nameField = "Name", countField = "Count") {
  if (!Array.isArray(rows)) return null;
  const out = [];
  for (const row of rows) {
    if (!row || !str(row[nameField]) || integer(row[countField]) === null) return null;
    const data = { itemName: row[nameField], itemCount: row[countField] };
    if (integer(row.MissionID) !== null) data.missionGameID = row.MissionID;
    if (integer(row.Stolen) !== null && row.Stolen > 0 && row.Stolen <= row[countField]) {
      out.push({ ...data, itemCount: row.Stolen, isStolen: true });
      if (row[countField] > row.Stolen) out.push({ ...data, itemCount: row[countField] - row.Stolen, isStolen: false });
    } else out.push(data);
  }
  return out.sort((a, b) => a.itemName.localeCompare(b.itemName) || (a.missionGameID || 0) - (b.missionGameID || 0));
}
function engineer(row) {
  const result = { engineerName: str(row.Engineer || row.english) };
  const stage = str(row.Progress || row.journalStatus);
  if (["Invited", "Acquainted", "Unlocked", "Barred"].includes(stage)) result.rankStage = stage;
  const rank = row.Rank ?? row.rank;
  if (integer(rank) !== null && rank >= 1 && rank <= 5) result.rankValue = rank;
  return result.engineerName && (result.rankStage || result.rankValue) ? result : null;
}
function engineering(source = {}) {
  const result = cleanData(source, { blueprintName: "BlueprintName", blueprintLevel: "Level", blueprintQuality: "Quality",
    experimentalEffect: "ExperimentalEffect" });
  if (Array.isArray(source.Modifiers)) result.modifiers = source.Modifiers.filter((row) => str(row.Label)).map((row) => ({
    name: row.Label, ...cleanData(row, { value: number(row.Value) === null ? "ValueStr" : "Value", originalValue: "OriginalValue", lessIsGood: "LessIsGood" })
  }));
  return Object.keys(result).length ? result : null;
}
function shipIdentity(event, context = {}) {
  const type = str(event.Ship || event.ShipType || context.shipType);
  const id = integer(event.ShipID ?? context.shipId);
  // Fighters and SRVs are not independent commander ships on Inara.
  return type && id !== null && !/(?:fighter)|^(testbuggy|scarab|scorpion|combat_.*)$/i.test(type)
    ? { shipType: type, shipGameID: id } : null;
}
function modules(rows) {
  if (!Array.isArray(rows) || rows.some((row) => !str(row?.Slot) || !str(row?.Item))) return null;
  return rows.map((row) => {
    const result = { slotName: row.Slot, itemName: row.Item, ...cleanData(row, { itemHealth: "Health", isOn: "On", isHot: "Hot",
      itemPriority: "Priority", itemAmmoClip: "AmmoInClip", itemAmmoHopper: "AmmoInHopper", itemValue: "Value" }) };
    const modifications = engineering(row.Engineering); if (modifications) result.engineering = modifications;
    return result;
  });
}
function location(event, commander = {}) {
  const system = str(event.StarSystem || event.System || commander.system);
  if (!system) return null;
  const data = { starsystemName: system };
  const coords = event.StarPos || commander.coordinates;
  if (Array.isArray(coords) && coords.length === 3 && coords.every((v) => number(v) !== null)) data.starsystemCoords = coords;
  return data;
}
function bodyPosition(event, data) {
  const body = str(event.Body || event.BodyName);
  if (body) data.starsystemBodyName = body;
  if (number(event.Latitude) !== null && number(event.Longitude) !== null) data.starsystemBodyCoords = [event.Latitude, event.Longitude];
}
function travellingShip(event, context) {
  if (event.Taxi || context.taxi) return event.Dropship || context.dropship ? { isTaxiDropship: true } : { isTaxiShuttle: true };
  return shipIdentity({}, context) || {};
}

/** Pure snapshot mapping; timestamps describe the observation, never the button click. */
function buildSnapshotEvents(state = {}, context = {}) {
  if (!validateLive(context).ok) return [];
  const now = nowValue(context.now), events = [];
  const add = (eventName, eventTimestamp, eventData) => {
    if (validTime(eventTimestamp, now)) events.push({ eventName, eventTimestamp, eventData });
  };
  const commander = obj(state.commander), pilot = obj(state.pilot), stocks = obj(state.engineering);
  const cargoAt = context.cargoForSyncAt || commander.cargoForSyncAt;
  if ((commander.vehicle === "ship" || context.cargoIsShip === true) && cargoAt) {
    const data = inventory(Array.isArray(commander.cargoForSync) ? commander.cargoForSync : commander.cargoForSync?.Inventory);
    if (data !== null) add("setCommanderInventoryCargo", cargoAt, data);
  }
  // A partial material-category snapshot would erase the other two inventories on Inara.
  const known = obj(stocks.shopping?.knownCategories), materialRows = stocks.materials;
  if (CATEGORIES.every((category) => known[category] === true) && Array.isArray(materialRows) &&
      materialRows.length && materialRows.every((row) => integer(row.owned) !== null)) {
    const times = CATEGORIES.map((category) => stocks.shopping?.updatedAt?.[category]);
    if (times.every((at) => validTime(at, now))) {
      const at = times.reduce((a, b) => Date.parse(a) > Date.parse(b) ? a : b);
      const rows = materialRows.map((row) => ({ Name: row.symbol || row.key, Count: row.owned }));
      const data = inventory(rows); if (data !== null) add("setCommanderInventoryMaterials", at, data);
    }
  }
  for (const row of Array.isArray(stocks.engineers) ? stocks.engineers : []) {
    // Use journalStatus, never the manual unlocked checkbox shown by the planner.
    const data = engineer(row); if (data) add("setCommanderRankEngineer", row.updatedAt, data);
  }
  const loan = context.loan ?? pilot.loan;
  // Inara treats an omitted commanderLoan as zero. Do not accidentally forgive an unknown loan.
  if (integer(pilot.credits) !== null && integer(loan) !== null && validTime(pilot.creditsAt, now)) {
    add("setCommanderCredits", pilot.creditsAt, { commanderCredits: pilot.credits, commanderLoan: loan });
  }
  const positionAt = context.locationUpdatedAt || commander.locationUpdatedAt;
  const current = location({}, commander);
  if (current) {
    if (commander.docked && str(commander.station)) {
      current.stationName = commander.station;
      if (integer(commander.marketId) !== null) current.marketID = commander.marketId;
    }
    add("setCommanderTravelLocation", positionAt, current);
  }
  // The API documents no carrier balance, fuel or warehouse event. Do not masquerade those as ship cargo.
  return events;
}

/** Called only for newly appended journal records; root integration gates replay and commander identity. */
function journalEvents(event, context = {}) {
  if (!event || !str(event.event) || context.replay || context.historical || !validateLive(context).ok) return [];
  const now = nowValue(context.now);
  if (!validTime(event.timestamp, now)) return [];
  const result = [], commander = obj(context.commander || context.state?.commander);
  const add = (eventName, eventData) => result.push({ eventName, eventTimestamp: event.timestamp, eventData });
  const type = event.event;
  if (type === "LoadGame" && integer(event.Credits) !== null) {
    add("setCommanderCredits", { commanderCredits: event.Credits, ...(integer(event.Loan) !== null ? { commanderLoan: event.Loan } : {}) });
  }
  if (type === "Statistics") {
    // Pass the complete original Statistics payload. ED Copilot's own mining stats are not game stats.
    const data = Object.fromEntries(Object.entries(event).filter(([key]) => key !== "event" && key !== "timestamp"));
    if (Object.keys(data).length) add("setCommanderGameStatistics", data);
  }
  if (["Rank", "Promotion", "Progress"].includes(type)) {
    const progress = type === "Progress", rows = [];
    for (const [field, rankName] of Object.entries(RANKS)) {
      const value = event[field];
      if (progress ? number(value) !== null && value >= 0 && value <= 100 : integer(value) !== null && value <= 14) {
        rows.push({ rankName, [progress ? "rankProgress" : "rankValue"]: progress ? value / 100 : value });
      }
    }
    if (rows.length) add("setCommanderRankPilot", rows);
  }
  if (type === "EngineerProgress") {
    const data = (Array.isArray(event.Engineers) ? event.Engineers : [event]).map(engineer).filter(Boolean);
    if (data.length) add("setCommanderRankEngineer", data);
  }
  if (type === "Reputation") {
    const data = Object.entries(event).filter(([key, value]) => ["Empire", "Federation", "Alliance", "Independent"].includes(key) && number(value) !== null && Math.abs(value) <= 100)
      .map(([key, value]) => ({ majorfactionName: key.toLowerCase(), majorfactionReputation: value / 100 }));
    if (data.length) add("setCommanderReputationMajorFaction", data);
  }
  if (["Location", "FSDJump", "CarrierJump"].includes(type) && Array.isArray(event.Factions)) {
    const data = event.Factions.filter((row) => str(row.Name) && number(row.MyReputation) !== null && Math.abs(row.MyReputation) <= 100)
      .map((row) => ({ minorfactionName: row.Name, minorfactionReputation: row.MyReputation / 100 }));
    if (data.length) add("setCommanderReputationMinorFaction", data);
  }
  if (["Powerplay", "PowerplayRank", "PowerplayJoin", "PowerplayDefect", "PowerplayLeave"].includes(type)) {
    const powerName = str(type === "PowerplayDefect" ? event.ToPower : event.Power);
    const rankValue = type === "PowerplayLeave" ? -1 : ["PowerplayJoin", "PowerplayDefect"].includes(type) ? 1 : integer(event.Rank);
    if (powerName && rankValue !== null) add("setCommanderRankPower", { powerName, rankValue, ...(integer(event.Merits) !== null ? { meritsValue: event.Merits } : {}) });
  }
  if (type === "Cargo" && (!event.Vessel || String(event.Vessel).toLowerCase() === "ship")) {
    const data = inventory(event.Inventory); if (data !== null) add("setCommanderInventoryCargo", data);
  }
  if (type === "Materials" && ["Raw", "Manufactured", "Encoded"].every((key) => Array.isArray(event[key]))) {
    const data = inventory([...event.Raw, ...event.Manufactured, ...event.Encoded]);
    if (data !== null) add("setCommanderInventoryMaterials", data);
  }
  if (["ShipLocker", "ShipLockerMaterials"].includes(type)) {
    const groups = ["Items", "Components", "Data", "Consumables"];
    if (groups.every((key) => inventory(event[key]) !== null)) {
      const data = groups.flatMap((key) => inventory(event[key]).map((row) => ({ ...row, itemType: key, itemLocation: "ShipLocker" })));
      // Reset only empty groups; resetting nonempty groups separately would be unsafe on partial acknowledgements.
      const empty = groups.filter((key) => event[key].length === 0).map((itemType) => ({ itemType, itemLocation: "ShipLocker" }));
      if (empty.length) add("resetCommanderInventory", empty);
      if (data.length) add("setCommanderInventory", data);
    }
  }
  if (["Loadout", "SetUserShipName"].includes(type)) {
    const id = shipIdentity(event, context);
    if (id) {
      const data = { ...id, ...cleanData(event, { shipName: type === "SetUserShipName" ? "UserShipName" : "ShipName",
        shipIdent: type === "SetUserShipName" ? "UserShipId" : "ShipIdent", isHot: "Hot", shipHullValue: "HullValue",
        shipModulesValue: "ModulesValue", shipRebuyCost: "Rebuy", shipMaxJumpRange: "MaxJumpRange", shipCargoCapacity: "CargoCapacity" }) };
      if (type === "Loadout") data.isCurrentShip = true;
      add("setCommanderShip", data);
      const loadout = modules(event.Modules);
      if (type === "Loadout" && loadout !== null) add("setCommanderShipLoadout", { ...id, shipLoadout: loadout });
    }
  }
  if (["ShipyardBuy", "ShipyardSwap"].includes(type)) {
    if (str(event.SellOldShip) && integer(event.SellShipID) !== null) add("delCommanderShip", { shipType: event.SellOldShip, shipGameID: event.SellShipID });
    if (str(event.StoreOldShip) && integer(event.StoreShipID) !== null && str(commander.system) && str(commander.station)) {
      add("setCommanderShip", { shipType: event.StoreOldShip, shipGameID: event.StoreShipID, starsystemName: commander.system,
        stationName: commander.station, ...(integer(commander.marketId) !== null ? { marketID: commander.marketId } : {}) });
    }
    if (type === "ShipyardSwap") { const id = shipIdentity(event, context); if (id) add("setCommanderShip", { ...id, isCurrentShip: true }); }
  }
  if (type === "ShipyardNew") {
    const id = shipIdentity({ ...event, ShipID: event.NewShipID ?? event.ShipID }, context); if (id) add("addCommanderShip", id);
  }
  if (["ShipyardSell", "SellShipOnRebuy"].includes(type)) {
    const id = shipIdentity({ ...event, ShipID: event.SellShipID ?? event.ShipID }, context); if (id) add("delCommanderShip", id);
  }
  if (type === "ShipyardTransfer") {
    const id = shipIdentity(event, context), destination = location({}, commander);
    if (id && destination && str(commander.station)) add("setCommanderShipTransfer", { ...id, starsystemName: commander.system,
      stationName: commander.station, ...cleanData(event, { transferTime: "TransferTime" }),
      ...(integer(commander.marketId) !== null ? { marketID: commander.marketId } : {}) });
  }
  if (type === "StoredShips" && Array.isArray(event.ShipsHere) && Array.isArray(event.ShipsRemote)) {
    for (const row of [...event.ShipsHere, ...event.ShipsRemote]) {
      const id = shipIdentity({ ShipType: row.ShipType, ShipID: row.ShipID }); if (!id) continue;
      const here = event.ShipsHere.includes(row);
      add("setCommanderShip", { ...id, ...cleanData(row, { shipName: "Name", isHot: "Hot", starsystemName: "StarSystem", marketID: "ShipMarketID" }),
        ...(here && str(event.StarSystem || commander.system) ? { starsystemName: event.StarSystem || commander.system } : {}),
        ...(here && str(event.StationName || commander.station) ? { stationName: event.StationName || commander.station } : {}),
        ...(here && integer(event.MarketID) !== null ? { marketID: event.MarketID } : {}) });
    }
  }
  if (type === "StoredModules" && Array.isArray(event.Items)) {
    if (event.Items.every((row) => str(row.Name))) add("setCommanderStorageModules", event.Items.map((row) => {
      const data = { itemName: row.Name, ...cleanData(row, { itemValue: "BuyPrice", isHot: "Hot", starsystemName: "StarSystem", marketID: "MarketID" }) };
      const modifications = engineering(row.Engineering ? typeof row.Engineering === "object" ? row.Engineering :
        { BlueprintName: row.Engineering, Level: row.Level, Quality: row.Quality } : {});
      if (modifications) data.engineering = modifications;
      return data;
    }));
  }
  if (["SuitLoadout", "CreateSuitLoadout"].includes(type) && integer(event.LoadoutID) !== null && integer(event.SuitID) !== null &&
      str(event.SuitName) && Array.isArray(event.Modules) && Array.isArray(event.SuitMods)) {
    if (event.Modules.every((row) => str(row.SlotName) && str(row.ModuleName) && integer(row.SuitModuleID) !== null && integer(row.Class) !== null)) {
      add("setCommanderSuitLoadout", { loadoutGameID: event.LoadoutID, loadoutName: event.LoadoutName || "", suitGameID: event.SuitID,
        suitType: event.SuitName, suitMods: event.SuitMods.filter((value) => typeof value === "string"), suitLoadout: event.Modules.map((row) => ({
          slotName: row.SlotName, itemName: row.ModuleName, itemClass: row.Class, itemGameID: row.SuitModuleID,
          engineering: (Array.isArray(row.WeaponMods) ? row.WeaponMods : []).filter((value) => typeof value === "string").map((blueprintName) => ({ blueprintName }))
        })) });
    }
  }
  if (type === "DeleteSuitLoadout" && integer(event.LoadoutID) !== null) add("delCommanderSuitLoadout", { loadoutGameID: event.LoadoutID });
  if (type === "RenameSuitLoadout" && integer(event.LoadoutID) !== null && str(event.LoadoutName)) {
    add("updateCommanderSuitLoadout", { loadoutGameID: event.LoadoutID, loadoutName: event.LoadoutName });
  }
  if (["Location", "FSDJump", "CarrierJump", "Docked", "Touchdown", "ApproachSettlement", "SupercruiseExit"].includes(type)) {
    const data = location(event, commander);
    if (data) {
      let apiName = "setCommanderTravelLocation";
      if (type === "FSDJump" && number(event.JumpDist) !== null) { apiName = "addCommanderTravelFSDJump"; data.jumpDistance = event.JumpDist; }
      if (type === "CarrierJump") apiName = "addCommanderTravelCarrierJump";
      if (type === "Docked" && !context.suppressDock) apiName = "addCommanderTravelDock";
      if (type === "Touchdown" && event.PlayerControlled === true && str(event.Body || event.BodyName)) apiName = "addCommanderTravelLand";
      if (type === "Docked" || type === "CarrierJump" || type === "Location" && event.Docked || type === "ApproachSettlement") {
        const name = str(type === "ApproachSettlement" ? event.Name : event.StationName);
        if (name) data.stationName = name;
        if (integer(event.MarketID) !== null) data.marketID = event.MarketID;
      }
      if (event.BodyType === "Planet" || ["Touchdown", "ApproachSettlement"].includes(type)) bodyPosition(event, data);
      if (apiName.startsWith("addCommanderTravel")) Object.assign(data, travellingShip(event, context));
      // Ignore recalled/unpiloted ship touchdown; it is not the commander's location.
      if (!(type === "Touchdown" && event.PlayerControlled !== true)) add(apiName, data);
    }
  }
  if (type === "MissionAccepted" && integer(event.MissionID) !== null && str(event.Name)) {
    const data = { missionGameID: event.MissionID, missionName: event.Name, ...cleanData(event, { missionExpiry: "Expiry", influenceGain: "Influence",
      reputationGain: "Reputation", minorfactionNameOrigin: "Faction", starsystemNameTarget: "DestinationSystem", stationNameTarget: "DestinationStation",
      minorfactionNameTarget: "TargetFaction", commodityName: "Commodity", commodityCount: "Count", targetName: "Target", targetType: "TargetType",
      killCount: "KillCount", passengerType: "PassengerType", passengerCount: "PassengerCount", passengerIsVIP: "PassengerVIPs", passengerIsWanted: "PassengerWanted" }) };
    if (str(commander.system)) data.starsystemNameOrigin = commander.system;
    if (str(commander.station)) data.stationNameOrigin = commander.station;
    add("addCommanderMission", data);
  }
  if (["MissionCompleted", "MissionFailed", "MissionAbandoned"].includes(type) && integer(event.MissionID) !== null) {
    const data = { missionGameID: event.MissionID };
    if (type === "MissionCompleted") {
      Object.assign(data, cleanData(event, { donationCredits: "Donation", rewardCredits: "Reward" }));
      if (Array.isArray(event.PermitsAwarded)) {
        data.rewardPermits = event.PermitsAwarded.filter((name) => str(name)).map((starsystemName) => ({ starsystemName }));
        for (const permit of data.rewardPermits) add("addCommanderPermit", permit);
      }
      for (const [key, field] of [["rewardCommodities", "CommodityReward"], ["rewardMaterials", "MaterialsReward"]]) {
        const rows = inventory(event[field]); if (rows !== null) data[key] = rows;
      }
      if (Array.isArray(event.FactionEffects)) data.minorfactionEffects = event.FactionEffects.filter((row) => str(row.Faction)).map((row) => {
        const gains = (Array.isArray(row.Influence) ? row.Influence : []).map((x) => str(x.Influence)).filter((x) => /^\++$/.test(x));
        return { minorfactionName: row.Faction, ...cleanData(row, { reputationGain: "Reputation" }),
          ...(gains.length ? { influenceGain: gains.sort((a, b) => b.length - a.length)[0] } : {}) };
      });
    }
    add({ MissionCompleted: "setCommanderMissionCompleted", MissionFailed: "setCommanderMissionFailed", MissionAbandoned: "setCommanderMissionAbandoned" }[type], data);
  }
  return result;
}

function safeMessage(value, apiKey) {
  return str(value).split(apiKey || "\u0000").join("[скрыто]").replace(/[\u0000-\u001f]/g, " ").slice(0, 500);
}
function retryDelay(value, now) {
  if (!value) return 60000;
  const seconds = Number(value);
  const delay = Number.isFinite(seconds) ? seconds * 1000 : Date.parse(value) - now;
  return Number.isFinite(delay) ? Math.max(30000, Math.min(delay, 86400000)) : 60000;
}
async function responseJSON(response, signal) {
  const length = Number(response.headers?.get?.("content-length"));
  if (length > MAX_RESPONSE_BYTES) throw new InaraError("Слишком большой ответ Inara.", "RESPONSE_SIZE", true);
  let text;
  if (response.body?.getReader) {
    const reader = response.body.getReader(), chunks = []; let bytes = 0;
    try {
      for (;;) {
        signal.throwIfAborted();
        const { value, done } = await reader.read(); if (done) break;
        bytes += value.byteLength;
        if (bytes > MAX_RESPONSE_BYTES) throw new InaraError("Слишком большой ответ Inara.", "RESPONSE_SIZE", true);
        chunks.push(Buffer.from(value));
      }
    } finally { try { await reader.cancel(); } catch {} }
    text = Buffer.concat(chunks).toString("utf8");
  } else text = await response.text();
  signal.throwIfAborted();
  if (Buffer.byteLength(text, "utf8") > MAX_RESPONSE_BYTES) throw new InaraError("Слишком большой ответ Inara.", "RESPONSE_SIZE", true);
  try { return JSON.parse(text); } catch { throw new InaraError("Inara вернула пустой или некорректный ответ. Повторим позже.", "RESPONSE_INVALID", true); }
}

/** One attempt only: caller persists pending events, acknowledges individual results and schedules backoff. */
async function sendBatch(options = {}) {
  const { commanderName, frontierId, apiKey, events, appVersion = "1.8.0", signal, isBeingDeveloped = true } = options;
  const fetchImpl = options.fetchImpl || options.fetch || globalThis.fetch, now = nowValue(options.now);
  if (!str(commanderName)) throw new InaraError("Имя командира пока не прочитано из игры.", "COMMANDER_REQUIRED");
  if (!str(apiKey) || apiKey.length > 2048 || /[\r\n\u0000]/.test(apiKey)) throw new InaraError("Укажите личный API-ключ Inara в настройках.", "KEY_REQUIRED");
  if (!/^\d+\.\d+(?:\.\d+)?(?:[-+][\w.-]+)?$/.test(appVersion)) throw new InaraError("Некорректная версия приложения для Inara.", "VERSION_INVALID");
  if (!Array.isArray(events) || events.length > 100) throw new InaraError("Пакет Inara должен содержать не более 100 событий.", "BATCH_INVALID");
  const results = [], outbound = [], indexes = [];
  for (let index = 0; index < events.length; index++) {
    const event = events[index];
    if (!event || !SUPPORTED_EVENTS.has(event.eventName) || !validTime(event.eventTimestamp, now) || event.eventData == null) {
      results[index] = { index, eventCustomID: index + 1, status: 410, ok: false, retryable: false,
        message: "Событие устарело или не поддерживается API Inara." };
      continue;
    }
    indexes.push(index);
    outbound.push({ eventName: event.eventName, eventTimestamp: event.eventTimestamp, eventData: event.eventData, eventCustomID: index + 1 });
  }
  if (!outbound.length) return { ok: results.every((row) => row.ok), headerStatus: null, results, retryable: false, setupRequired: false, message: "Нет новых событий для Inara." };
  const body = JSON.stringify({ header: { appName: APP_NAME, appVersion, isBeingDeveloped: isBeingDeveloped !== false, APIkey: apiKey,
    commanderName: str(commanderName), ...(str(frontierId) ? { commanderFrontierID: str(frontierId) } : {}) }, events: outbound });
  if (Buffer.byteLength(body, "utf8") > 1024 * 1024) throw new InaraError("Слишком большой пакет Inara; уменьшите размер очереди отправки.", "BATCH_SIZE");
  const controller = new AbortController(); let timedOut = false;
  const timeout = setTimeout(() => { timedOut = true; controller.abort(); }, Math.max(100, Math.min(options.timeoutMs ?? 20000, 120000)));
  timeout.unref?.();
  const abort = () => controller.abort();
  if (signal?.aborted) controller.abort(); else signal?.addEventListener("abort", abort, { once: true });
  try {
    controller.signal.throwIfAborted();
    const response = await fetchImpl(ENDPOINT, { method: "POST", redirect: "error", headers: {
      "Content-Type": "application/json", Accept: "application/json", "User-Agent": APP_NAME + "/" + appVersion }, body, signal: controller.signal });
    if (!response.ok) {
      const retryable = response.status === 408 || response.status === 425 || response.status === 429 || response.status >= 500;
      throw new InaraError("Inara: HTTP " + response.status + ". " + (retryable ? "Повторим отправку позже." : "Проверьте доступ приложения и ключ в настройках."),
        "HTTP_" + response.status, retryable, retryable ? retryDelay(response.headers?.get?.("retry-after"), now) : 0);
    }
    const data = await responseJSON(response, controller.signal), headerStatus = integer(data?.header?.eventStatus);
    if (headerStatus === null) throw new InaraError("Ответ Inara не содержит статуса пакета.", "RESPONSE_INVALID", true);
    const headerOK = headerStatus === 200 || headerStatus === 202;
    const message = safeMessage(data.header.eventStatusText, apiKey);
    for (let i = 0; i < indexes.length; i++) {
      const index = indexes[i], row = obj(data.events?.[i]), status = integer(row.eventStatus);
      const matched = row.eventCustomID == null || row.eventCustomID === index + 1;
      const finalStatus = headerOK && matched ? status : headerOK ? null : headerStatus;
      const ok = finalStatus === 200 || finalStatus === 202;
      const retryable = headerOK ? finalStatus === null || finalStatus >= 500 || finalStatus === 429 : headerStatus >= 500 || headerStatus === 429;
      results[index] = { index, eventCustomID: index + 1, status: finalStatus, ok, retryable,
        message: safeMessage(headerOK ? row.eventStatusText : message, apiKey) || (ok ? "" : retryable ? "Не получено подтверждение события; повторим позже." : "Inara отклонила событие.") };
    }
    return { ok: results.every((row) => row.ok), headerStatus, results, retryable: results.some((row) => row.retryable),
      setupRequired: !headerOK && headerStatus === 400, message: message || (headerOK ? "" : "Inara отклонила запрос. Проверьте личный API-ключ и повторите позже.") };
  } catch (error) {
    if (error instanceof InaraError) throw error;
    if (signal?.aborted) throw new InaraError("Отправка в Inara отменена.", "ABORTED", false);
    if (timedOut) throw new InaraError("Inara не ответила вовремя; повторим позже.", "TIMEOUT", true, 60000);
    // Do not propagate fetch errors: their message may include a request or credentials.
    throw new InaraError("Не удалось связаться с Inara; повторим отправку позже.", "NETWORK", true, 60000);
  } finally {
    clearTimeout(timeout); signal?.removeEventListener("abort", abort);
  }
}

module.exports = { ENDPOINT, APP_NAME, MAX_AGE_MS, InaraError, SUPPORTED_EVENTS, validateLive,
  buildSnapshotEvents, journalEvents, sendBatch, eventKey, snapshotKey };
