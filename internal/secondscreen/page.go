package secondscreen

const page = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ED Copilot</title><link rel="stylesheet" href="/screen.css"><script defer src="/screen.js"></script></head><body><header><h1>ED Copilot</h1><select id="mode" aria-label="Screen" hidden></select></header><main><form id="pair" hidden><label for="code">Pairing code / Код сопряжения</label><div><input id="code" inputmode="numeric" pattern="[0-9]{8}" maxlength="8" required autocomplete="off"><button type="submit">Pair / Подключить</button></div></form><p id="status" role="status"></p><div id="cards"></div></main></body></html>`

const script = `
'use strict';
const pair=document.querySelector('#pair'),code=document.querySelector('#code'),mode=document.querySelector('#mode'),status=document.querySelector('#status'),cards=document.querySelector('#cards');
const labels={en:{navigation:'Navigation',expedition:'Expedition',engineering:'Engineering',colonization:'Colonisation',offline:'Connection interrupted. Retrying…',map:'Expedition map'},ru:{navigation:'Навигация',expedition:'Экспедиция',engineering:'Инженерия',colonization:'Колонизация',offline:'Связь прервана. Подключаемся снова…',map:'Карта экспедиции'}};
let current='',signature='',pending=false,locale='en';
function put(parent,tag,text){const node=document.createElement(tag);node.textContent=String(text||'');parent.append(node);return node;}
function map(parent,points){
 if(!Array.isArray(points)||points.length<2)return;
 const xs=points.map(p=>p[0]),zs=points.map(p=>p[1]);
 const left=Math.min(...xs),right=Math.max(...xs),bottom=Math.min(...zs),top=Math.max(...zs);
 const scale=Math.min(368/Math.max(1,right-left),176/Math.max(1,top-bottom));
 const project=([x,z])=>[200+(x-(left+right)/2)*scale,104-(z-(bottom+top)/2)*scale];
 const svg=document.createElementNS('http://www.w3.org/2000/svg','svg');
 svg.setAttribute('viewBox','0 0 400 208');svg.setAttribute('role','img');svg.setAttribute('aria-label',labels[locale].map);
 const line=document.createElementNS(svg.namespaceURI,'polyline');
 line.setAttribute('points',points.map(p=>project(p).join(',')).join(' '));svg.append(line);
 const endpoint=document.createElementNS(svg.namespaceURI,'circle'),[x,y]=project(points.at(-1));
 endpoint.setAttribute('cx',x);endpoint.setAttribute('cy',y);endpoint.setAttribute('r',5);svg.append(endpoint);
 parent.append(svg);
}
async function update(){
 if(pending)return;
 pending=true;
 try{
  const response=await fetch('/api/state?mode='+encodeURIComponent(current),{cache:'no-store'});
  if(response.status===401){pair.hidden=false;mode.hidden=true;cards.replaceChildren();signature='';status.textContent='';return;}
  if(response.status===429)return;
  if(!response.ok)throw new Error('connection');
  const data=await response.json();current=data.mode;locale=data.language==='ru'?'ru':'en';
  document.documentElement.lang=locale;status.textContent='';pair.hidden=true;mode.hidden=false;
  const next=JSON.stringify(data);
  if(next===signature)return;
  signature=next;
  mode.replaceChildren();
  for(const key of data.modes||[]){const option=put(mode,'option',labels[locale][key]||key);option.value=key;option.selected=key===current;}
  cards.replaceChildren();
  for(const item of data.cards||[]){
   const card=put(cards,'section','');put(card,'h2',item.title);const rows=put(card,'dl','');
   for(const itemRow of item.rows||[]){const row=put(rows,'div','');put(row,'dt',itemRow.label);put(row,'dd',itemRow.value);}
   map(card,item.map);
  }
 }catch{status.textContent=labels[locale].offline;cards.replaceChildren();signature='';}
 finally{pending=false;}
}
pair.addEventListener('submit',async event=>{
 event.preventDefault();
 const button=pair.querySelector('button');button.disabled=true;
 try{
  const result=await fetch('/api/pair',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({code:code.value.trim()})});
  if(!result.ok){status.textContent='Check the code in ED Copilot / Проверьте код в ED Copilot';return;}
  code.value='';await update();
 }catch{status.textContent=labels[locale].offline;}finally{button.disabled=false;}
});
mode.addEventListener('change',()=>{current=mode.value;void update();});
void update();
setInterval(update,3000);
`

const style = `
:root{color-scheme:dark;font:16px/1.45 "Segoe UI",system-ui,sans-serif;background:#16191d;color:#edf1f4}
*{box-sizing:border-box}body{margin:0}header,main{width:min(100%,820px);margin:auto;padding:16px}header{display:flex;align-items:center;justify-content:space-between;gap:12px}
h1{font-size:23px;margin:0}h2{font-size:17px;margin:0 0 10px}section,form{background:#20252b;border:1px solid #343a42;border-radius:12px;padding:16px;margin-bottom:12px}
input,button,select{font:inherit;min-height:44px;padding:8px 12px;color:inherit;background:#293039;border:1px solid #4c5662;border-radius:8px}
form div{display:flex;flex-wrap:wrap;gap:8px;margin-top:10px}input{min-width:110px;width:min(55vw,230px)}
dl{margin:0}dl div{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:12px;padding:7px 0;border-top:1px solid #343a42}
dt{color:#bbc5cd}dd{margin:0;text-align:right;overflow-wrap:anywhere}button{cursor:pointer}button:focus-visible,input:focus-visible,select:focus-visible{outline:2px solid #8ccaff;outline-offset:3px}
svg{display:block;width:100%;background:#16191d;border-radius:8px;margin-top:10px}polyline{fill:none;stroke:#8ccaff;stroke-width:2}circle{fill:#8ccaff}
`
