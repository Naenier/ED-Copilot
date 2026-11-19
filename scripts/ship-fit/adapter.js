import LZString from 'lz-string';
import { fromUrlSafe } from './upstream/utils/UtilityFunctions';
import Ship from './upstream/shipyard/Ship';
import Module from './upstream/shipyard/Module';
import * as Calc from './upstream/shipyard/Calculations';
import { ModuleGroupToName } from './upstream/shipyard/Constants';
import { toDetailedBuild } from './upstream/shipyard/Serializer';
import { HARDPOINT_NUM_TO_CLASS, SHIP_FD_NAME_TO_CORIOLIS_NAME } from './upstream/utils/CompanionApiUtils';
import { getPercent, setQualityCB } from './upstream/utils/BlueprintFunctions';
import { _addModifications } from './upstream/utils/JournalUtils';
import { Modifications, Ships } from 'coriolis-data/dist';

const coreKeys = ['PowerPlant', 'MainEngines', 'FrameShiftDrive', 'LifeSupport', 'PowerDistributor', 'Radar', 'FuelTank'];
const coreNames = ['Power Plant', 'Thrusters', 'Frame Shift Drive', 'Life Support', 'Power Distributor', 'Sensors', 'Fuel Tank'];
const damageTypes = ['absolute', 'kinetic', 'thermal', 'explosive', 'caustic'];
const namedSlots = { Military: 'Military', Limpets: 'LimpetController', Fighter: 'FighterBay', Cargo: 'Cargo', Passenger: 'Passenger' };
const maxCodeLength = 32768;
const settings = ['cargo', 'fuel', 'eng', 'sys', 'wep'];
const rewardRackBlueprint = 'CargoRack_IncreasedCapacity';
const legacyRailEffects = new Set(['special_feedback_cascade', 'special_super_penetrator', 'special_plasma_slug']);
const own = (object, key) => Object.hasOwn(object, key);
const finite = n => Number.isFinite(n) ? n : null;
const clamp = (n, max, fallback) => Number.isFinite(n) ? Math.min(max, Math.max(0, n)) : fallback;
const lower = value => String(value ?? '').toLowerCase();
const itemSymbol = value => lower(value).trim().replace(/^\$/, '').replace(/;$/, '').replace(/_name$/, '');
const normalShipName = value => lower(value).replace(/[^a-z0-9]/g, '');
const fail = message => { throw new Error(message); };
const roundPips = value => Math.round(value * 2) / 2;

const shipIds = new Map(Object.entries(SHIP_FD_NAME_TO_CORIOLIS_NAME).map(([key, id]) => [normalShipName(key), id]));
for (const [id, data] of Object.entries(Ships)) {
  shipIds.set(normalShipName(id), id);
  shipIds.set(normalShipName(data.properties.name), id);
  if (data.frontierSymbol) shipIds.set(normalShipName(data.frontierSymbol), id);
}

function makeShip(id) {
  if (typeof id !== 'string' || !own(Ships, id)) fail('Неизвестная модель корабля.');
  const data = Ships[id];
  const ship = new Ship(id, data.properties, data.slots);
  ship.cargoHatch.m.symbol = data.cargoHatchSymbol || 'modularcargobaydoor';
  return ship;
}

function validName(value, fallback) {
  return typeof value === 'string' && value.trim() ? value.trim().slice(0, 120) : fallback;
}

function toFit(ship, name, options = {}) {
  return {
    version: 1, shipId: ship.id, name: validName(name, ship.name), code: ship.toString(),
    ...Object.fromEntries(settings.filter(key => Number.isFinite(options[key])).map(key => [key, options[key]])),
  };
}

export function ships() {
  return Object.entries(Ships).map(([id, data]) => ({
    id, name: data.properties.name, size: data.properties.class, manufacturer: data.properties.manufacturer,
  })).sort((a, b) => a.name.localeCompare(b.name));
}

export function create(shipId) {
  const ship = makeShip(shipId).buildWith(Ships[shipId].defaults);
  return toFit(ship);
}

let memo;
function restore(fit, mutable = false) {
  if (!fit || fit.version !== 1 || typeof fit.code !== 'string' || fit.code.length > maxCodeLength ||
      !/^[A-Za-z0-5][A-Za-z0-9.-]*(?:[+/=:\-_][A-Za-z0-9.+/=:\-_]*)?$/.test(fit.code)) {
    fail('Не удалось прочитать код сборки.');
  }
  if (!mutable && memo?.shipId === fit.shipId && memo.code === fit.code) return memo.ship;
  const ship = makeShip(fit.shipId);
  try {
    ship.buildFrom(fit.code);
  } catch {
    fail('Не удалось прочитать код сборки.');
  }
  if (ship.standard.some(slot => !slot.m) || !Number.isFinite(ship.dryMass)) {
    fail('В сборке отсутствует обязательный модуль.');
  }
  const moduleCode = fit.code.split('.')[0];
  if (moduleCode.startsWith('A') && ship.toString().split('.')[0].replace(/-+$/, '') !== moduleCode.replace(/-+$/, '')) {
    fail('Код сборки содержит неподдерживаемый модуль или отсек.');
  }
  if (!mutable) memo = { shipId: fit.shipId, code: fit.code, ship };
  return ship;
}

