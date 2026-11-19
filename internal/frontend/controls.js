(()=>{
'use strict';
const t=(key,vars)=>window.CopilotI18n?.t(key,vars)??String(key??'');
const th=(u,key,vars)=>u.esc(t(key,vars));
const contextLabels={general:'Общее управление',ship:'Корабль',srv:'ТРП',onfoot:'Пешком'};
const filterLabels={...contextLabels,camera:'Камера',scanner:'Сканеры'};
const keyLabels={
  GamePad_FaceDown:'A',GamePad_FaceRight:'B',GamePad_FaceLeft:'X',GamePad_FaceUp:'Y',
  GamePad_DPadUp:'Крестовина ↑',GamePad_DPadDown:'Крестовина ↓',GamePad_DPadLeft:'Крестовина ←',GamePad_DPadRight:'Крестовина →',
  GamePad_LBumper:'LB',GamePad_RBumper:'RB',GamePad_LTrigger:'LT',GamePad_RTrigger:'RT',
  Pos_GamePad_LTrigger:'LT',Pos_GamePad_RTrigger:'RT',
  GamePad_LThumb:'LS',GamePad_RThumb:'RS',GamePad_Start:'Menu',GamePad_Back:'View',
  GamePad_LStickX:'LS X',GamePad_LStickY:'LS Y',GamePad_RStickX:'RS X',GamePad_RStickY:'RS Y',
  Key_LeftShift:'L Shift',Key_RightShift:'R Shift',Key_LeftControl:'L Ctrl',Key_RightControl:'R Ctrl',
  Key_LeftAlt:'L Alt',Key_RightAlt:'R Alt',Key_Space:'Пробел',Key_Enter:'Enter',Key_Escape:'Esc',
  Key_UpArrow:'↑',Key_DownArrow:'↓',Key_LeftArrow:'←',Key_RightArrow:'→',
  Mouse_1:'ЛКМ',Mouse_2:'ПКМ',Mouse_3:'СКМ'
};
const slotLabels={primary:'Основное назначение',secondary:'Дополнительное назначение',axis:'Ось'};
const button=(u,label,action,icon='',style='quiet',attrs='')=>u.btn(t(label),'controls-'+action,icon,style,attrs);
const profile=C=>C.controlsUI?.data?.profile;
function local(C) {
  return C.controlsUI??={tab:'general',query:'',context:'',data:null,loading:false,busy:false,error:''};
}
function active(C) {return C?.tab==='settings'&&local(C).tab==='controls';}
function tabTitle() {return window.CopilotI18n?.language==='en'?'Controls':'Управление';}
const settingsTabs={general:'Общие настройки',controls:'Управление',diagnostics:'Журнал приложения'};
function tabs(C,u) {
  return `<nav class="segmented controls-settings-tabs" aria-label="${th(u,'Разделы настроек')}">
    ${Object.entries(settingsTabs).map(([id,label])=>`<button type="button" data-click="controls-tab" data-tab="${id}"
      class="${local(C).tab===id?'selected':''}"${local(C).tab===id?' aria-current="page"':''}>${id==='controls'?u.esc(tabTitle()):th(u,label)}</button>`).join('')}
  </nav>`;
}
const contextOf=action=>{
  if(action.context&&Object.hasOwn(filterLabels,action.context)) return action.context;
  const id=action.id;
  if(/Humanoid|CommanderCreator/.test(id)) return 'onfoot';
  if(/Buggy|Drive|Steering|Steer|RecallDismissShip|(?:De|In)creaseSpeedButton|VerticalThrustersButton/.test(id)) return 'srv';
  if(/Camera|FreeCam|VanityCam|^Cam|HeadLook/.test(id)) return 'camera';
  if(/ExplorationFSS|ExplorationSAA|^SAA/.test(id)) return 'scanner';
  if(/^(UI_|CycleNextPanel|CyclePreviousPanel|CycleNextPage|CyclePreviousPage|Pause|Friends|Mouse|Galnet|Store|HMD)/.test(id)) return 'general';
  return 'ship';
};
const humanize=id=>String(id||'').replace(/_/g,' ').replace(/([a-z])([A-Z])/g,'$1 $2').replace(/([A-Z]+)([A-Z][a-z])/g,'$1 $2');
function labelFor(action) {
  if(window.CopilotI18n?.language==='en') return humanize(action.id);
  return window.ControlActionLabels?.[action.id]||t('Действие вне справочника');
}
function keyName(key='') {
  if(!key) return '';
  if(keyLabels[key]) return t(keyLabels[key]);
  const direction=key.match(/^(Pos|Neg)_(.+)$/);
  if(direction) return `${keyName(direction[2])} ${direction[1]==='Pos'?'+':'−'}`;
  const button=key.match(/^(?:Joy|Mouse)_(\d+)$/);
  if(button) return t('Кнопка {number}',{number:button[1]});
  const axis=key.match(/^(?:Joy|Mouse)_([XYZRUVA])Axis$/);
  if(axis) return t('Ось {axis}',{axis:axis[1]});
  const numpad=key.match(/^Key_Numpad_(\d)$/);
  if(numpad) return t('Цифровая клавиша {number}',{number:numpad[1]});
  if(!key.startsWith('Key_')) return t('Кнопка устройства');
  const name=key.slice(4);
  return name.length===1?name.toUpperCase():t(humanize(name));
}
function inputDevice(device,key) {
  if(device==='Keyboard') return t('Клавиатура');
  if(device==='Mouse') return t('Мышь');
  return t(key?.includes('GamePad_')?'Геймпад':'Контроллер');
}
function running(C) {
  const l=local(C);
  return l.gameRunning??l.data?.gameRunning??(C.state?.connection?.gameRunning===true);
}
const notSet=slot=>!slot?.key||!slot.device||slot.device==='{NoDevice}';
function binding(u,slot) {
  if(notSet(slot)) return `<span class="controls-unbound">${th(u,'Не назначено')}</span>`;
  const parts=[...(slot.modifiers||[]),slot];
  return `<span class="controls-chord">${parts.map((item,index)=>`${index?'<span aria-hidden="true">+</span>':''}
    <kbd title="${u.esc(inputDevice(item.device,item.key))}">${u.esc(keyName(item.key))}</kbd>`).join('')}</span>`;
}
const canonical=slot=>notSet(slot)?'':[
  slot.device,slot.key,
  ...(slot.modifiers||[]).map(item=>item.device+'|'+item.key).sort()
].join('\u0000').toLowerCase();
function matchesContext(a,b) {
  const first=contextOf(a), second=contextOf(b);
  if(first==='general'||second==='general') return true;
  if(first!==second) return false;
  const variant=id=>id.endsWith('_Landing')?'landing':id.endsWith('Alternate')?'alternate':id.startsWith('MultiCrew')?'multi':'standard';
  return first!=='ship'||variant(a.id)===variant(b.id);
}
function matchingBindings(detail,action,slot,replacement) {
  const key=canonical(replacement);
  if(!key) return [];
  return (detail.actions||[]).filter(other=>other.kind===action.kind&&matchesContext(action,other)&&
    other.slots?.some(item=>(other.id!==action.id||item.id!==slot)&&canonical(item)===key));
}
function renderActions(C,u) {
  const l=local(C), detail=profile(C);
  if(!detail) return '';
  const query=l.query.trim().toLocaleLowerCase();
  const buttonQuery=/^(?:[axyb]|lb|rb|lt|rt|ls|rs)$/.test(query);
  const norm=value=>String(value??'').toLocaleLowerCase();
  const options=(detail.actions||[]).filter(action=>{
    if(l.context&&contextOf(action)!==l.context) return false;
    if(!query) return true;
    const parts=(action.slots||[]).flatMap(slot=>[slot,...(slot.modifiers||[])]);
    if(buttonQuery) return parts.some(item=>norm(keyName(item.key))===query);
    const chords=(action.slots||[]).map(slot=>[...(slot.modifiers||[]),slot].map(item=>keyName(item.key)).join(' + '));
    return [action.id,labelFor(action),humanize(action.id),...chords,
      ...parts.flatMap(item=>[item.device,item.key,keyName(item.key)])
    ].some(value=>norm(value).includes(query));
  });
  if(!options.length) return `<p class="controls-empty">${th(u,'Подходящих действий нет')}</p>`;
  const grouped=new Map();
  for(const action of options) {
    const category=contextOf(action);
    if(!grouped.has(category)) grouped.set(category,[]);
    grouped.get(category).push(action);
  }
  const disabled=running(C)||!!detail.readOnly||l.busy||l.loading;
  return [...grouped].map(([category,actions])=>`<section class="controls-category">
    <h3>${th(u,filterLabels[category])}</h3>
    ${actions.map(action=>`<div class="controls-action">
      <div class="controls-action-name"><strong>${u.esc(labelFor(action))}</strong></div>
      <div class="controls-action-slots">${(action.slots||[]).map(slot=>`<button type="button" class="controls-slot"
        data-click="controls-edit" data-action="${u.esc(action.id)}" data-slot="${u.esc(slot.id)}" ${disabled?'disabled':''}>
        <small>${th(u,slotLabels[slot.id]||slot.id)}</small>${binding(u,slot)}</button>`).join('')}</div>
    </div>`).join('')}
  </section>`).join('');
}
function renderProfiles(C,u) {
  const l=local(C), data=l.data, detail=profile(C), blocked=running(C)||l.busy||l.loading;
  const options=(data.profiles||[]).map(item=>`<option value="${u.esc(item.id)}"${item.id===detail?.id?' selected':''}>${u.esc(item.name)}${item.readOnly?` · ${th(u,'Встроенный')}`:''}</option>`);
  return `<section class="panel controls-profiles">
    <h2>${th(u,'Профили управления')}</h2>
    <div class="controls-active">${(data.selections||[]).map(row=>`<div>
      <span>${th(u,contextLabels[row.context]||row.context)}</span>
      <strong>${u.esc(row.preset||t('Не выбран'))}</strong>
    </div>`).join('')}</div>
    ${data.selectionError?`<p class="notice" role="alert">${th(u,data.selectionError)}</p>`:''}
    ${data.skippedProfiles?`<p class="controls-profile-meta">${th(u,'Пропущено неподдерживаемых файлов {count}.',{count:u.fmt(data.skippedProfiles)})}</p>`:''}
    <div class="controls-profile-tools">
      <label class="controls-field"><span>${th(u,'Открытый профиль')}</span>
        <select id="controls-profile" ${!options.length||l.busy||l.loading?'disabled':''}>
          ${options.length?options.join(''):`<option value="">${th(u,'Профили не найдены')}</option>`}
        </select></label>
      ${button(u,'Дублировать','clone','copy','quiet',!detail||blocked?'disabled':'')}
      ${button(u,'Назначить профиль','activate','check','quiet',!detail||blocked||data.selectionValid===false?'disabled':'')}
      ${button(u,'Импортировать .binds','import','arrow-down','quiet',blocked?'disabled':'')}
      ${button(u,'Экспортировать .binds','export','arrow-up','quiet',!detail||l.busy||l.loading?'disabled':'')}
    </div>
    ${detail?`<div class="controls-profile-meta"><code>${u.esc(detail.file||detail.name)}</code>
      ${detail.keyboardLayout?`<span>${th(u,'Раскладка')} <b>${u.esc(detail.keyboardLayout)}</b></span>`:''}
      ${detail.readOnly?`<span>${th(u,'Встроенный профиль. Для изменения создайте копию.')}</span>`:''}
    </div>`:''}
  </section>`;
}
function renderBackups(C,u) {
  const l=local(C), rows=l.data.backups||[];
  const reasons={manual:'Вручную',edit:'Изменение назначения',activate:'Выбор активного профиля',clone:'Дублирование',import:'Импорт профиля',restore:'Перед восстановлением'};
  const date=value=>u.date?.(value,{dateStyle:'medium',timeStyle:'short'})||String(value||'—');
  return `<details class="panel controls-backups"${l.backupsOpen?' open':''}>
    <summary>${u.icon('shield-check')}<h2>${th(u,'Резервные копии управления')}</h2><span>${u.esc(rows.length)}</span>${u.icon('chevron-down')}</summary>
    <div class="controls-backup-list">${button(u,'Создать копию','backup','plus','quiet',l.busy?'disabled':'')}
      ${rows.length?rows.map(row=>`<div class="controls-backup-row"><div><strong>${u.esc(date(row.createdAt))}</strong>
        <span>${th(u,reasons[row.reason]||row.reason)}${row.profileNames?.length?' · '+u.esc(row.profileNames.join(', ')):''}</span>
      </div>${button(u,'Восстановить','restore','clock','quiet',`data-backup="${u.esc(row.id)}" ${running(C)||l.busy?'disabled':''}`)}</div>`).join(''):
        `<p class="controls-empty">${th(u,'Резервных копий пока нет')}</p>`}
    </div>
  </details>`;
}
function render(C,u) {
  const l=local(C), detail=profile(C);
  return `<section class="controls-workspace" data-controls-workspace>
    <div class="controls-toolbar">
      <h2>${th(u,'Управление Elite Dangerous')}</h2>
      ${button(u,'Обновить','refresh','refresh-cw','quiet',l.loading||l.busy?'disabled':'')}
    </div>
    ${l.error?`<p class="notice" role="alert">${u.esc(l.error)}</p>`:''}
    ${running(C)?`<p class="notice">${th(u,'Elite Dangerous запущена. Закройте игру, чтобы изменить или восстановить профиль.')}</p>`:''}
    ${!l.data?l.loading||!l.error?`<p class="panel controls-empty" role="status">${th(u,'Читаем профили управления…')}</p>`:'':
      `${!l.data.available?`<p class="notice">${th(u,'Папка назначений Elite Dangerous пока недоступна.')}</p>`:''}
      ${l.loading?`<p class="controls-empty" role="status">${th(u,'Свежие данные профиля загружаются…')}</p>`:''}
      ${renderProfiles(C,u)}${renderBackups(C,u)}
      ${detail?`<section class="panel controls-actions">
        <header><h2>${th(u,'Назначения')}</h2>
          <div class="controls-filters"><input type="search" id="controls-query" value="${u.esc(l.query)}"
            placeholder="${th(u,'Найти действие или клавишу')}" aria-label="${th(u,'Найти действие или клавишу')}">
            <select id="controls-context" aria-label="${th(u,'Контекст управления')}">
              ${[['','Все контексты'],...Object.entries(filterLabels)].map(([id,label])=>`<option value="${id}"${l.context===id?' selected':''}>${th(u,label)}</option>`).join('')}
            </select>
          </div></header><div id="controls-action-list">${renderActions(C,u)}</div>
      </section>`:''}`}
  </section>`;
}
const keyboardCodes={
  Backspace:'Backspace',Tab:'Tab',Enter:'Enter',Space:'Space',CapsLock:'CapsLock',
  ShiftLeft:'LeftShift',ShiftRight:'RightShift',ControlLeft:'LeftControl',ControlRight:'RightControl',
  AltLeft:'LeftAlt',AltRight:'RightAlt',MetaLeft:'LeftWin',MetaRight:'RightWin',ContextMenu:'Apps',
  ArrowUp:'UpArrow',ArrowDown:'DownArrow',ArrowLeft:'LeftArrow',ArrowRight:'RightArrow',
  Insert:'Insert',Delete:'Delete',Home:'Home',End:'End',PageUp:'PageUp',PageDown:'PageDown',
  PrintScreen:'SYSRQ',Pause:'Pause',NumLock:'NumLock',ScrollLock:'ScrollLock',
  Backquote:'Grave',Minus:'Minus',Equal:'Equals',BracketLeft:'LeftBracket',BracketRight:'RightBracket',
  Backslash:'BackSlash',Semicolon:'SemiColon',Quote:'Apostrophe',Comma:'Comma',Period:'Period',Slash:'Slash',
  IntlBackslash:'OEM_102',NumpadDivide:'Numpad_Divide',NumpadMultiply:'Numpad_Multiply',
  NumpadSubtract:'Numpad_Subtract',NumpadAdd:'Numpad_Add',NumpadEnter:'Numpad_Enter',
  NumpadDecimal:'Numpad_Decimal',NumpadComma:'Numpad_Comma',NumpadEqual:'Numpad_Equals'
};
const russianLetters='фисвуапршолдьтщзйкыегмцчня';
const russianPunctuation={BracketLeft:'х',BracketRight:'ъ',Semicolon:'ж',Quote:'э',Backquote:'ё',Comma:'б',Period:'ю'};
function eliteKeyboardKey(code='',layout) {
  const russian=/^ru(?:[-_]|$)/i.test(layout||'');
  if(/^Key[A-Z]$/.test(code)) return 'Key_'+(russian?russianLetters[code.charCodeAt(3)-65]:code[3]);
  if(russian&&russianPunctuation[code]) return 'Key_'+russianPunctuation[code];
  if(/^Digit[0-9]$/.test(code)) return 'Key_'+code[5];
  if(/^Numpad[0-9]$/.test(code)) return 'Key_Numpad_'+code[6];
  if(/^F(?:[1-9]|1[0-5])$/.test(code)) return 'Key_'+code;
  return keyboardCodes[code]?'Key_'+keyboardCodes[code]:'';
}
function gamepadDevice(detail,slot) {
  const gamepad=item=>!notSet(item)&&/(?:^|_)GamePad_/.test(item.key);
  if(gamepad(slot)) return slot.device;
  const uses=new Map();
  for(const action of detail.actions||[]) for(const bound of action.slots||[]) {
    for(const part of [bound,...(bound.modifiers||[])]) {
      if(gamepad(part)) uses.set(part.device,(uses.get(part.device)||0)+1);
    }
  }
  return [...uses].sort((a,b)=>b[1]-a[1])[0]?.[0]||'GamePad';
}
const passthroughKeys=new Set(['Key_ц','Key_ы','Key_1','Key_2','Key_3','Key_4']);
function gamepadFirst(detail) {
  let pad=0, keys=0;
  for(const action of detail.actions||[]) for(const slot of action.slots||[]) {
    for(const part of [slot,...(slot.modifiers||[])]) {
      if(notSet(part)) continue;
      if(part.device==='Keyboard'&&passthroughKeys.has(part.key)) keys++;
      else if(/(?:^|_)GamePad_/.test(part.key)) pad++;
      else return false;
    }
  }
  return pad>0&&pad>=keys;
}
function editableTuning(action,slot) {
  return slot.id==='axis'?(typeof action.inverted==='boolean'||action.deadzone!=null):
    typeof action.toggle==='boolean'||typeof slot.hold==='boolean';
}
function editor(C,u,detail,action,slot) {
  const l=local(C), axis=slot.id==='axis', tuning=editableTuning(action,slot);
  const edit={profileId:detail.id,revision:detail.revision,action:action.id,slot:slot.id,
    keyboardLayout:detail.keyboardLayout,gamepad:gamepadDevice(detail,slot),gamepadFirst:gamepadFirst(detail),
    pressed:new Map(),blocked:new Set()};
  l.edit=edit;
  const body=`<form id="controls-binding-form" class="controls-binding-form">
    <div class="controls-capture-box">
      <div class="controls-capture-current"><span>${th(u,slotLabels[slot.id]||slot.id)}</span>${binding(u,slot)}</div>
      <div id="controls-capture-focus" class="controls-capture-target" tabindex="0" aria-labelledby="controls-capture-prompt">
        <strong id="controls-capture-prompt">${th(u,axis?'Отклоните стик или нажмите курок':
          edit.gamepadFirst?'Нажмите кнопку или сочетание на геймпаде':'Нажмите кнопку или сочетание клавиш')}</strong>
        <div id="controls-capture-result" role="status" aria-live="polite"></div>
      </div>
      ${button(u,'Повторить захват','capture-retry','refresh-cw','quiet','id="controls-capture-retry" disabled')}
    </div>
    ${tuning?`<details class="controls-axis-options"><summary>${th(u,'Параметры назначения')}</summary>
      ${axis?`<div class="controls-axis">
        ${typeof action.inverted==='boolean'?`<label><input type="checkbox" id="controls-invert"${action.inverted?' checked':''}> ${th(u,'Инвертировать ось')}</label>`:''}
        ${action.deadzone!=null?`<label class="controls-field"><span>${th(u,'Мёртвая зона, 0–1')}</span><input id="controls-deadzone" type="text" inputmode="decimal"
          value="${u.esc(action.deadzone)}" maxlength="18"></label>`:''}</div>`:
        `${typeof action.toggle==='boolean'?`<label class="controls-toggle"><input id="controls-toggle" type="checkbox"${action.toggle?' checked':''}> ${th(u,'Переключатель вместо удержания')}</label>`:''}
        ${typeof slot.hold==='boolean'?`<label class="controls-toggle"><input id="controls-hold" type="checkbox"${slot.hold?' checked':''}> ${th(u,'Удерживать кнопку')}</label>`:''}`}
    </details>`:''}
    <div id="controls-edit-notice" class="notice" role="status" hidden></div>
  </form>`;
  const footer=button(u,'Очистить назначение','clear-binding','x','quiet',notSet(slot)?'disabled':'')+
    '<span class="spacer"></span>'+u.btn(t('Отмена'),'close-dialog','','quiet')+
    button(u,'Сохранить назначение','save-binding','check','primary','id="controls-binding-save" disabled');
  u.dialog(labelFor(action),body,footer,true);
  return startCapture(C,u,edit);
}
function showEditNotice(u,text) {
  const node=u.$('#controls-edit-notice');
  if(node) {node.hidden=!text;node.textContent=text||'';}
}
function collectEdit(u,edit,clear=false) {
  const change={action:edit.action,slot:edit.slot};
  if(clear) return {...change,device:'',key:'',modifiers:[]};
  if(edit.candidate) Object.assign(change,edit.candidate);
  if(edit.slot==='axis') {
    const raw=u.$('#controls-deadzone')?.value.trim().replace(',','.');
    const invert=u.$('#controls-invert');
    if(invert) change.inverted=invert.checked;
    if(raw) {
      if(!/^(?:\d+(?:\.\d+)?|\.\d+)$/.test(raw)||Number(raw)<0||Number(raw)>1)
        throw new Error(t('Мёртвая зона должна быть числом от 0 до 1.'));
      change.deadzone=Number(raw);
    }
  }
  for(const [field,id] of [['toggle','#controls-toggle'],['hold','#controls-hold']]) {
    const flag=u.$(id);
    if(flag) change[field]=flag.checked;
  }
  return change;
}
function showCurrentConflicts(C,u) {
  const l=local(C), detail=profile(C), edit=l.edit;
  const action=detail?.actions?.find(item=>item.id===edit?.action);
  if(!action) return;
  l.confirmedConflict='';
  const potential=edit.candidate?matchingBindings(detail,action,edit.slot,edit.candidate):[];
  showEditNotice(u,potential.length?t('Возможное пересечение с действиями {list}.',{list:potential.map(labelFor).join(', ')}):'');
}
function syncCapture(C,u,edit,notice='') {
  const output=u.$('#controls-capture-result');
  if(output) output.innerHTML=edit.candidate?binding(u,edit.candidate):th(u,notice||
    (edit.slot==='axis'?'Ожидаем движение оси геймпада…':
      edit.gamepadFirst?'Ожидаем ввод с геймпада…':'Ожидаем кнопку геймпада или клавишу…'));
  const save=u.$('#controls-binding-save');
  if(save) save.disabled=running(C)||!(edit.candidate||edit.tuningChanged);
  const retry=u.$('#controls-capture-retry');
  if(retry) retry.disabled=running(C)||edit.waiting;
}
function cancelCapture(u,edit) {
  const id=edit?.captureId;
  if(!id) return;
  edit.captureId='';
  void u.call('controls.capture.cancel',{captureId:id}).catch(()=>{});
}
function finishCapture(C,u,edit,candidate) {
  cancelCapture(u,edit);
  edit.waiting=false;
  edit.pressed.clear();
  edit.blocked.clear();
  edit.candidate=candidate;
  syncCapture(C,u,edit);
  showCurrentConflicts(C,u);
}
let captureSequence=0;
async function startCapture(C,u,edit) {
  cancelCapture(u,edit);
  const captureId='binding-'+Date.now()+'-'+(++captureSequence);
  edit.captureId=captureId;
  edit.waiting=true;
  edit.candidate=null;
  edit.pressed.clear();
  edit.blocked.clear();
  syncCapture(C,u,edit);
  showEditNotice(u,'');
  u.$('#controls-capture-focus')?.focus();
  const current=()=>local(C).edit===edit&&edit.captureId===captureId;
  try {
    const result=await u.call('controls.capture',{captureId,kind:edit.slot==='axis'?'axis':'button'});
    if(!current()) return;
    edit.captureId='';
    if(typeof result?.key==='string'&&/^(?:(?:Pos|Neg)_)?GamePad_/.test(result.key)) {
      const device=edit.gamepad;
      const keys=[...new Set(Array.isArray(result.modifiers)?result.modifiers:[])]
        .filter(key=>typeof key==='string'&&key!==result.key&&/^(?:(?:Pos|Neg)_)?GamePad_/.test(key));
      finishCapture(C,u,edit,{device,key:result.key,modifiers:keys.map(key=>({device,key}))});
      return;
    }
    const keyboardAvailable=edit.slot!=='axis';
    edit.waiting=!!result?.unavailable&&keyboardAvailable;
    const retry=u.$('#controls-capture-retry');
    const message=result?.unavailable?
      keyboardAvailable&&!edit.gamepadFirst?'Геймпад не обнаружен. Можно нажать клавишу.':'Геймпад не обнаружен. Подключите геймпад и повторите захват.':
      result?.timedOut?'Время захвата истекло. Повторите попытку.':'Захват отменён.';
    syncCapture(C,u,edit,message);
    if(retry) retry.disabled=running(C);
  } catch(error) {
    if(!current()) return;
    edit.captureId='';
    edit.waiting=false;
    syncCapture(C,u,edit,t(error.message||'Не удалось начать захват.'));
  }
}
function inputPair(edit,event,type) {
  if(type==='keyboard') {
    const key=eliteKeyboardKey(event.code,edit.keyboardLayout);
    return key?{device:'Keyboard',key}:null;
  }
  const button={0:1,2:2,1:3,3:4}[event.button];
  return button?{device:'Mouse',key:'Mouse_'+button}:null;
}
function captureInput(C,u,event,type,released) {
  const edit=local(C).edit, target=u.$('#controls-capture-focus');
  if(!edit?.waiting||edit.slot==='axis'||!target?.contains(event.target)||running(C)) return;
  if(type==='keyboard'&&event.code==='Escape') return; // The dialog handles Escape and cancels capture.
  const pair=inputPair(edit,event,type);
  if(!pair) return;
  event.preventDefault();
  event.stopImmediatePropagation?.();
  const id=type==='keyboard'?event.code:'mouse-'+event.button;
  if(edit.gamepadFirst&&(pair.device!=='Keyboard'||!passthroughKeys.has(pair.key))) {
    if(released) edit.blocked.delete(id);
    else {
      edit.blocked.add(id);
      edit.pressed.clear();
      if(!event.repeat) syncCapture(C,u,edit,'Для этого профиля используйте геймпад или клавиши Ц, Ы и 1–4.');
    }
    return;
  }
  if(edit.blocked.size) return;
  if(!released) {
    if(!event.repeat&&!edit.pressed.has(id)&&edit.pressed.size<9) {
      cancelCapture(u,edit);
      edit.pressed.set(id,pair);
      const pressed=[...edit.pressed.values()];
      const output=u.$('#controls-capture-result');
      if(output) output.innerHTML=binding(u,{...pressed.pop(),modifiers:pressed});
    }
    return;
  }
  if(!edit.pressed.has(id)) return;
  const pressed=[...edit.pressed.values()];
  finishCapture(C,u,edit,{...pressed.pop(),modifiers:pressed});
}
function layerClosed(C,u) {
  const edit=C.controlsUI?.edit;
  if(!edit) return;
  cancelCapture(u,edit);
  delete C.controlsUI.edit;
}
function setGameRunning(C,u,running) {
  if(typeof running!=='boolean') return;
  local(C).gameRunning=running;
  const edit=local(C).edit;
  if(edit&&running&&edit.waiting) {
    cancelCapture(u,edit);
    edit.waiting=false;
    syncCapture(C,u,edit,'Захват отменён.');
  }
  for(const action of ['clear-binding','apply-clone','apply-activate','apply-restore']) {
    const button=u.$(`#layers [data-click="controls-${action}"]`);
    if(!button) continue;
    if(running&&!button.disabled) {
      button.dataset.controlsGameDisabled='true';
      button.disabled=true;
    }
    if(!running&&button.dataset.controlsGameDisabled) {
      delete button.dataset.controlsGameDisabled;
      button.disabled=false;
    }
  }
  if(edit) {
    const save=u.$('#controls-binding-save');
    if(save) save.disabled=running||!(edit.candidate||edit.tuningChanged);
    const retry=u.$('#controls-capture-retry');
    if(retry) retry.disabled=running||edit.waiting;
  }
}
async function refresh(C,u,profileId) {
  const l=local(C), epoch=(l.epoch||0)+1;
  l.epoch=epoch;
  l.loading=true;
  l.error='';
  try {
    const selected=profileId??profile(C)?.id??'';
    let data;
    try {
      data=await u.call('controls.view',{profileId:selected});
    } catch(error) {
      if(!selected||t(error.message)!==t('Профиль управления не найден.')) throw error;
      data=await u.call('controls.view',{profileId:''});
    }
    if(l.epoch===epoch) {
      l.data=data;
      setGameRunning(C,u,data?.gameRunning);
    }
  } catch(error) {
    if(l.epoch===epoch) l.error=t(error.message||'Не удалось открыть профили управления.');
  } finally {
    if(l.epoch===epoch) l.loading=false;
    if(active(C)) u.renderPage();
  }
}
function pollStatus(C,u) {
  const l=local(C);
  if(l.timer||typeof window.setInterval!=='function') return;
  l.timer=window.setInterval(async()=>{
    if(!active(C)) {
      window.clearInterval(l.timer);
      delete l.timer;
      return;
    }
    if(l.statusPending||l.loading||document.hidden) return;
    l.statusPending=true;
    try {
      const status=await u.call('controls.status');
      if(!active(C)||typeof status?.gameRunning!=='boolean') return;
      const changed=running(C)!==status.gameRunning;
      setGameRunning(C,u,status.gameRunning);
      if(changed) await refresh(C,u);
    } catch(_) {
      // The file operations independently check whether Elite Dangerous is running.
    } finally {
      l.statusPending=false;
    }
  },3000);
}
function requireClosed(C) {
  if(running(C)) throw new Error(t('Закройте Elite Dangerous перед изменением профилей управления.'));
}
async function mutate(C,u,body,message,close=true) {
  const l=local(C);
  if(l.busy) return;
  if(body.action!=='backup') requireClosed(C);
  l.busy=true;
  try {
    const result=await u.call('controls.update',body);
    if(close) u.closeLayer();
    if(result?.profiles) {
      l.data=result;
      setGameRunning(C,u,result.gameRunning);
    }
    else await refresh(C,u,result?.profileId??body.profileId);
    u.toast(t(message));
  } finally {
    l.busy=false;
    if(active(C)) u.renderPage();
  }
}
async function handle(action,node,C,u) {
  const l=local(C), detail=profile(C), name=action.slice('controls-'.length);
  if(name==='tab') {
    l.tab=Object.hasOwn(settingsTabs,node.dataset.tab)?node.dataset.tab:'general';
    if(l.tab==='controls'&&!l.loading) {
      const pending=refresh(C,u);
      u.renderPage();
      await pending;
    } else u.renderPage();
    return;
  }
  if(!active(C)) return;
  if(name==='refresh') {
    const pending=refresh(C,u);
    u.renderPage();
    await pending;
    return;
  }
  if(name==='import'||name==='export') {
    if(l.busy||name==='export'&&!detail) return;
    if(name==='import') requireClosed(C);
    l.busy=true;
    node.disabled=true;
    try {
      const result=await u.call('native',{action:'controls-'+name,...(name==='export'?{profileId:detail.id,filename:detail.file}:{})});
      if(result?.cancelled) return;
      if(name==='import') {
        if(result?.profiles) {
          l.data=result;
          setGameRunning(C,u,result.gameRunning);
        }
        else await refresh(C,u,result?.profileId);
      }
      u.toast(t(name==='import'?'Профиль управления импортирован':'Профиль управления экспортирован'));
    } finally {
      l.busy=false;
      node.disabled=false;
      if(active(C)) u.renderPage();
    }
    return;
  }
  if(name==='backup') return mutate(C,u,{action:'backup'},'Копия управления создана',false);
  if(name==='restore') {
    requireClosed(C);
    const row=l.data?.backups?.find(item=>item.id===node.dataset.backup);
    if(!row) throw new Error(t('Резервная копия недоступна.'));
    return u.dialog(t('Восстановить назначения?'),
      `<p>${th(u,'Профили и активные назначения из копии будут восстановлены. Текущее состояние будет сохранено отдельно.')}</p>`,
      u.btn(t('Отмена'),'close-dialog','','quiet')+
      button(u,'Восстановить','apply-restore','clock','primary',`data-backup="${u.esc(row.id)}"`));
  }
  if(name==='apply-restore') return mutate(C,u,{action:'restore',backupId:node.dataset.backup},'Настройки управления восстановлены');
  if(!detail) return;
  requireClosed(C);
  if(name==='clone') {
    return u.dialog(t('Дублировать профиль управления'),`<form id="controls-clone-form" class="controls-binding-form">
      <label class="controls-field"><span>${th(u,'Имя нового профиля')}</span><input id="controls-clone-name" autocomplete="off" maxlength="48"
        value="${u.esc((detail.name+' '+t('Копия')).slice(0,48))}" required></label></form>`,
      u.btn(t('Отмена'),'close-dialog','','quiet')+button(u,'Создать профиль','apply-clone','plus','primary'));
  }
  if(name==='apply-clone') {
    const value=u.$('#controls-clone-name')?.value.trim();
    if(!value) throw new Error(t('Введите имя нового профиля.'));
    return mutate(C,u,{action:'clone',profileId:detail.id,expectedRevision:detail.revision,name:value},'Профиль управления создан');
  }
  if(name==='activate') {
    return u.dialog(t('Назначить профиль'),`<form id="controls-activate-form" class="controls-binding-form">
      <label class="controls-field"><span>${th(u,'Контекст управления')}</span><select id="controls-activate-context">
        ${[['ship',contextLabels.ship],...Object.entries(contextLabels).filter(([key])=>key!=='ship'),['all','Все контексты']]
          .map(([context,label])=>`<option value="${context}">${th(u,label)}</option>`).join('')}
      </select></label></form>`,
      u.btn(t('Отмена'),'close-dialog','','quiet')+button(u,'Применить','apply-activate','check','primary'));
  }
  if(name==='apply-activate') {
    const context=u.$('#controls-activate-context')?.value;
    if(!context) return;
    return mutate(C,u,{action:'activate',profileId:detail.id,context,expectedRevision:detail.revision},'Активный профиль изменён');
  }
  if(detail.readOnly) return;
  if(name==='capture-retry'&&l.edit) return startCapture(C,u,l.edit);
  if(name==='edit') {
    const binding=detail.actions?.find(item=>item.id===node.dataset.action);
    const slot=binding?.slots?.find(item=>item.id===node.dataset.slot);
    if(binding&&slot) return editor(C,u,detail,binding,slot);
    return;
  }
  if(name==='save-binding'||name==='clear-binding') {
    const edit=l.edit, binding=detail.actions?.find(item=>item.id===edit?.action);
    if(!binding||edit.profileId!==detail.id||edit.revision!==detail.revision)
      throw new Error(t('Профиль обновился. Откройте назначение заново.'));
    if(name==='save-binding'&&!edit.candidate&&!edit.tuningChanged) return;
    const change=collectEdit(u,edit,name==='clear-binding');
    const collisions=matchingBindings(detail,binding,edit.slot,change);
    const signature=canonical(change);
    if(collisions.length&&l.confirmedConflict!==signature) {
      l.confirmedConflict=signature;
      showEditNotice(u,t('Возможное пересечение с действиями {list}. Нажмите «Сохранить назначение» ещё раз, чтобы применить.',{list:collisions.map(labelFor).join(', ')}));
      return;
    }
    cancelCapture(u,edit);
    edit.waiting=false;
    return mutate(C,u,{action:'edit',profileId:detail.id,expectedRevision:edit.revision,changes:[change]},'Назначение сохранено');
  }
}
const listeners=new WeakMap();
function init(C,u) {
  if(!active(C)) return;
  const l=local(C);
  if(!l.loading&&!l.data&&!l.error) void refresh(C,u);
  pollStatus(C,u);
  const host=u.$('[data-controls-workspace]');
  if(!host||listeners.has(host)) return;
  listeners.set(host,true);
  const repaint=()=>{const list=u.$('#controls-action-list');if(list) list.innerHTML=renderActions(C,u);};
  host.addEventListener('input',event=>{
    if(event.target.id==='controls-query'){l.query=event.target.value;repaint();}
  });
  host.addEventListener('change',event=>{
    if(event.target.id==='controls-context'){l.context=event.target.value;repaint();}
    if(event.target.id==='controls-profile'&&event.target.value){
      l.query='';
      const pending=refresh(C,u,event.target.value);
      u.renderPage();
      void pending;
    }
  });
  host.addEventListener('toggle',event=>{
    if(event.target.matches?.('.controls-backups')) l.backupsOpen=event.target.open;
  },true);
  if(!init.bound&&typeof document!=='undefined') {
    init.bound=true;
    document.addEventListener('input',event=>{
      if(!l.edit||!event.target.closest?.('.controls-axis-options')) return;
      l.edit.tuningChanged=true;
      const save=u.$('#controls-binding-save');
      if(save) save.disabled=running(C);
    });
    document.addEventListener('keydown',event=>captureInput(C,u,event,'keyboard',false),true);
    document.addEventListener('keyup',event=>captureInput(C,u,event,'keyboard',true),true);
    document.addEventListener('pointerdown',event=>captureInput(C,u,event,'mouse',false),true);
    document.addEventListener('pointerup',event=>captureInput(C,u,event,'mouse',true),true);
    document.addEventListener('contextmenu',event=>{
      if(l.edit?.waiting&&u.$('#controls-capture-focus')?.contains(event.target)) event.preventDefault();
    });
    document.addEventListener('focusout',event=>{
      if(event.target.id!=='controls-capture-focus'||!l.edit) return;
      l.edit.pressed.clear();
      l.edit.blocked.clear();
    });
    document.addEventListener('submit',event=>{
      const actions={'controls-binding-form':'save-binding','controls-clone-form':'apply-clone','controls-activate-form':'apply-activate'};
      const action=actions[event.target.id];
      if(!action||!active(C)) return;
      event.preventDefault();
      handle('controls-'+action,null,C,u).catch(error=>u.toast(t(error.message),true));
    });
  }
}
window.ControlsUI={active,tabs,render,init,handle,layerClosed};
})();
