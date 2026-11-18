(() => {
  'use strict';

  const nonce = document.currentScript.nonce;

  function workspace(name, active, title, path) {
    let pending = false;
    let failed = false;

    function render(C,u) {
      const status = failed ? '<p role="alert">' + u.esc(u.t('Не удалось загрузить раздел.')) + '</p>' +
        '<button type="button" class="pill" data-workspace-retry>' + u.esc(u.t('Повторить')) + '</button>' :
        '<p role="status">' + u.esc(u.t('Загрузка…')) + '</p>';
      return u.heading(u.t(title),'') + '<div>' + status + '</div>';
    }

    function init(C,u) {
      if (pending) return;
      if (failed) {
        const retry = u.$('#page [data-workspace-retry]');
        if (retry) retry.onclick = () => { failed = false; u.renderPage(); };
        return;
      }
      pending = true;
      const asset = document.createElement('script');
      asset.src = path;
      asset.nonce = nonce;
      asset.onload = asset.onerror = () => {
        asset.remove();
        pending = false;
        failed = window[name] === lazy;
        if (active(C)) u.renderPage();
      };
      document.head.append(asset);
    }

    const lazy = {active,render,init};
    window[name] = lazy;
  }

  const fitting = 'Конструктор кораблей';
  workspace('EDCopilotShipFits', C => C.tab === 'engineering' && C.mode === fitting, fitting, 'workspace/ship-fits.js');
  workspace('NearestUI', C => C.tab === 'nearest', 'Поиск ближайших', 'workspace/nearest.js');
})();
