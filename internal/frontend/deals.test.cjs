'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
function fixture(){
 const observedAt=new Date(Date.now()-3600000).toISOString();
 const station=(id,system,name,price,supply,demand)=>({id,system,station:name,type:'Coriolis',pad:'L',distanceLs:420,observedAt,updatedAt:observedAt,ageHours:1,quotes:[{key:'gold',price,supply,demand,quantity:50}]});
 const leg={key:'gold',name:'Золото',buy:station('A','Alpha','Source Port',100,1000,0),sell:station('B','Beta','Buyer Port',200,0,1000),quantity:50,buyPrice:100,sellPrice:200,supply:1000,demand:1000,safeDemand:250,purchaseCost:5000,revenue:10000,grossMargin:5000,operatingCost:500,netEstimate:4500,profitPerTonne:100,marginPercent:100,distanceLy:30,approachDistanceLy:5,minutes:15,profitPerHour:18000,limitingFactors:['capital']};
 const back={...leg,key:'silver',name:'Серебро',buy:{...leg.sell},sell:{...leg.buy}};
 const route={id:'route|1',legs:[leg,back],roundTrip:true,netEstimate:9000,grossMargin:10000,minutes:30,profitPerHour:18000,distanceLy:60,minimumCapital:5500,warnings:[]};
 const C={tab:'deals',state:{commander:{system:'Alpha',cargoCapacity:100,cargoTons:20},pilot:{credits:100000},tradeRoutes:{results:[route],checked:2,total:4,assumptions:'Доход по введённому времени.'},goods:[{key:'gold',name:'Золото'},{key:'silver',name:'Серебро'}],savedTradeRoutes:[]}};
 const listeners=new Map(),ctx={window:{},document:{addEventListener:(type,handler)=>listeners.set(type,handler)},Date,Intl};vm.runInNewContext(fs.readFileSync(path.join(__dirname,'deals.js'),'utf8'),ctx);
 const calls=[],inputs=[],dialogs=[],toasts=[],error={textContent:'',classList:{remove(){}}},query={focus(){this.focused=true},setSelectionRange(start){this.start=start}};
 const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
 const u={esc,icon:()=>'<svg></svg>',fmt:value=>value==null||value===''?'—':new Intl.NumberFormat('ru-RU').format(Number(value)),money:value=>value==null?'—':value+' кр.',btn:(label,action,icon,style,attrs='')=>`<button data-click="${action}" ${attrs}>${esc(label)}</button>`,badge:text=>`<span>${esc(text)}</span>`,heading:(title,description,extra='')=>`<h1>${esc(title)}</h1><p>${esc(description)}</p>${extra}`,call:async(op,params)=>{calls.push({op,params:params===undefined?undefined:JSON.parse(JSON.stringify(params))});return {ok:true}},loadView:async()=>{},renderPage(){},toast:text=>toasts.push(text),dialog:(title,html,footer)=>dialogs.push({title,html,footer}),closeLayer(){},$:selector=>selector==='#deals-form-error'?error:selector==='#deals-query'?query:null,$$:()=>inputs};
 const button=(extra={})=>({dataset:{id:route.id,leg:'0',...extra}});
 const input=(key,value,extra={})=>({dataset:{dealsFilter:key},value,type:'text',matches:()=>true,...extra});
 return{api:ctx.window.DealsUI,C,u,calls,inputs,dialogs,listeners,route,leg,button,input,error,query,toasts};
}
test('renders both complete legs, systems and stations, price, stock, demand, age and arrival',()=>{
 const {api,C,u,leg}=fixture(),html=api.render(C,u);
 for(const text of ['Alpha','Beta','Source Port','Buyer Port','Золото','Серебро','Запас','Спрос','От звезды','Туда','Обратно','Начальный капитал','по вашему времени','Назначить покупателя','Сохранить план плеча','Сохранить весь рейс'])assert.ok(html.includes(text),text);
 assert.equal((html.match(/data-copy="system"/g)||[]).length,4);assert.equal((html.match(/data-copy="station"/g)||[]).length,4);assert.ok(html.includes('datetime="'+leg.buy.observedAt+'"'));assert.doesNotMatch(html,/NaN|undefined|onclick=/);
});
test('uses known free hold and real pilot balance while preserving unknown and zero',()=>{
 const {api,C,u}=fixture();let html=api.render(C,u);assert.match(html,/data-deals-filter="cargoCapacity" value="80"/);assert.match(html,/data-deals-filter="credits" value="100000"/);
 C.dealsUI=null;C.state.commander.cargoCapacity=null;C.state.pilot.credits=null;html=api.render(C,u);assert.match(html,/data-deals-filter="cargoCapacity" value=""/);assert.match(html,/data-deals-filter="credits" value=""/);
 C.dealsUI=null;C.state.commander.cargoCapacity=0;C.state.pilot.credits=0;html=api.render(C,u);assert.match(html,/data-deals-filter="cargoCapacity" value="0"/);assert.match(html,/data-deals-filter="credits" value="0"/);
});
test('search sends the actual normalized filters, six resources, and Russian numeric input',async()=>{
 const {api,C,u,calls,inputs,input}=fixture();inputs.push(input('cargoCapacity','120'),input('credits','12 500,5'),input('marketType','carriers'),input('resource0','gold'),input('resource1','gold'),input('resource2',''),input('minDemand','2 000'));
 await api.handle('deals-search',null,C,u);const body=calls[0].params.values;assert.equal(calls[0].op,'deals-search');assert.equal(body.credits,12500.5);assert.equal(body.cargoCapacity,120);assert.equal(body.includeCarriers,true);assert.equal(body.minDemand,2000);assert.equal(body.resourceKeys.filter(key=>key==='gold').length,1);assert.equal(body.resourceKeys.includes(''),false);
});
test('invalid or unknown finance prevents any external search and gives a field error',async()=>{
 const {api,C,u,calls,inputs,input,error}=fixture();inputs.push(input('credits',''));await assert.rejects(api.handle('deals-search',null,C,u),/Бюджет/);assert.equal(calls.length,0);assert.match(error.textContent,/укажите число/);
});
test('copy selects each exact endpoint field through native clipboard without navigation or backend mutation',async()=>{
 const {api,C,u,calls,button}=fixture();await api.handle('deals-copy',button({side:'buy',copy:'system'}),C,u);await api.handle('deals-copy',button({side:'sell',copy:'station',leg:'1'}),C,u);
 assert.deepEqual(calls,[{op:'native',params:{action:'copy',text:'Alpha'}},{op:'native',params:{action:'copy',text:'Source Port'}}]);assert.equal(C.selected,undefined);
});
test('selected leg destination and saved plan carry complete quote snapshots and correct quantity',async()=>{
 const {api,C,u,calls,button}=fixture();await api.handle('deals-destination',button({leg:'1'}),C,u);assert.equal(calls[0].op,'deals-destination');assert.equal(calls[0].params.values.destination.system,'Alpha');
 await api.handle('deals-plan',button({leg:'1'}),C,u);const plan=calls[1].params.values;assert.equal(plan.key,'silver');assert.equal(plan.quantity,50);assert.equal(plan.source,'Beta');assert.equal(plan.destination,'Alpha');assert.equal(plan.buy.station,'Buyer Port');assert.equal(plan.sell.station,'Source Port');
});
test('full route save preserves both legs, saved routes can be opened and removal requires confirmation',async()=>{
 const {api,C,u,calls,button,route,dialogs}=fixture();await api.handle('deals-save',button(),C,u);assert.equal(calls[0].op,'deals-saved-route');assert.equal(calls[0].params.values.route.legs.length,2);
 C.state.savedTradeRoutes=[{...route,id:'saved-1'}];await api.handle('deals-view',{dataset:{view:'saved'}},C,u);assert.match(api.render(C,u),/deals-delete/);await api.handle('deals-select',button({id:'saved-1'}),C,u);assert.equal(C.dealsUI.selected,'saved-1');
 await api.handle('deals-delete',button({id:'saved-1'}),C,u);assert.equal(calls.length,1);assert.match(dialogs[0].footer,/close-dialog/);assert.match(dialogs[0].footer,/deals-delete-confirm/);
 await api.handle('deals-delete-confirm',button({id:'saved-1'}),C,u);assert.deepEqual(calls[1].params.values,{action:'delete-route',id:'saved-1'});
});
test('result pagination mounts only ten complete cards and filtering searches both systems and stations',async()=>{
 const {api,C,u,route,query}=fixture();C.state.tradeRoutes.results=Array.from({length:25},(_,i)=>({...route,id:'route'+i}));let html=api.render(C,u);assert.equal((html.match(/data-deals-route=/g)||[]).length,10);assert.match(html,/1 \/ 3/);
 await api.handle('deals-next',null,C,u);html=api.render(C,u);assert.match(html,/2 \/ 3/);api.handleInput({target:{id:'deals-query',value:'missing',selectionStart:7}},C,u);assert.match(api.render(C,u),/Совпадений нет/);assert.equal(query.focused,true);assert.equal(query.start,7);
 api.handleInput({target:{id:'deals-query',value:'Buyer Port',selectionStart:10}},C,u);assert.equal((api.render(C,u).match(/data-deals-route=/g)||[]).length,10);
});
test('backend text is escaped and unknown observation values never become a fresh quote',()=>{
 const {api,C,u,leg}=fixture();leg.buy.system='<img src=x onerror=alert(1)>';leg.buy.station='<script>x</script>';leg.buy.observedAt=null;leg.buy.updatedAt=null;leg.buy.ageHours=null;leg.buy.distanceLs=null;const html=api.render(C,u);assert.match(html,/&lt;img/);assert.match(html,/&lt;script&gt;/);assert.match(html,/Время цены неизвестно/);assert.match(html,/От звезды: <b>— св. с./);assert.doesNotMatch(html,/<img|<script/);
});
test('unrelated global input does not initialize stale route filters and buttons do not submit twice',()=>{
 const {api,C,u,listeners}=fixture();assert.equal(api.handleInput({target:{id:'settings-other',matches:()=>false}},C,u),false);assert.equal(C.dealsUI,undefined);api.init(C,u);const first=listeners.get('submit');api.init(C,u);assert.equal(listeners.get('submit'),first);assert.match(api.render(C,u),/data-click="deals-search" type="button"/);
});
test('opening trade finds routes automatically using confirmed hold, credits and ship pad',async()=>{
 const {api,C,u,calls}=fixture();
 C.mode='Поиск';C.state.tradeRoutes={};C.state.commander.shipPad='M';
 assert.match(api.render(C,u),/Подбираем выгодные рейсы/);
 assert.doesNotMatch(api.render(C,u),/Рейсы не найдены/);
 const first=api.init(C,u), duplicate=api.init(C,u);
 await Promise.all([first,duplicate]);
 assert.equal(calls.length,1);
 const {values}=calls[0].params;
 assert.equal(calls[0].op,'deals-search');
 assert.equal(values.automatic,true);
 assert.equal(values.system,'Alpha');
 assert.equal(values.cargoCapacity,80);
 assert.equal(values.credits,100000);
 assert.equal(values.minPad,'M');
 C.state.tradeRoutes={options:values,updatedAt:new Date().toISOString(),results:[]};
 await api.init(C,u);
 assert.equal(calls.length,1);
});

