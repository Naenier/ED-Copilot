/* Trade routes use public market observations; Go owns quantity and profit math. */
(()=>{
'use strict';
const t=(value,vars)=>window.CopilotI18n?.t(value,vars)??String(value??'');
const th=(value,vars)=>t(value,vars).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const defaults={system:'',cargoCapacity:null,maxDistance:20,maxAgeHours:24,minPad:'L',maxArrivalLs:1000,allowSurface:false,
  minSupply:10000,minDemand:10000,roundTrip:true,sourcePower:'',sourcePowerState:'',targetPower:'',targetPowerState:'',orderBy:'profit'};
const specs=[
  ['cargoCapacity','Вместимость трюма, т',1,25000,true],
  ['maxDistance','Макс. дальность маршрута, св. л.',.01,5000,false],
  ['maxAgeHours','Макс. возраст цен, ч',1,720,false],
  ['maxArrivalLs','Макс. расстояние до станции, св. с.',0,1e7,false],
  ['minSupply','Минимальный запас, т',0,1e9,true],
  ['minDemand','Минимальный спрос, т',0,1e9,true]
];
const powerNames=['Aisling Duval','Archon Delaine','Arissa Lavigny-Duval','Denton Patreus','Edmund Mahon','Felicia Winters',
  'Jerome Archer','Li Yong-Rui','Nakato Kaine','Pranav Antal','Yuri Grom','Zemina Torval'];
const powerStates=[
  ['Unoccupied','Незанятая'],['Exploited','Эксплуатируемая'],['Fortified','Укреплённая'],['Stronghold','Оплот'],['Contested','Оспариваемая']
];
const sortLabels=[['profit','Прибыль за рейс'],['profit-per-ton','Прибыль за тонну'],['margin','Рентабельность'],['distance','Длина маршрута']];
const stationTypes={outpost:'Аванпост',planetaryoutpost:'Планетарный аванпост',crateroutpost:'Планетарный аванпост',planetaryport:'Планетарный порт',craterport:'Планетарный порт',planetarysettlement:'Планетарное поселение',settlement:'Поселение',onfootsettlement:'Планетарное поселение',odysseysettlement:'Планетарное поселение',coriolis:'Космопорт «Кориолис»',coriolisstarport:'Космопорт «Кориолис»',ocellus:'Космопорт «Оцеллус»',ocellusstarport:'Космопорт «Оцеллус»',orbis:'Космопорт «Орбис»',orbisstarport:'Космопорт «Орбис»',asteroidbase:'Астероидная база',fleetcarrier:'Флотоносец',megaship:'Мегакорабль'};
function stationType(raw){const text=String(raw??'').trim();return t(stationTypes[text.toLowerCase().replace(/[\s_-]+/g,'')]||text||"Тип неизвестен");}
const model=C=>C.state?.tradeRoutes||{};
const validRoute=row=>!!row&&typeof row==='object'&&!!row.id&&Array.isArray(row.legs)&&row.legs.length>=1&&row.legs.length<=2&&row.legs.every(leg=>leg&&typeof leg==='object'&&leg.key&&leg.buy&&typeof leg.buy==='object'&&leg.sell&&typeof leg.sell==='object'&&Number.isFinite(Number(leg.quantity))&&Number(leg.quantity)>0);
const resultRows=C=>(Array.isArray(model(C).results)?model(C).results:[]).filter(validRoute);
const savedRows=C=>(Array.isArray(C.state.savedTradeRoutes)?C.state.savedTradeRoutes:[]).filter(validRoute);
let latest;
function liveInputs(C) {
  const commander = C.state.commander || {};
  const capacity = finite(commander.cargoCapacity) ?
    Math.max(0, Math.floor(Number(commander.cargoCapacity))) : null;
  return {
    system: String(commander.system || '').trim(),
    cargoCapacity: capacity,
    minPad: commander.shipPad || defaults.minPad
  };
}
function local(C) {
  if (!C.dealsUI) {
    const observed = model(C).options || {};
    C.dealsUI = {
      filters: {...defaults, ...liveInputs(C), ...Object.fromEntries(Object.keys(defaults)
        .filter(key => observed[key] != null).map(key => [key, observed[key]]))},
      view: C.mode==='Сохранённые'?'saved':'results', page: 0, query: '', selected: '', dirty: false, manual: false,
      starting: false, autoOrigin: '', attemptedAt: 0, autoError: ''
    };
  }
  if(C.mode==='Сохранённые') C.dealsUI.view='saved';
  else if(C.mode==='Поиск'&&C.dealsUI.view==='saved') C.dealsUI.view='results';
  return C.dealsUI;
}

const autoFreshnessMs = 10 * 60 * 1000;
const autoRetryMs = 2 * 60 * 1000;

function automaticInputs(C) {
  const observed = liveInputs(C);
  const issues = [];
  if (!observed.system) issues.push(t("Укажите исходную систему."));
  if (!Number.isFinite(observed.cargoCapacity)) issues.push(t('Укажите вместимость трюма.'));
  else if (observed.cargoCapacity < 1) issues.push(t('У корабля нет грузового трюма.'));
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
  l.filters = {...request.values};
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
function field(u,key,label,value) {
  return `<label class="deals-field"><span>${th(label)}</span><input type="text" inputmode="${key==='system'?'text':'decimal'}"
    data-deals-filter="${key}" value="${u.esc(value??'')}" placeholder="${th(key==='system'?'Название системы':'Укажите значение')}" autocomplete="off"></label>`;
}
function select(u,key,label,value,choices) {
  const options=choices.map(([id,title])=>`<option value="${u.esc(id)}"${String(value)===String(id)?' selected':''}>${th(title)}</option>`);
  return `<label class="deals-field"><span>${th(label)}</span><select data-deals-filter="${key}">${options.join('')}</select></label>`;
}
function numberChoices(u,value,presets,unit,special='') {
  const number=Number(value);
  const amounts=[...new Set([...presets,...(finite(value)?[number]:[])])].sort((a,b)=>a-b);
  return amounts.map(amount=>[amount,amount===0&&special?t(special):units(u,amount,unit)]);
}
function searchForm(C,u) {
  const f=local(C).filters, m=model(C);
  const powerChoices=[['','Любая сила'],['none','Без контроля силы'],...powerNames.map(power=>[power,power])];
  const stateChoices=[['','Любое состояние'],...powerStates];
  const padChoices=[['L','L · большая'],['M','M · средняя'],['S','S · малая']];
  const surfaceChoices=[['false','Только орбитальные'],['true','Включая станции на поверхности']];
  const numberField=(index,presets,unit,special)=>{
    const [key,label]=specs[index];
    return select(u,key,label,f[key],numberChoices(u,f[key],presets,unit,special));
  };
  return `<form id="deals-search-form" class="panel deals-search" novalidate>
    <div class="deals-form-grid">
      <div>${field(u,'system','Рядом с системой',f.system)}${field(u,'cargoCapacity',specs[0][1],f.cargoCapacity)}</div>
      <div>${numberField(1,[5,10,15,20,30,40,50,75,100,150,200,300,500],' св. л.')}
        ${numberField(2,[1,3,6,12,24,48,72,168,336,720],' ч')}</div>
    </div>
    <fieldset class="deals-form-group">
      <legend>${th('Свойства станции')}</legend>
      <div class="deals-form-grid">
        <div>${select(u,'minPad','Минимальная площадка',f.minPad,padChoices)}
          ${numberField(3,[0,100,250,500,1000,2000,5000,10000,25000,50000,100000],' св. с.','Без ограничения')}
          ${select(u,'allowSurface','Наземные станции',f.allowSurface,surfaceChoices)}</div>
        <div>${numberField(4,[0,100,500,1000,2500,5000,10000,20000,50000,100000],' т','Любой объём')}
          ${numberField(5,[0,100,500,1000,2500,5000,10000,20000,50000,100000],' т','Любой объём')}
          <label class="deals-check"><input type="checkbox" data-deals-filter="roundTrip"${f.roundTrip?' checked':''}>${th('Включать рейсы туда и обратно')}</label></div>
      </div>
    </fieldset>
    <fieldset class="deals-form-group">
      <legend>${th('Политика')}</legend>
      <div class="deals-form-grid">
        <div>${select(u,'sourcePower','Сила в системе отправления',f.sourcePower,powerChoices)}
          ${select(u,'sourcePowerState','Статус Powerplay в системе отправления',f.sourcePowerState,stateChoices)}</div>
        <div>${select(u,'targetPower','Сила в системе назначения',f.targetPower,powerChoices)}
          ${select(u,'targetPowerState','Статус Powerplay в системе назначения',f.targetPowerState,stateChoices)}</div>
      </div>
    </fieldset>
    ${select(u,'orderBy','Сортировать по',f.orderBy,sortLabels)}
    <div id="deals-form-error" class="notice hidden" role="alert"></div>
    <button type="submit" class="pill primary deals-submit">${u.icon('search')}${th(m.running?'Перезапустить поиск':'Найти торговые маршруты')}</button>
  </form>`;
}
const quoteTimestamp=station=>station.observedAt||station.sourceUpdatedAt||station.updatedAt||'';
const finite=value=>value!==''&&value!=null&&Number.isFinite(Number(value));
const units=(u,value,unit)=>finite(value)?`${u.esc(u.fmt(value))}${th(unit)}`:'—';
const exactMoney=(u,value,unit=' кр.')=>units(u,value,unit);
function observation(station,u) {
  const raw=quoteTimestamp(station), stamp=Date.parse(raw), now=Date.now();
  if(!Number.isFinite(stamp)) return `<span class="deals-unverified">${th('Время цены неизвестно')}</span>`;
  if(stamp>now+600000) return `<span class="deals-unverified">${th('Время котировки в будущем')}</span>`;
  const hours=Math.max(0,(now-stamp)/3600000);
  const locale=window.CopilotI18n?.locale()||'ru-RU';
  const age=hours>=48?units(u,Math.floor(hours/24),' д назад'):
    hours>=1?units(u,Math.floor(hours),' ч назад'):units(u,Math.floor(hours*60),' мин назад');
  return `<time datetime="${u.esc(raw)}" title="${u.esc(new Date(stamp).toLocaleString(locale)+t(' · местное время'))}">${age}</time>`;
}
function routeObservation(route,u) {
  const timestamps=route.legs.flatMap(leg=>[quoteTimestamp(leg.buy),quoteTimestamp(leg.sell)]);
  const oldest=timestamps.every(at=>Number.isFinite(Date.parse(at))&&Date.parse(at)<=Date.now()+600000)?
    timestamps.reduce((a,b)=>Date.parse(a)<Date.parse(b)?a:b):'';
  return observation({observedAt:oldest},u);
}
function endpoint(u,route,side) {
  const market=route.legs[0][side];
  const copy=(key,label)=>btn(u,'','copy','copy','icon-only',
    `data-id="${u.esc(route.id)}" data-leg="0" data-side="${side}" data-copy="${key}" title="${th(label)}" aria-label="${th(label)}"${market[key]?'':' disabled'}`);
  return `<div class="deals-endpoint">
    <div class="deals-endpoint-title">
      <span class="deals-station">${u.esc(market.station||t('Станция неизвестна'))}${copy('station','Копировать название станции')}</span>
      <span class="deals-system">${u.esc(market.system||t('Система неизвестна'))}${copy('system','Копировать систему')}</span>
    </div>
    <div class="deals-station-meta">
      <span>${u.esc(stationType(market.type))}</span>
      <span>${market.pad?`${th('Площадка ')}${u.esc(market.pad)}`:th('Площадка неизвестна')}</span>
      <span>${th('От звезды')} <b>${units(u,market.distanceLs,' св. с.')}</b></span>
    </div>
  </div>`;
}
function legQuote(u,leg,side) {
  const buy=side==='buy';
  const label=buy?'Закупка':'Продажа', price=leg[buy?'buyPrice':'sellPrice'];
  const stock=leg[buy?'supply':'demand'], stockLabel=buy?'Запас':'Спрос';
  return `<div class="deals-quote ${buy?'buy':'sell'}" data-side="${side}">
    <div class="deals-good"><span>${th(label)}</span><strong>${th(leg.name||leg.key)}</strong></div>
    <div class="deals-quote-values"><strong>${exactMoney(u,price,' кр./т')}</strong>
      <span>${th(stockLabel)} <b>${units(u,stock,' т')}</b>${!buy&&leg.demandUnlimited?th(' · без ограничения'):''}</span>
      ${buy?'':`<span class="deals-leg-profit">${Number(leg.profitPerTonne)>0?'+':''}${exactMoney(u,leg.profitPerTonne,' кр./т')} <b>${units(u,leg.marginPercent,'%')}</b></span>`}
    </div>
    <div class="deals-observed">${u.icon('clock')}${observation(leg[side],u)}</div>
  </div>`;
}
function legCard(u,leg,index) {
  const reverse=index===1, direction=reverse?'Обратно':'Туда';
  return `<section class="deals-leg${reverse?' reverse':''}" aria-label="${th(direction)}">
    <div class="deals-leg-path">
      ${legQuote(u,leg,reverse?'sell':'buy')}
      <div class="deals-between" title="${th(direction)}">${u.icon(reverse?'chevron-left':'chevron-right')}</div>
      ${legQuote(u,leg,reverse?'buy':'sell')}
    </div>
    <div class="deals-leg-quantity">${th(direction)} · ${units(u,leg.quantity,' т')}</div>
  </section>`;
}
function metric(label,value,style='') {
  return `<div${style?` class="${style}"`:''}><dt>${th(label)}</dt><dd>${value}</dd></div>`;
}
function measured(run) {
  return run?.completedRounds>0&&finite(run.lastRoundSeconds)&&Number(run.lastRoundSeconds)>0;
}
function routeStats(C,u,route,active) {
  const first=route.legs[0], run=active?C.state.tradeRun:null;
  const distance=route.legDistanceLy??first.distanceLy??route.distanceLy;
  const approach=route.originDistanceLy??route.approachDistanceLy??first.approachDistanceLy;
  const perUnit=route.marginPerTon??(route.legs.every(leg=>finite(leg.profitPerTonne))?
    route.legs.reduce((value,leg)=>value+Number(leg.profitPerTonne),0):null);
  const actual=measured(run)&&finite(run.actualProfitPerHour);
  const hourly=measured(run)?(actual?run.actualProfitPerHour:run.profitPerHour):null;
  const power=Number(route.powerBonusProfit)>0&&route.powerBonusPower;
  const bonuses=power?`<dl class="deals-power-stats" title="${th('Бонус Powerplay при полном ранге')}">
    <div class="deals-bonus-heading">${u.icon('shield-check')}<span>${th('Бонус Powerplay при полном ранге')} · ${u.esc(t(route.powerBonusPower))}</span></div>
    ${metric('Бонус за тонну',exactMoney(u,route.powerBonusPerUnit,' кр./т'))}
    ${metric('Бонус за рейс',exactMoney(u,route.powerBonusProfit))}
    ${metric('Прибыль с бонусом',exactMoney(u,route.profitWithPowerBonus),'deals-emphasis')}
    ${measured(run)&&finite(route.profitWithPowerBonus)?
      metric('Расчёт за час с бонусом',exactMoney(u,Math.round(3600*route.profitWithPowerBonus/run.lastRoundSeconds),' кр./ч')):''}
  </dl>`:'';
  return `<aside class="deals-route-stats">
    <dl>
      ${metric('Расстояние между системами',units(u,distance,' св. л.'))}
      ${route.legs.length>1&&finite(route.distanceLy)&&route.distanceLy!==distance?
        metric('Полный круг',units(u,route.distanceLy,' св. л.')):''}
      ${metric('До первой системы',units(u,approach,' св. л.'))}
      ${metric('Самая старая цена',routeObservation(route,u))}
    </dl>
    <dl>
      ${metric('Прибыль за тонну',exactMoney(u,perUnit,' кр./т'))}
      ${metric('Прибыль за рейс',exactMoney(u,route.netEstimate),'deals-emphasis')}
      ${metric('Рентабельность',route.legs.map(leg=>units(u,leg.marginPercent,'%')).join(' / '))}
      ${finite(hourly)?metric(actual?'Фактическая прибыль за час':'Прибыль за час по времени круга',exactMoney(u,hourly,' кр./ч')):''}
    </dl>
    ${bonuses}
  </aside>`;
}
function routeMatchesRun(route,run) {
  return !!run?.routeId&&(run.routeId===route.id||run.routeId===route.searchID);
}
function routeCard(C,u,route) {
  const l=local(C), legs=route.legs, active=routeMatchesRun(route,C.state.tradeRun), saved=l.view==='saved';
  const favorite=saved||savedRows(C).some(item=>item.id===route.id||item.searchID===route.id);
  const favoriteLabel=favorite?'В избранном':'Добавить маршрут в избранное';
  const attrs=`data-id="${u.esc(route.id)}"`;
  return `<article class="panel deals-route ${active?'selected':''}" data-deals-route="${u.esc(route.id)}">
    <header class="deals-route-header">
      <div class="deals-endpoint-pair">${endpoint(u,route,'buy')}<div class="deals-between">${u.icon(legs.length>1?'arrow-left-right':'arrow-right')}</div>${endpoint(u,route,'sell')}</div>
      <div class="deals-route-tools">
        <span>${saved?u.esc(savedRouteLabel(route)):`${th(legs.length>1?'Рейс туда и обратно':'Рейс в одну сторону')}`}</span>
        <button class="deals-star${favorite?' favorite':''}" type="button" data-click="deals-save" ${attrs}
          aria-label="${th(favoriteLabel)}" title="${th(favoriteLabel)}" aria-pressed="${favorite?'true':'false'}"${favorite?' disabled':''}>${favorite?'★':'☆'}</button>
      </div>
    </header>
    <div class="deals-card-body">
      <div class="deals-route-legs">${legs.map((leg,index)=>legCard(u,leg,index)).join('')}</div>
      ${routeStats(C,u,route,active)}
    </div>
    ${(route.warnings||[]).length?`<details class="deals-warnings"><summary>${th('Примечания к маршруту')}</summary>${route.warnings.map(warning=>`<p>${th(warning)}</p>`).join('')}</details>`:''}
    <footer>
      ${btn(u,t(active?'Рейс отслеживается':'Отслеживать рейс'),'track','route',active?'quiet':'primary',attrs+(active?' disabled':''))}
      <span class="spacer"></span>${saved?savedRouteControls(C,u,route):''}
    </footer>
  </article>`;
}
function routeList(C){const l=local(C),rows=l.view==='saved'?savedRows(C):resultRows(C),query=l.query.trim().toLocaleLowerCase('ru');return query?rows.filter(route=>JSON.stringify((route.legs||[]).map(leg=>[leg.name,leg.key,leg.buy?.station,leg.buy?.system,leg.sell?.station,leg.sell?.system])).toLocaleLowerCase('ru').includes(query)):rows;}
function activePlan(C,u){const plan=C.state.tradePlan;if(!plan?.key)return '';const good=(C.state.goods||[]).find(row=>row.key===plan.key),name=plan.name||good?.name||plan.key;const point=(side,label,legacy)=>{const market=plan[side]||{},system=market.system||(typeof plan[legacy]==='string'?plan[legacy]:plan[legacy]?.system)||t("Система неизвестна");return `<div><span>${label}</span><strong>${u.esc(system)}</strong><small>${u.esc(market.station||t("Станция не указана"))}</small><b>${u.esc(u.fmt(plan[side+'Price']))}${th(" кр./т")}</b></div>`;};return `<details class="panel deals-active-plan" ${resultRows(C).length?'':'open'}><summary>${u.icon('circle-check')}<strong>${th("Текущий план перевозки")}</strong><span>${u.esc(name)} · ${u.esc(u.fmt(plan.quantity))}${th(" т")}</span>${u.icon('chevron-down')}</summary><div class="deals-active-points">${point('buy',t("Закупка"),'source')}<span class="deals-active-arrow">${u.icon('arrow-right')}</span>${point('sell',t("Продажа"),'destination')}${btn(u,t("Убрать план"),'plan-clear','x','quiet')}</div></details>`;}
function runTarget(u,point,label) {
  if(!point) return '';
  const actions=['sell','buy'].filter(side=>point[side]).map(side=>{
    const order=point[side];
    return `<div class="deals-run-action">
      <span>${th(side==='sell'?'Продажа':'Закупка')}</span>
      <strong>${th(order.name||order.key)} · ${units(u,order.quantity,' т')}</strong>
      <span>${exactMoney(u,order.price,' кр./т')}</span>
    </div>`;
  });
  return `<div class="deals-run-point">
    <span>${th(label)}</span>
    <strong>${u.esc(point.station||t('Станция неизвестна'))} <span>· ${u.esc(point.system||t('Система неизвестна'))}</span></strong>
    ${actions.join('')}
  </div>`;
}
function activeRun(C,u) {
  const run=C.state.tradeRun;
  if(!run?.routeId) return '';
  const phases={source:'Перелёт к станции закупки',outbound:'Рейс туда',return:'Обратный рейс',completed:'Рейс завершён'};
  const completed=Number(run.completedRounds)||0, measuredRound=measured(run), actual=measuredRound&&finite(run.actualProfitPerHour);
  const actualProfit=actual?run.actualRoundProfit:run.lastRoundProfit;
  const hourly=actual?run.actualProfitPerHour:run.profitPerHour;
  const sameStation=run.atStation&&run.target&&run.atStation.system===run.target.system&&run.atStation.station===run.target.station;
  return `<section class="panel deals-run" aria-label="${th('Торговый рейс под наблюдением')}">
    <header>
      <div><h2>${th('Торговый рейс под наблюдением')}</h2>${phases[run.phase]?`<span>${th(phases[run.phase])}</span>`:''}</div>
      ${btn(u,t('Остановить отслеживание'),'track-stop','x','quiet')}
    </header>
    <div class="deals-run-content">
      ${runTarget(u,run.atStation,'На станции')}${sameStation?'':runTarget(u,run.target,'Следующая станция')}
      ${measuredRound?`<dl class="deals-run-metrics">
        ${metric('Завершено кругов',units(u,completed,''))}
        ${metric('Последний круг',units(u,Math.round(run.lastRoundSeconds/6)/10,' мин'))}
        ${metric(actual?'Прибыль последнего круга':'Расчёт прибыли за круг',exactMoney(u,actualProfit))}
        ${finite(hourly)?metric(actual?'Фактическая прибыль за час':'Прибыль за час по времени круга',exactMoney(u,hourly,' кр./ч'),'deals-emphasis'):''}
      </dl>`:''}
    </div>
  </section>`;
}
function render(C, u) {
  const l=local(C), m=model(C), rows=routeList(C);
  const pages=Math.max(1,Math.ceil(rows.length/10));
  l.page=Math.max(0,Math.min(l.page,pages-1));
  const showingSaved=l.view==='saved', workspace=C.state.planWorkspace;
  const owner=showingSaved&&workspace?.name?
    workspace.name+(workspace.branch==='legacy'?' · Legacy':workspace.branch==='live'?' · Live':''):'';
  const loading=!showingSaved&&(m.running||l.starting||automaticRequest(C));
  const {issues}=automaticInputs(C);
  const error=!showingSaved&&(l.autoError||m.error);
  const noRowsTitle=l.query?'Совпадений нет':showingSaved?'Нет сохранённых рейсов':'Рейсы не найдены';
  const noRowsHint=!showingSaved&&!l.query&&!m.updatedAt&&issues.length?
    `<p>${u.esc(issues.join(' '))}</p>`:'';
  const cards=rows.slice(l.page*10,l.page*10+10).map(route=>routeCard(C,u,route)).join('');
  const empty=loading?'':`<div class="panel deals-empty">${u.icon(showingSaved?'bookmark':'route')}
    <h2>${th(noRowsTitle)}</h2>${noRowsHint}
    ${l.query?btn(u,t('Очистить фильтр'),'clear-query','x','quiet'):''}</div>`;
  const tab=(key,label,count)=>`<button type="button" data-click="deals-view" data-view="${key}"
    class="${l.view===key?'selected':''}"${l.view===key?' aria-current="page"':''}>${th(label)}${count==null?'':` <span>${u.esc(count)}</span>`}</button>`;
  return `<div class="deals-page">
    ${u.heading(t('Торговые маршруты'),owner,m.running?btn(u,t('Остановить'),'cancel','x','quiet'):'')}
    <nav class="deals-tabs segmented" aria-label="${th('Торговые маршруты')}">
      ${tab('results','Лучшие торговые маршруты',resultRows(C).length)}
      ${tab('search','Поиск торговых маршрутов')}
      ${tab('saved','Избранные маршруты',savedRows(C).length)}
    </nav>
    ${activeRun(C,u)}
    ${showingSaved?'':searchForm(C,u)}${activePlan(C,u)}
    ${rows.length||l.query?`<div class="deals-listbar">
      <input id="deals-query" placeholder="${th('Фильтр по системе, станции, товару')}" value="${u.esc(l.query)}" aria-label="${th('Фильтр результатов')}">
    </div>`:''}
    ${loading?`<div class="deals-progress" role="status">${u.icon('refresh-cw')}${th('Подбираем выгодные рейсы…')}
      ${m.total?`<span>${u.esc(m.checked||0)} / ${u.esc(m.total)}</span>`:''}</div>`:''}
    ${error?`<div class="notice" role="alert">${th(error)}</div>`:''}
    ${!showingSaved&&(m.results?.length||0)>resultRows(C).length?`<div class="deals-coverage">${th('Требуется повторный поиск.')}</div>`:''}
    ${!showingSaved&&m.coverageLimited?`<div class="deals-coverage">${m.coverageScope==='regional'?th('Проверено ближайших рынков: {count}',{count:u.fmt(m.checked||0)}):th('Ограниченная выборка рынков')}</div>`:''}
    <div class="deals-results">${cards||empty}</div>
    ${pages>1?`<div class="deals-pager">
      ${btn(u,t('Назад'),'prev','chevron-left','quiet',l.page===0?'disabled':'')}
      <span>${l.page+1} / ${pages} · ${rows.length}${th(' рейсов')}</span>
      ${btn(u,t('Далее'),'next','chevron-right','quiet',l.page===pages-1?'disabled':'')}
    </div>`:''}
  </div>`;
}
function capture(C,u) {
  const f=local(C).filters;
  for(const node of u.$$('[data-deals-filter]')) {
    const key=node.dataset.dealsFilter;
    if(key) f[key]=node.type==='checkbox'?node.checked:node.value;
  }
  return f;
}
function payload(C,u) {
  const f=capture(C,u), out=Object.fromEntries(Object.keys(defaults).map(key=>[key,f[key]]));
  out.system=String(f.system??'').trim();
  if(!out.system) throw new Error(t("Укажите исходную систему поиска."));
  for(const [key,label,min,max,integer] of specs) {
    const text=String(f[key]??'').trim().replace(/[\s\u00a0\u202f]/g,'').replace(',','.');
    if(!/^\d+(?:\.\d+)?$/.test(text)) throw new Error(t('Введите число в поле «{field}».',{field:t(label)}));
    const value=Number(text);
    if(!Number.isFinite(value)||value<min||value>max||integer&&!Number.isInteger(value))
      throw new Error(t('Проверьте диапазон значений поля «{field}».',{field:t(label)}));
    out[key]=value;
  }
  out.allowSurface=String(out.allowSurface)==='true';
  out.roundTrip=String(out.roundTrip)==='true';
  return out;
}
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
async function handle(action,b,C,u) {
  if(!action.startsWith('deals-')) return false;
  const l=local(C), name=action.slice(6);
  if(name==='search') {
    let values;
    try {
      values=payload(C,u);
    } catch(error) {
      const box=u.$('#deals-form-error');
      if(box){box.textContent=error.message;box.classList.remove('hidden');}
      throw error;
    }
    l.autoError='';
    await u.call('deals-search',{values:{...values,automatic:false}});
    l.manual=true;
    l.page=0;
    l.selected='';
    l.query='';
    l.dirty=false;
    await u.loadView();
    return true;
  }
  if(name==='plan-clear') {
    u.dialog(t('Убрать текущий план перевозки?'),`<p>${th('Убрать перевозку из текущих задач.')}</p>`,
      u.btn(t('Отмена'),'close-dialog')+btn(u,t('Убрать план'),'plan-clear-confirm','x','primary'));
    return true;
  }
  if(name==='plan-clear-confirm') {
    await u.call('deals-plan-clear');
    u.closeLayer();
    await u.loadView();
    u.toast(t('Текущий план перевозки убран'));
    return true;
  }
  if(name==='cancel') {
    l.manual=true;
    await u.call('deals-cancel');
    await u.loadView();
    u.toast(t('Поиск остановлен'));
    return true;
  }
  if(name==='view') {
    l.view=['saved','search'].includes(b.dataset.view)?b.dataset.view:'results';
    C.mode=l.view==='saved'?'Сохранённые':'Поиск';
    C.selected='';
    l.page=0;
    l.query='';
    await u.loadView();
    return true;
  }
  if(name==='prev'||name==='next') {
    l.page+=name==='next'?1:-1;
    u.renderPage();
    return true;
  }
  if(name==='clear-query') {
    l.query='';
    l.page=0;
    u.renderPage();
    return true;
  }
  const expectedPlanWorkspace=C.state.planWorkspace?.key||'';
  if(name==='track-stop') {
    await u.call('deals-track',{values:{action:'stop',expectedPlanWorkspace}});
    await u.loadView();
    return true;
  }
  const route=find(C,b?.dataset.id);
  if(!route) throw new Error(t('Рейс больше не найден. Обновите результаты.'));
  if(name==='track'||name==='select') {
    if(name==='track') await u.call('deals-track',{values:{action:'start',routeId:route.id,expectedPlanWorkspace}});
    l.selected=route.id;
    C.selected=route.id;
    await u.loadView();
    u.toast(t(name==='track'?'Отслеживание рейса включено':'Рейс выбран'));
    return true;
  }
  if(name==='save') {
    await u.call('deals-saved-route',{values:savedRouteRequest(C,{action:'save-route',route:{...route,options:route.options||model(C).options||{}}})});
    await u.loadView();
    u.toast(t('Полный рейс сохранён'));
    return true;
  }
  if(name==='scope') {
    const scope=route.scope==='personal'?'shared':'personal';
    await u.call('deals-saved-route',{values:savedRouteRequest(C,{action:'set-scope',id:route.id,scope})});
    await u.loadView();
    u.toast(t('Владелец рейса изменён'));
    return true;
  }
  if(name==='delete') {
    u.dialog(t('Удалить сохранённый рейс?'),`<p>${th('Он исчезнет из списка сохранённых. Рыночные предложения останутся.')}</p>`,
      u.btn(t('Отмена'),'close-dialog')+btn(u,t('Удалить'),'delete-confirm','trash-2','primary',`data-id="${u.esc(route.id)}"`));
    return true;
  }
  if(name==='delete-confirm') {
    await u.call('deals-saved-route',{values:savedRouteRequest(C,{action:'delete-route',id:route.id})});
    u.closeLayer();
    await u.loadView();
    u.toast(t('Сохранённый рейс удалён'));
    return true;
  }
  const leg=route.legs[Number(b.dataset.leg)];
  if(!leg) throw new Error(t('Выберите участок рейса.'));
  if(name==='copy') {
    const station=leg[b.dataset.side]||{}, key=b.dataset.copy==='station'?'station':'system';
    if(!station[key]) throw new Error(t('Название неизвестно.'));
    await u.call('native',{action:'copy',text:station[key]});
    u.toast(t(key==='system'?'Система скопирована':'Название станции скопировано'));
    return true;
  }
  if(name==='destination') {
    await u.call('deals-destination',{values:{destination:{...leg.sell,selectionMode:'sell',quoteOptions:route.options||model(C).options||{}}}});
    await u.loadView();
    u.toast(t('Покупатель назначен для оверлея'));
    return true;
  }
  if(name==='plan') {
    await u.call('deals-plan',{values:{key:leg.key,quantity:leg.quantity,buyPrice:leg.buyPrice,sellPrice:leg.sellPrice,
      source:leg.buy.system||'',destination:leg.sell.system||'',buy:leg.buy,sell:leg.sell}});
    await u.loadView();
    u.toast(t('План перевозки сохранён'));
    return true;
  }
  return false;
}
function handleInput(event,C,u) {
  const node=event.target, key=node?.dataset?.dealsFilter;
  if(!key && node?.id!=='deals-query') return false;
  const l=local(C);
  if(node.id==='deals-query') {
    const start=node.selectionStart;
    l.query=node.value;
    l.page=0;
    u.renderPage();
    const next=u.$('#deals-query');
    next?.focus();
    try{next?.setSelectionRange(start,start);}catch(_){}
    return true;
  }
  l.filters[key]=node.type==='checkbox'?node.checked:node.value;
  l.dirty=true;
  return true;
}
async function handleChange(event,C,u) {
  return handleInput(event,C,u);
}
function init(C,u) {
  latest={C,u};
  if(typeof document!=='undefined'&&!init.bound) {
    init.bound=true;
    document.addEventListener('submit',event=>{
      if(event.target.id!=='deals-search-form'||!latest) return;
      event.preventDefault();
      handle('deals-search',null,latest.C,latest.u).catch(error=>latest.u.toast(error.message,true));
    });
  }
  return startAutomatic(C,u);
}
async function smoke(C,u) {
  const html=searchForm(C,u);
  const fields=[...html.matchAll(/data-deals-filter="([^"]+)"/g)].map(match=>match[1]).sort();
  return [
    {name:t('Поля поиска торговых маршрутов'),ok:fields.join('|')===Object.keys(defaults).sort().join('|')},
    {name:t('Избранные маршруты'),ok:render(C,u).includes('data-view="saved"')}
  ];
}
window.DealsUI={render,handle,handleInput,handleChange,init,smoke};
})();
