(function () {
  "use strict";
  const U = window.MiningUI;
  const { h, n, credits, dateTime, duration, pill } = U;
  let context;
  const drafts = {};
  const minutesDuration = (value) => duration(value == null ? null : value * 60);
  const field = (label, control) => '<label class="field"><span>' + h(label) + '</span>' + control + '</label>';
  const button = (label, action, value = "") => '<button class="button button-subtle" type="button" data-expedition-action="' + action + '" data-value="' + h(value) + '">' + h(label) + '</button>';
  const panel = (title, body) => '<section class="panel"><div class="panel-heading"><h2>' + h(title) + '</h2></div>' + body + '</section>';
  const metric = (title, value, note = "") => '<div class="metric"><div class="metric-label">' + h(title) + '</div><div class="metric-value numeric">' + value + '</div><div class="metric-note">' + h(note) + '</div></div>';
  const heading = (title, description) => '<div class="page-heading"><div><p class="eyebrow">ПЛАНИРОВАНИЕ И ЛОГИСТИКА</p><h1>' + h(title) + '</h1><p>' + h(description) + '</p></div></div>';
  const note = (text, error = false) => text ? '<div class="info-strip' + (error ? ' is-warning' : '') + '"><div>' + h(text) + '</div></div>' : '';
  function input(form, values, key, label, type = "number", attrs = "") {
    return field(label, '<input data-expedition-field="' + form + '" data-focus="' + form + '-' + key + '" name="' + key + '" type="' + type + '" value="' + h(values[key] ?? "") + '" ' + attrs + '>');
  }
  function check(form, values, key, label) {
    return '<label class="operation-check"><input type="checkbox" name="' + key + '" data-expedition-field="' + form + '"' + (values[key] ? ' checked' : '') + '><span>' + h(label) + '</span></label>';
  }
  function select(form, values, key, label, options) {
    return field(label, '<select name="' + key + '" data-expedition-field="' + form + '" data-focus="' + form + '-' + key + '">' + options.map(([value, text]) => '<option value="' + h(value) + '"' + (String(values[key]) === String(value) ? ' selected' : '') + '>' + h(text) + '</option>').join('') + '</select>');
  }
  function values(form, defaults) { return { ...defaults, ...drafts[form] }; }
  function links(sources) { return (sources || []).map((source) => '<a href="' + h(source.url) + '" target="_blank" rel="noreferrer">' + h(source.name || source.title) + '</a>').join(' · '); }
  function fleetInputs(state) {
    const c = state.carrier || {}, account = state.frontier || {}, live = account.summary?.carrier;
    const fields = { sourceSystem: c.location?.system || state.commander.system,
      usedCapacityTons: c.space?.capacityTons != null && c.space?.freeTons != null ? c.space.capacityTons - c.space.freeTons : 0,
      tankTons: c.fuel?.tankTons ?? 1000, cargoTritiumTons: c.fuel?.reserveTons ?? 0 };
    if (live?.owned && (!c.callsign || c.callsign === live.callsign) &&
        (!c.statsAt || Date.parse(account.lastCarrierSync) >= Date.parse(c.statsAt))) {
      fields.sourceSystem = live.system || fields.sourceSystem;
      fields.usedCapacityTons = live.usedCapacity ?? fields.usedCapacityTons;
      fields.tankTons = live.fuel ?? fields.tankTons;
      fields.cargoTritiumTons = (live.cargo || []).filter((item) => String(item.name).toLowerCase() === 'tritium').reduce((sum, item) => sum + item.count, 0);
    }
    return fields;
  }

  function renderRoute(state) {
    const route = state.carrierRoute || {};
    const carrier = state.carrier || {};
    const o = values('fleet-route', { ...fleetInputs(state),
      destinationSystem: '', waypoints: '', refuelSystems: '', calculateStartingFuel: true,
      reserveTritiumTons: 0, jumpMinutes: 15, cooldownMinutes: 5, overheadMinutes: 1, maxQueueMinutes: 0,
      miningTonsPerHour: 150, refuelMinutes: 0, tritiumPrice: 0,
      ...(route.route || route.options?.destinationSystem ? route.options : {}), waypoints: (route.options?.waypoints || []).join('\n'), refuelSystems: (route.options?.refuelSystems || []).join('\n'),
      departureTime: route.options?.departureTime ? localDate(route.options.departureTime) : '' });
    const s = route.summary || {};
    const stops = route.route?.stops || [];
    const advanced = '<details class="expedition-details" data-preserve-open="fleet-route-advanced"><summary>Время, запас топлива и дозаправка</summary><div class="form-grid expedition-form">' +
      input('fleet-route', o, 'departureTime', 'Плановое отправление (местное время)', 'datetime-local') +
      input('fleet-route', o, 'reserveTritiumTons', 'Целевой остаток трития по прибытии, т', 'number', 'min="0" max="26000"') +
      input('fleet-route', o, 'jumpMinutes', 'Подготовка и прыжок, мин', 'number', 'min="1" max="120" step="0.1"') +
      input('fleet-route', o, 'cooldownMinutes', 'Восстановление между прыжками, мин', 'number', 'min="0" max="120" step="0.1"') +
      input('fleet-route', o, 'overheadMinutes', 'Ваши действия между прыжками, мин', 'number', 'min="0" max="120" step="0.1"') +
      input('fleet-route', o, 'maxQueueMinutes', 'Дополнительная очередь на прыжок, до мин', 'number', 'min="0" max="240"') +
      input('fleet-route', o, 'miningTonsPerHour', 'Скорость добычи трития, т/ч', 'number', 'min="1" max="10000"') +
      input('fleet-route', o, 'refuelMinutes', 'Перелив / закупка на остановке, мин', 'number', 'min="0" max="1440"') +
      input('fleet-route', o, 'tritiumPrice', 'Цена трития для оценки затрат, кр/т', 'number', 'min="0" max="100000000"') + '</div>' +
      select('fleet-route', { refuelMethod: o.refuelMethod || 'mine' }, 'refuelMethod', 'Как пополняете тритий в пути', [['mine', 'Добыча — добавить время по т/ч'], ['buy', 'Закупка / погрузка'], ['manual', 'Ручная оценка времени дозаправки']]) +
      '<div class="form-grid expedition-form">' + field('Промежуточные системы по порядку (одна на строке)', '<textarea name="waypoints" rows="4" data-expedition-field="fleet-route" data-focus="fleet-route-waypoints">' + h(o.waypoints) + '</textarea>') +
      field('На каких промежуточных остановках пополнить запас (одна на строке)', '<textarea name="refuelSystems" rows="4" data-expedition-field="fleet-route" data-focus="fleet-route-refuelSystems">' + h(o.refuelSystems) + '</textarea>') + '</div></details>';
    let result = '';
    if (stops.length) result = '<div class="metrics">' + metric('Прыжки', n(s.totalJumps, true), n(s.distanceLy) + ' св. г.') +
      metric('Расход трития', n(s.fuelTons) + '<small> т</small>', 'Стоимость: ' + credits(s.fuelCost)) +
      metric('Всего времени', minutesDuration(s.totalMinutes), 'Прибытие: ' + dateTime(s.estimatedArrival)) +
      metric('Тритий к прибытию', n(s.arrivalTritiumTons) + '<small> т</small>', 'Нехватка до резерва: ' + n(s.reserveShortfallTons) + ' т') + '</div>' +
      panel('План полёта', '<div class="panel-body"><div class="pill-row">' + pill('Маршрут Spansh', 'blue') +
        pill('Пополнить в пути ' + n(s.restockTons) + ' т', s.restockTons ? 'potential' : 'muted') +
        pill('Добыча ' + minutesDuration(s.miningMinutes), 'muted') + '</div>' +
        '<div class="summary-grid"><div><small>На старте: бак / склад</small><strong>' + n(s.startingTankTons) + ' / ' + n(s.startingCargoTritiumTons) + ' т</strong></div><div><small>Время с введённой очередью</small><strong>' + minutesDuration(s.timeRange?.minMinutes) + ' — ' + minutesDuration(s.timeRange?.maxMinutes) + '</strong></div><div><small>Прыжков осталось</small><strong>' + n(s.remainingJumps, true) + '</strong></div></div>' +
        '<p class="panel-note">Масса на старте: ' + n(route.route?.mass?.startingTotalMassTons) + ' т · завершение с конечной дозаправкой: ' + dateTime(s.estimatedComplete) + '</p>' +
        '<p class="panel-note">Бак и склад в таблице показаны после прибытия и предусмотренного перелива. «Пополнить» — требуемое добавление топлива, доступность закупки или добычи нужно подтвердить. Время — оценка с вашими задержками; верхняя оценка ограничена введённой очередью, реальная очередь может быть больше.</p>' +
        '<a class="button button-small button-subtle" href="/api/carrier/route.csv" download>↓ Экспорт CSV</a>' +
        button('Копировать следующую систему', 'route-next') + '</div><div class="table-wrap"><table><thead><tr><th>№ / система</th><th class="table-num">Прыжок, св. г.</th><th class="table-num">Расход, т</th><th class="table-num">Бак / склад, т</th><th class="table-num">Пополнить, т</th><th>Прибытие</th><th></th></tr></thead><tbody>' +
        stops.map((stop) => '<tr class="' + (stop.visited ? 'route-visited' : '') + '"><td><strong>' + h(stop.index) + '. ' + h(stop.name) + '</strong><div class="pill-row">' +
          (stop.isDestination ? pill('Цель', 'blue') : '') + (stop.hasIcyRing ? pill(stop.isPristine ? 'Нетронутые ледяные кольца' : 'Ледяные кольца', 'muted') : '') +
          (stop.visited ? pill('Пройдено', 'confirmed') : '') + '</div></td><td class="table-num">' + n(stop.distanceLy) + '</td><td class="table-num">' + n(stop.fuelUsedTons) + '</td><td class="table-num">' + n(stop.tankTons) + ' / ' + n(stop.cargoTritiumTons) +
          '</td><td class="table-num">' + (stop.mustRestock ? '⚠ ' : '') + n(stop.restockTons) + '</td><td>' + dateTime(stop.estimatedArrival) + '</td><td><div class="actions-wrap">' + button('Копировать', 'copy', stop.name) + button('Пройдено до сюда', 'route-visited', stop.index) + '</div></td></tr>').join('') + '</tbody></table></div>');
    return heading('Маршрут флотоносца', 'Промежуточные системы, топливо, дозаправки и расписание. Расчёт пути выполняет Spansh.') +
      panel('Параметры экспедиции', '<form id="fleet-route-form" class="panel-body"><div class="form-grid expedition-form">' +
        input('fleet-route', o, 'sourceSystem', 'Откуда', 'text', 'required maxlength="150"') + input('fleet-route', o, 'destinationSystem', 'Куда', 'text', 'required maxlength="150"') +
        input('fleet-route', o, 'usedCapacityTons', 'Занято из 25 000 т (включая тритий на складе)', 'number', 'required min="0" max="25000"') +
        input('fleet-route', o, 'tankTons', 'Тритий в баке, т', 'number', 'min="0" max="1000"') +
        input('fleet-route', o, 'cargoTritiumTons', 'Тритий на складе, т', 'number', 'min="0" max="25000"') + '</div>' +
        check('fleet-route', o, 'calculateStartingFuel', 'Подобрать стартовый запас топлива автоматически (как на Spansh)') +
        '<p class="panel-note">При выключенном подборе используется указанный реальный запас. Занятая вместимость включает груз, службы и резервы из управления флотоносцем; бак в неё не входит. Не прибавляйте складской тритий второй раз.</p>' + advanced +
        '<div class="actions-wrap"><button class="button" type="submit"' + (route.running ? ' disabled' : '') + '>Рассчитать маршрут</button>' +
        (route.running ? button('Отменить', 'route-cancel') : '') + button('Подставить данные флотоносца', 'route-current') + '</div></form>') +
      note(route.progress?.label || ({ ready: 'Маршрут рассчитан и сохранён.', cancelled: 'Расчёт отменён.', error: 'Не удалось рассчитать маршрут.' }[route.status]) || (route.running ? 'Идёт расчёт…' : '')) + note(route.error, true) + (route.warnings || []).map((w) => note(typeof w === 'string' ? w : w.text, true)).join('') + result +
      (route.assumptions?.length ? '<details class="expedition-details" data-preserve-open="fleet-route-assumptions"><summary>Как рассчитаны топливо и время</summary><ul class="plain-list">' + route.assumptions.map((a) => '<li>' + h(typeof a === 'string' ? a : a.text) + '</li>').join('') + '</ul></details>' : '') +
      '<p class="panel-note">' + (route.route?.sourceUrl ? '<a href="' + h(route.route.sourceUrl) + '" target="_blank" rel="noreferrer">Открыть тот же расчёт на Spansh</a> · ' : '') + links(route.sources) + '</p>';
  }
  function localDate(value) { const d = new Date(value); return Number.isFinite(d.getTime()) ? new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16) : ''; }

  function renderTradeRoutes(state) {
    const routes = state.tradeRoutes || {};
    const o = values('trade-routes', { system: state.commander.system, resourceKey: '', cargoCapacity: state.commander.cargoCapacity || 128,
      credits: state.frontier?.summary?.profile?.credits ?? 10000000, radiusLy: 100, maxLegLy: 100, maxAgeHours: 24,
      carrierMaxAgeHours: 6, minutesPerLeg: 15, operatingCostPerLeg: 0, minPad: 'L', maxArrivalLs: 10000, demandRatio: 4,
      roundTrip: false, includeCarriers: false, includePlanetary: true, ...routes.options,
      resourceKey: routes.options?.resourceKeys?.length === 1 ? routes.options.resourceKeys[0] : '' });
    const goods = (state.goods || []).filter((g) => g.kind !== 'raw').sort((a, b) => a.name.localeCompare(b.name, 'ru'));
    const rows = routes.results || [];
    return heading('Прибыльные торговые рейсы', 'Сравнение закупки и продажи с учётом бюджета, трюма, спроса, запаса и расстояния. Можно искать замкнутый рейс.') +
      panel('Условия поиска', '<form id="trade-routes-form" class="panel-body"><div class="form-grid expedition-form">' +
        input('trade-routes', o, 'system', 'Область вокруг системы', 'text', 'required') +
        select('trade-routes', o, 'resourceKey', 'Товар', [['', 'Подобрать из 6 основных товаров'], ...goods.map((g) => [g.key, g.name])]) +
        input('trade-routes', o, 'cargoCapacity', 'Полезный трюм, т', 'number', 'required min="1" max="25000"') +
        input('trade-routes', o, 'credits', 'Доступно на закупку, кр.', 'number', 'required min="1"') +
        input('trade-routes', o, 'radiusLy', 'Радиус поиска рынков, св. г.', 'number', 'min="5" max="5000"') +
        input('trade-routes', o, 'maxLegLy', 'Максимум между рынками, св. г.', 'number', 'min="1" max="5000"') +
        input('trade-routes', o, 'maxAgeHours', 'Возраст цен станций, не более ч', 'number', 'min="1" max="720"') +
        input('trade-routes', o, 'carrierMaxAgeHours', 'Возраст цен флотоносцев, не более ч', 'number', 'min="0.25" max="168" step="0.25"') +
        input('trade-routes', o, 'maxArrivalLs', 'От звезды, до св. с. (0 — любой)', 'number', 'min="0"') +
        select('trade-routes', o, 'minPad', 'Посадочная площадка', [['L', 'Большая'], ['M', 'Средняя'], ['S', 'Малая'], ['any', 'Любая']]) +
        input('trade-routes', o, 'minutesPerLeg', 'Ваше время на один гружёный рейс, мин', 'number', 'min="1"') +
        input('trade-routes', o, 'operatingCostPerLeg', 'Расходы на один рейс, кр.', 'number', 'min="0"') +
        input('trade-routes', o, 'demandRatio', 'Спрос не ниже объёма ×', 'number', 'min="1" max="20"') + '</div><div class="operation-checks">' +
        check('trade-routes', o, 'roundTrip', 'Замкнутый рейс с товаром обратно') + check('trade-routes', o, 'includeCarriers', 'Включить флотоносцы') + check('trade-routes', o, 'includePlanetary', 'Включить планетарные рынки') + '</div>' +
        '<div class="actions-wrap"><button class="button" type="submit"' + (routes.running ? ' disabled' : '') + '>Найти рейсы</button>' + (routes.running ? button('Отменить', 'trade-cancel') : '') + '</div></form>') +
      note(routes.status) + note(routes.error, true) + note(routes.assumptions || 'Время рейса задаётся вручную и включает погрузку, полёт и посадку. Расстояние между системами — по прямой; цепочку прыжков корабля прокладывайте в игре.') +
      (routes.coverageLimited ? note('Поиск сравнивает до 120 рынков на сторону для каждого из шести выбранных товаров. Лучший результат во всей галактике не гарантируется.') : '') +
      '<div class="stack">' + rows.slice(0, 30).map((row, index) => panel('Рейс ' + (index + 1) + ' · ' + credits(row.netEstimate),
        '<div class="panel-body"><div class="pill-row">' + pill(credits(row.profitPerHour) + '/ч по вашему времени', 'blue') + pill(duration(row.minutes * 60), 'muted') + '</div>' +
        row.legs.map((leg) => '<div class="trade-leg"><div><strong>' + h(leg.name) + ' · ' + n(leg.quantity, true) + ' т</strong><p>' + h(leg.buy.system + ' / ' + leg.buy.station) + ' → ' + h(leg.sell.system + ' / ' + leg.sell.station) + '</p><small>Закупка ' + credits(leg.buyPrice) + '/т · продажа ' + credits(leg.sellPrice) + '/т · ' + n(leg.distanceLy) + ' св. г. · прибыль ' + credits(leg.netEstimate) + '</small><p class="panel-note">Запас ' + n(leg.supply, true) + ' т · спрос ' + n(leg.demand, true) + ' т · котировки ' + dateTime(leg.buy.updatedAt) + ' / ' + dateTime(leg.sell.updatedAt) + '</p></div><div class="actions-wrap">' + button('Копировать закупку', 'copy', leg.buy.system) + button('Копировать продажу', 'copy', leg.sell.system) + button('Вести груз на продажу', 'trade-destination', index + ':' + row.legs.indexOf(leg)) + '</div></div>').join('') + '</div>')).join('') + '</div>' +
      '<p class="panel-note">' + links(routes.sources) + '</p>';
  }

  function renderAccount(state) {
    const account = state.frontier || {};
    const profile = account.summary?.profile;
    const carrier = account.summary?.carrier;
    let summary = '';
    if (profile) summary += '<div class="metrics">' + metric('Командир', h(profile.commander)) + metric('Кредиты', credits(profile.credits)) + metric('Система', h(profile.system)) + metric('Корабль', h(profile.ship)) + '</div>';
    if (carrier?.owned) summary += panel('Снимок флотоносца Frontier', '<div class="panel-body"><h2>' + h(carrier.name + ' · ' + carrier.callsign) + '</h2><p>' + h(carrier.system) + '</p><div class="summary-grid">' +
      '<div><small>Тритий в баке</small><strong>' + n(carrier.fuel) + ' т</strong></div><div><small>Баланс</small><strong>' + credits(carrier.balance) + '</strong></div><div><small>Свободный склад</small><strong>' + n(carrier.freeSpace) + ' т</strong></div></div>' +
      '<p class="panel-note">Получено ' + dateTime(account.lastCarrierSync) + '. Снимок CAPI показан отдельно от истории журналов.</p>' +
      (state.carrier?.owned && state.carrier.callsign === carrier.callsign ? HButtonWarehouse() : '') + '</div>' +
      '<div class="table-wrap"><table><thead><tr><th>Товар на складе</th><th class="table-num">Количество</th></tr></thead><tbody>' + (carrier.cargo || []).map((item) => '<tr><td>' + h((state.goods || []).find((g) => g.key === String(item.name).toLowerCase().replace(/[^a-z0-9]/g, ''))?.name || item.localized || item.name) + '</td><td class="table-num">' + n(item.count, true) + '</td></tr>').join('') + '</tbody></table></div>');
    return heading('Аккаунт Frontier', 'Войдите в аккаунт, чтобы обновлять сведения о командире, корабле и личном флотоносце.') +
      note(account.error, true) + panel('Подключение', '<div class="panel-body"><div class="pill-row">' + pill(account.connected ? 'Вход сохранён' : account.pending ? 'Ожидаем входа в браузере' : 'Аккаунт не подключён', account.connected ? 'confirmed' : 'muted') + pill(account.storageAvailable ? 'Хранилище Windows' : 'Нужно настольное приложение', 'blue') + '</div>' +
        '<p>Последняя синхронизация: ' + dateTime(account.lastSync) + '</p><div class="actions-wrap">' +
        button('Войти через Frontier', 'frontier-login') + (account.connected ? button(account.syncing ? 'Синхронизация…' : 'Обновить данные', 'frontier-sync') + button('Выйти', 'frontier-logout') : '') + '</div>' +
        '<p class="panel-note">Пароль вводится на сайте Frontier. Вход сохраняется в защищённом хранилище Windows. Профиль обновляется не чаще раза в минуту, флотоносец — раз в 15 минут; сведения могут запаздывать относительно игры.</p></div>') +
      summary;
  }
  function HButtonWarehouse() { return button('Сверить склад по этому снимку Frontier', 'frontier-warehouse'); }

  function init(api) {
    context = api;
    document.addEventListener('input', (event) => {
      const input = event.target;
      const form = input.dataset.expeditionField;
      if (!form) return;
      drafts[form] ||= {};
      drafts[form][input.name] = input.type === 'checkbox' ? input.checked : input.value;
    });
    document.addEventListener('submit', async (event) => {
      const form = event.target;
      const paths = { 'fleet-route-form': '/api/carrier/route', 'trade-routes-form': '/api/trading/routes', 'mining-plan-form': '/api/mining/plan' };
      if (!paths[form.id]) return;
      event.preventDefault();
      const body = Object.fromEntries(new FormData(form));
      for (const input of form.querySelectorAll('input[type="checkbox"]')) body[input.name] = input.checked;
      if (form.id === 'fleet-route-form') {
        for (const key of ['waypoints', 'refuelSystems']) body[key] = String(body[key] || '').split(/[\n;]+/).map((s) => s.trim()).filter(Boolean);
        body.departureTime = body.departureTime ? new Date(body.departureTime).toISOString() : null;
      }
      if (form.id === 'trade-routes-form' && !body.resourceKey) delete body.resourceKey;
      await context.perform(paths[form.id], body);
    });
    document.addEventListener('click', async (event) => {
      const action = event.target.closest('[data-expedition-action]');
      if (!action) return;
      const key = action.dataset.expeditionAction, value = action.dataset.value;
      const state = context.getState();
      if (key === 'copy') { await U.copyText(value); context.showToast('Название системы скопировано'); }
      else if (key === 'route-cancel') await context.perform('/api/carrier/route/cancel', {});
      else if (key === 'route-visited') await context.perform('/api/carrier/route/visited', { index: Number(value) });
      else if (key === 'route-next') {
        const stop = (state.carrierRoute?.route?.stops || []).find((s) => !s.visited && s.index > 0);
        if (stop) { await U.copyText(stop.name); context.showToast('Следующая система скопирована'); }
      } else if (key === 'route-current') {
        drafts['fleet-route'] = { ...drafts['fleet-route'], ...fleetInputs(state) }; context.render();
      } else if (key === 'trade-cancel') await context.perform('/api/trading/routes/cancel', {});
      else if (key === 'trade-destination') {
        const [row, leg] = value.split(':').map(Number);
        const destination = state.tradeRoutes?.results?.[row]?.legs?.[leg]?.sell;
        if (destination) await context.perform('/api/trading/destination', { destination }, 'Место продажи добавлено в оверлей торговли');
      } else if (key === 'frontier-login') {
        const result = await context.perform('/api/frontier/login', {});
        if (result?.url && !window.edMining) window.open(result.url, '_blank', 'noopener');
      } else if (key === 'frontier-sync') await context.perform('/api/frontier/sync', { carrier: true });
      else if (key === 'frontier-warehouse') await context.perform('/api/frontier/warehouse', {}, 'Снимок склада добавлен в учёт');
      else if (key === 'frontier-logout') await context.perform('/api/frontier/logout', {});
      else if (key === 'mining-reset') await context.perform('/api/mining/reset', {}, 'Начат новый заход добычи');
    });
  }
  window.MiningExpedition = { init, renderRoute, renderTradeRoutes, renderAccount, helpers: { field, input, check, select, values, heading, panel, metric, note, links, button } };
})();
