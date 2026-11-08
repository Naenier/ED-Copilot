'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

function fixture(kind = 'fleet') {
  const listeners = new Map();
  const calls = [];
  const options = { jumpsPerWeek: 0, reserveWeeks: 12, cargoTons: 0, bankBalance: 0, purchaseBudget: null, tritiumPerJump: null, tritiumPrice: null };
  const summary = { name: kind === 'fleet' ? 'Личный флотоносец' : 'Флотоносец эскадрильи', totalBuyPrice: kind === 'fleet' ? 5000000000 : 25000000000, hullBuyPrice: kind === 'fleet' ? 5000000000 : 25000000000, baseWeeklyUpkeep: 5000000, weeklyUpkeep: 5000000, availableCargoTons: 25000, reserveWeeks: 12, minimumBankReserve: 60000000, recommendedBankReserve: null, bankWholeWeeksWithoutFuel: 0, weeklyCostWithFuel: null, monthlyUpkeep: 21428571, yearlyUpkeep: 260714286, requiredBudget: null, overCapacityTons: 0 };
  const C = { mode: 'Сводка', state: { carrierCalculator: { selectedType: kind, name: summary.name, options, summary, services: [{ key: 'refuel', name: 'Заправка', state: 'off', mandatory: false, buyPrice: 0, weeklyUpkeep: 0, capacityTons: 0, installationPrice: 40000000, activeWeeklyUpkeep: 1500000, suspendedWeeklyUpkeep: 750000 }, { key: 'repair', name: 'Ремонт', state: 'active', mandatory: false, buyPrice: 50000000, weeklyUpkeep: 1500000, capacityTons: 180 }, ...(kind === 'squadron' ? [{ key: 'shipyard', name: 'Верфь', state: 'active', mandatory: true, buyPrice: 0, weeklyUpkeep: 0, capacityTons: 3000 }] : [])], comparison: [{ carrierType: 'fleet', hullBuyPrice: 5000000000, capacityTons: 25000 }, { carrierType: 'squadron', hullBuyPrice: 25000000000, capacityTons: 60000 }], settings: { plans: { fleet: options, squadron: options } }, notes: ['Учебное допущение'], source: { url: 'https://pilotstradenetwork.com/tools/carrier-calculator/', checkedAt: '2026-11-08' } } } };
  const ctx = { window: {}, document: { addEventListener: (type, listener) => listeners.set(type, listener) }, Intl };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'calculator.js'), 'utf8'), ctx);
  const elements = [];
  const box = { textContent: '', classList: { remove() {}, add() {} } };
  const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const ui = {
    esc, icon: () => '<svg></svg>', fmt: value => value == null || value === '' ? '—' : new Intl.NumberFormat('ru-RU').format(Number(value)), money: value => value == null ? '—' : `${value} кр.`,
    btn: (label, action, ico, style, attrs = '') => `<button data-click="${action}" ${attrs}>${esc(label)}</button>`, badge: text => `<span>${esc(text)}</span>`, heading: (title, description, extra) => `<h1>${title}</h1><p>${description}</p>${extra || ''}`,
    call: async (op, params) => { calls.push({ op, params: JSON.parse(JSON.stringify(params)) }); if (op === 'calculator' && params.values.action === 'select') { C.state.carrierCalculator.selectedType = params.values.carrierType; C.state.carrierCalculator.options = C.state.carrierCalculator.settings.plans[params.values.carrierType]; } return { ok: true }; },
    toast() {}, loadView: async () => {}, navigate: async () => {}, renderPage() {}, dialog() {}, closeLayer() {}, $: () => box, $$: () => elements,
  };
  return { api: ctx.window.CalculatorUI, C, ui, calls, listeners, elements, box };
}

