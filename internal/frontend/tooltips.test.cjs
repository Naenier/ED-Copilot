'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
function fixture(){
 const events=new Map(),windowEvents=new Map(),timers=new Map(),observers=[];let sequence=0,now=0;
 class Element{
  constructor(tag='div'){this.tagName=tag.toUpperCase();this.attrs=new Map();this.children=[];this.parentElement=null;this.style={};this.hidden=false;this.textContent='';this.rect={left:8,right:44,top:40,bottom:76,width:36,height:36};}
  set innerHTML(value){throw Error('Tooltips must use plain text');}
  setAttribute(key,value){this.attrs.set(key,String(value));}
  getAttribute(key){return this.attrs.has(key)?this.attrs.get(key):null;}
  hasAttribute(key){return this.attrs.has(key);}
  removeAttribute(key){this.attrs.delete(key);}
  append(node){node.parentElement=this;this.children.push(node);}
  remove(){if(this.parentElement)this.parentElement.children=this.parentElement.children.filter(x=>x!==this);this.parentElement=null;}
  get isConnected(){return this.root===true||!!this.parentElement?.isConnected;}
  contains(node){return node===this||this.children.some(child=>child.contains(node));}
  matches(selector){return selector.split(',').some(s=>{s=s.trim();if(s===':focus-visible')return this.focusVisible===true;if(s==='.rail')return this.rail===true;if(s==='[hidden]')return this.hidden;if(s==='[aria-hidden="true"]')return this.getAttribute('aria-hidden')==='true';if(s==='[role="button"]')return this.getAttribute('role')==='button';if(s.startsWith('['))return this.hasAttribute(s.slice(1,-1));return this.tagName.toLowerCase()===s;});}
  closest(selector){for(let node=this;node;node=node.parentElement)if(node.matches(selector))return node;return null;}
  getClientRects(){return this.isConnected&&!this.hidden?[this.rect]:[];}
  getBoundingClientRect(){return this.id==='copilot-tooltip'?{width:Math.min(320,this.textContent.length*7+22),height:32}:this.rect;}
 }
 const body=new Element();body.root=true;
 const add=(map,event,fn)=>{const list=map.get(event)||[];list.push(fn);map.set(event,list);};
 const document={body,hidden:false,createElement:tag=>new Element(tag),addEventListener:(event,fn)=>add(events,event,fn)};
 const window={innerWidth:800,innerHeight:600,addEventListener:(event,fn)=>add(windowEvents,event,fn),setTimeout(fn,delay){const id=++sequence;timers.set(id,{fn,at:now+delay});return id;},clearTimeout:id=>timers.delete(id),MutationObserver:class{constructor(fn){this.fn=fn;this.active=false;observers.push(this);}observe(){this.active=true;}disconnect(){this.active=false;}}};
 vm.runInNewContext(fs.readFileSync(__dirname+'/tooltips.js','utf8'),{window,document});const api=window.TooltipUI;api.init();
 const emit=(event,data={})=>{for(const fn of events.get(event)||[])fn({pointerType:'mouse',preventDefault(){},stopImmediatePropagation(){},...data});};
 function advance(ms){const until=now+ms;while(true){const due=[...timers].filter(([,x])=>x.at<=until).sort((a,b)=>a[1].at-b[1].at)[0];if(!due)break;now=due[1].at;timers.delete(due[0]);due[1].fn();}now=until;}
 function button(text='Торговля',rail=true){const holder=new Element('nav');holder.rail=rail;body.append(holder);const node=new Element('button');node.setAttribute('title',text);holder.append(node);return node;}
 return {api,window,document,events,observers,Element,button,emit,advance,tip:()=>body.children.find(x=>x.id==='copilot-tooltip'),mutate:()=>observers.filter(x=>x.active).forEach(x=>x.fn()),windowEvent:event=>(windowEvents.get(event)||[]).forEach(fn=>fn({}))};
}
test('native title is suppressed immediately and disabled icon controls keep a name and plain tooltip text',()=>{
 const f=fixture(),node=f.button('<img src=x onerror=alert(1)>');node.setAttribute('disabled','');f.emit('pointerover',{target:node});
 assert.equal(node.hasAttribute('title'),false);assert.equal(node.getAttribute('aria-label'),'<img src=x onerror=alert(1)>');assert.equal(f.tip(),undefined);
 f.advance(350);assert.equal(f.tip().textContent,'<img src=x onerror=alert(1)>');assert.equal(f.tip().getAttribute('role'),'tooltip');assert.equal(f.tip().parentElement,f.document.body);
});
test('keyboard tooltips preserve labels and existing descriptions, and Escape removes only their own token',()=>{
 const f=fixture(),node=f.button('Командир');node.setAttribute('aria-label','Existing name');node.setAttribute('aria-describedby','existing-help');node.focusVisible=true;
 f.emit('keydown',{key:'Tab'});f.emit('focusin',{target:node});f.advance(200);
 assert.equal(node.getAttribute('aria-label'),'Existing name');assert.equal(node.getAttribute('aria-describedby'),'existing-help copilot-tooltip');
 f.emit('keydown',{key:'Escape'});assert.equal(f.tip().hidden,true);assert.equal(node.getAttribute('aria-describedby'),'existing-help');
});
test('moving inside an SVG does not restart the hint, and the popup itself is hoverable across its gap',()=>{
 const f=fixture(),node=f.button(),svg=new f.Element('svg');node.append(svg);f.emit('pointerover',{target:node});f.advance(200);f.emit('pointerover',{target:svg,relatedTarget:node});f.advance(150);assert.equal(f.tip().hidden,false);
 f.emit('pointerout',{target:svg,relatedTarget:f.document.body});f.advance(50);f.emit('pointerover',{target:f.tip(),relatedTarget:f.document.body});f.advance(200);assert.equal(f.tip().hidden,false);
 f.emit('pointerout',{target:f.tip(),relatedTarget:f.document.body});f.advance(120);assert.equal(f.tip().hidden,true);
});
test('Escape dismisses a visible hint before reaching the surrounding dialog handler',()=>{
 const f=fixture(),node=f.button();let prevented=0,stopped=0;f.emit('pointerover',{target:node});f.advance(350);
 f.emit('keydown',{key:'Escape',preventDefault(){prevented++;},stopImmediatePropagation(){stopped++;}});assert.equal(prevented,1);assert.equal(stopped,1);assert.equal(f.tip().hidden,true);
 f.emit('keydown',{key:'Escape',preventDefault(){prevented++;},stopImmediatePropagation(){stopped++;}});assert.equal(prevented,1);assert.equal(stopped,1);
});
test('pointer clicks do not create a keyboard hint and a quick pass over a control never opens one',()=>{
 const f=fixture(),node=f.button();f.emit('pointerover',{target:node});f.emit('pointerout',{target:node,relatedTarget:f.document.body});f.advance(1000);assert.equal(f.tip(),undefined);
 f.emit('pointerdown',{target:node});f.emit('focusin',{target:node});f.advance(1000);assert.equal(f.tip(),undefined);
});
test('leaving just before the opening delay never flashes a hint outside its trigger',()=>{
 const f=fixture(),node=f.button();f.emit('pointerover',{target:node});f.advance(300);f.emit('pointerout',{target:node,relatedTarget:f.document.body});f.advance(100);assert.equal(f.tip(),undefined);f.advance(500);assert.equal(f.tip(),undefined);
});
test('scroll, resize, blur, hidden documents and removed triggers cancel pending and visible hints',()=>{
 for(const action of ['scroll','resize','blur','hidden','removed'])for(const visible of [false,true]){
  const f=fixture(),node=f.button();f.emit('pointerover',{target:node});if(visible)f.advance(350);
  if(action==='scroll')f.emit('scroll',{target:f.document.body});else if(['resize','blur'].includes(action))f.windowEvent(action);else if(action==='hidden'){f.document.hidden=true;f.emit('visibilitychange');}else{node.remove();f.mutate();}
  f.advance(1000);assert.ok(!f.tip()||f.tip().hidden);assert.equal(f.observers.some(x=>x.active),false);
 }
});
test('active tooltip follows changed translated labels and stops if its trigger becomes hidden',()=>{
 const f=fixture(),node=f.button('Trade');f.emit('pointerover',{target:node});f.advance(350);node.setAttribute('title','Торговля');f.mutate();assert.equal(f.tip().textContent,'Торговля');assert.equal(node.hasAttribute('title'),false);node.hidden=true;f.mutate();assert.equal(f.tip().hidden,true);
});
test('tooltip scroll remains usable and initialization never duplicates event handlers',()=>{
 const f=fixture(),node=f.button();const counts=[...f.events].map(([event,list])=>[event,list.length]);f.api.init();assert.deepEqual([...f.events].map(([event,list])=>[event,list.length]),counts);
 f.emit('pointerover',{target:node});f.advance(350);f.emit('scroll',{target:f.tip()});assert.equal(f.tip().hidden,false);f.emit('pointerdown',{target:node});assert.equal(f.tip().hidden,true);
});
test('placement flips at the screen edges and clamps long hints inside the viewport',()=>{
 const p=fixture().api.placement;
 assert.deepEqual({...p({left:8,right:44,top:40,bottom:76},{width:90,height:32},{width:800,height:600},'right')},{left:52,top:42});
 assert.deepEqual({...p({left:760,right:792,top:40,bottom:76},{width:250,height:32},{width:800,height:600},'right')},{left:502,top:42});
 const bottom=p({left:350,right:390,top:560,bottom:592},{width:310,height:50},{width:800,height:600});assert.equal(bottom.top,502);
 const narrow=p({left:0,right:20,top:0,bottom:20},{width:600,height:32},{width:200,height:80});assert.equal(narrow.left,8);assert.ok(narrow.top>=8);
});
