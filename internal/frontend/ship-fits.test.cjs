const assert = require('node:assert/strict');
const { test } = require('node:test');

require('./ship-fit-engine.js');
globalThis.window = { EDCopilotShipFitEngine: globalThis.EDCopilotShipFitEngine };
require('./ship-fits.js');

test('the fitting bill follows selected grades and refreshes when the material inventory changes', async () => {
  const fitter = window.EDCopilotShipFitEngine;
  const workspace = window.EDCopilotShipFits;
  const C = {
    tab: 'engineering', mode: 'Конструктор кораблей',
    state: { planWorkspace: { key: 'pilot' }, connection: { scanning: true }, shipBuildRevision: 1 },
  };
  const host = { addEventListener() {} };
  const panel = {};
  const requests = [];
  const u = {
    t: text => text, esc: value => String(value), fmt: value => String(value),
    locale: () => 'ru-RU', icon: () => '', heading: () => '',
    $: selector => selector === '#ship-fits' ? host : panel,
    call(op, input) {
      assert.equal(op, 'ship-fit-materials');
      assert.equal(input.expectedPlanWorkspace, 'pilot');
      return new Promise(resolve => requests.push({ input, resolve }));
    },
  };
  const stock = fitter.create('sidewinder');
  const fit = grade => fitter.edit(stock, 'FrameShiftDrive', {
    engineering: { blueprint: 'FSD_LongRange', grade, quality: 1 },
  });

  workspace.render(C, u);
  Object.assign(C.shipFitUI, { loaded: true, build: fit(2), name: 'Test fitting' });
  workspace.init(C, u);
  assert.equal(requests.length, 0);

  C.state.connection.scanning = false;
  workspace.init(C, u);
  C.shipFitUI.build = fit(3);
  workspace.init(C, u);
  assert.deepEqual(requests.map(call => call.input.modules[0].grade), [2, 3]);
  assert.equal(requests[1].input.modules[0].blueprint, 'FSD_LongRange');

  const material = (category, name, required, owned) => {
    const missing = owned == null ? null : Math.max(0, required-owned);
    return { category, required, missing, rows: [{ name, required, owned, missing }] };
  };
  const bill = owned => ({
    groups: [
      material('raw', 'Углерод', 7, owned),
      material('manufactured', 'Химические процессоры', 4, 4),
      material('encoded', 'Данные о следах', 2, 0),
    ],
    estimated: true,
  });
  requests[1].resolve(bill(null));
  await Promise.resolve();
  assert.match(panel.innerHTML, /Углерод<\/th><td>7<\/td><td>—<\/td><td>—/);
  assert.match(panel.innerHTML, /Запас материалов ещё не получен из журнала/);

  requests[0].resolve(bill(7));
  await Promise.resolve();
  assert.match(panel.innerHTML, /Углерод<\/th><td>7<\/td><td>—<\/td><td>—/);

  C.state.shipBuildRevision++;
  workspace.refreshMaterials(C, u);
  requests[2].resolve(bill(5));
  await Promise.resolve();
  assert.match(panel.innerHTML, /Углерод<\/th><td>7<\/td><td>5<\/td><td class="fit-material-deficit">2/);
  assert.match(panel.innerHTML, /Сырьё.*Промышленные материалы.*Данные.*Итого/);
  assert.match(panel.innerHTML, /Итого<\/th><td>13<\/td><td>9<\/td><td class="fit-material-deficit">4/);
});
