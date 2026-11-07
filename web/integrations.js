(function () {
  'use strict';

  const U = window.MiningUI;
  const { h, n, dateTime, pill } = U;
  let context;
  let initialized = false;
  let enabledDraft = null;
  let pending = false;
  const row = (label, value) => '<div class="sync-status-row"><span>' + h(label) + '</span><strong>' + h(value == null || value === '' ? '—' : value) + '</strong></div>';

  function preservePassword() {
    // Keep the actual input across journal-driven renders, without a secret JS draft.
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

  function render(state) {
    preservePassword();
    const status = state.integrations || {};
    const enabled = enabledDraft ?? status.enabled === true;
    const disabled = pending || !status.storageAvailable || Boolean(status.storageError);
    const label = status.busy ? 'Отправляем события' : status.blockedReason ||
      (status.lastError ? 'Не удалось отправить' : status.lastSuccess ? 'Последняя отправка подтверждена' : 'Ожидаем новых событий игры');
    const error = status.storageError || status.lastError;
    return '<div class="page-heading"><div><p class="eyebrow">Личный профиль</p><h1>Синхронизация с Inara</h1><p>Обновление сведений о командире по новым событиям игры.</p></div></div>' +
      '<section class="panel"><div class="panel-heading"><h2>Inara</h2></div><form id="integrations-form" class="panel-body">' +
      '<div class="pill-row">' + pill(label, status.busy || status.lastSuccess && !status.blockedReason && !error ? 'blue' : 'muted') + '</div>' +
      '<div class="form-grid"><label class="field"><span>Личный API-ключ Inara</span><input id="sync-inara-key" name="apiKey" type="password" maxlength="2048" autocomplete="new-password" spellcheck="false" placeholder="' +
        (status.apiKeySet ? 'Ключ сохранён · вставьте новый для замены' : 'Вставьте ключ из настроек API на Inara') + '"' + (disabled ? ' disabled' : '') + '></label></div>' +
      '<p class="panel-note"><a href="https://inara.cz/elite/cmdr-settings-api/" target="_blank" rel="noopener noreferrer">Настройки API на Inara</a>. Пустое поле сохраняет прежний ключ.</p>' +
      '<label class="operation-check"><input type="checkbox" name="enabled" data-sync-enabled' + (enabled ? ' checked' : '') + (disabled ? ' disabled' : '') + '><span>Отправлять события в Inara</span></label>' +
      '<div class="actions-wrap"><button class="button" type="submit"' + (disabled ? ' disabled' : '') + '>' + (pending ? 'Сохраняем…' : 'Сохранить') + '</button></div>' +
      (error ? '<div class="info-strip is-warning"><div>' + h(error) + '</div></div>' : '') +
      row('Последняя успешная отправка', status.lastSuccess ? dateTime(status.lastSuccess) : 'Ещё не было') +
      '<details class="expedition-details" data-preserve-open="inara-details"><summary>Подробности отправки</summary>' +
      row('Командир', status.boundCommander?.name || status.commander?.name) +
      row('Событий в очереди', n(status.queued || 0, true)) +
      row('Подтверждено событий', n(status.sent || 0, true)) +
      (status.failed ? row('Пропущено событий', n(status.failed, true)) : '') +
      (status.nextRetryAt ? row('Следующая попытка', dateTime(status.nextRetryAt)) : '') +
      '<p class="panel-note">Передаются поддерживаемые сведения личного профиля: баланс, корабли и оснащение, груз, материалы, ранги, миссии и перемещения. Отправка работает для текущего командира в Elite Dangerous Live. При сетевой ошибке очередь сохраняется и повторяется с паузой.</p></details></form></section>';
  }

  function init(api) {
    context = api;
    if (initialized) return;
    initialized = true;
    document.addEventListener('change', (event) => {
      if (event.target.matches('[data-sync-enabled]')) enabledDraft = event.target.checked;
    });
    document.addEventListener('submit', async (event) => {
      if (event.target.id !== 'integrations-form') return;
      event.preventDefault();
      if (pending) return;
      const form = event.target;
      const input = form.querySelector('#sync-inara-key');
      const body = { enabled: form.elements.enabled.checked };
      if (input?.value.trim()) body.apiKey = input.value.trim();
      if (input) input.value = '';
      enabledDraft = body.enabled;
      pending = true; context.render();
      try {
        const result = await context.perform('/api/integrations/configure', body, 'Настройки Inara сохранены');
        if (result?.ok) enabledDraft = null;
      } finally {
        delete body.apiKey;
        pending = false; context.render();
      }
    });
  }

  window.CopilotIntegrations = { init, render };
})();
