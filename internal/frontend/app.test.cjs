'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function fixture() {
  const nodes = new Map(), selectors = new Map(), styles = new Map(), classes = new Set(), messages = [], dialogs = [], listeners = new Map();
  const host = { scrollTop: 0, contains: element => !!element?.inPage };
  nodes.set('page-scroll', host);
  nodes.set('layers', { append: node => selectors.set('.menu', node) });
  const document = {
    activeElement: null,
    documentElement: { style: { setProperty: (key, value) => styles.set(key, value), removeProperty: key => styles.delete(key) }, classList: { toggle: (key, value) => value ? classes.add(key) : classes.delete(key) } },
    querySelector: selector => selectors.get(selector) || nodes.get(selector.slice(1)) || null,
    querySelectorAll: selector => selectors.get(selector) || [],
    getElementById: id => nodes.get(id), addEventListener: (event, handler) => listeners.set(event, handler), createElement: () => ({ style: {}, remove() { selectors.delete('.menu'); } }),
  };
  const window = { addEventListener() {}, COPILOT_SMOKE: false };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'overview.js'), 'utf8'), {window});
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'settings-pane.js'), 'utf8'), {window});
  let source = fs.readFileSync(path.join(__dirname, 'app.js'), 'utf8');
  source = source.replace(/\nboot\(\);\s*\}\)\(\);\s*$/, `
window.TestUI={C,referenceCell,context,uiContext,emptyStateInfo,renderEmptyState,renderGeneral,tableWindow,renderTablePager,renderOverview,renderChrome,rendererKind,beginNavigation,renderPage,connectionStatus,renderActivity,applyPalette,previewTheme,actionForm,datetimeInput,actionValues,submitAction,loadView,restoreView,captureView,handleClick,renderModes,renderSettings,history,
 setCall(value){call=value;},setRender(value){renderPage=value;},setNavigate(value){navigate=value;},setLoad(value){loadView=value;},setDialog(value){dialog=value;},setClose(value){closeLayer=value;},setToast(value){toast=value;}};
})();`);
  vm.runInNewContext(source, { window, document, Intl, innerWidth: 1280, innerHeight: 800, localStorage: { getItem() { throw new Error('about:blank'); } }, setTimeout() {}, clearTimeout() {}, setInterval() {} });
  const api = window.TestUI;
  api.setToast((message, error) => messages.push({ message, error }));
  api.setDialog((title, body, footer) => dialogs.push({ title, body, footer }));
  api.setClose(() => { api.C.layer = false; api.C.themePreview = null; });
  return { api, window, document, nodes, selectors, styles, classes, messages, dialogs, host, listeners };
}

test('reading actions target the active lesson instead of a table row', () => {
  const { api } = fixture();
  Object.assign(api.C, { tab: 'learning', lesson: 'trade-alerts.practice', selected: 'trade-alerts.basics' });
  assert.equal(api.context().selected, 'trade-alerts.practice');
  api.C.tab = 'targets';
  assert.equal(api.context().selected, 'trade-alerts.basics');
});

test('entity links open the matching reference without replacing row selection actions', () => {
  const { api } = fixture();
  const data={moduleKey:'am',blueprintKey:'AFM_Shielded',experimentalKey:'test',key:'carbon',system:'Test / Система'};
  for(const [column,id] of [['Модуль','module/am'],['Чертёж','blueprint/AFM_Shielded'],['Материал','material/carbon'],['Экспериментальный эффект','experimental/test'],['Товар','commodity/carbon']]){
    const html=api.referenceCell({Data:data},column,'<Название>');
    assert.ok(html.includes('data-reference-id="'+id+'"'));
    assert.ok(html.includes('&lt;Название&gt;'));
  }
  assert.ok(api.referenceCell({Data:data},'Система','Test').includes('system/Test%20%2F%20%D0%A1'));
  assert.equal(api.referenceCell({Data:data},'Количество','5'),'5');
  assert.equal(api.referenceCell({Data:{}},'Модуль','Неизвестно'),'Неизвестно');
});

test('module UI contract supplies query-all and never inserts an overlay strip into reading mode', () => {
  const { api, nodes } = fixture();
  assert.equal(typeof api.uiContext().$$, 'function');
  let insertions = 0;
  nodes.set('page', { insertAdjacentHTML() { insertions++; } });
  api.C.page = {}; api.C.tab = 'learning'; api.C.state = { settings: { overlayEnabled: true } };
  api.renderActivity();
  assert.equal(insertions, 0);
});

