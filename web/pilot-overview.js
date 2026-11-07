(function () {
  "use strict";

  const { h, n, credits, dateTime, duration, pill } = window.MiningUI;
  let context;
  let initialized = false;
  const finite = (value) => value != null && value !== "" && Number.isFinite(Number(value)) ? Number(value) : null;
  const time = (value) => Number.isFinite(Date.parse(value)) ? Date.parse(value) : null;
  const array = (value) => Array.isArray(value) ? value : [];
  const link = (label, tab, extra = "") => '<a class="button button-small button-subtle" href="#' + tab + '" ' + extra + '>' + h(label) + '</a>';
  const action = (label, key, value = "", style = "button-subtle") => '<button type="button" class="button button-small ' + style + '" data-pilot-action="' + key + '" data-value="' + h(value) + '">' + h(label) + '</button>';
  const panel = (title, body, right = "") => '<section class="panel"><div class="panel-heading"><h2>' + h(title) + '</h2>' + right + '</div>' + body + '</section>';
  const metric = (label, value, note) => '<div class="metric"><div class="metric-label">' + h(label) + '</div><div class="metric-value numeric">' + value + '</div><div class="metric-note">' + h(note || "") + '</div></div>';
  const kv = (label, value) => '<div class="pilot-kv"><span>' + h(label) + '</span><strong>' + value + '</strong></div>';
  const note = (text) => '<p class="panel-note">' + h(text) + '</p>';

  function balance(state) {
    const pilot = state.pilot || {};
    const api = state.frontier?.summary?.profile;
    const journal = { value: finite(pilot.credits), timestamp: pilot.creditsAt, source: pilot.creditsSource || "Журнал игры" };
    if (finite(api?.credits) != null && (journal.value == null || (time(state.frontier?.lastSync) ?? 0) >= (time(journal.timestamp) ?? 0))) {
      return { value: Number(api.credits), timestamp: state.frontier.lastSync, source: "Frontier CAPI" };
    }
    return journal;
  }

  function planCard({ category, title, detail, facts = [], actions = "", warning = "", symbol = "⌖" }) {
    return '<article class="pilot-plan-card"><div class="pilot-plan-symbol" aria-hidden="true">' + h(symbol) + '</div><div class="pilot-plan-content"><small>' + h(category) + '</small><h3>' + h(title) + '</h3><p>' + h(detail || "") + '</p>' +
      (facts.length ? '<div class="pill-row">' + facts.map(([text, style]) => pill(text, style || "muted")).join("") + '</div>' : "") +
      (warning ? '<p class="pilot-plan-warning">' + h(warning) + '</p>' : "") +
      (actions ? '<div class="actions-wrap">' + actions + '</div>' : "") + '</div></article>';
  }

  function plans(state) {
    const rows = [];
    const colony = state.colonization?.activeProject;
    if (colony && !["complete", "cancelled"].includes(colony.status)) rows.push(planCard({
      category: "КОЛОНИЗАЦИЯ", title: colony.targetSystem || colony.name,
      detail: colony.faction || "Строительство и снабжение новой системы",
      facts: [["Объектов в плане: " + n(array(colony.buildings).length, true)]],
      actions: link("Строительство и поставки", "colonization"), symbol: "⌂",
    }));
    const pilot = state.pilot || {}, commander = state.commander || {};
    const carrier = state.carrier || {}, route = state.carrierRoute || {};
    const pending = carrier.jump?.pending;
    const next = array(route.route?.stops).find((stop) => !stop.visited && stop.index > 0);
    if (pending || next) {
      const remaining = pending?.departureMs != null ? Math.max(0, (pending.departureMs - Date.now()) / 1000) : null;
      rows.push(planCard({ category: "ФЛОТОНОСЕЦ", title: pending?.system || next?.name || "План перелёта",
        detail: pending ? remaining > 0 ? "Назначенный прыжок: до отправления " + duration(remaining) : "Ожидается подтверждение отправления или прибытия" : "Следующая система сохранённого маршрута",
        facts: [
          ...(route.summary?.remainingJumps != null ? [["Осталось прыжков: " + n(route.summary.remainingJumps, true)]] : []),
          ...(next?.fuelUsedTons != null ? [["Следующий прыжок: " + n(next.fuelUsedTons) + " т трития"]] : []),
        ], warning: route.summary?.reserveShortfallTons > 0 ? "В плане не хватает " + n(route.summary.reserveShortfallTons) + " т до целевого резерва." : "",
        actions: link("Маршрут", "route") + link("Мой флотоносец", "carrier") + action("Копировать систему", "copy", pending?.system || next?.name || ""), symbol: "▱" }));
    }
    const activeMissions = array(pilot.missions).filter((mission) => !["completed", "failed", "abandoned", "cancelled"].includes(mission.status));
    if (activeMissions.length) {
      const ordered = [...activeMissions].sort((a, b) => (time(a.expiry) ?? Infinity) - (time(b.expiry) ?? Infinity));
      const first = ordered[0];
      rows.push(planCard({ category: "МИССИИ", title: first.name || "Принятая миссия",
        detail: [first.destinationSystem, first.destinationStation].filter(Boolean).join(" · ") || "Условия и назначение — по последним записям журнала",
        facts: [["Активных миссий: " + n(activeMissions.length, true), "blue"], ...(first.expiry ? [["Срок: " + dateTime(first.expiry)]] : [])],
        warning: time(first.expiry) != null && time(first.expiry) < Date.now() ? "Срок по журналу истёк. Проверьте текущий статус миссии в игре." : "",
        actions: action("Все миссии и цели", "targets", "plan") + (first.destinationSystem ? action("Копировать систему", "copy", first.destinationSystem) : ""), symbol: "▤" }));
    }
    const navTarget = pilot.navTarget || {};
    const remainingRoute = array(pilot.navRoute).filter((stop) => !stop.visited && stop.system !== commander.system);
    const nextShip = remainingRoute[0];
    if (navTarget.system && navTarget.system !== commander.system || nextShip) {
      const system = nextShip?.system || navTarget.system;
      rows.push(planCard({ category: "НАВИГАЦИЯ", title: system,
        detail: nextShip ? "Следующая система маршрута, проложенного в игре" : "Выбранная цель гиперпрыжка",
        facts: [...(navTarget.remainingJumps != null ? [["Осталось прыжков: " + n(navTarget.remainingJumps, true)]] : remainingRoute.length ? [["В маршруте ещё систем: " + n(remainingRoute.length, true)]] : [])],
        actions: action("Навигация и цели", "targets", "plan") + action("Копировать систему", "copy", system), symbol: "⌁" }));
    }
    const destination = state.trading?.destination;
    if (destination) rows.push(planCard({ category: "ТОРГОВЛЯ", title: destination.station || "Выбранный рынок",
      detail: [destination.system, destination.distanceLy != null ? n(destination.distanceLy) + " св. г." : null].filter(Boolean).join(" · "),
      facts: [["Котировка корзины: " + credits(destination.estimatedTotal), "blue"], ["Наблюдение: " + dateTime(destination.updatedAt)]],
      warning: destination.expired ? "Котировка старше выбранного ограничения. Обновите поиск перед перевозкой." : destination.cargoChanged ? "Состав трюма изменился после выбора рынка. Обновите расчёт груза." : "",
      actions: link("Проверить рынок", "sale") + (destination.system ? action("Копировать систему", "copy", destination.system) : ""), symbol: "⇄" }));
    const engineering = state.engineering || {}, shopping = engineering.shopping || {};
    if (array(engineering.items).length) {
      const missing = array(engineering.materials).filter((material) => material.needed > 0 && (material.missing > 0 || material.owned == null));
      rows.push(planCard({ category: "ИНЖЕНЕРИЯ", title: shopping.complete ? "Материалы для плана собраны" : "Подготовка корабельных улучшений",
        detail: missing.slice(0, 2).map((material) => material.name + (material.missing != null ? " — ещё " + n(material.missing, true) : " — запас неизвестен")).join(" · ") || "Проверьте доступ к нужному инженеру и оставшиеся применения чертежей",
        facts: [["Пунктов плана: " + n(engineering.items.length, true)], [shopping.missingUnits != null ? "Не хватает материалов: " + n(shopping.missingUnits, true) : "Нужен снимок материалов", shopping.complete ? "confirmed" : "potential"]],
        actions: link("Открыть план", "engineering") + action("Цели для инженерии", "targets", "engineering"), symbol: "⚒" }));
    }
    const priority = (value) => ({ high: 3, medium: 2, low: 1 }[value] ?? finite(value) ?? 0);
    const objectives = array(state.objectives || state.goals).filter((goal) => !["completed", "cancelled"].includes(goal.status))
      .sort((a, b) => priority(b.priority) - priority(a.priority));
    for (const goal of objectives.slice(0, 2)) rows.push(planCard({ category: "МОЯ ЦЕЛЬ", title: goal.title || "Личная задача",
      detail: [goal.system, goal.station || goal.body, goal.notes].filter(Boolean).join(" · "),
      facts: [], actions: action("Открыть цели", "targets", "plan") + (goal.system ? action("Копировать систему", "copy", goal.system) : ""), symbol: "⌖" }));
    if (state.tradePlan && !destination) rows.push(planCard({ category: "ТОРГОВЫЙ ПЛАН", title: state.tradePlan.name || "Закупка и продажа",
      detail: [state.tradePlan.source, state.tradePlan.destination].filter(Boolean).join(" → ") || "Расчёт с введёнными ценами",
      facts: [[n(state.tradePlan.quantity) + " т"], ["Плановая маржа: " + credits(state.tradePlan.grossMargin), "blue"]],
      actions: link("Открыть расчёт", "sale"), symbol: "⇄" }));
    const run = state.mining?.current;
    if (run) rows.push(planCard({ category: "ТЕКУЩАЯ ОПЕРАЦИЯ", title: "Идёт заход добычи",
      detail: [run.system, run.body].filter(Boolean).join(" · ") || "Статистика действий по журналу",
      facts: [["Переработано: " + n(run.refinedTons) + " т"], ["Темп: " + n(run.tonsPerHour) + " т/ч"]],
      actions: link("Результат захода", "mining"), symbol: "⛏" }));
    const bookmarks = array(state.bookmarks);
    if (bookmarks.length && rows.length < 6) rows.push(planCard({ category: "СОХРАНЁННЫЕ МЕСТА", title: "Места для следующих поездок",
      detail: bookmarks.slice(0, 2).map((entry) => [entry.system, entry.ring || entry.body || entry.label].filter(Boolean).join(" · ")).join("; "),
      facts: [["Сохранено: " + n(bookmarks.length, true)]], actions: action("Открыть места", "targets", "mining"), symbol: "⌖" }));
    return rows;
  }

  function renderPlans(state) {
    const cards = plans(state);
    return panel("Текущие планы и задачи", cards.length
      ? '<div class="pilot-plans">' + cards.slice(0, 6).join("") + '</div>' + (cards.length > 6 ? '<div class="panel-body">' + note("На обзоре показаны первые шесть планов. Остальные доступны в разделах приложения.") + action("Все цели", "targets", "plan") + '</div>' : "")
      : '<div class="pilot-no-plans"><span aria-hidden="true">⌖</span><h3>Куда отправимся дальше?</h3><p>Добавьте личную цель, примите миссию в игре или подготовьте торговый, инженерный либо экспедиционный план.</p><div class="actions-wrap">' + action("Мои цели", "targets", "plan", "button-primary") + link("Инженерия", "engineering") + link("Торговля", "sale") + '</div></div>',
      '<button type="button" class="soft-link pilot-soft-button" data-pilot-action="targets" data-value="plan">Все цели →</button>');
  }

  function cargoPanel(state) {
    const commander = state.commander || {}, cargo = array(commander.cargo).filter((item) => item.count > 0);
    return panel("На борту", '<div class="panel-body">' +
      (cargo.length ? '<ul class="pilot-cargo-list">' + cargo.slice(0, 5).map((item) => '<li><span>' + h(item.name || item.key) + '</span><strong>' + n(item.count) + ' т</strong></li>').join("") + '</ul>' + (cargo.length > 5 ? note("Ещё видов груза: " + n(cargo.length - 5, true)) : "") : note("Товарный груз по текущим данным отсутствует.")) +
      '<div class="actions-wrap">' + link("Найти рынок", "sale") + link("Учёт сделок", "sale") + '</div></div>');
  }

  function carrierPanel(state) {
    const carrier = state.carrier;
    if (!carrier?.owned) return "";
    return panel(carrier.name || "Мой флотоносец", '<div class="panel-body">' + note([carrier.callsign, carrier.location?.system].filter(Boolean).join(" · ")) +
      kv("Тритий в баке", n(carrier.fuel?.tankTons) + " т") + kv("Свободный склад", n(carrier.space?.freeTons) + " т") +
      kv("Баланс флотоносца", credits(carrier.finance?.balance)) +
      (carrier.fuel?.needsRefresh ? note("После прыжка нужен новый снимок топлива.") : "") + '<div class="actions-wrap">' + link("Управление и учёт", "carrier") + link("Маршрут", "route") + '</div></div>');
  }

  function recentPanel(state) {
    const recent = array(state.pilot?.recent).slice(0, 8);
    return panel("Журнал полёта", recent.length ? '<div class="pilot-events">' + recent.map((entry) =>
      '<div class="pilot-event"><time>' + h(dateTime(entry.timestamp)) + '</time><div><strong>' + h(entry.label || entry.event || "Событие") + '</strong><p>' + h([entry.system, entry.station, entry.detail].filter(Boolean).join(" · ")) + '</p></div></div>').join("") + '</div>' :
      '<div class="panel-body">' + note("Перелёты, стыковки, миссии и другие события появятся после чтения журналов командира. Новые записи подхватываются автоматически.") + '</div>');
  }

  function tools() {
    const card = (symbol, title, description, actions) => '<article class="pilot-tool"><span aria-hidden="true">' + h(symbol) + '</span><div><h3>' + h(title) + '</h3><p>' + h(description) + '</p><div class="actions-wrap">' + actions + '</div></div></article>';
    return '<section class="pilot-tools" aria-label="Инструменты и занятия">' +
      card("⇄", "Торговля и логистика", "Сравнение рынков, прибыльных рейсов и перевозки груза.", link("Рынки", "sale") + link("Рейсы", "deals")) +
      card("⌂", "Колонизация", "Выбор системы, порядок построек и снабжение строительных складов.", link("Открыть планировщик", "colonization")) +
      card("⚒", "Корабль и инженерия", "Материалы, чертежи, доступ к инженерам и справочник кораблей.", link("План улучшений", "engineering") + action("Корабли", "encyclopedia", "catalog-ships")) +
      card("⌁", "Путешествия и исследования", "Навигация, цели поездок и знания для экспедиции.", action("Цели", "targets", "plan") + action("Исследования", "encyclopedia", "exploration")) +
      card("◈", "Занятия в галактике", "Бой, миссии, Odyssey и другие направления в энциклопедии.", action("Бой", "encyclopedia", "combat") + action("Все разделы", "encyclopedia", "all")) + '</section>';
  }

  function render(state = {}) {
    const commander = state.commander || {}, pilot = state.pilot || {}, creditsSnapshot = balance(state);
    const used = finite(commander.cargoTons), capacity = commander.vehicle === "srv" ? finite(commander.srvCargoCapacity) : finite(commander.cargoCapacity);
    const fill = used != null && capacity > 0 ? Math.min(100, Math.max(0, used / capacity * 100)) : null;
    const vehicle = { ship: "В корабле", srv: "В ТРП", foot: "Пешком", unknown: "Положение уточняется" }[commander.vehicle] || "Положение уточняется";
    const location = commander.docked && commander.station ? commander.station : commander.body;
    const fuel = pilot.fuel || {}, session = pilot.session || {};
    return '<div class="pilot-overview"><div class="page-heading"><div><p class="eyebrow">РАБОЧЕЕ МЕСТО ПИЛОТА</p><h1>Панель командира</h1><p>Положение, состояние корабля и следующие шаги — для выбранных вами задач.</p></div><div class="heading-actions">' + action("Мои цели", "targets", "plan", "button-primary") + '<button class="button button-subtle" type="button" data-help-topic="overview">? Как пользоваться</button></div></div>' +
      '<section class="pilot-position"><div><small>ПОЛОЖЕНИЕ ПО ЖУРНАЛУ</small><h2>' + h(commander.system || "Ожидание данных игры") + '</h2><p>' + h(location || "Станция или тело ещё не определены") + '</p><div class="pill-row">' + pill(vehicle, "blue") + (state.connection?.gameRunning === false ? pill("Последний сеанс", "muted") : "") + (commander.docked ? pill("На станции", "confirmed") : "") + (commander.name ? pill("CMDR " + commander.name, "muted") : "") + '</div></div><div class="pilot-ship"><small>КОРАБЛЬ</small><strong>' + h(commander.ship || "Нет снимка корабля") + '</strong>' +
      (commander.system ? action("Копировать систему", "copy", commander.system) : link("Подключить журнал", "settings")) + '</div></section>' +
      '<div class="metrics pilot-metrics">' +
      metric("Баланс по снимку", credits(creditsSnapshot.value), creditsSnapshot.value != null ? creditsSnapshot.source + " · " + dateTime(creditsSnapshot.timestamp) : "Появится из журнала или Frontier") +
      metric(commander.vehicle === "srv" ? "Трюм ТРП" : "Товарный трюм", n(used) + '<small> / ' + n(capacity) + ' т</small>', fill != null ? n(fill) + "% занято" : "Вместимость не подтверждена") +
      metric("Основное топливо", n(fuel.mainTons) + '<small> / ' + n(fuel.mainCapacityTons) + ' т</small>', fuel.updatedAt ? "Снимок " + dateTime(fuel.updatedAt) : "Нужен свежий снимок корабля") +
      metric("Прыжков за сеанс", n(session.jumps, true), session.distanceLy != null ? n(session.distanceLy) + " св. г." + (session.since ? " · с " + dateTime(session.since) : "") : "По доступным событиям перелётов") + '</div>' +
      '<div class="pilot-dashboard"><div class="stack">' + renderPlans(state) + recentPanel(state) + '</div><aside class="stack">' + cargoPanel(state) + carrierPanel(state) +
      panel("Ваши инструменты", '<div class="panel-body"><p class="panel-note">Разделы приложения доступны независимо от состава оверлея. Готовьте планы и сверяйте данные в удобном для вас порядке.</p><div class="actions-wrap">' + action("Все цели", "targets", "plan") + action("Энциклопедия", "encyclopedia", "all") + link("Руководство", "help") + '</div></div>') + '</aside></div>' + tools() + '</div>';
  }

  function init(api) {
    context = api;
    if (initialized) return;
    initialized = true;
    document.addEventListener("click", async (event) => {
      const button = event.target.closest("[data-pilot-action]");
      if (!button) return;
      const action = button.dataset.pilotAction, value = button.dataset.value;
      try {
        if (action === "copy") {
          if (!value) return;
          await window.MiningUI.copyText(value);
          context.showToast?.("Название системы скопировано");
        } else if (action === "targets") {
          if (window.CopilotTargets?.open) window.CopilotTargets.open(value);
          else if (context.openTargets) context.openTargets(value);
          else location.hash = "targets";
        } else if (action === "encyclopedia") {
          if (window.CopilotEncyclopedia?.open) window.CopilotEncyclopedia.open(value === "all" ? undefined : value);
          else location.hash = "encyclopedia";
        }
      } catch (error) { context.showToast?.(error.message || "Не удалось выполнить действие.", true); }
    });
  }

  window.CopilotOverview = { init, render };
})();
