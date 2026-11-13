const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'ship-engineering.js'), 'utf8');
const translationDir = path.join(__dirname, '../localization');
const english = Object.assign({}, ...fs.readdirSync(translationDir).filter(file => file.endsWith('_en.json')).map(file =>
  JSON.parse(fs.readFileSync(path.join(translationDir, file), 'utf8'))));
const modes = {engineering: 'Текущий корабль'};
const moduleRow = (id, status, fields) => ({id, slot: id, status, ...fields});
const installed = [
  moduleRow('FrameShiftDrive', 'in-progress', {moduleKey: 'fsd', item: 'int_hyperdrive_size6_class5', name: 'РСД', english: 'Frame Shift Drive', moduleClass: '6A',
    blueprintKey: 'FSD_LongRange', blueprintName: 'Увеличенная дальность', blueprintEnglish: 'Increased Range', grade: 3, quality: .42, maxGrade: 5,
    experimentalKey: 'special_fsd_heavy', experimentalName: 'Увеличенная масса', experimentalEnglish: 'Deep Charge'}),
  moduleRow('PowerPlant', 'complete', {name: 'Реактор', english: 'Power Plant', grade: 5, quality: 1, maxGrade: 5}),
  moduleRow('MainEngines', 'available', {name: 'Двигатели', english: 'Thrusters', item: 'int_engine_size6_class5', maxGrade: 5}),
  moduleRow('Slot01_Size5', 'locked', {name: 'Генератор щита', english: 'Shield Generator', maxGrade: 5}),
  moduleRow('Slot02_Size4', 'unknown', {name: 'Неизвестный модуль', english: 'Unknown module'})
];
const engineer = {key: '300100', name: 'Фелисити Фарсир', english: 'Felicity Farseer', system: 'Deciat', base: 'Farseer Inc.',
  status: 'invited', maxGrade: 5};
const material = {entryId: 'material/arsenic', name: 'Мышьяк', english: 'Arsenic', count: 8, owned: 3, missing: 5};
const detail = {
  slot: 'FrameShiftDrive', blueprintKey: 'FSD_LongRange', grade: 5, experimentalKey: 'special_fsd_heavy',
  blueprints: [
    {key: 'FSD_LongRange', name: 'Увеличенная дальность', english: 'Increased Range', maxGrade: 5, engineers: [engineer]},
    {key: 'FSD_FastBoot', name: 'Ускоренный запуск', english: 'Faster Boot', maxGrade: 5, engineers: [engineer]}
  ],
  experimentals: [{key: 'special_fsd_heavy', name: 'Увеличенная масса', english: 'Deep Charge'}],
  engineer,
  unlockSteps: [{...engineer, steps: [
    {status: 'done', text: 'Лишнее выполненное требование', english: 'Already fulfilled'},
    {status: 'pending', text: 'Передайте 1 единицу сплава.', english: 'Donate 1 unit of alloy.', current: 0, target: 1}
  ]}],
  recipe: {confidence: 'estimate', totals: [material], grades: [
    {grade: 2, status: 'done', rolls: 0},
    {grade: 4, status: 'pending', rolls: 4, materials: [{...material, count: 4}]}
  ]}
};

function fixture(read, language = 'en') {
  const C = {tab: 'engineering', mode: modes.engineering, state: {
    planWorkspace: {key: 'commander-a'}, commander: {fid: 'a', shipId: '3', ship: 'Asp Explorer'}, engineeringRoadmapRevision: 1
  }};
  const window = {}, events = {}, calls = [];
  vm.runInNewContext(source, {window});
  const host = {ownerDocument: {activeElement: null, getElementById: () => null}, contains: () => false,
    querySelector: selector => selector === '[data-ship-eng-list]' ? list : null, querySelectorAll: () => [],
    addEventListener: (name, fn) => { events[name] = fn; }};
  const list = {innerHTML: '', contains: () => false};
  const esc = value => String(value ?? '').replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;');
  const ui = {t: (key, values = {}) => (language === 'en' ? english[key] || key : key).replace(/\{(\w+)\}/g, (_, k) => values[k] ?? '{' + k + '}'),
    esc, fmt: String, locale: () => language, heading: heading => '<h1>' + esc(heading) + '</h1>',
    icon: () => '', $: () => host, $$: () => [], call: (op, body) => { calls.push([op, {...body}]); return read(body, op); }};
  const api = window.ShipEngineeringUI;
  return {C, api, ui, calls, host, list,
    draw: () => api.render(C, ui),
    initialize: () => { api.init(C, ui); return C.shipEngineering.read; },
    click: dataset => { events.click({target: {closest: () => ({dataset})}}); return dataset.shipEngAction === 'upsert' ||
      dataset.shipEngAction === 'remove' ? C.shipEngineering.write : C.shipEngineering.read; },
    change: (field, value) => { events.change({target: {dataset: {shipEngTarget: field}, value}}); return C.shipEngineering.read; },
    input: value => { events.input({target: {id: 'ship-eng-search', value}}); }};
}

