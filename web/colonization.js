(function () {
  'use strict';
  const U = window.MiningUI, { h, n, dateTime, pill } = U;
  const views = { selection: ['1', 'Выбор системы'], construction: ['2', 'План строительства'], supplies: ['3', 'Снабжение'] };
  const projectStatuses = { planned: 'В плане', claimed: 'Заявка подана', building: 'Строительство', complete: 'Завершён', cancelled: 'Отменён' };
  const buildingStatuses = { planned: 'В плане', building: 'Строится', complete: 'Построено', cancelled: 'Отменено' };
  let context, initialized = false, busy = false, dragged = '';
  let ui = { view: 'selection', query: '', availability: 'all', sort: 'distance', searchDraft: null,
    projectKey: '', projectEdit: false, projectDraft: null, addOpen: false, buildingDraft: {},
    editBuilding: '', editDraft: null, supplyBuilding: '', bomOpen: false, bomDraft: [], bomSource: '', deliveryOpen: false, deliveryDraft: {}, manualProjectDraft: {}, logisticsDraft: null, depotDraft: {}, adviceOpen: null, confirm: null };
  const list = value => Array.isArray(value) ? value : [];
  const num = value => typeof value === 'number' && Number.isFinite(value) ? value : null;
  const idOf = value => String(value?.id ?? value?.key ?? '');
  const commodityOf = value => String(value?.commodity ?? value?.key ?? '');
  const button = (label, action, id = '', disabled = false, extra = '') => '<button type="button" class="button button-small button-subtle" data-colonization-action="' + action + '" data-value="' + h(id) + '"' + (disabled || busy ? ' disabled' : '') + ' ' + extra + '>' + h(label) + '</button>';
  const field = (label, control) => '<label class="field"><span>' + h(label) + '</span>' + control + '</label>';
  const input = (scope, name, label, value, attrs = '') => field(label, '<input name="' + name + '" data-colonization-field="' + scope + '" data-focus="colonization-' + scope + '-' + name + '" value="' + h(value ?? '') + '" ' + attrs + '>');
  const options = (rows, selected) => rows.map(([value, label]) => '<option value="' + h(value) + '"' + (String(value) === String(selected) ? ' selected' : '') + '>' + h(label) + '</option>').join('');
  const select = (scope, name, label, value, rows, attrs = '') => field(label, '<select name="' + name + '" data-colonization-field="' + scope + '" data-focus="colonization-' + scope + '-' + name + '" ' + attrs + '>' + options(rows, value) + '</select>');
  const panel = (title, body, extra = '') => '<section class="panel colonization-panel"><div class="panel-heading"><h2>' + h(title) + '</h2>' + extra + '</div>' + body + '</section>';
  const note = (text, warning = false) => text ? '<div class="info-strip' + (warning ? ' is-warning' : '') + '"><div>' + h(text) + '</div></div>' : '';
  const metric = (label, value, sub = '') => '<div class="metric"><div class="metric-label">' + h(label) + '</div><div class="metric-value numeric">' + value + '</div><div class="metric-note">' + h(sub) + '</div></div>';
  const blank = (title, text, action = '') => '<div class="colonization-empty"><span aria-hidden="true">⌂</span><h2>' + h(title) + '</h2><p>' + h(text) + '</p>' + action + '</div>';
  const sourceLabel = value => {
    const text = typeof value === 'string' ? value : value?.name || value?.label || '';
    const labels = { journal: 'Журнал игры', contribution: 'новые поставки', depot: 'Стройплощадка', manual: 'Личная оценка', plan: 'План', catalog: 'Каталог', spansh: 'Spansh' };
    return text.split('+').map(part => labels[part.toLowerCase()] || part).join(' + ');
  };
  function dateLocal(value) {
    if (!value || !Number.isFinite(Date.parse(value))) return '';
    const date = new Date(value);
    return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
  }
  function model(state) {
    const data = state.colonization || {}, projects = list(data.projects);
    const project = data.activeProject || projects.find(item => idOf(item) === String(data.selectedProjectId)) || null;
    return { data, projects, project, catalog: list(data.catalog?.types || data.catalog), buildings: list(project?.buildings) };
  }
  function projectContext(project) {
    if (ui.projectKey === idOf(project)) return;
    ui.projectKey = idOf(project); ui.projectEdit = false; ui.projectDraft = null; ui.addOpen = false; ui.buildingDraft = {};
    ui.editBuilding = ''; ui.editDraft = null; ui.supplyBuilding = ''; ui.bomOpen = false; ui.bomDraft = []; ui.deliveryOpen = false; ui.deliveryDraft = {}; ui.logisticsDraft = null; ui.depotDraft = {}; ui.adviceOpen = null; ui.confirm = null;
  }
  function goods(state) {
    const result = new Map();
    for (const item of [...list(state.resources), ...list(state.goods)]) if (item.kind !== 'raw' && item.key) result.set(item.key, item);
    return [...result.values()].sort((left, right) => String(left.name || left.key).localeCompare(String(right.name || right.key), 'ru'));
  }
  function goodName(state, row) { return row.name || row.localizedName || goods(state).find(item => item.key === commodityOf(row))?.name || commodityOf(row) || 'Неизвестный товар'; }
  function messages(values) { return list(values).map(value => typeof value === 'string' ? value : value?.message || value?.reason).filter(Boolean); }
  function projectPicker(m) {
    if (!m.projects.length) return '';
    return '<div class="colonization-project-picker">' + field('Текущий проект', '<select id="colonization-project" aria-label="Текущий проект колонизации">' +
      options(m.projects.map(project => [idOf(project), (project.name || project.targetSystem || 'Проект') + ' · ' + (projectStatuses[project.status] || 'В плане')]), idOf(m.project)) + '</select>') +
      '<div class="actions-wrap">' + button('Новая система', 'new-project') + (m.project ? button('Копировать систему', 'copy-system', m.project.targetSystem) : '') + '</div></div>';
  }
  function currentSearch(m, state) {
    return ui.searchDraft || { originSystem: m.data.originSystem || m.data.config?.originSystem || m.data.settings?.originSystem || m.data.search?.options?.originSystem || '',
      faction: m.data.faction || m.data.config?.faction || m.data.settings?.faction || m.data.search?.options?.faction || '' };
  }
  function availability(row) {
    const value = row.availability || row.status;
    return value === 'unavailable' || row.eligible === false ? ['Не подходит', 'potential', 'unavailable'] :
      value === 'available' && row.availabilityConfirmed === true ? ['Доступность подтверждена', 'confirmed', 'available'] : ['Проверить в игре', 'muted', 'candidate'];
  }
  function selection(state, m) {
    const search = m.data.search || {}, values = currentSearch(m, state);
    const factions = list(search.originFactions || search.factions || m.data.factions);
    const origins = list(m.data.origins || search.origins);
    const setup = '<form id="colonization-search-form" class="panel-body"><div class="form-grid">' +
      input('search', 'originSystem', 'Исходная обитаемая система', values.originSystem, 'required maxlength="160" list="colonization-origins" placeholder="Например, система с нужной фракцией"') +
      input('search', 'faction', 'Малая фракция', values.faction, 'maxlength="160" list="colonization-factions" placeholder="Выберите после поиска или введите название"') + '</div>' +
      '<datalist id="colonization-origins">' + origins.map(row => '<option value="' + h(typeof row === 'string' ? row : row.system || row.name) + '"></option>').join('') + '</datalist>' +
      '<datalist id="colonization-factions">' + factions.map(row => '<option value="' + h(typeof row === 'string' ? row : row.name || row.faction) + '"></option>').join('') + '</datalist>' +
      '<p class="panel-note">Начните с исходной системы. Поиск уточнит присутствующие фракции и покажет соседние кандидаты. Выберите нужную фракцию и повторите поиск.</p><div class="actions-wrap"><button class="button button-primary" type="submit"' + (busy || search.running ? ' disabled' : '') + '>' + (search.running ? 'Ищем системы…' : 'Найти системы для колонизации') + '</button>' +
      button('Взять текущую систему', 'use-current', state.commander?.system, !state.commander?.system) + '</div></form>';
    let results = list(search.results).filter(row => (!ui.query || [row.system, row.name, ...(row.reasons || [])].join(' ').toLowerCase().includes(ui.query.toLowerCase())) &&
      (ui.availability === 'all' || availability(row)[2] === ui.availability));
    results.sort((left, right) => ui.sort === 'name' ? String(left.system || left.name).localeCompare(String(right.system || right.name)) : (num(left.distanceLy) ?? Infinity) - (num(right.distanceLy) ?? Infinity));
    const filters = '<div class="colonization-result-filters">' + input('filter', 'query', 'Найти в результатах', ui.query, 'type="search" placeholder="Название системы или условие"') +
      select('filter', 'availability', 'Показать', ui.availability, [['all', 'Все результаты'], ['candidate', 'Требуют проверки'], ['unavailable', 'Не подходят']]) +
      select('filter', 'sort', 'Порядок', ui.sort, [['distance', 'Сначала ближайшие'], ['name', 'По названию']]) + '</div>';
    const rows = results.map(row => {
      const status = availability(row), name = row.system || row.name || 'Система не указана';
      const reasons = messages(row.reasons || row.warnings);
      return '<tr><td><strong>' + h(name) + '</strong>' + (row.bodyCount != null ? '<small>Известных тел: ' + n(row.bodyCount, true) + '</small>' : '') + '</td><td class="table-num">' + n(row.distanceLy) + '</td><td>' + pill(status[0], status[1]) +
        (reasons.length ? '<small>' + h(reasons.join(' · ')) + '</small>' : '') + '</td><td><small>' + h(sourceLabel(row.source) || sourceLabel(search.source) || 'Каталог систем') + '</small><small>' + h(row.checkedAt || row.updatedAt ? dateTime(row.checkedAt || row.updatedAt) : 'Время наблюдения не указано') + '</small></td><td><div class="actions-wrap">' +
        button('Выбрать для плана', 'choose-candidate', idOf(row) || name, status[2] === 'unavailable') + button('Копировать', 'copy-system', name) + '</div></td></tr>';
    }).join('');
    const stations = list(search.claimStations || search.stations);
    const stationBody = stations.length ? '<div class="colonization-origin-stations">' + stations.map(station => '<div><strong>' + h(station.name || station.stationName || 'Станция') + '</strong><small>' + h(station.faction || station.controllingFaction || 'Фракция не указана') + '</small>' +
      ((station.distanceToArrival ?? station.distanceLs) != null ? '<small>От звезды: ' + n(station.distanceToArrival ?? station.distanceLs, true) + ' св. с.</small>' : '') + '</div>').join('') + '</div>' : '<p class="panel-note">После поиска здесь появятся известные станции исходной системы. Наличие и условия услуги колонизации проверяются в игре.</p>';
    const manual = '<details class="colonization-manual-target" data-preserve-open="colonization-manual-target"><summary>Указать целевую систему вручную</summary><form id="colonization-manual-project" class="form-grid">' +
      input('manual-target', 'targetSystem', 'Целевая система', ui.manualProjectDraft.targetSystem, 'required maxlength="160" placeholder="Точное название из игры"') + input('manual-target', 'name', 'Название проекта, необязательно', ui.manualProjectDraft.name, 'maxlength="120"') +
      '<div><button class="button button-subtle" type="submit"' + (busy ? ' disabled' : '') + '>Создать личный план</button></div></form><p class="panel-note">Ручной выбор не подтверждает доступность системы и не подаёт заявку в игре.</p></details>';
    return '<div class="colonization-selection-layout"><div class="colonization-main">' + panel('Откуда начинается колонизация', setup) +
      note(search.error, true) + note(({ resolving: 'Читаем исходную систему…', searching: 'Ищем соседние системы…', complete: 'Поиск завершён.', cancelled: 'Поиск отменён. Показана полученная часть.' })[search.status]) +
      messages(search.warnings).map(message => note(message, true)).join('') +
      panel('Кандидаты для новой системы', '<div class="panel-body">' + filters + '<p class="panel-note">Отсутствие населения в каталоге не подтверждает свободную заявку на колонизацию. Перед оплатой проверьте систему и разрешённую дальность в игре.</p></div>' +
        (search.results?.length ? '<div class="panel-body"><div class="pill-row">' + pill(search.completeness?.complete ? 'Известные базе системы загружены' : 'Список неполный', search.completeness?.complete ? 'blue' : 'potential') + (m.data.rules?.rangeLy ? pill('Радиус ' + n(m.data.rules.rangeLy) + ' св. л.', 'muted') : '') + '</div><p class="panel-note">' + h(search.completeness?.reason || 'Полнота поиска не подтверждена.') + '</p></div>' : '') +
        (rows ? '<div class="table-wrap"><table class="colonization-candidates"><thead><tr><th>Система</th><th class="table-num">Св. л.</th><th>Проверка</th><th>Источник</th><th></th></tr></thead><tbody>' + rows + '</tbody></table></div>' : blank(search.running ? 'Поиск выполняется' : search.results?.length ? 'Нет результатов по этим условиям' : 'Выберите исходную систему', 'Кандидаты появятся после поиска. Окончательная проверка и оформление колонизации выполняются в Elite Dangerous.')) + '<div class="panel-body">' + manual + '</div>') +
      '</div><aside class="colonization-aside">' + panel('Порядок действий', '<ol class="colonization-guide"><li>Выберите обитаемую исходную систему и малую фракцию.</li><li>Найдите соседнюю систему и проверьте её в игре.</li><li>Создайте личный план и задайте первый объект.</li><li>После заявки посетите стройплощадку: её список ресурсов появится в снабжении.</li></ol>') +
      panel('Станции исходной системы', '<div class="panel-body">' + stationBody + '</div>') + '</aside></div>';
  }
  function dependencies(m, draft, prefix, exclude = '') {
    const buildings = m.buildings.filter(item => idOf(item) !== exclude && item.status !== 'cancelled');
    if (!buildings.length) return '<p class="panel-note">Пока нет предыдущих этапов, от которых можно зависеть.</p>';
    return '<fieldset class="colonization-dependencies"><legend>Начать после этих этапов</legend>' + buildings.map(item => '<label><input type="checkbox" name="dependsOn" value="' + h(idOf(item)) + '" data-colonization-field="' + prefix + '"' + (list(draft.dependsOn).includes(idOf(item)) ? ' checked' : '') + '><span>' + h(item.name || item.typeName || 'Объект') + '</span></label>').join('') + '</fieldset>';
  }
  function buildingForm(m, editing) {
    const value = editing ? ui.editDraft || editing : ui.buildingDraft;
    const scope = editing ? 'edit-building' : 'building';
    const catalog = m.catalog.map(type => [idOf(type), (type.name || type.title || idOf(type)) + (type.tier ? ' · уровень ' + type.tier : '')]);
    const selectedType = value.typeId || m.catalog[0]?.id || '', type = m.catalog.find(item => idOf(item) === selectedType);
    return '<form id="' + (editing ? 'colonization-edit-building' : 'colonization-add-building') + '" class="colonization-building-form"><div class="form-grid">' +
      (!editing ? select(scope, 'typeId', 'Объект строительства', selectedType, [['', 'Выберите тип объекта'], ...catalog], 'required') : '') +
      input(scope, 'name', 'Название этапа, необязательно', value.name, 'maxlength="120"') + input(scope, 'body', 'Тело или место', value.body, 'maxlength="160" placeholder="Планета, луна или орбита"') +
      (editing ? select(scope, 'status', 'Личная отметка этапа', value.manualStatus || '', [['', 'Не менять отметку'], ...Object.entries(buildingStatuses)]) : '') + '</div>' +
      (!editing ? '<label class="operation-check"><input type="checkbox" name="isPrimary" data-colonization-field="building"' + (value.isPrimary ?? !m.buildings.some(item => item.isPrimary) ? ' checked' : '') + '><span>Первый, основной объект системы</span></label>' : '') +
      (type?.note ? '<p class="panel-note">' + h(type.note) + '</p>' : '') +
      '<details class="colonization-point-overrides" data-preserve-open="colonization-point-overrides"><summary>Уточнить уровень и очки по экрану архитектора</summary><p class="panel-note">Заполняйте только если знаете параметры конкретного варианта в игре. Пустые поля используют известные данные каталога.</p><div class="form-grid">' +
      select(scope, 'tierOverride', 'Уровень варианта', value.tierOverride ?? '', [['', 'По каталогу'], ['1', 'Уровень 1'], ['2', 'Уровень 2'], ['3', 'Уровень 3']]) + input(scope, 'pointCostOverride', 'Стоимость начала, очки', value.pointCostOverride, 'type="number" min="0" max="100000" step="1"') +
      input(scope, 'pointRewardOverride', 'Награда за завершение, очки', value.pointRewardOverride, 'type="number" min="0" max="100000" step="1"') + '</div></details>' + dependencies(m, value, scope, idOf(editing)) + (editing ? field('Заметки', '<textarea name="notes" data-colonization-field="edit-building" data-focus="colonization-edit-building-notes" rows="3" maxlength="1500">' + h(value.notes || '') + '</textarea>') : '') +
      '<div class="actions-wrap"><button class="button button-primary" type="submit"' + (busy ? ' disabled' : '') + '>' + (editing ? 'Сохранить этап' : 'Добавить в очередь') + '</button>' + button('Отменить', editing ? 'cancel-edit' : 'cancel-add') + '</div></form>';
  }
  function buildingResources(building) { return list(building?.supplies || building?.resources || building?.requirements); }
  function isActual(building) { return building?.authoritative === true || building?.billSource === 'journal' || building?.source === 'journal' || buildingResources(building).some(row => row.authoritative === true); }
  function totals(rows) {
    const sum = key => rows.length && rows.every(row => num(row[key]) != null) ? rows.reduce((total, row) => total + row[key], 0) : null;
    const required = sum('required'), delivered = sum('delivered'), missing = sum('missing');
    return { required, delivered, missing, progress: required > 0 && delivered != null ? Math.max(0, Math.min(100, delivered / required * 100)) : null };
  }
  function construction(state, m) {
    const project = m.project;
    if (!project) return blank('Сначала выберите систему', 'Создайте проект из кандидата или укажите систему вручную.', button('Выбрать систему', 'view', 'selection'));
    const warnings = messages(project.warnings || project.validation?.warnings);
    const projectDraft = ui.projectDraft || project;
    const editProject = ui.projectEdit ? '<form id="colonization-project-form" class="panel-body"><div class="form-grid">' + input('project', 'name', 'Название проекта', projectDraft.name || project.targetSystem, 'maxlength="120"') +
      select('project', 'status', 'Личная отметка проекта', projectDraft.manualStatus || projectDraft.status || 'planned', Object.entries(projectStatuses)) + select('project', 'goalEconomy', 'Желаемая экономика', projectDraft.goalEconomy || '', [['', 'Выберите направление'], ...list(m.data.economies).map(item => [item.id, item.name])]) +
      input('project', 'pointsTier2', 'Доступные очки T2 по экрану игры', projectDraft.pointsTier2 ?? projectDraft.points?.tier2, 'type="number" min="0" max="100000" step="1" placeholder="Неизвестно"') + input('project', 'pointsTier3', 'Доступные очки T3 по экрану игры', projectDraft.pointsTier3 ?? projectDraft.points?.tier3, 'type="number" min="0" max="100000" step="1" placeholder="Неизвестно"') +
      input('project', 'externalPortCount', 'Начатые порты T2/T3 вне этой очереди', projectDraft.externalPortCount ?? 0, 'type="number" min="0" max="1000" step="1"') +
      input('project', 'claimedAt', 'Когда оформлена заявка', dateLocal(projectDraft.claimedAt), 'type="datetime-local"') + input('project', 'deadline', 'Свой крайний срок, если известен', projectDraft.deadlineSource === 'manual' ? dateLocal(projectDraft.deadline) : '', 'type="datetime-local"') + '</div><p class="panel-note">Текущий баланс очков уже включает построенные объекты. Введите доступный остаток; завершённые этапы не начисляются повторно.</p>' +
      field('Заметки', '<textarea name="notes" data-colonization-field="project" data-focus="colonization-project-notes" rows="3" maxlength="2000">' + h(projectDraft.notes || '') + '</textarea>') +
      '<div class="actions-wrap"><button type="submit" class="button button-primary">Сохранить проект</button>' + button('Отменить', 'cancel-project-edit') + button('Удалить личный план', 'confirm-project-remove', idOf(project)) + '</div><p class="panel-note">Эти отметки ведут ваш план. Заявка и действия строительства выполняются в игре.</p></form>' : '';
    const projectHero = panel(project.targetSystem || project.name || 'Новая система', '<div class="panel-body colonization-project-summary"><div><div class="pill-row">' + pill(projectStatuses[project.status] || 'В плане', project.status === 'complete' ? 'confirmed' : 'blue') +
      (project.faction ? pill(project.faction, 'muted') : '') + '</div><p>' + h(project.originSystem ? 'Исходная система: ' + project.originSystem : 'Исходная система не задана') + '</p>' +
      (project.goalEconomy ? '<p>Цель: ' + h(project.economyPlan?.name || m.data.economies?.find(item => item.id === project.goalEconomy)?.name || project.goalEconomy) + '</p>' : '') + (project.deadline ? '<p>' + (project.deadlineSource === 'claim-estimate' ? 'Расчётный срок основного порта' : 'Срок по вашему плану') + ': <strong>' + h(dateTime(project.deadline)) + '</strong></p>' : '') + '</div><div class="actions-wrap">' + button('Параметры проекта', 'edit-project') + button('К снабжению', 'view', 'supplies') + '</div></div>' + editProject);
    const cards = m.buildings.map((building, index) => {
      const rows = buildingResources(building), total = totals(rows), actual = isActual(building), type = m.catalog.find(item => idOf(item) === String(building.typeId));
      const prerequisites = list(building.dependsOn).map(id => m.buildings.find(item => idOf(item) === String(id))?.name || 'Предыдущий этап');
      const issues = [...messages(building.warnings || building.validation?.warnings), ...messages(building.prerequisites)];
      const depots = list(m.data.depots).filter(depot => !project.targetSystem || !depot.system || depot.system === project.targetSystem);
      const depotLink = depots.length ? '<form class="colonization-link-depot" data-building="' + h(idOf(building)) + '">' + field('Стройплощадка из журнала', '<select name="marketId"><option value="">Выберите посещённую стройплощадку</option>' +
        options(depots.map(depot => [depot.marketId, depot.stationName || depot.station || depot.name || depot.system || 'Стройплощадка']), ui.depotDraft[idOf(building)] ?? building.marketId ?? building.depot?.marketId ?? '') + '</select>') + '<button type="submit" class="button button-small button-subtle">Привязать</button></form>' : '<p class="panel-note">Чтобы получить точный список ресурсов, посетите стройплощадку в игре и откройте её экран строительства.</p>';
      return '<article class="colonization-building" data-colonization-building="' + h(idOf(building)) + '" draggable="' + (ui.editBuilding !== idOf(building)) + '"><div class="colonization-building-heading"><div class="colonization-building-order"><span aria-hidden="true">⠿</span><strong>' + (index + 1) + '</strong></div><div><div class="pill-row">' +
        pill(building.status === 'failed' ? 'Строительство провалено' : buildingStatuses[building.status] || 'В плане', building.status === 'failed' ? 'potential' : building.status === 'complete' ? 'confirmed' : 'muted') + (building.isPrimary ? pill('Основной объект', 'blue') : '') + ((building.tier || type?.tier) ? pill('Уровень ' + (building.tier || type.tier), 'muted') : '') +
        '</div><h3>' + h(building.name || type?.name || building.typeName || 'Объект') + '</h3><p>' + h([type?.name, building.body].filter(Boolean).join(' · ')) + '</p></div><div class="colonization-building-moves">' + button('↑', 'move-up', idOf(building), index === 0, 'aria-label="Переместить этап выше"') + button('↓', 'move-down', idOf(building), index === m.buildings.length - 1, 'aria-label="Переместить этап ниже"') + '</div></div>' +
        (prerequisites.length ? '<p class="colonization-dependency-note">После: ' + h(prerequisites.join(' · ')) + '</p>' : '') +
        ((building.pointCost != null || building.pointReward != null) ? '<p class="colonization-dependency-note">Очки: начало ' + n(building.pointCost, true) + ' · за завершение ' + n(building.pointReward, true) + (building.pointSource === 'manual' ? ' · ваша оценка' : '') + '</p>' : '') +
        (issues.length ? '<div class="colonization-warnings">' + issues.map(issue => '<p>' + h(issue) + '</p>').join('') + '</div>' : '') +
        '<div class="colonization-building-bill"><span>' + h(rows.length ? actual ? 'Ресурсы по стройплощадке' : 'Плановая оценка ресурсов' : 'Список ресурсов пока неизвестен') + '</span><strong>' + (total.missing != null ? 'Осталось ' + n(total.missing, true) + ' т' : '—') + '</strong></div>' +
        (total.progress != null ? '<div class="progress" aria-label="Обеспеченность ресурсами"><span style="width:' + total.progress + '%"></span></div>' : '') +
        '<div class="actions-wrap">' + button('Ресурсы и поставки', 'building-supplies', idOf(building)) + button('Изменить', 'edit-building', idOf(building)) + button('Убрать этап', 'confirm-building-remove', idOf(building)) + '</div>' +
        (ui.editBuilding === idOf(building) ? buildingForm(m, building) + depotLink : '') + '</article>';
    }).join('');
    const advice = list(project.buildAdvice);
    const adviceCard = advice.length ? '<details class="panel colonization-advice" data-colonization-advice' + ((ui.adviceOpen ?? !m.buildings.length) ? ' open' : '') + '><summary>Рекомендации по развитию</summary><div class="panel-body">' + advice.map(item => '<section><h3>' + h(item.title) + '</h3><p>' + h(item.text) + '</p>' +
      (list(item.typeIds).length ? '<p class="colonization-advice-types">' + h(item.typeIds.map(id => m.catalog.find(type => idOf(type) === id)?.name).filter(Boolean).join(' · ')) + '</p>' : '') + '</section>').join('') + '<p class="panel-note">Это рекомендации для вашего плана. Размещение, доступные слоты и итоговую экономику проверяйте в игре.</p></div></details>' : '';
    return projectHero + adviceCard + (project.economyPlan?.guidance ? note(project.economyPlan.guidance) + '<p class="panel-note">' + h(project.economyPlan.warning || '') + '</p>' : '') + (warnings.length ? warnings.map(message => note(message, true)).join('') : '') +
      panel('Очередь строительства', '<div class="panel-body"><p class="panel-note">Задайте основной объект, затем вспомогательные сооружения. Порядок можно менять перетаскиванием или стрелками; зависимости и уровни проверяются по плану.</p>' +
        (cards || blank('Каким будет первый объект?', 'Добавьте основной объект системы. Список ресурсов появится из стройплощадки или из вашей плановой оценки.')) +
        (ui.addOpen ? buildingForm(m) : button('Добавить объект', 'show-add', '', !m.catalog.length)) + (!m.catalog.length ? '<p class="panel-note">Каталог объектов пока недоступен.</p>' : '') + '</div>');
  }
  function resourceRows(m, building) { return building ? buildingResources(building) : list(m.data.aggregateSupplies || m.project?.aggregateSupplies || m.project?.supplies); }
  function bomEditor(state, building) {
    const items = goods(state);
    const rows = ui.bomDraft.map((row, index) => '<div class="colonization-bom-row">' + field('Товар', '<select data-colonization-bom="commodity" data-index="' + index + '">' +
      options([['', 'Выберите товар'], ...items.map(item => [item.key, item.name || item.key]), ...(!items.some(item => item.key === row.commodity) && row.commodity ? [[row.commodity, row.name || row.commodity]] : [])], row.commodity) + '</select>') +
      field('Нужно, т', '<input type="number" min="1" max="10000000" step="1" required data-colonization-bom="required" data-index="' + index + '" data-focus="colonization-bom-' + index + '" value="' + h(row.required ?? '') + '">') + button('Убрать', 'bom-remove', String(index)) + '</div>').join('');
    return '<form id="colonization-bom-form" class="colonization-bom-form"><h3>Ваш плановый список</h3><p class="panel-note">Это оценка для выбранного этапа. Если стройплощадка уже передала точные потребности, её данные имеют приоритет.</p>' + rows +
      '<div class="actions-wrap">' + button('Добавить товар', 'bom-add') + '</div>' + input('bom', 'sourceNote', 'Основание плановой оценки', ui.bomSource, 'required maxlength="300" placeholder="Например, список со стройплощадки или собственный расчёт"') +
      '<div class="actions-wrap"><button type="submit" class="button button-primary">Сохранить оценку для этапа</button>' + button('Отменить', 'cancel-bom') + '</div></form>';
  }
  function deliveryForm(state, building) {
    const rows = resourceRows(model(state), building), value = ui.deliveryDraft;
    return '<form id="colonization-delivery-form" class="colonization-delivery-form"><h3>Личная отметка поставки</h3><p class="panel-note">Привязана только к этому этапу. Поставки из журнала учитываются отдельно; ручная отметка не добавляется повторно к подтверждённому остатку стройплощадки.</p><div class="form-grid">' +
      select('delivery', 'commodity', 'Товар', value.commodity || '', [['', 'Выберите товар'], ...rows.map(row => [commodityOf(row), goodName(state, row)])], 'required') + input('delivery', 'count', 'Передано, т', value.count, 'type="number" min="1" step="1" required') +
      input('delivery', 'notes', 'Заметка', value.notes, 'maxlength="500"') + '</div><div class="actions-wrap"><button type="submit" class="button button-primary">Отметить поставку</button>' + button('Отменить', 'cancel-delivery') + '</div></form>';
  }
  function supplies(state, m) {
    if (!m.project) return blank('Пока нет проекта', 'Выберите целевую систему и добавьте хотя бы один объект.', button('Выбрать систему', 'view', 'selection'));
    const building = m.buildings.find(item => idOf(item) === ui.supplyBuilding) || null, rows = resourceRows(m, building), total = totals(rows);
    const cargoKnown = Boolean(state.commander?.cargoUpdatedAt), cargo = list(state.commander?.cargo);
    const capacity = num(m.data.cargoCapacity ?? m.data.config?.cargoCapacity ?? m.data.settings?.cargoCapacity), trips = total.missing != null && capacity > 0 ? Math.ceil(total.missing / capacity) : null;
    const table = rows.map(row => {
      const key = commodityOf(row), onShip = Object.hasOwn(row, 'cargoAvailable') ? row.cargoAvailable : cargoKnown ? cargo.find(item => item.key === key)?.count || 0 : null;
      const onCarrier = num(row.carrierAvailable);
      const source = sourceLabel(row.source) || (row.authoritative ? 'Стройплощадка' : 'План');
      return '<tr><td><strong>' + h(goodName(state, row)) + '</strong><small>' + h(source) + (row.updatedAt ? ' · ' + h(dateTime(row.updatedAt)) : '') + '</small></td><td class="table-num">' + n(row.required, true) + '</td><td class="table-num">' + n(row.delivered, true) + '</td><td class="table-num colonization-missing">' + n(row.missing, true) + '</td><td class="table-num">' + n(onShip, true) + '</td><td class="table-num">' + n(onCarrier, true) + '</td><td>' +
        button('Найти где купить', 'buy', key, !(num(row.missing) > 0)) + '</td></tr>';
    }).join('');
    const journalDeliveries = list(m.project.journalDeliveries).flatMap(entry => list(entry.resources).map(resource => ({ ...resource, timestamp: entry.timestamp, source: 'journal', buildingId: idOf(m.buildings.find(item => String(item.marketId) === String(entry.marketId))) })));
    const deliveries = [...list(m.project.manualDeliveries || m.project.deliveries || m.data.deliveries), ...journalDeliveries].filter(row => !building || String(row.buildingId) === idOf(building)).sort((left, right) => String(right.timestamp || '').localeCompare(String(left.timestamp || '')));
    const log = deliveries.length ? '<div class="table-wrap"><table><thead><tr><th>Время</th><th>Объект</th><th>Товар</th><th class="table-num">Тонны</th><th>Основание</th><th></th></tr></thead><tbody>' + deliveries.slice(0, 100).map(row => '<tr><td>' + h(dateTime(row.timestamp)) + '</td><td>' + h(m.buildings.find(item => idOf(item) === String(row.buildingId))?.name || 'Этап не привязан') + '</td><td>' + h(goodName(state, row)) + '</td><td class="table-num">' + n(row.count ?? row.amount, true) + '</td><td>' + h(row.source === 'manual' ? 'Личная отметка' : sourceLabel(row.source) || 'Журнал игры') + (row.notes ? '<small>' + h(row.notes) + '</small>' : '') + '</td><td>' + (row.source === 'manual' ? button('Убрать отметку', 'remove-delivery', row.id) : '') + '</td></tr>').join('') + '</tbody></table></div>' : '<div class="panel-body"><p class="panel-note">Ваши поставки появятся после передачи ресурсов в игре. Ручные отметки также привязаны к конкретному этапу.</p></div>';
    return '<div class="colonization-supply-toolbar">' + field('Ресурсы для этапа', '<select id="colonization-supply-building">' + options([['', 'Все этапы проекта'], ...m.buildings.map(item => [idOf(item), item.name || item.typeName || 'Объект'])], ui.supplyBuilding) + '</select>') +
      '<div class="actions-wrap">' + button('Скопировать список закупки', 'copy-supplies', '', !rows.length) + (building ? button('Задать план ресурсов', 'show-bom') : '') + (building ? button('Личная отметка поставки', 'show-delivery', '', !rows.length) : '') + '</div></div>' +
      (!building && m.project.unknownBills ? note('Для ' + n(m.project.unknownBills, true) + ' этапов потребность ещё неизвестна. Общая таблица охватывает только известные списки.', true) : '') +
      '<div class="metric-grid colonization-supply-metrics">' + metric('Осталось доставить', total.missing == null ? '—' : n(total.missing, true) + ' т', 'По известным спискам выбранных этапов') +
      metric('Подтверждено / учтено', total.delivered == null ? '—' : n(total.delivered, true) + ' т', 'Проверяйте основание каждого списка') + metric('Рейсов по вместимости', n(trips, true), capacity > 0 ? 'При загрузке по ' + n(capacity, true) + ' т' : 'Задайте вместимость для оценки') + '</div>' +
      panel(building ? building.name || 'Снабжение этапа' : 'Снабжение всего проекта', '<div class="panel-body"><p class="panel-note">Остаток стройплощадки уже учитывает её полученные грузы. Личная история поставок показана отдельно и не прибавляется повторно к этому счётчику.</p>' +
        '<form id="colonization-logistics-form" class="colonization-logistics">' + input('logistics', 'cargoCapacity', 'Вместимость для расчёта рейсов, т', ui.logisticsDraft ?? capacity, 'type="number" min="1" max="25000" step="1"') + '<button type="submit" class="button button-small button-subtle">Сохранить</button></form></div>' +
        (table ? '<div class="table-wrap"><table class="colonization-resources"><thead><tr><th>Товар и источник</th><th class="table-num">Нужно, т</th><th class="table-num">Сдано, т</th><th class="table-num">Осталось, т</th><th class="table-num">На борту, т</th><th class="table-num">На ФК, т</th><th></th></tr></thead><tbody>' + table + '</tbody></table></div>' : blank('Список ресурсов ещё не известен', building ? 'Посетите привязанную стройплощадку или добавьте свою оценку потребностей этого этапа.' : 'Выберите этап и задайте оценку, либо посетите стройплощадку для получения точного списка.')) +
        (building && ui.bomOpen ? '<div class="panel-body">' + bomEditor(state, building) + '</div>' : '') + (building && ui.deliveryOpen ? '<div class="panel-body">' + deliveryForm(state, building) + '</div>' : '')) +
      panel('Ваши поставки по проекту', log);
  }
  function confirmation() {
    if (!ui.confirm) return '';
    return '<div class="colonization-confirm" role="alert"><strong>' + h(ui.confirm.kind === 'project' ? 'Удалить личный план этого проекта?' : 'Убрать этот этап из личного плана?') + '</strong><p>Это изменит только записи ED Copilot. Действия колонизации в игре не отменяются.</p><div class="actions-wrap">' + button('Удалить из плана', 'remove-confirmed', ui.confirm.id) + button('Отменить', 'cancel-remove') + '</div></div>';
  }
  function render(state) {
    const m = model(state); projectContext(m.project);
    return '<div class="copilot-colonization"><div class="page-heading"><div><p class="eyebrow">НОВАЯ СИСТЕМА · ЛИЧНЫЙ ПЛАН</p><h1>Колонизация</h1><p>Выберите исходную фракцию и систему, спланируйте объекты и соберите ресурсы для каждого этапа.</p></div><button class="button button-subtle" type="button" data-help-topic="colonization">? Как пользоваться</button></div>' +
      projectPicker(m) + '<div class="colonization-workflow" role="tablist" aria-label="Шаги колонизации">' + Object.entries(views).map(([id, [number, title]]) => '<button type="button" role="tab" aria-selected="' + (ui.view === id) + '" data-colonization-view="' + id + '" class="' + (ui.view === id ? 'is-active' : '') + '"><span>' + number + '</span><strong>' + title + '</strong></button>').join('') + '</div>' +
      confirmation() + (ui.view === 'construction' ? construction(state, m) : ui.view === 'supplies' ? supplies(state, m) : selection(state, m)) + '</div>';
  }
  async function perform(body, message) {
    if (busy) return null;
    busy = true; context.render();
    try { return await context.perform('/api/colonization', body, message); }
    finally { busy = false; context.render(); }
  }
  function formValues(form) { return Object.fromEntries(new FormData(form)); }
  function captureFields(scope, form) {
    const values = formValues(form);
    if (scope === 'building' || scope === 'edit-building') {
      values.dependsOn = [...form.querySelectorAll('[name="dependsOn"]:checked')].map(input => input.value);
      values.isPrimary = Boolean(form.querySelector('[name="isPrimary"]:checked'));
    }
    return values;
  }
  function init(api) {
    context = api; if (initialized) return; initialized = true;
    document.addEventListener('input', event => {
      const input = event.target, scope = input.dataset.colonizationField;
      if (input.dataset.colonizationBom) {
        const row = ui.bomDraft[Number(input.dataset.index)]; if (row) row[input.dataset.colonizationBom] = input.dataset.colonizationBom === 'required' ? Number(input.value) : input.value; return;
      }
      if (!scope) return;
      if (scope === 'search') ui.searchDraft = { ...currentSearch(model(context.getState()), context.getState()), [input.name]: input.value };
      else if (scope === 'filter') { ui[input.name] = input.value; context.render(); }
      else if (scope === 'building') ui.buildingDraft = captureFields(scope, input.closest('form'));
      else if (scope === 'edit-building') { ui.editDraft = captureFields(scope, input.closest('form')); ui.editDraft.manualStatus = ui.editDraft.status; }
      else if (scope === 'project') ui.projectDraft = { ...(ui.projectDraft || model(context.getState()).project), ...formValues(input.closest('form')) };
      else if (scope === 'delivery') ui.deliveryDraft = formValues(input.closest('form'));
      else if (scope === 'bom') ui.bomSource = input.value;
      else if (scope === 'manual-target') ui.manualProjectDraft = formValues(input.closest('form'));
      else if (scope === 'logistics') ui.logisticsDraft = input.value;
    });
    document.addEventListener('change', async event => {
      if (event.target.id === 'colonization-project') { await perform({ action: 'select-project', id: event.target.value }); return; }
      if (event.target.id === 'colonization-supply-building') { ui.supplyBuilding = event.target.value; ui.bomOpen = ui.deliveryOpen = false; context.render(); }
      if (event.target.name === 'marketId' && event.target.closest('.colonization-link-depot')) ui.depotDraft[event.target.closest('.colonization-link-depot').dataset.building] = event.target.value;
    });
    document.addEventListener('submit', async event => {
      const form = event.target;
      if (!form.id?.startsWith('colonization-') && !form.classList.contains('colonization-link-depot')) return;
      event.preventDefault();
      const state = context.getState(), m = model(state), values = formValues(form);
      if (form.id === 'colonization-search-form') {
        const configured = await perform({ action: 'configure', originSystem: values.originSystem, faction: values.faction });
        if (configured) await context.perform('/api/colonization/search', { originSystem: values.originSystem, faction: values.faction });
      } else if (form.id === 'colonization-manual-project') {
        const result = await perform({ action: 'create-project', targetSystem: values.targetSystem, name: values.name });
        if (result) { ui.view = 'construction'; context.render(); }
      } else if (form.id === 'colonization-project-form') {
        const { pointsTier2, pointsTier3, ...fields } = values;
        const result = await perform({ action: 'update-project', id: idOf(m.project), ...fields,
          points: { tier2: pointsTier2 === '' ? null : Number(pointsTier2), tier3: pointsTier3 === '' ? null : Number(pointsTier3) },
          claimedAt: values.claimedAt ? new Date(values.claimedAt).toISOString() : null,
          deadline: values.deadline ? new Date(values.deadline).toISOString() : null });
        if (result) { ui.projectEdit = false; ui.projectDraft = null; context.render(); }
      } else if (form.id === 'colonization-add-building') {
        const body = captureFields('building', form); const result = await perform({ action: 'add-building', projectId: idOf(m.project), ...body });
        if (result) { ui.addOpen = false; ui.buildingDraft = {}; context.render(); }
      } else if (form.id === 'colonization-edit-building') {
        const { isPrimary, status, ...body } = captureFields('edit-building', form);
        const result = await perform({ action: 'update-building', projectId: idOf(m.project), id: ui.editBuilding, ...body, ...(status ? { status } : {}) });
        if (result) { ui.editBuilding = ''; ui.editDraft = null; context.render(); }
      } else if (form.classList.contains('colonization-link-depot')) {
        if (values.marketId) await perform({ action: 'link-depot', projectId: idOf(m.project), buildingId: form.dataset.building, marketId: Number(values.marketId) });
      } else if (form.id === 'colonization-bom-form') {
        const resources = ui.bomDraft.filter(row => row.commodity && Number(row.required) > 0).map(row => ({ commodity: row.commodity, required: Number(row.required) }));
        const result = await perform({ action: 'set-resource-plan', projectId: idOf(m.project), buildingId: ui.supplyBuilding, resources, sourceNote: ui.bomSource });
        if (result) { ui.bomOpen = false; context.render(); }
      } else if (form.id === 'colonization-delivery-form') {
        const result = await perform({ action: 'record-delivery', projectId: idOf(m.project), buildingId: ui.supplyBuilding, commodity: values.commodity, count: Number(values.count), notes: values.notes });
        if (result) { ui.deliveryOpen = false; ui.deliveryDraft = {}; context.render(); }
      } else if (form.id === 'colonization-logistics-form') {
        await perform({ action: 'configure', cargoCapacity: Number(values.cargoCapacity) });
      }
    });
    document.addEventListener('click', async event => {
      const view = event.target.closest('[data-colonization-view]');
      if (view) { ui.view = view.dataset.colonizationView; context.render(); return; }
      const control = event.target.closest('[data-colonization-action]'); if (!control || control.disabled) return;
      const action = control.dataset.colonizationAction, value = control.dataset.value, state = context.getState(), m = model(state);
      if (action === 'view') ui.view = value;
      else if (action === 'new-project') ui.view = 'selection';
      else if (action === 'use-current') ui.searchDraft = { ...currentSearch(m, state), originSystem: value };
      else if (action === 'copy-system') { await U.copyText(value); context.showToast('Название системы скопировано'); return; }
      else if (action === 'choose-candidate') {
        const row = list(m.data.search?.results).find(item => (idOf(item) || item.system || item.name) === value);
        if (row && await perform({ action: 'create-project', candidateId: row.id, targetSystem: row.system || row.name })) ui.view = 'construction';
      } else if (action === 'edit-project') { ui.projectEdit = true; ui.projectDraft = structuredClone(m.project); }
      else if (action === 'cancel-project-edit') { ui.projectEdit = false; ui.projectDraft = null; }
      else if (action === 'show-add') { ui.addOpen = true; ui.buildingDraft = { typeId: m.catalog[0]?.id || '', isPrimary: !m.buildings.some(item => item.isPrimary), dependsOn: [] }; }
      else if (action === 'cancel-add') ui.addOpen = false;
      else if (action === 'edit-building') { const building = m.buildings.find(item => idOf(item) === value); ui.editBuilding = value; ui.editDraft = structuredClone(building); }
      else if (action === 'cancel-edit') { ui.editBuilding = ''; ui.editDraft = null; }
      else if (action === 'move-up' || action === 'move-down') { await perform({ action: 'move-building', projectId: idOf(m.project), id: value, direction: action === 'move-up' ? 'up' : 'down' }); return; }
      else if (action === 'building-supplies') { ui.supplyBuilding = value; ui.view = 'supplies'; ui.bomOpen = ui.deliveryOpen = false; }
      else if (action === 'show-bom') {
        const building = m.buildings.find(item => idOf(item) === ui.supplyBuilding);
        ui.bomDraft = buildingResources(building).map(row => ({ commodity: commodityOf(row), required: row.required, name: row.name }));
        if (!ui.bomDraft.length) ui.bomDraft = [{ commodity: '', required: '' }]; ui.bomOpen = true; ui.bomSource = '';
      } else if (action === 'cancel-bom') ui.bomOpen = false;
      else if (action === 'bom-add') ui.bomDraft.push({ commodity: '', required: '' });
      else if (action === 'bom-remove') ui.bomDraft.splice(Number(value), 1);
      else if (action === 'show-delivery') { ui.deliveryOpen = true; ui.deliveryDraft = {}; }
      else if (action === 'cancel-delivery') ui.deliveryOpen = false;
      else if (action === 'remove-delivery') { await perform({ action: 'remove-delivery', projectId: idOf(m.project), id: value }); return; }
      else if (action === 'confirm-project-remove' || action === 'confirm-building-remove') ui.confirm = { kind: action === 'confirm-project-remove' ? 'project' : 'building', id: value };
      else if (action === 'cancel-remove') ui.confirm = null;
      else if (action === 'remove-confirmed') {
        const kind = ui.confirm?.kind, result = await perform(kind === 'project' ? { action: 'remove-project', id: value } : { action: 'remove-building', projectId: idOf(m.project), id: value });
        if (result) { ui.confirm = null; if (kind === 'project') ui.view = 'selection'; }
      } else if (action === 'buy') {
        const building = m.buildings.find(item => idOf(item) === ui.supplyBuilding), row = resourceRows(m, building).find(item => commodityOf(item) === value);
        if (window.MiningOperations.openPurchase) window.MiningOperations.openPurchase({ resourceKey: value, quantity: row?.missing || 1, system: state.commander?.system || m.project.originSystem });
        else { location.hash = 'sale'; context.showToast('Выберите покупку товара: ' + goodName(state, row || { commodity: value })); }
        return;
      } else if (action === 'copy-supplies') {
        const building = m.buildings.find(item => idOf(item) === ui.supplyBuilding), rows = resourceRows(m, building).filter(row => num(row.missing) > 0);
        await U.copyText([m.project.targetSystem, building?.name, ...rows.map(row => goodName(state, row) + ': ' + n(row.missing, true) + ' т')].filter(Boolean).join('\n'));
        context.showToast('Список закупки скопирован'); return;
      }
      context.render();
    });
    document.addEventListener('dragstart', event => {
      const building = event.target.closest('[data-colonization-building]'); if (!building || building.draggable !== true || !event.dataTransfer) return;
      dragged = building.dataset.colonizationBuilding; event.dataTransfer.setData('text/plain', dragged); event.dataTransfer.effectAllowed = 'move'; building.classList.add('is-dragging');
    });
    document.addEventListener('dragover', event => {
      const building = event.target.closest('[data-colonization-building]'); if (!dragged || !building) return;
      event.preventDefault(); if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
    });
    document.addEventListener('drop', async event => {
      const target = event.target.closest('[data-colonization-building]'); if (!dragged || !target) return;
      event.preventDefault(); const source = dragged; dragged = ''; const m = model(context.getState()), ids = m.buildings.map(idOf).filter(id => id !== source), bounds = target.getBoundingClientRect();
      const index = ids.indexOf(target.dataset.colonizationBuilding); if (index < 0) return;
      ids.splice(index + (event.clientY > bounds.top + bounds.height / 2 ? 1 : 0), 0, source);
      await perform({ action: 'reorder-buildings', projectId: idOf(m.project), ids });
    });
    document.addEventListener('dragend', () => { dragged = ''; for (const card of document.querySelectorAll('.colonization-building.is-dragging')) card.classList.remove('is-dragging'); });
    document.addEventListener('toggle', event => { if (event.target.hasAttribute?.('data-colonization-advice')) ui.adviceOpen = event.target.open; }, true);
  }
  window.CopilotColonization = { init, render, capture: () => structuredClone(ui), restore: value => { if (value && typeof value === 'object') ui = { ...ui, ...value }; } };
})();
