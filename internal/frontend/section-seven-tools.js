(() => {
  'use strict';

  const views = {'notifications': 'notifications.view', 'second-screen': 'second-screen.view'};
  const categories = {finding: 'Находки', engineering: 'Материалы для инженерии', task: 'Завершение задач', carrier: 'Прыжок флотоносца', fuel: 'Мало топлива'};
  const modes = {navigation: 'Навигация', expedition: 'Экспедиция', engineering: 'Инженерия', colonization: 'Колонизация'};
  const active = C => Object.hasOwn(views, C.tab);
  const scope = C => C.state.planWorkspace?.key || '';
  const local = C => {
    if (!C.sectionSevenTools || C.sectionSevenTools.tab !== C.tab || C.sectionSevenTools.owner !== scope(C)) {
      C.sectionSevenTools = {tab: C.tab, owner: scope(C), data: null, fetched: 0, busy: false};
    }
    return C.sectionSevenTools;
  };
  const options = (u, pairs, value) => pairs.map(([key, label]) =>
    `<option value="${u.esc(key)}" ${String(value) === key ? 'selected' : ''}>${u.esc(u.t(label))}</option>`).join('');
  const date = (value, u) => value && new Date(value).getFullYear() > 1900 ?
    new Intl.DateTimeFormat(u.locale(), {dateStyle: 'short', timeStyle: 'short'}).format(new Date(value)) : '';
  const checked = value => value ? 'checked' : '';
  const button = (u, label, action, icon, attr = '') => u.btn(label, 'section-seven-' + action, icon, '', attr);

  function notifications(data, C, u) {
    const p = data.preferences;
    const rules = Object.entries(categories).map(([id, name]) => {
      const rule = p.rules[id];
      return `<div class="s7-rule">
        <label><input name="${id}-enabled" type="checkbox" ${checked(rule.enabled)}> ${u.esc(u.t(name))}</label>
        <label><input name="${id}-visual" type="checkbox" ${checked(rule.visual)}> ${u.esc(u.t('Оверлей'))}</label>
        <label class="s7-priority"><span class="s7-label">${u.esc(u.t('Приоритет'))}</span>
          <select name="${id}-priority">${options(u, [['1', 'Обычный'], ['2', 'Высокий'], ['3', 'Срочный']], rule.priority)}</select>
        </label>
      </div>`;
    }).join('');
    const en = C.state.settings?.language === 'en';
    const history = data.events || [];
    const events = history.slice(0, 40).map((event, index) => {
      const title = (en ? event.englishTitle : event.title) || event.title;
      const details = [(en ? event.englishDetail : event.detail) || event.detail, event.body || event.system].filter(Boolean).join(' · ');
      return `<div class="s7-event"><div><strong>${u.esc(title)}</strong>
        ${details ? `<div class="s7-label">${u.esc(details)}</div>` : ''}</div>
        <span class="s7-label">${u.esc(date(event.observedAt, u))}</span>
        ${event.link ? button(u, 'Открыть раздел', 'event', 'arrow-right', `data-index="${index}"`) : ''}</div>`;
    }).join('');
    return `<form id="s7-notifications" class="panel s7-panel">
      <label class="s7-enable"><input name="enabled" type="checkbox" ${checked(p.enabled)}> ${u.esc(u.t('Включить уведомления'))}</label>
      <div class="s7-rules">${rules}</div>
      <div class="s7-actions">
        <button class="pill primary" type="submit">${u.esc(u.t('Сохранить'))}</button>
      </div>
    </form>
    <section class="panel s7-panel"><h2>${u.esc(u.t('История уведомлений'))}</h2>
      ${events || `<p class="s7-label">${u.esc(u.t('Уведомлений пока нет.'))}</p>`}
      ${history.length > 40 ? `<p class="s7-label">${u.esc(u.t('Показаны 40 последних уведомлений.'))}</p>` : ''}
    </section>`;
  }

  function screens(data, u) {
    if (!data.running) {
      const addresses = (data.addresses || []).map(ip => [ip, ip]);
      return `<form id="s7-screens" class="panel s7-panel">
        <h2>${u.esc(u.t('Доступ в локальной сети'))}</h2>
        ${addresses.length ? `<label class="s7-field"><span>${u.esc(u.t('Адрес компьютера'))}</span><select name="host">${options(u, addresses, addresses[0][0])}</select></label>
        <div class="s7-actions">${Object.entries(modes).map(([id, label]) => `<label><input name="modes" value="${id}" type="checkbox" checked> ${u.esc(u.t(label))}</label>`).join('')}</div>
        <div><button class="pill primary" type="submit">${u.esc(u.t('Начать сопряжение'))}</button></div>` :
          `<p class="s7-label">${u.esc(u.t('Подключите компьютер к локальной сети.'))}</p>`}
      </form>`;
    }
    const devices = (data.devices || []).map(device => `<div class="s7-event"><strong>${u.esc(device.address)}</strong>
      <span class="s7-label">${u.esc(date(device.lastSeen, u) || date(device.pairedAt, u))}</span>
      ${button(u, 'Отозвать доступ', 'revoke', 'x', `data-id="${u.esc(device.id)}"`)}</div>`).join('');
    return `<section class="panel s7-panel"><h2>${u.esc(u.t('Сопряжение устройства'))}</h2>
      <div class="s7-event"><div><div class="s7-label">${u.esc(u.t('Откройте этот адрес на втором устройстве'))}</div>
        <strong class="s7-address">${u.esc(data.url)}</strong></div>${button(u, 'Копировать адрес', 'copy-url', 'copy')}</div>
      ${data.code ? `<div class="s7-event"><div><div class="s7-label">${u.esc(u.t('Одноразовый код'))}</div>
        <strong class="s7-code">${u.esc(data.code)}</strong></div><span class="s7-label">${u.esc(date(data.codeExpires, u))}</span>
        ${button(u, 'Копировать код', 'copy-code', 'copy')}</div>` : ''}
      <details><summary>${u.esc(u.t('Сертификат HTTPS'))}</summary>
        <p class="s7-label">${u.esc(u.t('Браузер может запросить подтверждение локального сертификата. Сверьте его отпечаток перед вводом кода.'))}</p>
        <code class="s7-address">${u.esc(data.fingerprint)}</code></details>
      <div class="s7-actions">${button(u, 'Новый код', 'rotate', 'refresh-cw')}${button(u, 'Остановить доступ', 'stop', 'x')}</div>
    </section><section class="panel s7-panel"><h2>${u.esc(u.t('Подключённые устройства'))}</h2>
      ${devices || `<p class="s7-label">${u.esc(u.t('Подключённых устройств нет.'))}</p>`}
    </section>`;
  }

  function render(C, u) {
    const s = local(C);
    const title = C.tab === 'notifications' ? 'Уведомления' : 'Второй экран';
    const description = C.tab === 'notifications' ? 'Выберите события для оверлея.' : 'Откройте выбранные панели на телефоне или другом компьютере в вашей сети.';
    return u.heading(u.t(title), u.t(description)) + `<div class="s7-tools">${s.error ?
      `<p role="alert">${u.esc(s.error)}</p>${button(u, 'Повторить', 'retry', 'refresh-cw')}` : s.data ?
        (C.tab === 'notifications' ? notifications(s.data, C, u) : screens(s.data, u)) :
        `<p role="status">${u.esc(u.t('Загрузка…'))}</p>`}</div>`;
  }

  function init(C, u) {
    if (!active(C)) return;
    const s = local(C);
    if (s.busy || Date.now() - s.fetched < 4000 || s.error) return;
    s.fetched = Date.now();
    s.busy = true;
    void u.call(views[s.tab], {expectedPlanWorkspace: s.owner}).then(data => {
      if (!active(C) || local(C) !== s) return;
      if (JSON.stringify(data) !== JSON.stringify(s.data)) {
        s.data = data;
        u.renderPage({preserveContent: true});
      }
    }).catch(error => {
      if (active(C) && local(C) === s) {
        s.error = error.message;
        u.renderPage({preserveContent: true});
      }
    }).finally(() => { s.busy = false; });
  }

  async function update(C, u, values) {
    const s = local(C);
    const op = s.tab === 'notifications' ? 'notifications.update' : 'second-screen.update';
    const result = await u.call(op, {...values, expectedPlanWorkspace: s.owner});
    if (active(C) && local(C) === s) {
      s.data = result;
      s.fetched = Date.now();
      u.renderPage({preserveContent: true});
    }
  }

  function preferences(fields) {
    const rules = Object.fromEntries(Object.keys(categories).map(id => [id, {
      enabled: fields.has(id + '-enabled'), visual: fields.has(id + '-visual'), priority: Number(fields.get(id + '-priority'))
    }]));
    return {enabled: fields.has('enabled'), rules};
  }

  async function submit(form, C, u) {
    const fields = new FormData(form);
    if (form.id === 's7-screens') {
      return update(C, u, {action: 'start', host: fields.get('host'), modes: fields.getAll('modes')});
    }
    await update(C, u, {preferences: preferences(fields)});
    u.toast(u.t('Настройки сохранены'));
  }

  async function handle(action, control, C, u) {
    const s = local(C);
    const verb = action.slice('section-seven-'.length);
    if (verb === 'retry') {
      s.error = '';
      s.fetched = 0;
      init(C, u);
    } else if (verb === 'event') {
      const event = s.data?.events?.[Number(control.dataset.index)];
      if (!event?.link) return;
      const mode = ({exploration:'Находки', engineering:'План', overview:'Миссии'})[event.link] || '';
      await u.navigate(event.link, {mode,prefill:{systemAddress:event.systemAddress,systemName:event.system,bodyId:event.bodyId}});
    } else if (verb === 'copy-url' || verb === 'copy-code') {
      const value = verb === 'copy-url' ? s.data?.url : s.data?.code;
      if (value) await u.call('native', {action: 'copy', text: value});
    } else {
      await update(C, u, {action: verb, id: control.dataset.id});
    }
  }

  function leave(C) {
    C.sectionSevenTools = null;
  }

  window.SectionSevenToolsUI = {active, render, init, submit, handle, leave};
})();
