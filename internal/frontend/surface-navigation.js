(() => {
  'use strict';
  const bound = new WeakSet();
  const active = C => C.tab === 'surface-navigation';
  const label = (u, value, vars) => u.esc(u.t(value, vars));
  const known = value => typeof value === 'number' && Number.isFinite(value);
  const coordinatesKnown = value => known(value?.latitude) && known(value?.longitude);
  const coordinateText = (value, format = String) => coordinatesKnown(value)
    ? [format(value.latitude), format(value.longitude)].join(' / ')
    : '';
  const coordinateToken = '[+-]?(?:\\d+(?:[.,]\\d+)?|[.,]\\d+)';
  const separatedPair = new RegExp('^(' + coordinateToken + ')\\s*(?:[/;]\\s*|\\s+)(' + coordinateToken + ')$');
  const commaPair = /^([+-]?(?:\d+(?:\.\d+)?|\.\d+))\s*,\s*([+-]?(?:\d+(?:\.\d+)?|\.\d+))$/;

  function parseCoordinates(value) {
    const text = String(value ?? '').trim().replaceAll('−', '-');
    const match = separatedPair.exec(text) || commaPair.exec(text);
    if (!match) return null;
    const [latitude, longitude] = match.slice(1).map(part => Number(part.replace(',', '.')));
    if (!Number.isFinite(latitude) || Math.abs(latitude) > 90 ||
      !Number.isFinite(longitude) || Math.abs(longitude) > 180) return null;
    return {latitude, longitude};
  }

  function draft(C) {
    const owner = C.state.surfaceNavigation?.owner ?? C.state.planWorkspace?.key ?? '';
    if (C.surfaceNavigationDraft?.owner !== owner) C.surfaceNavigationDraft = {owner, value: null, busy: false, error: ''};
    return C.surfaceNavigationDraft;
  }

  function latest(C, projection) {
    const previous = C.surfaceNavigationLatest;
    if (known(previous?.revision) && known(projection?.revision) && projection.revision < previous.revision) return previous;
    if (projection) C.surfaceNavigationLatest = projection;
    return projection;
  }

  function receive(C, u, projection) {
    if (!active(C) || !projection) return;
    const previousOwner = draft(C).owner;
    C.state.surfaceNavigation = latest(C, projection);
    if (C.page && !C.layer) {
      if (draft(C).owner !== previousOwner) u.renderPage();
      else refresh(C, u);
    }
  }

  const statuses = {
    'waiting-position': 'Ожидаем координаты от игры. Приблизьтесь к поверхности.',
    'wrong-body': 'Цель на другом небесном теле',
    arrived: 'Точка достигнута'
  };

  function metrics(model, u) {
    const {target, current, guidance, status} = model;
    const coordinateFormat = new Intl.NumberFormat(u.locale(), {useGrouping: false, minimumFractionDigits: 4, maximumFractionDigits: 8});
    const coordinate = point => coordinateText(point, value => coordinateFormat.format(value)) || '—';
    const body = point => [point?.systemName, point?.bodyName].filter((value, i, names) => value && names.indexOf(value) === i).join(' · ');
    const live = status === 'active' || status === 'arrived';
    const bearing = live && known(guidance?.bearing) ? u.fmt((Math.round(guidance.bearing * 10) % 3600) / 10) + '°' : '—';
    const relative = live && known(guidance?.relativeHeading) ? Math.round(guidance.relativeHeading) : null;
    const direction = relative === null ? '' : relative === 0 ? u.t('Прямо') :
      u.t(relative < 0 ? 'Левее на {angle}°' : 'Правее на {angle}°', {angle: u.fmt(Math.abs(relative))});
    const meters = live && known(guidance?.distanceM) ? guidance.distanceM : null;
    const distance = meters === null ? '—' : meters < 1000 ? u.fmt(Math.round(meters)) + u.t(' м') : u.fmt(meters / 1000) + u.t(' км');
    const card = (title, value, detail, icon) => `<div class="surface-nav-reading">
      <dt>${u.icon(icon)}${label(u, title)}</dt><dd>${u.esc(value)}</dd>${detail ? `<dd class="surface-nav-detail">${u.esc(detail)}</dd>` : ''}
    </div>`;
    return `<dl class="surface-nav-readings">
      ${card('Текущие координаты', coordinate(current), body(current), 'compass')}
      ${card('Координаты цели', coordinate(target), body(target), 'map-pin')}
      ${card('Курс к цели', bearing, status === 'active' && bearing === '—' ? u.t('Курс не определён') : direction, 'arrow-up-right')}
      ${card('По поверхности', distance, live && meters === null ? u.t('Радиус тела неизвестен') : '', 'route')}
    </dl>`;
  }

  function render(C, u) {
    const model = C.state.surfaceNavigation || {};
    const state = draft(C);
    return u.heading(u.t('Полёт по координатам')) + `<section class="panel surface-nav" data-surface-navigation data-plan-workspace="${u.esc(state.owner)}">
      <form class="surface-nav-form" data-surface-nav-form novalidate>
        <label for="surface-nav-coordinates">${label(u, 'Широта / долгота')}
          <input id="surface-nav-coordinates" name="coordinates" type="text" maxlength="96" placeholder="44.4825 / -61.4854"
            autocomplete="off" spellcheck="false" aria-describedby="surface-nav-error" ${state.error ? 'aria-invalid="true"' : ''}
            value="${u.esc(state.value ?? coordinateText(model.target))}">
        </label>
        <button type="submit" class="pill primary" ${state.busy ? 'disabled' : ''}>${u.icon('map-pin')}${label(u, 'Задать цель')}</button>
        <button type="button" class="pill quiet" data-surface-nav-clear ${!model.target ? 'hidden' : ''} ${state.busy ? 'disabled' : ''}>${label(u, 'Снять цель')}</button>
        <p class="surface-nav-error" id="surface-nav-error" role="alert" ${state.error ? '' : 'hidden'}>${label(u, state.error)}</p>
      </form>
      <div data-surface-nav-readings>${metrics(model, u)}</div>
      <p class="surface-nav-status" data-surface-nav-status role="status" ${statuses[model.status] ? '' : 'hidden'}>${statuses[model.status] ? label(u, statuses[model.status]) : ''}</p>
    </section>`;
  }

  function refresh(C, u, projection = C.state.surfaceNavigation) {
    const root = active(C) && u.$('[data-surface-navigation]');
    if (!root || !projection) return;
    const readings = root.querySelector('[data-surface-nav-readings]');
    const markup = metrics(projection, u);
    if (readings.surfaceMarkup !== markup) {
      readings.innerHTML = markup;
      readings.surfaceMarkup = markup;
    }
    const status = root.querySelector('[data-surface-nav-status]');
    const message = statuses[projection.status] ? u.t(statuses[projection.status]) : '';
    if (status.textContent !== message) status.textContent = message;
    status.hidden = !message;
    root.querySelector('[data-surface-nav-clear]').hidden = !projection.target;
  }

  function init(C, u) {
    const root = active(C) && u.$('[data-surface-navigation]');
    if (!root || bound.has(root)) return;
    bound.add(root);
    const form = root.querySelector('[data-surface-nav-form]');
    const input = form.elements.coordinates;
    const clear = form.querySelector('[data-surface-nav-clear]');
    const state = draft(C);
    const expectedPlanWorkspace = root.dataset.planWorkspace;
    const syncForm = () => {
      const liveForm = active(C) && u.$('[data-surface-nav-form]');
      if (!liveForm || draft(C) !== state) return;
      liveForm.querySelector('[type="submit"]').disabled = state.busy;
      liveForm.querySelector('[data-surface-nav-clear]').disabled = state.busy;
      const error = liveForm.querySelector('#surface-nav-error');
      error.textContent = u.t(state.error);
      error.hidden = !state.error;
      if (state.error) liveForm.elements.coordinates.setAttribute('aria-invalid', 'true');
      else liveForm.elements.coordinates.removeAttribute('aria-invalid');
      if (state.value === null) liveForm.elements.coordinates.value = coordinateText(C.state.surfaceNavigation?.target);
    };
    const update = async (params, enteredValue) => {
      if (state.busy || draft(C) !== state) return;
      state.busy = true;
      state.error = '';
      syncForm();
      try {
        const result = await u.call('surface-navigation.update', {...params, expectedPlanWorkspace});
        if (draft(C) !== state) return;
        if (result?.surfaceNavigation) {
          C.state.surfaceNavigation = latest(C, result.surfaceNavigation);
          if (draft(C) !== state) return;
          refresh(C, u);
        }
        // Keep text the pilot started editing during the save.
        if (state.value === enteredValue) state.value = null;
      } catch (failure) {
        state.error = failure.message;
      } finally {
        state.busy = false;
        syncForm();
      }
    };
    input.addEventListener('input', () => {
      if (draft(C) !== state) return;
      state.value = input.value;
      if (state.error) {
        state.error = '';
        syncForm();
      }
    });
    form.addEventListener('submit', event => {
      event.preventDefault();
      if (state.busy || draft(C) !== state) return;
      const point = parseCoordinates(input.value);
      if (!point) {
        state.value = input.value;
        state.error = 'Введите широту от −90 до 90 и долготу от −180 до 180.';
        syncForm();
        input.focus();
        return;
      }
      state.value = input.value;
      return update({action: 'set', ...point}, input.value);
    });
    clear.addEventListener('click', () => update({action: 'clear'}, input.value));
  }

  async function smoke(C, u) {
    const root = u.$('[data-surface-navigation]');
    const form = root?.querySelector('[data-surface-nav-form]');
    const input = form?.elements.coordinates;
    const submit = form?.querySelector('[type="submit"]');
    const clear = form?.querySelector('[data-surface-nav-clear]');
    if (!input || !submit || !clear) return [{name: 'Surface coordinates form renders', ok: false}];
    const waitForSave = async () => {
      for (let tick = 0; tick < 80 && draft(C).busy; tick++) await new Promise(resolve => setTimeout(resolve, 25));
      return !draft(C).busy && !draft(C).error;
    };
    input.value = '44,4825 / −61,4854';
    input.dispatchEvent(new Event('input', {bubbles: true}));
    form.requestSubmit(submit);
    const saved = await waitForSave();
    const target = C.state.surfaceNavigation?.target;
    const reads = root.querySelector('[data-surface-nav-readings]');
    const shownLatitude = new Intl.NumberFormat(u.locale(), {useGrouping: false, maximumFractionDigits: 4}).format(44.4825);
    const check = (name, ok) => ({name, ok: !!ok});
    const results = [check('Surface coordinates accepts a pasted decimal-comma pair',
      saved && target?.latitude === 44.4825 && target?.longitude === -61.4854 && reads?.textContent.includes(shownLatitude))];
    clear.click();
    const cleared = await waitForSave();
    results.push(check('Surface coordinates clears the target in the workspace', cleared && !C.state.surfaceNavigation?.target && clear.hidden && !input.value));
    return results;
  }

  window.SurfaceNavigationUI = {active, render, init, receive, latest, smoke};
})();
