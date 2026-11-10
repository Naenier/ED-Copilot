/* Live EDSM views for the encyclopedia. Only the open section is requested. */
(() => {
  'use strict';

  const tabs = [
    ['overview', 'Обзор'], ['bodies', 'Небесные тела'], ['stations', 'Станции'],
    ['factions', 'Фракции'], ['nearby', 'Соседние системы'],
  ];
  const systemsFilters = [
    ['allegiance', 'Принадлежность'], ['government', 'Правительство'],
    ['economy', 'Экономика'], ['security', 'Безопасность'],
  ];
  const vocabulary = {
    Federation: 'Федерация', Empire: 'Империя', Alliance: 'Альянс', Independent: 'Независимая',
    'Pilots Federation': 'Федерация пилотов', Anarchy: 'Анархия', Confederacy: 'Конфедерация',
    Cooperative: 'Кооператив', Corporate: 'Корпорация', Communism: 'Коммунизм', Democracy: 'Демократия',
    Dictatorship: 'Диктатура', Feudal: 'Феодализм', Patronage: 'Патронаж', Theocracy: 'Теократия',
    High: 'Высокая', Medium: 'Средняя', Low: 'Низкая', Lawless: 'Беззаконие', None: 'Нет',
    Agriculture: 'Сельское хозяйство', Industrial: 'Промышленность', Extraction: 'Добыча', Refinery: 'Переработка',
    'High Tech': 'Высокие технологии', Tourism: 'Туризм', Service: 'Услуги', Military: 'Военная',
    Colony: 'Колония', Terraforming: 'Терраформирование', Prison: 'Тюрьма',
    Expansion: 'Экспансия', Retreat: 'Отступление', Outbreak: 'Эпидемия', War: 'Война',
    'Civil War': 'Гражданская война', Election: 'Выборы', Boom: 'Бум', Bust: 'Спад',
    'Civil Liberty': 'Гражданская свобода', 'Civil Unrest': 'Гражданские беспорядки',
    Famine: 'Голод', Lockdown: 'Блокада', Investment: 'Инвестиции', Star: 'Звезда', Planet: 'Планета',
  };
  const cache = new WeakMap();
  const t = (u, key, vars) => u.t ? u.t(key, vars) : key.replace(/\{(\w+)\}/g, (whole, name) => vars?.[name] ?? whole);
  const label = (u, key, vars) => u.esc(t(u, key, vars));
  const esc = (u, value) => u.esc(String(value ?? ''));
  const list = value => Array.isArray(value) ? value : [];
  const present = value => value !== undefined && value !== null && value !== '';
  const numeric = value => present(value) && Number.isFinite(Number(value));
  const name = value => String(value ?? '').trim().slice(0, 150);
  const term = (u, value) => esc(u, present(value) ? (Object.hasOwn(vocabulary, value) ? t(u, vocabulary[value]) : value) : '—');
  const unit = (u, value, suffix, decimals = 2) => numeric(value) ? esc(u, window.CopilotI18n?.number?.(value, { maximumFractionDigits: decimals }) || u.fmt(Number(value))) + (suffix ? ' ' + label(u, suffix) : '') : '—';
  const date = (u, value) => {
    const raw = String(value ?? '');
    const stamp = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(raw) ? raw.replace(' ', 'T') + 'Z' : raw;
    return Number.isFinite(Date.parse(stamp)) ? esc(u, u.date?.(stamp, { dateStyle: 'medium', timeStyle: 'short' }) || new Date(stamp).toLocaleString()) : '';
  };
  const yesNo = (u, value) => typeof value === 'boolean' ? label(u, value ? 'Да' : 'Нет') : '—';
  const directoryVisible = (C, r) => C.tab === 'encyclopedia' && C.reference === r && r.category === 'systems' && !r.entryId;
  const readerVisible = (C, r) => window.ReferenceUI?.active(C) && C.reference === r && r.entry?.category === 'systems' && !!r.entryId;
  const center = C => name(C.state?.commander?.system) || 'Sol';

  function state(r) {
    if (!cache.has(r)) cache.set(r, { directory: null, reader: null });
    return cache.get(r);
  }
  function directory(C, r) {
    const s = state(r);
    if (!s.directory) s.directory = {
      exact: '', center: center(C), radius: '25', filters: {}, page: 0,
      initial: false, loading: false, token: 0, result: null, validation: '',
    };
    return s.directory;
  }
  function reader(r) {
    const s = state(r);
    if (!s.reader || s.reader.id !== r.entryId || s.reader.entryToken !== r.entryToken) s.reader = {
      id: r.entryId, entryToken: r.entryToken, tab: 'overview', requests: new Map(), filters: {}, pages: {},
    };
    return s.reader;
  }

  function systemButton(u, system) {
    return `<button type="button" class="ref-inline-link" data-click="reference-open" data-id="system/${esc(u, encodeURIComponent(system))}">${esc(u, system)}${u.icon('arrow-up-right')}</button>`;
  }
  function field(u, id, title, value, system = true) {
    return `<label for="${id}"><span>${label(u, title)}</span><input id="${id}" name="${system ? 'system' : 'radius'}" value="${esc(u, value)}" ${system ? 'type="search" data-system-search="single" maxlength="150" autocomplete="off"' : 'type="number" min="1" max="100" step="1"'} required></label>`;
  }
  function submit(u, key, icon) {
    return `<button type="submit" class="pill primary">${u.icon(icon)}${label(u, key)}</button>`;
  }
  function action(u, key, id, attrs = '') {
    return `<button type="button" class="pill" data-click="reference-galaxy-${id}" ${attrs}>${label(u, key)}</button>`;
  }
  function select(u, scope, key, title, value, choices) {
    return `<label><span>${label(u, title)}</span><select data-galaxy-filter="${key}" data-galaxy-scope="${scope}"><option value="">${label(u, 'Любое значение')}</option>${choices.map(([option, display]) => `<option value="${esc(u, option)}" ${value === option ? 'selected' : ''}>${display}</option>`).join('')}</select></label>`;
  }
  function choices(u, rows, key) {
    return Array.from(new Set(rows.map(row => row[key]).filter(Boolean))).sort().map(value => [value, term(u, value)]);
  }
  function factList(u, fields) {
    const visible = fields.filter(([, value]) => present(value) && value !== '—');
    return visible.length ? `<dl class="ref-properties galaxy-facts">${visible.map(([key, value]) => `<div><dt>${label(u, key)}</dt><dd>${value}</dd></div>`).join('')}</dl>` : '';
  }
  function table(u, columns, rows) {
    return `<div class="ref-table-wrap galaxy-table"><table class="ref-table"><thead><tr>${columns.map(column => `<th scope="col">${label(u, column)}</th>`).join('')}</tr></thead><tbody>${rows.map(row => `<tr>${row.map(cell => `<td>${cell}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`;
  }
  function status(u, result, pending, scope) {
    if (pending || !result) return `<p class="galaxy-notice" role="status">${label(u, 'Загрузка данных EDSM…')}</p>`;
    if (result.status === 'ok') return '';
    const message = {
      'not-found': 'Система не найдена в EDSM.',
      'unavailable': 'EDSM временно недоступен.',
      'rate-limited': 'EDSM временно ограничил запросы.',
      'disabled': 'Сетевой доступ к EDSM отключён.',
      'invalid': 'Проверьте название системы и радиус.',
    }[result.status] || 'EDSM временно недоступен.';
    const retry = date(u, result.retryAt);
    const canRetry = !['disabled', 'not-found', 'invalid'].includes(result.status);
    return `<div class="galaxy-notice" role="status"><span>${label(u, message)}${retry ? ' ' + label(u, 'Повторите после {time}.', { time: retry }) : ''}</span>${canRetry ? action(u, 'Повторить', 'retry', `data-scope="${scope}"`) : ''}</div>`;
  }
  function noRows(u) {
    return `<p class="galaxy-notice" role="status">${label(u, 'Нет записей для выбранных условий.')}</p>`;
  }
  function page(u, rows, number, scope, renderRows) {
    const size = 30, max = Math.max(0, Math.ceil(rows.length / size) - 1), selected = Math.min(number, max);
    if (!rows.length) return noRows(u);
    const start = selected * size, end = Math.min(start + size, rows.length);
    return renderRows(rows.slice(start, end)) + (max ? `<nav class="galaxy-pagination" aria-label="${label(u, 'Страницы результатов')}">${action(u, 'Назад', 'page', `data-scope="${scope}" data-step="-1" ${selected <= 0 ? 'disabled' : ''}`)}<span>${label(u, 'Показано {start}–{end} из {total}', { start: u.fmt(start + 1), end: u.fmt(end), total: u.fmt(rows.length) })}</span>${action(u, 'Далее', 'page', `data-scope="${scope}" data-step="1" ${selected >= max ? 'disabled' : ''}`)}</nav>` : '');
  }
  function nearbyRows(u, result, filters, pageNumber, scope) {
    const rows = list(result.rows).filter(row => row?.name && systemsFilters.every(([key]) => !filters[key] || row[key] === filters[key]) &&
      (!filters.population || numeric(row.population) && (filters.population === 'populated' ? row.population > 0 : Number(row.population) === 0)));
    const source = ['EDSM', 'Spansh'].find(value => value.toLowerCase() === String(result.source || '').toLowerCase());
    return (source ? `<p class="galaxy-note">${label(u, 'Источник: {source}', { source })}</p>` : '') +
      (result.truncated ? `<p class="galaxy-note">${label(u, 'Показана часть соседних систем. Уменьшите радиус, чтобы сузить поиск.')}</p>` : '') +
      page(u, rows, pageNumber, scope, selected => table(u,
        ['Система', 'Расстояние, св. л.', 'Принадлежность', 'Правительство', 'Экономика', 'Безопасность', 'Население'],
        selected.map(row => [systemButton(u, row.name), unit(u, row.distance), term(u, row.allegiance),
          term(u, row.government), term(u, row.economy), term(u, row.security), unit(u, row.population, '', 0)])));
  }
  function nearbyFilters(u, rows, filters, scope) {
    if (!rows.length) return '';
    const opts = [['populated', label(u, 'Населённые')], ['empty', label(u, 'Ненаселённые')]];
    return `<div class="galaxy-filters">${select(u, scope, 'population', 'Население', filters.population, opts)}${systemsFilters
      .filter(([key]) => rows.some(row => row[key])).map(([key, title]) => select(u, scope, key, title, filters[key], choices(u, rows, key))).join('')}</div>`;
  }
  function directoryResults(u, s) {
    if (s.validation) return `<p class="galaxy-notice" role="alert">${label(u, s.validation)}</p>`;
    if (s.loading || s.result?.status !== 'ok') return status(u, s.result, s.loading, 'directory');
    return nearbyFilters(u, list(s.result.rows), s.filters, 'directory') +
      `<div id="galaxy-directory-table" aria-live="polite">${nearbyRows(u, s.result, s.filters, s.page, 'directory')}</div>`;
  }
  function renderDirectory(C, r, u) {
    const s = directory(C, r);
    return `<section class="galaxy-directory" aria-label="${label(u, 'Каталог EDSM')}">
      <form id="galaxy-exact" class="galaxy-form">${field(u, 'galaxy-exact-name', 'Название системы', s.exact)}${submit(u, 'Открыть систему', 'search')}</form>
      <section class="galaxy-nearby-card"><h3>${label(u, 'Соседние системы')}</h3>
        <form id="galaxy-nearby" class="galaxy-form galaxy-radius-form">${field(u, 'galaxy-center', 'Исходная система', s.center)}${field(u, 'galaxy-radius', 'Радиус, св. л.', s.radius, false)}${submit(u, 'Найти системы', 'search')}</form>
        <div id="galaxy-directory-results" aria-live="polite">${directoryResults(u, s)}</div>
      </section>
    </section>`;
  }

  function overview(u, item) {
    const coordinates = [item.x, item.y, item.z];
    return factList(u, [
      ['Координаты X / Y / Z', coordinates.every(numeric) ? coordinates.map(value => unit(u, value, '', 5)).join(' / ') : '—'],
      ['Принадлежность', term(u, item.allegiance)], ['Правительство', term(u, item.government)],
      ['Экономика', [item.economy, item.secondEconomy].filter(present).map(value => term(u, value)).join(' · ')],
      ['Безопасность', term(u, item.security)], ['Население', unit(u, item.population, '', 0)],
      ['Контролирующая фракция', esc(u, item.faction)], ['Состояние фракции', term(u, item.factionState)],
      ['Разрешение на вход', item.permitRequired === true && item.permitName ? esc(u, item.permitName) : yesNo(u, item.permitRequired)],
      ['Главная звезда', [item.starName, item.starType].filter(present).map(value => esc(u, value)).join(' · ')],
      ['Заправка у звезды', yesNo(u, item.scoopable)],
    ]) || noRows(u);
  }
  function bodyMaterials(row) {
    return list(row.materials).filter(material => material?.name && numeric(material.percent)).sort((a, b) => b.percent - a.percent);
  }
  function bodyDetails(u, row) {
    const materials = bodyMaterials(row).map(material => `<span>${esc(u, material.name)} <strong>${unit(u, material.percent)}%</strong></span>`).join('');
    const rings = list(row.rings).map(ring => `<span>${esc(u, ring.name)} · ${term(u, ring.type)}${numeric(ring.innerRadius) && numeric(ring.outerRadius) ? ' · ' + unit(u, ring.innerRadius, '', 0) + '–' + unit(u, ring.outerRadius, 'км', 0) : ''}</span>`).join('');
    const details = factList(u, [
      ['Гравитация', unit(u, row.gravity, 'g')], ['Температура', unit(u, row.temperature, 'K')],
      ['Атмосфера', term(u, row.atmosphere)], ['Вулканизм', term(u, row.volcanism)], ['Терраформирование', term(u, row.terraforming)],
      ['Данные обновлены', date(u, row.updatedAt)],
    ]);
    if (!materials && !rings && !details) return '—';
    return `<details class="galaxy-details"><summary>${label(u, 'Подробнее')}</summary>${details}
      ${materials ? `<h4>${label(u, 'Материалы')}</h4><div class="galaxy-tags">${materials}</div>` : ''}
      ${rings ? `<h4>${label(u, 'Кольца')}</h4><div class="galaxy-tags">${rings}</div>` : ''}</details>`;
  }
  function stationDetails(u, row) {
    const fields = factList(u, [
      ['Принадлежность', term(u, row.allegiance)], ['Правительство', term(u, row.government)],
      ['Контролирующая фракция', esc(u, row.faction)], ['Рынок', yesNo(u, row.market)],
      ['Верфь', yesNo(u, row.shipyard)], ['Оснащение', yesNo(u, row.outfitting)], ['Данные обновлены', date(u, row.updatedAt)],
    ]);
    const services = list(row.services).map(value => `<span>${term(u, value)}</span>`).join('');
    return fields || services ? `<details class="galaxy-details"><summary>${label(u, 'Подробнее')}</summary>${fields}
      ${services ? `<h4>${label(u, 'Услуги')}</h4><div class="galaxy-tags">${services}</div>` : ''}</details>` : '—';
  }
  const stateLabels = (u, values) => list(values).map(value => typeof value === 'string' ? term(u, value) : term(u, value?.state || value?.name)).filter(value => value !== '—').join(', ');
  function sectionRows(u, section, rows, filters, pageNumber) {
    if (section === 'bodies') {
      rows = rows.filter(row => (!filters.type || row.type === filters.type) &&
        (!filters.landing || (filters.landing === 'landable' ? row.landable === true : row.landable === false)) &&
        (!filters.ring || list(row.rings).length) && (!filters.material || bodyMaterials(row).some(material => material.name === filters.material)));
      return page(u, rows, pageNumber, section, selected => table(u, ['Небесное тело', 'Тип', 'От точки прибытия, св. с.', 'Посадка', 'Характеристики'],
        selected.map(row => [esc(u, row.name), term(u, row.subType || row.type), unit(u, row.distance), yesNo(u, row.landable), bodyDetails(u, row)])));
    }
    if (section === 'stations') {
      rows = rows.filter(row => (!filters.type || row.type === filters.type) && (!filters.service || list(row.services).includes(filters.service)));
      return page(u, rows, pageNumber, section, selected => table(u, ['Станция', 'Тип', 'Экономика', 'От точки прибытия, св. с.', 'Услуги'],
        selected.map(row => [esc(u, row.name) + (present(row.body?.name || (typeof row.body === 'string' && row.body)) ? `<small>${esc(u, row.body?.name || row.body)}</small>` : ''),
          term(u, row.type), term(u, row.economy), unit(u, row.distance), stationDetails(u, row)])));
    }
    return page(u, rows, pageNumber, section, selected => table(u, ['Фракция', 'Принадлежность', 'Правительство', 'Влияние', 'Состояние фракции'],
      selected.map(row => [esc(u, row.name) + (row.player ? `<small>${label(u, 'Фракция игроков')}</small>` : ''),
        term(u, row.allegiance), term(u, row.government), numeric(row.influence) ? unit(u, row.influence) + '%' : '—',
        (stateLabels(u, row.activeStates) || term(u, row.state)) +
          (list(row.pendingStates).length ? `<small>${label(u, 'Ожидаются')}: ${stateLabels(u, row.pendingStates)}</small>` : '') +
          (list(row.recoveringStates).length ? `<small>${label(u, 'Завершаются')}: ${stateLabels(u, row.recoveringStates)}</small>` : '') +
          (date(u, row.updatedAt) ? `<small>${label(u, 'Данные обновлены')}: ${date(u, row.updatedAt)}</small>` : '')])));
  }
  function sectionFilters(u, section, rows, filters) {
    if (!rows.length || section === 'factions') return '';
    const type = select(u, section, 'type', 'Тип', filters.type, choices(u, rows, 'type'));
    if (section === 'bodies') {
      const materials = [...new Set(rows.flatMap(row => bodyMaterials(row).map(material => material.name)))].sort();
      return `<div class="galaxy-filters">${type}
        ${select(u, section, 'landing', 'Посадка', filters.landing, [['landable', label(u, 'Возможна')], ['not-landable', label(u, 'Невозможна')]])}
        ${select(u, section, 'ring', 'Кольца', filters.ring, [['with-rings', label(u, 'Есть кольца')]])}
        ${materials.length ? select(u, section, 'material', 'Материал', filters.material, materials.map(item => [item, esc(u, item)])) : ''}</div>`;
    }
    const services = [...new Set(rows.flatMap(row => list(row.services)))].sort();
    return `<div class="galaxy-filters">${type}${services.length ? select(u, section, 'service', 'Услуга', filters.service, services.map(item => [item, term(u, item)])) : ''}</div>`;
  }
  function panel(u, s) {
    const request = s.requests.get(s.tab), result = request?.result;
    if (request?.loading || result?.status !== 'ok') return status(u, result, request?.loading, 'reader');
    if (s.tab === 'overview') return result.system ? overview(u, result.system) : noRows(u);
    const filters = s.filters[s.tab] || {}, pageNumber = s.pages[s.tab] || 0;
    const rows = list(result.rows).filter(row => row?.name);
    const content = s.tab === 'nearby' ? nearbyRows(u, result, filters, pageNumber, s.tab) : sectionRows(u, s.tab, rows, filters, pageNumber);
    const info = s.tab === 'nearby' ? `<p class="galaxy-note">${label(u, 'Радиус: {radius} св. л.', { radius: 25 })}</p>` :
      result.truncated ? `<p class="galaxy-note">${label(u, 'Показана часть записей EDSM.')}</p>` : '';
    return info + (s.tab === 'nearby' ? nearbyFilters(u, rows, filters, s.tab) : sectionFilters(u, s.tab, rows, filters)) +
      `<div id="galaxy-reader-table" aria-live="polite">${content}</div>`;
  }
  function renderReader(r, u) {
    const s = reader(r);
    return `<section class="galaxy-reader" aria-label="EDSM"><nav class="segmented galaxy-tabs" aria-label="${label(u, 'Разделы системы')}">
      ${tabs.map(([id, title]) => `<button type="button" data-click="reference-galaxy-tab" data-id="${id}" class="${s.tab === id ? 'selected' : ''}" aria-pressed="${s.tab === id}">${label(u, title)}</button>`).join('')}
    </nav><div id="galaxy-system-panel" aria-live="polite">${panel(u, s)}</div></section>`;
  }

  function replace(u, selector, html) {
    const node = u.$(selector);
    if (node) node.innerHTML = html;
    else u.renderPage();
  }
  async function loadNearby(C, r, u, s) {
    const currentCenter = name(s.center), radius = Number(s.radius);
    const key = JSON.stringify([currentCenter.toLowerCase(), radius]);
    if (s.loading && s.requestKey === key) return;
    s.requestKey = key;
    const token = ++s.token;
    s.validation = !currentCenter || !Number.isInteger(radius) || radius < 1 || radius > 100 ? 'Укажите систему и радиус от 1 до 100 св. л.' : '';
    s.page = 0; s.filters = {}; s.result = null; s.loading = !s.validation;
    if (directoryVisible(C, r)) replace(u, '#galaxy-directory-results', directoryResults(u, s));
    if (s.validation) return;
    try {
      const result = await u.call('galaxy.nearby', { name: currentCenter, radius });
      if (token !== s.token) return;
      s.result = result || { status: 'unavailable' };
    } catch (_) {
      if (token !== s.token) return;
      s.result = { status: 'unavailable' };
    }
    s.loading = false;
    if (directoryVisible(C, r)) replace(u, '#galaxy-directory-results', directoryResults(u, s));
  }
  async function loadReader(C, r, u, force = false) {
    if (!readerVisible(C, r)) return;
    const s = reader(r), section = s.tab, currentName = name(r.entry.title);
    if (!currentName || !force && s.requests.has(section)) return;
    const previous = s.requests.get(section);
    if (previous?.loading || force && Date.parse(previous?.result?.retryAt) > Date.now()) return;
    const request = { loading: true, result: null };
    s.requests.set(section, request);
    const stillCurrent = () => readerVisible(C, r) && reader(r) === s && s.tab === section && s.requests.get(section) === request;
    if (force && stillCurrent()) replace(u, '#galaxy-system-panel', panel(u, s));
    try {
      request.result = (section === 'nearby' ?
        await u.call('galaxy.nearby', { name: currentName, radius: 25 }) :
        await u.call('galaxy.system', { name: currentName, section: section === 'overview' ? 'summary' : section })) || { status: 'unavailable' };
    } catch (_) {
      request.result = { status: 'unavailable' };
    }
    request.loading = false;
    if (stillCurrent()) replace(u, '#galaxy-system-panel', panel(u, s));
  }

  function init(C, r, u) {
    if (directoryVisible(C, r)) {
      const s = directory(C, r);
      if (!s.initial) { s.initial = true; void loadNearby(C, r, u, s); }
    }
    if (readerVisible(C, r)) void loadReader(C, r, u);
  }
  function handleInput(target, C, r) {
    const key = { 'galaxy-exact-name': 'exact', 'galaxy-center': 'center', 'galaxy-radius': 'radius' }[target?.id];
    if (!key || !directoryVisible(C, r)) return false;
    directory(C, r)[key] = String(target.value ?? '').slice(0, 150);
    return true;
  }
  function renderFiltered(u, s, scope) {
    if (scope === 'directory') {
      if (s.result?.status === 'ok') replace(u, '#galaxy-directory-table', nearbyRows(u, s.result, s.filters, s.page, scope));
      return;
    }
    const result = s.requests.get(scope)?.result, filters = s.filters[scope] || {}, pageNumber = s.pages[scope] || 0;
    if (!result || result.status !== 'ok') return;
    replace(u, '#galaxy-reader-table', scope === 'nearby' ? nearbyRows(u, result, filters, pageNumber, scope) :
      sectionRows(u, scope, list(result.rows).filter(row => row?.name), filters, pageNumber));
  }
  function handleChange(target, C, r, u) {
    const key = target?.dataset?.galaxyFilter, scope = target?.dataset?.galaxyScope;
    if (!key) return false;
    if (scope === 'directory' && directoryVisible(C, r)) {
      const s = directory(C, r);
      if (s.result?.status === 'ok') {
        s.filters[key] = target.value; s.page = 0;
        renderFiltered(u, s, scope);
      }
      return true;
    }
    if (!readerVisible(C, r)) return false;
    const s = reader(r);
    if (s.tab === scope) {
      (s.filters[scope] ||= {})[key] = target.value; s.pages[scope] = 0;
      renderFiltered(u, s, scope);
    }
    return true;
  }
  function submitForm(event, C, r, u, open) {
    if (!directoryVisible(C, r) || !['galaxy-exact', 'galaxy-nearby'].includes(event.target?.id)) return false;
    event.preventDefault();
    const s = directory(C, r);
    if (event.target.id === 'galaxy-nearby') void loadNearby(C, r, u, s);
    else {
      const system = name(s.exact);
      if (system) void open('system/' + encodeURIComponent(system));
    }
    return true;
  }
  async function handleClick(actionID, button, C, r, u) {
    const id = button?.dataset?.id, scope = button?.dataset?.scope;
    if (actionID === 'tab' && readerVisible(C, r) && tabs.some(([key]) => key === id)) {
      reader(r).tab = id;
      u.renderPage();
      u.$('.galaxy-tabs [data-id="' + id + '"]')?.focus?.({ preventScroll: true });
      await loadReader(C, r, u);
      return true;
    }
    if (actionID === 'retry') {
      if (scope === 'directory' && directoryVisible(C, r)) {
        const s = directory(C, r);
        if (!s.loading && !(Date.parse(s.result?.retryAt) > Date.now())) await loadNearby(C, r, u, s);
      }
      else if (scope === 'reader') await loadReader(C, r, u, true);
      return true;
    }
    if (actionID === 'page') {
      const step = Number(button?.dataset?.step);
      if (step !== -1 && step !== 1) return true;
      if (scope === 'directory' && directoryVisible(C, r)) {
        const s = directory(C, r);
        s.page = Math.max(0, s.page + step); renderFiltered(u, s, scope);
      } else if (readerVisible(C, r) && reader(r).tab === scope) {
        const s = reader(r);
        s.pages[scope] = Math.max(0, (s.pages[scope] || 0) + step); renderFiltered(u, s, scope);
      }
      return true;
    }
    return false;
  }
  window.GalaxyUI = { init, renderDirectory, renderReader, handleInput, handleChange, submitForm, handleClick,
    isOverview: r => reader(r).tab === 'overview' };
})();
