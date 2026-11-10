'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

function fixture() {
 const leg=(id,key,quantity)=>({id,name:id==='outbound'?'Туда':'Обратно',commodityKey:key,commodityName:key==='wine'?'Вино':'Золото',quantity,effectiveQuantity:quantity,sourceSystem:id==='outbound'?'Synthetic Origin':'HIP 58832',destinationSystem:id==='outbound'?'HIP 58832':'Synthetic Origin',buyPrice:null,sellPrice:null,loadingMode:'self',unloadingMode:'self',loadRewardPerTon:0,unloadRewardPerTon:0,loadingHours:null,unloadingHours:null,jumps:null,tritiumTons:null,travelHours:null,buyMarket:{},sellMarket:{}});
 const C={tab:'carrier-trade',mode:'Рейс',state:{commander:{system:'Synthetic Origin'},resources:[{key:'gold',name:'Золото',kind:'commodity'},{key:'wine',name:'Вино',kind:'commodity'}],carrierTrade:{options:{preset:'booze-cruise',carrierType:'fleet',returnTrip:true,servicesTons:0,otherCargoTons:0,tankTons:null,cargoTritiumTons:null,arrivalReserveTons:null,tritiumPrice:null,weeklyUpkeep:5000000,durationHours:null,extraCosts:0,carrierInvestment:5000000000,haulerPad:'M',haulerCapacity:400,maxQuoteAgeHours:24,demandRatio:4,holidayConfirmed:false},legs:[leg('outbound','wine',10000),leg('return','gold',0)],summary:{status:'needs-data',profit:null,workingCapital:null,availableCargoTons:null},issues:[],missing:['tritiumPrice'],notes:[],sources:[]},trading:{results:[],options:{}}}};
 const ctx={window:{},document:{addEventListener(){}},Intl,setTimeout,clearTimeout,Date};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'carrier-trade.js'),'utf8'),ctx);
 const calls=[],elements=[],filters=[],dialogs=[],toasts=[],boxes=new Map();
 const box=()=>({innerHTML:'',textContent:'',classList:{remove(){},add(){}}});
 boxes.set('#carrier-trade-error',box());boxes.set('#carrier-trade-unsaved',box());
 const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
 const ui={esc,icon:()=>'<svg></svg>',fmt:value=>value==null||value===''?'—':new Intl.NumberFormat('ru-RU').format(Number(value)),money:value=>value==null?'—':`${value} кр.`,btn:(label,action,ico,style,attrs='')=>`<button data-click="${action}" ${attrs}>${esc(label)}</button>`,badge:text=>`<span>${esc(text)}</span>`,heading:(title,description,extra)=>`<h1>${title}</h1><p>${description}</p>${extra||''}`,
  call:async(op,params)=>{const data=JSON.parse(JSON.stringify(params));calls.push({op,params:data});if(op==='carrier-trade'){const b=data.values,m=C.state.carrierTrade;if(b.action==='configure'){Object.assign(m.options,b.options);for(const [id,changes] of Object.entries(b.legs||{}))Object.assign(m.legs.find(row=>row.id===id),changes);}if(b.action==='market'){const leg=m.legs.find(row=>row.id===b.leg),q=b.market.quotes.find(row=>row.key===leg.commodityKey);leg[b.side+'Price']=q.price;leg[b.side+'Market']={...b.market};}}return {ok:true};},
  toast:text=>toasts.push(text),loadView:async()=>{},navigate:async()=>{},renderPage(){},dialog:(title,html,footer)=>{dialogs.push({title,html,footer});boxes.set('#carrier-trade-market-results',box());},closeLayer:()=>boxes.delete('#carrier-trade-market-results'),$:selector=>boxes.get(selector)||null,$$:selector=>selector==='[data-trade-filter]'?filters:elements,
 };
 const node=(value,dataset,extra={})=>({value,dataset,type:'text',tagName:'INPUT',matches:()=>true,...extra});
 return {api:ctx.window.CarrierTradeUI,C,ui,calls,elements,filters,dialogs,toasts,boxes,node};
}

