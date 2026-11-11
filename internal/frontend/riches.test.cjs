'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const translations = require('../localization/riches_ui_en.json');
const esc = value => String(value ?? '').replace(/[&<>"]/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[char]);

function ui(language = 'ru') {
  const calls = [];
  const dialogs = [];
  const t = (value, variables) => {
    const translated = language === 'en' ? translations[value] || value : value;
    return String(translated ?? '').replace(/\{(\w+)\}/g, (_, key) => variables?.[key] ?? '');
  };
  return {
    calls, dialogs, t, esc, fmt: String, icon: () => '',
    heading: title => '<h1>' + esc(title) + '</h1>',
    btn: (label, action, _icon, _style, attributes = '') => '<button data-click="' + action + '" ' + attributes + '>' + esc(t(label)) + '</button>',
    call: async (operation, params) => calls.push({ operation, params }),
    dialog: (title, html) => dialogs.push({ title, html }),
    renderPage() {}, loadView() {}, toast() {},
  };
}

const window = {};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'riches.js'), 'utf8'), { window });
const planner = window.RichesUI;

test('the planner includes both system lookups, reward filters, and all three checked options', () => {
  const C = { tab: 'riches', state: { commander: { system: 'Brestla' }, riches: { suggestedJumpRangeLy: 78.81 } } };
  const page = planner.render(C, ui('en'));
  assert.match(page, /<h1>Road to Riches<\/h1>/);
  assert.match(page, /data-system-search="single" value="Brestla"/);
  assert.match(page, /name="destination"[^>]+data-system-search="single"[^>]+value=""/);
  assert.match(page, /name="jumpRangeLy"[^>]+value="78\.81"/);
  assert.match(page, /name="maxResults"[^>]+value="100"/);
  assert.match(page, /name="maxDistanceLs"[^>]+value="10000"/);
  assert.match(page, /name="minMappingValue"[^>]+value="1000000"/);
  for (const name of ['useMappingValue', 'avoidThargoids', 'loop']) {
    assert.match(page, new RegExp('type="checkbox" name="' + name + '"[^>]+ checked'));
  }
  assert.match(page, /class="riches-earth".*Earth-like world/);
  assert.match(page, /class="riches-water".*Water world/);
  assert.match(page, /Minimum DSS reward/);
  assert.match(page, /Maximum arrival distance/);
});

test('the route retains body designations, rewards and separate travelled/remaining jumps', () => {
  const earth = { name: 'Example & Sol A 2', designation: 'A 2', type: 'earthLike', distanceLs: 242, scanCredits: 265842, mappingCredits: 1151985 };
  const water = { name: 'Example & Sol A 3', designation: 'A 3', type: 'waterWorld', distanceLs: 315, scanCredits: 100000, mappingCredits: 420000, mapped: true };
  const C = { tab: 'riches', state: { riches: {
    route: { origin: 'Brestla', maxResults: 100, stops: [{ system: 'Example & Sol', estimatedJumps: 2, bodies: [earth, water] }], truncated: true },
    progress: { nextSystem: 'Example & Sol', nextTarget: earth, completedJumps: 3, remainingJumps: 2, totalStops: 1, totalMappingCredits: 1571985 },
    search: { status: 'В заданном радиусе подходящих миров не найдено' },
  } } };
  const u = ui('en');
  const page = planner.render(C, u);
  assert.match(page, /rowspan="2".*Example &amp; Sol/);
  assert.match(page, /A 2<\/span><small>Example &amp; Sol A 2/);
  assert.match(page, /265842/);
  assert.match(page, /1151985/);
  assert.match(page, /Mapped/);
  assert.match(page, /No matching worlds were found/);
  assert.match(page, /Shows up to 100/);
  const overview = planner.renderOverview(C, u);
  assert.match(overview, /Planet in the next system/);
  assert.match(overview, /3 \/ 2/);
  assert.match(overview, /1571985/);
});

test('submits every filter including explicit zero and unchecked options', async () => {
  const u = ui(), C = { tab: 'riches', state: { planWorkspace: { key: 'profile' } } };
  const values = { origin: ' Brestla ', destination: ' Sol ', jumpRangeLy: '78,81', radiusLy: '1000', maxResults: '75', maxDistanceLs: '0', minMappingValue: '0' };
  const flags = { useMappingValue: false, avoidThargoids: false, loop: false };
  const form = { elements: { namedItem: key => Object.hasOwn(flags, key) ? { checked: flags[key] } : { value: values[key] } } };
  await planner.submit(form, C, u);
  await planner.handleClick('riches-copy-next', {}, C, u);
  assert.deepEqual(JSON.parse(JSON.stringify(u.calls)), [
    { operation: 'riches.plan', params: { values: { origin: 'Brestla', destination: 'Sol', jumpRangeLy: 78.81, radiusLy: 1000, maxResults: 75, maxDistanceLs: 0, minMappingValue: 0, ...flags, expectedPlanWorkspace: 'profile' } } },
    { operation: 'riches.copy' },
  ]);
  values.jumpRangeLy = '501';
  await assert.rejects(planner.submit(form, C, u), /от 0.1 до 500/);
  values.jumpRangeLy = '80';
  values.maxResults = '70.5';
  await assert.rejects(planner.submit(form, C, u), /целое число от 1 до 2000/);
});

