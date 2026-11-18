import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const coreNames = ['powerPlant', 'thrusters', 'frameShiftDrive', 'lifeSupport', 'powerDistributor', 'sensors', 'fuelTank'];
const coreGroups = ['pp', 't', 'fsd', 'ls', 'pd', 's', 'ft'];
const coreKeys = ['PowerPlant', 'MainEngines', 'FrameShiftDrive', 'LifeSupport', 'PowerDistributor', 'Radar', 'FuelTank'];
const hardpointSize = ['', 'Small', 'Medium', 'Large', 'Huge'];
const buildSymbol = value => value.toLowerCase();
const asModule = (record, id, group) => ({
  id, grp: group, symbol: record.symbol, name: record.name, class: record.class, rating: record.rating,
  cost: record.cost ?? 0, mass: record.mass, power: record.powerDraw, integrity: record.integrity, supplemental: true,
  allowedShips: record.restrictedToShips,
});

function readJSONC(almanac, file) {
  const contents = readFileSync(join(almanac, 'data/ships', file + '.jsonc'), 'utf8');
  return JSON.parse(contents.replace(/"(?:\\.|[^"\\])*"|(\/\*[\s\S]*?\*\/|\/\/[^\r\n]*)/g, (token, comment) => comment ? '' : token));
}

function fittedModuleID(modules, symbol) {
  const id = modules.find(candidate => buildSymbol(candidate.symbol || '') === buildSymbol(symbol || ''))?.id;
  if (symbol && id == null) throw new Error('Unknown fitted component: ' + symbol);
  return id || 0;
}

function supplementalShip(hull, registry, armour, fit, internals, hardpoints, standard) {
  const bulkheads = armour.filter(part => part.ship === hull.name)
    .sort((a, b) => Number(b.symbol.endsWith('_Default')) - Number(a.symbol.endsWith('_Default')))
    .map((part, index) => ({
      id: 'b' + index, grp: 'bh', symbol: part.symbol, name: part.name, cost: part.cost, mass: part.mass,
      hullboost: part.hullBoost, kinres: part.kineticResistance, thermres: part.thermalResistance,
      explres: part.explosiveResistance, causres: part.causticResistance, supplemental: true,
    }));
  const loadout = Object.fromEntries(fit.modules.map(module => [module.slot, module.symbol]));
  const hardpointCounters = {};
  const hardpointKeys = hull.hardpoints.map(({ size }) => {
    hardpointCounters[size] = (hardpointCounters[size] || 0) + 1;
    return hardpointSize[size] + 'Hardpoint' + hardpointCounters[size];
  });
  const internalRestrictions = {
    passenger: { pce: 1, pci: 1, pcm: 1, pcq: 1 },
    cargo: { cr: 1, crl: 1, ft: 1 },
    military: { hr: 1, ghrp: 1, mahr: 1, mrp: 1, gmrp: 1, scb: 1 },
    planetaryApproachSuite: { pas: 1 },
  };
  const internalNames = { passenger: 'Passenger', cargo: 'Cargo', military: 'Military', planetaryApproachSuite: 'PlanetaryApproachSuite' };
  const internalCounters = { slot: 0 };
  const optional = hull.optional.map(({ size, restriction, name }) => {
    const prefix = internalNames[restriction];
    let journalKey = name || prefix;
    if (!journalKey) journalKey = 'Slot' + String(++internalCounters.slot).padStart(2, '0') + '_Size' + size;
    else if (!name && prefix !== 'PlanetaryApproachSuite') {
      internalCounters[prefix] = (internalCounters[prefix] || 0) + 1;
      journalKey = prefix + String(internalCounters[prefix]).padStart(2, '0');
    }
    return { class: size, name: prefix, eligible: internalRestrictions[restriction], journalKey };
  });
  return {
    edID: registry.edID,
    frontierSymbol: hull.symbol,
    supplemental: true,
    properties: {
      name: hull.name, manufacturer: hull.manufacturer, class: { small: 1, medium: 2, large: 3 }[hull.size],
      hullCost: hull.hullCost, speed: hull.maximumSpeed, boost: hull.boost, minthrust: registry.minThrust,
      boostEnergy: registry.boostEnergy, boostInt: registry.boostInterval, fighterHangars: registry.fighterHangars,
      baseShieldStrength: hull.baseShieldStrength, baseArmour: hull.baseArmour, hullMass: hull.hullMass,
      hardness: hull.hardness, masslock: hull.masslock, heatCapacity: hull.heatCapacity, heatDissipation: hull.heatDissipation,
      pitch: hull.pitch, roll: hull.roll, yaw: hull.yaw, minPitch: hull.minPitch, minRoll: hull.minRoll, minYaw: hull.minYaw,
      pipSpeed: (1 - hull.minPitch / hull.pitch) / 4, crew: hull.crew, reserveFuelCapacity: hull.reserveFuelCapacity
    },
    retailCost: hull.retailCost,
    bulkheads,
    slots: { standard: coreNames.map(key => hull.core[key]), hardpoints: hull.hardpoints.map(slot => slot.size).concat(Array(hull.utility).fill(0)), internal: optional },
    defaults: {
      standard: coreKeys.map((key, index) => fittedModuleID(standard[coreGroups[index]], loadout[key])),
      hardpoints: hardpointKeys.map(key => fittedModuleID(hardpoints, loadout[key])).concat(Array(hull.utility).fill(0)),
      internal: optional.map(slot => fittedModuleID(internals, loadout[slot.journalKey])),
    },
  };
}