test('carrier trade separates the two legs and keeps unknown market prices blank',()=>{
 const {api,C,ui}=fixture();const html=api.render(C,ui);
 assert.match(html,/Рейс туда/);assert.match(html,/Возвращение/);assert.match(html,/Пустой перелёт тоже расходует топливо/);
 assert.equal((html.match(/data-trade-field="buyPrice"/g)||[]).length,1);
 assert.match(html,/data-trade-leg="return" data-trade-field="tritiumTons"/);
 assert.doesNotMatch(html,/data-trade-leg="return" data-trade-field="quantity"|data-trade-option="(?:otherCargoTons|tankTons|cargoTritiumTons|arrivalReserveTons)"/);
 assert.match(html,/data-trade-field="buyPrice"[^>]*placeholder="Неизвестно"/);
 assert.match(html,/Неизвестные расходы/);assert.match(html,/капитал|Капитал/);
 assert.doesNotMatch(html,/NaN|undefined|Infinity|onclick=/);
 C.mode='Расходы';assert.match(api.render(C,ui),/Закупка уже учтена/);
 C.mode='Проверки';assert.match(api.render(C,ui),/Укажите цены, объём, топливо и длительность рейса/);
});

test('carrier trade requests profitable opportunities once and opens only a qualifying full load',async()=>{
 const {api,C,ui,calls}=fixture();
 C.mode='';
 assert.match(api.render(C,ui),/Сравниваем цены и объёмы/);
 assert.doesNotMatch(api.render(C,ui),/opportunity-scenario|Обычный рейс/);
 await api.init(C,ui);await api.init(C,ui);
 assert.equal(calls.filter(x=>x.params.values?.action==='opportunities').length,1);
 assert.equal(calls[0].params.values.scenario,'profitable');
 const updatedAt=new Date().toISOString();
 C.state.carrierTrade.opportunities={updatedAt,options:{scenario:'profitable',system:''},results:[
  {id:'partial',commodityName:'Short load',quantity:2999,summary:{availableCargoTons:3000,profit:600000000}},
  {id:'loss',commodityName:'Loss marker',quantity:3000,summary:{availableCargoTons:3000,profit:-1}},
  {id:'low',commodityName:'Low profit',quantity:3000,summary:{availableCargoTons:3000,profit:399999999}},
  {id:'unknown',commodityName:'Unknown profit',quantity:3000,summary:{availableCargoTons:3000,profit:null}},
  {id:'offer',commodityName:'Золото',quantity:3000,
  buy:{station:'Seller',system:'Alpha',price:200,updatedAt,orders:[{station:'Seller',system:'Alpha',quantity:1000,price:190},{station:'Another',system:'Gamma',quantity:2000,price:205}]},
  sell:{station:'Gold Buyer',system:'Beta',price:3000,updatedAt},summary:{profit:400000000,availableCargoTons:3000.4,jumpCount:18,fuelTons:988,servicesTons:1750,cargoTritiumTons:87,arrivalReserveTons:99,loadingHours:.5,loadingSorties:3,unloadingHours:1.5,unloadingSorties:9,fuelLoadingHours:.5,travelHours:6,workingHours:8.5},
  fuelPurchaseTons:1087,fuelPurchaseCost:54350000,
  route:{loaded:[{from:'Alpha',to:'Beta',distanceLy:50}]}
 }]};
 const html=api.render(C,ui);
 assert.match(html,/Another/);assert.match(html,/Gold Buyer/);assert.match(html,/Системы маршрута/);assert.match(html,/400000000/);
 assert.match(html,/data-system="Gamma"/);assert.match(html,/Чистое время рейса.*8,5 ч/);
 assert.match(html,/Погрузка товара · ходок 3/);assert.match(html,/Разгрузка товара · ходок 9/);
 assert.doesNotMatch(html,/Short load|Loss marker|Low profit|Unknown profit|Rackham/);
 assert.match(html,/Резерв трития \(10%\).*99 т/);assert.match(html,/54350000 кр\./);
 await assert.rejects(api.handle('carrier-trade-use-opportunity',{dataset:{index:'0'}},C,ui),/больше недоступен/);
 await api.handle('carrier-trade-use-opportunity',{dataset:{index:'4'}},C,ui);
 assert.equal(calls.at(-1).params.values.action,'apply-opportunity');
 assert.equal(calls.at(-1).params.values.id,'offer');
 assert.equal(C.mode,'Рейс');
});

