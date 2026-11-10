'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
const flush = () => new Promise(resolve => setImmediate(resolve));
function deferred() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
}
function setup(handler = () => ({ status: 'ok', rows: [] }), mode = 'Системы') {
  const calls = [], listeners = new Map();
  const host = { scrollTop: 0, addEventListener: (type, fn) => listeners.set(type, fn) };
  const sections = [{ id: 'bodies', title: 'Наблюдения журнала', paragraphs: ['Планета из журнала'] }];
  const C = { tab: 'encyclopedia', mode, state: { commander: { system: 'Lave' } } };
  const w = {};
  const context = vm.createContext({ window: w, setTimeout, clearTimeout });
  for (const file of ['galaxy.js', 'reference.js']) vm.runInContext(fs.readFileSync(path.join(__dirname, file), 'utf8'), context);
  const api = w.ReferenceUI;
  const nodes = Object.fromEntries(['#galaxy-directory-results', '#galaxy-directory-table', '#galaxy-system-panel', '#galaxy-reader-table'].map(id => [id, {}]));
  const ui = {
    esc, fmt: value => String(value), icon: () => '', btn: label => esc(label),
    $: selector => selector === '#page-scroll' ? host : nodes[selector],
    renderPage: () => { api.render(C, ui); api.init(C, ui); },
    call(op, params) {
      calls.push([op, JSON.parse(JSON.stringify(params))]);
      if (op === 'reference.list') return Promise.resolve({ entries: [], total: 0 });
      if (op === 'reference.entry') return Promise.resolve({
        entry: { id: params.id, category: 'systems', title: decodeURIComponent(params.id.slice('system/'.length)), sections },
      });
      return handler(op, params);
    },
  };
  return {
    C, api, ui, calls, nodes,
    render: () => api.render(C, ui),
    start: () => api.init(C, ui),
    click: (key, dataset) => api.handleClick('reference-galaxy-' + key, { dataset }, C, ui),
    open: id => api.open(id, C, ui),
    input: (id, value) => api.handleInput({ target: { id, value } }, C, ui),
    filter: (scope, key, value) => api.handleChange({ target: { dataset: { galaxyFilter: key, galaxyScope: scope }, value } }, C, ui),
    submit(id) {
      let prevented = false;
      listeners.get('submit')({ target: { id }, preventDefault() { prevented = true; } });
      assert.equal(prevented, true);
    },
  };
}

test('Systems is the only catalogue that loads nearby data, with one initial query and online form controls', async () => {
  const f = setup(() => ({ status: 'ok', source: 'Spansh', rows: [
    { name: 'Leesti', distance: 1.25, population: 1000, allegiance: 'Alliance' },
    { name: 'Vox <script>', distance: 2, population: 0 },
    { name: 'Unknown population', distance: 3 },
  ] }), 'Статьи');
  await f.start();
  assert.equal(f.calls.some(([op]) => op === 'galaxy.nearby'), false);
  f.api.navigate('encyclopedia', { mode: 'Системы' }, f.C);
  await f.start();
  f.start();
  await flush();
  assert.deepEqual(f.calls.filter(([op]) => op === 'galaxy.nearby'), [['galaxy.nearby', { name: 'Lave', radius: 25 }]]);
  assert.match(f.render(), /data-system-search="single"/);
  assert.match(f.render(), /<h1>Системы<\/h1>/);
  assert.match(f.render(), /Источник: Spansh/);
  assert.match(f.render(), /data-id="system\/Leesti"/);
  assert.match(f.render(), /Vox &lt;script&gt;/);
  await f.filter('directory', 'population', 'empty');
  assert.match(f.render(), /Vox &lt;script&gt;/);
  assert.doesNotMatch(f.render(), /Unknown population|Leesti/);
  assert.equal(f.calls.filter(([op]) => op === 'galaxy.nearby').length, 1);
});

