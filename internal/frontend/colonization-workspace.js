(()=>{
'use strict';
const t=(value,vars)=>window.CopilotI18n?.t(value,vars)??String(value??'');
const th=(value,vars)=>t(value,vars).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const same=(a,b)=>String(a||'').trim().toLocaleLowerCase('ru-RU')===String(b||'').trim().toLocaleLowerCase('ru-RU');
const known=value=>value!==null&&value!==undefined&&value!==''&&Number.isFinite(Number(value));
const statusNames={planned:'Личный план',claimed:'Заявка подана',building:'Строительство',complete:'Завершена',cancelled:'Отменена'};
function active(C){return C?.tab==='colonization';}
function selectionMode(C){return ['',"Выбор системы",'selection'].includes(C.mode||'');}
function state(C){
 if(!C.colonizationWorkspace){
  const colony=C.state?.colonization||{};
  C.colonizationWorkspace={origin:String(colony.search?.options?.originSystem||colony.originSystem||colony.config?.originSystem||C.state?.commander?.system||''),faction:'',station:'',pending:false,error:''};
 }
 return C.colonizationWorkspace;
}
function stationKey(station){return [station.marketId||'',station.name||'',station.faction||''].join('::');}
function normalized(value){return String(value||'').trim().toLowerCase().replace(/[^a-z0-9]/g,'');}
function realFaction(value){return !['carrier','fleetcarrier','squadroncarrier','strongholdcarrier','megaship','constructiondepot','constructionsite','depot'].includes(normalized(value));}
function eligibleSourceStation(station){
 const type=normalized(station?.type||station?.stationType);
 return !!station&&station.claimSourceEligible!==false&&station.hasColonisationContact!==false&&!station.isCarrier&&!station.constructionDepot&&!station.invalidClaimFaction&&!/carrier|megaship|construction|depot/.test(type)&&realFaction(station.faction);
}
function source(C){
 const r=state(C),search=C.state?.colonization?.search||{},ready=!!search.origin?.name&&same(r.origin,search.origin.name);
 const available=Array.isArray(search.claimStations)?search.claimStations:Array.isArray(search.origin?.stations)?search.origin.stations:[];
 const stations=ready?available.filter(station=>eligibleSourceStation(station)&&(!station.system||same(station.system,search.origin.name))):[];
 return {r,search,ready,stations,selected:stations.find(station=>stationKey(station)===r.station)};
}
function format(value,u,suffix=''){return known(value)?u.fmt(Number(value))+suffix:t("Неизвестно");}
function action(C,u,id,label,icon='arrow-right'){
 const item=(C.actions||[]).find(a=>a.id===id);
 return item?u.btn(label||item.title,'action',icon,'quiet','type="button" data-action-id="'+u.esc(id)+'" '+(item.disabled?'disabled':'')):'';
}
function metric(label,value,detail,u){return '<div class="colony-summary-metric"><span>'+u.esc(label)+'</span><strong>'+u.esc(value)+'</strong><small>'+u.esc(detail||'')+'</small></div>';}
function date(value){
 if(!value)return '';
 const parsed=new Date(value);return Number.isNaN(+parsed)?'':parsed.toLocaleString(window.CopilotI18n?.locale()||'ru-RU',{day:'2-digit',month:'2-digit',year:'numeric',hour:'2-digit',minute:'2-digit'});
}
function summary(C,u){
 const colony=C.state?.colonization||{},project=colony.activeProject;
 if(!project?.id)return '<section class="colony-start"><div class="colony-start-symbol">'+u.icon('orbit')+`</div><div><span class="eyebrow">${th("От первой заявки к собственной системе")}</span><h2>${th("Спланируйте новую колонию")}</h2><p>${th("Проверьте исходную станцию, выберите соседнюю систему, затем добавьте объекты и очередь снабжения. Каждая ваша колония получает отдельный план.")}</p></div>`+action(C,u,'colony.create',t("Добавить план вручную"),'plus')+'</section>';
 const buildings=project.buildings||[],unfinished=buildings.filter(item=>!['complete','cancelled','failed'].includes(item.status));
 const unknown=Number(project.unknownBills)||0,missing=known(project.totalMissing)?Number(project.totalMissing):null;
 const remaining=missing===null||unknown>0&&missing===0?t("Неизвестно"):(unknown>0?t("Не менее "):'')+u.fmt(missing)+t(" т");
 const next=project.nextTask||{},deadline=project.primaryComplete||project.deadlineActive===false?'':date(project.effectiveDeadline??project.deadline),warnings=project.warnings||[];
 const needs=(project.operationalSupplies||project.supplies||[]).filter(item=>known(item.missing)&&Number(item.missing)>0).slice(0,4);
 return '<section class="colony-overview" data-colony-id="'+u.esc(project.id)+`"><header><div class="colony-overview-title"><span class="eyebrow">${th("Выбранная колония")}</span><h2>`+u.esc(project.name||project.targetSystem)+'</h2><div class="colony-overview-meta"><span>'+u.icon('orbit')+u.esc(project.targetSystem||t("Система не задана"))+'</span>'+u.badge(t(statusNames[project.status]||"Статус не задан"))+'</div></div><div class="colony-overview-actions">'+(project.targetSystem?u.btn(t("Копировать систему"),'colony-workspace-copy','copy','quiet','type="button" data-system="'+u.esc(project.targetSystem)+'"'):'')+action(C,u,'colony.edit',t("Параметры"),'settings')+`</div></header><dl class="colony-origin-summary"><div><dt>${th("Исходная система")}</dt><dd>`+u.esc(project.originSystem||t("Не задана"))+`</dd></div><div><dt>${th("Фракция колонии")}</dt><dd>`+u.esc(project.faction||t("Не задана"))+`</dd></div><div><dt>${th("Направление")}</dt><dd>`+u.esc(project.economyPlan?.name||t("Не задано"))+'</dd></div></dl><div class="colony-summary-metrics">'+metric(t("Объектов в плане"),u.fmt(buildings.length),t("В строительстве: ")+u.fmt(project.concurrent||0),u)+metric(t("Осталось доставить"),remaining,unknown?t("Без ")+u.fmt(unknown)+t(" неизвестных смет"):t("По учтённым требованиям"),u)+metric(t("Сметы объектов"),u.fmt(Math.max(0,unfinished.length-unknown))+' / '+u.fmt(unfinished.length),t("Для незавершённых объектов"),u)+metric(t("Очки архитектора"),'T2 '+format(project.points?.tier2,u)+' · T3 '+format(project.points?.tier3,u),t("Текущий остаток из параметров колонии"),u)+'</div>'+(next.text?'<div class="colony-next-step"><span>'+u.icon(next.kind==='delivery'?'package':'arrow-right')+`</span><div><strong>${th("Следующий шаг")}</strong><p>`+u.esc(next.text)+'</p>'+(deadline?`<small>${th("Срок первичного порта: ")}`+u.esc(deadline)+(project.deadlineSource==='claim-estimate'?t(" · расчёт по дате заявки"):'')+'</small>':'')+'</div>'+(next.kind==='delivery'?`<button type="button" class="pill quiet" data-mode="Снабжение">`+u.icon('arrow-right')+`${th("К снабжению")}</button>`:'')+'</div>':'')+(needs.length?`<div class="colony-supply-preview"><div class="colony-supply-heading"><strong>${th("Ближайшие потребности")}</strong><button type="button" data-mode="Снабжение">${th("Все товары ")}`+u.icon('arrow-right')+'</button></div><div class="colony-supply-items">'+needs.map(item=>'<div><span>'+u.esc(item.name||item.commodity)+'</span><strong>'+u.esc(format(item.missing,u,t(" т")))+`</strong><small>${th("В корабле: ")}`+u.esc(format(item.cargoAvailable,u,t(" т")))+'</small></div>').join('')+'</div></div>':'')+(warnings.length?`<details class="colony-data-warnings"><summary>${th("Проверить данные · ")}`+u.esc(u.fmt(warnings.length))+'</summary><ul>'+warnings.map(item=>'<li>'+u.esc(item)+'</li>').join('')+'</ul></details>':'')+'</section>';
}
function originSearch(C,u){
 const {r,search,ready,stations,selected}=source(C),running=!!search.running||r.pending;
 const contact=selected?.hasColonisationContact===true?t("Контакт колонизации обнаружен"):t("Наличие контакта не подтверждено — проверьте в игре");
 const faction=selected?.faction||(realFaction(r.faction)?r.faction:'');
 const step2=ready?`<div class="colony-source-step" data-colony-source-ready><div class="colony-step-heading"><span>2</span><div><h3>${th("Выберите исходную станцию и её фракцию")}</h3><p>${th("Фракция станции определяет сторону колонизации. Присутствия фракции в системе недостаточно.")}</p></div></div>`+(stations.length?`<label class="colony-source-field"><span>${th("Станция в ")}`+u.esc(search.origin.name)+`</span><select id="colony-workspace-station"><option value="">${th("Выберите станцию")}</option>`+stations.map(station=>'<option value="'+u.esc(stationKey(station))+'" '+(r.station===stationKey(station)?'selected':'')+' '+(!station.faction?'disabled':'')+'>'+u.esc(station.name||t("Без названия"))+' · '+u.esc(station.faction||t("Фракция неизвестна"))+(station.hasColonisationContact===true?'':t(" · контакт не подтверждён"))+'</option>').join('')+'</select></label>':`<p class="colony-source-note">${th("В базе нет подходящих станций. Если вы видите станцию и контакт в игре, укажите её фракцию вручную.")}</p>`)+(selected?'<div class="colony-station-check"><strong>'+u.esc(selected.name)+'</strong><span>'+u.esc(selected.type||t("Тип станции не указан"))+(known(selected.distanceLs)?' · '+u.esc(u.fmt(selected.distanceLs))+t(" св. с."):'')+'</span><span class="'+(selected.hasColonisationContact===true?'confirmed':'')+'">'+u.icon(selected.hasColonisationContact===true?'circle-check':'circle-help')+u.esc(contact)+'</span></div>':'')+`<label class="colony-source-field"><span>${th("Фракция исходной станции")}</span><input id="colony-workspace-faction" value="`+u.esc(faction)+`" list="colony-workspace-factions" placeholder="${th("Выберите станцию или введите фракцию из игры")}" autocomplete="off"><datalist id="colony-workspace-factions">`+(search.factions||[]).filter(item=>realFaction(item.name)).map(item=>'<option value="'+u.esc(item.name)+'"></option>').join('')+'</datalist></label><div class="colony-source-submit">'+u.btn(running?t("Поиск…"):t("Найти системы для этой фракции"),'colony-workspace-find','search','primary','type="button" '+(running?'disabled':''))+`</div><p class="colony-source-note">${th("Контакт и доступность заявки подтвердите в игре. Выбор станции здесь не создаёт заявку и не меняет выбранную колонию.")}</p>`+(search.options?.faction?`<p class="colony-source-note">${th("Сейчас показаны станции фракции «")}`+u.esc(search.options.faction)+`${th("». Повторная проверка системы вернёт список всех станций.")}</p>`:'')+'</div>':`<div class="colony-source-step colony-source-await"><div class="colony-step-heading"><span>2</span><div><h3>${th("Станция → фракция → новая система")}</h3><p>${th("После проверки появятся известные станции и их контролирующие фракции. Выберите станцию, от которой планируете колонизацию.")}</p></div></div></div>`;
 return `<section class="colony-source-workspace" aria-label="${th("Подбор системы для колонии")}"><div class="colony-source-step"><div class="colony-step-heading"><span>1</span><div><h3>${th("Проверьте исходную обитаемую систему")}</h3><p>${th("Сначала найдём её станции. Фракцию не нужно угадывать до первого поиска.")}</p></div></div><label class="colony-source-field"><span>${th("Исходная система")}</span><input id="colony-workspace-origin" value="`+u.esc(r.origin)+`" placeholder="${th("Точное название системы")}" autocomplete="off" maxlength="200"></label>`+u.btn(running?t("Проверяем…"):t("Проверить систему"),'colony-workspace-check','search','quiet','type="button" '+(running?'disabled':''))+`<p class="colony-source-note">${th("При первой проверке также загрузятся соседние кандидаты в радиусе колонизации. После выбора станции поиск можно уточнить под её фракцию.")}</p></div>`+step2+(ready?`<div class="colony-source-step colony-source-await" data-colony-source-stale hidden><p class="colony-source-note">${th("Исходная система изменена. Проверьте её, чтобы выбрать подходящую станцию.")}</p></div>`:'')+'<div class="colony-workspace-error" role="alert" '+(!r.error?'hidden':'')+'>'+u.esc(r.error)+'</div>'+(search.error?'<p class="colony-search-error" role="status">'+u.esc(search.error)+'</p>':'')+'</section>';
}
function render(C,u){if(!active(C))return '';return '<div data-colony-workspace>'+summary(C,u)+(selectionMode(C)?originSearch(C,u):'')+'</div>';}
function clearError(C,u){const r=state(C);r.error='';const node=u.$('.colony-workspace-error');if(node){node.textContent='';node.hidden=true;}}
function clearStation(C,u,changedOrigin=false){
 const selector=u.$('#colony-workspace-station');if(selector)selector.value='';
 const check=u.$('.colony-station-check');if(check)check.hidden=true;
 if(changedOrigin){
  const field=u.$('#colony-workspace-faction');if(field)field.value='';
  const ready=same(state(C).origin,C.state?.colonization?.search?.origin?.name);
  const details=u.$('[data-colony-source-ready]');if(details)details.hidden=!ready;
  const stale=u.$('[data-colony-source-stale]');if(stale)stale.hidden=ready;
 }
}
function handleInput(event,C,u){
 if(!active(C))return false;
 const target=event.target,r=state(C);
 if(target?.id==='colony-workspace-origin'){r.origin=target.value;r.station='';r.faction='';clearStation(C,u,true);clearError(C,u);return true;}
 if(target?.id==='colony-workspace-faction'){r.faction=target.value;r.station='';clearStation(C,u);clearError(C,u);return true;}
 return false;
}
function handleChange(event,C,u){
 if(!active(C)||event.target?.id!=='colony-workspace-station')return false;
 const {r,stations}=source(C),station=stations.find(item=>stationKey(item)===event.target.value);
 r.station=station?stationKey(station):'';r.faction=station?.faction||'';r.error='';
 u.renderPage();u.$('#colony-workspace-station')?.focus?.({preventScroll:true});return true;
}
async function handleClick(action,button,C,u){
 if(!active(C)||!action?.startsWith('colony-workspace-'))return false;
 if(action==='colony-workspace-copy'){
  const system=C.state?.colonization?.activeProject?.targetSystem;
  if(system){await u.call('native',{action:'copy',text:system});u.toast(t("Система колонии скопирована"));}return true;
 }
 if(!['colony-workspace-check','colony-workspace-find'].includes(action))return false;
 const {r,search,ready,selected}=source(C);
 if(r.pending||search.running)return true;
 const origin=r.origin.trim(),faction=action==='colony-workspace-check'?'':String(selected?.faction||r.faction).trim();
 r.error='';
 if(!origin)r.error=t("Укажите точное название исходной обитаемой системы.");
 else if(action==='colony-workspace-find'&&!ready)r.error=t("Сначала проверьте выбранную исходную систему, затем выберите станцию.");
 else if(action==='colony-workspace-find'&&!faction)r.error=t("Выберите исходную станцию или укажите её контролирующую фракцию по данным игры.");
 else if(action==='colony-workspace-find'&&!realFaction(faction))r.error=t("Выберите малую фракцию обычной исходной станции, а не служебный тип флотоносца или стройплощадки.");
 if(r.error){u.renderPage();return true;}
 r.pending=true;u.renderPage();
 try{
  await u.call('action',{...(u.context?.()||{}),tab:'colonization',mode:'Выбор системы',query:'',selected:'',actionId:'colony.search',values:{originSystem:origin,faction}});
  if(active(C))await u.loadView();
 }catch(error){r.error=error?.message||t("Не удалось начать поиск. Проверьте название системы и повторите.");}
 finally{r.pending=false;if(active(C))u.renderPage();}
 return true;
}
function init(C,u){
 if(!active(C))return;
 const r=state(C),{stations}=source(C);
 if(r.station&&!stations.some(item=>stationKey(item)===r.station)){r.station='';r.faction='';}
 if(!realFaction(r.faction))r.faction='';
}
window.ColonyWorkspaceUI={active,render,init,handleClick,handleInput,handleChange};
})();
