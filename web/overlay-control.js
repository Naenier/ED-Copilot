(function () {
  'use strict';
  let context, nativeState = null, popup = null, initialized = false, busy = false;
  let timer, unsubscribe;
  function enabled() {
    if (window.edMining) return nativeState ? nativeState.open === true && nativeState.hidden !== true : context.getState()?.settings?.overlayEnabled !== false;
    return context.getState()?.settings?.overlayEnabled !== false && !!popup && !popup.closed;
  }
  function update() {
    if (!context) return;
    if (!window.edMining && context.getState()?.settings?.overlayEnabled === false && popup) { popup.close(); popup = null; }
    const on = enabled();
    const bar = document.querySelector('.activity-bar');
    if (bar) bar.hidden = !on;
    const button = document.getElementById('overlay-toggle');
    if (button) {
      button.textContent = on ? '▱ Скрыть оверлей' : '▱ Включить оверлей';
      button.setAttribute('aria-pressed', String(on));
      button.title = on ? 'Скрыть оверлей и выбор его занятия' : 'Показать оверлей и выбор его занятия';
      button.disabled = busy;
    }
  }
  async function toggle() {
    if (busy) return;
    busy = true; update();
    try {
      const bridge = window.edMining;
      if (bridge?.toggleOverlay) nativeState = await bridge.toggleOverlay();
      else if (bridge?.setOverlay) nativeState = await bridge.setOverlay(!enabled());
      else {
        if (enabled()) {
          popup?.close(); popup = null;
          await context.perform('/api/settings', { overlayEnabled: false });
        } else {
          popup = window.open('/overlay.html', 'ed-copilot-overlay', 'width=420,height=600');
          if (!popup) throw new Error('Браузер заблокировал окно оверлея. Разрешите всплывающие окна для приложения.');
          if (!await context.perform('/api/settings', { overlayEnabled: true })) { popup.close(); popup = null; }
        }
      }
    } catch (error) { context.showToast(error.message, true); }
    finally { busy = false; update(); }
  }
  function init(api) {
    context = api;
    if (initialized) return;
    initialized = true;
    document.getElementById('overlay-toggle').addEventListener('click', toggle);
    if (window.edMining?.onOverlayState) unsubscribe = window.edMining.onOverlayState((value) => { nativeState = value; update(); });
    if (window.edMining?.getOverlayState) window.edMining.getOverlayState().then((value) => { nativeState = value; update(); }).catch(() => update());
    if (!window.edMining) timer = setInterval(() => {
      if (popup?.closed) { popup = null; update(); void context.perform('/api/settings', { overlayEnabled: false }); }
    }, 1000);
    window.addEventListener('beforeunload', () => { unsubscribe?.(); clearInterval(timer); popup?.close(); });
    update();
  }
  window.CopilotOverlayControl = { init, update };
})();