function descriptors(ship) {
  const hardpointCounts = {};
  const data = Ships[ship.id];
  const optionalCounts = {};
  let optionalIndex = 0;
  return [
    { key: 'Armour', kind: 'armour', name: 'Bulkheads', slot: ship.bulkheads, required: true },
    ...ship.standard.map((slot, index) => ({ key: coreKeys[index], name: coreNames[index], slot, kind: 'core', required: true })),
    { key: 'CargoHatch', kind: 'hatch', name: 'Cargo Hatch', slot: ship.cargoHatch, required: true },
    ...ship.hardpoints.map((slot, i) => {
      const size = slot.maxClass;
      let index = hardpointCounts[size] ?? 0;
      index += ship.id === 'type_8_transport' && size === 1 && index === 2 ? 2 : 1;
      hardpointCounts[size] = index;
      return {
        key: data.journalSlots?.hardpoints[i] || HARDPOINT_NUM_TO_CLASS[size] + (slot.name || '') + 'Hardpoint' + index,
        name: slot.name || '', kind: size ? 'hardpoint' : 'utility', slot,
      };
    }),
    ...ship.internal.map((slot, i) => {
      const exactKey = data.slots.internal[i].journalKey || data.journalSlots?.internal[i];
      if (slot.name === 'PlanetaryApproachSuite') return { key: slot.name, name: slot.name, kind: 'optional', slot, required: true };
      const prefix = namedSlots[slot.name];
      if (prefix) {
        const index = (optionalCounts[prefix] ?? 0) + 1;
        optionalCounts[prefix] = index;
        return { key: exactKey || prefix + String(index).padStart(2, '0'), name: slot.name, kind: 'optional', slot };
      }
      const key = 'Slot' + String(++optionalIndex).padStart(2, '0') + '_Size' + slot.maxClass;
      return { key: exactKey || key, name: '', kind: 'optional', slot };
    }),
  ];
}

function describeModule(m) {
  if (!m) return null;
  return {
    id: String(m.id ?? 'cargo-hatch'), symbol: m.symbol || (m.id == null ? 'modularcargobaydoor' : ''),
    edId: m.edID ?? null, name: m.name || ModuleGroupToName[m.grp] || '',
    group: m.grp || '', groupName: ModuleGroupToName[m.grp] || m.name || '',
    mount: m.mount || '', size: Number(m.class) || 0, rating: m.rating || '', missile: m.missile || '',
    ...(m.preEngineered ? {
      preEngineered: true,
      engineeringLocked: !m.preEngineered.reengineerable,
      factoryExperimental: m.preEngineered.experimentalEffects?.[0] ?? '',
    } : {}),
    mass: finite(m.getMass()), power: finite(m.getPowerUsage()), cost: finite(m.cost), integrity: finite(m.getIntegrity()),
    range: finite(m.getRange()), damage: finite(m.getDamage()), dps: finite(m.getDps()), sustainedDps: finite(m.getSDps()),
    eps: finite(m.getEps()), hps: finite(m.getHps()), ammo: finite(m.getAmmo()), clip: finite(m.getClip()),
    rateOfFire: finite(m.getRoF()), piercing: finite(m.getPiercing()), falloff: finite(m.getFalloff()), shotSpeed: finite(m.getShotSpeed()),
    probeRadius: finite(m.get('proberadius')),
  };
}

function describeEngineering(m) {
  const bp = m?.blueprint;
  if (!bp?.fdname) return null;
  const percent = getPercent(m);
  return {
    blueprint: bp.fdname, symbol: bp.fdname, name: bp.name || bp.fdname, grade: bp.grade,
    experimental: bp.special?.edname || '', experimentalName: bp.special?.name || '',
    quality: percent === null && bp.grades?.[bp.grade] ? (Object.values(bp.grades[bp.grade].features).every(([a, b]) => a === b) ? 1 : null) : clamp(percent / 100, 1, null),
  };
}

export function slots(fit) {
  const ship = restore(fit);
  return descriptors(ship).map(({ slot, key, kind, name, required }) => {
    const retracted = ship.getSlotStatus(slot, false);
    return {
      key, kind, name, required: Boolean(required), size: slot.maxClass ?? 0,
      module: describeModule(slot.m), on: kind === 'armour' || Boolean(slot.enabled),
      priority: (slot.priority ?? 0) + 1,
      engineering: describeEngineering(slot.m),
      retracted: kind === 'armour' ? true : retracted ? retracted === 3 : null,
      deployed: kind === 'armour' || ship.getSlotStatus(slot, true) === 3,
    };
  });
}

function descriptor(ship, slotKey) {
  return descriptors(ship).find(entry => entry.key === slotKey) || fail('Выберите отсек корабля.');
}

function restrictedFit(ship, slot, m) {
  if (m.allowedShips && !m.allowedShips.includes(Ships[ship.id].frontierSymbol)) return false;
  if (lower(m.name).startsWith('missing') || m.rating === 'Z' || /^\w+_missing_/i.test(m.symbol || '')) return false;
  if (slot.name === 'PlanetaryApproachSuite') return m.grp === 'pas';
  if (m.grp === 'pas') return false;
  if (m.grp === 'crl') return slot.name === 'Cargo';
  if (m.id === '7W') return slot.name === 'Limpets';
  return true;
}

