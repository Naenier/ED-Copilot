(function () {
  'use strict';
  const modes = [['windowed', 'В окне'], ['fullscreen', 'Полноэкранный'], ['borderless', 'Безрамочное окно']];
  let context, display = null, selected = null, pending = false, initialized = false, lastError = '';
  const bridge = () => window.edMining;
  const native = () => typeof bridge()?.getDisplayState === 'function' && typeof bridge()?.setDisplayMode === 'function';

  function controls() {
    const actions = document.querySelector('.header-actions');
    if (!actions) return;
    let group = document.getElementById('copilot-window-controls');
    if (!group) {
      group = document.createElement('div');
      group.id = 'copilot-window-controls'; group.className = 'copilot-window-controls'; group.hidden = true;
      group.setAttribute('aria-label', 'Управление окном');
      group.innerHTML = '<button type="button" data-display-window-action="minimize" title="Свернуть" aria-label="Свернуть окно">—</button>' +
        '<button type="button" data-display-window-action="restore" title="Вернуться в оконный режим" aria-label="Вернуться в оконный режим">❐</button>' +
        '<button type="button" data-display-window-action="close" title="Скрыть в область уведомлений" aria-label="Скрыть окно в область уведомлений">×</button>';
      actions.append(group);
    }
    const mode = display?.mode || 'windowed';
    document.documentElement.dataset.displayMode = mode;
    group.hidden = !native() || mode === 'windowed';
    for (const button of group.querySelectorAll('button')) button.disabled = pending || display?.changing === true;
  }

  function update(value) {
    display = value && typeof value === 'object' ? value : { mode: 'windowed' };
    if (display.error && display.error !== lastError) context?.showToast(display.error, true);
    lastError = display.error || '';
    controls();
    context?.render();
  }

  function render() {
    const h = window.MiningUI.h;
    const supported = native(), mode = selected || display?.pendingMode || display?.mode || 'windowed';
    const busy = pending || display?.changing === true;
    return '<section class="panel window-display-panel"><div class="panel-heading"><h2>Режим окна приложения</h2><button type="button" class="button button-small button-subtle" data-help-topic="interface">? Как пользоваться</button></div><form id="window-display-form" class="panel-body">' +
      '<p>Выберите, как открывать основное окно ED Copilot. Оверлей игры работает независимо от этого режима.</p>' +
      '<div class="window-display-settings"><label class="field"><span>Режим отображения</span><select name="displayMode" data-focus="display-mode"' + (!supported || busy ? ' disabled' : '') + '>' +
      modes.map(([value, label]) => '<option value="' + value + '"' + (value === mode ? ' selected' : '') + '>' + label + '</option>').join('') +
      '</select></label><button class="button button-primary" type="submit"' + (!supported || !display || busy ? ' disabled' : '') + '>' + (busy ? 'Меняем режим…' : 'Применить режим') + '</button></div>' +
      (supported ? '<p class="panel-note">Полноэкранный режим занимает весь монитор. Безрамочное окно заполняет рабочую область и оставляет панель задач Windows доступной. Размер обычного окна запоминается.</p>' +
        '<p class="panel-note">F11 переключает полноэкранный и оконный режим. Esc возвращает обычное окно. В режимах без системной рамки справа вверху доступны кнопки сворачивания, возврата в окно и скрытия в область уведомлений.</p>' :
        '<p class="panel-note">Эта настройка доступна в настольном ED Copilot. Размером и полноэкранным режимом вкладки управляет ваш браузер.</p>') +
      (display?.error ? '<p class="window-display-error" role="status">' + h(display.error) + '</p>' : '') + '</form></section>';
  }

  async function apply(mode) {
    if (!native() || pending || display?.changing) return;
    pending = true; controls(); context.render();
    try { const value = await bridge().setDisplayMode(mode); selected = null; update(value); }
    catch (error) { context.showToast(error.message || 'Не удалось изменить режим окна.', true); }
    finally { pending = false; controls(); context.render(); }
  }

  function init(api) {
    context = api;
    if (initialized) return;
    initialized = true; controls();
    if (native()) {
      bridge().onDisplayState?.(update);
      bridge().getDisplayState().then(update).catch(error => context.showToast(error.message, true));
    }
    document.addEventListener('change', event => {
      if (event.target.name === 'displayMode' && event.target.closest('#window-display-form')) selected = event.target.value;
    });
    document.addEventListener('submit', event => {
      if (event.target.id !== 'window-display-form') return;
      event.preventDefault();
      void apply(event.target.elements.displayMode.value);
    });
    // The native before-input handler is primary. This also covers input sent
    // directly to the renderer by accessibility tools rather than the OS queue.
    document.addEventListener('keydown', event => {
      if (!native() || event.ctrlKey || event.altKey || event.metaKey) return;
      const key = event.key.toLowerCase(), mode = display?.mode || 'windowed';
      if (key !== 'f11' && !(key === 'escape' && mode !== 'windowed')) return;
      event.preventDefault();
      if (!event.repeat) void apply(key === 'f11' && mode === 'windowed' ? 'fullscreen' : 'windowed');
    }, true);
    document.addEventListener('click', async event => {
      const button = event.target.closest('[data-display-window-action]');
      if (!button || button.disabled || !native()) return;
      try {
        if (button.dataset.displayWindowAction === 'restore') await apply('windowed');
        else if (button.dataset.displayWindowAction === 'minimize') await bridge().minimize();
        else if (button.dataset.displayWindowAction === 'close') await bridge().close();
      } catch (error) { context.showToast(error.message, true); }
    });
  }

  window.CopilotWindowDisplay = { render, init };
})();
