'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

const ctx = {window: {}};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'trade-statistics.js'), 'utf8'), ctx);
const {render} = ctx.window.TradeStatsUI;
const esc = text => String(text ?? '').replace(/[<>]/g, value => value === '<' ? '&lt;' : '&gt;');
const u = {
  esc,
  t: (text, values) => text.replace(/\{(\w+)\}/g, (_, key) => values[key]),
  fmt: value => value ?? '—',
  money: value => value == null ? '—' : value + ' кр.',
  heading: (title, subtitle, actions) => `<h1>${esc(title)}</h1><p>${esc(subtitle)}</p>${actions}`,
  metric: (title, value, note) => `<div>${esc(title)} · ${esc(value)} · ${esc(note)}</div>`,
  btn: (title, action) => `<button data-click="${esc(action)}">${esc(title)}</button>`,
  renderModes: () => '<nav>Statistics tabs</nav>'
};

test('trade statistics combine signed realized margin, turnover and commodity totals', () => {
  const totals = {boughtTons: 10, soldTons: 12, buyCount: 1, sellCount: 2, revenue: 600, purchaseCost: 1000, grossMargin: -500, unknownCostTons: 2};
  const html = render({state: {stats: {since: '2026-11-05T00:00:00Z'}, tradeLedger: {
    totals,
    daily: [{...totals, date: '2026-11-07'}, {date: '2026-11-06', soldTons: 1, revenue: 50, unknownCostTons: 1}],
    byResource: [{...totals, name: '<Gold>'}, {name: 'Unknown purchase basis', soldTons: 1, unknownCostTons: 1}],
    recent: [{name: 'Raw transaction only'}]
  }}}, u);
  assert.match(html, /data-click="trade-stats-ledger"/);
  assert.match(html, /trade-stats-bar grossMargin loss/);
  assert.match(html, /-500 кр\./);
  assert.match(html, /2 т продано без известной себестоимости/);
  assert.match(html, /&lt;Gold&gt;/);
  assert.match(html, /class="loss">-500 · частично/);
  assert.match(html, /<td class="">—<\/td>/);
  assert.doesNotMatch(html, /Raw transaction only|<Gold>/);
});