test('game palette affects main surfaces only when explicitly enabled and rejects CSS injection', () => {
  const { api, styles, classes } = fixture();
  api.C.state = { desktop: { useGamePalette: false }, theme: { palette: { accent: '#0099ff', bg: '#001122', text: '#eeeeee', panel: '#102030', danger: 'red;position:fixed' } } };
  api.applyPalette();
  assert.equal(styles.get('--accent'), '#0099ff');
  assert.equal(styles.has('--bg'), false);
  assert.equal(styles.has('--danger'), false);
  api.C.state.desktop.useGamePalette = true; api.applyPalette();
  assert.equal(styles.get('--bg'), '#001122');
  assert.equal(styles.get('--text'), '#eeeeee');
  assert.equal(classes.has('game-palette'), true);
  api.C.state.desktop.useGamePalette = false; api.applyPalette();
  assert.equal(styles.has('--bg'), false);
});

test('import preview never applies a theme until the user presses apply', async () => {
  const { api, dialogs } = fixture();
  const calls = [], preview = { settings: { preset: 'blue' }, palette: { accent: '#28a0f0', text: '#eeeeee', panel: '#172534' }, source: '<script>injected</script>' };
  api.C.actions = [{ id: 'theme-import', fields: [] }];
  api.setCall(async (op, params) => { calls.push({ op, params }); return { themePreview: preview }; });
  await api.submitAction('theme-import', {});
  assert.equal(calls.length, 1);
  assert.equal(calls[0].op, 'action');
  assert.equal(api.C.themePreview, preview);
  assert.match(dialogs[0].footer, /data-click="apply-theme"/);
  assert.match(dialogs[0].body, /&lt;script&gt;/);
  assert.doesNotMatch(dialogs[0].body, /<script>/);
  api.setLoad(async () => {});
  await api.handleClick('apply-theme', { disabled: false });
  assert.equal(calls.at(-1).op, 'theme-apply');
  assert.deepEqual(calls.at(-1).params.values, { preset: 'blue' });
});

test('details are escaped readable text, without mutation or a saved notification', async () => {
  const { api, dialogs, messages } = fixture();
  api.setCall(async () => ({ details: 'Строительство\n<img src=x>', title: 'Колония' }));
  await api.submitAction('details', {});
  assert.equal(dialogs[0].title, 'Колония');
  assert.match(dialogs[0].body, /Строительство\n&lt;img src=x&gt;/);
  assert.equal(messages.length, 0);
});

test('copy, URL, export cancellation and saved file receive accurate notifications', async () => {
  for (const [result, nativeResult, expected] of [
    [{ copy: 'Система' }, { ok: true }, /Скопировано/],
    [{ url: 'https://example.com' }, { ok: true }, /браузере/],
    [{ export: 'route.json' }, { cancelled: true }, /отменено/],
    [{ export: 'route.json' }, { ok: true, file: 'route.json' }, /route\.json/],
  ]) {
    const { api, messages } = fixture();
    api.setCall(async op => op === 'action' ? result : nativeResult);
    api.setLoad(async () => {});
    await api.submitAction('operation', {});
    assert.match(messages.at(-1).message, expected);
    assert.doesNotMatch(messages.at(-1).message, /Изменения сохранены/);
  }
});

test('automatic row and mode selection fetch action defaults for the selected row', async () => {
  const { api } = fixture();
  const calls = [], page = { Rows: [{ ID: 'one' }, { ID: 'two' }], Modes: ['Планы'], Columns: ['Название'] };
  api.setCall(async (op, params) => { calls.push(JSON.parse(JSON.stringify(params))); return { state: {}, page, actions: [{ id: 'edit', fields: [{ key: 'name', default: params.selected }] }] }; });
  api.setRender(() => {});
  await api.loadView();
  assert.equal(calls.length, 2);
  assert.equal(calls[1].selected, 'one');
  assert.equal(calls[1].mode, 'Планы');
  assert.equal(api.C.actions[0].fields[0].default, 'one');
  api.C.selected = 'two'; await api.loadView();
  assert.equal(api.C.actions[0].fields[0].default, 'two');
});

test('quiet polling leaves form input alone and renders deferred changes after blur', async () => {
  const { api, document, window } = fixture();
  let calls = 0, renders = 0;
  api.setCall(async () => { calls++; return { state: { counter: calls }, page: { Rows: [], Modes: [], Columns: ['x'] }, actions: [] }; });
  api.setRender(() => { renders++; });
  document.activeElement = { closest: () => ({}) };
  await api.loadView(true); assert.equal(calls, 0);
  document.activeElement = null; await api.loadView(true);
  assert.equal(calls, 1); assert.equal(renders, 1);
  window.COPILOT_SMOKE = true; await api.loadView(true); assert.equal(calls, 1);
});

