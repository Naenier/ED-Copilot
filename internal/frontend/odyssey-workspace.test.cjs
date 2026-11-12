'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const source=fs.readFileSync(path.join(__dirname,'odyssey-workspace.js'),'utf8');
const dictionary=Object.assign({},...fs.readdirSync(path.join(__dirname,'../localization')).filter(name=>name.endsWith('_en.json')).map(file=>JSON.parse(fs.readFileSync(path.join(__dirname,'../localization',file),'utf8'))));
const copy=value=>JSON.parse(JSON.stringify(value));
const esc=value=>String(value??'').replace(/[&<>"']/g,key=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[key]));
const wait=()=>new Promise(resolve=>setImmediate(resolve));
function fixture(lang='en'){
 const window={},events=new Map(),calls=[],focused=[];
 vm.runInNewContext(source,{window,Date});
 const api=window.OdysseyUI,C={tab:'engineering',mode:'Снаряжение Odyssey',state:{planWorkspace:{key:'owner-a'},settings:{language:lang},odysseyRevision:1}};
 let rpc=async()=>sample(),markup='';
 const host={ownerDocument:{activeElement:null,getElementById:id=>({focus:()=>focused.push(id)})},
  addEventListener:(name,handler)=>events.set(name,handler),get innerHTML(){return markup;},set innerHTML(value){markup=value;}};
 const t=(key,vars={})=>String(lang==='en'?dictionary[key]??key:key).replace(/{(\w+)}/g,(all,name)=>vars[name]??all);
 const ui={t,esc,fmt:String,locale:()=>lang==='en'?'en-GB':'ru-RU',date:stamp=>stamp.slice(0,10),
  heading:title=>'<h1>'+esc(title)+'</h1>',badge:value=>'<span>'+esc(value)+'</span>',$:selector=>selector==='#odyssey-workspace'?host:null,
  call:async(op,params)=>{calls.push({op,params:copy(params)});return rpc(op,params);}};
 function click(action,dataset={}){
  events.get('click')({target:{closest:()=>({dataset:{odysseyAction:action,...dataset}})}});
 }
 return {C,ui,api,host,events,calls,focused,click,setRPC:value=>{rpc=value;}};
}
const suit={key:'utilitysuit',name:'Маверик',english:'Maverick',moduleKey:'utilitysuit',slot:'suit',primarySlots:1};
const primary={key:'rifle',name:'Карабин',english:'Rifle',moduleKey:'weapon',damageType:'kinetic',slot:'primary'};
const secondary={key:'pistol',name:'Пистолет',english:'Pistol',moduleKey:'weapon',damageType:'plasma',slot:'secondary'};
const mod={key:'quieter',name:'Подавление шума',english:'Audio masking',moduleKey:'utilitysuit'};
const engineer={key:'engineer',name:'Инженер',english:'Engineer',entryId:'engineer/engineer'};
const material={key:'steel',name:'Сталь',english:'Steel',entryId:'material/steel',count:5,backpack:0,locker:null,missing:null,missingAtMost:5,backpackObservedAt:'2030-04-21T00:00:00Z'};
function sample(){
 const loadouts=[{id:'kit-1',name:'План <A>',reserve:true,items:[{itemKey:suit.key,equipmentKey:'utilitysuit:11',startingGrade:1,targetGrade:3,modKeys:[mod.key]}]},
  {id:'kit-2',name:'Evening kit',reserve:false,items:[{itemKey:primary.key,startingGrade:1,targetGrade:2,modKeys:[]}]}];
 const item={...loadouts[0].items[0],name:'Маверик',english:'Maverick',currentGrade:2,installedMods:[],conditions:['missing-recipe'],
  steps:[{kind:'grade',grade:3,status:'pending',materials:[material]},{kind:'modification',...mod,status:'pending',engineers:[engineer]}]};
 return {workspace:'owner-a',selectedId:'kit-1',loadouts,activeLoadout:{...loadouts[0],items:[item]},equipment:[suit,primary,secondary],
  ownedEquipment:[{key:'utilitysuit:11',itemKey:suit.key,grade:2,english:'Maverick'}],modifications:[mod],materials:[material],portfolioMaterials:[{...material,count:8,locker:8,sharedMissing:0}]};
}

test('Odyssey loads only for its Engineering subsection and distinguishes a confirmed zero from unknown stock',async()=>{
 const f=fixture();
 f.C.mode='План';f.api.init(f.C,f.ui);assert.equal(f.calls.length,0);
 f.C.mode='Снаряжение Odyssey';assert.match(f.api.render(f.C,f.ui),/<h1>Odyssey loadouts<\/h1>.*Loading equipment/);
 f.api.init(f.C,f.ui);await f.C.odysseyUI.pending;
 assert.deepEqual(f.calls,[{op:'odyssey.loadouts',params:{expectedPlanWorkspace:'owner-a'}}]);
 const html=f.api.render(f.C,f.ui);
 assert.match(html,/План &lt;A&gt;/);assert.match(html,/Maverick.*Grade 2 → 3/);
 assert.match(html,/Upgrade to grade 3/);assert.match(html,/material\/steel.*Steel/);assert.match(html,/engineer\/engineer.*Engineer/);
 assert.match(html,/Audio masking/);assert.match(html,/The recipe for this upgrade is unknown/);
 assert.match(html,/Backpack.*2030-04-21.*Ship locker/);assert.match(html,/<td>5<\/td><td>0<\/td><td>Unknown<\/td><td>At most 5<\/td>/);
 assert.match(html,/Requirements including reservations.*<td>8<\/td><td>0<\/td><td>8<\/td><td>0<\/td>/);
 f.api.init(f.C,f.ui);assert.equal(f.calls.length,1);
 f.C.state.odysseyRevision++;f.api.init(f.C,f.ui);await f.C.odysseyUI.pending;assert.equal(f.calls.length,2);
});

test('Odyssey never presents an incomplete recipe as zero demand',async()=>{
 const f=fixture('ru');
 f.setRPC(async()=>({...sample(),materials:[],portfolioMaterials:[],activeComplete:false,portfolioComplete:false}));
 f.api.init(f.C,f.ui);await f.C.odysseyUI.pending;
 const html=f.api.render(f.C,f.ui);
 assert.match(html,/Часть улучшений не вошла в расчёт материалов/);
 assert.doesNotMatch(html,/Материалы для этих улучшений не требуются/);
});

test('a journal update during the initial Odyssey request triggers a current follow-up without another render',async()=>{
 const f=fixture();let resolveOlder;
 f.setRPC(()=>f.calls.length===1?new Promise(resolve=>{resolveOlder=resolve;}):Promise.resolve(sample()));
 f.api.init(f.C,f.ui);const older=f.C.odysseyUI.pending;
 f.C.state.odysseyRevision++;
 f.api.init(f.C,f.ui);
 assert.equal(f.calls.length,1);
 resolveOlder({...sample(),activeLoadout:{...sample().activeLoadout,name:'Older journal data'}});
 await older;
 assert.equal(f.calls.length,2);
 await f.C.odysseyUI.pending;
 assert.doesNotMatch(f.host.innerHTML,/Older journal data/);
 assert.match(f.host.innerHTML,/План &lt;A&gt;/);
});

test('Odyssey saves a compatible edited loadout and switches to another saved goal',async()=>{
 const f=fixture();
 f.setRPC(async(op,params)=>{
  const data=sample();if(op==='odyssey.loadout-update'&&params.action==='select')data.selectedId=params.id;
  return data;
 });
 f.api.init(f.C,f.ui);await f.C.odysseyUI.pending;
 f.click('edit');
 assert.match(f.host.innerHTML,/Edit loadout/);assert.match(f.host.innerHTML,/Audio masking/);
 f.events.get('input')({target:{id:'odyssey-kit-name',value:'Updated'}});
 f.click('add-item');
 assert.match(f.host.innerHTML,/Primary weapon/);
 f.click('add-item');
 assert.match(f.host.innerHTML,/Secondary weapon/);
 assert.doesNotMatch(f.host.innerHTML,/data-odyssey-action="add-item"/);
 f.events.get('submit')({target:{id:'odyssey-kit-form'},preventDefault(){}});
 await wait();
 assert.deepEqual(f.calls[1],{op:'odyssey.loadout-update',params:{action:'save',id:'kit-1',name:'Updated',reserve:true,
  items:[{itemKey:'utilitysuit',equipmentKey:'utilitysuit:11',startingGrade:1,targetGrade:3,modKeys:['quieter']},
   {itemKey:'rifle',equipmentKey:'',startingGrade:1,targetGrade:1,modKeys:[]},
   {itemKey:'pistol',equipmentKey:'',startingGrade:1,targetGrade:1,modKeys:[]}],expectedPlanWorkspace:'owner-a'}});
 f.events.get('change')({target:{id:'odyssey-active-kit',value:'kit-2'}});await wait();
 assert.deepEqual(f.calls[2],{op:'odyssey.loadout-update',params:{action:'select',id:'kit-2',expectedPlanWorkspace:'owner-a'}});
});

test('Odyssey discards older workspace data and exposes no write control without a commander',async()=>{
 const f=fixture();let release;
 f.setRPC((_,params)=>params.expectedPlanWorkspace==='owner-a'?new Promise(resolve=>{release=resolve;}):Promise.resolve({loadouts:[]}));
 f.api.init(f.C,f.ui);const older=f.C.odysseyUI.pending;
 f.C.state.planWorkspace={key:''};f.api.init(f.C,f.ui);await f.C.odysseyUI.pending;
 release(sample());await older;
 const html=f.api.render(f.C,f.ui);
 assert.doesNotMatch(html,/Evening kit|План/);assert.match(html,/id="odyssey-new-kit" disabled/);
 assert.match(html,/Start a game session to load your commander/);
});
