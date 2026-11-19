const assert = require('node:assert/strict');
const { test } = require('node:test');

require('./ship-fit-engine.js');
const fitter = globalThis.EDCopilotShipFitEngine;

function installed(fit, key) {
  return fitter.slots(fit).find(slot => slot.key === key);
}

test('all supported hulls have a valid factory fit and survive a module-by-module SLEF round trip', () => {
  const ships = fitter.ships();
  assert.equal(ships.length, 49);
  assert.ok(ships.some(ship => ship.id === 'lynx_highliner'));
  assert.ok(ships.some(ship => ship.id === 'anaconda_mk_ii'));
  for (const { id } of ships) {
    const fit = fitter.create(id);
    const stats = fitter.stats(fit);
    assert.ok(stats.mass > 0, id);
    assert.ok(stats.speed > 0, id);
    assert.ok(stats.fuelCapacity > 0, id);
    assert.ok(stats.powerAvailable > 0, id);
    assert.deepEqual(fitter.parse(fitter.export(fit)), [fit], id);
    const slef = JSON.parse(fitter.export(fit, 'slef', '2.8.20'));
    assert.equal(slef[0].header.appVersion, '2.8.20', id);
    delete slef[0].header.appURL;
    assert.equal(fitter.parse(JSON.stringify(slef))[0].code.split('.')[0], fit.code.split('.')[0], id);
  }
});

test('cargo, fuel and engine pips affect the performance of the selected fit', () => {
  const fit = fitter.create('sidewinder');
  const empty = fitter.stats(fit, { eng: 4, sys: 1, wep: 1 });
  const loaded = fitter.stats(fit, { eng: 4, sys: 1, wep: 1, cargo: empty.cargoCapacity });
  const noPips = fitter.stats(fit, { eng: 0, sys: 4, wep: 2 });
  assert.equal(loaded.mass - empty.mass, empty.cargoCapacity);
  assert.ok(loaded.speed < empty.speed);
  assert.ok(loaded.jumpRange < empty.jumpRange);
  assert.ok(noPips.speed < empty.speed);
  assert.ok(noPips.shieldEffective.kinetic > empty.shieldEffective.kinetic);
  assert.equal(fitter.stats(fit, { fuel: 0 }).jumpRange, 0);
});

test('engineering and experimental effects update performance and round-trip through Coriolis', () => {
  const key = 'FrameShiftDrive';
  const stock = fitter.create('sidewinder');
  const drive = fitter.candidates(stock, key).find(module => module.size === 2 && module.rating === 'A');
  const base = fitter.edit(stock, key, { moduleId: drive.id });
  const basic = fitter.edit(base, key, { engineering: { blueprint: 'FSD_LongRange', grade: 5, quality: 1 } });
  const fit = fitter.edit(basic, key, { engineering: { experimental: 'special_fsd_heavy' }, priority: 3 });
  assert.equal(installed(stock, key).module.rating, 'E');
  assert.equal(installed(fit, key).priority, 3);
  assert.equal(installed(fit, key).engineering.experimental, 'special_fsd_heavy');
  assert.ok(fitter.stats(basic).jumpRange > fitter.stats(base).jumpRange);
  assert.ok(fitter.stats(fit).jumpRange > fitter.stats(basic).jumpRange);
  assert.equal(fitter.parse(fitter.export(fit, 'coriolis'))[0].code, fit.code);
  assert.equal(fitter.parse(fitter.toCoriolisURL(fit))[0].code, fit.code);
  const cleared = fitter.edit(fit, key, { engineering: null });
  assert.equal(installed(cleared, key).engineering, null);
  assert.equal(fitter.stats(cleared).jumpRange, fitter.stats(base).jumpRange);
});

test('pre-engineered modules expose the bundled experimental separately from an added effect', () => {
  const fit = fitter.create('python');
  const driveKey = 'FrameShiftDrive';
  const factoryDrive = fitter.candidates(fit, driveKey).find(module => module.engineeringLocked && module.size === 5);
  const upgraded = fitter.edit(fit, driveKey, { moduleId: factoryDrive.id });
  const fitted = fitter.edit(upgraded, driveKey, { engineering: { experimental: 'special_fsd_heavy' } });
  assert.equal(installed(fitted, driveKey).module.factoryExperimental, '');
  assert.equal(installed(fitted, driveKey).engineering.experimental, 'special_fsd_heavy');

  const weaponKey = 'MediumHardpoint1';
  const factoryWeapon = fitter.candidates(fit, weaponKey).find(module => module.factoryExperimental === 'special_feedback_cascade');
  const mounted = installed(fitter.edit(fit, weaponKey, { moduleId: factoryWeapon.id }), weaponKey);
  assert.equal(mounted.module.engineeringLocked, true);
  assert.equal(mounted.module.factoryExperimental, mounted.engineering.experimental);
});

