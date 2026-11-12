'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const dictionary=Object.assign({},...fs.readdirSync(path.join(__dirname,'../localization')).filter(name=>name.endsWith('_en.json')).map(file=>JSON.parse(fs.readFileSync(path.join(__dirname,'../localization',file),'utf8'))));
const esc=value=>String(value??'').replace(/[&<>"']/g,key=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[key]));
const clone=value=>JSON.parse(JSON.stringify(value));
const wait=()=>new Promise(resolve=>setImmediate(resolve));
function fixture(file,global,mode,handler){
 const window={},events=new Map(),calls=[],nav=[];
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,file+'.js'),'utf8'),{window,Date});
 const api=window[global],C={tab:'engineering',mode,state:{revision:1,planWorkspace:{key:'test-owner'},settings:{language:'en'}}};
 const host={ownerDocument:{activeElement:null,getElementById:()=>null},addEventListener:(type,fn)=>events.set(type,fn),innerHTML:''};
 const u={t:key=>dictionary[key]??key,locale:()=>'en-GB',esc,fmt:String,heading:title=>'<h1>'+esc(title)+'</h1>',icon:()=>'',toast:()=>{},
  call:async(op,body)=>{calls.push({op,body:clone(body)});return handler(op,body);},navigate:async(tab,options)=>{nav.push({tab,options});C.tab=tab;},$:()=>host};
 const click=(name,attrs={})=>events.get('click')({target:{closest:()=>({dataset:{[file==='ship-builds'?'buildAction':'itineraryAction']:name,...attrs}})}});
 return {api,C,host,events,calls,nav,u,click};
}

test('ship outfitting previews before persisting and uses the selected builder for export',async()=>{
 const observed={name:'Observed <Asp>',ship:'Asp Explorer',format:'journal',stats:{maximumRange:42},slots:[{slot:'FutureSlot',item:'future',name:'Unknown fitting'}]};
 const target={id:'target-a',name:'Imported',ship:'Asp Explorer',format:'slef',stats:{maximumRange:48},slots:[]};
 const raw='[{"header":{"appName":"EDSY"},"data":{"Ship":"asp","Modules":[]}}]';
 const f=fixture('ship-builds','ShipBuildUI','Оснащение корабля',(op,body)=>{
  if(op==='ship-builds')return body.text?{preview:[target]}:{observed,targets:[]};
  if(op==='ship-builds-update')return {observed,target,targets:[target],selectedId:'target-a'};
  if(op==='ship-builds-export')return {url:'https://edsy.org/#/I=example'};
  return {};
 });
 f.api.init(f.C,f.u);await f.C.shipBuildUI.read;
 assert.match(f.api.render(f.C,f.u),/Observed &lt;Asp&gt;/);
 assert.match(f.api.render(f.C,f.u),/Unknown fitting/);
 f.click('import');f.events.get('input')({target:{id:'ship-build-json',value:raw}});f.click('preview');await wait();
 assert.deepEqual(f.calls[1],{op:'ship-builds',body:{text:raw,expectedPlanWorkspace:'test-owner'}});
 assert.match(f.host.innerHTML,/Imported · Asp Explorer · SLEF/);
 f.events.get('submit')({target:{id:'ship-build-import'},preventDefault(){}});await wait();
 assert.deepEqual(f.calls[2],{op:'ship-builds-update',body:{action:'import',text:raw,index:0,expectedPlanWorkspace:'test-owner'}});
 f.click('edsy');await wait();
 assert.deepEqual(f.calls[3],{op:'ship-builds-export',body:{id:'target-a',provider:'edsy',format:'slef',expectedPlanWorkspace:'test-owner'}});
 assert.deepEqual(f.calls[4],{op:'native',body:{action:'open',url:'https://edsy.org/#/I=example'}});
});

test('engineer itinerary displays preparations separately and sends only feasible ordered stops to the ship route',async()=>{
 const ready={id:'engineer:1',kind:'engineer',action:'upgrade',name:'Engineer A',system:'Deciat',station:'Farseer Inc.',tasks:[{module:'FSD',blueprint:'Long Range',grade:3}]};
 const trader={id:'trader:raw',kind:'trader',action:'trade',category:'raw',system:'82 Eridani',station:'Bresnik Port',
  tasks:[{fromKey:'top',fromName:'Top material',toKey:'low',toName:'Low material',requiredFrom:1,received:3}]};
 const paused={...ready,id:'engineer:2',name:'Paused',system:'Elsewhere'};
 const blocked={reason:'access-required',module:'Thrusters',requirement:{text:'Нужно приглашение',english:'Invitation required'}};
 let data={ownerKnown:true,source:'plan',stops:[trader,ready],deferred:[paused],blocked:[blocked]};
 const f=fixture('engineering-itinerary','EngineeringItineraryUI','Поездка к инженерам',(op,body)=>{
  if(op==='engineering-itinerary-update'&&body.action==='move')data={...data,stops:[ready,trader]};
  return data;
 });
 f.api.init(f.C,f.u);await f.C.engineeringItineraryUI.read;
 const html=f.api.render(f.C,f.u);
 assert.match(html,/Farseer Inc\./);assert.match(html,/Invitation required/);assert.match(html,/Bresnik Port/);
 f.click('up',{stopId:ready.id});await wait();
 assert.deepEqual(f.calls[1],{op:'engineering-itinerary-update',body:{action:'move',id:ready.id,direction:-1,visibleIds:[trader.id,ready.id],expectedPlanWorkspace:'test-owner'}});
 f.click('route');await wait();
 assert.equal(f.calls[2].op,'ship-route');
 assert.deepEqual(f.calls[2].body.waypoints.map(row=>row.system),['Deciat','82 Eridani']);
 assert.match(f.calls[2].body.waypoints[1].action,/3 × Low material/);
 assert.equal(f.nav[0].tab,'ship-route');
});
