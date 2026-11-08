'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const clone = value => JSON.parse(JSON.stringify(value));
const esc = value => String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]));

function fixture(route = {}) {
  const calls = [], toasts = [], window = {};
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'route-workspace.js'), 'utf8'), { window });
  const C = { tab: 'route', state: { carrierRoute: route } };
  const u = { esc, fmt: value => String(value), icon: name => `<svg data-icon="${esc(name)}"></svg>`,
    call: async (op, values) => { calls.push({ op, values: clone(values) }); }, toast: (text, error) => toasts.push({ text, error }) };
  return { C, u, calls, toasts, api: window.RouteWorkspaceUI, render() { return window.RouteWorkspaceUI.renderSummary(C, u); } };
}
function ready() {
  return { status: 'ready', running: false, options: { tritiumPrice: 50000 }, route: { sourceSystem: 'Synthetic A', destinationSystem: 'Synthetic C', activeIndex: 2, stops: [
    { name: 'Synthetic A', visited: true, tankTons: 500, cargoTritiumTons: 200 },
    { name: 'Synthetic B', visited: false, distanceLy: 400, fuelUsedTons: 99, transferTons: 0, restockTons: 0 },
    { name: 'Synthetic C', visited: false, distanceLy: 350, fuelUsedTons: 88, transferTons: 50, restockTons: 0 },
  ] }, summary: { totalJumps: 2, completedJumps: 0, remainingJumps: 2, fuelTons: 187, totalMinutes: 37, timeRange: { minMinutes: 37, maxMinutes: 57, bounded: false }, fuelCost: null, restockTons: 0, restockStops: 0,
    startingTankTons: 500, startingCargoTritiumTons: 200, arrivalTankTons: 513, arrivalCargoTritiumTons: 0, reserveTritiumTons: 500, reserveShortfallTons: 0, estimatedArrival: '2026-11-08T12:37:00Z', estimatedArrivalLatest: '2026-11-08T12:57:00Z' }, warnings: [] };
}

test('empty and single-origin routes offer setup without fabricated zero fuel, time or cost', () => {
  for (const route of [{}, { status: 'idle', route: { stops: [{ name: 'Origin', visited: true }] }, summary: { fuelTons: 0 } }]) {
    const f = fixture(route), html = f.render();
    assert.match(html, /Подготовьте маршрут флотоносца/); assert.doesNotMatch(html, /rw-metric-value|0 кр\.|0 мин|NaN|undefined/);
    assert.equal(f.calls.length, 0);
  }
});
test('planning exposes the backend progress label and marks a preserved result as previous', () => {
  const route = ready(); route.running = true; route.status = 'resolving'; route.progress = { label: 'Проверяем системы' };
  const f = fixture(route), html = f.render();
  assert.match(html, /Рассчитываем маршрут/); assert.match(html, /Проверяем системы/); assert.match(html, /Предыдущий расчёт/);
  assert.match(html, /data-click="route-workspace-copy" disabled/);
});
test('pending without an old route is a clear status and does not display an invented result', () => {
  const html = fixture({ status: 'routing', running: true }).render();
  assert.match(html, /role="status"/); assert.doesNotMatch(html, /rw-next-facts|rw-metric-value|Подготовьте маршрут/);
});
test('errors are explicit, escaped and distinguish an old calculation from the failed request', () => {
  const route = ready(); route.status = 'error'; route.error = '<img src=x onerror=alert(1)>';
  const html = fixture(route).render();
  assert.match(html, /role="alert"/); assert.match(html, /Маршрут не рассчитан/); assert.match(html, /Ниже сохранён предыдущий расчёт/);
  assert.match(html, /&lt;img src=x onerror=alert\(1\)&gt;/); assert.doesNotMatch(html, /<img src=x/);
});
test('next jump follows visited stops, excludes the source and ignores selected-row activeIndex', () => {
  const f = fixture(ready()), html = f.render();
  assert.match(html, /<h2>Synthetic B<\/h2>/); assert.match(html, /aria-valuemax="2" aria-valuenow="0"/); assert.match(html, /Осталось 2/);
  f.C.state.carrierRoute.route.stops[1].visited = true;
  const updated = f.render(); assert.match(updated, /<h2>Synthetic C<\/h2>/); assert.match(updated, /aria-valuenow="1"/); assert.match(updated, /width:50%/);
  assert.equal(f.C.state.carrierRoute.summary.completedJumps, 0, 'view must not rewrite backend state');
});
test('completed route has no false next destination and preserves whole-route totals', async () => {
  const route = ready(); route.route.stops.forEach(stop => { stop.visited = true; });
  const f = fixture(route), html = f.render();
  assert.match(html, /Маршрут пройден/); assert.match(html, /Все точки посещены/); assert.match(html, /187 т/); assert.doesNotMatch(html, /rw-next-facts/);
  await f.api.handle('route-workspace-copy', null, f.C, f.u); assert.equal(f.calls.length, 0);
});
test('time range and arrival come only from backend estimates and are labeled as the original plan', () => {
  const f = fixture(ready()), html = f.render();
  assert.match(html, /37 мин — 57 мин/); assert.match(html, /Полный исходный расчёт/); assert.match(html, /Расчётное прибытие/); assert.match(html, /Время не пересчитывается/);
  f.C.state.carrierRoute.summary = { fuelTons: 3, timeRange: { minMinutes: null, maxMinutes: null } };
  const missing = f.render(); assert.match(missing, /Время по плану[\s\S]*?Неизвестно/); assert.doesNotMatch(missing, /37 мин|57 мин|Invalid Date|NaN/);
});
test('unknown prices remain unknown while a known zero cost is preserved', () => {
  const f = fixture(ready()); assert.match(f.render(), /Стоимость топлива[\s\S]*?Неизвестна/);
  f.C.state.carrierRoute.summary.fuelCost = 0; assert.match(f.render(), /Стоимость топлива[\s\S]*?0 кр\./);
  f.C.state.carrierRoute.summary.fuelCost = 9350000; assert.match(f.render(), /9350000 кр\./);
});