test('automatic routes do not guess empty cargo, funding or saved-route intent',async()=>{
 const {api,C,u,calls}=fixture();
 C.state.tradeRoutes={};C.mode='Поиск';C.state.commander.cargoKnown=false;
 assert.match(api.render(C,u),/Укажите свободный трюм/);
 await api.init(C,u);
 C.state.commander.cargoKnown=true;C.state.pilot.credits=null;
 assert.match(api.render(C,u),/Укажите бюджет закупки/);
 await api.init(C,u);
 C.state.pilot.credits=100000;C.mode='Сохранённые';
 await api.init(C,u);
 assert.equal(calls.length,0);
});
test('an empty manual search from a previous session does not suppress automatic suggestions',async()=>{
 const {api,C,u,calls}=fixture();
 C.mode='Поиск';C.state.commander.shipPad='M';
 C.state.tradeRoutes={options:{system:'Alpha',automatic:false},updatedAt:new Date().toISOString(),results:[]};
 await api.init(C,u);
 assert.equal(calls.length,1);
 assert.equal(calls[0].params.values.automatic,true);
});
test('component smoke is independent of selected result or saved view',async()=>{const {api,C,u}=fixture();api.render(C,u);C.dealsUI.view='saved';assert.ok((await api.smoke(C,u)).every(check=>check.ok));});