test('explicit refresh restores nested scroll, keyboard focus and text selection', async () => {
  const { api, document, nodes, selectors, host } = fixture();
  let focusOptions, selection;
  document.activeElement = { id: 'table-query', inPage: true, selectionStart: 2, selectionEnd: 5, selectionDirection: 'forward' };
  host.scrollTop = 318;
  selectors.set('.table-scroll,.inspector,.sidebar-scroll', [{ scrollTop: 43, scrollLeft: 27 }]);
  api.setCall(async () => ({ state: {}, page: { Rows: [], Modes: [], Columns: ['x'] }, actions: [] }));
  api.setRender(() => {
    host.scrollTop = 0;
    nodes.set('table-query', { focus: options => { focusOptions = options; }, setSelectionRange: (...args) => { selection = args; } });
    selectors.set('.table-scroll,.inspector,.sidebar-scroll', [{ scrollTop: 0, scrollLeft: 0 }]);
  });
  await api.loadView();
  assert.equal(host.scrollTop, 318);
  assert.equal(selectors.get('.table-scroll,.inspector,.sidebar-scroll')[0].scrollLeft, 27);
  assert.equal(focusOptions.preventScroll, true);
  assert.deepEqual(selection, [2, 5, 'forward']);
});

test('dashboard renders every overview mode without forcing a table search', () => {
  const { api } = fixture();
  api.C.page = { Modes: ['Планы', 'Корабль', 'Снаряжение', 'Материалы', 'Груз', 'Статус'] };
  const html = api.renderModes(true);
  assert.equal((html.match(/data-mode=/g) || []).length, 6);
  assert.doesNotMatch(html, /table-query/);
});

test('dashboard uses the Go pilot credit observation, including zero and unknown balance', () => {
  const { api } = fixture();
  api.C.page = { Modes: ['Планы'] }; api.C.actions = [];
  for (const balance of [1234567890, 0, null]) {
    // Commander intentionally carries a different value to expose the former wrong lookup.
    api.C.state = { commander: { credits: 999 }, pilot: { credits: balance, creditsSource: 'Status.json' } };
    const html = api.renderOverview();
    const card = html.match(/Баланс командира<\/div><div class="metric-value">([^<]*)<\/div><div class="metric-foot">([^<]*)/);
    assert.ok(card);
    const expected = balance === null ? '—' : new Intl.NumberFormat('ru-RU', { notation: 'compact', maximumFractionDigits: 1 }).format(balance) + ' кр.';
    assert.equal(card[1], expected);
    assert.equal(card[2], 'Средства командира');
    assert.doesNotMatch(html,/Status\.json/);
  }
});

test('dashboard binds the carrier location and preserves a known zero cargo capacity', () => {
  const { api } = fixture(); api.C.page = { Modes: ['Планы'] }; api.C.actions = [];
  api.C.state = { commander: { ship: 'Sidewinder', cargoTons: 0, cargoCapacity: 0 }, carrier: { owned: true, name: 'Синтетический носитель', callsign: 'ABC-123', location: { system: 'Synthetic Carrier System' } } };
  const html = api.renderOverview();
  assert.match(html, /ABC-123 · Synthetic Carrier System/);
  assert.match(html, /Вместимость 0 т/);
  assert.doesNotMatch(html, /Вместимость уточняется/);
  delete api.C.state.commander.cargoCapacity;
  assert.match(api.renderOverview(), /Вместимость уточняется в игре/);
});

test('header reports the actual journal Reader.State lifecycle rather than assuming connection', () => {
  const { api, nodes } = fixture();
  nodes.set('page-toolbar', {}); nodes.set('sidebar-ship', {}); nodes.set('sidebar-system', {});
  const cases = [
    [{}, 'Журнал не настроен'],
    [{ journalDir: 'Synthetic Journals', eventCount: 0 }, 'Ожидаем события игры'],
    [{ journalDir: 'Synthetic Journals', scanning: true }, 'Читаем журналы'],
    [{ journalDir: 'Synthetic Journals', eventCount: 42, lastEvent: 'LoadGame', lastUpdate: '2026-11-08T08:00:00Z' }, 'Журнал подключён'],
    [{ journalDir: 'Synthetic Journals', error: 'Папка недоступна', lastUpdate: '2026-11-08T08:00:00Z' }, 'Журнал недоступен'],
  ];
  for (const [connection, text] of cases) {
    api.C.state = { connection, commander: { ship: 'Panther Clipper Mk II', system: 'Synthetic Origin' } };
    api.renderChrome();
    assert.equal(api.connectionStatus(), text);
    assert.ok(nodes.get('page-toolbar').innerHTML.includes(text));
    assert.equal(nodes.get('sidebar-ship').textContent, 'Panther Clipper Mk II');
    assert.equal(nodes.get('sidebar-system').textContent, 'Synthetic Origin');
  }
});

