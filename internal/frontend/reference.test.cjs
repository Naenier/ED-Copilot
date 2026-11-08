'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const clone = value => JSON.parse(JSON.stringify(value));
const esc = value => String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]));
const pixel = 'data:image/png;base64,iVBORw0KGgo=';
const categoryRows = [
  { id: 'articles', title: 'Статьи', count: 35 }, { id: 'ships', title: 'Корабли', count: 49 },
  { id: 'engineers', title: 'Инженеры', count: 38 }, { id: 'commodities', title: 'Товары', count: 40 },
  { id: 'blueprints', title: 'Чертежи', count: 36 },
];
function catalogue(entries = [{ id: 'ship/test', category: 'ships', title: 'Тестовый корабль', summary: 'Для дальних перелётов', image: { src: pixel, alt: 'Корабль', fit: 'contain' }, meta: [{ label: 'Площадка', value: 'Средняя' }] }], total = entries.length) {
  return { categories: categoryRows, groups: [{ id: 'travel', title: 'Перелёты', count: 12 }], entries, total, offset: 0, limit: 24 };
}
function entry(id = 'ship/test') {
  return { id, category: id.split('/')[0] === 'ship' ? 'ships' : 'modules', title: id === 'ship/test' ? 'Тестовый корабль' : 'Силовая установка', subtitle: 'Полная справочная страница', description: 'Подготовьте корабль к следующему путешествию.', image: { src: pixel, alt: 'Корабль', fit: 'contain' }, properties: [{ label: 'Груз', value: 0 }, { label: 'Модель', value: 'Тест' }], sections: [
    { id: 'overview', title: 'Назначение', type: 'prose', paragraphs: ['Первая часть справочного текста.', 'Вторая часть.'] },
    { id: 'requirements', title: 'Подготовка', type: 'requirements', items: [{ title: 'Знакомство', text: 'Найдите мастерскую.', fulfilled: true }, { title: 'Поставка', text: 'Подготовьте груз.' }] },
    { id: 'specs', title: 'Характеристики', type: 'table', columns: ['Показатель', 'Значение'], rows: [['Масса', '400 т']] },
  ], related: [{ id: 'module/pp', category: 'modules', title: 'Силовая установка', subtitle: 'Подбор оснащения' }] };
}
function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; }
async function microtasks() { for (let i = 0; i < 8; i++) await Promise.resolve(); }

function fixture(options = {}) {
  const calls = [], navigation = [], timers = new Map(), nodes = new Map(), handlers = new Map(), toasts = [];
  let sequence = 0, renders = 0, html = '', api, rpc;
  const host = { scrollTop: 0, addEventListener(type, callback) { const entries = handlers.get(type) || []; entries.push(callback); handlers.set(type, entries); } };
  nodes.set('#page-scroll', host);
  const C = { tab: 'encyclopedia', mode: 'Корабли', state: {} };
  const ui = {
    esc, fmt: value => String(value), icon: name => `<svg data-icon="${esc(name)}"></svg>`,
    btn: (label, action, icon, kind, attrs = '') => `<button data-click="${action}" ${attrs}>${esc(label)}</button>`,
    call: async (op, params) => { calls.push({ op, params: clone(params) }); return rpc(op, params); },
    renderPage() { renders++; html = api.render(C, ui); if (options.autoInit) api.init(C, ui); },
    navigate: async (id, opts) => { navigation.push({ id, opts: clone(opts) }); },
    toast: (message, error) => toasts.push({ message, error }),
    $: selector => nodes.get(selector) || null,
  };
  rpc = async (op, params) => op === 'reference.entry' ? { entry: entry(params.id) } : catalogue();
  const window = {};
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'reference.js'), 'utf8'), {
    window, setTimeout(callback) { timers.set(++sequence, callback); return sequence; }, clearTimeout(id) { timers.delete(id); },
  });
  api = window.ReferenceUI;
  return { api, C, ui, calls, navigation, host, handlers, nodes, timers, toasts,
    setRPC(value) { rpc = value; }, get renders() { return renders; }, get html() { return html; },
    async flushTimers() { const ready = [...timers.values()]; timers.clear(); for (const callback of ready) callback(); await microtasks(); },
  };
}