test('raw child-market pages and malformed legacy records never become empty route cards',async()=>{
 const {api,C,u,route,calls,button}=fixture();C.state.tradeRoutes.results=[{id:'market-legacy',station:'Legacy Empty Port'},null,{}, {id:'bad',legs:[]},{id:'bad-leg',legs:[{}]},route];
 let html=api.render(C,u);assert.equal((html.match(/data-deals-route=/g)||[]).length,1);assert.doesNotMatch(html,/Legacy Empty Port/);assert.match(html,/Требуется повторный поиск/);assert.match(html,/Результаты <span>1<\/span>/);
 await assert.rejects(api.handle('deals-select',button({id:'market-legacy'}),C,u),/Рейс больше не найден/);assert.equal(calls.length,0);
 C.state.tradeRoutes.results=[{id:'market-only',station:'Legacy Empty Port'}];html=api.render(C,u);assert.equal((html.match(/data-deals-route=/g)||[]).length,0);assert.match(html,/<details class="panel deals-search" open>/);
 C.state.savedTradeRoutes=[{id:'broken',station:'Broken saved station'}];C.dealsUI.view='saved';assert.doesNotMatch(api.render(C,u),/Broken saved station/);
});

test('current shipment plan exposes both saved quote points compactly and keeps legacy zero values',()=>{
 const {api,C,u,leg}=fixture();C.state.tradePlan={key:'gold',name:'Золото',quantity:50,buyPrice:100,sellPrice:200,buy:leg.buy,sell:leg.sell};let html=api.render(C,u);assert.match(html,/Текущий план перевозки/);assert.match(html,/<details class="panel deals-active-plan" >/);assert.match(html,/Source Port/);assert.match(html,/Buyer Port/);assert.match(html,/deals-plan-clear/);
 C.state.tradePlan={key:'gold',quantity:1,buyPrice:0,sellPrice:0,source:'Legacy source',destination:'Legacy destination'};html=api.render(C,u);assert.match(html,/Legacy source/);assert.match(html,/Legacy destination/);assert.equal((html.match(/Станция не указана/g)||[]).length,2);assert.match(html,/<b>0 кр\.\/т<\/b>/);
 C.state.tradeRoutes.results=[];assert.match(api.render(C,u),/<details class="panel deals-active-plan" open>/);
});