const loadout = {loadoutKnown: true, ship: 'Asp Explorer', shipName: 'Scout <One>', modules: installed};
const view = input => Promise.resolve({...loadout, detail: input.slot ? detail : undefined});

test('current ship separates the five states, shows installed progress and can filter modules locally', async () => {
  const h = fixture(view);
  await h.initialize();
  assert.match(h.draw(), /<h1>Current ship<\/h1>/);
  assert.match(h.host.innerHTML, /Scout &lt;One>/);
  assert.match(h.host.innerHTML, /6A Frame Shift Drive/);
  assert.match(h.host.innerHTML, /class="ship-eng-status in-progress">In progress/);
  assert.match(h.host.innerHTML, /class="ship-eng-status complete">Fully upgraded/);
  assert.match(h.host.innerHTML, /class="ship-eng-status available">Ready to start/);
  assert.match(h.host.innerHTML, /class="ship-eng-status locked">Engineer access required/);
  assert.match(h.host.innerHTML, /class="ship-eng-status unknown">Unknown/);
  assert.match(h.host.innerHTML, /G3 \/ G5/);
  assert.match(h.host.innerHTML, /value="42" aria-label="Grade progress"/);
  assert.match(h.host.innerHTML, /data-reference-id="blueprint\/FSD_LongRange"/);
  assert.match(h.host.innerHTML, /data-reference-id="experimental\/special_fsd_heavy"/);
  h.click({shipEngFilter: 'locked'});
  assert.match(h.host.innerHTML, /Shield Generator/);
  assert.doesNotMatch(h.host.innerHTML, /data-ship-eng-row="FrameShiftDrive"/);
  h.input('internal slot 1');
  assert.match(h.list.innerHTML, /Shield Generator/);
  h.input('Thrusters');
  assert.match(h.list.innerHTML, /Nothing found/);
  assert.deepEqual(h.calls, [['ship-engineering-plan', {expectedPlanWorkspace: 'commander-a'}]]);
});