test('reference renders immediately and lazily loads one bounded catalogue request without blocking controls', async () => {
  const f = fixture({ autoInit: true }), pending = deferred();
  f.setRPC(() => pending.promise);
  assert.match(f.api.render(f.C, f.ui), /Энциклопедия/);
  assert.equal(f.calls.length, 0);
  const loading = f.api.init(f.C, f.ui);
  assert.equal(f.C.reference.loading, true);
  assert.match(f.api.render(f.C, f.ui), /Загрузка справочника/);
  assert.match(f.api.render(f.C, f.ui), /id="reference-query"/);
  f.api.init(f.C, f.ui); await microtasks();
  assert.equal(f.calls.length, 1);
  assert.deepEqual(f.calls[0], { op: 'reference.list', params: { category: 'ships', query: '', group: '', offset: 0, limit: 24 } });
  pending.resolve(catalogue()); await loading;
  assert.equal(f.C.reference.loading, false);
  const html = f.api.render(f.C, f.ui);
  assert.match(html, /data-click="reference-open" data-id="ship\/test"/);
  assert.match(html, /<img[^>]+loading="lazy"/);
  assert.equal(f.calls.filter(call => call.op === 'reference.entry').length, 0);
});

test('categories and logical groups use backend filters and reset pagination', async () => {
  const f = fixture(); await f.api.init(f.C, f.ui);
  f.C.reference.offset = 48; f.C.reference.query = 'старый запрос'; f.C.reference.group = 'old';
  await f.api.handleClick('reference-category', { dataset: { id: 'blueprints' } }, f.C, f.ui);
  assert.deepEqual(f.calls.at(-1).params, { category: 'blueprints', query: '', group: '', offset: 0, limit: 24 });
  f.C.reference.offset = 24;
  await f.api.handleChange({ target: { id: 'reference-group', value: 'travel' } }, f.C, f.ui);
  assert.equal(f.calls.at(-1).params.group, 'travel');
  assert.equal(f.calls.at(-1).params.offset, 0);
});

test('debounced Cyrillic search ignores older results even when replies arrive out of order', async () => {
  const f = fixture(), old = deferred(), newer = deferred();
  await f.api.init(f.C, f.ui);
  f.setRPC((op, params) => params.query === 'старый' ? old.promise : newer.promise);
  f.api.handleInput({ target: { id: 'reference-query', value: 'старый' } }, f.C, f.ui); await f.flushTimers();
  f.api.handleInput({ target: { id: 'reference-query', value: 'новый' } }, f.C, f.ui); await f.flushTimers();
  newer.resolve(catalogue([{ id: 'ship/new', category: 'ships', title: 'Новый результат' }])); await microtasks();
  old.resolve(catalogue([{ id: 'ship/old', category: 'ships', title: 'Старый результат' }])); await microtasks();
  const html = f.api.render(f.C, f.ui);
  assert.match(html, /Новый результат/); assert.doesNotMatch(html, /Старый результат/);
  assert.equal(f.C.reference.query, 'новый');
  assert.deepEqual(f.calls.slice(-2).map(call => call.params.query), ['старый', 'новый']);
});