test('clearing current plan requires explicit confirmation and uses only the clear-plan API',async()=>{
 const {api,C,u,calls,dialogs}=fixture();C.state.tradePlan={key:'gold',quantity:50};const before=JSON.stringify(C.state.tradePlan);
 await api.handle('deals-plan-clear',null,C,u);assert.equal(calls.length,0);assert.match(dialogs[0].footer,/close-dialog/);assert.match(dialogs[0].footer,/deals-plan-clear-confirm/);u.closeLayer();assert.equal(JSON.stringify(C.state.tradePlan),before);
 await api.handle('deals-plan-clear-confirm',null,C,u);assert.deepEqual(calls,[{op:'deals-plan-clear',params:undefined}]);
});

test('station types are localized for display while raw market types survive copy and save commands',async()=>{
 const {api,C,u,leg,route,button,calls}=fixture();leg.buy.type='Planetary Outpost';leg.sell.type='Ocellus Starport';const before=JSON.stringify(route);
 let html=api.render(C,u);assert.match(html,/Планетарный аванпост/);assert.match(html,/Космопорт «Оцеллус»/);assert.doesNotMatch(html,/Planetary Outpost|Ocellus Starport/);assert.equal(JSON.stringify(route),before);
 await api.handle('deals-plan',button(),C,u);assert.equal(calls[0].params.values.buy.type,'Planetary Outpost');assert.equal(calls[0].params.values.sell.type,'Ocellus Starport');
 await api.handle('deals-copy',button({side:'buy',copy:'station'}),C,u);assert.equal(calls[1].params.text,'Source Port');
 leg.buy.type='<img src=x onerror=alert(1)>';html=api.render(C,u);assert.match(html,/&lt;img/);assert.doesNotMatch(html,/<img/);
});

test('unknown costs and capital remain unknown and a missing leg result never has a positive sign',()=>{
 const {api,C,u,leg,route}=fixture();leg.operatingCost=null;leg.netEstimate=null;route.minimumCapital=null;route.netEstimate=null;route.profitPerHour=null;
 const html=api.render(C,u);assert.match(html,/Расходы: <b>—<\/b>/);assert.match(html,/Начальный капитал<\/span><strong>—<\/strong>/);assert.doesNotMatch(html,/\+—/);
 leg.operatingCost=0;leg.netEstimate=0;assert.match(api.render(C,u),/Расходы: <b>0 кр\.<\/b>/);
});

test('price age never treats missing timestamps or a future observation as a fresh zero-hour quote',()=>{
 const {api,C,u,leg,route}=fixture();leg.buy.observedAt=null;leg.buy.updatedAt=null;leg.buy.ageHours=0;let html=api.render(C,u);assert.match(html,/Время цены неизвестно/);assert.match(html,/deals-observed stale/);
 leg.buy.observedAt=new Date(Date.now()+48*3600000).toISOString();html=api.render(C,u);assert.match(html,/Время котировки в будущем/);assert.match(html,/Проверьте перед рейсом/);
 leg.buy.observedAt=new Date(Date.now()-30*3600000).toISOString();route.options={maxAgeHours:24,carrierMaxAgeHours:6};html=api.render(C,u);assert.match(html,/30 ч назад/);assert.match(html,/местное время/);assert.match(html,/deals-observed stale/);
});
