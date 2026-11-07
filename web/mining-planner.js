(function () {
  'use strict';
  const { h, n, credits, duration, dateTime, pill } = window.MiningUI;
  const H = window.MiningExpedition.helpers;
  function render(state) {
    const mining = state.mining || {};
    const run = mining.current;
    const latest = run || mining.history?.[0];
    const preflight = mining.preflight || {};
    const cargo = mining.cargo || {};
    const plan = mining.plan || {};
    const o = H.values('mining-plan', mining.options || { method: 'laser', resourceKey: 'platinum', mode: 'mining', tripMinutes: 20, operatingCost: 0 });
    const goods = (state.resources || []).filter((r) => r.kind === 'commodity');
    const checks = { ok: ['Готово', 'confirmed'], missing: ['Отсутствует', 'potential'], disabled: ['Выключено / повреждено', 'potential'], empty: ['Нет запаса', 'potential'], full: ['Трюм полон', 'potential'], unknown: ['Нет данных', 'muted'] };
    const checkBody = '<div class="panel-body"><p>' + h(preflight.ship || state.commander.ship) + ' · ' + dateTime(preflight.timestamp) + '</p>' +
      (preflight.checks || []).map((check) => '<div class="preflight-item">' + pill((checks[check.status] || checks.unknown)[0], (checks[check.status] || checks.unknown)[1]) +
        '<div><strong>' + h(check.name) + '</strong><p>' + (check.required ? 'Необходимо для выбранного способа' : 'Рекомендуется') + (check.status !== 'unknown' && check.count != null ? ' · ' + n(check.count, true) : '') + '</p></div></div>').join('') +
      '<p class="panel-note">' + h(preflight.help || 'Оценка основана на последнем Loadout из журнала. После смены оснащения нужен новый снимок.') + '</p></div>';
    const composition = latest?.composition || [];
    const yields = latest?.yields || [];
    const yieldTable = yields.length ? '<div class="table-wrap"><table><thead><tr><th>Ресурс</th><th class="table-num">Переработано, т</th><th class="table-num">Сырьё, ед.</th><th class="table-num">т/ч</th></tr></thead><tbody>' +
      yields.map((row) => '<tr><td>' + h(row.name) + '</td><td class="table-num">' + n(row.refinedTons) + '</td><td class="table-num">' + n(row.materialUnits) + '</td><td class="table-num">' + n(row.tonsPerHour) + '</td></tr>').join('') + '</tbody></table></div>' : '<div class="panel-body"><p class="panel-note">После первого действия добычи появится текущий заход.</p></div>';
    const compositionTable = composition.length ? '<div class="table-wrap"><table><thead><tr><th>Ресурс</th><th class="table-num">Встречен</th><th class="table-num">Средний %</th><th class="table-num">Мин. / макс. %</th><th class="table-num">Ядра</th><th class="table-num">Содержание ≥20% / ≥30%</th></tr></thead><tbody>' +
      composition.map((row) => '<tr><td>' + h(row.name) + '</td><td class="table-num">' + n(row.occurrencePercent) + '%</td><td class="table-num">' + n(row.averagePercent) + '</td><td class="table-num">' + n(row.minPercent) + ' / ' + n(row.maxPercent) + '</td><td class="table-num">' + n(row.motherlodes, true) + '</td><td class="table-num">' + n(row.above?.find((a) => a.threshold === 20)?.percentOfAllScans) + '% / ' + n(row.above?.find((a) => a.threshold === 30)?.percentOfAllScans) + '%</td></tr>').join('') + '</tbody></table></div>' : '';
    return H.heading('Заход добычи и подготовка', 'Оснащение, фактический темп, статистика разведки и план заработка для текущего способа добычи.') +
      '<div class="metrics">' + H.metric('Добыто за заход', n(latest?.refinedTons) + '<small> т</small>', run ? 'Текущий заход' : 'Последний завершённый') +
      H.metric('Скорость', n(latest?.tonsPerHour) + '<small> т/ч</small>', 'С учётом пауз внутри захода') +
      H.metric('Дроны', n(cargo.limpets, true), 'Разведчики: ' + n(latest?.prospectors, true) + ' · сборщики: ' + n(latest?.collectors, true)) +
      H.metric('До полного трюма', cargo.minutesToFull != null ? duration(cargo.minutesToFull * 60) : '—', 'Свободно ' + n(cargo.freeTons) + ' т') + '</div>' +
      H.note(latest?.clockCapped ? 'После пяти минут без новых событий оценка часов остановлена. Следующее событие журнала уточнит время захода.' : '') +
      '<div class="carrier-grid"><div class="stack">' + H.panel('План добычи и перевозки', '<form id="mining-plan-form" class="panel-body"><div class="form-grid expedition-form">' +
      H.select('mining-plan', o, 'method', 'Способ добычи для проверки оснащения', Object.entries(window.MiningUI.methods).filter(([key]) => key !== 'all')) +
      H.select('mining-plan', o, 'resourceKey', 'Целевой ресурс', goods.map((g) => [g.key, g.name])) +
      H.select('mining-plan', o, 'mode', 'Источник товара', [['mining', 'Собственная добыча'], ['trade', 'Закупка и перепродажа']]) +
      H.input('mining-plan', o, 'targetTons', 'Целевой объём, т', 'number', 'min="0" max="25000" placeholder="Вместимость трюма"') +
      H.input('mining-plan', o, 'sellPrice', 'Цена продажи, кр/т', 'number', 'min="0" placeholder="Последняя известная"') +
      H.input('mining-plan', o, 'purchasePrice', 'Цена закупки / затраты на тонну, кр.', 'number', 'min="0"') +
      H.input('mining-plan', o, 'tonsPerHour', 'Скорость добычи, т/ч', 'number', 'min="0" placeholder="По журналу выбранного ресурса"') +
      H.input('mining-plan', o, 'tripMinutes', 'Время полного рейса, мин', 'number', 'min="0"') +
      H.input('mining-plan', o, 'operatingCost', 'Расходы на один рейс, кр.', 'number', 'min="0"') +
      H.input('mining-plan', o, 'targetCredits', 'Денежная цель, кр.', 'number', 'min="0"') + '</div><div class="actions-wrap"><button class="button" type="submit">Обновить план</button>' + H.button('Новый заход', 'mining-reset') + '</div></form>' +
      '<div class="panel-body"><div class="summary-grid"><div><small>Ожидаемый результат</small><strong>' + credits(plan.profit) + '</strong></div><div><small>Полное время</small><strong>' + (plan.totalMinutes != null ? duration(plan.totalMinutes * 60) : '—') + '</strong></div><div><small>Результат в час</small><strong>' + credits(plan.estimatedProfitPerHour) + '</strong></div></div>' +
      '<div class="carrier-kv"><span>На борту / осталось добыть</span><strong>' + n(plan.aboardTons) + ' / ' + n(plan.remainingTons) + ' т</strong></div>' +
      '<div class="carrier-kv"><span>Рейсов</span><strong>' + n(plan.trips, true) + '</strong></div>' +
      '<div class="carrier-kv"><span>Выручка / закупка / расходы</span><strong>' + credits(plan.revenue) + ' / ' + credits(plan.acquisitionCost) + ' / ' + credits(plan.operatingCost) + '</strong></div>' +
      '<div class="carrier-kv"><span>Для денежной цели, до расходов</span><strong>' + n(plan.tonsForCreditTarget) + ' т</strong></div>' +
      '<p class="panel-note">Темп: ' + (plan.rateSource === 'manual' ? 'задан вручную' : plan.rateSource === 'journal' ? 'по журналу выбранного ресурса' : 'неизвестен') + '. Цена: ' + h(plan.priceSource || 'неизвестна') + (plan.priceAt ? ' · ' + dateTime(plan.priceAt) : '') + '</p>' +
      (plan.warnings || []).map((w) => H.note(w, true)).join('') + '<p class="panel-note">' + h(plan.help) + '</p></div>') +
      H.panel('Результат захода', yieldTable + '<div class="panel-body"><p class="panel-note">' + h(latest?.help || '') + '</p></div>') +
      (compositionTable ? H.panel('Качество разведанных астероидов', compositionTable) : '') + '</div><aside class="stack">' + H.panel('Готовность к добыче', checkBody) +
      H.panel('Последние заходы', '<div class="panel-body">' + (mining.history || []).slice(0, 12).map((r) => '<div class="carrier-kv"><span>' + dateTime(r.startedAt) + '<br>' + h(r.system || '') + '<br>' + h(r.closeReason || '') + '</span><strong>' + n(r.refinedTons) + ' т · ' + n(r.tonsPerHour) + ' т/ч<br>' + duration(r.elapsedSeconds) + '</strong></div>').join('') + '</div>') + '</aside></div>' +
      '<p class="panel-note">' + H.links(mining.sources) + '</p>';
  }
  window.MiningPlannerUI = { render };
})();
