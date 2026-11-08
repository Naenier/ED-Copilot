'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
function load(smoke){
 const window={COPILOT_SMOKE:smoke,ReferenceUI:{navigate(){}}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'v23-smoke.js'),'utf8'),{window,setTimeout});
 return window.V23Smoke;
}

test('V23 integration probes cannot execute against an ordinary runtime or a truthy non-boolean flag',async()=>{
 for(const flag of [false,undefined,'true',1]){
  const api=load(flag);let calls=0;
  await assert.rejects(api.run({}, {handleClick(){calls++;},call(){calls++;},navigate(){calls++;}}),/только в изолированной проверке/);
  assert.equal(calls,0);
 }
});

test('V23 requires the real root handler contract before any profile mutation',async()=>{
 const api=load(true);let calls=0;
 await assert.rejects(api.run({}, {call(){calls++;},navigate(){calls++;}}),/настоящий обработчик/);
 assert.equal(calls,0);
});

test('a failing later integration probe restores inspector and the original view without swallowing the failure',async()=>{
 for(const inspector of [false,true]){
  const api=load(true),calls=[],clicks=[],C={tab:'overview',mode:'Планы',query:'initial query',selected:'initial-row',prefill:{key:'test'},lesson:'lesson',course:'course',focus:false,inspector,nav:['overview','settings'],dealsUI:{view:'saved'},settingsPane:{topic:'theme'},colonizationWorkspace:{origin:'Previous origin'},state:{}};
  const before=JSON.parse(JSON.stringify(C));
  const toolbar={dataset:{click:'inspector'}},close={dataset:{click:'inspector'}},pane={hidden:false};
  let persisted=inspector?undefined:false;
  const u={
   async navigate(tab,options){if(tab==='deals')throw new Error('Synthetic later probe failed');C.tab=tab;C.mode=options.mode||'';C.query='';C.selected='';C.prefill=options.prefill||{};},
   async handleClick(action){clicks.push(action);assert.equal(action,'inspector');C.inspector=!C.inspector;persisted=C.inspector;},
   async call(op,params){calls.push({op,params});if(op==='view')return {state:{desktop:{inspectorOpen:persisted}}};if(op==='desktop'){persisted=params.values.inspectorOpen;return {ok:true};}throw new Error('Unexpected service call: '+op);},
   $(selector){if(selector==='#page-toolbar [data-click="inspector"]')return toolbar;if(selector==='.settings-help [data-click="inspector"]')return C.inspector?close:null;if(selector==='.settings-help')return C.inspector?pane:null;return null;},
   async loadView(){},
  };
  await assert.rejects(api.run(C,u),/Synthetic later probe failed/);
  for(const key of ['tab','mode','query','selected','prefill','lesson','course','focus','inspector','nav','dealsUI','settingsPane','colonizationWorkspace'])assert.deepEqual(JSON.parse(JSON.stringify(C[key])),before[key],key);
  assert.equal(persisted,inspector);assert.deepEqual(clicks,Array(inspector?4:3).fill('inspector'));
  assert.ok(calls.every(call=>['view','desktop'].includes(call.op)));
 }
});
