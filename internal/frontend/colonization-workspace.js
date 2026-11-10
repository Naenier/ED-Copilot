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
function summary(C,u,project=C.state?.colonization?.activeProject){
 const colony=C.state?.colonization||{};
 if(!project?.id)return '<section class="colony-start"><div class="colony-start-symbol">'+u.icon('orbit')+`</div><div><span class="eyebrow">${th("Колонизация")}</span><h2>${th("Колонии не добавлены")}</h2><p>${th("Найдите систему или добавьте колонию вручную.")}</p></div>`+action(C,u,'colony.create',t("Добавить план вручную"),'plus')+'</section>';
 const buildings=project.buildings||[],unfinished=buildings.filter(item=>!['complete','cancelled','failed'].includes(item.status));
 const unknown=Number(project.unknownBills)||0,missing=known(project.totalMissing)?Number(project.totalMissing):null;
 const remaining=missing===null||unknown>0&&missing===0?t("Неизвестно"):(unknown>0?t("Не менее "):'')+u.fmt(missing)+t(" т");
 const next=project.nextTask||{},deadline=project.primaryComplete||project.deadlineActive===false?'':date(project.effectiveDeadline??project.deadline),warnings=project.warnings||[];
 const needs=(project.operationalSupplies||project.supplies||[]).filter(item=>known(item.missing)&&Number(item.missing)>0).slice(0,4);
 return '<section class="colony-overview" data-colony-id="'+u.esc(project.id)+`"><header><div class="colony-overview-title"><h2>`+u.esc(project.name||project.targetSystem)+'</h2><div class="colony-overview-meta"><span>'+u.icon('orbit')+u.esc(project.targetSystem||t("Система не задана"))+'</span>'+u.badge(t(statusNames[project.status]||"Статус не задан"))+'</div></div><div class="colony-overview-actions">'+(project.targetSystem?u.btn(t("Копировать систему"),'colony-workspace-copy','copy','quiet','type="button" data-project="'+u.esc(project.id)+'"'):'')+(colony.activeProject?.id===project.id?action(C,u,'colony.edit',t("Параметры"),'settings'):'')+u.btn(t('Строительство'),'colony-workspace-open','arrow-right','quiet','data-project="'+u.esc(project.id)+'"')+`</div></header><dl class="colony-origin-summary"><div><dt>${th("Исходная система")}</dt><dd>`+u.esc(project.originSystem||t("Не задана"))+`</dd></div><div><dt>${th("Фракция колонии")}</dt><dd>`+u.esc(project.faction||t("Не задана"))+`</dd></div><div><dt>${th("Направление")}</dt><dd>`+u.esc(project.economyPlan?.name||t("Не задано"))+'</dd></div></dl><div class="colony-summary-metrics">'+metric(t("Объектов в плане"),u.fmt(buildings.length),t("В строительстве: ")+u.fmt(project.concurrent||0),u)+metric(t("Осталось доставить"),remaining,unknown?t("Без ")+u.fmt(unknown)+t(" неизвестных смет"):t("По учтённым требованиям"),u)+metric(t("Сметы объектов"),u.fmt(Math.max(0,unfinished.length-unknown))+' / '+u.fmt(unfinished.length),t("Для незавершённых объектов"),u)+metric(t("Очки архитектора"),'T2 '+format(project.points?.tier2,u)+' · T3 '+format(project.points?.tier3,u),t("Введено в параметрах колонии"),u)+'</div>'+(next.text?'<div class="colony-next-step"><span>'+u.icon(next.kind==='delivery'?'package':'arrow-right')+`</span><div><strong>${th("Следующий шаг")}</strong><p>`+u.esc(next.text)+'</p>'+(deadline?`<small>${th("Срок первичного порта: ")}`+u.esc(deadline)+(project.deadlineSource==='claim-estimate'?t(" · расчёт по дате заявки"):'')+'</small>':'')+'</div>'+'</div>':'')+(needs.length?`<div class="colony-supply-preview"><div class="colony-supply-heading"><strong>${th("Ближайшие потребности")}</strong></div><div class="colony-supply-items">`+needs.map(item=>'<div><span>'+u.esc(item.name||item.commodity)+'</span><strong>'+u.esc(format(item.missing,u,t(" т")))+`</strong><small>${th("В корабле: ")}`+u.esc(format(item.cargoAvailable,u,t(" т")))+'</small>'+(colony.includeCarrierWarehouse?'<small>'+th('На флотоносце: ')+u.esc(format(item.carrierAvailable,u,t(' т')))+'</small><small>'+th('Купить: ')+u.esc(known(item.toBuy)?format(item.toBuy,u,t(' т')):known(item.toBuyAtMost)?t('Не более ')+format(item.toBuyAtMost,u,t(' т')):t('Неизвестно'))+'</small>':'')+'</div>').join('')+'</div></div>':'')+(warnings.length?`<details class="colony-data-warnings"><summary>${th("Проверить данные · ")}`+u.esc(u.fmt(warnings.length))+'</summary><ul>'+warnings.map(item=>'<li>'+u.esc(item)+'</li>').join('')+'</ul></details>':'')+'</section>';
}
function originSearch(C,u){
 const {r,search,ready,stations,selected}=source(C),running=!!search.running||r.pending;
 const contact=selected?.hasColonisationContact===true?t("Контакт колонизации обнаружен"):t("Наличие контакта не подтверждено — проверьте в игре");
 const faction=selected?.faction||(realFaction(r.faction)?r.faction:'');
 const step2=ready?`<div class="colony-source-step" data-colony-source-ready><div class="colony-step-heading"><span>2</span><div><h3>${th("Выберите исходную станцию и её фракцию")}</h3><p>${th("Используется контролирующая фракция выбранной станции.")}</p></div></div>`+(stations.length?`<label class="colony-source-field"><span>${th("Станция в ")}`+u.esc(search.origin.name)+`</span><select id="colony-workspace-station"><option value="">${th("Выберите станцию")}</option>`+stations.map(station=>'<option value="'+u.esc(stationKey(station))+'" '+(r.station===stationKey(station)?'selected':'')+' '+(!station.faction?'disabled':'')+'>'+u.esc(station.name||t("Без названия"))+' · '+u.esc(station.faction||t("Фракция неизвестна"))+(station.hasColonisationContact===true?'':t(" · контакт не подтверждён"))+'</option>').join('')+'</select></label>':`<p class="colony-source-note">${th("В базе нет подходящих станций. Если вы видите станцию и контакт в игре, укажите её фракцию вручную.")}</p>`)+(selected?'<div class="colony-station-check"><strong>'+u.esc(selected.name)+'</strong><span>'+u.esc(selected.type||t("Тип станции не указан"))+(known(selected.distanceLs)?' · '+u.esc(u.fmt(selected.distanceLs))+t(" св. с."):'')+'</span><span class="'+(selected.hasColonisationContact===true?'confirmed':'')+'">'+u.icon(selected.hasColonisationContact===true?'circle-check':'circle-help')+u.esc(contact)+'</span></div>':'')+`<label class="colony-source-field"><span>${th("Фракция исходной станции")}</span><input id="colony-workspace-faction" value="`+u.esc(faction)+`" list="colony-workspace-factions" placeholder="${th("Выберите станцию или введите фракцию из игры")}" autocomplete="off"><datalist id="colony-workspace-factions">`+(search.factions||[]).filter(item=>realFaction(item.name)).map(item=>'<option value="'+u.esc(item.name)+'"></option>').join('')+'</datalist></label><div class="colony-source-submit">'+u.btn(running?t("Поиск…"):t("Найти системы для этой фракции"),'colony-workspace-find','search','primary','type="button" '+(running?'disabled':''))+`</div><p class="colony-source-note">${th("Доступность заявки проверьте у контакта колонизации в игре.")}</p>`+(search.options?.faction?`<p class="colony-source-note">${th("Сейчас показаны станции фракции «")}`+u.esc(search.options.faction)+`${th("». Повторная проверка системы вернёт список всех станций.")}</p>`:'')+'</div>':`<div class="colony-source-step colony-source-await"><div class="colony-step-heading"><span>2</span><div><h3>${th("Станция → фракция → новая система")}</h3><p>${th("Проверьте исходную систему, затем выберите станцию.")}</p></div></div></div>`;
 return `<section class="colony-source-workspace" aria-label="${th("Подбор системы для колонии")}"><div class="colony-source-step"><div class="colony-step-heading"><span>1</span><div><h3>${th("Проверьте исходную обитаемую систему")}</h3><p>${th("Поиск загрузит известные станции системы.")}</p></div></div><label class="colony-source-field"><span>${th("Исходная система")}</span><input id="colony-workspace-origin" value="`+u.esc(r.origin)+`" placeholder="${th("Точное название системы")}" autocomplete="off" maxlength="200"></label>`+u.btn(running?t("Проверяем…"):t("Проверить систему"),'colony-workspace-check','search','quiet','type="button" '+(running?'disabled':''))+`<p class="colony-source-note">${th("Выберите станцию, чтобы уточнить поиск по её фракции.")}</p></div>`+step2+(ready?`<div class="colony-source-step colony-source-await" data-colony-source-stale hidden><p class="colony-source-note">${th("Исходная система изменена. Проверьте её, чтобы выбрать подходящую станцию.")}</p></div>`:'')+'<div class="colony-workspace-error" role="alert" '+(!r.error?'hidden':'')+'>'+u.esc(r.error)+'</div>'+(search.error?'<p class="colony-search-error" role="status">'+u.esc(search.error)+'</p>':'')+'</section>';
}
function stockAccessScope(C){const id=(C.state?.colonization?.carrierWarehouses||[]).find(item=>item.kind==='squadron')?.id,fid=C.state?.commander?.fid;return id&&fid?String(fid)+'|'+String(id):'';}
function credits(value,u){return known(value)?u.fmt(Number(value))+t(' кр.'):t('Неизвестно');}
function budgetValue(item,u,atMost){
 if(item?.complete===true&&known(item.value))return credits(item.value,u);
 if(atMost?.complete===true&&known(atMost.value))return t('Не более ')+credits(atMost.value,u);
 if(known(item?.knownValue)&&Number(item.knownValue)>0)return t('Не менее ')+credits(item.knownValue,u);
 return t('Неизвестно');
}
function warehouseTime(item,u){
 let text=item.receivedAt?' · '+th('Получено ')+u.esc(date(item.receivedAt)):'';
 if(item.awaitingConfirmation&&item.updatedAt&&item.updatedAt!==item.receivedAt)text+=' · '+th('Скорректировано по журналу ')+u.esc(date(item.updatedAt));
 return text;
}
function budgetPriceQuality(C,cost,u){
 const freshness=window.FreshnessUI;
 if(!freshness||!cost.commodityCount)return '';
 const stale=Number(cost.stalePriceCount)||0,undated=Number(cost.undatedPriceCount)||0;
 const missing=Math.max(0,Number(cost.commodityCount)-(Number(cost.pricedCommodityCount)||0));
 const language=C.state?.settings?.language==='ru'?'ru':'en';
 const lines=[];
 if(stale)lines.push([t('Цен старше 7 дней'),u.fmt(stale)]);
 if(undated)lines.push([t('Цен неизвестного возраста'),u.fmt(undated)]);
 if(missing)lines.push([t('Товаров без средней цены'),u.fmt(missing)]);
 const oldest=freshness.formatDate(cost.oldestPriceAt,language);
 if(oldest)lines.push([t('Дата самой старой цены'),oldest]);
 return freshness.details(t(missing?'Неполная оценка':stale?'Устаревшая оценка':undated?'Возраст цен неизвестен':'Даты цен'),lines,u.esc,!!(stale||undated||missing));
}
function budget(C,u){
 const project=C.state?.colonization?.activeProject,cost=project?.costSummary;
 if(!project?.id||!cost)return '';
 const unknownBills=Number(cost.unknownBills)||0,manualBills=Number(cost.manualBills)||0;
 const warnings=[];
 if(unknownBills)warnings.push(t('Неизвестных смет: ')+u.fmt(unknownBills));
 if(manualBills)warnings.push(t('Ручных смет: ')+u.fmt(manualBills));
 const covered=[metric(t('Принято стройками'),budgetValue(cost.delivered,u),t('Стоимость уже принятых ресурсов по тем же ценам'),u),metric(t('Покрыто грузом корабля'),budgetValue(cost.cargoCovered,u),t('Груз ещё нужно передать на стройку'),u),metric(t('Покрыто складом флотоносца'),budgetValue(cost.carrierCovered,u),t('В пределах потребности и включённого учёта'),u)].join('');
 return '<section class="colony-budget" data-colony-budget="'+u.esc(project.id)+'"><header><div><h3>'+th('Бюджет ресурсов колонии')+'</h3></div><span class="colony-budget-scope">'+th('В плане: ')+u.esc(u.fmt(cost.plannedBuildings||0))+' · '+th('Строятся: ')+u.esc(u.fmt(cost.buildingBuildings||0))+'</span></header><div class="colony-budget-basis"><span>'+th('По средним ценам галактики')+'</span>'+budgetPriceQuality(C,cost,u)+'</div><div class="colony-budget-metrics">'+metric(t('Полная смета ресурсов'),budgetValue(cost.required,u),t('Все ресурсы запланированных и строящихся объектов'),u)+metric(t('Осталось доставить'),budgetValue(cost.missing,u),t('Стоимость ресурсов, ещё не принятых стройками'),u)+metric(t('Нужно закупить'),budgetValue(cost.toBuy,u,cost.toBuyAtMost),t('После учёта корабля и включённых складов'),u)+'</div>'+(warnings.length?'<p class="colony-budget-warning" role="status">'+u.esc(warnings.join(' · '))+'</p>':'')+'<details class="colony-budget-breakdown"><summary>'+th('Из чего складывается бюджет')+'</summary><div class="colony-budget-metrics">'+covered+'</div><p>'+th('Готовые, отменённые и неудавшиеся объекты исключены. Запас распределяется один раз между потребностями этой колонии; между разными колониями он не резервируется. ')+'</p></details></section>';
}
function warehouseOptions(C,u){
 const colony=C.state?.colonization||{},r=state(C),enabled=colony.includeCarrierWarehouse===true,canConfirm=!!stockAccessScope(C);
 return '<div class="colony-warehouse-options" role="group" aria-label="'+th('Учёт склада флотоносца')+'"><label class="route-clipboard-option"><input id="colony-include-carrier-stock" type="checkbox" '+(enabled?'checked ':'')+(r.warehousePending?'disabled ':'')+'><span>'+th('Учитывать склад флотоносца')+'<small>'+th('Прочитанный запас флотоносца уменьшает закупку. Между колониями груз не резервируется.')+'</small></span></label>'+(enabled?'<label class="route-clipboard-option"><input id="colony-squadron-stock-access" type="checkbox" '+(colony.squadronWarehouseAccessConfirmed?'checked ':'')+(r.warehousePending||!canConfirm&&!colony.squadronWarehouseAccessConfirmed?'disabled ':'')+'><span>'+th('Доступен груз эскадры')+'<small>'+th(canConfirm?'Включите, если можете забирать груз эскадры.':'После входа в Frontier данные флотоносца эскадры обновляются автоматически.')+'</small></span></label>':'')+'<div class="colony-source-note">'+(colony.carrierWarehouses||[]).map(item=>'<p><strong>'+th(item.kind==='squadron'?'Флотоносец эскадрильи':'Личный флотоносец')+(item.callsign?' · '+u.esc(item.callsign):'')+'</strong> · '+u.esc(format(item.knownTons,u,t(' т')))+(item.included?' · '+th(item.known?'Учтён в плане':'Ожидает данных'):' · '+th('Не учтён в плане'))+warehouseTime(item,u)+'<br>'+u.esc(t(item.reason||''))+'</p>').join('')+'</div>'+(r.warehouseError?'<p role="alert" class="colony-search-error">'+u.esc(r.warehouseError)+'</p>':'')+'</div>';
}
function budgetContext(C,u){
 const project=C.state?.colonization?.activeProject;
 if(!project?.id)return summary(C,u);
 return '<header class="colony-budget-context" data-colony-id="'+u.esc(project.id)+'"><div><span class="eyebrow">'+th('Выбранная колония')+'</span><h2>'+u.esc(project.name||project.targetSystem)+'</h2><div class="colony-overview-meta">'+(!same(project.name,project.targetSystem)?'<span>'+u.icon('orbit')+u.esc(project.targetSystem||t('Система не задана'))+'</span>':'')+u.badge(t(statusNames[project.status]||'Статус не задан'))+'</div></div><div class="colony-overview-actions">'+(project.targetSystem?u.btn(t('Копировать систему'),'colony-workspace-copy','copy','quiet','type="button" data-project="'+u.esc(project.id)+'"'):'')+'</div></header>';
}
function render(C,u){
 if(!active(C))return '';
 if(C.mode==='Бюджет')return '<div data-colony-workspace class="colony-budget-workspace">'+budgetContext(C,u)+budget(C,u)+'<details class="colony-budget-stock"><summary>'+th('Запасы и учёт')+'</summary>'+warehouseOptions(C,u)+'</details></div>';
 if(selectionMode(C)&&C.mode)return '<div data-colony-workspace>'+originSearch(C,u)+action(C,u,'colony.create',t('Добавить колонию вручную'),'plus')+'</div>';
 if(!C.mode||C.mode==='Обзор'){
  const colony=C.state?.colonization||{},projects=colony.projects?.length?colony.projects:colony.activeProject?.id?[colony.activeProject]:[];
  return '<div data-colony-workspace class="colony-overview-list">'+(projects.length?projects.map(project=>summary(C,u,project)).join(''):summary(C,u,null))+
   '<div class="colony-overview-actions">'+u.btn(t('Выбрать систему'),'colony-workspace-new','search','quiet')+(projects.length?action(C,u,'colony.create',t('Добавить колонию вручную'),'plus'):'')+'</div></div>';
 }
 return '';
}
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
 if(!active(C))return false;
 const target=event.target;
 if(['colony-include-carrier-stock','colony-squadron-stock-access'].includes(target?.id)){
  const r=state(C);if(r.warehousePending)return true;
  if(target.id==='colony-squadron-stock-access'&&target.checked&&!stockAccessScope(C))return true;
  const colony=C.state?.colonization||{},values={includeCarrierWarehouse:colony.includeCarrierWarehouse===true,squadronWarehouseAccessConfirmed:colony.squadronWarehouseAccessConfirmed===true};
  values[target.id==='colony-include-carrier-stock'?'includeCarrierWarehouse':'squadronWarehouseAccessConfirmed']=target.checked===true;
  if(values.squadronWarehouseAccessConfirmed&&stockAccessScope(C))values.squadronWarehouseAccessScope=stockAccessScope(C);
  r.warehousePending=true;r.warehouseError='';u.renderPage();
  r.warehouseSave=(async()=>{
   try{await u.call('action',{...(u.context?.()||{}),tab:'colonization',actionId:'colony.carrier-stock',values});Object.assign(colony,values);if(active(C))await u.loadView();}
   catch(error){r.warehouseError=error?.message||t('Не удалось сохранить учёт склада. Повторите действие.');}
   finally{r.warehousePending=false;if(active(C)){u.renderPage();u.$('#'+target.id)?.focus?.({preventScroll:true});}}
  })();return true;
 }
 if(target?.id==='colony-workspace-origin'&&selectionMode(C)){queueSearch(C,u,'colony-workspace-check');return true;}
 if(target?.id!=='colony-workspace-station')return false;
 const {r,stations}=source(C),station=stations.find(item=>stationKey(item)===event.target.value);
 r.station=station?stationKey(station):'';r.faction=station?.faction||'';r.error='';
 u.renderPage();u.$('#colony-workspace-station')?.focus?.({preventScroll:true});
 if(station)queueSearch(C,u,'colony-workspace-find');return true;
}

