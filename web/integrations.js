(function () {
  'use strict';

  const U = window.MiningUI;
  const { h, n, dateTime, pill } = U;
  const REPOSITORY = 'https://github.com/Naenier/ED-Copilot';
  const INARA_API_SETTINGS = 'https://inara.cz/elite/cmdr-settings-api/';
  const INARA_ARTIE = 'https://inara.cz/elite/cmdr/1/';
  const INARA_GUIDE = 'https://inara.cz/elite/inara-api-devguide/';
  let context;
  let initialized = false;
  let draft = null; // Non-secret checkboxes only. The API key stays in its input.
  let pairingToken = '';
  let pending = false;

  const panel = (title, content) => '<section class="panel"><div class="panel-heading"><h2>' + h(title) + '</h2></div><div class="panel-body">' + content + '</div></section>';
  const note = (text, warning) => text ? '<div class="info-strip' + (warning ? ' is-warning' : '') + '"><div>' + h(text) + '</div></div>' : '';
  const link = (url, title) => '<a href="' + h(url) + '" target="_blank" rel="noopener noreferrer">' + h(title) + '</a>';
  const check = (name, label, checked, disabled) => '<label class="operation-check"><input type="checkbox" name="' + h(name) + '" data-sync-setting' + (checked ? ' checked' : '') + (disabled ? ' disabled' : '') + '><span>' + h(label) + '</span></label>';
  const action = (label, name, disabled, style) => '<button class="button ' + (style || 'button-subtle') + '" type="button" data-sync-action="' + h(name) + '"' + (disabled ? ' disabled' : '') + '>' + h(label) + '</button>';
  const row = (label, value) => '<div class="sync-status-row"><span>' + h(label) + '</span><strong>' + h(value == null || value === '' ? '—' : value) + '</strong></div>';
  const textError = value => typeof value === 'string' ? value : value?.message || value?.error || '';
  const statusTime = value => value ? dateTime(value) : 'Ещё не было';

  function inaraRegistrationRequest(state = {}) {
    const version = /^\d+\.\d+\.\d+$/.test(state.app?.version || '') ? state.app.version : 'development build';
    return [
      'Hello Artie,',
      '',
      'I would like to register ED Copilot for development/testing access to the Inara API.',
      '',
      'Exact application name sent in header.appName: ED Copilot',
      'Current version: ' + version,
      'Homepage/source: ' + REPOSITORY,
      '',
      'Purpose: A Windows desktop companion I use for my own Elite Dangerous Live commander. It reads local game journals and optional Frontier CAPI snapshots. The Inara integration updates supported commander-profile information: credits, ships/loadouts, cargo/materials, ranks, missions and travel events. It does not request market, station or star-system datasets from Inara.',
      '',
      'Short description: Russian-language Elite Dangerous desktop companion with an activity-based overlay, trading/mining tools, engineering plans, fleet-carrier planning and optional commander-profile export to Inara.',
      '',
      'The integration is in development and sends isBeingDeveloped=true. It uses batched requests, retries with backoff, Live-only checks, journal replay exclusion and event deduplication. Automated protocol checks use synthetic data and mocked responses; live-account export still needs validation after application approval. Only sources are published so far, without binary releases.',
      '',
      'Please let me know if you need more information or changes before approving the application name. Thank you.',
    ].join('\n');
  }

  function inaraApprovalSteps(inara) {
    if (inara.appRegistered) return '<p class="panel-note">Вы отметили, что Inara разрешила имя ED Copilot. Это сохранённая отметка, а не проверка на сайте. Успех соединения подтверждается ответом Inara в разделе «Состояние отправки».</p>';
    return note(inara.keySet ? 'Личный ключ уже сохранён. Остался отдельный разовый шаг: Inara должна разрешить имя приложения ED Copilot.' : 'Для прямой отправки нужны два отдельных шага: личный ключ командира и разрешение имени ED Copilot со стороны Inara.', false) +
      '<ol class="sync-steps"><li>Войдите на Inara и откройте профиль автора сервиса — Artie.</li>' +
      '<li>Скопируйте текст запроса кнопкой ниже и отправьте Artie личное сообщение. В нём уже указаны название, назначение и ссылка на исходники приложения.</li>' +
      '<li>Дождитесь ответа, что имя <strong>ED Copilot</strong> разрешено для API. Только затем отметьте подтверждение ниже и сохраните настройки.</li></ol>' +
      '<div class="actions-wrap">' + link(INARA_ARTIE, 'Открыть профиль Artie') + action('Скопировать запрос для Inara', 'copy-inara-request') + '</div>' +
      '<p class="panel-note">Кнопка только копирует заготовку. Проверьте её и отправьте самостоятельно. В текст не входят ваш API-ключ, Client ID Frontier или данные аккаунта. ' + link(INARA_GUIDE, 'Требования Inara к новым приложениям') + '.</p>' +
      '<p class="panel-note">Пока разрешения нет, можно выбрать «Отправляет EDMC» и настроить Inara в самом EDMC. Он уже указан Inara среди поддерживаемых программ.</p>';
  }

  function defaults(status) {
    return {
      publisher: ['off', 'copilot', 'edmc'].includes(status.publisher) ? status.publisher : 'off',
      inaraEnabled: status.inara?.enabled === true,
      eddnEnabled: status.eddn?.enabled === true,
      inaraAppRegistered: status.inara?.appRegistered === true,
    };
  }

  function preservePassword() {
    // The main view rerenders on journal events. Move the actual password input
    // back afterwards, rather than copying its value into a JS draft or HTML.
    const existing = document.getElementById('sync-inara-key');
    if (!existing || existing.type !== 'password') return;
    const focused = document.activeElement === existing;
    const selection = focused ? [existing.selectionStart, existing.selectionEnd] : null;
    queueMicrotask(() => {
      const replacement = document.getElementById('sync-inara-key');
      if (!replacement || replacement === existing) return;
      existing.disabled = replacement.disabled;
      existing.placeholder = replacement.placeholder;
      replacement.replaceWith(existing);
      if (focused) {
        existing.focus({ preventScroll: true });
        if (selection?.[0] != null) existing.setSelectionRange(...selection);
      }
    });
  }

  function serviceStatus(title, service, publisher, extra) {
    const active = publisher === 'copilot' && service.enabled;
    const error = textError(service.lastError);
    let label = active ? 'Выбран ED Copilot' : publisher === 'edmc' ? 'Отправка через EDMC' : 'Отправка выключена';
    if (title === 'Inara' && active) label = !service.keySet ? 'Нужен личный ключ' : !service.appRegistered ? 'Нужно разрешение Inara для ED Copilot' : service.setupRequired ? 'Проверьте доступ и ключ' : service.lastSuccess ? 'Последняя отправка подтверждена' : 'Настроено, ждём ответа Inara';
    return panel(title, '<div class="pill-row">' + pill(label, active && (title !== 'Inara' || (service.keySet && service.appRegistered && !service.setupRequired)) ? 'blue' : 'muted') +
      (service.queued ? pill('В очереди: ' + n(service.queued, true), 'muted') : '') + '</div>' +
      row('Успешная отправка', statusTime(service.lastSuccess)) +
      row('Отправлено / отклонено', n(service.sent || 0, true) + ' / ' + n(service.failed || 0, true)) +
      (service.nextRetryAt ? row('Следующая попытка', statusTime(service.nextRetryAt)) : '') +
      (extra || '') + (error ? note(error, true) : '') +
      (publisher === 'edmc' ? '<p class="panel-note">Результат внешней отправки проверяйте в EDMC. Локальная связь с Copilot подтверждает только работу моста.</p>' : ''));
  }

  function pairingPanel(status, disabled) {
    const bridge = status.edmc || {};
    let content = '<p>EDMC отправляет данные своими плагинами. Дополнительный мост показывает в Copilot состояние связи и последние наблюдения.</p>' +
      '<div class="pill-row">' + pill(bridge.lastSeen ? 'Последний контакт: ' + dateTime(bridge.lastSeen) : bridge.paired ? 'Код создан, ждём EDMC' : 'Мост не настроен', bridge.lastSeen ? 'blue' : 'muted') + '</div>' +
      row('Командир в EDMC', typeof bridge.commander === 'string' ? bridge.commander : bridge.commander?.name) +
      (typeof bridge.status === 'string' ? row('Состояние моста', bridge.status) : '') +
      (bridge.version ? row('Версия моста', bridge.version) : '') +
      '<ol class="sync-steps"><li>В EDMC откройте Settings → Plugins → Open, затем закройте EDMC.</li>' +
      '<li>Скопируйте папку <code>EDCopilot</code> из <code>C:\\Games\\ED Copilot\\edmc-plugin</code> в открытую папку плагинов.</li>' +
      '<li>Создайте код кнопкой ниже. Запустите EDMC, откройте Settings → ED Copilot, вставьте код и включите связь.</li>' +
      '<li>В EDMC отдельно включите отправку в EDDN и Inara, укажите там ключ Inara.</li></ol>' +
      '<div class="actions-wrap">' + action(bridge.paired ? 'Создать новый код' : 'Создать код сопряжения', 'pair', disabled) + '</div>' +
      (bridge.paired ? '<p class="panel-note">Новый код заменяет прежний: его потребуется обновить в настройках плагина EDMC.</p>' : '');
    if (pairingToken) content += '<div class="sync-pairing"><label class="field"><span>Код для настроек EDMC</span><input id="sync-pair-token" type="text" readonly autocomplete="off" spellcheck="false" value="' + h(pairingToken) + '"></label><div class="actions-wrap">' + action('Копировать код', 'copy-pair') + action('Скрыть код', 'hide-pair') + '</div><p class="panel-note">Код показан по вашему запросу и хранится только в этом окне. Передавайте его только локальному плагину.</p></div>';
    return panel('Связать EDMC с Copilot', content);
  }

  function render(state) {
    preservePassword();
    const status = state.integrations || {};
    const settings = draft || defaults(status);
    const inara = status.inara || {}, eddn = status.eddn || {};
    const locked = pending || status.busy === true;
    const native = settings.publisher === 'copilot';
    const unavailable = status.storageAvailable === false;
    const cmdr = status.commander?.name || state.commander?.name || 'Командир ещё не определён';
    const modes = [
      ['off', 'Не отправлять', 'Все данные остаются в приложении.'],
      ['copilot', 'Отправляет ED Copilot', 'Прямая отправка в выбранные сервисы.'],
      ['edmc', 'Отправляет EDMC', 'Использовать настроенные плагины EDMC.'],
    ];
    let warnings = '';
    if (unavailable) warnings += note(textError(status.storageError) || 'Для сохранения ключей и работы интеграций запустите настольное приложение ED Copilot с хранилищем Windows.', true);
    if (status.boundCommander && status.identityMatches === false) warnings += '<div class="info-strip is-warning"><div><p>Настройки привязаны к командиру ' + h(status.boundCommander.name || 'из другого профиля') + '. Отправка приостановлена.</p><p>Чтобы настроить текущего командира, прежние очередь и ключ Inara будут удалены, а отправка в сервисы выключена. Затем укажите новый ключ и выберите сервисы.</p><div class="actions-wrap">' + action('Настроить для текущего командира', 'rebind', locked || unavailable || !status.commander?.fid) + '</div></div></div>';
    if (status.live?.ok === false && settings.publisher === 'copilot') warnings += note(status.live.reason || 'Для отправки нужны свежие события текущего сеанса Elite Dangerous Live. Запустите игру и дождитесь обновления журнала.', false);
    const form = '<form id="integrations-form" class="sync-form">' + panel('Кто отправляет данные',
      '<div class="sync-modes">' + modes.map(([value, title, description]) => '<label class="sync-mode' + (settings.publisher === value ? ' is-selected' : '') + '"><input type="radio" name="publisher" value="' + value + '" data-sync-setting' + (settings.publisher === value ? ' checked' : '') + (locked ? ' disabled' : '') + '><span><strong>' + title + '</strong><small>' + description + '</small></span></label>').join('') + '</div>' +
      '<p class="panel-note">Выберите одного отправителя. Если используете ED Copilot, отключите отправку в те же сервисы в EDMC. Самостоятельно менять настройки EDMC приложение не будет.</p>') +
      '<div class="sync-columns">' + panel('Inara · личный профиль',
        check('inaraEnabled', 'Отправлять профиль и поддерживаемые события в Inara', settings.inaraEnabled, !native || locked) +
        '<p>Баланс, корабль, груз, материалы, ранги, миссии и поддерживаемые события перемещений — в профиль вашего командира. Для флотоносца передаётся событие перелёта; личный склад не синхронизируется.</p>' +
        '<div class="pill-row">' + pill(inara.keySet ? 'Ключ сохранён' : 'Нужен API-ключ Inara', inara.keySet ? 'blue' : 'muted') + '</div>' +
        '<label class="field"><span>' + (inara.keySet ? 'Новый API-ключ (если нужно заменить)' : 'Личный API-ключ Inara') + '</span><input id="sync-inara-key" type="password" name="inaraApiKey" maxlength="512" autocomplete="new-password" spellcheck="false" placeholder="' + (inara.keySet ? 'Сохранённый ключ не отображается' : 'Вставьте ключ из настроек командира Inara') + '"' + (!native || locked || unavailable ? ' disabled' : '') + '></label>' +
        '<p class="panel-note">' + link(INARA_API_SETTINGS, 'Открыть настройки API на Inara') + '. Если ключ уже вставлен и сохранён, создавать новый не нужно. Пароль от Inara здесь не используется.</p>' +
        '<h3>Разрешить отправку из ED Copilot</h3>' + inaraApprovalSteps(inara) +
        check('inaraAppRegistered', 'Я получил от Inara подтверждение: имя ED Copilot разрешено для API', settings.inaraAppRegistered, !native || locked) +
        '<p class="panel-note">Эта галочка лишь сохраняет ваше подтверждение. Она не отправляет заявку и не выдаёт разрешение на стороне Inara. Вход Frontier и его Client ID относятся к другому сервису.</p>' +
        (inara.setupRequired ? note(typeof inara.setupRequired === 'string' ? inara.setupRequired : !inara.keySet ? 'Сохраните личный API-ключ из настроек командира Inara.' : !inara.appRegistered ? 'Ключ сохранён; ожидается разрешение имени ED Copilot со стороны Inara.' : 'Inara не подтвердила доступ. Проверьте ответ сервиса ниже.', true) : '') +
        (inara.keySet ? '<div class="actions-wrap">' + action('Удалить сохранённый ключ', 'clear-key', locked || unavailable) + '</div>' : '')) +
      panel('EDDN · общая база наблюдений',
        check('eddnEnabled', 'Делиться публичными данными через EDDN', settings.eddnEnabled, !native || locked) +
        '<p>Цены и спрос текущего рынка, ассортимент станции, системы, сканы тел, колец и сигналы. Этими наблюдениями пользуются сторонние сайты и приложения.</p>' +
        '<p class="panel-note">EDDN не требует ключа. Баланс, миссии, личная репутация и личный склад флотоносца туда не отправляются. Идентификатор отправителя — имя командира; сеть обфусцирует его перед ретрансляцией.</p>' +
        '<p class="panel-note">Журнал читается только для новых наблюдений. Перечитывание истории не создаёт массовую отправку старых рынков. Для станционных данных CAPI проверяется совпадение текущего места.</p>') + '</div>' +
      '<div class="sync-save"><button class="button button-primary" type="submit"' + (locked || unavailable ? ' disabled' : '') + '>' + (pending ? 'Сохраняем…' : 'Сохранить настройки') + '</button><span>Профиль: ' + h(cmdr) + '</span></div></form>';
    const syncDisabled = locked || status.publisher !== 'copilot' || status.identityMatches === false || status.live?.ok !== true || (!inara.enabled && !eddn.enabled);
    return '<div class="copilot-sync"><div class="page-heading"><div><p class="eyebrow">ДАННЫЕ И СООБЩЕСТВО</p><h1>Синхронизация</h1><p>Свяжите личный профиль Inara и общую базу EDDN или оставьте отправку привычному EDMC.</p></div><button class="button button-subtle" type="button" data-help-topic="integrations">? Как настроить</button></div>' + warnings + form +
      (settings.publisher === 'edmc' ? pairingPanel(status, locked || unavailable) : '') +
      '<div class="sync-queue-heading"><div><h2>Состояние отправки</h2><p class="panel-note">В очереди: ' + n(status.queue?.total || 0, true) + ' · Последняя проверка: ' + h(statusTime(status.lastSync)) + '</p></div>' + action(status.busy ? 'Отправляем…' : 'Отправить очередь сейчас', 'sync', syncDisabled) + '</div>' +
      '<div class="sync-columns">' + serviceStatus('Inara', inara, status.publisher || 'off', inara.appRegistered ? '' : row('Регистрация приложения', 'Не подтверждена')) + serviceStatus('EDDN', eddn, status.publisher || 'off') + '</div>' +
      '<p class="panel-note sync-footer">Доставка в EDDN не гарантирует мгновенного обновления каждого сайта. При сетевой ошибке приложение повторяет попытку с паузой; ошибки формата показываются отдельно. ' + link(REPOSITORY, 'Исходный код ED Copilot') + ' · <a href="#account">Подключение Frontier</a></p></div>';
  }

  function readSettings(form) {
    return {
      publisher: form.querySelector('input[name="publisher"]:checked')?.value || 'off',
      inaraEnabled: form.elements.inaraEnabled.checked,
      eddnEnabled: form.elements.eddnEnabled.checked,
      inaraAppRegistered: form.elements.inaraAppRegistered.checked,
    };
  }

  function init(api) {
    context = api;
    if (initialized) return;
    initialized = true;
    document.addEventListener('change', event => {
      if (!event.target.matches('[data-sync-setting]')) return;
      const form = event.target.closest('#integrations-form');
      if (!form) return;
      draft = readSettings(form);
      if (event.target.name === 'publisher') { pairingToken = ''; context.render(); }
    });
    document.addEventListener('submit', async event => {
      const form = event.target;
      if (form.id !== 'integrations-form') return;
      event.preventDefault();
      if (pending) return;
      const body = readSettings(form);
      const key = form.querySelector('#sync-inara-key');
      if (key && !key.disabled && key.value.trim()) body.inaraApiKey = key.value.trim();
      if (key) key.value = '';
      pending = true;
      context.render();
      try {
        const result = await context.perform('/api/integrations/configure', body, 'Настройки синхронизации сохранены');
        if (result) draft = null;
      } finally {
        delete body.inaraApiKey;
        pending = false;
        context.render();
      }
    });
    document.addEventListener('click', async event => {
      const button = event.target.closest('[data-sync-action]');
      if (!button || button.disabled) return;
      const name = button.dataset.syncAction;
      const remote = ['sync', 'clear-key', 'pair', 'rebind'].includes(name);
      if (remote && pending) return;
      if (remote) { pending = true; context.render(); }
      try {
        if (name === 'copy-pair') {
          if (pairingToken) { await U.copyText(pairingToken); context.showToast('Код сопряжения скопирован'); }
        } else if (name === 'copy-inara-request') {
          await U.copyText(inaraRegistrationRequest(context.getState()));
          context.showToast('Текст запроса скопирован. Проверьте его и отправьте Artie на Inara.');
        } else if (name === 'hide-pair') {
          pairingToken = ''; context.render();
        } else if (name === 'sync') {
          await context.perform('/api/integrations/sync', {}, 'Очередь проверена');
        } else if (name === 'clear-key') {
          const key = document.getElementById('sync-inara-key');
          if (key) key.value = '';
          await context.perform('/api/integrations/configure', { clearInaraKey: true }, 'Ключ Inara удалён');
        } else if (name === 'pair') {
          const result = await context.perform('/api/integrations/pair', {});
          pairingToken = typeof result?.token === 'string' ? result.token : '';
          context.render();
        } else if (name === 'rebind') {
          const key = document.getElementById('sync-inara-key');
          if (key) key.value = '';
          pairingToken = '';
          const result = await context.perform('/api/integrations/configure', {
            publisher: 'copilot', rebindCommander: true, clearInaraKey: true,
            inaraEnabled: false, eddnEnabled: false,
          }, 'Настройки подготовлены для текущего командира. Выберите сервисы и сохраните.');
          if (result) draft = null;
        }
      } catch (error) { context.showToast(error.message || 'Не удалось выполнить действие', true); }
      finally { if (remote) { pending = false; context.render(); } }
    });
    window.addEventListener('beforeunload', () => { pairingToken = ''; draft = null; });
  }

  window.CopilotIntegrations = { init, render, inaraRegistrationRequest };
})();
