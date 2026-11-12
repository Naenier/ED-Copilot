(()=>{
'use strict';
const t=(value,vars)=>window.CopilotI18n?.t(value,vars)??String(value??'');
const th=(value,vars)=>t(value,vars).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const same=(a,b)=>String(a||'').trim().toLocaleLowerCase('ru-RU')===String(b||'').trim().toLocaleLowerCase('ru-RU');
const known=value=>value!==null&&value!==undefined&&value!==''&&Number.isFinite(Number(value));
const statusNames={planned:'Личный план',claimed:'Заявка подана',building:'Строительство',complete:'Завершена',cancelled:'Отменена'};
const developmentLabels={initialPopulation:'Индекс начального населения',maxPopulation:'Индекс предельного населения',security:'Безопасность',technology:'Технологический уровень',wealth:'Благосостояние',livingStandards:'Уровень жизни',development:'Уровень развития',architectScore:'Баллы системы'};
const developmentEconomies={colony:'Колониальная',hightech:'Высокие технологии',refinery:'Переработка',industrial:'Промышленность',extraction:'Добыча',agriculture:'Сельское хозяйство',military:'Военная',tourism:'Туризм',service:'Услуги',contraband:'Контрабанда',mixed:'Смешанная',terraforming:'Терраформирование'};
const developmentUnlocks={'human-broker':'Технологический брокер людей','tourism-settlements':'Туристические поселения','survey-linked':'Universal Cartographics и Vista Genomics при сильной связи','exploration-hub':'Исследовательский узел','market-linked':'Рынок на подходящих аванпостах при сильной связи','security-installation':'Орбитальная станция безопасности','outpost-hub':'Опорный узел','industrial-hub':'Промышленный узел','black-market-linked':'Чёрный рынок при сильной связи','lounge-system':'Зал экипажа на подходящих портах T1','market-system':'Рынок на подходящих аванпостах системы','military-hub':'Военный узел','shipyard-system':'Верфь на подходящих планетарных портах T1','outfitting-system':'Оснащение на подходящих портах системы','vista-system':'Vista Genomics на подходящих портах','cartographics-system':'Universal Cartographics на подходящих портах','shipyard-linked':'Верфь на подходящих портах T1 при сильной связи','outfitting-linked':'Оснащение на подходящих портах при сильной связи','pioneer-supplies':'Магазин Pioneer Supplies','market-port':'Товарный рынок','technology-floor':'Минимальный технологический уровень системы 35','military-installation':'Орбитальная военная установка','research-installation':'Орбитальная исследовательская станция','tourist-installation':'Орбитальная туристическая установка'};
const settlementPrerequisites={'settlement-agriculture':'Поселение сельскохозяйственного профиля','settlement-extraction':'Поселение добывающего профиля','settlement-military':'Поселение военного профиля','settlement-hightech':'Поселение биологических исследований','settlement-tourism':'Поселение туристического профиля'};
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
function metric(label,value,detail,u){return '<div class="colony-summary-metric"><span>'+u.esc(label)+'</span><strong>'+u.esc(value)+'</strong>'+(detail?'<small>'+u.esc(detail)+'</small>':'')+'</div>';}
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
function budgetPriceDetails(C,cost,u){
 const freshness=window.FreshnessUI;
 if(!freshness||!cost.commodityCount)return '';
 const missing=Math.max(0,Number(cost.commodityCount)-(Number(cost.pricedCommodityCount)||0));
 const language=C.state?.settings?.language==='ru'?'ru':'en';
 const lines=[];
 if(missing)lines.push([t('Товаров без средней цены'),u.fmt(missing)]);
 const oldest=freshness.formatDate(cost.oldestPriceAt,language);
 if(oldest)lines.push([t('Дата самой старой цены'),oldest]);
 return lines.length?freshness.details(t(missing?'Неполная оценка':'Даты цен'),lines,u.esc,missing>0):'';
}
function budget(C,u){
 const project=C.state?.colonization?.activeProject,cost=project?.costSummary;
 if(!project?.id||!cost)return '';
 const unknownBills=Number(cost.unknownBills)||0,manualBills=Number(cost.manualBills)||0;
 const warnings=[];
 if(unknownBills)warnings.push(t('Неизвестных смет: ')+u.fmt(unknownBills));
 if(manualBills)warnings.push(t('Ручных смет: ')+u.fmt(manualBills));
 const covered=[metric(t('Принято стройками'),budgetValue(cost.delivered,u),t('Стоимость уже принятых ресурсов по тем же ценам'),u),metric(t('Покрыто грузом корабля'),budgetValue(cost.cargoCovered,u),t('Груз ещё нужно передать на стройку'),u),metric(t('Покрыто складом флотоносца'),budgetValue(cost.carrierCovered,u),t('В пределах потребности и включённого учёта'),u)].join('');
 return '<section class="colony-budget" data-colony-budget="'+u.esc(project.id)+'"><header><div><h3>'+th('Бюджет ресурсов колонии')+'</h3></div><span class="colony-budget-scope">'+th('В плане: ')+u.esc(u.fmt(cost.plannedBuildings||0))+' · '+th('Строятся: ')+u.esc(u.fmt(cost.buildingBuildings||0))+'</span></header><div class="colony-budget-basis"><span>'+th('По средним ценам галактики')+'</span>'+budgetPriceDetails(C,cost,u)+'</div><div class="colony-budget-metrics">'+metric(t('Полная смета ресурсов'),budgetValue(cost.required,u),t('Все ресурсы запланированных и строящихся объектов'),u)+metric(t('Осталось доставить'),budgetValue(cost.missing,u),t('Стоимость ресурсов, ещё не принятых стройками'),u)+metric(t('Нужно закупить'),budgetValue(cost.toBuy,u,cost.toBuyAtMost),t('После учёта корабля и включённых складов'),u)+'</div>'+(warnings.length?'<p class="colony-budget-warning" role="status">'+u.esc(warnings.join(' · '))+'</p>':'')+'<details class="colony-budget-breakdown"><summary>'+th('Из чего складывается бюджет')+'</summary><div class="colony-budget-metrics">'+covered+'</div><p>'+th('Учитываются запланированные и строящиеся объекты этой колонии.')+'</p></details></section>';
}
function warehouseOptions(C,u){
 const colony=C.state?.colonization||{},r=state(C),enabled=colony.includeCarrierWarehouse===true,canConfirm=!!stockAccessScope(C);
 return '<div class="colony-warehouse-options" role="group" aria-label="'+th('Учёт склада флотоносца')+'"><label class="route-clipboard-option"><input id="colony-include-carrier-stock" type="checkbox" '+(enabled?'checked ':'')+(r.warehousePending?'disabled ':'')+'><span>'+th('Учитывать склад флотоносца')+'</span></label>'+(enabled?'<label class="route-clipboard-option"><input id="colony-squadron-stock-access" type="checkbox" '+(colony.squadronWarehouseAccessConfirmed?'checked ':'')+(r.warehousePending||!canConfirm&&!colony.squadronWarehouseAccessConfirmed?'disabled ':'')+'><span>'+th('Доступен груз эскадры')+'<small>'+th(canConfirm?'Включите, если можете забирать груз эскадры.':'После входа в Frontier данные флотоносца эскадры обновляются автоматически.')+'</small></span></label>':'')+'<div class="colony-source-note">'+(colony.carrierWarehouses||[]).map(item=>'<p><strong>'+th(item.kind==='squadron'?'Флотоносец эскадрильи':'Личный флотоносец')+(item.callsign?' · '+u.esc(item.callsign):'')+'</strong> · '+u.esc(format(item.knownTons,u,t(' т')))+(item.included?' · '+th(item.known?'Учтён в плане':'Ожидает данных'):' · '+th('Не учтён в плане'))+warehouseTime(item,u)+'<br>'+u.esc(t(item.reason||''))+'</p>').join('')+'</div>'+(r.warehouseError?'<p role="alert" class="colony-search-error">'+u.esc(r.warehouseError)+'</p>':'')+'</div>';
}
function budgetContext(C,u){
 const project=C.state?.colonization?.activeProject;
 if(!project?.id)return summary(C,u);
 return '<header class="colony-budget-context" data-colony-id="'+u.esc(project.id)+'"><div><span class="eyebrow">'+th('Выбранная колония')+'</span><h2>'+u.esc(project.name||project.targetSystem)+'</h2><div class="colony-overview-meta">'+(!same(project.name,project.targetSystem)?'<span>'+u.icon('orbit')+u.esc(project.targetSystem||t('Система не задана'))+'</span>':'')+u.badge(t(statusNames[project.status]||'Статус не задан'))+'</div></div><div class="colony-overview-actions">'+(project.targetSystem?u.btn(t('Копировать систему'),'colony-workspace-copy','copy','quiet','type="button" data-project="'+u.esc(project.id)+'"'):'')+'</div></header>';
}
function portfolio(C){
 const workspace=C.state?.planWorkspace?.key;
 const r=state(C);
 if(!r.portfolio||r.portfolio.workspace!==workspace){
  r.portfolio={workspace,open:false,excludedProjects:new Set(),excludedBuildings:new Set(),request:0};
 }
 return r.portfolio;
}
function portfolioSelection(C,p){
 const colony=C.state?.colonization||{};
 const projects=(colony.projects?.length?colony.projects:[colony.activeProject])
  .filter(project=>project?.id&&project.status!=='cancelled')
  .map(project=>({project,buildings:(project.buildings||[]).filter(building=>
   building.id&&['planned','building'].includes(building.effectiveStatus||building.status))}))
  .filter(group=>group.buildings.length);
 const included=projects.filter(({project})=>!p.excludedProjects.has(project.id));
 const buildings=included.flatMap(group=>group.buildings);
 const chosen=buildings.filter(building=>!p.excludedBuildings.has(building.id));
 return {projects,included,buildings,chosen};
}
function portfolioControl(u,kind,index,id,label,checked){
 const inputID='colony-portfolio-'+kind+'-'+index;
 return '<label><input id="'+inputID+'" type="checkbox" data-portfolio-'+kind+'="'+u.esc(id)+'" '+(checked?'checked':'')+'><span>'+u.esc(label)+'</span></label>';
}
function portfolioFilters(C,u,p,selection=portfolioSelection(C,p)){
 const projects=selection.projects.map(({project},index)=>portfolioControl(u,'project',index,project.id,project.name||project.targetSystem,!p.excludedProjects.has(project.id))).join('');
 const groups=selection.included.map(({project,buildings},index)=>'<fieldset><legend>'+u.esc(project.name||project.targetSystem)+'</legend><div class="colony-portfolio-options">'+buildings.map((building,position)=>
  portfolioControl(u,'building',index+'-'+position,building.id,building.name||building.displayName,!p.excludedBuildings.has(building.id))).join('')+'</div></fieldset>').join('');
 return '<fieldset><legend>'+th('Колонии')+'</legend><div class="colony-portfolio-options">'+projects+'</div></fieldset>'+
  (groups?'<details class="colony-portfolio-buildings" '+(p.showBuildings?'open':'')+'><summary>'+th('Объекты строительства')+' · '+u.esc(u.fmt(selection.chosen.length))+' / '+u.esc(u.fmt(selection.buildings.length))+'</summary>'+groups+'</details>':'');
}
function portfolioQuantity(value,u,prefix=''){
 return known(value)?prefix+u.fmt(Number(value))+t(' т'):t('Неизвестно');
}
function portfolioBound(exact,lower,upper,u){
 if(known(exact))return portfolioQuantity(exact,u);
 if(known(upper))return portfolioQuantity(upper,u,t('Не более '));
 return portfolioQuantity(lower>0?lower:null,u,t('Не менее '));
}
const reservationSources={ship:'Груз корабля',fleet:'Личный флотоносец',squadron:'Флотоносец эскадрильи'};
const reservationStatus={active:'Зарезервирован',fulfilled:'Доставлен',released:'Освобождён',cancelled:'Отменён'};
const reservationKey=row=>JSON.stringify([row.projectId,row.buildingId,row.commodity]);
function reservationChoices(p){
 return (p.result?.reservable||[]).filter(row=>row.sources?.some(source=>Number(source.available)>=1));
}
function reservationControls(p,u){
 const choices=reservationChoices(p);
 if(!choices.length)return '';
 const selected=choices.find(row=>reservationKey(row)===p.reservationKey)||choices[0];
 const sources=selected.sources.filter(source=>Number(source.available)>=1);
 const current=sources.find(source=>source.source===p.reservationSource)||sources[0];
 const maximum=Math.floor(Number(current.available));
 const options=choices.map(row=>'<option value="'+u.esc(reservationKey(row))+'" '+(row===selected?'selected':'')+'>'+u.esc(row.buildingName+' · '+t(row.name||row.commodity))+'</option>').join('');
 const stocks=sources.map(source=>'<option value="'+u.esc(source.source)+'" '+(source===current?'selected':'')+'>'+th(reservationSources[source.source]||source.source)+' · '+u.esc(portfolioQuantity(source.available,u))+'</option>').join('');
 const field=(label,control)=>'<label><span>'+th(label)+'</span>'+control+'</label>';
 return field('Стройка и товар','<select id="colony-reservation-target" name="target">'+options+'</select>')+
  field('Запас','<select id="colony-reservation-source" name="source">'+stocks+'</select>')+
  field('Количество, т','<input id="colony-reservation-count" name="count" type="number" required min="1" step="1" max="'+maximum+'" value="'+maximum+'">')+
  '<button type="submit" class="pill quiet">'+th('Зарезервировать')+'</button>';
}
function renderReservations(p,u){
 if(!p.workspace)return '';
 const history=p.result?.reservations||[],controls=reservationControls(p,u);
 if(!history.length&&!controls)return '';
 const rows=history.map(row=>{
  const details=' · '+row.buildingName+' · '+t(row.name||row.commodity),data='data-reservation-id="'+u.esc(row.id)+'"';
  const transitLabel=row.inTransit?'Вернуть в резерв':'Отметить в пути';
  const transit=row.status==='active'?u.btn(t(transitLabel),'colony-workspace-reservation-transit','route','quiet',data+' id="colony-transit-'+u.esc(row.id)+'" aria-label="'+u.esc(t(transitLabel)+details)+'"'):'';
  const cancel=row.status==='active'?u.btn(t('Снять резерв'),'colony-workspace-reservation-cancel','x','quiet',data+' aria-label="'+u.esc(t('Снять резерв')+details)+'"'):'';
  const destination=row.targetSystem?'<small>'+u.esc(row.targetSystem)+'</small>':'';
  const status=row.inTransit?'В пути':reservationStatus[row.status]||row.status;
  return '<tr><th scope="row" class="colony-reservation-destination">'+u.esc(row.buildingName)+destination+'</th><td>'+u.esc(t(row.name||row.commodity))+'</td><td>'+th(reservationSources[row.source]||row.source)+'</td><td>'+
   u.esc(portfolioQuantity(row.remaining,u))+'</td><td>'+u.esc(portfolioQuantity(row.fulfilled,u))+'</td><td>'+th(status)+'</td><td><div class="colony-reservation-actions">'+transit+cancel+'</div></td></tr>';
 }).join('');
 const headings=['Стройка','Товар','Запас','Зарезервировано, т','Доставлено, т','Статус'];
 return '<details class="colony-portfolio-reservations" id="colony-portfolio-reservations" '+(p.showReservations?'open':'')+'><summary>'+th('Резервы')+'</summary>'+
  (controls?'<form id="colony-reservation-form">'+controls+'</form>':'')+
  (rows?'<div class="colony-portfolio-table"><table><thead><tr>'+headings.map(heading=>'<th scope="col">'+th(heading)+'</th>').join('')+'<th scope="col"><span class="visually-hidden">'+th('Действие')+'</span></th></tr></thead><tbody>'+rows+'</tbody></table></div>':'')+'</details>';
}
function portfolioPurchaseQuantity(row){
 return Math.ceil(known(row?.toBuy)?Number(row.toBuy):Number(row?.toBuyAtMost)||0);
}
function portfolioResults(C,u,p,selection=portfolioSelection(C,p)){
 if(!selection.chosen.length)return '<p class="colony-portfolio-status" role="status">'+th(selection.included.length?'Выберите объекты строительства.':'Выберите колонии.')+'</p>';
 if(p.error)return '<p class="colony-portfolio-status" role="alert">'+u.esc(p.error)+'</p>'+u.btn(t('Повторить'),'colony-workspace-portfolio-retry','refresh-cw','quiet','type="button"');
 if(!p.result)return '<p class="colony-portfolio-status" role="status">'+th('Рассчитываем снабжение…')+'</p>';
 const {supplies=[],buildingCount,unknownBills,projectCount}=p.result;
 const warehouse=C.state?.colonization?.includeCarrierWarehouse===true;
 const rows=supplies.filter(row=>Number(row.missing)>0);
 let missing=0,covered=0,toBuy=0,toBuyAtMost=0;
 let coverageComplete=!unknownBills,buyComplete=!unknownBills,buyBoundComplete=!unknownBills;
 for(const row of rows){
  const demand=Number(row.missing),ship=Number(row.cargoAvailable)||0,carrier=warehouse?(Number(row.carrierAvailable)||0):0;
  missing+=demand;
  covered+=Math.min(demand,ship+carrier);
  coverageComplete&&=ship+carrier>=demand||known(row.cargoAvailable)&&(!warehouse||row.carrierComplete===true);
  const upper=known(row.toBuy)?row.toBuy:row.toBuyAtMost;
  buyComplete&&=known(row.toBuy);
  buyBoundComplete&&=known(upper);
  toBuy+=Number(row.toBuy)||0;
  toBuyAtMost+=Number(upper)||0;
 }
 const totals=metric(t('Осталось доставить'),portfolioBound(unknownBills?null:missing,missing,null,u),'',u)+
  metric(t('Покрыто запасами'),portfolioBound(coverageComplete?covered:null,covered,null,u),'',u)+
  metric(t('Закупить'),portfolioBound(buyComplete?toBuy:null,toBuy,buyBoundComplete?toBuyAtMost:null,u),'',u);
 const showReservations=rows.some(row=>Number(row.reservedHere)>0||Number(row.reservedElsewhere)>0);
 const showTransit=rows.some(row=>Number(row.inTransit)>0);
 const headings=['Товар','Осталось, т','В корабле, т',...(warehouse?['На флотоносце, т']:[]),
  ...(showReservations?['Зарезервировано здесь, т','В резерве других строек, т']:[]),...(showTransit?['Из них в пути, т']:[]),'Закупить, т'];
 const cell=value=>'<td>'+u.esc(value)+'</td>';
 const table=rows.length?'<div class="colony-portfolio-table"><table><thead><tr>'+headings.map(heading=>'<th scope="col">'+th(heading)+'</th>').join('')+'</tr></thead><tbody>'+rows.map(row=>{
  const available=warehouse?(row.carrierComplete===true?portfolioQuantity(row.carrierAvailable,u):portfolioBound(null,Number(row.carrierAvailable)||0,null,u)):'';
  const reserves=showReservations?cell(portfolioQuantity(row.reservedHere,u))+cell(portfolioQuantity(row.reservedElsewhere,u)):'';
  const market=portfolioPurchaseQuantity(row)>0?u.btn(t('Где купить'),'colony-workspace-portfolio-market','search','quiet','data-commodity="'+u.esc(row.commodity)+'" aria-label="'+u.esc(t('Где купить')+' · '+t(row.name||row.commodity))+'"'):'';
  return '<tr><th scope="row">'+u.esc(t(row.name||row.commodity))+'</th>'+cell(portfolioQuantity(row.missing,u))+cell(portfolioQuantity(row.cargoAvailable,u))+
   (warehouse?cell(available):'')+reserves+(showTransit?cell(portfolioQuantity(row.inTransit,u)):'')+'<td>'+u.esc(portfolioBound(row.toBuy,null,row.toBuyAtMost,u))+market+'</td></tr>';
 }).join('')+'</tbody></table></div>':'';
 return '<div class="colony-portfolio-counts"><span>'+th('Колоний: ')+u.esc(u.fmt(projectCount))+' · '+th('Объектов: ')+u.esc(u.fmt(buildingCount))+'</span>'+
  (unknownBills?'<span class="colony-budget-warning">'+th('Неизвестных смет: ')+u.esc(u.fmt(unknownBills))+'</span>':'')+'</div>'+
  '<div class="colony-budget-metrics">'+totals+'</div>'+table+renderReservations(p,u);
}
function renderPortfolio(C,u){
 const p=portfolio(C),selection=portfolioSelection(C,p);
 if(!selection.projects.length)return '';
 return '<details class="colony-budget-stock colony-portfolio" id="colony-portfolio" '+(p.open?'open':'')+'><summary>'+th('Сводное снабжение')+'</summary>'+
  '<div class="colony-portfolio-body"><p class="colony-portfolio-note">'+th('Общий запас учитывается один раз.')+'</p>'+
  '<div id="colony-portfolio-filters">'+(p.open?portfolioFilters(C,u,p,selection):'')+'</div>'+
  '<div id="colony-portfolio-results">'+(p.open?portfolioResults(C,u,p,selection):'')+'</div></div></details>';
}
function refreshPortfolio(C,u,p,force=false){
 if(!p.open||!active(C)||C.mode!=='Бюджет')return;
 const {included,buildings,chosen}=portfolioSelection(C,p),colony=C.state?.colonization||{};
 const projectIds=included.filter(group=>group.buildings.some(building=>!p.excludedBuildings.has(building.id))).map(({project})=>project.id);
 const buildingIds=chosen.length===buildings.length?undefined:chosen.map(building=>building.id);
 const signature=JSON.stringify([projectIds,buildingIds,C.state?.settings?.language,colony.cargoKnown,colony.includeCarrierWarehouse,colony.carrierWarehouseComplete,
  chosen.map(building=>[building.id,(building.requirements||[]).map(row=>[row.commodity,row.name,row.missing,row.cargoAvailable,row.carrierAvailable])])]);
 if(!force&&p.signature===signature)return;
 p.signature=signature;p.error='';p.result=null;
 const request=++p.request;
 const show=()=>drawPortfolioResults(C,u,p);
 show();
 if(!chosen.length)return;
 const params={projectIds};
 if(buildingIds)params.buildingIds=buildingIds;
 if(typeof p.workspace==='string')params.expectedPlanWorkspace=p.workspace;
 const current=()=>p===portfolio(C)&&p.open&&p.request===request;
 p.pending=(async()=>{
  try{const result=await u.call('colony.supply-portfolio',params);if(current())p.result=result;}
  catch(error){if(current())p.error=t(error?.message||'Не удалось рассчитать снабжение.');}
  finally{if(current()&&active(C)&&C.mode==='Бюджет')show();}
 })();
}
function trackPortfolioBuildingDisclosure(host,p){
 const details=host?.querySelector('.colony-portfolio-buildings');
 if(details)details.ontoggle=()=>{p.showBuildings=details.open;};
}
function trackPortfolioReservations(u,p){
 const details=u.$('#colony-portfolio-reservations');
 if(details)details.ontoggle=()=>{if(details===u.$('#colony-portfolio-reservations'))p.showReservations=details.open;};
}
function drawPortfolioResults(C,u,p){
 const host=u.$('#colony-portfolio-results');if(!host)return;
 const focus=host.ownerDocument?.activeElement;
 const keepFocus=host.contains?.(focus);
 host.innerHTML=portfolioResults(C,u,p);
 trackPortfolioReservations(u,p);
 if(keepFocus){
  const restored=focus.id&&host.ownerDocument.getElementById(focus.id);
  const summary=u.$('#colony-portfolio-reservations summary')||u.$('#colony-portfolio > summary');
  (restored||summary)?.focus({preventScroll:true});
 }
}
async function updateReservation(C,u,p,params,button){
 if(!p.workspace||p.reservationPending)return;
 p.reservationPending=true;if(button)button.disabled=true;
 try{
  await u.call('colony.supply-reservation',{...params,expectedPlanWorkspace:p.workspace});
  if(p===portfolio(C)&&active(C)&&C.mode==='Бюджет'){
   refreshPortfolio(C,u,p,true);
   const message=params.action==='transit'?(params.inTransit?'Партия в пути':'Партия возвращена в резерв'):params.action==='reserve'?'Резерв сохранён':'Резерв снят';
   u.toast(t(message));
  }
 }catch(error){if(p===portfolio(C))u.toast(t(error?.message||'Не удалось изменить резерв.'),true);}
 finally{p.reservationPending=false;if(button)button.disabled=false;}
}
async function submitReservation(form,C,u){
 if(!active(C)||C.mode!=='Бюджет')return;
 const p=portfolio(C),target=form.elements.target.value,source=form.elements.source.value,count=Number(form.elements.count.value);
 const row=reservationChoices(p).find(item=>reservationKey(item)===target);
 const available=row?.sources.find(item=>item.source===source)?.available;
 if(!Number.isInteger(count)||count<1||!known(available)||count>Number(available))return;
 await updateReservation(C,u,p,{action:'reserve',projectId:row.projectId,buildingId:row.buildingId,commodity:row.commodity,source,count},form.querySelector('[type=submit]'));
}
function searchStatus(C){
 const {r,search}=source(C);
 let message='';
 if(r.pending||search.running)message='Ищем системы для колонизации';
 else if(search.status==='cancelled')message='Поиск отменён.';
 else if(search.status==='complete'&&search.completeness?.complete!==true)message='Поиск завершён не полностью. Повторите поиск.';
 else if(search.status==='complete'&&!C.page?.Rows?.length){
  const filtered=C.query?.trim()&&search.results?.some(result=>result.availability==='candidate');
  message=filtered?'Нет совпадений':'Подходящих систем в радиусе 15 св. л. не найдено.';
 }
 return message?`<p class="colony-search-status" role="status">${th(message)}</p>`:'';
}
function render(C,u){
 if(!active(C))return '';
 if(C.mode==='Бюджет')return '<div data-colony-workspace class="colony-budget-workspace">'+budgetContext(C,u)+budget(C,u)+'<details class="colony-budget-stock"><summary>'+th('Запасы и учёт')+'</summary>'+warehouseOptions(C,u)+'</details>'+renderPortfolio(C,u)+'</div>';
 if(selectionMode(C)&&C.mode)return '<div data-colony-workspace>'+originSearch(C,u)+action(C,u,'colony.create',t('Добавить колонию вручную'),'plus')+searchStatus(C)+'</div>';
 if(!C.mode||C.mode==='Обзор'){
  const colony=C.state?.colonization||{},projects=colony.projects?.length?colony.projects:colony.activeProject?.id?[colony.activeProject]:[];
  return '<div data-colony-workspace class="colony-overview-list">'+(projects.length?projects.map(project=>summary(C,u,project)+renderDevelopment(project,u,true)).join(''):summary(C,u,null))+
   '<div class="colony-overview-actions">'+u.btn(t('Выбрать систему'),'colony-workspace-new','search','quiet')+(projects.length?action(C,u,'colony.create',t('Добавить колонию вручную'),'plus'):'')+'</div></div>';
 }
 if(C.mode==='Строительство')return renderDevelopment(C.state?.colonization?.activeProject,u,false);
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
 if(C.mode==='Бюджет'&&['colony-reservation-target','colony-reservation-source'].includes(target?.id)){
  const p=portfolio(C),form=target.form;
  if(target.id==='colony-reservation-target'){p.reservationKey=target.value;p.reservationSource='';}
  else p.reservationSource=target.value;
  if(form){form.innerHTML=reservationControls(p,u);u.$('#'+target.id)?.focus({preventScroll:true});}
  return true;
 }
 const {portfolioProject,portfolioBuilding}=target?.dataset||{};
 if(C.mode==='Бюджет'&&(portfolioProject||portfolioBuilding)){
  const p=portfolio(C),exclusions=portfolioProject?p.excludedProjects:p.excludedBuildings,id=portfolioProject||portfolioBuilding;
  if(target.checked)exclusions.delete(id);else exclusions.add(id);
  const filters=u.$('#colony-portfolio-filters');
  if(filters&&p.open){
   p.showBuildings=filters.querySelector('.colony-portfolio-buildings')?.open===true;
   filters.innerHTML=portfolioFilters(C,u,p);
   trackPortfolioBuildingDisclosure(filters,p);
   filters.querySelector('#'+target.id)?.focus({preventScroll:true});
  }
  refreshPortfolio(C,u,p);return true;
 }
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

function signed(value,u){
 if(!known(value))return '—';
 return (Number(value)>0?'+':'')+u.fmt(Number(value));
}
function renderDevelopment(project,u,collapsed){
 const d=project?.development;
 if(!d?.metrics?.length||!d.builtCount&&!d.plannedCount)return '';
 const val=(value,count,total,delta=false)=>total>0&&count===0?'—':(delta?signed(value,u):u.fmt(Number(value)))+(count<total?' *':'');
 const metrics=d.metrics.filter(item=>Object.hasOwn(developmentLabels,item.key)).map(item=>{
  const built=val(item.built,item.builtKnown,d.builtCount),planned=val(item.planned,item.plannedKnown,d.plannedCount,true);
  const total=val(item.total,item.builtKnown+item.plannedKnown,d.builtCount+d.plannedCount);
  return '<tr><th scope="row">'+th(developmentLabels[item.key])+'</th><td>'+u.esc(built)+'</td><td>'+u.esc(planned)+'</td><td>'+u.esc(total)+'</td></tr>';
 }).join('');
 const incomplete=d.metrics.some(item=>item.builtKnown<d.builtCount||item.plannedKnown<d.plannedCount);
 const floor=d.plannedTechFloor?'<p class="colony-impact-note">'+th(d.builtTechFloor?'Порт T2/T3 даёт системе технологический уровень не ниже 35.':'После постройки порта T2/T3 технологический уровень системы будет не ниже 35.')+'</p>':'';
 return '<details class="panel colony-development" '+(collapsed?'':'open')+'><summary>'+th('Влияние плана на систему')+'</summary><div class="colony-development-body">'+
  '<p class="colony-impact-note">'+th('Расчёт по сооружениям в вашем плане. Индексы населения показывают вклад типа сооружения в заселение и ёмкость системы.')+'</p>'+
  '<div class="table-scroll"><table><thead><tr><th>'+th('Показатель')+'</th><th>'+th('Построено')+'</th><th>'+th('Прибавка по плану')+'</th><th>'+th('После завершения')+'</th></tr></thead><tbody>'+metrics+'</tbody></table></div>'+
  (incomplete?'<p class="colony-impact-note">* '+th('Учтены сооружения с известным типом и ролью. Укажите первичный порт и специализацию старых объектов для точного сравнения.')+'</p>':'')+
  (Number(d.externalPorts)>0?'<p class="colony-impact-note">'+th('Порты, указанные только числом в настройках колонии, не входят в расчёт.')+'</p>':'')+floor+'</div></details>';
}
function renderBuildingImpact(building,u,catalog){
 const d=building.impact;
 if(!d)return '';
 const stats=Object.entries(d.metrics||{}).filter(([key])=>Object.hasOwn(developmentLabels,key)).map(([key,item])=>'<div><dt>'+th(developmentLabels[key])+'</dt><dd>'+u.esc(signed(item.weighted,u))+(known(item.base)&&item.base!==item.weighted?' <small>'+th('Базовый вклад')+' '+u.esc(signed(item.base,u))+'</small>':'')+'</dd></div>').join('');
 const economy=value=>t(developmentEconomies[value]||value);
 const metadata=[];
 if(d.pad)metadata.push([t('Посадочная площадка'),d.pad==='none'?t('Без посадки'):d.pad]);
 if(d.economy)metadata.push([t('Базовая экономика'),economy(d.economy)]);
 if(d.linkEconomy)metadata.push([t('Передаёт по связям'),economy(d.linkEconomy)]);
 if(d.bodyType)metadata.push([t('Тип небесного тела'),t(d.bodyType)]);
 if(d.bodyEconomies?.length)metadata.push([t('Экономики по свойствам тела'),d.bodyEconomies.map(economy).join(' · ')]);
 if(known(building.pointCost))metadata.push([t('Цена начала в очках'),u.fmt(Number(building.pointCost))]);
 if(known(building.pointReward))metadata.push([t('Очки за завершение'),u.fmt(Number(building.pointReward))]);
 const observedNames={commodities:'Товарный рынок',market:'Товарный рынок',shipyard:'Верфь',outfitting:'Оснащение',exploration:'Universal Cartographics',vistagenomics:'Vista Genomics',blackmarket:'Чёрный рынок',crewlounge:'Зал экипажа',pioneersupplies:'Магазин Pioneer Supplies',technologybroker:'Технологический брокер людей',refuel:'Заправка',repair:'Ремонт',rearm:'Пополнение боеприпасов',contacts:'Контакты',missions:'Доска заданий',interstellarfactors:'Межзвёздные посредники'};
 const economyName=raw=>{
  const key=normalized(raw).replace(/^economy/,'').replace(/name$/,'').replace(/^agri$/,'agriculture');
  return Object.hasOwn(developmentEconomies,key)?economy(key):t(raw);
 };
 const observedEconomies=d.observedEconomies||[];
 const observations=observedEconomies.length?observedEconomies.map(item=>'<span>'+u.esc(economyName(item.name))+' '+u.esc(u.fmt(Number(item.fraction)*100))+'%</span>').join(''):d.observedEconomy?'<span>'+u.esc(economyName(d.observedEconomy))+'</span>':'';
 const services=[...new Set((d.observedServices||[]).map(item=>observedNames[normalized(item)]||item))];
 const required=(d.prerequisites||[]).map(id=>t(settlementPrerequisites[id]||(catalog||[]).find(item=>item.id===id)?.name||''));
 const unlocks=(d.unlocks||[]).map(id=>t(developmentUnlocks[id])).filter(Boolean);
 const list=(label,items)=>items.length?'<div class="colony-impact-list"><h3>'+th(label)+'</h3><ul>'+items.map(item=>'<li>'+u.esc(item)+'</li>').join('')+'</ul></div>':'';
 const links=(d.links||[]).map(item=>'<li><span>'+u.esc(item.name)+(item.body?' · '+u.esc(item.body):'')+'</span><small>'+th(item.strength==='strong'?'Сильная':'Слабая')+' · '+th(item.direction==='in'?'Входящая':'Исходящая')+' · '+th(item.state==='built'?'Объекты построены':'По плану')+'</small></li>').join('');
 const linkList=links?'<div class="colony-impact-list colony-impact-links"><h3>'+th('Ожидаемые экономические связи')+'</h3><ul>'+links+'</ul>'+(d.linkCount>d.links.length?'<small>'+th('Дополнительные связи')+' · '+u.esc(u.fmt(d.linkCount-d.links.length))+'</small>':'')+'</div>':'';
 const needsBody=!building.body&&!!building.kind;
 return '<section class="panel colony-impact" aria-label="'+th('Влияние сооружения')+'"><header><h2>'+th('Влияние сооружения')+'</h2>'+(stats?'<span class="badge">'+th('Расчёт по каталогу')+'</span>':'')+'</header>'+
  (metadata.length?'<div class="colony-impact-meta">'+metadata.map(([label,val])=>'<span>'+u.esc(label)+' <strong>'+u.esc(val)+'</strong></span>').join('')+'</div>':'')+
  (stats?'<dl class="colony-impact-grid">'+stats+'</dl>':'<p class="colony-impact-note">'+th('Укажите конкретный тип сооружения, чтобы рассчитать его вклад.')+'</p>')+
  '<div class="colony-impact-lists">'+list('Требуется в системе',required.filter(Boolean))+list('Что может открыть',unlocks)+linkList+'</div>'+
  (d.populationCapacityExample?'<p class="colony-impact-note">'+th('Замер ёмкости для такой связки порта и тела')+' · '+u.esc(u.fmt(d.populationCapacityExample))+' '+th('жителей')+'. '+th('Локальная ёмкость зависит от положения порта и других сооружений.')+'</p>':'')+
  (needsBody?'<p class="colony-impact-note">'+th('Укажите небесное тело в параметрах сооружения для расчёта экономических связей.')+'</p>':'')+
  (observations||services.length?'<details class="colony-impact-observed"><summary>'+th('Данные станции из журнала')+(d.observedAt?' · '+u.esc(date(d.observedAt)):'')+'</summary>'+
    (observations?'<div class="colony-impact-meta">'+observations+'</div>':'')+list('Службы станции',services.map(t))+'</details>':'')+'</section>';
}
function renderBuilding(C,u){
 const project=C.state?.colonization?.activeProject, building=project?.buildings?.find(item=>item.id===C.selected);
 if(!building)return '';
 const requirements=Array.isArray(building.requirements)?building.requirements:[];
 const impact=renderBuildingImpact(building,u,C.state?.colonization?.catalog);
 if(!requirements.length)return impact;
 const total=requirements.reduce((n,row)=>n+(known(row.required)?Number(row.required):0),0);
 const delivered=requirements.reduce((n,row)=>n+(known(row.required)&&known(row.delivered)?Math.min(Number(row.required),Number(row.delivered)):0),0);
 const complete=total>0&&requirements.every(row=>known(row.required)&&known(row.delivered));
 const headings=['Товар','Нужно, т','Сдано, т','Осталось, т'];
 return '<section class="panel colony-building-cost"><header><h2>'+u.esc(building.name||building.typeName||t('Строительство'))+'</h2>'+
  (complete?'<div class="colony-building-progress"><progress max="'+u.esc(total)+'" value="'+u.esc(delivered)+'" aria-label="'+th('Ход поставок')+'"></progress><span>'+u.esc(u.fmt(delivered))+' / '+u.esc(u.fmt(total))+th(' т')+'</span></div>':'')+'</header>'+
  '<div class="table-scroll"><table><thead><tr>'+headings.map(h=>'<th>'+th(h)+'</th>').join('')+'</tr></thead><tbody>'+requirements.map(row=>'<tr><td>'+u.esc(row.name||row.commodity)+'</td>'+[row.required,row.delivered,row.missing].map(v=>'<td>'+u.esc(format(v,u))+'</td>').join('')+'</tr>').join('')+'</tbody></table></div></section>'+impact;
}
async function handleClick(action,button,C,u){
 if(!active(C)||!action?.startsWith('colony-workspace-'))return false;
 if(action==='colony-workspace-portfolio-retry'&&C.mode==='Бюджет'){refreshPortfolio(C,u,portfolio(C),true);return true;}
 if(['colony-workspace-reservation-cancel','colony-workspace-reservation-transit'].includes(action)&&C.mode==='Бюджет'){
  const p=portfolio(C),id=button?.dataset?.reservationId;
  const reservation=p.result?.reservations?.find(row=>row.id===id&&row.status==='active');
  if(reservation){
   const request=action==='colony-workspace-reservation-transit'?{action:'transit',id,inTransit:!reservation.inTransit}:{action:'cancel',id};
   await updateReservation(C,u,p,request,button);
  }
  return true;
 }
 if(action==='colony-workspace-portfolio-market'&&C.mode==='Бюджет'){
  const p=portfolio(C),commodity=button?.dataset?.commodity;
  const row=p.result?.supplies?.find(item=>item.commodity===commodity),quantity=portfolioPurchaseQuantity(row);
  if(!row||quantity<=0||p.marketPending)return true;
  p.marketPending=true;button.disabled=true;
  try{
   const next=await u.call('colony.supply-market',{commodity,quantity,expectedPlanWorkspace:p.workspace});
   if(p!==portfolio(C)||!active(C)||C.mode!=='Бюджет')return true;
   await u.navigate(next.navigate,{mode:next.mode,prefill:next.prefill});
   if(C.tab===next.navigate)await u.handleClick('action',{dataset:{actionId:'market.search'}});
  }catch(error){if(p===portfolio(C))u.toast(t(error?.message||'Не удалось открыть закупку.'),true);}
  finally{p.marketPending=false;button.disabled=false;}
  return true;
 }
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
 if(C.mode==='Бюджет'){
  const p=portfolio(C),details=u.$('#colony-portfolio');
  if(details){
   details.ontoggle=()=>{
    if(details!==u.$('#colony-portfolio')||p!==portfolio(C)||details.open===p.open)return;
    p.open=details.open;
    if(p.open){
     const filters=u.$('#colony-portfolio-filters');
     if(filters){filters.innerHTML=portfolioFilters(C,u,p);trackPortfolioBuildingDisclosure(filters,p);}
     refreshPortfolio(C,u,p);
    }
    else{p.request++;p.signature='';p.result=null;p.error='';}
   };
   trackPortfolioBuildingDisclosure(details,p);
   trackPortfolioReservations(u,p);
   refreshPortfolio(C,u,p);
  }
  return;
 }
 const r=state(C),{stations}=source(C);
 if(r.station&&!stations.some(item=>stationKey(item)===r.station)){r.station='';r.faction='';}
 if(!realFaction(r.faction))r.faction='';
 if(selectionMode(C)&&C.mode&&r.autoOrigin!==r.origin&&r.origin){r.autoOrigin=r.origin;queueSearch(C,u,'colony-workspace-check');}
 if(selectionMode(C))flushSearch(C,u);
}
window.ColonyWorkspaceUI={active,render,renderBuilding,init,handleClick,handleInput,handleChange,submitReservation};
})();
