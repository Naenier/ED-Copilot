'use strict';
const {test}=require('node:test'), assert=require('node:assert/strict');
const fs=require('node:fs'), path=require('node:path'), vm=require('node:vm');

const source=fs.readFileSync(path.join(__dirname,'engineering-roadmap.js'),'utf8');
const dictionary=JSON.parse(fs.readFileSync(path.join(__dirname,'../localization/engineering_roadmap_ui_en.json'),'utf8'));
const escapeHTML=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const blueprint={key:'LongRange',name:'Увеличенная дальность',english:'Increased Range',maxGrade:5};
const moduleChoice={key:'fsd',name:'Рамочно-сдвиговый двигатель',english:'Frame Shift Drive'};
const material={key:'arsenic',name:'Мышьяк',english:'Arsenic',count:4,owned:0,missing:4};
const prerequisite={key:'first',kind:'ship',name:'Первый',english:'First engineer',status:'known',dependency:true,system:'A <System>',base:'Station & Base',
  steps:[{kind:'unlock',status:'pending',text:'Доставьте ресурс',english:'Deliver the commodity',current:0,target:5,items:[{...material,owned:null,missing:null}]}]};
const engineer={key:'second',kind:'ship',name:'Второй',english:'Second engineer',status:'unlocked',rank:4,
  steps:[{kind:'rank',status:'pending',text:'Повысьте репутацию',english:'Increase reputation'}],
  capabilities:[{moduleKey:moduleChoice.key,moduleName:moduleChoice.name,moduleEnglish:moduleChoice.english,blueprintKey:blueprint.key,blueprintName:blueprint.name,blueprintEnglish:blueprint.english,maxGrade:5}]};
const data={filters:{engineers:[prerequisite,engineer],modules:[moduleChoice],blueprints:[blueprint],experimentals:[],equipment:[]},
  engineers:[prerequisite,engineer],summary:{total:2,unlocked:1,maxRank:0},
  recipe:{moduleKey:moduleChoice.key,blueprintKey:blueprint.key,grade:5,confidence:'estimate',current:{grade:3,quality:.8},totals:[material],
    grades:[{grade:4,rolls:4,status:'pending',materials:[material]}],engineer}};

function fixture(call=async()=>data) {
  const listeners=new Map(),timers=new Map();
  let timer=0;
  const document={activeElement:null,getElementById:()=>null};
  const window={};
  const host={addEventListener:(type,listener)=>listeners.set(type,listener),querySelectorAll:()=>[]};
  const C={tab:'engineering',mode:'Прокачка',state:{commander:{fid:'a'},settings:{language:'en'},revision:1}};
  const u={call,esc:escapeHTML,fmt:String,icon:()=>'',heading:(title,description)=>`<h1>${escapeHTML(title)}</h1><p>${escapeHTML(description)}</p>`,
    $:selector=>selector==='[data-engineering-roadmap]'?host:null,
    t:(key,vars)=>(C.state.settings.language==='en'?dictionary[key]||key:key).replace(/\{(\w+)\}/g,(raw,name)=>vars?.[name]??raw)};
  vm.runInNewContext(source,{window,document,setTimeout:callback=>{timers.set(++timer,callback);return timer;},clearTimeout:id=>timers.delete(id)});
  return {api:window.EngineeringRoadmapUI,C,u,listeners,
    flush(){for(const [id,callback] of timers){timers.delete(id);callback();}}
  };
}

test('roadmap renders dependency order, workshop links and localized estimated material costs',async()=>{
  const {api,C,u}=fixture();
  await api.fetchPlan(C,u);
  let html=api.render(C,u);
  assert.match(html,/Engineer progression/);
  assert.ok(html.indexOf('data-engineering-card="first"')<html.indexOf('data-engineering-card="second"'));
  assert.match(html,/First engineer/);
  assert.match(html,/A &lt;System&gt; · Station &amp; Base/);
  assert.match(html,/data-system="A &lt;System&gt;"/);
  assert.match(html,/Prerequisite/);
  assert.match(html,/Deliver the commodity/);
  assert.match(html,/0 \/ 5/);
  assert.match(html,/Frame Shift Drive · Increased Range · G5/);
  assert.match(html,/applications is estimated/);
  assert.match(html,/Applications: 4/);
  assert.match(html,/Arsenic<\/th><td>4<\/td>\s*<td>0<\/td><td class="eng-shortage">4/);
  assert.match(html,/Arsenic<\/th><td>4<\/td>\s*<td>—<\/td>/);
  C.state.settings.language='ru';html=api.render(C,u);
  assert.match(html,/Прокачка инженеров/);
  assert.match(html,/Первый/);
  assert.match(html,/Доставьте ресурс/);
});

