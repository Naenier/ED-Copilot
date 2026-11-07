'use strict';

// Protocol implementation based on EDCD/EDDN's live schemas and Developers.md.
// Only public observations belong on EDDN. Never serialize an unfiltered journal
// event or CAPI response: those also contain credits, missions and private cargo.
const https = require('node:https');
const { gzipSync } = require('node:zlib');

const ENDPOINT = 'https://eddn.edcd.io:4430/upload/';
const MIN_RETRY_MS = 60_000;
const MAX_MESSAGE_BYTES = 2 * 1024 * 1024;
const SCHEMAS = Object.freeze({ journal: 'journal/1', commodity: 'commodity/3', outfitting: 'outfitting/2', shipyard: 'shipyard/2' });
const HORIZONS_SKU = 'ELITE_HORIZONS_V_PLANETARY_LANDINGS';
const string = value => typeof value === 'string' && value.length > 0 && value.length <= 8192 ? value : undefined;
const number = value => typeof value === 'number' && Number.isFinite(value) ? value : undefined;
const integer = value => Number.isSafeInteger(value) && value >= 0 ? value : undefined;
const boolean = value => typeof value === 'boolean' ? value : undefined;
const timestamp = value => typeof value === 'string' && /^\d{4}-\d\d-\d\dT/.test(value) && Number.isFinite(Date.parse(value)) ? value : undefined;
const array = spec => value => Array.isArray(value) ? value.map(spec).filter(value => value !== undefined).slice(0, 10000) : undefined;
const object = spec => value => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const result = {};
  for (const [key, parse] of Object.entries(spec)) {
    const parsed = parse(value[key]);
    if (parsed !== undefined) result[key] = parsed;
  }
  return Object.keys(result).length ? result : undefined;
};
const namedState = object({ State: string, Trend: number });
const faction = object({
  Name: string, FactionState: string, Government: string, Influence: number,
  Allegiance: string, Happiness: string, ActiveStates: array(namedState),
  PendingStates: array(namedState), RecoveringStates: array(namedState),
});
const systemFields = {
  Body: string, BodyID: integer, BodyType: string, SystemAllegiance: string,
  SystemEconomy: string, SystemSecondEconomy: string, SystemGovernment: string,
  SystemSecurity: string, Population: integer, Factions: array(faction),
  SystemFaction: object({ Name: string, FactionState: string }),
  Powers: array(string), PowerplayState: string, ControllingPower: string,
  PowerplayStateControlProgress: number, PowerplayStateReinforcement: number,
  PowerplayStateUndermining: number,
  PowerplayConflictProgress: array(object({ Power: string, ConflictProgress: number })),
};
const stationFields = {
  StationName: string, StationType: string, MarketID: integer,
  StationFaction: object({ Name: string, FactionState: string }),
  StationGovernment: string, StationAllegiance: string, StationEconomy: string,
  StationEconomies: array(object({ Name: string, Proportion: number })),
  StationServices: array(string), DistFromStarLS: number,
  LandingPads: object({ Small: integer, Medium: integer, Large: integer }),
};
const scanFields = {
  ScanType: string, BodyName: string, BodyID: integer,
  Parents: array(object({ Star: integer, Planet: integer, Null: integer })),
  DistanceFromArrivalLS: number, TidalLock: boolean, TerraformState: string,
  PlanetClass: string, Atmosphere: string, AtmosphereType: string,
  AtmosphereComposition: array(object({ Name: string, Percent: number })),
  Volcanism: string, MassEM: number, Radius: number, SurfaceGravity: number,
  SurfaceTemperature: number, SurfacePressure: number, Landable: boolean,
  Materials: array(object({ Name: string, Percent: number })),
  Composition: object({ Ice: number, Rock: number, Metal: number }),
  SemiMajorAxis: number, Eccentricity: number, OrbitalInclination: number,
  Periapsis: number, OrbitalPeriod: number, AscendingNode: number, MeanAnomaly: number,
  RotationPeriod: number, AxialTilt: number,
  Rings: array(object({ Name: string, RingClass: string, MassMT: number, InnerRad: number, OuterRad: number })),
  ReserveLevel: string, StarType: string, Subclass: integer, StellarMass: number,
  AbsoluteMagnitude: number, Age_MY: integer, Luminosity: string,
};
const JOURNAL_FIELDS = Object.freeze({
  Docked: stationFields,
  Location: { ...systemFields, ...stationFields },
  FSDJump: systemFields,
  CarrierJump: { ...systemFields, ...stationFields },
  Scan: scanFields,
  SAASignalsFound: {
    BodyName: string, BodyID: integer,
    Signals: array(object({ Type: string, Count: integer })),
    Genuses: array(object({ Genus: string })),
  },
});

