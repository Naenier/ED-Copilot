(function () {
  "use strict";

  const U = window.MiningUI;
  const { h, n, credits, dateTime, duration, pill } = U;
  let context;
  const ui = {
    mode: "sell", useCargo: true, useCarrierCargo: false, system: "", resourceKey: "", quantity: 1,
    radiusLy: 300, maxAgeHours: 24, carrierMaxAgeHours: 6, minDemand: 1, minSupply: 1,
    minPad: "L", includeCarriers: true, maxArrivalLs: 10000, demandRatio: 4,
    resultQuery: "", resultSort: "coverage", resultLimit: 20, initialized: false,
    weeklyUpkeep: "", reserveTritium: "", targetTankTons: 1000, carrierDraft: false,
    planDraft: {}, stockDraft: {}, planOpen: false, warehouseOpen: false,
    stockNewKey: "", stockNewCount: "", stockConfirmed: false,
  };
  const kindLabels = { deposit: "Пополнение", withdraw: "Снятие", upkeep: "Содержание", purchase: "Покупка",
    sale: "Продажа", fuel: "Заправка", jump: "Прыжок", transfer: "Перемещение" };
  const taxLabels = { all: "Общая ставка", refuel: "Заправка", repair: "Ремонт", rearm: "Боеприпасы",
    shipyard: "Верфь", outfitting: "Снаряжение", voucherredemption: "Обмен ваучеров",
    blackmarket: "Чёрный рынок", commodities: "Товарный рынок", exploration: "Картографические данные",
    bar: "Бар", pioneersupplies: "Снабжение «Пионер»" };

  function options(values, selected) {
    return Object.entries(values).map(([value, label]) => '<option value="' + h(value) + '"' +
      (String(selected) === value ? " selected" : "") + ">" + h(label) + "</option>").join("");
  }
  function button(label, action, value = "", style = "button-subtle") {
    return '<button type="button" class="button ' + style + '" data-action="' + h(action) +
      '" data-value="' + h(value) + '">' + h(label) + "</button>";
  }
  function heading(eyebrow, title, description, actions = "") {
    return '<div class="page-heading"><div><p class="eyebrow">' + h(eyebrow) + "</p><h1>" +
      h(title) + "</h1><p>" + h(description) + '</p></div><div class="heading-actions">' + actions + "</div></div>";
  }
  function panel(title, body, extra = "") {
    return '<section class="panel"><div class="panel-heading"><h2>' + h(title) + "</h2>" +
      extra + "</div>" + body + "</section>";
  }
  function field(label, control, grow = false) {
    return '<label class="field' + (grow ? " field-grow" : "") + '"><span>' + h(label) + "</span>" + control + "</label>";
  }
  function input(name, type = "number", attributes = "") {
    return '<input name="' + name + '" type="' + type + '" data-operation-field="' + name +
      '" data-focus="operation-' + name + '" value="' + h(ui[name]) + '" ' + attributes + ">";
  }
  function toggle(name, label, note = "") {
    return '<label class="operation-check"><input name="' + name + '" type="checkbox" data-operation-field="' +
      name + '"' + (ui[name] ? " checked" : "") + "><span>" + h(label) +
      (note ? "<small>" + h(note) + "</small>" : "") + "</span></label>";
  }
  function empty(title, text) {
    return '<div class="empty-state compact-empty"><div class="empty-icon" aria-hidden="true">◇</div><h2>' +
      h(title) + "</h2><p>" + h(text) + "</p></div>";
  }
  function metric(label, value, note = "") {
    return '<div class="metric"><div class="metric-label">' + h(label) +
      '</div><div class="metric-value numeric">' + value + '</div><div class="metric-note">' + note + "</div></div>";
  }
  function ageLabel(value, timestamp) {
    const hours = timestamp ? (Date.now() - Date.parse(timestamp)) / 3600000 : value != null ? Number(value) : null;
    if (hours == null || !Number.isFinite(hours)) return "Время котировки неизвестно";
    if (hours < 1) return "Менее часа назад";
    if (hours < 24) return n(hours) + " ч назад";
    return n(hours / 24) + " дн. назад";
  }
  function goods(state) {
    const all = [...(state.goods || []), ...(state.resources || [])].filter((item) => item.kind !== "raw");
    const unique = new Map();
    for (const item of all) {
      if (!item.key || /^(drones|limpets?)$/i.test(item.key)) continue;
      unique.set(item.key, item);
    }
    return [...unique.values()].sort((a, b) => (a.name || a.key).localeCompare(b.name || b.key, "ru"));
  }
  function selectedGoods(state) {
    const map = new Map(goods(state).map((item) => [item.key, item]));
    if (ui.useCargo && ui.mode === "sell") {
      const inventory = ui.useCarrierCargo ? state.carrier?.warehouse?.items : state.commander?.cargo;
      return (inventory || []).filter((item) => item.count > 0 && !/^(drones|limpets?)$/i.test(item.key))
        .map((item) => ({ ...item, name: map.get(item.key)?.name || item.name || item.key }));
    }
    const resource = map.get(ui.resourceKey);
    return resource ? [{ key: resource.key, name: resource.name || resource.key, count: Number(ui.quantity) || 1 }] : [];
  }
  function initialize(state) {
    if (ui.initialized) return;
    const saved = state.trading?.options;
    if (saved) {
      for (const key of Object.keys(ui)) if (Object.hasOwn(saved, key)) ui[key] = saved[key];
    }
    ui.initialized = true;
  }
  function resultRows(state) {
    const mode = state.trading?.options?.mode || ui.mode;
    const query = ui.resultQuery.trim().toLocaleLowerCase("ru-RU").replaceAll("ё", "е");
    const rows = (state.trading?.results || []).filter((row) => !query ||
      [row.system, row.station, ...(row.quotes || []).map((quote) => quote.name)].join(" ").toLocaleLowerCase("ru-RU").replaceAll("ё", "е").includes(query));
    return rows.sort((a, b) => {
      if (ui.resultSort === "distance") return (a.distanceLy ?? Infinity) - (b.distanceLy ?? Infinity);
      if (ui.resultSort === "freshness") return (a.ageHours ?? Infinity) - (b.ageHours ?? Infinity);
      if (ui.resultSort === "total") return mode === "buy"
        ? (a.estimatedTotal ?? Infinity) - (b.estimatedTotal ?? Infinity)
        : (b.estimatedTotal ?? -1) - (a.estimatedTotal ?? -1);
      const coverage = Number(b.coverageComplete) - Number(a.coverageComplete);
      if (coverage) return coverage;
      if (mode === "buy") return (a.averagePrice ?? Infinity) - (b.averagePrice ?? Infinity) ||
        (b.coveredTons || 0) - (a.coveredTons || 0) || (a.distanceLy ?? Infinity) - (b.distanceLy ?? Infinity);
      return Number(b.demandSafe) - Number(a.demandSafe) ||
        (b.estimatedTotal ?? -1) - (a.estimatedTotal ?? -1) || (b.coveredTons || 0) - (a.coveredTons || 0);
    });
  }
  function destination(state, compact = false) {
    const row = state.trading?.destination;
    if (!row) return "";
    return '<div class="destination-card"><div><small>ВЫБРАННЫЙ РЫНОК</small><h2>' +
      h(row.station || "Рынок") + '</h2><p>' + h(row.system) + " · " + n(row.distanceLy) +
      " св. г. · " + n(row.distanceLs, true) + " св. с.</p>" + (compact ? "" :
        '<div class="pill-row">' + pill(row.carrier ? "Флотоносец" : "Станция", "blue") +
        pill(row.expired ? "Котировка устарела" : ageLabel(row.ageHours, row.updatedAt), row.expired ? "potential" : "muted") + "</div>") +
      '</div><div class="destination-value"><strong>' + credits(row.estimatedTotal) +
      '</strong><small>за ' + n(row.coveredTons) + " т по котировкам поиска</small></div>" +
      '<div class="actions-wrap">' + button("Копировать систему", "operation-copy", row.system, "button-small button-subtle") +
      (compact ? '<a class="button button-small button-subtle" href="#sale">Покупатели</a>' :
        button("Снять цель", "trade-clear-destination", "", "button-small button-quiet")) + "</div></div>";
  }
  function quoteTable(row, mode) {
    return '<div class="table-wrap"><table class="market-quotes"><thead><tr><th>Товар</th><th class="table-num">' +
      (mode === "buy" ? "Покупка, кр./т" : "Продажа, кр./т") + '</th><th class="table-num">' +
      (mode === "buy" ? "Предложение, т" : "Спрос, т") + '</th><th class="table-num">В расчёте, т</th><th class="table-num">Сумма</th></tr></thead><tbody>' +
      (row.quotes || []).map((quote) => "<tr><td><strong>" + h(quote.name || quote.key) +
        '</strong><small class="quote-date">' + h(ageLabel(quote.ageHours, quote.updatedAt || row.updatedAt)) +
        '</small></td><td class="table-num">' + credits(quote.price) + '</td><td class="table-num">' +
        n(mode === "buy" ? quote.supply : quote.demand, true) + '</td><td class="table-num">' +
        n(quote.quantity) + '</td><td class="table-num">' + credits(quote.subtotal) + "</td></tr>").join("") +
      "</tbody></table></div>";
  }
  function marketResult(row, state, index) {
    const selected = state.trading?.destination?.id === row.id;
    const mode = state.trading?.options?.mode || ui.mode;
    const sourceUrl = U.safeUrl(row.sourceUrl);
    return '<article class="market-result' + (selected ? " is-selected" : "") + '"><div class="market-result-head"><span class="result-rank">' +
      n(index + 1, true) + '</span><div class="market-result-name"><h2>' + h(row.station) + "</h2><p>" +
      h(row.system) + '</p><div class="pill-row">' + pill(row.carrier ? "Флотоносец" : "Станция", "blue") +
      pill("Площадка " + (row.pad || "?"), "muted") + pill(row.expired ? "Котировка устарела" : ageLabel(row.ageHours, row.updatedAt),
        row.expired || row.ageHours > (row.carrier ? ui.carrierMaxAgeHours : ui.maxAgeHours) ? "potential" : "confirmed") +
      '</div></div><div class="market-result-total"><small>' + (mode === "buy" ? "РАСХОД НА ПОКУПКУ" : "СУММА ПРОДАЖИ") +
      "</small><strong>" + credits(row.estimatedTotal) + "</strong><span>" + n(row.coveredTons) + " т" +
      (row.missingTons > 0 ? " · ещё " + n(row.missingTons) + " т без предложения" : " · весь выбранный объём") +
      '</span></div></div><div class="market-result-route"><span>Перелёт <b>' + n(row.distanceLy) +
      " св. г.</b></span><span>От звезды <b>" + n(row.distanceLs, true) +
      " св. с.</b></span><span>Котировка <b>" + h(dateTime(row.updatedAt)) + "</b></span></div>" +
      quoteTable(row, mode) + ((row.warnings || []).length ? '<div class="market-result-warnings">' +
        row.warnings.map((warning) => "<p>◇ " + h(typeof warning === "string" ? warning : warning.text) + "</p>").join("") + "</div>" : "") +
      '<div class="market-result-actions">' + button(selected ? "✓ Выбран рынок" : "Везти сюда", "trade-select", row.id, "button-small " + (selected ? "button-subtle" : "button-primary")) +
      button("Копировать систему", "operation-copy", row.system, "button-small button-subtle") +
      (sourceUrl ? '<a class="button button-small button-quiet" href="' + h(sourceUrl) + '" target="_blank" rel="noreferrer">Источник ↗</a>' : "") +
      "</div></article>";
  }
  function tradeLedger(state) {
    const ledger = state.tradeLedger || {};
    const totals = ledger.totals || {};
    const recent = (ledger.recent || []).slice(0, 12);
    return panel("Торговый журнал",
      '<div class="metrics operation-ledger-metrics">' +
      metric("Внешние продажи", credits(totals.revenue), "По товарным операциям журнала") +
      metric("Закупки", credits(totals.purchaseCost), n(totals.boughtTons) + " т куплено") +
      metric("Маржа по известной себестоимости", credits(totals.grossMargin),
        n(totals.unknownCostTons) + " т продано с неизвестной себестоимостью") +
      metric("Эксплуатационные расходы", credits(totals.operatingCosts), "Учитываются отдельно от товарной маржи") +
      '</div><div class="panel-body"><p class="panel-note">' +
      h(ledger.help || "Закупки, внешние продажи и внутренние перемещения учитываются отдельно.") +
      '</p></div>' +
      (recent.length ? '<div class="table-wrap"><table><thead><tr><th>Время</th><th>Операция и товар</th><th class="table-num">Объём</th><th class="table-num">Сумма</th><th class="table-num">Маржа</th></tr></thead><tbody>' +
        recent.map((entry) => '<tr><td class="small muted">' + h(dateTime(entry.timestamp)) + "</td><td><strong>" +
          h(entry.name || "") + '</strong><small class="quote-date">' + h(entry.internal ? "Внутреннее перемещение" :
            ({ buy: "Покупка", sell: "Продажа", transfer: "Перемещение", cost: "Расход" }[entry.kind] || entry.kind || "")) +
          '</small></td><td class="table-num">' + n(entry.count) + '</td><td class="table-num">' +
          credits(entry.value) + '</td><td class="table-num">' + credits(entry.margin) + "</td></tr>").join("") +
        "</tbody></table></div>" : "") +
      '<div class="panel-body"><a class="button button-small button-subtle" href="/api/trading/export.csv">Экспорт торгового журнала CSV</a></div>');
  }
  function tradePlan(state) {
    const saved = state.tradePlan || {};
    const value = (key, fallback = "") => ui.planDraft[key] ?? saved[key] ?? fallback;
    const quantity = Number(value("quantity", ui.quantity));
    const buyPrice = Number(value("buyPrice"));
    const sellPrice = Number(value("sellPrice"));
    const complete = quantity > 0 && String(value("buyPrice")) !== "" && String(value("sellPrice")) !== "" &&
      [buyPrice, sellPrice].every(Number.isFinite);
    const itemOptions = Object.fromEntries(goods(state).map((item) => [item.key, item.name || item.key]));
    const planInput = (key, type = "number", fallback = "") => '<input name="' + key + '" type="' + type +
      '" data-plan-field="' + key + '" data-focus="plan-' + key + '" value="' + h(value(key, fallback)) +
      '"' + (type === "number" ? ' min="0" step="1"' : "") + ">";
    return '<details class="panel operation-plan" data-operation-open="planOpen"' + (ui.planOpen || saved.key ? " open" : "") + '><summary>Калькулятор перевозки' +
      (saved.name ? "<small>" + h(saved.name) + "</small>" : "") + '</summary><div class="panel-body"><p class="panel-note">План по выбранным ценам и объёму. Фактические закупки и продажи будут записаны в торговый журнал.</p>' +
      '<form id="trade-plan-form"><div class="form-grid">' +
      field("Товар", '<select name="key" data-plan-field="key" data-focus="plan-key">' +
        options({ "": "Выберите товар", ...itemOptions }, value("key", ui.resourceKey)) + "</select>") +
      field("Объём, т", planInput("quantity", "number", ui.quantity)) +
      field("Цена покупки, кр./т", planInput("buyPrice")) +
      field("Цена продажи, кр./т", planInput("sellPrice")) +
      field("Откуда", planInput("source", "text", state.commander?.system || "")) +
      field("Куда", planInput("destination", "text", state.trading?.destination?.system || "")) +
      '</div><div class="operation-plan-totals">' +
      '<span>Закупка <strong>' + credits(complete ? quantity * buyPrice : null) + "</strong></span>" +
      '<span>Продажа <strong>' + credits(complete ? quantity * sellPrice : null) + "</strong></span>" +
      '<span>Валовая маржа <strong>' + credits(complete ? quantity * (sellPrice - buyPrice) : null) + "</strong></span></div>" +
      '<div class="settings-actions"><button type="submit" class="button button-primary">Сохранить план</button>' +
      button("Подставить выбранную котировку", "trade-plan-quote", "", "button-subtle") +
      '</div></form></div></details>';
  }
  function renderMarket(state) {
    initialize(state);
    const trading = state.trading || {};
    const basket = selectedGoods(state);
    const rows = resultRows(state);
    const totalTons = basket.reduce((sum, item) => sum + Number(item.count || 0), 0);
    const commodityOptions = Object.fromEntries(goods(state).map((item) => [item.key, item.name || item.key]));
    const isRunning = trading.running === true;
    const warehouse = state.carrier?.warehouse;
    return heading("РЫНКИ И ПЕРЕВОЗКИ", "Куда везти груз",
      "Сравнение станций и флотоносцев по цене, доступному объёму, расстоянию и времени котировки.") +
      '<div class="operation-mode"><button type="button" data-action="trade-mode" data-value="sell" class="button ' + (ui.mode === "sell" ? "button-primary" : "button-subtle") +
      '">Продать груз</button><button type="button" data-action="trade-mode" data-value="buy" class="button ' + (ui.mode === "buy" ? "button-primary" : "button-subtle") + '">Купить товар</button></div>' +
      '<form id="trading-form" class="panel operation-search"><div class="panel-body"><div class="form-grid trading-form-grid">' +
      field("Исходная система", input("system", "text", 'placeholder="' + h(state.commander?.system || "Название системы") + '"'), true) +
      field("Радиус, св. г.", input("radiusLy", "number", 'min="1" max="2000"')) +
      field("Мин. посадочная площадка", '<select name="minPad" data-operation-field="minPad" data-focus="operation-minPad">' +
        options({ L: "Большая", M: "Средняя", S: "Малая", any: "Любая" }, ui.minPad) + "</select>") +
      field("От звезды, максимум св. с.", input("maxArrivalLs", "number", 'min="0" max="10000000"')) +
      field("Котировки станций не старше, ч", input("maxAgeHours", "number", 'min="1" max="720"')) +
      field("Котировки флотоносцев не старше, ч", input("carrierMaxAgeHours", "number", 'min="1" max="720"')) +
      field(ui.mode === "buy" ? "Минимальное предложение, т" : "Минимальный спрос, т", input(ui.mode === "buy" ? "minSupply" : "minDemand", "number", 'min="1"')) +
      (ui.mode === "sell" ? field("Предпочтительный запас спроса к грузу", '<select name="demandRatio" data-operation-field="demandRatio" data-focus="operation-demandRatio">' +
        options({ 1: "Не меньше груза", 4: "Не меньше 4 × груза", 10: "Не меньше 10 × груза" }, ui.demandRatio) + "</select>") : "") +
      '</div><div class="operation-switches">' +
      (ui.mode === "sell" ? toggle("useCargo", "Весь товарный груз", "Сравнить один рынок для нескольких товаров") +
        (state.carrier?.owned ? toggle("useCarrierCargo", "Использовать склад флотоносца", warehouse?.sourceLabel || "По доступным записям") : "") : "") +
      toggle("includeCarriers", "Учитывать флотоносцы", "Для них отдельно ограничена давность рынка") +
      '</div>' +
      (!ui.useCargo || ui.mode === "buy" ? '<div class="form-grid operation-commodity">' +
        field("Товар", '<select name="resourceKey" data-operation-field="resourceKey" data-focus="operation-resourceKey">' +
          options({ "": "Выберите товар", ...commodityOptions }, ui.resourceKey) + "</select>", true) +
        field("Объём, т", input("quantity", "number", 'min="1" max="1000000"')) + "</div>" : "") +
      '<div class="trade-basket"><span>В поиске <strong>' + n(totalTons) + " т</strong></span>" +
      (basket.length ? basket.slice(0, 12).map((item) => '<span class="resource-chip">' + h(item.name) + " " + n(item.count) + " т</span>").join("") :
        '<span class="muted small">Выберите товар или загрузите трюм.</span>') + "</div>" +
      (ui.useCarrierCargo && ui.mode === "sell" ? '<p class="settings-hint">Объём склада основан на доступном снимке и отслеженных перемещениях. Перед продажей сверяйте его с игрой.</p>' : "") +
      '<div class="settings-actions"><button class="button button-primary" type="submit"' + (isRunning ? " disabled" : "") + ">" +
      (isRunning ? "Поиск рынков…" : ui.mode === "buy" ? "Найти продавцов" : "Найти покупателей") + "</button>" +
      (isRunning ? button("Остановить", "trade-cancel", "", "button-subtle") : "") +
      '<span class="small muted">Рыночные данные обновляют командиры. Время котировки указано для каждого предложения.</span></div></div></form>' +
      (trading.status || trading.error ? '<div class="info-strip' + (trading.error ? " operation-error" : "") + '"><span>ⓘ</span><div>' +
        h(isRunning ? trading.progress?.label || trading.status || "" : trading.status || "") + (trading.error ? "<br>" + h(trading.error) : "") +
        (isRunning && trading.progress?.total ? '<div class="small">' + n(trading.progress.checked || 0, true) + " / " + n(trading.progress.total, true) + " товаров</div>" : "") +
        "</div></div>" : "") +
      destination(state) +
      '<div class="filters market-result-filters">' + field("Найти в результатах", '<input data-operation-control="resultQuery" data-focus="operation-resultQuery" value="' + h(ui.resultQuery) +
        '" placeholder="Система, станция или товар">', true) +
      field("Сортировка", '<select data-operation-control="resultSort" data-focus="operation-resultSort">' +
        options({ coverage: "Полнота груза и сумма", total: ui.mode === "buy" ? "Меньше расходов" : "Больше выручка", distance: "Ближе к вам", freshness: "Свежее котировка" }, ui.resultSort) + "</select>") +
      '<span class="count-label">' + n(rows.length, true) + " рынков</span></div>" +
      (rows.length ? '<div class="market-results">' + rows.slice(0, ui.resultLimit).map((row, index) => marketResult(row, state, index)).join("") + "</div>" +
        (rows.length > ui.resultLimit ? '<div class="operation-more">' + button("Показать ещё 20", "trade-more", "", "button-subtle") + "</div>" : "") :
        empty(isRunning ? "Получаем рыночные предложения" : trading.finishedAt || trading.updatedAt ? "Подходящих предложений нет" : "Найдите рынок для вашего груза",
          isRunning ? "Результаты появятся после ответов источника." : "Укажите систему, объём и требования к рынку. Чем шире радиус и допустимый возраст котировки, тем больше возможных покупателей.")) +
      '<div class="info-strip"><span>◇</span><div>Сумма рассчитана для доступного спроса или предложения. Котировки описывают рынок на момент обновления. Перед продажей проверьте цену и спрос в игре.</div></div>' +
      tradePlan(state) + tradeLedger(state);
  }

  function numericRow(label, value, unit = "") {
    return '<div class="carrier-kv"><span>' + h(label) + "</span><strong>" + n(value) + (value != null && unit ? " " + h(unit) : "") + "</strong></div>";
  }
  function creditRow(label, value) {
    return '<div class="carrier-kv"><span>' + h(label) + "</span><strong>" + credits(value) + "</strong></div>";
  }
  function serviceRow(service) {
    return '<div class="carrier-service"><span><strong>' + h(service.name || service.key) + "</strong>" +
      (service.crewName ? '<small>' + h(service.crewName) + "</small>" : "") + "</span>" +
      pill(service.installed === false ? "Не установлена" : service.enabled == null ? "Состояние неизвестно" : service.enabled === false ? "Приостановлена" : "Активна",
        service.installed === false || service.enabled == null ? "muted" : service.enabled === false ? "potential" : "confirmed") + "</div>";
  }
  function carrierWarehouse(carrier, state) {
    const warehouse = carrier.warehouse || {};
    const rows = warehouse.items || [];
    return panel("Товарный склад", '<div class="panel-body"><div class="pill-row">' +
      pill(warehouse.complete ? "Снимок склада" : "Отслеженные позиции", warehouse.complete ? "confirmed" : "potential") +
      '<span class="small muted">' + h(warehouse.sourceLabel || "По доступным событиям журнала") + "</span></div>" +
      '<p class="panel-note">' + (warehouse.complete ? "Количество показано на дату последнего снимка." :
        "Список отражает известные остатки и перемещения. Отсутствие товара в списке не подтверждает нулевой остаток.") + "</p></div>" +
      (rows.length ? '<div class="table-wrap"><table><thead><tr><th>Товар</th><th class="table-num">Учтено, т</th><th>Основание</th></tr></thead><tbody>' +
        rows.map((item) => "<tr><td><strong>" + h(item.name || item.key) + '</strong></td><td class="table-num">' +
          n(item.count, true) + '</td><td class="small muted">' + h(item.basis || "Перемещения") + "</td></tr>").join("") +
        "</tbody></table></div>" : empty("Остатки ещё не известны", "Перемещения между кораблём и флотоносцем появятся здесь после событий в журнале.")) +
      '<details class="warehouse-reconcile" data-operation-open="warehouseOpen"' + (ui.warehouseOpen ? " open" : "") + '><summary>Сверить остатки с игрой</summary><form id="carrier-stock-form"><p class="panel-note">Укажите полный состав товарного склада по экрану флотоносца. Этот снимок станет основанием для последующего учёта перемещений.</p>' +
      rows.map((item) => field(item.name || item.key, '<input type="number" min="0" step="1" data-stock-key="' + h(item.key) +
        '" data-focus="stock-' + h(item.key) + '" value="' + h(ui.stockDraft[item.key] ?? item.count ?? 0) + '">')).join("") +
      '<div class="form-grid">' + field("Добавить товар", '<select name="newKey" data-stock-new="stockNewKey" data-focus="stock-newKey">' +
        options({ "": "Выберите товар", ...Object.fromEntries(goods(state).filter((good) => !rows.some((row) => row.key === good.key))
          .map((good) => [good.key, good.name || good.key])) }, ui.stockNewKey) + "</select>") +
      field("Количество, т", '<input name="newCount" type="number" min="0" step="1" value="' + h(ui.stockNewCount) + '" data-stock-new="stockNewCount" data-focus="stock-newCount">') +
      '</div><label class="operation-check"><input name="complete" type="checkbox" data-stock-new="stockConfirmed" required' + (ui.stockConfirmed ? " checked" : "") + '><span>Я сверил полный состав склада с игрой</span></label>' +
      '<button type="submit" class="button button-primary">Сохранить полный снимок</button></form></details>',
      '<small>' + n(warehouse.knownTons) + " т учтено</small>");
  }
  function carrierOrders(carrier) {
    const rows = carrier.orders || [];
    return panel("Торговые заявки", rows.length ?
      '<div class="table-wrap"><table><thead><tr><th>Товар</th><th>Заявка флотоносца</th><th class="table-num">Цена, кр./т</th><th class="table-num">Объём, т</th><th>Запись</th></tr></thead><tbody>' +
      rows.map((row) => "<tr><td><strong>" + h(row.name || row.key) + "</strong></td><td>" +
        pill(row.side === "buy" ? "Покупает" : "Продаёт", row.side === "buy" ? "blue" : "confirmed") +
        '</td><td class="table-num">' + credits(row.price) + '</td><td class="table-num">' + n(row.quantity) +
        '</td><td class="small muted">' + h(dateTime(row.timestamp)) + "</td></tr>").join("") + "</tbody></table></div>" :
      empty("Заявок пока нет в журнале", "Выставленные в игре заявки появятся после записи об изменении рынка."));
  }
  function carrierTransactions(carrier) {
    const rows = (carrier.transactions || []).slice(0, 30);
    return panel("Последние операции", rows.length ?
      '<div class="table-wrap"><table><thead><tr><th>Время</th><th>Операция</th><th class="table-num">Сумма</th><th class="table-num">Баланс</th></tr></thead><tbody>' +
      rows.map((row) => '<tr><td class="small muted">' + h(dateTime(row.timestamp)) + "</td><td>" +
        h(row.label || kindLabels[row.kind] || row.kind) + '</td><td class="table-num">' + credits(row.amount) +
        '</td><td class="table-num">' + credits(row.balance) + "</td></tr>").join("") + "</tbody></table></div>" :
      empty("Операций пока нет", "Пополнение средств, списания, заправка и торговые события будут учитываться по журналу."));
  }
  function renderCarrier(state) {
    const carrier = state.carrier || {};
    if (!carrier.owned) return heading("ЛИЧНАЯ БАЗА И ЛОГИСТИКА", "Мой флотоносец",
      "Топливо, финансы, склад, службы и прыжки по записям вашего командира.") +
      panel("Подключение флотоносца", empty("Личный флотоносец ещё не появился в журнале",
        "После покупки откройте управление флотоносцем в игре. Данные его первого снимка появятся здесь автоматически.")) +
      '<div class="carrier-onboarding"><section class="panel"><div class="panel-heading"><h2>Что появится после покупки</h2></div><div class="panel-body"><ul class="plain-list">' +
      ["Название, позывной и местоположение", "Тритий в баке и известный резерв на складе",
        "Баланс, финансовый резерв и план содержания", "Установленные и приостановленные службы",
        "Отслеженные остатки, перемещения и торговые заявки", "Запланированный прыжок и подтверждённое прибытие"]
        .map((text) => "<li>" + h(text) + "</li>").join("") + "</ul></div></section>" +
      panel("Пока можно подготовить снабжение", '<div class="panel-body"><p class="panel-note">В разделе торговли выберите покупку трития и объём запаса. После покупки флотоносца текущие средства и топливо будут доступны для планирования.</p><a class="button button-subtle" href="#sale">Открыть торговлю</a></div>') + "</div>";
    const fuel = carrier.fuel || {};
    const finance = carrier.finance || {};
    const space = carrier.space || {};
    const jump = carrier.jump || {};
    const pending = jump.pending;
    const pendingSeconds = pending?.departureMs != null ? Math.max(0, (pending.departureMs - Date.now()) / 1000) : jump.secondsToDeparture;
    const fill = fuel.tankTons != null && fuel.tankCapacityTons > 0 ? Math.max(0, Math.min(100, fuel.tankTons / fuel.tankCapacityTons * 100)) : null;
    const system = carrier.location?.system;
    const totalAvailableFuel = fuel.tankTons != null && fuel.reserveTons != null ? fuel.tankTons + fuel.reserveTons : null;
    const advisories = carrier.advisories || [];
    const services = carrier.services || [];
    return heading("ЛИЧНАЯ БАЗА И ЛОГИСТИКА", carrier.name || "Мой флотоносец",
      [carrier.callsign, system ? "Система " + system : "Местоположение уточняется", carrier.statsAt ? "Снимок " + dateTime(carrier.statsAt) : ""].filter(Boolean).join(" · "),
      system ? button("Копировать систему", "operation-copy", system, "button-subtle") : "") +
      '<div class="metrics carrier-metrics">' +
      metric("Тритий в баке", n(fuel.tankTons) + '<em> / ' + n(fuel.tankCapacityTons) + " т</em>",
        fuel.reserveTons == null ? "Резерв на складе ещё не известен" : "На складе " + n(fuel.reserveTons) + " т",
      ) +
      metric("Средства флотоносца", credits(finance.balance), "Свободно " + credits(finance.availableBalance)) +
      metric("Вместимость", n(space.freeTons) + "<em> т свободно</em>", "Всего " + n(space.capacityTons) + " т") +
      metric("План содержания", finance.runwayWeeks != null ? n(finance.runwayWeeks) + "<em> нед.</em>" : "—",
        finance.weeklyUpkeep != null ? credits(finance.weeklyUpkeep) + " в неделю" : "Укажите недельные расходы в плане") +
      "</div>" +
      (advisories.length ? '<div class="carrier-advisories">' + advisories.map((item) => '<div class="info-strip' +
        (item.level === "warn" ? " operation-error" : "") + '"><span>◇</span><div>' + h(item.text) + "</div></div>").join("") + "</div>" : "") +
      '<div class="carrier-grid"><div class="stack">' +
      panel("Прыжок флотоносца", '<div class="panel-body">' +
        (pending ? '<div class="carrier-jump-target"><small>ЗАПЛАНИРОВАНО</small><h2>' + h(pending.system) +
          "</h2><p>" + h(pending.body || "") + '</p><div class="pill-row">' +
          '<span class="pill pill-potential" data-carrier-countdown>' +
          h(pendingSeconds > 0 ? "До отправления " + duration(pendingSeconds) : "Ожидается подтверждение прибытия") + "</span>" +
          '</div><p class="settings-hint">Отправление ' + h(dateTime(pending.departureTime)) + "</p>" +
          button("Копировать назначение", "operation-copy", pending.system, "button-small button-subtle") + "</div>" :
          '<div class="carrier-jump-target"><h2>' + h(jump.phase === "cooldown" ? "Восстановление после прыжка" : "Ожидает назначения") +
          '</h2><p class="panel-note">Назначенный в игре маршрут появится после записи запроса прыжка.</p></div>') +
        (jump.lastArrival ? '<hr class="subtle-divider"><p class="small">Последнее прибытие <strong>' +
          h(jump.lastArrival.system || jump.lastArrival.StarSystem || "") + "</strong> · " +
          h(dateTime(jump.lastArrival.timestamp || jump.lastArrival.at)) + "</p>" : "") +
        (jump.cooldownUntilMs ? '<p class="settings-hint">' + (jump.cooldownEstimated ? "Оценка окончания восстановления " : "Окончание восстановления ") +
          h(dateTime(jump.cooldownUntilMs)) + "</p>" : "") +
        "</div>") +
      panel("Топливо и грузовые отсеки", '<div class="panel-body">' +
        (fill != null ? '<div class="progress carrier-fuel"><span style="width:' + fill + '%"></span></div>' : "") +
        numericRow("Бак", fuel.tankTons, "т") + numericRow("Известный резерв трития", fuel.reserveTons, "т") +
        numericRow("Бак и известный резерв", totalAvailableFuel, "т") +
        numericRow("Дальность с текущим топливом", fuel.jumpRangeCurrent, "св. г.") +
        numericRow("Товарный груз по снимку", space.cargoTons, "т") + numericRow("Зарезервировано", space.reservedTons, "т") +
        numericRow("Команда и службы", space.crewTons, "т") +
        numericRow("Комплекты кораблей", space.shipPacksTons, "т") + numericRow("Комплекты модулей", space.modulePacksTons, "т") +
        '<a class="button button-small button-subtle" href="#sale" data-action="carrier-find-tritium">Найти тритий для снабжения</a></div>') +
      carrierWarehouse(carrier, state) + '</div><aside class="stack">' +
      panel("Финансы и доступ", '<div class="panel-body">' +
        creditRow("Общий баланс", finance.balance) + creditRow("Финансовый резерв", finance.reserveBalance) +
        creditRow("Доступно", finance.availableBalance) +
        '<div class="carrier-kv"><span>Доступ к стыковке</span><strong>' + h(carrier.dockingAccessLabel || carrier.dockingAccess || "Неизвестно") + "</strong></div>" +
        '<div class="carrier-kv"><span>Доступ с дурной славой</span><strong>' + (carrier.allowNotorious == null ? "—" : carrier.allowNotorious ? "Разрешён" : "Закрыт") +
        "</strong></div>" +
        (Object.keys(finance.taxRates || {}).length ? '<hr class="subtle-divider">' +
          Object.entries(finance.taxRates).map(([name, value]) => numericRow(taxLabels[name.toLowerCase()] || name, value, "%")).join("") : "") + "</div>") +
      panel("Службы", '<div class="panel-body">' + (services.length ? services.map(serviceRow).join("") :
        '<p class="panel-note">Данные об установленных службах ещё не получены.</p>') + "</div>") +
      panel("План содержания", '<div class="panel-body"><p class="panel-note">Плановые значения хранятся в компаньоне и помогают оценить запас средств и трития.</p>' +
        '<form id="carrier-planning-form"><div class="form-grid">' +
        field("Содержание в неделю, кр.", '<input name="weeklyUpkeep" type="number" min="0" step="1" data-carrier-field="weeklyUpkeep" data-focus="carrier-weeklyUpkeep" value="' +
          h(ui.carrierDraft ? ui.weeklyUpkeep : finance.weeklyUpkeep ?? "") + '" placeholder="Из экрана флотоносца">') +
        field("Целевой резерв трития на складе, т", '<input name="reserveTritium" type="number" min="0" step="1" data-carrier-field="reserveTritium" data-focus="carrier-reserveTritium" value="' +
          h(ui.carrierDraft ? ui.reserveTritium : carrier.planning?.reserveTritium ?? "") + '" placeholder="Нужный запас">') +
        field("Целевой запас в баке, т", '<input name="targetTankTons" type="number" min="0" max="1000" step="1" data-carrier-field="targetTankTons" data-focus="carrier-targetTankTons" value="' +
          h(ui.carrierDraft ? ui.targetTankTons : carrier.planning?.targetTankTons ?? 1000) + '">') +
        '</div><div class="settings-actions"><button class="button button-primary" type="submit">Сохранить план</button></div></form></div>') +
      "</aside></div>" + carrierOrders(carrier) + carrierTransactions(carrier) +
      '<div class="info-strip"><span>ⓘ</span><div>Параметры флотоносца обновляются по журналу игры. Дата снимка, основание складских остатков и подтверждение прибытия показаны отдельно.</div></div>';
  }

  async function submitTrading(form) {
    const data = new FormData(form);
    const request = { mode: ui.mode, useCargo: ui.mode === "sell" && data.has("useCargo"),
      useCarrierCargo: ui.mode === "sell" && data.has("useCarrierCargo"), includeCarriers: data.has("includeCarriers") };
    for (const key of ["system", "resourceKey", "minPad"]) request[key] = String(data.get(key) ?? ui[key]).trim();
    for (const key of ["quantity", "radiusLy", "maxAgeHours", "carrierMaxAgeHours", "minDemand", "minSupply", "maxArrivalLs", "demandRatio"])
      request[key] = Number(data.get(key) ?? ui[key]);
    request.system ||= context.getState().commander?.system || "";
    Object.assign(ui, request, { resultLimit: 20 });
    await context.perform("/api/trading/search", request);
    context.render();
  }
  function init(value) {
    context = value;
    document.addEventListener("input", (event) => {
      const input = event.target;
      if (input.dataset.operationField) ui[input.dataset.operationField] = input.type === "checkbox" ? input.checked : input.value;
      if (input.dataset.operationControl) {
        ui[input.dataset.operationControl] = input.value;
        ui.resultLimit = 20;
        context.render();
      }
      if (input.dataset.carrierField) {
        if (!ui.carrierDraft) {
          const carrier = context.getState().carrier || {};
          ui.weeklyUpkeep = carrier.finance?.weeklyUpkeep ?? "";
          ui.reserveTritium = carrier.planning?.reserveTritium ?? "";
          ui.targetTankTons = carrier.planning?.targetTankTons ?? 1000;
        }
        ui.carrierDraft = true;
        ui[input.dataset.carrierField] = input.value;
      }
      if (input.dataset.planField) {
        ui.planDraft[input.dataset.planField] = input.value;
        context.render();
      }
      if (input.dataset.stockKey) ui.stockDraft[input.dataset.stockKey] = input.value;
      if (input.dataset.stockNew) ui[input.dataset.stockNew] = input.type === "checkbox" ? input.checked : input.value;
    });
    document.addEventListener("change", (event) => {
      const input = event.target;
      if (input.dataset.operationField) {
        ui[input.dataset.operationField] = input.type === "checkbox" ? input.checked : input.value;
        context.render();
      }
      if (input.dataset.operationControl) {
        ui[input.dataset.operationControl] = input.value;
        context.render();
      }
      if (input.dataset.planField) ui.planDraft[input.dataset.planField] = input.value;
      if (input.dataset.stockNew) ui[input.dataset.stockNew] = input.type === "checkbox" ? input.checked : input.value;
    });
    document.addEventListener("submit", async (event) => {
      if (event.target.id === "trading-form") {
        event.preventDefault();
        await submitTrading(event.target);
      }
      if (event.target.id === "carrier-planning-form") {
        event.preventDefault();
        const data = new FormData(event.target);
        const result = await context.perform("/api/carrier/planning", {
          weeklyUpkeep: data.get("weeklyUpkeep") === "" ? null : Number(data.get("weeklyUpkeep")),
          reserveTritium: data.get("reserveTritium") === "" ? null : Number(data.get("reserveTritium")),
          targetTankTons: data.get("targetTankTons") === "" ? 1000 : Number(data.get("targetTankTons")),
        }, "План флотоносца сохранён");
        if (result) ui.carrierDraft = false;
      }
      if (event.target.id === "trade-plan-form") {
        event.preventDefault();
        const values = new FormData(event.target);
        const request = {};
        for (const key of ["key", "source", "destination"]) request[key] = String(values.get(key) || "");
        for (const key of ["quantity", "buyPrice", "sellPrice"]) request[key] = Number(values.get(key));
        const result = await context.perform("/api/trading/plan", request, "План перевозки сохранён");
        if (result) ui.planDraft = {};
      }
      if (event.target.id === "carrier-stock-form") {
        event.preventDefault();
        const values = new FormData(event.target);
        const items = [...event.target.querySelectorAll("[data-stock-key]")].map((input) => ({
          key: input.dataset.stockKey, count: Number(input.value),
        }));
        if (values.get("newKey")) items.push({ key: String(values.get("newKey")), count: Number(values.get("newCount")) });
        const result = await context.perform("/api/carrier/stock", { carrierId: context.getState().carrier?.id,
          items, timestamp: new Date().toISOString() }, "Снимок склада сохранён");
        if (result) {
          ui.stockDraft = {};
          ui.stockNewKey = "";
          ui.stockNewCount = "";
          ui.stockConfirmed = false;
        }
      }
    });
    document.addEventListener("click", async (event) => {
      const button = event.target.closest("[data-action]");
      if (!button) return;
      const action = button.dataset.action;
      const value = button.dataset.value;
      if (action === "trade-mode") {
        ui.mode = value;
        context.render();
      } else if (action === "trade-cancel") await context.perform("/api/trading/cancel", {});
      else if (action === "trade-more") {
        ui.resultLimit += 20;
        context.render();
      } else if (action === "trade-select") {
        const row = (context.getState().trading?.results || []).find((row) => row.id === value);
        if (row) await context.perform("/api/trading/destination", { destination: row }, "Рынок выбран для перевозки");
      } else if (action === "trade-clear-destination") {
        await context.perform("/api/trading/destination", { destination: null }, "Рынок убран из плана");
      } else if (action === "operation-copy") {
        await U.copyText(value).then(() => context.showToast("Система скопирована"), (error) => context.showToast(error.message, true));
      } else if (action === "trade-plan-quote") {
        const state = context.getState();
        const quote = state.trading?.destination?.quotes?.[0];
        if (!quote) return context.showToast("Сначала выберите рыночное предложение");
        const cost = state.tradeLedger?.inventory?.find((item) => item.key === quote.key)?.unitCost;
        ui.planDraft = { key: quote.key, quantity: quote.quantity, source: state.commander?.system || "",
          destination: state.trading.destination.system, buyPrice: cost ?? "", sellPrice: quote.price };
        ui.planOpen = true;
        context.render();
      } else if (action === "carrier-find-tritium") {
        ui.mode = "buy";
        ui.resourceKey = "tritium";
        ui.useCargo = false;
        ui.quantity = 1000;
        location.hash = "sale";
        context.render();
      }
    });
    document.addEventListener("toggle", (event) => {
      if (event.target.dataset.operationOpen) ui[event.target.dataset.operationOpen] = event.target.open;
    }, true);
    const clock = setInterval(() => {
      const pending = context.getState()?.carrier?.jump?.pending;
      const element = document.querySelector("[data-carrier-countdown]");
      if (!pending?.departureMs || !element) return;
      const remaining = Math.max(0, (pending.departureMs - Date.now()) / 1000);
      element.textContent = remaining > 0 ? "До отправления " + duration(remaining) : "Ожидается подтверждение прибытия";
    }, 1000);
    window.addEventListener("beforeunload", () => clearInterval(clock));
  }

  window.MiningOperations = { init, renderMarket, renderCarrier, destination };
})();
