'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function fixture(){
 const window={},handlers=new Map(),buttons=[],body={innerHTML:''},groups=Array.from({length:4},()=>({}));
 const host={addEventListener(type,callback){const events=handlers.get(type)||[];events.push(callback);handlers.set(type,events);}};
 const C={tab:'settings',inspector:true,state:{desktop:{displayMode:'windowed'},settings:{},connection:{}},actions:[{id:'settings.prices'},{id:'settings.import.palette',kind:'theme-import'},{id:'settings.journals',kind:'folder'},{id:'settings.filters'}]};
 const calls=[];
 const ui={esc,icon:name=>'<svg data-icon="'+esc(name)+'"></svg>',fmt:value=>String(value),
  btn:(label,action,icon,kind,attrs)=>'<button data-click="'+action+'" '+attrs+'>'+esc(label)+'</button>',
  $:selector=>selector==='[data-settings-workspace]'?host:selector==='[data-settings-help-content]'?body:null,
  $$:selector=>selector==='[data-settings-topic]'?buttons:selector==='.settings-group'?groups:[],
  call:(...args)=>{calls.push(args);throw Error('Help must never write settings or call a service');},
 };
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'settings-pane.js'),'utf8'),{window});
 function control(dataset={},id='',group=null){
  const item={dataset,id,attributes:{},setAttribute(key,value){this.attributes[key]=value;},closest(selector){if(selector==='button,input,select,textarea')return this;if(selector==='[data-settings-topic]')return this.dataset.settingsTopic?this:null;if(selector==='.settings-group')return group;return null;}};
  return item;
 }
 for(const topic of ['workspace','overlay','theme','journal','mining','prices'])buttons.push(control({settingsTopic:topic}));
 return {api:window.SettingsPaneUI,C,ui,calls,host,handlers,body,buttons,groups,control,
  fire(type,target){for(const handler of handlers.get(type)||[])handler({target});},
 };
}

test('settings visibility creates and removes a real help panel without changing saved visibility',()=>{
 const f=fixture(),main='<section id="settings-controls">Existing controls</section>';
 const before=JSON.stringify(f.C.state);
 let html=f.api.render(f.C,f.ui,main);
 assert.ok(html.includes(main));
 assert.match(html,/<aside class="inspector settings-help"/);
 assert.match(html,/data-click="inspector"[^>]+aria-label="Скрыть подробности"/);
 assert.equal(f.C.inspector,true);
 f.C.inspector=false;html=f.api.render(f.C,f.ui,main);
 assert.ok(html.includes(main));assert.doesNotMatch(html,/<aside|settings-help-topics/);
 assert.equal(f.C.inspector,false);assert.equal(JSON.stringify(f.C.state),before);
 assert.equal(f.calls.length,0);
});

test('the toolbar capability contract excludes every custom renderer without a details panel',()=>{
 const {api}=fixture();
 for(const kind of ['table','settings'])assert.equal(api.canToggle(kind),true,kind);
 for(const kind of ['overview','academy','learning','reference','calculator','carrier-trade','deals','',null,undefined])assert.equal(api.canToggle(kind),false,String(kind));
});

test('non-settings renders and hidden init leave unrelated pages and controller state alone',()=>{
 const f=fixture();f.C.tab='route';
 assert.equal(f.api.render(f.C,f.ui,'<table>Route</table>'),'<table>Route</table>');
 f.api.init(f.C,f.ui);assert.equal(f.handlers.size,0);assert.equal(f.C.settingsPane,undefined);
 f.C.tab='settings';f.C.inspector=false;f.api.init(f.C,f.ui);assert.equal(f.handlers.size,0);
});