function coordinates(value) {
  return Array.isArray(value) && value.length === 3 && value.every(item => number(item) !== undefined) ? value.slice() : undefined;
}

function enabled(context) {
  return context && string(context.commanderName) && (context.test === true || (context.liveGalaxy === true && context.isBeta !== true));
}

function flags(context) {
  const result = {};
  if (typeof context.horizons === 'boolean') result.horizons = context.horizons;
  if (typeof context.odyssey === 'boolean') result.odyssey = context.odyssey;
  return result;
}

function envelope(schema, message, context, capiEndpoint) {
  const gameversion = capiEndpoint ? `CAPI-Live-${capiEndpoint}` : (typeof context.gameVersion === 'string' ? context.gameVersion : '');
  return {
    $schemaRef: `https://eddn.edcd.io/schemas/${SCHEMAS[schema]}${context.test === true ? '/test' : ''}`,
    header: {
      uploaderID: context.commanderName,
      softwareName: 'ED Copilot',
      softwareVersion: string(context.softwareVersion) || '1.4.1',
      gameversion,
      // Trailing whitespace in the journal build identifier is significant.
      gamebuild: capiEndpoint ? '' : (typeof context.gameBuild === 'string' ? context.gameBuild : ''),
    },
    message: { ...message, ...flags(context) },
  };
}

function journalLocation(entry, context) {
  let name = string(entry.StarSystem);
  let address = integer(entry.SystemAddress);
  let position = coordinates(entry.StarPos);
  if (!name || address === undefined || !position) {
    // Do not attach stale coordinates if a journal stopped writing while the
    // commander moved. There must be an identity in the original observation.
    if (!name && address === undefined) return null;
    if (name && name !== context.systemName) return null;
    if (address !== undefined && address !== context.systemAddress) return null;
    name ||= string(context.systemName);
    address ??= integer(context.systemAddress);
    position ||= coordinates(context.starPos);
  }
  return name && address !== undefined && position ? { StarSystem: name, SystemAddress: address, StarPos: position } : null;
}

function buildJournalMessages(entry, context = {}) {
  if (!enabled(context) || !entry || !Object.hasOwn(JOURNAL_FIELDS, entry.event) || !timestamp(entry.timestamp)) return [];
  const location = journalLocation(entry, context);
  if (!location) return [];
  const publicFields = object(JOURNAL_FIELDS[entry.event])(entry) || {};
  return [envelope('journal', { timestamp: entry.timestamp, event: entry.event, ...location, ...publicFields }, context)];
}

function canonicalName(value) {
  const name = string(value);
  return name ? name.replace(/^\$/, '').replace(/_name;$/i, '').toLowerCase() : undefined;
}

function entries(value) {
  if (Array.isArray(value)) return value;
  return value && typeof value === 'object' ? Object.values(value) : [];
}

function uniqueNames(values) {
  return [...new Set(values.filter(value => string(value)))].sort();
}

function commodities(items, capi) {
  if (!Array.isArray(items)) return null;
  const result = [];
  for (const item of items) {
    if (!item || typeof item !== 'object') return null;
    const category = canonicalName(item.categoryname ?? item.Category);
    if (category === 'nonmarketable' || (typeof item.legality === 'string' && item.legality.length > 0)) continue;
    const commodity = { name: canonicalName(capi ? item.name : item.Name) };
    if (!commodity.name) return null;
    for (const [destination, journalKey] of Object.entries({
      meanPrice: 'MeanPrice', buyPrice: 'BuyPrice', stock: 'Stock', sellPrice: 'SellPrice', demand: 'Demand',
    })) {
      const value = integer(capi ? item[destination] : item[journalKey]);
      // Incomplete data is not an empty market: do not publish a partial list.
      if (value === undefined) return null;
      commodity[destination] = value;
    }
    for (const [destination, journalKey] of [['stockBracket', 'StockBracket'], ['demandBracket', 'DemandBracket']]) {
      const value = capi ? item[destination] : item[journalKey];
      if (![0, 1, 2, 3, ''].includes(value)) return null;
      commodity[destination] = value;
    }
    const statusFlags = uniqueNames(entries(item.statusFlags ?? item.StatusFlags));
    if (statusFlags.length) commodity.statusFlags = statusFlags;
    result.push(commodity);
  }
  return result.sort((left, right) => left.name.localeCompare(right.name));
}

