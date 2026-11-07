'use strict';
const assert = require('node:assert/strict');
const path = require('node:path');
const { ColonizationPlanner, classifyCandidate, availableCargo, RULES } = require(path.join(__dirname, '../lib/colonization.cjs'));
const { TradeLedger } = require('../lib/trade.cjs');
const trade = new TradeLedger({russianNames:require('../lib/commodity-ru.json')});
const ship = {commander:{vehicle:'ship'}};
trade.apply({event:'Cargo',Vessel:'Ship',Inventory:[{Name:'aluminium',Count:10}]},ship);
trade.apply({event:'ColonisationContribution',Contributions:[{Name:'$aluminium_name;',Amount:4}]},ship);
assert.equal(trade.snapshot(ship.commander).inventory.find(row=>row.key==='aluminium').count,6,'Construction supplies leave the cargo inventory.');
assert.equal(trade.good('aluminium').name,'Алюминий');
const now = () => Date.parse('2026-11-07T12:00:00Z');
const planner = new ColonizationPlanner({}, { now });
function selected() { return planner.snapshot().activeProject; }
function add(typeId, rest = {}) {
  planner.update({ action: 'add-building', projectId: selected().id, typeId, ...rest });
  return selected().buildings.at(-1);
}
function journal(event, offset) {
  planner.apply({ timestamp: '2026-11-07T10:00:00Z', ...event }, { file: 'Journal.synthetic.log', offset,
    commander: { system: 'Synthetic Target', systemAddress: 202, station: 'Synthetic Depot', vehicle: 'ship' } });
}