test('expired quotes are hidden and force a replacement search',async()=>{
 const {api,C,ui,calls}=fixture();C.mode='Рейсы';
 await api.init(C,ui);
 C.state.carrierTrade.opportunities={updatedAt:new Date().toISOString(),options:{scenario:'profitable',system:''},
  results:[{id:'old',commodityName:'Stale result',quantity:3000,summary:{availableCargoTons:3000,profit:400000000},buy:{price:2,updatedAt:'2000-01-01T00:00:00Z'}}]};
 assert.doesNotMatch(api.render(C,ui),/Stale result/);
 await api.init(C,ui);await api.init(C,ui);
 const requests=calls.filter(x=>x.params.values?.action==='opportunities');
 assert.equal(requests.length,2);assert.equal(requests[1].params.values.force,true);
});

test('a new commander scope starts a fresh search with identical filters',async()=>{
 const {api,C,ui,calls}=fixture();C.mode='Рейсы';
 C.state.carrierTrade.opportunityScope=1;
 await api.init(C,ui);
 C.state.carrierTrade.opportunityScope=2;
 await api.init(C,ui);await api.init(C,ui);
 const requests=calls.filter(x=>x.params.values?.action==='opportunities');
 assert.equal(requests.length,2);
 assert.deepEqual(requests[0].params,requests[1].params);
});

test('carrier searches respond to service space but not existing cargo or fuel',async()=>{
 const {api,C,ui,calls}=fixture();C.mode='Рейсы';
 const carrier={owned:true,location:{system:'Sol'},space:{crewTons:1000,shipPacksTons:0,modulePacksTons:0,freeTons:5000},fuel:{tankTons:800,reserveTons:100}};
 C.state.carrierTrade.context={carrier};
 await api.init(C,ui);
 carrier.space.freeTons=3000;
 carrier.fuel.tankTons=1000;
 carrier.fuel.reserveTons=500;
 await api.init(C,ui);
 assert.equal(calls.length,1);
 carrier.space.crewTons=1200;
 await api.init(C,ui);
 assert.equal(calls.length,2);
 assert.ok(calls.every(call=>call.params.values.scenario==='profitable'));
});

test('Peak keeps its fixed destination and lets the user select any catalog commodity',()=>{
 const {api,C,ui}=fixture();
 const html=api.render(C,ui);assert.match(html,/HIP 58832/);assert.match(html,/площадка M/);assert.match(html,/Праздник не подтверждён/);assert.match(html,/data-trade-option="holidayConfirmed"/);assert.doesNotMatch(html,/278.?000|270.?000|250.?000/);
 assert.match(html,/value="HIP 58832" data-trade-leg="outbound" data-trade-field="destinationSystem" readonly/);
 assert.match(html,/option value="gold"/);
});

test('a generated general plan keeps its selected commodity and editable endpoints',async()=>{
 const {api,C,ui,calls,elements,node}=fixture(),m=C.state.carrierTrade;
 Object.assign(m.options,{preset:'custom',plannedFullLoad:true});
 Object.assign(m.legs[0],{commodityKey:'newcommodity',commodityName:'Provider commodity',destinationSystem:'General Buyer'});
 Object.assign(m.legs[1],{sourceSystem:'General Buyer',destinationSystem:'Original System'});
 const html=api.render(C,ui);
 assert.match(html,/value="newcommodity" selected>Provider commodity/);
 assert.match(html,/value="General Buyer" data-trade-leg="outbound" data-trade-field="destinationSystem" >/);
 assert.doesNotMatch(html,/holidayConfirmed|data-trade-leg="return" data-trade-field="quantity"|data-trade-option="tankTons"/);
 elements.push(node('400',{tradeOption:'unloadHaulerCapacity'}),node('M',{tradeOption:'unloadHaulerPad'}));
 await api.handle('carrier-trade-save',null,C,ui);
 const {options,legs}=calls[0].params.values;
 assert.equal(options.plannedFullLoad,true);
 assert.equal(options.unloadHaulerCapacity,400);
 assert.equal(options.unloadHaulerPad,'M');
 assert.equal(legs.outbound.destinationSystem,'General Buyer');
 assert.equal(legs.return.sourceSystem,'General Buyer');
 assert.equal(legs.return.quantity,0);
});

