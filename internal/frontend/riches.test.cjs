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
  const t = (value, variables) => {
    const translated = language === 'en' ? translations[value] || value : value;
    return String(translated ?? '').replace(/\{(\w+)\}/g, (_, key) => variables?.[key] ?? '');
  };
  return {
    calls, t, esc, fmt: String, icon: () => '',
    heading: title => '<h1>' + esc(title) + '</h1>',
    btn: (label, action, _icon, _style, attributes = '') => '<button data-click="' + action + '" ' + attributes + '>' + esc(t(label)) + '</button>',
    call: async (operation, params) => calls.push({ operation, params }),
    renderPage() {}, loadView() {}, toast() {},
  };
}

const window = {};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'riches.js'), 'utf8'), { window });
const planner = window.RichesUI;

test('the planner pre-fills current system and range, with both FSS signs and no payout filter', () => {
  const C = { tab: 'riches', state: { commander: { system: 'Brestla' }, riches: { suggestedJumpRangeLy: 78.81 } } };
  const page = planner.render(C, ui('en'));
  assert.match(page, /<h1>Road to Riches<\/h1>/);
  assert.match(page, /data-system-search="single" value="Brestla"/);
  assert.match(page, /name="jumpRangeLy"[^>]+value="78\.81"/);
  assert.match(page, /class="riches-earth".*Earth-like world/);
  assert.match(page, /class="riches-water".*Water world/);
  assert.doesNotMatch(page, /minimum|mappingValue/i);
});

test('the route retains body designations, rewards and separate travelled/remaining jumps', () => {
  const earth = { name: 'Example & Sol A 2', designation: 'A 2', type: 'earthLike', distanceLs: 242, scanCredits: 265842, mappingCredits: 1151985 };
  const water = { name: 'Example & Sol A 3', designation: 'A 3', type: 'waterWorld', distanceLs: 315, scanCredits: 100000, mappingCredits: 420000, mapped: true };
  const C = { tab: 'riches', state: { riches: {
    route: { origin: 'Brestla', stops: [{ system: 'Example & Sol', estimatedJumps: 2, bodies: [earth, water] }], truncated: true },
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

test('submits localized decimals to the planner and lets the native app copy the next waypoint', async () => {
  const u = ui(), C = { tab: 'riches', state: {} };
  const values = { origin: ' Brestla ', jumpRangeLy: '78,81', radiusLy: '1000' };
  const form = { elements: { namedItem: key => ({ value: values[key] }) } };
  await planner.submit(form, C, u);
  await planner.handleClick('riches-copy-next', {}, C, u);
  assert.deepEqual(JSON.parse(JSON.stringify(u.calls)), [
    { operation: 'riches.plan', params: { values: { origin: 'Brestla', jumpRangeLy: 78.81, radiusLy: 1000 } } },
    { operation: 'riches.copy' },
  ]);
  values.jumpRangeLy = '501';
  await assert.rejects(planner.submit(form, C, u), /от 0.1 до 500/);
});
