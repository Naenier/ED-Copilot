(function () {
  "use strict";
  const U = window.MiningUI;
  const { h, n, credits, dateTime, pill } = U;
  const D = window.CopilotCodexData;
  let context;
  const ui = { section: "", record: "", query: "", catalogFilter: "all", resultType: "all", limit: 48 };
  const types = { raw: "Сырьё", manufactured: "Промышленные материалы", encoded: "Данные" };
  const tabs = { sale: "Торговля", deals: "Рейсы", mining: "Заход добычи", targets: "Цели", route: "Маршрут флотоносца", carrier: "Флотоносец", engineering: "Инженерия", help: "Руководство" };
  const catalogs = [
    { id: "catalog-ships", key: "ships", name: "Корабли", text: "Модели, площадки, корпуса и отсеки.", icon: "△" },
    { id: "catalog-goods", key: "goods", name: "Товары", text: "Товарные названия и известные цены.", icon: "⇄" },
    { id: "catalog-resources", key: "resources", name: "Добываемые ресурсы", text: "Все прежние карточки добычи и сырья.", icon: "◇" },
    { id: "catalog-materials", key: "materials", name: "Инженерные материалы", text: "Типы, семейства, редкость и запасы.", icon: "▦" },
    { id: "catalog-modules", key: "modules", name: "Семейства модулей", text: "Совместимые чертежи и эффекты.", icon: "▤" },
    { id: "catalog-blueprints", key: "blueprints", name: "Чертежи", text: "Ингредиенты каждого применения G1–G5.", icon: "⚒" },
    { id: "catalog-experimentals", key: "experimentals", name: "Экспериментальные эффекты", text: "Стоимость и совместимость эффектов.", icon: "✧" },
    { id: "catalog-engineers", key: "engineers", name: "Инженеры", text: "Базы, возможности и известный доступ.", icon: "⌖" },
  ];
  const normalization = (value) => String(value || "").toLocaleLowerCase("ru-RU").replaceAll("ё", "е").replace(/[^\p{L}\p{N}]+/gu, " ").trim();
  const button = (text, attribute, value = "", style = "button-small button-subtle") => '<button type="button" class="button ' + style + '" ' + attribute + '="' + h(value) + '">' + h(text) + '</button>';
  const note = (text) => '<p class="panel-note">' + h(text) + '</p>';
  const articleById = (id) => D.articles.find((row) => row.id === id);
  const sectionById = (id) => D.sections.find((row) => row.id === id) || catalogs.find((row) => row.id === id);
  const sources = (rows) => '<div class="codex-sources">' + rows.filter(Boolean).map((row) => U.safeUrl(row.url) ? '<a href="' + h(row.url) + '" target="_blank" rel="noopener noreferrer">' + h(row.title || row.name || "Источник") + '</a>' : '').join('') + '</div>';
  const table = (headers, rows) => '<div class="table-wrap"><table><thead><tr>' + headers.map((title) => '<th>' + h(title) + '</th>').join('') + '</tr></thead><tbody>' + rows.map((row) => '<tr>' + row.map((value) => '<td>' + value + '</td>').join('') + '</tr>').join('') + '</tbody></table></div>';
  const field = (label, value) => '<div class="codex-fact"><span>' + h(label) + '</span><strong>' + value + '</strong></div>';
  const categoryName = (key) => sectionById(key)?.name || key;
  const shipAliases = { sidewinder: "сайдвиндер", eagle: "игл орел", hauler: "хаулер", adder: "аддер", viper: "вайпер", cobramkiii: "кобра", type6: "тип 6", dolphin: "дельфин", type7: "тип 7", asp: "асп эксплорер", vulture: "валчер стервятник", empiretrader: "клипер имперский", orca: "орка косатка", type9: "тип 9", python: "питон пайтон", belugaliner: "белуга", ferdelance: "фер де ланс", anaconda: "анаконда", federationcorvette: "корвет", cutter: "каттер куттер", diamondback: "даймондбэк скаут", diamondbackxl: "даймондбэк эксплорер", empirecourier: "курьер", empireeagle: "имперский орел", krai: "крайт", mandalay: "мандалай", corsair: "корсар", panthermkii: "пантера клиппер", lakonminer: "тип 11 проспектор", explorernx: "каспиан эксплорер", anacondamkii: "анаконда мк2" };

  function entities(state) {
    const engineering = state.engineering || {};
    const catalog = engineering.catalog || {};
    const stocks = new Map((engineering.materials || []).map((row) => [row.key, row]));
    const compatibleModules = (key, field) => (catalog.modules || []).filter((module) => (module[field] || []).includes(key)).map((module) => module.name);
    return {
      ships: (window.CopilotCodexShips || []).map((row) => ({ ...row, title: row.name, subtitle: [row.manufacturer, row.pad ? 'Площадка ' + row.pad : 'Размер не подтверждён'].filter(Boolean).join(' · '), search: [row.name, row.symbol, row.manufacturer, shipAliases[row.key], row.name.includes('Krait') ? 'крайт' : '', row.name.includes('Cobra') ? 'кобра' : '', row.name.includes('Viper') ? 'вайпер' : '', row.name.includes('Type-') ? 'тип' : ''].join(' ') })),
      goods: (state.goods || []).filter((row) => row.kind !== 'raw').map((row) => ({ ...row, title: row.name || row.english || row.key, subtitle: row.english || row.key, search: [row.name, row.english, row.key, ...(row.aliases || [])].join(' ') })),
      resources: (state.resources || []).map((row) => ({ ...row, title: row.name, subtitle: row.kind === 'raw' ? 'Инженерное сырьё · G' + row.grade : (row.methods || []).map((method) => U.methods[method] || method).join(' · '), search: [row.name, row.english, row.key, row.description, ...(row.aliases || [])].join(' ') })),
      materials: (catalog.materials || []).map((row) => ({ ...row, ...stocks.get(row.key), title: row.name, subtitle: (types[row.category] || row.category) + ' · G' + row.grade, search: [row.name, row.english, row.key, types[row.category]].join(' ') })),
      modules: (catalog.modules || []).map((row) => ({ ...row, title: row.name, subtitle: n(row.blueprints?.length || 0, true) + ' чертежей · ' + n(row.experimentals?.length || 0, true) + ' эффектов', search: row.name + ' ' + row.key })),
      blueprints: (catalog.blueprints || []).map((row) => { const compatibility = compatibleModules(row.key, 'blueprints'); return { ...row, title: row.name, compatibility, subtitle: compatibility.join(' / ') + ' · ' + Object.keys(row.grades || {}).map((grade) => 'G' + grade).join(' · '), search: [row.name, row.english, row.key, ...compatibility].join(' ') }; }),
      experimentals: (catalog.experimentals || []).map((row) => { const compatibility = compatibleModules(row.key, 'experimentals'); return { ...row, title: row.name, compatibility, subtitle: compatibility.join(' / ') + (row.english ? ' · ' + row.english : ''), search: [row.name, row.english, row.key, ...compatibility].join(' ') }; }),
      engineers: (engineering.engineers || catalog.engineers || []).map((row) => ({ ...row, title: row.name, subtitle: [row.english, row.system].filter(Boolean).join(' · '), search: [row.name, row.english, row.system, row.base].join(' ') })),
    };
  }
  function recordButton(row, section, compact = false) {
    const subtitle = row.summary || row.subtitle || '';
    return '<button class="codex-record' + (compact ? ' is-compact' : '') + '" type="button" data-codex-record="' + h(section + ':' + (section.startsWith('catalog-') ? row.key : row.id)) + '"><span><strong>' + h(row.title) + '</strong><small' + (row.compatibility ? ' class="codex-compatibility-subtitle" title="' + h(subtitle) + '"' : '') + '>' + h(subtitle) + '</small></span><i aria-hidden="true">›</i></button>';
  }
  function cards(rows, section) { return '<div class="codex-record-grid">' + rows.map((row) => recordButton(row, section)).join('') + '</div>'; }
  function home(groups) {
    return '<section class="codex-intro"><div><h2>Чем хотите заняться?</h2><p>Начните с игрового направления. Внутри — короткие объяснения, практические проверки и переходы к связанным темам.</p></div><span>' + n(D.articles.length, true) + ' тем · ' + n(D.sections.length, true) + ' разделов</span></section>' +
      '<div class="codex-section-grid">' + D.sections.map((section) => '<button class="codex-section-card" type="button" data-codex-section="' + section.id + '"><span class="codex-section-icon">' + section.icon + '</span><span><strong>' + h(section.name) + '</strong><small>' + h(section.description) + '</small><em>' + D.articles.filter((article) => article.section === section.id).length + ' тем →</em></span></button>').join('') + '</div>' +
      '<section class="codex-intro"><div><h2>Справочные каталоги</h2><p>Сначала выберите каталог, затем найдите объект. Карточка открывает свойства и связанные записи.</p></div></section>' +
      '<div class="codex-catalog-grid">' + catalogs.map((catalog) => '<button type="button" class="codex-catalog-card" data-codex-section="' + catalog.id + '"><span>' + catalog.icon + '</span><strong>' + h(catalog.name) + '</strong><b>' + n(groups[catalog.key]?.length || 0, true) + '</b><small>' + h(catalog.text) + '</small></button>').join('') + '</div>' +
      note('Игровая энциклопедия объясняет мир и механики. Пошаговая работа с ED Copilot находится в отдельном разделе «Руководство». Каталоги доступны без интернета; цены и личные запасы имеют собственные источники и даты.');
  }
  function sidebar() {
    return '<nav class="codex-sidebar" aria-label="Разделы энциклопедии">' + button('Все разделы', 'data-codex-section', '', 'button-subtle') +
      '<h3>Игровые направления</h3>' + D.sections.map((section) => '<button type="button" data-codex-section="' + section.id + '" class="codex-nav-link' + (ui.section === section.id ? ' is-selected' : '') + '">' + h(section.name) + '</button>').join('') +
      '<h3>Каталог</h3><select data-codex-catalog aria-label="Выбрать справочный каталог"><option value="">Выберите каталог</option>' + catalogs.map((catalog) => '<option value="' + catalog.id + '"' + (catalog.id === ui.section ? ' selected' : '') + '>' + h(catalog.name) + '</option>').join('') + '</select></nav>';
  }
  function articleDetail(article) {
    const action = article.action === 'targets' ? button('Открыть места добычи', 'data-codex-mining') : article.action && tabs[article.action] ? '<a class="button button-small button-subtle" href="#' + article.action + '">Открыть: ' + h(tabs[article.action]) + '</a>' : '';
    return '<article class="panel codex-article"><div class="codex-article-head"><p class="eyebrow">' + h(categoryName(article.section)) + '</p><h2>' + h(article.title) + '</h2><p>' + h(article.summary) + '</p>' + action + '</div><div class="codex-article-body">' +
      article.body.map((paragraph) => '<p>' + h(paragraph) + '</p>').join('') + (article.tips?.length ? '<h3>Что проверить на практике</h3><ul>' + article.tips.map((tip) => '<li>' + h(tip) + '</li>').join('') + '</ul>' : '') +
      (article.related?.length ? '<h3>Связанные темы</h3><div class="codex-related">' + article.related.map((id) => { const related = articleById(id); return related ? button(related.title, 'data-codex-article', id) : ''; }).join('') + '</div>' : '') +
      '<div class="codex-reference"><small>Проверка справочника: ' + h(article.verifiedAt) + '. Базовые понятия; изменяемые условия и выплаты сверяйте с текущей игрой.</small>' + sources(article.sourceIds.map((id) => D.sources[id])) + '</div></div></article>';
  }
  function materialRows(state, rows) {
    const lookup = new Map((state.engineering?.catalog?.materials || []).map((row) => [row.key, row]));
    return table(['Материал', 'Количество'], rows.map((row) => [button(lookup.get(row.key)?.name || row.name || row.key, 'data-codex-record', 'catalog-materials:' + row.key), n(row.count, true)]));
  }
  function entityDetail(state, catalog, entity, groups) {
    const c = state.engineering?.catalog || {};
    let body = '', related = '', references = [];
    if (catalog.key === 'ships') {
      body = '<p>Модель корабля из каталога EDCD. Название оставлено в форме, удобной для поиска в игре. Оснащение и инженерия изменяют характеристики вашей сборки.</p>' +
        (entity.specsKnown ? '<div class="codex-facts">' + field('Производитель', h(entity.manufacturer)) + field('Площадка', h(entity.pad)) + field('Масса корпуса', n(entity.hullMass) + ' т') + field('Мест экипажа', n(entity.crew, true)) + field('Вспомогательные точки', n(entity.utilitySlots, true)) + field('Обычные внутренние отсеки', h(entity.optionalSlots.join(' / ') || '—')) + '</div>' +
          '<h3>Оружейные точки</h3>' + (entity.hardpoints.length ? table(['Размер', 'Количество'], entity.hardpoints.map((row) => ['Класс ' + row.grade, n(row.count, true)])) : note('Оружейные точки не указаны.')) +
          note('Это справочные параметры корпуса и обычных отсеков, а не готовая сборка. Специализированные отсеки могут иметь ограничения. Дальность, груз, щиты, скорость и стоимость полного оснащения проверяйте в конфигураторе или игре.') : note('Идентификатор и название подтверждены EDCD, но подробные параметры этой модели ещё не входят в локальный снимок Coriolis. Неизвестные характеристики не подставляются.'));
      related = ['pad-size', 'core-modules', 'ship-budget']; references = [{ title: 'EDCD · идентификаторы кораблей', url: 'https://github.com/EDCD/FDevIDs/blob/master/shipyard.csv' }, { title: 'EDCD · параметры корпусов', url: entity.sourceUrl }];
    } else if (catalog.key === 'goods') {
      const cargo = state.commander?.cargo?.find((row) => row.key === entity.key);
      body = '<p>Товар для перевозки в грузовом трюме. Цена наблюдения не является гарантированным предложением покупателя в другой системе.</p><div class="codex-facts">' +
        field('На текущем транспорте', n(cargo?.count ?? 0, true) + ' т') + field('Известная справочная цена', credits(entity.price)) + field('Источник цены', h(entity.priceSource || 'Нет наблюдения')) + field('Время наблюдения', dateTime(entity.priceAt)) + '</div><a class="button button-small button-subtle" href="#sale">Найти покупателя или продавца</a>';
      const resource = groups.resources.find((row) => row.key === entity.key);
      if (resource) body += '<h3>Добыча этого товара</h3>' + recordButton(resource, 'catalog-resources', true);
      related = ['commodity-market', 'trade-profit', 'market-freshness']; references = [D.sources.ids, D.sources.journal];
    } else if (catalog.key === 'resources') {
      body = '<p>' + h(entity.description || '') + '</p><div class="pill-row">' + (entity.methods || []).map((method) => pill(U.methods[method] || method, 'blue')).join('') + '</div>';
      if (entity.availability?.length) body += '<h3>Где и каким способом</h3>' + table(['Способ', 'Условия'], entity.availability.map((row) => [h(U.methods[row.method] || row.method), h((row.ringTypes || []).map((type) => U.ringTypes[type] || type).join(' · ') || (row.method === 'surface-planet' ? 'Планетарные участки добычи' : 'Соответствующие залежи и источники'))]));
      if (entity.kind === 'raw') { const material = groups.materials.find((row) => row.key === entity.key); body += note('Инженерное сырьё хранится в отдельном запасе, не занимает товарный трюм и не продаётся как руда.') + (material ? recordButton(material, 'catalog-materials', true) : ''); }
      else body += '<div class="codex-facts">' + field('Справочная цена', credits(entity.price)) + field('Источник', h(entity.priceSource || 'Нет цены')) + field('Наблюдение', dateTime(entity.priceAt)) + '</div>';
      if (entity.tips?.length) body += '<h3>Подсказки</h3><ul>' + entity.tips.map((tip) => '<li>' + h(tip) + '</li>').join('') + '</ul>';
      body += button('Открыть места добычи в «Целях»', 'data-codex-mining');
      related = ['mining-methods', entity.kind === 'raw' ? 'raw-collection' : 'hotspots']; references = entity.sources || [D.sources.ids];
    } else if (catalog.key === 'materials') {
      body = '<p>Корабельный инженерный материал. Текущий запас определяется полным снимком соответствующей категории и последующими событиями журнала.</p><div class="codex-facts">' +
        field('Тип', h(types[entity.category])) + field('Редкость', 'G' + h(entity.grade)) + field('Семейство обмена', h(entity.familyName || (entity.family == null ? 'Не обменивается' : entity.family))) + field('Предел хранения', n(entity.capacity, true)) + field('Известный запас', n(entity.owned, true)) + field('Нужно для плана', n(entity.needed, true)) + field('Не хватает', n(entity.missing, true)) + '</div>' +
        note(entity.tradeable ? 'Материал можно обменивать внутри соответствующего типа. Проверьте курс, целые пакеты, резерв плана и свободное место получаемого материала.' : 'Этот материал не относится к обычным обменным семействам торговцев.') + '<a class="button button-small button-subtle" href="#engineering">Открыть инженерный план</a>';
      related = ['material-types', 'material-trading']; references = [D.sources.ids, D.sources.engineering];
    } else if (catalog.key === 'modules') {
      body = '<p>Семейство корабельного оборудования. Размер, вариант, доступные отсеки и ограничения проверяются для конкретного модуля и корабля.</p><h3>Совместимые чертежи</h3>' + cards(groups.blueprints.filter((bp) => entity.blueprints.includes(bp.key)), 'catalog-blueprints');
      if (entity.experimentals?.length) body += '<h3>Экспериментальные эффекты</h3>' + cards(groups.experimentals.filter((effect) => entity.experimentals.includes(effect.key)), 'catalog-experimentals');
      related = ['module-size-rating', 'engineering-plan']; references = [D.sources.engineering];
    } else if (catalog.key === 'blueprints') {
      body = '<p>Ингредиенты ниже относятся к одному применению выбранной ступени на одном модуле. Для общего плана задавайте число оставшихся применений и модулей.</p>' + Object.entries(entity.grades || {}).map(([grade, rows]) => '<h3>G' + h(grade) + '</h3>' + materialRows(state, rows)).join('') + '<h3>Совместимые семейства</h3>' + cards(groups.modules.filter((module) => module.blueprints.includes(entity.key)), 'catalog-modules') + '<a class="button button-small button-subtle" href="#engineering">Добавить улучшение в план</a>';
      related = ['engineering-grades', 'experimental-effects']; references = [D.sources.engineering];
    } else if (catalog.key === 'experimentals') {
      body = '<p>Экспериментальный эффект применяется к совместимому корабельному модулю и имеет отдельную стоимость. В списке улучшений он учитывается один раз на каждый модуль.</p>' + materialRows(state, entity.materials || []) + '<h3>Совместимые семейства</h3>' + cards(groups.modules.filter((module) => module.experimentals.includes(entity.key)), 'catalog-modules');
      related = ['experimental-effects', 'engineering-plan']; references = [D.sources.engineering];
    } else if (catalog.key === 'engineers') {
      const status = { unlocked: 'Открыт', invited: 'Приглашение', known: 'Известен', acquainted: 'Знакомство', barred: 'Доступ закрыт', locked: 'Закрыт', unknown: 'Нет данных' };
      body = '<div class="codex-facts">' + field('Имя в источниках', h(entity.english)) + field('Система', h(entity.system || '—')) + field('База', h(entity.base || '—')) + field('Известный доступ', h(status[entity.status] || 'Нет данных') + (entity.manualUnlocked != null ? ' · вручную' : '')) + field('Репутация', n(entity.rank, true)) + '</div>' +
        note('Приглашение, условия открытия и доступная ступень — разные вещи. Ручная отметка ED Copilot не изменяет допуск в игре.') + '<a class="button button-small button-subtle" href="#engineering">Открыть список инженеров</a>';
      const capabilities = entity.capabilities || [];
      if (capabilities.length) body += '<h3>Поддерживаемые улучшения</h3>' + table(['Модуль', 'Чертёж', 'До ступени'], capabilities.map((row) => [h(row.moduleName), button(row.blueprintName, 'data-codex-record', 'catalog-blueprints:' + row.blueprintKey), 'G' + row.maxGrade]));
      related = ['engineer-access', 'pinned-blueprints']; references = [D.sources.ids, D.sources.engineering];
    }
    const links = related.map((id) => articleById(id)).filter(Boolean);
    return '<article class="panel codex-article"><div class="codex-article-head"><p class="eyebrow">' + h(catalog.name) + '</p><h2>' + h(entity.title) + '</h2><p>' + h(entity.subtitle || '') + '</p></div><div class="codex-article-body">' + body +
      (links.length ? '<h3>Понять механику</h3><div class="codex-related">' + links.map((article) => button(article.title, 'data-codex-article', article.id)).join('') + '</div>' : '') + '<div class="codex-reference"><small>Локальный справочник: ' + h(entity.verifiedAt || D.verifiedAt) + '. Неизвестные свойства обозначаются прочерком.</small>' + sources(references) + '</div></div></article>';
  }
  function catalogView(state, catalog, groups) {
    let rows = groups[catalog.key] || [];
    const filter = catalog.key === 'ships' ? [['all','Любая площадка'],['S','Малая'],['M','Средняя'],['L','Большая']]
      : catalog.key === 'materials' ? [['all','Все типы'], ...Object.entries(types)]
        : catalog.key === 'resources' ? [['all','Товары и сырьё'],['commodity','Товары'],['raw','Сырьё']] : null;
    if (ui.catalogFilter !== 'all') rows = rows.filter((row) => (catalog.key === 'ships' ? row.pad : catalog.key === 'materials' ? row.category : row.kind) === ui.catalogFilter);
    const filtered = rows.sort((a,b)=>a.title.localeCompare(b.title,'ru'));
    return '<section class="codex-intro"><div><h2>' + h(catalog.name) + '</h2><p>' + h(catalog.text) + '</p></div><span>' + n(filtered.length, true) + ' записей</span></section>' +
      (filter ? '<div class="codex-filter"><label class="field"><span>Отбор каталога</span><select data-codex-filter data-focus="codex-filter">' + filter.map(([value,title])=>'<option value="'+value+'"'+(ui.catalogFilter===value?' selected':'')+'>'+h(title)+'</option>').join('') + '</select></label></div>' : '') +
      cards(filtered.slice(0, ui.limit), catalog.id) + (filtered.length > ui.limit ? '<div class="codex-more">' + button('Показать ещё', 'data-codex-more') + '</div>' : '') +
      (!filtered.length ? note('В этой выборке записей пока нет. Снимите фильтр или дождитесь загрузки локальных данных.') : '') +
      (catalog.key === 'goods' ? note('В каталоге представлены известные программе товарные названия. Цены добавляются из наблюдений и ручных ориентиров; пустая цена не означает бесплатный товар.') : '');
  }
  function searchView(groups) {
    const aliases = { 'фсд': 'fsd', 'фсс': 'fss', 'дсс': 'dss', 'бгс': 'bgs', 'паверплей': 'powerplay', 'поверплей': 'powerplay', 'ско': 'sco' };
    const words = normalization(ui.query).split(' ').filter(Boolean).map((word) => aliases[word] || word);
    const records = [ ...D.articles.map((row) => ({ ...row, destination: row.section, recordKey: row.id, group: 'articles', search: [row.title,row.summary,...row.body,...row.tips].join(' ') })),
      ...catalogs.flatMap((catalog) => (groups[catalog.key] || []).map((row) => ({ ...row, destination: catalog.id, recordKey: row.key, group: catalog.key, summary: row.subtitle }))) ];
    const matches = records.filter((row) => { const text = normalization(row.search || row.title); return (ui.resultType === 'all' || row.group === ui.resultType) && words.every((word) => text.includes(word) || text.replaceAll(' ', '').includes(word)); })
      .sort((a,b)=>Number(normalization(b.title).startsWith(normalization(ui.query)))-Number(normalization(a.title).startsWith(normalization(ui.query))) || a.title.localeCompare(b.title,'ru'));
    return '<div class="codex-results-heading"><h2>Результаты поиска: ' + n(matches.length, true) + '</h2><label class="field"><span>Тип записи</span><select data-codex-type data-focus="codex-type">' + [['all','Вся энциклопедия'],['articles','Объяснения и занятия'],...catalogs.map((c)=>[c.key,c.name])].map(([value,title])=>'<option value="'+value+'"'+(ui.resultType===value?' selected':'')+'>'+h(title)+'</option>').join('') + '</select></label></div>' +
      '<div class="codex-record-grid">' + matches.slice(0,ui.limit).map((row) => '<div class="codex-search-entry"><span>' + h(row.group === 'articles' ? categoryName(row.section) : catalogs.find((c)=>c.key===row.group)?.name) + '</span>' + recordButton({ ...row, id: row.recordKey },row.destination) + '</div>').join('') + '</div>' +
      (matches.length > ui.limit ? '<div class="codex-more">' + button('Показать ещё', 'data-codex-more') + '</div>' : '') +
      (!matches.length ? '<div class="empty-state compact-empty"><h2>Ничего не найдено</h2><p>Попробуйте короткое понятие, русское или английское название: «щит», «FSD», «железо», «Python», «Powerplay».</p>' + button('Очистить поиск', 'data-codex-clear') + '</div>' : '');
  }
  function render(state) {
    const groups = entities(state);
    const section = sectionById(ui.section);
    const catalog = catalogs.find((row) => row.id === ui.section);
    const article = !catalog && ui.record ? articleById(ui.record) : null;
    const entity = catalog && ui.record ? (groups[catalog.key] || []).find((row) => row.key === ui.record) : null;
    let main;
    if (ui.query.trim()) main = searchView(groups);
    else if (article) main = articleDetail(article);
    else if (entity) main = entityDetail(state,catalog,entity,groups);
    else if (catalog) main = catalogView(state,catalog,groups);
    else if (section) main = '<section class="codex-intro"><div><h2>' + h(section.name) + '</h2><p>' + h(section.description) + '</p></div></section>' + cards(D.articles.filter((row)=>row.section===ui.section),ui.section);
    else main = home(groups);
    const crumbs = '<div class="codex-breadcrumbs">' + button('Энциклопедия', 'data-codex-section') + (section ? '<span>›</span>' + button(section.name,'data-codex-section',section.id) : '') + (article || entity ? '<span>›</span><strong>' + h((article || entity).title) + '</strong>' : '') + '</div>';
    return '<div class="page-heading"><div><p class="eyebrow">СПРАВОЧНИК ПИЛОТА</p><h1>Энциклопедия Elite Dangerous</h1><p>Полёт, корабли, занятия, галактика и справочные каталоги. Выберите раздел или найдите нужное название.</p></div></div>' +
      '<div class="codex-search"><label class="field"><span>Поиск по всей энциклопедии</span><input type="search" data-codex-search data-focus="codex-search" value="' + h(ui.query) + '" placeholder="Например: FSD, щиты, миссии, тритий, Python, экзобиология" aria-label="Поиск по энциклопедии"></label>' + (ui.query ? button('Сбросить','data-codex-clear') : '') + '</div>' + crumbs +
      (ui.section || ui.query.trim() ? '<div class="codex-layout">' + sidebar() + '<div class="codex-main">' + main + '</div></div>' : main);
  }
  function open(section = '', id = '') {
    const article = articleById(section);
    if (article && !id) { id = article.id; section = article.section; }
    ui.section = sectionById(section) ? section : '';
    ui.record = String(id || ''); ui.query = ''; ui.limit = 48; ui.catalogFilter = 'all'; ui.resultType = 'all';
    if (location.hash !== '#encyclopedia') location.hash = 'encyclopedia'; else context?.render();
    requestAnimationFrame(() => document.getElementById('content')?.scrollIntoView({ block: 'start' }));
  }
  function init(api) {
    context = api;
    document.addEventListener('input', (event) => { if (!event.target.matches('[data-codex-search]')) return; ui.query = event.target.value; ui.limit = 48; context.render(); });
    document.addEventListener('change', (event) => {
      if (event.target.matches('[data-codex-catalog]')) open(event.target.value);
      else if (event.target.matches('[data-codex-filter]')) { ui.catalogFilter = event.target.value; ui.limit=48; context.render(); }
      else if (event.target.matches('[data-codex-type]')) { ui.resultType=event.target.value; ui.limit=48; context.render(); }
    });
    document.addEventListener('click', (event) => {
      const target = event.target.closest('[data-codex-section],[data-codex-record],[data-codex-article],[data-codex-clear],[data-codex-more],[data-codex-mining]');
      if (!target) return;
      if (target.hasAttribute('data-codex-mining')) { if (window.CopilotTargets?.open) window.CopilotTargets.open('mining'); else location.hash='targets'; }
      else if (target.hasAttribute('data-codex-section')) open(target.dataset.codexSection);
      else if (target.hasAttribute('data-codex-article')) open(target.dataset.codexArticle);
      else if (target.hasAttribute('data-codex-record')) { const value=target.dataset.codexRecord, split=value.indexOf(':'); open(value.slice(0,split),value.slice(split+1)); }
      else if (target.hasAttribute('data-codex-clear')) { ui.query=''; ui.resultType='all'; context.render(); document.querySelector('[data-codex-search]')?.focus(); }
      else if (target.hasAttribute('data-codex-more')) { ui.limit+=48; context.render(); }
    });
  }
  window.CopilotEncyclopedia = { init, render, open, sectionIds: D.sections.map((row)=>row.id), catalogIds: catalogs.map((row)=>row.id) };
})();
