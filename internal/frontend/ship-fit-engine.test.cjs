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
    const slef = JSON.parse(fitter.export(fit, 'slef'));
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
