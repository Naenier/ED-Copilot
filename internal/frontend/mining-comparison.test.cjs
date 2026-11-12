const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const context = {window: {}};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'mining-comparison.js'), 'utf8'), context);
const ui = context.window.MiningComparisonUI;
const translations = require('../localization/mining_comparison_en.json');
const u = {
  esc: value => String(value).replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;'),
  t: value => translations[value] || {' кр.': ' Cr', ' т': ' t'}[value] || value,
  fmt: value => new Intl.NumberFormat('en-US').format(value),
  date: value => value,
  heading: value => '<h1>' + value + '</h1>',
  renderModes: () => ''
};

const run = (id, extra = {}) => ({
  id, startedAt:'2026-11-13T12:00:00Z', system:'Synthetic', body:'Synthetic 1 A Ring',
  ship:'Python', method:'laser', methodEvidence:'loadout', refinedTons:80,
  materialUnits:0, activeSeconds:1200, tripSeconds:2400, remainingTons:30,
  pricedRemainingTons:30, estimatedRemainingValue:900000, confirmedRevenue:1400000, soldTons:50,
  prospectScans:10, composition:[{key:'platinum',name:'Platinum',occurrencePercent:20,averagePercent:25,averageAllScansPercent:5}],
  ...extra
});

test('mining comparison displays confirmed sales, remaining estimates and prospect denominators separately', () => {
  const C = {tab:'mining',mode:'Сравнение заходов',state:{mining:{current:run('one'),history:[run('two')]}}};
  assert.equal(ui.active(C), true);
  const markup = ui.render(C,u);
  assert.match(markup, /Confirmed revenue<\/th><td>1,400,000 Cr/);
  assert.match(markup, /Estimated remaining value<\/th><td>≈ 900,000 Cr/);
  assert.match(markup, /Hit rate<\/th><td>20%/);
  assert.match(markup, /Average concentration in measured hits<\/th><td>25%/);
  assert.match(markup, /Average concentration across all scans<\/th><td>5%/);
  assert.match(markup, /Based on equipment/);
  assert.equal([...C.miningCompare.ids].join(','), 'one,two');
});

test('comparison limits selection and preserves unresolved sale provenance', () => {
  const rows = ['one','two','three','four','five'].map(id => run(id,{confirmedRevenue:undefined,originUncertain:true,soldTons:0}));
  const C = {tab:'mining',mode:'Сравнение заходов',miningCompare:{owner:'profile',ids:rows.slice(0,4).map(row=>row.id)},
    state:{planWorkspace:{key:'profile'},mining:{history:rows}}};
  const markup = ui.render(C,u);
  assert.match(markup, /data-mining-compare-run="five" disabled/);
  assert.match(markup, /Confirmed revenue<\/th><td>—/);
  assert.match(markup, /Some cargo has an unknown origin or price/);
  C.state.planWorkspace.key='new-profile';
  ui.render(C,u);
  assert.equal([...C.miningCompare.ids].join(','), 'one,two');
});