test('exact submit opens a percent-encoded arbitrary system name and requests only the summary', async () => {
  const f = setup((op, args) => op === 'galaxy.nearby' ? { status: 'ok', rows: [] } :
    { status: 'ok', system: { name: args.name, population: 0, x: 0, y: 1, z: -2, permitRequired: false } });
  await f.start();
  f.input('galaxy-exact-name', "Barnard's Star / Test");
  f.submit('galaxy-exact');
  await flush();
  assert.deepEqual(f.calls.find(([op]) => op === 'reference.entry')[1], { id: "system/Barnard's%20Star%20%2F%20Test" });
  assert.deepEqual(f.calls.filter(([op]) => op === 'galaxy.system'), [['galaxy.system', { name: "Barnard's Star / Test", section: 'summary' }]]);
  assert.match(f.render(), /Координаты X \/ Y \/ Z/);
  assert.match(f.render(), /0 \/ 1 \/ -2/);
  assert.match(f.render(), /Планета из журнала/);
});

test('a reader requests one section at a time and a late body result does not replace station content', async () => {
  const pending = deferred();
  const f = setup((op, args) => args.section === 'bodies' ? pending.promise :
    args.section === 'stations' ? { status: 'ok', rows: [{ name: 'M.Gorbachev', body: 'Earth', market: true, services: ['Refuel'] }] } :
    { status: 'ok', system: { name: args.name } });
  await f.open('system/Sol');
  await flush();
  await f.click('tab', { id: 'bodies' });
  await f.click('tab', { id: 'stations' });
  await flush();
  pending.resolve({ status: 'ok', rows: [{ name: 'Earth <svg>', type: 'Planet', landable: true, materials: [{ name: 'Iron', percent: 23.46 }] }] });
  await flush();
  assert.match(f.render(), /M\.Gorbachev/);
  assert.doesNotMatch(f.render(), /Earth &lt;svg&gt;|Планета из журнала/);
  assert.doesNotMatch(f.nodes['#galaxy-system-panel'].innerHTML, /Earth &lt;svg&gt;/);
  await f.click('tab', { id: 'bodies' });
  assert.match(f.render(), /Earth &lt;svg&gt;/);
  assert.match(f.render(), /Iron <strong>23\.46%/);
  assert.equal(f.calls.filter(([op, args]) => op === 'galaxy.system' && args.section === 'bodies').length, 1);
});

test('nearby requests validate the radius and ignore an older response after the query changes', async () => {
  const first = deferred(), second = deferred();
  const f = setup((op, args) => args.name === 'Lave' ? first.promise : second.promise);
  await f.start();
  f.submit('galaxy-nearby');
  assert.equal(f.calls.filter(([op]) => op === 'galaxy.nearby').length, 1);
  f.input('galaxy-center', 'Sol');
  f.input('galaxy-radius', '101');
  f.submit('galaxy-nearby');
  assert.match(f.render(), /от 1 до 100/);
  assert.equal(f.calls.filter(([op]) => op === 'galaxy.nearby').length, 1);
  f.input('galaxy-radius', '8');
  f.submit('galaxy-nearby');
  second.resolve({ status: 'ok', rows: [{ name: 'Sirius', distance: 8 }] });
  await flush();
  first.resolve({ status: 'ok', rows: [{ name: 'Leesti', distance: 1 }] });
  await flush();
  assert.match(f.render(), /Sirius/);
  assert.doesNotMatch(f.render(), /Leesti/);
});

test('rate-limited readers stay on the current tab without retrying before Retry-After', async () => {
  const f = setup((op, args) => args.section === 'factions' ?
    { status: 'rate-limited', retryAt: '2099-12-01T20:30:00Z' } :
    { status: 'ok', system: { name: args.name } });
  await f.open('system/Sol');
  await f.click('tab', { id: 'factions' });
  await flush();
  assert.match(f.render(), /EDSM временно ограничил запросы/);
  await f.click('retry', { scope: 'reader' });
  f.start();
  assert.equal(f.calls.filter(([op, args]) => op === 'galaxy.system' && args.section === 'factions').length, 1);
});