function templates(ship, { slot, kind, key }) {
  const set = ship.getAvailableModules();
  switch (kind) {
    case 'hatch': return [];
    case 'armour': return set.bulkheads;
    case 'core': return set.standard[coreKeys.indexOf(key)].filter(module => restrictedFit(ship, slot, module));
    case 'optional': {
      const available = Object.values(set.getInts(ship, slot.maxClass, slot.eligible)).flat();
      if (slot.name === 'Fighter') {
        available.push(...(set.internal.pv || []).filter(module => module.vesselHangar && module.class <= slot.maxClass));
      }
      return available.filter(module => restrictedFit(ship, slot, module));
    }
    default: return Object.values(set.getHps(ship, slot.maxClass, slot.eligible)).flat().filter(module => restrictedFit(ship, slot, module));
  }
}

export function candidates(fit, slotKey) {
  const ship = restore(fit);
  const entry = descriptor(ship, slotKey);
  return templates(ship, entry).map(template => describeModule(new Module({ template })));
}

function blueprintEntries(module) {
  if (!module) return [];
  const rules = Modifications.modules[module.grp];
  return Object.entries(rules?.blueprints || {}).flatMap(([id, spec]) => {
    const definition = Modifications.blueprints[id];
    if (!definition) return [];
    const readOnlyRack = id === rewardRackBlueprint && !module.preEngineered;
    if (readOnlyRack && module.blueprint?.fdname !== rewardRackBlueprint) return [];
    const factory = module.preEngineered;
    const currentEffect = module.blueprint?.special?.edname;
    return [{
      id, symbol: definition.fdname, name: definition.name || id,
      grades: readOnlyRack ? [module.blueprint.grade] : Object.keys(spec.grades || definition.grades || {}).map(Number).sort((a, b) => a - b),
      experimentals: (rules.specials || []).filter(specialId =>
        (specialId === currentEffect || factory?.canApplyExperimental !== false) &&
        (module.grp !== 'rg' || specialId === currentEffect || !legacyRailEffects.has(specialId)))
        .map(specialId => Modifications.specials[specialId]).filter(Boolean).map(special => ({
          id: special.edname, symbol: special.edname, name: special.name || special.edname,
        })),
    }];
  });
}

export function blueprints(fit, slotKey) {
  const ship = restore(fit);
  return blueprintEntries(descriptor(ship, slotKey).slot.m);
}

function editEngineering(ship, module, change) {
  if (!module) fail('Установите модуль перед инженерной доработкой.');
  const current = describeEngineering(module);
  const locked = module.preEngineered && !module.preEngineered.reengineerable;
  if (change == null) {
    if (locked) fail('Штатная доработка этого модуля сохраняется.');
    ship.clearModuleBlueprint(module);
    ship.clearModifications(module);
    return;
  }
  const id = change.blueprint ?? current?.blueprint;
  const grade = change.grade ?? current?.grade;
  const experiment = change.experimental ?? current?.experimental ?? '';
  const sameRecipe = current && id === current.blueprint && grade === current.grade;
  const keepModifiers = sameRecipe && (!Number.isFinite(change.quality) || change.quality === current.quality);
  const quality = clamp(change.quality ?? current?.quality, 1, 1);
  const choice = blueprintEntries(module).find(candidate => candidate.id === id || candidate.symbol === id);
  if (!choice || !choice.grades.includes(grade)) fail('Эта ступень чертежа недоступна для модуля.');
  if (choice.id === rewardRackBlueprint && !module.preEngineered && !keepModifiers) {
    fail('Эта ступень чертежа недоступна для модуля.');
  }
  const special = experiment ? choice.experimentals.find(item => item.id === experiment) : null;
  if (experiment && !special) fail('Этот экспериментальный эффект недоступен для модуля.');
  if (locked) {
    if (current?.blueprint !== choice.symbol || grade !== current.grade) fail('Штатная доработка этого модуля сохраняется.');
    const effects = module.preEngineered.experimentalEffects;
    if (effects?.length && experiment !== current.experimental) fail('Штатный экспериментальный эффект этого модуля сохраняется.');
    // Coriolis applies the original prerelease blueprints cumulatively when changing only the experimental effect.
    ship.setModuleSpecial(module, special ? Modifications.specials[special.id] : null);
    return;
  }
  if (keepModifiers) {
    ship.setModuleSpecial(module, special ? Modifications.specials[special.id] : null);
    return;
  }
  const bp = structuredClone(Modifications.blueprints[choice.id]);
  bp.grade = grade;
  ship.setModuleBlueprint(module, bp);
  setQualityCB(bp, quality, (feature, value) => ship.setModification(module, feature, value, false, true));
  ship.setModuleSpecial(module, special ? Modifications.specials[special.id] : null);
}

function experimentalWeaponExcess(modules) {
  let equipped = 0;
  let limit = 4;
  for (const module of modules) {
    if (module?.limitGroup === 'experimentalWeapon') equipped++;
    if (module?.limitIncrease?.group === 'experimentalWeapon') limit += module.limitIncrease.amount;
  }
  return Math.max(0, equipped - limit);
}

