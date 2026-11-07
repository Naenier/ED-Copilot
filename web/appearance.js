(function () {
  'use strict';
  const fields = { accent: 'Основной цвет HUD', text: 'Основной текст', muted: 'Второстепенный текст', bg: 'Фон окна', panel: 'Фон панелей', good: 'Успех и подтверждение', warn: 'Предупреждения', danger: 'Ошибки и опасность' };
  const keys = ['accent', 'accentBright', 'accentDim', 'text', 'muted', 'faint', 'bg', 'panel', 'panelRaised', 'line', 'good', 'warn', 'danger'];
  let context, draft = null, preview = null, busy = false, timer = null, serial = 0, manualPath = '';
  function applyPalette(palette) {
    if (!palette) return;
    for (const key of keys) if (/^#[0-9a-f]{6}$/i.test(palette[key] || '')) {
      document.documentElement.style.setProperty('--' + key.replace(/[A-Z]/g, char => '-' + char.toLowerCase()), palette[key]);
    }
    if (Number.isFinite(palette.overlayOpacity)) document.documentElement.style.setProperty('--overlay-opacity', String(Math.min(1, Math.max(.4, palette.overlayOpacity))));
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta && /^#[0-9a-f]{6}$/i.test(palette.bg || '')) meta.content = palette.bg;
  }
  function apply(theme) {
    if (preview && draft) return applyPalette(preview);
    if (!theme?.palette) return;
    applyPalette(theme.palette);
    try { localStorage.setItem('copilot-appearance', JSON.stringify(theme.palette)); } catch {}
  }
  try { applyPalette(JSON.parse(localStorage.getItem('copilot-appearance') || 'null')); } catch {}
  function settings(state) { return draft || state.theme?.settings || {}; }
  function render(state) {
    const U = window.MiningUI, h = U.h, theme = state.theme || {}, value = settings(state);
    const source = value.source;
    const palette = preview || theme.palette || {};
    const presets = theme.presets || [];
    const mode = value.mode || 'preset';
    return '<section class="panel appearance-panel"><div class="panel-heading"><h2>Оформление и цвета HUD</h2><button type="button" class="button button-small button-subtle" data-help-topic="appearance">? Как настроить</button></div><form id="appearance-form" class="panel-body">' +
      '<p>Шрифт Segoe UI поддерживает кириллицу. Выберите палитру, настройте цвета или скопируйте их из игры.</p>' +
      '<div class="appearance-presets">' + presets.map(p => '<button type="button" class="appearance-preset' + (mode === 'preset' && value.preset === p.id ? ' is-active' : '') + '" data-appearance-preset="' + h(p.id) + '"><i style="background:' + h(p.colors?.accent || '#28a0f0') + '"></i>' + h(p.name) + '</button>').join('') + '</div>' +
      '<div class="actions-wrap"><button class="button button-subtle" type="button" data-appearance-action="import"' + (busy ? ' disabled' : '') + '>Скопировать палитру из игры</button>' +
      (draft ? '<button class="button button-subtle" type="button" data-appearance-action="cancel">Отменить изменения</button>' : '') + '</div>' +
      '<p class="panel-note">Импорт читает активные настройки EDHM либо GraphicsConfigurationOverride.xml. Файлы игры остаются без изменений. Предпросмотр меняет это окно; сохранённая палитра применяется и к оверлею.</p>' +
      '<details class="appearance-import-path" data-preserve-open="appearance-import-path"><summary>Если конфигурация игры не найдена</summary><label class="field"><span>Путь к GraphicsConfigurationOverride.xml или XML-Profile.ini</span><input type="text" value="' + h(manualPath) + '" data-appearance-path data-focus="theme-import-path" placeholder="Оставьте пустым для автоматического поиска"></label><p class="panel-note">Укажите файл, который используется вашей игрой, и нажмите «Скопировать палитру из игры».</p></details>' +
      (source ? '<div class="appearance-source"><strong>' + h(source.name || source.kind) + '</strong><span>' + h(source.path || '') + '</span><small>' + h(source.modifiedAt ? 'Конфигурация изменена: ' + U.dateTime(source.modifiedAt) : '') + '</small>' + (source.notes || []).map(note => '<p>' + h(note) + '</p>').join('') + '</div>' : '') +
      '<div class="appearance-colors">' + Object.entries(fields).map(([key, label]) => { const color = mode === 'matrix' ? palette[key] : value.colors?.[key] || palette[key]; return '<label class="appearance-color"><span>' + h(label) + '</span><input type="color" name="' + key + '" data-appearance-color="' + key + '" data-focus="theme-' + key + '" value="' + h(color || '#000000') + '"><code>' + h(color || '') + '</code></label>'; }).join('') + '</div>' +
      '<label class="field appearance-opacity"><span>Непрозрачность фона оверлея · ' + Math.round((value.overlayOpacity ?? .82) * 100) + '%</span><input type="range" min="40" max="100" step="1" value="' + Math.round((value.overlayOpacity ?? .82) * 100) + '" data-appearance-opacity data-focus="theme-opacity"></label>' +
      '<div class="appearance-preview" aria-label="Предпросмотр палитры"><div><strong>ED COPILOT · ПРЕДПРОСМОТР</strong><span>Система · станция · положение корабля</span></div><p>Кириллица: Съешь ещё этих мягких французских булок, да выпей чаю.</p><div class="pill-row">' + U.pill('Подтверждено', 'confirmed') + U.pill('Проверьте данные', 'potential') + '<span class="appearance-danger">Нужна проверка</span></div></div>' +
      '<details class="appearance-matrix" data-preserve-open="appearance-matrix"><summary>Матрица RGB исходного оранжевого HUD игры</summary><p class="panel-note">Строки задают вклад исходного красного, зелёного и синего в новый цвет. Единичная матрица даёт исходный оранжевый HUD. Коэффициенты от −2 до 2; яркость ограничивается диапазоном экрана. EDHM дополнительно меняет отдельные элементы, поэтому его импорт учитывает и профиль цветов.</p><div class="appearance-matrix-grid"><span></span><span>R</span><span>G</span><span>B</span>' +
      ['Красный', 'Зелёный', 'Синий'].map((name, row) => '<span>' + name + '</span>' + [0, 1, 2].map(col => '<input type="number" min="-2" max="2" step="0.01" aria-label="' + name + ', канал ' + ['R', 'G', 'B'][col] + '" data-appearance-matrix="' + row + '-' + col + '" data-focus="theme-matrix-' + row + '-' + col + '" value="' + h(value.matrix?.[row]?.[col] ?? (row === col ? 1 : 0)) + '">').join('')).join('') + '</div></details>' +
      '<div class="settings-actions"><button class="button button-primary" type="submit"' + (busy ? ' disabled' : '') + '>' + (busy ? 'Сохраняем…' : 'Сохранить оформление') + '</button><span class="panel-note">' + (mode === 'game' ? 'Импортировано из игры' : mode === 'matrix' ? 'Матрица RGB' : mode === 'custom' ? 'Свои цвета' : 'Готовая палитра') + '</span></div></form></section>';
  }
  function edit() {
    if (!draft) draft = structuredClone(context.getState().theme?.settings || {});
    return draft;
  }
  async function requestPreview() {
    const id = ++serial;
    try {
      const result = await window.MiningUI.api('/api/theme/preview', { settings: draft });
      if (id !== serial || !draft) return;
      preview = result.theme.palette;
      applyPalette(preview);
      context.render();
    } catch (error) { if (id === serial) context.showToast(error.message, true); }
  }
  function schedule() { clearTimeout(timer); timer = setTimeout(requestPreview, 160); }
  function init(api) {
    context = api;
    document.addEventListener('input', event => {
      const input = event.target;
      if (!input.closest('#appearance-form')) return;
      if (input.hasAttribute('data-appearance-path')) { manualPath = input.value; return; }
      if (input.dataset.appearanceColor) {
        const value = edit();
        const current = value.mode === 'matrix' ? Object.fromEntries(Object.keys(fields).map(key => [key, (preview || context.getState().theme.palette)[key]])) : value.colors;
        value.colors = { ...current, [input.dataset.appearanceColor]: input.value }; value.mode = 'custom'; value.source = null; schedule();
      } else if (input.hasAttribute('data-appearance-opacity')) { edit().overlayOpacity = Number(input.value) / 100; schedule(); }
      else if (input.dataset.appearanceMatrix) {
        if (input.value === '' || !Number.isFinite(input.valueAsNumber)) return;
        const [row, col] = input.dataset.appearanceMatrix.split('-').map(Number), value = edit();
        value.matrix ??= [[1, 0, 0], [0, 1, 0], [0, 0, 1]]; value.matrix[row][col] = input.valueAsNumber; value.mode = 'matrix'; value.source = null; schedule();
      }
    });
    document.addEventListener('click', async event => {
      const preset = event.target.closest('[data-appearance-preset]');
      if (preset) {
        const value = edit(); value.mode = 'preset'; value.preset = preset.dataset.appearancePreset; value.source = null;
        const selected = context.getState().theme?.presets.find(p => p.id === value.preset);
        if (selected) { value.colors = { ...selected.colors }; value.matrix = structuredClone(selected.matrix); }
        await requestPreview(); return;
      }
      const button = event.target.closest('[data-appearance-action]');
      if (!button || busy) return;
      if (button.dataset.appearanceAction === 'cancel') {
        ++serial; clearTimeout(timer); draft = preview = null; apply(context.getState().theme); context.render();
      } else if (button.dataset.appearanceAction === 'import') {
        ++serial; clearTimeout(timer);
        busy = true; context.render();
        try {
          const result = await window.MiningUI.api('/api/theme/import', manualPath.trim() ? { filePath: manualPath.trim() } : {});
          const opacity = settings(context.getState()).overlayOpacity ?? .82;
          result.theme.settings.overlayOpacity = opacity; result.theme.palette.overlayOpacity = opacity;
          ++serial; clearTimeout(timer); draft = result.theme.settings; preview = result.theme.palette; applyPalette(preview);
          context.showToast('Цвета прочитаны. Нажмите «Сохранить оформление», чтобы применить их к оверлею.');
        } catch (error) { context.showToast(error.message, true); }
        finally { busy = false; context.render(); }
      }
    });
    document.addEventListener('submit', async event => {
      if (event.target.id !== 'appearance-form') return;
      event.preventDefault(); if (busy) return;
      ++serial; clearTimeout(timer); busy = true;
      const value = structuredClone(settings(context.getState())); context.render();
      try {
        const result = await context.perform('/api/theme', { settings: value }, 'Оформление сохранено');
        if (result) { draft = preview = null; apply(context.getState().theme); }
      } finally { busy = false; context.render(); }
    });
  }
  window.CopilotAppearance = { apply, render, init };
})();
