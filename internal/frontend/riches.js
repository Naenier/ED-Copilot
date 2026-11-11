/* Road to Riches shows the route and journal progress maintained by Go. */
(() => {
  'use strict';

  const typeLabels = { earthLike: 'Землеподобная планета', waterWorld: 'Водная планета' };
  const state = C => C.state?.riches || {};
  const text = (u, key, vars) => u.esc(u.t(key, vars));
  const known = value => typeof value === 'number' && Number.isFinite(value) && value >= 0;
  const number = (u, value, unit = '') => known(value) ? u.esc(u.fmt(value)) + (unit ? text(u, unit) : '') : '—';
  const credits = (u, value) => number(u, value, ' кр.');
  const field = node => node?.dataset?.richesField;
  const numericFields = {
    jumpRangeLy: { label: 'Дальность прыжка', unit: ' св. л.', min: 0.1, max: 500 },
    radiusLy: { label: 'Радиус поиска', unit: ' св. л.', min: 1, max: 5000 },
    maxResults: { label: 'Максимум систем', min: 1, max: 2000, integer: true },
    maxDistanceLs: { label: 'Макс. расстояние от точки прибытия', unit: ' св. с.', min: 0, max: 1000000 },
    minMappingValue: { label: 'Минимальная награда DSS', unit: ' кр.', min: 0, max: 1000000000, integer: true },
  };
  const checkboxFields = {
    useMappingValue: 'Подбирать планеты по награде DSS',
    avoidThargoids: 'Избегать известных систем с таргоидами',
    loop: 'Вернуться в стартовую систему',
  };
  const formKeys = ['origin', 'destination', ...Object.keys(numericFields), ...Object.keys(checkboxFields)];
  const pageSize = 50;

  function draft(C) {
    const owner = C.state?.planWorkspace?.key || '';
    if (!C.richesDraft || C.richesDraftOwner !== owner) {
      C.richesDraft = {};
      C.richesDraftOwner = owner;
      C.richesSearchHidden = false;
      C.richesPage = 0;
    }
    const { route, search, suggestedJumpRangeLy } = state(C);
    const oldRoute = { origin: route?.origin, jumpRangeLy: route?.jumpRangeLy, radiusLy: route?.radiusLy };
    const saved = { ...(route?.maxResults ? route : oldRoute), ...route?.options, ...search?.options, ...C.richesDraft };
    return {
      origin: saved.origin ?? C.state?.commander?.system ?? '',
      destination: saved.destination ?? '',
      jumpRangeLy: saved.jumpRangeLy ?? suggestedJumpRangeLy ?? 30,
      radiusLy: saved.radiusLy ?? 100,
      maxResults: saved.maxResults ?? 100,
      maxDistanceLs: saved.maxDistanceLs ?? 10000,
      minMappingValue: saved.minMappingValue ?? 1000000,
      useMappingValue: saved.useMappingValue ?? true,
      avoidThargoids: saved.avoidThargoids ?? true,
      loop: saved.loop ?? true,
    };
  }

  function metric(u, label, value, detail = '') {
    return `<div class="riches-metric"><dt>${text(u, label)}</dt><dd>${value}</dd>${detail ? `<small>${detail}</small>` : ''}</div>`;
  }

  function summary(C, u, overview = false) {
    const { progress = {}, route } = state(C);
    if (!(progress.totalStops > 0 || route?.stops?.length)) return '';
    const { nextSystem, finished } = progress;
    const target = progress.target || progress.nextTarget;
    const completed = known(progress.completedJumps) ? progress.completedJumps : 0;
    const remaining = known(progress.remainingJumps) ? progress.remainingJumps : 0;
    const completedStops = known(progress.completedStops) ? progress.completedStops : 0;
    const totalStops = known(progress.totalStops) ? progress.totalStops : route?.stops?.length || 0;
    const targetName = target?.name || target?.designation;
    const mappingBasis = progress.useMappingValue !== false;
    const targetReward = mappingBasis ? target?.mappingCredits : target?.scanCredits;
    const targetDetails = targetName
      ? [u.t(typeLabels[target.type] || target.type), u.t(mappingBasis ? 'Оценка DSS {credits}' : 'Оценка FSS {credits}', { credits: known(targetReward) ? u.fmt(targetReward) + u.t(' кр.') : '—' })].filter(Boolean).join(' · ')
      : '';
    const nextName = nextSystem || (finished ? u.t('Маршрут завершён') : '—');
    const actions = overview
      ? u.btn('Открыть маршрут', 'navigate-riches', 'arrow-right', 'quiet')
      : (nextSystem ? u.btn('Копировать следующую систему', 'riches-copy-next', 'copy', 'quiet') : '');
    return `<section class="panel riches-summary" aria-labelledby="riches-summary-title">
      <header class="riches-section-head">
        <h2 id="riches-summary-title">${u.icon(finished ? 'circle-check' : 'route')}${text(u, overview ? 'Дорога к богатству' : 'Прогресс маршрута')}</h2>${actions}
      </header>
      <dl class="riches-metrics">
        ${metric(u, progress.target ? 'Планета в текущей системе' : 'Планета в следующей системе', u.esc(targetName || '—'), u.esc(targetDetails))}
        ${metric(u, 'Следующая система', u.esc(nextName))}
        ${metric(u, 'Оценка всего маршрута', credits(u, progress.totalRewardCredits ?? progress.totalMappingCredits), text(u, mappingBasis ? 'По наградам DSS' : 'По наградам FSS'))}
        ${metric(u, 'Прыжки · сделано / осталось', number(u, completed) + ' / ' + number(u, remaining), remaining ? text(u, 'Остаток по оценке') : '')}
      </dl>
      ${totalStops > 0 ? `<div class="riches-progress">
        <progress max="${totalStops}" value="${Math.min(completedStops, totalStops)}" aria-label="${text(u, 'Посещённые системы')}"></progress>
        <span>${text(u, 'Системы · {visited} / {total}', { visited: u.fmt(completedStops), total: u.fmt(totalStops) })}</span>
      </div>` : ''}
    </section>`;
  }

  function fssGlyph(type) {
    const counts = type === 'earthLike' ? [1, 2, 2] : [2, 2, 2];
    const marks = counts.map((count, row) => {
      const height = 3 + row * 7;
      return (count === 1 ? [12] : [8, 16]).map(x => `M${x - 2} ${height + 4}l2 -4 2 4`).join(' ');
    }).join(' ');
    return `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="${marks}" fill="none" stroke="currentColor" stroke-width="1.65" stroke-linejoin="round" stroke-linecap="round"/></svg>`;
  }

  function scanner(u) {
    return `<section class="panel riches-scanner" aria-labelledby="riches-fss-title">
      <header class="riches-section-head">
        <h2 id="riches-fss-title">${u.icon('orbit')}${text(u, 'Сигналы FSS')}</h2>
        <a class="riches-source" href="#" data-external="https://ed-dsn.net/en/in-depth-spectral-analysis/">${text(u, 'Справка по FSS')}${u.icon('external-link')}</a>
      </header>
      <div class="riches-spectrum" aria-hidden="true">
        <svg viewBox="0 0 400 65" preserveAspectRatio="none">
          <path class="riches-spectrum-base" d="M0 31H400M0 55H400"/>
          <path class="riches-spectrum-wave" d="M39 31l4 -5 3 13 3 -18 2 14 3 -4M117 31l5 -4 2 10 3 -18 3 20 3 -13 3 5 3 -2M154 31l3 -9 3 18 3 -12 2 3M302 31l2 -9 2 16 3 -12 3 5M340 31l3 -5 3 15 3 -19 3 14 4 -8 4 3"/>
          <path class="riches-spectrum-earth" d="M202 31l3 -5 2 11 2 -20 3 27 3 -32 3 26 3 -16 2 10 2 -1M209 55h10"/>
          <path class="riches-spectrum-water" d="M227 31l4 -6 2 14 2 -25 3 31 3 -29 3 23 3 -15 3 7M232 55h13"/>
        </svg>
      </div>
      <div class="riches-fss-legend">
        <span class="riches-earth">${fssGlyph('earthLike')}${text(u, typeLabels.earthLike)}</span>
        <span class="riches-water">${fssGlyph('waterWorld')}${text(u, typeLabels.waterWorld)}</span>
      </div>
      <p class="muted">${text(u, 'Положение сигналов показано схематично. Проверьте тип планеты в блоке «Анализ сигнала» сканера FSS.')}</p>
    </section>`;
  }

  function form(C, u) {
    const { route, search = {}, networkDisabled } = state(C);
    const values = draft(C);
    const busy = Boolean(C.richesPending || search.running);
    const disabled = busy || networkDisabled ? 'disabled' : '';
    const hidden = Boolean(C.richesSearchHidden);
    const systemInput = (key, label, required = false) => `<label class="riches-field">
      <span>${text(u, label)}</span>
      <input id="riches-${key}" name="${key}" data-riches-field="${key}" data-system-search="single" value="${u.esc(values[key])}" type="text" maxlength="160" autocomplete="off" ${required ? 'required' : ''} ${disabled}>
    </label>`;
    const numericInput = ([key, { label, unit, integer }]) => `<label class="riches-field">
      <span>${text(u, label)}</span><span class="${unit ? 'riches-unit-input' : ''}">
        <input id="riches-${key}" name="${key}" data-riches-field="${key}" type="text" inputmode="${integer ? 'numeric' : 'decimal'}" maxlength="12" value="${u.esc(values[key])}" required ${disabled}>${unit ? `<span>${text(u, unit)}</span>` : ''}
      </span>
    </label>`;
    return `<section class="panel riches-plan" aria-labelledby="riches-plan-title">
      <header class="riches-section-head">
        <h2 id="riches-plan-title">${u.icon('compass')}${text(u, 'План маршрута')}</h2>
        ${route?.stops?.length && !busy ? u.btn(hidden ? 'Показать поиск' : 'Скрыть поиск', 'riches-toggle-search', hidden ? 'chevron-down' : 'arrow-up', 'quiet', `aria-expanded="${!hidden}" aria-controls="riches-route-form"`) : ''}
      </header>
      <form id="riches-route-form" ${hidden && !busy ? 'hidden' : ''}>
        <div class="riches-system-fields">
          ${systemInput('origin', 'Стартовая система', true)}
          ${systemInput('destination', 'Конечная система (необязательно)')}
        </div>
        <div class="riches-form-fields">
          ${Object.entries(numericFields).map(numericInput).join('')}
        </div>
        <div class="riches-form-options">
          ${Object.entries(checkboxFields).map(([key, label]) => `<label class="riches-option">
            <input id="riches-${key}" type="checkbox" name="${key}" data-riches-field="${key}" ${values[key] ? 'checked' : ''} ${disabled}>
            <span>${text(u, label)}</span>
          </label>`).join('')}
        </div>
        <p class="riches-form-hint muted">${text(u, 'Возврат действует, если конечная система не указана. Ноль в поле награды DSS или расстояния снимает ограничение.')}</p>
        <div class="riches-plan-actions">
          ${u.btn('Построить маршрут', 'riches-submit', 'route', 'primary', `type="submit" ${disabled}`)}
          ${u.btn('Поменять местами', 'riches-swap', 'arrow-left-right', 'quiet', `data-riches-swap ${disabled || !(String(values.origin).trim() && String(values.destination).trim()) ? 'disabled' : ''}`)}
          ${search.running ? u.btn('Отменить поиск', 'riches-cancel', 'x', 'quiet', C.richesPending ? 'disabled' : '') : ''}
          ${route?.stops?.length && !busy ? u.btn('Очистить маршрут', 'riches-clear', 'x', 'quiet') : ''}
        </div>
      </form>
      ${networkDisabled ? `<p class="muted">${text(u, 'Для построения маршрута нужен доступ к Spansh.')}</p>` : ''}
    </section>`;
  }

  function routeStatus(C, u) {
    const { search = {}, route } = state(C);
    if (C.richesPending === 'riches.plan' || search.running) {
      return `<div class="riches-status" role="status">${u.icon('refresh-cw')}<span>${text(u, 'Ищем планеты и строим маршрут…')}</span></div>`;
    }
    if (search.error) {
      return `<div class="riches-status is-error" role="alert">${u.icon('circle-help')}<span>${text(u, search.error)}</span></div>`;
    }
    const built = ['Маршрут построен', u.t('Маршрут построен')].includes(search.status);
    if (search.status && (!route?.stops?.length || !built)) {
      return `<div class="riches-status" role="status">${u.icon('circle-help')}<span>${text(u, search.status)}</span></div>`;
    }
    return '';
  }

  function copySystemButton(system, u) {
    return u.btn('', 'riches-copy-system', 'copy', 'icon-only quiet', `data-system="${u.esc(system)}" title="${text(u, 'Копировать систему')}" aria-label="${text(u, 'Копировать систему {system}', { system })}"`);
  }

  function transitLabel(stop, route) {
    return stop.system === route.origin ? 'Возврат в стартовую систему' : 'Конечная система';
  }

  function bodyRows(stop, index, route, progress, C, u) {
    const bodies = Array.isArray(stop.bodies) ? stop.bodies : [];
    if (!bodies.length && !stop.transit) return '';
    const current = stop.system === progress.currentSystem && (!stop.transit || stop.visited);
    const next = stop.system === progress.nextSystem && (!known(progress.completedStops) || index === progress.completedStops);
    const tone = current ? 'is-current' : next ? 'is-next' : stop.visited ? 'is-visited' : '';
    const tag = current ? 'Вы здесь' : next ? 'Следующая система' : '';
    const rowSpan = Math.max(bodies.length, 1);
    const system = `<th scope="rowgroup" rowspan="${rowSpan}"><span class="riches-system"><strong>${u.esc(stop.system)}</strong>
      ${copySystemButton(stop.system, u)}
      </span>${tag ? `<small>${text(u, tag)}</small>` : ''}</th>`;
    const hops = `<td class="riches-number" rowspan="${rowSpan}">${number(u, stop.estimatedJumps)}</td>`;
    if (!bodies.length) {
      return `<tbody class="riches-stop ${tone}"><tr><td class="riches-done">—</td>${system}<td colspan="6" class="riches-transit">${text(u, transitLabel(stop, route))}</td>${hops}</tr></tbody>`;
    }
    return `<tbody class="riches-stop ${tone}">${bodies.map((body, i) => {
      const status = body.mapped ? 'Картографировано' : body.scanned ? 'Сканировано' : '';
      const type = typeLabels[body.type] || body.type;
      const designation = body.designation || body.name;
      const done = body.completed || body.mapped;
      const fromJournal = body.journalCompleted || body.mapped;
      return `<tr>
        <td class="riches-done"><input type="checkbox" data-click="riches-complete-body" data-system="${u.esc(stop.system)}" data-body="${u.esc(body.name)}" aria-label="${text(u, 'Отметить планету {body}', { body: body.name })}" ${done ? 'checked' : ''} ${fromJournal || C.richesPending ? 'disabled' : ''}></td>
        ${i === 0 ? system : ''}
        <td><span class="riches-body-name">${u.esc(designation)}</span>${body.name && designation !== body.name ? `<small>${u.esc(body.name)}</small>` : ''}${status ? `<small class="riches-body-status">${u.icon('check')}${text(u, status)}</small>` : ''}</td>
        <td><span class="riches-world-type ${body.type === 'earthLike' ? 'riches-earth' : 'riches-water'}">${text(u, type)}</span></td>
        <td>${body.terraformable == null ? '—' : text(u, body.terraformable ? 'Да' : 'Нет')}</td>
        <td class="riches-number">${number(u, body.distanceLs, ' св. с.')}</td>
        <td class="riches-number">${credits(u, body.scanCredits)}</td>
        <td class="riches-number riches-map-value">${credits(u, body.mappingCredits)}</td>
        ${i === 0 ? hops : ''}
      </tr>`;
    }).join('')}</tbody>`;
  }

  function results(C, u) {
    const { route, progress = {} } = state(C);
    if (!route?.stops?.length) return '';
    const hasTargets = route.stops.some(stop => stop.bodies?.length);
    const columns = ['Готово', 'Система', 'Планета', 'Тип планеты', 'Терраформируемая', 'От звезды', 'Награда FSS', 'Награда DSS', 'Прыжки (оценка)'];
    const { origin, destination, maxResults } = { ...route, ...route.options };
    const endpoints = [origin && u.t('Старт · {system}', { system: origin }), destination && u.t('Финиш · {system}', { system: destination })].filter(Boolean);
    const disabled = C.richesPending ? 'disabled' : '';
    const pages = Math.ceil(route.stops.length / pageSize);
    const page = Math.min(C.richesPage || 0, pages - 1);
    const offset = page * pageSize;
    const stops = route.stops.slice(offset, offset + pageSize);
    return `<section class="panel riches-results" aria-labelledby="riches-results-title">
      <header class="riches-section-head"><h2 id="riches-results-title">${u.icon('route')}${text(u, 'Цели картографирования')}</h2><span>${u.esc(endpoints.join(' / '))}</span></header>
      <div class="riches-result-actions">
        ${u.btn('Показать маршрут', 'riches-show-route', 'route', '', disabled)}
        ${u.btn('Сохранить CSV', 'riches-export-csv', 'arrow-down', 'quiet', disabled)}
        ${u.btn('Сохранить JSON', 'riches-export-json', 'arrow-down', 'quiet', disabled)}
      </div>
      <div class="riches-table-scroll" tabindex="0" role="region" aria-labelledby="riches-results-title">
        <table class="riches-table"><thead><tr>${columns.map((label, i) => `<th scope="col" class="${i >= 5 ? 'riches-number' : ''}">${text(u, label)}</th>`).join('')}</tr></thead>
          ${stops.map((stop, index) => bodyRows(stop, offset + index, route, progress, C, u)).join('')}
        </table>
      </div>
      ${pages > 1 ? `<div class="riches-pages">
        ${u.btn('Назад', 'riches-page-back', 'chevron-left', 'quiet', page ? '' : 'disabled')}
        <span>${text(u, 'Страница {page} из {pages}', { page: u.fmt(page + 1), pages: u.fmt(pages) })}</span>
        ${u.btn('Вперёд', 'riches-page-next', 'chevron-right', 'quiet', page + 1 < pages ? '' : 'disabled')}
      </div>` : ''}
      <p class="muted">${text(u, hasTargets ? 'Награда DSS — оценка выплаты за планету после картографирования. Фактический маршрут между точками рассчитывает галактическая карта.' : 'По заданным фильтрам планеты не найдены.')}</p>
      ${route.truncated ? `<p class="muted">${text(u, 'Показано до {max} систем с подходящими планетами.', { max: u.fmt(maxResults || 100) })}</p>` : ''}
    </section>`;
  }

  function routeDiagram(route, progress, u) {
    const origin = route.stops[0]?.system === route.origin ? [] : [route.originCoordinates];
    const coordinates = [...origin, ...route.stops.map(stop => stop.coordinates)];
    const points = coordinates.filter(point => Array.isArray(point) && [point[0], point[2]].every(Number.isFinite));
    if (points.length < 2) return '';
    const xs = points.map(point => point[0]), zs = points.map(point => point[2]);
    const minX = Math.min(...xs), maxX = Math.max(...xs), minZ = Math.min(...zs), maxZ = Math.max(...zs);
    if (minX === maxX && minZ === maxZ) return '';
    const scale = Math.min(440 / (maxX - minX), 134 / (maxZ - minZ));
    const project = point => [(240 + (point[0] - (minX + maxX) / 2) * scale).toFixed(1), (85 - (point[2] - (minZ + maxZ) / 2) * scale).toFixed(1)];
    const plot = points.map(project);
    const end = (name, point) => `<circle class="${name}" cx="${point[0]}" cy="${point[1]}" r="4"/>`;
    const next = route.stops[progress.completedStops]?.coordinates;
    return `<svg class="riches-itinerary-map" viewBox="0 0 480 170" role="img" aria-label="${text(u, 'Схема маршрута в плоскости X/Z')}">
      <polyline points="${plot.map(point => point.join(',')).join(' ')}"/>
      ${end('riches-map-start', plot[0])}${end('riches-map-end', plot.at(-1))}
      ${points.includes(next) ? end('riches-map-next', project(next)) : ''}
    </svg>`;
  }

  function showRoute(C, u) {
    const { route, progress = {} } = state(C);
    if (!route?.stops?.length) return;
    const start = route.stops[0].system === route.origin ? '' : `<li>
      <strong>${u.esc(route.origin)}</strong><small>${text(u, 'Стартовая система')}</small>${copySystemButton(route.origin, u)}
    </li>`;
    const stops = route.stops.map((stop, index) => {
      const next = index === progress.completedStops && stop.system === progress.nextSystem;
      const details = [
        stop.transit ? text(u, transitLabel(stop, route)) : text(u, 'Цели · {count}', { count: u.fmt(stop.bodies?.length || 0) }),
        known(stop.distanceLy) ? number(u, stop.distanceLy, ' св. л.') : '',
        known(stop.estimatedJumps) ? text(u, 'Прыжки (оценка) · {count}', { count: u.fmt(stop.estimatedJumps) }) : '',
      ].filter(Boolean).join(' · ');
      return `<li ${next ? 'aria-current="step"' : ''}>
        <strong>${u.esc(stop.system)}</strong><small>${details}</small>${copySystemButton(stop.system, u)}
      </li>`;
    }).join('');
    u.dialog(u.t('Маршрут по системам'), `${routeDiagram(route, progress, u)}<ol class="riches-itinerary">${start}${stops}</ol>`, '', true);
  }

  function render(C, u) {
    const description = 'Маршрут к водным и землеподобным мирам. ED Copilot копирует следующую систему при построении маршрута и по прибытии в каждую точку.';
    return `<div class="riches-page">
      ${u.heading(u.t('Дорога к богатству'), u.t(description))}
      ${summary(C, u)}
      <div class="riches-planning-grid">${form(C, u)}${scanner(u)}</div>
      ${routeStatus(C, u)}
      ${results(C, u)}
    </div>`;
  }

  function handleInput(event, C) {
    if (C.tab !== 'riches') return false;
    const key = field(event.target);
    if (!formKeys.includes(key)) return false;
    draft(C);
    C.richesDraft[key] = Object.hasOwn(checkboxFields, key) ? event.target.checked : event.target.value;
    if (key === 'origin' || key === 'destination') {
      const swap = event.target.form?.querySelector('[data-riches-swap]');
      const values = draft(C);
      if (swap) swap.disabled = !(String(values.origin).trim() && String(values.destination).trim());
    }
    return true;
  }

  function parseNumber(value, u, { label, min, max, integer }) {
    const raw = String(value ?? '').trim().replace(',', '.');
    const result = Number(raw);
    if (!/^(?:\d+(?:\.\d+)?|\.\d+)$/.test(raw) || !Number.isFinite(result) || result < min || result > max || (integer && !Number.isInteger(result))) {
      const message = integer ? 'Для поля «{field}» введите целое число от {min} до {max}.' : 'Для поля «{field}» допустимы значения от {min} до {max}.';
      throw new Error(u.t(message, { field: u.t(label), min: u.fmt(min), max: u.fmt(max) }));
    }
    return result;
  }

  async function mutate(C, u, operation, params) {
    if (C.richesPending) return;
    C.richesPending = operation;
    u.renderPage();
    try {
      await u.call(operation, params || {});
    } finally {
      delete C.richesPending;
      if (C.tab === 'riches' || C.tab === 'overview') {
        u.renderPage();
        await u.loadView();
      }
    }
  }

  async function submit(formNode, C, u) {
    if (C.tab !== 'riches' || state(C).search?.running || state(C).networkDisabled || C.richesPending) return;
    const values = draft(C);
    for (const key of formKeys) {
      const input = formNode.elements.namedItem(key);
      if (input) values[key] = (Object.hasOwn(checkboxFields, key) ? input.checked : input.value) ?? values[key];
    }
    C.richesDraft = values;
    const origin = String(values.origin).trim();
    if (!origin) throw new Error(u.t('Укажите стартовую систему.'));
    const destination = String(values.destination).trim();
    const numbers = Object.fromEntries(Object.entries(numericFields).map(([key, field]) => [key, parseNumber(values[key], u, field)]));
    const options = Object.fromEntries(Object.keys(checkboxFields).map(key => [key, Boolean(values[key])]));
    C.richesPage = 0;
    await mutate(C, u, 'riches.plan', { values: { origin, destination, ...numbers, ...options, ...workspaceToken(C) } });
  }

  function workspaceToken(C) {
    const expectedPlanWorkspace = C.state?.planWorkspace?.key;
    return typeof expectedPlanWorkspace === 'string' ? { expectedPlanWorkspace } : {};
  }

  async function handleClick(action, button, C, u) {
    if (action === 'riches-copy-next') {
      await u.call('riches.copy');
      u.toast(u.t('Система скопирована'));
    } else if (action === 'riches-copy-system' && button.dataset.system) {
      await u.call('native', { action: 'copy', text: button.dataset.system });
      u.toast(u.t('Система скопирована'));
    } else if (action === 'riches-toggle-search') {
      C.richesSearchHidden = !C.richesSearchHidden;
      u.renderPage();
    } else if (action === 'riches-swap') {
      const { origin, destination } = draft(C);
      if (!String(origin).trim() || !String(destination).trim()) return;
      Object.assign(C.richesDraft, { origin: destination, destination: origin });
      u.renderPage();
    } else if (action === 'riches-show-route') {
      showRoute(C, u);
    } else if (action === 'riches-export-csv' || action === 'riches-export-json') {
      const exported = await u.call('native', { action: 'export', kind: 'riches', json: action === 'riches-export-json' });
      if (!exported?.cancelled) u.toast(exported?.file ? u.t('Файл сохранён: ') + exported.file : u.t('Файл сохранён'));
    } else if (action === 'riches-page-back' || action === 'riches-page-next') {
      const pages = Math.ceil((state(C).route?.stops?.length || 1) / pageSize);
      C.richesPage = Math.min(pages - 1, Math.max(0, (C.richesPage || 0) + (action === 'riches-page-next' ? 1 : -1)));
      u.renderPage();
    } else if (action === 'riches-complete-body' && button.dataset.system && button.dataset.body) {
      await mutate(C, u, 'riches.complete', { system: button.dataset.system, body: button.dataset.body, completed: button.checked, ...workspaceToken(C) });
    } else if (action === 'riches-cancel') {
      await mutate(C, u, 'riches.cancel');
    } else if (action === 'riches-clear') {
      u.dialog(u.t('Очистить маршрут'), `<p>${text(u, 'Сохранённый маршрут и его прогресс будут удалены.')}</p>`,
        u.btn('Отмена', 'close-dialog') + u.btn('Очистить маршрут', 'riches-clear-confirm', 'x', 'primary'));
    } else if (action === 'riches-clear-confirm') {
      u.closeLayer();
      await mutate(C, u, 'riches.clear');
    }
  }

  window.RichesUI = { render, renderOverview: (C, u) => summary(C, u, true), submit, handleClick, handleInput };
})();
