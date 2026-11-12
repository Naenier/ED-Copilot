(() => {
'use strict';
const mode='Поездка к инженерам';
const active=C=>C.tab==='engineering'&&C.mode===mode;
const array=value=>Array.isArray(value)?value:[];
const owner=C=>C.state?.planWorkspace?.key||'';
const revision=C=>JSON.stringify([C.state?.engineeringItineraryRevision??C.state?.engineeringRoadmapRevision??C.state?.revision,C.state?.commander?.system,C.state?.settings?.language]);
function model(C){
 if(!C.engineeringItineraryUI||C.engineeringItineraryUI.owner!==owner(C))C.engineeringItineraryUI={owner:owner(C),sequence:0};
 return C.engineeringItineraryUI;
}
const tr=(u,key,vars)=>u.esc(u.t(key,vars));
const english=u=>/^en\b/i.test(u.locale?.()||'');
const name=(u,row,key='name',en='english')=>u.t((english(u)?row?.[en]||row?.[key]:row?.[key]||row?.[en])||'');
const category={raw:'Сырьё',manufactured:'Производственные материалы',encoded:'Данные'};
const reason={'unknown-recipe':'Рецепт для выбранного улучшения неизвестен.', 'recipe-unconfirmed':'Расход материалов для этой цели не подтверждён.',
 'unknown-installed-upgrades':'Неизвестны улучшения на складском модуле.', 'access-required':'Сначала выполните требование инженера.',
 'materials-needed':'Нужны материалы'};
const actionTitles={trade:'Обменять материалы',upgrade:'Изготовить улучшения',unlock:'Открыть инженера'};
const button=(u,action,title,attrs='',icon='')=>'<button class="pill quiet" type="button" data-itinerary-action="'+action+'" '+attrs+'>'+(icon?u.icon(icon):'')+tr(u,title)+'</button>';
const reference=(u,id,title)=>id?'<button type="button" class="table-entity-link" data-reference-id="'+u.esc(id)+'">'+u.esc(title)+'</button>':u.esc(title);
function taskName(u,task){
 return [name(u,task,'module','moduleEnglish')||task.item,name(u,task,'blueprint','blueprintEnglish'),task.grade?'G'+task.grade:'',
  name(u,task,'experimental','experimentalEnglish'),task.quantity>1?'× '+u.fmt(task.quantity):''].filter(Boolean).join(' · ');
}
function requirement(u,value){
 if(!value)return '';
 const description=value.kind==='rank'&&value.target?u.t('Поднимите ранг инженера до G{grade}.',{grade:u.fmt(value.target)}):name(u,value,'text','english');
 const items=array(value.items).map(row=>u.esc(name(u,row)||row.key)+(row.count==null?'':' × '+u.esc(u.fmt(row.count))));
 return [u.esc(description),items.join(', ')].filter(Boolean).join(' · ');
}
function tradeTask(u,task){
 return '<span>'+tr(u,'Отдать')+' '+u.esc(u.fmt(task.requiredFrom))+' × '+reference(u,'material/'+task.fromKey,name(u,task,'fromName','fromEnglish'))+
  ' · '+tr(u,'Получить')+' '+u.esc(u.fmt(task.received))+' × '+reference(u,'material/'+task.toKey,name(u,task,'toName','toEnglish'))+'</span>';
}
function stopCard(u,stop,index,total,deferred=false){
 const identity='data-stop-id="'+u.esc(stop.id)+'"';
 const heading=stop.kind==='trader'?u.t(category[stop.category]||'Торговец материалами'):name(u,stop);
 const place=u.esc([stop.system,stop.station].filter(Boolean).join(' · '));
 const distance=stop.distance==null?'':'<small>'+u.esc(u.fmt(stop.distance))+tr(u,' св. л.')+'</small>';
 const tasks=array(stop.tasks).map(task=>'<li>'+(stop.kind==='trader'?tradeTask(u,task):task.requirement?requirement(u,task.requirement):u.esc(taskName(u,task)))+'</li>').join('');
 const moves=deferred?'':button(u,'up','Выше',identity+(index===0?' disabled':''),'arrow-up')+button(u,'down','Ниже',identity+(index>=total-1?' disabled':''),'chevron-down');
 return '<article class="itinerary-stop" data-itinerary-stop="'+u.esc(stop.id)+'"><header><div><h3>'+reference(u,stop.entryId,heading)+'</h3><p>'+place+distance+'</p></div><div class="itinerary-actions">'+moves+
  button(u,deferred?'restore':'defer',deferred?'Вернуть в поездку':'Отложить',identity)+
  button(u,'copy','Копировать систему','data-system="'+u.esc(stop.system)+'"','copy')+'</div></header>'+
  '<strong>'+tr(u,actionTitles[stop.action]||stop.action)+'</strong><ul>'+tasks+'</ul></article>';
}
function blockedCard(u,task){
 const who=task.engineer,nameText=who?reference(u,'engineer/'+who.key,name(u,who)):'';
 const missing=array(task.deficits).map(row=>reference(u,row.entryId,name(u,row)||row.key)+' · '+(row.missing==null?tr(u,'Неизвестно'):u.esc(u.fmt(row.missing)))).join(', ');
 return '<article class="itinerary-stop"><h3>'+u.esc(taskName(u,task))+'</h3>'+
  (nameText?'<p>'+nameText+' · '+u.esc([who.system,who.base].filter(Boolean).join(' · '))+'</p>':'')+
  (task.reason==='access-required'&&task.requirement?'':'<p>'+tr(u,reason[task.reason]||task.reason)+'</p>')+
  (task.requirement?'<p>'+requirement(u,task.requirement)+'</p>':'')+(missing?'<p>'+missing+'</p>':'')+'</article>';
}
function content(C,u){
 const r=model(C),data=r.data;
 const error=r.error?'<p class="itinerary-error" role="alert">'+u.esc(r.error)+'</p>':'';
 if(!data)return error||'<p role="status" class="itinerary-muted">'+tr(u,'Загружаем поездку…')+'</p>';
 if(!r.owner||data.ownerKnown===false)return '<p class="itinerary-muted">'+tr(u,'Сначала подключите журнал командира.')+'</p>';
 const activeStops=array(data.stops),later=array(data.deferred),blocked=array(data.blocked);
 const source='<label><span>'+tr(u,'Цель поездки')+'</span><select id="engineer-trip-source">'+
  '<option value="plan" '+(data.source!=='build'?'selected':'')+'>'+tr(u,'План улучшений')+'</option>'+
  '<option value="build" '+(data.source==='build'?'selected':'')+' '+(!data.buildAvailable&&data.source!=='build'?'disabled':'')+'>'+
  tr(u,'Целевая сборка')+(data.buildName?' · '+u.esc(data.buildName):'')+'</option></select></label>';
 const tools='<div class="itinerary-actions">'+source+
  (activeStops.length?button(u,'route','Передать в маршрут корабля','id="engineer-trip-route" '+(r.pending?'disabled':''),'route'):'')+
  (data.customized?button(u,'reset','Сбросить порядок и отложенные остановки'):'')+'</div>';
 const empty=!activeStops.length&&!blocked.length&&!later.length?'<p class="itinerary-muted">'+tr(u,data.goalCount?'Работ для поездки не осталось.':'Добавьте улучшения в план или выберите целевую сборку.')+'</p>':'';
 return tools+error+empty+
  (activeStops.length?'<section><h2>'+tr(u,'Готовые остановки')+'</h2>'+activeStops.map((stop,index)=>stopCard(u,stop,index,activeStops.length)).join('')+'</section>':'')+
  (later.length?'<section><h2>'+tr(u,'Отложенные остановки')+'</h2>'+later.map(stop=>stopCard(u,stop,0,1,true)).join('')+'</section>':'')+
  (blocked.length?'<section><h2>'+tr(u,'Требуется подготовка')+'</h2>'+blocked.map(task=>blockedCard(u,task)).join('')+'</section>':'');
}
function render(C,u){return u.heading(u.t(mode),'')+'<div class="itinerary-workspace" id="engineering-itinerary-workspace">'+content(C,u)+'</div>';}
function draw(C,u){
 if(!active(C))return;
 const host=u.$('#engineering-itinerary-workspace');if(!host)return;
 const focus=host.ownerDocument?.activeElement;
 const data=focus?.dataset;
 const hadFocus=host.contains?.(focus),html=content(C,u);
 if(host.itineraryMarkup===html)return;
 host.innerHTML=html;host.itineraryMarkup=html;
 if(hadFocus){
  const node=(focus.id&&host.ownerDocument.getElementById(focus.id))||[...host.querySelectorAll('[data-itinerary-action]')].find(node=>
   node.dataset.itineraryAction===data?.itineraryAction&&node.dataset.stopId===data?.stopId);
  node?.focus({preventScroll:true});
 }
}
function load(C,u){
 const r=model(C),stamp=revision(C);
 if(!active(C)||r.pending||r.loading||r.stamp===stamp)return;
 r.loading=true;r.stamp=stamp;
 const request=++r.sequence;
 r.read=(async()=>{
  try{
   const data=await u.call('engineering-itinerary',{expectedPlanWorkspace:r.owner});
   if(model(C)===r&&r.sequence===request&&stamp===revision(C)){r.data=data;r.error='';}
  }catch(error){if(model(C)===r&&r.sequence===request&&stamp===revision(C))r.error=u.t(error.message);}
  finally{
   if(model(C)===r&&r.sequence===request){
    r.loading=false;
    if(stamp!==revision(C))load(C,u);else draw(C,u);
   }
  }
 })();
}
async function run(C,u,work){
 const r=model(C);
 if(r.pending||!r.owner)return;
 r.pending=true;r.sequence++;r.loading=false;r.error='';
 try{await work(r);}catch(error){if(model(C)===r)r.error=u.t(error.message||'Не удалось обновить поездку.');}
 finally{
  r.pending=false;
  if(model(C)===r){draw(C,u);load(C,u);}
 }
}
async function update(C,u,input){
 await run(C,u,async r=>{
  const stamp=revision(C);
  const data=await u.call('engineering-itinerary-update',{...input,expectedPlanWorkspace:r.owner});
  if(model(C)===r){r.data=data;r.stamp=stamp;}
 });
}
function routeActions(u,stop){
 const tasks=array(stop.tasks);
 if(stop.kind==='trader'){
  return tasks.map(task=>u.t('Получить')+' '+u.fmt(task.received)+' × '+name(u,task,'toName','toEnglish')).join(', ');
 }
 if(stop.action==='unlock')return tasks.map(task=>name(u,task.requirement,'text','english')).filter(Boolean).join(', ');
 return tasks.map(task=>taskName(u,task)).filter(Boolean).join(', ');
}
function click(event,C,u){
 const button=event.target.closest?.('[data-itinerary-action]');if(!button)return;
 const r=model(C),data=button.dataset,id=data.stopId;
 switch(data.itineraryAction){
 case 'copy':
  void run(C,u,async()=>{await u.call('native',{action:'copy',text:data.system});u.toast(u.t('Система скопирована'));});return;
 case 'defer':case 'restore':
  void update(C,u,{action:'defer',id,deferred:data.itineraryAction==='defer'});return;
 case 'up':case 'down':
  void update(C,u,{action:'move',id,direction:data.itineraryAction==='up'?-1:1,visibleIds:array(r.data?.stops).map(stop=>stop.id)});return;
 case 'reset':void update(C,u,{action:'reset'});return;
 case 'route':
  void run(C,u,async current=>{
   const waypoints=array(current.data?.stops).map(stop=>({system:stop.system,station:stop.station,action:(routeActions(u,stop)||u.t(actionTitles[stop.action])).slice(0,180)}));
   await u.call('ship-route',{action:'import',name:u.t(mode),waypoints,expectedPlanWorkspace:current.owner});
   if(model(C)===current&&active(C))await u.navigate('ship-route');
  });
 }
}
function init(C,u){
 if(!active(C))return;
 const host=u.$('#engineering-itinerary-workspace');
 if(host&&!host.itineraryBound){
  host.itineraryBound=true;host.itineraryMarkup=content(C,u);
  host.addEventListener('click',event=>click(event,C,u));
  host.addEventListener('change',event=>{
   if(event.target.id==='engineer-trip-source')void update(C,u,{action:'source',source:event.target.value});
  });
 }
 load(C,u);
}
function leave(C){const r=C.engineeringItineraryUI;if(r?.loading){r.sequence++;r.loading=false;r.stamp=null;}}
window.EngineeringItineraryUI={active,render,init,leave};
})();