test('renders both carriers, nullable figures and built-in shipyard without editable control', () => {
  const { api, C, ui } = fixture('squadron');
  const html = api.render(C, ui);
  assert.match(html, /data-id="fleet"/);
  assert.match(html, /data-id="squadron"/);
  assert.equal((html.match(/data-calc-input=/g) || []).length, 7);
  assert.match(html, /data-calc-input="bankBalance"[^>]*value="0"/);
  assert.match(html, /data-calc-input="tritiumPrice"[^>]*value=""/);
  assert.match(html, /Включена в стоимость корпуса/);
  assert.doesNotMatch(html, /data-calc-service="shipyard"/);
  assert.doesNotMatch(html, /NaN|undefined/);
  C.mode = 'Сравнение';
  assert.match(api.render(C, ui), /Сравнение флотоносцев/);
});

test('escapes text coming from backend data', () => {
  const { api, C, ui } = fixture();
  C.state.carrierCalculator.services[0].name = '<img src=x onerror=alert(1)>';
  C.state.carrierCalculator.notes = ['<script>alert(1)</script>'];
  const html = api.render(C, ui);
  assert.match(html, /&lt;img/);
  assert.match(html, /&lt;script&gt;/);
  assert.doesNotMatch(html, /<img|<script/);
});

test('parses Russian number entry and keeps optional blanks unknown', async () => {
  const { api, C, ui, calls, elements } = fixture();
  for (const [key, value] of Object.entries({ jumpsPerWeek: '4', reserveWeeks: '12', cargoTons: '1 000', bankBalance: '0', purchaseBudget: '5 500 000 000', tritiumPerJump: '12,5', tritiumPrice: '' })) elements.push({ dataset: { calcInput: key }, value });
  await api.handle('calc-save', null, C, ui);
  const payload = calls[0].params.values;
  assert.equal(payload.action, 'configure');
  assert.equal(payload.carrierType, 'fleet');
  assert.equal(payload.cargoTons, 1000);
  assert.equal(payload.bankBalance, 0);
  assert.equal(payload.purchaseBudget, 5500000000);
  assert.equal(payload.tritiumPerJump, 12.5);
  assert.equal(payload.tritiumPrice, null);
});

test('invalid input does not reach the Go bridge', async () => {
  const { api, C, ui, calls, elements, box } = fixture();
  elements.push({ dataset: { calcInput: 'jumpsPerWeek' }, value: '1,5' });
  await assert.rejects(api.handle('calc-save', null, C, ui), /целое число/);
  assert.equal(calls.length, 0);
  assert.match(box.textContent, /Прыжков/);
});

test('service mutation is explicit and suspension does not install absent services', async () => {
  const { api, C, ui, calls } = fixture('squadron');
  await assert.rejects(api.change('shipyard', 'off', C, ui), /нельзя изменить/);
  await assert.rejects(api.change('missing', 'active', C, ui), /нельзя изменить/);
  assert.equal(calls.length, 0);
  await api.change('refuel', 'active', C, ui);
  assert.deepEqual(calls[0], { op: 'calculator', params: { values: { action: 'service', carrierType: 'squadron', serviceKey: 'refuel', state: 'active' } } });
  await api.handle('calc-suspend-installed', null, C, ui);
  assert.deepEqual(calls[1].params.values.services, { repair: 'suspended' });
});

test('preserves dirty per-type numeric drafts across carrier switches', async () => {
  const { api, C, ui, listeners } = fixture();
  api.render(C, ui);
  api.init(C, ui);
  const input = { dataset: { calcInput: 'jumpsPerWeek' }, value: '9', closest() { return this; } };
  listeners.get('input')({ target: input });
  assert.equal(C.calculatorDrafts.fleet.dirty, true);
  await api.handle('calc-type', { dataset: { id: 'squadron' } }, C, ui);
  api.render(C, ui);
  await api.handle('calc-type', { dataset: { id: 'fleet' } }, C, ui);
  assert.match(api.render(C, ui), /data-calc-input="jumpsPerWeek"[^>]*value="9"/);
  assert.match(api.render(C, ui), /Есть несохранённые параметры/);
});

test('component smoke remains valid after the comparison screen', async () => {
  const { api, C, ui } = fixture('squadron');
  C.mode = 'Сравнение';
  const report = await api.smoke(C, ui);
  assert.equal(report.length, 4);
  assert.ok(report.every(item => item.ok));
  assert.equal(C.mode, 'Сравнение');
});