test('swaps the form endpoints, preserves other edits and drops the draft when changing commander', async () => {
  const C = { tab: 'riches', state: { commander: { system: 'Brestla' }, planWorkspace: { key: 'one' } } };
  const u = ui('en');
  const input = (key, values) => planner.handleInput({ target: { dataset: { richesField: key }, ...values } }, C);
  input('destination', { value: 'Sol' });
  input('avoidThargoids', { checked: false });
  await planner.handleClick('riches-swap', {}, C, u);
  let page = planner.render(C, u);
  assert.match(page, /name="origin"[^>]+value="Sol"/);
  assert.match(page, /name="destination"[^>]+value="Brestla"/);
  assert.doesNotMatch(page, /name="avoidThargoids"[^>]+checked/);
  C.state = { commander: { system: 'Colonia' }, planWorkspace: { key: 'two' } };
  page = planner.render(C, u);
  assert.match(page, /name="origin"[^>]+value="Colonia"/);
  assert.match(page, /name="destination"[^>]+value=""/);
  assert.match(page, /name="avoidThargoids"[^>]+checked/);
});

test('the itinerary displays transit stops and maps, exports both formats and records manual completion', async () => {
  const C = { tab: 'riches', state: { planWorkspace: { key: 'current' }, riches: {
    route: { origin: 'Brestla', destination: 'Sol', originCoordinates: [-1, 0, 9], maxResults: 1, useMappingValue: false, stops: [
      { system: 'Target', coordinates: [1, 0, 2], bodies: [{ name: 'Target 1', type: 'waterWorld', terraformable: true, scanCredits: 100000, mappingCredits: 1100000 }] },
      { system: 'Sol', coordinates: [0, 0, 0], transit: true, estimatedJumps: 1 },
    ] },
    progress: { totalStops: 2, completedStops: 0, nextSystem: 'Target', nextTarget: { name: 'Target 1', type: 'waterWorld', scanCredits: 100000, mappingCredits: 1100000 }, totalRewardCredits: 100000, useMappingValue: false },
  } } };
  const u = ui('en');
  let page = planner.render(C, u);
  assert.match(page, /FSS estimate 100000/);
  assert.match(page, /Based on FSS rewards/);
  assert.match(page, /Terraformable/);
  assert.match(page, /class="riches-transit">Destination system/);
  assert.match(page, /data-click="riches-complete-body"[^>]+data-body="Target 1"/);
  await planner.handleClick('riches-toggle-search', {}, C, u);
  page = planner.render(C, u);
  assert.match(page, /<form id="riches-route-form" hidden>/);
  await planner.handleClick('riches-show-route', {}, C, u);
  assert.match(u.dialogs[0].title, /Route itinerary/);
  assert.match(u.dialogs[0].html, /<polyline points="/);
  assert.match(u.dialogs[0].html, /Brestla/);
  assert.match(u.dialogs[0].html, /Sol/);
  await planner.handleClick('riches-export-csv', {}, C, u);
  await planner.handleClick('riches-export-json', {}, C, u);
  await planner.handleClick('riches-complete-body', { checked: true, dataset: { system: 'Target', body: 'Target 1' } }, C, u);
  assert.deepEqual(JSON.parse(JSON.stringify(u.calls)), [
    { operation: 'native', params: { action: 'export', kind: 'riches', json: false } },
    { operation: 'native', params: { action: 'export', kind: 'riches', json: true } },
    { operation: 'riches.complete', params: { system: 'Target', body: 'Target 1', completed: true, expectedPlanWorkspace: 'current' } },
  ]);
});

test('large mapping tables render only one page of systems', async () => {
  const stops = Array.from({ length: 51 }, (_, i) => ({ system: 'Test ' + (i + 1), bodies: [{ name: 'World ' + (i + 1), type: 'earthLike' }] }));
  const C = { tab: 'riches', state: { riches: { route: { origin: 'Sol', maxResults: 51, stops } } } };
  const u = ui('en');
  assert.doesNotMatch(planner.render(C, u), /World 51/);
  await planner.handleClick('riches-page-next', {}, C, u);
  assert.match(planner.render(C, u), /World 51/);
  assert.match(planner.render(C, u), /Page 2 of 2/);
});