test('zero or missing configured tritium price never turns a legacy zero fuelCost into free fuel', () => {
  const f=fixture(ready()); f.C.state.carrierRoute.summary.fuelCost=0;
  for(const price of [0,null,undefined]) {f.C.state.carrierRoute.options.tritiumPrice=price;const html=f.render();assert.match(html,/Цена трития не задана/);assert.match(html,/Стоимость топлива[\s\S]*?Неизвестна/);assert.doesNotMatch(html,/Стоимость топлива[\s\S]*?rw-metric-value">0 кр\./);}
});

test('minimum-load plan is explicitly distinguished from a route retaining the actual inventory', () => {
  const f=fixture(ready()); f.C.state.carrierRoute.options.calculateStartingFuel=true;
  assert.match(f.render(),/План с минимальной начальной загрузкой/); assert.match(f.render(),/пересчитайте маршрут с фактическими остатками/);
  f.C.state.carrierRoute.options.calculateStartingFuel=false; assert.doesNotMatch(f.render(),/План с минимальной начальной загрузкой/);
});
test('tank, warehouse and reserve remain separate and a shortfall is visible', () => {
  const route = ready(); route.summary.reserveShortfallTons = 25;
  const html = fixture(route).render();
  assert.match(html, /На старте по плану/); assert.match(html, /Бак <strong>500 т/); assert.match(html, /Склад <strong>200 т/);
  assert.match(html, /По прибытии по плану/); assert.match(html, /rw-reserve-warning/); assert.match(html, /Не хватает <strong>25 т/);
});
test('copy uses the latest next destination and never an outdated button payload', async () => {
  const f = fixture(ready());
  assert.equal(await f.api.handle('route-workspace-copy', { dataset: { text: 'Incorrect old row' } }, f.C, f.u), true);
  assert.deepEqual(f.calls[0], { op: 'native', values: { action: 'copy', text: 'Synthetic B' } });
  f.C.state.carrierRoute.route.stops[1].visited = true;
  await f.api.handle('route-workspace-copy', null, f.C, f.u); assert.equal(f.calls.at(-1).values.text, 'Synthetic C');
});
test('pending or failed results cannot copy a preserved old target', async () => {
  for (const changes of [{ running: true }, { status: 'error', error: 'Failed' }]) {
    const f = fixture({ ...ready(), ...changes }); await f.api.handle('route-workspace-copy', null, f.C, f.u);
    assert.equal(f.calls.length, 0); assert.equal(f.toasts[0].error, true);
  }
});
test('system names and backend warnings are escaped and source URLs are not rendered', () => {
  const route = ready(); route.route.sourceSystem = '<script>alert(1)</script>'; route.route.stops[1].name = '" onclick="bad'; route.warnings = ['<svg onload=bad>']; route.sourceUrl = 'https://example.com/private';
  const html = fixture(route).render(); assert.match(html, /&lt;script&gt;/); assert.match(html, /&quot; onclick=&quot;bad/); assert.match(html, /&lt;svg onload=bad&gt;/);
  assert.doesNotMatch(html, /<script>|<svg onload=bad>|example\.com\/private/);
});
test('cancelled state and invalid date remain honest without adding a background poller', () => {
  const route = ready(); route.status = 'cancelled'; route.summary.estimatedArrival = 'not-a-date'; route.summary.estimatedArrivalLatest = null;
  const f = fixture(route), before = clone(f.C); f.api.init(f.C, f.u);
  assert.match(f.render(), /Расчёт остановлен/); assert.match(f.render(), /Расчётное прибытие <strong>Неизвестно/); assert.deepEqual(f.C, before); assert.equal(f.calls.length, 0);
});
test('unrelated pages and actions remain untouched', async () => {
  const f = fixture(ready()); assert.equal(await f.api.handle('route.calculate', null, f.C, f.u), false);
  f.C.tab = 'sale'; assert.equal(f.render(), ''); assert.equal(await f.api.handle('route-workspace-copy', null, f.C, f.u), false);
  assert.equal(f.calls.length, 0);
});
