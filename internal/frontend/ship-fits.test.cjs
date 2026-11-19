const assert = require('node:assert/strict');
const { test } = require('node:test');

require('./ship-fit-engine.js');
globalThis.window = { EDCopilotShipFitEngine: globalThis.EDCopilotShipFitEngine };
require('./ship-fits.js');

const fitter = window.EDCopilotShipFitEngine;
const workspace = window.EDCopilotShipFits;
const next = () => new Promise(resolve => setImmediate(resolve));
function element() {
  return { handlers:{}, addEventListener(name,fn) { this.handlers[name]=fn; } };
}
function click(node, action, value, disabled=false) {
  node.handlers.click({target:{closest:()=>({disabled,dataset:{fitAction:action,fitValue:value}})}});
}
function harness(call, build=fitter.create('sidewinder')) {
  const C = {
    tab:'engineering', mode:'Конструктор кораблей',
    state:{planWorkspace:{key:'pilot'},connection:{scanning:false},shipBuildRevision:1},
  };
  const host = element(), art = element();
  let layer;
  const u = {
    t:text=>text, esc:value=>String(value ?? ''), fmt:value=>String(value),
    locale:()=> 'ru-RU', icon:()=>'', heading:()=>'', toast(){},
    $(selector) { return {'#ship-fits':host,'#fit-schematic-image':art}[selector] || (layer?.selector === selector ? layer : null); },
    dialog(title,body,footer,wide) {
      layer = Object.assign(element(),{title,innerHTML:body,wide,selector:'#'+/id="([^"]+)"/.exec(body)[1]});
    },
    closeLayer() { layer = null; },
    call: (op,input) => op === 'ship-fit-schematic' ? Promise.resolve({image:{src:'data:image/jpeg;base64,ZmFrZQ=='}}) : call(op,input),
  };
  workspace.render(C,u);
  Object.assign(C.shipFitUI,{loaded:true,build,name:'Test fitting'});
  workspace.init(C,u);
  return {C,u,host,art,layer:()=>layer};
}

test('materials are requested only in a dialog and render as three separate tables', async () => {
  const stock = fitter.create('sidewinder');
  const fit = grade => fitter.edit(stock,'FrameShiftDrive',{
    engineering:{blueprint:'FSD_LongRange',grade,quality:1},
  });
  const requests = [];
  const {C,u,host,art,layer} = harness((op,input) => {
    assert.equal(op,'ship-fit-materials');
    assert.equal(input.expectedPlanWorkspace,'pilot');
    return new Promise(resolve => requests.push({input,resolve}));
  },fit(2));
  const page = workspace.render(C,u);
  assert.match(page,/data-fit-action="materials"/);
  assert.doesNotMatch(page,/fit-materials-content|Материалы для сборки/);
  await next();
  assert.match(art.innerHTML,/data:image\/jpeg;base64/);
  assert.equal(requests.length,0);

  C.state.connection.scanning = true;
  click(host,'materials');
  assert.equal(layer().wide,true);
  assert.match(layer().innerHTML,/Читаем игровой журнал/);
  assert.equal(requests.length,0);
  C.state.connection.scanning = false;
  workspace.refreshTelemetry(C,u);
  C.shipFitUI.build = fit(3);
  workspace.refreshTelemetry(C,u);
  assert.deepEqual(requests.map(call => call.input.modules[0].grade),[2,3]);

  const material = (category,name,required,owned) => {
    const missing = owned == null ? null : Math.max(0,required-owned);
    return {category,required,missing,rows:[{name,required,owned,missing}]};
  };
  const bill = owned => ({
    groups:[
      material('raw','Углерод',7,owned),
      material('manufactured','Химические процессоры',4,4),
      material('encoded','Данные о следах',2,0),
    ],
    estimated:true,
  });
  requests[1].resolve(bill(null));
  await next();
  assert.match(layer().innerHTML,/Углерод<\/th><td>7<\/td><td>—<\/td><td>—/);
  assert.match(layer().innerHTML,/Запас материалов ещё не получен из журнала/);
  assert.equal((layer().innerHTML.match(/<table>/g) || []).length,3);
  assert.match(layer().innerHTML,/fit-material-grid.*<caption>Сырьё.*<caption>Промышленные материалы.*<caption>Данные/);
  requests[0].resolve(bill(7));
  await next();
  assert.match(layer().innerHTML,/Углерод<\/th><td>7<\/td><td>—<\/td><td>—/);

  C.state.shipBuildRevision++;
  workspace.refreshTelemetry(C,u);
  requests[2].resolve(bill(5));
  await next();
  assert.match(layer().innerHTML,/Углерод<\/th><td>7<\/td><td>5<\/td><td class="fit-material-deficit">2/);
  assert.match(layer().innerHTML,/Всего нужно <strong>13<\/strong>.*Не хватает <strong class="fit-material-deficit">4/);
  u.closeLayer();
  C.state.shipBuildRevision++;
  workspace.refreshTelemetry(C,u);
  assert.equal(requests.length,3);
});

