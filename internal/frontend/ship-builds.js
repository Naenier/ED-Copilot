(() => {
'use strict';
const mode='Оснащение корабля';
const active=C=>C.tab==='engineering'&&C.mode===mode;
const rows=value=>Array.isArray(value)?value:[];
const revision=C=>C.state?.shipBuildRevision??C.state?.engineeringRoadmapRevision??C.state?.revision;
const owner=C=>C.state?.planWorkspace?.key||'';
const model=C=>{
 if(!C.shipBuildUI||C.shipBuildUI.owner!==owner(C))C.shipBuildUI={owner:owner(C),sequence:0};
 return C.shipBuildUI;
};
const t=(u,key)=>u.esc(u.t(key));
const fmt=(u,value)=>value==null?t(u,'Неизвестно'):u.esc(u.fmt(value));
const label=(u,key,field)=>'<label><span>'+t(u,key)+'</span>'+field+'</label>';
const opts=(u,values,current)=>values.map(([value,text])=>'<option value="'+u.esc(value)+'" '+(String(value)===String(current)?'selected':'')+'>'+u.esc(text)+'</option>').join('');
const button=(u,action,key,attrs='',style='quiet')=>'<button type="button" class="pill '+style+'" data-build-action="'+action+'" '+attrs+'>'+t(u,key)+'</button>';
const formats={slef:'SLEF',coriolis:'Coriolis Detailed',journal:'Игровой журнал'};
const statLabels={unladenMass:'Масса без груза, т',ladenMass:'Масса с грузом, т',cargo:'Вместимость трюма, т',
 maximumRange:'Максимальный прыжок, св. л.',unladenRange:'Прыжок без груза, св. л.',ladenRange:'Прыжок с грузом, св. л.',
 powerAvailable:'Мощность реактора, МВт',powerRetracted:'Потребление с убранным оружием, МВт',powerDeployed:'Потребление с развёрнутым оружием, МВт',cost:'Стоимость сборки, кр.'};
function table(u,head,body){
 return '<div class="build-table"><table><thead><tr>'+head.map(key=>'<th scope="col">'+t(u,key)+'</th>').join('')+'</tr></thead><tbody>'+body+'</tbody></table></div>';
}
const cell=(value,head=false)=>head?'<th scope="row">'+value+'</th>':'<td>'+value+'</td>';
function statsTable(u,comparison,before,target){
 const stats=Object.entries(statLabels).map(([key,label])=>{
  const values=comparison?.stats?.[key]||{before:before?.stats?.[key],after:target?.stats?.[key]};
  const delta=values.delta;
  return '<tr>'+cell(t(u,label),true)+cell(fmt(u,values.before))+cell(fmt(u,values.after))+cell(delta==null?'—':(delta>0?'+':'')+fmt(u,delta))+'</tr>';
 }).join('');
 return '<section class="panel build-panel"><h2>'+t(u,'Параметры сборки')+'</h2>'+table(u,['Параметр','Исходная сборка','Целевая сборка','Разница'],stats)+'</section>';
}
function slotCell(u,slot,power=null){
 if(!slot||slot.empty)return '—';
 const engineering=slot.engineering||{};
 const bp=engineering.BlueprintName_Localised||engineering.BlueprintName||engineering.blueprint?.name||'';
 const effect=engineering.ExperimentalEffect_Localised||engineering.ExperimentalEffect||'';
 const grade=engineering.Level??engineering.blueprint?.grade;
 const powerText=power===null?'':[slot.on==null?'':u.t(slot.on?'Включено':'Выключено'),
  slot.priority==null?'':u.t('Приоритет питания')+' '+u.fmt(Number(slot.priority)+power)].filter(Boolean).join(' · ');
 return '<strong>'+u.esc(slot.name||slot.item||slot.slot)+'</strong>'+
  (powerText?'<small>'+u.esc(powerText)+'</small>':'')+
  (bp||effect?'<small>'+u.esc([bp,grade!=null?'G'+grade:'',effect].filter(Boolean).join(' · '))+'</small>':'');
}
function slotTable(u,comparison,before,target){
 const changes={same:'Совпадает',replace:'Замена',changed:'Изменён',unobserved:'Слот не указан'};
 const compare=comparison?.compatibleLayout===true;
 const header=compare?['Слот','Исходная сборка','Целевая сборка','Изменение']:['Слот','Модуль','Питание','Приоритет питания'];
 const shown=compare?rows(comparison.slots):rows((target||before)?.slots);
 if(!shown.length)return '';
 const body=shown.map(slot=>'<tr>'+cell(u.esc(slot.slot),true)+(compare
  ?cell(slotCell(u,slot.before,target?.format==='coriolis'?0:1))+cell(slotCell(u,slot.after,target?.format==='coriolis'?0:1))+cell(t(u,changes[slot.change]||slot.change))
  :cell(slotCell(u,slot))+cell(slot.on==null?'—':t(u,slot.on?'Включено':'Выключено'))+cell(slot.priority==null?'—':fmt(u,Number(slot.priority)+(target?.format==='coriolis'?0:1))))+'</tr>').join('');
 return '<section class="panel build-panel"><h2>'+t(u,'Слоты и модули')+'</h2>'+
  (comparison&&!compare?'<p class="build-muted">'+t(u,'Слоты можно сопоставить только у одной модели корабля и совместимых форматов.')+'</p>':'')+table(u,header,body)+'</section>';
}
function planning(u,data){
 const purchases=rows(data.comparison?.shopping),needs=rows(data.needs?.materials);
 const unknown=data.needs?.complete===false;
 if(!purchases.length&&!needs.length&&!unknown)return '';
 const modules=purchases.length?'<h3>'+t(u,'Модули к покупке')+'</h3><ul>'+purchases.map(item=>'<li>'+u.esc(item.name||item.item)+' · '+u.esc(item.slot)+'</li>').join('')+'</ul>':'';
 const materials=needs.length?'<h3>'+t(u,'Материалы для улучшений')+'</h3>'+table(u,['Материал','Требуется','В запасе','Не хватает'],needs.map(item=>'<tr>'+
  cell(item.entryId?'<button class="table-entity-link" data-reference-id="'+u.esc(item.entryId)+'" type="button">'+u.esc(item.name)+'</button>':u.esc(item.name),true)+
  cell(fmt(u,item.count))+cell(fmt(u,item.owned))+cell(fmt(u,item.missing))+'</tr>').join('')):'';
 return '<section class="panel build-panel"><h2>'+t(u,'Подготовка целевой сборки')+'</h2>'+modules+
  (unknown?'<p class="build-muted">'+t(u,'Часть инженерных расходов неизвестна или рассчитана ориентировочно.')+'</p>':'')+materials+
  (data.needs?button(u,'itinerary','Поездка к инженерам','id="ship-build-itinerary"'):'')+'</section>';
}
function importEditor(r,u){
 const preview=rows(r.preview);
 return '<form id="ship-build-import" class="panel build-panel"><h2>'+t(u,'Импорт сборки')+'</h2><div class="build-import">'+
  label(u,'Файл JSON','<input id="ship-build-file" type="file" accept=".json,application/json">')+
  label(u,'Данные сборки','<textarea id="ship-build-json" rows="5" placeholder="SLEF / Coriolis Detailed / Loadout">'+u.esc(r.text||'')+'</textarea>')+
  '<div class="build-actions">'+button(u,'preview','Проверить импорт','id="ship-build-preview"')+button(u,'cancel-import','Отмена')+'</div>'+
  (preview.length?label(u,'Сборка из импорта','<select id="ship-build-import-choice">'+opts(u,preview.map((build,index)=>[index,build.name+' · '+build.ship+' · '+(formats[build.format]||build.format)]),r.previewIndex||0)+'</select>')+
   '<button class="pill primary" type="submit" '+(!r.owner||r.pending?'disabled':'')+'>'+t(u,'Сохранить сборку')+'</button>':'')+'</div></form>';
}
function slotEditor(r,u,target){
 const slots=rows(target.slots),selection=slots.find(slot=>slot.slot===r.editSlot)||slots[0];
 if(!selection)return '';
 const draft=r.slotDraft||{};
 return '<form id="ship-build-slot" class="panel build-panel"><h2>'+t(u,'Заменить модуль')+'</h2><div class="build-edit">'+
  label(u,'Слот','<select id="ship-build-edit-slot" name="slot">'+opts(u,slots.map(slot=>[slot.slot,slot.slot]),selection.slot)+'</select>')+
  label(u,'Игровой идентификатор модуля','<input id="ship-build-edit-item" name="item" required maxlength="120" value="'+u.esc(draft.item??selection.item)+'">')+
  label(u,'Питание','<select id="ship-build-edit-power" name="on">'+opts(u,[['',u.t('Не менять')],['true',u.t('Включено')],['false',u.t('Выключено')]],draft.on||'')+'</select>')+
  label(u,'Приоритет питания','<select id="ship-build-edit-priority" name="priority">'+opts(u,[['',u.t('Не менять')],...[1,2,3,4,5].map(n=>[n,String(n)])],draft.priority||'')+'</select>')+
  '<div class="build-actions">'+button(u,'cancel-slot','Отмена')+'<button type="submit" class="pill primary">'+t(u,'Сохранить')+'</button></div></div></form>';
}
function content(C,u){
 const r=model(C),data=r.data;
 const error=r.error?'<p class="build-error" role="alert">'+u.esc(r.error)+'</p>':'';
 if(!data)return error||'<p class="build-muted" role="status">'+t(u,'Загружаем оснащение…')+'</p>';
 const before=data.baseline||data.observed,target=data.target,saved=rows(data.targets);
 const choices=saved.length?label(u,'Целевая сборка','<select id="ship-build-target">'+opts(u,saved.map(build=>[build.id,build.name]),data.selectedId)+'</select>'):'';
 const controls='<div class="build-actions">'+choices+button(u,'import','Импорт сборки','id="ship-build-import-open"')+
  (data.observed&&r.owner?button(u,'baseline','Сохранить исходную сборку'):'')+
  (data.baseline&&r.owner?button(u,'copy','Создать вариант'):'')+'</div>';
 const base=before?'<article><small>'+t(u,data.baseline?'Исходная сборка':'Текущий корабль')+'</small><strong>'+u.esc(before.name)+'</strong><span>'+u.esc(before.ship)+'</span></article>':'';
 const goal=target?'<article><small>'+t(u,'Целевая сборка')+'</small><strong>'+u.esc(target.name)+'</strong><span>'+u.esc(target.ship)+'</span></article>':'';
 const edit=target?'<div class="build-actions">'+(target.format!=='coriolis'?button(u,'edit-slot','Заменить модуль','id="ship-build-edit-open"'):'')+
  button(u,'export','Экспорт JSON')+(target.format!=='coriolis'?button(u,'edsy','Открыть в EDSY'):'')+
  (target.format!=='coriolis'||target.providerURL?button(u,'coriolis','Открыть в Coriolis'):'')+
  (r.confirmRemove?'<span>'+t(u,'Удалить сборку?')+'</span>'+button(u,'remove','Удалить')+button(u,'cancel-remove','Отмена'):button(u,'ask-remove','Удалить'))+'</div>':
  data.baseline?'<div class="build-actions">'+button(u,'export-baseline','Экспорт JSON')+button(u,'edsy-baseline','Открыть в EDSY')+button(u,'coriolis-baseline','Открыть в Coriolis')+'</div>':'';
 const empty=!base&&!goal?'<p class="build-muted">'+t(u,'Оснащение появится после загрузки корабля в игре. Можно импортировать сборку из JSON.')+'</p>':'';
 return controls+error+(r.importOpen?importEditor(r,u):'')+empty+(base||goal?'<div class="build-summary">'+base+goal+'</div>':'')+edit+
  (r.editOpen&&target?slotEditor(r,u,target):'')+(before||target?statsTable(u,data.comparison,before,target):'')+
  slotTable(u,data.comparison,before,target)+planning(u,data);
}
function render(C,u){return u.heading(u.t(mode),'')+'<div class="build-workspace" id="ship-build-workspace">'+content(C,u)+'</div>';}
function draw(C,u,focusId=''){
 const host=active(C)&&u.$('#ship-build-workspace');
 if(!host)return;
 const activeElement=host.ownerDocument?.activeElement;
 const focus=focusId||(host.contains?.(activeElement)?activeElement.id:'');
 const html=content(C,u);
 if(host.buildMarkup!==html){host.innerHTML=html;host.buildMarkup=html;}
 if(focus)host.ownerDocument?.getElementById(focus)?.focus({preventScroll:true});
}
function load(C,u){
 const r=model(C),stamp=revision(C);
 if(r.pending||r.reading||r.data&&r.stamp===stamp)return;
 r.reading=true;
 const sequence=++r.sequence;
 r.read=(async()=>{
  try{
   const data=await u.call('ship-builds',{expectedPlanWorkspace:r.owner});
   if(model(C)===r&&sequence===r.sequence&&stamp===revision(C)){r.data=data;r.stamp=stamp;}
  }catch(error){if(model(C)===r&&sequence===r.sequence){r.error=u.t(error.message);r.stamp=stamp;}}
  finally{
   if(model(C)===r&&sequence===r.sequence){
    r.reading=false;
    if(active(C)&&stamp!==revision(C))load(C,u);
    else draw(C,u);
   }
  }
 })();
}
async function run(C,u,job){
 const r=model(C);
 if(r.pending)return;
 r.sequence++;r.reading=false;r.pending=true;r.error='';
 try{await job(r);}catch(error){if(model(C)===r)r.error=u.t(error.message||'Не удалось обработать сборку.');}
 finally{r.pending=false;if(model(C)===r&&active(C)){draw(C,u);load(C,u);}}
}
async function update(C,u,r,input){
 const stamp=revision(C);
 const data=await u.call('ship-builds-update',{...input,expectedPlanWorkspace:r.owner});
 if(model(C)!==r)return;
 r.data=data;r.stamp=stamp;r.confirmRemove=false;
}
function actions(event,C,u){
 const element=event.target.closest?.('[data-build-action]');
 if(!element)return;
 const action=element.dataset.buildAction,r=model(C),id=r.data?.target?.id;
 switch(action){
 case 'import':r.importOpen=true;draw(C,u,'ship-build-json');return;
 case 'cancel-import':r.importOpen=false;r.preview=null;r.text='';draw(C,u,'ship-build-import-open');return;
 case 'preview':void run(C,u,async current=>{
  const text=current.text||'';
  if(!text.trim())throw new Error('Вставьте JSON сборки размером до 2 МБ.');
  const data=await u.call('ship-builds',{text,expectedPlanWorkspace:current.owner});
  if(model(C)===current&&current.text===text){current.preview=data.preview;current.previewIndex=0;}
 });return;
 case 'ask-remove':r.confirmRemove=true;draw(C,u);return;
 case 'cancel-remove':r.confirmRemove=false;draw(C,u);return;
 case 'edit-slot':r.editOpen=true;r.editSlot='';r.slotDraft={};draw(C,u,'ship-build-edit-slot');return;
 case 'cancel-slot':r.editOpen=false;r.slotDraft={};draw(C,u,'ship-build-edit-open');return;
 case 'baseline':case 'copy':case 'remove':
  void run(C,u,current=>update(C,u,current,{action,id:action==='copy'?'baseline':id}));return;
 case 'itinerary':
  void run(C,u,async current=>{
   await u.call('engineering-itinerary-update',{action:'source',source:'build',expectedPlanWorkspace:current.owner});
   if(model(C)===current&&active(C))await u.navigate('engineering',{mode:'Поездка к инженерам'});
  });return;
 default:
  if(!/^(export|edsy|coriolis)(-baseline)?$/.test(action))return;
  void run(C,u,async current=>{
   const selection=action.endsWith('-baseline')?'baseline':id,provider=action.split('-')[0];
   const result=await u.call('ship-builds-export',{id:selection,provider:provider==='export'?'':provider,format:'slef',expectedPlanWorkspace:current.owner});
   if(model(C)!==current||!active(C))return;
   if(result.url)await u.call('native',{action:'open',url:result.url});
   else if(result.text)await u.call('native',{action:'save-json',name:result.name,text:result.text});
  });
 }
}
function init(C,u){
 if(!active(C))return;
 const host=u.$('#ship-build-workspace');
 if(host&&!host.buildBound){
  host.buildBound=true;host.buildMarkup=content(C,u);
  host.addEventListener('click',event=>actions(event,C,u));
  host.addEventListener('input',event=>{
   const target=event.target,r=model(C);
   if(target.id==='ship-build-json'){
    r.text=target.value;r.preview=null;r.fileRequest=(r.fileRequest||0)+1;
    const submit=target.form?.querySelector('[type=submit]');
    if(submit)submit.disabled=true;
   }
   if(target.id==='ship-build-edit-item'&&r.slotDraft)r.slotDraft.item=target.value;
  });
  host.addEventListener('change',async event=>{
   const target=event.target,r=model(C);
   if(target.id==='ship-build-target')void run(C,u,async current=>{await update(C,u,current,{action:'select',id:target.value});current.editOpen=false;current.slotDraft={};});
   if(target.id==='ship-build-import-choice')r.previewIndex=Number(target.value);
   if(target.id==='ship-build-edit-slot'){r.editSlot=target.value;r.slotDraft={};draw(C,u,target.id);}
   if(target.id==='ship-build-edit-power'&&r.slotDraft)r.slotDraft.on=target.value;
   if(target.id==='ship-build-edit-priority'&&r.slotDraft)r.slotDraft.priority=target.value;
   if(target.id==='ship-build-file'&&target.files?.[0]){
    const file=target.files[0],request=r.fileRequest=(r.fileRequest||0)+1;r.preview=null;
    if(file.size>2*1024*1024)r.error=u.t('Вставьте JSON сборки размером до 2 МБ.');
    else try{const content=await file.text();if(model(C)!==r||r.fileRequest!==request)return;r.text=content;r.error='';}
    catch{if(model(C)!==r||r.fileRequest!==request)return;r.error=u.t('Не удалось прочитать файл сборки.');}
    draw(C,u,'ship-build-json');
   }
  });
  host.addEventListener('submit',event=>{
   const form=event.target;
   if(!['ship-build-import','ship-build-slot'].includes(form.id))return;
   event.preventDefault();
   void run(C,u,async r=>{
    if(form.id==='ship-build-import'&&r.preview?.length){
     await update(C,u,r,{action:'import',text:r.text,index:r.previewIndex||0});
     r.importOpen=false;r.preview=null;r.text='';
    }else if(form.id==='ship-build-slot'){
     const field=form.elements,params={action:'replace',id:r.data.target.id,slot:field.slot.value,item:field.item.value.trim()};
     if(field.on.value!=='')params.on=field.on.value==='true';
     if(field.priority.value!=='')params.priority=Number(field.priority.value)-1;
     await update(C,u,r,params);r.editOpen=false;r.slotDraft={};
    }
   });
  });
 }
 load(C,u);
}
function leave(C){const r=C.shipBuildUI;if(r?.reading){r.sequence++;r.reading=false;r.stamp=null;}}
window.ShipBuildUI={active,render,init,leave};
})();