// Compact codes persisted by the application refer to these IDs.
const supplementalIDs = {
  Int_MkIILargeBuggyBay_Size2_Class3: 'q0',
  Int_MkIILargeBuggyBay_Size2_Class3_Free: 'q1',
  Int_MkIILargeBuggyBay_Size4_Class3: 'q2',
  Int_MkIILargeBuggyBay_Size4_Class3_Free: 'q3',
  Int_MkIILargeBuggyBay_Size6_Class3: 'q4',
  Int_MkIILargeBuggyBay_Size6_Class3_Free: 'q5',
  Int_FighterBay_Size5_Class1_Free: 'q6',
  Int_FighterBay_Size6_Class1_Free: 'q7',
  Int_FighterBay_Size7_Class1_Free: 'q8',
  Int_FighterBayMk2_Size5_Class1: 'q9',
  Int_FighterBayMk2_Size6_Class1: 'qA',
  Int_FighterBayMk2_Size7_Class1: 'qB',
  Int_FighterBayMk2_Size5_Class1_Free: 'qC',
  Int_FighterBayMk2_Size6_Class1_Free: 'qD',
  Int_FighterBayMk2_Size7_Class1_Free: 'qE',
  Int_LargeCargoRack_Size6_Class1: 'qF',
  Int_MkII_PassengerCabin_Size2_Class1: 'qG',
  Int_MkII_PassengerCabin_Size3_Class1: 'qH',
  Int_MkII_PassengerCabin_Size3_Class2: 'qI',
  Int_MkII_PassengerCabin_Size4_Class1: 'qJ',
  Int_MkII_PassengerCabin_Size4_Class2: 'qK',
  Int_MkII_PassengerCabin_Size5_Class1: 'qL',
  Int_MkII_PassengerCabin_Size5_Class2: 'qM',
  Int_MkII_PassengerCabin_Size6_Class1: 'qN',
  Int_MkII_PassengerCabin_Size6_Class2: 'qO',
};

