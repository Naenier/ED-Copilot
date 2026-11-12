const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const context = {window: {}, FormData: class {
  constructor(form) { this.values = form.values; }
  get(name) { return this.values[name] ?? null; }
  has(name) { return name in this.values; }
}};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'ship-route.js'), 'utf8'), context);
const ui = context.window.ShipRouteUI;
const translations = require('../localization/ship_route_radar_en.json');
const util = {
  esc: value => String(value).replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;'),
  t: value => translations[value] || {' т': ' t'}[value] || value,
  fmt: value => String(value),
  heading: value => '<h1>' + value + '</h1>'
};
const view = owner => ({tab:'ship-route',state:{planWorkspace:{key:owner},shipRoute:{
  ownerKnown:true, currentSystem:'Sol', suggestedJumpRangeLy:55, fuel:{mainTons:22},
  saved:[{id:'1',name:'Journey',active:true}],
  route:{id:'1',name:'Journey',mode:'exact',next:1,nextStop:{system:'Lave'},stops:[
    {system:'Sol',visited:true,starKind:'scoopable'},
    {system:'Lave',starKind:'unknown',fuelUsed:4,fuelLeft:28,fuelCaution:true,station:'Lave Station'}
  ]}
}}});

test('ship route shows observed fuel and modeled leg cost separately',()=>{
  const C=view('one');
  const html=ui.render(C,util);
  assert.match(html,/Fuel on board<\/small><strong>22 t/);
  assert.match(html,/Fuel used/);
  assert.match(html,/<td>4 t<\/td><td>28 t<\/td>/);
  assert.match(html,/Lave Station · Check fuel/);
  assert.match(html,/journal provides the maximum range/);
  C.shipRouteDraft.destination='Eranin';
  assert.match(ui.render(C,util),/name="destination" value="Eranin"/);
  C.state.planWorkspace.key='two';
  assert.doesNotMatch(ui.render(C,util),/Eranin/);
});

test('old rendered route controls submit the displayed commander key',async()=>{
  const C=view('one');
  ui.render(C,util);
  const listeners={};
  const sent=[];
  const root={dataset:{planWorkspace:'one'},addEventListener:(event,fn)=>{listeners[event]=fn;}};
  const u={...util,$:()=>root,context:()=>({}),call:async(name,body)=>sent.push({name,body}),loadView:async()=>{},toast:()=>{}};
  ui.init(C,u);
  const form={dataset:{shipRouteForm:'calculate'},values:{origin:'Sol',destination:'Lave',mode:'neutron',jumpRangeLy:'50'}};
  listeners.input({target:{form,name:'destination',type:'text',value:'Lave'}});
  assert.equal(C.shipRouteDraft.destination,'Lave');
  C.state.planWorkspace.key='two';
  ui.render(C,util);
  listeners.submit({preventDefault(){},target:form});
  await listeners.click({target:{closest:()=>({dataset:{shipRouteAction:'game'}})}});
  assert.equal(sent[0].name,'ship-route.calculate');
  assert.equal(sent[0].body.expectedPlanWorkspace,'one');
  assert.equal(sent[1].name,'ship-route');
  assert.equal(sent[1].body.expectedPlanWorkspace,'one');
  assert.equal(C.shipRouteDraft.destination,undefined);
});

test('large routes render only the current page of jump rows',()=>{
  const C=view('one'), route=C.state.shipRoute.route;
  route.stops=Array.from({length:170},(_,i)=>({system:'Stop '+(i+1),visited:i<85,starKind:'unknown'}));
  route.next=85;
  const followed=ui.render(C,util);
  assert.equal((followed.match(/<tr class=/g)||[]).length,80);
  assert.match(followed,/81–160 \/ 170/);
  assert.match(followed,/is-next">\s*<th scope="row"><span>86\./);
  C.shipRoutePage={id:'1',page:2};
  const earlier=ui.render(C,util);
  assert.equal((earlier.match(/<tr class=/g)||[]).length,10);
  assert.match(earlier,/161–170 \/ 170/);
  assert.match(earlier,/Go to next stop/);
});