test('saving a legacy general plan preserves its manual fuel and return cargo',async()=>{
 const {api,C,ui,calls,dialogs}=fixture(),m=C.state.carrierTrade;
 Object.assign(m.options,{preset:'custom',tankTons:800,cargoTritiumTons:200,arrivalReserveTons:80,otherCargoTons:500});
 m.legs[1].quantity=3000;
 const html=api.render(C,ui);
 assert.match(html,/data-trade-leg="return" data-trade-field="quantity"/);
 assert.match(html,/value="800" data-trade-option="tankTons"/);
 await api.handle('carrier-trade-save',null,C,ui);
 const {options,legs}=calls[0].params.values;
 assert.equal(options.otherCargoTons,500);
 assert.equal(options.cargoTritiumTons,200);
 assert.equal(options.arrivalReserveTons,80);
 assert.equal(legs.return.quantity,3000);
 await api.handle('carrier-trade-preset',null,C,ui);
 assert.equal(calls.length,1);
 assert.match(dialogs[0].footer,/Применить шаблон/);
 await api.handle('carrier-trade-preset-apply',null,C,ui);
 assert.deepEqual(calls[1].params.values,{action:'preset',preset:'booze-cruise',sourceSystem:'Synthetic Origin'});
});

test('empty completed results explain the full-load profit threshold',()=>{
 const {api,C,ui}=fixture();
 C.mode='Рейсы';
 C.state.carrierTrade.opportunities={updatedAt:new Date().toISOString(),options:{scenario:'profitable',system:''},results:[]};
 assert.match(api.render(C,ui),/Рейсов с полной загрузкой и чистой прибылью от 400 млн кр. не найдено/);
});

test('escapes market, commodity, issue and source text from backend',()=>{
 const {api,C,ui}=fixture();C.state.carrierTrade.legs[0].buyMarket.station='<img src=x onerror=alert(1)>';C.state.carrierTrade.legs[0].commodityName='<script>x</script>';
 let html=api.render(C,ui);assert.match(html,/&lt;img/);assert.doesNotMatch(html,/<img|<script/);
 C.mode='Проверки';C.state.carrierTrade.issues=[{title:'<script>x</script>',detail:'<img>',level:'error'}];C.state.carrierTrade.sources=[{title:'<svg>',url:'https://example.test/" x="'}];html=api.render(C,ui);assert.match(html,/&lt;script&gt;/);assert.match(html,/&quot; x=&quot;/);assert.doesNotMatch(html,/<script|<img/);
});

test('save parses Russian input and leaves the fuel load to the backend',async()=>{
 const {api,C,ui,calls,elements,node}=fixture();
 C.state.carrierTrade.options.otherCargoTons=5000;
 C.state.carrierTrade.legs[1].quantity=100;
 elements.push(node('50 000',{tradeOption:'tritiumPrice'}),node('24,5',{tradeOption:'durationHours'}),node('12 000',{tradeLeg:'outbound',tradeField:'quantity'}),node('0',{tradeLeg:'outbound',tradeField:'buyPrice'}));
 await api.handle('carrier-trade-save',null,C,ui);const body=calls[0].params.values;
 assert.equal(body.action,'configure');assert.equal(body.options.tritiumPrice,50000);assert.equal(body.options.durationHours,24.5);
 for(const key of ['tankTons','cargoTritiumTons','arrivalReserveTons'])assert.equal(Object.hasOwn(body.options,key),false);
 assert.equal(body.options.otherCargoTons,0);assert.equal(body.legs.outbound.quantity,12000);assert.equal(body.legs.outbound.buyPrice,0);assert.equal(body.legs.outbound.sellPrice,null);assert.equal(body.legs.return.quantity,0);
});

