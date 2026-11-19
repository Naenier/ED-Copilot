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
  const calls=[],dialogs=[],toasts=[],nodes=new Map(),timers=new Map(),listeners=new Map();
  const capture={result:{key:'GamePad_FaceLeft',modifiers:['GamePad_RBumper'],controller:0}};
  const sandbox={window:{CopilotTranslations:translations,
    setInterval(handler){timers.set(1,handler);return 1;},clearInterval:id=>timers.delete(id)},
    document:{addEventListener(type,fn){listeners.set(type,fn);}},Intl,Date};
  vm.runInNewContext(fs.readFileSync(path.join(__dirname,'i18n.js'),'utf8'),sandbox);
  sandbox.window.CopilotI18n.setLanguage(language);
  vm.runInNewContext(fs.readFileSync(path.join(__dirname,'controls-action-labels.js'),'utf8'),sandbox);
  vm.runInNewContext(fs.readFileSync(path.join(__dirname,'controls.js'),'utf8'),sandbox);
  const api=sandbox.window.ControlsUI;
  const esc=value=>String(value??'').replace(/[&<>"']/g,char=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
  const u={esc,fmt:value=>String(value),icon:name=>`<svg data-icon="${name}"></svg>`,
    btn:(text,action,icon,style,attrs='')=>`<button data-click="${action}" ${attrs}>${esc(text)}</button>`,
    renderPage(){},closeLayer(){api.layerClosed(C,u);},
    async call(op,params){
      calls.push({op,params:params==null?params:JSON.parse(JSON.stringify(params))});
      if(op==='controls.capture') return capture.result;
      if(op==='controls.capture.cancel') return {ok:true};
      return data;
    },
    $:selector=>nodes.get(selector),
    toast:message=>toasts.push(message),
    dialog(title,html,footer){
      dialogs.push({title,html,footer});
      if(html.includes('controls-capture-focus')) {
        nodes.set('#controls-capture-focus',{id:'controls-capture-focus',focus(){this.focused=true;},contains(target){return this===target;}});
        nodes.set('#controls-capture-result',{innerHTML:''});
        nodes.set('#controls-edit-notice',{hidden:true,textContent:''});
        nodes.set('#controls-binding-save',{disabled:true});
        nodes.set('#controls-capture-retry',{disabled:true});
      }
    }
  };
  const click=(name,dataset={})=>api.handle('controls-'+name,{dataset},C,u);
  const open=async()=>{
    await click('tab',{tab:'controls'});
    nodes.set('[data-controls-workspace]',{addEventListener(){}});
    api.init(C,u);
  };
  const key=(type,code)=>{
    const event={code,target:nodes.get('#controls-capture-focus'),preventDefault(){this.handled=true;},stopImmediatePropagation(){}};
    listeners.get(type)(event);
    return event.handled;
  };
  return {api,C,u,data,calls,dialogs,toasts,nodes,click,open,timers,capture,key};
}

test('Settings opens the controls editor and displays all active contexts with both key slots and axes',async()=>{
  const f=fixture('en');
  assert.match(f.api.tabs(f.C,f.u),/Controls/);
  assert.equal(f.api.active(f.C),false);
  await f.click('tab',{tab:'diagnostics'});
  assert.match(f.api.tabs(f.C,f.u),/data-tab="diagnostics"\s+class="selected" aria-current="page">Application log/);
  assert.deepEqual(f.calls,[]);
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
  assert.match(f.dialogs[0].html,/Нажмите кнопку или сочетание на геймпаде/);
  assert.doesNotMatch(f.dialogs[0].html,/15320A57|PrimaryFire|datalist/);
  assert.equal(f.calls[1].op,'controls.capture');
  assert.equal(f.calls[1].params.kind,'button');
  assert.equal(f.nodes.get('#controls-capture-focus').focused,true);
  assert.match(f.nodes.get('#controls-capture-result').innerHTML,/>RB<\/kbd>[\s\S]*>X<\/kbd>/);
  await f.click('save-binding');
  assert.deepEqual(f.calls[2],{op:'controls.update',params:{action:'edit',profileId:'local',expectedRevision:'revision-a',changes:[
    {action:'PrimaryFire',slot:'secondary',device,key:'GamePad_FaceLeft',modifiers:[{device,key:'GamePad_RBumper'}]}
  ]}});
});

test('axis tuning validates the range and sends inversion, deadzone and an existing modifier',async()=>{
  const f=fixture();
  await f.open();
  f.capture.result={key:'GamePad_LStickY',modifiers:['GamePad_FaceUp'],controller:0};
  await f.click('edit',{action:'YawAxisRaw',slot:'axis'});
  assert.equal(f.calls[1].params.kind,'axis');
  assert.match(f.dialogs[0].html,/Отклоните стик или нажмите курок/);
  f.nodes.set('#controls-invert',{checked:false});
  const deadzone={value:'1.05'};
  f.nodes.set('#controls-deadzone',deadzone);
  await assert.rejects(f.click('save-binding'),/Мёртвая зона/);
  assert.equal(f.calls.length,2);
  deadzone.value='0,15';
  await f.click('save-binding');
  assert.deepEqual(f.calls[2].params.changes,[{action:'YawAxisRaw',slot:'axis',device,key:'GamePad_LStickY',
    modifiers:[{device,key:'GamePad_FaceUp'}],inverted:false,deadzone:.15}]);
});

test('identical bindings in an overlapping context need a second save, separate contexts do not warn',async()=>{
  const f=fixture();
  await f.open();
  f.capture.result={key:'GamePad_FaceRight',modifiers:[],controller:0};
  await f.click('edit',{action:'PrimaryFire',slot:'secondary'});
  await f.click('save-binding');
  assert.equal(f.calls.length,2);
  assert.match(f.nodes.get('#controls-edit-notice').textContent,/Дополнительный огонь/);
  await f.click('save-binding');
  assert.equal(f.calls.length,3);
  f.capture.result={key:'GamePad_FaceUp',modifiers:[],controller:0};
  await f.click('edit',{action:'PrimaryFire',slot:'secondary'});
  await f.click('save-binding');
  assert.equal(f.calls.length,5);
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
  f.data.profile.revision='revision-b';
  await assert.rejects(f.click('save-binding'),/Профиль обновился/);
  assert.equal(f.calls.length,2);
  f.data.profile.readOnly=true;
  assert.match(f.api.render(f.C,f.u),/Встроенный профиль. Для изменения создайте копию/);
  await f.click('clone');
  f.nodes.set('#controls-clone-name',{value:'New Pad'});
  await f.click('apply-clone');
  assert.deepEqual(f.calls[2],{op:'controls.update',params:{action:'clone',profileId:'local',expectedRevision:'revision-b',name:'New Pad'}});
});

test('preserves physical Russian passthrough keys and ignores unrelated keyboard input on a gamepad profile',async()=>{
  const f=fixture();
  f.data.profile.keyboardLayout='ru-RU';
  f.capture.result={unavailable:true};
  await f.open();
  await f.click('edit',{action:'PrimaryFire',slot:'secondary'});
  assert.ok(f.key('keydown','KeyQ'));
  assert.match(f.nodes.get('#controls-capture-result').innerHTML,/используйте геймпад/);
  assert.ok(f.key('keyup','KeyQ'));
  assert.equal(f.nodes.get('#controls-binding-save').disabled,true);
  for(const [physical,key,label] of [['KeyW','Key_ц','Ц'],['KeyS','Key_ы','Ы']]) {
    if(physical==='KeyS') await f.click('edit',{action:'PrimaryFire',slot:'secondary'});
    f.key('keydown',physical);
    f.key('keyup',physical);
    assert.ok(f.nodes.get('#controls-capture-result').innerHTML.includes(`>${label}</kbd>`));
    await f.click('save-binding');
    assert.deepEqual(f.calls.at(-1).params.changes,[
      {action:'PrimaryFire',slot:'secondary',device:'Keyboard',key,modifiers:[]}
    ]);
  }
});

test('mixed profiles capture a physical keyboard chord and ignore a native result after keyboard input',async()=>{
  const f=fixture();
  f.data.profile.keyboardLayout='ru-RU';
  Object.assign(f.data.profile.actions.find(action=>action.id==='QuickCommsPanel').slots[0],
    {device:'Keyboard',key:'Key_Enter'});
  let resolvePad;
  f.capture.result=new Promise(resolve=>{resolvePad=resolve;});
  await f.open();
  const pending=f.click('edit',{action:'PrimaryFire',slot:'secondary'});
  const {captureId}=f.calls.at(-1).params;
  f.key('keydown','ControlLeft');
  f.key('keydown','KeyW');
  f.key('keyup','KeyW');
  resolvePad({key:'GamePad_FaceUp',controller:0});
  await pending;
  assert.deepEqual(f.calls.at(-1),{op:'controls.capture.cancel',params:{captureId}});
  await f.click('save-binding');
  assert.deepEqual(f.calls.at(-1).params.changes,[
    {action:'PrimaryFire',slot:'secondary',device:'Keyboard',key:'Key_ц',modifiers:[{device:'Keyboard',key:'Key_LeftControl'}]}
  ]);
});

test('closing the editor cancels its native capture without recording a binding',async()=>{
  const f=fixture();
  let resolvePad;
  f.capture.result=new Promise(resolve=>{resolvePad=resolve;});
  await f.open();
  const pending=f.click('edit',{action:'PrimaryFire',slot:'secondary'});
  const {captureId}=f.calls.at(-1).params;
  f.api.layerClosed(f.C,f.u);
  resolvePad({key:'GamePad_FaceUp',controller:0});
  await pending;
  assert.deepEqual(f.calls.at(-1),{op:'controls.capture.cancel',params:{captureId}});
  assert.equal(f.C.controlsUI.edit,undefined);
  assert.equal(f.calls.some(call=>call.op==='controls.update'),false);
});

test('Russian action rows hide XML identifiers while filtering can still use them',async()=>{
  const f=fixture();
  await f.open();
  f.C.controlsUI.query='YawAxisRaw';
  const html=f.api.render(f.C,f.u);
  assert.match(html,/Ось рыскания/);
  assert.doesNotMatch(html.replace(/<[^>]+>/g,''),/YawAxisRaw|GamePad_LStickX|15320A57/);
});
