/* Route workspace: Go owns route calculations; this view presents their result. */
(() => {
  'use strict';
const t=(value,vars)=>window.CopilotI18n?.t(value,vars)??String(value??'');
const th=(value,vars)=>t(value,vars).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const pendingStates = new Set(['resolving', 'submitting', 'queued', 'routing', 'planning', 'calculating', 'pending']);
  const known = value => typeof value === 'number' && Number.isFinite(value) && value >= 0;
  const state = C => C.state?.carrierRoute || {};
  const esc = (u, value) => u.esc(String(value ?? ''));
  const number = (u, value, unit = '') => known(value) ? esc(u, u.fmt(value)) + (unit ? ' ' + unit : '') : t("Неизвестно");
  const money = (u, value) => known(value) ? esc(u, u.fmt(value)) + t(" кр.") : t("Неизвестна");
  const label = value => typeof value === 'string' ? value.trim() : '';

  function model(C) {
    const s = state(C), route = s.route || {}, stops = Array.isArray(route.stops) ? route.stops : [], jumps = stops.slice(1);
    const hasRoute = stops.length > 1, pending = s.running === true || pendingStates.has(s.status), failed = s.status === 'error' || !!label(s.error);
    const completed = jumps.filter(stop => stop?.visited === true).length;
    return { s, route, stops, hasRoute, pending, failed, completed, total: jumps.length,
      remaining: jumps.length - completed, next: jumps.find(stop => stop && stop.visited !== true) || null,
      summary: s.summary || {}, finished: hasRoute && completed === jumps.length,
      source: label(route.sourceSystem) || label(stops[0]?.name), destination: label(route.destinationSystem) || label(stops.at(-1)?.name),
    };
  }
  function duration(u, value) {
    if (!known(value)) return t("Неизвестно");
    const minutes = Math.ceil(value), days = Math.floor(minutes / 1440), hours = Math.floor(minutes % 1440 / 60), rest = minutes % 60;
    return esc(u, [days ? days + t(" д") : '', hours ? hours + t(" ч") : '', rest || (!days && !hours) ? rest + t(" мин") : ''].filter(Boolean).join(' '));
  }
  function date(u, value) {
    if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}T/.test(value)) return t("Неизвестно");
    const parsed = new Date(value);
    if (!Number.isFinite(parsed.getTime())) return t("Неизвестно");
    return esc(u, parsed.toLocaleString(window.CopilotI18n?.locale()||'ru-RU', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' }));
  }
  function timeRange(u, summary) {
    const range = summary.timeRange || {}, first = known(range.minMinutes) ? range.minMinutes : summary.totalMinutes;
    if (!known(first)) return t("Неизвестно");
    return known(range.maxMinutes) && range.maxMinutes > first ? duration(u, first) + ' — ' + duration(u, range.maxMinutes) : duration(u, first);
  }
  function inventory(u, tank, cargo) {
    return `<span>${th("Бак ")}<strong>${number(u, tank, t("т"))}</strong></span><span>${th("Склад ")}<strong>${number(u, cargo, t("т"))}</strong></span>`;
  }
  function metric(u, icon, title, value, detail, extra = '') {
    return `<div class="rw-metric ${extra}"><span class="rw-metric-label">${u.icon(icon)}${title}</span><strong class="rw-metric-value">${value}</strong>${detail ? `<span class="rw-metric-detail">${detail}</span>` : ''}</div>`;
  }
  function statusHTML(m, u) {
    if (m.pending) return `<div class="rw-status rw-status-pending" role="status">${u.icon('refresh-cw')}<div><strong>${th("Рассчитываем маршрут")}</strong><span>${esc(u, label(m.s.progress?.label) || t("Запрашиваем системы, расход трития и остановки."))}${m.hasRoute ? t(" Предыдущий расчёт показан ниже и пока не обновлён.") : ''}</span></div></div>`;
    if (m.failed) return `<div class="rw-status rw-status-error" role="alert">${u.icon('circle-help')}<div><strong>${th("Маршрут не рассчитан")}</strong><span>${esc(u, label(m.s.error) || t("Не удалось получить результат. Проверьте параметры и повторите расчёт."))}${m.hasRoute ? t(" Ниже сохранён предыдущий расчёт.") : ''}</span></div></div>`;
    if (m.s.status === 'cancelled') return `<div class="rw-status" role="status">${u.icon('minus')}<div><strong>${th("Расчёт остановлен")}</strong><span>${m.hasRoute ? t("Предыдущий маршрут сохранён.") : t("Измените параметры и запустите расчёт снова.")}</span></div></div>`;
    return '';
  }
  function renderSummary(C, u) {
    if (C.tab !== 'route') return '';
    const m = model(C), status = statusHTML(m, u);
    if (!m.hasRoute) return `<section class="route-workspace" aria-label="${th("Сводка маршрута флотоносца")}">${status}${!m.pending && !m.failed ? `<div class="rw-empty">${u.icon('route')}<div><strong>${th("Подготовьте маршрут флотоносца")}</strong><p>${th("Укажите начало, назначение и загрузку. После расчёта здесь появятся следующий прыжок, топливо, резерв и время.")}</p></div></div>` : ''}</section>`;
    const s = m.summary, next = m.next, canCopy = !!label(next?.name) && !m.pending && !m.failed;
    const calculationOptions = m.pending || m.failed ? (m.route.options || m.s.options || {}) : (m.s.options || m.route.options || {});
    const hasPrice = known(calculationOptions.tritiumPrice) && calculationOptions.tritiumPrice > 0;
    const costKnown = known(s.fuelCost) && hasPrice;
    const costNote = hasPrice ? t("Оценка стоимости пока отсутствует") : t("Цена трития не задана");
    const minimumNotice = calculationOptions.calculateStartingFuel === true ? `<div class="rw-status rw-minimum">${u.icon('package')}<div><strong>${th("План с минимальной начальной загрузкой")}</strong><span>${th("Если сохраняете больший запас трития на флотоносце, отключите расчёт минимума и пересчитайте маршрут с фактическими остатками.")}</span></div></div>` : '';
    const progress = m.total ? Math.min(100, m.completed / m.total * 100) : 0;
    const routeLabel = `${esc(u, m.source || t("Начало неизвестно"))}<span aria-hidden="true">→</span>${esc(u, m.destination || t("Назначение неизвестно"))}`;
    const nextName = m.finished ? t("Маршрут пройден") : label(next?.name) || t("Следующая система неизвестна");
    const nextFacts = next ? `<dl class="rw-next-facts"><div><dt>${th("Дальность прыжка")}</dt><dd>${number(u, next.distanceLy, t("св. л."))}</dd></div><div><dt>${th("Расход на прыжок")}</dt><dd>${number(u, next.fuelUsedTons, t("т"))}</dd></div><div><dt>${th("Пополнить бак после прыжка")}</dt><dd>${number(u, next.transferTons, t("т"))}</dd></div>${known(next.restockTons) && next.restockTons > 0 ? `<div class="rw-refuel"><dt>${th("Получить в этой точке")}</dt><dd>${number(u, next.restockTons, t("т"))}</dd></div>` : ''}</dl>` : `<p class="rw-completed-note">${th("Все прыжки отмечены посещёнными. Итоги ниже относятся ко всему маршруту.")}</p>`;
    const arrival = date(u, s.estimatedArrival), latest = date(u, s.estimatedArrivalLatest);
    const eta = arrival === t('Неизвестно') ? arrival : arrival + (latest !== t('Неизвестно') && latest !== arrival ? ' — ' + latest : '');
    const warnings = Array.isArray(m.s.warnings) ? m.s.warnings.filter(value => typeof value === 'string' && value.trim()) : [];
    return `<section class="route-workspace ${m.pending || m.failed ? 'rw-previous' : ''}" aria-label="${th("Сводка маршрута флотоносца")}">${status}${minimumNotice}<div class="rw-route-line">${u.icon('route')}<span>${routeLabel}</span>${m.pending || m.failed ? `<small>${th("Предыдущий расчёт")}</small>` : ''}</div><div class="rw-primary"><div class="rw-next"><div class="rw-next-heading"><span>${u.icon(m.finished ? 'circle-check' : 'compass')}<small>${m.finished ? t("Полёт завершён") : t("Следующий прыжок")}</small></span><button type="button" class="rw-copy" data-click="route-workspace-copy" ${canCopy ? '' : 'disabled'} title="${th("Копировать следующую систему")}">${u.icon('copy')}${th("Копировать систему")}</button></div><h2>${esc(u, nextName)}</h2>${nextFacts}</div><div class="rw-progress"><span class="rw-progress-label">${th("Пройдено прыжков")}</span><strong>${m.completed}<span> / ${m.total}</span></strong><div class="rw-progress-track" role="progressbar" aria-valuemin="0" aria-valuemax="${m.total}" aria-valuenow="${m.completed}" aria-label="${th("Посещённые прыжки")}"><span style="width:${progress}%"></span></div><span class="rw-progress-remaining">${m.finished ? t("Все точки посещены") : `${t("Осталось ")}${m.remaining}`}</span><small>${th("По отметкам посещения в маршруте")}</small></div></div><div class="rw-metrics">${metric(u, 'package', t("Расход трития"), number(u, s.fuelTons, t("т")), t("За весь маршрут"))}${metric(u, 'clock', t("Время по плану"), timeRange(u, s), t("Полный исходный расчёт"))}${metric(u, 'wallet', t("Стоимость топлива"), money(u, costKnown ? s.fuelCost : null), costKnown ? t("Израсходованный тритий") : costNote)}${metric(u, 'refresh-cw', t("Получить в пути"), number(u, s.restockTons, t("т")), known(s.restockStops) ? `${number(u, s.restockStops)}${t(" остановок пополнения")}` : t("Остановки пополнения неизвестны"))}</div><div class="rw-fuel-plan"><div><h3>${th("На старте по плану")}</h3><div class="rw-inventory">${inventory(u, s.startingTankTons, s.startingCargoTritiumTons)}</div></div><div><h3>${th("По прибытии по плану")}</h3><div class="rw-inventory">${inventory(u, s.arrivalTankTons, s.arrivalCargoTritiumTons)}</div></div><div class="${known(s.reserveShortfallTons) && s.reserveShortfallTons > 0 ? 'rw-reserve-warning' : ''}"><h3>${th("Резерв после прибытия")}</h3><div class="rw-inventory"><span>${th("Цель ")}<strong>${number(u, s.reserveTritiumTons, t("т"))}</strong></span><span>${th("Не хватает ")}<strong>${number(u, s.reserveShortfallTons, t("т"))}</strong></span></div></div></div><div class="rw-arrival"><span>${u.icon('clock')}${th("Расчётное прибытие ")}<strong>${eta}</strong></span><small>${th("Время не пересчитывается по часам посещения. Очередь и ожидания могут увеличить его.")}</small></div>${warnings.length ? `<details class="rw-warnings"><summary>${u.icon('circle-help')}${th("Условия маршрута · ")}${warnings.length}</summary><ul>${warnings.map(warning => `<li>${esc(u, warning)}</li>`).join('')}</ul></details>` : ''}</section>`;
  }
  async function handle(action, _button, C, u) {
    if (action !== 'route-workspace-copy' || C.tab !== 'route') return false;
    const m = model(C), name = label(m.next?.name);
    if (!m.hasRoute || !name || m.pending || m.failed) { u.toast(t("Нет следующей точки для копирования"), true); return true; }
    await u.call('native', { action: 'copy', text: name });
    u.toast(t("Система скопирована"));
    return true;
  }
  function init() {} // Re-rendered from the normal view updates; no extra polling.
  window.RouteWorkspaceUI = { renderSummary, handle, init };
})();
