(() => {
  'use strict';

  const mode = 'Текущий корабль';
  const operation = 'ship-engineering-plan';
  const updateOperation = 'ship-engineering-plan-update';
  const active = C => C.tab === 'engineering' && C.mode === mode;
  const rows = value => Array.isArray(value) ? value : [];
  const statuses = {
    complete: ['Улучшен до максимума', 'complete'],
    'in-progress': ['В процессе', 'inProgress'],
    available: ['Можно улучшить', 'available'],
    locked: ['Нужен доступ к инженеру', 'locked'],
    unknown: ['Неизвестно', 'unknown']
  };
  const access = {
    unlocked: 'Доступен', invited: 'Приглашение получено', acquainted: 'Приглашение получено',
    known: 'Обнаружен', locked: 'Закрыт', barred: 'Доступ ограничен', unknown: 'Неизвестно'
  };
  const conditions = {
    'replaces-blueprint': 'Новый чертёж заменит установленное улучшение.',
    'unmapped-blueprint': 'Текущий чертёж не распознан. Расход рассчитан без зачёта этого улучшения.',
    'experimental-incompatible': 'Экспериментальный эффект несовместим с выбранным экземпляром.',
    'requires-blueprint': 'Сначала примените чертёж к выбранному экземпляру.',
    'live-only': 'Доступно в режиме Live.'
  };
  const goalStatuses = {
    pending: 'Запланировано', complete: 'Выполнено', unavailable: 'Нужна проверка'
  };
  const stopStatuses = {
    ready: 'Мастерская доступна', 'rank-required': 'Повысить ранг инженера', 'access-required': 'Открыть мастерскую',
    blocked: 'Сначала выполните условия', 'access-unknown': 'Доступ к мастерской неизвестен'
  };
  const blockedReasons = {
    'loadout-unavailable': 'Журнал ещё не подтвердил текущую сборку.',
    'module-changed': 'В этом слоте установлен другой модуль.',
    'goal-incompatible': 'Выбранное улучшение больше недоступно для этого модуля.',
    'workshop-unavailable': 'Для выбранного улучшения не найдена мастерская.',
    'access-barred': 'Текущий доступ к мастерской ограничен.'
  };
  const coreSlots = {
    armour: 'Броня', powerplant: 'Силовая установка', mainengines: 'Двигатели',
    frameshiftdrive: 'Рамочно-смещающий двигатель', lifesupport: 'Система жизнеобеспечения',
    powerdistributor: 'Распределитель питания', radar: 'Сенсоры', fueltank: 'Топливный бак'
  };

  function slotName(module, u) {
    const slot = String(module.slot || '');
    const raw = slot.toLowerCase();
    if (module.slotLabel) return u.t(module.slotLabel);
    if (coreSlots[raw]) return u.t(coreSlots[raw]);
    const optional = raw.match(/^(slot|military)(\d+)_size\d+$/);
    if (optional) return u.t(optional[1] === 'military' ? 'Военный отсек {number}' : 'Внутренний отсек {number}', {number: Number(optional[2])});
    const hardpoint = raw.match(/^(tiny|small|medium|large|huge)hardpoint(\d+)$/);
    if (hardpoint) {
      const caption = {
        tiny: 'Служебное крепление {number}', small: 'Малое крепление {number}', medium: 'Среднее крепление {number}',
        large: 'Большое крепление {number}', huge: 'Огромное крепление {number}'
      }[hardpoint[1]];
      return u.t(caption, {number: Number(hardpoint[2])});
    }
    return slot;
  }

  const owner = C => C.state?.planWorkspace?.key || '';
  const identity = C => JSON.stringify([owner(C), C.state?.commander?.fid, C.state?.commander?.shipId, C.state?.commander?.ship]);
  const revision = C => [C.state?.engineeringRoadmapRevision ?? C.state?.shipBuildRevision ?? C.state?.revision, C.state?.commander?.system];
  function model(C) {
    const key = identity(C);
    if (C.shipEngineering?.key !== key) C.shipEngineering = {key, view: 'modules', filter: 'all', query: '', target: {}, sequence: 0};
    return C.shipEngineering;
  }

  const tr = (u, text, values) => u.esc(u.t(text, values));
  const number = (u, value) => value == null ? '—' : u.esc(u.fmt(value));
  const title = (u, value, name = 'name', english = 'english') => /^en\b/i.test(u.locale())
    ? value?.[english] || u.t(value?.[name] || '')
    : value?.[name] || value?.[english] || '';
  const grade = (u, value) => Number(value) > 0 ? 'G' + number(u, value) : '—';
  const ref = (u, id, label) => id && label
    ? `<button type="button" class="table-entity-link" data-reference-id="${u.esc(id)}">${u.esc(label)}</button>`
    : u.esc(label || '');
  const entity = (u, kind, key, label) => ref(u, key ? kind + '/' + key : '', label);
  const targetFields = row => ({blueprintKey: row.blueprintKey, grade: row.grade, experimentalKey: row.experimentalKey || ''});
  const savedGoals = data => rows(data?.plan?.goals);
  const savedGoal = (data, slot) => savedGoals(data).find(goal => goal.slot === slot);
  const count = (u, value, item) => {
    if (value == null) return number(u, value);
    const sign = item.estimated ? '≈ ' : item.partial ? '≥ ' : '';
    return sign + number(u, value) + (item.estimated && item.partial ? ' + …' : '');
  };

  function materialTable(u, ingredients, {inventory = true, supplies = false, perRoll = false, remaining = false} = {}) {
    const quantityLabel = perRoll ? 'На одну попытку' : remaining ? 'Осталось' : inventory ? 'Требуется' : 'Количество';
    const missingLabel = perRoll ? 'Не хватает на попытку' : 'Не хватает';
    return `<div class="ship-eng-table"><table><thead><tr>
      <th scope="col">${tr(u, supplies ? 'Предмет или платёж' : 'Материал')}</th><th scope="col">${tr(u, quantityLabel)}</th>
      ${inventory ? `<th scope="col">${tr(u, 'В наличии')}</th><th scope="col">${tr(u, missingLabel)}</th>` : ''}
      </tr></thead><tbody>${ingredients.map(item => `<tr>
        <th scope="row">${ref(u, item.entryId || (!supplies && item.key && 'material/' + item.key), title(u, item) || item.key)}</th>
        <td>${count(u, item.count, item)}</td>${inventory ? `<td>${number(u, item.owned)}</td>
        <td class="${Number(item.missing) > 0 ? 'ship-eng-deficit' : ''}">${item.partial && item.missing === 0 ? '—' : count(u, item.missing, item)}</td>` : ''}</tr>`).join('')}</tbody></table></div>`;
  }

  function place(u, engineer) {
    if (!engineer?.system && !engineer?.base) return '';
    const location = u.esc([engineer.system, engineer.base].filter(Boolean).join(' · '));
    return `<span class="ship-eng-place">${location}${engineer.system ? `<button type="button" class="icon-button"
      data-click="table-copy-system" data-system="${u.esc(engineer.system)}" aria-label="${tr(u, 'Копировать систему {system}', {system: engineer.system})}"
      title="${tr(u, 'Копировать систему')}">${u.icon('copy')}</button>` : ''}</span>`;
  }

  function engineerRow(u, engineer, role = '') {
    const status = access[engineer.status];
    const limit = Number(engineer.maxGrade) > 0 ? tr(u, 'Чертёж до {grade}', {grade: 'G' + u.fmt(engineer.maxGrade)}) : '';
    const rank = Number(engineer.rank) > 0 ? tr(u, 'Ранг {grade}', {grade: 'G' + u.fmt(engineer.rank)}) : '';
    const available = Number(engineer.accessibleGrade) > 0 && engineer.accessibleGrade < engineer.maxGrade
      ? tr(u, 'Доступно сейчас: {grade}', {grade: 'G' + u.fmt(engineer.accessibleGrade)}) : '';
    return `<div class="ship-eng-engineer"><div><strong>${entity(u, 'engineer', engineer.key, title(u, engineer))}</strong>
      ${place(u, engineer)}</div><div class="ship-eng-access">
      ${[role ? tr(u, role) : '', status ? tr(u, status) : '', rank, limit, available].filter(Boolean).join(' · ')}</div></div>`;
  }

  function requirementList(u, steps) {
    return `<ol>${steps.map(step => {
      const description = title(u, step, 'text', 'english') ||
        (step.kind === 'rank' && Number(step.target) > 0 ? u.t('Поднимите ранг инженера до G{grade}.', {grade: u.fmt(step.target)}) : '');
      const prereq = entity(u, 'engineer', step.engineerKey, title(u, step, 'engineerName', 'engineerEnglish'));
      const progress = step.current != null && step.target != null ? number(u, step.current) + ' / ' + number(u, step.target) : '';
      const requirements = rows(step.items);
      return `<li><span>${u.esc(description)}</span>${prereq ? ' · ' + prereq : ''}
        ${progress ? `<small>${progress}</small>` : ''}
        ${requirements.length ? materialTable(u, requirements, {inventory: false, supplies: true}) : ''}</li>`;
    }).join('')}</ol>`;
  }

  function unlockSteps(u, chain) {
    const work = chain.map(engineer => ({engineer, steps: rows(engineer.steps).filter(step => step.status !== 'done')}))
      .filter(row => row.steps.length);
    if (!work.length) return '';
    return `<section class="ship-eng-unlock"><h3>${tr(u, 'Шаги для доступа')}</h3>${work.map(({engineer, steps}) =>
      `<div class="ship-eng-unlock-stage"><strong>${entity(u, 'engineer', engineer.key, title(u, engineer))}</strong>
      ${requirementList(u, steps)}</div>`).join('')}</section>`;
  }

  function remainingGrades(u, recipe) {
    const steps = rows(recipe.grades).filter(step => step.status !== 'done' && step.rolls != null && rows(step.materials).length);
    const effect = rows(recipe.experimentalMaterials);
    if (!steps.length && !effect.length) return '';
    return `<details class="ship-eng-breakdown" data-ship-eng-disclosure="materials"><summary>${tr(u, 'Расход по ступеням')}</summary>
      ${steps.map(step => `<section><h4>${grade(u, step.grade)} <small>${tr(u, 'Попыток по расчёту: {count}', {count: u.fmt(step.rolls)})}</small></h4>
        ${materialTable(u, step.materials, {inventory: false})}</section>`).join('')}
      ${effect.length ? `<section><h4>${tr(u, 'Экспериментальный эффект')}</h4>${materialTable(u, effect, {inventory: false})}</section>` : ''}</details>`;
  }

  function recipeMarkup(u, recipe) {
    if (!recipe) return '';
    const notes = rows(recipe.conditions).map(condition => conditions[condition]).filter(Boolean);
    const unknownSteps = rows(recipe.grades).filter(step => step.status !== 'done' && step.rolls == null && rows(step.perRoll).length);
    const materialKey = row => row.key || row.entryId || row.name;
    const totals = new Map(rows(recipe.totals).concat(rows(recipe.currencies)).map(row => [materialKey(row), row]));
    for (const step of unknownSteps) for (const row of step.perRoll) {
      const key = materialKey(row);
      const known = totals.get(key);
      totals.set(key, known ? {...known, partial: true} : {...row, count: null, missing: null});
    }
    const pending = rows(recipe.grades).some(step => step.status !== 'done') ||
      (recipe.experimentalStatus && recipe.experimentalStatus !== 'done');
    const confidence = recipe.totalsPartial ? 'Часть расходов известна только на одну попытку.' :
      recipe.confidence === 'unknown' && pending ? 'Точное количество материалов неизвестно.' : '';
    return `${notes.map(note => `<p class="ship-eng-note">${tr(u, note)}</p>`).join('')}
      ${confidence ? `<p class="ship-eng-note">${tr(u, confidence)}</p>` : ''}
      ${recipe.totalsEstimated ? `<p class="ship-eng-note">${tr(u, 'Знак ≈ отмечает оценку оставшегося расхода.')}</p>` : ''}
      ${[...totals.values()].some(item => item.estimated && item.partial) ? `<p class="ship-eng-note">${tr(u, 'Многоточие означает, что к оценке добавится неизвестный расход.')}</p>` : ''}
      ${totals.size ? `<section data-ship-eng-materials><h3>${tr(u, 'Материалы для улучшений')}</h3>${materialTable(u, [...totals.values()], {remaining: true})}</section>` : ''}
      ${unknownSteps.map(step => `<section><h4>${grade(u, step.grade)} <small>${tr(u, 'На одну попытку')}</small></h4>
        ${materialTable(u, step.perRoll, {perRoll: true})}</section>`).join('')}
      ${!pending && !totals.size && recipe.confidence === 'exact' && recipe.compatible !== false ? `<p>${tr(u, 'Это улучшение уже установлено.')}</p>` : ''}
      ${remainingGrades(u, recipe)}`;
  }

  function select(u, field, label, values, value) {
    const id = 'ship-eng-' + field;
    return `<label for="${id}"><span>${tr(u, label)}</span><select id="${id}" data-ship-eng-target="${field}">
      ${values.map(([key, text]) => `<option value="${u.esc(key)}" ${String(key) === String(value ?? '') ? 'selected' : ''}>${u.esc(text)}</option>`).join('')}</select></label>`;
  }

  function detailMarkup(u, data, r) {
    const error = r.error ? `<p role="alert">${u.esc(r.error)}</p>${retry(u)}` : '';
    if (!data?.detail || data.detail.slot !== r.slot) return error || `<p role="status">${tr(u, 'Загрузка…')}</p>`;
    const detail = data.detail;
    const module = rows(data.modules).find(row => String(row.slot || row.id) === r.slot);
    const blueprint = rows(detail.blueprints).find(row => row.key === detail.blueprintKey);
    const experimentals = rows(detail.experimentals);
    const effect = experimentals.find(row => row.key === detail.experimentalKey);
    const links = [
      module?.moduleKey ? entity(u, 'module', module.moduleKey, title(u, module)) : '',
      blueprint && blueprint.key !== module?.blueprintKey ? entity(u, 'blueprint', blueprint.key, title(u, blueprint)) : '',
      effect && effect.key !== module?.experimentalKey ? entity(u, 'experimental', effect.key, title(u, effect)) : ''
    ].filter(Boolean);
    const maxGrade = Math.min(5, Math.max(0, Number(blueprint?.maxGrade) || 0));
    const grades = Array.from({length: maxGrade}, (_, i) => [i + 1, 'G' + u.fmt(i + 1)]);
    const preferred = detail.engineer || detail.recipe?.engineer;
    const available = detail.availableEngineer?.key !== preferred?.key ? detail.availableEngineer : null;
    const candidates = rows(blueprint?.engineers).filter(engineer => engineer.key !== preferred?.key && engineer.key !== available?.key);
    if (!rows(detail.blueprints).length) return `<p>${tr(u, 'Для этого модуля нет доступных чертежей в справочнике.')}</p>`;
    const saved = savedGoal(data, r.slot);
    const matches = saved && saved.status !== 'unavailable' && saved.blueprintKey === detail.blueprintKey &&
      Number(saved.grade) === Number(detail.grade) && (saved.experimentalKey || '') === (detail.experimentalKey || '');
    const saveAction = data.shipID == null || data.shipID === '' ? '' : `<div class="ship-eng-save">
      <button id="ship-eng-save" type="button" class="pill primary" data-ship-eng-action="upsert"
        ${r.busy || r.writing || r.error || matches ? 'disabled' : ''}>${tr(u, matches ? 'Сохранено в плане' : saved ? 'Обновить цель' : 'Добавить улучшение в план')}</button>
      </div>`;
    return `${error}<div class="ship-eng-target" ${r.busy ? 'aria-busy="true"' : ''}>
      ${select(u, 'blueprintKey', 'Чертёж', rows(detail.blueprints).map(row => [row.key, title(u, row)]), detail.blueprintKey)}
      ${grades.length ? select(u, 'grade', 'Целевая ступень', grades, detail.grade) : ''}
      ${experimentals.length ? select(u, 'experimentalKey', 'Экспериментальный эффект',
        [['', u.t('Без эффекта')], ...experimentals.map(row => [row.key, title(u, row)])], detail.experimentalKey) : ''}
      </div>
      ${saveAction}
      ${links.length ? `<div class="ship-eng-reference">${links.join(' · ')}</div>` : ''}
      ${preferred || available || candidates.length ? `<section><h3>${tr(u, 'Инженеры')}</h3>
        ${preferred ? engineerRow(u, preferred, available || candidates.length ? 'Для целевой ступени' : '') : ''}
        ${available ? engineerRow(u, available) : ''}
        ${candidates.length ? `<details class="ship-eng-breakdown" data-ship-eng-disclosure="engineers"><summary>${tr(u, 'Другие инженеры ({count})', {count: u.fmt(candidates.length)})}</summary>
          ${candidates.map(engineer => engineerRow(u, engineer)).join('')}</details>` : ''}</section>` : ''}
      ${unlockSteps(u, rows(detail.unlockSteps))}
      ${recipeMarkup(u, detail.recipe)}`;
  }

  function moduleRow(u, module, r) {
    const slot = String(module.slot || module.id);
    const open = r.slot === slot;
    const name = title(u, module) || module.item;
    const blueprint = title(u, module, 'blueprintName', 'blueprintEnglish');
    const effect = title(u, module, 'experimentalName', 'experimentalEnglish');
    const quality = module.quality == null || !Number.isFinite(Number(module.quality)) ? null :
      Math.round(Math.max(0, Math.min(1, Number(module.quality))) * 100);
    const saved = savedGoal(r.data, slot);
    const current = blueprint ? entity(u, 'blueprint', module.blueprintKey, blueprint) : '';
    const status = statuses[module.status] ? module.status : 'unknown';
    const expand = tr(u, open ? 'Свернуть расчёт для {module}' : 'Рассчитать улучшения для {module}', {module: name});
    return `<tr data-ship-eng-row="${u.esc(slot)}"><th scope="row"><button type="button" class="ship-eng-module"
        data-ship-eng-slot="${u.esc(slot)}" aria-expanded="${open}" aria-label="${expand}">
        ${u.icon('chevron-right')}<span><strong>${u.esc([module.moduleClass, name].filter(Boolean).join(' '))}</strong>
        <small>${u.esc(slotName(module, u))}</small></span></button></th>
      <td><span class="ship-eng-status ${status}">${tr(u, statuses[status][0])}</span>
        ${saved && saved.status !== 'unavailable' ? `<small>${tr(u, 'В плане')} · ${grade(u, saved.grade)}</small>` : ''}</td>
      <td>${current || (Number(module.grade) > 0 ? tr(u, 'Чертёж неизвестен') : '—')}
        ${effect ? `<small>${entity(u, 'experimental', module.experimentalKey, effect)}</small>` : ''}</td>
      <td>${grade(u, module.grade)}${Number(module.maxGrade) > 0 ? ' / ' + grade(u, module.maxGrade) : ''}
        ${quality != null ? `<small class="ship-eng-quality"><progress max="100" value="${quality}" aria-label="${tr(u, 'Готовность ступени')}"></progress>${number(u, quality)}%</small>` : ''}</td></tr>
      ${open ? `<tr><td class="ship-eng-details" colspan="4"><div data-ship-eng-detail="${u.esc(slot)}">${detailMarkup(u, r.data, r)}</div></td></tr>` : ''}`;
  }

  function listMarkup(u, r) {
    const query = r.query.trim().toLocaleLowerCase();
    const modules = rows(r.data?.modules).filter(module => {
      if (r.filter !== 'all' && module.status !== r.filter) return false;
      return !query || [module.name, module.english, module.slot, slotName(module, u), module.moduleClass, module.blueprintName,
        module.blueprintEnglish].some(value => String(value || '').toLocaleLowerCase().includes(query));
    });
    return modules.length ? `<div class="ship-eng-table ship-eng-modules"><table>
      <thead><tr>${['Модуль и слот', 'Статус', 'Установленный чертёж', 'Ступень'].map(label => `<th scope="col">${tr(u, label)}</th>`).join('')}</tr></thead>
      <tbody>${modules.map(module => moduleRow(u, module, r)).join('')}</tbody></table></div>` :
      `<p class="ship-eng-note" role="status">${tr(u, 'Ничего не найдено')}</p>`;
  }

  const retry = u => `<button type="button" class="pill" data-ship-eng-retry>${tr(u, 'Повторить')}</button>`;
  const goalName = (u, goal) => [goal.moduleClass, title(u, goal) || slotName(goal, u)].filter(Boolean).join(' ');
  const goalUpgrade = (u, goal) => [title(u, goal, 'blueprintName', 'blueprintEnglish') || goal.blueprintKey,
    goal.grade ? 'G' + u.fmt(goal.grade) : '', title(u, goal, 'experimentalName', 'experimentalEnglish') || goal.experimentalKey].filter(Boolean).join(' · ');

  function planView(C, u) {
    const r = model(C), data = r.data, plan = data.plan, goals = savedGoals(data);
    const error = r.error ? `<p role="alert">${u.esc(r.error)}</p>` : '';
    if (!goals.length) return error + `<p class="ship-eng-note">${tr(u, 'Выберите модуль и добавьте улучшение в план корабля.')}</p>`;
    const materials = rows(plan.materials).concat(rows(plan.currencies));
    const supplies = rows(plan.unlockSupplies);
    const stops = rows(plan.itinerary?.stops);
    const blocked = rows(plan.itinerary?.blocked);
    const done = goals.every(goal => goal.status === 'complete');
    const modules = rows(data.modules);
    const goalTable = `<div class="ship-eng-table ship-eng-goals"><table>
      <thead><tr>${['Модуль и слот', 'Выбранное улучшение', 'Статус'].map(label => `<th scope="col">${tr(u, label)}</th>`).join('')}
        <th scope="col"><span class="visually-hidden">${tr(u, 'Действия')}</span></th></tr></thead>
      <tbody>${goals.map(goal => {
        const canEdit = modules.some(module => module.slot === goal.slot);
        return `<tr data-ship-eng-goal="${u.esc(goal.slot)}">
          <th scope="row">${canEdit ? `<button type="button" class="table-entity-link" data-ship-eng-action="edit"
            data-ship-eng-goal-slot="${u.esc(goal.slot)}">${u.esc(goalName(u, goal))}</button>` : u.esc(goalName(u, goal))}
            <small>${u.esc(slotName(goal, u))}</small></th>
          <td>${u.esc(goalUpgrade(u, goal))}</td><td>${tr(u, goalStatuses[goal.status] || 'Неизвестно')}</td>
          <td><button type="button" class="icon-button" data-ship-eng-action="remove" data-ship-eng-goal-slot="${u.esc(goal.slot)}"
            title="${tr(u, 'Удалить цель из плана')}" aria-label="${tr(u, 'Удалить из плана: {module}', {module: goalName(u, goal)})}"
            ${r.writing || data.shipID == null || data.shipID === '' ? 'disabled' : ''}>${u.icon('x')}</button></td></tr>`;
      }).join('')}</tbody></table></div>`;
    const costs = `<section class="ship-eng-plan-panel"><h3>${tr(u, 'Сводка материалов')}</h3>
      ${plan.totalsPartial ? `<p class="ship-eng-note">${tr(u, 'В сводке учтён известный расход. Знак ≥ означает минимальное количество.')}</p>` : ''}
      ${plan.totalsEstimated ? `<p class="ship-eng-note">${tr(u, 'Знак ≈ отмечает оценку оставшегося расхода.')}</p>` : ''}
      ${materials.some(item => item.estimated && item.partial) ? `<p class="ship-eng-note">${tr(u, 'Многоточие означает, что к оценке добавится неизвестный расход.')}</p>` : ''}
      ${materials.length ? materialTable(u, materials) : !plan.totalsPartial ?
        `<p class="ship-eng-note">${tr(u, 'Материалы для выбранных целей не требуются.')}</p>` : ''}
      ${supplies.length ? `<h4>${tr(u, 'Взносы за доступ к мастерским')}</h4>${materialTable(u, supplies, {supplies: true})}` : ''}
      </section>`;
    const itinerary = `<section class="ship-eng-plan-panel"><h3>${tr(u, 'Порядок посещения инженеров')}</h3>
      ${stops.length ? `<ol class="ship-eng-visits">${stops.map(stop => `<li data-ship-eng-visit="${u.esc(stop.id)}">
        <div><strong>${ref(u, stop.entryId, title(u, stop))}</strong>
          ${place(u, {...stop, base: stop.station})}</div>
        <p class="ship-eng-note">${stopStatuses[stop.status] ? tr(u, stopStatuses[stop.status]) : ''}
          ${stop.distance != null ? `<span>${number(u, stop.distance)}${tr(u, ' св. л.')}</span>` : ''}</p>
        ${rows(stop.steps).length ? requirementList(u, stop.steps) : ''}
        ${rows(stop.tasks).length ? `<ul>${stop.tasks.map(goal =>
          `<li>${u.esc([goalName(u, goal), goalUpgrade(u, goal)].filter(Boolean).join(' · '))}</li>`).join('')}</ul>` : ''}
      </li>`).join('')}</ol>` : done ? `<p class="ship-eng-note">${tr(u, 'Все выбранные улучшения уже установлены.')}</p>` : ''}
      ${blocked.length ? `<div class="ship-eng-blocked"><h4>${tr(u, 'Требуется подготовка')}</h4>
        <ul>${blocked.map(goal => `<li><strong>${u.esc(goalName(u, goal))}</strong>
          ${tr(u, blockedReasons[goal.reason] || 'Требуется проверка цели.')}</li>`).join('')}</ul></div>` : ''}
      </section>`;
    return error + goalTable + `<div class="ship-eng-plan-grid">${costs}${itinerary}</div>`;
  }

  function content(C, u) {
    const r = model(C), data = r.data;
    if (!data) return r.error ? `<p role="alert">${u.esc(r.error)}</p>${retry(u)}` :
      `<p class="ship-eng-note" role="status">${tr(u, 'Загрузка…')}</p>`;
    const ship = data.ship || C.state?.commander?.ship;
    const name = data.shipName && data.shipName !== ship ? [data.shipName, ship] : [ship];
    const headline = name[0] ? `<h2>${u.esc(name[0])}${name[1] ? `<small>${u.esc(name[1])}</small>` : ''}</h2>` : '';
    const views = data.plan ? `<div class="ship-eng-filters" role="group" aria-label="${tr(u, 'Текущий корабль')}">
      <button type="button" class="pill" data-ship-eng-view="modules" aria-pressed="${r.view === 'modules'}">${tr(u, 'Модули')}</button>
      <button type="button" class="pill" data-ship-eng-view="plan" aria-pressed="${r.view === 'plan'}">${tr(u, 'План корабля')}
        ${savedGoals(data).length ? `<small>${number(u, savedGoals(data).length)}</small>` : ''}</button></div>` : '';
    if (data.plan && r.view === 'plan') return headline + views + planView(C, u);
    if (!data.loadoutKnown) return `${headline}${views}<p class="ship-eng-note">${tr(u, 'Откройте снаряжение текущего корабля в игре, чтобы журнал записал его модули.')}</p>`;
    if (!rows(data.modules).length) return `${headline}${views}<p class="ship-eng-note">${tr(u, 'На корабле нет распознанных модулей, доступных для инженерии.')}</p>`;
    const counts = [['all', 'Все модули корабля', rows(data.modules).length], ...Object.entries(statuses).map(([key, [label, summary]]) =>
      [key, label, data.summary?.[summary] ?? rows(data.modules).filter(row => row.status === key).length]).filter(([, , n]) => n > 0)];
    return `<div class="ship-eng-toolbar">${headline}<input id="ship-eng-search" type="search" autocomplete="off"
      aria-label="${tr(u, 'Поиск модулей корабля')}" placeholder="${tr(u, 'Найти модуль или слот')}" value="${u.esc(r.query)}"></div>${views}
      <div class="ship-eng-filters" role="group" aria-label="${tr(u, 'Статус улучшения')}">${counts.map(([key, label, value]) =>
        `<button class="pill" type="button" data-ship-eng-filter="${key}" aria-pressed="${r.filter === key}">${tr(u, label)} <small>${number(u, value)}</small></button>`).join('')}</div>
      ${r.error && !r.slot ? `<p role="alert">${u.esc(r.error)}</p>${retry(u)}` : ''}
      <div data-ship-eng-list>${listMarkup(u, r)}</div>`;
  }

  function render(C, u) {
    return u.heading(u.t(mode), '') + `<div id="ship-engineering" class="ship-eng">${content(C, u)}</div>`;
  }

  function draw(C, u, listOnly = false) {
    if (!active(C)) return;
    const r = model(C), host = u.$('#ship-engineering');
    if (!host) return;
    const root = listOnly ? host.querySelector('[data-ship-eng-list]') : host;
    if (!root) return;
    const focus = host.ownerDocument.activeElement;
    const focused = root.contains(focus);
    const open = r.slot && [...host.querySelectorAll(`[data-ship-eng-detail] [data-ship-eng-disclosure][open]`)]
      .filter(node => node.closest('[data-ship-eng-detail]')?.dataset.shipEngDetail === r.slot)
      .map(node => node.dataset.shipEngDisclosure);
    const newMarkup = listOnly ? listMarkup(u, r) : content(C, u);
    if (root.shipEngMarkup === newMarkup) return;
    root.innerHTML = newMarkup;
    root.shipEngMarkup = newMarkup;
    if (open) for (const node of host.querySelectorAll('[data-ship-eng-disclosure]')) {
      if (open.includes(node.dataset.shipEngDisclosure)) node.open = true;
    }
    if (focused) {
      const slot = focus.dataset?.shipEngSlot;
      const filter = focus.dataset?.shipEngFilter;
      const view = focus.dataset?.shipEngView;
      const action = focus.dataset?.shipEngAction;
      const next = (focus.id && host.ownerDocument.getElementById(focus.id)) ||
        (slot && [...host.querySelectorAll('[data-ship-eng-slot]')].find(node => node.dataset.shipEngSlot === slot)) ||
        (filter && [...host.querySelectorAll('[data-ship-eng-filter]')].find(node => node.dataset.shipEngFilter === filter)) ||
        (view && host.querySelector(`[data-ship-eng-view="${r.view}"]`)) ||
        (action && [...host.querySelectorAll('[data-ship-eng-action]')].find(node =>
          node.dataset.shipEngAction === action && node.dataset.shipEngGoalSlot === focus.dataset?.shipEngGoalSlot));
      next?.focus({preventScroll: true});
    }
  }

  const stampFor = (C, r) => JSON.stringify([revision(C), r.slot, r.target]);
  function setData(r, data) {
    r.data = data;
    if (r.slot && !rows(data.modules).some(row => String(row.slot || row.id) === r.slot)) {
      r.slot = '';
      r.target = {};
    }
    if (r.filter !== 'all' && !rows(data.modules).some(row => row.status === r.filter)) r.filter = 'all';
  }

  function load(C, u) {
    if (!active(C)) return;
    const r = model(C), stamp = stampFor(C, r);
    if (r.writing) return r.write;
    if (r.stamp === stamp) return r.read;
    r.stamp = stamp;
    r.busy = true;
    r.error = '';
    const request = ++r.sequence;
    const body = {expectedPlanWorkspace: owner(C)};
    if (r.slot) Object.assign(body, {slot: r.slot}, r.target);
    r.read = (async () => {
      try {
        const data = await u.call(operation, body);
        if (model(C) !== r || request !== r.sequence) return;
        setData(r, data);
      } catch (error) {
        if (model(C) === r && request === r.sequence) r.error = u.t(error.message);
      } finally {
        if (model(C) === r && request === r.sequence) {
          r.busy = false;
          draw(C, u);
        }
      }
    })();
    return r.read;
  }

  function selectSlot(C, u, slot, fromPlan = false) {
    const r = model(C);
    if (r.writing) return;
    const saved = savedGoal(r.data, slot);
    r.slot = !fromPlan && r.slot === slot ? '' : slot;
    r.target = !r.slot || !saved ? {} : saved.status === 'unavailable' ? {blueprintKey: ''} : targetFields(saved);
    if (fromPlan) {
      r.view = 'modules';
      r.filter = 'all';
      r.query = '';
    }
    r.error = '';
    const pending = load(C, u);
    draw(C, u, !fromPlan);
    if (fromPlan) {
      const selected = [...u.$$('#ship-engineering [data-ship-eng-slot]')].find(node => node.dataset.shipEngSlot === slot);
      selected?.focus({preventScroll: true});
      selected?.scrollIntoView({block: 'nearest'});
    }
    return pending;
  }

  function updatePlan(C, u, action, slot = '') {
    const r = model(C), data = r.data, detail = data?.detail;
    if (r.writing || !owner(C) || data?.shipID == null || data.shipID === '') return;
    const input = {action, slot, expectedPlanWorkspace: owner(C), expectedShipID: String(data.shipID)};
    if (action === 'upsert') {
      const module = rows(data.modules).find(row => String(row.slot || row.id) === slot);
      if (r.busy || r.error || detail?.slot !== slot || !module?.item || !detail.blueprintKey) return;
      Object.assign(input, targetFields(detail), {item: module.item});
    }
    r.sequence++;
    r.busy = false;
    r.writing = true;
    r.error = '';
    const before = JSON.stringify(revision(C));
    draw(C, u);
    r.write = (async () => {
      try {
        const result = await u.call(updateOperation, input);
        if (model(C) !== r) return;
        if (action === 'upsert') r.target = targetFields(detail);
        if (action === 'remove' && r.slot === slot) {
          r.slot = '';
          r.target = {};
        }
        setData(r, !result.detail && r.slot ? {...result, detail: data.detail} : result);
        r.stamp = before === JSON.stringify(revision(C)) ? stampFor(C, r) : '';
        if (active(C)) u.toast?.(u.t(action === 'upsert' ? 'Цель сохранена' : 'Цель удалена'));
      } catch (error) {
        if (model(C) === r) r.error = u.t(error.message);
      } finally {
        if (model(C) === r) {
          r.writing = false;
          draw(C, u);
          if (!r.error && r.stamp === '') void load(C, u);
        }
      }
    })();
    return r.write;
  }

  function init(C, u) {
    if (!active(C)) return;
    const host = u.$('#ship-engineering');
    if (host && !host.shipEngBound) {
      host.shipEngBound = true;
      host.addEventListener('click', event => {
        const button = event.target.closest('[data-ship-eng-slot], [data-ship-eng-filter], [data-ship-eng-retry], [data-ship-eng-view], [data-ship-eng-action]');
        if (!button || !active(C)) return;
        const r = model(C);
        const {shipEngSlot, shipEngFilter, shipEngView, shipEngAction, shipEngGoalSlot} = button.dataset;
        if (shipEngAction === 'upsert') {
          void updatePlan(C, u, 'upsert', r.slot);
        } else if (shipEngAction === 'remove') {
          void updatePlan(C, u, 'remove', shipEngGoalSlot);
        } else if (shipEngAction === 'edit') {
          void selectSlot(C, u, shipEngGoalSlot, true);
        } else if (shipEngSlot) {
          void selectSlot(C, u, shipEngSlot);
        } else if (shipEngView) {
          r.view = shipEngView;
          draw(C, u);
        } else if (shipEngFilter) {
          r.filter = shipEngFilter;
          draw(C, u);
        } else {
          r.stamp = '';
          void load(C, u);
        }
      });
      host.addEventListener('input', event => {
        if (event.target.id !== 'ship-eng-search') return;
        model(C).query = event.target.value;
        draw(C, u, true);
      });
      host.addEventListener('change', event => {
        const field = event.target.dataset.shipEngTarget;
        if (!field) return;
        const r = model(C), detail = r.data?.detail, value = event.target.value;
        if (r.writing || !detail) return;
        const effective = targetFields(detail);
        r.target = {...effective, ...r.target, [field]: field === 'grade' ? Number(value) : value};
        if (field === 'blueprintKey') {
          delete r.target.grade;
          delete r.target.experimentalKey;
        }
        void load(C, u);
        const save = host.querySelector('#ship-eng-save');
        if (save) save.disabled = true;
      });
    }
    void load(C, u);
  }

  async function smoke(C, u) {
    await u.navigate('engineering', {mode});
    await load(C, u);
    const r = model(C), modules = rows(r.data?.modules);
    const checks = [{name: 'Current ship engineering renders observed modules', ok: r.data?.loadoutKnown === true &&
      modules.length > 0 && !!u.$('[data-ship-eng-row]')}];
    const module = modules.find(item => item.moduleKey === 'fsd');
    checks.push({name: 'Current ship engineering reads the installed FSD blueprint and grade', ok:
      !!module?.blueprintKey && module.grade > 0 && !!u.$('[data-ship-eng-row] [data-reference-id^="blueprint/"]')});
    const target = [...u.$$('#ship-engineering [data-ship-eng-slot]')].find(button => button.dataset.shipEngSlot === module?.slot);
    if (target) target.click();
    await r.read;
    checks.push({name: 'Current ship engineering opens a slot calculation', ok: !!target && r.data?.detail?.slot === module?.slot &&
      !!u.$('[data-ship-eng-target="blueprintKey"]') && !!u.$('[data-ship-eng-detail] [data-reference-id^="engineer/"]')});
    const second = modules.find(item => item.moduleKey === 't');
    if (!module || !second) {
      checks.push({name: 'Ship engineering plan fixture has two modules', ok: false});
      return checks;
    }
    const click = selector => {
      const control = u.$(selector);
      if (!control || control.disabled) return false;
      control.click();
      return true;
    };
    const firstSaved = click('[data-ship-eng-action="upsert"]');
    await r.write;
    const secondButton = [...u.$$('#ship-engineering [data-ship-eng-slot]')].find(button => button.dataset.shipEngSlot === second.slot);
    secondButton?.click();
    await r.read;
    const gradeControl = u.$('[data-ship-eng-target="grade"]');
    if (gradeControl && [...gradeControl.options].some(option => option.value === '3')) {
      gradeControl.value = '3';
      gradeControl.dispatchEvent(new Event('change', {bubbles: true}));
      await r.read;
    }
    const secondSaved = click('[data-ship-eng-action="upsert"]');
    await r.write;
    checks.push({name: 'Ship engineering plan saves two slot-specific targets', ok: firstSaved && secondSaved &&
      !!savedGoal(r.data, module.slot) && !!savedGoal(r.data, second.slot)});
    await u.navigate('overview');
    delete C.shipEngineering;
    await u.navigate('engineering', {mode});
    await load(C, u);
    const restored = model(C);
    const shown = click('[data-ship-eng-view="plan"]');
    const goals = savedGoals(restored.data);
    checks.push({name: 'Ship engineering plan reloads and shows aggregate materials and workshop order', ok: shown &&
      goals.length === 2 && rows(restored.data?.plan?.materials).length > 0 && rows(restored.data?.plan?.itinerary?.stops).length > 0 &&
      u.$$('#ship-engineering [data-ship-eng-goal]').length === 2 && !!u.$('#ship-engineering [data-ship-eng-visit]')});
    for (const goal of goals) {
      const remove = [...u.$$('#ship-engineering [data-ship-eng-action="remove"]')]
        .find(button => button.dataset.shipEngGoalSlot === goal.slot);
      remove?.click();
      await restored.write;
    }
    checks.push({name: 'Ship engineering plan removes saved slot targets', ok: goals.length === 2 &&
      !savedGoals(restored.data).length && !u.$('#ship-engineering [data-ship-eng-goal]')});
    click('[data-ship-eng-view="modules"]');
    const scanner = [...u.$$('#ship-engineering [data-ship-eng-slot]')].find(button => button.dataset.shipEngSlot === 'Slot12_Size1');
    scanner?.click();
    await restored.read;
    const scannerRecipe = restored.data?.detail?.recipe;
    const scannerTotal = u.$('[data-ship-eng-detail="Slot12_Size1"] [data-ship-eng-materials]');
    const materialRowMatches = (key, owned, missing) => {
      const material = rows(scannerRecipe?.totals).find(item => item.key === key);
      const reference = scannerTotal?.querySelector(`[data-reference-id="material/${key}"]`);
      const cells = reference?.closest('tr')?.querySelectorAll('td');
      return material?.count === 1 && material.owned === owned && material.missing === missing &&
        cells?.[0]?.textContent === '≈ ' + u.fmt(1) && cells?.[1]?.textContent === u.fmt(owned) &&
        cells?.[2]?.textContent === '≈ ' + u.fmt(missing);
    };
    checks.push(
      {name: 'Ship engineering DSS G5 fixture opens its slot and renders estimated materials', ok:
        !!scanner && !!scannerTotal && scannerRecipe?.totalsEstimated === true &&
        [...scannerTotal.querySelectorAll('th[scope="col"]')].map(cell => cell.textContent).join('|') ===
          ['Материал', 'Осталось', 'В наличии', 'Не хватает'].map(label => u.t(label)).join('|')},
      {name: 'Ship engineering DSS G5 shows on-hand mechanical components and two shortages', ok:
        materialRowMatches('mechanicalcomponents', 73, 0) && materialRowMatches('tin', 0, 1) &&
        materialRowMatches('protoradiolicalloys', 0, 1)}
    );
    return checks;
  }

  window.ShipEngineeringUI = {active, render, init, smoke};
})();
