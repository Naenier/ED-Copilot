'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

const translations=Object.assign({},...fs.readdirSync(path.join(__dirname,'../localization'))
  .filter(name=>name.endsWith('_en.json')).sort().map(name=>require('../localization/'+name)));
const device='15320A57';

function fixture(language='ru') {
  const bind=(id,key='',modifiers=[])=>({id,key,device:key?device:'{NoDevice}',modifiers});
  const buttonAction=(id,key,mods=[])=>({id,kind:'button',slots:[bind('primary',key,mods),bind('secondary')]});
  const data={
    available:true,gameRunning:false,selectionValid:true,
    profiles:[{id:'local',name:'Copilot Gamepad',file:'Copilot Gamepad.4.2.binds'},{id:'default',name:'ConsoleX360',readOnly:true}],
    selections:['general','ship','srv','onfoot'].map(context=>({context,preset:context==='ship'?'Copilot Gamepad':'ConsoleX360'})),
    profile:{id:'local',name:'Copilot Gamepad',file:'Copilot Gamepad.4.2.binds',revision:'revision-a',activeContexts:['ship'],actions:[
      buttonAction('PrimaryFire','GamePad_FaceDown',[{device,key:'GamePad_LBumper'}]),
      buttonAction('SecondaryFire','GamePad_FaceRight'),
      buttonAction('HumanoidJumpButton','GamePad_FaceUp'),
      buttonAction('QuickCommsPanel','GamePad_Back'),
      {id:'YawAxisRaw',kind:'axis',inverted:true,deadzone:.05,slots:[bind('axis','GamePad_LStickX')]}
    ]},
    backups:[{id:'backup-a',createdAt:'2026-11-15T06:00:00Z',reason:'manual',profileNames:['Copilot Gamepad']}]
  };
  const C={tab:'settings',state:{connection:{}}};
  const calls=[],dialogs=[],toasts=[],nodes=new Map(),timers=new Map();
  const sandbox={window:{CopilotTranslations:translations,
    setInterval(handler){timers.set(1,handler);return 1;},clearInterval:id=>timers.delete(id)},document:{addEventListener(){}},Intl,Date};
  vm.runInNewContext(fs.readFileSync(path.join(__dirname,'i18n.js'),'utf8'),sandbox);
  sandbox.window.CopilotI18n.setLanguage(language);
  vm.runInNewContext(fs.readFileSync(path.join(__dirname,'controls.js'),'utf8'),sandbox);
  const api=sandbox.window.ControlsUI;
  const esc=value=>String(value??'').replace(/[&<>"']/g,char=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
  const u={esc,fmt:value=>String(value),icon:name=>`<svg data-icon="${name}"></svg>`,
    btn:(text,action,icon,style,attrs='')=>`<button data-click="${action}" ${attrs}>${esc(text)}</button>`,
    renderPage(){},closeLayer(){},
    async call(op,params){calls.push({op,params:params==null?params:JSON.parse(JSON.stringify(params))});return data;},
    $:selector=>nodes.get(selector),
    toast:message=>toasts.push(message),
    dialog:(title,html,footer)=>dialogs.push({title,html,footer})
  };
  const click=(name,dataset={})=>api.handle('controls-'+name,{dataset},C,u);
  const open=()=>click('tab',{tab:'controls'});
  function form(deviceValue,keyValue,modifiers=[]) {
    const pair=(device,key)=>({
      querySelector:selector=>({value:selector.includes('device')?device:key})
    });
    const main=pair(deviceValue,keyValue);
    nodes.set('#controls-binding-form',{
      querySelector:()=>main,
      querySelectorAll:()=>modifiers.map(item=>pair(item.device,item.key))
    });
    nodes.set('#controls-edit-notice',{hidden:true,textContent:''});
    return main;
  }
  return {api,C,u,data,calls,dialogs,toasts,nodes,click,open,form,timers};
}

test('Settings opens the controls editor and displays all active contexts with both key slots and axes',async()=>{
  const f=fixture('en');
  assert.match(f.api.tabs(f.C,f.u),/Controls/);
  assert.equal(f.api.active(f.C),false);
  await f.open();
  assert.deepEqual(f.calls,[{op:'controls.view',params:{profileId:''}}]);
  const html=f.api.render(f.C,f.u);
  for(const text of ['General controls','Ship','SRV','On foot','Copilot Gamepad','ConsoleX360','Primary binding','Secondary binding','Axis','Control backups','Find an action or key','LB','Unbound'])
    assert.ok(html.includes(text),text);
  assert.doesNotMatch(html,/[А-Яа-яЁё]/);
  f.C.controlsUI.context='ship';
  f.C.controlsUI.query='QuickComms';
  const filtered=f.api.render(f.C,f.u);
  assert.match(filtered,/data-action="QuickCommsPanel"/);
  assert.doesNotMatch(filtered,/data-action="HumanoidJumpButton"/);
});

test('edits the alternative slot with a gamepad modifier and keeps game device identifiers',async()=>{
  const f=fixture();
  await f.open();
  await f.click('edit',{action:'PrimaryFire',slot:'secondary'});
  assert.match(f.dialogs[0].html,/Кнопки геймпада/);
  assert.match(f.dialogs[0].html,/data-device="15320A57"\s+data-key="GamePad_FaceLeft"/);
  assert.match(f.dialogs[0].html,/data-key="Pos_GamePad_LTrigger"/);
  f.form(device,'GamePad_FaceLeft',[{device,key:'GamePad_RBumper'}]);
  await f.click('save-binding');
  assert.deepEqual(f.calls[1],{op:'controls.update',params:{action:'edit',profileId:'local',expectedRevision:'revision-a',changes:[
    {action:'PrimaryFire',slot:'secondary',device,key:'GamePad_FaceLeft',modifiers:[{device,key:'GamePad_RBumper'}]}
  ]}});
});

test('axis tuning validates the range and sends inversion, deadzone and an existing modifier',async()=>{
  const f=fixture();
  await f.open();
  await f.click('edit',{action:'YawAxisRaw',slot:'axis'});
  assert.match(f.dialogs[0].html,/data-key="GamePad_RTrigger"/);
  f.form(device,'GamePad_LStickY',[{device,key:'GamePad_FaceUp'}]);
  f.nodes.set('#controls-invert',{checked:false});
  const deadzone={value:'1.05'};
  f.nodes.set('#controls-deadzone',deadzone);
  await assert.rejects(f.click('save-binding'),/Мёртвая зона/);
  assert.equal(f.calls.length,1);
  deadzone.value='0,15';
  await f.click('save-binding');
  assert.deepEqual(f.calls[1].params.changes,[{action:'YawAxisRaw',slot:'axis',device,key:'GamePad_LStickY',
    inverted:false,deadzone:.15,modifiers:[{device,key:'GamePad_FaceUp'}]}]);
});

test('identical bindings in an overlapping context need a second save, separate contexts do not warn',async()=>{
  const f=fixture();
  await f.open();
  await f.click('edit',{action:'PrimaryFire',slot:'secondary'});
  f.form(device,'GamePad_FaceRight');
  await f.click('save-binding');
  assert.equal(f.calls.length,1);
  assert.match(f.nodes.get('#controls-edit-notice').textContent,/Дополнительный огонь/);
  await f.click('save-binding');
  assert.equal(f.calls.length,2);
  await f.click('edit',{action:'PrimaryFire',slot:'secondary'});
  f.form(device,'GamePad_FaceUp');
  await f.click('save-binding');
  assert.equal(f.calls.length,3);
});

test('only the chosen context is activated, with explicit confirmation before backup restore',async()=>{
  const f=fixture('en');
  await f.open();
  await f.click('activate');
  assert.match(f.dialogs[0].html,/value="ship">Ship/);
  f.nodes.set('#controls-activate-context',{value:'ship'});
  await f.click('apply-activate');
  assert.equal(f.calls[1].params.context,'ship');
  await f.click('restore',{backup:'backup-a'});
  assert.equal(f.calls.length,2);
  assert.match(f.dialogs[1].footer,/controls-apply-restore/);
  await f.click('apply-restore',{backup:'backup-a'});
  assert.deepEqual(f.calls[2],{op:'controls.update',params:{action:'restore',backupId:'backup-a'}});
});

test('uses only native dialog operations for file exchange and permits export while running',async()=>{
  const f=fixture();
  f.data.gameRunning=true;
  await f.open();
  assert.match(f.api.render(f.C,f.u),/Elite Dangerous запущена/);
  await assert.rejects(f.click('clone'),/Закройте Elite Dangerous/);
  await assert.rejects(f.click('import'),/Закройте Elite Dangerous/);
  await f.click('export');
  assert.deepEqual(f.calls[1],{op:'native',params:{action:'controls-export',profileId:'local',filename:'Copilot Gamepad.4.2.binds'}});
  await f.click('backup');
  assert.deepEqual(f.calls[2],{op:'controls.update',params:{action:'backup'}});
  f.data.gameRunning=false;
  await f.click('refresh');
  await f.click('import');
  assert.deepEqual(f.calls[4],{op:'native',params:{action:'controls-import'}});
});

test('process status overrides stale journal state and reloads the view only on a transition',async()=>{
  const f=fixture();
  f.C.state.connection.gameRunning=true;
  await f.open();
  f.api.init(f.C,f.u);
  const poll=f.timers.get(1);
  assert.doesNotMatch(f.api.render(f.C,f.u),/Elite Dangerous запущена/);
  await poll();
  assert.deepEqual(f.calls.map(row=>row.op),['controls.view','controls.status']);
  f.data.gameRunning=true;
  await poll();
  assert.match(f.api.render(f.C,f.u),/Elite Dangerous запущена/);
  assert.deepEqual(f.calls.slice(-2).map(row=>row.op),['controls.status','controls.view']);
  f.data.gameRunning=false;
  await poll();
  assert.doesNotMatch(f.api.render(f.C,f.u),/Elite Dangerous запущена/);
  await f.click('tab',{tab:'general'});
  await poll();
  assert.equal(f.timers.size,0);
});

test('unknown or changed profiles never redirect a pending edit to a different slot',async()=>{
  const f=fixture();
  await f.open();
  await f.click('edit',{action:'PrimaryFire',slot:'primary'});
  f.form(device,'GamePad_FaceRight');
  f.data.profile.revision='revision-b';
  await assert.rejects(f.click('save-binding'),/Профиль обновился/);
  assert.equal(f.calls.length,1);
  f.data.profile.readOnly=true;
  assert.match(f.api.render(f.C,f.u),/Встроенный профиль. Для изменения создайте копию/);
  await f.click('clone');
  f.nodes.set('#controls-clone-name',{value:'New Pad'});
  await f.click('apply-clone');
  assert.deepEqual(f.calls[1],{op:'controls.update',params:{action:'clone',profileId:'local',expectedRevision:'revision-b',name:'New Pad'}});
});
