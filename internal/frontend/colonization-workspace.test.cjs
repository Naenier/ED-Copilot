'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const clone=value=>JSON.parse(JSON.stringify(value));
function deferred(){let resolve,reject;const promise=new Promise((yes,no)=>{resolve=yes;reject=no;});return {promise,resolve,reject};}
function fixture(){
 const window={},calls=[],toasts=[],nodes=new Map();let renders=0,loads=0,rpc=async()=>({ok:true});
 const C={tab:'colonization',mode:'Выбор системы',selected:'unrelated-row',query:'filter',actions:[{id:'colony.create'},{id:'colony.edit'}],state:{commander:{system:'Current system'},colonization:{projects:[],activeProject:null,search:{options:{},results:[],running:false}}}};
 const ui={esc,fmt:value=>String(value),icon:name=>'<svg data-icon="'+esc(name)+'"></svg>',badge:value=>'<span class="badge">'+esc(value)+'</span>',
  btn:(label,action,icon,kind,attrs)=>'<button data-click="'+action+'" '+attrs+'>'+esc(label)+'</button>',
  call:async(op,params)=>{calls.push({op,params:clone(params)});return rpc(op,params);},
  renderPage(){renders++;},loadView:async()=>{loads++;},toast:value=>toasts.push(value),
  context:()=>({tab:C.tab,mode:C.mode,selected:C.selected,query:C.query}),$:selector=>nodes.get(selector),
 };
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'colonization-workspace.js'),'utf8'),{window});
 const api=window.ColonyWorkspaceUI;
 const origin=(name='Home system')=>C.state.colonization.search={running:false,origin:{name,faction:'System controlling faction'},options:{originSystem:name},factions:[{name:'Station Faction'},{name:'System controlling faction'}],claimStations:[{marketId:9001,name:'Home port',system:name,faction:'Station Faction',hasColonisationContact:true,distanceLs:0,type:'Coriolis'},{marketId:9002,name:'Other port',system:name,faction:'Other Station Faction',hasColonisationContact:null}]};
 return {C,ui,api,calls,toasts,nodes,origin,setRPC(value){rpc=value;},get renders(){return renders;},get loads(){return loads;}};
}
function project(id,name,extra={}){
 return {id,name,targetSystem:name+' system',originSystem:name+' origin',faction:name+' faction',status:'building',buildings:[{status:'building'},{status:'planned'}],unknownBills:1,totalMissing:80,concurrent:1,points:{tier2:0,tier3:null},economyPlan:{name:'Промышленность'},operationalSupplies:[{commodity:'steel',name:'Сталь',missing:80,cargoAvailable:0}],nextTask:{kind:'delivery',text:'Доставить сталь — 80 т'},...extra};
}

test('initial colony workflow explains station discovery without an empty faction selector or implicit network request',()=>{
 const f=fixture(),before=clone(f.C.state),html=f.api.render(f.C,f.ui);
 assert.match(html,/Спланируйте новую колонию/);assert.match(html,/Проверить систему/);
 assert.match(html,/Фракцию не нужно угадывать/);assert.match(html,/Станция → фракция → новая система/);
 assert.doesNotMatch(html,/id="colony-workspace-station"|id="colony-workspace-faction"/);
 assert.equal(f.C.colonizationWorkspace.origin,'Current system');assert.equal(f.calls.length,0);
 assert.deepEqual(f.C.state,before);
 f.C.tab='overview';assert.equal(f.api.render(f.C,f.ui),'');
});

test('the first phase explicitly clears a saved faction and cannot accidentally act on a selected row or colony',async()=>{
 const f=fixture();f.C.state.colonization.config={originSystem:'Saved origin',faction:'Old faction'};
 f.api.render(f.C,f.ui);f.api.handleInput({target:{id:'colony-workspace-origin',value:' New origin '}},f.C,f.ui);
 await f.api.handleClick('colony-workspace-check',null,f.C,f.ui);
 assert.deepEqual(f.calls,[{op:'action',params:{tab:'colonization',mode:'Выбор системы',selected:'',query:'',actionId:'colony.search',values:{originSystem:'New origin',faction:''}}}]);
 assert.equal(f.loads,1);assert.equal(f.C.state.colonization.config.faction,'Old faction');
});

