'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function deferred(){let resolve,reject;const promise=new Promise((yes,no)=>{resolve=yes;reject=no;});return {promise,resolve,reject};}
function fixture(){
 const listeners=new Map(),calls=[],nodes=[],timers=new Map();let nextTimer=0,time=0,rpc=request=>({query:request.query,fieldId:request.fieldId,sequence:request.sequence,status:'ready',suggestions:[{name:request.query+' Canonical',match:'prefix'}]});
 class Node{
  constructor(tag='input',props={}){this.tagName=tag.toUpperCase();this.dataset={};this.attrs={};this.style={};this.value='';this.selectionStart=0;this.selectionEnd=0;this.isConnected=true;this.events=[];Object.assign(this,props);nodes.push(this);}
  setAttribute(key,value){this.attrs[key]=String(value);}getAttribute(key){return this.attrs[key]??null;}removeAttribute(key){delete this.attrs[key];}
  getBoundingClientRect(){return {left:80,top:70,bottom:110,width:250};}
  focus(){document.activeElement=this;}setSelectionRange(start,end){this.selectionStart=start;this.selectionEnd=end;}
  dispatchEvent(event){this.events.push(event.type);event.target=this;for(const handler of listeners.get(event.type)||[])handler(event);return true;}
  closest(selector){if(selector==='form')return this.form||null;if(selector==='[data-system-option]')return this.dataset.systemOption===undefined?null:this;if(selector==='[data-system-retry]')return this.retry?this:null;return null;}
  contains(node){return node===this||node?.owner===this;}querySelector(){return null;}
 }
 const document={activeElement:null,body:{append(node){node.isConnected=true;}},createElement:tag=>new Node(tag),addEventListener(name,fn){if(!listeners.has(name))listeners.set(name,[]);listeners.get(name).push(fn);}};
 class Event{constructor(type,options){this.type=type;Object.assign(this,options);}}
 const window={innerWidth:1000,innerHeight:800,Event,addEventListener(){}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'system-search.js'),'utf8'),{window,WeakMap,Set,Map,Number,String,Object,Math,Promise});
 const C={tab:'route',language:'ru',state:{}},ui={esc,call:async(op,params)=>{calls.push({op,params:{...params}});return op==='system-search'?rpc(params):{ok:true};}};
 const controller=window.SystemSearchUI.createController({document,window,Event,setTimeout(fn,ms){const id=++nextTimer;timers.set(id,{fn,at:time+ms});return id;},clearTimeout(id){timers.delete(id);}});
 controller.update(C,ui);controller.bind();
 async function tick(ms=300){time+=ms;for(const [id,timer]of [...timers])if(timer.at<=time){timers.delete(id);timer.fn();}for(let i=0;i<5;i++)await Promise.resolve();}
 function input(value,props={}){const node=new Node('input',{name:'system',value,selectionStart:value.length,selectionEnd:value.length,...props});document.activeElement=node;return node;}
 function key(node,value){const e={target:node,key:value,preventDefault(){this.prevented=true;},stopImmediatePropagation(){this.stopped=true;}};controller.keydown(e);return e;}
 return {api:window.SystemSearchUI,controller,C,ui,window,document,listeners,calls,nodes,timers,Node,input,key,tick,setRPC(fn){rpc=fn;}};
}
test('all system forms and inline controls are classified while station, faction and free catalogue filters are excluded',()=>{
 const f=fixture();
 for(const name of ['system','sourceSystem','destinationSystem','originSystem','targetSystem'])assert.equal(f.api.fieldMode(f.input('',{name})),'single',name);
 for(const name of ['source','destination']){assert.equal(f.api.fieldMode(f.input('',{name,form:{dataset:{id:'trade.plan'}}})),'single');assert.equal(f.api.fieldMode(f.input('',{name,form:{dataset:{id:'unrelated'}}})),'');}
 for(const id of ['colony-workspace-origin','reference-market-system'])assert.equal(f.api.fieldMode(f.input('',{name:'',id})),'single');
 for(const dataset of [{dealsFilter:'system'},{tradeFilter:'system'},{tradeField:'sourceSystem'},{tradeField:'destinationSystem'}])assert.equal(f.api.fieldMode(f.input('',{name:'',dataset})),'single');
 for(const name of ['waypoints','refuelSystems'])assert.equal(f.api.fieldMode(new f.Node('textarea',{name})),'lines');
 for(const name of ['station','faction','body','query','resourceKey','notes'])assert.equal(f.api.fieldMode(f.input('',{name})),name==='system'?'single':'');
 for(const id of ['reference-query','deals-query','command-query'])assert.equal(f.api.fieldMode(f.input('',{name:'',id})),'');
 assert.equal(f.api.fieldMode(f.input('',{dataset:{systemSearch:'off'}})),'');assert.equal(f.api.fieldMode(f.input('',{name:'futureField',dataset:{systemSearch:'single'}})),'single');
 assert.equal(f.api.fieldMode(f.input('',{disabled:true})),'');assert.equal(f.api.fieldMode(f.input('',{readOnly:true})),'');
});
test('debounce and minimum length prevent eager requests and only final text is queried',async()=>{
 const f=fixture(),node=f.input('So');f.controller.focus({target:node});await f.tick();assert.equal(f.calls.length,0);assert.equal(f.controller.inspect().status,'too-short');
 node.value='Sol';f.controller.input({target:node});await f.tick(100);node.value='Sola';f.controller.input({target:node});await f.tick(299);assert.equal(f.calls.length,0);await f.tick(1);
 assert.equal(f.calls.filter(c=>c.op==='system-search').length,1);assert.equal(f.calls[0].params.query,'Sola');assert.equal(f.calls[0].params.limit,20);assert.equal(f.controller.inspect().status,'ready');
});
test('stale response, removed control and cancelled fields cannot reopen or overwrite current suggestions',async()=>{
 const f=fixture(),old=deferred();let oldRequest;
 f.setRPC(r=>{if(r.query==='Old'){oldRequest=r;return old.promise;}return {...r,status:'ready',suggestions:[{name:'New canonical',match:'prefix'}]};});
 const node=f.input('Old');f.controller.focus({target:node});await f.tick();node.value='New';f.controller.input({target:node});await f.tick();
 assert.ok(f.calls.some(c=>c.op==='system-search-cancel'&&c.params.sequence===oldRequest.sequence));
 old.resolve({...oldRequest,status:'ready',suggestions:[{name:'Old stale'}]});await f.tick(0);assert.equal(f.controller.inspect().suggestions[0].name,'New canonical');
 const later=deferred();f.setRPC(()=>later.promise);node.value='Removed';f.controller.input({target:node});await f.tick();const r=f.calls.filter(c=>c.op==='system-search').at(-1).params;node.isConnected=false;
 later.resolve({...r,status:'ready',suggestions:[{name:'Detached result'}]});await f.tick(0);assert.equal(f.controller.inspect().popup.hidden,true);
});
test('keyboard combobox selection is explicit, does not submit the form, and preserves canonical entity text',async()=>{
 const f=fixture();f.setRPC(r=>({...r,status:'ready',suggestions:[{name:'Sagittarius A*',match:'correction'},{name:'Second System'}]}));
 const node=f.input('Sagittarious A*');node.attrs['aria-describedby']='existing-help';f.controller.focus({target:node});await f.tick();
 assert.equal(node.value,'Sagittarious A*');assert.equal(node.attrs.role,'combobox');assert.equal(node.attrs['aria-expanded'],'true');assert.match(node.attrs['aria-describedby'],/existing-help/);
 const down=f.key(node,'ArrowDown');assert.equal(down.prevented,true);assert.match(node.attrs['aria-activedescendant'],/-option-0$/);assert.match(f.controller.inspect().popup.innerHTML,/Возможно, вы имели в виду/);
 const enter=f.key(node,'Enter');assert.equal(enter.prevented,true);assert.equal(enter.stopped,true);assert.equal(node.value,'Sagittarius A*');assert.deepEqual(node.events,['input','change']);assert.equal(node.attrs['aria-expanded'],'false');assert.equal(f.document.activeElement,node);
});
test('Escape closes only the suggestion popup and Tab never silently accepts a name',async()=>{
 const f=fixture(),node=f.input('Sol');f.controller.focus({target:node});await f.tick();f.key(node,'ArrowDown');
 const escape=f.key(node,'Escape');assert.equal(escape.prevented,true);assert.equal(escape.stopped,true);assert.equal(node.value,'Sol');
 f.controller.focus({target:node});await f.tick();f.key(node,'ArrowDown');const tab=f.key(node,'Tab');assert.equal(tab.prevented,undefined);assert.equal(node.value,'Sol');assert.equal(f.controller.inspect().popup.hidden,true);
});
test('mouse selection keeps input focus and escapes provider names instead of creating markup',async()=>{
 const f=fixture();f.setRPC(r=>({...r,status:'ready',suggestions:[{name:'<img src=x onerror=bad()>',match:'correction'}]}));const node=f.input('Syn');f.controller.focus({target:node});await f.tick();
 const popup=f.controller.inspect().popup;assert.match(popup.innerHTML,/&lt;img/);assert.doesNotMatch(popup.innerHTML,/<img/);
 const option=new f.Node('div',{dataset:{systemOption:'0'},owner:popup});const e={target:option,preventDefault(){this.prevented=true;},stopImmediatePropagation(){this.stopped=true;}};
 assert.equal(f.controller.pointerdown(e),true);assert.equal(e.prevented,true);assert.equal(node.value,'<img src=x onerror=bad()>');assert.equal(f.document.activeElement,node);
});
test('multiline route fields query and replace only the active line while preserving CRLF and adjacent systems',async()=>{
 const f=fixture(),value='Alpha\r\nSagittarious A*\r\nGamma';const caret=value.indexOf('Sagitt')+5;
 const part=f.api.lineContext(value,caret);assert.equal(part.query,'Sagittarious A*');const replacement=f.api.replaceCurrentLine(value,caret,'Sagittarius A*');assert.equal(replacement.value,'Alpha\r\nSagittarius A*\r\nGamma');
 f.setRPC(r=>({...r,status:'ready',suggestions:[{name:'Sagittarius A*',match:'correction'}]}));const node=f.input(value,{tagName:'TEXTAREA',name:'waypoints',selectionStart:caret});f.controller.focus({target:node});await f.tick();
 assert.equal(f.calls[0].params.query,'Sagittarious A*');f.key(node,'ArrowDown');f.key(node,'Enter');assert.equal(node.value,replacement.value);assert.equal(node.selectionStart,'Alpha\r\nSagittarius A*'.length);
 for(const text of ['','Alpha\n','\nBeta']){const r=f.api.replaceCurrentLine(text,text.length,'Sol');assert.equal(r.value,text.includes('\n')?text.slice(0,text.lastIndexOf('\n')+1)+'Sol':'Sol');}
});
test('IME composition never sends partial words and response identity is enforced',async()=>{
 const f=fixture(),node=f.input('Syn');for(const handler of f.listeners.get('compositionstart'))handler({target:node});f.controller.input({target:node,isComposing:true});await f.tick();assert.equal(f.calls.length,0);
 f.setRPC(r=>({...r,fieldId:'wrong-field',status:'ready',suggestions:[{name:'Wrong'}]}));for(const handler of f.listeners.get('compositionend'))handler({target:node});await f.tick();assert.equal(f.calls.length,1);assert.equal(f.controller.inspect().suggestions.length,0);
});
test('errors, no results, broad responses and English UI retain manual entry and entity names',async()=>{
 const f=fixture(),node=f.input('Synthetic');
 f.setRPC(r=>({...r,status:'error',errorCode:'HTTP_503',suggestions:[]}));f.controller.focus({target:node});await f.tick();assert.equal(f.controller.inspect().status,'error');assert.equal(node.value,'Synthetic');assert.match(f.controller.inspect().popup.innerHTML,/повторить поиск/);assert.equal(f.key(node,'Enter').prevented,undefined);
 f.window.CopilotI18n={language:'en',t:value=>value};f.setRPC(r=>({...r,status:'empty',suggestions:[]}));f.controller.schedule(node,true);await f.tick(0);assert.match(f.controller.inspect().popup.innerHTML,/No matching system/);
 f.setRPC(r=>({...r,status:'ready',limited:true,suggestions:Array.from({length:50},(_,i)=>({name:'Система '+i}))}));f.controller.schedule(node,true);await f.tick(0);assert.equal(f.controller.inspect().suggestions.length,20);assert.match(f.controller.inspect().popup.innerHTML,/API result list is limited/);assert.match(f.controller.inspect().popup.innerHTML,/Система 0/);
 f.setRPC(r=>({...r,status:'rate-limited',retryAfterMs:120000,suggestions:[]}));f.controller.schedule(node,true);await f.tick(0);assert.match(f.controller.inspect().popup.innerHTML,/120s/);assert.equal(f.key(node,'ArrowDown').prevented,true);
});
test('repeated init binds once, popup follows lifecycle, and unrelated fields create no network work',async()=>{
 const f=fixture();f.controller.bind();assert.equal(f.listeners.get('input').length,1);const node=f.input('Station',{name:'station'});assert.equal(f.controller.focus({target:node}),false);await f.tick();assert.equal(f.calls.length,0);
 const system=f.input('Sol');f.controller.focus({target:system});await f.tick();f.controller.close();assert.equal(f.controller.inspect().popup.hidden,true);
});
test('a reloaded frontend uses a new request scope rather than inheriting backend cancellation high-water marks',async()=>{
 const first=fixture(),second=fixture();
 first.controller.focus({target:first.input('Sol')});second.controller.focus({target:second.input('Sol')});
 await first.tick();await second.tick();
 assert.notEqual(first.calls[0].params.fieldId,second.calls[0].params.fieldId);
 assert.ok(first.calls[0].params.fieldId.length<128);assert.equal(first.calls[0].params.sequence,1);assert.equal(second.calls[0].params.sequence,1);
});
