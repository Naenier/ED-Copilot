const assert = require('node:assert/strict');
const {test} = require('node:test');
const {readFileSync} = require('node:fs');
const vm = require('node:vm');

const script = readFileSync(__dirname+'/section-seven-tools.js','utf8');
const tick = () => new Promise(resolve=>setImmediate(resolve));
const catalog = [
  {id:'carrierArrived',group:'Флотоносец',englishGroup:'Fleet carrier',title:'Прыжок выполнен',
    englishTitle:'Jump completed',description:'Прибытие носителя.',englishDescription:'Carrier arrival.'},
  {id:'carrierCooldown',group:'Флотоносец',englishGroup:'Fleet carrier',title:'Охлаждение завершено',
    englishTitle:'Cooldown complete',description:'Можно планировать прыжок.',englishDescription:'Ready to schedule a jump.'},
  {id:'fuel',group:'Полёт',englishGroup:'Flight',title:'Мало топлива',englishTitle:'Low fuel'},
];
const response = {
  catalog,events:[],
  preferences:{enabled:true,sound:false,rules:{
    carrierArrived:{enabled:true,visual:true,sound:true,priority:2},
    carrierCooldown:{enabled:true,visual:true,sound:true,priority:1},
    fuel:{enabled:true,visual:true,sound:false,priority:3},
  }},
};

function fixture(call) {
  const window = {};
  const FormData = class {
    constructor(form) { this.fields = new Map(form.values || []); }
    has(key) { return this.fields.has(key); }
    get(key) { return this.fields.get(key) ?? null; }
  };
  vm.runInNewContext(script,{window,FormData,Intl,Date});
  const screen=window.SectionSevenToolsUI;
  const C={tab:'notifications',state:{planWorkspace:{key:'live:pilot'},settings:{language:'en'}}};
  const handlers={};
  const form={id:'s7-notifications',elements:[{disabled:false},{disabled:true}],
    addEventListener(type,fn){handlers[type]=fn;}};
  const u={t:text=>text,esc:value=>String(value??'').replaceAll('<','&lt;'),locale:()=> 'en-GB',
    btn:(title,action)=>'<button data-click="'+action+'">'+title+'</button>',heading:title=>title,
    $:()=>form,call,renderPage(){},toast(){}};
  return {screen,C,form,handlers,u};
}

test('notification catalog renders localized rules and each channel saves separately',async()=>{
  const calls=[];
  const {screen,C,u,form}=fixture(async(op,input)=>{calls.push([op,input]);return response;});
  screen.init(C,u);
  await tick();
  const html=screen.render(C,u);
  assert.match(html,/Fleet carrier/);
  assert.match(html,/Jump completed/);
  assert.match(html,/name="carrierArrived-sound" type="checkbox" checked/);
  assert.match(html,/name="sound" type="checkbox" *>/);
  form.values=[['sound','on'],['carrierArrived-enabled','on'],['carrierArrived-sound','on'],
    ['carrierArrived-priority','2'],['carrierCooldown-enabled','on'],['carrierCooldown-visual','on'],
    ['carrierCooldown-priority','1'],['fuel-priority','3']];
  await screen.submit(form,C,u);
  const preferences=JSON.parse(JSON.stringify(calls[1][1].preferences));
  assert.deepEqual(preferences,{enabled:false,sound:true,rules:{
    carrierArrived:{enabled:true,visual:false,sound:true,priority:2},
    carrierCooldown:{enabled:true,visual:true,sound:false,priority:1},
    fuel:{enabled:false,visual:false,sound:false,priority:3},
  }});
  assert.equal(calls[1][1].expectedPlanWorkspace,'live:pilot');
  await screen.handle('section-seven-sound-preview',{},C,u);
  assert.equal(calls[2][0],'native');
  assert.equal(calls[2][1].action,'notification-sound-preview');
});

test('a background history reply cannot discard in-progress notification changes',async()=>{
  let reply;
  const {screen,C,handlers,u}=fixture(()=>new Promise(resolve=>{reply=resolve;}));
  screen.init(C,u);
  reply(response);
  await tick();
  C.sectionSevenTools.fetched=0;
  screen.init(C,u);
  handlers.change();
  reply({...response,events:[{title:'Историческая запись',englishTitle:'History entry'}]});
  await tick();
  assert.doesNotMatch(screen.render(C,u),/History entry/);
  assert.equal(C.sectionSevenTools.dirty,true);
});

test('notification controls unlock after a failed save',async()=>{
  let rejectSave;
  const {screen,C,form,u}=fixture(op=>op==='notifications.view' ? Promise.resolve(response) :
    new Promise((resolve,reject)=>{rejectSave=reject;}));
  screen.init(C,u);
  await tick();
  const pending=screen.submit(form,C,u);
  assert.deepEqual(form.elements.map(control=>control.disabled),[true,true]);
  rejectSave(new Error('Save failed'));
  await assert.rejects(pending,/Save failed/);
  assert.deepEqual(form.elements.map(control=>control.disabled),[false,true]);
});