function stationIdentity(systemName, stationName, marketId, observedAt, context) {
  if (context.docked !== true || !string(systemName) || !string(stationName) || integer(marketId) === undefined || !timestamp(observedAt)) return null;
  if (systemName !== context.systemName || stationName !== context.stationName || marketId !== context.marketId) return null;
  return { timestamp: observedAt, systemName, stationName, marketId };
}

function moduleNames(items, capi) {
  return uniqueNames(entries(items).filter(item => {
    if (!item || typeof item !== 'object') return false;
    const name = capi ? item.name : item.Name;
    return typeof name === 'string' && /(^hpt_|^int_|_armour_)/i.test(name) &&
      name.toLowerCase() !== 'int_planetapproachsuite' && (item.sku == null || item.sku === HORIZONS_SKU);
  }).map(item => canonicalName(capi ? item.name : item.Name)));
}

function buildCapiMessages(snapshot, context) {
  const profile = snapshot.profile;
  if (!profile || profile.commander?.docked !== true || profile.commander?.name !== context.commanderName) return [];
  const systemName = profile.lastSystem?.name;
  const stationName = profile.lastStarport?.name;
  const marketId = profile.lastStarport?.id;
  if (context.gameRunning === false) {
    const colony = entries(snapshot.market?.economies).some(item => item?.name === 'Colony');
    const hardware = [
      ...entries(snapshot.shipyard?.modules),
      ...entries(snapshot.shipyard?.ships?.shipyard_list),
      ...entries(snapshot.shipyard?.ships?.unavailable_list),
    ];
    // A logged-out client cannot supply a current LoadGame expansion flag.
    // Missing CAPI hardware data is unknown, not proof of a Horizons=false.
    const observedHardware = snapshot.shipyard && (snapshot.shipyard.modules != null || snapshot.shipyard.ships != null);
    context = { ...context, horizons: colony || hardware.some(item => item?.sku === HORIZONS_SKU) ? true : observedHardware ? false : undefined };
  }
  const output = [];
  for (const [endpoint, data] of [['market', snapshot.market], ['shipyard', snapshot.shipyard]]) {
    if (!data || data.id !== marketId || (data.name && data.name !== stationName)) continue;
    const base = stationIdentity(systemName, stationName, marketId, snapshot.observedAt?.[endpoint] || data.timestamp, context);
    if (!base) continue;
    if (endpoint === 'market') {
      const list = commodities(data.commodities, true);
      if (!list) continue;
      const message = { ...base, commodities: list };
      const economies = entries(data.economies).map(object({ name: string, proportion: number })).filter(item => item?.name && item.proportion !== undefined);
      if (economies.length) message.economies = economies;
      const prohibited = uniqueNames(entries(data.prohibited).map(canonicalName));
      if (prohibited.length) message.prohibited = prohibited;
      output.push(envelope('commodity', message, context, 'market'));
    } else {
      const modules = moduleNames(data.modules, true);
      if (modules.length) output.push(envelope('outfitting', { ...base, modules }, context, 'shipyard'));
      const ships = uniqueNames([...entries(data.ships?.shipyard_list), ...entries(data.ships?.unavailable_list)].map(item => canonicalName(item?.name)));
      if (ships.length) output.push(envelope('shipyard', { ...base, ships }, context, 'shipyard'));
    }
  }
  return output;
}

function buildStationMessages(snapshot, context = {}) {
  if (!enabled(context) || !snapshot) return [];
  if (snapshot.source === 'capi') return buildCapiMessages(snapshot, context);
  if (!['Market', 'Outfitting', 'Shipyard'].includes(snapshot.event)) return [];
  const base = stationIdentity(snapshot.StarSystem, snapshot.StationName, snapshot.MarketID, snapshot.timestamp, context);
  if (!base) return [];
  if (snapshot.event === 'Market') {
    const list = commodities(snapshot.Items, false);
    if (!list) return [];
    const message = { ...base, commodities: list };
    if (string(snapshot.StationType)) message.stationType = snapshot.StationType;
    if (string(snapshot.CarrierDockingAccess)) message.carrierDockingAccess = snapshot.CarrierDockingAccess;
    return [envelope('commodity', message, context)];
  }
  if (snapshot.event === 'Outfitting') {
    const modules = moduleNames(snapshot.Items, false);
    return modules.length ? [envelope('outfitting', { ...base, modules }, context)] : [];
  }
  const ships = uniqueNames(entries(snapshot.PriceList ?? snapshot.Pricelist).map(item => canonicalName(item?.ShipType ?? item?.name)));
  return ships.length ? [envelope('shipyard', { ...base, ships }, context)] : [];
}

