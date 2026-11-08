/* These integration checks operate only on the disposable --smoke profile.
   No market, account or galaxy network request is started by this module. */
(()=>{
'use strict';
const clone=value=>value==null?value:JSON.parse(JSON.stringify(value));
const normalized=value=>String(value??'').replace(/\s+/g,' ').trim();
function useful(route){const result=clone(route);if(result)delete result.savedAt;return JSON.stringify(result);}
function planUseful(plan){
 if(!plan?.key)return null;
 return {key:plan.key,quantity:plan.quantity,buyPrice:plan.buyPrice,sellPrice:plan.sellPrice,source:plan.source||'',destination:plan.destination||'',buy:plan.buy||null,sell:plan.sell||null};
}
async function run(C,u){
 if(window.COPILOT_SMOKE!==true)throw new Error('Сценарий доступен только в изолированной проверке');
 if(typeof u.handleClick!=='function')throw new Error('Smoke требует настоящий обработчик интерфейса');
 const checks=[],check=(name,ok)=>{checks.push({name,ok:!!ok});if(!ok)throw new Error(name);};
 const original={tab:C.tab,mode:C.mode,query:C.query,selected:C.selected,prefill:clone(C.prefill),lesson:C.lesson,course:C.course,focus:C.focus,inspector:C.inspector,nav:[...(C.nav||[])],dealsUI:clone(C.dealsUI),settingsPane:clone(C.settingsPane),colonyWorkspace:clone(C.colonizationWorkspace),reference:C.reference,references:C.referenceSessions};
 let originalPlan,originalFavorites,originalColony,failure;
 const cleanupErrors=[];
 const visit=(tab,options={})=>u.navigate(tab,{...options,back:true});
 const click=async selector=>{
  const button=u.$(selector);if(!button||button.disabled)throw new Error('Недоступна кнопка smoke: '+selector);
  await u.handleClick(button.dataset.click,button);return button;
 };
 const fresh=async(tab,mode='')=>(await u.call('view',{tab,mode,query:'',selected:''})).state||{};
 const visible=node=>!!node&&!node.hidden&&(typeof getComputedStyle!=='function'||getComputedStyle(node).display!=='none');
 const textHas=(node,value)=>!!node&&normalized(node.textContent).includes(normalized(value));
 const ready=async()=>{
  const deadline=Date.now()+12000;
  while(Date.now()<deadline){
   const r=C.reference;if(r?.error||r?.entryError)throw new Error(r.error||r.entryError);
   if(r?.entry&&!r.entryLoading)return;
   await new Promise(resolve=>setTimeout(resolve,25));
  }
  throw new Error('Полная страница справочника не открылась');
 };
 try{
  await visit('settings');
  // A default-open pane can have no explicit persisted preference yet. Always
  // create the open state through the same handler a user actually clicks.
  if(C.inspector)await click('.settings-help [data-click="inspector"]');
  await click('#page-toolbar [data-click="inspector"]');
  check('Настройки: подробности открывают настоящую видимую панель',visible(u.$('.settings-help')));
  check('Настройки: открытая панель сохраняется через backend',(await fresh('settings')).desktop?.inspectorOpen===true);
  await click('.settings-help [data-click="inspector"]');
  check('Настройки: крестик скрывает панель и возвращает место',!u.$('.settings-help')&&C.inspector===false);
  check('Настройки: скрытое состояние читается повторным запросом',(await fresh('settings')).desktop?.inspectorOpen===false);
  await click('#page-toolbar [data-click="inspector"]');
  check('Настройки: панель открывается повторно',visible(u.$('.settings-help')));

  await visit('deals',{mode:'Поиск'});
  originalPlan=clone(C.state.tradePlan||{});originalFavorites=clone(C.state.savedTradeRoutes||[]);
  const route=(C.state.tradeRoutes?.results||[]).find(item=>item.legs?.length===2);
  check('Рейсы: синтетический результат содержит оба торговых плеча',!!route);
  C.dealsUI.view='results';C.dealsUI.query='';C.dealsUI.page=0;u.renderPage();
  let card=u.$$('[data-deals-route]').find(node=>node.dataset.dealsRoute===route.id);
  const legs=card?u.$$('.deals-leg',card):[];
  check('Рейсы: оба плеча имеют отдельные карточки закупки и продажи',legs.length===2&&legs.every(node=>u.$$('.deals-endpoint',node).length===2));
  for(let index=0;index<2;index++){
   const endpoints=u.$$('.deals-endpoint',legs[index]),leg=route.legs[index];
   check('Рейсы: системы, станции, цены и объёмы плеча '+(index+1),['buy','sell'].every((side,sideIndex)=>{
    const node=endpoints[sideIndex],station=leg[side];
    return textHas(u.$('.deals-system',node),station.system)&&textHas(u.$('.deals-station',node),station.station)&&textHas(u.$('.deals-quote',node),u.fmt(leg[side==='buy'?'buyPrice':'sellPrice']))&&textHas(u.$('.deals-quote',node),u.fmt(leg[side==='buy'?'supply':'demand']));
   }));
  }
  await u.handleClick('deals-save',u.$('[data-click="deals-save"]',card));
  const once=clone(C.state.savedTradeRoutes||[]),saved=once.find(item=>item.searchID===route.id)||once.find(item=>JSON.stringify(item.legs)===JSON.stringify(route.legs));
  check('Рейсы: сохранён весь рейс с двумя плечами',!!saved&&saved.legs.length===2);
  card=u.$$('[data-deals-route]').find(node=>node.dataset.dealsRoute===route.id);
  await u.handleClick('deals-save',u.$('[data-click="deals-save"]',card));
  const repeated=await fresh('deals','Сохранённые');
  check('Рейсы: повторное сохранение не создаёт дубликат',(repeated.savedTradeRoutes||[]).length===once.length&&(repeated.savedTradeRoutes||[]).filter(item=>item.id===saved.id).length===1);
  check('Рейсы: повторный запрос сохраняет обе котировки',JSON.stringify((repeated.savedTradeRoutes||[]).find(item=>item.id===saved.id)?.legs)===JSON.stringify(saved.legs));
  await click('[data-click="deals-view"][data-view="saved"]');
  card=u.$$('[data-deals-route]').find(node=>node.dataset.dealsRoute===saved.id);
  check('Рейсы: сохранённый маршрут открывается полной картой',!!card&&u.$$('.deals-leg',card).length===2);
  await u.handleClick('deals-select',u.$('[data-click="deals-select"]',card));
  card=u.$$('[data-deals-route]').find(node=>node.dataset.dealsRoute===saved.id);
  check('Рейсы: выбор сохранённого маршрута сохраняется при перерисовке',!!card?.classList.contains('selected')&&C.selected===saved.id);
  await u.handleClick('deals-plan',u.$('[data-click="deals-plan"][data-leg="1"]',card));
  const plan=(await fresh('deals')).tradePlan||{},second=saved.legs[1];
  check('Рейсы: план второго плеча сохраняет правильные системы и станции',plan.key===second.key&&plan.buy?.system===second.buy.system&&plan.buy?.station===second.buy.station&&plan.sell?.system===second.sell.system&&plan.sell?.station===second.sell.station&&Number(plan.quantity)===Number(second.quantity));

  await visit('colonization',{mode:'Строительство'});
  originalColony=C.state.colonization?.selectedProjectId;
  const projects=clone(C.state.colonization?.projects||[]);
  check('Колонии: в smoke есть два независимых плана',projects.length>=2&&projects[0].id!==projects[1].id);
  const requirements=[];
  for(const project of projects.slice(0,2)){
   await u.call('select-colony',{tab:'colonization',id:project.id});await u.loadView();
   const current=C.state.colonization?.activeProject,hero=u.$('[data-colony-id]');
   check('Колонии: выбранный план и шапка совпадают — '+project.name,current?.id===project.id&&hero?.dataset.colonyId===project.id&&textHas(u.$('.colony-overview-title',hero),project.name)&&textHas(hero,current.originSystem)&&textHas(hero,current.faction));
   const supplies=JSON.stringify((current?.supplies||[]).map(item=>[item.commodity,item.required,item.delivered,item.missing]));requirements.push(supplies);
   const remaining=u.$$('.colony-summary-metric',hero).find(node=>textHas(node,'Осталось доставить'));
   check('Колонии: снабжение принадлежит выбранному плану — '+project.name,!!remaining&&(Number(current.unknownBills)>0&&Number(current.totalMissing)===0?textHas(remaining,'Неизвестно'):textHas(remaining,u.fmt(current.totalMissing))));
  }
  check('Колонии: требования двух синтетических складов различаются',requirements[0]!==requirements[1]);

  await visit('route');
  const originalRoute=C.state.carrierRoute;
  try{
   // The render probe is deliberately local. It never asks Spansh to calculate
   // a route and never changes the persisted carrier route or clipboard.
   C.state.carrierRoute={status:'complete',running:false,options:{tritiumPrice:0},route:{sourceSystem:'SMOKE ROUTE SOURCE',destinationSystem:'SMOKE ROUTE NEXT',stops:[{name:'SMOKE ROUTE SOURCE',visited:true},{name:'SMOKE ROUTE NEXT',visited:false,distanceLy:420,fuelUsedTons:80,transferTons:0}]},summary:{fuelTons:80,fuelCost:0,totalMinutes:20,restockTons:0,restockStops:0}};
   u.renderPage();
   check('Маршрут: следующая система и кнопка копирования присутствуют',textHas(u.$('.rw-next h2'),'SMOKE ROUTE NEXT')&&!!u.$('[data-click="route-workspace-copy"]')&&!u.$('[data-click="route-workspace-copy"]').disabled);
   const fuel=u.$$('.rw-metric').find(node=>textHas(node,'Стоимость топлива'));
   check('Маршрут: неизвестная цена трития не выдаётся за бесплатное топливо',textHas(u.$('.rw-metric-value',fuel),'Неизвестна')&&textHas(fuel,'Цена трития не задана'));
  }finally{C.state.carrierRoute=originalRoute;u.renderPage();}

  // Use a temporary reader session so an existing library/reading position is
  // not overwritten. Both requests use only the embedded reference catalogue.
  C.referenceSessions={};delete C.reference;
  await visit('encyclopedia');
  const list=await u.call('reference.list',{category:'engineers',query:'Фарсир',limit:24});
  check('Справочник: встроенный каталог содержит инженера',!!list.entries?.[0]);
  await window.ReferenceUI.open(list.entries[0].id,C,u);await ready();
  const module=u.$('.ref-table [data-click="reference-open"][data-id^="module/"]');
  const blueprint=u.$('.ref-table [data-click="reference-open"][data-id^="blueprint/"]');
  check('Инженер: таблица улучшений содержит рабочие ссылки модулей и чертежей',!!module&&!!blueprint);
  await u.handleClick('reference-open',module);await ready();
  check('Инженер: ссылка модуля открывает полную страницу',C.reference.entry?.category==='modules'&&!!u.$('[data-reference="entry"]')&&!u.$('.inspector'));
  await click('[data-click="reference-back"]');await ready();
  await u.handleClick('reference-open',u.$('.ref-table [data-click="reference-open"][data-id^="blueprint/"]'));await ready();
  check('Инженер: ссылка чертежа открывает рецепты по уровням',C.reference.entry?.category==='blueprints'&&u.$$('.ref-grade').length>0);
 }catch(error){failure=error;}
 finally{
  const restore=async(name,work)=>{try{await work();}catch(error){cleanupErrors.push(name+': '+(error.message||error));}};
  if(originalFavorites)await restore('Сохранённые рейсы',async()=>{
   const current=(await fresh('deals')).savedTradeRoutes||[],ids=new Set(originalFavorites.map(item=>item.id));
   for(const item of current)if(!ids.has(item.id))await u.call('deals-saved-route',{values:{action:'delete-route',id:item.id}});
   for(const item of originalFavorites){const now=current.find(value=>value.id===item.id);if(useful(now)!==useful(item)){const route=clone(item);route.id=item.searchID||item.id;await u.call('deals-saved-route',{values:{action:'save-route',route,title:item.title||''}});}}
   const restored=(await fresh('deals')).savedTradeRoutes||[];
   if(restored.length!==originalFavorites.length||originalFavorites.some(item=>useful(item)!==useful(restored.find(value=>value.id===item.id))))throw new Error('Полезные данные сохранённых рейсов изменились');
  });
  if(originalPlan!==undefined)await restore('План перевозки',async()=>{
   await u.call(originalPlan.key?'deals-plan':'deals-plan-clear',originalPlan.key?{values:originalPlan}:{});
   if(JSON.stringify(planUseful((await fresh('deals')).tradePlan))!==JSON.stringify(planUseful(originalPlan)))throw new Error('Исходное плечо не восстановлено');
  });
  if(originalColony)await restore('Выбранная колония',()=>u.call('select-colony',{tab:'colonization',id:originalColony}));
  await restore('Панель подробностей',async()=>{C.inspector=original.inspector;await u.call('desktop',{values:{inspectorOpen:original.inspector}});});
  window.ReferenceUI?.navigate?.('overview',{},C);C.referenceSessions=original.references;C.reference=original.reference;
  C.dealsUI=original.dealsUI;C.settingsPane=original.settingsPane;C.colonizationWorkspace=original.colonyWorkspace;
  await restore('Основное меню',async()=>{if(JSON.stringify(C.nav)!==JSON.stringify(original.nav)){await u.call('navigation',{tabs:original.nav});C.nav=original.nav;}});
  await restore('Исходный экран',async()=>{
   await visit(original.tab,{mode:original.mode,prefill:original.prefill});
   C.query=original.query;C.selected=original.selected;C.lesson=original.lesson;C.course=original.course;C.focus=original.focus;
   await u.loadView();
  });
 }
 if(failure)throw new Error(failure.message+(cleanupErrors.length?' · Восстановление: '+cleanupErrors.join('; '):''));
 if(cleanupErrors.length)throw new Error('Не удалось восстановить smoke-профиль: '+cleanupErrors.join('; '));
 return checks;
}
window.V23Smoke={run};
})();