export function extend(rawData, almanac) {
  const hulls = readJSONC(almanac, 'ships');
  const core = readJSONC(almanac, 'modules-core');
  const internal = readJSONC(almanac, 'modules-internal');
  const loadouts = readJSONC(almanac, 'default-loadouts');
  const hardpoints = Object.values(rawData.Modules.hardpoints).flat();
  const allModules = Object.values(rawData.Modules).flatMap(groups => Object.values(groups).flat());
  const knownIDs = new Set(allModules.map(module => String(module.id)));
  const knownSymbols = new Set(allModules.map(module => buildSymbol(module.symbol || '')));
  function extraModule(row, category, group, extras) {
    const id = supplementalIDs[row.symbol];
    if (!id && knownSymbols.has(buildSymbol(row.symbol))) return;
    if (!id) throw new Error('Missing ID for ' + row.symbol);
    if (knownIDs.has(id) || knownSymbols.has(buildSymbol(row.symbol))) throw new Error('Outfitting compatibility check required for ' + row.symbol);
    rawData.Modules[category][group].push({ ...asModule(row, id, group), ...extras });
    knownIDs.add(id);
    knownSymbols.add(buildSymbol(row.symbol));
  }
  for (const row of internal) {
    if (/^Int_(?:FighterBay|FighterBayMk2)_Size[567]_Class1(?:_Free)?$/i.test(row.symbol)) {
      extraModule(row, 'internal', 'fh', {
        bays: row.class === 5 ? 1 : 2, rebuildsperbay: { 5: 6, 6: 8, 7: 30 }[row.class],
      });
    } else if (/^Int_MkIILargeBuggyBay_Size[246]_Class3(?:_Free)?$/i.test(row.symbol)) {
      const stats = { bays: { 2: 1, 4: 2, 6: 4 }[row.class], vesselHangar: true };
      extraModule(row, 'internal', 'pv', stats);
      if (!row.symbol.endsWith('_Free') && !internal.some(module => module.symbol === row.symbol + '_Free')) {
        // The bundled versions use the same mass and power draw. Their item IDs and
        // zero purchase price are recorded in EDSY's pinned October 2026 specifications.
        const edID = { 2: 129046184, 4: 129046185, 6: 129046186 }[row.class];
        extraModule({ ...row, symbol: row.symbol + '_Free', cost: 0 }, 'internal', 'pv', { ...stats, edID });
      }
    } else if (/^Int_MkII_PassengerCabin_/i.test(row.symbol)) {
      const group = /Class1$/i.test(row.symbol) ? 'pce' : 'pci';
      extraModule(row, 'internal', group, { passengers: row.cabinCapacity });
    } else if (/^Int_LargeCargoRack_Size6_/i.test(row.symbol)) {
      extraModule(row, 'internal', 'crl', { cargo: row.cargoCapacity });
    }
  }
  const internals = Object.values(rawData.Modules.internal).flat();
  const additions = {
    lynx_highliner: { symbol: 'MediumTransport01', edID: 129041442, minThrust: 73.75, boostEnergy: 14, boostInterval: 5 },
    anaconda_mk_ii: { symbol: 'AnacondaMkII', edID: 129046025, minThrust: 47, boostEnergy: 27, boostInterval: 6, fighterHangars: true },
  };
  for (const [id, observation] of Object.entries(additions)) {
    const hull = hulls.find(item => item.symbol === observation.symbol);
    const stock = loadouts.find(item => item.symbol === observation.symbol);
    rawData.Ships[id] = supplementalShip(hull, observation, core, stock, internals, hardpoints, rawData.Modules.standard);
  }
  rawData.Ships.imperial_corsair.defaults.internal.splice(9, 0, 0);
  const fitDataNames = new Map(hulls.map(hull => [hull.name.toLowerCase(), hull]));
  const aliases = { viper: 'viper mkiii', type_7_transport: 'type-7 transporter' };
  for (const [id, ship] of Object.entries(rawData.Ships)) {
    const hull = fitDataNames.get(aliases[id] || ship.properties.name.toLowerCase());
    if (!hull) continue;
    ship.frontierSymbol = hull.symbol;
    ship.journalSlots = {
      hardpoints: hull.hardpoints.map(slot => slot.name || ''),
      internal: hull.optional.map(slot => slot.name || ''),
    };
    // Coriolis module IDs/slot order stay stable for imported URLs. Apply audited flight figures
    // separately from that slot layout, which upcoming Coriolis builds may also adjust.
    for (const key of ['hullMass', 'baseShieldStrength', 'baseArmour', 'hardness', 'masslock', 'crew', 'pitch', 'yaw', 'roll', 'minPitch',
      'minYaw', 'minRoll', 'heatCapacity', 'heatDissipation', 'reserveFuelCapacity', 'boost']) {
      if (Number.isFinite(hull[key])) ship.properties[key] = hull[key];
    }
    ship.properties.speed = hull.maximumSpeed;
    ship.properties.minthrust = hull.minimumSpeed / hull.maximumSpeed * 100;
    ship.cargoHatchSymbol = loadouts.find(record => record.symbol === hull.symbol)?.modules.find(module => module.slot === 'CargoHatch')?.symbol;
  }
}
