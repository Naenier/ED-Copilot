(() => {
  'use strict';

  const day = 86400000;
  const bounds = {week:7, month:30, all:Infinity};
  const rangeLabels = {week:'7 дней',month:'30 дней',all:'Всё время'};
  const categories = {
    economy:'Основная экономика',secondEconomy:'Дополнительная экономика',
    security:'Безопасность',government:'Правительство',allegiance:'Принадлежность',
    controller:'Правящая фракция',state:'Состояние фракции',power:'Контролирующая держава',
    powerState:'Состояние Пауэрплея'
  };
  const powerMetrics = {powerControl:'Контроль державы',reinforcement:'Укрепление',undermining:'Подрыв',conflicts:'Конфликты'};
  const localizedValues = {
    None:'Отсутствует', High:'Высокая', Medium:'Средняя', Low:'Низкая', Lawless:'Беззаконие', Anarchy:'Анархия',
    Colony:'Колония', Agriculture:'Сельское хозяйство', 'High Tech':'Высокие технологии', Extraction:'Добыча',
    Industrial:'Промышленность', Military:'Военная', Refinery:'Переработка', Tourism:'Туризм', Service:'Услуги',
    Stronghold:'Оплот', Fortified:'Укреплённая', Exploited:'Эксплуатируемая', Contested:'Оспариваемая',
    Boom:'Бум', Bust:'Спад', War:'Война', Election:'Выборы', Expansion:'Экспансия', Retreat:'Отступление',
    Happy:'Довольна', Elated:'В восторге', Discontented:'Недовольна', Unhappy:'Несчастна', Despondent:'В отчаянии'
  };
  const bound = new WeakSet();
  const active = C => C.tab === 'colonization' && C.mode === 'Динамика системы';
  const tr = (u,text) => u.esc(u.t(text));
  const categorical = (u,value) => u.esc(u.locale?.().startsWith('ru') ? (localizedValues[value]||value) : value);
  const known = n => typeof n === 'number' && Number.isFinite(n);
  const number = (u,n,unit='') => known(n) ? u.esc(u.fmt(n))+unit : '—';
  const signed = (u,n,unit='') => known(n) ? (n>0?'+':'')+number(u,n,unit) : '—';
  const atLabel = (u,at,withTime=false) => u.esc(u.date?.(at,withTime?{dateStyle:'medium',timeStyle:'short'}:{month:'short',day:'numeric'}) || String(at));
  const series = (rows,key) => rows.flatMap(row => known(row[key]) ? [{at:row.at,value:row[key]}] : []);
  const lastKnown = (rows,key) => rows.findLast(row => typeof row[key] === 'string' && row[key] !== '');

  function state(C) {
    const data = C.state?.colonyHistory || {};
    const id = data.selectedProjectId || '';
    if (!C.colonyHistoryView || C.colonyHistoryView.id !== id) {
      C.colonyHistoryView = {id, range:'month', faction:''};
    }
    return {data, view:C.colonyHistoryView};
  }

  function samplesInRange(samples,range,end=Date.parse(samples.at(-1)?.at)) {
    if (!samples.length) return [];
    return samples.filter(row => Date.parse(row.at) >= end - bounds[range]*day);
  }

  function chart(u,points,description,percent=false,milestones=[]) {
    if (points.length < 2) {
      return `<p class="colony-history-empty">${tr(u,'Для графика нужно минимум два наблюдения.')}</p>`;
    }
    const width = 740, height = 182, left = 70, right = 25, top = 15, bottom = 30;
    const dates = points.map(point => Date.parse(point.at));
    const start = Math.min(...dates), end = Math.max(...dates);
    const values = points.map(point => point.value);
    const minimum = Math.min(...values), maximum = Math.max(...values);
    const low = percent ? 0 : Math.max(0, minimum - (maximum===minimum?1:0));
    const high = percent ? 1 : Math.max(maximum,minimum+1);
    const position = point => [
      left+(Date.parse(point.at)-start)/Math.max(1,end-start)*(width-left-right),
      top+(high-point.value)/(high-low)*(height-top-bottom)
    ];
    const grid = [0,.5,1].map(t => {
      const y = top+t*(height-top-bottom), value=high-t*(high-low);
      return `<line x1="${left}" y1="${y}" x2="${width-right}" y2="${y}"/>
        <text x="${left-9}" y="${y+4}" text-anchor="end">${number(u,percent?100*value:value,percent?'%':'')}</text>`;
    }).join('');
    const positions = points.map(position);
    const label = percent ? n=>number(u,n*100,'%') : n=>number(u,n);
    const curve = `<g class="colony-history-curve">
        <polyline points="${positions.map(pair => pair.map(n=>n.toFixed(1)).join(',')).join(' ')}"/>
        ${positions.map(([x,y],j)=>`<circle cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="2.3"><title>${atLabel(u,points[j].at,true)} · ${label(points[j].value)}</title></circle>`).join('')}
      </g>`;
    const markers = milestones.filter(item=>item.kind==='completion'&&Date.parse(item.at)>=start&&Date.parse(item.at)<=end)
      .slice(-30).map(item=>{
        const x = position({at:item.at,value:0})[0].toFixed(1);
        return `<g class="colony-history-marker"><title>${atLabel(u,item.at,true)} · ${tr(u,'Завершение подтверждено')} · ${u.esc(item.name)}</title>
          <line x1="${x}" y1="${top}" x2="${x}" y2="${height-bottom}"/></g>`;
      }).join('');
    return `<svg class="colony-history-chart" role="img" aria-label="${tr(u,description)}" viewBox="0 0 ${width} ${height}">
      <g class="colony-history-grid">${grid}</g>${markers}${curve}
      <text x="${left}" y="${height-7}">${atLabel(u,new Date(start).toISOString())}</text>
      <text x="${width-right}" y="${height-7}" text-anchor="end">${atLabel(u,new Date(end).toISOString())}</text>
    </svg>`;
  }

  function population(u,samples,milestones) {
    const points = series(samples,'population'), latest = points.at(-1), first = points[0];
    if (!latest) return '';
    const delta = points.length > 1 ? latest.value-first.value : null;
    const daily = delta == null || Date.parse(latest.at)-Date.parse(first.at)<day ? null :
      delta*day/(Date.parse(latest.at)-Date.parse(first.at));
    const display = [
      ['Последнее наблюдение',number(u,latest?.value),latest ? atLabel(u,latest.at,true) : ''],
      ['Изменение за период',signed(u,delta),known(delta)&&first.value>0?signed(u,delta/first.value*100,'%'):''],
      ['В среднем за сутки',signed(u,daily),'']
    ];
    return `<section class="panel colony-history-card">
      <h2>${tr(u,'Население')}</h2>
      <div class="colony-history-metrics">${display.map(([label,value,hint])=>`<div><span>${tr(u,label)}</span><strong>${value}</strong>${hint?`<small>${hint}</small>`:''}</div>`).join('')}</div>
      ${chart(u,points,'График населения',false,milestones)}
    </section>`;
  }

  function factions(u,samples,view) {
    const groups = new Map();
    for (const row of samples) {
      for (const faction of row.factions || []) {
        if (!known(faction.influence)) continue;
        const key = faction.name.toLowerCase(), group = groups.get(key) || {name:faction.name,points:[]};
        group.points.push({at:row.at,value:faction.influence,state:faction.state,happiness:faction.happiness});
        groups.set(key,group);
      }
    }
    if (!groups.size) return '';
    const ordered = [...groups.values()].sort((a,b)=>b.points.at(-1).value-a.points.at(-1).value);
    const group = groups.get(view.faction.toLowerCase()) || ordered[0];
    const latest = group.points.at(-1);
    const options = ordered.map(item=>`<option value="${u.esc(item.name)}" ${item===group?'selected':''}>${u.esc(item.name)}</option>`).join('');
    const growth = group.points.length>1 ? (latest.value-group.points[0].value)*100 : null;
    return `<section class="panel colony-history-card">
      <header class="colony-history-head"><h2>${tr(u,'Влияние фракций')}</h2>
        <select data-colony-history-faction aria-label="${tr(u,'Фракция')}">${options}</select></header>
      <div class="colony-history-metrics"><div><span>${tr(u,'Влияние')}</span><strong>${number(u,latest.value*100,'%')}</strong><small>${atLabel(u,latest.at,true)}</small></div>
        <div><span>${tr(u,'Изменение за период')}</span><strong>${signed(u,growth,' '+tr(u,'п. п.'))}</strong></div>
        ${latest.state||latest.happiness?`<div><span>${tr(u,'Состояние фракции')}</span><strong>${latest.state?categorical(u,latest.state):'—'}</strong>${latest.happiness?`<small>${categorical(u,latest.happiness)}</small>`:''}</div>`:''}</div>
      ${chart(u,group.points,'График влияния фракции',true)}
    </section>`;
  }

  function power(u,samples) {
    const models = Object.entries(powerMetrics).map(([key,label])=>({key,label,points:series(samples,key)})).filter(line=>line.points.length);
    if (!models.length) return '';
    return `<section class="panel colony-history-card">
      <h2>${tr(u,'Пауэрплей и конфликты')}</h2>
      <div class="colony-history-power">${models.map(line=>`<section><h3>${tr(u,line.label)}</h3>
        <strong>${number(u,line.key==='powerControl'?line.points.at(-1).value*100:line.points.at(-1).value,line.key==='powerControl'?'%':'')}</strong>
        ${chart(u,line.points,line.label,line.key==='powerControl')}</section>`).join('')}</div>
    </section>`;
  }

  function systemChanges(u,samples,all,milestones) {
    const latest = Object.entries(categories).flatMap(([key,label]) => {
      const row = lastKnown(all,key);
      return row ? [`<div><dt>${tr(u,label)}</dt><dd>${key==='controller'||key==='power'?u.esc(row[key]):categorical(u,row[key])}<small>${atLabel(u,row.at,true)}</small></dd></div>`] : [];
    });
    if (!latest.length&&!milestones.length) return '';
    const changes = [];
    const previous = {};
    for (const row of all) {
      for (const [key,label] of Object.entries(categories)) {
        if (!row[key]) continue;
        if (previous[key] && previous[key].toLowerCase()!==row[key].toLowerCase() && samples.includes(row)) {
          const format = key==='controller'||key==='power' ? u.esc : name=>categorical(u,name);
          changes.push({at:row.at,label,value:format(previous[key])+' → '+format(row[key])});
        }
        previous[key] = row[key];
      }
    }
    for (const item of milestones) {
      changes.push({at:item.at,label:item.kind==='completion'?'Завершение подтверждено':'Первое посещение объекта',value:u.esc(item.name)});
    }
    changes.sort((a,b)=>Date.parse(b.at)-Date.parse(a.at));
    return `<section class="panel colony-history-card"><h2>${tr(u,'Состояние системы')}</h2>
      ${latest.length?`<dl class="colony-history-properties">${latest.join('')}</dl>`:''}
      ${changes.length?`<h3>${tr(u,'Что изменилось')}</h3><ol class="colony-history-changes">${changes.slice(0,40).map(change=>`<li>
        <time datetime="${u.esc(change.at)}">${atLabel(u,change.at,true)}</time><span>${tr(u,change.label)}</span><strong>${change.value}</strong>
      </li>`).join('')}</ol>`:''}
    </section>`;
  }

  function render(C,u) {
    const {data,view} = state(C);
    const all = (data.samples || []).filter(row=>Number.isFinite(Date.parse(row.at)));
    const events = (data.milestones || []).filter(row=>Number.isFinite(Date.parse(row.at)));
    const title = u.t('Динамика системы');
    const description = data.system || '';
    if (!data.projects?.length) return u.heading(title,description)+`<p class="colony-history-empty">${tr(u,'Выберите колонию в обзоре.')}</p>`;
    const choices = data.projects.length>1 ? `<select data-colony-history-project aria-label="${tr(u,'Колония')}">${data.projects.map(project=>
      `<option value="${u.esc(project.id)}" ${project.id===data.selectedProjectId?'selected':''}>${u.esc(project.name||project.targetSystem)}</option>`).join('')}</select>` : '';
    const filters = all.length || events.length ? `<div class="segmented" aria-label="${tr(u,'Период')}">${Object.entries(rangeLabels).map(([range,label])=>
      `<button type="button" data-colony-history-range="${range}" aria-pressed="${range===view.range}" class="${range===view.range?'selected':''}">${tr(u,label)}</button>`).join('')}</div>` : '';
    const end = Math.max(Date.parse(all.at(-1)?.at)||0,Date.parse(events.at(-1)?.at)||0);
    const samples = samplesInRange(all,view.range,end), milestones = samplesInRange(events,view.range,end);
    return u.heading(title,description,choices+filters)+`<div data-colony-history class="colony-history">
      ${samples.length||milestones.length?population(u,samples,milestones)+factions(u,samples,view)+power(u,samples)+systemChanges(u,samples,all,milestones):
        `<section class="panel colony-history-card"><h2>${tr(u,'Пока нет наблюдений')}</h2>
        <p>${tr(u,'Показатели появятся после посещения системы или получения данных о ней.')}</p></section>`}
    </div>`;
  }

  function init(C,u) {
    const root = active(C) && u.$('#page');
    if (!root || bound.has(root)) return;
    bound.add(root);
    root.addEventListener('click',event=>{
      if (!active(C)) return;
      const range = event.target.closest('[data-colony-history-range]')?.dataset.colonyHistoryRange;
      if (!Object.hasOwn(bounds,range)) return;
      state(C).view.range = range;
      u.renderPage();
      u.$(`[data-colony-history-range="${range}"]`)?.focus({preventScroll:true});
    });
    root.addEventListener('change',async event=>{
      if (!active(C)) return;
      const input = event.target;
      if (input.matches('[data-colony-history-faction]')) {
        state(C).view.faction = input.value;
        u.renderPage();
        u.$('[data-colony-history-faction]')?.focus({preventScroll:true});
      }
      if (input.matches('[data-colony-history-project]')) {
        try { await u.call('select-colony',{...u.context(),id:input.value}); await u.loadView(); }
        catch (error) { u.toast(error.message,true); }
      }
    });
  }

  window.ColonyHistoryUI = {active,render,init,samplesInRange};
})();