function enforceFittingLimits(ship, slot, template) {
  const others = [...ship.internal, ...ship.hardpoints].filter(other => other !== slot).map(other => other.m);
  if (template?.exclusionGroup && template.exclusionGroup !== slot.m?.exclusionGroup &&
      others.some(module => module?.exclusionGroup === template.exclusionGroup)) {
    fail('Модуль этого типа уже установлен в другом отсеке.');
  }
  if (experimentalWeaponExcess([...others, template]) > experimentalWeaponExcess([...others, slot.m])) {
    fail('Превышен лимит экспериментального вооружения.');
  }
}

export function edit(fit, slotKey, change = {}) {
  const ship = restore(fit, true);
  const entry = descriptor(ship, slotKey);
  const { slot, kind, required } = entry;
  if (own(change, 'moduleId')) {
    const id = change.moduleId;
    if (!id && required) fail('Этот отсек должен быть оснащён модулем.');
    const template = id ? templates(ship, entry).find(module => String(module.id) === String(id)) : null;
    if (id && !template) fail('Модуль не подходит к этому отсеку.');
    if (id !== String(slot.m?.id ?? '')) {
      enforceFittingLimits(ship, slot, template);
      if (kind === 'armour') ship.useBulkhead(template.index);
      else ship.use(slot, template ? new Module({ template }) : null);
    }
  }
  if (own(change, 'engineering')) editEngineering(ship, slot.m, change.engineering);
  if (typeof change.on === 'boolean' && kind !== 'armour') ship.setSlotEnabled(slot, change.on);
  if (Number.isInteger(change.priority) && kind !== 'armour') ship.setSlotPriority(slot, change.priority - 1);
  return toFit(ship.updateModificationsString(), fit.name, fit);
}

function readPips({ eng, sys, wep }) {
  const e = roundPips(clamp(eng, 4, 2));
  const s = roundPips(clamp(sys, 4, 2));
  const w = roundPips(clamp(wep, 4, 2));
  return e + s + w === 6 ? [e, s, w] : [2, 2, 2];
}