test('colony date defaults retain Russian day/month and local wall time in editable forms', async () => {
  const { api, dialogs } = fixture();
  await api.actionForm({ id: 'colony.edit', title: 'Колония', fields: [
    { key: 'claimedAt', label: 'Заявка', kind: 'datetime', default: '07.11.2026 21:45' },
    { key: 'deadline', label: 'Срок', kind: 'datetime', default: '18.11.2026 09:30' },
  ] });
  assert.match(dialogs[0].body, /name="claimedAt"[^>]*value="2026-11-07T21:45"/);
  assert.match(dialogs[0].body, /name="deadline"[^>]*value="2026-11-18T09:30"/);
  assert.equal(api.datetimeInput('2026-11-07T21:45'), '2026-11-07T21:45');
  assert.equal(api.datetimeInput(null), '');
  const instant = new Date('2026-11-07T18:45:00Z'), pad = value => String(value).padStart(2, '0');
  assert.equal(api.datetimeInput(instant.toISOString()), `${instant.getFullYear()}-${pad(instant.getMonth()+1)}-${pad(instant.getDate())}T${pad(instant.getHours())}:${pad(instant.getMinutes())}`);
});

test('standard numeric forms accept Russian decimal and grouped input without changing text fields', async () => {
  const { api } = fixture(); let submitted;
  api.C.actions = [{ id: 'form', fields: [{ key: 'fraction', kind: 'number' }, { key: 'count', kind: 'integer' }, { key: 'optional', kind: 'number' }, { key: 'notes', kind: 'text' }] }];
  api.setCall(async (op, params) => { submitted = params.values; return { ok: true }; });
  api.setLoad(async () => {});
  const raw = { fraction: '0,75', count: '1\u202f000', optional: '', notes: '1 000,75 тонн' };
  await api.submitAction('form', raw);
  assert.equal(submitted.fraction, '0.75'); assert.equal(submitted.count, '1000');
  assert.equal(submitted.optional, ''); assert.equal(submitted.notes, '1 000,75 тонн');
  assert.equal(raw.fraction, '0,75');
});

test('F11 requests a toggle, including the second press', () => {
  const { api, listeners } = fixture(); const calls = [];
  api.setCall(async (op, params) => { calls.push({ op, params }); return { ok: true }; });
  let prevented = 0;
  for (let press = 0; press < 2; press++) listeners.get('keydown')({ key: 'F11', preventDefault() { prevented++; } });
  assert.equal(prevented, 2);
  assert.equal(calls.length, 2);
  for (const call of calls) assert.equal(call.params.action, 'toggle-fullscreen');
});

test('a quiet response arriving after focus does not consume the deferred render', async () => {
  const { api, document } = fixture(); let resolveFirst, renders = 0;
  const first = new Promise(resolve => { resolveFirst = resolve; }); let requests = 0;
  const result = { state: { newer: true }, page: { Rows: [], Modes: [], Columns: ['x'] }, actions: [] };
  api.setCall(async () => ++requests === 1 ? first : result); api.setRender(() => { renders++; });
  const pending = api.loadView(true);
  document.activeElement = { closest: () => ({}) }; resolveFirst(result); await pending;
  assert.equal(renders, 0); assert.equal(api.C.lastSignature, '');
  document.activeElement = null; await api.loadView(true);
  assert.equal(renders, 1); assert.equal(requests, 2);
});

test('Academy metadata open, next and previous actions switch the reader and restore the chosen lesson position', async () => {
  for (const action of ['learning.open', 'learning.next', 'learning.previous']) {
    const { api, messages } = fixture(); let mutations = 0; const selected = [];
    api.C.tab = 'learning'; api.C.lesson = 'old';
    api.setCall(async op => { if (op === 'action') mutations++; return { ok: true }; });
    api.setLoad(async () => { selected.push(api.context().selected); api.C.state.learning = { selectedLessonId: 'trade.practice', lessons: [{ id: 'trade.practice', courseId: 'trade', position: { ratio: 0.35, sectionId: 'exercise' } }] }; });
    await api.submitAction(action, {});
    assert.equal(api.C.lesson, 'trade.practice'); assert.equal(api.C.course, 'trade');
    assert.equal(api.C.academy.pendingRestore.lessonId, 'trade.practice');
    assert.equal(api.C.academy.pendingRestore.ratio, 0.35);
    assert.equal(api.C.academy.pendingRestore.sectionId, 'exercise');
    assert.deepEqual(selected, ['old', 'trade.practice']);
    assert.equal(mutations, 1); assert.equal(messages.length, 0);
  }
});