function retryDelay(header) {
  if (typeof header !== 'string' || !header) return MIN_RETRY_MS;
  const seconds = Number(header);
  const delay = Number.isFinite(seconds) ? seconds * 1000 : Date.parse(header) - Date.now();
  return Math.max(MIN_RETRY_MS, Math.min(24 * 60 * 60 * 1000, Number.isFinite(delay) ? delay : MIN_RETRY_MS));
}

// The caller owns durable queuing and deduplication. This primitive performs one
// upload only; it never silently replays private logs or retries a rejected body.
async function sendEddn(envelopeValue, options = {}) {
  const cancelled = () => ({ ok: false, status: 0, retryable: false, retryAfterMs: 0, error: 'Отменено' });
  if (options.signal?.aborted) return cancelled();
  const ref = envelopeValue?.$schemaRef;
  const supported = Object.values(SCHEMAS).some(schema => ref === `https://eddn.edcd.io/schemas/${schema}` || ref === `https://eddn.edcd.io/schemas/${schema}/test`);
  if (!supported || !envelopeValue.header?.uploaderID || !envelopeValue.message?.timestamp) {
    return { ok: false, status: 0, retryable: false, retryAfterMs: 0, error: 'Некорректное сообщение EDDN' };
  }
  let body;
  try {
    const json = Buffer.from(JSON.stringify(envelopeValue), 'utf8');
    if (json.length > MAX_MESSAGE_BYTES) return { ok: false, status: 413, retryable: false, retryAfterMs: 0, error: 'Сообщение EDDN слишком велико' };
    body = gzipSync(json);
  } catch {
    return { ok: false, status: 0, retryable: false, retryAfterMs: 0, error: 'Не удалось подготовить сообщение EDDN' };
  }
  const transport = options.transport || https.request;
  return new Promise(resolve => {
    let settled = false;
    let request;
    const onAbort = () => {
      finish(cancelled());
      request?.destroy(new Error('EDDN cancelled'));
    };
    const finish = result => {
      if (!settled) {
        settled = true;
        options.signal?.removeEventListener('abort', onAbort);
        resolve(result);
      }
    };
    options.signal?.addEventListener('abort', onAbort, { once: true });
    try {
      request = transport(ENDPOINT, {
        method: 'POST', timeout: options.timeoutMs || 20000,
        headers: {
          'Content-Type': 'application/json; charset=utf-8', 'Content-Encoding': 'gzip',
          'Content-Length': body.length, 'User-Agent': `EDCopilot/${envelopeValue.header.softwareVersion || '1.4.1'}`,
        },
      }, response => {
        let responseBody = '';
        response.setEncoding('utf8');
        response.on('data', chunk => { if (responseBody.length < 4096) responseBody += chunk; });
        response.on('error', () => finish({ ok: false, status: response.statusCode || 0, retryable: true, retryAfterMs: MIN_RETRY_MS, error: 'Соединение EDDN прервано' }));
        response.on('end', () => {
          const status = response.statusCode || 0;
          const ok = status === 200 && responseBody.trim() === 'OK';
          const retryable = !ok && (status === 0 || status === 408 || status === 429 || status >= 500);
          const error = ok ? null : status === 426 ? 'EDDN отклонил версию схемы: требуется обновление приложения' : status === 400 ? 'EDDN отклонил формат сообщения' : status === 413 ? 'EDDN отклонил слишком большое сообщение' : `EDDN: HTTP ${status}`;
          finish({ ok, status, retryable, retryAfterMs: retryable ? retryDelay(response.headers?.['retry-after']) : 0, error });
        });
      });
      request.on('error', () => finish({ ok: false, status: 0, retryable: true, retryAfterMs: MIN_RETRY_MS, error: 'Не удалось подключиться к EDDN' }));
      request.on('timeout', () => request.destroy(new Error('EDDN timeout')));
      if (settled || options.signal?.aborted) { onAbort(); return; }
      request.end(body);
    } catch {
      finish({ ok: false, status: 0, retryable: true, retryAfterMs: MIN_RETRY_MS, error: 'Не удалось подключиться к EDDN' });
    }
  });
}

module.exports = { ENDPOINT, MIN_RETRY_MS, SCHEMAS, buildJournalMessages, buildStationMessages, sendEddn };