export function stats(fit, options = {}) {
  const ship = restore(fit);
  const config = { ...fit, ...options };
  const cargo = clamp(config.cargo, ship.cargoCapacity, 0);
  const fuel = clamp(config.fuel, ship.fuelCapacity, ship.fuelCapacity);
  const [eng, sys, wep] = readPips(config);
  const canThrust = ship.canThrust(cargo, fuel);
  const canBoost = ship.canBoost(cargo, fuel) && ship.getSlotStatus(ship.standard[4]) === 3;
  const movement = (tonnage, boost) => (boost ? ship.canBoost(tonnage, fuel) && ship.getSlotStatus(ship.standard[4]) === 3 : ship.canThrust(tonnage, fuel))
    ? finite(ship.calcSpeed(eng, fuel, tonnage, boost)) : 0;
  const pp = ship.standard[0].m;
  const pd = ship.standard[4].m;
  const fsd = ship.standard[2].m;
  const mass = ship.dryMass + fuel + cargo;
  const jumpFuel = Math.min(fuel, fsd.getMaxFuelPerJump());
  const maxJumpFuel = Math.min(ship.fuelCapacity, fsd.getMaxFuelPerJump());
  const fsdOnline = ship.getSlotStatus(ship.standard[2]) === 3;
  const distance = (shipMass, usableFuel) => fsdOnline && usableFuel > 0 ? finite(Calc.jumpRange(shipMass, fsd, usableFuel, ship)) : 0;
  // Combat figures use the deployed power budget. Keep passive protection
  // even if the reactor cannot power other modules from the same band.
  const powered = slot => slot.m && (slot.m.getPowerUsage() === 0 || ship.getSlotStatus(slot, true) === 3);
  const combat = Object.assign(Object.create(ship), {
    hardpoints: ship.hardpoints.filter(powered),
    internal: ship.internal.filter(powered),
  });
  combat.recalculateDps().recalculateEps().recalculateHps();
  const shields = Calc.shieldMetrics(combat, sys);
  const rotation = (axis, minimum, formula) => {
    if (!canThrust) return 0;
    const pipsRatio = Number.isFinite(minimum) ? (1 - minimum / axis) / 4 : ship.pipSpeed;
    return finite(formula(mass + (ship.reserveFuelCapacity || 0), axis, ship.standard[1].m, pipsRatio, eng, ship.boost / ship.speed, false));
  };
  const armour = Calc.armourMetrics(combat);
  const shield = shields.summary || 0;
  const distOnline = ship.getSlotStatus(ship.standard[4], true) === 3;
  const metricsByType = (metrics, total, field, resistances) => Object.fromEntries(damageTypes.flatMap(type => {
    const multiplier = metrics[type]?.[field] ?? (type === 'absolute' && field === 'base' ? 1 : null);
    return multiplier == null ? [] : [[type, finite(resistances ? 1 - multiplier : total / multiplier)]];
  }));
  return {
    dryMass: ship.dryMass, mass, unladenMass: ship.dryMass + ship.fuelCapacity, ladenMass: ship.ladenMass,
    cargo, cargoCapacity: ship.cargoCapacity, passengerCapacity: ship.passengerCapacity,
    fuel, fuelCapacity: ship.fuelCapacity, reserveFuel: ship.reserveFuelCapacity || 0, eng, sys, wep,
    speed: movement(cargo, false), boostSpeed: movement(cargo, true),
    unladenSpeed: movement(0, false), ladenSpeed: movement(ship.cargoCapacity, false),
    unladenBoostSpeed: movement(0, true), ladenBoostSpeed: movement(ship.cargoCapacity, true),
    pitch: rotation(ship.pitch, ship.minPitch, Calc.calcPitch),
    roll: rotation(ship.roll, ship.minRoll, Calc.calcRoll),
    yaw: rotation(ship.yaw, ship.minYaw, Calc.calcYaw),
    canThrust, canBoost, boostInterval: canBoost && eng > 0 ? finite(Math.max(ship.boostInt || 0, Calc.calcBoost(ship) / Math.pow(eng / 4, 1.1))) : null,
    jumpRange: distance(mass, jumpFuel),
    // Frontier's Loadout.MaxJumpRange assumes an empty operating reserve. Live
    // flight scenarios below keep the reserve mass used by Coriolis physics.
    maximumRange: distance(ship.dryMass + maxJumpFuel - (ship.reserveFuelCapacity || 0), maxJumpFuel),
    unladenRange: distance(ship.dryMass + ship.fuelCapacity, maxJumpFuel),
    ladenRange: distance(ship.ladenMass, maxJumpFuel),
    totalRange: fsdOnline && jumpFuel > 0 ? finite(Calc.totalJumpRange(mass, fsd, fuel, ship)) : 0,
    powerAvailable: ship.powerAvailable, powerRetracted: ship.powerRetracted, powerDeployed: ship.powerDeployed,
    powerPriorities: ship.priorityBands.map((p, index) => ({ ...p, priority: index + 1 })),
    shield, armour: ship.armour, hardness: ship.hardness, shieldCells: ship.shieldCells,
    shieldResistances: shield ? metricsByType(shields, shield, 'base', true) : {},
    shieldEffective: shield ? metricsByType(shields, shield, 'total', false) : {},
    armourResistances: metricsByType(armour, armour.total, 'total', true),
    armourEffective: metricsByType(armour, armour.total, 'total', false),
    moduleArmour: armour.modulearmour, moduleProtection: armour.moduleprotection,
    shieldRecoveryTime: shield && distOnline ? finite(shields.recover) : null,
    shieldRegenTime: shield && distOnline ? finite(shields.recharge) : null,
    distributor: {
      systemsCapacity: pd.getSystemsCapacity(), enginesCapacity: pd.getEnginesCapacity(), weaponsCapacity: pd.getWeaponsCapacity(),
      systemsRecharge: pd.getSystemsRechargeRate(), enginesRecharge: pd.getEnginesRechargeRate(), weaponsRecharge: pd.getWeaponsRechargeRate(),
    },
    thermalEfficiency: finite(pp.getThermalEfficiency()), heatCapacity: finite(ship.heatCapacity), heatDissipation: finite(ship.heatDissipation),
    dps: combat.totalDps, sustainedDps: combat.totalSDps, eps: combat.totalEps, hps: combat.totalHps,
    weaponDrainTime: distOnline ? finite(Calc.timeToDrainWep(combat, wep)) : null,
    cost: ship.totalCost, rebuy: ship.totalCost * 0.05,
  };
}

function journalModule(ship, entry, observed) {
  const symbol = itemSymbol(observed.Item);
  const candidates = templates(ship, entry);
  const named = candidates.filter(template => symbol && itemSymbol(template.symbol) === symbol);
  const observedID = Number(observed.ModuleID ?? observed.edId);
  // The size 5/6 CG rewards share ordinary cargo-rack item and item IDs,
  // but are the only articles that ever carry this fixed blueprint.
  const reward = lower(observed.Engineering?.BlueprintName) === lower(rewardRackBlueprint) &&
    Number(observed.Engineering?.Level) === 5
    ? named.find(template => template.preEngineered?.blueprints?.includes(rewardRackBlueprint)) : null;
  return reward
    || named.find(template => observedID > 0 && observedID === template.edID)
    || named.find(template => !template.preEngineered)
    || named[0]
    || (entry.kind === 'armour' && /_armour_(grade1(?:_default)?|grade2|grade3|mirrored|reactive)$/.test(symbol)
      ? ship.availCS.bulkheads[/(grade1_default|grade1|grade2|grade3|mirrored|reactive)$/.exec(symbol)[1] === 'grade1_default' ? 0
        : ['grade1', 'grade2', 'grade3', 'mirrored', 'reactive'].indexOf(/(grade1|grade2|grade3|mirrored|reactive)$/.exec(symbol)[1]) + (ship.availCS.bulkheads.length > 5 ? 1 : 0)]
      : null);
}

function journalKey(value) {
  return lower(value).replace(/_size\d+$/, '').replace(/(hardpoint|slot|military|cargo|limpetcontroller|fighterbay)0+(\d+)$/, '$1$2');
}

