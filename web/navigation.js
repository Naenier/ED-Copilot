(function () {
  'use strict';
  const catalog = [
    ['overview', '◈', 'Обзор', 'Положение, корабль и текущие планы'],
    ['targets', '⌖', 'Цели', 'Личные задачи и поиск мест'],
    ['sale', '⇄', 'Торговля', 'Покупатели, цены и спрос'],
    ['encyclopedia', '▤', 'Энциклопедия', 'Справочник по игре'],
    ['statistics', '▥', 'Статистика', 'Результаты вашей деятельности'],
    ['settings', '⚙', 'Настройки', 'Оформление и параметры приложения'],
    ['deals', '↔', 'Рейсы', 'Поиск торговых маршрутов'],
    ['carrier', '▱', 'Флотоносец', 'Состояние вашего флотоносца'],
    ['route', '⌁', 'Маршрут флотоносца', 'Прыжки, тритий и время в пути'],
    ['mining', '⛏', 'Заход добычи', 'План и результаты добычи'],
    ['engineering', '⚒', 'Инженерия', 'Улучшения и список материалов'],
    ['colonization', '⌂', 'Колонизация', 'Поиск системы, строительство и снабжение'],
    ['account', '◉', 'Frontier', 'Подключение Companion API'],
    ['integrations', '⇅', 'Синхронизация', 'Личный профиль командира на Inara'],
    ['help', '?', 'Руководство', 'Инструкции по разделам'],
  ].map(([id, icon, label, description]) => ({ id, icon, label, description }));
  const defaults = ['overview', 'targets', 'sale', 'encyclopedia', 'statistics', 'settings'];
  const required = new Set(['overview', 'settings']);
  const ids = new Set(catalog.map(tab => tab.id));
  let context, root, current = [...defaults], pending = null, writes = Promise.resolve(), revision = 0;
  let menuOpen = false, dragged = '', initialized = false, lastMarkup = '';
  let loaded = false, deferredEnsure = null;
  const h = value => String(value ?? '').replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;');
  function normalize(value) {
    const result = Array.isArray(value) ? [...new Set(value.filter(id => ids.has(id)))] : [...defaults];
    if (!result.includes('overview')) result.unshift('overview');
    if (!result.includes('settings')) result.push('settings');
    return result;
  }
  function active() { return ids.has(location.hash.slice(1)) ? location.hash.slice(1) : 'overview'; }
  function activeInList() { return current.includes(active()) ? active() : 'overview'; }
  function focusTab(id) { root?.querySelector('[data-tab="' + id + '"]')?.focus({ preventScroll: true }); }
  function scrollActive() {
    const link = root?.querySelector('[data-tab="' + activeInList() + '"]');
    const strip = root?.querySelector('.copilot-tabstrip');
    if (!link || !strip) return;
    const item = link.closest('.copilot-tab'), left = item.offsetLeft - strip.offsetLeft;
    if (left < strip.scrollLeft) strip.scrollLeft = left;
    else if (left + item.offsetWidth > strip.scrollLeft + strip.clientWidth) strip.scrollLeft = left + item.offsetWidth - strip.clientWidth;
  }
  function markup() {
    const selected = activeInList(), hidden = catalog.filter(tab => !current.includes(tab.id));
    return '<div class="copilot-tabbar"><div class="copilot-tabstrip" role="tablist" aria-label="Открытые разделы">' + current.map(id => {
      const tab = catalog.find(item => item.id === id), isActive = id === selected;
      return '<div class="copilot-tab' + (isActive ? ' is-active' : '') + '" draggable="true" data-navigation-tab="' + id + '">' +
        '<a class="copilot-tab-link' + (isActive ? ' is-active' : '') + '" id="copilot-tab-' + id + '" href="#' + id + '" data-tab="' + id + '" role="tab" aria-selected="' + isActive + '" aria-controls="content" tabindex="' + (isActive ? '0' : '-1') + '" title="' + h(tab.label + ' · Alt+←/→ — переместить вкладку') + '"><span class="copilot-tab-icon" aria-hidden="true">' + h(tab.icon) + '</span><span class="copilot-tab-label">' + h(tab.label) + '</span></a>' +
        (required.has(id) ? '' : '<button type="button" class="copilot-tab-close" data-navigation-close="' + id + '" aria-label="Закрыть вкладку «' + h(tab.label) + '»" title="Закрыть вкладку"' + (loaded ? '' : ' disabled') + '><svg class="copilot-tab-close-mark" viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path d="M4 4L12 12M12 4L4 12" /></svg></button>') + '</div>';
    }).join('') + '</div><div class="copilot-tabtools"><button id="copilot-nav-add" class="copilot-nav-add" type="button" aria-label="Добавить вкладку" title="Добавить вкладку" aria-haspopup="menu" aria-controls="copilot-nav-menu" aria-expanded="' + menuOpen + '"' + (loaded ? '' : ' disabled') + '>+</button></div></div>' +
      '<div class="copilot-nav-menu" id="copilot-nav-menu" role="menu" aria-label="Добавить раздел"' + (menuOpen ? '' : ' hidden') + '><div class="copilot-nav-menu-heading" role="presentation">ДОБАВИТЬ ВКЛАДКУ</div>' +
      (hidden.length ? hidden.map(tab => '<button type="button" class="copilot-nav-feature" role="menuitem" data-navigation-add="' + tab.id + '"><span aria-hidden="true">' + h(tab.icon) + '</span><span><strong>' + h(tab.label) + '</strong><small>' + h(tab.description) + '</small></span></button>').join('') : '<p class="copilot-nav-menu-empty" role="presentation">Все разделы уже открыты.</p>') +
      '<div class="copilot-nav-menu-footer" role="presentation"><button type="button" role="menuitem" data-navigation-reset>Восстановить стандартные вкладки</button><p>Перетаскивайте вкладки мышью.<br>С клавиатуры: Alt+← или Alt+→.</p></div></div>';
  }
  function render({ focusId, focusMenu = false, keepFocus = true } = {}) {
    if (!root) return;
    const focused = keepFocus && root.contains(document.activeElement) ? document.activeElement : null;
    const focusKey = focused?.dataset.tab || focused?.dataset.navigationClose;
    const closeFocus = Boolean(focused?.dataset.navigationClose);
    const addFocus = focused?.id === 'copilot-nav-add';
    const scrollLeft = root.querySelector('.copilot-tabstrip')?.scrollLeft || 0;
    const next = markup();
    if (next !== lastMarkup) {
      root.innerHTML = next; lastMarkup = next;
      const strip = root.querySelector('.copilot-tabstrip'); if (strip) strip.scrollLeft = scrollLeft;
    }
    const panel = document.getElementById('content');
    if (panel) { panel.setAttribute('role', 'tabpanel'); panel.setAttribute('aria-labelledby', 'copilot-tab-' + activeInList()); }
    if (focusMenu) root.querySelector('[role="menuitem"]')?.focus({ preventScroll: true });
    else if (focusId) focusTab(focusId);
    else if (addFocus) root.querySelector('#copilot-nav-add')?.focus({ preventScroll: true });
    else if (focusKey) {
      const selector = closeFocus ? '[data-navigation-close="' + focusKey + '"]' : '[data-tab="' + focusKey + '"]';
      root.querySelector(selector)?.focus({ preventScroll: true });
    }
  }
  function update(state = context?.getState?.()) {
    if (Array.isArray(state?.navigation?.tabs)) loaded = true;
    current = normalize(pending || (loaded ? state?.navigation?.tabs || current : defaults));
    render();
    if (loaded && deferredEnsure) {
      const tab = deferredEnsure; deferredEnsure = null;
      if (!current.includes(tab)) void persist([...current, tab]);
    }
  }
  function persist(tabs, options = {}) {
    const next = normalize(tabs), id = ++revision;
    pending = current = next;
    if (options.activate) location.hash = options.activate;
    menuOpen = false; render({ focusId: options.focusId || options.activate }); scrollActive();
    const body = { tabs: [...next] };
    writes = writes.catch(() => {}).then(async () => {
      let result;
      try { result = await context.perform('/api/navigation', body); }
      catch (error) { context.showToast?.(error.message, true); result = null; }
      if (id !== revision) return;
      pending = null; update(context.getState());
      if (!result) scrollActive();
    });
    return writes;
  }
  function ensure(tab) {
    if (!ids.has(tab)) return false;
    if (!loaded) { deferredEnsure = tab; return true; }
    if (!current.includes(tab)) void persist([...current, tab]);
    else { render(); scrollActive(); }
    return true;
  }
  function close(tab) {
    if (!loaded || required.has(tab) || !current.includes(tab)) return;
    const index = current.indexOf(tab), next = current.filter(id => id !== tab);
    const chosen = active() === tab ? next[Math.min(index, next.length - 1)] || 'overview' : null;
    void persist(next, { activate: chosen, focusId: chosen || activeInList() });
  }
  function move(tab, amount) {
    if (!loaded) return;
    const index = current.indexOf(tab), destination = index + amount;
    if (index < 0 || destination < 0 || destination >= current.length) return;
    const next = [...current]; [next[index], next[destination]] = [next[destination], next[index]];
    void persist(next, { focusId: tab });
  }
  function setMenu(value) {
    menuOpen = value; render({ focusMenu: value, keepFocus: !value });
    if (!value) root.querySelector('#copilot-nav-add')?.focus({ preventScroll: true });
  }
  function init(api) {
    context = api; root = document.getElementById('copilot-navigation');
    if (!root || initialized) { update(); return; }
    initialized = true;
    root.addEventListener('click', event => {
      const target = event.target;
      if (target.closest('#copilot-nav-add')) { setMenu(!menuOpen); return; }
      const add = target.closest('[data-navigation-add]');
      if (add) { const id = add.dataset.navigationAdd; void persist([...current, id], { activate: id }); return; }
      const remove = target.closest('[data-navigation-close]');
      if (remove) { close(remove.dataset.navigationClose); return; }
      if (target.closest('[data-navigation-reset]')) {
        void persist(defaults, { activate: defaults.includes(active()) ? null : 'overview', focusId: defaults.includes(active()) ? active() : 'overview' });
        return;
      }
      const link = target.closest('[data-tab]');
      if (link) { event.preventDefault(); location.hash = link.dataset.tab; menuOpen = false; render(); scrollActive(); }
    });
    root.addEventListener('auxclick', event => {
      const tab = event.target.closest('[data-navigation-tab]');
      if (event.button === 1 && tab) { event.preventDefault(); close(tab.dataset.navigationTab); }
    });
    root.addEventListener('keydown', event => {
      if (event.key === 'Escape' && menuOpen) { event.preventDefault(); setMenu(false); return; }
      const menu = event.target.closest('#copilot-nav-menu');
      if (menu && ['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
        const items = [...menu.querySelectorAll('[role="menuitem"]')], index = items.indexOf(document.activeElement);
        const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
        event.preventDefault(); items[next]?.focus(); return;
      }
      const item = event.target.closest('[data-navigation-tab]');
      if (!item) return;
      const tab = item.dataset.navigationTab;
      if (event.altKey && ['ArrowLeft', 'ArrowRight'].includes(event.key)) { event.preventDefault(); move(tab, event.key === 'ArrowLeft' ? -1 : 1); return; }
      if (event.key === 'Delete') { event.preventDefault(); close(tab); return; }
      if (!event.altKey && ['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {
        const index = current.indexOf(tab), next = event.key === 'Home' ? 0 : event.key === 'End' ? current.length - 1 :
          (index + (event.key === 'ArrowRight' ? 1 : -1) + current.length) % current.length;
        event.preventDefault(); focusTab(current[next]);
        root.querySelector('[data-navigation-tab="' + current[next] + '"]')?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
      }
    });
    root.addEventListener('dragstart', event => {
      const item = event.target.closest('[data-navigation-tab]');
      if (!loaded || !item || !event.dataTransfer) { event.preventDefault(); return; }
      dragged = item.dataset.navigationTab; event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', dragged); item.classList.add('is-dragging');
    });
    root.addEventListener('dragover', event => {
      const item = event.target.closest('[data-navigation-tab]');
      if (!dragged || !item || item.dataset.navigationTab === dragged) return;
      event.preventDefault(); if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
      for (const tab of root.querySelectorAll('.is-drop-target')) tab.classList.remove('is-drop-target');
      item.classList.add('is-drop-target');
    });
    root.addEventListener('drop', event => {
      const item = event.target.closest('[data-navigation-tab]');
      if (!dragged || !item || !current.includes(dragged) || item.dataset.navigationTab === dragged) return;
      event.preventDefault();
      const target = item.dataset.navigationTab, bounds = item.getBoundingClientRect(), next = current.filter(id => id !== dragged);
      const index = next.indexOf(target) + (event.clientX > bounds.left + bounds.width / 2 ? 1 : 0), source = dragged;
      dragged = ''; next.splice(index, 0, source); void persist(next, { focusId: source });
    });
    root.addEventListener('dragend', () => { dragged = ''; for (const item of root.querySelectorAll('.is-dragging,.is-drop-target')) item.classList.remove('is-dragging', 'is-drop-target'); });
    document.addEventListener('pointerdown', event => { if (menuOpen && !root.contains(event.target)) { menuOpen = false; render({ keepFocus: false }); } });
    window.addEventListener('hashchange', () => { ensure(active()); render(); scrollActive(); });
    window.addEventListener('resize', () => requestAnimationFrame(scrollActive));
    update(); ensure(active());
  }
  window.CopilotNavigation = { init, update, ensure };
})();