test('page menu includes all actions even on custom settings pages', async () => {
  const { api, selectors } = fixture();
  api.C.actions = ['theme', 'journal', 'prices', 'hotkey'].map(id => ({ id, title: id }));
  await api.handleClick('page-menu', null);
  const html = selectors.get('.menu').innerHTML;
  for (const id of ['theme', 'journal', 'prices', 'hotkey']) assert.match(html, new RegExp('data-action-id="'+id+'"'));
  assert.equal((html.match(/data-action-id=/g) || []).length, 4);
});

test('system selection empty state offers the search form even when two colonies already exist', async () => {
  const { api, dialogs } = fixture();
  api.C.tab = 'colonization'; api.C.mode = 'Выбор системы';
  api.C.state.colonization = { selectedProjectId: 'alpha', activeProject: { id: 'alpha', buildings: [] }, projects: [{ id: 'alpha' }, { id: 'beta' }] };
  api.C.actions = [
    { id: 'colony.select', title: 'Выбрать колонию', fields: [{ key: 'id' }] },
    { id: 'colony.search', title: 'Найти системы для колонизации', fields: [{ key: 'originSystem', label: 'Исходная система', kind: 'text', default: 'Synthetic Origin' }, { key: 'faction', label: 'Фракция станции', kind: 'combo', default: 'Synthetic Faction' }] },
  ];
  const html = api.renderEmptyState();
  assert.match(html, /data-action-id="colony.search"/);
  assert.doesNotMatch(html, /data-action-id="colony.select"|Добавьте колонию/);
  await api.handleClick('action', { dataset: { actionId: api.emptyStateInfo().actionId } });
  assert.match(dialogs[0].body, /name="originSystem"[^>]*value="Synthetic Origin"/);
  assert.match(dialogs[0].body, /name="faction"[^>]*value="Synthetic Faction"/);
});

test('construction and supply empty states select the next action for the current colony', () => {
  const { api } = fixture(); api.C.tab = 'colonization';
  api.C.state.colonization = { activeProject: { id: 'alpha', buildings: [] } };
  api.C.actions = ['colony.select', 'colony.add-building', 'colony.resource-upsert'].map(id => ({ id, title: id, fields: [] }));
  api.C.mode = 'Строительство'; assert.match(api.renderEmptyState(), /data-action-id="colony.add-building"/);
  api.C.mode = 'Снабжение'; assert.match(api.renderEmptyState(), /data-mode="Строительство"/);
  assert.doesNotMatch(api.renderEmptyState(), /data-action-id="colony.select"/);
  api.C.state.colonization.activeProject.buildings = [{ id: 'port' }];
  assert.match(api.renderEmptyState(), /data-action-id="colony.resource-upsert"/);
  assert.match(api.renderEmptyState(), /постройку.*смету/);
});

test('an unmatched query offers only clearing the query and keeps the selected mode', async () => {
  const { api } = fixture(); let loads = 0;
  api.C.tab = 'colonization'; api.C.mode = 'Снабжение'; api.C.query = '<нет совпадений>';
  api.C.actions = [{ id: 'colony.select', title: 'Выбрать колонию', fields: [] }];
  const html = api.renderEmptyState();
  assert.match(html, /Нет совпадений/); assert.match(html, /data-click="clear-query"/);
  assert.match(html, /&lt;нет совпадений&gt;/); assert.doesNotMatch(html, /data-action-id=/);
  api.setLoad(async () => { loads++; }); await api.handleClick('clear-query', null);
  assert.equal(api.C.query, ''); assert.equal(api.C.mode, 'Снабжение'); assert.equal(loads, 1);
});

test('statistics does not prompt to reset a period and engineering empty prompts match the current mode', () => {
  const { api } = fixture(); api.C.tab = 'statistics'; api.C.mode = 'Торговля';
  api.C.actions = [{ id: 'stats.reset', title: 'Новый период учёта', fields: [] }];
  assert.match(api.renderEmptyState(), /Торговых сделок пока нет/);
  assert.doesNotMatch(api.renderEmptyState(), /data-action-id=/);
  api.C.tab = 'engineering'; api.C.mode = 'Обмен'; api.C.actions = [{ id: 'engineering.trade', title: 'Рассчитать обмен', fields: [] }];
  assert.match(api.renderEmptyState(), /data-action-id="engineering.trade"/);
  assert.doesNotMatch(api.renderEmptyState(), /Добавьте улучшение/);
  api.C.mode = 'Материалы плана'; assert.match(api.renderEmptyState(), /data-mode="План"/);
});