test('a station selection uses its exact controlling faction rather than the faction of its system',async()=>{
 const f=fixture();f.origin();f.C.state.colonization.originSystem='Home system';f.api.render(f.C,f.ui);
 let focused=0;f.nodes.set('#colony-workspace-station',{focus(){focused++;}});
 assert.equal(f.api.handleChange({target:{id:'colony-workspace-station',value:'9001::Home port::Station Faction'}},f.C,f.ui),true);
 assert.equal(f.C.colonizationWorkspace.faction,'Station Faction');assert.equal(focused,1);
 let html=f.api.render(f.C,f.ui);assert.match(html,/Контакт колонизации обнаружен/);assert.match(html,/0 св. с./);
 await f.api.handleClick('colony-workspace-find',null,f.C,f.ui);
 assert.equal(f.calls[0].params.values.faction,'Station Faction');assert.notEqual(f.calls[0].params.values.faction,f.C.state.colonization.search.origin.faction);
 assert.equal(f.calls[0].params.values.originSystem,'Home system');
 f.api.handleChange({target:{id:'colony-workspace-station',value:'9002::Other port::Other Station Faction'}},f.C,f.ui);
 html=f.api.render(f.C,f.ui);assert.match(html,/Наличие контакта не подтверждено/);assert.doesNotMatch(html,/Контакт колонизации обнаружен/);
});

test('changed origin invalidates old station choices and manual faction entry remains an explicit option',async()=>{
 const f=fixture();f.origin();f.C.state.colonization.originSystem='Home system';f.api.render(f.C,f.ui);
 f.api.handleChange({target:{id:'colony-workspace-station',value:'9001::Home port::Station Faction'}},f.C,f.ui);
 const station={value:'9001::Home port::Station Faction'},faction={value:'Station Faction'},check={hidden:false},details={hidden:false},stale={hidden:true};
 f.nodes.set('#colony-workspace-station',station);f.nodes.set('#colony-workspace-faction',faction);f.nodes.set('.colony-station-check',check);f.nodes.set('[data-colony-source-ready]',details);f.nodes.set('[data-colony-source-stale]',stale);
 f.api.handleInput({target:{id:'colony-workspace-origin',value:'Other origin'}},f.C,f.ui);
 assert.equal(f.C.colonizationWorkspace.station,'');assert.equal(f.C.colonizationWorkspace.faction,'');
 assert.equal(station.value,'');assert.equal(faction.value,'');assert.equal(check.hidden,true);assert.equal(details.hidden,true);assert.equal(stale.hidden,false);
 assert.doesNotMatch(f.api.render(f.C,f.ui),/id="colony-workspace-station"/);
 await f.api.handleClick('colony-workspace-find',null,f.C,f.ui);
 assert.equal(f.calls.length,0);assert.match(f.C.colonizationWorkspace.error,/Сначала проверьте/);
 f.origin('Other origin');
 f.api.handleInput({target:{id:'colony-workspace-faction',value:'Confirmed in game'}},f.C,f.ui);
 await f.api.handleClick('colony-workspace-find',null,f.C,f.ui);
 assert.equal(f.calls[0].params.values.faction,'Confirmed in game');
});

test('search validation and asynchronous failures preserve drafts and make retry available',async()=>{
 const f=fixture();f.api.render(f.C,f.ui);
 f.api.handleInput({target:{id:'colony-workspace-origin',value:' '}},f.C,f.ui);
 await f.api.handleClick('colony-workspace-check',null,f.C,f.ui);
 assert.equal(f.calls.length,0);assert.match(f.C.colonizationWorkspace.error,/Укажите точное название/);
 const pending=deferred();f.setRPC(()=>pending.promise);
 f.api.handleInput({target:{id:'colony-workspace-origin',value:'Keep this origin'}},f.C,f.ui);
 const request=f.api.handleClick('colony-workspace-check',null,f.C,f.ui);
 await f.api.handleClick('colony-workspace-check',null,f.C,f.ui);
 assert.equal(f.calls.length,1);assert.equal(f.C.colonizationWorkspace.pending,true);
 assert.match(f.api.render(f.C,f.ui),/data-click="colony-workspace-check"[^>]*disabled/);
 pending.reject(new Error('<Ошибка сети>'));await request;
 assert.equal(f.C.colonizationWorkspace.pending,false);assert.equal(f.C.colonizationWorkspace.origin,'Keep this origin');
 assert.match(f.api.render(f.C,f.ui),/&lt;Ошибка сети&gt;/);assert.equal(f.loads,0);
 f.setRPC(async()=>({ok:true}));await f.api.handleClick('colony-workspace-check',null,f.C,f.ui);assert.equal(f.calls.length,2);
});

