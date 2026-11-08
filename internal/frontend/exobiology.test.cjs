'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const clone=value=>JSON.parse(JSON.stringify(value));
const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const catalogue=Object.assign({},...fs.readdirSync(path.join(__dirname,'../localization')).filter(n=>n.endsWith('_en.json')).sort().map(n=>JSON.parse(fs.readFileSync(path.join(__dirname,'../localization',n),'utf8'))));
function deferred(){let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return {promise,resolve,reject};}
function fixture(language='ru'){
 const handlers=new Map(),window={CopilotTranslations:catalogue},context={window,document:{documentElement:{},addEventListener:(event,handler)=>{const list=handlers.get(event)||[];list.push(handler);handlers.set(event,list);}}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'i18n.js'),'utf8'),context);window.CopilotI18n.setLanguage(language);
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'exobiology.js'),'utf8'),context);
 const prediction={id:'exo/stratum-tectonicas',speciesId:'stratum-tectonicas',name:'Стратум Тектоникас',name_ru:'Стратум Тектоникас',name_en:'Stratum Tectonicas',genus:'Stratum',tier:'uncertain',estimatedCredits:19010800,minSampleDistanceM:500,matchedChecks:4,totalChecks:6,habitatMatchPercent:99,probabilityPercent:null,guaranteed:false,reasons:[],unknownConditions:['Давление неизвестно']};
 const body={key:'123:2',name:'Synthetic A 2',system:'Synthetic A',signals:3,mapped:true,firstFootfall:'unwalked',scan:{PlanetClass:'High metal content body',Atmosphere:'Thin Carbon Dioxide',SurfaceTemperature:175,SurfaceGravity:1.96133,DistanceFromArrivalLS:450},hostStar:{starType:'K'},predictions:[prediction],progress:{completeGenera:1}};
 const C={tab:'exobiology',mode:'Обзор',state:{settings:{language},exobiology:{settings:{includeBacterium:true,showUnlikely:false,minValue:0,minSignals:0,maxArrivalLs:100000,radiusLy:200,onlyUnwalked:false,pranavAntalBonus:false,trackSamples:true,predictionAlerts:true,searchQuery:''},selectedBody:body,currentBody:body,selectedBodyKey:body.key,currentBodyKey:body.key,bodies:[body],targets:[body],summary:{pendingSamples:1,pendingEstimatedCredits:19010800,pendingUnknownPrices:0,soldCredits:0},tracker:{bodyKey:body.key,speciesId:'stratum-tectonicas',name_ru:'Стратум Тектоникас',name_en:'Stratum Tectonicas',sampleCount:2,requiredSamples:3,phase:'tracking',minSampleDistanceM:500,nearestSampleDistanceM:null,remainingDistanceM:null,nearestSampleMeetsMin:null,recoveredSamples:2},pending:[{...prediction,system:'Synthetic A',body:body.name,sampleCount:3,phase:'complete',baseCredits:19010800,completedAt:'2026-11-08T12:00:00Z'}],sales:[],discoveries:[],bookmarks:[],surfaceMarks:[],position:null,predictionQuality:{observations:4,shown:3,unlikely:1,absent:0},stats:{since:'2026-11-01T12:00:00Z',periodCredits:0,periodCompletions:1,periodSales:0,firstFootfallBodies:1},alerts:[]}}};
 const calls=[],toasts=[],dialogs=[],nodes=new Map();let renders=0,loads=0,rpc=async()=>({ok:true});
 const u={esc,t:window.CopilotI18n.t,fmt:v=>window.CopilotI18n.number(v),money:v=>window.CopilotI18n.number(v)+' '+(language==='en'?'CR':'кр.'),date:window.CopilotI18n.date,
 icon:name=>'<svg data-icon="'+esc(name)+'"></svg>',btn:(label,action,ico='',kind='',attrs='')=>'<button type="button" data-click="'+esc(action)+'" '+attrs+'>'+esc(label)+'</button>',badge:(label,kind='')=>'<span class="badge '+esc(kind)+'">'+esc(label)+'</span>',heading:(title,description,action='')=>'<header><h1>'+esc(title)+'</h1><p>'+esc(description)+'</p>'+action+'</header>',
 call:async(op,params)=>{calls.push({op,params:clone(params)});return rpc(op,params);},loadView:async()=>{loads++;},renderPage(){renders++;},navigate:async(id,opts)=>{calls.push({op:'navigate',id,opts:clone(opts)});C.tab=id;},toast:(text,error)=>toasts.push({text,error}),dialog:(title,body,footer)=>dialogs.push({title,body,footer}),closeLayer(){C.layer=false;},$:selector=>nodes.get(selector)||null};
 window.ReferenceUI={open:async(id,context,ui)=>{assert.equal(context,C);assert.equal(ui,u);calls.push({op:'reference-open',id});}};
 return {api:window.ExobiologyUI,C,u,window,calls,toasts,dialogs,nodes,handlers,body,prediction,render(){return window.ExobiologyUI.render(C,u)},setRPC(value){rpc=value},get renders(){return renders},get loads(){return loads}};
}
const button=dataset=>({dataset});
test('all seven screens render in Russian and English while internal modes remain stable',()=>{
 for(const language of ['ru','en']){const f=fixture(language),names=['Обзор','Планеты','Образцы','Продажи','Находки','Места','Статистика'];for(const mode of names){f.C.mode=mode;const html=f.render();assert.match(html,/data-exobiology=/);assert.equal((html.match(/data-mode=/g)||[]).length,7);assert.ok(html.includes('data-mode="'+mode+'"'));assert.doesNotMatch(html,/NaN|undefined|onclick=/);assert.equal(f.calls.length,0);if(language==='en'){assert.match(html,/>Exobiology<\/h1>/);assert.doesNotMatch(html,/>Экзобиология<\/h1>/);}}}
});
test('sample tracker distinguishes unknown historical positions from confirmed sufficient distance',()=>{
 const f=fixture('en'),m=f.C.state.exobiology;let html=f.render();assert.equal((html.match(/<li class="complete">/g)||[]).length,2);assert.match(html,/Distance not known yet/);assert.match(html,/Samples without saved coordinates: 2/);assert.match(html,/Minimum separation/);assert.doesNotMatch(html,/99\s*%|probabilityPercent/);
 Object.assign(m.tracker,{positionKnown:true,nearestSampleDistanceM:550,remainingDistanceM:0,nearestSampleMeetsMin:true});html=f.render();assert.match(html,/Sufficient separation/);assert.match(html,/550 m/);assert.match(html,/Still to travel/);
 m.tracker.phase='complete';m.tracker.sampleCount=3;html=f.render();assert.match(html,/Three samples collected/);assert.equal((html.match(/<li class="complete">/g)||[]).length,3);
});
test('unknown body facts and valuations stay unknown and candidate reasons are disclosed on demand',()=>{
 const f=fixture('en');f.body.firstFootfall='unknown';f.body.scan={};f.body.valueRange={minimum:null,maximum:null};f.prediction.unknownConditions=['Unknown atmosphere'];let html=f.render();assert.match(html,/First-footfall status unknown/);assert.match(html,/Insufficient data/);assert.match(html,/<details class="exo-prediction-details">/);assert.match(html,/Unknown atmosphere/);assert.doesNotMatch(html,/<details class="exo-prediction-details" open|99%/);
 f.body.valueRange={minimum:100,maximum:500};html=f.render();assert.match(html,/100 CR — 500 CR/);assert.match(html,/not the sum of all candidates/);
});
test('empty filtered targets do not silently show all unfiltered planets',()=>{
 const f=fixture('en');f.C.mode='Планеты';f.C.state.exobiology.targets=[];let html=f.render();assert.match(html,/No matching planets/);assert.doesNotMatch(html,/class="panel exo-planet"/);
 f.C.state.exobiology.targets=Array.from({length:250},(_,i)=>({...f.body,key:'body-'+i,name:'Synthetic '+i}));html=f.render();assert.equal((html.match(/class="panel exo-planet"/g)||[]).length,12);assert.match(html,/of 200/);
});
test('body selection, bookmark actions and species deep links preserve exact identifiers',async()=>{
 const f=fixture('en');await f.api.handleChange({target:{dataset:{exoBody:''},value:'123:2'}},f.C,f.u);assert.deepEqual(f.calls[0],{op:'exobiology',params:{values:{action:'select-body',bodyKey:'123:2'}}});
 await f.api.handleClick('exo-bookmark',button({body:'123:2'}),f.C,f.u);await f.api.handleClick('exo-unbookmark',button({body:'123:2'}),f.C,f.u);assert.equal(f.calls[1].params.values.action,'bookmark');assert.equal(f.calls[2].params.values.action,'remove-bookmark');
 await f.api.handleClick('exo-species',button({species:'stratum-tectonicas'}),f.C,f.u);assert.equal(f.calls[3].opts.referenceCategory,'exobiology');assert.equal(f.calls[4].id,'exo/stratum-tectonicas');
});
test('criteria save uses normalized typed values and preserves drafts through repeated rendering',async()=>{
 const f=fixture('en');f.render();const input=(key,value,type='text')=>({target:{dataset:{exoConfig:key},value,type,checked:value}});
 f.api.handleInput(input('minValue','12 345,5'),f.C,f.u);f.api.handleInput(input('showUnlikely',true,'checkbox'),f.C,f.u);f.C.state.exobiology.settings.minValue=7;let html=f.render();assert.match(html,/data-exo-config="minValue"[^>]*value="12 345,5"/);assert.equal(f.C.state.exobiology.settings.minValue,7);
 await f.api.handleClick('exo-configure',null,f.C,f.u);const request=f.calls[0];assert.equal(request.op,'exobiology');assert.equal(request.params.values.minValue,12345.5);assert.equal(request.params.values.showUnlikely,true);assert.equal(request.params.values.action,'configure');
 f.api.handleInput(input('radiusLy',''),f.C,f.u);await assert.rejects(f.api.handleClick('exo-configure',null,f.C,f.u),/non-negative/);assert.equal(f.calls.length,1);
});
test('public site search is lazy, bounded and shows only the documented distance lower bound',async()=>{
 const f=fixture('en');f.C.mode='Места';f.render();assert.equal(f.calls.length,0);f.setRPC(async()=>({entries:[{system:'Настройки',region:'Orion',coordinatesKnown:false,distanceLy:null,distanceLowerBoundLy:123,observations:[f.prediction]}],total:25,offset:0,limit:12,observationDate:'2026-10-01'}));
 f.api.handleInput({target:{dataset:{exoSite:'query'},value:'Stratum'}},f.C,f.u);await f.api.handleClick('exo-sites-search',null,f.C,f.u);assert.equal(f.calls[0].op,'exo.sites');assert.equal(f.calls[0].params.limit,12);assert.equal(f.calls[0].params.query,'Stratum');let html=f.render();assert.match(html,/at least 123 ly/);assert.match(html,/Distance not confirmed/);assert.match(html,/omit the Y axis/);assert.match(html,/Настройки/);assert.doesNotMatch(html,/data-system-search/);
 await f.api.handleClick('exo-sites-next',null,f.C,f.u);assert.equal(f.calls[1].params.offset,12);await f.api.handleClick('exo-copy',button({system:'Настройки'}),f.C,f.u);assert.equal(f.calls.at(-1).params.text,'Настройки');
});
test('late site responses cannot replace newer results and failed searches remain retryable',async()=>{
 const f=fixture('en'),first=deferred(),second=deferred();let request=0;f.setRPC(()=>++request===1?first.promise:second.promise);const a=f.api.handleClick('exo-sites-search',null,f.C,f.u);f.C.exoWorkspace.sites.query='new';const b=f.api.handleClick('exo-sites-search',null,f.C,f.u);second.resolve({entries:[{system:'New system',observations:[]}],total:1,limit:12});await b;first.resolve({entries:[{system:'Old system'}],total:1,limit:12});await a;f.C.mode='Места';assert.match(f.render(),/New system/);assert.doesNotMatch(f.render(),/Old system/);
 f.setRPC(async()=>{throw Error('<network unavailable>')});await f.api.handleClick('exo-sites-search',null,f.C,f.u);const html=f.render();assert.match(html,/&lt;network unavailable&gt;/);assert.doesNotMatch(html,/data-click="exo-sites-search" disabled/);assert.match(html,/New system/);
});
test('manual waypoints use coordinates only from the selected body and validate before a write',async()=>{
 const f=fixture('en');f.C.state.exobiology.position={bodyKey:'different-body',latitude:0,longitude:0};await f.api.handleClick('exo-waypoint',null,f.C,f.u);assert.match(f.dialogs[0].body,/name="latitude"[^>]*value=""/);
 f.C.state.exobiology.position.bodyKey='123:2';await f.api.handleClick('exo-waypoint',null,f.C,f.u);assert.match(f.dialogs[1].body,/name="latitude"[^>]*value="0"/);
 const elements={bodyKey:{value:'123:2'},label:{value:'<Personal waypoint>'},latitude:{value:'91'},longitude:{value:'12,5'}},error={textContent:'',classList:{remove(){}}};f.nodes.set('#exo-waypoint-form',{elements});f.nodes.set('#exo-waypoint-error',error);await f.api.handleClick('exo-waypoint-save',null,f.C,f.u);assert.equal(f.calls.length,0);assert.match(error.textContent,/outside/);
 elements.latitude.value='-12,5';await f.api.handleClick('exo-waypoint-save',null,f.C,f.u);assert.deepEqual(f.calls[0].params.values,{action:'add-waypoint',bodyKey:'123:2',label:'<Personal waypoint>',latitude:-12.5,longitude:12.5});
});
test('sales, discoveries and saved places stay bounded and escape game and player fields',()=>{
 const f=fixture('en'),m=f.C.state.exobiology;f.body.name='<img src=x onerror=bad()>';f.body.system='Настройки';f.body.scan.Atmosphere='<script>bad()</script>';f.prediction.name_en='<svg onload=bad()>';f.prediction.unknownConditions=['<iframe src=bad>'];m.alerts=[{id:'" onclick="bad',message:'<b>warning</b>'}];let html=f.render();assert.match(html,/&lt;img/);assert.match(html,/&lt;svg/);assert.match(html,/&lt;script&gt;/);assert.doesNotMatch(html,/<img|<script|<iframe|" onclick=/);
 m.sales=Array.from({length:250},(_,i)=>({speciesId:'unknown',name_en:'Species',system:'Настройки',body:'<private>',credits:i===0?null:0}));f.C.mode='Продажи';html=f.render();assert.equal((html.match(/<tbody>/g)||[]).length,1);assert.equal((html.match(/<tr>/g)||[]).length,201);assert.match(html,/Unknown/);assert.match(html,/Настройки/);assert.match(html,/&lt;private&gt;/);
});
test('statistics reset is confirmed and leaves the history intact until the explicit action',async()=>{
 const f=fixture('en');f.C.mode='Статистика';const before=clone(f.C.state.exobiology);await f.api.handleClick('exo-reset-statistics',null,f.C,f.u);assert.equal(f.calls.length,0);assert.match(f.dialogs[0].body,/history will remain/);assert.deepEqual(f.C.state.exobiology,before);await f.api.handleClick('exo-reset-confirm',null,f.C,f.u);assert.equal(f.calls[0].params.values.action,'reset-statistics');
});
test('init binds once and ordinary fields do not trigger exobiology work',()=>{
 const f=fixture();for(let i=0;i<5;i++)f.api.init(f.C,f.u);assert.equal(f.handlers.get('submit').length,1);assert.equal(f.api.handleInput({target:{dataset:{},value:'Sol'}},f.C,f.u),false);assert.equal(f.calls.length,0);f.C.tab='sale';assert.equal(f.api.handleInput({target:{dataset:{exoConfig:'minValue'},value:'50'}},f.C,f.u),false);
});
test('rapid body choices are serialized and the latest choice opens the full workspace',async()=>{
 const f=fixture('en'),gate=deferred();f.C.mode='Планеты';let count=0;f.setRPC(()=>++count===1?gate.promise:Promise.resolve({ok:true}));
 const first=f.api.handleClick('exo-select-body',button({body:'body-A'}),f.C,f.u),second=f.api.handleClick('exo-select-body',button({body:'body-B'}),f.C,f.u);
 for(let i=0;i<5;i++)await Promise.resolve();assert.equal(f.calls.length,1);assert.equal(f.calls[0].params.values.bodyKey,'body-A');gate.resolve({ok:true});await Promise.all([first,second]);assert.equal(f.calls[1].params.values.bodyKey,'body-B');assert.equal(f.C.mode,'Обзор');assert.equal(f.loads,1);
});
test('saving criteria re-enables its control and backend changes refresh a clean draft',async()=>{
 const f=fixture('en');f.render();f.C.state.exobiology.settings.radiusLy=50;assert.match(f.render(),/data-exo-config="radiusLy"[^>]*value="50"/);
 let enabled=false;f.nodes.set('[data-click="exo-configure"]',{removeAttribute(key){assert.equal(key,'disabled');enabled=true;}});await f.api.handleClick('exo-configure',null,f.C,f.u);assert.equal(enabled,true);
});
test('genera progress keeps DSS evidence separate from samples and completed analysis',()=>{
 const f=fixture('en');f.body.progress.genera=[{name_en:'Bacterium',stage:'dss',sampleCount:0},{name_en:'Stratum',stage:'sampling',sampleCount:2},{name_en:'Tussock',stage:'complete',sampleCount:3}];const html=f.render();assert.match(html,/Genus confirmed by DSS/);assert.match(html,/Sampling · 2 \/ 3/);assert.match(html,/Analysis complete/);
});
test('surface marks distinguish scanner origins, approximate estimates, ship and real samples',()=>{
 const f=fixture('en');f.C.mode='Места';f.C.state.exobiology.surfaceMarks=[
  {id:'scan',kind:'codex',label:'Synthetic scanner',scannedFrom:true,confidence:'scanner-origin',latitude:1,longitude:2,distanceM:42,bearing:90},
  {id:'cross',kind:'codex',label:'Synthetic crossing',confidence:'triangulated-estimate',locationBasis:'crossing',latitude:3,longitude:4,distanceM:80,bearing:180},
  {id:'ship',kind:'ship',label:'Synthetic ship',confidence:'recorded-position',latitude:5,longitude:6,distanceM:null,bearing:null},
  {id:'sample',kind:'sample',label:'Synthetic sample',confidence:'recorded-position',latitude:7,longitude:8,distanceM:15,bearing:20},
  {id:'own',kind:'waypoint',label:'<private>',confidence:'recorded-position',latitude:9,longitude:10,distanceM:20,bearing:40}
 ];
 const html=f.render(),marks=html.slice(html.indexOf('<ul class="exo-marks">'),html.indexOf('</ul>',html.indexOf('<ul class="exo-marks">')));
 assert.match(marks,/Scanner position/);assert.match(marks,/not the plant/);assert.match(marks,/Approximate triangulation/);assert.match(marks,/≈ 80/);assert.match(marks,/≈ 180/);assert.match(marks,/>Ship</);assert.match(marks,/>Sample</);assert.match(marks,/Recorded position/);assert.match(marks,/&lt;private&gt;/);
 assert.equal((marks.match(/data-click="exo-remove-waypoint"/g)||[]).length,1);assert.match(marks,/data-id="own"/);assert.equal((marks.match(/≈ /g)||[]).length,2);
 f.C.mode='Образцы';assert.match(f.render(),/<th>Species<\/th>/);assert.equal(f.window.CopilotI18n.t('Вид'),'View');
});
test('system survey reports FSS and DSS evidence without inventing an unknown body total',()=>{
 const f=fixture('en');assert.doesNotMatch(f.render(),/exo-system-progress/);f.C.state.exobiology.system={name:'<Private system>',bodiesTotal:null,scannedBodies:3,fssComplete:false,biologicalBodies:2,biologicalSignals:4,mappedBiologicalBodies:1,sampledGenera:1,fullySampledBodies:0};
 let html=f.render();assert.match(html,/System survey/);assert.match(html,/&lt;Private system&gt;/);assert.match(html,/FSS incomplete/);assert.match(html,/total body count is unknown/);assert.doesNotMatch(html,/3 \/ 0/);
 f.C.state.exobiology.system.bodiesTotal=8;f.C.state.exobiology.system.fssComplete=true;html=f.render();assert.match(html,/FSS complete/);assert.match(html,/3 \/ 8/);assert.doesNotMatch(html,/total body count is unknown/);f.C.mode='Продажи';assert.doesNotMatch(f.render(),/exo-system-progress/);
});