test('the materials dialog clears inventory during replay and closes when the commander changes', async () => {
  const pending = [];
  const build = fitter.edit(fitter.create('sidewinder'),'FrameShiftDrive',{
    engineering:{blueprint:'FSD_LongRange',grade:1,quality:1},
  });
  const {C,u,host,layer} = harness((op,input) => {
    assert.equal(op,'ship-fit-materials');
    assert.equal(input.expectedPlanWorkspace,'pilot');
    return new Promise(resolve => pending.push(resolve));
  },build);
  const bill = {groups:[{category:'raw',required:1,missing:0,
    rows:[{name:'Сохранённый запас',required:1,owned:42,missing:0}]}]};
  click(host,'materials');
  pending[0](bill);
  await next();
  assert.match(layer().innerHTML,/Сохранённый запас/);

  C.state.shipBuildRevision++;
  workspace.refreshTelemetry(C,u);
  C.state.connection.scanning = true;
  workspace.refreshTelemetry(C,u);
  pending[1](bill);
  await next();
  assert.match(layer().innerHTML,/Читаем игровой журнал/);
  assert.doesNotMatch(layer().innerHTML,/Сохранённый запас/);

  C.state.connection.scanning = false;
  workspace.refreshTelemetry(C,u);
  C.state.planWorkspace = {key:'new commander'};
  workspace.refreshTelemetry(C,u);
  assert.equal(layer(),null);
  pending[2](bill);
  await next();
  assert.equal(layer(),null);
});

test('fleet import opens the selected observed ship and retains unsaved-fit confirmation', async () => {
  const stock = fitter.create('eagle');
  const loadout = {
    Ship:'Eagle',ShipName:'Тестовый Eagle',ShipID:12,
    Modules:fitter.slots(stock).filter(slot=>slot.module).map(slot=>({
      Slot:slot.key,Item:slot.kind === 'armour' ? 'Eagle_armour_grade1' : slot.module.symbol,On:slot.on,Priority:slot.priority-1,
    })),
  };
  const calls = [];
  const {C,u,host,layer} = harness((op,input) => {
    calls.push({op,input});
    if (op === 'ship-fits-fleet') return Promise.resolve({rows:[
      {shipId:'11',shipType:'Hauler',available:false},
      {shipId:'12',shipType:'Eagle',shipName:loadout.ShipName,available:true,source:'journal'},
    ]});
    assert.equal(op,'ship-fit-from-fleet');
    return Promise.resolve({loadout});
  });
  C.shipFitUI.dirty = true;
  C.state.connection.scanning = true;
  click(host,'fleet');
  assert.match(layer().innerHTML,/Читаем игровой журнал/);
  assert.equal(calls.length,0);
  C.state.connection.scanning = false;
  workspace.refreshTelemetry(C,u);
  await next();
  assert.match(layer().innerHTML,/Оснащение пока не считано/);
  assert.match(layer().innerHTML,/data-fit-value="11" disabled/);
  click(layer(),'fleet-open','12');
  assert.equal(layer(),null);
  assert.equal(calls.length,1);
  assert.match(host.innerHTML,/Есть несохранённые изменения/);
  click(host,'confirm');
  await next();
  assert.deepEqual(calls[1],{op:'ship-fit-from-fleet',input:{shipId:'12',expectedPlanWorkspace:'pilot'}});
  assert.equal(C.shipFitUI.build.shipId,'eagle',C.shipFitUI.error);
  assert.equal(C.shipFitUI.name,'Тестовый Eagle');
  assert.equal(C.shipFitUI.dirty,true);
});

test('library export preserves the database revision and stops after cancelling a volume', async () => {
  const calls = [];
  const {host} = harness((op,input) => {
    calls.push({op,input});
    if (op === 'native') return Promise.resolve({cancelled:input.name.endsWith('02.json')});
    assert.equal(op,'ship-fits-export');
    return Promise.resolve({
      fits:[{build:fitter.create(input.after ? 'eagle' : 'sidewinder')}],
      next:input.after ? 'second' : 'first', revision:'7',
    });
  });
  click(host,'export-library');
  await next();
  assert.deepEqual(calls.map(call=>call.op),['ship-fits-export','native','ship-fits-export','native']);
  assert.deepEqual(calls[2].input,{after:'first',revision:'7',expectedPlanWorkspace:'pilot'});
  assert.equal(calls[1].input.name,'ED Copilot Fittings - 01.json');
  assert.equal(JSON.parse(calls[1].input.text)[0].shipId,'sidewinder');
});

test('ready builds open the selected hull with provenance and normal draft handling', async t => {
  window.EDCopilotShipFitPresets = {version:1,builds:[
    {id:'sidewinder-intro',shipId:'sidewinder',role:'combat',title:'Первый бой',titleEn:'First engagement',
      build:fitter.create('sidewinder'),source:{kind:'adapted',title:'Coriolis',url:'https://coriolis.io/outfit/sidewinder'}},
    {id:'eagle-racing',shipId:'eagle',role:'racing',title:'Скоростной Eagle',titleEn:'Sprint Eagle',
      build:fitter.create('eagle'),source:{kind:'original',title:'ED Copilot'}},
  ]};
  t.after(() => delete window.EDCopilotShipFitPresets);
  const {C,u,host,layer} = harness(() => assert.fail('presets must be usable offline'));
  assert.match(workspace.render(C,u),/data-fit-action="presets"/);
  C.shipFitUI.dirty = true;
  click(host,'presets');
  assert.match(layer().innerHTML,/Первый бой/);
  assert.match(layer().innerHTML,/data-external="https:\/\/coriolis.io\/outfit\/sidewinder"/);
  assert.doesNotMatch(layer().innerHTML,/Скоростной Eagle/);
  click(layer(),'preset-open','sidewinder-intro');
  assert.equal(layer(),null);
  assert.match(host.innerHTML,/Есть несохранённые изменения/);
  click(host,'confirm');
  await next();
  assert.equal(C.shipFitUI.name,'Первый бой');
  assert.equal(C.shipFitUI.id,undefined);
});