test('invalid quantity is rejected before the bridge and remains visible in the draft',async()=>{
 const {api,C,ui,calls,elements,node,boxes}=fixture();const input=node('1,5',{tradeLeg:'outbound',tradeField:'quantity'});elements.push(input);api.handleInput({target:input},C,ui);
 await assert.rejects(api.handle('carrier-trade-save',null,C,ui),/Объём/);assert.equal(calls.length,0);assert.match(boxes.get('#carrier-trade-error').textContent,/допустимого/);assert.match(api.render(C,ui),/value="1,5"/);
});

test('switching to squadron preserves a valid minimum upkeep and mandatory service volume',async()=>{
 const {api,C,ui,calls,node}=fixture();await api.handleChange({target:node('squadron',{tradeOption:'carrierType'},{tagName:'SELECT'})},C,ui);await api.handle('carrier-trade-save',null,C,ui);
 assert.equal(calls[0].params.values.options.carrierType,'squadron');assert.equal(calls[0].params.values.options.weeklyUpkeep,10000000);assert.equal(calls[0].params.values.options.servicesTons,3000);assert.equal(calls[0].params.values.options.carrierInvestment,25000000000);
 assert.match(api.render(C,ui),/Расчёт Spansh поддерживает личный флотоносец/);
 assert.match(api.render(C,ui),/Расход трития эскадрильи нужно указать/);
});

test('changing commodity clears old prices and draft survives switching display modes',async()=>{
 const {api,C,ui,node}=fixture();C.state.carrierTrade.legs[0].buyPrice=1000;C.state.carrierTrade.legs[0].sellPrice=2000;
 await api.handleChange({target:node('liquor',{tradeLeg:'outbound',tradeField:'commodityKey'},{tagName:'SELECT'})},C,ui);
 assert.equal(C.carrierTradeDraft.legs.outbound.buyPrice,null);assert.equal(C.carrierTradeDraft.legs.outbound.sellPrice,null);C.mode='Расходы';api.render(C,ui);C.mode='Рейс';assert.match(api.render(C,ui),/value="liquor" selected/);assert.match(api.render(C,ui),/Есть несохранённые параметры/);
});

test('preset cancellation does not mutate the plan and imports address the selected leg',async()=>{
 const {api,C,ui,calls,dialogs}=fixture();await api.handle('carrier-trade-preset',null,C,ui);assert.equal(calls.length,0);assert.match(dialogs[0].footer,/close-dialog/);ui.closeLayer();
 await api.handle('carrier-trade-import-route',{dataset:{leg:'return'}},C,ui);assert.deepEqual(calls[0],{op:'carrier-trade',params:{values:{action:'import-route',leg:'return'}}});
 await api.handle('carrier-trade-import-carrier',{dataset:{}},C,ui);assert.equal(calls[1].params.values.action,'import-carrier');
});

test('a dirty plan is saved before importing or opening another calculation',async()=>{
 const {api,C,ui,calls,node}=fixture();api.handleInput({target:node('123',{tradeLeg:'outbound',tradeField:'quantity'})},C,ui);
 await api.handle('carrier-trade-import-route',{dataset:{leg:'outbound'}},C,ui);assert.equal(calls[0].params.values.action,'configure');assert.equal(calls[0].params.values.legs.outbound.quantity,123);assert.equal(calls[1].params.values.action,'import-route');
});

