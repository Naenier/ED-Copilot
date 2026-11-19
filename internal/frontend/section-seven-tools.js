(() => {
  'use strict';

  const views = {'notifications': 'notifications.view', 'second-screen': 'second-screen.view'};
  const modes = {navigation: 'Навигация', expedition: 'Экспедиция', engineering: 'Инженерия', colonization: 'Колонизация'};
  const active = C => Object.hasOwn(views, C.tab);
  const scope = C => C.state.planWorkspace?.key || '';
  const local = C => {
    if (!C.sectionSevenTools || C.sectionSevenTools.tab !== C.tab || C.sectionSevenTools.owner !== scope(C)) {
      C.sectionSevenTools = {tab: C.tab, owner: scope(C), data: null, fetched: 0, busy: false, request: 0, groups: new Map()};
    }
    return C.sectionSevenTools;
  };
  const options = (u, pairs, value) => pairs.map(([key, label]) =>
    `<option value="${u.esc(key)}" ${String(value) === key ? 'selected' : ''}>${u.esc(u.t(label))}</option>`).join('');
  const date = (value, u) => value && new Date(value).getFullYear() > 1900 ?
    new Intl.DateTimeFormat(u.locale(), {dateStyle: 'short', timeStyle: 'short'}).format(new Date(value)) : '';
  const checked = value => value ? 'checked' : '';
  const button = (u, label, action, icon, attr = '') => u.btn(label, 'section-seven-' + action, icon, '', attr);

  const eventTabs = new Set(['carrier','ship-route','riches','deals','colonization','engineering','exploration','exobiology','overview','targets']);
  function notifications(data, C, u, session) {
    const p = data.preferences;
    const en = C.state.settings?.language === 'en';
    const groups = new Map();
    for (const event of data.catalog || []) {
      if (!Object.hasOwn(p.rules,event.id)) continue;
      if (!groups.has(event.group)) groups.set(event.group,[]);
      groups.get(event.group).push(event);
    }
    const field = (name, value) => `<input id="s7-notification-${u.esc(name)}" name="${u.esc(name)}" type="checkbox" ${checked(value)}>`;
    const choices = [['1', 'Обычный'], ['2', 'Высокий'], ['3', 'Срочный']];
    const rules = [...groups].map(([group,events],index) => {
      const rows = events.map(event => {
        const id = event.id, rule = p.rules[id];
        const title = en ? event.englishTitle || event.title : event.title;
        const description = en ? event.englishDescription || event.description : event.description;
        return `<div class="s7-rule">
          <label class="s7-rule-name">${field(id+'-enabled',rule.enabled)} <span><strong>${u.esc(title)}</strong>
            ${description ? `<small>${u.esc(description)}</small>` : ''}</span></label>
          <label>${field(id+'-visual',rule.visual)} ${u.esc(u.t('Оверлей'))}</label>
          <label>${field(id+'-sound',rule.sound)} ${u.esc(u.t('Звук'))}</label>
          <label class="s7-priority"><span class="s7-label">${u.esc(u.t('Приоритет'))}</span>
            <select id="s7-notification-${u.esc(id)}-priority" name="${u.esc(id)}-priority">${options(u, choices, rule.priority)}</select>
          </label>
        </div>`;
      }).join('');
      return `<details class="s7-rule-group" data-notification-group="${u.esc(group)}" ${(session.groups.get(group) ?? (index === 0)) ? 'open' : ''}>
        <summary>${u.esc(en ? events[0].englishGroup || group : group)}</summary>${rows}</details>`;
    }).join('');
    const history = data.events || [];
    const events = history.slice(0, 40).map((event, index) => {
      const title = (en ? event.englishTitle : event.title) || event.title;
      const detail = (en ? event.englishDetail : event.detail) || event.detail || '';
      const location = event.body || event.system;
      const details = location && !detail.split(' · ').includes(location) ?
        [detail,location].filter(Boolean).join(' · ') : detail;
      return `<div class="s7-event"><div><strong>${u.esc(title)}</strong>
        ${details ? `<div class="s7-label">${u.esc(details)}</div>` : ''}</div>
        <span class="s7-label">${u.esc(date(event.observedAt, u))}</span>
        ${eventTabs.has(event.link) ? button(u, 'Открыть раздел', 'event', 'arrow-right', `data-index="${index}"`) : ''}</div>`;
    }).join('');
    return `<form id="s7-notifications" class="panel s7-panel">
      <div class="s7-notification-controls">
        <label class="s7-enable">${field('enabled',p.enabled)} ${u.esc(u.t('Включить уведомления'))}</label>
        <label class="s7-enable">${field('sound',p.sound)} ${u.esc(u.t('Звуки уведомлений'))}</label>
        ${button(u,'Прослушать сигнал','sound-preview','bell')}
      </div>
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
    const description = C.tab === 'notifications' ? 'Выберите события для оверлея и звуковых сигналов.' : 'Откройте выбранные панели на телефоне или другом компьютере в вашей сети.';
    return u.heading(u.t(title), u.t(description)) + `<div class="s7-tools">${s.error ?
      `<p role="alert">${u.esc(s.error)}</p>${button(u, 'Повторить', 'retry', 'refresh-cw')}` : s.data ?
        (C.tab === 'notifications' ? notifications(s.data, C, u, s) : screens(s.data, u)) :
        `<p role="status">${u.esc(u.t('Загрузка…'))}</p>`}</div>`;
  }

  function init(C, u) {
    if (!active(C)) return;
    const s = local(C);
    const form = s.tab === 'notifications' && u.$('#s7-notifications');
    if (form && !form.s7RulesBound) {
      form.s7RulesBound = true;
      form.addEventListener('change', () => { s.dirty = true; });
      form.addEventListener('toggle', event => {
        const group = event.target.dataset?.notificationGroup;
        if (group) s.groups.set(group,event.target.open);
      },true);
    }
    if (s.busy || s.saving || s.dirty || Date.now() - s.fetched < 4000 || s.error) return;
    s.fetched = Date.now();
    s.busy = true;
    const request = ++s.request;
    void u.call(views[s.tab], {expectedPlanWorkspace: s.owner}).then(data => {
      if (!active(C) || local(C) !== s || s.request !== request || s.dirty) return;
      if (JSON.stringify(data) !== JSON.stringify(s.data)) {
        s.data = data;
        u.renderPage({preserveContent: true});
      }
    }).catch(error => {
      if (active(C) && local(C) === s && s.request===request && !s.dirty) {
        s.error = error.message;
        u.renderPage({preserveContent: true});
      }
    }).finally(() => { s.busy = false; });
  }

  async function update(C, u, values) {
    const s = local(C);
    const request = ++s.request;
    const op = s.tab === 'notifications' ? 'notifications.update' : 'second-screen.update';
    const result = await u.call(op, {...values, expectedPlanWorkspace: s.owner});
    if (active(C) && local(C) === s && s.request === request) {
      s.dirty = false;
      s.data = result;
      s.fetched = Date.now();
      u.renderPage({preserveContent: true});
    }
  }

  function preferences(fields, catalog) {
    const rules = Object.fromEntries(catalog.map(({id}) => [id, {
      enabled: fields.has(id + '-enabled'), visual: fields.has(id + '-visual'),
      sound: fields.has(id + '-sound'), priority: Number(fields.get(id + '-priority'))
    }]));
    return {enabled: fields.has('enabled'), sound: fields.has('sound'), rules};
  }

  async function submit(form, C, u) {
    const fields = new FormData(form);
    if (form.id === 's7-screens') {
      return update(C, u, {action: 'start', host: fields.get('host'), modes: fields.getAll('modes')});
    }
    const s = local(C);
    if (s.saving) return;
    s.saving = true;
    const controls = [...form.elements].filter(control => !control.disabled);
    for (const control of controls) control.disabled = true;
    try {
      await update(C, u, {preferences: preferences(fields,s.data?.catalog || [])});
      if (active(C) && local(C)===s) u.toast(u.t('Настройки сохранены'));
    } finally {
      s.saving = false;
      for (const control of controls) control.disabled = false;
    }
  }

  async function handle(action, control, C, u) {
    const s = local(C);
    const verb = action.slice('section-seven-'.length);
    if (verb === 'retry') {
      s.error = '';
      s.fetched = 0;
      init(C, u);
    } else if (verb === 'sound-preview') {
      await u.call('native',{action:'notification-sound-preview'});
    } else if (verb === 'event') {
      const event = s.data?.events?.[Number(control.dataset.index)];
      if (!eventTabs.has(event?.link)) return;
      const modes = {exploration:'Находки', engineering:'План', overview:'Миссии'};
      const mode = ['engineerInvited','engineerUnlocked'].includes(event.kind) ? 'Прокачка' : modes[event.link] || '';
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