function applyJournalFireRate(module, perSecond) {
  if (!(perSecond > 0)) return;
  const baseInterval = 1 / module.get('rof', false);
  const burst = module.get('burst') || 1;
  const burstRate = module.get('burstrof') || 1;
  const charge = module.get('charge') || 0;
  const interval = burst / perSecond - (burst - 1) / burstRate - charge;
  // The journal gives shots per second, while Coriolis stores a modifier of
  // the time between bursts. Applying the ratio as-is reverses the change.
  if (Number.isFinite(baseInterval) && baseInterval > 0 && interval > 0) {
    module.setModValue('rof', (interval / baseInterval - 1) * 10000, true);
  }
}

function importEngineering(module, engineering) {
  if (!module || !engineering || typeof engineering !== 'object') return;
  const name = lower(engineering.BlueprintName);
  const grade = Number(engineering.Level);
  const blueprint = blueprintEntries(module).find(entry =>
    entry.grades.includes(grade) && (lower(entry.symbol) === name || lower(entry.id) === name));
  const experimental = itemSymbol(engineering.ExperimentalEffect);
  const special = own(Modifications.specials, experimental) || experimental === 'special_plasma_slug' ? experimental : null;
  const quality = clamp(engineering.Quality, 1, 1);
  const modifiers = Array.isArray(engineering.Modifiers) ? engineering.Modifiers.slice(0, 100).filter(mod =>
    mod && typeof mod.Label === 'string' && Number.isFinite(mod.Value) && Number.isFinite(mod.OriginalValue) &&
    (mod.Value !== 0 || mod.OriginalValue !== 0)) : [];
  const isRate = mod => /^(outfittingfieldtype_)?rateoffire$/i.test(mod.Label);
  if (modifiers.length || quality > 0) {
    _addModifications(module, modifiers.length ? modifiers.filter(mod => !isRate(mod)) : null, quality, blueprint?.id || null, grade, special);
    applyJournalFireRate(module, modifiers.find(isRate)?.Value);
  } else if (blueprint) {
    // Quality zero still has the recipe's starting values. The shared journal
    // converter treats zero as missing when there are no explicit modifiers.
    module.blueprint = { ...Modifications.blueprints[blueprint.id], grade, special: Modifications.specials[special] };
    setQualityCB(module.blueprint, 0, (feature, value) => module.setModValue(feature, value, false));
  }
}

export function fromLoadout(loadout) {
  if (!loadout || !Array.isArray(loadout.Modules) || loadout.Modules.length > 120) fail('В файле нет данных оснащения корабля.');
  const shipId = shipIds.get(normalShipName(loadout.Ship));
  if (!shipId) fail('Неизвестная модель корабля.');
  const ship = makeShip(shipId).buildWith(Ships[shipId].defaults);
  const entries = descriptors(ship);
  const unknown = [];
  const fitted = new Set();
  const cosmetics = /^(?:paintjob|shipname[0-9]+|shipid[0-9]+|decal[0-9]+|bobble[0-9]+|shipkit.+|weaponcolour|enginecolour|vesselvoice|hologram|shipcockpit|stringlights)$/i;
  for (const { kind, slot, required } of entries) {
    if (!required && kind !== 'hatch') ship.use(slot, null, true);
  }
  for (const observed of loadout.Modules) {
    if (!observed || typeof observed.Slot !== 'string' || typeof observed.Item !== 'string') fail('В записи оснащения есть неверный модуль.');
    if (cosmetics.test(observed.Slot)) continue;
    const observedKey = lower(observed.Slot);
    const sameNumber = entries.filter(candidate => journalKey(candidate.key) === journalKey(observed.Slot));
    const entry = entries.find(candidate => lower(candidate.key) === observedKey) || (sameNumber.length === 1 ? sameNumber[0] : null);
    if (!entry) {
      unknown.push(observed.Item);
      continue;
    }
    if (fitted.has(entry.key)) fail('Один отсек указан в сборке несколько раз.');
    fitted.add(entry.key);
    const { slot, kind } = entry;
    if (kind !== 'hatch') {
      const template = journalModule(ship, entry, observed);
      if (!template) {
        unknown.push(observed.Item);
        continue;
      }
      if (kind === 'armour') ship.useBulkhead(template.index, true);
      else ship.use(slot, new Module({ template }), true);
    }
    slot.enabled = observed.On !== false && observed.On !== 0;
    slot.priority = Number.isInteger(observed.Priority) ? clamp(observed.Priority, 4, 0) : 0;
    importEngineering(slot.m, observed.Engineering);
  }
  if (unknown.length) fail('Не найдены модули или отсеки: ' + unknown.slice(0, 5).join(', '));
  if (loadout.event === 'Loadout' && entries.some(entry =>
    (entry.kind === 'core' || entry.kind === 'armour') && !fitted.has(entry.key))) {
    fail('В журнале отсутствуют обязательные модули корабля.');
  }
  const fit = toFit(ship.updatePowerPrioritesString().updatePowerEnabledString().updateModificationsString(), loadout.ShipName);
  return toFit(restore(fit), fit.name);
}