test('slot planning shows remaining grades, materials, access steps and internal links', async () => {
  const h = fixture(view, 'ru');
  await h.initialize();
  await h.click({shipEngSlot: 'FrameShiftDrive'});
  assert.match(h.host.innerHTML, /data-reference-id="module\/fsd"/);
  assert.match(h.host.innerHTML, /data-reference-id="engineer\/300100"/);
  assert.match(h.host.innerHTML, /data-system="Deciat"/);
  assert.match(h.host.innerHTML, /Передайте 1 единицу сплава/);
  assert.doesNotMatch(h.host.innerHTML, /Лишнее выполненное требование/);
  assert.match(h.host.innerHTML, /data-reference-id="material\/arsenic"/);
  assert.match(h.host.innerHTML, /ship-eng-deficit">5/);
  assert.match(h.host.innerHTML, /<h4>G4 /);
  assert.doesNotMatch(h.host.innerHTML, /<h4>G2 /);
  await h.change('experimentalKey', '');
  assert.deepEqual(h.calls.at(-1)[1], {expectedPlanWorkspace: 'commander-a', slot: 'FrameShiftDrive',
    blueprintKey: 'FSD_LongRange', grade: 5, experimentalKey: ''});
  await h.change('blueprintKey', 'FSD_FastBoot');
  assert.deepEqual(h.calls.at(-1)[1], {expectedPlanWorkspace: 'commander-a', slot: 'FrameShiftDrive', blueprintKey: 'FSD_FastBoot'});
});

test('journal refresh updates the page, and a new ship drops old slot data and stale requests', async () => {
  let stale;
  const h = fixture(input => input.slot ? new Promise(resolve => { stale = resolve; }) :
    Promise.resolve(input.expectedPlanWorkspace === 'commander-a' ? loadout : {loadoutKnown: false, ship: 'Python'}));
  await h.initialize();
  h.C.state.engineeringRoadmapRevision++;
  await h.initialize();
  assert.equal(h.calls.length, 2);
  const pending = h.click({shipEngSlot: 'FrameShiftDrive'});
  h.C.state = {planWorkspace: {key: 'commander-b'}, commander: {fid: 'b', shipId: '9', ship: 'Python'}, engineeringRoadmapRevision: 3};
  await h.initialize();
  assert.match(h.host.innerHTML, /Python/);
  assert.match(h.host.innerHTML, /record its modules in the journal/);
  assert.doesNotMatch(h.host.innerHTML, /FrameShiftDrive/);
  stale({...loadout, detail});
  await pending;
  assert.doesNotMatch(h.host.innerHTML, /Scout|Deciat/);
});

test('per-roll costs and optional partial totals are marked when complete quantities are not known', async () => {
  const h = fixture(input => Promise.resolve({...loadout, detail: input.slot ? {...detail,
    recipe: {confidence: 'unknown', totalsPartial: true, totals: [{...material, count: 2}],
      grades: [{grade: 5, status: 'pending', perRoll: [{...material, count: 1}]}]}
  } : undefined}));
  await h.initialize();
  await h.click({shipEngSlot: 'FrameShiftDrive'});
  assert.match(h.host.innerHTML, /Some grades only have per-roll material costs/);
  assert.match(h.host.innerHTML, /<h4>G5 <small>Per roll/);
  assert.match(h.host.innerHTML, /Upgrade materials[\s\S]*Remaining[\s\S]*On hand[\s\S]*Missing/);
  assert.match(h.host.innerHTML, /<th scope="col">Missing per roll<\/th>/);
  assert.match(h.host.innerHTML, /<td>1<\/td><td>3<\/td>/);
});

test('partial DSS G5 shows estimated remaining quantities, on-hand stock and shortages without opening details', async () => {
  const scanner = moduleRow('Slot12_Size1', 'in-progress', {item: 'int_detailedsurfacescanner_tiny',
    moduleKey: 'ss', name: 'Подробный сканер поверхности', english: 'Detailed Surface Scanner', grade: 5, quality: .8, maxGrade: 5});
  const tin = {key: 'tin', english: 'Tin', count: 1, owned: 4, missing: 0, estimated: true};
  const alloys = {key: 'protoradiolicalloys', english: 'Proto Radiolic Alloys', count: 1, owned: 0, missing: 1, estimated: true};
  const exact = {key: 'test-exact', english: 'Exact Material', count: 2, owned: 4, missing: 0};
  const h = fixture(input => Promise.resolve({...loadout, modules: [scanner], detail: input.slot ? {...detail, slot: scanner.slot,
    recipe: {confidence: 'estimate', totalsEstimated: true, totals: [tin, alloys, exact],
      grades: [{grade: 5, status: 'estimated', rolls: 1, materials: [tin, alloys]}]}
  } : undefined}));
  await h.initialize();
  await h.click({shipEngSlot: scanner.slot});
  assert.match(h.host.innerHTML, /estimated remaining cost/);
  assert.match(h.host.innerHTML, /data-ship-eng-materials[\s\S]*Tin[\s\S]*<td>≈ 1<\/td><td>4<\/td>/);
  assert.match(h.host.innerHTML, /Proto Radiolic Alloys[\s\S]*<td>≈ 1<\/td><td>0<\/td>[\s\S]*ship-eng-deficit">≈ 1/);
  assert.match(h.host.innerHTML, /Exact Material<\/button><\/th>\s*<td>2<\/td><td>4<\/td>/);
  assert.doesNotMatch(h.host.innerHTML, /Missing per roll/);
});

test('two slot choices save independently and a fresh plan shows shared material totals and ordered visits', async () => {
  const second = installed[2];
  const saved = new Map();
  const currentDetail = input => {
    if (input.slot === detail.slot) return {...detail, ...(input.blueprintKey ? {blueprintKey: input.blueprintKey} : {}), grade: input.grade || 5};
    return {...detail, slot: second.slot, blueprintKey: 'Engine_Dirty', experimentalKey: '',
      grade: input.grade || 3, blueprints: [{key: 'Engine_Dirty', name: 'Грязная настройка', english: 'Dirty tuning', maxGrade: 5}]};
  };
  const workshop = {...engineer, id: 'Farseer', station: engineer.base, status: 'access-required',
    entryId: 'engineer/300100', distance: 34, steps: detail.unlockSteps[0].steps.slice(1), tasks: []};
  const respond = async (input, op) => {
    const goal = currentDetail(input);
    if (op === 'ship-engineering-plan-update') {
      assert.equal(input.expectedShipID, '0');
      if (input.action === 'upsert') {
        assert.equal(input.item, installed.find(row => row.slot === input.slot).item);
        saved.set(input.slot, {...installed.find(row => row.slot === input.slot), blueprintKey: goal.blueprintKey,
          grade: goal.grade, experimentalKey: goal.experimentalKey, status: 'pending'});
      } else saved.delete(input.slot);
    }
    const goals = [...saved.values()];
    const plan = {goals, totalsPartial: true, materials: [{...material, count: 8, owned: 3, missing: 5, partial: true}],
      unlockSupplies: [{name: 'Сплав', english: 'Alloy', count: 1}],
      itinerary: {stops: [{...workshop, tasks: goals}, {...workshop, id: 'second', name: 'Вторая мастерская',
        english: 'Second workshop', system: 'Sol', status: 'ready', steps: []}]}};
    return {...loadout, shipID: 0, plan, detail: input.slot && input.action !== 'remove' ? goal : undefined};
  };
  const h = fixture(respond);
  await h.initialize();
  await h.click({shipEngSlot: detail.slot});
  await h.click({shipEngAction: 'upsert'});
  assert.match(h.host.innerHTML, /Saved in plan/);
  await h.click({shipEngSlot: second.slot});
  await h.click({shipEngAction: 'upsert'});
  assert.equal(saved.size, 2);

  const fresh = fixture(respond);
  await fresh.initialize();
  fresh.click({shipEngView: 'plan'});
  const markup = fresh.host.innerHTML;
  assert.equal((markup.match(/data-ship-eng-goal=/g) || []).length, 2);
  assert.match(markup, /Material totals[\s\S]*Arsenic[\s\S]*≥ 8[\s\S]*ship-eng-deficit">≥ 5/);
  assert.match(markup, /Workshop access contributions[\s\S]*Alloy/);
  assert.match(markup, /Engineer visit order[\s\S]*Felicity Farseer[\s\S]*Donate 1 unit of alloy[\s\S]*Second workshop/);
  await fresh.click({shipEngAction: 'edit', shipEngGoalSlot: detail.slot});
  assert.equal(fresh.calls.at(-1)[1].blueprintKey, 'FSD_LongRange');
  await fresh.change('grade', '4');
  await fresh.click({shipEngAction: 'upsert'});
  assert.equal(saved.size, 2);
  assert.equal(saved.get(detail.slot).grade, 4);
  fresh.click({shipEngView: 'plan'});
  await fresh.click({shipEngAction: 'remove', shipEngGoalSlot: detail.slot});
  assert.equal(saved.size, 1);
  assert.doesNotMatch(fresh.host.innerHTML, /data-ship-eng-goal="FrameShiftDrive"/);
});

test('visits refresh from a new system and saved targets remain visible while the journal updates the loadout', async () => {
  let waiting = false;
  const target = {...installed[0], status: 'unavailable', reason: 'loadout-unavailable'};
  const h = fixture(async () => ({...loadout, loadoutKnown: !waiting, shipID: '3', modules: waiting ? [] : installed,
    plan: {goals: [target], totalsPartial: true, itinerary: {blocked: [target]}}}));
  await h.initialize();
  h.C.state.commander.system = 'Shinrarta Dezhra';
  await h.initialize();
  assert.equal(h.calls.length, 2);
  waiting = true;
  h.C.state.engineeringRoadmapRevision++;
  await h.initialize();
  h.click({shipEngView: 'plan'});
  assert.match(h.host.innerHTML, /Frame Shift Drive/);
  assert.match(h.host.innerHTML, /The journal has not confirmed the current loadout yet/);
  assert.match(h.host.innerHTML, /Remove from plan/);
  assert.doesNotMatch(h.host.innerHTML, /data-ship-eng-action="edit"/);
});
