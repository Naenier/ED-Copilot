'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
function fixture(language='en'){
 const at='2026-11-08T10:00:00Z',meta={known:true,source:'spansh',observedAt:at,fetchedAt:at};
 return {tab:'politics',state:{settings:{language},politics:{selectedSystem:'Fixture',currentSystem:'Current',activeSystem:{name:'Fixture',address:'9007199254740999',info:{...meta,controllingFaction:'<script>bad()</script>',population:0},factions:{...meta,entries:[{name:'War Faction',influence:0,state:'War',activeStates:['War'],pendingStates:['Boom']},{name:'Election Faction',influence:null,state:'Election',activeStates:['Election']}]},conflicts:{known:true,source:'journal',observedAt:at,entries:[{type:'war',status:'active',faction1:{name:'War Faction',stake:'Port',wonDays:0},faction2:{name:'Other',wonDays:2}}]},powerplay:{...meta,controllingPower:'Power A',state:'Stronghold',controlProgress:0,reinforcement:0,undermining:null},thargoid:{...meta,state:'None',progress:null}},systems:[{name:'Fixture'}],communityGoals:{...meta,source:'frontier-public',entries:[{id:'1',title:'<img onerror=bad()>',system:'Campaign System',station:'Campaign Port',objective:'Deliver <supplies>',currentTotal:0,target:100,description:'Reward <script>bad()</script>'}]},personalGoals:{known:true,source:'journal',observedAt:at,entries:[{id:'1',title:'Joined goal',playerContribution:0,bonus:200,percentileBand:75,tier:'Tier 1',complete:false}]},powerIndex:{...meta,powers:[{name:'Power A',indexedSystems:12},{name:'Power B',indexedSystems:0}],states:[{name:'Stronghold',indexedSystems:2},{name:'Fortified',indexedSystems:3}]},search:{...meta,kind:'powerplay',power:'Power A',state:'Contested',page:1,limit:10,total:22,entries:[{name:'Public result',powerplay:{...meta,controllingPower:'Power A',state:'Contested'}}]},status:{running:false},sources:[{name:'Spansh',url:'https://spansh.co.uk/systems'},{name:'bad',url:'javascript:bad()'}]}}};
}
function runtime(C=fixture()){
 const listeners=new Map(),scope=vm.createContext({window:{},document:{addEventListener:(name,fn)=>listeners.set(name,fn)}});
 vm.runInContext(fs.readFileSync(path.join(__dirname,'politics.js'),'utf8'),scope);
 const calls=[],esc=value=>String(value??'').replace(/[&<>"']/g,ch=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
 const u={esc,fmt:n=>String(n),heading:(title,sub)=>`<h1>${esc(title)}</h1><p>${esc(sub)}</p>`,call:async(op,body)=>{calls.push({op,body:JSON.parse(JSON.stringify(body))});return {};},loadView:async()=>calls.push({op:'loadView'}),renderPage:()=>calls.push({op:'renderPage'}),toast:()=>{},$:()=>null};
 return {C,u,api:scope.window.PoliticsUI,calls,listeners,window:scope.window,document:scope.document};
}

test('overview retains known zero, unknown data and escaped source names without starting network work',()=>{
 const r=runtime(),html=r.api.render(r.C,r.u);
 assert.match(html,/Galactic politics/);assert.match(html,/data-system-search="single"/);assert.match(html,/9007199254740999/);
 assert.match(html,/&lt;script&gt;bad/);assert.doesNotMatch(html,/<script>|javascript:bad/);assert.match(html,/>0<\/dd>/);assert.match(html,/Unknown/);
 assert.equal(r.calls.length,0,'render must never refresh public feeds');
 const ru=runtime(fixture('ru'));assert.match(ru.api.render(ru.C,ru.u),/Политика галактики/);
});

test('public and personal goals stay separate, descriptions are escaped and zero contribution remains visible',async()=>{
 const r=runtime();await r.api.handleClick('politics-mode',{dataset:{view:'goals'}},r.C,r.u);const html=r.api.render(r.C,r.u);
 assert.match(html,/Public community goals/);assert.match(html,/Your journal goals/);assert.match(html,/Your contribution/);assert.match(html,/percentile band/);
 assert.match(html,/&lt;img onerror=bad/);assert.match(html,/Deliver &lt;supplies&gt;/);assert.doesNotMatch(html,/<img|<script/);
 assert.match(html,/aria-valuenow="0"/);assert.equal(r.calls.filter(x=>x.op==='politics-refresh').length,0);
 await r.api.handleClick('politics-refresh',{dataset:{kind:'goals'}},r.C,r.u);
 assert.deepEqual(r.calls.find(x=>x.op==='politics-refresh').body,{values:{kind:'goals',page:0}});
});

test('power index supports actual power selection, contested filter and bounded pagination',async()=>{
 const r=runtime();await r.api.handleClick('politics-mode',{dataset:{view:'powerplay'}},r.C,r.u);let html=r.api.render(r.C,r.u);
 assert.match(html,/Galactic power index/);assert.match(html,/Power B/);assert.match(html,/Contested/);assert.match(html,/indexed systems/);
 r.api.handleInput({target:{dataset:{politicsField:'state'},value:'Contested'}},r.C,r.u);
 await r.api.handleClick('politics-power',{dataset:{power:'Power A'}},r.C,r.u);
 assert.deepEqual(r.calls.find(x=>x.op==='politics-refresh').body,{values:{kind:'powerplay',page:0,power:'Power A',state:'Contested'}});
 await r.api.handleClick('politics-page',{dataset:{kind:'powerplay',page:'2'}},r.C,r.u);
 assert.deepEqual(r.calls.filter(x=>x.op==='politics-refresh').at(-1).body,{values:{kind:'powerplay',page:2,power:'Power A',state:'Contested'}});
});

test('faction state filtering does not fabricate exact conflict opponents',async()=>{
 const r=runtime();await r.api.handleClick('politics-mode',{dataset:{view:'factions'}},r.C,r.u);
 r.api.handleChange({target:{dataset:{politicsField:'conflict'},value:'election'}},r.C,r.u);
 let html=r.api.render(r.C,r.u);assert.match(html,/Election Faction/);assert.doesNotMatch(html,/War Faction/);assert.match(html,/No matching conflict/);
 delete r.C.state.politics.activeSystem.conflicts;r.api.handleChange({target:{dataset:{politicsField:'conflict'},value:'war'}},r.C,r.u);
 html=r.api.render(r.C,r.u);assert.match(html,/does not establish its opponent or score/);
});

test('search results navigate to their system without hidden network refresh and names can be copied',async()=>{
 const r=runtime();await r.api.handleClick('politics-system',{dataset:{system:'Public result'}},r.C,r.u);
 assert.deepEqual(r.calls[0],{op:'politics',body:{values:{action:'select-system',system:'Public result'}}});
 assert.equal(r.calls.filter(c=>c.op==='politics-refresh').length,0);
 await r.api.handleClick('politics-copy',{dataset:{system:'Public result'}},r.C,r.u);
 assert.deepEqual(r.calls.at(-1),{op:'native',body:{action:'copy',text:'Public result'}});
 assert.equal(await r.api.handleClick('unrelated',{},r.C,r.u),false);
});

test('Thargoid index explicitly distinguishes an empty public snapshot and a decommissioned source',async()=>{
 const r=runtime();r.C.state.politics.search={known:true,kind:'thargoid',entries:[],total:0,indexHasAffectedStates:false,fetchedAt:'2026-11-08T10:00:00Z'};
 await r.api.handleClick('politics-mode',{dataset:{view:'thargoid'}},r.C,r.u);const html=r.api.render(r.C,r.u);
 assert.match(html,/DCoH Overwatch is decommissioned/);assert.match(html,/does not prove the absence/);assert.match(html,/Indexed matches/);assert.doesNotMatch(html,/galaxy is safe|no Thargoids remain/i);
});

test('system form uses typed autocomplete value and binds one submit handler',async()=>{
 const r=runtime();r.api.init(r.C,r.u);r.api.init(r.C,r.u);
 r.api.handleInput({target:{dataset:{politicsField:'system'},value:'Typed System'}},r.C,r.u);
 let prevented=false;r.listeners.get('submit')({target:{id:'politics-system-form'},preventDefault:()=>{prevented=true;}});
 await new Promise(resolve=>setImmediate(resolve));assert.equal(prevented,true);
 assert.deepEqual(r.calls.find(c=>c.op==='politics-refresh').body,{values:{kind:'system',page:0,system:'Typed System'}});
});

test('refresh immediately unlocks rendered controls and preserves the typed draft and input cursor',async()=>{
 const r=runtime();r.api.handleInput({target:{dataset:{politicsField:'system'},value:'Typed System'}},r.C,r.u);
 const focused={dataset:{politicsField:'system'},selectionStart:3,selectionEnd:7};r.document.activeElement=focused;
 let rendered='',restored=0,selection;
 const nextInput={focus:()=>{restored++;},setSelectionRange:(start,end)=>{selection=[start,end];}};
 r.u.$=selector=>selector==='[data-politics-field="system"]'?nextInput:null;
 r.u.loadView=async()=>{rendered=r.api.render(r.C,r.u);assert.match(rendered,/<button class="pill primary" type="submit" disabled>/);r.document.activeElement=null;};
 r.u.renderPage=()=>{rendered=r.api.render(r.C,r.u);};
 await r.api.handleClick('politics-refresh',{dataset:{kind:'system'}},r.C,r.u);
 assert.match(rendered,/<button class="pill primary" type="submit" >Look up system/);assert.doesNotMatch(rendered,/data-click="politics-current"[^>]*disabled/);
 assert.match(rendered,/value="Typed System"/);assert.equal(r.C.politicsUI.system,'Typed System');assert.equal(restored,1);assert.deepEqual(selection,[3,7]);
 const other=runtime();let rerenders=0;other.u.loadView=async()=>{other.C.tab='overview';};other.u.renderPage=()=>{rerenders++;};
 await other.api.handleClick('politics-refresh',{dataset:{kind:'system'}},other.C,other.u);assert.equal(rerenders,0);assert.equal(other.C.politicsUI.pending,false);
});

test('canonical and legacy localized government/security enums follow the interface language without changing identities',()=>{
 for(const [government,security] of [['Democracy','Medium'],['$government_Democracy;','$SYSTEM_SECURITY_medium;'],['Демократия','Средняя']]){
  const C=fixture('en');C.state.politics.activeSystem.info={known:true,source:'journal',government,security,controllingFaction:'Фракция Демократия'};
  const r=runtime(C);let html=r.api.render(C,r.u);assert.match(html,/>Democracy<\/dd>/);assert.match(html,/>Medium<\/dd>/);assert.match(html,/Фракция Демократия/);
  C.state.settings.language='ru';html=r.api.render(C,r.u);assert.match(html,/>Демократия<\/dd>/);assert.match(html,/>Средняя<\/dd>/);assert.match(html,/Фракция Демократия/);
  C.state.politics.activeSystem.info.government='FutureGovernance';assert.match(r.api.render(C,r.u),/FutureGovernance/);
 }
});

test('isolated smoke follows actual controls, preserves view state and never requests public data',async()=>{
 const r=runtime();await assert.rejects(r.api.smoke(r.C,r.u),/isolated synthetic/);
 r.window.COPILOT_SMOKE=true;r.C.politicsUI={view:'goals',system:'Typed value',power:'Power A',state:'Contested',query:'user query',conflict:'war',pending:false};
 const before={...r.C.politicsUI},details={open:false,querySelector:()=>({textContent:'Fictional reward details'})};
 r.u.$=selector=>{
  const mode=selector.match(/data-view="([^"]+)"/);if(mode)return {dataset:{click:'politics-mode',view:mode[1]}};
  if(selector==='#politics-system')return {dataset:{systemSearch:'single'}};
  if(selector==='.pol-goal details')return details;
  if(selector==='[data-click="politics-copy"]')return {dataset:{click:'politics-copy',system:'Fixture'},disabled:false};
  return {};
 };
 r.u.$$=()=>[{},{}];r.u.handleClick=(action,node)=>r.api.handleClick(action,node,r.C,r.u);
 const checks=await r.api.smoke(r.C,r.u);assert.equal(checks.length,8);assert.ok(checks.every(c=>c.ok));
 assert.deepEqual(JSON.parse(JSON.stringify(r.C.politicsUI)),before);assert.equal(r.calls.filter(c=>c.op==='politics-refresh').length,0);assert.equal(r.calls.filter(c=>c.op==='native').length,1);
});
