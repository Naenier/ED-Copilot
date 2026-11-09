/* Carrier trade: Go owns calculations, limits and market validation. */
(()=>{
'use strict';
const t=(value,vars)=>window.CopilotI18n?.t(value,vars)??String(value??'');
const th=(value,vars)=>t(value,vars).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const prefix='carrier-trade-',peakScenario='booze-cruise',peakSystem='HIP 58832',types=[['fleet','Личный'],['squadron','Эскадрильи']];
const peakGoods=[['wine','Вино'],['liquor','Спиртное']];
const optionFields=[
 ['tritiumPrice','Оценка трития, кр./т',true,1e9],['durationHours','Весь рейс, ч',true,100000],['servicesTons','Службы и комплекты, т',false,100000],['weeklyUpkeep','Содержание без прыжков, кр./нед.',false,1e12],['extraCosts','Прочие расходы, кр.',false,1e15],['carrierInvestment','Сумма для окупаемости, кр.',true,1e15],['haulerCapacity','Трюм перевозчика, т',true,100000],['maxQuoteAgeHours','Максимум возраста цены, ч',false,720],['demandRatio','Запас спроса к объёму',false,100],
];
const legFields=[['quantity','Объём, т',false,1000000],['buyPrice','Цена закупки, кр./т',true,1e9],['sellPrice','Цена продажи, кр./т',true,1e9],['loadRewardPerTon','Оплата погрузки, кр./т',false,1e9],['unloadRewardPerTon','Оплата продажи, кр./т',false,1e9],['loadingHours','Погрузка, ч',true,100000],['unloadingHours','Продажа, ч',true,100000],['jumps','Прыжков',true,50000],['tritiumTons','Расход трития, т',true,1000000],['travelHours','Перелёт и дозаправки, ч',true,100000]];
let latest=null,picker=null,pollTimer=null,queue=Promise.resolve();
const model=C=>C.state?.carrierTrade||{options:{},legs:[],summary:{},issues:[],missing:[]};
const run=task=>{const next=queue.then(task);queue=next.catch(()=>{});return next;};
const button=(u,label,action,ico='',style='',attrs='')=>u.btn(label,prefix+action,ico,style,attrs);
const numberText=value=>value===null||value===undefined?'':String(value);
function draft(C){
 if(!C.carrierTradeDraft){const m=model(C);C.carrierTradeDraft={dirty:false,options:{...m.options},legs:Object.fromEntries((m.legs||[]).map(leg=>[leg.id,{...leg}]))};}
 return C.carrierTradeDraft;
}
function numeric(spec,raw){
 const [key,label,optional,max]=spec,text=String(raw??'').trim().replace(/[\s\u00a0\u202f]/g,'').replace(',','.');
 if(text===''){if(optional)return null;throw new Error(t("Заполните поле «")+t(label)+'».');}
 if(!/^\d+(?:\.\d+)?$/.test(text))throw new Error(t(label)+t(": укажите неотрицательное число."));
 const value=Number(text);if(!Number.isFinite(value)||value>max||['quantity','jumps','haulerCapacity'].includes(key)&&!Number.isInteger(value))throw new Error(t(label)+t(": значение вне допустимого диапазона."));
 return value;
}
function input(u,key,label,value,attrs='',optional=true){return `<label class="ct-field"><span>${u.esc(t(label))}</span><input type="text" inputmode="decimal" value="${u.esc(numberText(value))}" ${attrs} placeholder="${optional?t("Неизвестно"):'0'}" autocomplete="off"></label>`;}
function optionInput(C,u,spec){return input(u,spec[0],spec[1],draft(C).options[spec[0]],`data-trade-option="${spec[0]}"`,spec[2]);}
function legInput(C,u,id,key){const spec=legFields.find(row=>row[0]===key);return input(u,key,spec[1],draft(C).legs[id]?.[key],`data-trade-leg="${id}" data-trade-field="${key}"`,spec[2]);}
function select(u,label,key,value,choices,attrs){return `<label class="ct-field"><span>${u.esc(t(label))}</span><select ${attrs}>${choices.map(([id,name])=>`<option value="${u.esc(id)}" ${String(value??'')===id?'selected':''}>${u.esc(t(name))}</option>`).join('')}</select></label>`;}
function unsaved(C,u){return `<div class="ct-unsaved ${draft(C).dirty?'':'hidden'}" id="carrier-trade-unsaved" role="status">${u.icon('circle')}${th("Есть несохранённые параметры. Итоги относятся к сохранённому плану.")}${button(u,t("Сохранить"),'save','check','quiet')}</div>`;}
const opportunityMode=C=>!C.mode||C.mode==='Рейсы';
function opportunityFilters(C){
 if(!C.carrierOpportunityFilters)C.carrierOpportunityFilters={system:'',credits:null};
 return C.carrierOpportunityFilters;
}
function departureSystem(C){
 const m=model(C);
 return opportunityFilters(C).system||
  (m.options?.carrierType!=='squadron'&&m.context?.carrier?.location?.system)||C.state?.commander?.system||'';
}
function opportunityKey(C){
 const m=model(C),carrier=m.context?.carrier||{},pilot=C.state?.pilot||{},o=m.options||{};
 const commander=C.state?.commander||{};
 const services=o.carrierType!=='squadron'&&carrier.owned&&carrier.space;
 const squadronFuel=o.carrierType==='squadron'?(m.legs||[]).map(leg=>leg.tritiumTons):null;
 return JSON.stringify([opportunityFilters(C),departureSystem(C),services?.crewTons,services?.shipPacksTons,
  services?.modulePacksTons,commander.shipModel,commander.shipType,
  commander.cargoCapacity,pilot.credits,o.carrierType,o.servicesTons,
  o.haulerPad,o.haulerCapacity,squadronFuel,o.tritiumPrice,
  o.weeklyUpkeep,o.durationHours,o.extraCosts,o.maxQuoteAgeHours,o.demandRatio,o.holidayConfirmed]);
}
function matchingOpportunities(C){
 const result=model(C).opportunities||{},request=result.options||{},filters=opportunityFilters(C);
 return request.scenario===peakScenario&&(request.system||'')===filters.system&&(request.credits??null)===filters.credits?result:{};
}
function opportunityQuotesExpired(result){
 const now=Date.now();
 return (result.results||[]).some(row=>{
  const maxAge=Number(row.options?.maxQuoteAgeHours||24)*3600000;
  return [row.buy,row.sell,row.fuelMarket].some(quote=>quote?.price!=null&&now-Date.parse(quote.updatedAt)>maxAge);
 });
}
function startOpportunities(C,u,force=false){
 const m=model(C);
 if(C.tab!=='carrier-trade'||!opportunityMode(C)||m.networkDisabled||window.COPILOT_SMOKE===true)return;
 const filters=opportunityFilters(C),key=opportunityKey(C),old=C.carrierOpportunityRequest,result=matchingOpportunities(C);
 if(!old?.pending&&opportunityQuotesExpired(result)&&old?.expiredAt!==result.updatedAt)force=true;
 if(!force&&old?.key===key&&(old.pending||Date.now()-old.started<10*60*1000))return;
 const request={key,pending:true,started:Date.now(),error:'',expiredAt:force?result.updatedAt:null};
 C.carrierOpportunityRequest=request;
 return u.call('carrier-trade',{values:{action:'opportunities',...filters,scenario:peakScenario,...(force?{force:true}:{})}}).then(()=>{
  if(C.carrierOpportunityRequest===request&&C.tab==='carrier-trade'&&opportunityMode(C))return u.loadView(true);
 }).catch(error=>{
  request.error=error.message;
  if(C.carrierOpportunityRequest===request&&C.tab==='carrier-trade'&&opportunityMode(C))u.renderPage();
 }).finally(()=>{request.pending=false;});
}
function marketAge(value,u){
 const when=Date.parse(value);
 return Number.isFinite(when)?u.fmt(Math.max(0,(Date.now()-when)/3600000))+t(" ч назад"):'';
}
function purchaseOrders(orders,u){
 if(!(orders?.length>1))return '';
 return `<ol class="ct-op-orders">${orders.map(order=>`<li><strong>${u.esc(order.station)}</strong><span>${u.esc(u.fmt(order.quantity))}${th(" т")} · ${u.esc(u.money(order.price))}${th("/т")}</span></li>`).join('')}</ol>`;
}
function copySystem(u,system){
 const label=th("Скопировать систему");
 return button(u,'','copy-system','copy','icon-only',`data-system="${u.esc(system)}" title="${label}" aria-label="${label}"`);
}
function opportunityMarket(row,side,u){
 const m=row[side]||{},buy=side==='buy',age=marketAge(m.updatedAt,u),availability=m[buy?'supply':'demand'];
 return `<section class="ct-op-market"><span>${th(buy?"Закупка":"Продажа")}${m.pad?' · '+th("Площадка ")+u.esc(m.pad):''}</span>
 <strong>${u.esc(m.orders?.length>1?t("Несколько станций"):m.station)}</strong><div class="ct-op-system">${u.esc(m.system)}${m.system?copySystem(u,m.system):''}</div>
 ${purchaseOrders(m.orders,u)}
 ${m.price==null?`<p>${th("Свежей котировки нет")}</p>`:`<dl><div><dt>${th(m.orders?.length>1?"Средняя цена, кр./т":"Цена, кр./т")}</dt><dd>${u.esc(u.fmt(m.price))}</dd></div>
 ${availability==null?'':`<div><dt>${th(buy?"Запас":"Спрос")}</dt><dd>${u.esc(u.fmt(availability))}${th(" т")}</dd></div>`}
 ${m.distanceLs==null?'':`<div><dt>${th("От звезды")}</dt><dd>${u.esc(u.fmt(m.distanceLs))}${th(" св. с.")}</dd></div>`}
 ${age?`<div><dt>${th("Котировка")}</dt><dd>${u.esc(age)}</dd></div>`:''}</dl>`}
 </section>`;
}
function opportunityPath(route,u){
 const phases=[['reposition','К станции закупки'],['loaded','С грузом'],['return','Пустой возврат']];
 const rows=phases.filter(([key])=>route?.[key]?.length);
 if(!rows.length)return '';
 return `<details class="ct-op-path"><summary>${th("Системы маршрута")}</summary>${rows.map(([key,label])=>`<section><h4>${th(label)}</h4><ol>${route[key].map(hop=>`<li><span>${u.esc(hop.from)} → ${u.esc(hop.to)}</span><span>${u.esc(u.fmt(hop.distanceLy))}${th(" св. л.")}</span>${copySystem(u,hop.to)}</li>`).join('')}</ol></section>`).join('')}</details>`;
}
function opportunityFuel(tons,market,origin,u,cost){
 if(!(tons>0))return '';
 const remote=market?.system&&origin&&market.system!==origin;
 const total=cost??(market?.price==null?null:tons*Number(market.price));
 const details=[market?.station,market?.system,total==null?'':u.money(total),remote?t("Доставка из другой системы"):''].filter(Boolean);
 return `<p class="ct-op-detail">${th("Подготовить тритий")} · ${u.esc(u.fmt(tons))}${th(" т")}${details.length?' · '+u.esc(details.join(' · ')):''}</p>`;
}
function opportunityCard(row,u,index,disabled){
 const s=row.summary||{},options=row.options||{};
 const metric=(name,value)=>`<div><dt>${th(name)}</dt><dd>${u.esc(value)}</dd></div>`;
 const quantity=(value,unit)=>value==null?'—':u.fmt(value)+(unit?' '+t(unit):'');
 return `<article class="panel ct-op-card" data-carrier-opportunity="${u.esc(row.id)}">
 <header><div><h2>${u.esc(row.commodityName)}</h2><span class="muted">${u.esc(quantity(row.quantity,'т'))}${s.availableCargoTons==null?'':` / ${u.esc(quantity(s.availableCargoTons,'т'))}`}</span></div>
 <div class="ct-op-profit"><span>${th("Прибыль за рейс")}</span><strong>${u.esc(u.money(s.profit))}</strong></div></header>
 <div class="ct-op-markets">${opportunityMarket(row,'buy',u)}${opportunityMarket(row,'sell',u)}</div>
 <dl class="ct-op-metrics">${metric("Закупка",u.money(row.purchaseCost))}${metric("Прыжки",quantity(s.jumpCount,''))}
 ${metric("Расход трития",quantity(s.fuelTons,'т'))}${metric("Стоимость топлива",u.money(s.fuelCost))}
 ${metric("Все расходы",u.money(s.totalOperatingCosts))}${metric("Время рейса",quantity(s.durationHours,'ч'))}
 ${metric("Службы и комплекты",quantity(s.servicesTons??options.servicesTons,'т'))}
 ${metric("Тритий в трюме",quantity(s.cargoTritiumTons,'т'))}
 ${metric("Резерв трития (10%)",quantity(s.arrivalReserveTons??options.arrivalReserveTons,'т'))}</dl>
 ${opportunityFuel(row.fuelPurchaseTons,row.fuelMarket,row.route?.origin,u,row.fuelPurchaseCost)}
 <footer>${opportunityPath(row.route,u)}${button(u,t("Открыть в планировщике"),'use-opportunity','arrow-right','primary',`data-index="${index}" ${disabled?'disabled':''}`)}</footer>
 </article>`;
}
function opportunities(C,u){
 const m=model(C),result=matchingOpportunities(C),request=C.carrierOpportunityRequest;
 const outdated=request&&request.key!==opportunityKey(C)||opportunityQuotesExpired(result);
 const loading=result.running||request?.pending||!result.updatedAt&&!request?.error&&!m.networkDisabled;
 const rows=outdated?[]:result.results||[];
 const origin=departureSystem(C);
 const status=outdated?t("Условия изменились. Обновляем рейсы…"):loading?(result.stage==='routes'?t("Рассчитываем прыжки и топливо…"):t("Сравниваем цены и объёмы…")):
  request?.error||result.error||result.status||(m.networkDisabled?t("Рынки недоступны в автономном режиме."):'');
 return `<section class="ct-opportunities" aria-busy="${!!loading}">
 <header class="ct-op-filters"><p class="ct-op-caption">${origin?u.esc(origin)+' → ':''}${th("HIP 58832 · Rackham’s Peak · площадка M")}</p>
 ${button(u,t("Условия подбора"),'opportunity-filters','settings','quiet')}</header>
 <p class="ct-op-caption">${th("Место под товар — вместимость за вычетом служб, комплектов и трития в трюме. Запас трития на весь рейс включает 10% сверх расхода.")}</p>
 ${status?`<p class="${rows.length?'ct-op-caption':'panel ct-op-status'}" role="status">${loading?u.icon('refresh-cw'):''}${u.esc(status)}</p>`:''}
 ${result.partial||result.limited?`<p class="ct-op-caption">${th(result.partial?"Часть рынков недоступна.":"Поиск охватывает часть рынков.")}</p>`:''}
 ${rows.length?`<p class="ct-op-caption">${th("Оценка времени включает 10 мин на погрузку или разгрузку каждого трюма перевозчика.")}${rows[0].estimatedCarrier?' '+th("Вместимость оценочная."):''}</p>`:''}
 ${rows.map((row,index)=>opportunityCard(row,u,index,loading)).join('')}
 </section>`;
}
function opportunitiesFilterDialog(C,u){
 const filters=opportunityFilters(C),origin=departureSystem(C);
 u.dialog(t("Условия подбора"),`<div class="ct-grid two">
 <label class="ct-field"><span>${th("Система отправления флотоносца")}</span><input id="ct-op-system" data-trade-filter="system" value="${u.esc(filters.system)}" placeholder="${u.esc(origin)}"></label>
 ${input(u,'credits',t("Бюджет на закупку, кр."),filters.credits,'id="ct-op-credits"')}</div>`,
 button(u,t("Применить"),'opportunity-filter-apply','check','primary'));
}
async function useOpportunity(C,u,index,confirmed){
 const row=matchingOpportunities(C).results?.[index];
 if(!row)throw new Error(t("Этот вариант больше недоступен."));
 if(!confirmed&&C.carrierTradeDraft?.dirty){
  u.dialog(t("Заменить несохранённый план?"),'',button(u,t("Открыть в планировщике"),'confirm-opportunity','arrow-right','primary',`data-index="${index}"`));
  return;
 }
 try{
  await u.call('carrier-trade',{values:{action:'apply-opportunity',id:row.id}});
 }catch(error){
  C.carrierOpportunityRequest=null;
  startOpportunities(C,u,true);
  throw error;
 }
 C.carrierTradeDraft=null;
 u.closeLayer();
 C.mode='Рейс';
 await u.loadView();
}
function top(C,u){
 const m=model(C);
 const tabs=`<div class="filters"><div class="segmented">${[['Рейсы','Крыша мира'],['Рейс','План рейса'],['Расходы','Расходы'],['Проверки','Проверки']].map(([mode,label])=>`<button data-mode="${mode}" class="${(C.mode||'Рейсы')===mode?'selected':''}">${th(label)}</button>`).join('')}</div></div>`;
 const cruise=`<div class="ct-cruise"><span class="ct-cruise-icon">${u.icon('route')}</span>
 <div><strong>${th("Баббл ↔ Rackham’s Peak")}</strong><p>${th("Вино и спиртное · HIP 58832 · максимальная площадка M.")}</p>
 <label class="ct-check"><input type="checkbox" data-trade-option="holidayConfirmed" ${draft(C).options.holidayConfirmed?'checked':''}>${th("Public Holiday подтверждён в игре")}</label></div>
 ${u.badge(m.options.holidayConfirmed?t("Событие отмечено вами"):t("Праздник не подтверждён"),m.options.holidayConfirmed?'good':'warn')}</div>`;
 const heading=u.heading(t("Торговля флотоносцем"),'')+tabs;
 if(opportunityMode(C)||m.options?.preset!==peakScenario)return heading;
 const controls=`<div class="ct-planbar">${button(u,t("Шаблон «Крыша мира»"),'preset','route','quiet')}<span class="spacer"></span>${button(u,t("Конфигурация флотоносца"),'import-carrier','ship','quiet')}</div>`;
 return heading+controls+cruise+unsaved(C,u);
}
function marketCard(C,u,leg,side){
 const source=leg[side+'Market']||{},buy=side==='buy',label=buy?t("Где закупаем"):t("Где продаём"),price=buy?'buyPrice':'sellPrice',id=leg.id;
 const compatible=source.padCompatible!==false;
 const station=buy?(source.orders?.length>1?t("Несколько станций"):source.station||t("Выберите станцию")):"Rackham’s Peak";
 const system=buy?(source.system||draft(C).legs[id]?.sourceSystem||t("Система не указана")):peakSystem;
 const pad=buy?source.pad:'M';
 return `<section class="ct-market-card"><div class="ct-market-head"><span>${u.icon(buy?'package':'wallet')}${label}</span>${pad?u.badge(t("Площадка ")+pad,compatible?'':'warn'):u.badge(t("Площадка неизвестна"),'warn')}</div><strong>${u.esc(station)}</strong><span class="ct-market-system">${u.esc(system)}</span>${purchaseOrders(source.orders,u)}<div class="ct-quote-meta"><span>${source.ageHours==null?t("Цена не подтверждена"):u.fmt(Math.max(0,source.ageHours))+t(" ч назад")}</span><span>${buy?t("Запас"):t("Спрос")}: ${u.fmt(source[buy?'supply':'demand'])}${th(" т")}</span></div>${legInput(C,u,id,price)}${button(u,buy?t("Найти продавцов"):t("Проверить котировку"),'markets','search','','data-leg="'+id+'" data-side="'+side+'"')}</section>`;
}
function legSystem(u,id,key,label,value,fixed=false){
 return `<label class="ct-field"><span>${th(label)}</span><input value="${u.esc(value||'')}" data-trade-leg="${id}" data-trade-field="${key}" ${fixed?'readonly':''}></label>`;
}
function legRoute(C,u,leg){
 const id=leg.id,squadron=draft(C).options.carrierType==='squadron';
 return `<div class="ct-route-head"><h3>${u.icon('route')}${th("Маршрут флотоносца")}</h3>
 ${button(u,t("Импортировать расчёт маршрута"),'import-route','route','quiet',`data-leg="${id}" ${squadron?`disabled title="${th("Расчёт Spansh поддерживает личный флотоносец")}"`:''}`)}</div>
 <div class="ct-grid three">${legInput(C,u,id,'jumps')}${legInput(C,u,id,'tritiumTons')}${legInput(C,u,id,'travelHours')}</div>
 ${opportunityFuel(leg.route?.fuelPurchaseTons,leg.route?.fuelMarket,leg.route?.positioningSource,u,leg.route?.fuelPurchaseCost)}${opportunityPath(leg.route,u)}
 ${squadron?`<p class="info-note ct-short-note">${th("Расход трития эскадрильи нужно указать по данным корабля.")}</p>`:''}`;
}
function legCard(C,u,leg){
 const id=leg.id,d=draft(C).legs[id]||{},outbound=id==='outbound';
 const head=`<header class="ct-section-head"><div class="ct-leg-number">${outbound?'01':'02'}</div><div><h2>${outbound?th("Рейс туда"):th("Возвращение")}</h2><p>${outbound?th("Основной товар и рынок продажи"):th("Пустой перелёт тоже расходует топливо.")}</p></div></header>`;
 const systems=outbound
  ?legSystem(u,id,'sourceSystem',"Откуда",d.sourceSystem)+legSystem(u,id,'destinationSystem',"Куда",peakSystem,true)
  :legSystem(u,id,'sourceSystem',"Откуда",peakSystem,true)+legSystem(u,id,'destinationSystem',"Куда",d.destinationSystem);
 if(!outbound)return `<section class="panel ct-leg">${head}<div class="ct-leg-basics">${systems}</div>${legRoute(C,u,leg)}</section>`;
 const labour=[['self','Своими силами'],['haulers','Другими пилотами']];
 return `<section class="panel ct-leg">${head}<div class="ct-leg-basics">
 ${select(u,"Товар",'commodityKey',d.commodityKey,peakGoods,`data-trade-leg="${id}" data-trade-field="commodityKey"`)}${legInput(C,u,id,'quantity')}${systems}</div>
 <div class="ct-markets">${marketCard(C,u,leg,'buy')}${marketCard(C,u,leg,'sell')}</div>${legRoute(C,u,leg)}
 <details class="ct-details"><summary>${th("Погрузка, продажа и оплата перевозчиков")}</summary><div class="ct-grid two">
 ${select(u,"Погрузка",'loadingMode',d.loadingMode,labour,`data-trade-leg="${id}" data-trade-field="loadingMode"`)}
 ${select(u,"Продажа",'unloadingMode',d.unloadingMode,labour,`data-trade-leg="${id}" data-trade-field="unloadingMode"`)}
 ${legInput(C,u,id,'loadRewardPerTon')}${legInput(C,u,id,'unloadRewardPerTon')}${legInput(C,u,id,'loadingHours')}${legInput(C,u,id,'unloadingHours')}</div>
 <p class="info-note">${th("При самостоятельной перевозке оплата другим пилотам не включается. При их помощи цена ордера закупки: ")}${u.fmt(leg.carrierBuyPrice)}${th(" кр./т; продажи: ")}${u.fmt(leg.carrierSellPrice)}${th(" кр./т. Это расчёт — ордера задаются в игре.")}</p></details></section>`;
}
function parameters(C,u){return `<details class="panel ct-details ct-parameters" open><summary>${u.icon('settings')}${th("Флотоносец, время и расходы")}</summary><div class="ct-grid two">${select(u,t("Тип флотоносца"),'carrierType',draft(C).options.carrierType,types,'data-trade-option="carrierType"')}${select(u,t("Площадка корабля-перевозчика"),'haulerPad',draft(C).options.haulerPad,[['',t("Не выбрано")],['S',t("S · малая")],['M',t("M · средняя")],['L',t("L · большая")]],'data-trade-option="haulerPad"')}${optionFields.map(spec=>optionInput(C,u,spec)).join('')}</div><p class="info-note">${th("Запас трития рассчитывается по расходу туда и обратно с резервом 10%. Тритий в баке хранится отдельно.")}</p></details>`;}
function summary(C,u){
 const m=model(C),s=m.summary||{},status={ready:[t("План проверен"),'good'],review:[t("Сценарная оценка"),'warn'],'needs-data':[t("Нужны исходные данные"),'warn'],blocked:[t("План требует исправления"),'warn']}[s.status]||[t("Настройте план"),''];
 const value=(label,number,unit=t("кр."))=>`<div class="ct-summary-row"><span>${u.esc(t(label))}</span><strong>${u.esc(u.fmt(number))}${number==null?'':' '+unit}</strong></div>`;
 return `<aside class="panel ct-summary"><div class="ct-summary-top">${u.badge(...status)}<span class="ct-summary-label">${th("Результат владельца за весь рейс")}</span><strong class="ct-profit ${s.profit!=null&&s.profit<0?'danger':''}">${u.esc(u.money(s.profit))}</strong><span class="muted">${s.profit==null?t("Неизвестные расходы пока не позволяют оценить итог."):t("После трития, прыжков, содержания и перевозчиков.")}</span></div>
 ${value("Маржа товаров",s.grossMargin)}${value("Все расходы",s.totalOperatingCosts)}${value("Полное время",s.durationHours,t("ч"))}${value("Прибыль за час",s.profitPerHour)}<div class="divider"></div>
 ${value("Капитал в рейсе",s.workingCapital)}${value("Безубыточная продажа туда",s.breakEvenSellPrice,t("кр./т"))}${value("Окупаемость",s.paybackTrips,t("рейсов"))}<div class="divider"></div>
 ${value("Свободно под товар",s.availableCargoTons,t("т"))}${value("Службы и комплекты",s.servicesTons,t("т"))}${value("Тритий в трюме",s.cargoTritiumTons,t("т"))}
 ${value("Расход за весь рейс",s.fuelTons,t("т"))}${value("Резерв трития (10%)",s.arrivalReserveTons??m.options?.arrivalReserveTons,t("т"))}
 ${s.overCapacityTons>0?`<div class="notice">${th("Не помещается ")}${u.esc(u.fmt(s.overCapacityTons))}${th(" т. Уменьшите объём товара.")}</div>`:''}
 <button class="ct-summary-link" data-click="${prefix}max">${th("Подставить доступный объём в рейс туда ")}${u.icon('arrow-right')}</button>
 ${button(u,t("Скопировать итог"),'copy','copy','quiet')}<p class="info-note">${th("Оценка чувствительна к цене и спросу. Окупаемость относится к введённой сумме вложений.")}</p></aside>`;
}
function costs(C,u){const m=model(C),s=m.summary||{},rows=[[t("Закупочная маржа товаров"),s.grossMargin],[t("Оплата других пилотов"),s.haulingFees],[t("Тритий"),s.fuelCost],[t("Обслуживание прыжков"),s.jumpCost],[t("Распределённое содержание"),s.allocatedUpkeep],[t("Прочие расходы"),s.extraCosts],[t("Результат рейса"),s.profit]];return `<section class="panel"><div class="ct-section-head"><div><h2>${th("Из чего складывается результат")}</h2><p>${th("Закупка уже учтена в марже товаров. Расходы не вычитаются дважды.")}</p></div></div><div class="ct-cost-list">${rows.map(([label,value],index)=>`<div class="ct-cost-row ${index===rows.length-1?'ct-cost-total':''}"><span>${u.esc(t(label))}</span><strong>${u.esc(u.money(value))}</strong></div>`).join('')}</div><div class="ct-grid two ct-detail-metrics"><div><span>${th("Резерв банка на недельные списания")}</span><strong>${u.esc(u.money(s.upkeepBankReserve))}</strong></div><div><span>${th("Оценка топливного обеспечения с резервом")}</span><strong>${u.esc(u.money(s.fuelProvisionValue))}</strong></div><div><span>${th("Рентабельность капитала")}</span><strong>${u.esc(u.fmt(s.returnOnCapitalPercent))} %</strong></div><div><span>${th("Безубыточный объём при тех же расходах")}</span><strong>${u.esc(u.fmt(s.breakEvenQuantity))}${th(" т")}</strong></div></div><div class="info-note">${(m.notes||[]).map(note=>`<p>${u.esc(note)}</p>`).join('')}</div></section>`;}
function checks(C,u){const m=model(C);return `<section class="panel"><h2>${th("Проверка рейса")}</h2>${(m.missing||[]).length?`<div class="notice"><strong>${th("Не хватает исходных данных")}</strong><p>${th("Укажите цены, объём, топливо и длительность рейса.")}</p></div>`:''}<div class="ct-check-list">${(m.issues||[]).map(issue=>`<article class="ct-check-item ${issue.level==='error'?'error':''}">${u.icon(issue.level==='error'?'x':'circle-help')}<div><strong>${u.esc(issue.title)}</strong><p>${u.esc(issue.detail)}</p></div></article>`).join('')||(m.missing?.length?'':`<p class="good">${th("Замечаний нет.")}</p>`)}</div><div class="ct-source-links">${(m.sources||[]).map(source=>`<a href="#" data-external="${u.esc(source.url)}">${u.icon('external-link')}${u.esc(source.title)}</a>`).join('')}</div></section>`;}
function render(C,u){
 const m=model(C),mode=C.mode||'Рейсы';
 if(opportunityMode(C))return `<div class="ct-page">${top(C,u)}${opportunities(C,u)}</div>`;
 if(m.options?.preset!==peakScenario){
  return `<div class="ct-page">${top(C,u)}<section class="panel"><h2>${th("План рейса к Крыше мира")}</h2>
  <p class="info-note">${th("Откройте найденный рейс или загрузите шаблон для расчёта вручную.")}</p>
  ${button(u,t("Шаблон «Крыша мира»"),'preset','route','primary')}</section></div>`;
 }
 return `<div class="ct-page">${top(C,u)}<div class="ct-layout"><div class="ct-main">${mode==='Расходы'?costs(C,u):mode==='Проверки'?checks(C,u):`<form id="carrier-trade-form" novalidate>${(m.legs||[]).map(leg=>legCard(C,u,leg)).join('')}${parameters(C,u)}<div class="ct-form-error hidden" id="carrier-trade-error" role="alert"></div><div class="ct-save-bar">${button(u,t("Сохранить и рассчитать"),'save','check','primary')}</div></form>`}</div>${summary(C,u)}</div></div>`;
}
function capture(C,u){const d=draft(C);for(const node of u.$$('[data-trade-option],[data-trade-leg]')){const value=node.type==='checkbox'?node.checked:node.value;if(node.dataset.tradeOption)d.options[node.dataset.tradeOption]=value;else if(d.legs[node.dataset.tradeLeg])d.legs[node.dataset.tradeLeg][node.dataset.tradeField]=value;}return d;}
function writableOptions(options){
 const writable={...options,otherCargoTons:0,returnTrip:true};
 delete writable.tankTons;
 delete writable.cargoTritiumTons;
 delete writable.arrivalReserveTons;
 return writable;
}
function payload(C,u){
 const d=capture(C,u),options=writableOptions(d.options),legs={};
 for(const spec of optionFields)options[spec[0]]=numeric(spec,options[spec[0]]);
 for(const [id,raw] of Object.entries(d.legs)){
  const leg={};
  for(const key of ['commodityKey','sourceSystem','destinationSystem','loadingMode','unloadingMode'])leg[key]=raw[key];
  for(const spec of legFields)leg[spec[0]]=numeric(spec,raw[spec[0]]);
  legs[id]=leg;
 }
 if(legs.outbound)legs.outbound.destinationSystem=peakSystem;
 if(legs.return)Object.assign(legs.return,{sourceSystem:peakSystem,quantity:0});
 return {action:'configure',options,legs};
}
async function save(C,u){let body;try{body=payload(C,u);}catch(error){const box=u.$('#carrier-trade-error');if(box){box.textContent=error.message;box.classList.remove('hidden');}throw error;}await run(async()=>{await u.call('carrier-trade',{values:body});C.carrierTradeDraft=null;await u.loadView();});u.toast(t("План рейса сохранён"));}
async function before(C,u){capture(C,u);if(draft(C).dirty)await save(C,u);}
async function mutate(C,u,body){await before(C,u);await run(async()=>{await u.call('carrier-trade',{values:body});C.carrierTradeDraft=null;await u.loadView();});}

function pickerRows(C){
 if(!picker)return [];
 const trade=C.state.trading||{},options=trade.options||{};
 if(options.mode!==picker.side||options.resourceKey!==picker.key||
   options.system&&picker.system&&options.system.toLowerCase()!==picker.system.toLowerCase())return [];
 const rows=trade.results||[];
 if(!picker.peakSale)return rows;
 return rows.filter(row=>row.system?.toUpperCase()===peakSystem&&row.station?.replace('’',"'").toLowerCase()==="rackham's peak");
}
function renderPickerResults(C,u){if(!picker||!u.$('#carrier-trade-market-results'))return;const trade=C.state.trading||{},rows=pickerRows(C);picker.rows=rows;const rank={S:1,M:2,L:3},ship=model(C).options.haulerPad;u.$('#carrier-trade-market-results').innerHTML=picker.searching||trade.running?`<div class="ct-market-status">${u.icon('refresh-cw')}${th("Поиск рынков выполняется…")}</div>`:trade.error?`<div class="notice">${u.esc(trade.error)}</div>`:rows.length?`<div class="table-scroll"><table class="ct-market-table"><thead><tr><th>${th("Станция и система")}</th><th>${th("Цена, кр./т")}</th><th>${picker.side==='buy'?t("Запас"):t("Спрос")}${th(", т")}</th><th>${th("Площадка")}</th><th>${th("Возраст")}</th><th>${th("От звезды")}</th><th></th></tr></thead><tbody>${rows.map((row,index)=>{const q=(row.quotes||[]).find(item=>item.key===picker.key)||{},bad=rank[row.pad]&&rank[ship]&&rank[row.pad]<rank[ship];return `<tr><td><strong>${u.esc(row.station)}</strong><small>${u.esc(row.system)} · ${u.esc(u.fmt(row.distanceLy))}${th(" св. л.")}</small></td><td>${u.esc(u.fmt(q.price))}</td><td>${u.esc(u.fmt(q[picker.side==='buy'?'supply':'demand']))}</td><td>${u.badge(row.pad||'?',bad?'warn':'')}</td><td>${u.esc(u.fmt(row.ageHours))}${th(" ч")}</td><td>${u.esc(u.fmt(row.distanceLs))}${th(" св. с.")}</td><td>${button(u,bad?t("Не подходит"):t("Выбрать"),'market-choose','','',`data-index="${index}" ${bad?`disabled title="${th("Корабль требует большую площадку")}"`:''}`)}</td></tr>`;}).join('')}</tbody></table></div>`:`<div class="empty-state"><strong>${th(picker.system?"Подходящих котировок нет":"Укажите исходную систему поиска.")}</strong></div>`;}
function schedulePicker(C,u){clearTimeout(pollTimer);pollTimer=setTimeout(async()=>{if(!picker||!u.$('#carrier-trade-market-results')){picker=null;return;}try{await u.loadView();renderPickerResults(C,u);}catch(error){u.toast(error.message,true);}if(picker&&u.$('#carrier-trade-market-results')&&C.state.trading?.running)schedulePicker(C,u);},1000);}
async function openMarkets(C,u,id,side){
 await before(C,u);
 const leg=(model(C).legs||[]).find(row=>row.id===id);
 if(!leg)throw new Error(t("Плечо рейса не найдено."));
 const m=model(C),age=m.options.maxQuoteAgeHours||24;
 const peakSale=id==='outbound'&&side==='sell';
 const system=peakSale?peakSystem:leg[side==='buy'?'sourceSystem':'destinationSystem']||departureSystem(C);
 picker={leg:id,side,key:leg.commodityKey,rows:[],system,peakSale,searching:false};
 u.dialog(side==='buy'?t("Где загрузить флотоносец"):t("Куда продать груз"),
 `<div class="ct-picker-filters">
 <label class="ct-field"><span>${th(peakSale?"Система продажи":"Рядом с системой")}</span><input data-trade-filter="system" value="${u.esc(system)}" placeholder="${th("Название системы")}" ${peakSale?'readonly':''}></label>
 ${select(u,t("Площадка"),'pad',m.options.haulerPad||'any',[['any',t("Любая")],['S',t("S и больше")],['M',t("M и больше")],['L','L']],'data-trade-filter="minPad"')}
 ${input(u,'radiusLy',t("Радиус, св. л."),100,'data-trade-filter="radiusLy"',false)}
 ${input(u,'maxAgeHours',t("Возраст цены, ч"),age,'data-trade-filter="maxAgeHours"',false)}
 ${input(u,'availability',side==='buy'?t("Мин. запас, т"):t("Мин. спрос, т"),Math.min(leg.quantity||1,1000),'data-trade-filter="availability"',false)}
 ${input(u,'maxArrivalLs',t("От звезды, не более св. с."),10000,'data-trade-filter="maxArrivalLs"',false)}
 </div><div id="carrier-trade-market-results"></div>`,button(u,t("Закрыть"),'market-close'),true);
 renderPickerResults(C,u);
 if(system&&!m.networkDisabled&&window.COPILOT_SMOKE!==true&&!pickerRows(C).length)await searchMarkets(C,u);
}
async function searchMarkets(C,u){
 if(!picker)return;
 const current=picker,leg=(model(C).legs||[]).find(row=>row.id===picker.leg);
 const filters={system:picker.system,mode:picker.side,resourceKey:picker.key,useCargo:false,useCarrierCargo:false,
  includeCarriers:false,quantity:leg?.quantity||1,minDemand:0,minSupply:0};
 for(const node of u.$$('[data-trade-filter]')){
  const key=node.dataset.tradeFilter;
  if(['system','minPad'].includes(key))filters[key]=node.value;
  else{
   const value=numeric([key,key,false,1e9],node.value);
   if(key==='availability')filters[picker.side==='buy'?'minSupply':'minDemand']=value;
   else filters[key]=value;
  }
 }
 if(picker.peakSale)filters.system=peakSystem;
 current.system=filters.system.trim();
 if(!current.system){renderPickerResults(C,u);return;}
 current.searching=true;
 renderPickerResults(C,u);
 try{
  await u.call('carrier-trade-markets',{values:filters});
  if(picker!==current)return;
  await u.loadView();
  schedulePicker(C,u);
 }finally{
  current.searching=false;
  if(picker===current)renderPickerResults(C,u);
 }
}
async function handle(action,b,C,u){if(!action.startsWith(prefix))return false;const name=action.slice(prefix.length);
 if(name==='opportunity-filters'){opportunitiesFilterDialog(C,u);return true;}
 if(name==='opportunity-filter-apply'){
  const filters=opportunityFilters(C);
  filters.system=String(u.$('#ct-op-system')?.value??'').trim();
  filters.credits=numeric(['credits','Бюджет на закупку, кр.',true,1e15],u.$('#ct-op-credits')?.value);
  if(filters.credits===0)throw new Error(t("Бюджет должен быть больше нуля."));
  C.carrierOpportunityRequest=null;
  u.closeLayer();
  u.renderPage();
  startOpportunities(C,u);
  return true;
 }
 if(name==='use-opportunity'||name==='confirm-opportunity'){
  await useOpportunity(C,u,Number(b.dataset.index),name==='confirm-opportunity');
  return true;
 }
 if(name==='copy-system'){
  await u.call('native',{action:'copy',text:b.dataset.system});
  u.toast(t("Система скопирована"));
  return true;
 }
 if(name==='save'){await save(C,u);return true;}
 if(name==='preset'){u.dialog(t("Загрузить шаблон «Крыша мира»?"),`<p>${th("Шаблон заменит цены, товар и маршрут в текущем плане. Конфигурация флотоносца сохранится.")}</p>`,u.btn(t("Отмена"),'close-dialog')+button(u,t("Применить шаблон"),'preset-apply','check','primary'));return true;}
 if(name==='preset-apply'){u.closeLayer();await mutate(C,u,{action:'preset',preset:peakScenario,sourceSystem:departureSystem(C)});u.toast(t("Шаблон рейса выбран"));return true;}
 if(name==='import-carrier'){await mutate(C,u,{action:'import-carrier'});u.toast(t("Конфигурация флотоносца загружена"));return true;}
 if(name==='import-route'){await mutate(C,u,{action:'import-route',leg:b.dataset.leg});u.toast(t("Маршрут и расход трития загружены"));return true;}
 if(name==='max'){const available=model(C).summary.availableCargoTons;if(available==null)throw new Error(t("Сначала укажите расход трития на оба плеча рейса."));capture(C,u);draft(C).legs.outbound.quantity=Math.floor(available);draft(C).dirty=true;u.renderPage();return true;}
 if(name==='markets'){await openMarkets(C,u,b.dataset.leg,b.dataset.side);return true;}
 if(name==='market-search'){await searchMarkets(C,u);return true;}
 if(name==='market-close'){picker=null;clearTimeout(pollTimer);u.closeLayer();return true;}
 if(name==='market-choose'){const market=picker?.rows[Number(b.dataset.index)];if(!market)throw new Error(t("Котировка больше недоступна. Повторите поиск."));await u.call('carrier-trade',{values:{action:'market',leg:picker.leg,side:picker.side,market}});picker=null;clearTimeout(pollTimer);C.carrierTradeDraft=null;u.closeLayer();await u.loadView();u.toast(t("Котировка добавлена в рейс"));return true;}
 if(name==='help'){u.dialog(t("Как спланировать торговлю флотоносцем"),`<div class="prose-small"><p>${th("1. Откройте рейс на вкладке «Крыша мира», чтобы перенести маршрут, топливо и котировки в план.")}</p><p>${th("2. Для перевозки на Rackham’s Peak выберите корабль с малой или средней площадкой.")}</p><p>${th("3. Проверьте расчёт расхода трития, расходы на перевозчиков и длительность. Подтвердите Public Holiday в игре.")}</p></div>`,u.btn(t("Закрыть"),'close-dialog'));return true;}
 if(name==='copy'){
  const m=model(C),s=m.summary;
  const legs=m.legs.map(leg=>{
   const systems=`${leg.sourceSystem||'—'} → ${leg.destinationSystem||'—'}`;
   const cargo=leg.id==='outbound'?`${leg.commodityName}, ${u.fmt(leg.effectiveQuantity)}${t(" т · ")}`:'';
   return `${leg.name} · ${cargo}${systems}`;
  });
  await u.call('native',{action:'copy',text:[t("Торговля флотоносцем"),...legs,t("Результат владельца: ")+u.money(s.profit),t("Тритий за весь рейс: ")+u.fmt(s.fuelTons)+t(" т"),t("Расходы: ")+u.money(s.totalOperatingCosts),t("Полное время: ")+u.fmt(s.durationHours)+t(" ч"),t("Безубыточная продажа туда: ")+u.fmt(s.breakEvenSellPrice)+t(" кр./т"),t("Цены и спрос требуют проверки в игре.")].join('\r\n')});
  u.toast(t("План рейса скопирован"));
  return true;
 }
 return false;
}
function handleInput(event,C,u){const node=event.target;if(!node?.matches?.('[data-trade-option],[data-trade-leg]'))return false;const d=draft(C),value=node.type==='checkbox'?node.checked:node.value;if(node.dataset.tradeOption)d.options[node.dataset.tradeOption]=value;else d.legs[node.dataset.tradeLeg][node.dataset.tradeField]=value;d.dirty=true;u.$('#carrier-trade-unsaved')?.classList.remove('hidden');return true;}
async function handleChange(event,C,u){if(event.target?.dataset?.tradeFilter&&picker&&u.$('#carrier-trade-market-results')){await searchMarkets(C,u);return true;}if(!handleInput(event,C,u))return false;const node=event.target;if(node.dataset.tradeField==='commodityKey'){const leg=draft(C).legs[node.dataset.tradeLeg];leg.buyPrice=null;leg.sellPrice=null;}if(node.dataset.tradeOption==='carrierType'){const options=draft(C).options,squadron=node.value==='squadron';options.weeklyUpkeep=Math.max(Number(options.weeklyUpkeep)||0,squadron?10000000:5000000);if(squadron)options.servicesTons=Math.max(Number(options.servicesTons)||0,3000);if(Number(options.carrierInvestment)===(squadron?5000000000:25000000000))options.carrierInvestment=squadron?25000000000:5000000000;}if(node.tagName==='SELECT'||node.type==='checkbox')u.renderPage();return true;}
function init(C,u){
 latest={C,u};
 const search=startOpportunities(C,u);
 if(typeof document!=='undefined'&&!init.bound){
  init.bound=true;
  document.addEventListener('submit',event=>{
   if(event.target.id!=='carrier-trade-form'||!latest)return;
   event.preventDefault();
   save(latest.C,latest.u).catch(error=>latest.u.toast(error.message,true));
  });
 }
 return search;
}
async function smoke(C,u){
 const html=render({...C,mode:'Рейс',carrierTradeDraft:null},u),checks=[{name:t("Торговля флотоносцем: два плеча рейса"),ok:html.includes('Рейс туда')&&html.includes('Возвращение')},{name:t("Торговля флотоносцем: расходы и безубыточность"),ok:html.includes('Безубыточная')&&html.includes('Тритий')},{name:t("Торговля флотоносцем: отдельный поиск рынков"),ok:html.includes(prefix+'markets')}];
 // Mutation scenarios run only in the host's isolated, network-disabled smoke profile.
 if(window.COPILOT_SMOKE!==true)return checks;
 const previousMode=C.mode,original=JSON.parse(JSON.stringify(model(C))),originalTrading=C.state.trading;
 const add=(name,ok)=>checks.push({name,ok:!!ok});
 try{
  C.mode='Рейс';C.carrierTradeDraft=null;await u.loadView();
  add(t("Торговля флотоносцем: рабочая форма"),!!u.$('#carrier-trade-form')&&
   !!u.$('[data-trade-option="servicesTons"]')&&!!u.$('[data-trade-leg="return"][data-trade-field="tritiumTons"]')&&
   !u.$('[data-trade-option="otherCargoTons"]')&&!u.$('[data-trade-leg="return"][data-trade-field="quantity"]'));
  const price=u.$('[data-trade-leg="outbound"][data-trade-field="buyPrice"]');
  if(price){price.value='';handleInput({target:price},C,u);await handle(prefix+'save',null,C,u);add(t("Торговля флотоносцем: пустая цена остаётся неизвестной"),model(C).legs.find(leg=>leg.id==='outbound')?.buyPrice===null);}else add(t("Торговля флотоносцем: пустая цена остаётся неизвестной"),false);
  const beforePreset=JSON.stringify(model(C).options);
  await handle(prefix+'preset',null,C,u);
  add(t("Торговля флотоносцем: подтверждение замены сценария"),!!u.$('[data-click="carrier-trade-preset-apply"]'));
  u.closeLayer();add(t("Торговля флотоносцем: отмена сохраняет план"),JSON.stringify(model(C).options)===beforePreset);
  await handle(prefix+'markets',{dataset:{leg:'outbound',side:'buy'}},C,u);
  const leg=model(C).legs.find(leg=>leg.id==='outbound'),synthetic={station:'Copilot Smoke Port',system:leg.sourceSystem||'Synthetic Trade System',pad:model(C).options.haulerPad==='L'?'L':'M',updatedAt:new Date().toISOString(),ageHours:0,distanceLy:0,distanceLs:100,source:'synthetic-smoke',quotes:[{key:leg.commodityKey,price:12345,supply:Math.max(1,leg.quantity)*10,demand:Math.max(1,leg.quantity)*10}]};
  C.state.trading={running:false,results:[synthetic],options:{mode:'buy',resourceKey:leg.commodityKey}};renderPickerResults(C,u);
  add(t("Торговля флотоносцем: выбор котировки с площадкой"),!!u.$('[data-click="carrier-trade-market-choose"]')&&u.$('#carrier-trade-market-results').textContent.includes('Copilot Smoke Port'));
  await handle(prefix+'market-choose',{dataset:{index:'0'}},C,u);
  const selected=model(C).legs.find(row=>row.id==='outbound');add(t("Торговля флотоносцем: котировка проходит через Go"),selected?.buyPrice===12345&&selected?.buyMarket?.station==='Copilot Smoke Port');
 }finally{
  picker=null;clearTimeout(pollTimer);u.closeLayer();
  const legs=Object.fromEntries(original.legs.map(leg=>[leg.id,leg]));await u.call('carrier-trade',{values:{action:'configure',options:writableOptions(original.options),legs}});
  C.carrierTradeDraft=null;C.mode=previousMode;C.state.trading=originalTrading;await u.loadView();
 }
 return checks;
}
window.CarrierTradeUI={render,handle,handleInput,handleChange,init,smoke};
})();
