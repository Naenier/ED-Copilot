"use strict";
const assert = require("node:assert/strict");
const { ENDPOINT, APP_NAME, InaraError, validateLive, buildSnapshotEvents, journalEvents, sendBatch, eventKey, snapshotKey } = require("../lib/inara.cjs");
const now = Date.parse("2026-11-07T12:00:00Z"), at = "2026-11-07T11:59:00Z";
const context = { now, gameVersion: "4.1.0.1234", commander: { system: "Test System", station: "Test Port", marketId: 1234 }, shipType: "anaconda", shipId: 7 };
const event = (type, data = {}) => ({ event: type, timestamp: at, ...data });
const mapped = (type, data = {}, extra = {}) => journalEvents(event(type, data), { ...context, ...extra });
const reply = (data, status = 200, headers = {}) => ({ ok: status >= 200 && status < 300, status,
  headers: new Headers(headers), text: async () => JSON.stringify(data) });
const args = { commanderName: "Synthetic Test Commander", frontierId: "F000000", apiKey: "synthetic-inara-key-never-real", now, appVersion: "1.4.1" };
const rejects = (promise, code, retryable) => assert.rejects(promise,
  (e) => e instanceof InaraError && e.code === code && (retryable === undefined || e.retryable === retryable));

async function main() {
  assert.equal(validateLive({ gameVersion: "4.0.0.1234" }).ok, true);
  assert.equal(validateLive({ gameVersion: "3.8.0", live: true }).ok, false);
  assert.equal(validateLive({ gameVersion: "4.0.0 Beta", live: true }).ok, false);
  assert.equal(validateLive({ live: true, multicrew: true }).ok, false);
  assert.equal(validateLive({}).ok, false);
  assert.deepEqual(journalEvents(event("LoadGame", { Credits: 10 })), [], "Unknown galaxy must not upload");
  assert.deepEqual(mapped("LoadGame", { Credits: 10 }, { replay: true }), [], "Replay must not upload");
  assert.deepEqual(mapped("LoadGame", { Credits: 10 }, { historical: true }), []);
  assert.deepEqual(mapped("LoadGame", { Credits: 10, timestamp: "2025-01-01T00:00:00Z" }), []);
  assert.deepEqual(mapped("LoadGame", { Credits: 10, timestamp: "2027-01-01T00:00:00Z" }), []);
  assert.deepEqual(mapped("LoadGame", { Credits: 0, Loan: 0 })[0].eventData, { commanderCredits: 0, commanderLoan: 0 });
  assert.deepEqual(mapped("MarketSell", { TotalSale: 123456 }), [], "Never derive authoritative wallet by journal arithmetic");

  const cargo = mapped("Cargo", { Vessel: "Ship", Inventory: [{ Name: "platinum", Count: 8 }, { Name: "drones", Count: 12 }] });
  assert.equal(cargo[0].eventName, "setCommanderInventoryCargo");
  assert.deepEqual(cargo[0].eventData.map((x) => x.itemName), ["drones", "platinum"]);
  assert.deepEqual(mapped("Cargo", { Vessel: "SRV", Inventory: [{ Name: "gold", Count: 2 }] }), []);
  assert.deepEqual(mapped("Cargo", { Vessel: "Ship" }), [], "Side-file notification is not empty inventory");
  assert.deepEqual(mapped("Cargo", { Vessel: "Ship", Inventory: [] })[0].eventData, [], "A known empty cargo clears Inara");
  assert.deepEqual(mapped("Cargo", { Vessel: "Ship", Inventory: [{ Name: "gold", Count: 10, Stolen: 4, MissionID: 22 }] })[0].eventData,
    [{ itemName: "gold", itemCount: 4, missionGameID: 22, isStolen: true }, { itemName: "gold", itemCount: 6, missionGameID: 22, isStolen: false }]);
  assert.deepEqual(mapped("Materials", { Raw: [{ Name: "iron", Count: 20 }] }), [], "Partial inventory must not erase unknown categories");
  assert.equal(mapped("Materials", { Raw: [{ Name: "iron", Count: 20 }], Manufactured: [], Encoded: [] })[0].eventData[0].itemName, "iron");
  const lockers = mapped("ShipLocker", { Items: [{ Name: "suitschematic", Count: 2 }], Components: [], Data: [], Consumables: [] });
  assert.deepEqual(lockers[0].eventData.map((x) => x.itemType), ["Components", "Data", "Consumables"]);
  assert.equal(lockers[1].eventData[0].itemType, "Items");
  assert.ok(!lockers[0].eventData.some((x) => x.itemType === "Items"), "A late retry of reset must not clear a successfully set nonempty group");

  const loadout = mapped("Loadout", { Ship: "anaconda", ShipID: 7, ShipName: "Test", ShipIdent: "T-001", CargoCapacity: 0,
    Modules: [{ Slot: "LargeHardpoint1", Item: "hpt_beamlaser_fixed_large", Health: 1, On: false, Priority: 0, AmmoInClip: 0,
      Engineering: { BlueprintName: "Weapon_LongRange", Level: 5, Quality: 0.7, ExperimentalEffect: "special_thermalvent",
        Modifiers: [{ Label: "PowerDraw", Value: 1.2, OriginalValue: 1, LessIsGood: true }, { Label: "DamageType", ValueStr: "Thermal" }] } }] });
  assert.deepEqual(loadout.map((x) => x.eventName), ["setCommanderShip", "setCommanderShipLoadout"]);
  assert.equal(loadout[0].eventData.shipCargoCapacity, 0);
  const module = loadout[1].eventData.shipLoadout[0];
  assert.equal(module.isOn, false); assert.equal(module.itemPriority, 0);
  assert.deepEqual(module.engineering.modifiers[1], { name: "DamageType", value: "Thermal" });
  assert.equal(mapped("Loadout", { Ship: "testbuggy", ShipID: 2, Modules: [] }).length, 0);
  assert.equal(mapped("Loadout", { Ship: "federation_fighter", ShipID: 2, Modules: [] }).length, 0);
  assert.equal(mapped("Loadout", { Ship: "anaconda", ShipID: 7, Modules: [{ Slot: "Bad" }] }).length, 1, "Malformed loadout must not erase modules");
  assert.deepEqual(mapped("ShipyardNew", { ShipType: "python", NewShipID: 9 })[0].eventData, { shipType: "python", shipGameID: 9 });
  assert.deepEqual(mapped("ShipyardSwap", { ShipType: "python", ShipID: 9, SellOldShip: "anaconda", SellShipID: 7 }).map((x) => x.eventName), ["delCommanderShip", "setCommanderShip"]);
  const stats = mapped("Statistics", { Bank_Account: { Current_Wealth: 123 }, Mining: { Quantity_Mined: 5 }, Exploration: { Systems_Visited: 3 } });
  assert.deepEqual(Object.keys(stats[0].eventData), ["Bank_Account", "Mining", "Exploration"]);
  assert.equal(mapped("Rank", { Explore: 8, Trade: 10 })[0].eventData.find((x) => x.rankName === "explore").rankValue, 8);
  assert.equal(mapped("Progress", { Explore: 42 })[0].eventData[0].rankProgress, 0.42);
  assert.equal(mapped("PowerplayLeave", { Power: "Test Power" })[0].eventData.rankValue, -1);
  const engineers = mapped("EngineerProgress", { Engineers: [{ Engineer: "Felicity Farseer", Progress: "Unlocked", Rank: 5 }, { Engineer: "Test", Progress: "Known" }] });
  assert.equal(engineers[0].eventData.length, 1);

  assert.equal(mapped("Location", { StarSystem: "Sol", Docked: true, StationName: "Test" }).at(-1).eventName, "setCommanderTravelLocation");
  assert.equal(mapped("Docked", { StarSystem: "Sol", StationName: "Test" }, { suppressDock: true })[0].eventName, "setCommanderTravelLocation");
  const travel = mapped("FSDJump", { StarSystem: "Sol", StarPos: [0, 0, 0], JumpDist: 14, Taxi: true })[0];
  assert.equal(travel.eventData.isTaxiShuttle, true); assert.equal(travel.eventData.shipType, undefined);
  const carrierJump = mapped("CarrierJump", { StarSystem: "Sol", StationName: "ABC-123", MarketID: 1234, JumpDist: 500 })[0];
  assert.equal(carrierJump.eventName, "addCommanderTravelCarrierJump");
  assert.equal(carrierJump.eventData.jumpDistance, undefined, "API says not to set carrier jumpDistance");
  assert.deepEqual(mapped("CarrierStats", { CarrierID: 5, FuelLevel: 1000, Finance: { CarrierBalance: 100000 } }), [], "No invented carrier-management API event");
  assert.deepEqual(mapped("Touchdown", { PlayerControlled: false, Body: "Sol A1" }), []);
  assert.equal(mapped("Touchdown", { PlayerControlled: true, Body: "Sol A1", Latitude: 3, Longitude: -4 })[0].eventName, "addCommanderTravelLand");
  const mission = mapped("MissionAccepted", { MissionID: 42, Name: "Mission_Delivery", Commodity: "gold", Count: 10, Expiry: "2026-11-08T10:00:00Z" });
  assert.equal(mission[0].eventData.starsystemNameOrigin, "Test System");
  assert.equal(mission.length, 1, "Mission metadata must not add mission cargo a second time");
  const completed = mapped("MissionCompleted", { MissionID: 42, Reward: 20000, MaterialsReward: [{ Name: "iron", Count: 2 }],
    FactionEffects: [{ Faction: "Test Faction", Influence: [{ Influence: "+" }, { Influence: "+++" }], Reputation: "++" }] });
  assert.equal(completed[0].eventData.minorfactionEffects[0].influenceGain, "+++");
  assert.equal(completed[0].eventData.rewardMaterials[0].itemCount, 2);

  const state = { commander: { vehicle: "ship", system: "Sol", cargo: [{ key: "platinum", count: 10 }],
    cargoForSync: [{ Name: "platinum", Count: 10 }], cargoForSyncAt: at },
    pilot: { credits: 2000, creditsAt: at }, engineering: { shopping: { knownCategories: { raw: true, manufactured: true, encoded: true },
      updatedAt: { raw: at, manufactured: at, encoded: at } }, materials: [{ symbol: "Iron", category: "raw", owned: 0 }],
      engineers: [{ english: "Felicity Farseer", status: "unlocked", manualUnlocked: true, journalStatus: "Invited", updatedAt: at, rank: null }] } };
  const snapshot = buildSnapshotEvents(state, { ...context, cargoUpdatedAt: at, locationUpdatedAt: at });
  assert.equal(snapshot.find((x) => x.eventName === "setCommanderRankEngineer").eventData.rankStage, "Invited");
  assert.equal(snapshot.find((x) => x.eventName === "setCommanderInventoryMaterials").eventData[0].itemCount, 0);
  state.engineering.shopping.knownCategories.encoded = false;
  assert.ok(!buildSnapshotEvents(state, context).some((x) => x.eventName === "setCommanderInventoryMaterials"));
  delete state.commander.cargoForSync;
  assert.ok(!buildSnapshotEvents(state, context).some((x) => x.eventName === "setCommanderInventoryCargo"), "Summary cargo must not erase flags and mission partitions");
  state.commander.vehicle = "srv";
  assert.ok(!buildSnapshotEvents(state, { ...context, cargoUpdatedAt: at }).some((x) => x.eventName === "setCommanderInventoryCargo"));
  const identity = { commanderName: "Test Commander", frontierId: "F000001" };
  assert.equal(eventKey(cargo[0], identity), eventKey(JSON.parse(JSON.stringify(cargo[0])), identity));
  assert.notEqual(eventKey(cargo[0], identity), eventKey(cargo[0], { frontierId: "F000002" }));
  assert.equal(snapshotKey(cargo[0]), "setCommanderInventoryCargo");
  assert.equal(snapshotKey(mission[0]), null);

  const events = [cargo[0], loadout[0], mission[0], completed[0]];
  let request;
  const sent = await sendBatch({ ...args, events, fetchImpl: async (url, options) => {
    request = { url, options, body: JSON.parse(options.body) };
    return reply({ header: { eventStatus: 200 }, events: [200, 202, 204, 400].map((eventStatus, i) => ({ eventStatus, eventCustomID: i + 1 })) });
  } });
  assert.equal(request.url, ENDPOINT); assert.equal(request.body.header.appName, APP_NAME);
  assert.equal(request.body.header.APIkey, args.apiKey); assert.equal(request.body.header.isBeingDeveloped, true);
  assert.equal(request.options.redirect, "error");
  assert.deepEqual(sent.results.map((x) => [x.ok, x.retryable]), [[true, false], [true, false], [false, false], [false, false]]);
  assert.ok(!JSON.stringify(sent).includes(args.apiKey));
  const missing = await sendBatch({ ...args, events: [cargo[0]], fetchImpl: async () => reply({ header: { eventStatus: 200 }, events: [] }) });
  assert.equal(missing.results[0].retryable, true, "No event acknowledgement is not success");
  const mismatch = await sendBatch({ ...args, events: [cargo[0]], fetchImpl: async () => reply({ header: { eventStatus: 200 }, events: [{ eventStatus: 200, eventCustomID: 333 }] }) });
  assert.equal(mismatch.results[0].ok, false);
  const notWhitelisted = await sendBatch({ ...args, events: [cargo[0]], fetchImpl: async () => reply({ header: { eventStatus: 400, eventStatusText: "Invalid application or " + args.apiKey }, events: [{ eventStatus: 200 }] }) });
  assert.equal(notWhitelisted.setupRequired, true); assert.equal(notWhitelisted.results[0].ok, false);
  assert.ok(!JSON.stringify(notWhitelisted).includes(args.apiKey));
  const filtered = await sendBatch({ ...args, events: [{ ...cargo[0], eventTimestamp: "2020-01-01T00:00:00Z" }, cargo[0]], fetchImpl: async (_url, options) => {
    assert.equal(JSON.parse(options.body).events.length, 1);
    assert.equal(JSON.parse(options.body).events[0].eventCustomID, 2);
    return reply({ header: { eventStatus: 200 }, events: [{ eventStatus: 200, eventCustomID: 2 }] });
  } });
  assert.equal(filtered.results[0].status, 410); assert.equal(filtered.results[0].retryable, false); assert.equal(filtered.results[1].ok, true);
  await rejects(sendBatch({ ...args, events, fetchImpl: async () => reply({}, 500, { "retry-after": "90" }) }), "HTTP_500", true);
  await rejects(sendBatch({ ...args, events, fetchImpl: async () => reply({}, 403) }), "HTTP_403", false);
  await rejects(sendBatch({ ...args, events, fetchImpl: async () => ({ ok: true, headers: new Headers(), text: async () => "not JSON" }) }), "RESPONSE_INVALID", true);
  await rejects(sendBatch({ ...args, events, fetchImpl: async () => reply({}, 200, { "content-length": String(3 * 1024 * 1024) }) }), "RESPONSE_SIZE", true);
  let safeError;
  try { await sendBatch({ ...args, events, fetchImpl: async () => { throw new Error("Request failed: " + args.apiKey); } }); } catch (e) { safeError = e; }
  assert.equal(safeError.code, "NETWORK"); assert.ok(!safeError.message.includes(args.apiKey));
  const controller = new AbortController(); controller.abort();
  await rejects(sendBatch({ ...args, events, signal: controller.signal, fetchImpl: async () => { assert.fail("Cancelled request must not start"); } }), "ABORTED", false);
  const waitingFetch = async (_url, options) => new Promise((_resolve, reject) => options.signal.addEventListener("abort", () => reject(new Error("abort")), { once: true }));
  const keepAlive = setInterval(() => {}, 500);
  try { await rejects(sendBatch({ ...args, events, timeoutMs: 100, fetchImpl: waitingFetch }), "TIMEOUT", true); }
  finally { clearInterval(keepAlive); }
  await rejects(sendBatch({ ...args, apiKey: "", events }), "KEY_REQUIRED", false);
  console.log("Inara mapping and transport tests passed (synthetic data, mocked HTTP only).");
}
main().catch((error) => { console.error(error); process.exitCode = 1; });

