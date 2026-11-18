(() => {
  'use strict';

  const mode = 'Конструктор кораблей';
  const active = C => C.tab === 'engineering' && C.mode === mode;
  const engine = () => window.EDCopilotShipFitEngine;
  const owner = C => C.state?.planWorkspace?.key || '';
  const h = (u, text, vars) => u.esc(u.t(text, vars));
  const sel = (current, value) => String(current ?? '') === String(value) ? ' selected' : '';
  const dis = unavailable => unavailable ? ' disabled' : '';
  const sort = u => new Intl.Collator(u.locale(), {numeric:true});
  const lower = value => String(value ?? '').toLowerCase();
  const russian = u => u.locale().startsWith('ru');
  const languageData = () => window.EdCopilotShipFitRU || {};
  const byID = (dict, id, name) => dict?.[id] || dict?.[lower(id)] || dict?.[name] || dict?.[lower(name)];
  const button = (u, action, title, {style='', icon='', value, disabled=false} = {}) =>
    '<button type="button" class="pill ' + style + '" data-fit-action="' + action + '"' +
    (value === undefined ? '' : ' data-fit-value="' + u.esc(value) + '"') + dis(disabled) + '>' +
    (icon ? u.icon(icon) : '') + h(u,title) + '</button>';
  const option = (u, value, title, current) => '<option value="' + u.esc(value) + '"' + sel(current,value) + '>' + u.esc(title) + '</option>';
  const label = (u, text, control) => '<label class="fit-field"><span>' + h(u,text) + '</span>' + control + '</label>';

  const groups = [
    ['core','Основные модули'], ['optional','Дополнительные модули'],
    ['hardpoint','Оружие'], ['utility','Вспомогательное оборудование'],
    ['armour','Броня'], ['hatch','Грузовой люк'],
  ];
  const materialGroups = [
    ['raw','Сырьё'], ['manufactured','Промышленные материалы'], ['encoded','Данные'],
  ];
  const coreNames = {
    PowerPlant:'Силовая установка', MainEngines:'Двигатели', FrameShiftDrive:'Двигатель FSD',
    LifeSupport:'Система жизнеобеспечения', PowerDistributor:'Распределитель питания', Radar:'Сенсоры', FuelTank:'Топливный бак',
  };
  const mountNames = {F:'Неподвижная установка', G:'Карданная установка', T:'Турельная установка'};
  const damageTypes = {kinetic:'Кинетический', thermal:'Термический', explosive:'Взрывной', caustic:'Каустический', absolute:'Абсолютный'};
  const statSections = [
    ['Полёт', [
      ['dryMass','Сухая масса',' т'], ['mass','Текущая масса',' т'], ['unladenMass','Масса без груза',' т'],
      ['ladenMass','Масса с полным трюмом',' т'], ['speed','Скорость',' м/с'], ['boostSpeed','Форсаж',' м/с'],
      ['unladenSpeed','Скорость без груза',' м/с'], ['ladenSpeed','Скорость с полным трюмом',' м/с'],
      ['unladenBoostSpeed','Форсаж без груза',' м/с'], ['ladenBoostSpeed','Форсаж с полным трюмом',' м/с'],
      ['pitch','Тангаж',' °/с'], ['roll','Крен',' °/с'], ['yaw','Рыскание',' °/с'], ['boostInterval','Интервал форсажа',' с'],
    ]],
    ['Прыжки', [
      ['jumpRange','Текущий прыжок',' св. л.'], ['maximumRange','Максимальный прыжок',' св. л.'],
      ['unladenRange','Прыжок без груза',' св. л.'], ['ladenRange','Прыжок с полным трюмом',' св. л.'],
      ['totalRange','Дальность на запасе топлива',' св. л.'], ['cargoCapacity','Вместимость трюма',' т'],
      ['fuelCapacity','Объём топливных баков',' т'], ['reserveFuel','Резервное топливо',' т'],
      ['passengerCapacity','Пассажирские места',''],
    ]],
    ['Энергия', [
      ['powerAvailable','Мощность реактора',' МВт'], ['powerRetracted','С убранным оружием',' МВт'],
      ['powerDeployed','С развёрнутым оружием',' МВт'], ['thermalEfficiency','Теплоэффективность',''],
      ['heatCapacity','Теплоёмкость',''], ['eps','Энергия на стрельбу в секунду',' МДж'],
      ['hps','Тепло при стрельбе в секунду',''], ['weaponDrainTime','Время до разрядки оружия',' с'],
    ]],
    ['Бой', [
      ['shield','Прочность щита',' МДж'], ['armour','Прочность корпуса',''], ['hardness','Твёрдость корпуса',''],
      ['moduleArmour','Прочность модулей',''],
      ['moduleProtection','Защита модулей',''], ['shieldRecoveryTime','Восстановление после сбития щита',' с'],
      ['shieldRegenTime','Зарядка щита',' с'], ['dps','Урон в секунду',''], ['sustainedDps','Урон в секунду с перезарядкой',''],
    ]],
  ];

  function model(C) {
    if (!C.shipFitUI || C.shipFitUI.owner !== owner(C)) {
      C.shipFitUI = {owner:owner(C), fits:[], cfg:{}, expanded:new Set(['Полёт']), importText:'', query:'', limit:30};
    }
    return C.shipFitUI;
  }

  function shipName(u, id, raw) {
    return (russian(u) && byID(languageData().ships,id,raw)) || raw || engine()?.ships().find(ship => ship.id === id)?.name || id;
  }
  function moduleName(u, mod) {
    if (!mod) return u.t('Пусто');
    if (!russian(u)) return mod.name;
    const db = languageData();
    return db.variants?.[mod.id] || byID(db.modules,mod.symbol) || byID(db.moduleIds,mod.edId) ||
      byID(db.names,mod.name) || byID(db.groups,mod.group,mod.groupName) || u.t('Неизвестный модуль');
  }
  function blueprintName(u, item) {
    return (russian(u) && byID(languageData().blueprints,item.symbol || item.id,item.name)) || item.name;
  }
  function experimentalName(u, item) {
    return (russian(u) && byID(languageData().experimentals,item.id,item.name)) || item.name;
  }
  function buildName(u, build) {
    const stock = engine().ships().find(ship => ship.id === build.shipId)?.name;
    return !build.name || build.name === stock ? shipName(u,build.shipId,stock) : build.name;
  }
  function slotName(u, slot, index) {
    const named = {Military:'Военный отсек',Limpets:'Отсек для контроллеров дронов',Fighter:'Отсек для истребителей',Cargo:'Грузовой отсек',PlanetaryApproachSuite:'Посадочное оборудование'};
    if (named[slot.name]) return u.t(named[slot.name]);
    return {
      core: () => u.t(coreNames[slot.key] || slot.name),
      armour: () => u.t('Броня'), hatch: () => u.t('Грузовой люк'),
    }[slot.kind]?.() || u.t('{name} {index}', {
      name:u.t({optional:'Отсек',hardpoint:'Точка подвески',utility:'Крепление'}[slot.kind]), index:u.fmt(index+1),
    });
  }
  function className(u, mod) {
    return [mod.size == null ? '' : String(mod.size) + (mod.rating || ''), mod.mount && u.t(mountNames[mod.mount])].filter(Boolean).join(' · ');
  }
  function fitState(r) {
    if (r.calculated?.build !== r.build || r.calculated?.cfg !== r.cfg) {
      r.calculated = {build:r.build, cfg:r.cfg, slots:engine().slots(r.build), stats:engine().stats(r.build,r.cfg)};
    }
    return r.calculated;
  }
  function value(u, number, unit='') {
    return number == null || !Number.isFinite(Number(number)) ? '—' : u.esc(u.fmt(number)) + h(u,unit);
  }
  function metrics(u,s) {
    const rows = [
      ['Текущий прыжок',s.jumpRange,' св. л.','Без груза',s.unladenRange,' св. л.'],
      ['Скорость',s.speed,' м/с','Форсаж',s.boostSpeed,' м/с'],
      ['Вместимость трюма',s.cargoCapacity,' т','Текущая масса',s.mass,' т'],
      ['Стоимость сборки',s.cost,' кр.','Страховка',s.rebuy,' кр.'],
    ];
    return rows.map(([title,n,unit,detail,count,otherUnit]) =>
      '<div class="fit-metric"><span>' + h(u,title) + '</span><strong>' + value(u,n,unit) + '</strong>' +
      (detail ? '<small>' + h(u,detail) + ' ' + value(u,count,otherUnit) + '</small>' : '') + '</div>').join('');
  }

  const toolbar = (r,u) => {
    const fits = [...r.fits].sort((a,b) => sort(u).compare(a.name,b.name));
    return '<div class="fit-toolbar"><div class="fit-actions">' +
      '<select id="fit-library" aria-label="' + h(u,'Сохранённые сборки') + '"' + dis(!fits.length || r.busy) + '>' +
      option(u,'',u.t('Сохранённые сборки'),r.id) +
      fits.map(fit => option(u,fit.id,[fit.name,shipName(u,fit.ship)].filter((text,index) => index===0 || text!==fit.name).join(' · '),r.id)).join('') +
      '</select>' + (r.build ? button(u,'new','Новая сборка',{icon:'plus',disabled:r.busy}) : '') +
      button(u,'current','Текущий корабль',{icon:'ship',disabled:r.busy}) + '</div><div class="fit-actions">' +
      button(u,'import','Импорт',{disabled:r.busy}) +
      (fits.length ? button(u,'export-library','Экспорт библиотеки',{disabled:r.busy}) : '') + '</div></div>';
  };
  function newForm(r,u) {
    if (!r.showNew && r.build) return '';
    const ships = [...engine().ships()].sort((a,b) => sort(u).compare(shipName(u,a.id,a.name),shipName(u,b.id,b.name)));
    return '<form id="fit-new-form" class="panel fit-box fit-new"><div><h2>' + h(u,'Выберите корабль') + '</h2>' +
      '<select id="fit-new-ship" name="ship" required aria-label="' + h(u,'Корабль') + '">' + option(u,'',u.t('Выберите корабль'),r.newShip) +
      ships.map(ship => option(u,ship.id,shipName(u,ship.id,ship.name),r.newShip)).join('') + '</select></div>' +
      '<button type="submit" class="pill primary"' + dis(r.busy) + '>' + h(u,'Создать сборку') + '</button>' +
      (r.build ? button(u,'new','Отмена') : '') + '</form>';
  }
  function importForm(r,u) {
    if (!r.showImport) return '';
    return '<section class="panel fit-box fit-import"><header><h2>' + h(u,'Импорт сборки') + '</h2>' +
      button(u,'import','Закрыть',{style:'quiet',disabled:r.busy}) + '</header>' +
      label(u,'Файл JSON','<input type="file" id="fit-import-file" accept=".json,application/json"' + dis(r.busy) + '>') +
      label(u,'JSON или ссылка Coriolis','<textarea id="fit-import-text" rows="4" maxlength="2000000" spellcheck="false"' +
        dis(r.busy) + '>' + u.esc(r.importText) + '</textarea>') +
      '<div class="fit-actions">' + button(u,'preview','Проверить импорт',{disabled:r.busy}) + '</div>' +
      (r.imported?.length ? '<div id="fit-import-selection" class="fit-actions">' +
        label(u,'Сборка из импорта','<select id="fit-import-choice">' + r.imported.map((build,i) =>
          option(u,i,buildName(u,build),r.importIndex)).join('') + '</select>') +
        button(u,'open-import','Открыть в конструкторе',{disabled:r.busy}) +
        button(u,'save-import','Сохранить все в библиотеку',{style:'primary',disabled:r.busy}) + '</div>' : '') + '</section>';
  }
  function feedback(r,u) {
    return (r.error ? '<p class="fit-alert" role="alert">' + u.esc(r.error) + '</p>' : '') +
      (r.confirm ? '<div class="fit-confirm"><span>' + h(u,r.confirm.title) + '</span>' +
        button(u,'confirm',r.confirm.button,{style:'accent',disabled:r.busy}) + button(u,'cancel','Отмена') + '</div>' : '');
  }

  function slotEngineering(u,r,slot) {
    const current = slot.engineering;
    if (!current?.blueprint) return '';
    const bp = engine().blueprints(r.build,slot.key).find(item => (item.symbol || item.id) === current.blueprint);
    if (!bp) return '';
    const exp = bp.experimentals?.find(item => item.id === current.experimental);
    return [blueprintName(u,bp),'G'+current.grade,exp && experimentalName(u,exp)].filter(Boolean).join(' · ');
  }
  function slotGroups(r,u,slots) {
    return groups.map(([kind,title]) => {
      const rows = slots.filter(slot => slot.kind === kind);
      if (!rows.length) return '';
      return '<section class="panel fit-box fit-slots"><h2>' + h(u,title) + '</h2><ul>' +
        rows.map((slot,index) => {
          const mod = slot.module;
          const details = mod && [className(u,mod),slotEngineering(u,r,slot),mod.preEngineered && u.t('Модуль с заводской доработкой')].filter(Boolean).join(' · ');
          return '<li class="fit-slot"><div class="fit-slot-label"><span>' + u.esc(slotName(u,slot,index)) + '</span>' +
            (slot.size != null && !['armour','hatch'].includes(slot.kind) ? '<small>' + h(u,'Класс {size}',{size:slot.size}) + '</small>' : '') +
            '</div><button type="button" class="fit-slot-module" data-fit-action="edit-slot" data-fit-value="' + u.esc(slot.key) +
            '" aria-expanded="' + (r.slotKey === slot.key) + '" aria-controls="fit-inspector"><span><strong>' +
            u.esc(moduleName(u,mod)) + '</strong>' + (details ? '<small>' + u.esc(details) + '</small>' : '') +
            '</span>' + u.icon('chevron-right') + '</button>' +
            (mod ? '<div class="fit-slot-context">' + (mod.power > 0 ? '<span>' + value(u,mod.power,' МВт') + '</span>' : '') +
              (slot.on === false ? '<small>' + h(u,'Выключено') + '</small>' :
                slot.retracted === false || slot.deployed === false ? '<small class="fit-unpowered">' +
                h(u,slot.retracted === false ? 'Нет питания при убранном оружии' : 'Нет питания при развёрнутом оружии') + '</small>' : '') + '</div>' : '') + '</li>';
        }).join('') + '</ul></section>';
    }).join('');
  }

  function conditions(r,u,s) {
    const numeric = (name,title,n,max,step,type) => label(u,title,
      '<input id="fit-'+name+'" data-fit-'+type+'="'+name+'" type="number" min="0" max="' + u.esc(max || 0) +
      '" step="'+step+'" value="' + u.esc(n ?? 0) + '"' + dis(!max) + '>');
    return '<section class="panel fit-box fit-conditions"><h2>' + h(u,'Условия расчёта') + '</h2><div class="fit-fields">' +
      numeric('cargo','Груз, т',s.cargo,s.cargoCapacity,0.1,'condition') +
      numeric('fuel','Топливо, т',s.fuel,s.fuelCapacity,0.1,'condition') + '</div><div class="fit-actions">' +
      button(u,'fill-cargo','Полный трюм',{disabled:!s.cargoCapacity}) +
      button(u,'empty-cargo','Пустой трюм',{disabled:!s.cargoCapacity}) +
      button(u,'fill-fuel','Полный бак',{disabled:!s.fuelCapacity}) + '</div><fieldset><legend>' +
      h(u,'Распределитель питания · 6 делений') + '</legend><div class="fit-fields three">' +
      [['sys','Системы'],['eng','Двигатели'],['wep','Оружие']].map(([name,title]) =>
        numeric(name,title,r.cfg[name] ?? 2,4,0.5,'pip')).join('') + '</div></fieldset></section>';
  }
  function powerChart(u,s) {
    if (!(s.powerAvailable > 0)) return '';
    const bar = (title,n) => {
      const percent = 100 * n / s.powerAvailable;
      return '<div class="fit-power-bar"><span>' + h(u,title) + '</span><progress max="100" value="' +
        Math.max(0,Math.min(100,percent)) + '" class="' + (percent > 100 ? 'over' : '') + '" aria-label="' + h(u,title) +
        '"></progress><strong>' + value(u,percent) + '%</strong></div>';
    };
    const priorities = s.powerPriorities || [];
    const rows = priorities.map(item => '<tr><th scope="row">' + value(u,item.priority) +
      '</th><td>' + value(u,item.retractedSum) + '</td><td>' + value(u,item.deployedSum) + '</td></tr>').join('');
    return '<div class="fit-power-chart">' +
      (s.powerRetracted == null ? '' : bar('Убрано',s.powerRetracted)) +
      (s.powerDeployed == null ? '' : bar('Развёрнуто',s.powerDeployed)) +
      (rows ? '<table><caption>' + h(u,'Нагрузка по приоритетам, МВт') + '</caption><thead><tr><th scope="col">' +
        h(u,'Приоритет') + '</th><th scope="col">' + h(u,'Убрано') + '</th><th scope="col">' + h(u,'Развёрнуто') +
        '</th></tr></thead><tbody>' + rows + '</tbody></table>' : '') + '</div>';
  }
  function resistanceTable(u,s,title,shield,armour,scale=1) {
    if (!s[shield] && !s[armour]) return '';
    const cell = n => n == null ? '—' : value(u,n*scale) + (scale === 100 ? '%' : '');
    const rows = Object.entries(damageTypes).filter(([k]) => s[shield]?.[k] != null || s[armour]?.[k] != null)
      .map(([k,t]) => '<tr><th scope="row">' + h(u,t) + '</th><td>' + cell(s[shield]?.[k]) +
        '</td><td>' + cell(s[armour]?.[k]) + '</td></tr>').join('');
    return rows ? '<table><caption>' + h(u,title) + '</caption><thead><tr><th scope="col">' + h(u,'Тип урона') +
      '</th><th scope="col">' + h(u,'Щит') + '</th><th scope="col">' + h(u,'Корпус') +
      '</th></tr></thead><tbody>' + rows + '</tbody></table>' : '';
  }
  const dl = (u,fields,data) => '<dl>' + fields.filter(([key]) => data[key] != null).map(([key,title,unit]) =>
    '<div><dt>' + h(u,title) + '</dt><dd>' + value(u,data[key],unit) + '</dd></div>').join('') + '</dl>';
  function details(r,u,s) {
    const distributor = s.distributor && dl(u,[
      ['systemsCapacity','Заряд систем',' МДж'], ['enginesCapacity','Заряд двигателей',' МДж'], ['weaponsCapacity','Заряд оружия',' МДж'],
      ['systemsRecharge','Подзарядка систем',' МВт'], ['enginesRecharge','Подзарядка двигателей',' МВт'], ['weaponsRecharge','Подзарядка оружия',' МВт'],
    ],s.distributor);
    return '<div class="fit-stat-details">' + statSections.map(([section,fields]) => {
      if (!fields.some(([key]) => s[key] != null)) return '';
      const extra = section === 'Энергия' ? powerChart(u,s) + (distributor || '') : section === 'Бой' ?
        resistanceTable(u,s,'Сопротивление урону','shieldResistances','armourResistances',100) +
        resistanceTable(u,s,'Эффективная прочность','shieldEffective','armourEffective') : '';
      return '<details class="panel fit-stat-section" data-fit-section="' + u.esc(section) +
        '"' + (r.expanded.has(section) ? ' open' : '') + '><summary>' + h(u,section) + '</summary>' +
        dl(u,fields,s) + extra + '</details>';
    }).join('') + '</div>';
  }

  function engineering(r,u,slot) {
    const blueprints = engine().blueprints(r.build,slot.key);
    if (!blueprints.length) return '';
    const current = slot.engineering || {};
    const selected = blueprints.find(item => (item.symbol || item.id) === current.blueprint);
    return '<div class="fit-engineering"><h3>' + h(u,'Инженерные улучшения') + '</h3>' +
      label(u,'Чертёж','<select id="fit-blueprint"' + dis(slot.module?.engineeringLocked) + '>' +
        option(u,'',u.t('Без улучшений'),current.blueprint) +
        [...blueprints].sort((a,b) => sort(u).compare(blueprintName(u,a),blueprintName(u,b))).map(item =>
          option(u,item.symbol || item.id,blueprintName(u,item),current.blueprint)).join('') + '</select>') +
      (selected ? '<div class="fit-fields">' + label(u,'Ступень','<select id="fit-grade"' + dis(slot.module?.engineeringLocked) + '>' +
        selected.grades.map(n => option(u,n,'G'+n,current.grade)).join('') + '</select>') +
        label(u,'Завершённость, %','<input id="fit-quality" type="number" min="0" max="100" step="1" value="' +
          u.esc(Math.round((current.quality ?? 1)*100)) + '"' + dis(slot.module?.engineeringLocked) + '>') + '</div>' : '') +
      (selected?.experimentals?.length ? label(u,'Экспериментальный эффект','<select id="fit-experimental">' +
        option(u,'',u.t('Без эффекта'),current.experimental) +
        [...selected.experimentals].sort((a,b) => sort(u).compare(experimentalName(u,a),experimentalName(u,b))).map(item =>
          option(u,item.id,experimentalName(u,item),current.experimental)).join('') + '</select>') : '') + '</div>';
  }

  function candidates(r,u,slot) {
    const terms = lower(r.query).trim().split(/\s+/).filter(Boolean);
    const collator = sort(u);
    const options = r.candidates.filter(mod => {
      const text = lower([moduleName(u,mod),mod.name,className(u,mod)].join(' '));
      return terms.every(term => text.includes(term));
    }).sort((a,b) => collator.compare(moduleName(u,a),moduleName(u,b)) || b.size-a.size || collator.compare(a.rating,b.rating));
    return (!slot.required && slot.module && ['optional','utility','hardpoint'].includes(slot.kind) ?
      button(u,'remove-module','Убрать модуль') : '') +
      '<ul class="fit-candidates">' + options.slice(0,r.limit).map(mod => {
        const selected = mod.id === slot.module?.id;
        return '<li><button type="button" data-fit-action="choose-module" data-fit-value="' + u.esc(mod.id) +
          '" data-selected="' + selected + '"' + dis(selected) + '><span><strong>' + u.esc(moduleName(u,mod)) +
          '</strong><small>' + u.esc([className(u,mod),mod.preEngineered && u.t('Модуль с заводской доработкой')].filter(Boolean).join(' · ')) +
          '</small></span>' +
          (mod.mass == null ? '' : '<small>' + value(u,mod.mass,' т') + '</small>') + '</button></li>';
      }).join('') + '</ul>' +
      (options.length > r.limit ? button(u,'more','Показать ещё') : '') +
      (!options.length && r.candidates.length ? '<p class="fit-muted">' + h(u,'Подходящих модулей нет.') + '</p>' : '');
  }

  function inspector(r,u,slots) {
    const slot = slots.find(item => item.key === r.slotKey);
    if (!slot) return '';
    if (r.candidateBuild !== r.build || r.candidateSlot !== slot.key) {
      r.candidateBuild = r.build;
      r.candidateSlot = slot.key;
      r.candidates = engine().candidates(r.build,slot.key);
    }
    const mod = slot.module, index = slots.filter(item => item.kind===slot.kind).indexOf(slot);
    const moduleStats = [
      ['mass','Масса',' т'], ['power','Потребление',' МВт'], ['cost','Цена',' кр.'], ['integrity','Прочность'],
      ['damage','Урон'], ['dps','Урон в секунду'], ['sustainedDps','Урон в секунду с перезарядкой'], ['range','Дальность',' м'],
      ['falloff','Начало падения урона',' м'], ['shotSpeed','Скорость снаряда',' м/с'], ['piercing','Пробитие'],
      ['ammo','Запас боеприпасов'], ['clip','Боеприпасов в магазине'], ['rateOfFire','Темп стрельбы'],
      ['eps','Энергия на стрельбу в секунду',' МДж'], ['hps','Тепло при стрельбе в секунду'],
    ];
    return '<section id="fit-inspector" tabindex="-1" class="panel fit-box fit-inspector"><header><h2>' + u.esc(slotName(u,slot,index)) + '</h2>' +
      button(u,'close-slot','Закрыть',{style:'quiet'}) + '</header>' +
      (mod ? '<div class="fit-selected"><strong>' + u.esc(moduleName(u,mod)) + '</strong><small>' + u.esc(className(u,mod)) +
        '</small>' + dl(u,slot.kind === 'hardpoint' ? moduleStats : moduleStats.slice(0,4),mod) +
        (slot.canControlPower !== false && slot.kind !== 'armour' && mod.power > 0 ?
          '<div class="fit-controls">' + label(u,'Приоритет питания','<select id="fit-priority">' +
            [1,2,3,4,5].map(n=>option(u,n,n,slot.priority)).join('') + '</select>') +
          '<label class="fit-check"><input type="checkbox" id="fit-module-power"' +
            (slot.on !== false ? ' checked' : '') + '> ' + h(u,'Включено') + '</label></div>' : '') +
        engineering(r,u,slot) + '</div>' : '') +
      (r.candidates.length ? label(u,'Подходящие модули','<input type="search" id="fit-module-search" autocomplete="off" placeholder="' +
        h(u,'Поиск модуля') + '" value="' + u.esc(r.query) + '">') : '') +
      '<div id="fit-candidate-items">' + candidates(r,u,slot) + '</div></section>';
  }
  function comparison(r,u,s) {
    if (!r.fits.length) return '';
    const sorted = [...r.fits].sort((a,b)=>sort(u).compare(a.name,b.name));
    const control = label(u,'Сравнить со сборкой','<select id="fit-compare"' + dis(r.busy) + '>' +
      option(u,'',u.t('Выберите сборку'),r.compare?.id) +
      sorted.map(fit=>option(u,fit.id,fit.name,r.compare?.id)).join('') + '</select>');
    if (!r.compare) return '<div class="panel fit-box fit-comparison">' + control + '</div>';
    const baseline = engine().stats(r.compare.build,{cargo:s.cargo,fuel:s.fuel,eng:s.eng,sys:s.sys,wep:s.wep});
    const metrics = [
      ['mass','Текущая масса',' т'],['speed','Скорость',' м/с'],['boostSpeed','Форсаж',' м/с'],
      ['jumpRange','Текущий прыжок',' св. л.'],['cargoCapacity','Вместимость трюма',' т'],
      ['shield','Прочность щита',' МДж'],['armour','Прочность корпуса',''],['dps','Урон в секунду',''],
      ['powerDeployed','С развёрнутым оружием',' МВт'],['cost','Стоимость сборки',' кр.'],
    ];
    return '<section class="panel fit-box fit-comparison"><h2>' + h(u,'Сравнение сборок') + '</h2>' + control +
      '<table><thead><tr><th scope="col">' + h(u,'Параметр') + '</th><th scope="col">' + u.esc(r.compare.name) +
      '</th><th scope="col">' + u.esc(r.name) + '</th><th scope="col">' + h(u,'Разница') + '</th></tr></thead><tbody>' +
      metrics.map(([k,title,unit]) => {
        const delta = s[k] == null || baseline[k] == null ? null : s[k]-baseline[k];
        return '<tr><th scope="row">' + h(u,title) + '</th><td>' + value(u,baseline[k],unit) + '</td><td>' + value(u,s[k],unit) +
          '</td><td>' + (delta > 0 ? '+' : '') + value(u,delta,unit) + '</td></tr>';
      }).join('') + '</tbody></table></section>';
  }
  function materialInput(r) {
    if (r.materialInput?.build !== r.build) {
      const modules = fitState(r).slots.filter(slot => slot.engineering?.blueprint).map(slot => {
        const {blueprint,grade,quality,experimental} = slot.engineering;
        const {engineeringLocked,factoryExperimental} = slot.module || {};
        return {slot:slot.key,blueprint,grade,quality,experimental,engineeringLocked,factoryExperimental};
      });
      r.materialInput = {build:r.build, modules, key:JSON.stringify(modules)};
    }
    return r.materialInput;
  }
  function materialsContent(r,u) {
    const {modules,key} = materialInput(r);
    if (!modules.length) return '<p class="fit-muted">' + h(u,'Улучшения не выбраны') + '</p>';
    const result = r.materials?.key === key ? r.materials : null;
    if (result?.error) return '<p class="fit-alert" role="status">' + h(u,'Не удалось рассчитать материалы.') + ' ' +
      button(u,'materials-retry','Повторить',{style:'quiet'}) + '</p>';
    if (!result?.data) return '<p class="fit-muted" role="status">' + h(u,'Рассчитываем материалы…') + '</p>';
    const {groups, unavailable, currencies, estimated} = result.data;
    const allGroups = groups || [];
    const row = (name,required,owned,missing,scope='row') =>
      '<tr' + (scope === 'rowgroup' ? ' class="fit-material-group"' : '') + '><th scope="' + scope + '">' +
      u.esc(name) + '</th><td>' + value(u,required) + '</td><td>' + value(u,owned) +
      '</td><td' + (missing > 0 ? ' class="fit-material-deficit"' : '') + '>' + value(u,missing) + '</td></tr>';
    const collator = sort(u);
    const known = rows => rows.every(item => item.owned != null && item.missing != null);
    const stock = rows => rows.reduce((sum,item) => sum+item.owned,0);
    const tables = materialGroups.map(([category,title]) => {
      const group = allGroups.find(group => group.category === category);
      const rows = [...(group?.rows || [])].sort((a,b) => (b.missing || 0)-(a.missing || 0) ||
        collator.compare(materialName(u,a),materialName(u,b)));
      const covered = known(rows);
      return '<tbody>' + row(u.t(title),group?.required ?? 0,rows.length && covered ? stock(rows) : null,
        covered ? group?.missing ?? 0 : null,'rowgroup') +
        rows.map(item => row(materialName(u,item),item.required,item.owned,item.missing)).join('') + '</tbody>';
    }).join('');
    const allRows = allGroups.flatMap(group => group.rows || []);
    const stockUnknown = !known(allRows);
    const total = allGroups.reduce((sum,group) => sum+group.required,0);
    const shortage = stockUnknown ? null : allGroups.reduce((sum,group) => sum+(group.missing ?? 0),0);
    const slots = unavailable?.length ? fitState(r).slots : [];
    const missingRecipes = (unavailable || []).map(item => {
      const index = slots.findIndex(slot => slot.key === item.slot);
      if (index < 0) return item.slot;
      const slot = slots[index];
      const number = slots.slice(0,index).filter(other => other.kind === slot.kind).length;
      return slotName(u,slot,number) + ' · ' + moduleName(u,slot.module);
    });
    const table = allRows.length ? '<table><thead><tr><th scope="col">' + h(u,'Материал') + '</th><th scope="col">' +
      h(u,'Нужно') + '</th><th scope="col">' + h(u,'В запасе') + '</th><th scope="col">' +
      h(u,'Не хватает') + '</th></tr></thead>' + tables + '<tfoot>' +
      row(u.t('Итого'),total,stockUnknown ? null : stock(allRows),shortage) + '</tfoot></table>' : '';
    const currencyList = currencies?.length ? '<dl class="fit-material-currencies">' + currencies.map(item =>
      '<div><dt>' + u.esc(materialName(u,item)) + '</dt><dd>' + value(u,item.required) + '</dd></div>').join('') + '</dl>' : '';
    if (!table && !currencyList && !missingRecipes.length) return '<p class="fit-muted">' + h(u,'Материалы не требуются') + '</p>';
    return table + currencyList +
      (estimated ? '<p class="fit-muted">' + h(u,'Расход с нуля при 5-м ранге у инженера.') + '</p>' : '') +
      (stockUnknown ? '<p class="fit-muted">' + h(u,'Запас материалов ещё не получен из журнала.') + '</p>' : '') +
      (missingRecipes.length ? '<p class="fit-muted">' +
        h(u,'Расход неполный. Не хватает данных о рецептах для {modules}.',{modules:[...new Set(missingRecipes)].join(', ')}) + '</p>' : '');
  }
  function materialName(u,item) {
    return russian(u) ? item.name || item.english || item.key : item.english || u.t(item.name || item.key);
  }
  function materials(r,u) {
    return '<section class="panel fit-box fit-materials"><h2>' + h(u,'Материалы для сборки') +
      '</h2><div id="fit-materials-content">' + materialsContent(r,u) + '</div></section>';
  }
  function editor(r,u) {
    if (!r.build) return '';
    const {slots,stats} = fitState(r);
    return '<div class="fit-editor"><div class="panel fit-box fit-identity"><div class="fit-identity-main">' +
      '<span class="fit-identity-icon">' + u.icon('ship') + '</span><div><h2>' + u.esc(shipName(u,r.build.shipId)) + '</h2>' +
      '<input id="fit-name" aria-label="' + h(u,'Название сборки') + '" maxlength="120" placeholder="' +
      h(u,'Название сборки') + '" value="' + u.esc(r.name) + '"></div></div><div class="fit-actions">' +
      button(u,'save','Сохранить сборку',{style:'primary',icon:'check',disabled:r.busy}) +
      (r.id ? button(u,'copy','Сохранить копию',{disabled:r.busy}) : '') +
      '<select id="fit-export-format" aria-label="' + h(u,'Формат экспорта') + '"' + dis(r.busy) + '>' +
      option(u,'native','ED Copilot JSON',r.exportFormat || 'native') + option(u,'slef','SLEF',r.exportFormat) +
      ((engine().supportsCoriolis?.(r.build) ?? true) ? option(u,'coriolis','Coriolis Detailed',r.exportFormat) : '') + '</select>' +
      button(u,'export','Экспорт JSON',{disabled:r.busy}) +
      (engine().toCoriolisURL && (engine().supportsCoriolis?.(r.build) ?? true) ?
        button(u,'link','Скопировать ссылку Coriolis',{disabled:r.busy}) : '') +
      (r.id ? button(u,'delete','Удалить',{style:'quiet',disabled:r.busy}) : '') + '</div></div>' +
      '<div class="panel fit-summary">' + metrics(u,stats) + '</div>' +
      (stats.canThrust === false ? '<p class="fit-alert" role="status">' + h(u,'По ограничениям массы корабль не может разогнаться') + '</p>' :
        stats.canBoost === false ? '<p class="fit-alert" role="status">' + h(u,'Форсаж недоступен') + '</p>' : '') +
      '<div class="fit-body"><div class="fit-slot-list">' + slotGroups(r,u,slots) + '</div><aside class="fit-details">' +
      inspector(r,u,slots) + materials(r,u) + conditions(r,u,stats) + details(r,u,stats) + '</aside></div>' +
      comparison(r,u,stats) + '</div>';
  }
  function content(r,u) {
    if (!engine()) return '<p class="fit-alert" role="alert">' + h(u,'Не удалось загрузить каталог оснащения.') + '</p>';
    const top = toolbar(r,u) + feedback(r,u);
    if (!r.loaded && !r.error) return top + '<p class="fit-muted" role="status">' + h(u,'Загружаем библиотеку сборок…') + '</p>';
    return top + (!r.loaded && r.error ? button(u,'retry','Повторить') : '') + importForm(r,u) + newForm(r,u) + editor(r,u);
  }
  function render(C,u) {
    return u.heading(u.t(mode),'') + '<div id="ship-fits" class="ship-fits">' + content(model(C),u) + '</div>';
  }
  function draw(C,u,focus) {
    const host = active(C) && u.$('#ship-fits');
    if (!host) return;
    const r = model(C);
    const markup = content(r,u);
    if (host.fitMarkup !== markup) {
      host.innerHTML = markup;
      host.fitMarkup = markup;
    }
    if (focus) (u.$(focus,host) || (focus==='#fit-module-search' ? u.$('#fit-inspector',host) : null))?.focus();
    void loadMaterials(C,u,r);
  }

  const request = (r,u,op,params={}) => u.call(op,{...params,expectedPlanWorkspace:r.owner});
  async function loadMaterials(C,u,r) {
    if (!r.build || C.state?.connection?.scanning) return;
    const {modules,key} = materialInput(r);
    const revision = C.state?.shipBuildRevision;
    if (!modules.length) {
      r.materials = null;
      return;
    }
    if (r.materials?.key === key && r.materials.revision === revision) return;
    const result = {key,revision,data:r.materials?.key === key ? r.materials.data : null};
    r.materials = result;
    try {
      result.data = await request(r,u,'ship-fit-materials',{modules});
    } catch {
      result.error = true;
    }
    if (!active(C) || model(C) !== r || r.materials !== result || materialInput(r).key !== key) return;
    const content = u.$('#fit-materials-content');
    if (content) content.innerHTML = materialsContent(r,u);
  }
  async function refresh(C,u,r) {
    const data = await request(r,u,'ship-fits-list');
    if (model(C)!==r) return;
    r.fits = data.fits || [];
    if (r.compare && !r.fits.some(fit=>fit.id===r.compare.id)) r.compare = null;
    r.observed = data.observed || null;
    r.loaded = true;
  }
  function install(r,u,build,record) {
    engine().slots(build);
    r.build = {...build};
    r.id = record?.id;
    r.name = record?.name || buildName(u,build);
    r.cfg = Object.fromEntries(['cargo','fuel','sys','eng','wep'].filter(key=>Number.isFinite(build[key])).map(key=>[key,build[key]]));
    r.dirty = !record;
    r.showNew = false;
    r.showImport = false;
    r.exportFormat = 'native';
    r.slotKey = '';
    r.query = '';
    r.confirm = null;
  }
  function run(C,u,r,work,focus) {
    if (r.busy) return;
    r.error = '';
    r.busy = true;
    draw(C,u);
    Promise.resolve().then(work).catch(error => {
      if (model(C)===r) r.error = u.t(error?.message || 'Не удалось выполнить действие.');
    }).finally(() => {
      if (model(C)!==r) return;
      r.busy = false;
      draw(C,u,focus);
    });
  }
  function guard(C,u,r,work) {
    if (r.busy) return;
    if (r.dirty) {
      r.confirm = {title:'Есть несохранённые изменения. Продолжить?',button:'Продолжить',work};
      draw(C,u,'[data-fit-action="cancel"]');
      return;
    }
    run(C,u,r,work);
  }
  function record(build,name,cfg={}) {
    name = name.trim();
    if (!name) throw new Error('Укажите название сборки.');
    return {name,ship:build.shipId,build:{...build,...cfg,name}};
  }
  async function save(C,u,r,copy=false) {
    const suffix = ' · ' + u.t('Копия');
    const name = copy ? [...r.name].slice(0,120-suffix.length).join('') + suffix : r.name;
    const fit = record(r.build,name,r.cfg);
    const data = await request(r,u,'ship-fit-save',{...fit,id:copy ? undefined : r.id});
    if (model(C)!==r) return;
    r.id = data.fit.id;
    if (r.compare?.id === r.id) r.compare = data.fit;
    r.name = data.fit.name;
    r.dirty = false;
    await refresh(C,u,r);
    u.toast(u.t('Сборка сохранена.'));
  }
  function edit(C,u,r,change,focus) {
    if (r.busy || !r.slotKey) return;
    try {
      r.build = engine().edit(r.build,r.slotKey,change);
      r.dirty = true;
      r.error = '';
    } catch (error) {
      r.error = u.t(error.message);
    }
    draw(C,u,focus);
  }
  function configure(C,u,r,changes,focus) {
    r.cfg = {...r.cfg,...changes};
    r.dirty = true;
    draw(C,u,focus);
  }
  function redistribute(r,field,amount) {
    const [a,b] = ['sys','eng','wep'].filter(key=>key!==field);
    const oldA = r.cfg[a] ?? 2, oldB = r.cfg[b] ?? 2;
    const nextA = Math.min(4,Math.max(0,6-amount-oldB));
    return {[field]:amount,[a]:nextA,[b]:6-amount-nextA};
  }

  async function exportFile(u,name,text) {
    const filename = name.replace(/[<>:"/\\|?*\u0000-\u001f]/g,'_').slice(0,120) + '.json';
    await u.call('native',{action:'save-json',name:filename,text});
  }
  function action(C,u,r,which,id) {
    if (r.busy) return;
    switch (which) {
      case 'retry': run(C,u,r,() => refresh(C,u,r)); return;
      case 'materials-retry': r.materials = null; draw(C,u); return;
      case 'new':
        r.showNew = !r.showNew; draw(C,u,'#fit-new-ship'); return;
      case 'import':
        r.showImport = !r.showImport;
        r.fileRequest = (r.fileRequest || 0)+1;
        draw(C,u,r.showImport ? '#fit-import-text' : '[data-fit-action="import"]'); return;
      case 'edit-slot':
        r.slotKey = id; r.query = ''; r.limit = 30;
        draw(C,u,'#fit-module-search'); return;
      case 'close-slot': {
        const slot = r.slotKey;
        r.slotKey = ''; r.query = ''; draw(C,u);
        u.$$('#ship-fits [data-fit-action="edit-slot"]').find(node=>node.dataset.fitValue===slot)?.focus();
        return;
      }
      case 'choose-module': edit(C,u,r,{moduleId:id},'#fit-module-search'); return;
      case 'remove-module': edit(C,u,r,{moduleId:null},'#fit-module-search'); return;
      case 'more': r.limit += 30; draw(C,u,'#fit-module-search'); return;
      case 'fill-cargo': configure(C,u,r,{cargo:fitState(r).stats.cargoCapacity},'#fit-cargo'); return;
      case 'empty-cargo': configure(C,u,r,{cargo:0},'#fit-cargo'); return;
      case 'fill-fuel': configure(C,u,r,{fuel:fitState(r).stats.fuelCapacity},'#fit-fuel'); return;
      case 'preview':
        run(C,u,r,() => {
          if (!r.importText.trim()) throw new Error('Вставьте JSON или ссылку Coriolis.');
          r.imported = engine().parse(r.importText);
          if (!r.imported?.length) throw new Error('В импорте нет поддерживаемых сборок.');
          r.importIndex = 0;
        },'#fit-import-choice'); return;
      case 'open-import':
        {
          const build = r.imported[r.importIndex];
          guard(C,u,r,() => install(r,u,build));
        } return;
      case 'save-import':
        run(C,u,r,async () => {
          const fits = r.imported.map(build=>record(build,buildName(u,build)));
          if (new TextEncoder().encode(JSON.stringify({fits})).length>768*1024) {
            throw new Error('Слишком много сборок для одного импорта.');
          }
          await request(r,u,'ship-fits-import',{fits});
          if (model(C)!==r) return;
          await refresh(C,u,r);
          r.showImport = false; r.imported = null; r.importText = '';
          u.toast(u.t('Библиотека обновлена.'));
        }); return;
      case 'current':
        guard(C,u,r,async () => {
          await refresh(C,u,r);
          if (model(C)!==r) return;
          if (!r.observed) throw new Error('В журнале пока нет оснащения текущего корабля.');
          install(r,u,engine().fromLoadout(r.observed));
        }); return;
      case 'save': case 'copy':
        run(C,u,r,() => save(C,u,r,which==='copy')); return;
      case 'export':
        run(C,u,r,() => {
          const {build,name} = record(r.build,r.name,r.cfg);
          return exportFile(u,name,engine().export(build,(engine().supportsCoriolis?.(build) === false && r.exportFormat === 'coriolis') ? 'native' : r.exportFormat));
        }); return;
      case 'export-library':
        run(C,u,r,async () => {
          const {fits} = await request(r,u,'ship-fits-export');
          if (model(C)===r) await exportFile(u,'ED Copilot Fittings',JSON.stringify(fits.map(fit=>fit.build)));
        }); return;
      case 'link':
        run(C,u,r,async () => {
          const {build} = record(r.build,r.name,r.cfg);
          await u.call('native',{action:'copy',text:engine().toCoriolisURL(build)});
          u.toast(u.t('Ссылка скопирована.'));
        }); return;
      case 'delete': {
        const fitId = r.id;
        r.confirm = {title:'Удалить сборку из библиотеки?',button:'Удалить',work:async () => {
          await request(r,u,'ship-fit-delete',{id:fitId});
          if (model(C)!==r) return;
          r.id = undefined;
          await refresh(C,u,r);
        }};
        draw(C,u,'[data-fit-action="cancel"]'); return;
      }
      case 'confirm': {
        const work = r.confirm?.work;
        r.confirm = null;
        if (work) run(C,u,r,work);
        return;
      }
      case 'cancel': r.confirm = null; draw(C,u); return;
    }
  }
  function input(r,u,field) {
    if (field.id === 'fit-name') {
      r.name = field.value;
      r.dirty = true;
    }
    if (field.id === 'fit-import-text') {
      r.importText = field.value;
      r.imported = null;
      r.fileRequest = (r.fileRequest || 0)+1;
      u.$('#fit-import-selection')?.remove();
    }
    if (field.id === 'fit-module-search') {
      r.query = field.value;
      r.limit = 30;
      const slot = fitState(r).slots.find(item=>item.key===r.slotKey);
      const target = u.$('#fit-candidate-items');
      if (slot && target) target.innerHTML = candidates(r,u,slot);
    }
  }
  async function readFile(C,u,r,file) {
    const request = r.fileRequest = (r.fileRequest || 0)+1;
    try {
      if (file.size>2*1024*1024) throw new Error('Выберите JSON-файл размером до 2 МБ.');
      const text = await file.text();
      if (model(C)!==r || r.fileRequest!==request) return;
      r.importText = text;
      r.imported = null;
      r.error = '';
    } catch (error) {
      if (model(C)===r && r.fileRequest===request) r.error = u.t(error.message);
    }
    if (model(C)===r && r.fileRequest===request) draw(C,u,'#fit-import-text');
  }
  function change(C,u,r,field) {
    if (r.busy) return;
    switch (field.id) {
      case 'fit-library': {
        const id = field.value;
        if (!id) return;
        field.value = r.id || '';
        guard(C,u,r,async () => {
          const {fit} = await request(r,u,'ship-fit-get',{id});
          if (model(C)===r) install(r,u,fit.build,fit);
        });
        return;
      }
      case 'fit-compare': {
        const id = field.value;
        if (!id) {r.compare = null; draw(C,u,'#fit-compare'); return;}
        run(C,u,r,async () => {
          const {fit} = await request(r,u,'ship-fit-get',{id});
          if (model(C)!==r) return;
          engine().slots(fit.build);
          r.compare = fit;
        },'#fit-compare');
        return;
      }
      case 'fit-export-format': r.exportFormat = field.value; return;
      case 'fit-new-ship': r.newShip = field.value; return;
      case 'fit-import-choice': r.importIndex = Number(field.value); return;
      case 'fit-import-file': if (field.files?.length) void readFile(C,u,r,field.files[0]); return;
      case 'fit-module-power': edit(C,u,r,{on:field.checked},'#fit-module-power'); return;
      case 'fit-priority': edit(C,u,r,{priority:Number(field.value)},'#fit-priority'); return;
    }
    if (field.dataset.fitCondition) {
      const amount = Math.min(Number(field.max),Math.max(0,Number(field.value)||0));
      configure(C,u,r,{[field.dataset.fitCondition]:amount},'#'+field.id);
      return;
    }
    if (field.dataset.fitPip) {
      const amount = Math.min(4,Math.max(0,Math.round(Number(field.value)*2)/2 || 0));
      configure(C,u,r,redistribute(r,field.dataset.fitPip,amount),'#'+field.id);
      return;
    }
    if (!['fit-blueprint','fit-grade','fit-experimental','fit-quality'].includes(field.id)) return;
    const slot = fitState(r).slots.find(item=>item.key===r.slotKey);
    if (!slot) return;
    const bp = engine().blueprints(r.build,slot.key).find(item=>(item.symbol || item.id)===field.value);
    const current = slot.engineering || {};
    const engineering = {
      'fit-blueprint':bp ? {blueprint:bp.symbol || bp.id,grade:Math.max(...bp.grades),quality:1} : null,
      'fit-grade':{...current,grade:Number(field.value),quality:1},
      'fit-experimental':{...current,experimental:field.value},
      'fit-quality':{...current,quality:Math.min(1,Math.max(0,Number(field.value)/100 || 0))},
    }[field.id];
    edit(C,u,r,{engineering},'#'+field.id);
  }
  function attach(C,u) {
    const host = active(C) && u.$('#ship-fits');
    if (!host || !engine()) return;
    const r = model(C);
    void loadMaterials(C,u,r);
    if (host.fitBound) return;
    host.fitBound = true;
    host.fitMarkup = content(r,u);
    host.addEventListener('click', event => {
      const control = event.target.closest('[data-fit-action]');
      if (control) action(C,u,model(C),control.dataset.fitAction,control.dataset.fitValue);
    });
    host.addEventListener('input', event => input(model(C),u,event.target));
    host.addEventListener('change', event => change(C,u,model(C),event.target));
    host.addEventListener('submit', event => {
      if (event.target.id !== 'fit-new-form') return;
      event.preventDefault();
      const ship = event.target.elements.ship.value;
      const r = model(C);
      guard(C,u,r,() => install(r,u,engine().create(ship)));
    });
    host.addEventListener('toggle', event => {
      const panel = event.target.dataset?.fitSection;
      if (!panel || !host.contains(event.target)) return;
      const expanded = model(C).expanded;
      if (event.target.open) expanded.add(panel);
      else expanded.delete(panel);
    },true);
    if (!r.loaded && !r.busy && !r.error) run(C,u,r,() => refresh(C,u,r));
  }

  function refreshMaterials(C,u) {
    if (active(C)) void loadMaterials(C,u,model(C));
  }

  window.EDCopilotShipFits = {active,render,attach,init:attach,refreshMaterials};
})();
