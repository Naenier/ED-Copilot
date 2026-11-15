(()=>{
'use strict';
const t=(key,vars)=>window.CopilotI18n?.t(key,vars)??String(key??'');
const th=(u,key,vars)=>u.esc(t(key,vars));
const contextLabels={general:'Общее управление',ship:'Корабль',srv:'ТРП',onfoot:'Пешком'};
const filterLabels={...contextLabels,camera:'Камера',scanner:'Сканеры'};
const keyLabels={
  GamePad_FaceDown:'A',GamePad_FaceRight:'B',GamePad_FaceLeft:'X',GamePad_FaceUp:'Y',
  GamePad_DPadUp:'D-pad ↑',GamePad_DPadDown:'D-pad ↓',GamePad_DPadLeft:'D-pad ←',GamePad_DPadRight:'D-pad →',
  GamePad_LBumper:'LB',GamePad_RBumper:'RB',GamePad_LTrigger:'LT',GamePad_RTrigger:'RT',
  Pos_GamePad_LTrigger:'LT',Pos_GamePad_RTrigger:'RT',
  GamePad_LThumb:'LS',GamePad_RThumb:'RS',GamePad_Start:'Menu',GamePad_Back:'View',
  GamePad_LStickX:'LS X',GamePad_LStickY:'LS Y',GamePad_RStickX:'RS X',GamePad_RStickY:'RS Y',
  Key_LeftShift:'L Shift',Key_RightShift:'R Shift',Key_LeftControl:'L Ctrl',Key_RightControl:'R Ctrl',
  Key_LeftAlt:'L Alt',Key_RightAlt:'R Alt',Key_Space:'Space',Key_Enter:'Enter',Key_Escape:'Esc',
  Key_UpArrow:'↑',Key_DownArrow:'↓',Key_LeftArrow:'←',Key_RightArrow:'→',
  Mouse_1:'ЛКМ',Mouse_2:'ПКМ',Mouse_3:'СКМ'
};
const controllerButtons=[
  'GamePad_FaceDown','GamePad_FaceRight','GamePad_FaceLeft','GamePad_FaceUp',
  'GamePad_DPadUp','GamePad_DPadDown','GamePad_DPadLeft','GamePad_DPadRight',
  'GamePad_LBumper','GamePad_RBumper','Pos_GamePad_LTrigger','Pos_GamePad_RTrigger',
  'GamePad_LThumb','GamePad_RThumb','GamePad_Back','GamePad_Start'
];
const controllerAxes=['GamePad_LStickX','GamePad_LStickY','GamePad_RStickX','GamePad_RStickY','GamePad_LTrigger','GamePad_RTrigger'];
const keyboardKeys=[...'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'].map(char=>'Key_'+char).concat(
  Array.from({length:12},(_,index)=>'Key_F'+(index+1)),Object.keys(keyLabels).filter(key=>key.startsWith('Key_')));
const actionLabels={
  YawAxisRaw:'Рыскание',RollAxisRaw:'Крен',PitchAxisRaw:'Тангаж',LateralThrustRaw:'Боковая тяга',
  VerticalThrustRaw:'Вертикальная тяга',AheadThrust:'Продольная тяга',ThrottleAxis:'Ось газа',
  LeftThrustButton:'Тяга влево',RightThrustButton:'Тяга вправо',UpThrustButton:'Тяга вверх',DownThrustButton:'Тяга вниз',
  ForwardThrustButton:'Тяга вперёд',BackwardThrustButton:'Тяга назад',ForwardKey:'Увеличить тягу',BackwardKey:'Уменьшить тягу',
  ToggleFlightAssist:'Помощь в полёте',UseBoostJuice:'Форсаж',HyperSuperCombination:'Рамочно-сдвиговый двигатель',
  Supercruise:'Суперкруиз',Hyperspace:'Гиперпрыжок',OrbitLinesToggle:'Линии орбит',ToggleReverseThrottleInput:'Реверс тяги',
  SetSpeedZero:'Нулевая тяга',PrimaryFire:'Основной огонь',SecondaryFire:'Дополнительный огонь',
  DeployHardpointToggle:'Выпустить или убрать оружие',CycleFireGroupNext:'Следующая огневая группа',
  CycleFireGroupPrevious:'Предыдущая огневая группа',DeployHeatSink:'Сброс теплоотвода',UseShieldCell:'Батарея щита',
  FireChaffLauncher:'Дипольные отражатели',ChargeECM:'Электронное противодействие',ToggleCargoScoop:'Грузовой ковш',
  LandingGearToggle:'Посадочное шасси',EjectAllCargo:'Сбросить весь груз',ShipSpotLightToggle:'Фары корабля',
  NightVisionToggle:'Ночное видение',SelectTarget:'Выбрать цель',CycleNextTarget:'Следующая цель',
  CyclePreviousTarget:'Предыдущая цель',SelectHighestThreat:'Наибольшая угроза',
  CycleNextHostileTarget:'Следующая враждебная цель',CyclePreviousHostileTarget:'Предыдущая враждебная цель',
  TargetNextRouteSystem:'Следующая система маршрута',IncreaseEnginesPower:'Мощность двигателей',
  IncreaseWeaponsPower:'Мощность оружия',IncreaseSystemsPower:'Мощность систем',
  ResetPowerDistribution:'Сброс распределения мощности',UIFocus:'Фокус интерфейса',FocusLeftPanel:'Левая панель',
  FocusRightPanel:'Правая панель',FocusCommsPanel:'Панель связи',FocusRadarPanel:'Нижняя панель',
  QuickCommsPanel:'Быстрое сообщение',GalaxyMapOpen:'Карта галактики',SystemMapOpen:'Карта системы',
  HeadLookToggle:'Осмотр кабины',Pause:'Пауза',FriendsMenu:'Меню друзей',OpenCodexGoToDiscovery:'Кодекс',
  PlayerHUDModeToggle:'Переключить режим интерфейса',ExplorationFSSEnter:'Открыть ПСС',
  UI_Up:'Меню вверх',UI_Down:'Меню вниз',UI_Left:'Меню влево',UI_Right:'Меню вправо',UI_Select:'Подтвердить',UI_Back:'Назад',
  UI_Toggle:'Переключить элемент меню',CycleNextPanel:'Следующая вкладка',CyclePreviousPanel:'Предыдущая вкладка',
  CycleNextPage:'Следующая страница',CyclePreviousPage:'Предыдущая страница',
  ToggleDriveAssist:'Помощь при вождении',SteeringAxis:'Рулевое управление',SteerLeftButton:'Повернуть налево',
  SteerRightButton:'Повернуть направо',DriveSpeedAxis:'Ось скорости ТРП',AutoBreakBuggyButton:'Стояночный тормоз',
  VerticalThrustersButton:'Вертикальные двигатели ТРП',BuggyPrimaryFireButton:'Основной огонь ТРП',
  BuggySecondaryFireButton:'Дополнительный огонь ТРП',HeadlightsBuggyButton:'Фары ТРП',ToggleBuggyTurretButton:'Турель ТРП',
  RecallDismissShip:'Вызвать или отпустить корабль',SelectTarget_Buggy:'Выбрать цель с ТРП',
  HumanoidForwardAxis:'Движение вперёд и назад',HumanoidForwardButton:'Шаг вперёд',HumanoidBackwardButton:'Шаг назад',
  HumanoidStrafeAxis:'Движение в стороны',HumanoidStrafeLeftButton:'Шаг влево',HumanoidStrafeRightButton:'Шаг вправо',
  HumanoidRotateAxis:'Горизонтальный обзор',HumanoidPitchAxis:'Вертикальный обзор',HumanoidSprintButton:'Бег',
  HumanoidWalkButton:'Ходьба',HumanoidCrouchButton:'Присесть',HumanoidJumpButton:'Прыгнуть',
  HumanoidPrimaryInteractButton:'Основное взаимодействие',HumanoidSecondaryInteractButton:'Вторичное взаимодействие',
  HumanoidItemWheelButton:'Колесо предметов',HumanoidEmoteWheelButton:'Колесо эмоций',
  HumanoidPrimaryFireButton:'Стрельба',HumanoidZoomButton:'Прицелиться',HumanoidThrowGrenadeButton:'Бросить гранату',
  HumanoidMeleeButton:'Ближний бой',HumanoidReloadButton:'Перезарядить',HumanoidSwitchWeapon:'Сменить оружие',
  HumanoidHideWeaponButton:'Убрать оружие',HumanoidToggleFlashlightButton:'Фонарик',
  HumanoidToggleNightVisionButton:'Ночное видение костюма',HumanoidToggleShieldsButton:'Щит костюма',
  HumanoidHealthPack:'Аптечка',HumanoidBattery:'Батарея костюма',HumanoidSwitchToRechargeTool:'Энергосвязь',
  HumanoidSwitchToCompAnalyser:'Анализатор состава',HumanoidSwitchToSuitTool:'Инструмент костюма',
  HumanoidToggleToolModeButton:'Режим инструмента',HumanoidOpenAccessPanelButton:'Панель доступа',
  PhotoCameraToggle:'Фоторежим',ToggleFreeCam:'Свободная камера',FreeCamToggleHUD:'Интерфейс камеры',
  QuitCamera:'Закрыть камеру',ExplorationFSSDiscoveryScan:'Сканирование системы',ExplorationFSSQuit:'Выйти из ПСС',
  ExplorationFSSRadioTuningX_Raw:'Настройка частоты ПСС',ExplorationFSSZoomIn:'Приблизить в ПСС',
  ExplorationFSSZoomOut:'Отдалить в ПСС',ExplorationSAAChangeScannedAreaViewToggle:'Сменить карту поверхности',
  ExplorationSAAExitThirdPerson:'Выйти из картографирования'
};
const slotLabels={primary:'Основное назначение',secondary:'Дополнительное назначение',axis:'Ось'};
const button=(u,label,action,icon='',style='quiet',attrs='')=>u.btn(t(label),'controls-'+action,icon,style,attrs);
const profile=C=>C.controlsUI?.data?.profile;
function local(C) {
  return C.controlsUI??={tab:'general',query:'',context:'',data:null,loading:false,busy:false,error:''};
}
function active(C) {return C?.tab==='settings'&&local(C).tab==='controls';}
function tabTitle() {return window.CopilotI18n?.language==='en'?'Controls':'Управление';}
function tabs(C,u) {
  return `<nav class="segmented controls-settings-tabs" aria-label="${th(u,'Разделы настроек')}">
    ${['general','controls'].map(id=>`<button type="button" data-click="controls-tab" data-tab="${id}"
      class="${local(C).tab===id?'selected':''}"${local(C).tab===id?' aria-current="page"':''}>${id==='controls'?u.esc(tabTitle()):th(u,'Общие настройки')}</button>`).join('')}
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
  const english=humanize(action.id);
  if(window.CopilotI18n?.language==='en') return english;
  const base=action.id.replace(/_(?:Humanoid|Buggy)$/,'');
  return actionLabels[action.id]||actionLabels[base]||english;
}
const keyName=key=>t(keyLabels[key]||String(key||'').replace(/^Key_/,''));
function devices(detail) {
  const list=new Map([['Keyboard',t('Клавиатура')],['Mouse',t('Мышь')]]);
  for(const action of detail?.actions||[]) for(const slot of action.slots||[]) {
    for(const item of [slot,...(slot.modifiers||[])]) {
      if(!item.device||item.device==='{NoDevice}') continue;
      if(/GamePad_/i.test(item.key)) list.set(item.device,`${t('Геймпад')} · ${item.device}`);
      else if(!list.has(item.device)) list.set(item.device,item.device);
    }
  }
  return list;
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
    <kbd title="${u.esc(item.device+' · '+item.key)}">${u.esc(keyName(item.key))}</kbd>`).join('')}</span>`;
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
      <div class="controls-action-name"><strong>${u.esc(labelFor(action))}</strong><code>${u.esc(action.id)}</code></div>
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
function pairInputs(u,label,pair,listID,devicesID) {
  return `<div class="controls-pair">
    <label class="controls-field"><span>${th(u,'Устройство')}</span><input type="text" data-control-role="device" list="${devicesID}"
      value="${u.esc(pair.device==='{NoDevice}'?'':pair.device||'')}" maxlength="80" autocomplete="off"></label>
    <label class="controls-field"><span>${th(u,label)}</span><input type="text" data-control-role="key" list="${listID}"
      value="${u.esc(pair.key||'')}" maxlength="100" autocomplete="off"></label>
  </div>`;
}
function datalist(id,rows,u) {
  return `<datalist id="${id}">${rows.map(([value,label])=>`<option value="${u.esc(value)}" label="${u.esc(label)}"></option>`).join('')}</datalist>`;
}
function editor(C,u,detail,action,slot) {
  const l=local(C), axis=slot.id==='axis', devs=devices(detail);
  const controller=[...devs].find(([id,label])=>id===slot.device&&label.startsWith(t('Геймпад')+' · '))||
    [...devs].find(([,label])=>label.startsWith(t('Геймпад')+' · '));
  const mods=slot.modifiers||[];
  const edit={profileId:detail.id,revision:detail.revision,action:action.id,slot:slot.id};
  l.edit=edit;
  l.keyTarget=null;
  const observed=new Set((detail.actions||[]).flatMap(item=>(item.slots||[]).flatMap(row=>[
    row.key,...(row.modifiers||[]).map(mod=>mod.key)
  ])).filter(Boolean));
  const keyOptions=keys=>[...new Set([...keys,...observed])].map(key=>[key,keyName(key)]);
  const deviceList=datalist('controls-device-options',[...devs],u);
  const keyList=datalist('controls-key-options',keyOptions([
    ...controllerButtons,...(axis?controllerAxes:[]),...keyboardKeys,...Object.keys(keyLabels).filter(key=>key.startsWith('Mouse_'))
  ]),u);
  const shortcut=(key)=>`<button type="button" data-click="controls-fill-key" data-device="${u.esc(controller?.[0]||'')}"
    data-key="${u.esc(key)}" title="${u.esc(key)}">${u.esc(keyName(key))}</button>`;
  const row=(item,index)=>`<div class="controls-modifier" data-controls-modifier>
    ${pairInputs(u,t('Клавиша модификатора {number}',{number:index+1}),item,'controls-key-options','controls-device-options')}
    ${button(u,'','remove-modifier','x','quiet',`title="${th(u,'Удалить модификатор')}" aria-label="${th(u,'Удалить модификатор')}"`)}
  </div>`;
  const body=`<form id="controls-binding-form" class="controls-binding-form">
    <p class="controls-edit-caption"><code>${u.esc(action.id)}</code> · ${th(u,slotLabels[slot.id]||slot.id)}</p>
    ${deviceList}${keyList}
    <div class="controls-edit-pair" data-controls-main>${pairInputs(u,axis?'Код оси':'Кнопка или клавиша',slot,'controls-key-options','controls-device-options')}</div>
    ${controller?`<section class="controls-quick-keys">
      <div><strong>${th(u,axis?'Оси геймпада':'Кнопки геймпада')}</strong><code>${u.esc(controller[0])}</code></div>
      <div>${(axis?controllerAxes:controllerButtons).map(shortcut).join('')}</div>
    </section>`:''}
    ${axis?`<div class="controls-axis">
      ${typeof action.inverted==='boolean'?`<label><input type="checkbox" id="controls-invert"${action.inverted?' checked':''}> ${th(u,'Инвертировать ось')}</label>`:''}
      ${action.deadzone!=null?`<label class="controls-field"><span>${th(u,'Мёртвая зона, 0–1')}</span><input id="controls-deadzone" type="text" inputmode="decimal"
        value="${u.esc(action.deadzone)}" maxlength="18"></label>`:''}</div>`:
      typeof action.toggle==='boolean'?`<label class="controls-toggle"><input id="controls-toggle" type="checkbox"${action.toggle?' checked':''}> ${th(u,'Переключатель вместо удержания')}</label>`:''}
    ${!axis&&typeof slot.hold==='boolean'?`<label class="controls-toggle"><input id="controls-hold" type="checkbox"${slot.hold?' checked':''}> ${th(u,'Удерживать кнопку')}</label>`:''}
    <section class="controls-modifiers"><header><h3>${th(u,'Модификаторы')}</h3>${button(u,'Добавить модификатор','add-modifier','plus','quiet')}</header>
        <div id="controls-modifiers">${mods.map(row).join('')}</div>
        <template id="controls-modifier-template">${row({},0)}</template>
    </section>
    <div id="controls-edit-notice" class="notice" role="status" hidden></div>
  </form>`;
  const footer=button(u,'Очистить назначение','clear-binding','x','quiet',notSet(slot)?'disabled':'')+
    '<span class="spacer"></span>'+u.btn(t('Отмена'),'close-dialog','','quiet')+
    button(u,'Сохранить назначение','save-binding','check','primary');
  u.dialog(labelFor(action),body,footer,true);
}
function readPair(host) {
  return {device:host?.querySelector('[data-control-role="device"]')?.value.trim()||'',
    key:host?.querySelector('[data-control-role="key"]')?.value.trim()||''};
}
function showEditNotice(u,text) {
  const node=u.$('#controls-edit-notice');
  if(node) {node.hidden=!text;node.textContent=text||'';}
}
function collectEdit(u,edit,clear=false) {
  const form=u.$('#controls-binding-form');
  if(!form) throw new Error(t('Откройте назначение заново.'));
  const pair=clear?{device:'',key:''}:readPair(form.querySelector('[data-controls-main]'));
  if(!!pair.device!==!!pair.key) throw new Error(t('Укажите устройство и клавишу вместе.'));
  const change={action:edit.action,slot:edit.slot,...pair};
  if(!clear&&edit.slot==='axis') {
    const raw=u.$('#controls-deadzone')?.value.trim().replace(',','.');
    const invert=u.$('#controls-invert');
    if(invert) change.inverted=invert.checked;
    if(raw) {
      if(!/^(?:\d+(?:\.\d+)?|\.\d+)$/.test(raw)||Number(raw)<0||Number(raw)>1)
        throw new Error(t('Мёртвая зона должна быть числом от 0 до 1.'));
      change.deadzone=Number(raw);
    }
  }
  const modifiers=clear||!pair.key?[]:[...form.querySelectorAll('[data-controls-modifier]')].map(readPair)
    .filter(item=>item.device||item.key);
  if(modifiers.some(item=>!item.key||!item.device)) throw new Error(t('Заполните устройство и клавишу каждого модификатора.'));
  const parts=[pair,...modifiers].filter(item=>item.device&&item.key).map(canonical);
  if(new Set(parts).size!==parts.length) throw new Error(t('Одна клавиша не может повторяться в сочетании.'));
  change.modifiers=modifiers;
  const toggle=u.$('#controls-toggle');
  if(toggle&&!clear) change.toggle=toggle.checked;
  const hold=u.$('#controls-hold');
  if(hold&&!clear) change.hold=hold.checked;
  return change;
}
function showCurrentConflicts(C,u) {
  const l=local(C), detail=profile(C), edit=l.edit;
  const action=detail?.actions?.find(item=>item.id===edit?.action);
  if(!action||!u.$('#controls-binding-form')) return;
  l.confirmedConflict='';
  let change;
  try {change=collectEdit(u,edit);} catch(_) {showEditNotice(u,'');return;}
  const potential=matchingBindings(detail,action,edit.slot,change);
  showEditNotice(u,potential.length?t('Возможное пересечение с действиями {list}.',{list:potential.map(labelFor).join(', ')}):'');
}
function setGameRunning(C,u,running) {
  if(typeof running!=='boolean') return;
  local(C).gameRunning=running;
  for(const action of ['save-binding','clear-binding','apply-clone','apply-activate','apply-restore']) {
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
    l.tab=node.dataset.tab==='controls'?'controls':'general';
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
  if(name==='fill-key') {
    const form=u.$('#controls-binding-form');
    const host=l.keyTarget&&form?.contains(l.keyTarget)?l.keyTarget:form?.querySelector('[data-controls-main]');
    if(!host||!node.dataset.device) return;
    host.querySelector('[data-control-role="device"]').value=node.dataset.device;
    host.querySelector('[data-control-role="key"]').value=node.dataset.key;
    showCurrentConflicts(C,u);
    return;
  }
  if(name==='add-modifier'||name==='remove-modifier') {
    const list=u.$('#controls-modifiers');
    if(!list) return;
    if(name==='remove-modifier') node.closest('[data-controls-modifier]')?.remove();
    else {
      if(list.children.length>=8) return;
      list.appendChild(u.$('#controls-modifier-template').content.cloneNode(true));
    }
    [...list.children].forEach((child,index)=>{
      const title=child.querySelector('[data-control-role="key"]')?.closest('label')?.querySelector('span');
      if(title) title.textContent=t('Клавиша модификатора {number}',{number:index+1});
    });
    list.lastElementChild?.querySelector('input')?.focus();
    showCurrentConflicts(C,u);
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
  if(name==='edit') {
    const binding=detail.actions?.find(item=>item.id===node.dataset.action);
    const slot=binding?.slots?.find(item=>item.id===node.dataset.slot);
    if(binding&&slot) {
      editor(C,u,detail,binding,slot);
      showCurrentConflicts(C,u);
    }
    return;
  }
  if(name==='save-binding'||name==='clear-binding') {
    const edit=l.edit, binding=detail.actions?.find(item=>item.id===edit?.action);
    if(!binding||edit.profileId!==detail.id||edit.revision!==detail.revision)
      throw new Error(t('Профиль обновился. Откройте назначение заново.'));
    const change=collectEdit(u,edit,name==='clear-binding');
    const collisions=matchingBindings(detail,binding,edit.slot,change);
    const signature=canonical(change);
    if(collisions.length&&l.confirmedConflict!==signature) {
      l.confirmedConflict=signature;
      showEditNotice(u,t('Возможное пересечение с действиями {list}. Нажмите «Сохранить назначение» ещё раз, чтобы применить.',{list:collisions.map(labelFor).join(', ')}));
      return;
    }
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
      if(active(C)&&event.target.closest?.('#controls-binding-form')) showCurrentConflicts(C,u);
    });
    document.addEventListener('focusin',event=>{
      if(active(C)&&event.target.dataset?.controlRole) l.keyTarget=event.target.closest('[data-controls-main],[data-controls-modifier]');
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
window.ControlsUI={active,tabs,render,init,handle};
})();
