'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const clone=value=>JSON.parse(JSON.stringify(value));
function load(flag,document={documentElement:{lang:'ru'}}){const window={COPILOT_SMOKE:flag,CopilotI18n:{language:'ru'},ReferenceUI:{navigate(){}}};vm.runInNewContext(fs.readFileSync(path.join(__dirname,'v24-smoke.js'),'utf8'),{window,document,setTimeout,Event,performance});return {api:window.V24Smoke,window,document};}
test('V24 smoke cannot execute on an ordinary profile or with a truthy non-boolean flag',async()=>{
 for(const flag of [false,undefined,1,'true']){const f=load(flag);let calls=0;await assert.rejects(f.api.run({}, {handleClick(){calls++},call(){calls++},navigate(){calls++}}),/isolated smoke profile/);assert.equal(calls,0);}
});
test('V24 requires the actual root action contract before any read or mutation',async()=>{
 const f=load(true);let calls=0;await assert.rejects(f.api.run({}, {call(){calls++},navigate(){calls++}}),/real root interface handlers/);assert.equal(calls,0);
});
test('a failure after an actual language form restores settings, selected body, menu and original view',async()=>{
 const f=load(true),calls=[],submissions=[],originalExo={settings:{radiusLy:200,minValue:100},selectedBodyKey:'synthetic:2',bodies:[{key:'synthetic:1'},{key:'synthetic:2'}],pending:[{}],sales:[{}],tracker:{sampleCount:2,anchors:[{latitude:0,longitude:0},{latitude:0,longitude:0.02}],positionKnown:false,nearestSampleDistanceM:null,remainingDistanceM:null,nearestSampleMeetsMin:null},bookmarks:[],surfaceMarks:[]};
 const live=clone(originalExo),C={tab:'overview',mode:'Планы',query:'Настройки',selected:'private-row',prefill:{system:'Настройки'},nav:['overview','settings'],focus:false,lesson:'lesson',course:'course',exoWorkspace:{query:'Draft'},reference:{query:'original'},referenceSessions:{old:{}},state:{settings:{language:'ru'}}};
 const before=clone(C);C.t=value=>f.window.CopilotI18n.language==='en'?({'Настройки':'Settings','Продажи':'Sales','Вид':'View'})[value]||value:value;
 let form=null,failed=false;
 const u={
  async navigate(tab,opts){if(!C.nav.includes(tab))C.nav.push(tab);if(tab==='exobiology'&&!failed){failed=true;live.settings.radiusLy=125;live.selectedBodyKey='synthetic:1';live.bookmarks.push({bodyKey:'synthetic:new'});live.surfaceMarks.push({id:'new',kind:'waypoint'});throw new Error('Synthetic screen failure');}C.tab=tab;C.mode=opts.mode||'';C.query='';C.selected='';C.prefill=opts.prefill||{};},
  $(selector){if(selector==='#form-error')return null;if(selector==='[data-action-id="settings.language"]')return {dataset:{click:'action',actionId:'settings.language'}};if(selector==='#action-form')return form;return null;},
  async handleClick(action,node){assert.equal(action,'action');assert.equal(node.dataset.actionId,'settings.language');C.layer=true;form={dataset:{id:'settings.language'},elements:{language:{value:'ru'}},dispatchEvent(event){assert.equal(event.type,'submit');submissions.push(this.elements.language.value);C.state.settings.language=this.elements.language.value;f.window.CopilotI18n.language=this.elements.language.value;f.document.documentElement.lang=this.elements.language.value;form=null;C.layer=false;return true;}};},
  async call(op,params){calls.push({op,params:clone(params)});if(op==='view')return {state:{settings:clone(C.state.settings),exobiology:clone(live)}};if(op==='exobiology'){const value=params.values;if(value.action==='configure')live.settings={...value};delete live.settings.action;if(value.action==='remove-bookmark')live.bookmarks=live.bookmarks.filter(b=>b.bodyKey!==value.bodyKey);if(value.action==='remove-waypoint')live.surfaceMarks=live.surfaceMarks.filter(m=>m.id!==value.id);if(value.action==='select-body')live.selectedBodyKey=value.bodyKey;return {ok:true};}if(op==='settings'){C.state.settings={...C.state.settings,...params.values};return {ok:true};}if(op==='navigation')return {ok:true};throw new Error('Unexpected RPC: '+op);},
  closeLayer(){form=null;C.layer=false;},async loadView(){f.window.CopilotI18n.language=C.state.settings.language;f.document.documentElement.lang=C.state.settings.language;}
 };
 await assert.rejects(f.api.run(C,u),/Synthetic screen failure/);
 assert.deepEqual(submissions,['en']);assert.deepEqual(live,originalExo);assert.equal(C.state.settings.language,'ru');assert.equal(f.document.documentElement.lang,'ru');
 for(const key of ['tab','mode','query','selected','prefill','nav','focus','lesson','course','exoWorkspace','reference','referenceSessions'])assert.deepEqual(clone(C[key]),before[key],key);
 assert.ok(calls.some(c=>c.op==='exobiology'&&c.params.values.action==='remove-waypoint'));assert.ok(calls.some(c=>c.op==='exobiology'&&c.params.values.action==='remove-bookmark'));assert.ok(calls.some(c=>c.op==='navigation'));
 assert.ok(calls.every(c=>['view','settings','exobiology','navigation'].includes(c.op)),'cleanup must not contact accounts, markets or public network services');
});