test('large tables keep all rows searchable while mounting at most one hundred rows',()=>{
 const {api}=fixture();api.C.tab='targets';api.C.mode='Места добычи';api.C.inspector=false;api.C.state={};api.C.actions=[];api.C.page={Title:'Места добычи',Description:'',Summary:'',Modes:[],Columns:['Система'],Rows:Array.from({length:240},(_,i)=>({ID:String(i),Cells:['Система '+i]}))};
 let html=api.renderGeneral();assert.equal((html.match(/data-row=/g)||[]).length,100);assert.match(html,/1–100 из 240/);assert.match(html,/data-click="table-next"/);assert.equal(api.C.page.Rows.length,240);
 api.C.tablePage=2;html=api.renderGeneral();assert.equal((html.match(/data-row=/g)||[]).length,40);assert.match(html,/201–240 из 240/);assert.match(html,/data-row="239"/);assert.doesNotMatch(html,/data-row="0"/);
 api.C.query='Система 239';api.C.page.Rows=[api.C.page.Rows.at(-1)];const window=api.tableWindow();assert.equal(window.page,0);assert.equal(window.rows[0].ID,'239');assert.equal(api.renderTablePager(window),'');
});


test('asynchronous desktop save errors remain visible to the user',()=>{const {window,messages}=fixture();window.copilotReceive({event:'notice',data:{message:'Не удалось сохранить настройки'}});assert.equal(messages.length,1);assert.equal(messages[0].message,'Не удалось сохранить настройки');assert.equal(messages[0].error,true);});


test('canonical view context opens a screen with matching actions in one request',async()=>{const {api}=fixture();let calls=0,renders=0;api.C.tab='overview';api.C.mode='';api.C.selected='';api.setCall(async()=>{calls++;return {context:{mode:'Планы',selected:'goal-b'},page:{Title:'Обзор',Modes:['Планы'],Rows:[{ID:'goal-a'},{ID:'goal-b'}]},actions:[{id:'goal.edit',fields:[{key:'title',default:'Вторая цель'}]}],state:{}};});api.setRender(()=>{renders++;});await api.loadView();assert.equal(calls,1);assert.equal(renders,1);assert.equal(api.C.selected,'goal-b');assert.equal(api.C.mode,'Планы');assert.equal(api.C.actions[0].fields[0].default,'Вторая цель');});

test('a hidden webview never polls the backend for a quiet refresh',async()=>{const {api,document}=fixture();document.hidden=true;let calls=0;api.setCall(async()=>{calls++;return {};});await api.loadView(true);assert.equal(calls,0);});

test('the real command shortcut opens the launcher with Russian physical K and Latin synthetic fallback', () => {
  for (const keys of [
    { key: 'л', code: 'KeyK', ctrlKey: true },
    { key: 'Л', code: 'KeyK', ctrlKey: true, shiftKey: true },
    { key: 'л', code: 'KeyK', metaKey: true },
    { key: 'k', ctrlKey: true },
    { key: 'K', code: 'Unidentified', ctrlKey: true },
  ]) {
    const { api, nodes, selectors, listeners, dialogs } = fixture();
    const results = { innerHTML: '' };
    nodes.set('command-results', results);
    selectors.set('.dialog-body', { style: {} });
    api.C.tabs = [{ ID: 'overview', Title: 'Обзор' }, { ID: 'sale', Title: 'Торговля' }];
    let prevented = 0;
    listeners.get('keydown')({ ...keys, preventDefault() { prevented++; } });
    assert.equal(prevented, 1, JSON.stringify(keys));
    assert.equal(dialogs.length, 1);
    assert.equal(dialogs[0].title, 'Быстрый переход');
    assert.match(dialogs[0].body, /id="command-query"/);
    assert.match(results.innerHTML, /data-nav="overview"/);
    assert.match(results.innerHTML, /data-nav="sale"/);
  }
});

test('the command shortcut requires a modifier and ignores unrelated physical keys', () => {
  for (const keys of [
    { key: 'л', code: 'KeyK' },
    { key: 'k', code: 'KeyK' },
    { key: 'о', code: 'KeyJ', ctrlKey: true },
    { key: 'л', ctrlKey: true },
  ]) {
    const { listeners, dialogs } = fixture();
    let prevented = 0;
    listeners.get('keydown')({ ...keys, preventDefault() { prevented++; } });
    assert.equal(dialogs.length, 0, JSON.stringify(keys));
    assert.equal(prevented, 0);
  }
});

