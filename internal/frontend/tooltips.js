/* A single local tooltip portal: no native popups, per-control listeners or polling. */
(()=>{'use strict';
const id='copilot-tooltip',selector='[data-tooltip],[title]';
function placement(rect,size,viewport,side='bottom'){
 const edge=8,gap=8,width=Math.min(size.width,Math.max(0,viewport.width-2*edge)),height=size.height;
 let left,top;
 if(side==='right'){
  left=rect.right+gap;top=(rect.top+rect.bottom-height)/2;
  if(left+width>viewport.width-edge)left=rect.left-width-gap;
 }else{
  left=(rect.left+rect.right-width)/2;top=rect.bottom+gap;
  if(top+height>viewport.height-edge)top=rect.top-height-gap;
 }
 return {left:Math.round(Math.max(edge,Math.min(left,viewport.width-width-edge))),top:Math.round(Math.max(edge,Math.min(top,viewport.height-height-edge)))};
}
function create(doc,win){
 let anchor=null,portal=null,opening=null,closing=null,observer=null,ownedDescription=false,keyboard=false;
 const later=(fn,ms)=>win.setTimeout(fn,ms),cancel=timer=>{if(timer!==null)win.clearTimeout(timer);};
 function inside(node,parent){return !!node&&!!parent&&(node===parent||parent.contains(node));}
 function normalise(node){
  if(!node)return '';
  if(node.hasAttribute('title')){
   const text=node.getAttribute('title')||'';
   node.setAttribute('data-tooltip',text);node.removeAttribute('title');
   if(text&&!node.hasAttribute('aria-label')&&!node.hasAttribute('aria-labelledby')&&!String(node.textContent||'').trim()&&node.matches('button,a,[role="button"]'))node.setAttribute('aria-label',text);
  }
  return String(node.getAttribute('data-tooltip')||'').trim();
 }
 function target(node){return node?.closest?.(selector)||null;}
 function live(node){return node?.isConnected&&!doc.hidden&&!node.closest('[hidden],[aria-hidden="true"]')&&node.getClientRects().length>0;}
 function clearDescription(){
  if(anchor&&ownedDescription){const tokens=(anchor.getAttribute('aria-describedby')||'').split(/\s+/).filter(token=>token&&token!==id);if(tokens.length)anchor.setAttribute('aria-describedby',tokens.join(' '));else anchor.removeAttribute('aria-describedby');}
  ownedDescription=false;
 }
 function hide(){
  cancel(opening);cancel(closing);opening=closing=null;clearDescription();
  if(portal){portal.hidden=true;portal.textContent='';}
  observer?.disconnect();observer=null;anchor=null;
 }
 function position(){
  const point=placement(anchor.getBoundingClientRect(),portal.getBoundingClientRect(),{width:win.innerWidth,height:win.innerHeight},anchor.closest('.rail')?'right':'bottom');
  portal.style.left=point.left+'px';portal.style.top=point.top+'px';
 }
 function show(node){
  opening=null;if(anchor!==node||!live(node)){hide();return;}
  const text=normalise(node);if(!text){hide();return;}
  if(!portal?.isConnected){portal=doc.createElement('div');portal.id=id;portal.className='app-tooltip';portal.setAttribute('role','tooltip');doc.body.append(portal);}
  portal.textContent=text;portal.hidden=false;portal.style.visibility='hidden';position();portal.style.visibility='';
  const tokens=(node.getAttribute('aria-describedby')||'').split(/\s+/).filter(Boolean);
  if(!tokens.includes(id)){tokens.push(id);node.setAttribute('aria-describedby',tokens.join(' '));ownedDescription=true;}
 }
 function open(node,delay){
  if(!node||!normalise(node)||!live(node))return;
  cancel(closing);closing=null;
  if(anchor===node)return;
  const warm=portal&&!portal.hidden;hide();anchor=node;
  if(win.MutationObserver){observer=new win.MutationObserver(()=>{
   if(!live(node)){hide();return;}
   const text=normalise(node);if(!text){hide();return;}
   if(portal&&!portal.hidden&&portal.textContent!==text){portal.textContent=text;position();}
  });observer.observe(doc.body,{childList:true,subtree:true,attributes:true,attributeFilter:['title','data-tooltip','hidden','aria-hidden','style','class']});}
  opening=later(()=>show(node),warm?70:delay);
 }
 function postponeHide(){if(opening!==null){hide();return;}cancel(closing);closing=later(hide,120);}
 const on=(host,event,fn,options)=>host.addEventListener(event,fn,options);
 on(doc,'pointerover',event=>{
  if(event.pointerType==='touch')return;
  if(inside(event.target,portal)){cancel(closing);closing=null;return;}
  const node=target(event.target);if(!node||inside(event.relatedTarget,node))return;open(node,350);
 },true);
 on(doc,'pointerout',event=>{
  if(!anchor)return;
  if(inside(event.target,anchor)&&!inside(event.relatedTarget,anchor)&&!inside(event.relatedTarget,portal))postponeHide();
  else if(inside(event.target,portal)&&!inside(event.relatedTarget,portal)&&!inside(event.relatedTarget,anchor))postponeHide();
 },true);
 on(doc,'focusin',event=>{const node=target(event.target);if(node&&(keyboard||node.matches(':focus-visible')))open(node,200);},true);
 on(doc,'focusout',event=>{if(inside(event.target,anchor))hide();},true);
 on(doc,'pointerdown',()=>{keyboard=false;hide();},true);
 on(doc,'keydown',event=>{const visible=portal&&!portal.hidden;keyboard=true;hide();if(visible&&event.key==='Escape'){event.preventDefault();event.stopImmediatePropagation();}},true);
 on(doc,'scroll',event=>{if(!inside(event.target,portal))hide();},true);on(doc,'visibilitychange',hide);on(win,'resize',hide);on(win,'blur',hide);
 return Object.freeze({hide});
}
let instance;
window.TooltipUI=Object.freeze({placement,create,init(){return instance||(instance=create(document,window));},hide(){instance?.hide();}});
})();
