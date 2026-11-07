(function () {
  'use strict';
  const U = window.MiningUI, { h, n, pill, dateTime, credits } = U;
  let context;
  const kinds = { travel: 'Перелёт', trade: 'Торговля', mining: 'Добыча', engineering: 'Инженерия', exploration: 'Исследования', combat: 'Боевые задачи', mission: 'Поручение', carrier: 'Флотоносец', odyssey: 'На поверхности', other: 'Другое' };
  const ui = { scope: 'plan', status: 'active', kind: 'all', query: '', formOpen: false, draft: {}, engineerFilter: 'planned' };
  const button = (label, action, value = '') => '<button class="button button-small button-subtle" type="button" data-target-action="' + action + '" data-value="' + h(value) + '">' + h(label) + '</button>';
  const field = (label, content) => '<label class="field"><span>' + h(label) + '</span>' + content + '</label>';
  const options = (items, selected) => Object.entries(items).map(([value, text]) => '<option value="' + h(value) + '"' + (value === selected ? ' selected' : '') + '>' + h(text) + '</option>').join('');
  const panel = (title, body, aside = '') => '<section class="panel"><div class="panel-heading"><h2>' + h(title) + '</h2>' + aside + '</div>' + body + '</section>';
  const empty = (text) => '<div class="panel-body"><p class="panel-note">' + h(text) + '</p></div>';
  function input(key, label, extra = '') { return field(label, '<input name="' + key + '" data-target-draft="' + key + '" data-focus="objective-' + key + '" value="' + h(ui.draft[key] || '') + '" ' + extra + '>'); }
  function goalForm() {
    if (!ui.formOpen) return '';
    return panel(ui.draft.id ? 'Изменить цель' : 'Новая личная цель', '<form id="objective-form" class="panel-body"><div class="form-grid pilot-target-form">' +
      input('title', 'Что сделать', 'required maxlength="120" placeholder="Например, улучшить двигатели Python"') +
      field('Занятие', '<select name="kind" data-target-draft="kind" data-focus="objective-kind">' + options(kinds, ui.draft.kind || 'travel') + '</select>') +
      input('system', 'Система', 'maxlength="150" placeholder="Если цель связана с местом"') + input('station', 'Станция или база', 'maxlength="150"') + input('body', 'Тело или участок', 'maxlength="150"') +
      field('Приоритет', '<select name="priority" data-target-draft="priority" data-focus="objective-priority">' + options({ normal: 'Обычный', high: 'Важный' }, ui.draft.priority || 'normal') + '</select>') +
      '</div>' + field('Заметки', '<textarea name="notes" data-target-draft="notes" data-focus="objective-notes" rows="3" maxlength="1200" placeholder="Что взять с собой, нужные товары, оснащение или условия">' + h(ui.draft.notes || '') + '</textarea>') +
      '<div class="actions-wrap"><button type="submit" class="button">' + (ui.draft.id ? 'Сохранить цель' : 'Добавить цель') + '</button>' + button('Отменить', 'cancel-form') + '</div><p class="panel-note">Это ваш список дел. Отметка выполнения не меняет поручения или маршрут в игре.</p></form>');
  }
  function goalCard(goal) {
    return '<article class="pilot-goal"><div><div class="pill-row">' + pill(kinds[goal.kind] || 'Цель', 'blue') +
      (goal.priority === 'high' ? pill('Важная', 'potential') : '') + (goal.status === 'completed' ? pill('Выполнена', 'confirmed') : '') + '</div><h3>' + h(goal.title) + '</h3><p>' + h([goal.system, goal.station, goal.body].filter(Boolean).join(' · ')) + '</p>' +
      (goal.notes ? '<p class="pilot-goal-notes">' + h(goal.notes) + '</p>' : '') + '</div><div class="actions-wrap">' +
      (goal.system ? button('Копировать систему', 'copy', goal.system) : '') + button('Изменить', 'edit', goal.id) +
      button(goal.status === 'completed' ? 'Вернуть в работу' : 'Выполнено', goal.status === 'completed' ? 'reopen' : 'complete', goal.id) + button('Убрать', 'remove', goal.id) + '</div></article>';
  }
  function renderGoals(state) {
    const rows = (state.objectives || []).filter((row) => (ui.status === 'all' || row.status === ui.status) && (ui.kind === 'all' || row.kind === ui.kind) &&
      (!ui.query || [row.title, row.system, row.station, row.body, row.notes].join(' ').toLowerCase().includes(ui.query.toLowerCase())))
      .sort((a, b) => Number(b.priority === 'high') - Number(a.priority === 'high') || String(b.createdAt || '').localeCompare(String(a.createdAt || '')));
    return panel('Личный план', '<div class="panel-body pilot-target-filters">' +
      field('Найти цель', '<input type="search" data-target-field="query" data-focus="objective-query" value="' + h(ui.query) + '" placeholder="Задача, система или заметка">') +
      field('Состояние', '<select data-target-field="status" data-focus="objective-status">' + options({ active: 'В работе', completed: 'Выполненные', all: 'Все' }, ui.status) + '</select>') +
      field('Занятие', '<select data-target-field="kind" data-focus="objective-filter-kind">' + options({ all: 'Все занятия', ...kinds }, ui.kind) + '</select>') + '</div>' +
      (rows.length ? '<div class="pilot-goal-list">' + rows.map(goalCard).join('') + '</div>' : empty('Пока нет целей в этой выборке. Добавьте перелёт, покупку, посещение инженера, исследование или любую другую задачу.')),
      button('Добавить цель', 'show-form'));
  }
  function navigation(state) {
    const pilot = state.pilot || {}, target = pilot.navTarget;
    const route = (pilot.navRoute || []).filter((row) => !row.visited);
    return panel('Маршрут корабля из игры', target?.system ? '<div class="panel-body"><p class="eyebrow">СЛЕДУЮЩАЯ СИСТЕМА</p><h3>' + h(target.system) + '</h3><p class="panel-note">Осталось прыжков: ' + n(target.remainingJumps ?? (route.length || null), true) + '</p><div class="actions-wrap">' + button('Копировать систему', 'copy', target.system) +
      button('Добавить в личный план', 'nav-goal') + '</div>' + (route.length ? '<ol class="pilot-nav-stops">' + route.slice(0, 8).map((row) => '<li><span>' + h(row.system) + '</span>' + (row.starClass ? '<small>' + h(row.starClass) + '</small>' : '') + '</li>').join('') + '</ol>' + (route.length > 8 ? '<p class="panel-note">Дальше ещё ' + n(route.length - 8, true) + ' систем.</p>' : '') : '') + '</div>' : empty('Проложите маршрут на карте галактики или выберите следующую систему в игре. Здесь появится известное назначение. Для отдельного плана флотоносца используйте «Маршрут».'));
  }
  function missions(state) {
    const rows = state.pilot?.missions || [];
    return panel('Поручения из журнала', rows.length ? '<div class="pilot-goal-list">' + rows.map((row) => '<article class="pilot-goal"><div><div class="pill-row">' + pill(row.expired ? 'Срок истёк' : row.status === 'ready' ? 'Готово к сдаче' : 'В работе', row.expired ? 'potential' : 'blue') + '</div><h3>' + h(row.name) + '</h3><p>' + h([row.destinationSystem, row.destinationStation].filter(Boolean).join(' · ') || 'Назначение в доступном журнале не указано') + '</p><small>Срок: ' + dateTime(row.expiry) + ' · награда: ' + credits(row.reward) + '</small></div><div class="actions-wrap">' +
      (row.destinationSystem ? button('Копировать систему', 'copy', row.destinationSystem) : '') + button('В личный план', 'mission-goal', row.id) + '</div></article>').join('') + '</div>' : empty('Активные поручения появятся после соответствующих записей игры. Личные задачи можно добавить независимо от них.'));
  }
  function renderPlan(state) {
    const next = state.carrierRoute?.route?.stops?.find((row) => !row.visited && row.index > 0);
    const market = state.trading?.destination;
    return '<div class="stack">' + goalForm() + '<div class="pilot-target-columns"><div class="stack">' + renderGoals(state) + missions(state) + '</div><aside class="stack">' + navigation(state) +
      (next ? panel('План флотоносца', '<div class="panel-body"><h3>' + h(next.name) + '</h3><p class="panel-note">Следующая точка · расход по плану ' + n(next.fuelUsedTons) + ' т трития.</p><a class="button button-small button-subtle" href="#route">Открыть маршрут флотоносца</a></div>') : '') +
      panel('Выбранный рынок', market ? '<div class="panel-body"><h3>' + h(market.station) + '</h3><p class="panel-note">' + h(market.system) + ' · ' + credits(market.estimatedTotal) + '</p>' +
        (market.expired ? pill('Обновите котировку', 'potential') : '') + '<p class="panel-note">Наблюдение: ' + dateTime(market.updatedAt) + '</p><a class="button button-small button-subtle" href="#sale">Проверить рынок и груз</a></div>' : empty('В «Торговле» найдите рынок и выберите, куда везти груз.')) + '</aside></div></div>';
  }
  function renderEngineering(state) {
    const eng = state.engineering || {}, items = eng.items || [], all = eng.engineers || [];
    const needed = (eng.materials || []).filter((m) => m.needed > 0 && (m.missing > 0 || m.owned == null));
    const selected = new Set(items.flatMap((item) => item.engineerKeys || []));
    const engineers = all.filter((e) => ui.engineerFilter !== 'planned' || !items.length || selected.has(e.key)).sort((a, b) => Number(b.status === 'unlocked') - Number(a.status === 'unlocked'));
    return '<div class="pilot-target-columns"><div class="stack">' + panel('Материалы для запланированных улучшений', needed.length ? '<div class="table-wrap"><table><thead><tr><th>Материал</th><th class="table-num">Есть</th><th class="table-num">Не хватает</th></tr></thead><tbody>' + needed.map((row) => '<tr><td>' + h(row.name) + '<small class="engineering-english">G' + h(row.grade) + ' · ' + h(row.categoryName || '') + '</small></td><td class="table-num">' + n(row.owned, true) + '</td><td class="table-num">' + n(row.missing, true) + '</td></tr>').join('') + '</tbody></table></div>' : empty(items.length ? 'Известного запаса достаточно для текущего списка улучшений.' : 'Добавьте нужные улучшения в «Инженерии»: здесь появятся материалы и подходящие инженеры.'), '<a class="soft-link" href="#engineering">Открыть план →</a>') +
      panel('Кого посетить', '<div class="panel-body">' + field('Инженеры', '<select data-target-field="engineerFilter" data-focus="target-engineer-filter">' + options({ planned: 'Для выбранных чертежей', all: 'Все инженеры' }, ui.engineerFilter) + '</select>') + '</div><div class="pilot-goal-list">' + engineers.map((engineer) => '<article class="pilot-goal"><div><div class="pill-row">' + pill(engineer.status === 'unlocked' ? 'Открыт' : 'Проверьте доступ', engineer.status === 'unlocked' ? 'confirmed' : 'muted') + '</div><h3>' + h(engineer.name) + '</h3><p>' + h([engineer.system, engineer.base].filter(Boolean).join(' · ')) + '</p></div><div class="actions-wrap">' + button('Копировать систему', 'copy', engineer.system) + button('Запланировать посещение', 'engineer-goal', engineer.key) + '</div></article>').join('') + '</div>') + '</div><aside class="stack">' +
      panel('Как подготовиться', '<div class="panel-body"><p class="panel-note">Подходящие инженеры выбраны по максимальной ступени каждого чертежа в вашем плане. Доступ проверяется отдельно: запас материалов сам по себе не открывает инженера.</p><div class="actions-wrap"><a class="button button-small button-subtle" href="#engineering">Улучшения и обмены</a><button class="button button-small button-subtle" data-help-topic="engineering">Помощь по инженерии</button></div></div>') + '</aside></div>';
  }
  const activities = [
    ['travel', 'Полёт и навигация', 'Маршрут корабля, перелёт к нужной системе и подготовка перед дорогой.', 'navigation', 'plan'],
    ['trade', 'Торговля и перевозки', 'Сравните рынки, спрос и запас либо найдите прибыльный торговый рейс.', 'trade', 'sale'],
    ['exploration', 'Исследования', 'Изучайте системы, картографируйте планеты и сохраняйте места для посещения.', 'exploration', 'plan'],
    ['combat', 'Боевые задачи', 'Подготовьте оснащение и место вылета, проверьте законность целей и последствия боя.', 'combat', 'plan'],
    ['engineering', 'Инженерные улучшения', 'Соберите материалы, рассчитайте обмены и запланируйте посещение инженера.', 'engineering', 'engineering'],
    ['carrier', 'Флотоносец и снабжение', 'Топливо, склад, перевозки и экспедиционный маршрут личного флотоносца.', 'carrier', 'carrier'],
    ['odyssey', 'Задачи на поверхности', 'Пешие поручения, снаряжение, поселения и поездки на ТРП.', 'odyssey', 'plan'],
    ['mining', 'Добыча ресурсов', 'Кольца, астероиды, участки на планетах и сбор сырья для улучшений.', 'mining', 'mining'],
  ];
  function renderDiscovery() {
    return '<div class="pilot-activity-grid">' + activities.map(([kind, title, text, section, destination]) => '<article class="panel"><div class="panel-body"><h2>' + h(title) + '</h2><p>' + h(text) + '</p><div class="actions-wrap">' + button('Добавить задачу', 'kind-goal', kind) + button('Что нужно знать', 'encyclopedia', section) +
      (['sale', 'carrier'].includes(destination) ? '<a href="#' + destination + '" class="button button-small button-subtle">Открыть инструмент</a>' : destination === 'mining' || destination === 'engineering' ? button('Открыть инструмент', 'tab', destination) : '') + '</div></div></article>').join('') + '</div>';
  }
  function render(state) {
    const tabs = { plan: 'План и поручения', engineering: 'Инженеры и материалы', mining: 'Места добычи', discovery: 'Другие занятия' };
    const description = ui.scope === 'mining' ? 'Поиск ресурсов — один из инструментов общего плана. Сохраняйте места и связывайте их со своими задачами.' : 'Личные задачи, назначения из игры и подготовка к разным занятиям. Выбор панелей оверлея не ограничивает этот план.';
    return '<div class="page-heading"><div><p class="eyebrow">ВАШ СЛЕДУЮЩИЙ ШАГ</p><h1>Цели и планы</h1><p>' + h(description) + '</p></div><div class="heading-actions">' + button('Добавить цель', 'show-form') + '</div></div>' +
      '<div class="engineering-tabs" role="tablist" aria-label="Виды целей">' + Object.entries(tabs).map(([key, text]) => '<button class="button ' + (ui.scope === key ? 'button-primary' : 'button-subtle') + '" type="button" role="tab" aria-selected="' + (ui.scope === key) + '" data-target-action="tab" data-value="' + key + '">' + h(text) + '</button>').join('') + '</div>' +
      ({ plan: renderPlan, engineering: renderEngineering, mining: () => context.renderMiningTargets(), discovery: renderDiscovery }[ui.scope] || renderPlan)(state);
  }
  function open(scope = 'plan', draft) {
    ui.scope = ['plan', 'engineering', 'mining', 'discovery'].includes(scope) ? scope : 'plan';
    if (draft) { ui.draft = { ...draft }; ui.formOpen = true; ui.scope = 'plan'; }
    if (location.hash !== '#targets') location.hash = 'targets'; else context?.render();
  }
  function init(api) {
    context = api;
    document.addEventListener('input', (event) => {
      if (event.target.dataset.targetDraft) ui.draft[event.target.dataset.targetDraft] = event.target.value;
      if (event.target.dataset.targetField) { ui[event.target.dataset.targetField] = event.target.value; context.render(); }
    });
    document.addEventListener('submit', async (event) => {
      if (event.target.id !== 'objective-form') return;
      event.preventDefault();
      const body = { ...ui.draft, ...Object.fromEntries(new FormData(event.target)), action: ui.draft.id ? 'update' : 'add' };
      if (await context.perform('/api/objectives', body, 'Личный план сохранён')) { ui.draft = {}; ui.formOpen = false; context.render(); }
    });
    document.addEventListener('click', async (event) => {
      const target = event.target.closest('[data-target-action]'); if (!target) return;
      const action = target.dataset.targetAction, value = target.dataset.value, state = context.getState();
      if (action === 'tab') { ui.scope = value; context.render(); }
      else if (action === 'show-form') { ui.scope = 'plan'; ui.formOpen = true; context.render(); document.getElementById('objective-form')?.scrollIntoView({ block: 'start' }); }
      else if (action === 'cancel-form') { ui.formOpen = false; ui.draft = {}; context.render(); }
      else if (action === 'copy') { await U.copyText(value); context.showToast('Система скопирована'); }
      else if (action === 'edit') { const row = state.objectives?.find((row) => row.id === value); if (row) open('plan', row); }
      else if (['complete', 'reopen'].includes(action)) await context.perform('/api/objectives', { action: 'status', id: value, status: action === 'complete' ? 'completed' : 'active' });
      else if (action === 'remove') await context.perform('/api/objectives', { action: 'remove', id: value });
      else if (action === 'kind-goal') open('plan', { kind: value });
      else if (action === 'encyclopedia') window.CopilotEncyclopedia.open(value);
      else if (action === 'nav-goal') open('plan', { kind: 'travel', title: 'Перелёт в ' + state.pilot.navTarget.system, system: state.pilot.navTarget.system });
      else if (action === 'mission-goal') { const row = state.pilot?.missions?.find((row) => row.id === value); if (row) open('plan', { kind: 'mission', title: row.name, system: row.destinationSystem, station: row.destinationStation, notes: 'Поручение из игры. Срок: ' + (row.expiry ? new Date(row.expiry).toLocaleString('ru-RU') : 'неизвестен') }); }
      else if (action === 'engineer-goal') { const row = state.engineering?.engineers?.find((row) => row.key === value); if (row) open('plan', { kind: 'engineering', title: 'Посетить: ' + row.name, system: row.system, station: row.base }); }
    });
  }
  window.CopilotTargets = { init, render, open };
})();