function renderBuilding(C,u){
 const project=C.state?.colonization?.activeProject, building=project?.buildings?.find(item=>item.id===C.selected);
 if(!building)return '';
 const requirements=Array.isArray(building.requirements)?building.requirements:[];
 if(!requirements.length)return '';
 const total=requirements.reduce((n,row)=>n+(known(row.required)?Number(row.required):0),0);
 const delivered=requirements.reduce((n,row)=>n+(known(row.required)&&known(row.delivered)?Math.min(Number(row.required),Number(row.delivered)):0),0);
 const complete=total>0&&requirements.every(row=>known(row.required)&&known(row.delivered));
 const headings=['Товар','Нужно, т','Сдано, т','Осталось, т'];
 return '<section class="panel colony-building-cost"><header><h2>'+u.esc(building.name||building.typeName||t('Строительство'))+'</h2>'+
  (complete?'<div class="colony-building-progress"><progress max="'+u.esc(total)+'" value="'+u.esc(delivered)+'" aria-label="'+th('Ход поставок')+'"></progress><span>'+u.esc(u.fmt(delivered))+' / '+u.esc(u.fmt(total))+th(' т')+'</span></div>':'')+'</header>'+
  '<div class="table-scroll"><table><thead><tr>'+headings.map(h=>'<th>'+th(h)+'</th>').join('')+'</tr></thead><tbody>'+requirements.map(row=>'<tr><td>'+u.esc(row.name||row.commodity)+'</td>'+[row.required,row.delivered,row.missing].map(v=>'<td>'+u.esc(format(v,u))+'</td>').join('')+'</tr>').join('')+'</tbody></table></div></section>';
}
async function handleClick(action,button,C,u){
 if(!active(C)||!action?.startsWith('colony-workspace-'))return false;
 if(action==='colony-workspace-new'){await u.navigate('colonization',{mode:'Выбор системы'});return true;}
 if(action==='colony-workspace-open'){
  const id=button.dataset.project;
  if(id&&id!==C.state?.colonization?.selectedProjectId)await u.call('select-colony',{...u.context?.(),id});
  if(active(C))await u.navigate('colonization',{mode:'Строительство'});
  return true;
 }
 if(action==='colony-workspace-copy'){
  const colony=C.state?.colonization||{},id=button?.dataset?.project;
  const project=id?(colony.projects||[]).find(item=>item.id===id)||((colony.activeProject?.id===id)?colony.activeProject:null):colony.activeProject;
  const system=project?.targetSystem;
  if(system){await u.call('native',{action:'copy',text:system});u.toast(t("Система колонии скопирована"));}return true;
 }
 if(!['colony-workspace-check','colony-workspace-find'].includes(action))return false;
 const {r,search,ready,selected}=source(C);
 if(r.pending||search.running)return true;
 if(r.queuedSearch?.action===action)r.queuedSearch=null;
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
function queueSearch(C,u,action){
 if(window.COPILOT_SMOKE||typeof window.setTimeout!=='function')return;
 const r=state(C),origin=r.origin.trim();if(!origin)return;
 r.queuedSearch={action,origin,station:r.station};
 flushSearch(C,u);
}
function flushSearch(C,u){
 const {r,ready,search}=source(C),pending=r.queuedSearch;
 if(!pending||r.autoTimer||r.pending||search.running)return;
 const valid=()=>active(C)&&selectionMode(C)&&same(pending.origin,r.origin)&&(pending.action!=='colony-workspace-find'||pending.station===r.station);
 if(!valid()||pending.action==='colony-workspace-check'&&ready){r.queuedSearch=null;return;}
 r.autoTimer=window.setTimeout(()=>{
  r.autoTimer=null;
  if(r.queuedSearch!==pending){flushSearch(C,u);return;}
  if(!valid()){r.queuedSearch=null;return;}
  if(r.pending||source(C).search.running)return;
  r.queuedSearch=null;void handleClick(pending.action,null,C,u);
 },250);
}
function init(C,u){
 if(!active(C))return;
 const r=state(C),{stations}=source(C);
 if(r.station&&!stations.some(item=>stationKey(item)===r.station)){r.station='';r.faction='';}
 if(!realFaction(r.faction))r.faction='';
 if(selectionMode(C)&&C.mode&&r.autoOrigin!==r.origin&&r.origin){r.autoOrigin=r.origin;queueSearch(C,u,'colony-workspace-check');}
 if(selectionMode(C))flushSearch(C,u);
}
window.ColonyWorkspaceUI={active,render,renderBuilding,init,handleClick,handleInput,handleChange};
})();
