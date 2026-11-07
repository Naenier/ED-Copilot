(function () {
  "use strict";
  const U = window.MiningUI;
  const { h, n, pill, dateTime } = U;
  let context;
  const ui = { tab: "list", draft: null, materialQuery: "", category: "all", shortagesOnly: true,
    engineerQuery: "", importText: "", tradeDraft: {}, importOpen: false };
  const categories = { raw: "Сырьё", manufactured: "Промышленные материалы", encoded: "Данные" };
  const b = (text, action, value = "", extra = "") => '<button type="button" class="button button-small button-subtle" data-engineering-action="' + action + '" data-value="' + h(value) + '" ' + extra + '>' + h(text) + '</button>';
  const field = (label, control) => '<label class="field"><span>' + h(label) + '</span>' + control + '</label>';
  const options = (items, selected) => items.map(([value, text]) => '<option value="' + h(value) + '"' + (String(value) === String(selected) ? ' selected' : '') + '>' + h(text) + '</option>').join('');
  const panel = (title, body, note = '') => '<section class="panel"><div class="panel-heading"><h2>' + h(title) + '</h2>' + note + '</div>' + body + '</section>';
  const note = (text) => '<p class="panel-note">' + h(text) + '</p>';
  const select = (key, label, items, value, scope = 'draft') => field(label, '<select data-engineering-field="' + key + '" data-engineering-scope="' + scope + '" data-focus="engineering-' + key + '" name="' + key + '">' + options(items, value) + '</select>');
  const input = (key, label, value, type = 'number', attrs = '', scope = 'draft') => field(label, '<input data-engineering-field="' + key + '" data-engineering-scope="' + scope + '" data-focus="engineering-' + key + '" name="' + key + '" value="' + h(value ?? '') + '" type="' + type + '" ' + attrs + '>');
  function data(state) { return state.engineering || { catalog: { modules: [], blueprints: [], experimentals: [], materials: [] }, items: [], materials: [], shopping: {}, trader: { offers: [] }, engineers: [] }; }
  function draftFor(eng) {
    const modules = eng.catalog?.modules || [];
    if (!ui.draft) {
      const module = modules[0];
      const blueprint = eng.catalog?.blueprints?.find((bp) => bp.key === module?.blueprints?.[0]);
      ui.draft = { moduleKey: module?.key || '', blueprintKey: blueprint?.key || '', experimentalKey: '', quantity: 1,
        rolls: [1, 2, 3, 4, 5].map((count, index) => blueprint?.grades?.[index + 1] ? count : 0), label: '' };
    }
    return ui.draft;
  }
  function materialName(eng, key) { return eng.catalog?.materials?.find((m) => m.key === key)?.name || key; }
  function familyLabel(eng, material) {
    if (material.familyName) return material.familyName;
    if (material.family == null) return 'Не обменивается';
    const first = eng.catalog.materials.filter((m) => m.category === material.category && m.family === material.family).sort((a, b) => a.grade - b.grade)[0];
    return first ? 'Группа «' + first.name + '»' : '';
  }
  function engineerSkills(eng, engineer) {
    const result = new Map();
    for (const module of eng.catalog.modules) for (const [key, grades] of Object.entries(module.engineers || {})) {
      const maxGrade = Math.max(0, ...Object.entries(grades).filter(([, names]) => names.includes(engineer.english) || names.includes(engineer.name)).map(([grade]) => Number(grade)));
      if (maxGrade > 0) result.set(key, Math.max(maxGrade, result.get(key) || 0));
    }
    return [...result].map(([key, grade]) => ({ ...eng.catalog.blueprints.find((bp) => bp.key === key), grade }));
  }
  function recipeText(eng, rows) { return (rows || []).map((row) => materialName(eng, row.key) + ' ×' + row.count).join(' · '); }
  function renderAdd(eng) {
    const draft = draftFor(eng), modules = eng.catalog?.modules || [];
    const module = modules.find((item) => item.key === draft.moduleKey);
    const blueprints = (eng.catalog?.blueprints || []).filter((bp) => module?.blueprints?.includes(bp.key));
    const blueprint = blueprints.find((bp) => bp.key === draft.blueprintKey);
    const effects = (eng.catalog?.experimentals || []).filter((effect) => module?.experimentals?.includes(effect.key));
    const selectedGrade = Math.max(1, ...draft.rolls.map((count, index) => count > 0 ? index + 1 : 0));
    const engineerNames = module?.engineers?.[blueprint?.key]?.[selectedGrade] || [];
    const engineers = (eng.engineers || []).filter((engineer) => engineerNames.includes(engineer.english) || engineerNames.includes(engineer.name));
    return panel(draft.id ? 'Редактирование улучшения' : 'Добавить улучшение', '<form id="engineering-add-form" class="panel-body"><div class="form-grid engineering-form">' +
      select('moduleKey', 'Модуль', modules.map((m) => [m.key, m.name]), draft.moduleKey) +
      select('blueprintKey', 'Чертёж', blueprints.map((bp) => [bp.key, bp.name]), draft.blueprintKey) +
      select('experimentalKey', 'Экспериментальный эффект', [['', 'Без экспериментального эффекта'], ...effects.map((e) => [e.key, e.name])], draft.experimentalKey) +
      input('quantity', 'Число одинаковых модулей', draft.quantity, 'number', 'required min="1" max="100"') +
      input('label', 'Название для себя', draft.label, 'text', 'maxlength="120" placeholder="Например, двигатели Python"') + '</div>' +
      '<fieldset class="engineering-rolls"><legend>Количество применений на каждом уровне</legend>' + [1, 2, 3, 4, 5].map((grade) => {
        const recipe = blueprint?.grades?.[grade];
        return '<label><span>G' + grade + '</span><input type="number" min="0" max="100" name="roll-' + grade + '" data-engineering-roll="' + (grade - 1) + '" data-focus="engineering-roll-' + grade + '" value="' + h(recipe ? draft.rolls[grade - 1] : 0) + '"' + (!recipe ? ' disabled' : '') + '><small>' + h(recipe ? recipeText(eng, recipe) : 'Недоступен') + '</small></label>';
      }).join('') + '</fieldset>' + note('0 — пропустить уровень. Рецепт умножается на число применений и модулей. Экспериментальный эффект добавляется один раз на каждый модуль. Число применений задаёт ваш план; фактическая прокачка зависит от репутации и уже выполненных улучшений.') +
      (engineers.length ? note('Могут выполнить G' + selectedGrade + ': ' + engineers.map((e) => e.name).join(', ')) : '') +
      '<div class="actions-wrap"><button class="button" type="submit"' + (!blueprint ? ' disabled' : '') + '>' + (draft.id ? 'Сохранить изменение' : 'Добавить в список') + '</button>' + (draft.id ? b('Отменить редактирование', 'edit-cancel') : '') + '</div></form>');
  }
  function renderItems(eng) {
    return panel('Запланированные улучшения', !(eng.items || []).length ? '<div class="panel-body">' + note('Выберите модуль, чертёж и число применений, затем добавьте их в список. Потребность в материалах суммируется автоматически.') + '</div>' :
      '<div class="engineering-items">' + eng.items.map((item) => {
        const module = eng.catalog.modules.find((m) => m.key === item.moduleKey);
        const bp = eng.catalog.blueprints.find((b) => b.key === item.blueprintKey);
        const effect = eng.catalog.experimentals.find((e) => e.key === item.experimentalKey);
        return '<article class="engineering-item"><div><h3>' + h(item.label || module?.name || item.moduleKey) + ' <small>×' + n(item.quantity, true) + '</small></h3><p>' + h(bp?.name || item.blueprintKey) + (effect ? ' · ' + h(effect.name) : '') + '</p><div class="pill-row">' + item.rolls.map((count, index) => count > 0 ? pill('G' + (index + 1) + ' ×' + count, 'muted') : '').join('') + '</div><p class="panel-note">' + h(recipeText(eng, item.materials)) + '</p></div><div class="actions-wrap">' + b('Изменить', 'edit', item.id) + b('Убрать из плана', 'remove', item.id) + '</div></article>';
      }).join('') + '</div>');
  }
  function renderMaterials(eng) {
    const query = ui.materialQuery.trim().toLowerCase();
    const rows = (eng.materials || []).filter((m) => (!query || [m.name, m.english, m.key].some((name) => String(name || '').toLowerCase().includes(query))) &&
      (ui.category === 'all' || m.category === ui.category) && (!ui.shortagesOnly || m.needed > 0));
    return panel('Материалы и список покупок', '<div class="panel-body engineering-filters">' +
      input('materialQuery', 'Найти материал', ui.materialQuery, 'search', 'placeholder="Русское или английское название"', 'ui') +
      select('category', 'Тип материала', [['all', 'Все типы'], ...Object.entries(categories)], ui.category, 'ui') +
      '<label class="operation-check"><input type="checkbox" data-engineering-field="shortagesOnly" data-engineering-scope="ui"' + (ui.shortagesOnly ? ' checked' : '') + '><span>Только материалы плана</span></label></div>' +
      '<div class="table-wrap"><table><thead><tr><th>Материал</th><th>Тип / группа</th><th class="table-num">Есть</th><th class="table-num">Нужно</th><th class="table-num">Не хватает</th><th></th></tr></thead><tbody>' +
      rows.map((m) => '<tr><td><strong>' + h(m.name) + '</strong><small class="engineering-english">' + h(m.english) + ' · G' + h(m.grade) + '</small></td><td>' + h(categories[m.category] || m.category) + '<br><small>' + h(familyLabel(eng, m)) + '</small></td><td class="table-num">' + n(m.owned, true) + ' / ' + n(m.capacity, true) + '</td><td class="table-num">' + n(m.needed, true) + '</td><td class="table-num">' + (m.missing > 0 ? pill(n(m.missing, true), 'potential') : m.needed > 0 && m.missing === 0 ? pill('Хватает', 'confirmed') : m.owned == null && m.needed > 0 ? '—' : n(m.missing, true)) + '</td><td>' + (m.tradeable !== false ? b('Подобрать обмен', 'trade-material', m.key) : 'Не обменивается') + '</td></tr>').join('') + '</tbody></table></div>' +
      (!rows.length ? '<div class="panel-body">' + note('В этой выборке материалов нет. Снимите фильтр или добавьте улучшения в план.') + '</div>' : '') +
      '<div class="panel-body"><div class="actions-wrap">' + b('Копировать список', 'copy-shopping') + '<a class="button button-small button-subtle" href="/api/engineering/materials.csv" download>↓ Материалы CSV</a></div>' + note('«Нужно» — полная стоимость текущего плана, «не хватает» — разница с известным запасом. Запас читается из журнала; инженерные материалы хранятся отдельно от товарного трюма.') + '</div>');
  }
  function renderList(eng) {
    return '<div class="stack">' + renderAdd(eng) + renderItems(eng) + renderMaterials(eng) +
      panel('Сохранить или перенести план', '<div class="panel-body"><div class="actions-wrap"><a class="button button-small button-subtle" href="/api/engineering/plan.json" download>↓ Экспорт плана JSON</a>' + b('Импортировать план', 'import-toggle') + '</div>' +
        (ui.importOpen ? '<form id="engineering-import-form"><label class="field"><span>Вставьте экспортированный план ED Copilot</span><textarea rows="6" data-engineering-field="importText" data-engineering-scope="ui" data-focus="engineering-importText" name="plan" maxlength="300000">' + h(ui.importText) + '</textarea></label><button class="button" type="submit">Заменить список улучшений этим планом</button></form>' : '') + note('План сохраняется автоматически в профиле ED Copilot. Экспорт содержит улучшения; ваш аккаунт, токены и запас материалов в него не входят. Импорт заменяет текущий список после проверки.') + '</div>') + '</div>';
  }
  function renderTrader(eng) {
    const trader = eng.trader || {};
    const o = { tradeTargetKey: trader.targetKey || eng.catalog.materials[0]?.key || '', tradeTargetAmount: trader.targetAmount ?? 1,
      protectPlan: trader.protectPlan !== false, showAllTrades: trader.showAllTrades === true, ...ui.tradeDraft };
    const offers = trader.offers || [];
    const target = eng.materials.find((m) => m.key === o.tradeTargetKey);
    return '<div class="stack">' + panel('Подобрать обмен материалов', '<form id="engineering-trade-form" class="panel-body"><div class="form-grid engineering-form">' +
      select('tradeTargetKey', 'Какой материал получить', eng.catalog.materials.filter((m) => m.tradeable).map((m) => [m.key, m.name + ' · G' + m.grade + ' · ' + categories[m.category]]), o.tradeTargetKey, 'trade') +
      input('tradeTargetAmount', 'Сколько получить дополнительно, ед.', o.tradeTargetAmount, 'number', 'required min="1" max="10000"', 'trade') + '</div><div class="operation-checks">' +
      '<label class="operation-check"><input name="protectPlan" type="checkbox" data-engineering-field="protectPlan" data-engineering-scope="trade"' + (o.protectPlan ? ' checked' : '') + '><span>Сохранить материалы, нужные для плана</span></label>' +
      '<label class="operation-check"><input name="showAllTrades" type="checkbox" data-engineering-field="showAllTrades" data-engineering-scope="trade"' + (o.showAllTrades ? ' checked' : '') + '><span>Показывать также недоступные обмены</span></label></div><button class="button" type="submit">Рассчитать варианты</button></form>') +
      '<div class="info-strip"><div>У одного торговца можно обменять материалы только одного типа: сырьё, промышленные материалы или данные. В своей группе: 1 → 3 на уровень вниз и 6 → 1 на уровень вверх. Между группами добавляется обмен 6 → 1 на том же уровне. Расчёт округляется до целых пакетов обмена.</div></div>' +
      panel('Варианты для ' + (target?.name || materialName(eng, trader.targetKey)), '<div class="table-wrap"><table><thead><tr><th>Отдать</th><th>Курс пакета</th><th class="table-num">Потребуется</th><th class="table-num">Получите</th><th class="table-num">Доступно</th><th class="table-num">Максимум получить</th><th>Возможность</th></tr></thead><tbody>' +
        offers.map((offer) => '<tr><td><strong>' + h(offer.fromName || materialName(eng, offer.fromKey)) + '</strong><small class="engineering-english">G' + h(eng.catalog.materials.find((m) => m.key === offer.fromKey)?.grade) + '</small></td><td>' + n(offer.cost, true) + ' → ' + n(offer.gain, true) + '</td><td class="table-num">' + n(offer.requiredFrom, true) + '</td><td class="table-num">' + n(offer.received, true) + '</td><td class="table-num">' + n(offer.available, true) + '</td><td class="table-num">' + n(offer.maxGain, true) + '</td><td>' + tradeStatus(offer) + '</td></tr>').join('') + '</tbody></table></div>' +
        (!offers.length ? '<div class="panel-body">' + note('Подходящих вариантов пока нет. Проверьте журнал, объём и переключатель недоступных обменов.') + '</div>' : '') +
        '<div class="panel-body">' + note('Варианты — альтернативы: один и тот же запас нельзя потратить сразу на все строки. Обмен выполняется вручную в игре. Показанный максимум учитывает доступный остаток и целые пакеты; свободное место в запасе целевого материала также ограничивает обмен.') + '</div>') + '</div>';
  }
  function tradeStatus(offer) {
    if (offer.storageFits === false) return pill('Нет места для полного пакета', 'potential');
    if (offer.available == null || offer.storageFits == null) return pill('Запас неизвестен', 'muted');
    return pill(offer.affordable ? 'Хватает' : 'Не хватает', offer.affordable ? 'confirmed' : 'potential');
  }
  function renderEngineers(eng) {
    const query = ui.engineerQuery.toLowerCase().trim();
    const engineers = (eng.engineers || []).filter((e) => [e.name, e.english, e.system, e.base].some((s) => String(s || '').toLowerCase().includes(query)));
    const labels = { unlocked: ['Открыт', 'confirmed'], invited: ['Приглашение', 'blue'], known: ['Известен', 'muted'], acquainted: ['Знакомство', 'blue'], barred: ['Доступ закрыт', 'potential'], locked: ['Закрыт', 'muted'], unknown: ['Неизвестно', 'muted'] };
    return '<div class="stack">' + panel('Доступ к инженерам', '<div class="panel-body">' + input('engineerQuery', 'Поиск по имени, системе или базе', ui.engineerQuery, 'search', '', 'ui') +
      note('Статусы и репутация обновляются по журналу. Для старых открытий, которых в журнале нет, можно задать ручную отметку. «По журналу» удаляет ручную отметку. Имена инженеров и систем приведены так, чтобы их можно было найти в игре.') + '</div>') +
      '<div class="engineering-engineers">' + engineers.map((engineer) => {
        const label = engineer.manualUnlocked != null ? [engineer.manualUnlocked ? 'Открыт · вручную' : 'Закрыт · вручную', engineer.manualUnlocked ? 'confirmed' : 'muted'] : labels[engineer.status] || labels.unknown;
        const blueprints = engineerSkills(eng, engineer);
        return '<article class="panel"><div class="panel-heading"><h2>' + h(engineer.name) + '</h2>' + pill(label[0], label[1]) + '</div><div class="panel-body"><p>' + h(engineer.english) + '</p><p>' + h([engineer.system, engineer.base].filter(Boolean).join(' · ')) + '</p>' +
          (engineer.rank != null ? '<p>Репутация: ' + n(engineer.rank, true) + (engineer.progress != null ? ' · ' + n(engineer.progress) + '%' : '') + '</p>' : '') +
          (engineer.requirement || engineer.requirements ? note(engineer.requirement || engineer.requirements) : '') +
          (blueprints.length ? '<div class="pill-row">' + blueprints.map((bp) => pill(bp.name + ' · G' + bp.grade, 'muted')).join('') + '</div>' : '') +
          '<div class="actions-wrap">' + b('Открыт', 'engineer-status', engineer.key + ':yes') + b('Закрыт', 'engineer-status', engineer.key + ':no') + b('По журналу', 'engineer-status', engineer.key + ':auto') +
          (engineer.system ? b('Копировать систему', 'copy', engineer.system) : '') + '</div></div></article>';
      }).join('') + '</div></div>';
  }
  function render(state) {
    const eng = data(state), shopping = eng.shopping || {};
    return '<div class="page-heading"><div><p class="eyebrow">ПОДГОТОВКА К УЛУЧШЕНИЯМ</p><h1>Инженерный планировщик</h1><p>Чертежи, экспериментальные эффекты, материалы, обмены и доступ к инженерам в одном списке.</p></div><div class="heading-actions"><button class="button button-subtle" data-help-topic="engineering" type="button">? Как пользоваться</button></div></div>' +
      '<div class="metrics engineering-metrics"><div class="metric"><div class="metric-label">Улучшений в плане</div><div class="metric-value">' + n(eng.items.length, true) + '</div></div><div class="metric"><div class="metric-label">Материалов нужно</div><div class="metric-value">' + n(shopping.neededUnits, true) + '</div></div><div class="metric"><div class="metric-label">Не хватает</div><div class="metric-value">' + n(shopping.missingUnits, true) + '</div><div class="metric-note">' + (shopping.inventoryKnown ? 'С учётом запаса по журналу' : 'Ожидается полный снимок запасов') + '</div></div><div class="metric"><div class="metric-label">Инженеров открыто</div><div class="metric-value">' + n((eng.engineers || []).filter((e) => e.manualUnlocked === true || e.manualUnlocked == null && e.status === 'unlocked').length, true) + '<small> / ' + n(eng.engineers?.length, true) + '</small></div></div></div>' +
      '<div class="engineering-tabs" role="tablist" aria-label="Инженерный планировщик">' + [['list', 'Список улучшений'], ['trader', 'Торговец материалами'], ['engineers', 'Инженеры']].map(([key, title]) => '<button class="button ' + (ui.tab === key ? 'button-primary' : 'button-subtle') + '" type="button" role="tab" aria-selected="' + (ui.tab === key) + '" data-engineering-action="tab" data-value="' + key + '">' + title + '</button>').join('') + '</div>' +
      ({ list: renderList, trader: renderTrader, engineers: renderEngineers }[ui.tab] || renderList)(eng) +
      '<p class="panel-note engineering-sources">' + h(eng.help?.data || '') + '</p><p class="panel-note engineering-sources">Рецепты и справочные данные: ' + (eng.sources || []).map((source) => '<a href="' + h(source.url) + '" target="_blank" rel="noreferrer">' + h(source.title || source.name) + '</a>').join(' · ') + '</p>';
  }
  function init(api) {
    context = api;
    document.addEventListener('input', (event) => {
      const element = event.target, key = element.dataset.engineeringField, scope = element.dataset.engineeringScope;
      if (element.dataset.engineeringRoll != null) { draftFor(data(context.getState())).rolls[Number(element.dataset.engineeringRoll)] = Number(element.value); return; }
      if (!key) return;
      const value = element.type === 'checkbox' ? element.checked : element.value;
      if (scope === 'ui') { ui[key] = value; if (key !== 'importText') context.render(); }
      else if (scope === 'trade') ui.tradeDraft[key] = value;
      else draftFor(data(context.getState()))[key] = value;
    });
    document.addEventListener('change', (event) => {
      const key = event.target.dataset.engineeringField;
      if (!['moduleKey', 'blueprintKey'].includes(key)) return;
      const eng = data(context.getState()), draft = draftFor(eng);
      if (key === 'moduleKey') { const module = eng.catalog.modules.find((m) => m.key === draft.moduleKey); draft.blueprintKey = module?.blueprints?.[0] || ''; draft.experimentalKey = ''; }
      const bp = eng.catalog.blueprints.find((b) => b.key === draft.blueprintKey);
      draft.rolls = draft.rolls.map((count, index) => bp?.grades?.[index + 1] ? count : 0);
      context.render();
    });
    document.addEventListener('submit', async (event) => {
      const form = event.target;
      if (!form.id?.startsWith('engineering-')) return;
      event.preventDefault();
      if (form.id === 'engineering-add-form') {
        const draft = draftFor(data(context.getState()));
        const result = await context.perform('/api/engineering', { action: draft.id ? 'update' : 'add', ...draft, quantity: Number(draft.quantity), experimentalKey: draft.experimentalKey || null }, 'Список улучшений сохранён');
        if (result && draft.id) { ui.draft = null; context.render(); }
      } else if (form.id === 'engineering-trade-form') {
        const values = Object.fromEntries(new FormData(form));
        // Selects are controlled by their persistent draft; checkbox false must be sent explicitly.
        for (const element of form.querySelectorAll('[data-engineering-field]')) values[element.dataset.engineeringField] = element.type === 'checkbox' ? element.checked : element.value;
        const result = await context.perform('/api/engineering', { action: 'configure', ...values, tradeTargetAmount: Number(values.tradeTargetAmount) });
        if (result) { ui.tradeDraft = {}; context.render(); }
      } else if (form.id === 'engineering-import-form') {
        try { const plan = JSON.parse(ui.importText); const result = await context.perform('/api/engineering', { action: 'import', plan }, 'План импортирован'); if (result) { ui.draft = null; ui.importText = ''; ui.importOpen = false; context.render(); } }
        catch { context.showToast('Не удалось прочитать JSON плана.', true); }
      }
    });
    document.addEventListener('click', async (event) => {
      const element = event.target.closest('[data-engineering-action]');
      if (!element) return;
      const action = element.dataset.engineeringAction, value = element.dataset.value, eng = data(context.getState());
      if (action === 'tab') { ui.tab = value; context.render(); }
      else if (action === 'remove') await context.perform('/api/engineering', { action: 'remove', id: value }, 'Улучшение убрано из плана');
      else if (action === 'edit') { const item = eng.items.find((item) => item.id === value); if (item) { ui.draft = { ...item, rolls: [...item.rolls] }; context.render(); document.getElementById('engineering-add-form')?.scrollIntoView({ block: 'start' }); } }
      else if (action === 'edit-cancel') { ui.draft = null; context.render(); }
      else if (action === 'engineer-status') { const split = value.lastIndexOf(':'); await context.perform('/api/engineering', { action: 'engineer', key: value.slice(0, split), unlocked: value.slice(split + 1) === 'auto' ? null : value.slice(split + 1) === 'yes' }); }
      else if (action === 'trade-material') { const item = eng.materials.find((m) => m.key === value); ui.tab = 'trader'; ui.tradeDraft = {}; await context.perform('/api/engineering', { action: 'configure', tradeTargetKey: value, tradeTargetAmount: item?.missing > 0 ? item.missing : 1 }); }
      else if (action === 'import-toggle') { ui.importOpen = !ui.importOpen; context.render(); }
      else if (action === 'copy') { await U.copyText(value); context.showToast('Система скопирована'); }
      else if (action === 'copy-shopping') {
        const rows = eng.materials.filter((m) => m.needed > 0).map((m) => m.name + ': нужно ' + m.needed + ', есть ' + (m.owned ?? '?') + ', не хватает ' + (m.missing ?? '?'));
        await U.copyText(rows.join('\n')); context.showToast('Список материалов скопирован');
      }
    });
  }
  window.MiningEngineering = { init, render };
})();
