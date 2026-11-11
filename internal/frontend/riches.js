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

  function draft(C) {
    if (!C.richesDraft) C.richesDraft = {};
    const { route = {}, suggestedJumpRangeLy } = state(C);
    return {
      origin: C.state?.commander?.system || route.origin || '',
      jumpRangeLy: route.jumpRangeLy || suggestedJumpRangeLy || 30,
      radiusLy: route.radiusLy || 100,
      ...C.richesDraft,
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
    const targetDetails = targetName
      ? [u.t(typeLabels[target.type] || target.type), u.t('Оценка DSS {credits}', { credits: known(target.mappingCredits) ? u.fmt(target.mappingCredits) + u.t(' кр.') : '—' })].filter(Boolean).join(' · ')
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
        ${metric(u, 'Оценка всего маршрута', credits(u, progress.totalMappingCredits), text(u, 'По наградам DSS'))}
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
    const fields = [
      ['jumpRangeLy', 'Дальность прыжка', ' св. л.'],
      ['radiusLy', 'Радиус поиска от стартовой системы', ' св. л.'],
    ];
    return `<section class="panel riches-plan" aria-labelledby="riches-plan-title">
      <header class="riches-section-head"><h2 id="riches-plan-title">${u.icon('compass')}${text(u, 'План маршрута')}</h2></header>
      <form id="riches-route-form">
        <label class="riches-field">
          <span>${text(u, 'Стартовая система')}</span>
          <input id="riches-origin" name="origin" data-riches-field="origin" data-system-search="single" value="${u.esc(values.origin)}" type="text" maxlength="160" autocomplete="off" required ${disabled}>
        </label>
        <div class="riches-distance-fields">
          ${fields.map(([key, label, unit]) => `<label class="riches-field">
            <span>${text(u, label)}</span><span class="riches-unit-input">
              <input id="riches-${key}" name="${key}" data-riches-field="${key}" type="text" inputmode="decimal" maxlength="12" value="${u.esc(values[key])}" required ${disabled}><span>${text(u, unit)}</span>
            </span>
          </label>`).join('')}
        </div>
        <div class="riches-plan-actions">
          ${u.btn('Построить маршрут', 'riches-submit', 'route', 'primary', `type="submit" ${disabled}`)}
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

  function bodyRows(stop, progress, u) {
    const bodies = Array.isArray(stop.bodies) ? stop.bodies : [];
    if (!bodies.length) return '';
    const current = stop.system === progress.currentSystem;
    const next = stop.system === progress.nextSystem;
    const tone = current ? 'is-current' : next ? 'is-next' : stop.visited ? 'is-visited' : '';
    const tag = current ? 'Вы здесь' : next ? 'Следующая система' : '';
    const system = `<th scope="rowgroup" rowspan="${bodies.length}"><span class="riches-system"><strong>${u.esc(stop.system)}</strong>
      ${u.btn('', 'riches-copy-system', 'copy', 'icon-only quiet', `data-system="${u.esc(stop.system)}" title="${text(u, 'Копировать систему')}" aria-label="${text(u, 'Копировать систему {system}', { system: stop.system })}"`)}
      </span>${tag ? `<small>${text(u, tag)}</small>` : ''}</th>`;
    const hops = `<td class="riches-number" rowspan="${bodies.length}">${number(u, stop.estimatedJumps)}</td>`;
    return `<tbody class="riches-stop ${tone}">${bodies.map((body, i) => {
      const status = body.mapped ? 'Картографировано' : body.scanned ? 'Сканировано' : '';
      const type = typeLabels[body.type] || body.type;
      const designation = body.designation || body.name;
      return `<tr>
        ${i === 0 ? system : ''}
        <td><span class="riches-body-name">${u.esc(designation)}</span>${body.name && designation !== body.name ? `<small>${u.esc(body.name)}</small>` : ''}${status ? `<small class="riches-body-status">${u.icon('check')}${text(u, status)}</small>` : ''}</td>
        <td><span class="riches-world-type ${body.type === 'earthLike' ? 'riches-earth' : 'riches-water'}">${text(u, type)}</span></td>
        <td class="riches-number">${number(u, body.distanceLs, ' св. с.')}</td>
        <td class="riches-number">${credits(u, body.scanCredits)}</td>
        <td class="riches-number riches-map-value">${credits(u, body.mappingCredits)}</td>
        ${i === 0 ? hops : ''}
      </tr>`;
    }).join('')}</tbody>`;
  }

  function results(C, u) {
    const { route, progress = {} } = state(C);
    if (!route?.stops?.some(stop => stop.bodies?.length)) return '';
    const columns = ['Система', 'Планета', 'Тип планеты', 'От звезды', 'Награда FSS', 'Награда DSS', 'Прыжки (оценка)'];
    const origin = route.origin ? `<span>${text(u, 'Старт · {system}', { system: route.origin })}</span>` : '';
    return `<section class="panel riches-results" aria-labelledby="riches-results-title">
      <header class="riches-section-head"><h2 id="riches-results-title">${u.icon('route')}${text(u, 'Цели картографирования')}</h2>${origin}</header>
      <div class="riches-table-scroll" tabindex="0" role="region" aria-labelledby="riches-results-title">
        <table class="riches-table"><thead><tr>${columns.map((label, i) => `<th scope="col" class="${i >= 3 ? 'riches-number' : ''}">${text(u, label)}</th>`).join('')}</tr></thead>
          ${route.stops.map(stop => bodyRows(stop, progress, u)).join('')}
        </table>
      </div>
      <p class="muted">${text(u, 'Награда DSS — оценка выплаты за планету после картографирования. Фактический маршрут между точками рассчитывает галактическая карта.')}</p>
      ${route.truncated ? `<p class="muted">${text(u, 'Показано до 100 ближайших систем с подходящими планетами.')}</p>` : ''}
    </section>`;
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
    if (!['origin', 'jumpRangeLy', 'radiusLy'].includes(key)) return false;
    draft(C);
    C.richesDraft[key] = event.target.value;
    return true;
  }

  function positiveNumber(value, u, label, min, max) {
    const raw = String(value ?? '').trim().replace(',', '.');
    const result = Number(raw);
    if (!/^(?:\d+(?:\.\d+)?|\.\d+)$/.test(raw) || !Number.isFinite(result) || result < min || result > max) {
      throw new Error(u.t('Для поля «{field}» допустимы значения от {min} до {max}.', { field: u.t(label), min: u.fmt(min), max: u.fmt(max) }));
    }
    return result;
  }

  async function mutate(C, u, operation, values) {
    if (C.richesPending) return;
    C.richesPending = operation;
    u.renderPage();
    try {
      await u.call(operation, values ? { values } : {});
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
    const values = Object.fromEntries(['origin', 'jumpRangeLy', 'radiusLy'].map(key => [key, formNode.elements.namedItem(key)?.value || '']));
    C.richesDraft = values;
    const origin = values.origin.trim();
    if (!origin) throw new Error(u.t('Укажите стартовую систему.'));
    const jumpRangeLy = positiveNumber(values.jumpRangeLy, u, 'Дальность прыжка', 0.1, 500);
    const radiusLy = positiveNumber(values.radiusLy, u, 'Радиус поиска от стартовой системы', 1, 5000);
    const expectedPlanWorkspace = C.state.planWorkspace?.key;
    await mutate(C, u, 'riches.plan', { origin, jumpRangeLy, radiusLy, ...(expectedPlanWorkspace ? { expectedPlanWorkspace } : {}) });
  }

  async function handleClick(action, button, C, u) {
    if (action === 'riches-copy-next') {
      await u.call('riches.copy');
      u.toast(u.t('Система скопирована'));
    } else if (action === 'riches-copy-system' && button.dataset.system) {
      await u.call('native', { action: 'copy', text: button.dataset.system });
      u.toast(u.t('Система скопирована'));
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