test('colony summary switches between independent projects and distinguishes unknown bills from zero remaining',()=>{
 const f=fixture();f.C.mode='Строительство';
 const alpha=project('a','Alpha'),beta=project('b','Beta',{unknownBills:2,totalMissing:0,points:{tier2:null,tier3:0},operationalSupplies:[]});
 f.C.state.colonization.projects=[alpha,beta];f.C.state.colonization.activeProject=alpha;
 let html=f.api.render(f.C,f.ui);
 assert.match(html,/data-colony-id="a"/);assert.match(html,/Alpha origin/);assert.match(html,/Alpha faction/);
 assert.match(html,/Не менее 80 т/);assert.match(html,/T2 0 · T3 Неизвестно/);assert.match(html,/В корабле: 0 т/);
 assert.doesNotMatch(html,/id="colony-workspace-origin"|Beta origin/);
 f.C.state.colonization.activeProject=beta;html=f.api.render(f.C,f.ui);
 assert.match(html,/data-colony-id="b"/);assert.match(html,/Beta origin/);assert.doesNotMatch(html,/Alpha origin/);
 assert.match(html,/<span>Осталось доставить<\/span><strong>Неизвестно/);
 assert.match(html,/T2 Неизвестно · T3 0/);
 beta.unknownBills=0;html=f.api.render(f.C,f.ui);assert.match(html,/<span>Осталось доставить<\/span><strong>0 т/);
});

test('copy uses the active colony rather than stale button data and every rendered value is escaped',async()=>{
 const f=fixture();f.C.state.colonization.activeProject=project('a','<Colony>',{targetSystem:'Actual current system',warnings:['<script>bad()</script>'],nextTask:{text:'<img src=x>'}});
 let html=f.api.render(f.C,f.ui);
 assert.match(html,/&lt;Colony&gt;/);assert.match(html,/&lt;script&gt;bad\(\)&lt;\/script&gt;/);assert.match(html,/&lt;img src=x&gt;/);
 assert.doesNotMatch(html,/<script|<img/);
 await f.api.handleClick('colony-workspace-copy',{dataset:{system:'Stale or forged system'}},f.C,f.ui);
 assert.deepEqual(f.calls[0],{op:'native',params:{action:'copy',text:'Actual current system'}});assert.equal(f.toasts.length,1);
 f.origin();f.C.colonizationWorkspace.origin='Home system';f.C.state.colonization.search.claimStations[0].name='<Station>';
 html=f.api.render(f.C,f.ui);assert.match(html,/&lt;Station&gt;/);
});

test('unrelated root events and hidden screens cause no action, RPC, timer or persistence changes',async()=>{
 const f=fixture();f.C.tab='settings';
 assert.equal(f.api.handleInput({target:{id:'colony-workspace-origin',value:'X'}},f.C,f.ui),false);
 assert.equal(f.api.handleChange({target:{id:'colony-workspace-station',value:'X'}},f.C,f.ui),false);
 assert.equal(await f.api.handleClick('colony-workspace-check',null,f.C,f.ui),false);assert.equal(f.C.colonizationWorkspace,undefined);
 f.C.tab='colonization';assert.equal(f.api.handleInput({target:{id:'other',value:'X'}},f.C,f.ui),false);
 assert.equal(f.api.handleChange({target:{id:'other',value:'X'}},f.C,f.ui),false);
 assert.equal(await f.api.handleClick('action',null,f.C,f.ui),false);assert.equal(f.calls.length,0);
});