test('topic buttons explain the active setting and focus follows the real control without RPC or losing input',()=>{
 const f=fixture();f.api.render(f.C,f.ui,'');f.api.init(f.C,f.ui);f.api.init(f.C,f.ui);
 assert.equal(f.handlers.get('click').length,1);assert.equal(f.handlers.get('focusin').length,1);
 const input=f.control({actionId:'settings.prices'});input.value='12500';
 f.fire('focusin',input);
 assert.equal(f.C.settingsPane.topic,'prices');assert.equal(input.value,'12500');
 assert.match(f.body.innerHTML,/для оценки груза/);assert.match(f.body.innerHTML,/Пустое поле сбрасывает/);assert.match(f.body.innerHTML,/Ноль задаёт нулевую оценку/);
 assert.equal(f.buttons.find(button=>button.dataset.settingsTopic==='prices').attributes['aria-pressed'],'true');
 f.fire('click',f.buttons.find(button=>button.dataset.settingsTopic==='overlay'));
 assert.equal(f.C.settingsPane.topic,'overlay');assert.match(f.body.innerHTML,/только при активном окне Elite Dangerous/);
 f.fire('focusin',f.control({},'main-game-palette'));
 assert.equal(f.C.settingsPane.topic,'theme');assert.match(f.body.innerHTML,/предпросмотр/);
 f.fire('focusin',f.control({actionId:'settings.filters'}));
 assert.equal(f.C.settingsPane.topic,'mining');assert.match(f.body.innerHTML,/лазерной добыче/);
 assert.equal(f.C.inspector,true);assert.equal(f.calls.length,0);
});

test('context follows blocks, keyboard focus and existing folder actions rather than assuming action order',()=>{
 const f=fixture();f.api.render(f.C,f.ui,'');f.api.init(f.C,f.ui);
 for(const [index,topic] of ['workspace','overlay','theme','journal'].entries()){
  f.fire('focusin',f.control({},'',f.groups[index]));assert.equal(f.C.settingsPane.topic,topic);
 }
 f.fire('focusin',f.control({actionId:'settings.journals'}));assert.equal(f.C.settingsPane.topic,'journal');
 assert.match(f.body.innerHTML,/Папка не выбрана/);
 assert.match(f.body.innerHTML,/Нет данных/);
 f.fire('focusin',f.control({click:'customize'}));assert.equal(f.C.settingsPane.topic,'workspace');
 const previous=f.C.settingsPane.topic;f.fire('click',f.control({settingsTopic:'untrusted-topic'}));assert.equal(f.C.settingsPane.topic,previous);
});

test('zero event counts and palette state are meaningful while supplied text and colors stay escaped',()=>{
 const f=fixture();
 f.C.state.connection={eventCount:0,journalDir:'<img src=x onerror=alert(1)>',error:'<script>bad()</script>'};
 f.C.settingsPane={topic:'journal'};
 let html=f.api.render(f.C,f.ui,'');
 assert.match(html,/<dt>Прочитано событий<\/dt><dd>0<\/dd>/);
 assert.match(html,/&lt;img src=x onerror=alert\(1\)&gt;/);
 assert.match(html,/&lt;script&gt;bad\(\)&lt;\/script&gt;/);
 assert.doesNotMatch(html,/<img|<script/);
 f.C.settingsPane.topic='theme';
 f.C.state.theme={settings:{preset:'blue'},presets:[{id:'blue',name:'<Палитра>'}],palette:{accent:'#A1B2C3',text:'red;position:fixed',panel:'" onmouseover="bad()'}};
 html=f.api.render(f.C,f.ui,'');
 assert.match(html,/&lt;Палитра&gt;/);assert.match(html,/background:#A1B2C3/);
 assert.doesNotMatch(html,/position:fixed|onmouseover/);
 assert.equal(f.calls.length,0);
});

test('help only exposes normal user workflows and every icon is available in the offline registry',()=>{
 const f=fixture(),registry={};vm.runInNewContext(fs.readFileSync(path.join(__dirname,'icons.js'),'utf8'),{window:registry});
 let all='';for(const topic of ['workspace','overlay','theme','journal','mining','prices']){f.C.settingsPane={topic};all+=f.api.render(f.C,f.ui,'');}
 for(const match of all.matchAll(/data-icon="([^"]+)"/g))assert.ok(registry.CopilotIcons[match[1]],match[1]);
 assert.doesNotMatch(all,/client.?id|OAuth|Developer Zone|EDMC|data-click="action"|https?:\/\//i);
});

test('a visible details pane remains reachable at the narrow layout breakpoint',()=>{
 const css=fs.readFileSync(path.join(__dirname,'settings-pane.css'),'utf8');
 assert.match(css,/@media\(max-width:1020px\)[\s\S]+\.settings-help\.inspector,\.data-split>\.inspector\{display:block;position:fixed/);
 assert.match(css,/width:min\(330px,calc\(100vw - 36px\)\)/);
 assert.match(css,/\.settings-workspace\.has-help \.settings-grid\{grid-template-columns:1fr\}/);
});