function fromCoriolisCode({ shipId, name, code }) {
  const fit = { version: 1, shipId, name, code };
  const ship = restore(fit);
  const control = code.split('.')[4];
  if (!control) return toFit(ship, name);
  const values = LZString.decompressFromBase64(fromUrlSafe(control))?.split('/') || [];
  const fields = { sys: 0, eng: 1, wep: 2, fuel: 4, cargo: 5 };
  const options = Object.fromEntries(Object.entries(fields).map(([field, index]) => [field, Number.parseFloat(values[index])]));
  [options.eng, options.sys, options.wep] = readPips(options);
  return toFit(ship, name, options);
}

function coriolisCode(fit, ship) {
  if (!settings.some(field => Number.isFinite(fit[field]))) return ship.toString();
  const [eng, sys, wep] = readPips(fit);
  const fuel = clamp(fit.fuel, ship.fuelCapacity, ship.fuelCapacity);
  const cargo = clamp(fit.cargo, ship.cargoCapacity, 0);
  return ship.toString() + '.' + LZString.compressToBase64([sys, eng, wep, 0, fuel, cargo, 'eagle', '', 1000, 0, 0, 0].join('/'));
}

function parseURL(text, name) {
  let url;
  try { url = new URL(text); } catch { return null; }
  if (url.protocol !== 'https:' || !['coriolis.io', 'www.coriolis.io'].includes(url.hostname)) return null;
  const path = /^\/outfit\/([a-z0-9_]+)\/?$/.exec(url.pathname);
  const code = url.searchParams.get('code');
  if (!path || !code) fail('В ссылке Coriolis нет полной сборки.');
  return fromCoriolisCode({ shipId: path[1], name: name || url.searchParams.get('bn'), code });
}

function parseRecord(value) {
  if (!value || typeof value !== 'object') fail('Файл не содержит сборку корабля.');
  if (value.build && typeof value.build === 'object') return parseRecord(value.build);
  if (value.version === 1 && value.shipId && value.code) return toFit(restore(value), value.name, value);
  const reference = value.references?.find?.(item => typeof item?.code === 'string');
  if (reference) {
    const shipId = reference.shipId || shipIds.get(normalShipName(value.ship));
    return fromCoriolisCode({ shipId, name: value.name, code: reference.code });
  }
  if (value.data?.Modules && value.header?.appName !== 'ED Copilot') return fromLoadout(value.data);
  if (typeof value.header?.appURL === 'string' && ['Coriolis', 'ED Copilot'].includes(value.header.appName)) {
    const fit = parseURL(value.header.appURL);
    if (fit) return fit;
  }
  if (value.data?.Modules) return fromLoadout(value.data);
  if (value.Modules) return fromLoadout(value);
  fail('Поддерживаются JSON ED Copilot, Coriolis, SLEF и событие Loadout из журнала.');
}

export function parse(text) {
  if (typeof text !== 'string' || text.length > 2_000_000) fail('Файл сборки слишком большой.');
  const input = text.trim();
  if (/^https:\/\//i.test(input)) {
    const fit = parseURL(input);
    if (fit) return [fit];
    fail('Вставьте полную ссылку на сборку Coriolis.');
  }
  let payload;
  try { payload = JSON.parse(input); } catch { fail('Не удалось прочитать JSON сборки.'); }
  if (payload?.builds && !Array.isArray(payload.builds) && typeof payload.builds === 'object') {
    const records = Object.entries(payload.builds).flatMap(([shipId, builds]) =>
      Object.entries(builds || {}).map(([name, code]) => ({ shipId, name, code })));
    if (!records.length || records.length > 1000) fail('В файле должно быть от 1 до 1000 сборок.');
    return records.map(fromCoriolisCode);
  }
  const records = Array.isArray(payload) ? payload : Array.isArray(payload?.fits) ? payload.fits : [payload];
  if (!records.length || records.length > 1000) fail('В файле должно быть от 1 до 1000 сборок.');
  return records.map(parseRecord);
}

export function supportsCoriolis(fit) {
  const ship = restore(fit);
  return !Ships[ship.id].supplemental && !descriptors(ship).some(({ slot }) => slot.m?.supplemental);
}

export function toCoriolisURL(fit) {
  if (!supportsCoriolis(fit)) fail('В Coriolis пока нет корабля или модуля из этой сборки.');
  const ship = restore(fit);
  const params = new URLSearchParams({ code: coriolisCode(fit, ship), bn: validName(fit.name, ship.name) });
  return 'https://coriolis.io/outfit/' + ship.id + '?' + params;
}

