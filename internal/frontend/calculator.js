/* Ownership-cost UI. All figures and forecasts are calculated by Go. */
(() => {
  'use strict';
  const types = [
    { id: 'fleet', name: 'Личный флотоносец', english: 'Drake Class · Fleet Carrier', icon: 'ship', detail: 'Ваш собственный мобильный порт и склад.' },
    { id: 'squadron', name: 'Флотоносец эскадрильи', english: 'Javelin Class · Squadron Carrier', icon: 'layers', detail: 'Совместное владение и бюджет эскадрильи.' }
  ];
  const fields = [
    { key: 'jumpsPerWeek', label: 'Прыжков за неделю', required: true, integer: true, max: 10000, help: 'Обслуживание каждого прыжка считается отдельно от трития.' },
    { key: 'reserveWeeks', label: 'Резерв содержания, недель', required: true, integer: true, max: 520, help: 'На сколько недель вы хотите заранее отложить средства.' },
    { key: 'cargoTons', label: 'Другие грузы и пакеты, т', required: true, max: 1000000, help: 'Товары, тритий в трюме, пакеты кораблей и модулей. Без отдельного топливного бака.' },
    { key: 'bankBalance', label: 'Уже в банке флотоносца, кр.', integer: true, max: 1e15, help: 'Необязательно. Покажем, на сколько полных недель хватит этого остатка.' },
    { key: 'purchaseBudget', label: 'Ваш бюджет покупки и резерва, кр.', integer: true, max: 1e15, help: 'Необязательно. Сравним его со стоимостью покупки и выбранным резервом.' },
    { key: 'tritiumPerJump', label: 'Средний расход трития, т/прыжок', max: 1000, help: 'Ваша оценка. Для конкретного перелёта используйте маршрут флотоносца.' },
    { key: 'tritiumPrice', label: 'Цена трития, кр./т', integer: true, max: 1e9, help: 'Пустое поле означает неизвестную цену. Это не бесплатное топливо.' }
  ];
  const serviceIcons = { refuel: 'circle', repair: 'wrench', rearm: 'package', voucherredemption: 'wallet', shipyard: 'ship', outfitting: 'wrench', blackmarket: 'package', exploration: 'compass', bartender: 'circle', vistagenomics: 'circle', pioneersupplies: 'package' };
  let mutation = Promise.resolve();
  let latest = null;
  let listenersReady = false;
  const known = value => value !== null && value !== undefined && value !== '' && Number.isFinite(Number(value));
  const current = C => C.state?.carrierCalculator || {};
  const typeID = C => current(C).selectedType || 'fleet';
  const valueText = value => known(value) ? String(value) : '';
  const exactMoney = (value, ui) => known(value) ? ui.fmt(value) + ' кр.' : '—';
  const run = task => { const result = mutation.then(task); mutation = result.catch(() => {}); return result; };

  function draft(C, kind = typeID(C)) {
    C.calculatorDrafts ||= {};
    const stored = C.calculatorDrafts[kind];
    if (stored?.dirty) return stored;
    const options = kind === typeID(C) ? current(C).options || {} : current(C).settings?.plans?.[kind] || {};
    const record = { dirty: false, values: Object.fromEntries(fields.map(field => [field.key, valueText(options[field.key])])) };
    C.calculatorDrafts[kind] = record;
    return record;
  }
  function metric(ui, label, value, foot, icon = '') {
    return `<div class="metric"><div class="metric-label">${icon ? ui.icon(icon) : ''}${ui.esc(label)}</div><div class="metric-value">${ui.esc(value)}</div><div class="metric-foot">${ui.esc(foot || '')}</div></div>`;
  }
  function modes(C, ui) {
    const selected = C.mode || 'Сводка';
    return `<div class="calc-mode-row"><div class="segmented" aria-label="Экраны калькулятора">${[['Сводка', 'Конфигурация'], ['Службы', 'Службы'], ['Сравнение', 'Сравнение типов']].map(([value, label]) => `<button type="button" data-mode="${value}" class="${selected === value ? 'selected' : ''}" aria-pressed="${selected === value}">${label}</button>`).join('')}</div><span class="spacer"></span><span class="calc-local-label">${ui.icon('shield-check')}Личный план · сохраняется на этом компьютере</span></div>`;
  }
  function typeCards(C, ui) {
    const calculator = current(C);
    const comparison = calculator.comparison || [];
    return `<div class="carrier-type">${types.map(type => {
      const totals = comparison.find(item => item.carrierType === type.id) || {};
      const selected = type.id === typeID(C);
      return `<button type="button" class="type-card ${selected ? 'selected' : ''}" data-click="calc-type" data-id="${type.id}" aria-pressed="${selected}"><span class="calc-type-top">${ui.icon(type.icon)}${selected ? ui.badge('Выбран', 'blue') : '<span class="calc-type-subtle">Отдельный план</span>'}</span><h3>${type.name}</h3><p>${type.english}</p><div class="calc-type-price" title="${ui.esc(exactMoney(totals.hullBuyPrice, ui))}">${ui.esc(ui.money(totals.hullBuyPrice))}<small>корпус</small></div><div class="calc-type-bottom"><span>${ui.esc(ui.fmt(totals.capacityTons))} т общей вместимости</span>${ui.icon('arrow-right')}</div></button>`;
    }).join('')}</div>`;
  }
  function dirtyNotice(C, ui) {
    const dirty = draft(C).dirty;
    return `<div class="calc-unsaved ${dirty ? '' : 'hidden'}" id="calc-unsaved" role="status">${ui.icon('circle')}<span>Есть несохранённые параметры. Итоги пока рассчитаны по последним сохранённым значениям.</span>${ui.btn('Сохранить', 'calc-save', '', 'quiet', 'type="button"')}</div>`;
  }
  function controls(C, ui) {
    const record = draft(C);
    return `<section class="panel calc-settings" id="calc-settings"><div class="calc-section-title"><div><h2>Ваш сценарий использования</h2><p>Задайте график полётов и бюджет. Каждый тип хранит свой набор параметров.</p></div>${ui.icon('settings')}</div><form id="calculator-inputs" novalidate><div class="calculator-inputs">${fields.map(field => `<label class="calc-input-label"><span>${ui.esc(field.label)}${field.required ? '' : '<small>необязательно</small>'}</span><input type="text" inputmode="decimal" data-calc-input="${field.key}" name="${field.key}" value="${ui.esc(record.values[field.key])}" placeholder="${field.required ? '0' : 'Неизвестно'}" autocomplete="off" aria-describedby="calc-help-${field.key}"><small id="calc-help-${field.key}">${ui.esc(field.help)}</small></label>`).join('')}</div><div class="calc-form-error hidden" id="calc-form-error" role="alert"></div><div class="calc-input-actions"><span>Содержание не зависит от того, заходите ли вы в игру.</span>${ui.btn('Сохранить параметры', 'calc-save', 'check', 'primary', 'type="button"')}</div></form></section>`;
  }
  function services(C, ui) {
    const calculator = current(C);
    const rows = calculator.services || [];
    const summary = calculator.summary || {};
    return `<section class="calc-services"><div class="section-heading"><div><h2>Службы флотоносца</h2><p class="calc-section-description">Выберите только нужное. Приостановка уменьшает содержание, но не освобождает объём.</p></div><div class="calc-service-tools">${ui.btn('Все работают', 'calc-all-active', '', 'quiet', 'type="button"')}${ui.btn('Приостановить', 'calc-suspend-installed', '', 'quiet', 'type="button"')}${ui.btn('Снять', 'calc-remove-services', '', 'quiet', 'type="button"')}</div></div><div class="table-panel"><div class="table-scroll"><table class="calc-module-table"><thead><tr><th>Служба</th><th>Состояние</th><th>В плане, кр.</th><th>За неделю, кр.</th><th>Объём, т</th></tr></thead><tbody><tr class="calc-core-row"><td><span class="calc-service-name">${ui.icon('ship')}Основные службы</span></td><td>${ui.badge('Обязательные')}</td><td title="Стоимость корпуса">${ui.esc(ui.fmt(summary.hullBuyPrice))}</td><td>${ui.esc(ui.fmt(summary.baseWeeklyUpkeep))}</td><td>—</td></tr>${rows.map(service => {
      const mandatory = !!service.mandatory;
      const state = service.state || 'off';
      const info = mandatory ? 'Встроенная верфь: бесплатно, постоянно занимает 3 000 т.' : `Установка ${exactMoney(service.installationPrice, ui)}; работает ${exactMoney(service.activeWeeklyUpkeep, ui)}/нед.; приостановлена ${exactMoney(service.suspendedWeeklyUpkeep, ui)}/нед.`;
      return `<tr class="${state === 'off' ? 'calc-uninstalled' : ''}"><td title="${ui.esc(info)}"><span class="calc-service-name">${ui.icon(serviceIcons[service.key] || 'circle')}<span>${ui.esc(service.name)}${mandatory ? '<small>Включена в стоимость корпуса</small>' : ''}</span></span></td><td>${mandatory ? `<span class="calc-mandatory">${ui.icon('check')}${ui.badge('Встроена · работает', 'blue')}</span>` : `<select class="module-select" data-calc-service="${ui.esc(service.key)}" aria-label="${ui.esc(service.name)}: состояние">${[['off', 'Не установлена'], ['active', 'Работает'], ['suspended', 'Приостановлена']].map(([value, label]) => `<option value="${value}" ${state === value ? 'selected' : ''}>${label}</option>`).join('')}</select>`}</td><td>${ui.esc(ui.fmt(service.buyPrice))}</td><td>${ui.esc(ui.fmt(service.weeklyUpkeep))}</td><td>${ui.esc(ui.fmt(service.capacityTons))}</td></tr>`;
    }).join('')}</tbody></table></div><div class="calc-table-foot">Цены установки видны в подсказке названия службы. Неустановленные службы не включаются в итог.</div></div></section>`;
  }
  function summaryPanel(C, ui) {
    const calculator = current(C), s = calculator.summary || {};
    let budget = `<div class="calc-budget-note"><strong>Оцените стартовый бюджет</strong><p>Укажите доступную сумму в параметрах — сравним её с покупкой и резервом.</p>${ui.btn('Задать бюджет', 'calc-config', 'wallet', 'quiet', 'type="button"')}</div>`;
    if (known(s.purchaseBudget)) {
      if (s.affordable === true) budget = `<div class="calc-budget-note good">${ui.badge('Бюджета достаточно', 'good')}<p>Для выбранной покупки и резерва. Товарные запасы и пакеты приобретаются отдельно.</p></div>`;
      else if (s.affordable === false) budget = `<div class="calc-budget-note warn">${ui.badge('Нужен больший бюджет', 'warn')}<strong>${ui.esc(exactMoney(known(s.budgetGap) ? s.budgetGap : s.minimumBudgetGap, ui))}</strong><p>${known(s.budgetGap) ? 'Не хватает до покупки и полного резерва.' : 'Не хватает как минимум. Расходы на топливо пока неизвестны.'}</p></div>`;
      else budget = `<div class="calc-budget-note warn">${ui.badge('Добавьте оценку топлива', 'warn')}<p>На покупку и минимальное содержание средств хватает. Для полного сравнения нужны расход и цена трития.</p></div>`;
    }
    const wholeWeeks = known(s.bankWholeWeeksWithFuel) ? s.bankWholeWeeksWithFuel : s.bankWholeWeeksWithoutFuel;
    return `<aside class="panel calculator-summary calc-total-panel"><div class="calc-total-title"><span class="eyebrow">Итог вашего плана</span><h3>${ui.esc(calculator.name || 'Флотоносец')}</h3></div>${metric(ui, 'Покупка корпуса и служб', ui.money(s.totalBuyPrice), exactMoney(s.totalBuyPrice, ui), 'wallet')}${metric(ui, 'Содержание за неделю', exactMoney(s.weeklyUpkeep, ui), 'Включая обслуживание прыжков, без трития', 'clock')}${metric(ui, 'В неделю с топливом', exactMoney(s.weeklyCostWithFuel, ui), s.fuelCostKnown ? 'По вашей оценке расхода и цены' : 'Укажите расход и цену трития', 'route')}${metric(ui, 'Свободная вместимость', ui.fmt(s.availableCargoTons) + ' т', 'После служб и других грузов', 'package')}<div class="calc-reserve"><div><span>Резерв на ${ui.esc(ui.fmt(s.reserveWeeks))} нед.</span><strong>${ui.esc(exactMoney(s.recommendedBankReserve, ui))}</strong></div><small>Без топлива — ${ui.esc(exactMoney(s.minimumBankReserve, ui))}</small><div><span>Банк обеспечит</span><strong>${known(wholeWeeks) ? ui.esc(ui.fmt(wholeWeeks)) + ' нед.' : '—'}</strong></div><small>${known(s.bankWholeWeeksWithFuel) ? 'Полные недели с учётом топлива' : 'По содержанию, без покупки трития'}</small></div>${budget}<p class="info-note">Это личный расчёт расходов. Выбранные службы и деньги в игре не изменяются.</p></aside>`;
  }
  function horizons(C, ui) {
    const s = current(C).summary || {};
    return `<div class="calc-period-grid">${metric(ui, '30 дней содержания', exactMoney(s.monthlyUpkeep, ui), 'Пропорциональная оценка · без трития')}${metric(ui, '365 дней содержания', exactMoney(s.yearlyUpkeep, ui), 'Пропорциональная оценка · без трития')}${metric(ui, 'Покупка и полный резерв', ui.money(s.requiredBudget), known(s.requiredBudget) ? exactMoney(s.requiredBudget, ui) : 'Для полного итога нужна оценка топлива')}</div>`;
  }
  function comparison(C, ui) {
    const rows = current(C).comparison || [];
    const keys = [
      ['Стоимость корпуса', 'hullBuyPrice', 'money'], ['Покупка корпуса и служб', 'totalBuyPrice', 'money'], ['Прыжков в неделю', 'jumpsPerWeek', 'number'], ['Содержание за неделю', 'weeklyUpkeep', 'money'], ['За неделю с топливом', 'weeklyCostWithFuel', 'money'], ['30 дней содержания', 'monthlyUpkeep', 'money'], ['365 дней содержания', 'yearlyUpkeep', 'money'], ['Общая вместимость', 'capacityTons', 'tons'], ['Занято службами', 'servicesCapacityTons', 'tons'], ['Другие грузы и пакеты', 'cargoTons', 'tons'], ['Свободно', 'availableCargoTons', 'tons'], ['Резерв, недель', 'reserveWeeks', 'number'], ['Покупка и полный резерв', 'requiredBudget', 'money']
    ];
    const cell = (row, key, kind) => kind === 'money' ? exactMoney(row?.[key], ui) : ui.fmt(row?.[key]) + (kind === 'tons' ? ' т' : '');
    return `<section class="panel calc-compare-intro"><h2>Сравните два независимых сценария</h2><p>Здесь показаны сохранённые конфигурации. У каждого типа свои службы, грузы, график прыжков и бюджет. Для равного сравнения задайте одинаковые условия в обоих планах.</p></section><div class="table-panel calc-compare"><div class="table-scroll"><table><thead><tr><th>Показатель</th>${types.map(type => `<th>${ui.esc(type.name)}</th>`).join('')}</tr></thead><tbody>${keys.map(([label, key, format]) => `<tr><td>${ui.esc(label)}</td>${types.map(type => `<td>${ui.esc(cell(rows.find(row => row.carrierType === type.id), key, format))}</td>`).join('')}</tr>`).join('')}</tbody><tfoot><tr><td></td>${types.map(type => `<td>${ui.btn('Открыть план', 'calc-type', 'arrow-right', 'quiet', `type="button" data-id="${type.id}"`)}</td>`).join('')}</tr></tfoot></table></div></div>`;
  }
  function assumptions(C, ui) {
    const calculator = current(C);
    return `<details class="calc-assumptions"><summary>${ui.icon('circle-help')}Как считаются расходы</summary><div>${(calculator.notes || []).map(note => `<p>${ui.esc(note)}</p>`).join('')}<div class="calc-source">${ui.btn('Проверить данные на PTN', 'calc-source', 'external-link', 'quiet', 'type="button"')}<span>Параметры проверены ${ui.esc(calculator.source?.checkedAt || '—')}</span></div></div></details>`;
  }
  function render(C, ui) {
    const calculator = current(C), s = calculator.summary || {};
    const extra = ui.btn('Скопировать итог', 'calc-copy', 'copy', 'quiet', 'type="button"') + ui.btn('Сбросить план', 'calc-reset-confirm', 'refresh-cw', 'quiet', 'type="button"');
    const lead = ui.heading('Калькулятор флотоносца', 'Выберите тип, настройте службы и оцените покупку, содержание и запас средств.', extra) + modes(C, ui) + typeCards(C, ui) + dirtyNotice(C, ui);
    const warning = Number(s.overCapacityTons || 0) > 0 ? `<div class="notice" role="status">${ui.icon('package')} Грузы превышают доступную вместимость на <strong>${ui.esc(ui.fmt(s.overCapacityTons))} т</strong>. Уменьшите загрузку или снимите ненужные службы. Приостановка объём не освобождает.</div>` : '';
    if (C.mode === 'Сравнение') return `<div class="calc-root">${lead}${comparison(C, ui)}${assumptions(C, ui)}</div>`;
    return `<div class="calc-root">${lead}${warning}<div class="calculator-layout"><div class="calc-main">${C.mode === 'Службы' ? '' : controls(C, ui) + horizons(C, ui)}${services(C, ui)}${assumptions(C, ui)}<div class="calc-bottom-link">${ui.icon('route')}Для дальнего перелёта с расчётом расхода по остановкам используйте ${ui.btn('Маршрут флотоносца', 'calc-route', '', 'quiet', 'type="button"')}.</div></div>${summaryPanel(C, ui)}</div></div>`;
  }

  function parseInput(field, raw) {
    const text = String(raw ?? '').trim().replace(/[\s\u00a0\u202f]/g, '').replace(/−/g, '-').replace(',', '.');
    if (text === '') {
      if (!field.required) return null;
      throw new Error('Заполните поле «' + field.label + '».');
    }
    if (!/^-?\d+(?:\.\d+)?$/.test(text)) throw new Error(field.label + ': укажите число. Разряды можно разделять пробелами.');
    const number = Number(text);
    if (!Number.isFinite(number) || number < 0 || number > field.max || field.integer && !Number.isInteger(number)) throw new Error(field.label + ': ' + (field.integer ? 'целое число' : 'число') + ' от 0 до ' + new Intl.NumberFormat('ru-RU').format(field.max) + '.');
    return number;
  }
  function save(C, ui) {
    const kind = typeID(C), record = draft(C, kind);
    for (const element of ui.$$('[data-calc-input]')) record.values[element.dataset.calcInput] = element.value;
    let values;
    try { values = Object.fromEntries(fields.map(field => [field.key, parseInput(field, record.values[field.key])])); }
    catch (error) {
      const box = ui.$('#calc-form-error');
      if (box) { box.textContent = error.message; box.classList.remove('hidden'); }
      throw error;
    }
    return run(async () => {
      await ui.call('calculator', { values: { action: 'configure', carrierType: kind, ...values } });
      delete C.calculatorDrafts[kind];
      await ui.loadView();
      ui.toast('Параметры расчёта сохранены');
    });
  }
  async function handle(action, button, C, ui) {
    if (!action.startsWith('calc-')) return false;
    if (action === 'calc-save') { await save(C, ui); return true; }
    if (action === 'calc-type') {
      const kind = button?.dataset.id;
      if (!types.some(type => type.id === kind)) throw new Error('Тип флотоносца не найден.');
      await run(async () => { await ui.call('calculator', { values: { action: 'select', carrierType: kind } }); await ui.loadView(); });
      return true;
    }
    if (action === 'calc-config' || action === 'calc-compare') {
      C.mode = action === 'calc-config' ? 'Сводка' : 'Сравнение';
      await ui.loadView();
      if (action === 'calc-config') ui.$('#calc-settings')?.scrollIntoView({ block: 'start', behavior: 'smooth' });
      return true;
    }
    if (action === 'calc-reset-confirm') {
      const cal = current(C);
      ui.dialog('Сбросить этот план?', `<p class="prose-small">${ui.esc(cal.name)}: службы, график прыжков и бюджет вернутся к начальным значениям. План второго типа сохранится.</p>`, ui.btn('Отмена', 'close-dialog') + ui.btn('Сбросить план', 'calc-reset', '', 'primary', `data-id="${ui.esc(typeID(C))}"`));
      return true;
    }
    if (action === 'calc-reset') {
      const kind = button?.dataset.id || typeID(C);
      await run(async () => { await ui.call('calculator', { values: { action: 'reset', carrierType: kind } }); if (C.calculatorDrafts) delete C.calculatorDrafts[kind]; ui.closeLayer(); await ui.loadView(); ui.toast('План сброшен'); });
      return true;
    }
    if (['calc-all-active', 'calc-suspend-installed', 'calc-remove-services'].includes(action)) {
      const kind = typeID(C), services = {};
      for (const service of current(C).services || []) {
        if (service.mandatory || action === 'calc-suspend-installed' && service.state === 'off') continue;
        services[service.key] = action === 'calc-all-active' ? 'active' : action === 'calc-suspend-installed' ? 'suspended' : 'off';
      }
      await run(async () => { await ui.call('calculator', { values: { action: 'configure', carrierType: kind, services } }); await ui.loadView(); });
      return true;
    }
    if (action === 'calc-route') { await ui.navigate('route'); return true; }
    if (action === 'calc-source') { await ui.call('native', { action: 'open', url: current(C).source?.url || 'https://pilotstradenetwork.com/tools/carrier-calculator/' }); return true; }
    if (action === 'calc-copy') {
      const cal = current(C), s = cal.summary || {};
      const text = [cal.name, 'Покупка: ' + exactMoney(s.totalBuyPrice, ui), 'Содержание в неделю, без топлива: ' + exactMoney(s.weeklyUpkeep, ui), 'За неделю с топливом: ' + exactMoney(s.weeklyCostWithFuel, ui), 'Свободная вместимость: ' + ui.fmt(s.availableCargoTons) + ' т', 'Резерв на ' + ui.fmt(s.reserveWeeks) + ' недель: ' + exactMoney(s.recommendedBankReserve, ui), 'Покупка и полный резерв: ' + exactMoney(s.requiredBudget, ui)].join('\r\n');
      await ui.call('native', { action: 'copy', text }); ui.toast('Итог расчёта скопирован'); return true;
    }
    return false;
  }
  function change(serviceId, value, C, ui) {
    const kind = typeID(C);
    const service = (current(C).services || []).find(item => item.key === serviceId);
    if (!service || service.mandatory || !['off', 'active', 'suspended'].includes(value)) return Promise.reject(new Error('Состояние этой службы нельзя изменить.'));
    return run(async () => { await ui.call('calculator', { values: { action: 'service', carrierType: kind, serviceKey: serviceId, state: value } }); await ui.loadView(); });
  }
  function init(C, ui) {
    latest = { C, ui };
    if (listenersReady) return;
    listenersReady = true;
    document.addEventListener('input', event => {
      const input = event.target.closest?.('[data-calc-input]');
      if (!input || !latest) return;
      const { C, ui } = latest, record = draft(C);
      record.values[input.dataset.calcInput] = input.value;
      record.dirty = true;
      ui.$('#calc-unsaved')?.classList.remove('hidden');
    });
    document.addEventListener('submit', event => {
      if (event.target.id !== 'calculator-inputs' || !latest) return;
      event.preventDefault();
      save(latest.C, latest.ui).catch(error => latest.ui.toast(error.message, true));
    });
  }
  async function smoke(C, ui) {
    const calculator = current(C), html = render({ ...C, mode: 'Сводка' }, ui);
    return [
      { name: 'Калькулятор: два типа флотоносца', ok: types.every(type => html.includes(`data-id="${type.id}"`)) },
      { name: 'Калькулятор: отдельные сохранённые планы', ok: !!calculator.settings?.plans?.fleet && !!calculator.settings?.plans?.squadron },
      { name: 'Калькулятор: бюджет и резерв', ok: html.includes('reserveWeeks') && html.includes('purchaseBudget') },
      { name: 'Калькулятор: встроенные службы не редактируются', ok: !(calculator.services || []).some(service => service.mandatory && html.includes(`data-calc-service="${service.key}"`)) }
    ];
  }
  window.CalculatorUI = { render, handle, change, init, smoke };
})();