function mountedPage(f){
 const attributes=new Map(),page={innerHTML:'<div>Previous screen and obsolete controls</div>',setAttribute(key,value){attributes.set(key,value);},removeAttribute(key){attributes.delete(key);}};
 f.nodes.set('page',page);
 for(const id of ['page-toolbar','navigation','sidebar-ship','sidebar-system'])f.nodes.set(id,{innerHTML:'',textContent:''});
 return {page,attributes};
}

test('the actual toolbar offers details only for the renderer that can display a real panel',()=>{
 const f=fixture();mountedPage(f);const {api,nodes}=f;
 api.C.page={Rows:[{ID:'selected',Cells:['Known row']}],Columns:['Value'],Modes:[]};
 for(const [tab,mode,kind,details] of [
  ['settings','','settings',true],['overview','','overview',false],['overview','Планы','overview',false],['overview','Состояние','overview',false],['overview','Корабль и груз','table',true],
  ['encyclopedia','Корабли','reference',false],['engineering','Инженеры','reference',false],['targets','Инженеры','reference',false],['engineering','План','table',true],
  ['learning','','learning',false],['carrier-calculator','','carrier-calculator',false],['carrier-trade','','carrier-trade',false],['deals','Поиск','deals',false],
  ['route','Маршрут','table',true],['colonization','Строительство','table',true],['sale','Продать','table',true],
 ]){
  api.C.tab=tab;api.C.mode=mode;api.renderChrome();
  assert.equal(api.rendererKind(),kind,tab+' / '+mode);
  assert.equal(nodes.get('page-toolbar').innerHTML.includes('data-click="inspector"'),details,tab+' / '+mode);
 }
 api.C.page.Rows=[];
 for(const tab of ['sale','route','colonization']){api.C.tab=tab;api.C.mode='';api.renderChrome();assert.equal(nodes.get('page-toolbar').innerHTML.includes('data-click="inspector"'),false,'Empty '+tab+' has no details panel');}
});

test('the real details handler removes and restores the settings panel and persists both choices',async()=>{
 const f=fixture(),{page}=mountedPage(f),{api}=f,calls=[];
 api.C.tab='settings';api.C.inspector=true;api.C.page={Title:'Настройки',Rows:[],Columns:[],Modes:[]};
 api.setCall(async(op,params)=>{calls.push({op,params});return {ok:true};});
 api.renderPage();assert.match(page.innerHTML,/<aside class="inspector settings-help"/);
 await api.handleClick('inspector',{});
 assert.equal(api.C.inspector,false);assert.doesNotMatch(page.innerHTML,/<aside class="inspector settings-help"/);
 await api.handleClick('inspector',{});
 assert.equal(api.C.inspector,true);assert.match(page.innerHTML,/<aside class="inspector settings-help"/);
 assert.deepEqual(calls.map(item=>[item.op,item.params.values.inspectorOpen]),[['desktop',false],['desktop',true]]);
});

test('navigation replaces obsolete content immediately and invalidates a reply during asynchronous menu saving',async()=>{
 const f=fixture(),{page,attributes}=mountedPage(f),{api}=f;
 let resolveOld,resolveNavigation;
 const oldReply=new Promise(resolve=>{resolveOld=resolve;}),navigationSaved=new Promise(resolve=>{resolveNavigation=resolve;});
 const response=title=>({context:{mode:'Торговля',selected:'row'},state:{responseTitle:title},page:{Title:title,Description:'',Rows:[{ID:'row',Cells:[title]}],Columns:['Название'],Modes:['Торговля']},actions:[]});
 api.C.tab='route';api.C.mode='Маршрут';api.C.inspector=false;api.C.page={Title:'Old',Rows:[],Columns:[],Modes:[]};api.C.actions=[{id:'obsolete'}];api.C.help='Obsolete section help';
 api.C.tabs=[{ID:'route',Title:'Маршрут флотоносца'},{ID:'statistics',Title:'Статистика'}];api.C.nav=['route'];
 api.setCall(async(op,params)=>op==='navigation'?navigationSaved:params.tab==='route'?oldReply:response('Latest statistics response'));
 const previousRequest=api.loadView();
 const navigation=api.uiContext().navigate('statistics');
 assert.equal(api.C.page,null);assert.equal(api.C.actions.length,0);assert.equal(api.C.help,'');
 assert.match(page.innerHTML,/Открываю раздел/);assert.doesNotMatch(page.innerHTML,/Previous screen|obsolete controls/);
 assert.equal(attributes.get('aria-busy'),'true');
 resolveOld(response('Outdated route response'));await previousRequest;
 const duringMenuSave={page:api.C.page,html:page.innerHTML};
 resolveNavigation({ok:true});await navigation;
 assert.equal(duringMenuSave.page,null);assert.match(duringMenuSave.html,/Открываю раздел/);assert.doesNotMatch(duringMenuSave.html,/Outdated route response/);
 assert.equal(api.C.tab,'statistics');assert.equal(api.C.state.responseTitle,'Latest statistics response');
 assert.match(page.innerHTML,/Latest statistics response/);assert.doesNotMatch(page.innerHTML,/Outdated route response|Открываю раздел/);
 assert.equal(attributes.has('aria-busy'),false);
});

