/* First-use loading for section-owned scripts and styles from the embedded origin. */
(() => {
  'use strict';

  const nonce = document.currentScript.nonce;
  const byTab = new Map(Object.entries({
    learning: 'academy', 'carrier-calculator': 'calculator', settings: 'settings',
    deals: 'trade-routes', 'carrier-trade': 'carrier-trade', statistics: 'trade-statistics',
    engineering: 'engineering', colonization: 'colonization', exobiology: 'exobiology',
    mining: 'mining', route: 'carrier-route', 'ship-route': 'ship-route',
    exploration: 'exploration', riches: 'cartography',
    notifications: 'section-tools', 'second-screen': 'section-tools',
  }));
  const loads = new Map();

  function load(tab, mode) {
    if (tab === 'engineering' && (mode === 'Конструктор кораблей' || mode === 'Инженеры')) return;
    if (tab === 'statistics' && mode !== 'Торговля') return;
    const bundle = byTab.get(tab);
    if (!bundle) return;
    if (loads.has(bundle)) return loads.get(bundle);

    const asset = document.createElement('script');
    asset.src = 'workspace/' + bundle + '.js';
    asset.nonce = nonce;
    const promise = new Promise((resolve, reject) => {
      asset.onload = () => { asset.remove(); resolve(); };
      asset.onerror = () => {
        asset.remove();
        loads.delete(bundle);
        reject(new Error(window.CopilotI18n.t('Не удалось загрузить раздел.')));
      };
    });
    loads.set(bundle, promise);
    document.head.append(asset);
    return promise;
  }

  window.WorkspaceAssets = {load};
})();