test('market picker uses supply or demand, pad and freshness filters then applies the chosen quote',async()=>{
 const {api,C,ui,calls,filters,boxes,node}=fixture();
 C.state.carrierTrade.options.haulerPad='L';
 const peakQuote={station:"Rackham's Peak",system:'HIP 58832',pad:'M',ageHours:1,distanceLs:100,quotes:[{key:'wine',price:4000,demand:50000}]};
 C.state.trading={running:false,options:{mode:'sell',resourceKey:'wine'},results:[{...peakQuote,station:'Other buyer',system:'B'},peakQuote]};
 await api.handle('carrier-trade-markets',{dataset:{leg:'outbound',side:'sell'}},C,ui);assert.match(boxes.get('#carrier-trade-market-results').innerHTML,/Rackham/);assert.match(boxes.get('#carrier-trade-market-results').innerHTML,/Площадка/);assert.doesNotMatch(boxes.get('#carrier-trade-market-results').innerHTML,/Other buyer|Не подходит/);
 filters.push(node('B',{tradeFilter:'system'}),node('M',{tradeFilter:'minPad'}),node('100',{tradeFilter:'radiusLy'}),node('6',{tradeFilter:'maxAgeHours'}),node('40 000',{tradeFilter:'availability'}),node('1000',{tradeFilter:'maxArrivalLs'}));
 await api.handle('carrier-trade-market-search',null,C,ui);const search=calls.find(row=>row.op==='carrier-trade-markets').params.values;assert.equal(search.system,'HIP 58832');assert.equal(search.minDemand,40000);assert.equal(search.minSupply,0);assert.equal(search.maxAgeHours,6);assert.equal(search.minPad,'M');assert.equal(search.includeCarriers,false);
 await api.handle('carrier-trade-market-choose',{dataset:{index:'0'}},C,ui);const command=calls.at(-1).params.values;assert.equal(command.action,'market');assert.equal(command.leg,'outbound');assert.equal(command.side,'sell');assert.equal(C.state.carrierTrade.legs[0].sellPrice,4000);assert.equal(boxes.has('#carrier-trade-market-results'),false);
});

test('a general plan can choose a buyer away from the Peak',async()=>{
 const {api,C,ui,calls,boxes}=fixture(),m=C.state.carrierTrade;
 Object.assign(m.options,{preset:'custom',plannedFullLoad:true});
 Object.assign(m.legs[0],{commodityKey:'gold',destinationSystem:'Beta'});
 const quote={station:'General Gold Buyer',system:'Beta',pad:'L',quotes:[{key:'gold',price:20000,demand:50000}]};
 C.state.trading={running:false,options:{mode:'sell',resourceKey:'gold',system:'Beta'},results:[quote]};
 await api.handle('carrier-trade-markets',{dataset:{leg:'outbound',side:'sell'}},C,ui);
 assert.match(boxes.get('#carrier-trade-market-results').innerHTML,/General Gold Buyer/);
 await api.handle('carrier-trade-market-choose',{dataset:{index:'0'}},C,ui);
 assert.equal(calls[0].params.values.market.system,'Beta');
});

test('market picker does not show unrelated prior results and marks a too-large ship',async()=>{
 const {api,C,ui,boxes}=fixture();C.state.carrierTrade.options.haulerPad='L';C.state.trading={running:false,options:{mode:'sell',resourceKey:'wine'},results:[{station:'Stale wrong commodity',pad:'M'}]};await api.handle('carrier-trade-markets',{dataset:{leg:'outbound',side:'buy'}},C,ui);assert.doesNotMatch(boxes.get('#carrier-trade-market-results').innerHTML,/Stale wrong/);await api.handle('carrier-trade-market-close',null,C,ui);
 C.state.trading={running:false,options:{mode:'buy',resourceKey:'wine'},results:[{station:'M only',pad:'M',quotes:[{key:'wine',price:1000,supply:1}]}]};await api.handle('carrier-trade-markets',{dataset:{leg:'outbound',side:'buy'}},C,ui);assert.match(boxes.get('#carrier-trade-market-results').innerHTML,/disabled title="Корабль требует большую площадку"/);await api.handle('carrier-trade-market-close',null,C,ui);
});

test('component smoke is mode independent and mutates nothing outside an isolated smoke host',async()=>{
 const {api,C,ui,calls}=fixture();C.mode='Проверки';const report=await api.smoke(C,ui);assert.equal(report.length,3);assert.ok(report.every(item=>item.ok));assert.equal(C.mode,'Проверки');assert.equal(calls.length,0);
});
