(() => {
  'use strict';
  const bound=new WeakSet(), scales=[0,100,250,500,1000,2500,5000,10000,25000];
  const active=C=>C.tab==='exobiology'&&C.mode==='Радар поверхности';
  const text=(u,key)=>u.esc(u.t(key));
  const distance=(u,value)=>value==null?'—':u.esc(u.fmt(value)+u.t(' м'));
  const pointLabel=(u,point)=>point.kind==='ship'?text(u,'Корабль'):u.esc(point.label||u.t('Точка поверхности'));
  const kind=point=>point.uncertaintyM>0?'estimated':['ship','srv','sample'].includes(point.kind)?point.kind:'waypoint';
  const legends={ship:'Корабль',srv:'ТРП',sample:'Образец',waypoint:'Личная точка',estimated:'Приблизительное место'};

  function plot(model,u) {
    const range=Math.max(100,Number(model.scaleM)||500);
    const size=160,center=180;
    const pos=value=>center+Math.max(-5,Math.min(5,Number(value)/range))*size;
    const pointSvg=point=>{
      const d=Math.hypot(point.x,point.y), category=kind(point);
      if(d>range+Math.max(point.exclusionM||0,point.uncertaintyM||0)&&!point.target) return '';
      const x=pos(point.x),y=pos(point.y);
      const ring=(radius,style)=>radius>0?`<circle class="${style}" cx="${x}" cy="${y}" r="${Math.min(600,radius/range*size)}"/>`:'';
      const zones=ring(point.exclusionM,'sample-exclusion')+ring(point.uncertaintyM,'search-area');
      if(d>range&&!point.target)return zones;
      const factor=d>range?(range*.94)/d:1;
      const dx=pos(point.x*factor),dy=pos(point.y*factor);
      const marker=category==='ship'||category==='srv'||point.locationBasis==='scanned-from'
        ?`<rect x="${dx-4}" y="${dy-4}" width="8" height="8" rx="1"/>`
        :`<circle cx="${dx}" cy="${dy}" r="4"/>`;
      return zones+`<g class="radar-mark ${category}${point.target?' is-target':''}"><title>${pointLabel(u,point)}</title>${marker}
      ${point.target?`<circle class="target-ring" cx="${dx}" cy="${dy}" r="9"/>`:''}</g>`;
    };
    const trail=(model.trail||[]).map((point,index)=>`${index===0||point.break?'M':'L'}${pos(point.x)},${pos(point.y)}`).join(' ');
    const angle=model.headingUp?-(Number(model.heading)||0):0;
    return `<svg class="surface-radar-map${model.stale?' is-stale':''}" viewBox="0 0 360 360" role="img" aria-label="${text(u,'Карта поверхности')}">
      <defs><clipPath id="surface-radar-clip"><circle cx="180" cy="180" r="160"/></clipPath></defs>
      <circle class="radar-grid" cx="180" cy="180" r="160"/><circle class="radar-grid" cx="180" cy="180" r="80"/>
      <path class="radar-grid" d="M20 180H340 M180 20V340"/>
      <text class="radar-north" x="180" y="33" transform="rotate(${angle} 180 180)" text-anchor="middle">N</text>
      <g clip-path="url(#surface-radar-clip)"><path class="radar-trail" d="${trail}"/>
      ${(model.points||[]).map(pointSvg).join('')}</g>
      <path class="radar-player" d="M180 172 L186 185 L180 182 L174 185 Z"/>
    </svg>`;
  }

  function render(C,u) {
    const model=C.state.surfaceRadar||C.state.exobiology?.surfaceRadar||{};
    const heading=u.heading(u.t('Радар поверхности'))+u.renderModes(true);
    if (!model.available) return heading+`<section class="panel empty-state"><strong>${text(u,'Позиция поверхности недоступна')}</strong><p>${text(u,'Подлетите к поверхности, чтобы получить координаты и курс.')}</p></section>`;
    const points=model.points||[],target=model.target;
    const scale=scales.map(value=>`<option value="${value}"${value===model.selectedScaleM?' selected':''}>${value===0?text(u,'Автоматически'):distance(u,value)}</option>`).join('');
    const list=points.map(point=>`<button type="button" class="surface-radar-point${point.target?' is-target':''}" data-radar-target="${u.esc(point.id)}">
      <span class="radar-key ${kind(point)}" aria-hidden="true"></span><strong>${pointLabel(u,point)}</strong><span>${model.stale?'':(point.uncertaintyM>0?'≈ ':'')+distance(u,point.distanceM)+' · '+u.esc(u.fmt(point.bearing))+'°'}</span></button>`).join('');
    const keys=[...new Set(points.map(kind))].map(item=>`<span><i class="radar-key ${item}"></i>${text(u,legends[item])}</span>`).join('');
    return heading+`<section class="panel surface-radar-panel" data-surface-radar>
      <header><div><h2>${u.esc(model.body)}</h2><span>${model.headingUp?text(u,'Курс вверх'):text(u,'Север вверх')} · ${distance(u,model.scaleM)}</span></div>
        <label>${text(u,'Масштаб')} <select data-radar-scale>${scale}</select></label></header>
      ${model.stale?`<p class="surface-radar-warning" role="status">${text(u,'Позиция не обновляется')}</p>`:''}
      <div class="surface-radar-layout"><div>${plot(model,u)}<div class="surface-radar-legend">${keys}</div></div>
        <div class="surface-radar-side">
        ${target?`<div class="surface-radar-target"><span>${text(u,'Цель')}</span><strong>${pointLabel(u,target)}</strong>
          ${model.stale?'':`<span>${target.uncertaintyM>0?'≈ ':''}${distance(u,target.distanceM)} · ${u.esc(u.fmt(target.bearing))}°</span>`}
          <button type="button" class="pill quiet" data-radar-target="">${text(u,'Снять цель')}</button></div>`:''}
        ${list?`<div class="surface-radar-list">${list}</div>`:''}
        <div class="surface-radar-buttons">
        ${model.stale?'':`<button type="button" class="pill quiet" data-radar-action="waypoint">${text(u,'Отметить мою позицию')}</button>`}
        ${model.trail?.length?`<button type="button" class="pill quiet" data-radar-action="radar-clear-track">${text(u,'Очистить трек')}</button>`:''}
        </div></div>
      </div></section>`;
  }

  function init(C,u) {
    const root=active(C)&&u.$('[data-surface-radar]');
    if(!root||bound.has(root))return;
    bound.add(root);
    const update=async values=>{
      const model=C.state.surfaceRadar||C.state.exobiology?.surfaceRadar;
      try{await u.call('exobiology',{values:{bodyKey:model?.bodyKey,...values}});await u.loadView();}
      catch(error){u.toast(error.message,true);}
    };
    root.addEventListener('change',event=>{
      if(event.target.matches('[data-radar-scale]'))void update({action:'radar-scale',scaleM:Number(event.target.value)});
    });
    root.addEventListener('click',event=>{
      const point=event.target.closest('[data-radar-target]');
      if(point){void update({action:'radar-target',id:point.dataset.radarTarget});return;}
      const control=event.target.closest('[data-radar-action]');
      if(!control)return;
      if(control.dataset.radarAction==='waypoint'){
        const model=C.state.surfaceRadar||C.state.exobiology?.surfaceRadar;
        if(model?.available&&!model.stale)void update({action:'add-waypoint',bodyKey:model.bodyKey,latitude:model.latitude,longitude:model.longitude});
      }else{
        u.dialog(u.t('Очистить трек этого тела?'),'',`<button type="button" class="pill primary" data-radar-confirm>${text(u,'Очистить трек')}</button>`);
        const model=C.state.surfaceRadar||C.state.exobiology?.surfaceRadar;
        u.$('[data-radar-confirm]')?.addEventListener('click',()=>{u.closeLayer();void update({action:'radar-clear-track',bodyKey:model?.bodyKey});});
      }
    });
  }
  window.SurfaceRadarUI={active,render,init};
})();
