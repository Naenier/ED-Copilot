(function () {
  "use strict";
  const U = window.MiningUI;
  const { h, n, credits, pill, dateTime, duration } = U;
  const root = document.getElementById("mining-overlay");
  const activities = { flight: "ПОЛЁТ И НАВИГАЦИЯ", asteroid: "ДОБЫЧА В КОЛЬЦАХ", surface: "ПЛАНЕТАРНАЯ ДОБЫЧА",
    raw: "СБОР СЫРЬЯ", trade: "ТОРГОВЛЯ", carrier: "ФЛОТОНОСЕЦ", engineering: "МАТЕРИАЛЫ ДЛЯ ИНЖЕНЕРИИ", colonization: "КОЛОНИЗАЦИЯ И СНАБЖЕНИЕ" };
  let state;
  let online = false;
  let lastHeight = 0;

  function metrics(rows) {
    return '<div class="overlay-metrics">' + rows.map(([label, value]) =>
      "<div><small>" + h(label) + "</small><strong>" + value + "</strong></div>").join("") + "</div>";
  }
  function section(title, body, badge = "") {
    return '<div class="overlay-prospect"><div class="overlay-prospect-heading"><strong>' +
      h(title) + "</strong>" + badge + "</div>" + body + "</div>";
  }
  function row(label, value) {
    return '<div class="overlay-context"><span>' + h(label) + "</span><strong>" + value + "</strong></div>";
  }
  function empty(text) { return '<p class="overlay-empty">' + h(text) + "</p>"; }
  function hold() {
    const commander = state.commander || {};
    const used = Number(commander.cargoTons) || 0;
    const capacity = commander.vehicle === "srv" ? commander.srvCargoCapacity : commander.cargoCapacity;
    const fill = capacity > 0 ? Math.max(0, Math.min(100, used / capacity * 100)) : 0;
    const value = U.resourceValue(commander.cargo, [...(state.resources || []), ...(state.goods || [])]);
    return metrics([["ТРЮМ", n(used, true) + " <em>/ " + n(capacity, true) + " т</em>"],
      ["ТЕМП ДОБЫЧИ", n(state.mining?.current?.tonsPerHour) + " <em>т/ч</em>"],
      ["ОЦЕНКА ГРУЗА", credits(value.total)]]) +
      '<div class="progress' + (fill > 90 ? " is-warn" : "") + '"><span style="width:' + fill + '%"></span></div>' +
      (commander.vehicle === "ship" && state.mining?.cargo?.limpets != null ? row("Дроны в трюме", n(state.mining.cargo.limpets, true)) : "");
  }
  function targetFor(activity) {
    const commander = state.commander || {};
    const matches = (target) => activity === "asteroid" ? target.kind === "ring"
      : activity === "surface" ? target.kind === "surface" && target.miningSites > 0
        : target.kind === "surface" && (target.methods || []).includes("raw");
    const available = (state.targets || []).filter(matches);
    const current = available.find((target) => target.system === commander.system &&
      [target.body, target.ring].includes(commander.body));
    if (current) return { target: current, label: "ТЕКУЩЕЕ МЕСТО" };
    const method = activity === "surface" ? "surface-planet" : activity === "raw" ? "raw" : state.settings?.method || "all";
    const saved = (state.bookmarks || []).find((entry) => entry.target && matches(entry.target) &&
      U.eligibleTarget(entry.target, state, { method }));
    if (saved) return { target: saved.target, label: "ЦЕЛЬ ВЫЛЕТА" };
    const best = available.filter((target) => U.eligibleTarget(target, state, { method }))
      .sort((a, b) => (a.distanceLy ?? Infinity) - (b.distanceLy ?? Infinity) || b.score - a.score)[0];
    return best ? { target: best, label: "БЛИЖАЙШАЯ ЦЕЛЬ" } : null;
  }
  function targetMarkup(activity) {
    const chosen = targetFor(activity);
    if (!chosen) return "";
    const target = chosen.target;
    return '<div class="overlay-target"><small>' + h(chosen.label) + "</small><strong>" +
      h(target.ring || target.body) + "</strong>" + row(target.system, n(target.distanceLy) + " св. г.") +
      (activity === "surface" ? pill("Участков " + n(target.miningSites, true), "blue") :
        activity === "raw" ? (target.materials || []).filter((item) => item.grade >= 3).slice(0, 3)
          .map((item) => row(item.name, n(item.percent) + "% · G" + n(item.grade, true))).join("") :
          U.tier(target)) + "</div>";
  }
  function asteroid() {
    const prospect = state.prospect;
    if (!prospect) return hold() + section("РАЗВЕДКА АСТЕРОИДА",
      empty("Запустите разведывательного дрона, чтобы увидеть состав."), "<span>ОЖИДАНИЕ</span>") + targetMarkup("asteroid");
    const decision = prospect.decision === "mine" ? pill("ДОБЫВАТЬ", "confirmed")
      : prospect.decision === "skip" ? pill("НИЗКИЙ ПРИОРИТЕТ", "potential") : pill("НУЖНЫ ДАННЫЕ", "muted");
    const title = prospect.motherlode ? "ЯДРО · " + U.resourceName(state.resources, prospect.motherlode) : "СОСТАВ АСТЕРОИДА";
    const materials = (prospect.materials || []).slice(0, 4).map((item) =>
      '<div class="overlay-material"><span>' + h(item.name || item.key) +
      '</span><div class="progress"><span style="width:' + Math.max(0, Math.min(100, Number(item.percent) || 0)) +
      '%"></span></div><strong>' + n(item.percent) + "%</strong></div>").join("");
    return hold() + section(title, materials + row("Осталось для лазера", n(prospect.remaining) + "%") +
      row("Смесь по известным ценам", prospect.estimatedPerTonne > 0 ? credits(prospect.estimatedPerTonne) + " / т" : "—") +
      (prospect.reasons?.[0] ? empty(prospect.reasons[0]) : ""), decision) + targetMarkup("asteroid");
  }
  function surface() {
    const site = state.surface || {};
    const products = site.lastRefined || [];
    return hold() + section("УЧАСТОК И ПЕРЕРАБОТКА",
      row(site.site || state.commander?.body || "Место пока не определено", "") +
      (products.length ? products.slice(0, 4).map((item) => row(item.name || item.key, n(item.count) + " т")).join("") :
        empty("Переработанные товары появятся после событий добычи в журнале."))) + targetMarkup("surface");
  }
  function raw() {
    const inventory = state.commander?.materials || [];
    const recent = (state.stats?.recent || []).filter((event) => event.kind === "material").slice(0, 4);
    return metrics([["СОБРАНО", n(state.stats?.total?.materialUnits || 0) + " <em>ед.</em>"],
      ["КАТЕГОРИИ 3–4", n(inventory.filter((item) => item.grade >= 3).reduce((sum, item) => sum + item.count, 0)) + " <em>ед.</em>"],
      ["В ЗАПАСЕ", n(inventory.reduce((sum, item) => sum + item.count, 0)) + " <em>ед.</em>"]]) +
      section("ПОСЛЕДНИЙ СБОР", recent.length ? recent.map((event) => row(event.name || event.key, "+" + n(event.count))).join("") :
        empty("Сбор сырьевых материалов появится здесь по журналу.")) +
      section("РЕДКОЕ СЫРЬЁ В ЗАПАСЕ", inventory.filter((item) => item.grade >= 3).sort((a, b) => b.grade - a.grade)
        .slice(0, 5).map((item) => row(item.name || item.key, n(item.count) + " · G" + n(item.grade, true))).join("") ||
        empty("Запас материалов ещё не известен.")) + targetMarkup("raw");
  }
  function trade() {
    const destination = state.trading?.destination;
    const plan = state.tradePlan;
    const cargo = state.commander?.cargo || [];
    const tons = cargo.filter((item) => !/^(drones|limpets?)$/i.test(item.key)).reduce((sum, item) => sum + item.count, 0);
    let result = metrics([["ТОВАРНЫЙ ГРУЗ", n(tons) + " <em>т</em>"],
      ["КОТИРОВКА ГРУЗА", credits(destination?.estimatedTotal)],
      ["ПЛАНОВАЯ МАРЖА", credits(plan?.grossMargin)]]);
    if (!destination) return result + section("РЫНОК ПЕРЕВОЗКИ",
      empty("В разделе «Торговля» найдите покупателя и нажмите «Везти сюда»."));
    const quotes = (destination.quotes || []).slice(0, 4);
    return result + section(destination.station || "ВЫБРАННЫЙ РЫНОК",
      row(destination.system, n(destination.distanceLy) + " св. г.") +
      row("От звезды", n(destination.distanceLs, true) + " св. с.") +
      row("Площадка", h(destination.pad || "Неизвестна")) +
      quotes.map((quote) => '<div class="overlay-quote"><strong>' + h(quote.name || quote.key) +
        "</strong>" + row(credits(quote.price) + " / т", n(quote.demand ?? quote.supply, true) + " т " +
          (quote.demand != null ? "спрос" : "предложение")) + "</div>").join("") +
      empty((destination.expired ? "Котировка устарела. " : "") + "Данные " + dateTime(destination.updatedAt) +
        ". В расчёте " + n(destination.coveredTons) + " т" + (destination.missingTons > 0 ?
        ", ещё " + n(destination.missingTons) + " т без предложения." : ".")),
      pill(destination.carrier ? "ФЛОТОНОСЕЦ" : "СТАНЦИЯ", "blue"));
  }
  function carrier() {
    const carrier = state.carrier || {};
    if (!carrier.owned) return section("ЛИЧНЫЙ ФЛОТОНОСЕЦ",
      empty("После покупки откройте управление флотоносцем в игре. Его снимок появится здесь."));
    const fuel = carrier.fuel || {};
    const jump = carrier.jump || {};
    const route = state.carrierRoute;
    const next = route?.route?.stops?.find((stop) => !stop.visited && stop.index > 0);
    const remaining = jump.pending?.departureMs ? Math.max(0, (jump.pending.departureMs - Date.now()) / 1000) : jump.secondsToDeparture;
    return metrics([["ТРИТИЙ", n(fuel.tankTons) + " <em>т в баке</em>"],
      ["СРЕДСТВА", credits(carrier.finance?.availableBalance ?? carrier.finance?.balance)],
      ["СВОБОДНО", n(carrier.space?.freeTons) + " <em>т</em>"]]) +
      section(carrier.name || carrier.callsign || "ФЛОТОНОСЕЦ", row(carrier.callsign || "", h(carrier.location?.system || "Система не подтверждена")) +
        row("Известный резерв трития", n(fuel.reserveTons) + " т") +
        row("Содержание в неделю", credits(carrier.finance?.weeklyUpkeep)) +
        (fuel.needsRefresh ? empty("Данные топлива требуют нового снимка после прыжка.") : "")) +
      section("ПРЫЖОК", jump.pending ? row(jump.pending.system || "Назначение", remaining > 0 ? "Через " + duration(remaining) :
        "Ожидается подтверждение") + empty("Отправление " + dateTime(jump.pending.departureTime)) :
        empty(jump.phase === "cooldown" ? "Восстановление после прыжка." : "Назначение пока не задано.")) +
      (next ? section("МАРШРУТ ФЛОТОНОСЦА", row("Следующая система", h(next.name)) +
        row("Прыжков осталось", n(route.summary?.remainingJumps, true)) + row("Расход на следующий прыжок", n(next.fuelUsedTons) + " т") +
        (next.restockTons > 0 ? row("Нужно пополнить", n(next.restockTons) + " т") : "")) : "") +
      (carrier.advisories?.[0] ? empty(carrier.advisories[0].text) : "");
  }
  function flight() {
    const pilot = state.pilot || {}, commander = state.commander || {}, nav = pilot.navTarget;
    const goals = (state.objectives || []).filter((row) => row.status === 'active').slice(0, 2);
    const missions = (pilot.missions || []).slice(0, 2);
    return metrics([["ТОПЛИВО КОРАБЛЯ", n(pilot.fuel?.mainTons) + " <em>/ " + n(pilot.fuel?.mainCapacityTons) + " т</em>"],
      ["ТРЮМ", n(commander.cargoTons, true) + " <em>т</em>"], ["ПРЫЖКОВ ЗА СЕАНС", n(pilot.session?.jumps, true)]]) +
      section("КУРС КОРАБЛЯ", nav?.system ? row("Следующая система", h(nav.system)) + row("Осталось прыжков", n(nav.remainingJumps, true)) : empty("Выберите систему или проложите маршрут на карте галактики.")) +
      (commander.station ? row("Станция", h(commander.station)) : '') +
      (goals.length ? section("ЛИЧНЫЙ ПЛАН", goals.map((goal) => row(goal.title, h(goal.system || ''))).join('')) : '') +
      (missions.length ? section("ПОРУЧЕНИЯ", missions.map((mission) => row(mission.name, h(mission.destinationSystem || '') + (mission.expired ? ' · срок истёк' : ''))).join('')) : '') +
      (!goals.length && !missions.length ? empty("В «Целях» можно составить личный план для любого занятия.") : '');
  }
  function engineering() {
    const eng = state.engineering || {};
    const shopping = eng.shopping || {};
    const materials = (eng.materials || []).filter((m) => m.needed > 0 && (m.missing > 0 || m.owned == null));
    const type = { raw: "СЫРЬЁ", manufactured: "ПРОМ.", encoded: "ДАННЫЕ" };
    return metrics([["УЛУЧШЕНИЙ", n(eng.items?.length, true)], ["НУЖНО МАТЕРИАЛОВ", n(shopping.neededUnits, true)],
      ["НЕ ХВАТАЕТ", n(shopping.missingUnits, true)]]) +
      section("СПИСОК МАТЕРИАЛОВ", !(eng.items || []).length ? empty("Добавьте улучшения во вкладке «Инженерия».") :
        materials.length ? materials.slice(0, 6).map((m) => row(m.name, (m.owned == null ? "Нужно " + n(m.needed, true) : "Ещё " + n(m.missing, true)) +
          " · G" + h(m.grade) + " " + h(type[m.category] || ""))).join("") +
          (materials.length > 6 ? empty("Ещё материалов в списке: " + n(materials.length - 6, true)) : "") : empty("Известного запаса хватает на текущий план.")) +
      (!shopping.inventoryKnown && (eng.items || []).length ? empty("Запасы не полностью известны. Откройте игру для нового снимка материалов.") : "");
  }
  function colonization() {
    const colony = state.colonization || {}, project = colony.activeProject;
    if (!project) return section("ПЛАН КОЛОНИЗАЦИИ", empty("Добавьте вкладку «Колонизация» через «+» и выберите проект системы."));
    if (["complete", "cancelled"].includes(project.status)) return section(project.targetSystem || project.name || "КОЛОНИЗАЦИЯ",
      empty(project.status === "complete" ? "Проект завершён. Выберите следующий проект колонизации в приложении." : "Проект отменён. Выберите активный проект в приложении."));
    const task = project.nextTask;
    const building = (project.buildings || []).find(item => item.id === task?.buildingId);
    const resources = (building?.requirements || project.supplies || []).filter(item => item.missing > 0).sort((a, b) => b.missing - a.missing);
    const capacity = colony.cargoCapacity || state.commander?.cargoCapacity;
    const missingKnown = project.unknownBills === 0 || project.totalMissing > 0;
    return metrics([["ОСТАЛОСЬ ПО СМЕТАМ", missingKnown ? n(project.totalMissing, true) + " <em>т</em>" : "—"],
      ["СТРОИТСЯ", n(project.concurrent, true)],
      ["РЕЙСОВ", capacity > 0 && missingKnown ? n(Math.ceil(project.totalMissing / capacity), true) : "—"]]) +
      section(project.targetSystem || project.name || "КОЛОНИЗАЦИЯ", (project.faction ? row("Фракция", h(project.faction)) : "") +
        (task ? empty(task.text) : empty("Проект завершён или приостановлен.")) +
        (project.deadline ? row("Срок первичного порта", h(dateTime(project.deadline))) : "")) +
      section(building?.name || "СНАБЖЕНИЕ ПРОЕКТА", resources.length ? resources.slice(0, 5).map(item =>
        row(item.name || item.commodity, "Ещё " + n(item.missing, true) + " т") +
        (item.cargoAvailable > 0 ? row("На борту для поставки", n(Math.min(item.missing, item.cargoAvailable), true) + " т") : "")).join("") +
        (resources.some(item => !item.authoritative) ? empty("Часть потребности задана вручную. Точные объёмы — на стройплощадке.") : empty("Остаток по журналу стройплощадки и принятым поставкам.")) :
        empty(project.unknownBills > 0 ? "Посетите стройплощадку, чтобы получить список товаров." : "В известных списках недостающих ресурсов нет.")) +
      (project.unknownBills > 0 ? empty("Объектов без списка ресурсов: " + n(project.unknownBills, true) + ". Их тоннаж ещё не включён в расчёт.") : "");
  }
  function render() {
    if (!state) return;
    const commander = state.commander || {};
    const activity = state.settings?.activity || "flight";
    const renderers = { flight, asteroid, surface, raw, trade, carrier, engineering, colonization };
    const vehicle = { ship: "КОРАБЛЬ", srv: "ТРП", foot: "ПЕШКОМ", unknown: "ОЖИДАНИЕ" }[commander.vehicle] || "ОЖИДАНИЕ";
    root.innerHTML = '<div class="overlay-header"><strong><img src="copilot.svg" alt="" width="18" height="18" />ED COPILOT</strong><span>' + vehicle +
      '</span></div><div class="overlay-context"><strong>' + h(commander.system || "Ожидание положения") +
      "</strong><span>" + h(commander.name ? "CMDR " + commander.name : "") +
      '</span></div><div class="overlay-activity">' + h(activities[activity] || activities.asteroid) + "</div>" +
      (renderers[activity] || flight)() +
      (!online ? '<p class="overlay-disconnected">Соединение восстанавливается</p>' : "");
    requestAnimationFrame(() => {
      const height = Math.ceil(root.getBoundingClientRect().height + 8);
      if (height !== lastHeight) {
        lastHeight = height;
        window.edMining?.resizeOverlay?.({ height });
      }
    });
  }
  const subscription = U.subscribe((next) => { state = next; window.CopilotAppearance.apply(next.theme); render(); },
    (connected) => { online = connected; render(); });
  const clock = setInterval(() => {
    if (state?.settings?.activity === "carrier" && state.carrier?.jump?.pending) render();
  }, 1000);
  window.addEventListener("beforeunload", () => { subscription.close(); clearInterval(clock); });
})();
