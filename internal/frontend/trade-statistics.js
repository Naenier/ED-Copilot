(() => {
  'use strict';

  const series = [
    {key: 'revenue', label: 'Выручка'},
    {key: 'purchaseCost', label: 'Закупки'},
    {key: 'grossMargin', label: 'Валовая маржа'}
  ];
  const amount = value => Number.isFinite(Number(value)) ? Number(value) : 0;
  const traded = row => amount(row.boughtTons) > 0 || amount(row.soldTons) > 0;
  const realized = row => amount(row.soldTons) > amount(row.unknownCostTons) ? amount(row.grossMargin) : null;
  const label = (u, value, vars) => u.esc(u.t(value, vars));
  const value = (row, key) => key === 'grossMargin' ? realized(row) : amount(row[key]);
  const credits = (u, number) => number === null ? '—' : u.fmt(number) + u.t(' кр.');

  function date(u, raw, compact = false) {
    if (!raw) return '';
    const parsed = new Date(raw);
    if (!Number.isFinite(parsed.getTime())) return '';
    const options = compact ? {day: 'numeric', month: 'short', timeZone: 'UTC'} :
      {day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC'};
    return u.date?.(parsed, options) || new Intl.DateTimeFormat(u.locale?.() || 'ru-RU', options).format(parsed);
  }

  function renderChart(rows, u) {
    if (!rows.length) return '';
    const values = rows.flatMap(row => series.map(item => value(row, item.key) || 0));
    const maximum = Math.max(1, ...values);
    const minimum = Math.min(0, ...values);
    const range = maximum - minimum;
    const zero = -minimum / range * 100;
    const labelStep = Math.ceil(rows.length / 6);
    const bars = rows.map((row, index) => {
      const fullDate = date(u, row.date);
      const tooltip = [fullDate, ...series.map(item => u.t(item.label) + ' · ' + credits(u, value(row, item.key)))];
      const mark = index % labelStep === 0 || index === rows.length - 1 ? date(u, row.date, true) : '';
      return `<div class="trade-stats-day" title="${u.esc(tooltip.join('\n'))}">
        <div class="trade-stats-day-bars">${series.map(item => {
          const point = value(row, item.key) || 0;
          const height = Math.abs(point) / range * 100;
          const bottom = zero + Math.min(0, point) / range * 100;
          return `<span class="trade-stats-bar ${item.key}${point < 0 ? ' loss' : ''}"
            style="bottom:${bottom}%;height:${height}%"></span>`;
        }).join('')}</div><span class="trade-stats-day-label">${u.esc(mark)}</span></div>`;
    }).join('');
    const legend = series.map(item => `<li class="${item.key}">${label(u, item.label)}</li>`).join('');
    return `<section class="panel trade-stats-section">
      <header><div><h2>${label(u, 'Торговля по дням')}</h2>
      <p>${label(u, 'Последние 30 торговых дней · UTC')}</p></div>
      <ul class="trade-stats-legend">${legend}</ul></header>
      <div class="trade-stats-plot" aria-hidden="true" style="--zero:${zero}%">${bars}</div>
      <details class="trade-stats-day-details"><summary>${label(u, 'Значения по дням')}</summary>
      ${table(rows.slice().reverse(), u, true)}</details></section>`;
  }

  function table(rows, u, daily = false) {
    const titles = [daily ? 'Дата' : 'Товар', 'Куплено, т', 'Продано, т', 'Закупки, кр.', 'Выручка, кр.', 'Маржа, кр.'];
    const body = rows.map(row => {
      const margin = realized(row);
      const unknown = amount(row.unknownCostTons);
      const knownShare = unknown > 0 && margin !== null ? label(u, ' · частично') : '';
      return `<tr><th scope="row">${u.esc(daily ? date(u, row.date) : row.name || row.key)}</th>
        <td>${u.esc(u.fmt(amount(row.boughtTons)))}</td><td>${u.esc(u.fmt(amount(row.soldTons)))}</td>
        <td>${u.esc(u.fmt(amount(row.purchaseCost)))}</td><td>${u.esc(u.fmt(amount(row.revenue)))}</td>
        <td class="${margin !== null && margin < 0 ? 'loss' : ''}">${u.esc(u.fmt(margin))}${knownShare}</td></tr>`;
    }).join('');
    return `<div class="trade-stats-table"><table><thead><tr>
      ${titles.map(title => `<th scope="col">${label(u, title)}</th>`).join('')}</tr></thead><tbody>${body}</tbody></table></div>`;
  }

  function render(C, u) {
    const ledger = C.state.tradeLedger || {};
    const totals = ledger.totals || {};
    const days = (ledger.daily || []).filter(row => traded(row) && date(u, row.date))
      .slice(0, 30).reverse();
    const goods = (ledger.byResource || []).filter(traded);
    const start = date(u, C.state.stats?.since);
    const subtitle = start ? u.t('Учёт с {date}', {date: start}) : '';
    const reset = C.actions?.find(action => action.id === 'stats.reset');
    const resetButton = reset ? u.btn(reset.title, 'action', 'refresh-cw', 'quiet', 'data-action-id="stats.reset"') : '';
    const journal = u.btn(u.t('Журнал сделок'), 'trade-stats-ledger', 'book-open', 'quiet');
    const controls = u.heading(u.t('Статистика торговли'), subtitle, journal + resetButton) + u.renderModes(true);
    if (!traded(totals)) {
      return controls + `<div class="panel empty-state"><strong>${label(u, 'Торговых сделок пока нет')}</strong></div>`;
    }
    const unknown = amount(totals.unknownCostTons);
    const marginNote = unknown > 0 ? u.t('{tons} т продано без известной себестоимости', {tons: u.fmt(unknown)}) :
      u.t('Себестоимость проданного груза учтена');
    const metrics = [
      u.metric(u.t('Выручка'), u.money(amount(totals.revenue)), u.t('Продаж: {count}', {count: u.fmt(amount(totals.sellCount))}), 'wallet'),
      u.metric(u.t('Закупки'), u.money(amount(totals.purchaseCost)), u.t('Покупок: {count}', {count: u.fmt(amount(totals.buyCount))}), 'package'),
      u.metric(u.t('Валовая маржа'), u.money(realized(totals)), amount(totals.soldTons) > 0 ? marginNote : '', 'chart-no-axes-combined'),
      u.metric(u.t('Продано'), u.fmt(amount(totals.soldTons)) + u.t(' т'), u.t('Куплено {tons} т', {tons: u.fmt(amount(totals.boughtTons))}), 'arrow-left-right')
    ];
    const commoditySection = goods.length ? `<section class="panel trade-stats-section"><header><h2>${label(u, 'По товарам')}</h2></header>
      ${table(goods, u)}</section>` : '';
    return controls + `<div class="trade-stats">
      <div class="metrics trade-stats-metrics">${metrics.join('')}</div>
      ${renderChart(days, u)}${commoditySection}</div>`;
  }

  window.TradeStatsUI = {render};
})();
