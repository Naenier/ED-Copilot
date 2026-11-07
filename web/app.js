(function () {
  "use strict";

  const U = window.MiningUI;
  const { h, n, credits, dateTime, duration, pill } = U;
  const content = document.getElementById("content");
  const tabs = ["overview", "targets", "sale", "deals", "carrier", "encyclopedia", "statistics", "mining", "engineering", "route", "settings", "account", "integrations", "help"];
  let state = null;
  let connection = false;
  let initialSettings = false;
  let composing = false;
  let toastTimer;
  const ui = {
    tab: tabs.includes(location.hash.slice(1)) ? location.hash.slice(1) : "overview",
    targetQuery: "",
    targetKind: "all",
    targetSort: "distance",
    targetResource: "",
    minTier: "medium",
    method: "all",
    resourceQuery: "",
    resourceKind: "all",
    searchSystem: "",
    radiusLy: 50,
    busy: false,
    priceDraft: {},
    settingsDraft: {},
  };

  const methods = U.methods;
  function options(items, selected) {
    return Object.entries(items)
      .map(
        ([value, text]) =>
          '<option value="' +
          h(value) +
          '"' +
          (selected === value ? " selected" : "") +
          ">" +
          h(text) +
          "</option>",
      )
      .join("");
  }

  function field(label, input, extraClass = "") {
    return '<label class="field ' + extraClass + '"><span>' + h(label) + "</span>" + input + "</label>";
  }

  function heading(eyebrow, title, description, actions = "") {
    return (
      '<div class="page-heading"><div><p class="eyebrow">' +
      h(eyebrow) +
      "</p><h1>" +
      h(title) +
      "</h1><p>" +
      h(description) +
      '</p></div><div class="heading-actions">' +
      actions +
      "</div></div>"
    );
  }

  function panel(title, body, note = "", extraClass = "") {
    return (
      '<section class="panel ' +
      extraClass +
      '"><div class="panel-heading"><h2>' +
      h(title) +
      "</h2>" +
      note +
      "</div>" +
      body +
      "</section>"
    );
  }

  function empty(title, text, action = "") {
    return (
      '<div class="empty-state compact-empty"><div class="empty-icon" aria-hidden="true">◇</div><h2>' +
      h(title) +
      "</h2><p>" +
      h(text) +
      "</p>" +
      action +
      "</div>"
    );
  }

  function metric(label, value, note, extra = "") {
    return (
      '<div class="metric"><div class="metric-label">' +
      h(label) +
      '</div><div class="metric-value numeric">' +
      value +
      '</div><div class="metric-note">' +
      note +
      "</div>" +
      extra +
      "</div>"
    );
  }

  function button(label, action, value = "", style = "") {
    return (
      '<button class="button ' +
      style +
      '" type="button" data-action="' +
      h(action) +
      '" data-value="' +
      h(value) +
      '">' +
      h(label) +
      "</button>"
    );
  }

  function resourceMap() {
    return new Map((state.resources || []).map((item) => [item.key, item]));
  }

  function targetTitle(target) {
    return target.ring || target.body || target.system || "Место добычи";
  }

  function targetResourceChips(target, limit = 7) {
    const rows = target.hotspots?.length
      ? target.hotspots
      : target.resources?.length
        ? target.resources
        : target.materials || [];
    const seen = new Set();
    const visible = rows.filter((resource) => {
      if (seen.has(resource.key || resource.name)) return false;
      seen.add(resource.key || resource.name);
      return true;
    });
    let html = visible
      .slice(0, limit)
      .map(
        (resource) =>
          '<span class="resource-chip ' +
          (target.hotspots?.length ? "confirmed" : "") +
          '">' +
          h(resource.name || resource.key) +
          (resource.count != null
            ? " ×" + n(resource.count, true)
            : resource.percent != null
              ? " " + n(resource.percent) + "%"
              : "") +
          "</span>",
      )
      .join("");
    if (visible.length > limit)
      html += '<span class="resource-chip">ещё ' + n(visible.length - limit, true) + "</span>";
    if (target.miningSites > 0 && target.compositionUnknown)
      html += '<span class="resource-chip">Состав требует разведки</span>';
    return '<div class="resource-chips">' + html + "</div>";
  }

  function filteredTargets() {
    const query = ui.targetQuery.toLocaleLowerCase("ru-RU").trim().replaceAll("ё", "е");
    const resources = resourceMap();
    return (state.targets || [])
      .filter((target) => {
        if (!U.eligibleTarget(target, state, { minTier: ui.minTier, method: ui.method })) return false;
        if (ui.targetKind !== "all" && ui.targetKind !== "raw" && target.kind !== ui.targetKind) return false;
        if (
          ui.targetKind === "raw" &&
          !(target.materials || []).length &&
          !(target.resources || []).some((resource) => resources.get(resource.key)?.kind === "raw")
        )
          return false;
        if (
          ui.targetResource &&
          ![...(target.resources || []), ...(target.hotspots || []), ...(target.materials || [])].some(
            (resource) => resource.key === ui.targetResource,
          )
        )
          return false;
        const haystack = [
          target.system,
          target.body,
          target.ring,
          target.ringType,
          ...(target.resources || []).map((item) => item.name),
        ]
          .join(" ")
          .toLocaleLowerCase("ru-RU")
          .replaceAll("ё", "е");
        return !query || haystack.includes(query);
      })
      .sort((a, b) =>
        ui.targetSort === "distance"
          ? (a.distanceLy ?? Infinity) - (b.distanceLy ?? Infinity) || (b.score || 0) - (a.score || 0)
          : (b.score || 0) - (a.score || 0) || (a.distanceLy ?? Infinity) - (b.distanceLy ?? Infinity),
      );
  }

  function targetActions(target, small = true) {
    const saved = (state.bookmarks || []).some((bookmark) => bookmark.targetId === target.id);
    return (
      button(
        saved ? "✓ В маршруте" : "+ В маршрут",
        "bookmark",
        target.id,
        small ? "button-small button-subtle" : "button-subtle",
      ) +
      button(
        "Копировать систему",
        "copy",
        target.system,
        small ? "button-small button-quiet" : "button-small button-quiet",
      )
    );
  }

  function targetRow(target) {
    return (
      '<div class="target-row"><div><div class="target-name"><strong>' +
      h(targetTitle(target)) +
      "</strong>" +
      U.tier(target) +
      '</div><p class="target-meta">' +
      h(target.system) +
      " · " +
      h(
        target.kind === "ring"
          ? U.ringTypes[target.ringType] || target.ringType || "Тип кольца неизвестен"
          : "Поверхность",
      ) +
      (target.miningSites > 0 ? " · мест добычи: " + n(target.miningSites, true) : "") +
      "</p>" +
      targetResourceChips(target, 4) +
      '</div><div class="target-side"><span class="target-distance">' +
      n(target.distanceLy) +
      " <small>св. г.</small></span>" +
      button("+", "bookmark", target.id, "button-small button-subtle") +
      "</div></div>"
    );
  }

  function prospectPanel() {
    const prospect = state.prospect;
    if (!prospect)
      return panel(
        "Последний разведанный астероид",
        empty(
          "Запустите разведывательного дрона",
          "Состав, доля минералов и наличие ядра поступят из события ProspectedAsteroid в игровом журнале.",
        ),
        pill("ОЖИДАНИЕ", "muted"),
      );
    const decision =
      prospect.decision === "mine"
        ? ["Стоит добывать", "confirmed"]
        : prospect.decision === "skip"
          ? ["Низкий приоритет", "potential"]
          : ["Оценка требует данных", "muted"];
    const bars = (prospect.materials || [])
      .map(
        (material) =>
          '<div class="material-bar"><div class="resource-name"><i></i><span>' +
          h(material.name || material.key) +
          '</span></div><div class="progress"><span style="width:' +
          Math.max(0, Math.min(100, Number(material.percent) || 0)) +
          '%"></span></div><span class="value numeric">' +
          n(material.percent) +
          "%</span></div>",
      )
      .join("");
    const body =
      '<div class="panel-body"><div class="prospect-heading"><div class="prospect-title"><span class="prospect-icon" aria-hidden="true">⬡</span><div><h2>' +
      h(
        prospect.motherlode
          ? "Ядро · " + U.resourceName(state.resources, prospect.motherlode)
          : "Разведка астероида",
      ) +
      "</h2><p>" +
      h(prospect.system || state.commander?.system || "Система неизвестна") +
      " · " +
      dateTime(prospect.timestamp) +
      "</p></div></div>" +
      pill(decision[0], decision[1]) +
      "</div>" +
      bars +
      '<div class="summary-grid"><div><small>Осталось в астероиде</small><strong>' +
      n(prospect.remaining) +
      "%</strong></div><div><small>Содержание материалов</small><strong>" +
      h(prospect.content || "Не указано") +
      "</strong></div><div><small>Смесь по известным ценам</small><strong>" +
      credits(prospect.estimatedPerTonne > 0 ? prospect.estimatedPerTonne : null) +
      '</strong></div></div><hr class="subtle-divider"><p class="panel-note">' +
      h(
        (prospect.reasons || []).join(" ") ||
          "Содержание и цены помогают сравнивать астероиды. Полный объём извлекаемой руды журнал не сообщает.",
      ) +
      "</p></div>";
    return panel("Последний разведанный астероид", body, "<small>ИЗ ЖУРНАЛА ИГРЫ</small>");
  }

  function cargoPanel() {
    const cargo = state.commander?.cargo || [];
    const rows = cargo.length
      ? '<div class="table-wrap"><table><thead><tr><th>Груз</th><th class="table-num">Тонн</th><th class="table-num">Оценка</th></tr></thead><tbody>' +
        cargo
          .map((item) => {
            const resource = resourceMap().get(item.key);
            return (
              "<tr><td>" +
              h(item.name || item.key) +
              '</td><td class="table-num">' +
              n(item.count, true) +
              '</td><td class="table-num">' +
              credits(resource?.price > 0 ? resource.price * item.count : null) +
              "</td></tr>"
            );
          })
          .join("") +
        "</tbody></table></div>"
      : empty(
          "Трюм пока пуст",
          "Текущий состав груза читается из Cargo.json. Запасы инженерных материалов учитываются отдельно.",
        );
    return panel("Содержимое трюма", rows, "<small>КОРAБЛЬ</small>");
  }

  function surfacePanel() {
    const surface = state.surface;
    const body =
      '<div class="panel-body"><div class="row row-between"><div><h2>' +
      h(surface?.site || surface?.body || "Место ещё не выбрано") +
      '</h2><p class="panel-note">' +
      h(surface?.system || state.commander?.system || "Ожидание положения") +
      "</p></div>" +
      pill(state.commander?.vehicle === "srv" ? "В ТРП" : "ПОВЕРХНОСТЬ", "blue") +
      "</div>" +
      (surface?.lastRefined?.length
        ? '<div class="resource-chips" style="margin-top:15px">' +
          surface.lastRefined
            .map(
              (item) =>
                '<span class="resource-chip">' +
                h(item.name || item.key) +
                " · " +
                n(item.count) +
                " т</span>",
            )
            .join("") +
          "</div>"
        : "") +
      '<hr class="subtle-divider"><p class="panel-note">Журнал подтверждает места планетарной добычи и переработанные тонны. Состав и объём конкретной залежи требуют разведки на месте.</p></div>';
    return panel("Планетарная добыча", body);
  }

  function renderOverview() {
    const commander = state.commander || {};
    const total = state.stats?.total || {};
    const cargoValue = U.resourceValue(commander.cargo, state.resources);
    const used = Number(commander.cargoTons) || 0;
    const capacity = commander.cargoCapacity;
    const capacityText =
      capacity == null ? "<small>/ ? т</small>" : "<small>/ " + n(capacity, true) + " т</small>";
    const fill = capacity > 0 ? Math.max(0, Math.min(100, (used / capacity) * 100)) : 0;
    const targets = filteredTargets();
    const resourcesProduced = state.stats?.byResource || [];
    const latest = (state.stats?.recent || []).slice(0, 5);
    const activity = latest.length
      ? '<div class="table-wrap"><table><thead><tr><th>Время</th><th>Событие</th><th class="table-num">Количество</th></tr></thead><tbody>' +
        latest
          .map(
            (entry) =>
              '<tr><td class="muted">' +
              dateTime(entry.timestamp) +
              "</td><td>" +
              h(entry.name || eventName(entry.kind)) +
              '<div class="tiny muted">' +
              h(eventName(entry.kind)) +
              "</div>" +
              '</td><td class="table-num">' +
              n(entry.count, true) +
              (entry.value > 0 ? " · " + credits(entry.value) : "") +
              "</td></tr>",
          )
          .join("") +
        "</tbody></table></div>"
      : empty(
          "Событий добычи пока нет",
          "Разведка астероидов, переработка руды и сбор материалов появятся здесь автоматически.",
        );
    const production = resourcesProduced
      .filter((entry) => (entry.refinedTons || 0) > 0)
      .sort((a, b) => b.refinedTons - a.refinedTons);
    const productionHtml = production.length
      ? '<div class="panel-body"><div class="chart-list">' +
        production
          .slice(0, 5)
          .map(
            (entry) =>
              '<div class="chart-row"><span>' +
              h(entry.name || entry.key) +
              '</span><div class="chart-track"><span style="width:' +
              Math.max(3, (entry.refinedTons / production[0].refinedTons) * 100) +
              '%"></span></div><strong>' +
              n(entry.refinedTons) +
              " т</strong></div>",
          )
          .join("") +
        "</div></div>"
      : empty("Начните добычу", "Здесь появится распределение переработанной руды по ресурсам.");
    return (
      heading(
        "РАБОЧЕЕ МЕСТО ШАХТЁРА",
        "Добыча под контролем",
        "Разведка, трюм, места добычи и фактические результаты из журнала полёта.",
        '<a class="button button-subtle" href="#targets">⌖ Найти места добычи</a>',
      ) +
      window.MiningOperations.destination(state, true) +
      '<div class="metrics">' +
      metric(
        "Трюм корабля",
        n(used, true) + " " + capacityText,
        capacity ? n(fill) + "% занято" : "Вместимость появится после Loadout",
        '<div class="progress ' +
          (fill > 90 ? "is-warn" : "") +
          '"><span style="width:' +
          fill +
          '%"></span></div>',
      ) +
      metric(
        "Переработано",
        n(total.refinedTons || 0, true) + " <small>т</small>",
        "За период учёта · " + duration(state.stats?.elapsedSeconds),
      ) +
      metric(
        "Оценка груза",
        credits(cargoValue.total),
        cargoValue.unknownTons > 0
          ? n(cargoValue.unknownTons) + " т без цены"
          : "По известным справочным ценам",
      ) +
      metric("Продажи добычи", credits(total.miningRevenue || 0), "Товары с подтверждённой переработкой") +
      '</div><div class="dashboard-grid"><div class="stack">' +
      (commander.vehicle === "srv" ? surfacePanel() : prospectPanel()) +
      panel(
        "Места для следующего вылета",
        targets.length
          ? targets.slice(0, 4).map(targetRow).join("")
          : empty(
              "Подходящих мест пока нет",
              "Просканируйте тела и кольца или запустите поиск систем. По умолчанию показаны средние и высокие оценки, а также подтверждённые места с неизвестным составом.",
              '<a class="button button-subtle" href="#targets">Открыть поиск</a>',
            ),
        '<a class="soft-link" href="#targets">Все цели →</a>',
      ) +
      panel("Последние события", activity) +
      '</div><aside class="stack">' +
      cargoPanel() +
      panel("Переработка по ресурсам", productionHtml) +
      panel(
        "Оценки и подтверждения",
        '<div class="panel-body"><p class="panel-note">' +
          "Сигналы горячих точек подтверждают ресурс в кольце. Тип кольца показывает подходящие условия, но сам по себе не подтверждает находку.</p>" +
          '<p class="panel-note">Планетарное место добычи может быть подтверждено даже без данных о руде. Такие места отмечены отдельно.</p>' +
          '<p class="panel-note">Продажная стоимость рассчитывается только при известной цене. Цена станции и время её получения видны в справочнике.</p></div>',
      ) +
      "</aside></div>"
    );
  }

  function targetCard(target) {
    const sourceNames = { journal: "Игровой журнал", edsm: "EDSM", spansh: "Spansh" };
    const details =
      '<div class="target-details"><div><small>Тип</small><span>' +
      h(
        target.kind === "ring"
          ? U.ringTypes[target.ringType] || target.ringType || "Неизвестен"
          : target.miningSites > 0
            ? "Планетарное место добычи"
            : "Сбор сырья",
      ) +
      "</span></div><div><small>От звезды прибытия</small><span>" +
      n(target.distanceLs) +
      " св. с.</span></div>" +
      (target.reserve
        ? "<div><small>Запасы</small><span>" +
          h(U.reserves[target.reserve] || target.reserve) +
          "</span></div>"
        : "") +
      (target.kind === "surface"
        ? "<div><small>Гравитация</small><span>" + n(target.gravity) + " g</span></div>"
        : "") +
      (target.miningSites > 0
        ? "<div><small>Подтверждённые места</small><span>" + n(target.miningSites, true) + "</span></div>"
        : "") +
      "</div>";
    const recommendations = [target.recommendation, ...(target.reasons || [])].filter(Boolean);
    const observed = (target.observedResources || [])
      .map(
        (resource) =>
          '<span class="resource-chip confirmed">' +
          h(resource.name || resource.key) +
          " · " +
          n(resource.count ?? resource.tons) +
          " т</span>",
      )
      .join("");
    const siteFinds = (target.siteFinds || [])
      .slice(0, 3)
      .map(
        (site) =>
          '<div class="tiny muted">' +
          h(site.site || "Участок не указан") +
          " · " +
          h(
            (site.resources || [])
              .map(
                (resource) =>
                  (resource.name || resource.key) + " " + n(resource.count ?? resource.tons) + " т",
              )
              .join(", "),
          ) +
          (site.timestamp ? " · " + dateTime(site.timestamp) : "") +
          "</div>",
      )
      .join("");
    const observedMarkup =
      observed || siteFinds
        ? '<div class="target-observed"><small>ФАКТИЧЕСКАЯ ДОБЫЧА</small><div class="resource-chips">' +
          observed +
          "</div>" +
          siteFinds +
          '<p class="tiny muted">Результаты переработки на этом теле. Состав и объём остальных залежей требуют разведки.</p></div>'
        : "";
    return (
      '<article class="target-card"><div class="target-card-main"><div class="target-card-top"><span class="pill-row">' +
      U.confidence(target) +
      U.tier(target) +
      '</span><span class="tiny muted">' +
      h(sourceNames[target.source] || target.source || "") +
      (target.updatedAt ? " · " + dateTime(target.updatedAt) : "") +
      '</span></div><h2><span class="target-type-icon" aria-hidden="true">' +
      (target.kind === "ring" ? "◎" : "◒") +
      "</span>" +
      h(targetTitle(target)) +
      '</h2><p class="target-meta">' +
      h(target.system) +
      (target.body && target.ring ? " · " + h(target.body) : "") +
      "</p>" +
      details +
      targetResourceChips(target) +
      observedMarkup +
      '<div class="method-note">' +
      h((target.methods || []).map((method) => methods[method] || method).join(" · ")) +
      '</div><p class="target-explanation">' +
      h(recommendations.join(" ")) +
      "</p></div>" +
      '<aside class="target-card-aside"><span class="target-distance">' +
      n(target.distanceLy) +
      " <small>св. г.</small></span>" +
      '<span class="target-score">Оценка ' +
      n(target.score, true) +
      "</span>" +
      targetActions(target) +
      "</aside></article>"
    );
  }

  function renderTargets() {
    const search = state.search || {};
    const targets = filteredTargets();
    const resources = (state.resources || []).filter((resource) => resource.kind === "commodity");
    return (
      heading(
        "РАЗВЕДКА И ПОИСК",
        "Места добычи",
        "Подтверждённые находки и кандидаты из ваших сканов и открытых каталогов.",
      ) +
      '<form class="search-strip" id="search-form">' +
      field(
        "Система отсчёта",
        '<input name="system" data-focus="search-system" value="' +
          h(ui.searchSystem) +
          '" placeholder="' +
          h(state.commander?.system || "Текущая система или название") +
          '">',
        "field-grow",
      ) +
      field(
        "Радиус, св. г.",
        '<input name="radiusLy" data-focus="search-radius" type="number" min="1" max="1000" value="' +
          h(ui.radiusLy) +
          '">',
      ) +
      field(
        "Ресурс",
        '<select name="resourceKey" data-focus="search-resource">' +
          options(
            {
              "": "Все ресурсы",
              ...Object.fromEntries(resources.map((resource) => [resource.key, resource.name])),
            },
            ui.targetResource,
          ) +
          "</select>",
      ) +
      '<button class="button button-primary" type="submit"' +
      (search.running ? " disabled" : "") +
      ">" +
      (search.running ? "Поиск…" : "⌖ Искать рядом") +
      "</button>" +
      (search.running ? button("Остановить", "cancel-search", "", "button-subtle") : "") +
      "</form>" +
      (search.running || search.status || search.error
        ? '<p class="search-summary">' +
          h(search.error || search.status || "Проверяем системы рядом") +
          (search.checked
            ? " · проверено " + n(search.checked, true) + (search.total ? " из " + n(search.total, true) : "")
            : "") +
          "</p>"
        : "") +
      '<div class="filters">' +
      field(
        "Поиск в списке",
        '<input data-focus="target-query" data-control="targetQuery" value="' +
          h(ui.targetQuery) +
          '" placeholder="Система, тело, ресурс">',
        "field-grow",
      ) +
      field(
        "Где добывать",
        '<select data-control="targetKind" data-focus="target-kind">' +
          options(
            { all: "Кольца и поверхность", ring: "Кольца", surface: "Поверхность", raw: "Сырьё для синтеза" },
            ui.targetKind,
          ) +
          "</select>",
      ) +
      field(
        "Способ",
        '<select data-control="method" data-focus="target-method">' +
          options(methods, ui.method) +
          "</select>",
      ) +
      field(
        "Приоритет",
        '<select data-control="minTier" data-focus="target-tier">' +
          options({ medium: "Средний и высокий", high: "Высокий", low: "Любой" }, ui.minTier) +
          "</select>",
      ) +
      field(
        "Сортировка",
        '<select data-control="targetSort" data-focus="target-sort">' +
          options({ score: "По оценке", distance: "По расстоянию" }, ui.targetSort) +
          "</select>",
      ) +
      "</div>" +
      '<div class="row row-between" style="margin-bottom:15px"><span class="count-label">Показано ' +
      n(targets.length, true) +
      " из " +
      n(state.targets?.length || 0, true) +
      '</span><div class="legend"><span><i></i>Подтверждённые данные</span><span class="potential"><i></i>Нужна разведка</span></div></div>' +
      (targets.length
        ? '<div class="target-list">' + targets.map(targetCard).join("") + "</div>"
        : panel(
            "Результаты",
            empty(
              "По выбранным условиям целей нет",
              "Расширьте фильтры, увеличьте радиус поиска или просканируйте новые тела. Подтверждённые места с неизвестным составом остаются в списке среднего приоритета.",
            ),
          )) +
      '<p class="panel-note" style="margin-top:18px">Открытые каталоги могут быть неполными. Горячая точка не гарантирует конкретный процент руды в каждом астероиде. Состав планетарных залежей уточняется на месте.</p>'
    );
  }

  function sourceLinks(sources) {
    return (
      '<div class="source-links">' +
      (sources || [])
        .map((source) => {
          const url = U.safeUrl(source.url);
          return url
            ? '<a href="' +
                h(url) +
                '" target="_blank" rel="noopener noreferrer">' +
                h(source.title || "Источник") +
                " ↗</a>"
            : "";
        })
        .join("") +
      "</div>"
    );
  }

  function resourceCard(resource) {
    const tips = (resource.tips || []).map((tip) => "<p>" + h(tip) + "</p>").join("");
    const availability = (resource.availability || [])
      .map(
        (entry) =>
          '<div class="availability-row"><strong>' +
          h(methods[entry.method] || entry.method) +
          "</strong><span>" +
          h(
            (entry.ringTypes || []).map((type) => U.ringTypes[type] || type).join(" · ") ||
              (entry.method === "surface-planet"
                ? "Планетарные места добычи"
                : "Поверхность и фрагменты астероидов"),
          ) +
          "</span></div>",
      )
      .join("");
    const value =
      resource.kind === "raw"
        ? '<p class="panel-note">Отдельный запас материалов. В товарном трюме места не занимает.</p>'
        : '<div class="row row-between"><span class="small muted">Справочная цена</span><strong class="text-accent numeric">' +
          credits(resource.price > 0 ? resource.price : null) +
          '</strong></div><p class="tiny">' +
          h(resource.priceSource || "Цена не задана") +
          (resource.priceAt ? " · " + dateTime(resource.priceAt) : "") +
          "</p>";
    return (
      '<article class="resource-card"><div class="resource-card-header"><div><h2>' +
      h(resource.name) +
      "</h2><small>" +
      h(resource.english || resource.key) +
      '</small></div><span class="resource-symbol">' +
      h(resource.kind === "raw" ? resource.english?.slice(0, 2) || "R" : "◇") +
      '</span></div><div class="resource-card-body"><div class="pill-row">' +
      U.tier(resource) +
      pill(resource.kind === "raw" ? "Материал · класс " + n(resource.grade, true) : "Товар", "muted") +
      "</div><p>" +
      h(resource.description || "Сведения о ресурсе из справочника.") +
      '</p><div class="resource-facts">' +
      (resource.methods || []).map((method) => pill(methods[method] || method, "blue")).join("") +
      '</div><div class="availability-list">' +
      availability +
      '</div><hr class="subtle-divider">' +
      value +
      sourceLinks(resource.sources) +
      "</div>" +
      (tips ? "<details><summary>Подсказки по добыче</summary>" + tips + "</details>" : "") +
      "</article>"
    );
  }

  function renderEncyclopedia() {
    const query = ui.resourceQuery.toLocaleLowerCase("ru-RU").replaceAll("ё", "е").trim();
    const resources = (state.resources || []).filter((resource) => {
      if (ui.resourceKind !== "all" && resource.kind !== ui.resourceKind) return false;
      const abbreviation = (resource.english || "")
        .split(/\s+/)
        .map((word) => word[0] || "")
        .join("")
        .toLowerCase();
      return (
        !query ||
        (abbreviation.length > 1 && query === abbreviation) ||
        [resource.name, resource.english, resource.key, ...(resource.aliases || [])]
          .join(" ")
          .toLocaleLowerCase("ru-RU")
          .replaceAll("ё", "е")
          .includes(query)
      );
    });
    return (
      heading(
        "СПРАВОЧНИК ШАХТЁРА",
        "Ресурсы и способы добычи",
        "Названия на русском и английском, условия, инструменты и источники. Цена служит ориентиром для сравнения.",
      ) +
      '<div class="filters">' +
      field(
        "Найти ресурс",
        '<input data-control="resourceQuery" data-focus="resource-query" value="' +
          h(ui.resourceQuery) +
          '" placeholder="Платина, osmium, железо…">',
        "field-grow",
      ) +
      field(
        "Раздел",
        '<select data-control="resourceKind" data-focus="resource-kind">' +
          options(
            { all: "Все ресурсы", commodity: "Товары для продажи", raw: "Инженерные материалы" },
            ui.resourceKind,
          ) +
          "</select>",
      ) +
      '<span class="count-label">' +
      n(resources.length, true) +
      " ресурсов</span></div>" +
      '<div class="info-strip"><span aria-hidden="true">ⓘ</span><div>Ценность и редкость — ориентиры справочника. Для товаров фактическую выручку определяет рынок. Сырьё для синтеза и инженеров хранится в отдельном запасе и не занимает трюм.</div></div>' +
      (resources.length
        ? '<div class="catalog-grid">' + resources.map(resourceCard).join("") + "</div>"
        : empty(
            "Ресурс не найден",
            "Попробуйте русское или английское название и проверьте выбранный раздел.",
          ))
    );
  }

  function eventName(kind) {
    return (
      {
        refined: "Переработка руды",
        MiningRefined: "Переработка руды",
        material: "Сбор сырья",
        MaterialCollected: "Сбор сырья",
        prospect: "Разведка астероида",
        ProspectedAsteroid: "Разведка астероида",
        sale: "Продажа",
        MarketSell: "Продажа",
        cracked: "Раскол астероида",
        AsteroidCracked: "Раскол астероида",
      }[kind] ||
      kind ||
      "Событие"
    );
  }

  function renderStatistics() {
    const stats = state.stats || {};
    const total = stats.total || {};
    const rows = stats.byResource || [];
    const sessions = stats.sessions || [];
    const resourceTable = rows.length
      ? '<div class="table-wrap"><table><thead><tr><th>Ресурс</th><th class="table-num">Переработано, т</th><th class="table-num">Сырьё, ед.</th><th class="table-num">Продано, т</th><th class="table-num">Продажи</th></tr></thead><tbody>' +
        rows
          .map(
            (row) =>
              '<tr><td class="table-name">' +
              h(row.name || row.key) +
              '</td><td class="table-num">' +
              n(row.refinedTons || 0, true) +
              '</td><td class="table-num">' +
              n(row.materialUnits || 0, true) +
              '</td><td class="table-num">' +
              n(row.soldTons || 0, true) +
              '</td><td class="table-num">' +
              credits(row.revenue || 0) +
              "</td></tr>",
          )
          .join("") +
        "</tbody></table></div>"
      : empty(
          "Учёт начнётся с первого события",
          "Данные собираются из записей добычи, сбора сырья и продаж в игровых журналах.",
        );
    const sessionTable = sessions.length
      ? '<div class="table-wrap"><table><thead><tr><th>Дата</th><th class="table-num">Переработано, т</th><th class="table-num">Сырьё, ед.</th><th class="table-num">Разведано</th><th class="table-num">Продажи</th></tr></thead><tbody>' +
        sessions
          .map(
            (session) =>
              "<tr><td>" +
              dateTime(session.date, true) +
              '</td><td class="table-num">' +
              n(session.refinedTons || 0, true) +
              '</td><td class="table-num">' +
              n(session.materialUnits || 0, true) +
              '</td><td class="table-num">' +
              n(session.prospected || 0, true) +
              '</td><td class="table-num">' +
              credits(session.revenue || 0) +
              "</td></tr>",
          )
          .join("") +
        "</tbody></table></div>"
      : empty("История пока пуста", "Результаты группируются по датам событий журнала.");
    return (
      heading(
        "ФАКТИЧЕСКИЕ РЕЗУЛЬТАТЫ",
        "Статистика добычи",
        "С " + dateTime(stats.since) + ". Тонны руды, инженерные материалы и продажи учитываются раздельно.",
        '<a class="button button-subtle" href="/api/export.csv" download>↓ Экспорт CSV</a>' +
          button("Новый период", "reset-session", "", "button-subtle"),
      ) +
      '<div class="metrics">' +
      metric(
        "Переработано",
        n(total.refinedTons || 0, true) + " <small>т</small>",
        "Разведано астероидов: " + n(total.prospected || 0, true),
      ) +
      metric(
        "Собрано сырья",
        n(total.materialUnits || 0, true) + " <small>ед.</small>",
        "Материалы для синтеза и инженеров",
      ) +
      metric(
        "Продажи ресурсов",
        credits(total.revenue || 0),
        "Всего продано " + n(total.soldTons || 0, true) + " т",
      ) +
      metric("Продажи своей добычи", credits(total.miningRevenue || 0), "Сопоставлены с MiningRefined") +
      '</div><div class="info-strip"><span aria-hidden="true">ⓘ</span><div>«Продажи ресурсов» включают добываемые товары, которые могли быть куплены раньше. «Продажи своей добычи» учитывают только объём, связанный с известной переработкой руды. Период учёта: ' +
      duration(stats.elapsedSeconds) +
      ". Расходы на снаряжение, дронов и перелёты могут учитываться отдельно в игре.</div></div>" +
      '<div class="stack">' +
      panel("По ресурсам", resourceTable) +
      panel("По датам", sessionTable) +
      panel(
        "Разведка",
        '<div class="panel-body"><div class="summary-grid" style="margin-top:0"><div><small>Высокий приоритет</small><strong>' +
          n(total.accepted || 0, true) +
          "</strong></div><div><small>Низкий приоритет</small><strong>" +
          n(total.rejected || 0, true) +
          "</strong></div><div><small>Расколото астероидов</small><strong>" +
          n(total.asteroidsCracked || 0, true) +
          "</strong></div></div></div>",
      ) +
      "</div>"
    );
  }

  function renderRoute() {
    const bookmarks = state.bookmarks || [];
    const rows = bookmarks.length
      ? '<ol class="route-list">' +
        bookmarks
          .map((bookmark, index) => {
            const target = bookmark.target;
            return (
              '<li class="route-item"><span class="route-number">' +
              n(index + 1, true) +
              "</span><div><h2>" +
              h(bookmark.system || bookmark.label) +
              "</h2><small>" +
              h(bookmark.ring || bookmark.body || bookmark.label) +
              (target?.distanceLy != null
                ? " · " + n(target.distanceLy) + " св. г. от текущего положения"
                : "") +
              '</small><div class="pill-row" style="margin-top:8px">' +
              (target ? U.confidence(target) + U.tier(target) : pill("Сохранённое место", "muted")) +
              '</div></div><div class="actions-wrap">' +
              button("Копировать систему", "copy", bookmark.system, "button-small button-subtle") +
              button("Убрать", "remove-bookmark", bookmark.id, "button-small button-quiet") +
              "</div></li>"
            );
          })
          .join("") +
        "</ol>"
      : empty(
          "Выберите первую цель",
          "В разделе «Цели» добавьте интересующие кольца и места добычи. Системы можно копировать для прокладки маршрута в игре.",
          '<a class="button button-subtle" href="#targets">Выбрать цели</a>',
        );
    return (
      heading(
        "ПЛАН ВЫЛЕТА",
        "Сохранённые места",
        "Ваш список целей для добычи. Перед перелётом проверьте запас топлива, дальность прыжка и снаряжение.",
        bookmarks.length ? button("Копировать системы", "copy-route", "", "button-subtle") : "",
      ) +
      '<div class="info-strip"><span aria-hidden="true">⌁</span><div>Список хранит выбранные места в порядке добавления. Кнопка копирует название системы для поиска в карте галактики игры.</div></div>' +
      panel("Цели вылета", rows, "<small>" + n(bookmarks.length, true) + " СОХРАНЕНО</small>")
    );
  }

  function settingsValue(key, fallback = "") {
    return ui.settingsDraft[key] ?? state.settings?.[key] ?? fallback;
  }

  function settingsCheckbox(key, label, description) {
    return (
      '<label class="switch-row"><span><p>' +
      h(label) +
      "</p><small>" +
      h(description) +
      '</small></span><input name="' +
      h(key) +
      '" type="checkbox"' +
      (settingsValue(key, false) ? " checked" : "") +
      "></label>"
    );
  }

  function renderSettings() {
    const journalDir = state.connection?.journalDir || "";
    const resourcePrices = (state.resources || []).filter((resource) => resource.kind === "commodity");
    const prices = resourcePrices
      .map(
        (resource) =>
          "<tr><td>" +
          h(resource.name) +
          '</td><td class="table-num"><input class="price-input" type="number" min="0" step="1" data-price-key="' +
          h(resource.key) +
          '" data-focus="price-' +
          h(resource.key) +
          '" value="' +
          h(ui.priceDraft[resource.key] ?? resource.price ?? "") +
          '" placeholder="Не задана" aria-label="Справочная цена ' +
          h(resource.name) +
          '"></td><td class="small muted">' +
          h(resource.priceSource || "—") +
          (resource.priceAt ? " · " + dateTime(resource.priceAt) : "") +
          "</td></tr>",
      )
      .join("");
    return (
      heading(
        "ЛОКАЛЬНЫЕ НАСТРОЙКИ",
        "Параметры компаньона",
        "Папка журналов, способы добычи, фильтры и справочные цены.",
      ) +
      '<div class="settings-columns"><div class="stack">' +
      panel(
        "Игровые журналы",
        '<div class="panel-body"><form id="journal-form"><label class="field"><span>Папка Journal.*.log, Status.json и Cargo.json</span>' +
          '<input name="path" data-focus="journal-path" class="journal-path" value="' +
          h(ui.settingsDraft.journalDir ?? journalDir) +
          '" placeholder="C:\\Users\\…\\Saved Games\\Frontier Developments\\Elite Dangerous"></label>' +
          '<div class="settings-actions">' +
          button("Выбрать папку", "choose-journal", "", "button-subtle") +
          '<button class="button button-primary" type="submit">Применить</button></div></form><hr class="subtle-divider"><p class="settings-hint">' +
          "Прочитано файлов: " +
          n(state.connection?.filesRead || 0, true) +
          ". Последнее событие: " +
          dateTime(state.connection?.lastEvent) +
          ".</p></div>",
      ) +
      panel(
        "Приоритеты добычи",
        '<form id="settings-form"><div class="panel-body"><div class="form-grid">' +
          field(
            "Минимальный приоритет",
            '<select name="minTier" data-focus="settings-tier">' +
              options(
                { medium: "Средний и высокий", high: "Высокий", low: "Любой" },
                settingsValue("minTier", "medium"),
              ) +
              "</select>",
          ) +
          field(
            "Основной способ",
            '<select name="method" data-focus="settings-method">' +
              options(methods, settingsValue("method", "all")) +
              "</select>",
          ) +
          field(
            "Радиус поиска, св. г.",
            '<input name="radiusLy" data-focus="settings-radius" type="number" min="1" max="1000" value="' +
              h(settingsValue("radiusLy", 50)) +
              '">',
          ) +
          field(
            "Макс. расстояние от звезды, св. с.",
            '<input name="maxArrivalLs" data-focus="settings-arrival" type="number" min="0" max="10000000" value="' +
              h(settingsValue("maxArrivalLs", 10000)) +
              '">',
          ) +
          field(
            "Мин. содержание для лазера, %",
            '<input name="minLaserPercent" data-focus="settings-percent" type="number" min="0" max="100" step="1" value="' +
              h(settingsValue("minLaserPercent", 15)) +
              '">',
          ) +
          '</div><hr class="subtle-divider">' +
          settingsCheckbox(
            "hideDepleted",
            "Скрывать истощённые кольца для лазерной добычи",
            "Для ядер такие кольца остаются доступными",
          ) +
          settingsCheckbox(
            "overlayEnabled",
            "Показывать оверлей",
            "Трюм, разведка и результаты поверх игры",
          ) +
          settingsCheckbox(
            "hideUnfocused",
            "Скрывать оверлей при переключении из игры",
            "Возвращать при активации Elite Dangerous",
          ) +
          '<div class="settings-actions"><button class="button button-primary" type="submit">Сохранить настройки</button></div></div></form>',
      ) +
      '</div><aside class="stack">' +
      panel(
        "Источники и точность",
        '<div class="panel-body"><p class="panel-note">Сканирование тел, горячие точки, текущая руда и результаты добычи читаются из локального журнала игры. Поиск рядом дополняет их открытыми каталогами.</p>' +
          '<p class="panel-note">Цена, количество товара и состав астероида — разные показатели. Пустая цена означает, что данных для денежной оценки пока нет.</p>' +
          sourceLinks(state.app?.sources) +
          "</div>",
      ) +
      panel(
        "Управление",
        '<div class="panel-body"><div class="row row-between"><span class="small">Показать / скрыть оверлей</span><span class="mini-key">Ctrl Alt H</span></div>' +
          '<p class="settings-hint" style="margin-top:12px">Оверлей пропускает клики в игру. Его положение подстраивается под режим корабля или высадки.</p>' +
          '<hr class="subtle-divider">' +
          button("Перечитать журналы", "rescan", "", "button-subtle") +
          "</div>",
      ) +
      "</aside></div>" +
      '<section class="panel settings-prices"><div class="panel-heading"><h2>Справочные цены товаров</h2><small>ЗА ТОННУ</small></div>' +
      '<div class="panel-body"><p class="panel-note">Введите ориентир для оценки груза и сравнения состава астероидов. Фактическая цена зависит от станции и времени продажи.</p>' +
      (state.market?.station
        ? '<p class="panel-note">Последний рынок: ' +
          h(state.market.station) +
          " · " +
          h(state.market.system) +
          " · " +
          dateTime(state.market.timestamp) +
          "</p>"
        : "") +
      '</div><form id="prices-form"><div class="table-wrap"><table><thead><tr><th>Ресурс</th><th class="table-num">Цена, кр.</th><th>Источник / дата</th></tr></thead><tbody>' +
      prices +
      '</tbody></table></div><div class="panel-body"><button class="button button-primary" type="submit">Сохранить цены</button></div></form></section>'
    );
  }

  function render() {
    if (!state || composing) return;
    const active = document.activeElement;
    const focus = active?.getAttribute("data-focus");
    const selection = active?.selectionStart == null ? null : [active.selectionStart, active.selectionEnd];
    const expanded = new Set([...content.querySelectorAll('details[data-preserve-open][open]')].map((entry) => entry.dataset.preserveOpen));
    const renderers = {
      overview: () => window.CopilotOverview.render(state),
      targets: () => window.CopilotTargets.render(state),
      sale: () => window.MiningOperations.renderMarket(state),
      deals: () => window.MiningExpedition.renderTradeRoutes(state),
      carrier: () => window.MiningOperations.renderCarrier(state),
      encyclopedia: () => window.CopilotEncyclopedia ? window.CopilotEncyclopedia.render(state) : renderEncyclopedia(),
      statistics: renderStatistics,
      mining: () => window.MiningPlannerUI.render(state),
      engineering: () => window.MiningEngineering.render(state),
      help: () => window.MiningHelp.render(state),
      route: () => window.MiningExpedition.renderRoute(state) + '<details class="expedition-details"><summary>Сохранённые места добычи</summary>' + renderRoute() + '</details>',
      account: () => window.MiningExpedition.renderAccount(state),
      integrations: () => window.CopilotIntegrations.render(state),
      settings: renderSettings,
    };
    content.innerHTML = renderers[ui.tab]();
    const helpButton = document.getElementById("context-help");
    helpButton.dataset.helpTopic = window.MiningHelp.topicForTab[ui.tab] || "start";
    helpButton.textContent = ui.tab === "help" ? "? Начало работы" : "? Помощь по разделу";
    for (const entry of content.querySelectorAll('details[data-preserve-open]')) entry.open = expanded.has(entry.dataset.preserveOpen);
    for (const link of document.querySelectorAll("[data-tab]")) {
      link.classList.toggle("is-active", link.dataset.tab === ui.tab);
      if (link.dataset.tab === ui.tab) link.setAttribute("aria-current", "page");
      else link.removeAttribute("aria-current");
    }
    if (focus) {
      const replacement = [...content.querySelectorAll("[data-focus]")].find(
        (input) => input.dataset.focus === focus,
      );
      if (replacement) {
        replacement.focus({ preventScroll: true });
        if (selection && replacement.setSelectionRange && !["number", "range"].includes(replacement.type)) {
          replacement.setSelectionRange(...selection);
        }
      }
    }
  }

  function updateState(next) {
    state = next;
    if (!initialSettings && next.settings) {
      ui.minTier = next.settings.minTier || "medium";
      ui.method = next.settings.method || "all";
      ui.radiusLy = next.settings.radiusLy || 50;
      initialSettings = true;
    }
    const commander = state.commander || {};
    const activity = state.settings?.activity || "asteroid";
    document.getElementById("activity-select").value = activity;
    document.getElementById("activity-description").textContent = {
      flight: "В оверлее курс корабля, топливо, поручения и личные цели.",
      asteroid: "В оверлее состав астероида, трюм и подходящее кольцо.",
      surface: "В оверлее участок, переработанные ресурсы и трюм ТРП.",
      raw: "В оверлее собранное сырьё, категории и запас материалов.",
      trade: "В оверлее выбранный рынок, спрос, цена и путь с грузом.",
      carrier: "В оверлее топливо, средства, свободный склад и прыжок.",
      engineering: "В оверлее недостающие материалы для запланированных улучшений.",
    }[activity] || "";
    document.getElementById("header-context").innerHTML =
      "<strong>" +
      h(commander.system || "Ожидание положения") +
      "</strong><small>" +
      h(commander.name ? "CMDR " + commander.name : "Командир ещё не определён") +
      (commander.ship ? " · " + h(commander.ship) : "") +
      "</small>";
    const status = state.connection || {};
    const banner = document.getElementById("status-banner");
    const text =
      status.error ||
      (status.scanning
        ? "Читаем журналы: " + n(status.filesRead || 0, true) + " / " + n(status.filesTotal || 0, true)
        : !status.filesRead
          ? "Игровые журналы пока не найдены. Выберите папку в настройках."
          : "");
    banner.textContent = text;
    banner.hidden = !text;
    window.CopilotOverlayControl.update();
    render();
  }

  function showToast(message, error = false) {
    const toast = document.getElementById("toast");
    toast.textContent = message;
    toast.classList.toggle("is-error", error);
    toast.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(
      () => {
        toast.hidden = true;
      },
      error ? 7000 : 3500,
    );
  }

  async function perform(path, data, message) {
    try {
      const result = await U.api(path, data);
      if (result.state) updateState(result.state);
      else await subscription.refresh();
      if (message) showToast(message);
      return result;
    } catch (error) {
      showToast(error.message, true);
      return null;
    }
  }

  document.addEventListener("click", async (event) => {
    const action = event.target.closest("[data-action]");
    if (!action) return;
    const value = action.dataset.value;
    if (action.dataset.action === "copy") {
      await U.copyText(value).then(
        () => showToast("Название системы скопировано"),
        (error) => showToast(error.message, true),
      );
    } else if (action.dataset.action === "bookmark") {
      if ((state.bookmarks || []).some((bookmark) => bookmark.targetId === value))
        return showToast("Эта цель уже в маршруте");
      await perform("/api/bookmarks", { action: "add", targetId: value }, "Место добавлено в маршрут");
    } else if (action.dataset.action === "remove-bookmark") {
      await perform("/api/bookmarks", { action: "remove", id: value }, "Место убрано из маршрута");
    } else if (action.dataset.action === "copy-route") {
      const systems = [
        ...new Set((state.bookmarks || []).map((bookmark) => bookmark.system).filter(Boolean)),
      ];
      await U.copyText(systems.join("\n")).then(
        () => showToast("Системы маршрута скопированы"),
        (error) => showToast(error.message, true),
      );
    } else if (action.dataset.action === "rescan") {
      await perform("/api/rescan", {}, "Чтение журналов запущено");
    } else if (action.dataset.action === "cancel-search") {
      await perform("/api/search/cancel", {}, "Поиск остановлен");
    } else if (action.dataset.action === "reset-session") {
      if (
        window.confirm(
          "Начать новый период статистики? Текущий период будет завершён, игровые журналы сохранятся.",
        )
      ) {
        await perform("/api/session", { action: "reset" }, "Начат новый период учёта");
      }
    } else if (action.dataset.action === "choose-journal") {
      if (!window.edMining?.chooseJournalDir) return showToast("Введите полный путь к папке журналов в поле");
      const selected = await window.edMining.chooseJournalDir();
      if (selected) {
        ui.settingsDraft.journalDir = selected;
        render();
      }
    }
  });

  document.addEventListener("input", (event) => {
    const input = event.target;
    if (input.dataset.priceKey) ui.priceDraft[input.dataset.priceKey] = input.value;
    if (input.closest("#journal-form") && input.name === "path") ui.settingsDraft.journalDir = input.value;
    if (input.closest("#search-form")) {
      if (input.name === "system") ui.searchSystem = input.value;
      if (input.name === "radiusLy") ui.radiusLy = input.value;
    }
    if (input.closest("#settings-form") && input.name) {
      ui.settingsDraft[input.name] = input.type === "checkbox" ? input.checked : input.value;
    }
    if (input.dataset.control) {
      ui[input.dataset.control] = input.value;
      if (input.tagName !== "SELECT") render();
    }
  });

  document.addEventListener("change", (event) => {
    const input = event.target;
    if (input.dataset.control) {
      ui[input.dataset.control] = input.value;
      if (["method", "minTier"].includes(input.dataset.control)) {
        perform("/api/settings", { [input.dataset.control]: input.value });
      }
      render();
    }
    if (input.closest("#search-form") && input.name === "resourceKey") ui.targetResource = input.value;
    if (input.closest("#settings-form") && input.name)
      ui.settingsDraft[input.name] = input.type === "checkbox" ? input.checked : input.value;
  });

  document.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.target;
    const values = new FormData(form);
    if (form.id === "search-form") {
      ui.searchSystem = String(values.get("system") || "").trim();
      ui.radiusLy = Number(values.get("radiusLy")) || 50;
      ui.targetResource = String(values.get("resourceKey") || "");
      await perform("/api/search", {
        system: ui.searchSystem || state.commander?.system || "",
        radiusLy: ui.radiusLy,
        resourceKey: ui.targetResource,
      });
    } else if (form.id === "journal-form") {
      const directory = String(values.get("path") || "").trim();
      if (!directory) return showToast("Укажите папку игровых журналов", true);
      const result = await perform("/api/journal-dir", { path: directory }, "Папка журналов применена");
      if (result) delete ui.settingsDraft.journalDir;
    } else if (form.id === "settings-form") {
      const settings = {
        minTier: values.get("minTier"),
        method: values.get("method"),
        radiusLy: Number(values.get("radiusLy")),
        maxArrivalLs: Number(values.get("maxArrivalLs")),
        minLaserPercent: Number(values.get("minLaserPercent")),
        hideDepleted: values.has("hideDepleted"),
        overlayEnabled: values.has("overlayEnabled"),
        hideUnfocused: values.has("hideUnfocused"),
      };
      const result = await perform("/api/settings", settings, "Настройки сохранены");
      if (result) {
        ui.settingsDraft = {};
        ui.minTier = settings.minTier;
        ui.method = settings.method;
        ui.radiusLy = settings.radiusLy;
        window.edMining?.setOverlay?.(settings.overlayEnabled);
      }
    } else if (form.id === "prices-form") {
      const prices = [...form.querySelectorAll("[data-price-key]")]
        .filter((input) => input.value.trim() !== "")
        .map((input) => ({ key: input.dataset.priceKey, price: Number(input.value) }));
      if (prices.some((row) => !Number.isFinite(row.price) || row.price < 0))
        return showToast("Цены должны быть положительными числами или нулём", true);
      const result = await perform("/api/market", { prices }, "Справочные цены сохранены");
      if (result) ui.priceDraft = {};
    }
  });

  document.getElementById("activity-select").addEventListener("change", async (event) => {
    await perform("/api/settings", { activity: event.target.value }, "Рабочий режим изменён");
  });

  window.addEventListener("hashchange", () => {
    const next = location.hash.slice(1);
    ui.tab = tabs.includes(next) ? next : "overview";
    render();
    window.scrollTo(0, 0);
  });
  document.addEventListener("compositionstart", () => {
    composing = true;
  });
  document.addEventListener("compositionend", () => {
    composing = false;
    render();
  });
  document.addEventListener("click", (event) => {
    const anchor = event.target.closest('a[target="_blank"]');
    if (anchor && window.edMining?.openExternal) {
      event.preventDefault();
      window.edMining.openExternal(anchor.href).catch((error) => showToast(error.message, true));
    }
  });

  window.MiningOperations.init({ getState: () => state, render, perform, showToast });
  window.MiningExpedition.init({ getState: () => state, render, perform, showToast });
  window.MiningEngineering.init({ getState: () => state, render, perform, showToast });
  window.MiningHelp.init({ getState: () => state, render, showToast });
  window.CopilotOverlayControl.init({ getState: () => state, perform, showToast });
  window.CopilotOverview.init({ getState: () => state, render, perform, showToast });
  window.CopilotTargets.init({ getState: () => state, render, perform, showToast, renderMiningTargets: renderTargets });
  window.CopilotEncyclopedia.init({ getState: () => state, render, perform, showToast });
  window.CopilotIntegrations.init({ getState: () => state, render, perform, showToast });
  const subscription = U.subscribe(updateState, (connected, message) => {
    connection = connected;
    const indicator = document.getElementById("connection");
    indicator.className = "connection " + (connection ? "is-live" : "is-offline");
    indicator.innerHTML = "<i></i>" + h(connection ? "Журнал подключён" : "Нет соединения");
    indicator.title = message || "Обновления поступают автоматически";
  });
  window.addEventListener("beforeunload", () => subscription.close());
})();
