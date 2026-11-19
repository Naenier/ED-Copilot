'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { readFileSync } = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

test('the nearest body pickers retain the exact concrete Spansh values', () => {
  const window = {};
  vm.runInNewContext(readFileSync(path.join(__dirname, 'nearest-options.js'), 'utf8'), {window});
  const options = window.NearestOptions;
  const ids = values => Array.from(values, item => item[0]).sort();
  const bodyTypes = [...ids(options.starTypes), ...ids(options.planetTypes)].sort();
  assert.deepEqual(bodyTypes, Array.from(options.subtype).sort());
  assert.deepEqual(ids(options.atmosphereTypes), Array.from(options.atmosphere).sort());
  assert.deepEqual(ids(options.volcanismTypes), Array.from(options.volcanism).sort());
  for (const section of [options.starTypes,options.planetTypes,options.atmosphereTypes,options.volcanismTypes]) {
    for (const [value] of section) assert.ok(options.spaceLabels[value], 'Missing Russian label for '+value);
  }
  assert.ok(ids(options.starTypes).includes('White Dwarf (DCV) Star'));
  assert.ok(ids(options.planetTypes).includes('Class V gas giant'));
});

function nearbyView(id, values, groups = {}) {
  const window = { NearestPicker: { sync() {} } };
  const context = { window };
  for (const script of ['nearest-options.js', 'nearest.js']) {
    vm.runInNewContext(readFileSync(path.join(__dirname, script), 'utf8'), context);
  }
  const requests = [];
  const events = {};
  const host = {
    querySelector() { return {}; },
    addEventListener(type, callback) { events[type] = callback; },
  };
  const state = { id, values: { origin: 'Sol', ...values }, groups, details: {}, page: 0, primed: true };
  const controller = { tab: 'nearest', nearestUI: { section: id, sessions: { [id]: state } } };
  const ui = {
    $: () => host, heading: () => '', locale: () => 'ru-RU', esc: value => String(value ?? ''),
    t: value => value, icon: () => '', fmt: String,
    call: async (op, args) => {
      requests.push([op, args]);
      return { status: 'ok', rows: [] };
    },
  };
  const html = window.NearestUI.render(controller, ui);
  window.NearestUI.init(controller, ui);
  return {
    html,
    submit() {
      events.submit({ target: { id: 'nearest-form' }, preventDefault() {} });
      assert.equal(requests[0]?.[0], 'nearest.search');
      return requests[0][1];
    },
  };
}

test('orbital installation mode sends only supported conditions and clears the regional radius', () => {
  const view = nearbyView('stations', {
    stationType: 'orbital-installations', owner: 'Keep in station form', minPad: 'L',
    maxArrivalLs: '120', sort: 'arrival', region: 'Inner Orion Spur', radiusLy: '8', security: 'High',
  }, { services: ['Repair'] });
  assert.doesNotMatch(view.html, /name="(?:owner|minPad|maxArrivalLs|add-services)"/);
  const q = view.submit();
  assert.equal(q.region, 'Inner Orion Spur');
  assert.equal(q.origin, '');
  assert.equal(q.sort, 'name');
  assert.equal(q.filters.security, 'High');
  assert.equal(q.filters.radiusLy, undefined);
  assert.equal(q.filters.services, undefined);
  assert.equal(q.filters.owner, undefined);
});

test('signal search field labels describe the entity that is searched', () => {
  const body = nearbyView('signals', { signal: 'geological', name: 'Europa' }).html;
  assert.match(body, /Название небесного тела.*name="name".*data-system-search="body"/);
  const ship = nearbyView('signals', { signal: 'megaship', name: 'Rescue' }).html;
  assert.match(ship, /Название мегакорабля.*name="name".*data-system-search="station"/);
  const beacon = nearbyView('signals', { signal: 'compromised-nav-beacon' }).html;
  assert.match(beacon, /Название сигнала или системы/);
});