test('failed navigation shows a recoverable error instead of the obsolete page or endless loading',async()=>{
 const f=fixture(),{page,attributes}=mountedPage(f),{api}=f;
 api.C.tab='statistics';api.beginNavigation();
 api.setCall(async()=>{throw new Error('<Temporary failure>');});
 await api.loadView();
 assert.match(page.innerHTML,/Не удалось открыть раздел/);assert.match(page.innerHTML,/&lt;Temporary failure&gt;/);
 assert.match(page.innerHTML,/data-click="reload-page"/);assert.doesNotMatch(page.innerHTML,/Previous screen|Открываю раздел/);
 assert.equal(attributes.has('aria-busy'),false);
});

function disclosure(kind,open,owner=''){
 return {className:kind,open,closest(selector){return selector==='[data-deals-route]'&&owner?{dataset:{dealsRoute:owner}}:null;}};
}

test('quiet refresh preserves both opened and deliberately closed disclosures after replacing their DOM nodes',async()=>{
 const {api,selectors}=fixture();
 api.C.tab='deals';api.C.mode='Поиск';
 const before=[disclosure('panel deals-active-plan',true),disclosure('panel deals-search',false),disclosure('deals-more',true),disclosure('rw-warnings',false)];
 const defaults=[disclosure('panel deals-active-plan',false),disclosure('panel deals-search',true),disclosure('deals-more',false),disclosure('rw-warnings',true)];
 selectors.set('#page details',before);
 api.setCall(async()=>({context:{mode:'Поиск',selected:''},page:{Rows:[],Modes:[],Columns:[]},state:{revision:2},actions:[]}));
 api.setRender(()=>selectors.set('#page details',defaults));
 await api.loadView(true);
 assert.deepEqual(defaults.map(node=>node.open),[true,false,true,false]);
 assert.notEqual(selectors.get('#page details')[0],before[0]);
 assert.deepEqual(before.map(node=>node.open),[true,false,true,false]);
});

test('quiet disclosure restore cannot alias a different route owner, changed section or shifted index',async()=>{
 const {api,selectors}=fixture();api.C.tab='deals';api.C.mode='Поиск';
 const before=[disclosure('same-class',true,'route-a'),disclosure('same-class',false,'route-b'),disclosure('rw-warnings',true),disclosure('calc-assumptions',false),disclosure('same-class',true,'unchanged-route')];
 const after=[disclosure('same-class',false,'route-b'),disclosure('same-class',true,'route-a'),disclosure('colony-data-warnings',false),disclosure('new-section',true),disclosure('same-class',false,'unchanged-route'),disclosure('calc-assumptions',true)];
 selectors.set('#page details',before);
 api.setCall(async()=>({context:{mode:'Поиск',selected:''},page:{Rows:[],Modes:[],Columns:[]},state:{revision:3},actions:[]}));
 api.setRender(()=>selectors.set('#page details',after));
 await api.loadView(true);
 assert.deepEqual(after.map(node=>node.open),[false,true,false,true,true,true]);
});

test('an explicit refresh uses fresh disclosure defaults instead of restoring a prior open state',async()=>{
 const {api,selectors}=fixture();api.C.tab='deals';api.C.mode='Поиск';
 const before=[disclosure('panel deals-active-plan',true),disclosure('panel deals-search',false),disclosure('deals-more',true)];
 const defaults=[disclosure('panel deals-active-plan',false),disclosure('panel deals-search',true),disclosure('deals-more',false)];
 selectors.set('#page details',before);
 api.setCall(async()=>({context:{mode:'Поиск',selected:''},page:{Rows:[],Modes:[],Columns:[]},state:{revision:4},actions:[]}));
 api.setRender(()=>selectors.set('#page details',defaults));
 await api.loadView(false);
 assert.deepEqual(defaults.map(node=>node.open),[false,true,false]);
});
