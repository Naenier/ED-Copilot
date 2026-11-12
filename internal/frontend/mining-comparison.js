(() => {
  'use strict';

  const limit = 4;
  const methods = {laser:'Лазерная',core:'Ядра',subsurface:'Подповерхностная',surface:'Поверхность астероида','surface-planet':'Планетарная',raw:'Инженерное сырьё'};
  const bound = new WeakSet();
  const active = C => C.tab === 'mining' && C.mode === 'Сравнение заходов';
  const label = (u, value) => u.esc(u.t(value));
  const known = value => typeof value === 'number' && Number.isFinite(value);
  const number = (u, value, unit = '') => known(value) ? u.esc(u.fmt(value) + u.t(unit)) : '—';
  const date = (u, value) => value ? u.esc(u.date?.(value, {dateStyle:'short',timeStyle:'short'}) || value) : '—';
  const place = run => {
    const bodyNamesSystem = run.system && run.body?.startsWith(run.system + ' ');
    return [bodyNamesSystem ? '' : run.system, run.body !== run.system ? run.body : '', run.site].filter(Boolean).join(' · ');
  };

  function duration(u, value) {
    if (!known(value)) return '—';
    const seconds = Math.floor(value);
    if (seconds < 60) return number(u, seconds, ' с');
    if (seconds < 3600) return number(u, Math.floor(seconds / 60), ' мин') + ' ' + number(u, seconds % 60, ' с');
    return number(u, Math.floor(seconds / 3600), ' ч') + ' ' + number(u, Math.floor(seconds % 3600 / 60), ' мин');
  }

  function selection(C, runs) {
    const owner = C.state.planWorkspace?.key || '';
    if (C.miningCompare?.owner !== owner) {
      C.miningCompare = {owner, ids:runs.slice(0,2).map(run => run.id), resource:''};
    }
    const allowed = new Set(runs.map(run => run.id));
    C.miningCompare.ids = C.miningCompare.ids.filter(id => allowed.has(id));
    return C.miningCompare;
  }

  function table(u, rows, columns, sectionLabel) {
    return `<div class="mining-compare-table"><table aria-label="${label(u, sectionLabel)}"><thead><tr>
      <th scope="col">${label(u,'Показатель')}</th>${columns.map(run => `<th scope="col">${date(u,run.startedAt)}</th>`).join('')}
      </tr></thead><tbody>${rows.map(([title, cell]) => `<tr><th scope="row">${label(u,title)}</th>${columns.map(run => `<td>${cell(run)}</td>`).join('')}</tr>`).join('')}</tbody>
    </table></div>`;
  }

  function report(u, runs) {
    const markedTons = (run, key) => {
      const tons = run[key] || 0;
      if (run.originUncertain && tons === 0) return '—';
      return (run.originUncertain ? '≥ ' : '') + number(u, tons, ' т');
    };
    const confirmed = run => {
      if (run.unpricedSoldTons > 0 && !known(run.confirmedRevenue)) return '—';
      if (run.originUncertain && !known(run.confirmedRevenue)) return '—';
      const partial = run.originUncertain || run.unpricedSoldTons > 0;
      return (partial ? '≥ ' : '') + number(u,run.confirmedRevenue ?? 0,' кр.');
    };
    const methodsCell = run => {
      const name = methods[run.method];
      if (!name) return '—';
      return label(u,name) + (run.methodEvidence === 'loadout' ? '<small>' + label(u,'По оснащению') + '</small>' : '');
    };
    const rows = [
      ['Место добычи', run => u.esc(place(run) || '—')],
      ['Корабль', run => u.esc(run.ship || '—')],
      ['Способ добычи', methodsCell],
      ['Активная добыча', run => duration(u, run.activeSeconds)],
      ['От отстыковки до стыковки', run => duration(u,run.tripSeconds)],
      ['Добыто, т', run => number(u,run.refinedTons)],
      ['Собрано сырья, ед.', run => number(u,run.materialUnits)],
      ['Темп добычи, т/ч', run => number(u,run.activeTonsPerHour)],
      ['Темп за весь рейс, т/ч', run => number(u,run.tripTonsPerHour)],
      ['Продано из этого захода', run => markedTons(run,'soldTons')],
      ['Подтверждённая выручка', confirmed],
      ['Остаток добытого груза', run => markedTons(run,'remainingTons')],
      ['Из остатка на флотоносце', run => markedTons(run,'carrierRemainingTons')],
      ['Оценка остатка', run => {
        if (!known(run.estimatedRemainingValue)) return '—';
        const partial = run.originUncertain || run.pricedRemainingTons < run.remainingTons;
        return (partial ? '≥ ' : '≈ ') + number(u,run.estimatedRemainingValue,' кр.');
      }]
    ];
    const uncertain = runs.some(run => run.originUncertain || run.unpricedSoldTons > 0 || run.remainingTons > (run.pricedRemainingTons ?? 0));
    const note = uncertain ? `<p class="info-note">${label(u,'Для части груза неизвестно происхождение или цена. В отчёте показана только подтверждаемая часть.')}</p>` : '';
    return `<section class="panel mining-compare-panel"><h2>${label(u,'Результаты заходов')}</h2>
      ${table(u, rows.filter(([title]) => title !== 'Из остатка на флотоносце' || runs.some(run => run.carrierRemainingTons > 0)), runs,'Результаты заходов')}
      <p class="info-note">${label(u,'Активное время — от первой до последней добычной записи.')}</p>${note}</section>`;
  }

  function prospecting(u, runs, selected) {
    const resources = new Map();
    for (const run of runs) for (const resource of run.composition || []) resources.set(resource.key,resource.name);
    if (!resources.size) return '';
    const resource = resources.has(selected) ? selected : resources.keys().next().value;
    const data = run => run.composition?.find(row => row.key === resource);
    const percent = value => known(value) ? number(u,value) + '%' : '—';
    const rows = [
      ['Разведано астероидов', run => number(u,run.prospectScans)],
      ['Частота находок', run => percent(run.prospectScans > 0 ? data(run)?.occurrencePercent ?? 0 : null)],
      ['Среднее содержание по измеренным находкам', run => percent(data(run)?.averagePercent)],
      ['Среднее содержание по всем сканам', run => percent(run.prospectScans > 0 ? (data(run) ? data(run).averageAllScansPercent : 0) : null)]
    ];
    return `<section class="panel mining-compare-panel"><header><h2>${label(u,'Разведка астероидов')}</h2>
      <label>${label(u,'Ресурс')} <select data-mining-compare-resource>
      ${[...resources].map(([key,name]) => `<option value="${u.esc(key)}"${key===resource?' selected':''}>${label(u,name || key)}</option>`).join('')}
      </select></label></header>${table(u, rows, runs,'Разведка астероидов')}</section>`;
  }

  function render(C,u) {
    const mining = C.state.mining || {};
    const runs = [mining.current,...(mining.history || [])].filter(run => run?.id);
    const controls = u.heading(u.t('Сравнение заходов')) + u.renderModes(true);
    if (!runs.length) return controls + `<div class="panel empty-state"><strong>${label(u,'Добычных заходов пока нет')}</strong></div>`;
    const state = selection(C,runs);
    const chosen = runs.filter(run => state.ids.includes(run.id));
    const choices = runs.map((run,index) => {
      const checked = state.ids.includes(run.id);
      const location = place(run);
      return `<label class="mining-compare-choice" for="mining-run-${index}">
        <input type="checkbox" id="mining-run-${index}" data-mining-compare-run="${u.esc(run.id)}"${checked?' checked':state.ids.length>=limit?' disabled':''}>
        <span><strong>${date(u,run.startedAt)}</strong>${location?'<small>'+u.esc(location)+'</small>':''}</span>
        <span>${number(u,run.refinedTons,' т')}</span></label>`;
    }).join('');
    return controls + `<div data-mining-comparison><section class="panel mining-compare-panel"><header>
      <h2>${label(u,'Добычные заходы')}</h2><span>${label(u,'До четырёх заходов')}</span></header>
      <div class="mining-compare-choices">${choices}</div></section>
      ${chosen.length ? report(u,chosen)+prospecting(u,chosen,state.resource) : `<div class="panel empty-state"><strong>${label(u,'Выберите заходы для сравнения')}</strong></div>`}
      </div>`;
  }

  function init(C,u) {
    const root = active(C) && u.$('[data-mining-comparison]');
    if (!root || bound.has(root)) return;
    bound.add(root);
    root.addEventListener('change',event => {
      const node = event.target, id = node.dataset?.miningCompareRun;
      if (id && node.checked && C.miningCompare.ids.length < limit) C.miningCompare.ids.push(id);
      else if (id && !node.checked) C.miningCompare.ids = C.miningCompare.ids.filter(value => value !== id);
      else if (node.matches('[data-mining-compare-resource]')) C.miningCompare.resource = node.value;
      else return;
      u.renderPage();
      u.$(node.id ? '#' + node.id : '[data-mining-compare-resource]')?.focus({preventScroll:true});
    });
  }

  window.MiningComparisonUI = {active,render,init};
})();
