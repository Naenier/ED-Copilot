'use strict';

// Offline protocol verification. Fixtures are unchanged JSON Schemas downloaded
// from EDCD/EDDN's live branch. All HTTP calls below use a fake transport.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { EventEmitter } = require('node:events');
const { gunzipSync } = require('node:zlib');
const { ENDPOINT, MIN_RETRY_MS, buildJournalMessages, buildStationMessages, sendEddn } = require('../lib/eddn.cjs');

const schemaFiles = {
  'journal/1': 'journal-v1.0.json', 'commodity/3': 'commodity-v3.0.json',
  'outfitting/2': 'outfitting-v2.0.json', 'shipyard/2': 'shipyard-v2.0.json',
};
const schemas = Object.fromEntries(Object.entries(schemaFiles).map(([name, file]) => [name, JSON.parse(fs.readFileSync(path.join(__dirname, 'fixtures', 'eddn', `live-${file}`), 'utf8'))]));

// The schemas use this Draft-04 subset. Resolving $refs (including disallowed's
// `not`) matters: a simple properties-only check would miss privacy violations.
function conforms(value, spec, root, location = '$') {
  if (spec.$ref) {
    const target = spec.$ref.replace(/^#\//, '').split('/').reduce((item, key) => item[key], root);
    return conforms(value, target, root, location);
  }
  if (spec.not) {
    let valid = true;
    try { conforms(value, spec.not, root, location); } catch { valid = false; }
    assert.equal(valid, false, `${location} disallowed`);
  }
  const types = Array.isArray(spec.type) ? spec.type : spec.type ? [spec.type] : [];
  if (types.length) assert(types.some(type => type === 'null' ? value === null : type === 'array' ? Array.isArray(value) : type === 'integer' ? Number.isInteger(value) : type === 'number' ? typeof value === 'number' && Number.isFinite(value) : type === 'object' ? value !== null && typeof value === 'object' && !Array.isArray(value) : typeof value === type), `${location} type ${types}`);
  if (spec.enum) assert(spec.enum.some(candidate => JSON.stringify(candidate) === JSON.stringify(value)), `${location} enum`);
  if (typeof value === 'string') {
    if (spec.minLength !== undefined) assert(value.length >= spec.minLength, `${location} minLength`);
    if (spec.pattern) assert(new RegExp(spec.pattern).test(value), `${location} pattern`);
    if (spec.format === 'date-time') assert(Number.isFinite(Date.parse(value)), `${location} date-time`);
  }
  if (Array.isArray(value)) {
    if (spec.minItems !== undefined) assert(value.length >= spec.minItems, `${location} minItems`);
    if (spec.maxItems !== undefined) assert(value.length <= spec.maxItems, `${location} maxItems`);
    if (spec.uniqueItems) assert.equal(new Set(value.map(item => JSON.stringify(item))).size, value.length, `${location} uniqueItems`);
    if (spec.items) value.forEach((item, index) => conforms(item, spec.items, root, `${location}[${index}]`));
  } else if (value && typeof value === 'object') {
    for (const required of spec.required || []) assert(Object.hasOwn(value, required), `${location}.${required} required`);
    for (const [key, item] of Object.entries(value)) {
      let matched = false;
      if (Object.hasOwn(spec.properties || {}, key)) { conforms(item, spec.properties[key], root, `${location}.${key}`); matched = true; }
      for (const [pattern, child] of Object.entries(spec.patternProperties || {})) {
        if (new RegExp(pattern).test(key)) { conforms(item, child, root, `${location}.${key}`); matched = true; }
      }
      if (!matched && spec.additionalProperties === false) assert.fail(`${location}.${key} additionalProperties`);
    }
  }
}

let checked = 0;
function validate(envelopes) {
  for (const envelope of envelopes) {
    const name = envelope.$schemaRef.replace('https://eddn.edcd.io/schemas/', '').replace(/\/test$/, '');
    assert(schemas[name], `known schema ${name}`);
    conforms(envelope, schemas[name], schemas[name]);
    checked++;
  }
  return envelopes;
}

const now = '2026-11-07T08:00:00Z';
const context = {
  commanderName: 'Offline Test Commander', softwareVersion: '1.4.1',
  gameVersion: '4.1.0.0', gameBuild: 'r123/r0 ',
  liveGalaxy: true, systemName: 'Sol', systemAddress: 10477373803, starPos: [0, 0, 0],
  stationName: 'Galileo', marketId: 128666762, docked: true, horizons: true, odyssey: true,
};
const scan = {
  timestamp: now, event: 'Scan', SystemAddress: context.systemAddress,
  BodyName: 'Sol 1', BodyID: 1, PlanetClass: 'Rocky body', ReserveLevel: 'PristineResources',
  Rings: [{ Name: 'Sol 1 A Ring', RingClass: 'eRingClass_MetalRich', InnerRad: 100, OuterRad: 200, MassMT: 123, Secret: 'PRIVATE' }],
  Materials: [{ Name: 'Iron', Name_Localised: 'PRIVATE', Percent: 10 }],
  Commander: 'PRIVATE', FID: 'PRIVATE', Credits: 123456, Missions: ['PRIVATE'],
  Wanted: true, FuelLevel: 12, Latitude: 14, Longitude: 16, VoucherAmount: 444,
  BodyName_Localised: 'PRIVATE', IsNewEntry: true, UnknownFutureSecret: { token: 'PRIVATE' },
};
const [scanEnvelope] = validate(buildJournalMessages(scan, context));
assert.deepEqual(scanEnvelope.message.StarPos, [0, 0, 0]);
assert.equal(scanEnvelope.header.gamebuild, 'r123/r0 ');
assert.equal(JSON.stringify(scanEnvelope.message).includes('PRIVATE'), false);
assert.equal(scanEnvelope.message.FuelLevel, undefined);
assert.equal(scanEnvelope.message.Latitude, undefined);
assert.equal(scanEnvelope.message.IsNewEntry, undefined);
assert.equal(buildJournalMessages({ ...scan, SystemAddress: 999 }, context).length, 0);
assert.equal(buildJournalMessages({ timestamp: now, event: 'Scan', BodyName: 'Unknown' }, context).length, 0);
assert.equal(buildJournalMessages({ ...scan, event: 'MissionAccepted' }, context).length, 0);
assert.equal(buildJournalMessages(scan, { ...context, liveGalaxy: false }).length, 0);
assert.equal(buildJournalMessages(scan, { ...context, isBeta: true }).length, 0);
assert.equal(buildJournalMessages(scan, { ...context, commanderName: '' }).length, 0);
const [testEnvelope] = validate(buildJournalMessages(scan, { ...context, test: true, liveGalaxy: false, isBeta: true }));
assert(testEnvelope.$schemaRef.endsWith('/test'));
const [unknownFlags] = validate(buildJournalMessages(scan, { ...context, horizons: undefined, odyssey: undefined }));
assert.equal(Object.hasOwn(unknownFlags.message, 'horizons'), false);
assert.equal(Object.hasOwn(unknownFlags.message, 'odyssey'), false);

for (const event of ['Location', 'FSDJump', 'CarrierJump', 'Docked']) {
  const [observation] = validate(buildJournalMessages({
    timestamp: now, event, StarSystem: 'Achenar', SystemAddress: 123, StarPos: [67, -119, 24],
    Wanted: true, JumpDist: 123, ActiveFine: 42,
    Factions: [{ Name: 'Test Public Faction', Influence: 0.5, MyReputation: 100, HomeSystem: 34, SquadronFaction: true, Happiness_Localised: 'PRIVATE', PendingStates: [{ State: 'Boom', Trend: 1, Secret: 'PRIVATE' }] }],
  }, context));
  assert.equal(observation.message.StarSystem, 'Achenar');
  assert.equal(JSON.stringify(observation.message).includes('PRIVATE'), false);
  if (observation.message.Factions) assert.equal(observation.message.Factions[0].MyReputation, undefined);
}
validate(buildJournalMessages({ timestamp: now, event: 'SAASignalsFound', SystemAddress: context.systemAddress, BodyName: 'Sol 1', BodyID: 1, Signals: [{ Type: '$SAA_SignalType_Geological;', Type_Localised: 'PRIVATE', Count: 3 }] }, context));

const commodity = { Name: '$Platinum_name;', MeanPrice: 20000, BuyPrice: 23000, SellPrice: 22000, Stock: 10, StockBracket: 2, Demand: 30000, DemandBracket: 3, Name_Localised: 'PRIVATE', Category: '$MARKET_category_Metals;', Category_Localised: 'PRIVATE', id: 12849, Producer: true, Rare: false };
const market = { timestamp: now, event: 'Market', StarSystem: context.systemName, StationName: context.stationName, MarketID: context.marketId, StationType: 'FleetCarrier', CarrierDockingAccess: 'all', Items: [commodity] };
const [marketEnvelope] = validate(buildStationMessages(market, context));
assert.equal(marketEnvelope.message.commodities[0].name, 'platinum');
assert.equal(Object.hasOwn(marketEnvelope.message.commodities[0], 'id'), false);
assert.equal(Object.hasOwn(marketEnvelope.message, 'economies'), false);
assert.equal(JSON.stringify(marketEnvelope.message).includes('PRIVATE'), false);
assert.equal(buildStationMessages({ ...market, MarketID: 999 }, context).length, 0);
assert.equal(buildStationMessages({ ...market, StationName: 'Stale' }, context).length, 0);
assert.equal(buildStationMessages(market, { ...context, docked: false }).length, 0);
assert.equal(buildStationMessages({ ...market, Items: [{ ...commodity, MeanPrice: undefined }] }, context).length, 0);
assert.equal(validate(buildStationMessages({ ...market, Items: [] }, context))[0].message.commodities.length, 0);
assert.equal(validate(buildStationMessages({ ...market, Items: [{ ...commodity, StockBracket: '' }] }, context))[0].message.commodities[0].stockBracket, '');
const [outfitting] = validate(buildStationMessages({ ...market, event: 'Outfitting', Items: [
  { Name: 'Hpt_MiningLaser_Fixed_Small' }, { Name: 'Hpt_MiningLaser_Fixed_Small' },
  { Name: 'Int_CargoRack_Size4_Class1', sku: 'ELITE_HORIZONS_V_PLANETARY_LANDINGS' },
  { Name: 'Int_PlanetApproachSuite' }, { Name: 'PaintJob_Cobra' }, { Name: 'Hpt_SecretUnlock', sku: 'PRIVATE_POWERPLAY' },
] }, context));
assert.deepEqual(outfitting.message.modules, ['hpt_mininglaser_fixed_small', 'int_cargorack_size4_class1']);
const [shipyard] = validate(buildStationMessages({ ...market, event: 'Shipyard', Pricelist: [{ ShipType: 'Type9' }, { ShipType: 'Type9' }, { ShipType: 'Python' }] }, context));
assert.deepEqual(shipyard.message.ships, ['python', 'type9']);

const capi = {
  source: 'capi', observedAt: { market: now, shipyard: now },
  profile: { commander: { name: context.commanderName, docked: true, credits: 'PRIVATE' }, lastSystem: { name: context.systemName }, lastStarport: { id: context.marketId, name: context.stationName } },
  market: { id: context.marketId, name: context.stationName, commodities: [
    { name: 'platinum', meanPrice: 20000, buyPrice: 23000, sellPrice: 22000, stock: 10, stockBracket: 2, demand: 30000, demandBracket: 3, categoryname: 'Metals', statusFlags: ['Producer'], legality: '' },
    { name: 'drones', categoryname: 'NonMarketable' },
    { name: 'narcotics', legality: 'Illegal' },
  ], economies: { 1: { name: 'Industrial', proportion: 1, Secret: 'PRIVATE' } }, prohibited: { 1: 'Narcotics' } },
  shipyard: { id: context.marketId, name: context.stationName,
    modules: { 1: { name: 'Int_CargoRack_Size4_Class1', cost: 'PRIVATE' }, 2: { name: 'Hpt_Powerplay', sku: 'rank_locked' } },
    ships: { shipyard_list: { 1: { name: 'Python' } }, unavailable_list: [{ name: 'CobraMkIV' }] },
  },
};
const capiMessages = validate(buildStationMessages(capi, context));
assert.equal(capiMessages.length, 3);
assert.equal(capiMessages[0].message.commodities.length, 1);
assert.equal(capiMessages[0].header.gameversion, 'CAPI-Live-market');
assert.equal(capiMessages[1].header.gameversion, 'CAPI-Live-shipyard');
assert.equal(capiMessages[1].header.gamebuild, '');
assert.deepEqual(capiMessages[2].message.ships, ['cobramkiv', 'python']);
assert.equal(JSON.stringify(capiMessages).includes('PRIVATE'), false);
assert.equal(buildStationMessages({ ...capi, profile: { ...capi.profile, commander: { name: 'Wrong', docked: true } } }, context).length, 0);
assert.equal(buildStationMessages({ ...capi, profile: { ...capi.profile, commander: { name: context.commanderName, docked: false } } }, context).length, 0);
assert.equal(buildStationMessages({ ...capi, market: { ...capi.market, id: 3 }, shipyard: null }, context).length, 0);
assert.equal(buildStationMessages({ ...capi, profile: { ...capi.profile, lastSystem: { name: 'Wrong' } } }, context).length, 0);
const [closedClientMarket] = validate(buildStationMessages({ ...capi, shipyard: null }, { ...context, gameRunning: false }));
assert.equal(Object.hasOwn(closedClientMarket.message, 'horizons'), false);
const [colonyMarket] = validate(buildStationMessages({ ...capi, market: { ...capi.market, economies: [{ name: 'Colony', proportion: 1 }] }, shipyard: null }, { ...context, gameRunning: false }));
assert.equal(colonyMarket.message.horizons, true);

function fakeTransport(status, responseBody, retryAfter, failNetwork) {
  return (url, options, callback) => {
    assert.equal(url, ENDPOINT);
    assert.equal(options.method, 'POST');
    assert.equal(options.headers['Content-Encoding'], 'gzip');
    assert.match(options.headers['Content-Type'], /^application\/json/);
    const request = new EventEmitter();
    request.destroy = error => request.emit('error', error);
    request.end = body => {
      assert.equal(body.length, options.headers['Content-Length']);
      const decoded = JSON.parse(gunzipSync(body));
      assert.deepEqual(decoded, marketEnvelope);
      queueMicrotask(() => {
        if (failNetwork) return request.emit('error', new Error('Offline mock'));
        const response = new EventEmitter();
        response.setEncoding = () => {};
        response.statusCode = status;
        response.headers = { 'retry-after': retryAfter };
        callback(response);
        response.emit('data', responseBody);
        response.emit('end');
      });
    };
    return request;
  };
}

(async () => {
  assert.equal((await sendEddn(marketEnvelope, { transport: fakeTransport(200, 'OK') })).ok, true);
  for (const status of [400, 413, 426]) {
    const result = await sendEddn(marketEnvelope, { transport: fakeTransport(status, 'Rejected') });
    assert.equal(result.retryable, false);
    assert.equal(result.retryAfterMs, 0);
  }
  for (const status of [429, 503]) {
    const result = await sendEddn(marketEnvelope, { transport: fakeTransport(status, 'Busy', '120') });
    assert.equal(result.retryable, true);
    assert.equal(result.retryAfterMs, 120000);
  }
  const offline = await sendEddn(marketEnvelope, { transport: fakeTransport(0, '', '', true) });
  assert.equal(offline.retryable, true);
  assert.equal(offline.retryAfterMs, MIN_RETRY_MS);
  const rejected = await sendEddn({ $schemaRef: 'https://untrusted.invalid/schema' }, { transport: () => assert.fail('Invalid schema must not be sent') });
  assert.equal(rejected.retryable, false);
  const beforeStart = new AbortController();
  beforeStart.abort();
  const preCancelled = await sendEddn(marketEnvelope, { signal: beforeStart.signal, transport: () => assert.fail('Cancelled request must not start') });
  assert.equal(preCancelled.retryable, false);
  assert.equal(preCancelled.error, 'Отменено');
  const inFlight = new AbortController();
  let destroyed = false;
  const cancelled = sendEddn(marketEnvelope, { signal: inFlight.signal, transport: () => {
    const request = new EventEmitter();
    request.end = () => queueMicrotask(() => inFlight.abort());
    request.destroy = error => { destroyed = true; request.emit('error', error); };
    return request;
  } });
  assert.equal((await cancelled).error, 'Отменено');
  assert.equal(destroyed, true);
  console.log(`EDDN offline checks passed: ${checked} messages conform to four official live schemas; gzip, privacy, CAPI lag and retry policies verified. No journal data uploaded.`);
})().catch(error => { console.error(error); process.exitCode = 1; });

