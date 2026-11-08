'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
function fixture(language='en'){
 const window={},calls=[],C={state:{app:{version:'2.6.0'},settings:{language,autoUpdateEnabled:true},updates:{status:'staged',availableVersion:'2.7.0'}}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'updates.js'),'utf8'),{window});
 const esc=s=>String(s??'').replace(/</g,'&lt;').replace(/>/g,'&gt;');
 const u={esc,icon:()=>'',btn:(label,action,_i,_c,attrs='')=>`<button data-click="${action}" ${attrs}>${label}</button>`,context:()=>({tab:'settings'}),call:async(...args)=>calls.push(args),loadView:async()=>{},closeLayer:()=>{C.layer=false;},dialog:(...args)=>{calls.push(['dialog',...args]);C.layer=true;}};
 return {window,C,u,calls,api:window.UpdatesUI};
}
test('update setting is enabled by default and explains clean-exit installation in both languages',()=>{
 for(const lang of ['en','ru']){const {C,u,api}=fixture(lang);const html=api.renderSettings(C,u);assert.match(html,/id="auto-update-enabled" checked/);assert.match(html,lang==='en'?/fully exit/:/полном завершении/);assert.match(html,/2.7.0/);C.state.settings.autoUpdateEnabled=false;assert.doesNotMatch(api.renderSettings(C,u),/id="auto-update-enabled" checked/);}
});
test('disabling updates sends a typed false and reverts checkbox on failure',async()=>{
 const {C,u,calls,api}=fixture();const input={id:'auto-update-enabled',checked:false};assert.equal(await api.handleChange({target:input},C,u),true);assert.equal(calls[0][0],'settings');assert.equal(calls[0][1].values.autoUpdateEnabled,false);assert.equal(input.disabled,false);
 u.call=async()=>{throw Error('save failed');};await assert.rejects(api.handleChange({target:input},C,u),/save failed/);assert.equal(input.checked,true);
});
test('changelog appears once after verified installation, escapes release text, and waits for other dialogs',async()=>{
 const {C,u,calls,api}=fixture();assert.equal(api.showNotice(C,u),false);C.state.updates.notice={version:'2.7.0',body:'<script>alert(1)</script>'};C.layer=true;assert.equal(api.showNotice(C,u),false);C.layer=false;assert.equal(api.showNotice(C,u),true);assert.match(calls[0][2],/&lt;script&gt;/);assert.doesNotMatch(calls[0][2],/<script>/);C.layer=false;assert.equal(api.showNotice(C,u),false);await api.handleClick('updates-dismiss',{},C,u);assert.equal(calls.at(-1)[0],'updates-dismiss');
});
test('documentation and automated smoke never open an update notification',()=>{
 const {window,C,u,api}=fixture();window.COPILOT_SMOKE=true;C.state.updates.notice={version:'2.7.0'};assert.equal(api.showNotice(C,u),false);
});