test('multi-engineer selection is debounced and a module change clears incompatible options',async()=>{
  const calls=[];
  const {api,C,u,listeners,flush}=fixture(async(op,body)=>{calls.push([op,JSON.parse(JSON.stringify(body))]);return data;});
  api.init(C,u);
  await Promise.resolve();
  const change=listeners.get('change');
  change({target:{dataset:{engineeringEngineer:'second'},checked:true}});
  change({target:{dataset:{engineeringEngineer:'first'},checked:true}});
  flush();
  await Promise.resolve();
  assert.equal(calls.length,2);
  assert.deepEqual(calls.at(-1),['engineering-roadmap',{engineerIDs:['first','second'],moduleKey:'',blueprintKey:'',grade:0,experimentalKey:'',equipmentKey:''}]);
  const local=C.engineeringRoadmap.filters;
  Object.assign(local,{moduleKey:'thrusters',blueprintKey:'dirty',grade:4,experimentalKey:'x',equipmentKey:'y'});
  change({target:{dataset:{engineeringFilter:'moduleKey'},value:'fsd'}});
  flush();
  await Promise.resolve();
  assert.deepEqual(calls.at(-1)[1],{engineerIDs:['first','second'],moduleKey:'fsd',blueprintKey:'',grade:0,experimentalKey:'',equipmentKey:''});
});

test('a late roadmap response cannot replace a newer selection or reveal the previous commander',async()=>{
  const requests=[];
  const {api,C,u}=fixture(()=>new Promise(resolve=>requests.push(resolve)));
  const before=api.fetchPlan(C,u);
  api.preselect(C,'second');
  const after=api.fetchPlan(C,u);
  requests[1]({...data,engineers:[engineer]});
  await after;
  requests[0]({...data,engineers:[prerequisite]});
  await before;
  assert.match(api.render(C,u),/data-engineering-card="second"/);
  assert.doesNotMatch(api.render(C,u),/data-engineering-card="first"/);
  C.state.commander.fid='b';
  assert.doesNotMatch(api.render(C,u),/data-engineering-card="second"/);
  const pending=api.fetchPlan(C,u);
  api.leave(C);C.tab='overview';requests[2](data);
  await pending;
  assert.equal(api.active(C),false);
});

test('quiet view polls use the engineering revision and reload once on a language change',async()=>{
  const requests=[];
  const {api,C,u}=fixture(async(_,params)=>{requests.push(params);return data;});
  C.state.engineeringRoadmapRevision=2;
  C.state.planWorkspace={key:'live-a',branch:'live'};
  await api.fetchPlan(C,u);
  assert.equal(requests[0].expectedPlanWorkspace,'live-a');
  C.state.revision++;
  await api.fetchPlan(C,u);
  assert.equal(requests.length,1);
  C.state.engineeringRoadmapRevision++;
  await api.fetchPlan(C,u);
  assert.equal(requests.length,2);
  C.state.settings.language='ru';
  await api.fetchPlan(C,u);
  assert.equal(requests.length,3);
  C.state.planWorkspace={key:'legacy-a',branch:'legacy'};
  assert.doesNotMatch(api.render(C,u),/data-engineering-card="second"/);
});

test('legacy recipes expose per-application costs and Merc variants do not inflate the regular total',async()=>{
  const {api,C,u}=fixture(async()=>({...data,
    recipe:{...data.recipe,totals:null,confidence:'unknown',conditions:['rank-engineer'],grades:[{grade:3,perRoll:[material],materials:[],effects:[{label:'Range',value:'+14%'}]}]},
    specialRecipes:[{key:'special',english:'Merc variant',acquisition:'mercenary',variants:[{english:'Merc equipment',initialGrade:1,mercCoins:950}],grades:[{grade:2,perRoll:[material]}]}]
  }));
  await api.fetchPlan(C,u);
  const html=api.render(C,u);
  assert.match(html,/Raise engineer reputation to the target grade/);
  assert.match(html,/Per application/);
  assert.match(html,/Range<\/dt><dd>\+14%/);
  assert.match(html,/Merc Coins: 950/);
  assert.match(html,/Special modules/);
  assert.doesNotMatch(html.split('class="eng-order-list"')[0],/On hand/);
});
