(() => {
'use strict';

const mode='Снаряжение Odyssey';
const active=C=>C.tab==='engineering'&&C.mode===mode;
const list=value=>Array.isArray(value)?value:[];
const workspace=C=>C.state?.planWorkspace?.key||'';
function model(C){
 const owner=workspace(C);
 if(!C.odysseyUI||C.odysseyUI.owner!==owner)C.odysseyUI={owner,sequence:0};
 return C.odysseyUI;
}
const tr=(u,value,vars)=>u.esc(u.t(value,vars));
const number=(u,value)=>value==null?tr(u,'Неизвестно'):u.esc(u.fmt(value));
const name=(u,item)=>item?u.t(/^en\b/i.test(u.locale?.()||'')?item.english||item.name||item.key:item.name||item.english||item.key):'';
function equipment(r,key){return list(r.data?.equipment).find(item=>item.key===key);}
function selected(r){return r.data?.activeLoadout;}
function copyItems(items){
 return list(items).map(({itemKey,equipmentKey,startingGrade,targetGrade,modKeys})=>({
  itemKey,equipmentKey:equipmentKey||'',startingGrade:startingGrade||1,targetGrade:targetGrade||1,modKeys:[...list(modKeys)],
 }));
}
function draftFor(kit){
 return {id:kit?.id||'',name:kit?.name||'',reserve:kit?kit.reserve===true:true,items:copyItems(kit?.items)};
}
function options(u,rows,value){
 return rows.map(row=>'<option value="'+u.esc(row.key)+'" '+(String(row.key)===String(value)?'selected':'')+'>'+u.esc(row.text)+'</option>').join('');
}
function label(u,text,control){
 return '<label class="odyssey-field"><span>'+tr(u,text)+'</span>'+control+'</label>';
}
function button(u,action,title,style='quiet',attrs=''){
 return '<button type="button" class="pill '+style+'" data-odyssey-action="'+action+'" '+attrs+'>'+tr(u,title)+'</button>';
}
function compatible(item,mod){
 return item&&item.moduleKey===mod.moduleKey&&(!mod.damageType||mod.damageType===item.damageType);
}
function availableEquipment(r,index=-1){
 const rows=list(r.data?.equipment),others=r.draft.items.filter((_,position)=>position!==index).map(line=>equipment(r,line.itemKey));
 const suit=others.find(item=>item?.slot==='suit'),primary=others.filter(item=>item?.slot==='primary').length;
 const secondary=others.some(item=>item?.slot==='secondary');
 return rows.filter(item=>item.key===r.draft.items[index]?.itemKey||
  item.slot==='suit'&&!suit&&primary<=(item.primarySlots??(item.key==='tacticalsuit'?2:1))||
  item.slot==='primary'&&primary<(suit?.primarySlots??(suit&&suit.key!=='tacticalsuit'?1:2))||
  item.slot==='secondary'&&!secondary);
}
function editorItem(r,u,line,index){
 const gear=equipment(r,line.itemKey);
 const alreadyBound=new Set(r.draft.items.filter((_,position)=>position!==index).map(item=>item.equipmentKey));
 const owned=list(r.data.ownedEquipment).filter(item=>item.itemKey===line.itemKey&&!alreadyBound.has(item.key));
 const instance=owned.find(item=>item.key===line.equipmentKey);
 const mods=list(r.data.modifications).filter(mod=>compatible(gear,mod));
 const grades=[1,2,3,4,5].map(value=>({key:value,text:u.fmt(value)}));
 const select=(field,values,current,extra='')=>'<select id="odyssey-'+field+'-'+index+'" data-odyssey-field="'+field+'" data-index="'+index+'" '+extra+'>'+options(u,values,current)+'</select>';
 const slot={suit:'Костюм',primary:'Основное оружие',secondary:'Дополнительное оружие'}[gear?.slot]||'Снаряжение';
 const gearChoices=availableEquipment(r,index).map(item=>({key:item.key,text:name(u,item)}));
 const stockChoices=[{key:'',text:u.t('Новый экземпляр')},...owned.map((item,position)=>({
  key:item.key,text:name(u,item)+' · '+u.t('Экземпляр {number}',{number:u.fmt(position+1)})+' · '+u.t('Ступень {grade}',{grade:item.grade??'—'}),
 }))];
 if(line.equipmentKey&&!instance)stockChoices.push({key:line.equipmentKey,text:u.t('Экземпляр отсутствует в журнале')});
 const start=line.equipmentKey?'<div class="odyssey-field"><span>'+tr(u,'Текущий класс')+'</span><strong>'+number(u,instance?.grade)+'</strong></div>':
  label(u,'Начальная ступень',select('startingGrade',grades,line.startingGrade));
 const minimum=instance?.grade||line.startingGrade;
 const targetGrades=grades.filter(row=>row.key>=minimum||row.key===line.targetGrade);
 const slots=gear?.grades?.[line.targetGrade]?.slots;
 const modOptions=mods.map((mod,position)=>{
  const checked=line.modKeys.includes(mod.key);
  const full=!line.equipmentKey&&slots!=null&&line.modKeys.length>=slots;
  return '<label><input type="checkbox" id="odyssey-mod-'+index+'-'+position+'" data-odyssey-mod="'+u.esc(mod.key)+'" data-index="'+index+'" '+
   (checked?'checked':full?'disabled':'')+'><span>'+u.esc(name(u,mod))+'</span></label>';
 }).join('');
 return '<section class="odyssey-editor-item"><header><h3>'+tr(u,slot)+'</h3>'+
  button(u,'remove-item','Убрать','quiet','data-index="'+index+'" aria-label="'+tr(u,'Убрать из комплекта')+' · '+u.esc(name(u,gear))+'"')+'</header>'+
  '<div class="odyssey-fields">'+label(u,'Снаряжение',select('itemKey',gearChoices,line.itemKey))+
  label(u,'Экземпляр',select('equipmentKey',stockChoices,line.equipmentKey))+start+
  label(u,'Целевая ступень',select('targetGrade',targetGrades,line.targetGrade))+'</div>'+
  (modOptions?'<details class="odyssey-mod-picker" data-odyssey-mod-picker="'+index+'" '+(r.openMods?.has(index)?'open':'')+'><summary>'+tr(u,'Модификации')+' · '+u.esc(u.fmt(line.modKeys.length))+
   (!line.equipmentKey&&slots!=null?' / '+u.esc(u.fmt(slots)):'')+'</summary><div>'+modOptions+'</div></details>':'')+
  '</section>';
}
function editor(r,u){
 const d=r.draft;
 return '<form class="odyssey-editor panel" id="odyssey-kit-form"><div class="odyssey-editor-heading"><h2>'+tr(u,d.id?'Редактировать комплект':'Новый комплект')+'</h2></div>'+
  '<div class="odyssey-kit-fields">'+label(u,'Название комплекта','<input id="odyssey-kit-name" maxlength="100" required value="'+u.esc(d.name)+'">')+
  '<label class="odyssey-check"><input id="odyssey-kit-reserve" type="checkbox" '+(d.reserve?'checked':'')+'><span>'+tr(u,'Резервировать материалы для комплекта')+'</span></label></div>'+
  d.items.map((line,index)=>editorItem(r,u,line,index)).join('')+
  (r.error?'<p class="odyssey-error" role="alert">'+u.esc(r.error)+'</p>':'')+
  '<div class="odyssey-actions">'+(d.items.length<4&&availableEquipment(r).length?button(u,'add-item','Добавить снаряжение','quiet','id="odyssey-add-item"'):'')+
  '<div class="spacer"></div>'+button(u,'cancel-edit','Отмена')+
  '<button id="odyssey-save-kit" type="submit" class="pill primary" '+(r.writing||!d.items.length?'disabled':'')+'>'+tr(u,r.writing?'Сохраняем…':'Сохранить')+'</button></div></form>';
}
const stepStates={done:'Выполнено',available:'Доступно',pending:'Требуется',blocked:'Доступ ограничен',unknown:'Неизвестно'};
const reasons={
 'equipment-unavailable':'Экземпляр больше не виден в журнале. Выберите другой экземпляр или новый предмет.',
 'weapon-incompatible':'Эта модификация несовместима с типом выбранного оружия.',
 'weapon-type-unknown':'Тип оружия не подтверждён журналом.',
 'slots-unknown':'Количество свободных слотов не подтверждено журналом.',
 'no-free-slot':'Нет свободного слота для модификации.',
 'upgrade-equipment':'Повысьте ступень снаряжения, чтобы открыть слот для модификации.',
 'grade-unknown':'Текущий класс экземпляра неизвестен.',
 'mods-unknown':'Установленные модификации неизвестны.',
 'incompatible-goals':'У одного экземпляра несовместимые цели в разных комплектах.',
 'missing-recipe':'Рецепт для этого улучшения неизвестен.',
};
function reference(u,id,title){
 return id?'<button type="button" class="table-entity-link" data-reference-id="'+u.esc(id)+'">'+u.esc(title)+'</button>':u.esc(title);
}
function stepMarkup(u,step){
 const title=step.title?u.t(step.title):step.kind==='grade'
  ?u.t('Улучшить до ступени {grade}',{grade:u.fmt(step.grade||step.toGrade)}):name(u,step)||u.t('Модификация');
 const source=list(step.engineers).map(engineer=>reference(u,engineer.entryId,name(u,engineer))).join(', ');
 const state=stepStates[step.status];
 const materials=step.status==='done'?'':list(step.materials).map(row=>reference(u,row.entryId,name(u,row))+' × '+number(u,row.count)).join(', ');
 const price=step.status!=='done'&&Number(step.credits)>0?tr(u,'{count} кр.',{count:u.fmt(step.credits)}):'';
 return '<li><div><strong>'+u.esc(title)+'</strong>'+(state?'<small>'+tr(u,state)+'</small>':'')+(source?'<span>'+source+'</span>':'')+
  (materials||price?'<span>'+[materials,price].filter(Boolean).join(' · ')+'</span>':'')+'</div></li>';
}
function kit(r,u){
 const current=selected(r);
 if(!current)return '';
 return '<section class="panel odyssey-plan"><header><h2>'+u.esc(current.name)+'</h2>'+(current.reserve?u.badge(u.t('Резерв включён')):'')+'</header>'+
  '<div class="odyssey-equipment">'+list(current.items).map(item=>{
   const steps=list(item.steps),conditions=list(item.conditions).map(condition=>reasons[typeof condition==='string'?condition:condition?.code]||
    condition?.text||condition?.message).filter(Boolean);
   const installed=list(item.installedMods).map(mod=>typeof mod==='string'?
    name(u,list(r.data.modifications).find(entry=>entry.key===mod||entry.journalSymbol===mod)):name(u,mod)).filter(Boolean);
   const start=item.equipmentKey?item.currentGrade:item.startingGrade??item.currentGrade;
   return '<article><h3>'+u.esc(name(u,item))+'</h3><p>'+tr(u,'Ступень {current} → {target}',{current:start==null?u.t('неизвестна'):u.fmt(start),target:u.fmt(item.targetGrade)})+'</p>'+
    (installed.length?'<p>'+tr(u,'Установлено')+' · '+u.esc(installed.join(', '))+'</p>':'')+
    (steps.length?'<ol>'+steps.map(step=>stepMarkup(u,step)).join('')+'</ol>':'')+
    (conditions.length?'<ul class="odyssey-conditions">'+conditions.map(condition=>'<li>'+tr(u,condition)+'</li>').join('')+'</ul>':'')+'</article>';
  }).join('')+'</div></section>';
}
function observed(u,value){
 if(!value)return '';
 const display=u.date?.(value,{day:'numeric',month:'short',hour:'2-digit',minute:'2-digit'})||value;
 return '<small><time datetime="'+u.esc(value)+'">'+u.esc(display)+'</time></small>';
}
function stockTable(u,rows,{complete,combined=false}={}){
 const incomplete=complete===false?'<p class="odyssey-empty">'+tr(u,'Часть улучшений не вошла в расчёт материалов.')+'</p>':'';
 if(!rows.length)return incomplete||'<p class="odyssey-empty">'+tr(u,'Материалы для этих улучшений не требуются.')+'</p>';
 const multipleStamps={};
 const head=(label,stamp='')=>{
  const dates=stamp?[...new Set(rows.map(row=>row[stamp]).filter(Boolean))]:[];
  if(stamp)multipleStamps[stamp]=dates.length>1;
  return '<th scope="col">'+tr(u,label)+(dates.length===1?observed(u,dates[0]):'')+'</th>';
 };
 const reserved=!combined&&rows.some(row=>Number(row.reservedOther)>0),spare=rows.some(row=>Number(row.spare)>0);
 return incomplete+'<div class="odyssey-table"><table><thead><tr>'+head('Материал')+head('Требуется')+
  head('Рюкзак','backpackObservedAt')+head('Шкафчик','lockerObservedAt')+(reserved?head('Резерв других комплектов'):'')+head('Не хватает')+(spare?head('Излишек'):'')+'</tr></thead><tbody>'+
  rows.map(row=>{
   const missing=combined?row.sharedMissing:row.missing,upper=combined?row.sharedMissingAtMost:row.missingAtMost;
   const deficit=missing!=null?number(u,missing):upper!=null?tr(u,'Не более {count}',{count:u.fmt(upper)}):tr(u,'Неизвестно');
   const stock=(key,stamp)=>'<td>'+number(u,row[key])+(multipleStamps[stamp]?observed(u,row[stamp]):'')+'</td>';
   return '<tr><th scope="row">'+reference(u,row.entryId,name(u,row))+'</th><td>'+number(u,row.count)+'</td>'+stock('backpack','backpackObservedAt')+stock('locker','lockerObservedAt')+
    (reserved?'<td>'+number(u,row.reservedOther)+'</td>':'')+'<td>'+deficit+'</td>'+(spare?'<td>'+number(u,row.spare)+'</td>':'')+'</tr>';
  }).join('')+'</tbody></table></div>';
}
function content(C,u){
 const r=model(C),data=r.data;
 if(!data)return r.error?'<p class="odyssey-error" role="alert">'+u.esc(r.error)+'</p>'+button(u,'retry','Повторить'):
  '<p class="odyssey-empty" role="status">'+tr(u,'Загружаем снаряжение…')+'</p>';
 if(r.draft)return editor(r,u);
 const loadouts=list(data.loadouts),current=selected(r),readonly=!r.owner;
 const picker=loadouts.length?label(u,'Активный комплект','<select id="odyssey-active-kit" '+(r.writing?'disabled':'')+'>'+
  options(u,loadouts.map(kit=>({key:kit.id,text:kit.name})),data.selectedId)+'</select>'):'';
 const controls='<div class="odyssey-toolbar">'+picker+button(u,'new','Новый комплект','primary','id="odyssey-new-kit" '+(readonly||r.writing?'disabled':''))+
  (current?button(u,'edit','Изменить','quiet','id="odyssey-edit-kit" '+(readonly||r.writing?'disabled':''))+
   button(u,'ask-remove','Удалить','quiet','id="odyssey-delete-kit" '+(readonly||r.writing?'disabled':'')):'')+'</div>';
 const remove=r.confirmRemove?'<div class="odyssey-confirm"><span>'+tr(u,'Удалить комплект?')+'</span>'+button(u,'remove','Удалить')+button(u,'cancel-remove','Отмена','quiet','id="odyssey-confirm-cancel"')+'</div>':'';
 const notice=readonly?'<p class="odyssey-empty">'+tr(u,'Чтобы сохранить комплект, загрузите командира из журнала игры.')+'</p>':
  !loadouts.length?'<p class="odyssey-empty">'+tr(u,'Сохранённых комплектов пока нет.')+'</p>':'';
 const materials=list(data.materials),portfolio=list(data.portfolioMaterials);
 return controls+remove+(r.error?'<p class="odyssey-error" role="alert">'+u.esc(r.error)+'</p>':'')+notice+kit(r,u)+
  (current?'<section class="panel odyssey-stock"><h2>'+tr(u,'Материалы комплекта')+'</h2>'+stockTable(u,materials,{complete:data.activeComplete})+'</section>':'')+
  (loadouts.length>1&&(portfolio.length||data.portfolioComplete===false)?'<details class="panel odyssey-stock" data-odyssey-portfolio '+(r.portfolioOpen?'open':'')+'><summary>'+tr(u,'Потребность с учётом резервов')+'</summary>'+stockTable(u,portfolio,{complete:data.portfolioComplete,combined:true})+'</details>':'');
}
function render(C,u){
 return u.heading(u.t(mode),'')+'<div id="odyssey-workspace" class="odyssey-workspace">'+content(C,u)+'</div>';
}
function draw(C,u,focusId){
 if(!active(C))return;
 const host=u.$('#odyssey-workspace');if(!host)return;
 const activeElement=host.ownerDocument?.activeElement;
 const target=focusId||(host.contains?.(activeElement)?activeElement.id:'');
 const html=content(C,u);
 if(host.odysseyMarkup!==html){host.innerHTML=html;host.odysseyMarkup=html;}
 if(target)host.ownerDocument?.getElementById(target)?.focus({preventScroll:true});
}
function readSignature(C){
 return JSON.stringify([C.state?.odysseyRevision??C.state?.engineeringRoadmapRevision,C.state?.settings?.language||'en']);
}
function load(C,u,force=false){
 if(!active(C))return;
 const r=model(C),signature=readSignature(C);
 if(!force&&(r.loading||r.signature===signature))return;
 r.loading=true;r.signature=signature;
 const request=++r.sequence;
 const valid=()=>model(C)===r&&r.sequence===request;
 const latest=()=>valid()&&signature===readSignature(C);
 r.pending=(async()=>{
  try{
   const data=await u.call('odyssey.loadouts',{expectedPlanWorkspace:r.owner});
   if(latest()){r.data=data;if(!r.draft)r.error='';}
  }catch(error){
   if(latest()&&!r.draft)r.error=u.t(error?.message||'Не удалось загрузить снаряжение.');
  }finally{
   if(valid()){
    r.loading=false;
    if(signature!==readSignature(C))load(C,u);
    else draw(C,u);
   }
  }
 })();
}
async function mutate(C,u,params){
 const r=model(C);
 if(!r.owner||r.writing)return;
 let focusId='';
 r.sequence++;r.loading=false;r.writing=true;r.error='';draw(C,u);
 try{
  const result=await u.call('odyssey.loadout-update',{...params,expectedPlanWorkspace:r.owner});
  if(model(C)!==r)return;
  r.draft=null;r.confirmRemove=false;
  if(Array.isArray(result?.loadouts)){r.data=result;focusId=result.loadouts.length?'odyssey-active-kit':'odyssey-new-kit';}
  else load(C,u,true);
 }catch(error){
  if(model(C)===r){r.error=u.t(error?.message||'Не удалось сохранить комплект.');if(r.draft)focusId='odyssey-save-kit';}
 }finally{
  r.writing=false;
  if(model(C)===r){draw(C,u,focusId);load(C,u);}
 }
}
function draftEvent(event,C,u){
 const r=model(C),target=event.target,d=r.draft;
 if(!d)return;
 if(target.id==='odyssey-kit-name'){d.name=target.value;return;}
 if(target.id==='odyssey-kit-reserve'){d.reserve=target.checked;return;}
 const index=Number(target.dataset?.index),line=d.items[index];
 if(!line)return;
 if(target.dataset.odysseyMod){
  const key=target.dataset.odysseyMod;
  line.modKeys=target.checked?[...new Set([...line.modKeys,key])]:line.modKeys.filter(item=>item!==key);
 }else{
  const field=target.dataset.odysseyField;
  if(field==='itemKey'){d.items[index]={itemKey:target.value,equipmentKey:'',startingGrade:1,targetGrade:1,modKeys:[]};}
  else if(field==='equipmentKey'){
   line.equipmentKey=target.value;
   const instance=list(r.data?.ownedEquipment).find(item=>item.key===target.value);
   line.targetGrade=Math.max(line.targetGrade,instance?.grade||line.startingGrade);
  }else if(field==='startingGrade'){line.startingGrade=Number(target.value);line.targetGrade=Math.max(line.targetGrade,line.startingGrade);}
  else if(field==='targetGrade')line.targetGrade=Number(target.value);
  else return;
 }
 draw(C,u,target.id);
}
function newItem(r){
 const candidates=availableEquipment(r);
 const item=candidates.find(item=>item.slot==='suit')||candidates.find(item=>item.slot==='primary')||candidates[0];
 if(item)r.draft.items.push({itemKey:item.key,equipmentKey:'',startingGrade:1,targetGrade:1,modKeys:[]});
}
function handleAction(action,target,C,u){
 const r=model(C),current=selected(r);
 switch(action){
 case 'retry':load(C,u,true);return;
 case 'new':case 'edit':
  r.draft=draftFor(action==='edit'?current:null);r.error='';r.openMods=new Set();
  if(!r.draft.items.length)newItem(r);
  draw(C,u,'odyssey-kit-name');return;
 case 'cancel-edit':r.draft=null;r.error='';draw(C,u,current?'odyssey-edit-kit':'odyssey-new-kit');return;
 case 'add-item':if(r.draft&&r.draft.items.length<4)newItem(r);draw(C,u,'odyssey-itemKey-'+(r.draft.items.length-1));return;
 case 'remove-item':r.draft?.items.splice(Number(target.dataset.index),1);r.openMods?.clear();draw(C,u);return;
 case 'ask-remove':r.confirmRemove=true;draw(C,u,'odyssey-confirm-cancel');return;
 case 'cancel-remove':r.confirmRemove=false;draw(C,u,'odyssey-delete-kit');return;
 case 'remove':if(current)void mutate(C,u,{action:'remove',id:current.id});return;
 }
}
function init(C,u){
 if(!active(C))return;
 const host=u.$('#odyssey-workspace'),r=model(C);
 if(host&&!host.odysseyBound){
  host.odysseyBound=true;host.odysseyMarkup=content(C,u);
  host.addEventListener('click',event=>{
   const target=event.target.closest?.('[data-odyssey-action]');
   if(target)handleAction(target.dataset.odysseyAction,target,C,u);
  });
  host.addEventListener('input',event=>{if(event.target.id==='odyssey-kit-name')draftEvent(event,C,u);});
  host.addEventListener('change',event=>{
   if(event.target.id==='odyssey-active-kit'){if(event.target.value)void mutate(C,u,{action:'select',id:event.target.value});return;}
   draftEvent(event,C,u);
  });
  host.addEventListener('submit',event=>{
   if(event.target.id!=='odyssey-kit-form')return;
   event.preventDefault();
   const draft=model(C).draft;
   if(draft?.items.length)void mutate(C,u,{action:'save',...draft,name:draft.name.trim()});
  });
  host.addEventListener('toggle',event=>{
   const d=event.target,live=model(C);
   if(d.hasAttribute?.('data-odyssey-portfolio'))live.portfolioOpen=d.open;
   const index=d.dataset?.odysseyModPicker;
   if(index!==undefined&&live.openMods){if(d.open)live.openMods.add(Number(index));else live.openMods.delete(Number(index));}
  },true);
 }
 if(!r.writing)load(C,u);
}
function leave(C){
 const r=C.odysseyUI;
 if(r?.loading){r.sequence++;r.loading=false;r.signature=null;}
}
window.OdysseyUI={active,render,init,leave};
})();
