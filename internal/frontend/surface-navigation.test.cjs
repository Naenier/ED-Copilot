'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const window = {};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'surface-navigation.js'), 'utf8'), {window});
const ui = window.SurfaceNavigationUI;
const localeDir = path.join(__dirname, '..', 'localization');
const catalog = Object.assign({}, ...fs.readdirSync(localeDir).filter(name => name.endsWith('_en.json'))
  .sort().map(name => JSON.parse(fs.readFileSync(path.join(localeDir, name), 'utf8'))));
const english = {
  esc: text => String(text).replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('"', '&quot;'),
  t: (text, vars) => (catalog[text] ?? text).replace(/\{(\w+)\}/g, (match, key) => vars?.[key] ?? match),
  fmt: String,
  locale: () => 'en-US',
  icon: () => '',
  heading: text => `<h1>${text}</h1>`
};
const coordinates = (latitude, longitude) => ({latitude, longitude, bodyName: 'Sol A 1'});
const activeProjection = {
  status: 'active', target: coordinates(44.4825, -61.4854), current: coordinates(0, 0),
  guidance: {bearing: 0, relativeHeading: -38, distanceM: 12750}
};
const view = (surfaceNavigation = {status: 'no-target'}) =>
  ({tab: 'surface-navigation', state: {planWorkspace: {key: 'pilot-1'}, surfaceNavigation}});

test('surface navigation shows precise coordinates and no guidance for a different body', () => {
  const C = view(activeProjection);
  const markup = ui.render(C, english);
  assert.match(markup, /<h1>Surface coordinates<\/h1>/);
  assert.match(markup, /44\.4825 \/ -61\.4854/);
  assert.match(markup, /0\.0000 \/ 0\.0000/);
  assert.match(markup, /<dd>0°<\/dd>/);
  assert.match(markup, /Turn 38° left/);
  assert.match(markup, /12\.75 km/);
  C.state.surfaceNavigation = {...activeProjection, status: 'wrong-body'};
  const wrongBody = ui.render(C, english);
  assert.match(wrongBody, /Target is on another body/);
  assert.doesNotMatch(wrongBody, /Turn 38° left|12\.75 km/);
  C.state.surfaceNavigation = {...activeProjection, guidance: {bearing: 75}};
  assert.match(ui.render(C, english), /Body radius unknown/);
});

function mount(C, call) {
  const listeners = {};
  const input = {value: '', attributes: {}, addEventListener: (_, fn) => {listeners.input = fn;},
    setAttribute(name, value) {this.attributes[name] = value;}, removeAttribute(name) {delete this.attributes[name];},
    focus() {this.focused = true;}};
  const submit = {};
  const clear = {addEventListener: (_, fn) => {listeners.clear = fn;}};
  const error = {};
  const readings = {};
  const status = {};
  const form = {elements: {coordinates: input}, addEventListener: (_, fn) => {listeners.submit = fn;},
    querySelector(selector) {return {'[type="submit"]': submit, '[data-surface-nav-clear]': clear, '#surface-nav-error': error}[selector];}};
  const root = {dataset: {planWorkspace: C.state.planWorkspace.key}, querySelector(selector) {
    return {'[data-surface-nav-form]': form, '[data-surface-nav-clear]': clear, '[data-surface-nav-readings]': readings, '[data-surface-nav-status]': status}[selector];
  }};
  const u = {...english, call, $: selector => selector === '[data-surface-navigation]' ? root : form};
  ui.init(C, u);
  return {u, input, readings, status, clear, error, listeners, submit: () => listeners.submit({preventDefault() {}})};
}

test('pasted decimal pairs save exact signed values, invalid coordinates stay local, and clear removes the target', async () => {
  const C = view(), sent = [];
  const screen = mount(C, async (command, params) => {
    sent.push({...params, command});
    return {surfaceNavigation: params.action === 'set' ? {...activeProjection, target: coordinates(params.latitude, params.longitude)} : {status: 'no-target'}};
  });
  for (const invalid of ['91 / 0', '0 / -180.01', '45', '1e2 / 0', 'Infinity / 45', '1,2,3']) {
    screen.input.value = invalid;
    await screen.submit();
    assert.equal(screen.input.attributes['aria-invalid'], 'true');
    assert.match(screen.error.textContent, /Enter a latitude/);
  }
  assert.equal(sent.length, 0);
  for (const pasted of ['44,4825 / −61,4854', '44.4825, -61.4854', '+44.4825 -61.4854']) {
    screen.input.value = pasted;
    await screen.submit();
    assert.deepEqual(sent.at(-1), {command: 'surface-navigation.update', action: 'set', latitude: 44.4825, longitude: -61.4854, expectedPlanWorkspace: 'pilot-1'});
    assert.equal(screen.error.hidden, true);
  }
  assert.equal(screen.clear.hidden, false);
  assert.match(screen.readings.innerHTML, /44\.4825 \/ -61\.4854/);
  await screen.listeners.clear();
  assert.equal(screen.input.value, '');
  assert.equal(screen.clear.hidden, true);
  assert.deepEqual(sent.at(-1), {command: 'surface-navigation.update', action: 'clear', expectedPlanWorkspace: 'pilot-1'});
});

test('telemetry only refreshes readings while the target text is being edited', () => {
  const C = view();
  C.page = {};
  const screen = mount(C, () => {});
  screen.input.value = '5,1 / −';
  screen.listeners.input();
  ui.receive(C, screen.u, {...activeProjection, revision: 1, owner: 'pilot-1'});
  assert.equal(screen.input.value, '5,1 / −');
  assert.match(screen.readings.innerHTML, /Turn 38° left/);
  assert.match(ui.render(C, english), /value="5,1 \/ −"/);
  C.state.planWorkspace.key = 'pilot-2';
  C.state.surfaceNavigation = {};
  assert.doesNotMatch(ui.render(C, english), /value="5,1 \/ −"/);
});

test('a delayed push cannot restore a target after clearing and another commander gets a clean form', async () => {
  const C = view(activeProjection);
  C.page = {};
  const screen = mount(C, async () => ({surfaceNavigation: {status: 'no-target', revision: 4, owner: 'pilot-1'}}));
  ui.receive(C, screen.u, {...activeProjection, revision: 3, owner: 'pilot-1'});
  await screen.listeners.clear();
  ui.receive(C, screen.u, {...activeProjection, revision: 3, owner: 'pilot-1'});
  assert.equal(screen.clear.hidden, true);
  assert.equal(C.state.surfaceNavigation.target, undefined);
  assert.equal(ui.latest(C, {...activeProjection, revision: 2, owner: 'pilot-1'}).revision, 4);
  screen.input.value = '11 / 22';
  screen.listeners.input();
  let renders = 0;
  screen.u.renderPage = () => {renders++;};
  ui.receive(C, screen.u, {status: 'no-target', revision: 5, owner: 'pilot-2'});
  assert.equal(renders, 1);
  assert.doesNotMatch(ui.render(C, english), /value="11 \/ 22"/);
});