test('screen pagination requests only the next bounded page and keeps search context', async () => {
  const f = fixture();
  f.setRPC(async (op, params) => catalogue(Array.from({ length: params.offset === 48 ? 2 : 24 }, (_, index) => ({ id: 'ship/' + (params.offset + index), category: 'ships', title: 'Корабль ' + (params.offset + index) })), 50));
  await f.api.init(f.C, f.ui);
  f.C.reference.query = 'корабль';
  await f.api.handleClick('reference-next', null, f.C, f.ui);
  assert.equal(f.calls.at(-1).params.offset, 24); assert.equal(f.calls.at(-1).params.query, 'корабль');
  await f.api.handleClick('reference-next', null, f.C, f.ui);
  const html = f.api.render(f.C, f.ui);
  assert.match(html, /49–50/); assert.equal((html.match(/class="ref-card /g) || []).length, 2);
  assert.match(html, /data-click="reference-next"[^>]*disabled/);
  const count = f.calls.length; await f.api.handleClick('reference-next', null, f.C, f.ui); assert.equal(f.calls.length, count);
  await f.api.handleClick('reference-prev', null, f.C, f.ui); assert.equal(f.calls.at(-1).params.offset, 24);
});

test('opening an entity loads a complete page with hero, properties, requirements and related navigation', async () => {
  const f = fixture(); await f.api.init(f.C, f.ui);
  await f.api.handleClick('reference-open', { dataset: { id: 'ship/test' } }, f.C, f.ui);
  const html = f.api.render(f.C, f.ui);
  assert.equal(f.calls.at(-1).op, 'reference.entry');
  assert.match(html, /data-reference="entry"/); assert.match(html, /<h1>Тестовый корабль<\/h1>/);
  assert.match(html, /ref-art-hero/); assert.match(html, /<dt>Груз<\/dt><dd>0<\/dd>/);
  assert.match(html, /ref-requirements/); assert.match(html, /ref-entry-toc/);
  assert.match(html, /data-id="module\/pp"/); assert.doesNotMatch(html, /class="inspector"|role="dialog"/);
});

test('related pages retain reading history and return to the same filtered catalogue and scroll position', async () => {
  const f = fixture({ autoInit: true }); await f.api.init(f.C, f.ui);
  f.host.scrollTop = 185;
  await f.api.open(f.C, f.ui, 'ship/test');
  f.host.scrollTop = 360;
  await f.api.open(f.C, f.ui, 'module/pp');
  assert.equal(f.C.reference.history.length, 1);
  await f.api.handleClick('reference-back', null, f.C, f.ui);
  assert.equal(f.C.reference.entry.id, 'ship/test'); assert.equal(f.host.scrollTop, 360);
  await f.api.handleClick('reference-library', null, f.C, f.ui);
  assert.equal(f.C.reference.entryId, ''); assert.equal(f.host.scrollTop, 185);
  assert.equal(f.C.reference.category, 'ships');
  assert.equal(f.calls.filter(call => call.op === 'reference.list').length, 1);
});

test('late entity replies cannot replace a more recently opened page', async () => {
  const f = fixture(), first = deferred(), second = deferred();
  await f.api.init(f.C, f.ui);
  f.setRPC((op, params) => params.id === 'ship/first' ? first.promise : second.promise);
  const loadingFirst = f.api.open(f.C, f.ui, 'ship/first'); await microtasks();
  const loadingSecond = f.api.open(f.C, f.ui, 'ship/second'); await microtasks();
  second.resolve({ entry: { ...entry('ship/second'), title: 'Вторая страница' } }); await loadingSecond;
  first.resolve({ entry: { ...entry('ship/first'), title: 'Первая страница' } }); await loadingFirst;
  assert.match(f.api.render(f.C, f.ui), /Вторая страница/);
  assert.doesNotMatch(f.api.render(f.C, f.ui), /<h1>Первая страница/);
});

test('catalogue and entry errors are escaped, remain recoverable and never create an automatic retry loop', async () => {
  const f = fixture({ autoInit: true });
  f.setRPC(async () => { throw new Error('<img src=x> Ошибка чтения https://example.com/private'); });
  await f.api.init(f.C, f.ui); await microtasks();
  assert.equal(f.calls.length, 1);
  const html = f.api.render(f.C, f.ui);
  assert.match(html, /role="alert"/); assert.match(html, /&lt;img src=x&gt;/);
  assert.doesNotMatch(html, /<img src=x>|https:\/\//);
  f.setRPC(async () => catalogue());
  await f.api.handleClick('reference-retry', null, f.C, f.ui);
  assert.equal(f.calls.length, 2); assert.equal(f.C.reference.error, '');
  f.setRPC(async () => { throw new Error('Запись недоступна'); });
  await f.api.open(f.C, f.ui, 'ship/test');
  assert.match(f.api.render(f.C, f.ui), /data-click="reference-retry-entry"/);
  f.setRPC(async () => ({ entry: entry() }));
  await f.api.handleClick('reference-retry-entry', null, f.C, f.ui);
  assert.match(f.api.render(f.C, f.ui), /<h1>Тестовый корабль/);
});

test('empty search offers a clear action instead of a misleading empty detail pane', async () => {
  const f = fixture(); f.setRPC(async () => catalogue([])); await f.api.init(f.C, f.ui);
  f.C.reference.query = 'нет такого корабля';
  assert.match(f.api.render(f.C, f.ui), /Ничего не найдено/);
  assert.match(f.api.render(f.C, f.ui), /data-click="reference-clear"/);
  await f.api.handleClick('reference-clear', null, f.C, f.ui);
  assert.equal(f.C.reference.query, ''); assert.equal(f.C.reference.group, ''); assert.equal(f.C.reference.offset, 0);
});

test('untrusted entity text is escaped, external source text is omitted and only embedded images are accepted', async () => {
  const f = fixture();
  f.setRPC(async () => ({ entry: { id: 'article/test', category: 'articles', title: '<img src=x onerror=bad()>', description: 'Текст https://example.com/source', image: { src: 'javascript:bad()' }, sourceURLs: ['https://hidden.example.com'], sections: [
    { id: 'body', type: 'prose', title: '<script>bad()</script>', paragraphs: ['<b>Текст</b>'] },
    { id: 'sources', type: 'prose', title: 'Подробнее', paragraphs: ['Не показывать этот блок'] },
    { id: 'meta', type: 'references', title: 'References', paragraphs: ['Скрытые ссылки'] },
  ] } }));
  await f.api.open(f.C, f.ui, 'article/test');
  const html = f.api.render(f.C, f.ui);
  assert.match(html, /&lt;img/); assert.match(html, /&lt;script&gt;/); assert.match(html, /&lt;b&gt;Текст/);
  assert.doesNotMatch(html, /<script>|<img src=x|javascript:|https:\/\/|Не показывать|Скрытые ссылки|sourceURLs/);
});

test('grouped blueprint pages expose complete grade recipes with material and effect links', async () => {
  const f = fixture();
  f.setRPC(async () => ({ entry: { id: 'blueprint/long-range', category: 'blueprints', title: 'Увеличенная дальность', sections: [
    { id: 'module-fsd', type: 'grades', title: 'Рамочный двигатель', grades: [
      { grade: 1, title: 'Первый уровень', materials: [{ name: 'Мышьяк', count: 1, entryId: 'material/arsenic' }], effects: [{ label: 'Дальность', value: '+10%' }] },
      { grade: 2, materials: [{ name: 'Датамайн', count: 2, entryId: 'material/data' }], effects: [{ label: 'Масса', value: '+5%' }] },
    ] },
  ] } }));
  await f.api.open(f.C, f.ui, 'blueprint/long-range');
  const html = f.api.render(f.C, f.ui);
  assert.equal((html.match(/<details class="ref-grade"/g) || []).length, 2);
  assert.match(html, /Потребуется на одну попытку/); assert.match(html, /\+10%/);
  assert.match(html, /data-id="material\/arsenic"/); assert.match(html, /× 2/);
});

test('commodity buy and sell actions navigate to trade with the exact resource already selected', async () => {
  const f = fixture();
  f.setRPC(async () => ({ entry: { id: 'commodity/platinum', category: 'commodities', title: 'Платина', commodityKey: 'platinum', sections: [] } }));
  await f.api.open(f.C, f.ui, 'commodity/platinum');
  const html = f.api.render(f.C, f.ui);
  assert.match(html, /Покупка и продажа/); assert.match(html, /Купить/); assert.match(html, /Найти покупателей/);
  await f.api.handleClick('reference-trade-buy', { dataset: { key: 'platinum' } }, f.C, f.ui);
  await f.api.handleClick('reference-trade-sell', { dataset: { key: 'platinum' } }, f.C, f.ui);
  assert.deepEqual(f.navigation, [
    { id: 'sale', opts: { mode: 'Купить', prefill: { resourceKey: 'platinum' } } },
    { id: 'sale', opts: { mode: 'Продать', prefill: { resourceKey: 'platinum' } } },
  ]);
});

test('engineering engineers use their own catalogue session and still open related full entity pages', async () => {
  const f = fixture(); await f.api.init(f.C, f.ui);
  f.C.reference.query = 'корабль'; f.C.tab = 'engineering'; f.C.mode = 'Инженеры';
  await f.api.init(f.C, f.ui);
  assert.equal(f.calls.at(-1).params.category, 'engineers');
  assert.doesNotMatch(f.api.render(f.C, f.ui), /ref-category-nav/);
  const calls = f.calls.length;
  assert.equal(await f.api.setCategory(f.C, f.ui, 'ships'), false); assert.equal(f.calls.length, calls);
  await f.api.open(f.C, f.ui, 'module/pp'); assert.match(f.api.render(f.C, f.ui), /data-reference="entry"/);
  f.C.tab = 'encyclopedia'; f.C.mode = 'Корабли'; f.api.render(f.C, f.ui);
  assert.equal(f.C.reference.query, 'корабль'); assert.equal(f.C.reference.category, 'ships');
});

test('the image fallback handler binds once and unrelated root-forwarded events do nothing', async () => {
  const f = fixture(); await f.api.init(f.C, f.ui); await f.api.init(f.C, f.ui);
  assert.equal(f.handlers.get('error').length, 1);
  assert.equal(f.handlers.has('input'), false); assert.equal(f.handlers.has('change'), false);
  const fallback = { hidden: true }, image = { hidden: false, matches: selector => selector === '[data-reference-image]', parentElement: { querySelector: () => fallback } };
  f.handlers.get('error')[0]({ target: image });
  assert.equal(image.hidden, true); assert.equal(fallback.hidden, false);
  const calls = f.calls.length;
  assert.equal(f.api.handleInput({ target: { id: 'other', value: 'x' } }, f.C, f.ui), false);
  assert.equal(await f.api.handleChange({ target: { id: 'other' } }, f.C, f.ui), false);
  assert.equal(await f.api.handleClick('unrelated-action', null, f.C, f.ui), false);
  assert.equal(f.calls.length, calls);
});

test('async search redraw preserves the focused query and its caret', async () => {
  const f = fixture(); let focuses = 0, caret;
  const input = { matches: selector => selector === ':focus', selectionStart: 4, selectionEnd: 4, focus() { focuses++; }, setSelectionRange(start, end) { caret = [start, end]; } };
  f.nodes.set('#reference-query', input);
  await f.api.init(f.C, f.ui);
  assert.ok(focuses >= 1); assert.deepEqual(caret, [4, 4]);
});

test('public deep links accept the root ID-first contract and system copy is an explicit native action', async () => {
  const f = fixture();
  f.setRPC(async op => op === 'native' ? { ok: true } : { id: 'system/Sol', category: 'systems', title: 'Sol', properties: [{ label: 'Инженер', value: 'Тест', entryId: 'engineer/300100' }], sections: [], actions: [{ type: 'copy-system', system: 'Sol', label: 'Копировать систему' }] });
  await f.api.open('system/Sol', f.C, f.ui);
  const html = f.api.render(f.C, f.ui);
  assert.match(html, /<h1>Sol/); assert.match(html, /data-click="reference-copy-system"/);
  assert.match(html, /data-id="engineer\/300100"/);
  await f.api.handleClick('reference-copy-system', { dataset: { index: '0' } }, f.C, f.ui);
  assert.deepEqual(f.calls.at(-1), { op: 'native', params: { action: 'copy', text: 'Sol' } });
});

test('targets engineers are active reference screens and show the section mode controls', async () => {
  const f = fixture(); f.C.tab = 'targets'; f.C.mode = 'Инженеры';
  f.ui.renderModes = segmentsOnly => segmentsOnly ? '<nav data-test-modes>Личные цели · Инженеры</nav>' : '';
  assert.equal(f.api.active(f.C), true);
  await f.api.init(f.C, f.ui);
  assert.equal(f.calls.at(-1).params.category, 'engineers');
  assert.match(f.api.render(f.C, f.ui), /data-test-modes/);
});

test('quiet renders and ordinary navigation preserve catalogue filters while explicit category links change them', async () => {
  const f = fixture(); await f.api.init(f.C, f.ui);
  f.C.reference.query = 'дальний'; f.C.reference.group = 'travel'; f.C.reference.offset = 24;
  f.api.render(f.C, f.ui); f.api.navigate('encyclopedia', {}, f.C);
  assert.equal(f.C.reference.query, 'дальний'); assert.equal(f.C.reference.offset, 24);
  f.api.navigate('encyclopedia', { mode: 'Материалы' }, f.C);
  assert.equal(f.C.reference.category, 'materials'); assert.equal(f.C.reference.query, ''); assert.equal(f.C.reference.offset, 0);
});

test('personal engineering data distinguishes unknown and zero stocks and annotates full grade recipes', async () => {
  const f = fixture();
  const personal = { engineers: [{ key: '300100', status: 'unlocked', rank: 4 }], materials: [{ key: 'arsenic', owned: 0 }, { key: 'iron', owned: 5 }] };
  f.setRPC(async (op, params) => ({ ...entry(params.id), id: params.id, category: params.id.startsWith('engineer/') ? 'engineers' : params.id.startsWith('material/') ? 'materials' : 'blueprints', personal, sections: [{ id: 'grades', type: 'grades', title: 'Улучшение', grades: [{ grade: 1, materials: [{ name: 'Железо', count: 1, entryId: 'material/iron' }], effects: [] }] }] }));
  await f.api.open('engineer/300100', f.C, f.ui);
  assert.match(f.api.render(f.C, f.ui), /Доступен/); assert.match(f.api.render(f.C, f.ui), /ранг 4/);
  await f.api.open('material/arsenic', f.C, f.ui);
  assert.match(f.api.render(f.C, f.ui), /В вашем запасе: 0/);
  await f.api.open('material/unknown', f.C, f.ui);
  assert.match(f.api.render(f.C, f.ui), /Личный запас пока неизвестен/);
  assert.match(f.api.render(f.C, f.ui), /Есть 5/); assert.match(f.api.render(f.C, f.ui), /ref-material is-enough/);
});

test('commodity market search is explicit, displays real quote fields and ignores another commodity search', async () => {
  const f = fixture({ autoInit: true }); f.C.state.commander = { system: 'Sol' };
  let searched = false;
  f.setRPC(async (op, params) => {
    if (op === 'reference.entry') return { id: 'commodity/platinum', category: 'commodities', title: 'Платина', commodityKey: 'platinum', sections: [] };
    if (op === 'reference.market-search') { searched = true; return { ok: true, started: true }; }
    if (op === 'reference.markets') return searched ? { running: false, options: { resourceKey: 'platinum', mode: 'sell' }, results: [{ station: 'Тестовая станция', system: 'Alpha', distanceLy: 12.5, updatedAt: '2026-11-08T14:30:00Z', quotes: [{ key: 'platinum', price: 250000, demand: 0, supply: 10 }] }] } : { running: false, options: { resourceKey: 'gold', mode: 'sell' }, results: [{ station: 'Чужой поиск', quotes: [{ key: 'gold', price: 1 }] }] };
    return catalogue();
  });
  await f.api.open('commodity/platinum', f.C, f.ui); await microtasks();
  assert.equal(f.calls.filter(call => call.op === 'reference.market-search').length, 0);
  assert.doesNotMatch(f.api.render(f.C, f.ui), /Чужой поиск/);
  f.api.handleInput({ target: { id: 'reference-market-quantity', value: '20' } }, f.C, f.ui);
  f.api.handleInput({ target: { id: 'reference-market-radius', value: '100,5' } }, f.C, f.ui);
  await f.api.handleClick('reference-find-markets', null, f.C, f.ui);
  const request = f.calls.find(call => call.op === 'reference.market-search');
  assert.equal(request.params.values.resourceKey, 'platinum'); assert.equal(request.params.values.mode, 'sell');
  assert.equal(request.params.values.system, 'Sol'); assert.equal(request.params.values.quantity, 20); assert.equal(request.params.values.radiusLy, 100.5);
  const html = f.api.render(f.C, f.ui);
  assert.match(html, /Тестовая станция/); assert.match(html, /250000/); assert.match(html, /12.5/); assert.match(html, /08.11.2026 14:30 UTC/);
  assert.match(html, /<td>0<\/td>/);
});

test('invalid commodity search stays local and active market polling stops when leaving the entry', async () => {
  const f = fixture({ autoInit: true });
  f.setRPC(async op => op === 'reference.entry' ? { id: 'commodity/platinum', category: 'commodities', title: 'Платина', commodityKey: 'platinum', sections: [] } : op === 'reference.markets' ? { running: true, options: { resourceKey: 'platinum', mode: 'sell' }, results: [] } : { ok: true });
  await f.api.open('commodity/platinum', f.C, f.ui); await microtasks();
  f.C.reference.market.searching = false;
  await f.api.handleClick('reference-find-markets', null, f.C, f.ui);
  assert.equal(f.calls.filter(call => call.op === 'reference.market-search').length, 0);
  assert.match(f.api.render(f.C, f.ui), /Укажите исходную систему/);
  await f.api.handleClick('reference-library', null, f.C, f.ui);
  const count = f.calls.filter(call => call.op === 'reference.markets').length;
  await f.flushTimers();
  assert.equal(f.calls.filter(call => call.op === 'reference.markets').length, count);
});

test('the all-category search is bounded and a selected group remains visible when a query has no matches', async () => {
  const f = fixture(); await f.api.init(f.C, f.ui);
  assert.match(f.api.render(f.C, f.ui), /data-id="all"/);
  await f.api.setCategory(f.C, f.ui, 'all');
  assert.equal(f.calls.at(-1).params.category, 'all'); assert.equal(f.calls.at(-1).params.limit, 24);
  f.C.reference.group = 'travel'; f.C.reference.groupName = 'Перелёты'; f.C.reference.list = { entries: [], total: 0, groups: [], categories: categoryRows };
  const html = f.api.render(f.C, f.ui);
  assert.match(html, /value="travel" selected>Перелёты/);
});

test('experimental installation recipes do not invent a grade zero', async () => {
  const f = fixture();
  f.setRPC(async () => ({ id: 'experimental/test', category: 'experimentals', title: 'Экспериментальный эффект', sections: [{ id: 'recipe', title: 'Рамочный двигатель', type: 'grades', grades: [{ grade: 0, title: 'Установка эффекта', materials: [{ name: 'Железо', count: 1, entryId: 'material/iron' }] }] }] }));
  await f.api.open('experimental/test', f.C, f.ui);
  const html = f.api.render(f.C, f.ui);
  assert.match(html, /Установка эффекта/); assert.doesNotMatch(html, /уровня 0|ref-grade-number">0/);
});

test('a hidden reference screen pauses local market polling until it is shown again', async () => {
  const f = fixture({ autoInit: true });
  f.host.ownerDocument = { hidden: false };
  f.setRPC(async op => op === 'reference.entry' ? { id: 'commodity/platinum', category: 'commodities', title: 'Платина', commodityKey: 'platinum', sections: [] } : { running: true, options: { resourceKey: 'platinum', mode: 'sell' }, results: [] });
  await f.api.open('commodity/platinum', f.C, f.ui); await microtasks();
  const calls = f.calls.length;
  f.host.ownerDocument.hidden = true; await f.flushTimers();
  assert.equal(f.calls.length, calls); assert.equal(f.C.reference.market.needsRefresh, true);
  f.host.ownerDocument.hidden = false; await f.api.init(f.C, f.ui); await microtasks();
  assert.equal(f.calls.length, calls + 1);
});