test('source station choices exclude carriers, construction sites, depots and explicit absent contacts',async()=>{
 const f=fixture();f.origin();f.C.state.colonization.originSystem='Home system';
 const search=f.C.state.colonization.search;
 const rejected=[{marketId:1,name:'Excluded Fleet',type:'FleetCarrier',faction:'FleetCarrier'},{marketId:2,name:'Excluded Squadron',type:'Squadron Carrier',faction:'Actual Faction'},{marketId:3,name:'Excluded Site',type:'OrbitalConstructionSite',faction:'Builder Corporation'},{marketId:4,name:'Excluded Depot',type:'ColonisationConstructionDepot',faction:'Builder Corporation'},{marketId:5,name:'Excluded Explicit False',type:'Coriolis',faction:'Actual Faction',hasColonisationContact:false},{marketId:6,name:'Excluded Legacy',faction:'FleetCarrier'},{marketId:7,name:'Excluded Known Depot',faction:'Actual Faction',constructionDepot:true}];
 search.claimStations.push(...rejected);search.factions.push({name:'FleetCarrier'},{name:'Independent Carrier Traders'});
 search.claimStations.push({marketId:8,name:'Depot Named Ordinary Port',type:'Coriolis',faction:'Actual Faction',hasColonisationContact:true});
 const html=f.api.render(f.C,f.ui);
 for(const row of rejected)assert.doesNotMatch(html,new RegExp(row.name));
 assert.doesNotMatch(html,/value="FleetCarrier"/);assert.match(html,/Independent Carrier Traders/);assert.match(html,/Depot Named Ordinary Port/);assert.match(html,/Home port/);
 f.api.handleChange({target:{id:'colony-workspace-station',value:'1::Excluded Fleet::FleetCarrier'}},f.C,f.ui);
 assert.equal(f.C.colonizationWorkspace.station,'');assert.equal(f.C.colonizationWorkspace.faction,'');
 await f.api.handleClick('colony-workspace-find',null,f.C,f.ui);assert.equal(f.calls.length,0);
});

test('legacy origin station fallback is filtered and an explicit empty eligible list never falls back to all stations',()=>{
 const f=fixture();f.origin();f.C.state.colonization.originSystem='Home system';
 const search=f.C.state.colonization.search;
 search.origin.stations=[...search.claimStations,{marketId:3,name:'Fallback Carrier',type:'Fleet Carrier',faction:'FleetCarrier'},{marketId:4,name:'Fallback Depot',type:'Depot',faction:'Real Faction'}];delete search.claimStations;
 let html=f.api.render(f.C,f.ui);assert.match(html,/Home port/);assert.doesNotMatch(html,/Fallback Carrier|Fallback Depot/);
 search.claimStations=[];html=f.api.render(f.C,f.ui);assert.doesNotMatch(html,/id="colony-workspace-station"/);
});

test('unknown contact remains selectable with an explicit game check and stale ineligible selections are cleared',()=>{
 const f=fixture();f.origin();f.C.state.colonization.originSystem='Home system';f.api.render(f.C,f.ui);
 f.api.handleChange({target:{id:'colony-workspace-station',value:'9002::Other port::Other Station Faction'}},f.C,f.ui);
 let html=f.api.render(f.C,f.ui);assert.match(html,/контакт не подтверждён/);assert.match(html,/Наличие контакта не подтверждено — проверьте в игре/);
 f.C.state.colonization.search.claimStations[1].hasColonisationContact=false;f.api.init(f.C,f.ui);
 assert.equal(f.C.colonizationWorkspace.station,'');assert.equal(f.C.colonizationWorkspace.faction,'');
 html=f.api.render(f.C,f.ui);assert.doesNotMatch(html,/Other port/);
});

test('manual placeholder faction is rejected before any search or persistence request',async()=>{
 const f=fixture();f.origin();f.C.state.colonization.originSystem='Home system';f.api.render(f.C,f.ui);
 f.api.handleInput({target:{id:'colony-workspace-faction',value:'FleetCarrier'}},f.C,f.ui);
 await f.api.handleClick('colony-workspace-find',null,f.C,f.ui);
 assert.equal(f.calls.length,0);assert.match(f.C.colonizationWorkspace.error,/малую фракцию обычной/);
});