test('power priority, allowed mount class and newer modules are retained', () => {
  const hull = fitter.create('explorer_nx');
  const slot = 'Slot06_Size5';
  const available = fitter.candidates(hull, slot);
  assert.ok(available.every(module => module.size <= 5));
  const garage = available.find(module => module.symbol === 'Int_MkIILargeBuggyBay_Size4_Class3_Free');
  assert.deepEqual([garage.mass, garage.power, garage.cost], [15, 0.8, 0]);
  const fitted = fitter.edit(hull, slot, { moduleId: garage.id, on: false, priority: 4 });
  assert.equal(fitter.supportsCoriolis(fitted), false);
  const slef = JSON.parse(fitter.export(fitted, 'slef'));
  slef[0].data.Modules.push({ Slot: 'ShipName0', Item: 'nameplate_shipname_white' });
  const imported = fitter.parse(JSON.stringify(slef))[0];
  const fittedGarage = installed(imported, slot);
  assert.equal(fittedGarage.module.id, garage.id);
  assert.equal(fittedGarage.priority, 4);
  assert.equal(fittedGarage.on, false);
  const sidewinder = fitter.create('sidewinder');
  assert.throws(() => fitter.edit(sidewinder, 'Slot01_Size2', { moduleId: garage.id }), /отсеку/);
});

test('Coriolis share links and backup restore each named fit and its load scenario', () => {
  const stock = fitter.create('sidewinder');
  const fit = { ...stock, name: 'Configured', cargo: 2, fuel: 1.25, eng: 4, sys: 0.5, wep: 1.5 };
  const link = fitter.toCoriolisURL(fit);
  const code = new URL(link).searchParams.get('code');
  assert.deepEqual(fitter.parse(link), [fit]);
  assert.deepEqual(fitter.parse(fitter.export(fit, 'coriolis')), [fit]);
  const backup = { builds: { sidewinder: { Configured: code }, eagle: { Factory: fitter.create('eagle').code } } };
  const [sidewinder, eagle] = fitter.parse(JSON.stringify(backup));
  assert.deepEqual(sidewinder, fit);
  assert.equal(eagle.name, 'Factory');
});

test('operational stats follow the jump and combat power states', () => {
  const stock = fitter.create('sidewinder');
  const offlineDrive = fitter.edit(stock, 'FrameShiftDrive', { on: false });
  for (const key of ['jumpRange', 'maximumRange', 'unladenRange', 'ladenRange', 'totalRange']) {
    assert.equal(fitter.stats(offlineDrive)[key], 0, key);
  }
  const reactorOff = fitter.stats(fitter.edit(stock, 'PowerPlant', { on: false }));
  assert.deepEqual([reactorOff.powerAvailable, reactorOff.speed, reactorOff.dps, reactorOff.shield], [0, 0, 0, 0]);
  assert.equal(fitter.stats(stock, { sys: 4, eng: 1, wep: 1 }).shieldResistances.absolute, 0);
  assert.ok(fitter.stats(stock, { sys: 4, eng: 1, wep: 1 }).shieldEffective.absolute > fitter.stats(stock).shield);

  const weapon = fitter.candidates(stock, 'SmallHardpoint1').find(item => item.group === 'ggc');
  const first = fitter.edit(stock, 'SmallHardpoint1', { moduleId: weapon.id, priority: 5 });
  const overloaded = fitter.edit(first, 'SmallHardpoint2', { moduleId: weapon.id, priority: 5 });
  const stats = fitter.stats(overloaded);
  assert.ok(stats.powerDeployed > stats.powerAvailable);
  assert.ok(stats.jumpRange > 0);
  assert.deepEqual([stats.dps, stats.sustainedDps, stats.eps, stats.hps], [0, 0, 0, 0]);
  assert.equal(stats.weaponDrainTime, null);
  assert.equal(installed(overloaded, 'SmallHardpoint1').deployed, false);
  assert.equal(installed(fitter.edit(overloaded, 'SmallHardpoint2', { on: false }), 'SmallHardpoint1').retracted, null);
  assert.ok(fitter.stats(fitter.edit(overloaded, 'SmallHardpoint2', { on: false })).dps > 0);
});