async function main() {
  assert.equal(RULES.rangeLy, 15);
  const origin = { x: 0, y: 0, z: 0 };
  const candidate = { id64: 202, name: 'Synthetic Target', x: 3, y: 4, z: 0, population: 0,
    needs_permit: false, is_colonised: false, is_being_colonised: false, stations: [] };
  assert.equal(classifyCandidate(candidate, origin, '').availability, 'candidate');
  assert.equal(classifyCandidate({ ...candidate, population: 1 }, origin, '').availability, 'unavailable');
  assert.equal(classifyCandidate({ ...candidate, stations: [{ type: 'Asteroid base' }] }, origin, '').availability, 'unavailable');
  assert.equal(classifyCandidate({ ...candidate, stations: [{ type: 'Fleet Carrier' }] }, origin, '').availability, 'candidate');
  assert.equal(classifyCandidate({ ...candidate, is_being_colonised: undefined }, origin, '').availability, 'unknown');
  assert.equal(classifyCandidate({ ...candidate, x: 16 }, origin, '').availability, 'unavailable');

  planner.update({ action: 'configure', originSystem: 'Synthetic Origin', faction: 'Station Faction', cargoCapacity: 500 });
  planner.update({ action: 'create-project', targetSystem: 'Synthetic Target', systemAddress: 202 });
  const primary = add('coriolis', { isPrimary: true });
  planner.update({ action: 'set-resource-plan', projectId: selected().id, buildingId: primary.id,
    resources: [{ commodity: '$steel_name;', required: 100 }, { commodity: 'steel', required: 10 }], sourceNote: 'Synthetic estimate' });
  assert.equal(selected().buildings[0].requirements[0].required, 110);
  assert.throws(() => planner.update({ action: 'set-resource-plan', projectId: selected().id, buildingId: primary.id,
    resources: [{ commodity: 'steel', required: 100 }] }), /источник/i);
  planner.update({ action: 'record-delivery', projectId: selected().id, buildingId: primary.id, commodity: 'steel', count: 20 });
  assert.equal(selected().buildings[0].requirements[0].missing, 90);
  assert.equal(selected().buildings[0].requirements[0].authoritative, false);
  assert.throws(() => planner.update({ action: 'record-delivery', projectId: selected().id, buildingId: primary.id, commodity: 'steel', count: 1.5 }), /целое/);
  assert.equal(selected().buildings[0].pointCost, 0);
  assert(!selected().buildings[0].warnings.some(x => /Не хватает/.test(x)));

  const depot = { event: 'ColonisationConstructionDepot', MarketID: 900001, ConstructionProgress: 0.4,
    ConstructionComplete: false, ConstructionFailed: false,
    ResourcesRequired: [{ Name: '$steel_name;', Name_Localised: 'Сталь', RequiredAmount: 100, ProvidedAmount: 40, Payment: 5 }] };
  journal(depot, 100);
  planner.update({ action: 'link-depot', projectId: selected().id, buildingId: primary.id, marketId: 900001 });
  let building = selected().buildings[0];
  assert.equal(building.requirements[0].required, 100, 'journal replaces estimate');
  assert.equal(building.requirements[0].delivered, 40, 'manual mark never doubles authoritative total');
  assert.equal(building.requirements[0].manualDelivered, 20);
  assert.equal(building.progress, 0.4);
  assert.equal(building.authoritative, true);
  const contribution = { event: 'ColonisationContribution', MarketID: 900001, Contributions: [{ Name: '$steel_name;', Amount: 10 }] };
  journal(contribution, 200); journal(contribution, 200); journal(contribution, 300);
  building = selected().buildings[0];
  assert.equal(building.requirements[0].delivered, 60, 'same timestamp + distinct offsets both count');
  assert.equal(building.requirements[0].pendingOwn, 20);
  assert.equal(selected().journalDeliveries.length, 2, 'same source cursor is idempotent');
  journal({ ...depot, ResourcesRequired: [{ ...depot.ResourcesRequired[0], ProvidedAmount: 70 }] }, 400);
  assert.equal(selected().buildings[0].requirements[0].delivered, 70, 'new total incorporates all commanders without re-adding own ledger');
  assert.equal(selected().buildings[0].ownDelivered, 20, 'personal total distinct from all-commanders total');
  assert.equal(selected().totalMissing, 30);
  const saved = planner.exportSettings();
  assert.equal(saved.projects[0].deliveries.length, 1);
  assert(!('depots' in saved));
  planner.reset();
  journal(depot, 100); journal(contribution, 200); journal(contribution, 300);
  assert.equal(selected().buildings[0].requirements[0].delivered, 60, 'rescan rebuilds once');
  assert.equal(selected().deliveries.length, 1, 'manual plan survives replay');

  const cargoContext = { vehicle: 'ship', cargoForSync: [
    { Name: 'steel', Count: 40, Stolen: 10 },
    { Name: 'steel', Count: 100, Stolen: 0, MissionID: 123 },
    { Name: 'titanium', Count: 4, Stolen: 0 },
  ] };
  assert.equal(availableCargo(cargoContext).get('steel'), 30);
  assert.equal(availableCargo({ vehicle: 'ship', cargo: [{ key: 'steel', count: 200 }] }), null);
  assert.equal(availableCargo({ ...cargoContext, vehicle: 'srv' }), null);
  let state = planner.snapshot({ commander: cargoContext, carrier: { owned: true, warehouse: { complete: false, items: [{ key: 'steel', count: 3 }] } } });
  assert.equal(state.aggregateSupplies[0].cargoAvailable, 30);
  assert.equal(state.aggregateSupplies[0].carrierAvailable, 3);
  assert.equal(state.aggregateSupplies[0].coveredByCargo, 30);
  const second = add('hub-refinery');
  planner.update({ action: 'set-resource-plan', projectId: selected().id, buildingId: second.id,
    resources: [{ commodity: 'steel', required: 100 }, { commodity: 'titanium', required: 20 }], sourceNote: 'Synthetic estimate' });
  state = planner.snapshot({ commander: cargoContext, carrier: { owned: true, warehouse: { complete: false, items: [{ key: 'steel', count: 3 }] } } });
  assert.equal(state.aggregateSupplies.find(x => x.commodity === 'steel').coveredByCargo, 30, 'single cargo allocation across all buildings');
  assert.equal(state.aggregateSupplies.find(x => x.commodity === 'titanium').carrierAvailable, null, 'partial inventory never implies absent item=zero');
  assert.equal(state.aggregateSupplies.find(x => x.commodity === 'titanium').toBuy, null);
  assert.equal(planner.snapshot().aggregateSupplies[0].cargoAvailable, null);
  planner.update({ action: 'update-building', projectId: selected().id, id: second.id, dependsOn: [primary.id] });
  assert.throws(() => planner.update({ action: 'update-building', projectId: selected().id, id: primary.id, dependsOn: [second.id] }), /цикл/);
  assert.deepEqual(selected().buildings[0].dependsOn, [], 'failed update rolls back');
  planner.update({ action: 'reorder-buildings', projectId: selected().id, ids: [second.id, primary.id] });
  assert(!selected().buildings[0].warnings.some(x => /позже/.test(x)), 'already-started prerequisite can finish before the planned stage');
  assert.throws(() => planner.update({ action: 'reorder-buildings', projectId: selected().id, ids: [second.id, second.id] }), /ровно один/);
  assert.throws(() => planner.update({ action: 'link-depot', projectId: selected().id, buildingId: second.id, marketId: 900001 }), /уже связан/);

  journal({ event: 'ColonisationSystemClaim', StarSystem: 'Synthetic Target', SystemAddress: 202 }, 500);
  journal({ event: 'ColonisationBeaconDeployed', timestamp: '2026-11-07T10:30:00Z' }, 600);
  assert.equal(selected().claimedAt, '2026-11-07T10:00:00.000Z');
  assert.equal(selected().beaconDeadline, '2026-11-08T10:00:00.000Z');
  assert.equal(selected().deadline, '2026-12-05T10:00:00.000Z');
  assert.equal(selected().status, 'building');
  journal({ ...depot, ConstructionComplete: true, ConstructionProgress: 1 }, 700);
  assert.equal(selected().buildings.find(x => x.id === primary.id).status, 'complete');
  assert(!selected().supplies.some(x => x.buildings.some(b => b.id === primary.id)), 'completed depot excluded from shopping list');

  const pointsPlanner = new ColonizationPlanner({}, { now });
  pointsPlanner.update({ action: 'create-project', targetSystem: 'Synthetic Points' });
  const pointId = pointsPlanner.snapshot().activeProject.id;
  pointsPlanner.update({ action: 'update-project', id: pointId, points: { tier2: 100, tier3: 100 } });
  for (const typeId of ['orbis', 'ocellus', 'coriolis', 'coriolis']) pointsPlanner.update({ action: 'add-building', projectId: pointId, typeId });
  assert.deepEqual(pointsPlanner.snapshot().activeProject.buildings.map(x => x.pointCost), [6, 6, 5, 7], 'shared port counter, first two same baseline');
  assert.deepEqual(pointsPlanner.snapshot().activeProject.pointsProjected, { tier2: 88, tier3: 90 });
  const pointBuildings = pointsPlanner.snapshot().activeProject.buildings;
  pointsPlanner.update({ action: 'update-building', projectId: pointId, id: pointBuildings[0].id, status: 'complete' });
  assert.equal(pointsPlanner.snapshot().activeProject.buildings[0].pointsAfter.tier3, 100, 'current balance already includes completed facilities');
  pointsPlanner.update({ action: 'update-building', projectId: pointId, id: pointBuildings[1].id, status: 'building' });
  assert.equal(pointsPlanner.snapshot().activeProject.buildings[1].pointsAfter.tier3, 100, 'in-progress port point cost already paid');

  const dependencies = new ColonizationPlanner({}, { now });
  dependencies.update({ action: 'create-project', targetSystem: 'Synthetic Dependencies' });
  const dependencyProject = dependencies.snapshot().activeProject.id;
  dependencies.update({ action: 'add-building', projectId: dependencyProject, typeId: 'hub-refinery' });
  const dependencyBuilding = dependencies.snapshot().activeProject.buildings[0].id;
  dependencies.update({ action: 'add-building', projectId: dependencyProject, typeId: 'orbis', dependsOn: [dependencyBuilding] });
  const finalBuilding = dependencies.snapshot().activeProject.buildings[1].id;
  dependencies.update({ action: 'reorder-buildings', projectId: dependencyProject, ids: [finalBuilding, dependencyBuilding] });
  assert(dependencies.snapshot().activeProject.buildings[0].warnings.some(x => /позже/.test(x)));

  const calls = [];
  const knownOrigin = { id64: 101, name: 'Synthetic Origin', x: 0, y: 0, z: 0, population: 1,
    controlling_minor_faction: 'System Faction', minor_faction_presences: [{ name: 'System Faction', influence: 0.7 }, { name: 'Station Faction', influence: 0.3 }],
    stations: [
      { market_id: 111, name: 'Alpha', type: 'Orbis Starport', controlling_minor_faction: 'System Faction', services: ['System Colonisation'] },
      { market_id: 112, name: 'Beta', type: 'Coriolis Starport', controlling_minor_faction: 'Station Faction', services: ['System Colonisation'] },
    ] };
  const mock = new ColonizationPlanner({}, { now, fetchImpl: async (url, options) => {
    calls.push({ url, options });
    let data;
    if (url.includes('/field_values/')) data = { min_max: [{ id64: 777, name: 'Synthetic Origin Other' }, { id64: 101, name: 'Synthetic Origin' }] };
    else if (url.endsWith('/system/101')) data = { record: knownOrigin };
    else if (url.endsWith('/systems/search')) {
      const body = JSON.parse(options.body);
      assert.deepEqual(body.filters, { distance: { min: 0, max: 15 } }, 'missing public flags are retained for local unknown classification');
      data = body.page === 0 ? { count: 3, results: [knownOrigin, candidate] }
        : { count: 3, results: [{ ...candidate, id64: 203, name: 'Unknown Claim', is_being_colonised: undefined }] };
    } else throw new Error(`Unexpected mock URL ${url}`);
    return { ok: true, status: 200, json: async () => data };
  } });
  const started = await mock.startSearch({ originSystem: 'Synthetic Origin', faction: 'Station Faction' });
  assert.equal(started.running, true);
  await mock.searchPromise;
  const searched = mock.snapshot().search;
  assert.equal(searched.error, ''); assert.equal(searched.running, false);
  assert.equal(searched.completeness.complete, true);
  assert.equal(searched.completeness.pages, 2);
  assert.equal(searched.results.length, 2);
  assert.equal(searched.results[1].availability, 'unknown');
  assert.equal(searched.claimStations.length, 1);
  assert.equal(searched.claimStations[0].faction, 'Station Faction', 'station faction is not silently replaced by system controller');
  assert.equal(searched.claimStations[0].name, 'Beta');
  assert.equal(searched.factions.length, 2);
  assert(calls.some(c => c.url.endsWith('/system/101')));
  mock.update({ action: 'create-project', candidateId: searched.results[0].id });
  assert.equal(mock.snapshot().activeProject.faction, 'Station Faction');
  assert.equal(mock.snapshot().activeProject.targetSystem, 'Synthetic Target');
  assert.equal(mock.snapshot().config.originSystem, 'Synthetic Origin');
  assert.equal(mock.snapshot().search.originFactions.length, 2);

  const failing = new ColonizationPlanner({}, { fetchImpl: async () => ({ ok: false, status: 503 }) });
  await failing.startSearch({ originSystem: 'Synthetic Origin' }); await failing.searchPromise;
  assert.equal(failing.snapshot().search.status, 'error');
  assert.equal(failing.snapshot().search.completeness.complete, false);
  assert.match(failing.snapshot().search.error, /503/);
  await assert.rejects(() => failing.startSearch({ originSystem: '' }), /происхождения/);

  const repeated = new ColonizationPlanner({}, { now, fetchImpl: async url => ({ ok: true, json: async () =>
    url.includes('/field_values/') ? { min_max: [{ id64: 101, name: knownOrigin.name }] }
      : url.endsWith('/system/101') ? { record: knownOrigin } : { count: 3, results: [knownOrigin, candidate] } }) });
  await repeated.startSearch({ originSystem: knownOrigin.name }); await repeated.searchPromise;
  assert.equal(repeated.snapshot().search.completeness.complete, false);
  assert.match(repeated.snapshot().search.error, /повторяющиеся/);
  let resolveCancelled;
  const cancelled = new ColonizationPlanner({}, { fetchImpl: () => new Promise(resolve => { resolveCancelled = resolve; }) });
  await cancelled.startSearch({ originSystem: knownOrigin.name });
  cancelled.cancel();
  resolveCancelled({ ok: true, json: async () => ({ min_max: [] }) });
  await cancelled.searchPromise;
  assert.equal(cancelled.snapshot().search.status, 'cancelled');
  assert.equal(cancelled.snapshot().search.running, false);
  assert.equal(cancelled.snapshot().search.error, '');

  const restored = new ColonizationPlanner(saved, { now });
  assert.equal(restored.snapshot().activeProject.deliveries.length, 1);
  assert.equal(restored.snapshot().depots.length, 0);
  assert.equal(restored.snapshot().activeProject.buildings[0].billSource, 'manual');
  journal({ event: 'LoadGame', FID: 'Synthetic-A' }, 800);
  journal({ event: 'LoadGame', FID: 'Synthetic-B' }, 900);
  assert.equal(planner.snapshot().depots.length, 0, 'different commander observed data cleared');
  console.log('PASS colonization: candidates, station factions, complete pagination, exact journal dedupe/replay, authoritative bills, cargo legality, dependencies, points, persistence.');
}
main().catch(error => { console.error(error); process.exitCode = 1; });
