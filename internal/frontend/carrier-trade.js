/* Carrier trade: Go owns calculations, limits and market validation. */
(()=>{
'use strict';
const t=(value,vars)=>window.CopilotI18n?.t(value,vars)??String(value??'');
const th=(value,vars)=>t(value,vars).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const prefix='carrier-trade-',types=[['fleet','Личный'],['squadron','Эскадрильи']];
const optionFields=[
 ['tritiumPrice','Оценка трития, кр./т',true,1e9],['durationHours','Весь рейс, ч',true,100000],['servicesTons','Службы и комплекты, т',false,100000],['otherCargoTons','Другой груз, т',false,1000000],['tankTons','Тритий в баке перед вылетом, т',false,1000],['cargoTritiumTons','Тритий в трюме, т',true,100000],['arrivalReserveTons','Резерв после возвращения, т',false,100000],['weeklyUpkeep','Содержание без прыжков, кр./нед.',false,1e12],['extraCosts','Прочие расходы, кр.',false,1e15],['carrierInvestment','Сумма для окупаемости, кр.',true,1e15],['haulerCapacity','Трюм перевозчика, т',true,100000],['maxQuoteAgeHours','Максимум возраста цены, ч',false,720],['demandRatio','Запас спроса к объёму',false,100],
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
 if(!C.carrierOpportunityFilters)C.carrierOpportunityFilters={scenario:'custom',system:'',credits:null};
 return C.carrierOpportunityFilters;
}
function opportunityKey(C){
 const m=model(C),stock=m.context?.carrier||{},pilot=C.state?.pilot||{},o=m.options||{};
 const commander=C.state?.commander||{};
 return JSON.stringify([opportunityFilters(C),stock.location?.system,stock.space?.freeTons,stock.fuel?.tankTons,
  stock.fuel?.reserveTons,stock.fuel?.needsRefresh,commander.system,commander.shipModel,commander.shipType,
  commander.cargoCapacity,pilot.credits,o.carrierType,o.servicesTons,
  o.otherCargoTons,o.haulerPad,o.haulerCapacity,o.tankTons,o.cargoTritiumTons,o.arrivalReserveTons,o.tritiumPrice,
  o.weeklyUpkeep,o.durationHours,o.extraCosts,o.maxQuoteAgeHours,o.demandRatio,o.holidayConfirmed]);
}
function matchingOpportunities(C){
 const result=model(C).opportunities||{},request=result.options||{},filters=opportunityFilters(C);
 return request.scenario===filters.scenario&&(request.system||'')===filters.system&&(request.credits??null)===filters.credits?result:{};
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
 return u.call('carrier-trade',{values:{action:'opportunities',...filters,...(force?{force:true}:{})}}).then(()=>{
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
function opportunityFuel(tons,market,origin,u){
 if(!(tons>0))return '';
 const remote=market?.system&&origin&&market.system!==origin;
 return `<p class="ct-op-detail">${th("Пополнить тритий")}: ${u.esc(u.fmt(tons))}${th(" т")} · ${u.esc(market?.station||'')} · ${u.esc(market?.system||'')} · ${u.esc(u.money(tons*Number(market?.price||0)))}${remote?' · '+th("Доставка из другой системы"):''}</p>`;
}
function opportunityCard(row,u,index,disabled){
 const s=row.summary||{};
 const metric=(name,value)=>`<div><dt>${th(name)}</dt><dd>${u.esc(value)}</dd></div>`;
 const quantity=(value,unit)=>value==null?'—':u.fmt(value)+(unit?' '+t(unit):'');
 return `<article class="panel ct-op-card" data-carrier-opportunity="${u.esc(row.id)}">
 <header><div><h2>${u.esc(row.commodityName)}</h2><span class="muted">${u.esc(quantity(row.quantity,'т'))}${s.availableCargoTons==null?'':` / ${u.esc(quantity(s.availableCargoTons,'т'))}`}</span></div>
 <div class="ct-op-profit"><span>${th("Прибыль за рейс")}</span><strong>${u.esc(u.money(s.profit))}</strong></div></header>
 <div class="ct-op-markets">${opportunityMarket(row,'buy',u)}${opportunityMarket(row,'sell',u)}</div>
 <dl class="ct-op-metrics">${metric("Закупка",u.money(row.purchaseCost))}${metric("Прыжки",quantity(s.jumpCount,''))}
 ${metric("Расход трития",quantity(s.fuelTons,'т'))}${metric("Стоимость топлива",u.money(s.fuelCost))}
 ${metric("Все расходы",u.money(s.totalOperatingCosts))}${metric("Время рейса",quantity(s.durationHours,'ч'))}</dl>
 ${opportunityFuel(row.fuelPurchaseTons,row.fuelMarket,row.route?.origin,u)}
 ${s.fuelTons==null?`<p class="ct-op-detail">${th("Расход трития эскадрильи нужно указать по данным корабля.")}</p>`:''}
 <footer>${opportunityPath(row.route,u)}${button(u,t("Открыть в планировщике"),'use-opportunity','arrow-right','primary',`data-index="${index}" ${disabled?'disabled':''}`)}</footer>
 </article>`;
}
function opportunities(C,u){
 const m=model(C),filters=opportunityFilters(C),result=matchingOpportunities(C),request=C.carrierOpportunityRequest;
 const outdated=request&&request.key!==opportunityKey(C)||opportunityQuotesExpired(result);
 const loading=result.running||request?.pending||!result.updatedAt&&!request?.error&&!m.networkDisabled;
 const rows=outdated?[]:result.results||[];
 const origin=filters.system||m.context?.carrier?.location?.system||C.state?.commander?.system||'';
 const subtitle=filters.scenario==='booze-cruise'?t("HIP 58832 · Rackham’s Peak · площадка M"):t("Рейсы рядом с флотоносцем");
 const status=outdated?t("Условия изменились. Обновляем рейсы…"):loading?(result.stage==='routes'?t("Рассчитываем прыжки и топливо…"):t("Сравниваем цены и объёмы…")):
  request?.error||result.error||result.status||(m.networkDisabled?t("Рынки недоступны в автономном режиме."):'');
 return `<section class="ct-opportunities" aria-busy="${!!loading}">
 <header class="ct-op-filters"><div class="segmented" aria-label="${th("Тип рейса")}">
 <button type="button" data-click="${prefix}opportunity-scenario" data-id="custom" class="${filters.scenario==='custom'?'selected':''}">${th("Рядом")}</button>
 <button type="button" data-click="${prefix}opportunity-scenario" data-id="booze-cruise" class="${filters.scenario==='booze-cruise'?'selected':''}">Booze Cruise</button></div>
 ${button(u,t("Условия подбора"),'opportunity-filters','settings','quiet')}</header>
 <p class="ct-op-caption">${u.esc(subtitle)}${origin?' · '+u.esc(origin):''}</p>
 ${status?`<p class="${rows.length?'ct-op-caption':'panel ct-op-status'}" role="status">${loading?u.icon('refresh-cw'):''}${u.esc(status)}</p>`:''}
 ${result.partial||result.limited?`<p class="ct-op-caption">${th(result.partial?"Часть рынков недоступна.":"Поиск охватывает часть рынков.")}</p>`:''}
 ${rows.length?`<p class="ct-op-caption">${th("Оценка времени включает 10 мин на погрузку или разгрузку каждого трюма перевозчика.")}${rows[0].estimatedCarrier?' '+th("Вместимость оценочная."):''}</p>`:''}
 ${rows.map((row,index)=>opportunityCard(row,u,index,loading)).join('')}
 </section>`;
}
function opportunitiesFilterDialog(C,u){
 const filters=opportunityFilters(C),origin=model(C).context?.carrier?.location?.system||C.state?.commander?.system||'';
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
 const m=model(C),booze=m.options?.preset==='booze-cruise',planner=!opportunityMode(C);
 const tabs=`<div class="filters"><div class="segmented">${[['Рейсы','Рейсы'],['Рейс','План рейса'],['Расходы','Расходы'],['Проверки','Проверки']].map(([mode,label])=>`<button data-mode="${mode}" class="${(C.mode||'Рейсы')===mode?'selected':''}">${th(label)}</button>`).join('')}</div></div>`;
 const controls=`<div class="ct-planbar"><div class="segmented">
 <button data-click="${prefix}preset" data-id="custom" class="${!booze?'selected':''}">${th("Обычный рейс")}</button>
 <button data-click="${prefix}preset" data-id="booze-cruise" class="${booze?'selected':''}">${u.icon('ship')} Booze Cruise</button>
 </div><span class="spacer"></span>${button(u,t("Конфигурация флотоносца"),'import-carrier','ship','quiet')}</div>`;
 const cruise=booze?`<div class="ct-cruise"><span class="ct-cruise-icon">${u.icon('route')}</span>
 <div><strong>${th("Баббл ↔ Rackham’s Peak")}</strong><p>${th("Вино и спиртное · HIP 58832 · максимальная площадка M.")}</p>
 <label class="ct-check"><input type="checkbox" data-trade-option="holidayConfirmed" ${draft(C).options.holidayConfirmed?'checked':''}>${th("Public Holiday подтверждён в игре")}</label></div>
 ${u.badge(m.options.holidayConfirmed?t("Событие отмечено вами"):t("Праздник не подтверждён"),m.options.holidayConfirmed?'good':'warn')}</div>`:'';
 return u.heading(t("Торговля флотоносцем"),'')+tabs+(planner?controls+cruise+unsaved(C,u):'');
}
function marketCard(C,u,leg,side){
 const source=leg[side+'Market']||{},buy=side==='buy',label=buy?t("Где закупаем"):t("Где продаём"),price=buy?'buyPrice':'sellPrice',id=leg.id;
 const compatible=source.padCompatible!==false;
 return `<section class="ct-market-card"><div class="ct-market-head"><span>${u.icon(buy?'package':'wallet')}${label}</span>${source.pad?u.badge(t("Площадка ")+source.pad,compatible?'':'warn'):u.badge(t("Площадка неизвестна"),'warn')}</div><strong>${u.esc(source.orders?.length>1?t("Несколько станций"):source.station||t("Выберите станцию"))}</strong><span class="ct-market-system">${u.esc(source.system||draft(C).legs[id]?.[buy?'sourceSystem':'destinationSystem']||t("Система не указана"))}</span>${purchaseOrders(source.orders,u)}<div class="ct-quote-meta"><span>${source.ageHours==null?t("Цена не подтверждена"):u.fmt(Math.max(0,source.ageHours))+t(" ч назад")}</span><span>${buy?t("Запас"):t("Спрос")}: ${u.fmt(source[buy?'supply':'demand'])}${th(" т")}</span></div>${legInput(C,u,id,price)}${button(u,t("Найти ")+(buy?t("продавцов"):t("покупателей")),'markets','search','','data-leg="'+id+'" data-side="'+side+'"')}</section>`;
}
function legCard(C,u,leg){
 const id=leg.id,d=draft(C).legs[id]||{},disabled=id==='return'&&!draft(C).options.returnTrip;
 const goods=new Map();for(const item of [...(C.state.resources||[]),...(C.state.goods||[])])if(item.kind!=='raw'&&item.key)goods.set(item.key,item.name||item.key);
 if(d.commodityKey&&!goods.has(d.commodityKey))goods.set(d.commodityKey,leg.commodityName||d.commodityKey);
 const choices=[...goods].sort((a,b)=>a[1].localeCompare(b[1],window.CopilotI18n?.locale()||'ru-RU'));
 return `<section class="panel ct-leg ${disabled?'ct-leg-disabled':''}"><header class="ct-section-head"><div class="ct-leg-number">${id==='outbound'?'01':'02'}</div><div><h2>${id==='outbound'?t("Рейс туда"):t("Возвращение")}</h2><p>${id==='outbound'?t("Основной товар и рынок продажи"):t("Пустой перелёт тоже расходует топливо. Укажите товар, если берёте обратный груз.")}</p></div>${id==='return'?`<label class="ct-check"><input type="checkbox" data-trade-option="returnTrip" ${draft(C).options.returnTrip?'checked':''}>${th("Учитывать")}</label>`:''}</header>${disabled?`<p class="info-note">${th("Обратное плечо исключено из расходов и времени.")}</p>`:`<div class="ct-leg-basics">${select(u,t("Товар"),'commodityKey',d.commodityKey,choices,`data-trade-leg="${id}" data-trade-field="commodityKey"`)}${legInput(C,u,id,'quantity')}<label class="ct-field"><span>${th("Откуда")}</span><input value="${u.esc(d.sourceSystem||'')}" data-trade-leg="${id}" data-trade-field="sourceSystem" placeholder="${th("Исходная система")}"></label><label class="ct-field"><span>${th("Куда")}</span><input value="${u.esc(d.destinationSystem||'')}" data-trade-leg="${id}" data-trade-field="destinationSystem" placeholder="${th("Система назначения")}"></label></div><div class="ct-markets">${marketCard(C,u,leg,'buy')}${marketCard(C,u,leg,'sell')}</div><div class="ct-route-head"><h3>${u.icon('route')}${th("Маршрут флотоносца")}</h3>${button(u,t("Импортировать расчёт маршрута"),'import-route','route','quiet',`data-leg="${id}" ${draft(C).options.carrierType==='squadron'?`disabled title="${th("Расчёт Spansh поддерживает личный флотоносец")}"`:''}`)}</div><div class="ct-grid three">${legInput(C,u,id,'jumps')}${legInput(C,u,id,'tritiumTons')}${legInput(C,u,id,'travelHours')}</div>${opportunityFuel(leg.route?.fuelPurchaseTons,leg.route?.fuelMarket,leg.route?.positioningSource,u)}${opportunityPath(leg.route,u)}<p class="info-note ct-short-note">${th("Расход зависит от загрузки, служб и трития. Расчёт Spansh в приложении относится к личному флотоносцу; для эскадрильи укажите расход из подходящего расчёта вручную.")}</p><details class="ct-details"><summary>${th("Погрузка, продажа и оплата перевозчиков")}</summary><div class="ct-grid two">${select(u,t("Погрузка"),'loadingMode',d.loadingMode,[['self',t("Своими силами")],['haulers',t("Другими пилотами")]],`data-trade-leg="${id}" data-trade-field="loadingMode"`)}${select(u,t("Продажа"),'unloadingMode',d.unloadingMode,[['self',t("Своими силами")],['haulers',t("Другими пилотами")]],`data-trade-leg="${id}" data-trade-field="unloadingMode"`)}${legInput(C,u,id,'loadRewardPerTon')}${legInput(C,u,id,'unloadRewardPerTon')}${legInput(C,u,id,'loadingHours')}${legInput(C,u,id,'unloadingHours')}</div><p class="info-note">${th("При самостоятельной перевозке оплата другим пилотам не включается. При их помощи цена ордера закупки: ")}${u.fmt(leg.carrierBuyPrice)}${th(" кр./т; продажи: ")}${u.fmt(leg.carrierSellPrice)}${th(" кр./т. Это расчёт — ордера задаются в игре.")}</p></details>`}</section>`;
}
function parameters(C,u){return `<details class="panel ct-details ct-parameters" open><summary>${u.icon('settings')}${th("Флотоносец, время и расходы")}</summary><div class="ct-grid two">${select(u,t("Тип флотоносца"),'carrierType',draft(C).options.carrierType,types,'data-trade-option="carrierType"')}${select(u,t("Площадка корабля-перевозчика"),'haulerPad',draft(C).options.haulerPad,[['',t("Не выбрано")],['S',t("S · малая")],['M',t("M · средняя")],['L',t("L · большая")]],'data-trade-option="haulerPad"')}${optionFields.map(spec=>optionInput(C,u,spec)).join('')}</div><p class="info-note">${th("Пустой тритий в трюме рассчитывается автоматически для двух плеч и резерва. Тритий в баке не занимает товарный трюм. Полное время можно указать сразу — вместе с погрузкой, ожиданием и продажей.")}</p></details>`;}
function summary(C,u){
 const m=model(C),s=m.summary||{},status={ready:[t("План проверен"),'good'],review:[t("Сценарная оценка"),'warn'],'needs-data':[t("Нужны исходные данные"),'warn'],blocked:[t("План требует исправления"),'warn']}[s.status]||[t("Настройте план"),''];
 const value=(label,number,unit=t("кр."))=>`<div class="ct-summary-row"><span>${u.esc(t(label))}</span><strong>${u.esc(u.fmt(number))}${number==null?'':' '+unit}</strong></div>`;
 return `<aside class="panel ct-summary"><div class="ct-summary-top">${u.badge(...status)}<span class="ct-summary-label">${th("Результат владельца за весь рейс")}</span><strong class="ct-profit ${s.profit!=null&&s.profit<0?'danger':''}">${u.esc(u.money(s.profit))}</strong><span class="muted">${s.profit==null?t("Неизвестные расходы пока не позволяют оценить итог."):t("После трития, прыжков, содержания и перевозчиков.")}</span></div>${value(t("Маржа товаров"),s.grossMargin)}${value(t("Все расходы"),s.totalOperatingCosts)}${value(t("Полное время"),s.durationHours,t("ч"))}${value(t("Прибыль за час"),s.profitPerHour)}<div class="divider"></div>${value(t("Капитал в рейсе"),s.workingCapital)}${value(t("Безубыточная продажа туда"),s.breakEvenSellPrice,t("кр./т"))}${value(t("Окупаемость"),s.paybackTrips,t("рейсов"))}<div class="divider"></div>${value(t("Свободно под товар"),s.availableCargoTons,t("т"))}${value(t("Тритий в трюме"),s.cargoTritiumTons,t("т"))}${value(t("Расход за весь рейс"),s.fuelTons,t("т"))}${s.overCapacityTons>0?`<div class="notice">${th("Не помещается ")}${u.esc(u.fmt(s.overCapacityTons))}${th(" т. Уменьшите груз или освободите трюм.")}</div>`:''}<button class="ct-summary-link" data-click="${prefix}max">${th("Подставить доступный объём в рейс туда ")}${u.icon('arrow-right')}</button>${button(u,t("Скопировать итог"),'copy','copy','quiet')}<p class="info-note">${th("Оценка чувствительна к цене и спросу. Окупаемость относится к введённой сумме вложений.")}</p></aside>`;
}
function costs(C,u){const m=model(C),s=m.summary||{},rows=[[t("Закупочная маржа товаров"),s.grossMargin],[t("Оплата других пилотов"),s.haulingFees],[t("Тритий"),s.fuelCost],[t("Обслуживание прыжков"),s.jumpCost],[t("Распределённое содержание"),s.allocatedUpkeep],[t("Прочие расходы"),s.extraCosts],[t("Результат рейса"),s.profit]];return `<section class="panel"><div class="ct-section-head"><div><h2>${th("Из чего складывается результат")}</h2><p>${th("Закупка уже учтена в марже товаров. Расходы не вычитаются дважды.")}</p></div></div><div class="ct-cost-list">${rows.map(([label,value],index)=>`<div class="ct-cost-row ${index===rows.length-1?'ct-cost-total':''}"><span>${u.esc(t(label))}</span><strong>${u.esc(u.money(value))}</strong></div>`).join('')}</div><div class="ct-grid two ct-detail-metrics"><div><span>${th("Резерв банка на недельные списания")}</span><strong>${u.esc(u.money(s.upkeepBankReserve))}</strong></div><div><span>${th("Оценка топливного обеспечения с резервом")}</span><strong>${u.esc(u.money(s.fuelProvisionValue))}</strong></div><div><span>${th("Рентабельность капитала")}</span><strong>${u.esc(u.fmt(s.returnOnCapitalPercent))} %</strong></div><div><span>${th("Безубыточный объём при тех же расходах")}</span><strong>${u.esc(u.fmt(s.breakEvenQuantity))}${th(" т")}</strong></div></div><div class="info-note">${(m.notes||[]).map(note=>`<p>${u.esc(note)}</p>`).join('')}</div></section>`;}
function checks(C,u){const m=model(C);return `<section class="panel"><h2>${th("Проверка рейса")}</h2>${(m.missing||[]).length?`<div class="notice"><strong>${th("Не хватает исходных данных")}</strong><p>${th("Укажите цены, объём, топливо и длительность рейса.")}</p></div>`:''}<div class="ct-check-list">${(m.issues||[]).map(issue=>`<article class="ct-check-item ${issue.level==='error'?'error':''}">${u.icon(issue.level==='error'?'x':'circle-help')}<div><strong>${u.esc(issue.title)}</strong><p>${u.esc(issue.detail)}</p></div></article>`).join('')||(m.missing?.length?'':`<p class="good">${th("Замечаний нет.")}</p>`)}</div><div class="ct-source-links">${(m.sources||[]).map(source=>`<a href="#" data-external="${u.esc(source.url)}">${u.icon('external-link')}${u.esc(source.title)}</a>`).join('')}</div></section>`;}
function render(C,u){
 const m=model(C),mode=C.mode||'Рейсы';
 if(opportunityMode(C))return `<div class="ct-page">${top(C,u)}${opportunities(C,u)}</div>`;
 return `<div class="ct-page">${top(C,u)}<div class="ct-layout"><div class="ct-main">${mode==='Расходы'?costs(C,u):mode==='Проверки'?checks(C,u):`<form id="carrier-trade-form" novalidate>${(m.legs||[]).map(leg=>legCard(C,u,leg)).join('')}${parameters(C,u)}<div class="ct-form-error hidden" id="carrier-trade-error" role="alert"></div><div class="ct-save-bar">${button(u,t("Сохранить и рассчитать"),'save','check','primary')}</div></form>`}</div>${summary(C,u)}</div></div>`;
}
function capture(C,u){const d=draft(C);for(const node of u.$$('[data-trade-option],[data-trade-leg]')){const value=node.type==='checkbox'?node.checked:node.value;if(node.dataset.tradeOption)d.options[node.dataset.tradeOption]=value;else if(d.legs[node.dataset.tradeLeg])d.legs[node.dataset.tradeLeg][node.dataset.tradeField]=value;}return d;}
function payload(C,u){const d=capture(C,u),options={...d.options},legs={};for(const spec of optionFields)options[spec[0]]=numeric(spec,options[spec[0]]);for(const [id,raw] of Object.entries(d.legs)){const leg={};for(const key of ['commodityKey','sourceSystem','destinationSystem','loadingMode','unloadingMode'])leg[key]=raw[key];for(const spec of legFields)leg[spec[0]]=numeric(spec,raw[spec[0]]);legs[id]=leg;}return {action:'configure',options,legs};}
async function save(C,u){let body;try{body=payload(C,u);}catch(error){const box=u.$('#carrier-trade-error');if(box){box.textContent=error.message;box.classList.remove('hidden');}throw error;}await run(async()=>{await u.call('carrier-trade',{values:body});C.carrierTradeDraft=null;await u.loadView();});u.toast(t("План рейса сохранён"));}
async function before(C,u){capture(C,u);if(draft(C).dirty)await save(C,u);}
async function mutate(C,u,body){await before(C,u);await run(async()=>{await u.call('carrier-trade',{values:body});C.carrierTradeDraft=null;await u.loadView();});}

function pickerRows(C){
 if(!picker)return [];
 const trade=C.state.trading||{},options=trade.options||{};
 if(options.mode!==picker.side||options.resourceKey!==picker.key||
   options.system&&picker.system&&options.system.toLowerCase()!==picker.system.toLowerCase())return [];
 return trade.results||[];
}
function renderPickerResults(C,u){if(!picker||!u.$('#carrier-trade-market-results'))return;const trade=C.state.trading||{},rows=pickerRows(C);picker.rows=rows;const rank={S:1,M:2,L:3},ship=model(C).options.haulerPad;u.$('#carrier-trade-market-results').innerHTML=picker.searching||trade.running?`<div class="ct-market-status">${u.icon('refresh-cw')}${th("Поиск рынков выполняется…")}</div>`:trade.error?`<div class="notice">${u.esc(trade.error)}</div>`:rows.length?`<div class="table-scroll"><table class="ct-market-table"><thead><tr><th>${th("Станция и система")}</th><th>${th("Цена, кр./т")}</th><th>${picker.side==='buy'?t("Запас"):t("Спрос")}${th(", т")}</th><th>${th("Площадка")}</th><th>${th("Возраст")}</th><th>${th("От звезды")}</th><th></th></tr></thead><tbody>${rows.map((row,index)=>{const q=(row.quotes||[]).find(item=>item.key===picker.key)||{},bad=rank[row.pad]&&rank[ship]&&rank[row.pad]<rank[ship];return `<tr><td><strong>${u.esc(row.station)}</strong><small>${u.esc(row.system)} · ${u.esc(u.fmt(row.distanceLy))}${th(" св. л.")}</small></td><td>${u.esc(u.fmt(q.price))}</td><td>${u.esc(u.fmt(q[picker.side==='buy'?'supply':'demand']))}</td><td>${u.badge(row.pad||'?',bad?'warn':'')}</td><td>${u.esc(u.fmt(row.ageHours))}${th(" ч")}</td><td>${u.esc(u.fmt(row.distanceLs))}${th(" св. с.")}</td><td>${button(u,bad?t("Не подходит"):t("Выбрать"),'market-choose','','',`data-index="${index}" ${bad?`disabled title="${th("Корабль требует большую площадку")}"`:''}`)}</td></tr>`;}).join('')}</tbody></table></div>`:`<div class="empty-state"><strong>${th(picker.system?"Подходящих котировок нет":"Укажите исходную систему поиска.")}</strong></div>`;}
function schedulePicker(C,u){clearTimeout(pollTimer);pollTimer=setTimeout(async()=>{if(!picker||!u.$('#carrier-trade-market-results')){picker=null;return;}try{await u.loadView();renderPickerResults(C,u);}catch(error){u.toast(error.message,true);}if(picker&&u.$('#carrier-trade-market-results')&&C.state.trading?.running)schedulePicker(C,u);},1000);}
async function openMarkets(C,u,id,side){
 await before(C,u);
 const leg=(model(C).legs||[]).find(row=>row.id===id);
 if(!leg)throw new Error(t("Плечо рейса не найдено."));
 const m=model(C),age=m.options.maxQuoteAgeHours||24;
 const system=leg[side==='buy'?'sourceSystem':'destinationSystem']||m.context?.carrier?.location?.system||C.state?.commander?.system||'';
 picker={leg:id,side,key:leg.commodityKey,rows:[],system,searching:false};
 u.dialog(side==='buy'?t("Где загрузить флотоносец"):t("Куда продать груз"),
 `<div class="ct-picker-filters">
 <label class="ct-field"><span>${th("Рядом с системой")}</span><input data-trade-filter="system" value="${u.esc(system)}" placeholder="${th("Название системы")}"></label>
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
 if(name==='opportunity-scenario'){
  const scenario=b.dataset.id;
  if(opportunityFilters(C).scenario===scenario)return true;
  opportunityFilters(C).scenario=scenario;
  C.carrierOpportunityRequest=null;
  u.renderPage();
  startOpportunities(C,u);
  return true;
 }
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
 if(name==='preset'){u.dialog(t("Заменить сценарий рейса?"),`<p>${th("Цены, товар и маршрут будут очищены. Конфигурация флотоносца сохранится. Новая цена вина не подставляется автоматически.")}</p>`,u.btn(t("Отмена"),'close-dialog')+button(u,t("Применить шаблон"),'preset-apply','check','primary',`data-id="${u.esc(b.dataset.id)}"`));return true;}
 if(name==='preset-apply'){u.closeLayer();await mutate(C,u,{action:'preset',preset:b.dataset.id,sourceSystem:C.state.commander?.system||''});u.toast(t("Шаблон рейса выбран"));return true;}
 if(name==='import-carrier'){await mutate(C,u,{action:'import-carrier'});u.toast(t("Конфигурация флотоносца загружена"));return true;}
 if(name==='import-route'){await mutate(C,u,{action:'import-route',leg:b.dataset.leg});u.toast(t("Маршрут и расход трития загружены"));return true;}
 if(name==='max'){const available=model(C).summary.availableCargoTons;if(available==null)throw new Error(t("Сначала задайте тритий или расход обоих плеч для расчёта свободного трюма."));capture(C,u);draft(C).legs.outbound.quantity=Math.floor(available);draft(C).dirty=true;u.renderPage();return true;}
 if(name==='markets'){await openMarkets(C,u,b.dataset.leg,b.dataset.side);return true;}
 if(name==='market-search'){await searchMarkets(C,u);return true;}
 if(name==='market-close'){picker=null;clearTimeout(pollTimer);u.closeLayer();return true;}
 if(name==='market-choose'){const market=picker?.rows[Number(b.dataset.index)];if(!market)throw new Error(t("Котировка больше недоступна. Повторите поиск."));await u.call('carrier-trade',{values:{action:'market',leg:picker.leg,side:picker.side,market}});picker=null;clearTimeout(pollTimer);C.carrierTradeDraft=null;u.closeLayer();await u.loadView();u.toast(t("Котировка добавлена в рейс"));return true;}
 if(name==='help'){u.dialog(t("Как спланировать торговлю флотоносцем"),`<div class="prose-small"><p>${th("1. Выберите обычный рейс или Booze Cruise, затем загрузите конфигурацию флотоносца.")}</p><p>${th("2. Укажите объёмы и свежие рынки. Для Rackham’s Peak нужен S/M-перевозчик; большой корабль не сможет продать груз на станции.")}</p><p>${th("3. Рассчитайте маршрут флотоносца с этой загрузкой и импортируйте его для каждого плеча. Обратный пустой перелёт также расходует тритий.")}</p><p>${th("4. Укажите оценку трития и полную длительность с погрузкой, ожиданием, продажей и возвращением. При помощи других пилотов внесите оплату за тонну.")}</p><p>${th("5. Проверьте результат и замечания. Праздничные цены Booze Cruise и наличие Public Holiday не гарантированы.")}</p></div>`,u.btn(t("Закрыть"),'close-dialog'));return true;}
 if(name==='copy'){const m=model(C),s=m.summary;await u.call('native',{action:'copy',text:[t("Торговля флотоносцем"),...m.legs.map(leg=>`${leg.name}: ${leg.commodityName}, ${u.fmt(leg.effectiveQuantity)}${t(" т · ")}${leg.sourceSystem||'—'} → ${leg.destinationSystem||'—'}`),t("Результат владельца: ")+u.money(s.profit),t("Тритий за весь рейс: ")+u.fmt(s.fuelTons)+t(" т"),t("Расходы: ")+u.money(s.totalOperatingCosts),t("Полное время: ")+u.fmt(s.durationHours)+t(" ч"),t("Безубыточная продажа туда: ")+u.fmt(s.breakEvenSellPrice)+t(" кр./т"),t("Цены и спрос требуют проверки в игре.")].join('\r\n')});u.toast(t("План рейса скопирован"));return true;}
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
  add(t("Торговля флотоносцем: рабочая форма"),!!u.$('#carrier-trade-form')&&u.$$('[data-trade-option]').length>=13&&u.$$('[data-trade-leg="outbound"]').length>=10);
  const price=u.$('[data-trade-leg="outbound"][data-trade-field="buyPrice"]');
  if(price){price.value='';handleInput({target:price},C,u);await handle(prefix+'save',null,C,u);add(t("Торговля флотоносцем: пустая цена остаётся неизвестной"),model(C).legs.find(leg=>leg.id==='outbound')?.buyPrice===null);}else add(t("Торговля флотоносцем: пустая цена остаётся неизвестной"),false);
  const beforePreset=JSON.stringify(model(C).options);
  await handle(prefix+'preset',{dataset:{id:'booze-cruise'}},C,u);
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
  const legs=Object.fromEntries(original.legs.map(leg=>[leg.id,leg]));await u.call('carrier-trade',{values:{action:'configure',options:original.options,legs}});
  C.carrierTradeDraft=null;C.mode=previousMode;C.state.trading=originalTrading;await u.loadView();
 }
 return checks;
}
window.CarrierTradeUI={render,handle,handleInput,handleChange,init,smoke};
})();