test('exclusive shield generators and Guardian boosters, and the experimental-weapon limit, follow the hull rules', () => {
  const stock = fitter.create('anaconda');
  const optional = 'Slot04_Size6', second = 'Slot06_Size5';
  const regularShield = fitter.slots(stock).find(slot => slot.module?.group === 'sg').key;
  const biWeave = fitter.candidates(stock, optional).find(module => module.group === 'bsg');
  assert.throws(() => fitter.edit(stock, optional, { moduleId: biWeave.id }), /уже установлен/);
  assert.doesNotThrow(() => fitter.edit(fitter.edit(stock, regularShield, { moduleId: null }), optional, { moduleId: biWeave.id }));

  const booster = fitter.candidates(stock, optional).find(module => module.group === 'gfsb');
  assert.throws(() => fitter.edit(fitter.edit(stock, optional, { moduleId: booster.id }), second, { moduleId: booster.id }), /уже установлен/);

  const hardpoints = fitter.slots(stock).filter(slot => slot.kind === 'hardpoint').slice(0, 5);
  const gun = fitter.candidates(stock, hardpoints[0].key).find(module => module.group === 'ggc');
  const assembled = hardpoints.slice(0, 4).reduce((fit, slot) => fitter.edit(fit, slot.key, { moduleId: gun.id }), stock);
  assert.throws(() => fitter.edit(assembled, hardpoints[4].key, { moduleId: gun.id }), /лимит/);
  const stabilizer = fitter.candidates(stock, optional).find(module => module.group === 'ews' && module.size === 3);
  const stabilized = fitter.edit(assembled, optional, { moduleId: stabilizer.id });
  const five = fitter.edit(stabilized, hardpoints[4].key, { moduleId: gun.id });
  assert.throws(() => fitter.edit(five, optional, { moduleId: null }), /лимит/);
});

function loadout(fit) {
  return JSON.parse(fitter.export(fit, 'slef'))[0].data;
}

test('fleet journal imports preserve large hangars and reject incomplete and duplicate mounts', () => {
  const dto = loadout(fitter.create('type_10_defender'));
  dto.event = 'Loadout';
  dto.Modules.push({
    Slot: 'Slot06_Size4', Item: '$int_largebuggybay_size4_class3_free_name;', On: false, Priority: 3,
  });
  const hangar = installed(fitter.fromLoadout(dto), 'Slot06_Size4');
  assert.deepEqual([hangar.module.mass, hangar.module.power, hangar.module.cost, hangar.module.edId], [20, 1.2, 0, 129046182]);
  assert.deepEqual([hangar.on, hangar.priority], [false, 4]);
  const incomplete = { ...dto, Modules: dto.Modules.filter(module => module.Slot !== 'FrameShiftDrive') };
  assert.throws(() => fitter.fromLoadout(incomplete), /обязательные модули/);
  const duplicate = { ...dto, Modules: [...dto.Modules, { ...dto.Modules.at(-1), Slot: 'slot6' }] };
  assert.throws(() => fitter.fromLoadout(duplicate), /несколько раз/);
});

