(() => {
  'use strict';
  const bound = new WeakSet();
  const rowsPerPage = 80;
  const active = C => C.tab === 'ship-route';
  const label = (u,key) => u.esc(u.t(key));
  const format = (u,value,unit='') => value==null ? '—' : u.esc(u.fmt(value)+u.t(unit));
  const sourceLabels = {game:'Маршрут из игры',manual:'Путевые точки',neutron:'Нейтронный маршрут',exact:'Точный маршрут'};
  const classLabels = {scoopable:'Подходит для заправки',neutron:'Нейтронная звезда',unscoopable:'Заправка недоступна',unknown:'Класс звезды неизвестен'};
  const fields = [
    ['baseMass','Масса корабля без груза и топлива, т',1,100000],
    ['optimalMass','Оптимальная масса FSD, т',1,100000],
    ['fuelPower','Степень расхода FSD',1,5],
    ['fuelMultiplier','Коэффициент расхода FSD',0.000001,1],
    ['maxFuelPerJump','Макс. топлива на прыжок, т',0.01,100],
    ['tankSize','Основной бак, т',1,2000],
    ['internalTankSize','Внутренний бак, т',0,100],
    ['reserveSize','Оставить в резерве, т',0,2000],
    ['rangeBoost','Добавка дальности, св. л.',0,50]
  ];
  const input = (u,key,title,value,attrs='') => `<label>${label(u,title)}<input name="${key}" value="${u.esc(value ?? '')}" ${attrs}></label>`;
  const action = (u,key,title,extra='') => `<button type="button" class="pill quiet" data-ship-route-action="${key}" ${extra}>${label(u,title)}</button>`;

  function controls(model,u,draft={}) {
    const status = model.search?.status;
    const busy = ['queued','running','searching'].includes(status) || model.search?.running===true;
    return `<section class="panel ship-route-panel">
      <div class="ship-route-bar"><h2>${label(u,'Новое путешествие')}</h2>${action(u,'game','Взять маршрут из игры',model.gameRouteKnown?'':'disabled')}</div>
      <details><summary>${label(u,'Импорт путевых точек')}</summary>
        <form data-ship-route-form="import" class="ship-route-form">
          ${input(u,'name','Название путешествия','','maxlength="120"')}
          <label class="ship-route-wide">${label(u,'Системы — по одной в строке')}
          <textarea name="waypoints" data-system-search="multiline" rows="5" required></textarea></label>
          <button class="pill primary" type="submit">${label(u,'Сохранить путешествие')}</button>
        </form>
      </details>
      <details data-ship-route-calc ${busy||model.search?.error?'open':''}><summary>${label(u,'Рассчитать маршрут корабля')}</summary>
        <form data-ship-route-form="calculate" class="ship-route-form">
          ${input(u,'origin','Откуда',draft.origin??model.currentSystem,'data-system-search="single" maxlength="160" required')}
          ${input(u,'destination','Куда',draft.destination,'data-system-search="single" maxlength="160" required')}
          <label>${label(u,'Способ расчёта')}<select name="mode"><option value="neutron">${label(u,sourceLabels.neutron)}</option><option value="exact" ${draft.mode==='exact'?'selected':''}>${label(u,sourceLabels.exact)}</option></select></label>
          ${input(u,'jumpRangeLy','Дальность с грузом, св. л.',draft.jumpRangeLy??model.suggestedJumpRangeLy,'type="number" step="any" min="1" max="500"')}
          ${model.suggestedJumpRangeLy>0?`<p class="ship-route-wide info-note">${label(u,'Из журнала подставляется максимальная дальность. Укажите дальность с текущим грузом.')}</p>`:''}
          ${input(u,'estimatedFuel','Макс. топлива на прыжок, т',draft.estimatedFuel,'type="number" step="any" min="0" max="100" '+(draft.mode==='exact'?'disabled':''))}
          <label class="check-label"><input name="autoCopy" type="checkbox" ${draft.autoCopy?'checked':''}>${label(u,'Копировать следующую систему после прибытия')}</label>
          <div data-ship-neutron class="ship-route-wide info-note" ${draft.mode==='exact'?'hidden':''}>${label(u,'Нейтронный расчёт даёт ориентиры. Промежуточные прыжки прокладываются в игре.')}</div>
          <fieldset data-ship-exact class="ship-route-wide ship-route-form" ${draft.mode==='exact'?'':'hidden disabled'}>
            <legend>${label(u,'Характеристики корабля для точного расчёта')}</legend>
            ${fields.map(([key,title,min,max])=>input(u,key,title,draft[key]??(key==='tankSize'?model.fuel?.mainCapacityTons:(min===0?0:'')),`type="number" step="any" min="${min}" max="${max}" required`)).join('')}
            ${input(u,'cargo','Груз, т',draft.cargo??model.cargoTons??0,'type="number" step="any" min="0" max="10000" required')}
            <label class="check-label"><input name="useSupercharge" type="checkbox" ${draft.useSupercharge?'checked':''}>${label(u,'Использовать нейтронный разгон')}</label>
          </fieldset>
          <button class="pill primary" type="submit" ${busy?'disabled':''}>${label(u,'Рассчитать')}</button>
          ${busy?action(u,'cancel','Остановить расчёт'):''}
          ${busy?`<p class="ship-route-wide info-note" role="status">${label(u,'Рассчитываем маршрут…')}</p>`:''}
          ${model.search?.error?`<p class="ship-route-wide info-note" role="status">${label(u,model.search.error)}</p>`:''}
        </form>
      </details>
    </section>`;
  }

  function routeView(C,model,u) {
    const route=model.route;
    if (!route) return '';
    const next=route.nextStop, stops=route.stops||[];
    const isManual=['manual','game'].includes(route.mode);
    const exact=route.mode==='exact';
    const lastPage=Math.max(0,Math.ceil(stops.length/rowsPerPage)-1);
    const livePage=Math.min(lastPage,Math.floor((route.next||0)/rowsPerPage));
    const requested=C.shipRoutePage?.id===route.id?C.shipRoutePage.page:null;
    const page=requested==null?livePage:Math.min(lastPage,Math.max(0,requested));
    const offset=page*rowsPerPage;
    const saved=(model.saved||[]).map(item=>`<option value="${u.esc(item.id)}"${item.active?' selected':''}>${u.esc(item.name)}</option>`).join('');
    const instruction=row=>[row.station&&u.esc(row.station),row.action&&u.esc(row.action),
      row.refuel&&label(u,'Заправиться'),row.boost&&label(u,'Разогнать FSD'),row.fuelCaution&&label(u,'Проверьте запас топлива'),
      row.rangeCaution&&label(u,'Проверьте дальность прыжка')].filter(Boolean).join(' · ') || '—';
    const rows=stops.slice(offset,offset+rowsPerPage).map((row,i)=>{
      const index=offset+i;
      return `<tr class="${row.visited?'is-visited':index===route.next?'is-next':''}">
      <th scope="row"><span>${index+1}.</span> ${u.esc(row.system)}${action(u,'copy','Копировать систему',`data-system="${u.esc(row.system)}"`)}</th>
      <td>${format(u,row.distanceLy,' св. л.')}${row.estimatedJumps>1?`<small>≥ ${format(u,row.estimatedJumps)} ${label(u,'прыжков')}</small>`:row.jumps>1?`<small>≈ ${format(u,row.jumps)} ${label(u,'прыжков')}</small>`:''}</td>
      <td>${row.starClass&&row.starClass!=='scoopable'?`<strong>${u.esc(row.starClass)}</strong> `:''}${label(u,classLabels[row.starKind]||classLabels.unknown)}</td>
      <td>${format(u,row.distanceToScoopLy,' св. л.')}</td>
      ${exact?`<td>${format(u,row.fuelUsed,' т')}</td><td>${format(u,row.fuelLeft,' т')}</td>`:''}
      <td>${instruction(row)}</td>
      ${isManual?`<td>${row.visited?'':action(u,'move','Выше',`data-index="${index}" data-delta="-1" ${index<=route.next?'disabled':''}`)+action(u,'move','Ниже',`data-index="${index}" data-delta="1" ${index===stops.length-1?'disabled':''}`)}</td>`:''}
    </tr>`;}).join('');
    return `<section class="panel ship-route-panel">
      <header class="ship-route-bar"><div><h2>${u.esc(route.name)}</h2><p>${label(u,sourceLabels[route.mode])}</p></div>
      <label>${label(u,'Путешествия')} <select data-ship-route-pick>${saved}</select></label></header>
      <div class="ship-route-metrics">
        <div><small>${label(u,'Следующая остановка')}</small><strong>${next?u.esc(next.system):label(u,'Маршрут пройден')}</strong>${next?.station?`<small>${u.esc(next.station)}</small>`:''}</div>
        <div><small>${label(u,'Пройдено точек')}</small><strong>${format(u,route.next)} / ${format(u,stops.length)}</strong></div>
        <div><small>${label(u,'Топливо на борту')}</small><strong>${format(u,model.fuel?.mainTons,' т')}</strong></div>
        ${route.fuelSafeJumps==null?'':`<div><small>${label(u,'Прыжки при максимальном расходе')}</small><strong>${format(u,route.fuelSafeJumps)}</strong></div>`}
      </div>
      <div class="ship-route-bar">${next?action(u,'copy','Копировать следующую систему',`data-system="${u.esc(next.system)}"`):''}
      <label class="check-label"><input type="checkbox" data-ship-route-autocopy ${route.autoCopy?'checked':''}>${label(u,'Копировать следующую систему после прибытия')}</label>
      ${action(u,'remove','Удалить путешествие',`data-route-id="${u.esc(route.id)}"`)}</div>
      <div class="ship-route-table"><table><thead><tr><th scope="col">${label(u,'Система')}</th><th scope="col">${label(u,'Расстояние')}</th>
      <th scope="col">${label(u,'Звезда')}</th><th scope="col">${label(u,'До звезды для заправки')}</th>
      ${exact?`<th scope="col">${label(u,'Расход топлива')}</th><th scope="col">${label(u,'Расчётный остаток топлива')}</th>`:''}
      <th scope="col">${label(u,'Действие')}</th>${isManual?`<th scope="col">${label(u,'Порядок')}</th>`:''}</tr></thead><tbody>${rows}</tbody></table></div>
      ${lastPage?`<div class="ship-route-pagination">
        ${action(u,'page','Назад',`data-page="${page-1}" ${page===0?'disabled':''}`)}
        <span>${format(u,offset+1)}–${format(u,Math.min(offset+rowsPerPage,stops.length))} / ${format(u,stops.length)}</span>
        ${action(u,'page','Далее',`data-page="${page+1}" ${page===lastPage?'disabled':''}`)}
        ${requested!=null?action(u,'follow','К следующей остановке'):''}</div>`:''}
      ${route.mode==='neutron'?`<p class="info-note">${label(u,'Нейтронный расчёт даёт ориентиры. Промежуточные прыжки прокладываются в игре.')}</p>`:''}
      ${exact?`<p class="info-note">${label(u,'Spansh начинает расчёт с полного основного бака.')}</p>`:''}
    </section>`;
  }

  function render(C,u) {
    const model=C.state.shipRoute||{};
    const header=u.heading(u.t('Маршрут корабля'));
    const owner=C.state.planWorkspace?.key||'';
    if (C.shipRouteOwner!==owner) { C.shipRouteOwner=owner; C.shipRouteDraft={}; C.shipRoutePage=null; }
    if (!model.ownerKnown) return header+`<div class="panel empty-state"><strong>${label(u,'Сначала подключите журнал командира.')}</strong></div>`;
    return header+`<div data-ship-route data-plan-workspace="${u.esc(owner)}">${routeView(C,model,u)}${controls(model,u,C.shipRouteDraft)}</div>`;
  }

  function init(C,u) {
    const root=active(C)&&u.$('[data-ship-route]');
    if (!root||bound.has(root)) return;
    bound.add(root);
    const expectedPlanWorkspace=root.dataset.planWorkspace;
    const request=async (command,body) => {
      try {
        await u.call(command,{...u.context(),...body,expectedPlanWorkspace});
        await u.loadView();
      } catch (error) { u.toast(error.message,true); }
    };
    const rememberInput=event=>{
      const node=event.target;
      if (node.form?.dataset.shipRouteForm!=='calculate'||!node.name||C.shipRouteOwner!==expectedPlanWorkspace) return;
      C.shipRouteDraft[node.name]=node.type==='checkbox'?node.checked:node.value;
    };
    root.addEventListener('input',rememberInput);
    root.addEventListener('click',async event=>{
      const button=event.target.closest('[data-ship-route-action]');
      if (!button) return;
      const key=button.dataset.shipRouteAction;
      if (key==='page'||key==='follow') {
        if (C.shipRouteOwner!==expectedPlanWorkspace) return;
        C.shipRoutePage=key==='follow'?null:{id:C.state.shipRoute?.route?.id,page:Number(button.dataset.page)};
        u.renderPage();
        return;
      }
      if (key==='copy') {
        try { await u.call('native',{action:'copy',text:button.dataset.system}); u.toast(u.t('Система скопирована')); }
        catch (error) { u.toast(error.message,true); }
        return;
      }
      if (key==='remove') {
        u.dialog(u.t('Удалить путешествие?'),'',`<button class="pill primary" type="button" data-ship-route-confirm>${label(u,'Удалить')}</button>`);
        u.$('[data-ship-route-confirm]')?.addEventListener('click',()=>{u.closeLayer(); void request('ship-route',{action:'remove',id:button.dataset.routeId});});
        return;
      }
      await request('ship-route',{action:key,index:Number(button.dataset.index||0),delta:Number(button.dataset.delta||0)});
    });
    root.addEventListener('change',event=>{
      rememberInput(event);
      const node=event.target;
      if (node.matches('[data-ship-route-pick]')) {
        if (C.shipRouteOwner===expectedPlanWorkspace) C.shipRoutePage=null;
        void request('ship-route',{action:'activate',id:node.value});
      }
      if (node.matches('[data-ship-route-autocopy]')) void request('ship-route',{action:'autocopy',enabled:node.checked});
      if (node.name==='mode') {
        const section=node.form.querySelector('[data-ship-exact]');
        section.hidden=section.disabled=node.value!=='exact';
        node.form.querySelector('[data-ship-neutron]').hidden=node.value==='exact';
        node.form.elements.estimatedFuel.disabled=node.value==='exact';
      }
    });
    root.addEventListener('submit',event=>{
      event.preventDefault();
      const form=event.target, data=new FormData(form);
      if (form.dataset.shipRouteForm==='import') {
        void request('ship-route',{action:'import',name:data.get('name'),waypoints:data.get('waypoints')});
        return;
      }
      const mode=data.get('mode');
      const values={origin:data.get('origin'),destination:data.get('destination'),mode,autoCopy:data.has('autoCopy')};
      if (data.get('jumpRangeLy')) values.jumpRangeLy=Number(data.get('jumpRangeLy'));
      if (mode==='exact') {
        const build=Object.fromEntries(fields.map(([key])=>[key,Number(data.get(key))]));
        build.useSupercharge=data.has('useSupercharge');
        values.shipBuild=JSON.stringify(build);
        values.cargo=Number(data.get('cargo'));
      } else if (data.get('estimatedFuel')) values.maxFuelPerJump=Number(data.get('estimatedFuel'));
      void request('ship-route.calculate',values);
    });
  }
  window.ShipRouteUI={active,render,init};
})();
