/* Offline reference library. The Go catalogue owns entities, grouping and search. */
(() => {
  'use strict';
  const categories = [
    ['articles', 'Статьи', 'book-open'], ['ships', 'Корабли', 'ship'],
    ['engineers', 'Инженеры', 'users'], ['commodities', 'Товары', 'package'],
    ['modules', 'Модули', 'wrench'], ['materials', 'Материалы', 'layers'],
    ['blueprints', 'Чертежи', 'target'], ['experimentals', 'Эффекты', 'compass'], ['systems', 'Системы', 'compass'],
  ];
  const modeCategory = { 'Статьи': 'articles', 'Корабли': 'ships', 'Инженеры': 'engineers', 'Товары': 'commodities', 'Модули': 'modules', 'Материалы': 'materials', 'Чертежи': 'blueprints', 'Эффекты': 'experimentals' };
  const bindings = new WeakSet();
  const active = C => C.tab === 'encyclopedia' || (['engineering', 'targets'].includes(C.tab) && C.mode === 'Инженеры');
  const scope = C => ['engineering', 'targets'].includes(C.tab) ? 'engineers' : 'library';
  const plain = value => String(value ?? '').replace(/\bhttps?:\/\/[^\s<>"']+/gi, '').trim();
  const text = (ui, value) => ui.esc(plain(value));
  const valueText = (value, ui) => value === null || value === undefined || value === '' ? '—' : typeof value === 'boolean' ? (value ? 'Да' : 'Нет') : typeof value === 'number' ? ui.fmt(value) : Array.isArray(value) ? value.map(item => plain(item)).filter(Boolean).join(', ') : typeof value === 'object' ? '—' : plain(value);
  const listKey = r => JSON.stringify([r.category, r.query.trim(), r.group, r.offset, r.limit]);
  const validID = id => typeof id === 'string' && id.length > 0 && id.length <= 512;

  function state(C) {
    const key = scope(C);
    const sessions = C.referenceSessions || (C.referenceSessions = {});
    if (!sessions[key]) sessions[key] = {
      scope: key, category: key === 'engineers' ? 'engineers' : modeCategory[C.mode] || 'articles',
      query: '', group: '', offset: 0, limit: 24, list: null, loadedKey: '', requestKey: '',
      listToken: 0, loading: false, error: '', listPromise: null, searchTimer: null,
      entryId: '', entry: null, entryToken: 0, entryLoading: false, entryError: '', entryPromise: null,
      history: [], listScroll: 0, restoreScroll: null, personal: {}, market: null,
    };
    C.reference = sessions[key];
    return sessions[key];
  }

  function categoryTitle(r, id) {
    return (r.list?.categories || []).find(item => item.id === id)?.title || categories.find(item => item[0] === id)?.[1] || (id === 'all' ? 'Все разделы' : 'Справочник');
  }
  function categoryIcon(id) { return categories.find(item => item[0] === id)?.[2] || 'compass'; }
  function groupTitle(r, entry) { return entry.groupTitle || r.list?.groups?.find(group => group.id === entry.group)?.title || categoryTitle(r, entry.category || r.category); }
  function personalItem(r, entry, collection) {
    const key = String(entry.engineerKey || entry.materialKey || entry.key || String(entry.id || '').split('/').slice(1).join('/'));
    return (r.personal?.[collection] || []).find(item => String(item.key) === key);
  }
  function engineerStatus(row) {
    return ({ unlocked: 'Доступен', locked: 'Не открыт', invited: 'Есть приглашение', known: 'Есть сведения', unknown: 'Нет сведений' })[String(row?.status || '').toLowerCase()] || 'Нет сведений';
  }
  function personalBanner(r, entry, ui) {
    if (entry.category === 'engineers') {
      const row = entry.personalStatus || personalItem(r, entry, 'engineers'), known = row && row.status !== 'unknown';
      return `<section class="ref-personal-banner ${row?.status === 'unlocked' ? 'is-unlocked' : ''}">${ui.icon(row?.status === 'unlocked' ? 'circle-check' : 'circle')}<div><strong>${text(ui, known ? engineerStatus(row) : 'Открытие пока не подтверждено')}</strong><p>${text(ui, known ? 'Ваш статус у инженера' + (row.rank === null || row.rank === undefined ? '' : ' · ранг ' + valueText(row.rank, ui)) : 'Условия знакомства приведены ниже. Личный статус появится после получения данных игры.')}</p></div></section>`;
    }
    if (entry.category === 'materials') {
      const row = personalItem(r, entry, 'materials'), known = row?.owned !== null && row?.owned !== undefined;
      return `<section class="ref-personal-banner">${ui.icon('layers')}<div><strong>${known ? 'В вашем запасе: ' + text(ui, ui.fmt(row.owned)) : 'Личный запас пока неизвестен'}</strong><p>${known ? 'Последнее известное количество материала.' : 'Неизвестный запас не считается нулевым.'}</p></div></section>`;
    }
    return '';
  }
  function imageSource(image) {
    const src = typeof image === 'string' ? image : image?.src;
    return typeof src === 'string' && /^data:image\/(?:png|jpeg|webp|gif);base64,[a-zA-Z0-9+/=\s]+$/.test(src) ? src : '';
  }
  function imageHTML(entry, ui, hero = false) {
    const src = imageSource(entry.image), fit = entry.category === 'ships' ? 'contain' : entry.image?.fit === 'cover' || entry.category === 'engineers' ? 'cover' : 'contain';
    return `<div class="ref-art ${hero ? 'ref-art-hero' : ''} ref-art-${ui.esc(entry.category || 'entity')} ${src ? 'has-image' : 'no-image'}"><span class="ref-image-placeholder" ${src ? 'hidden' : ''}>${ui.icon(categoryIcon(entry.category))}</span>${src ? `<img data-reference-image src="${ui.esc(src)}" alt="${text(ui, entry.image?.alt || entry.title)}" class="ref-image-${fit}" loading="${hero ? 'eager' : 'lazy'}" decoding="async">` : ''}</div>`;
  }
  function internalLink(item, ui, className = '') {
    const id = item.entryId || item.id, label = item.title || item.name || item.label || 'Открыть';
    return validID(id) ? `<button type="button" class="ref-inline-link ${className}" data-click="reference-open" data-id="${ui.esc(id)}">${text(ui, label)}${ui.icon('arrow-up-right')}</button>` : text(ui, label);
  }
  function facts(items, ui, className = '') {
    return `<dl class="ref-properties ${className}">${(items || []).filter(item => item?.label || item?.title).map(item => `<div><dt>${text(ui, item.label || item.title)}</dt><dd>${item.entryId ? internalLink({ ...item, title: valueText(item.value ?? item.text, ui) }, ui) : text(ui, valueText(item.value ?? item.text, ui))}</dd></div>`).join('')}</dl>`;
  }
  function card(entry, r, ui) {
    const visual = entry.category === 'ships' || entry.category === 'engineers';
    const meta = (entry.meta || entry.properties || []).slice(0, 3), status = entry.personalStatus || personalItem(r, entry, 'engineers');
    return `<button type="button" class="ref-card ${visual ? 'ref-visual-card' : 'ref-text-card'} ref-card-${ui.esc(entry.category || r.category)}" data-click="reference-open" data-id="${ui.esc(entry.id)}">${visual ? imageHTML(entry, ui) : `<span class="ref-card-symbol">${ui.icon(categoryIcon(entry.category || r.category))}</span>`}<span class="ref-card-copy"><span class="ref-card-kicker">${text(ui, groupTitle(r, entry))}</span><strong>${text(ui, entry.title)}</strong>${entry.subtitle ? `<span class="ref-card-subtitle">${text(ui, entry.subtitle)}</span>` : ''}${entry.summary || entry.description ? `<span class="ref-card-summary">${text(ui, entry.summary || entry.description)}</span>` : ''}${meta.length ? `<span class="ref-card-meta">${meta.map(item => `<span><small>${text(ui, item.label)}</small>${text(ui, valueText(item.value, ui))}</span>`).join('')}</span>` : ''}${entry.badges?.length || (entry.category === 'engineers' && status && status.status !== 'unknown') ? `<span class="ref-card-badges">${entry.category === 'engineers' && status && status.status !== 'unknown' ? `<span class="ref-tag ${status.status === 'unlocked' ? 'is-unlocked' : ''}">${text(ui, engineerStatus(status))}</span>` : ''}${(entry.badges || []).slice(0, 3).map(item => `<span class="ref-tag">${text(ui, typeof item === 'string' ? item : item.label)}</span>`).join('')}</span>` : ''}<span class="ref-card-open">Открыть страницу ${ui.icon('arrow-right')}</span></span></button>`;
  }
  function categoryNavigation(r, ui) {
    if (r.scope === 'engineers') return '';
    const catalogueCategories = r.list?.categories?.length ? r.list.categories : categories.map(([id, title]) => ({ id, title }));
    const available = catalogueCategories.some(item => item.id === 'all') ? catalogueCategories : [{ id: 'all', title: 'Всё' }, ...catalogueCategories];
    return `<nav class="ref-category-nav" aria-label="Разделы энциклопедии">${available.map(item => `<button type="button" data-click="reference-category" data-id="${ui.esc(item.id)}" class="${r.category === item.id ? 'selected' : ''}" aria-pressed="${r.category === item.id}">${ui.icon(categoryIcon(item.id))}<span>${text(ui, item.title)}</span>${Number.isFinite(Number(item.count)) ? `<small>${text(ui, ui.fmt(item.count))}</small>` : ''}</button>`).join('')}</nav>`;
  }
  function skeleton(ui, reader = false) {
    if (reader) return `<div class="ref-reader-loading" role="status"><div class="ref-skeleton ref-skeleton-title"></div><div class="ref-skeleton ref-skeleton-line"></div><div class="ref-skeleton ref-skeleton-picture"></div><div class="ref-skeleton ref-skeleton-line"></div><div class="ref-skeleton ref-skeleton-line short"></div><span class="muted">Открываем страницу…</span></div>`;
    return `<div class="ref-grid ref-loading-grid" role="status" aria-label="Загрузка справочника">${Array.from({ length: 6 }, () => '<div class="ref-skeleton-card"><div class="ref-skeleton ref-skeleton-picture"></div><div class="ref-skeleton ref-skeleton-line"></div><div class="ref-skeleton ref-skeleton-line short"></div></div>').join('')}</div>`;
  }
  function errorPanel(message, ui, entry = false) {
    return `<section class="ref-message ref-error" role="alert">${ui.icon('circle-help')}<div><h2>${entry ? 'Не удалось открыть страницу' : 'Не удалось загрузить справочник'}</h2><p>${text(ui, message || 'Попробуйте ещё раз.')}</p>${ui.btn('Повторить', entry ? 'reference-retry-entry' : 'reference-retry', 'refresh-cw', 'primary')}</div></section>`;
  }
  function catalogue(r, ui) {
    const list = r.list || {}, entries = list.entries || [], total = Math.max(0, Number(list.total) || 0);
    const title = r.scope === 'engineers' ? 'Инженеры' : 'Энциклопедия';
    const intro = r.scope === 'engineers' ? 'Условия знакомства, доступные улучшения и всё необходимое для следующего визита.' : 'Знакомьтесь с кораблями, находите нужные материалы и разбирайтесь в устройстве галактики.';
    const groups = [...(list.groups || [])];
    if (r.group && !groups.some(group => group.id === r.group)) groups.unshift({ id: r.group, title: r.groupName || 'Выбранная группа' });
    const toolbar = `<div class="ref-catalog-toolbar"><label class="ref-search" for="reference-query">${ui.icon('search')}<input id="reference-query" type="search" placeholder="${r.scope === 'engineers' ? 'Найти инженера, систему или улучшение' : 'Поиск в выбранном разделе'}" value="${ui.esc(r.query)}" autocomplete="off"><span class="ref-search-key">${text(ui, categoryTitle(r, r.category))}</span></label>${groups.length ? `<label class="ref-group-label"><span>Группа</span><select id="reference-group" aria-label="Группа справочника"><option value="">Все группы</option>${groups.filter(group => group.id).map(group => `<option value="${ui.esc(group.id)}" ${r.group === group.id ? 'selected' : ''}>${text(ui, group.title)}${group.count === undefined ? '' : ' · ' + text(ui, ui.fmt(group.count))}</option>`).join('')}</select></label>` : ''}</div>`;
    let body;
    if (r.error) body = errorPanel(r.error, ui);
    else if (r.loading && !r.list) body = skeleton(ui);
    else if (!entries.length) body = `<section class="ref-message ref-empty">${ui.icon('search')}<div><h2>${r.query || r.group ? 'Ничего не найдено' : 'Здесь пока нет записей'}</h2><p>${r.query || r.group ? 'Попробуйте другой запрос или вернитесь ко всему разделу.' : 'Выберите соседний раздел или обновите справочник.'}</p>${ui.btn(r.query || r.group ? 'Сбросить поиск и фильтры' : 'Обновить', r.query || r.group ? 'reference-clear' : 'reference-retry', r.query || r.group ? 'x' : 'refresh-cw', 'quiet')}</div></section>`;
    else body = `<div class="ref-results-heading"><h2>${text(ui, categoryTitle(r, r.category))}</h2><span>${r.loading ? '<span class="ref-inline-loading">Обновляем список…</span>' : text(ui, ui.fmt(total) + ' записей')}</span></div><div class="ref-grid ref-grid-${ui.esc(r.category)} ${r.loading ? 'is-updating' : ''}" aria-busy="${r.loading}">${entries.map(entry => card(entry, r, ui)).join('')}</div>${total > r.limit ? `<nav class="ref-pagination" aria-label="Страницы справочника">${ui.btn('Назад', 'reference-prev', 'arrow-left', 'quiet', `type="button" ${r.offset <= 0 || r.loading ? 'disabled' : ''}`)}<span>${text(ui, ui.fmt(r.offset + 1))}–${text(ui, ui.fmt(Math.min(r.offset + entries.length, total)))} <span class="muted">из ${text(ui, ui.fmt(total))}</span></span>${ui.btn('Далее', 'reference-next', 'arrow-right', 'quiet', `type="button" ${r.offset + r.limit >= total || r.loading ? 'disabled' : ''}`)}</nav>` : ''}`;
    return `<section class="reference-root ref-library" data-reference="catalog"><header class="ref-library-heading"><div class="eyebrow">${r.scope === 'engineers' ? 'Мастерские галактики' : 'Бортовая библиотека'}</div><h1>${title}</h1><p>${intro}</p></header>${r.scope === 'engineers' && ui.renderModes ? ui.renderModes(true) : ''}${categoryNavigation(r, ui)}${toolbar}<div class="ref-results" aria-live="polite">${body}</div></section>`;
  }

  function sectionIsVisible(section) {
    return !/^(?:sources?|references?|links?)$/i.test(section.type || '') && !/^(?:sources?|references?|links?)$/i.test(section.id || '') && !/(?:источники|ссылки на источники|внешние ссылки)/i.test(section.title || '');
  }
  function sectionItems(section, ui, requirements = false) {
    return `<ul class="${requirements ? 'ref-requirements' : 'ref-notes'}">${(section.items || []).map(raw => {
      const item = typeof raw === 'string' ? { text: raw } : raw;
      const done = item.fulfilled === true || ['complete', 'unlocked', 'confirmed'].includes(item.status);
      return `<li>${requirements ? `<span class="ref-requirement-icon ${done ? 'is-complete' : ''}">${ui.icon(done ? 'circle-check' : 'circle')}</span>` : ''}<div>${item.title ? `<strong>${item.entryId ? internalLink(item, ui) : text(ui, item.title)}</strong>` : ''}${item.text || item.description ? `<p>${text(ui, item.text || item.description)}</p>` : ''}</div></li>`;
    }).join('')}</ul>`;
  }
  function table(section, ui) {
    const columns = section.columns || [], rows = section.rows || [];
    return `<div class="ref-table-wrap"><table class="ref-table"><thead><tr>${columns.map(column => `<th scope="col">${text(ui, typeof column === 'object' ? column.label || column.title : column)}</th>`).join('')}</tr></thead><tbody>${rows.map(row => `<tr>${(Array.isArray(row) ? row : columns.map(column => row[column.key])).map(cell => `<td>${cell && typeof cell === 'object' && cell.entryId ? internalLink(cell, ui) : text(ui, valueText(cell, ui))}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`;
  }
  function grades(section, ui, r) {
    return `<div class="ref-grade-list">${(section.grades || []).map((grade, index) => `<details class="ref-grade" ${index === 0 ? 'open' : ''}><summary><span class="ref-grade-number">${Number(grade.grade) > 0 ? text(ui, grade.grade) : ui.icon('wrench')}</span><span><strong>${text(ui, grade.title || (Number(grade.grade) > 0 ? 'Улучшение уровня ' + grade.grade : 'Установка эффекта'))}</strong><small>${text(ui, (grade.materials || []).map(item => (item.name || item.title) + ' × ' + valueText(item.count, ui)).join(' · '))}</small></span>${ui.icon('chevron-down')}</summary><div class="ref-grade-body">${grade.effects?.length ? facts(grade.effects, ui, 'ref-grade-effects') : ''}<h3>Потребуется на одну попытку</h3><div class="ref-materials">${(grade.materials || []).map(material => {
      const stock = personalItem(r, { id: material.entryId, key: material.key }, 'materials'), known = stock?.owned !== undefined && stock?.owned !== null;
      return `<div class="ref-material ${known && Number(stock.owned) >= Number(material.count) ? 'is-enough' : ''}">${ui.icon('layers')}<span>${material.entryId ? internalLink({ ...material, title: material.name || material.title }, ui) : text(ui, material.name || material.title)}${known ? `<small>Есть ${text(ui, ui.fmt(stock.owned))}</small>` : ''}</span><strong>× ${text(ui, valueText(material.count, ui))}</strong></div>`;
    }).join('')}</div></div></details>`).join('')}</div>`;
  }
  function renderSection(section, index, ui, r) {
    const id = 'reference-section-' + index;
    let body = (section.paragraphs || []).map(paragraph => `<p>${text(ui, paragraph)}</p>`).join('');
    if (section.type === 'facts') body += facts(section.properties || section.items || [], ui);
    else if (section.type === 'table') body += table(section, ui);
    else if (section.type === 'grades') body += grades(section, ui, r);
    else if (section.items?.length) body += sectionItems(section, ui, section.type === 'requirements');
    return `<section class="ref-entry-section" id="${id}" data-reference-section="${index}">${section.title ? `<h2>${text(ui, section.title)}</h2>` : ''}${body}</section>`;
  }
  function commodityKey(entry) { return entry.commodityKey || entry.actions?.find(action => action.type === 'trade')?.commodityKey || ''; }
  function currentMarket(r) {
    const market = r.market, options = market?.result?.options || {};
    return !!market && options.resourceKey === market.key && (options.mode || 'sell') === market.mode;
  }
  function marketRows(r) {
    if (!currentMarket(r)) return [];
    return (r.market.result.results || []).flatMap(row => {
      const quote = (row.quotes || []).find(item => item.key === r.market.key);
      return quote ? [{ ...row, quote }] : [];
    });
  }
  function marketDate(value) {
    const raw = String(value || ''), match = raw.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/);
    return match ? `${match[3]}.${match[2]}.${match[1]} ${match[4]}:${match[5]} UTC` : 'Время неизвестно';
  }
  function trading(r, entry, ui) {
    const key = entry.commodityKey || entry.actions?.find(action => action.type === 'trade')?.commodityKey;
    if (!key || entry.category !== 'commodities') return '';
    const market = r.market || { key, mode: 'sell', system: '', quantity: '1', radius: '100' }, buying = market.mode === 'buy';
    const rows = marketRows(r), running = market.searching || (currentMarket(r) && market.result?.running), status = market.error || (currentMarket(r) ? market.result?.error || market.result?.status : '');
    const tableHTML = rows.length ? `<div class="ref-table-wrap"><table class="ref-table ref-market-table"><thead><tr><th>Станция и система</th><th>Цена, кр./т</th><th>${buying ? 'Запас, т' : 'Спрос, т'}</th><th>От вас, св. л.</th><th>Наблюдение</th></tr></thead><tbody>${rows.map(row => `<tr><td><strong>${text(ui, row.station || 'Рынок')}</strong><small>${text(ui, row.system)}</small></td><td>${text(ui, valueText(row.quote.price, ui))}</td><td>${text(ui, valueText(row.quote[buying ? 'supply' : 'demand'], ui))}</td><td>${text(ui, valueText(row.distanceLy, ui))}</td><td>${row.expired ? '<span class="ref-market-stale">Устарела</span>' : ''}${text(ui, marketDate(row.updatedAt || row.quote.updatedAt))}</td></tr>`).join('')}</tbody></table></div>` : `<div class="ref-market-empty">${ui.icon('compass')}<p>${running ? 'Ищем подходящие рынки. Справочник и остальные разделы остаются доступны.' : currentMarket(r) && market.result ? 'Подходящие предложения не найдены. Измените область поиска или откройте все фильтры.' : 'Укажите исходную систему и запросите предложения для этого товара.'}</p></div>`;
    return `<section class="ref-entry-section ref-trade-section"><div class="ref-section-title"><div><span class="eyebrow">Перейти к делу</span><h2>Покупка и продажа</h2></div>${ui.icon('arrow-left-right')}</div><p>Сравните рынки по цене, доступному объёму и расстоянию. Дата наблюдения помогает оценить свежесть предложения.</p><div class="ref-market-modes segmented" aria-label="Операция с товаром">${[['buy', 'Купить'], ['sell', 'Продать']].map(([id, label]) => `<button type="button" data-click="reference-market-mode" data-id="${id}" class="${market.mode === id ? 'selected' : ''}" aria-pressed="${market.mode === id}">${label}</button>`).join('')}</div><div class="ref-market-controls"><label><span>Исходная система</span><input id="reference-market-system" value="${ui.esc(market.system)}" placeholder="Например, Sol" autocomplete="off"></label><label><span>Количество, т</span><input id="reference-market-quantity" value="${ui.esc(market.quantity)}" inputmode="numeric"></label><label><span>Радиус, св. л.</span><input id="reference-market-radius" value="${ui.esc(market.radius)}" inputmode="decimal"></label>${ui.btn(running ? 'Поиск…' : (buying ? 'Найти продавцов' : 'Найти покупателей'), 'reference-find-markets', 'search', 'primary', `type="button" ${running ? 'disabled' : ''}`)}</div>${status ? `<p class="ref-market-status ${market.error || (currentMarket(r) && market.result?.error) ? 'is-error' : ''}" role="status">${text(ui, status)}</p>` : ''}${tableHTML}<div class="ref-market-footer"><span>${rows.length ? text(ui, ui.fmt(rows.length) + ' предложений') : 'Цены меняются по мере торговли в игре'}</span>${ui.btn('Все фильтры в торговле', buying ? 'reference-trade-buy' : 'reference-trade-sell', 'arrow-right', 'quiet', `type="button" data-key="${ui.esc(key)}"`)}</div></section>`;
  }
  function entity(r, ui) {
    const actions = (r.entry?.actions || []).map((item, index) => item.type === 'copy-system' ? ui.btn(plain(item.label || 'Копировать систему'), 'reference-copy-system', 'copy', 'quiet', `type="button" data-index="${index}"`) : '').join('');
    const back = `<div class="ref-entry-nav">${ui.btn(r.history.length ? 'Назад' : (r.scope === 'engineers' ? 'Все инженеры' : 'К списку'), 'reference-back', 'arrow-left', 'quiet', 'type="button"')}${r.history.length ? ui.btn(r.scope === 'engineers' ? 'Все инженеры' : 'Каталог', 'reference-library', 'layout-grid', 'quiet', 'type="button"') : ''}<span class="spacer"></span>${actions}<span class="ref-entry-context">${text(ui, categoryTitle(r, r.entry?.category || r.category))}</span></div>`;
    if (r.entryError) return `<section class="reference-root ref-reader">${back}${errorPanel(r.entryError, ui, true)}</section>`;
    if (r.entryLoading || !r.entry) return `<section class="reference-root ref-reader" aria-busy="true">${back}${skeleton(ui, true)}</section>`;
    const entry = r.entry, article = entry.category === 'articles';
    const sections = (entry.sections || []).filter(sectionIsVisible);
    const hero = `<header class="ref-entry-hero ${article ? 'ref-article-hero' : ''} ${imageSource(entry.image) ? 'with-image' : ''}"><div class="ref-hero-copy"><div class="eyebrow">${text(ui, entry.groupTitle || categoryTitle(r, entry.category))}</div><h1>${text(ui, entry.title)}</h1>${entry.subtitle ? `<div class="ref-entry-subtitle">${text(ui, entry.subtitle)}</div>` : ''}${entry.description || entry.summary ? `<p class="ref-entry-lead">${text(ui, entry.description || entry.summary)}</p>` : ''}${entry.badges?.length ? `<div class="ref-entry-badges">${entry.badges.map(item => `<span class="ref-tag">${text(ui, typeof item === 'string' ? item : item.label)}</span>`).join('')}</div>` : ''}</div>${imageSource(entry.image) ? imageHTML(entry, ui, true) : ''}</header>`;
    const related = (entry.related || []).filter(item => validID(item.id || item.entryId));
    return `<section class="reference-root ref-reader ${article ? 'ref-article-reader' : ''}" data-reference="entry" data-entry-id="${ui.esc(entry.id)}">${back}${hero}${entry.properties?.length ? facts(entry.properties, ui, 'ref-hero-properties') : ''}${personalBanner(r, entry, ui)}<div class="ref-reader-layout"><article class="ref-entry-content">${sections.map((section, index) => renderSection(section, index, ui, r)).join('')}${trading(r, entry, ui)}${related.length ? `<section class="ref-entry-section"><h2>Связанные записи</h2><div class="ref-related-grid">${related.map(item => `<button type="button" data-click="reference-open" data-id="${ui.esc(item.id || item.entryId)}" class="ref-related">${ui.icon(categoryIcon(item.category))}<span><strong>${text(ui, item.title)}</strong>${item.subtitle ? `<small>${text(ui, item.subtitle)}</small>` : ''}</span>${ui.icon('arrow-up-right')}</button>`).join('')}</div></section>` : ''}</article>${sections.filter(section => section.title).length > 2 ? `<aside class="ref-entry-toc" aria-label="На этой странице"><strong>На этой странице</strong>${sections.map((section, index) => section.title ? `<button type="button" data-click="reference-section" data-id="${index}">${text(ui, section.title)}</button>` : '').join('')}</aside>` : ''}</div></section>`;
  }

  function render(C, ui) { const r = state(C); return r.entryId ? entity(r, ui) : catalogue(r, ui); }
  function redraw(C, ui, r) {
    if (!active(C) || state(C) !== r) return;
    const input = ui.$('#reference-query'), focused = input?.matches?.(':focus');
    const selection = focused ? [input.selectionStart, input.selectionEnd] : null;
    ui.renderPage();
    if (focused) {
      const next = ui.$('#reference-query');
      next?.focus?.({ preventScroll: true });
      if (selection && selection[0] !== null) next?.setSelectionRange?.(...selection);
    }
  }
  async function loadList(C, ui, force = false) {
    const r = state(C), key = listKey(r);
    if (!force && r.requestKey === key && r.listPromise) return r.listPromise.catch(() => null);
    if (!force && r.loadedKey === key && !r.error) return r.list;
    const token = ++r.listToken;
    r.requestKey = key; r.loading = true; r.error = '';
    const params = { category: r.category, query: r.query.trim(), group: r.group, offset: r.offset, limit: r.limit };
    const request = Promise.resolve().then(() => ui.call('reference.list', params));
    r.listPromise = request;
    redraw(C, ui, r);
    try {
      const result = await request;
      if (token !== r.listToken || key !== listKey(r)) return null;
      r.list = result || { entries: [], total: 0 }; r.loadedKey = key; r.loading = false; r.listPromise = null;
      if (result?.personal) r.personal = result.personal;
      if (!r.entryId) redraw(C, ui, r);
      return result;
    } catch (error) {
      if (token !== r.listToken || key !== listKey(r)) return null;
      r.loading = false; r.listPromise = null; r.error = error?.message || 'Не удалось прочитать справочник.';
      if (!r.entryId) redraw(C, ui, r);
      return null;
    }
  }
  async function loadEntry(C, ui) {
    const r = state(C), id = r.entryId;
    if (!validID(id)) return null;
    const token = ++r.entryToken;
    r.entryLoading = true; r.entryError = ''; r.entry = null;
    const request = Promise.resolve().then(() => ui.call('reference.entry', { id }));
    r.entryPromise = request; redraw(C, ui, r);
    try {
      const result = await request;
      if (token !== r.entryToken || id !== r.entryId) return null;
      const entry = result?.entry || result;
      if (!entry || !validID(entry.id)) throw new Error('Запись не найдена. Вернитесь к списку и выберите другую.');
      r.entry = entry; r.personal = result?.personal || {}; r.entryLoading = false; r.entryPromise = null;
      if (entry.category === 'commodities' && commodityKey(entry)) {
        const key = commodityKey(entry);
        if (r.market?.key !== key) r.market = { key, mode: 'sell', system: C.state?.commander?.system || '', quantity: '1', radius: '100', result: null, error: '', token: 0, searching: false, pollTimer: null, polls: 0 };
        refreshMarkets(C, ui, r);
      }
      redraw(C, ui, r);
      return entry;
    } catch (error) {
      if (token !== r.entryToken || id !== r.entryId) return null;
      r.entryLoading = false; r.entryPromise = null; r.entryError = error?.message || 'Не удалось прочитать запись.';
      redraw(C, ui, r); return null;
    }
  }
  async function open(C, ui, id, push = true) {
    if (!validID(id)) return false;
    const r = state(C), host = ui.$('#page-scroll');
    stopMarkets(r);
    if (push && r.entryId && r.entryId !== id) r.history.push({ id: r.entryId, scroll: host?.scrollTop || 0 });
    else if (!r.entryId) r.listScroll = host?.scrollTop || 0;
    r.entryId = id; r.restoreScroll = 0;
    await loadEntry(C, ui); return true;
  }
  function returnToLibrary(C, ui) {
    const r = state(C); stopMarkets(r); r.entryToken++; r.entryId = ''; r.entry = null; r.entryError = ''; r.entryLoading = false; r.entryPromise = null; r.history = [];
    r.restoreScroll = r.listScroll; redraw(C, ui, r); return loadList(C, ui);
  }
  async function setCategory(C, ui, category) {
    if (typeof category !== 'string' || !category || category.length > 64) return false;
    const r = state(C);
    if (r.scope === 'engineers' && category !== 'engineers') return false;
    stopMarkets(r);
    clearTimeout(r.searchTimer); r.listToken++; r.category = category; r.query = ''; r.group = ''; r.offset = 0; r.entryId = ''; r.entryToken++; r.entry = null; r.history = [];
    r.restoreScroll = 0; await loadList(C, ui); return true;
  }
  async function handleClick(action, button, C, ui) {
    if (!action?.startsWith('reference-')) return false;
    const r = state(C), id = button?.dataset?.id;
    if (action === 'reference-open') return open(C, ui, id);
    if (action === 'reference-category') return setCategory(C, ui, id);
    if (action === 'reference-library') { await returnToLibrary(C, ui); return true; }
    if (action === 'reference-back') {
      if (r.history.length) { const previous = r.history.pop(); await open(C, ui, previous.id, false); r.restoreScroll = previous.scroll; redraw(C, ui, r); }
      else await returnToLibrary(C, ui);
      return true;
    }
    if (action === 'reference-retry') { await loadList(C, ui, true); return true; }
    if (action === 'reference-retry-entry') { await loadEntry(C, ui); return true; }
    if (action === 'reference-clear') { clearTimeout(r.searchTimer); r.query = ''; r.group = ''; r.offset = 0; await loadList(C, ui, true); return true; }
    if (action === 'reference-prev' || action === 'reference-next') {
      if (r.loading) return true;
      const next = r.offset + (action === 'reference-next' ? r.limit : -r.limit), total = Number(r.list?.total) || 0;
      if (next >= 0 && next < total) { r.offset = next; r.restoreScroll = 0; await loadList(C, ui); }
      return true;
    }
    if (action === 'reference-section') { if (/^\d+$/.test(id || '')) ui.$('#reference-section-' + id)?.scrollIntoView?.({ block: 'start', behavior: 'smooth' }); return true; }
    if (action === 'reference-market-mode') {
      if (r.market && ['buy', 'sell'].includes(id)) { stopMarkets(r); r.market.mode = id; r.market.error = ''; r.market.searching = false; redraw(C, ui, r); await refreshMarkets(C, ui, r); }
      return true;
    }
    if (action === 'reference-find-markets') { await findMarkets(C, ui, r); return true; }
    if (action === 'reference-copy-system') {
      const item = r.entry?.actions?.[Number(button?.dataset?.index)];
      if (item?.type === 'copy-system' && typeof item.system === 'string') { await ui.call('native', { action: 'copy', text: item.system }); ui.toast('Система скопирована'); }
      return true;
    }
    if (action === 'reference-trade-buy' || action === 'reference-trade-sell') {
      const key = button?.dataset?.key;
      if (typeof key === 'string' && key.length > 0 && key.length <= 128) await ui.navigate('sale', { mode: action === 'reference-trade-buy' ? 'Купить' : 'Продать', prefill: { resourceKey: key } });
      return true;
    }
    return false;
  }
  function handleInput(event, C, ui) {
    if (['reference-market-system', 'reference-market-quantity', 'reference-market-radius'].includes(event.target?.id)) {
      const r = state(C), key = { 'reference-market-system': 'system', 'reference-market-quantity': 'quantity', 'reference-market-radius': 'radius' }[event.target.id];
      if (r.market) r.market[key] = String(event.target.value || '').slice(0, key === 'system' ? 128 : 24);
      return true;
    }
    if (event.target?.id !== 'reference-query') return false;
    const r = state(C); r.query = String(event.target.value || '').slice(0, 256); r.offset = 0; r.listToken++; clearTimeout(r.searchTimer);
    r.searchTimer = setTimeout(() => { if (active(C) && state(C) === r && !r.entryId) loadList(C, ui); }, 200);
    return true;
  }
  async function handleChange(event, C, ui) {
    if (event.target?.id !== 'reference-group') return false;
    const r = state(C); clearTimeout(r.searchTimer); r.group = String(event.target.value || '').slice(0, 128); r.groupName = r.list?.groups?.find(group => group.id === r.group)?.title || ''; r.offset = 0;
    await loadList(C, ui); return true;
  }
  function init(C, ui) {
    if (!active(C)) return null;
    const r = state(C), host = ui.$('#page-scroll');
    if (host && !bindings.has(host)) {
      bindings.add(host);
      host.addEventListener('error', event => {
        if (!event.target?.matches?.('[data-reference-image]')) return;
        event.target.hidden = true;
        const fallback = event.target.parentElement?.querySelector('.ref-image-placeholder');
        if (fallback) fallback.hidden = false;
      }, true);
    }
    if (r.restoreScroll !== null && host) { host.scrollTop = r.restoreScroll; r.restoreScroll = null; }
    if (r.entry?.category === 'commodities' && r.market && (r.market.needsRefresh || (r.market.searching && r.market.polls < 65)) && !r.market.starting && !r.market.pollTimer && !r.market.inFlightToken) refreshMarkets(C, ui, r);
    if (r.entryId) return r.entry || r.entryLoading || r.entryError ? r.entryPromise?.catch(() => null) : loadEntry(C, ui);
    if (r.error && r.requestKey === listKey(r)) return null;
    return loadList(C, ui);
  }
  function stopMarkets(r) { if (r.market) { r.market.token++; r.market.inFlightToken = 0; r.market.starting = false; r.market.needsRefresh = true; clearTimeout(r.market.pollTimer); r.market.pollTimer = null; } }
  async function refreshMarkets(C, ui, r) {
    if (!r.market || !r.entryId || r.market.inFlightToken) return;
    if (!active(C) || state(C) !== r || ui.$('#page-scroll')?.ownerDocument?.hidden) { r.market.needsRefresh = true; return; }
    const market = r.market, token = ++market.token, entryID = r.entryId;
    market.inFlightToken = token; market.needsRefresh = false;
    try {
      const result = await ui.call('reference.markets', {});
      if (token !== market.token || r.entryId !== entryID) return;
      market.inFlightToken = 0; market.result = result || {}; market.searching = currentMarket(r) && !!result?.running;
      if (market.searching && ++market.polls < 65 && active(C) && state(C) === r) market.pollTimer = setTimeout(() => { market.pollTimer = null; refreshMarkets(C, ui, r); }, 1200);
      if (active(C) && state(C) === r) redraw(C, ui, r);
    } catch (error) {
      if (token !== market.token || r.entryId !== entryID) return;
      market.inFlightToken = 0; market.searching = false; market.error = error?.message || 'Не удалось обновить рынки.'; redraw(C, ui, r);
    }
  }
  async function findMarkets(C, ui, r) {
    const market = r.market;
    if (!market || market.searching) return;
    const number = value => Number(String(value || '').trim().replace(/[\s\u00a0\u202f]/g, '').replace(',', '.'));
    const quantity = number(market.quantity), radius = number(market.radius), system = market.system.trim();
    if (!system) market.error = 'Укажите исходную систему для поиска.';
    else if (!Number.isInteger(quantity) || quantity < 1 || quantity > 25000) market.error = 'Укажите целое количество от 1 до 25 000 т.';
    else if (!Number.isFinite(radius) || radius < 5 || radius > 5000) market.error = 'Укажите радиус от 5 до 5 000 св. л.';
    else market.error = '';
    if (market.error) { redraw(C, ui, r); return; }
    stopMarkets(r); const entryID = r.entryId, token = market.token;
    market.searching = true; market.starting = true; market.polls = 0; redraw(C, ui, r);
    try {
      await ui.call('reference.market-search', { values: { mode: market.mode, resourceKey: market.key, system, quantity, radiusLy: radius, useCargo: false, includeCarriers: true, maxAgeHours: 24, carrierMaxAgeHours: 6, maxArrivalLs: 10000, minPad: 'L', minDemand: 1, minSupply: 1, demandRatio: 4 } });
      if (r.entryId !== entryID || market.token !== token) return;
      market.starting = false;
      await refreshMarkets(C, ui, r);
    } catch (error) { if (r.entryId !== entryID || market.token !== token) return; market.starting = false; market.searching = false; market.error = error?.message || 'Не удалось начать поиск рынков.'; redraw(C, ui, r); }
  }
  function navigate(id, opts, C) {
    for (const r of Object.values(C.referenceSessions || {})) { clearTimeout(r.searchTimer); stopMarkets(r); }
    if (id !== 'encyclopedia') return;
    const r = state(C), category = opts?.referenceCategory || modeCategory[opts?.mode];
    if (category && category !== r.category) { r.category = category; r.query = ''; r.group = ''; r.offset = 0; r.listToken++; r.entryToken++; r.entryId = ''; r.entry = null; r.history = []; }
  }
  // open(id,C,ui) is the public deep-link contract. Accept the original
  // C-first order as well for capture/smoke callers created during integration.
  const publicOpen = (first, second, third) => typeof first === 'string' ? open(second, third, first) : open(first, second, third);
  window.ReferenceUI = { active, render, init, handleClick, handleInput, handleChange, open: publicOpen, setCategory, navigate, isReading: C => !!state(C).entryId };
})();