test('journal weapon rate, falloff, and probe radius preserve recorded modifiers through a SLEF round trip', () => {
  const stock = fitter.create('sidewinder');
  const key = 'SmallHardpoint1';
  const weapon = fitter.candidates(stock, key).find(module => module.group === 'mc' && module.mount === 'F');
  const fitted = fitter.edit(stock, key, { moduleId: weapon.id });
  const original = installed(fitted, key).module;
  const dto = loadout(fitted);
  const copiedWeapon = dto.Modules.find(module => module.Slot === key);
  const rate = original.rateOfFire * 1.2;
  const falloff = original.falloff * 0.8;
  copiedWeapon.Engineering = { BlueprintName: 'Weapon_RapidFire', Level: 2, Quality: 0.5, Modifiers: [
    { Label: 'RateOfFire', Value: rate, OriginalValue: original.rateOfFire },
    { Label: 'DamageFalloffRange', Value: falloff, OriginalValue: original.falloff },
  ] };
  dto.Modules.push({ Slot: 'Slot03_Size1', Item: 'int_detailedsurfacescanner_tiny', Engineering: {
    BlueprintName: 'Sensor_Expanded', Level: 5, Quality: 0.8, Modifiers: [
      { Label: 'PowerDraw', Value: 0, OriginalValue: 0 }, { Label: 'DSS_PatchRadius', Value: 28, OriginalValue: 20 },
    ],
  } });
  const imported = fitter.fromLoadout(dto);
  const assertModifiers = fit => {
    const module = installed(fit, key).module;
    assert.ok(Math.abs(module.rateOfFire - rate) < 0.002, module.rateOfFire);
    assert.equal(module.falloff, falloff);
    assert.ok(Math.abs(installed(fit, 'Slot03_Size1').module.probeRadius - 0.28) < 0.0001);
  };
  assertModifiers(imported);
  const exported = JSON.parse(fitter.export(imported, 'slef'));
  delete exported[0].header.appURL;
  assertModifiers(fitter.parse(JSON.stringify(exported))[0]);
  assertModifiers(fitter.parse(JSON.stringify([{
    header: { appName: 'Coriolis', appURL: fitter.toCoriolisURL(stock) }, data: dto,
  }]))[0]);
});

test('an unsupported compact component cannot disappear silently from imported builds', () => {
  const fit = fitter.create('sidewinder');
  const corrupt = { ...fit, code: fit.code.replace('.', 'zz.') };
  assert.throws(() => fitter.parse(JSON.stringify(corrupt)), /модуль или отсек/);
});

test('the cargo-rack reward remains importable without offering its fixed upgrade as a workshop recipe', () => {
  const stock = fitter.create('anaconda');
  const key = 'Slot06_Size5';
  const rack = fitter.candidates(stock, key).find(module => module.symbol === 'Int_CargoRack_Size5_Class1' && !module.preEngineered);
  const installedRack = fitter.edit(stock, key, { moduleId: rack.id });
  const blueprint = 'CargoRack_IncreasedCapacity';
  assert.equal(fitter.blueprints(installedRack, key).length, 0);
  assert.throws(() => fitter.edit(installedRack, key, { engineering: { blueprint, grade: 5, quality: 1 } }), /недоступна/);
  const dto = loadout(installedRack);
  dto.Modules.find(module => module.Slot === key).Engineering = { BlueprintName: blueprint, Level: 5, Quality: 1 };
  const reward = fitter.fromLoadout(dto);
  assert.equal(installed(reward, key).module.engineeringLocked, true);
  assert.equal(installed(reward, key).engineering.blueprint, blueprint);
  assert.ok(fitter.stats(reward).cargoCapacity > fitter.stats(installedRack).cargoCapacity);

  const legacy = fitter.parse(JSON.stringify({ ...stock,
    code: 'A0pyttFolodDsyf5------1717--------05044j-0304-----00-4F.Iw18ZUkA.Aw18ZUkA.H4sIAAAAAAAAA5P9l8XAwPCXFUhYFfAyMPz/zwAAb4HQbRMAAAA=',
  }))[0];
  assert.equal(installed(legacy, key).engineering.blueprint, blueprint);
  assert.ok(fitter.stats(legacy).cargoCapacity > fitter.stats(installedRack).cargoCapacity);
});

test('normal rail guns offer the current workshop effects and preserve a fixed legacy reward', () => {
  const stock = fitter.create('python');
  const key = 'MediumHardpoint1';
  const rail = fitter.candidates(stock, key).find(module => module.group === 'rg' && !module.preEngineered);
  const normal = fitter.edit(stock, key, { moduleId: rail.id });
  const effects = fitter.blueprints(normal, key)[0].experimentals.map(effect => effect.id);
  for (const id of ['special_feedback_cascade', 'special_plasma_slug', 'special_super_penetrator']) {
    assert.ok(effects.includes(id + '_cooled'), id);
    assert.ok(!effects.includes(id), id);
  }
  const factory = fitter.candidates(stock, key).find(module => module.factoryExperimental === 'special_feedback_cascade');
  const reward = fitter.edit(stock, key, { moduleId: factory.id });
  assert.equal(installed(reward, key).engineering.experimental, 'special_feedback_cascade');
});