const journalStats = [
  ['mass', 'Mass'], ['integrity', 'Integrity'], ['power', 'PowerDraw'],
  ['pgen', 'PowerCapacity'], ['eff', 'HeatEfficiency'], ['boot', 'BootTime'],
  ['maxfuel', 'MaxFuelPerJump'], ['engcap', 'EnginesCapacity'], ['engrate', 'EnginesRecharge'],
  ['syscap', 'SystemsCapacity'], ['sysrate', 'SystemsRecharge'], ['wepcap', 'WeaponsCapacity'],
  ['weprate', 'WeaponsRecharge'], ['damage', 'Damage'], ['distdraw', 'DistributorDraw'],
  ['ammo', 'AmmoMaximum'], ['clip', 'AmmoClipSize'], ['reload', 'ReloadTime'],
  ['range', 'Range'], ['piercing', 'ArmourPenetration'], ['shotspeed', 'ShotSpeed'], ['jitter', 'Jitter'],
  ['regen', 'RegenRate'], ['brokenregen', 'BrokenRegenRate'], ['thermload', 'ThermalLoad'],
  ['shieldreinforcement', 'ShieldBankReinforcement'], ['duration', 'ShieldBankDuration'],
  ['spinup', 'ShieldBankSpinUp'], ['hullreinforcement', 'DefenceModifierHealthAddition'],
  ['ranget', 'FSDInterdictorRange'], ['facinglimit', 'FSDInterdictorFacingLimit'],
  ['scanrate', 'DSS_RateMult'], ['scantime', 'ScannerTimeToScan'], ['angle', 'SensorTargetScanAngle'],
  ['cargo', 'CargoCapacity'], ['proberadius', 'DSS_PatchRadius', 100],
  ['kinres', 'KineticResistance', 100], ['thermres', 'ThermicResistance', 100],
  ['explres', 'ExplosiveResistance', 100], ['causres', 'CausticResistance', 100],
  ['shieldboost', 'DefenceModifierShieldMultiplier', 100], ['hullboost', 'DefenceModifierHealthMultiplier', 100],
];

function slefModifiers(module) {
  const attributes = journalStats.slice();
  if (module.grp === 'fsd') attributes.push(['optmass', 'FSDOptimalMass']);
  if (module.grp === 't') attributes.push(['optmass', 'EngineOptimalMass'], ['optmul', 'EngineOptPerformance']);
  if (['sg', 'psg', 'bsg'].includes(module.grp)) attributes.push(['optmass', 'ShieldGenOptimalMass'], ['optmul', 'ShieldGenStrength']);
  const output = [];
  function changed(label, value, original) {
    if (Number.isFinite(value) && Number.isFinite(original) && Math.abs(value - original) > 1e-8) {
      output.push({ Label: label, Value: Number(value.toFixed(6)), OriginalValue: Number(original.toFixed(6)) });
    }
  }
  for (const [field, label, scale = 1] of attributes) {
    const value = module.get(field);
    const original = module.get(field, false);
    if (value != null && original != null) changed(label, value * scale, original * scale);
  }
  changed('RateOfFire', module.getRoF(), module.getRoF(false));
  changed('DamageFalloffRange', module.getFalloff(), module.get('falloff', false));
  return output;
}

function exportFit(fit, format = 'native', appVersion = '1') {
  const ship = restore(fit);
  const name = validName(fit.name, ship.name);
  if (format === 'coriolis') {
    if (!supportsCoriolis(fit)) fail('В Coriolis пока нет корабля или модуля из этой сборки.');
    const build = toDetailedBuild(name, ship);
    build.references[0].url = toCoriolisURL(fit);
    build.references[0].code = coriolisCode(fit, ship);
    return JSON.stringify(build, null, 2);
  }
  if (format === 'slef') {
    const Modules = descriptors(ship).flatMap(({ slot, key, kind }) => {
      if (!slot.m || key === 'PlanetaryApproachSuite') return [];
      let symbol = slot.m.symbol;
      if (!symbol && kind === 'armour') {
        const offset = Ships[ship.id].bulkheads.length > 5 ? 1 : 0;
        const suffix = slot.m.index === 0 && offset ? 'grade1_default'
          : ['grade1', 'grade2', 'grade3', 'mirrored', 'reactive'][slot.m.index - offset];
        symbol = Ships[ship.id].frontierSymbol + '_armour_' + suffix;
      }
      if (!symbol) fail('У модуля отсутствует игровой идентификатор для экспорта.');
      const engineering = describeEngineering(slot.m);
      const modifiers = engineering ? slefModifiers(slot.m) : [];
      return [{
        Slot: key, Item: symbol, On: kind === 'armour' || Boolean(slot.enabled), Priority: slot.priority ?? 0,
        ...(slot.m.edID ? { ModuleID: slot.m.edID } : {}),
        ...(engineering ? { Engineering: {
          BlueprintName: engineering.symbol, Level: engineering.grade, Quality: engineering.quality ?? 1,
          ...(engineering.experimental ? { ExperimentalEffect: engineering.experimental } : {}),
          ...(modifiers.length ? { Modifiers: modifiers } : {}),
        } } : {}),
      }];
    });
    return JSON.stringify([{
      header: { appName: 'ED Copilot', appVersion, ...(supportsCoriolis(fit) ? { appURL: toCoriolisURL(fit) } : {}) },
      data: { Ship: Ships[ship.id].frontierSymbol || ship.id, ShipName: name, Modules },
    }], null, 2);
  }
  return JSON.stringify(toFit(ship, name, fit), null, 2);
}

globalThis.EDCopilotShipFitEngine = {
  ships, create, slots, candidates, edit, stats, blueprints, parse, export: exportFit, fromLoadout, toCoriolisURL, supportsCoriolis,
};
