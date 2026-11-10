/* Trade routes use public market observations; Go owns quantity and profit math. */
(()=>{
'use strict';
const t=(value,vars)=>window.CopilotI18n?.t(value,vars)??String(value??'');
const th=(value,vars)=>t(value,vars).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const defaults={resourceKeys:['gold','silver','agronomictreatment','bertrandite','indite','bauxite'],radiusLy:100,maxLegLy:100,maxAgeHours:24,carrierMaxAgeHours:6,minutesPerLeg:15,operatingCostPerLeg:0,demandRatio:4,minSupply:1,minDemand:1,maxArrivalLs:10000,minPad:'L',marketType:'stations',includePlanetary:true,roundTrip:false,orderBy:'profitPerHour'};
const specs=[['cargoCapacity','Свободный трюм, т',1,25000,true],['credits','Бюджет закупки, кр.',0,1e15,false],['radiusLy','Радиус поиска, св. л.',5,5000,false],['maxLegLy','Длина плеча, св. л.',1,5000,false],['maxAgeHours','Возраст цен станций, ч',1,720,false],['carrierMaxAgeHours','Возраст цен флотоносцев, ч',.25,168,false],['minutesPerLeg','Всё плечо с торговлей, мин',1,1440,false],['operatingCostPerLeg','Прочие расходы на плечо, кр.',0,1e12,false],['minSupply','Минимальный запас, т',1,1e9,false],['minDemand','Минимальный спрос, т',1,1e9,false],['demandRatio','Запас спроса к грузу',1,20,false],['maxArrivalLs','От звезды не дальше, св. с.',0,1e7,false]];
const stationTypes={outpost:'Аванпост',planetaryoutpost:'Планетарный аванпост',crateroutpost:'Планетарный аванпост',planetaryport:'Планетарный порт',craterport:'Планетарный порт',planetarysettlement:'Планетарное поселение',settlement:'Поселение',onfootsettlement:'Планетарное поселение',odysseysettlement:'Планетарное поселение',coriolis:'Космопорт «Кориолис»',coriolisstarport:'Космопорт «Кориолис»',ocellus:'Космопорт «Оцеллус»',ocellusstarport:'Космопорт «Оцеллус»',orbis:'Космопорт «Орбис»',orbisstarport:'Космопорт «Орбис»',asteroidbase:'Астероидная база',fleetcarrier:'Флотоносец',megaship:'Мегакорабль'};
function stationType(raw){const text=String(raw??'').trim();return t(stationTypes[text.toLowerCase().replace(/[\s_-]+/g,'')]||text||"Тип неизвестен");}
const model=C=>C.state?.tradeRoutes||{};
const validRoute=row=>!!row&&typeof row==='object'&&!!row.id&&Array.isArray(row.legs)&&row.legs.length>=1&&row.legs.length<=2&&row.legs.every(leg=>leg&&typeof leg==='object'&&leg.key&&leg.buy&&typeof leg.buy==='object'&&leg.sell&&typeof leg.sell==='object'&&Number.isFinite(Number(leg.quantity))&&Number(leg.quantity)>0);
const resultRows=C=>(Array.isArray(model(C).results)?model(C).results:[]).filter(validRoute);
const savedRows=C=>(Array.isArray(C.state.savedTradeRoutes)?C.state.savedTradeRoutes:[]).filter(validRoute);
let latest;
function liveInputs(C) {
  const commander = C.state.commander || {};
  const capacity = commander.cargoCapacity == null || commander.cargoKnown === false ? null :
    Math.max(0, Math.floor(Number(commander.cargoCapacity) - Number(commander.cargoTons || 0)));
  return {
    system: String(commander.system || '').trim(),
    cargoCapacity: capacity,
    credits: C.state.pilot?.credits ?? null,
    minPad: commander.shipPad || defaults.minPad
  };
}
function local(C) {
  if (!C.dealsUI) {
    const observed = model(C).options || {};
    C.dealsUI = {
      filters: {...defaults, ...liveInputs(C), ...observed, resourceKeys: [...(observed.resourceKeys || defaults.resourceKeys)]},
      view: 'results', page: 0, query: '', selected: '', dirty: false, manual: false,
      starting: false, autoOrigin: '', attemptedAt: 0, autoError: ''
    };
  }
  return C.dealsUI;
}

const autoFreshnessMs = 10 * 60 * 1000;
const autoRetryMs = 2 * 60 * 1000;

function automaticInputs(C) {
  const observed = liveInputs(C);
  const issues = [];
  if (!observed.system) issues.push(t("Укажите исходную систему."));
  if (!Number.isFinite(observed.cargoCapacity)) issues.push(t("Укажите свободный трюм."));
  else if (observed.cargoCapacity < 1) issues.push(t("В трюме нет свободного места."));
  if (observed.credits == null || !Number.isFinite(Number(observed.credits))) issues.push(t("Укажите бюджет закупки."));
  else if (Number(observed.credits) <= 0) issues.push(t("Нет средств на закупку."));
  return {observed, issues};
}

function automaticRequest(C) {
  const l = local(C), m = model(C);
  if (window.COPILOT_SMOKE || C.tab !== 'deals' || l.view !== 'results' || l.query || l.manual ||
      l.dirty || l.starting || m.running) return null;
  const {observed, issues} = automaticInputs(C);
  if (issues.length) return null;
  const origin = observed.system.toLocaleLowerCase('en') + '|' + observed.minPad;
  const date = Date.parse(m.updatedAt || '');
  const fresh = Number.isFinite(date) && date <= Date.now() && Date.now() - date < autoFreshnessMs;
  // Keep useful recent results, but don't let a previous empty/manual search
  // disable suggestions on the first visit in a new application session.
  if (m.options?.automatic !== true && fresh && resultRows(C).length &&
      String(m.options?.system || '').toLocaleLowerCase('en') === observed.system.toLocaleLowerCase('en')) return null;
  if (m.options?.automatic && Object.entries(observed).every(([field, value]) => String(m.options[field]) === String(value)) &&
      fresh) return null;
  if (l.autoOrigin === origin && Date.now() - l.attemptedAt < autoRetryMs) return null;
  return {origin, values: {...defaults, ...observed, automatic: true}};
}

async function startAutomatic(C, u) {
  const request = automaticRequest(C);
  if (!request) return;
  const l = local(C);
  l.starting = true;
  l.autoOrigin = request.origin;
  l.attemptedAt = Date.now();
  l.autoError = '';
  l.filters = {...request.values, resourceKeys: [...request.values.resourceKeys]};
  l.page = 0;
  try {
    await u.call('deals-search', {values: request.values});
    if (C.tab === 'deals' && l.view === 'results') await u.loadView();
  } catch (error) {
    l.autoError = error.message || t("Не удалось подобрать рейсы.");
  } finally {
    l.starting = false;
    if (C.tab === 'deals' && l.view === 'results' && l.autoError) u.renderPage();
  }
}
const btn=(u,label,action,icon='',style='',attrs='')=>u.btn(label,'deals-'+action,icon,style,'type="button" '+attrs);
function field(u,key,label,value,extra=''){return `<label class="deals-field"><span>${u.esc(t(label))}</span><input type="text" inputmode="${key==='system'?'text':'decimal'}" data-deals-filter="${key}" value="${u.esc(value??'')}" placeholder="${key==='system'?t("Название системы"):t("Укажите значение")}" autocomplete="off" ${extra}></label>`;}
function select(u,key,label,value,choices){return `<label class="deals-field"><span>${u.esc(t(label))}</span><select data-deals-filter="${key}">${choices.map(([id,title])=>`<option value="${u.esc(id)}" ${String(value)===id?'selected':''}>${u.esc(title)}</option>`).join('')}</select></label>`;}
function searchForm(C,u){const l=local(C),f=l.filters,goods=new Map();for(const good of [...(C.state.goods||[]),...(C.state.resources||[])])if(good.key&&good.kind!=='raw'&&good.key!=='drones')goods.set(good.key,good.name||good.key);for(const key of f.resourceKeys)if(!goods.has(key))goods.set(key,key);const choices=[['',t("Не искать")],...[...goods].sort((a,b)=>a[1].localeCompare(b[1],window.CopilotI18n?.locale()||'ru-RU'))];return `<details class="panel deals-search" ${!resultRows(C).length&&!model(C).running&&(automaticInputs(C).issues.length||l.dirty||l.autoError||(model(C).results||[]).length)?'open':''}><summary>${u.icon('search')}${th("Условия поиска")}<span>${u.esc(f.system||t("Укажите систему"))} · ${u.esc(u.fmt(f.cargoCapacity))}${th(" т · ")}${f.roundTrip?t("Туда и обратно"):t("Одно плечо")}</span>${u.icon('chevron-down')}</summary><form id="deals-search-form" novalidate><div class="deals-primary">${field(u,'system',t("Рядом с системой"),f.system)}${field(u,'cargoCapacity',t("Свободный трюм, т"),f.cargoCapacity)}${field(u,'credits',t("Бюджет закупки, кр."),f.credits)}${select(u,'minPad',t("Площадка вашего корабля"),f.minPad,[['L',t("L · большая")],['M',t("M · средняя")],['S',t("S · малая")]])}</div><div class="deals-quick"><label><input type="checkbox" data-deals-filter="roundTrip" ${f.roundTrip?'checked':''}>${th("Найти обратный товар A → B → A")}</label>${btn(u,t("Взять текущий корабль и баланс"),'current','ship','quiet')}</div><div class="deals-commodity-grid">${Array.from({length:6},(_,i)=>select(u,'resource'+i,t("Товар ")+(i+1),f.resourceKeys[i]||'',choices)).join('')}</div><details class="deals-more"><summary>${th("Ограничения рынков, расходы и оценка времени")}</summary><div class="deals-advanced">${specs.slice(2).map(([key,label])=>field(u,key,label,f[key])).join('')}${select(u,'marketType',t("Типы рынков"),f.marketType,[['stations',t("Только станции")],['all',t("Станции и флотоносцы")],['carriers',t("Только флотоносцы")]])}${select(u,'orderBy',t("Сортировать результаты"),f.orderBy,[['profitPerHour',t("Прибыль за час")],['profit',t("Прибыль за рейс")],['distance',t("Короче перелёт")],['purchaseCost',t("Меньше начальный капитал")]])}</div><label class="deals-check"><input type="checkbox" data-deals-filter="includePlanetary" ${f.includePlanetary?'checked':''}>${th("Включать планетарные порты")}</label><p class="info-note">${th("Время одного плеча; перелёт до первой закупки не входит.")}</p></details><div id="deals-form-error" class="notice hidden" role="alert"></div><div class="deals-search-footer">${btn(u,model(C).running?t("Перезапустить поиск"):t("Обновить рейсы"),'search','search','primary')}</div></form></details>`;}
function age(station){const at=Date.parse(station.observedAt||station.sourceUpdatedAt||station.updatedAt||'');return Number.isFinite(at)&&at<=Date.now()+600000?Math.max(0,(Date.now()-at)/3600000):null;}
function observation(station,u){const raw=station.observedAt||station.sourceUpdatedAt||station.updatedAt,date=new Date(raw||'');if(!raw||!Number.isFinite(date.getTime()))return `<span class="deals-unverified">${th("Время цены неизвестно")}</span>`;if(date.getTime()>Date.now()+600000)return `<span class="deals-unverified">${th("Время котировки в будущем")}</span>`;return `<time datetime="${u.esc(raw)}" title="${u.esc(date.toLocaleString(window.CopilotI18n?.locale()||'ru-RU')+t(" · местное время"))}">${u.esc(date.toLocaleString(window.CopilotI18n?.locale()||'ru-RU',{day:'2-digit',month:'2-digit',hour:'2-digit',minute:'2-digit'}))} · ${u.esc(u.fmt(age(station)))}${th(" ч назад")}</time>`;}
function endpoint(C,u,route,leg,index,side){const station=leg[side]||{},buy=side==='buy',at=age(station),limits=route.options||model(C).options||local(C).filters,limit=Number((station.carrier?limits.carrierMaxAgeHours:limits.maxAgeHours)??(station.carrier?defaults.carrierMaxAgeHours:defaults.maxAgeHours)),stale=at==null||at>limit;return `<section class="deals-endpoint"><div class="deals-endpoint-kind"><span>${buy?t("Закупка"):t("Продажа")}</span>${u.badge(station.pad?t("Площадка ")+station.pad:t("Площадка неизвестна"),station.pad?'':'warn')}${station.carrier?u.badge(t("Флотоносец"),'warn'):''}</div><div class="deals-system"><strong>${u.esc(station.system||t("Система неизвестна"))}</strong>${btn(u,'','copy','copy','icon-only',`data-id="${u.esc(route.id)}" data-leg="${index}" data-side="${side}" data-copy="system" title="${th("Копировать систему ")}${buy?t("закупки"):t("продажи")}" aria-label="${th("Копировать систему ")}${buy?t("закупки"):t("продажи")}" ${!station.system?'disabled':''}`)}</div><div class="deals-station"><span>${u.esc(station.station||t("Станция неизвестна"))}</span>${btn(u,'','copy','copy','icon-only',`data-id="${u.esc(route.id)}" data-leg="${index}" data-side="${side}" data-copy="station" title="${th("Копировать название станции")}" aria-label="${th("Копировать название станции")}" ${!station.station?'disabled':''}`)}</div><div class="deals-station-meta"><span>${u.esc(stationType(station.type))}${station.planetary?t(" · на поверхности"):''}</span><span>${th("От звезды: ")}<b>${u.esc(u.fmt(station.distanceLs))}${th(" св. с.")}</b></span></div><div class="deals-quote"><div><span>${buy?t("Покупаем за"):t("Продаём за")}</span><strong>${u.esc(u.fmt(leg[buy?'buyPrice':'sellPrice']))} <small>${th("кр./т")}</small></strong></div><div><span>${buy?t("Запас"):t("Спрос")}</span><strong>${u.esc(u.fmt(leg[buy?'supply':'demand']))} <small>${th("т")}</small></strong></div></div><div class="deals-observed ${stale?'stale':''}">${u.icon('clock')}${observation(station,u)}${stale?`<span>${th("Проверьте перед рейсом")}</span>`:''}</div></section>`;}
function legCard(C,u,route,leg,index){const factors={cargo:t("свободный трюм"),supply:t("запас станции"),safeDemand:t("спрос с запасом"),capital:t("бюджет")},reasons=(leg.limitingFactors||[]).map(x=>factors[x]||x);return `<section class="deals-leg"><header><span class="deals-leg-number">0${index+1}</span><h3>${u.esc(leg.name||leg.key)}<span>${index===0?t("Туда"):t("Обратно")} · ${u.esc(u.fmt(leg.quantity))}${th(" т")}</span></h3><div class="deals-leg-profit"><strong>${leg.netEstimate!=null&&Number(leg.netEstimate)>0?'+':''}${u.esc(u.money(leg.netEstimate))}</strong><span>${th("После расходов плеча")}</span></div></header><div class="deals-endpoints">${endpoint(C,u,route,leg,index,'buy')}<div class="deals-between">${u.icon('arrow-right')}<span>${u.esc(u.fmt(leg.distanceLy))}<small>${th("св. л.")}</small></span></div>${endpoint(C,u,route,leg,index,'sell')}</div><div class="deals-leg-math"><span>${th("Маржа: ")}<b>${u.esc(u.fmt(leg.profitPerTonne))}${th(" кр./т")}</b> · ${u.esc(u.fmt(leg.marginPercent))}%</span><span>${th("Закупка: ")}<b>${u.esc(u.money(leg.purchaseCost))}</b></span><span>${th("Расходы: ")}<b>${u.esc(u.money(leg.operatingCost))}</b></span><span>${th("Спрос с запасом: ")}<b>${u.esc(u.fmt(leg.safeDemand))}${th(" т")}</b></span></div>${reasons.length?`<div class="deals-volume-note">${u.icon('package')}${th("Объём ограничивает: ")}${u.esc(reasons.join(', '))}.</div>`:''}<div class="deals-leg-actions">${btn(u,t("Назначить покупателя"),'destination','target','',`data-id="${u.esc(route.id)}" data-leg="${index}"`)}${btn(u,t("Сохранить план плеча"),'plan','check','quiet',`data-id="${u.esc(route.id)}" data-leg="${index}"`)}</div></section>`;}
function routeCard(C,u,route,index){const l=local(C),legs=route.legs||[],first=legs[0]||{},selected=l.selected===route.id,approach=route.approachDistanceLy??first.approachDistanceLy;return `<article class="panel deals-route ${selected?'selected':''}" data-deals-route="${u.esc(route.id)}"><header class="deals-route-header"><div class="deals-route-title"><span class="deals-result-number">${String(index+1).padStart(2,'0')}</span><div><h2>${u.esc(first.buy?.system||t("Закупка"))} ${route.roundTrip||legs.length>1?'↔':'→'} ${u.esc(first.sell?.system||t("Продажа"))}</h2><p>${l.view==="saved"?u.esc(savedRouteLabel(route))+" · ":""}${legs.length>1?t("Два плеча с обратным грузом"):t("Односторонний торговый рейс")}${approach==null?'':`${t(" · до первой станции ")}${u.esc(u.fmt(approach))}${t(" св. л.")}`}</p></div></div><div class="deals-route-total"><span>${th("Прибыль за ")}${legs.length>1?t("полный круг"):t("плечо")}</span><strong>${u.esc(u.money(route.netEstimate))}</strong></div></header><div class="deals-route-metrics"><div><span>${th("За час · по вашему времени")}</span><strong>${u.esc(u.money(route.profitPerHour))}</strong></div><div><span>${th("Время ")}${legs.length>1?t("обоих плеч"):t("плеча")}</span><strong>${u.esc(u.fmt(route.minutes))}${th(" мин")}</strong></div><div><span>${th("Начальный капитал")}</span><strong>${u.esc(u.money(route.minimumCapital))}</strong></div><div><span>${th("Перелёт по маршруту")}</span><strong>${u.esc(u.fmt(route.distanceLy??first.distanceLy))}${th(" св. л.")}</strong></div></div>${legs.map((leg,i)=>legCard(C,u,route,leg,i)).join('')}${(route.warnings||[]).length?`<div class="deals-warnings">${route.warnings.map(text=>`<p>${u.icon('circle-help')}${u.esc(text)}</p>`).join('')}</div>`:''}<footer>${btn(u,selected?t("Рейс выбран"):t("Выбрать рейс"),'select','check',selected?'primary':'quiet',`data-id="${u.esc(route.id)}"`)}<span class="spacer"></span>${l.view==='saved'?savedRouteControls(C,u,route):btn(u,t("Сохранить весь рейс"),'save','bookmark','quiet',`data-id="${u.esc(route.id)}"`)}</footer></article>`;}
function routeList(C){const l=local(C),rows=l.view==='saved'?savedRows(C):resultRows(C),query=l.query.trim().toLocaleLowerCase('ru');return query?rows.filter(route=>JSON.stringify((route.legs||[]).map(leg=>[leg.name,leg.key,leg.buy?.station,leg.buy?.system,leg.sell?.station,leg.sell?.system])).toLocaleLowerCase('ru').includes(query)):rows;}
function activePlan(C,u){const plan=C.state.tradePlan;if(!plan?.key)return '';const good=(C.state.goods||[]).find(row=>row.key===plan.key),name=plan.name||good?.name||plan.key;const point=(side,label,legacy)=>{const market=plan[side]||{},system=market.system||(typeof plan[legacy]==='string'?plan[legacy]:plan[legacy]?.system)||t("Система неизвестна");return `<div><span>${label}</span><strong>${u.esc(system)}</strong><small>${u.esc(market.station||t("Станция не указана"))}</small><b>${u.esc(u.fmt(plan[side+'Price']))}${th(" кр./т")}</b></div>`;};return `<details class="panel deals-active-plan" ${resultRows(C).length?'':'open'}><summary>${u.icon('circle-check')}<strong>${th("Текущий план перевозки")}</strong><span>${u.esc(name)} · ${u.esc(u.fmt(plan.quantity))}${th(" т")}</span>${u.icon('chevron-down')}</summary><div class="deals-active-points">${point('buy',t("Закупка"),'source')}<span class="deals-active-arrow">${u.icon('arrow-right')}</span>${point('sell',t("Продажа"),'destination')}${btn(u,t("Убрать план"),'plan-clear','x','quiet')}</div></details>`;}
function render(C, u) {
  const l = local(C), m = model(C), rows = routeList(C);
  const pages = Math.max(1, Math.ceil(rows.length / 10));
  l.page = Math.max(0, Math.min(l.page, pages - 1));
  const showingResults = l.view === 'results';
  const workspace = C.state.planWorkspace;
  const owner = !showingResults && workspace?.name ?
    workspace.name + (workspace.branch === 'legacy' ? ' · Legacy' : workspace.branch === 'live' ? ' · Live' : '') : '';
  const loading = showingResults && (m.running || l.starting || automaticRequest(C));
  const {issues} = automaticInputs(C);
  const error = showingResults && (l.autoError || m.error);
  const noRowsTitle = l.query ? t("Совпадений нет") :
    showingResults ? t("Рейсы не найдены") : t("Нет сохранённых рейсов");
  const noRowsHint = showingResults && !l.query && !m.updatedAt && issues.length ?
    `<p>${u.esc(issues.join(' '))}</p>` : '';
  const cards = rows.slice(l.page * 10, l.page * 10 + 10)
    .map((route, index) => routeCard(C, u, route, l.page * 10 + index)).join('');
  const empty = loading ? '' : `<div class="panel deals-empty">${u.icon(showingResults ? 'route' : 'bookmark')}
    <h2>${u.esc(noRowsTitle)}</h2>${noRowsHint}
    ${l.query ? btn(u, t("Очистить фильтр"), 'clear-query', 'x', 'quiet') : ''}</div>`;
  return `<div class="deals-page">
    ${u.heading(t("Торговые рейсы"), owner, m.running ? btn(u, t("Остановить"), 'cancel', 'x', 'quiet') : '')}
    ${searchForm(C,u)}${activePlan(C,u)}
    <div class="deals-listbar"><div class="segmented">
      <button data-click="deals-view" data-view="results" class="${showingResults ? 'selected' : ''}">${th("Результаты ")}<span>${resultRows(C).length}</span></button>
      <button data-click="deals-view" data-view="saved" class="${showingResults ? '' : 'selected'}">${th("Сохранённые ")}<span>${savedRows(C).length}</span></button>
    </div><span class="spacer"></span>
    <input id="deals-query" placeholder="${th("Фильтр по системе, станции, товару")}" value="${u.esc(l.query)}" aria-label="${th("Фильтр результатов")}"></div>
    ${loading ? `<div class="deals-progress" role="status">${u.icon('refresh-cw')}${th("Подбираем выгодные рейсы…")}
      ${m.total ? `<span>${u.esc(m.checked || 0)} / ${u.esc(m.total)}</span>` : ''}</div>` : ''}
    ${error ? `<div class="notice" role="alert">${u.esc(t(error))}</div>` : ''}
    ${showingResults && (m.results?.length || 0) > resultRows(C).length ? `<div class="deals-coverage">${th("Требуется повторный поиск.")}</div>` : ''}
    ${showingResults && m.coverageLimited ? `<div class="deals-coverage">${th("Ограниченная выборка рынков")}</div>` : ''}
    <div class="deals-results">${cards || empty}</div>
    ${pages > 1 ? `<div class="deals-pager">${btn(u,t("Назад"),'prev','chevron-left','quiet',l.page===0?'disabled':'')}
      <span>${l.page+1} / ${pages} · ${rows.length}${th(" рейсов")}</span>
      ${btn(u,t("Далее"),'next','chevron-right','quiet',l.page===pages-1?'disabled':'')}</div>` : ''}
    ${rows.length ? `<details class="deals-disclaimer"><summary>${th("Расчёт прибыли")}</summary>
      <p>${th("Прибыль = выручка − закупка − расходы рейса. Время задаётся в настройках.")}</p></details>` : ''}
  </div>`;
}
function capture(C,u){const f=local(C).filters;for(const node of u.$$('[data-deals-filter]')){const key=node.dataset.dealsFilter,value=node.type==='checkbox'?node.checked:node.value;if(/^resource\d$/.test(key))f.resourceKeys[Number(key.slice(8))]=value;else f[key]=value;}return f;}
function payload(C,u){const f={...capture(C,u)},out={...f,resourceKeys:[...new Set(f.resourceKeys.filter(Boolean))]};if(!out.system?.trim())throw new Error(t("Укажите исходную систему поиска."));out.system=out.system.trim();if(!out.resourceKeys.length)throw new Error(t("Выберите хотя бы один товар."));for(const [key,label,min,max,integer]of specs){const text=String(f[key]??'').trim().replace(/[\s\u00a0\u202f]/g,'').replace(',','.');if(!/^\d+(?:\.\d+)?$/.test(text))throw new Error(t(label)+t(": укажите число."));const value=Number(text);if(!Number.isFinite(value)||value<min||value>max||integer&&!Number.isInteger(value))throw new Error(t(label)+t(": значение вне допустимого диапазона."));out[key]=value;}out.includeCarriers=out.marketType!=='stations';return out;}
function find(C,id){return [...resultRows(C),...savedRows(C)].find(route=>route.id===id);}
function savedRouteLabel(route) {
  return route.scope === 'personal' ? t("Личный рейс") : t("Общий рейс");
}
function savedRouteControls(C, u, route) {
  const attrs = `data-id="${u.esc(route.id)}"`;
  const next = route.scope === 'personal' ? 'shared' : 'personal';
  const change = C.state.planWorkspace?.key ?
    btn(u,next==='shared'?t("Сделать общим"):t("Сделать личным"),'scope','bookmark','quiet',attrs) : '';
  return change + btn(u,t("Удалить сохранённый"),'delete','trash-2','quiet',attrs);
}
function savedRouteRequest(C, values) {
  const owner = C.state.planWorkspace;
  return owner ? {...values,expectedPlanWorkspace:owner.key} : values;
}
async function handle(action,b,C,u){if(!action.startsWith('deals-'))return false;const l=local(C),name=action.slice(6);if(name==='search'){let values;try{values=payload(C,u);}catch(error){const box=u.$('#deals-form-error');if(box){box.textContent=error.message;box.classList.remove('hidden');}throw error;}l.manual=true;l.autoError='';await u.call('deals-search',{values:{...values,automatic:false}});l.view='results';C.mode='Поиск';l.page=0;l.selected='';l.dirty=false;await u.loadView();return true;}if(name==='plan-clear'){u.dialog(t("Убрать текущий план перевозки?"),`<p>${th("План плеча исчезнет из текущих задач. Сохранённые маршруты и история сделок останутся.")}</p>`,u.btn(t("Отмена"),'close-dialog')+btn(u,t("Убрать план"),'plan-clear-confirm','x','primary'));return true;}if(name==='plan-clear-confirm'){await u.call('deals-plan-clear');u.closeLayer();await u.loadView();u.toast(t("Текущий план перевозки убран"));return true;}if(name==='cancel'){l.manual=true;await u.call('deals-cancel');await u.loadView();u.toast(t("Поиск остановлен"));return true;}if(name==='current'){for(const [key,value] of Object.entries(liveInputs(C)))if(value!==null&&value!=='')l.filters[key]=value;l.dirty=true;u.renderPage();return true;}if(name==='view'){l.view=b.dataset.view==='saved'?'saved':'results';C.mode=l.view==='saved'?'Сохранённые':'Поиск';C.selected='';l.page=0;await u.loadView();return true;}if(name==='prev'||name==='next'){l.page+=name==='next'?1:-1;u.renderPage();return true;}if(name==='clear-query'){l.query='';l.page=0;u.renderPage();return true;}const route=find(C,b?.dataset.id);if(!route)throw new Error(t("Рейс больше не найден. Обновите результаты."));if(name==='select'){l.selected=route.id;C.selected=route.id;await u.loadView();u.toast(t("Рейс выбран. Назначьте покупателя нужного плеча или сохраните план."));return true;}if(name==='save'){await u.call('deals-saved-route',{values:savedRouteRequest(C,{action:'save-route',route:{...route,options:route.options||model(C).options||{}}})});await u.loadView();u.toast(t("Полный рейс сохранён"));return true;}if(name==='scope'){const scope=route.scope==='personal'?'shared':'personal';await u.call('deals-saved-route',{values:savedRouteRequest(C,{action:'set-scope',id:route.id,scope})});await u.loadView();u.toast(t("Владелец рейса изменён"));return true;}if(name==='delete'){u.dialog(t("Удалить сохранённый рейс?"),`<p>${th("Он исчезнет из списка сохранённых. Рыночные предложения останутся.")}</p>`,u.btn(t("Отмена"),'close-dialog')+btn(u,t("Удалить"),'delete-confirm','trash-2','primary',`data-id="${u.esc(route.id)}"`));return true;}if(name==='delete-confirm'){await u.call('deals-saved-route',{values:savedRouteRequest(C,{action:'delete-route',id:route.id})});u.closeLayer();await u.loadView();u.toast(t("Сохранённый рейс удалён"));return true;}const leg=(route.legs||[])[Number(b.dataset.leg)];if(!leg)throw new Error(t("Выберите плечо рейса."));if(name==='copy'){const station=leg[b.dataset.side]||{},key=b.dataset.copy==='station'?'station':'system';if(!station[key])throw new Error(t("Название неизвестно."));await u.call('native',{action:'copy',text:station[key]});u.toast(key==='system'?t("Система скопирована"):t("Название станции скопировано"));return true;}if(name==='destination'){await u.call('deals-destination',{values:{destination:{...leg.sell,selectionMode:'sell',quoteOptions:route.options||model(C).options||{}}}});await u.loadView();u.toast(t("Покупатель назначен для оверлея"));return true;}if(name==='plan'){await u.call('deals-plan',{values:{key:leg.key,quantity:leg.quantity,buyPrice:leg.buyPrice,sellPrice:leg.sellPrice,source:leg.buy?.system||'',destination:leg.sell?.system||'',buy:leg.buy,sell:leg.sell}});await u.loadView();u.toast(t("План плеча сохранён"));return true;}return false;}
function handleInput(event,C,u){const node=event.target;if(node?.id!=='deals-query'&&!node?.matches?.('[data-deals-filter]'))return false;const l=local(C);if(node?.id==='deals-query'){const start=node.selectionStart;l.query=node.value;l.page=0;u.renderPage();const next=u.$('#deals-query');next?.focus();try{next?.setSelectionRange(start,start);}catch(_){}return true;}const key=node.dataset.dealsFilter,value=node.type==='checkbox'?node.checked:node.value;if(/^resource\d$/.test(key))l.filters.resourceKeys[Number(key.slice(8))]=value;else l.filters[key]=value;l.dirty=true;return true;}
async function handleChange(event,C,u){return handleInput(event,C,u);}
function init(C, u) {
  latest = {C, u};
  if (C.mode === 'Сохранённые') local(C).view = 'saved';
  else if (C.mode === 'Поиск') local(C).view = 'results';
  if (typeof document !== 'undefined' && !init.bound) {
    init.bound = true;
    document.addEventListener('submit', event => {
      if (event.target.id !== 'deals-search-form' || !latest) return;
      event.preventDefault();
      handle('deals-search', null, latest.C, latest.u).catch(error => latest.u.toast(error.message, true));
    });
  }
  return startAutomatic(C, u);
}
async function smoke(C, u) {
  const html = render(C, u);
  return [
    {name: t("Рейсы: отдельная форма и выбор площадки"), ok: html.includes('deals-search-form') && html.includes('data-deals-filter="minPad"')},
    {name: t("Рейсы: бюджет, время и реальные ограничения"), ok: ['credits', 'maxAgeHours', 'roundTrip'].every(key => html.includes('data-deals-filter="' + key + '"'))},
    {name: t("Рейсы: сохранённые маршруты доступны"), ok: html.includes('data-view="saved"')}
  ];
}
window.DealsUI={render,handle,handleInput,handleChange,init,smoke};
})();
