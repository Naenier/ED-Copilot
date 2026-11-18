(() => {
'use strict';

const sections = [
  ['stations','Станции','dock'], ['outfitting','Снаряжение','package'],
  ['factions','Малые фракции','users'], ['systems','Звёздные системы','orbit'],
  ['bodies','Небесные тела','globe'], ['signals','Сигналы','radio'],
  ['misc','Разное','ellipsis'],
];
const options = window.NearestOptions || {};
const list = value => Array.isArray(value) ? value : [];
const h = (u,key,vars) => u.esc(u.t(key,vars));
const display = (u,value) => h(u,String(value ?? ''));
const text = value => String(value ?? '').trim();
const active = C => C.tab === 'nearest';
const ru = u => u.locale().startsWith('ru');
const validPage = new Set(sections.map(([id])=>id));
let sequence = Date.now()*1000;

const colonization = [['colonized','Колонизированные'],['construction','В процессе колонизации'],['uncolonized','Неколонизированные']];
const population = [['populated','Населённые'],['unpopulated','Ненаселённые']];
const presence = [['present','Присутствует'],['controlling','Контролирует систему'],['not-controlling','Присутствует без контроля'],['exclude','Отсутствует']];
const logic = [['all','Все выбранные'],['any','Любое из выбранных'],['none','Ни одного из выбранных']];
const pad = [['S','Малая'],['M','Средняя'],['L','Большая']];
const ground = [['all','Все наземные станции'],['legacy','Кроме поселений Odyssey'],['no','Только орбитальные']];
const modes = [
  ['conflicts','Войны и гражданские войны'],['restore','Миссии по восстановлению'],['mining','Миссии по добыче'],
  ['massacre','Боевые миссии против пиратов'],['multi-state','Фракции с несколькими состояниями'],['engineers','Инженеры'],
];
const signals = [
  ['tourist-beacon','Туристический маяк'],['navigation-beacon','Навигационный маяк'],
  ['compromised-nav-beacon','Скомпрометированный навигационный маяк'],['guardian-beacon','Маяк Стражей'],
  ['guardian-ruin','Руины Стражей'],['guardian-structure','Структура Стражей'],['thargoid-structure','Структура таргоидов'],
  ['thargoid-spire','Шпиль таргоидов'],['megaship','Мегакорабль'],['installation','Установка'],
  ['wrecked-ship','Обломки корабля'],['abandoned-base','Заброшенная база'],['planetary-mining','Участок планетарной добычи'],
  ['biological','Биологические сигналы'],['geological','Геологические сигналы'],
  ['human','Сигналы людей'],['guardian','Сигналы Стражей'],['thargoid','Сигналы таргоидов'],
];

const terms = {
  'Material Trader':'Торговец материалами','Technology Broker':'Технологический брокер',
  'Interstellar Factors Contact':'Межзвёздный посредник','Pioneer Supplies':'Товары первопроходцев',
  'Fleet Carrier':'Флотоносец','Stronghold Carrier':'Флотоносец оплота',
  'Universal Cartographics':'Универсальная картография','Vista Genomics':'Виста Геномикс',
  'Outfitting':'Оснащение','Shipyard':'Верфь','Market':'Рынок','Black Market':'Чёрный рынок',
  'Refuel':'Заправка','Repair':'Ремонт','Restock':'Пополнение боеприпасов','Workshop':'Мастерская',
  'Crew Lounge':'Комната экипажа','Frontline Solutions':'Фронтлайн Солюшнс','Apex Interstellar':'Апекс Интерстеллар',
  'Fleet Carrier Administration':'Управление флотом','Fleet Carrier Fuel':'Заправка флотоносца',
  'Raw':'Сырьё','Manufactured':'Промышленные материалы','Encoded':'Данные','Guardian':'Стражи','Human':'Люди',
  'Orbis Starport':'Орбитальная станция «Орбис»','Coriolis Starport':'Орбитальная станция «Кориолис»',
  'Ocellus Starport':'Орбитальная станция «Оцеллус»','Outpost':'Аванпост','Planetary Port':'Планетарный порт',
  'Planetary Outpost':'Наземный аванпост','Settlement':'Поселение','Megaship':'Мегакорабль','Asteroid Base':'База в астероиде',
  'Federation':'Федерация','Empire':'Империя','Alliance':'Альянс','Independent':'Независимая',
  'Anarchy':'Анархия','Communism':'Коммунизм','Confederacy':'Конфедерация','Cooperative':'Кооператив',
  'Corporate':'Корпорация','Democracy':'Демократия','Dictatorship':'Диктатура','Feudal':'Феодализм',
  'Patronage':'Патронаж','Prison Colony':'Тюремная колония','Theocracy':'Теократия',
  'High':'Высокая','Medium':'Средняя','Low':'Низкая','Lawless':'Беззаконие','None':'Нет',
  'Agriculture':'Сельское хозяйство','Industrial':'Промышленность','Extraction':'Добыча','Refinery':'Переработка',
  'High Tech':'Высокие технологии','Tourism':'Туризм','Service':'Услуги','Military':'Военная',
  'Colony':'Колония','Terraforming':'Терраформирование','Prison':'Тюрьма',
  'War':'Война','Civil War':'Гражданская война','Election':'Выборы','Boom':'Бум','Bust':'Спад',
  'Expansion':'Экспансия','Retreat':'Отступление','Outbreak':'Эпидемия','Civil Liberty':'Гражданская свобода',
  'Civil Unrest':'Гражданские беспорядки','Famine':'Голод','Lockdown':'Блокада','Investment':'Инвестиции',
  'Exploited':'Эксплуатация','Fortified':'Укрепление','Stronghold':'Оплот','Unoccupied':'Не занята',
  'Rocky':'Каменистые','Icy':'Ледяные','Metal Rich':'Богатые металлами','Metallic':'Металлические',
  'Pristine':'Нетронутые','Major':'Значительные','Common':'Обычные','Depleted':'Истощённые',
  'Water world':'Водный мир','Earth-like world':'Землеподобный мир','Ammonia world':'Аммиачный мир',
  'High metal content world':'Мир с высоким содержанием металлов','Metal-rich body':'Богатое металлами тело',
  'Rocky body':'Каменистое тело','Icy body':'Ледяное тело','Rocky Ice world':'Каменисто-ледяной мир',
  'Black Hole':'Чёрная дыра','Neutron Star':'Нейтронная звезда','Supermassive Black Hole':'Сверхмассивная чёрная дыра',
  'Not terraformable':'Не терраформируется','Terraformable':'Подходит для терраформирования',
  'Terraformed':'Терраформировано','Terraforming candidate':'Кандидат на терраформирование',
  'Iron':'Железо','Nickel':'Никель','Sulphur':'Сера','Carbon':'Углерод','Phosphorus':'Фосфор',
  'Chromium':'Хром','Manganese':'Марганец','Cadmium':'Кадмий','Molybdenum':'Молибден','Zinc':'Цинк',
  'Arsenic':'Мышьяк','Germanium':'Германий','Vanadium':'Ванадий','Niobium':'Ниобий','Tungsten':'Вольфрам',
  'Selenium':'Селен','Tin':'Олово','Tellurium':'Теллур','Ruthenium':'Рутений','Yttrium':'Иттрий',
  'Technetium':'Технеций','Antimony':'Сурьма','Polonium':'Полоний','Mercury':'Ртуть','Zirconium':'Цирконий',
};
function term(u,key) {
  return ru(u) ? options.moduleLabels?.[key] || terms[key] || u.t(key) : key;
}
const optPair = (u,item) => Array.isArray(item)?[item[0],u.t(item[1])]:[String(item),term(u,String(item))];
const field = (u,label,content) => '<label class="near-field"><span>'+h(u,label)+'</span>'+content+'</label>';
function option(u,key,label,selected) {
  return '<option value="'+u.esc(key)+'"'+(String(key)===String(selected)?' selected':'')+'>'+u.esc(label)+'</option>';
}
function selectControl(u,key,items,value,blank,attrs='') {
  return '<select name="'+key+'" '+attrs+'>'+(blank==null?'':option(u,'',u.t(blank),value))+
    list(items).map(item=>optPair(u,item)).map(([id,title])=>option(u,id,title,value)).join('')+'</select>';
}
function select(u,s,key,label,items,blank='Любое значение') {
  return field(u,label,selectControl(u,key,items,s.values[key],blank));
}
function input(u,s,key,label,attrs='') {
  return field(u,label,'<input name="'+key+'" value="'+u.esc(s.values[key] ?? '')+'" '+attrs+'>');
}
function check(u,s,key,label) {
  return '<label class="near-check"><input type="checkbox" name="'+key+'"'+(s.values[key]?' checked':'')+'> '+h(u,label)+'</label>';
}
function button(u,action,label,icon='',attrs='') {
  return '<button type="button" class="pill '+(action==='clear'?'quiet':'')+'" data-near-action="'+action+'" '+attrs+'>'+
    (icon?u.icon(icon):'')+h(u,label)+'</button>';
}
function nearbyModel(C) {
  if (!C.nearestUI) C.nearestUI = {section:'stations', sessions:{}};
  return C.nearestUI;
}
function session(C,id=nearbyModel(C).section) {
  const model=nearbyModel(C);
  if (!model.sessions[id]) model.sessions[id]={
    id,values:{origin:C.state?.commander?.system || 'Sol', surface:'all',sort:id==='bodies'?'materials':'distance', presenceMode:'present',
      ownerMode:'owner',ownerStateMode:'all',factionStateMode:'all',stateMode:'all',ringKind:'ring',
      stronghold:'any',kind:'conflicts',signal:'tourist-beacon'},
    groups:{},details:{},page:0,primed:false,
  };
  return model.sessions[id];
}
const grid = (...fields) => '<div class="near-grid">'+fields.filter(Boolean).join('')+'</div>';
const span = html => '<div class="near-span">'+html+'</div>';
function chips(u,s,key,label,items) {
  const chosen=list(s.groups[key]);
  return '<div class="near-multiple"><label><span>'+h(u,label)+'</span>'+
    selectControl(u,'add-'+key,list(items).filter(item=>!chosen.includes(Array.isArray(item)?item[0]:item)),'','Добавить','data-near-add="'+key+'"')+
    '</label><div class="near-chips">'+chosen.map((value,index)=>chip(u,key,index,term(u,value),key==='materials')).join('')+'</div></div>';
}
function chip(u,key,index,label,reorder=false) {
  const attrs='data-group="'+key+'" data-index="'+index+'"';
  return '<span class="near-chip"><span>'+u.esc(label)+'</span>'+
    (reorder&&index>0?'<button type="button" data-near-action="prioritize" '+attrs+' title="'+h(u,'Повысить приоритет')+'" aria-label="'+h(u,'Повысить приоритет')+'">'+u.icon('chevron-up')+'</button>':'')+
    '<button type="button" data-near-action="remove" '+attrs+' title="'+h(u,'Убрать')+'" aria-label="'+h(u,'Убрать')+' '+u.esc(label)+'">'+u.icon('x')+'</button></span>';
}
function savedDisclosure(u,s,key,title,body) {
  return '<details class="near-options" data-near-disclosure="'+key+'"'+(s.details[key]?' open':'')+'><summary>'+h(u,title)+'</summary>'+body+'</details>';
}
function properties(u,s,showStates=false) {
  return grid(
    select(u,s,'systemAllegiance','Принадлежность системы',options.allegiance),
    select(u,s,'population','Население',population),
    select(u,s,'systemGovernment','Правительство системы',options.government),
    select(u,s,'security','Безопасность',options.security),
    select(u,s,'systemEconomy','Экономика системы',options.economy),
    select(u,s,'colonization','Колонизация',colonization),
    select(u,s,'power','Держава',options.power),
    select(u,s,'powerState','Состояние Пауэрплей',options.powerState),
    showStates?chips(u,s,'factionStates','Состояние контролирующей фракции',options.factionStates):'',
    showStates?select(u,s,'stateMode','Совпадение состояний',logic,null):'',
    input(u,s,'presenceFaction','Малая фракция в системе','maxlength="150"'),
    select(u,s,'presenceMode','Присутствие фракции',presence,null),
  );
}
function origin(u,s) {
  const region = ['stations','factions','systems','bodies','signals'].includes(s.id) ?
    select(u,s,'region','Регион Галактики',options.regions,'Любой регион') : '';
  return grid(input(u,s,'origin','Ближайшая система','type="search" data-system-search="single" maxlength="150"'),
    region, input(u,s,'radiusLy','Радиус поиска, св. л.','type="number" min="1" max="50000" step="1" placeholder="'+h(u,'Любой')+'"'+(s.values.region?' disabled':'')),
    ['stations','factions','systems','bodies','misc'].includes(s.id) ? input(u,s,'name','Название','type="search" maxlength="150"'):'');
}
function stationLimits(u,s) {
  return [
    select(u,s,'minPad','Минимальная посадочная площадка',pad),
    input(u,s,'maxArrivalLs','До станции, не более, св. с.','type="number" min="0" max="10000000" step="1"'),
    select(u,s,'surface','Наземные станции',ground,null),
    check(u,s,'ignoreCarriers','Исключить флотоносцы'),
  ];
}
function stations(u,s) {
  const services=list(s.groups.services);
  return grid(
    select(u,s,'allegiance','Принадлежность станции',options.allegiance),
    select(u,s,'government','Правительство станции',options.government),
    select(u,s,'economy','Экономика станции',options.economy),
    select(u,s,'stationType','Тип станции',options.stationTypes),
    ...stationLimits(u,s),span(chips(u,s,'services','Услуги станции',options.services)),
    services.includes('Material Trader')?select(u,s,'materialTrader','Тип торговца материалами',['Raw','Manufactured','Encoded']):'',
    services.includes('Technology Broker')?select(u,s,'technologyBroker','Тип технологического брокера',['Human','Guardian']):'',
  ) + savedDisclosure(u,s,'owner','Владелец станции',grid(
    input(u,s,'owner','Название фракции','maxlength="150"'),
    select(u,s,'ownerMode','Участие фракции',[['owner','Владелец'],['not','Не владелец']],null),
    select(u,s,'ownerAllegiance','Принадлежность владельца',options.allegiance),
    select(u,s,'ownerGovernment','Правительство владельца',options.government),
    check(u,s,'ownerPlayerOnly','Только фракции игроков'),
    check(u,s,'ownerControllingOnly','Только контролирующие фракции'),
    chips(u,s,'ownerStates','Состояние владельца',options.factionStates),
    select(u,s,'ownerStateMode','Совпадение состояний',logic,null),
  )) + savedDisclosure(u,s,'systems','Свойства звёздной системы',properties(u,s));
}
function moduleLabel(u,module) {
  return [[module.class,module.rating].filter(item=>item!==undefined).join(''),term(u,module.name),
    module.mount?u.t(({fixed:'Фиксированное',gimballed:'Карданное',turreted:'Турельное'})[module.mount]):''].filter(Boolean).join(' · ');
}
function outfitting(u,s) {
  return grid(span(chips(u,s,'ships','Корабли',options.ships)),
    span('<fieldset class="near-modules"><legend>'+h(u,'Модули')+'</legend>'+
      '<div class="near-module-picker">'+select(u,s,'moduleName','Модуль',options.modules,'Выбрать модуль')+
      select(u,s,'moduleClass','Класс',[0,1,2,3,4,5,6,7,8])+
      select(u,s,'moduleRating','Рейтинг',['A','B','C','D','E','F','G','H','I'])+
      select(u,s,'moduleMount','Крепление',[['fixed','Фиксированное'],['gimballed','Карданное'],['turreted','Турельное']])+
      button(u,'module','Добавить','plus',s.values.moduleName?'':'disabled')+'</div>'+
      '<div class="near-chips">'+list(s.groups.modules).map((item,index)=>chip(u,'modules',index,moduleLabel(u,item))).join('')+
      '</div></fieldset>'),
    ...stationLimits(u,s),
    select(u,s,'stronghold','Флотоносцы оплота',[['any','Включить'],['exclude','Исключить'],['friendly','Только своей державы']],null),
    check(u,s,'discountedOnly','Только со скидкой'),
    check(u,s,'personalEquipment','Станции с отделом персонального снаряжения'));
}
function factions(u,s) {
  return grid(select(u,s,'factionAllegiance','Принадлежность фракции',options.allegiance),
    select(u,s,'factionGovernment','Правительство фракции',options.government),
    check(u,s,'playerOnly','Только фракции игроков'),check(u,s,'controllerOnly','Только контролирующие фракции'),
    chips(u,s,'factionStates','Состояние фракции',options.factionStates),
    select(u,s,'factionStateMode','Совпадение состояний',logic,null))+
    savedDisclosure(u,s,'systems','Свойства звёздной системы',properties(u,s));
}
function bodies(u,s) {
  const star = value => /Star|dwarf|giant\)|Hole/i.test(value) || /white-yellow|yellow-orange|herbig/i.test(value);
  return grid(select(u,s,'ringKind','Кольца или пояса',[['ring','Кольца'],['belt','Пояса']],null),
    select(u,s,'ringType','Состав колец или поясов',options.rings),
    select(u,s,'reserve','Запасы ресурсов',options.reserve),
    select(u,s,'starClass','Класс звезды',list(options.subtype).filter(star)),
    select(u,s,'planetType','Тип планеты',list(options.subtype).filter(value=>!star(value))),
    select(u,s,'volcanism','Вулканизм',options.volcanism),
    select(u,s,'atmosphere','Атмосфера',options.atmosphere),
    select(u,s,'terraforming','Терраформирование',options.terraforming),
    check(u,s,'tidalLock','Синхронное вращение'),check(u,s,'landable','Только доступные для посадки'),
    chips(u,s,'materials','Материалы по приоритету',options.materials),
    select(u,s,'sort','Сортировать по',[['distance','Дистанция от системы'],['arrival','Расстояние от звезды'],['materials','Доля материалов']],null),
    select(u,s,'power','Держава',options.power),
    input(u,s,'presenceFaction','Малая фракция в системе','maxlength="150"'));
}
function form(u,s) {
  const specific = {
    stations:()=>stations(u,s),outfitting:()=>outfitting(u,s),factions:()=>factions(u,s),
    systems:()=>properties(u,s,true),bodies:()=>bodies(u,s),
    signals:()=>grid(span(select(u,s,'signal','Сигнал',signals,null)))+
      savedDisclosure(u,s,'systems','Свойства звёздной системы',properties(u,s)),
    misc:()=>grid(span(select(u,s,'kind','Что искать',modes,null))),
  }[s.id]();
  return '<form id="nearest-form" class="near-form panel">'+origin(u,s)+specific+
    '<div class="near-actions"><button type="submit" class="pill primary"'+(s.pending?' disabled':'')+'>'+
    u.icon('search')+h(u,'Поиск')+'</button>'+button(u,'clear','Сбросить фильтры')+
    (s.pending?button(u,'cancel','Отменить'):'')+'</div></form>';
}

const commonColumns = [['system','Система'],['distanceLy','Расстояние, св. л.']];
const tableColumns = {
  stations: [['name','Станция'],...commonColumns,['type','Тип станции'],['pad','Площадка'],['distanceLs','До станции, св. с.'],['faction','Владелец'],['services','Услуги станции']],
  outfitting: [['name','Станция'],...commonColumns,['pad','Площадка'],['distanceLs','До станции, св. с.'],['items','Оснащение'],['updatedAt','Наблюдение']],
  factions: [['name','Фракция'],...commonColumns,['allegiance','Принадлежность'],['government','Правительство'],['influence','Влияние'],['states','Состояния']],
  systems: [['system','Система'],['distanceLy','Расстояние, св. л.'],['allegiance','Принадлежность'],['government','Правительство'],
    ['economy','Экономика'],['security','Безопасность'],['population','Население'],['faction','Контролирующая фракция'],['power','Держава']],
  bodies: [['name','Тело'],...commonColumns,['type','Тип'],['distanceLs','От звезды, св. с.'],['gravity','Гравитация, g'],['materials','Материалы'],['rings','Кольца или пояса']],
  signals: [['name','Сигнал'],...commonColumns,['body','Тело'],['type','Тип'],['distanceLs','От звезды, св. с.']],
};
const miscColumns={
  conflicts: [...commonColumns,['faction','Фракция'],['opponent','Соперник'],['state','Состояние'],['surfaceConflicts','Наземные конфликты']],
  'multi-state': [['name','Фракция'],...commonColumns,['states','Состояния']],
  engineers: [['name','Инженер'],...commonColumns,['station','База']],
  restore: [...commonColumns,['station','Станция'],['state','Состояние']],
  mining: [...commonColumns,['faction','Фракция'],['state','Состояние']],
  massacre: [...commonColumns,['faction','Фракция'],['state','Состояние']],
};
const fmt = (u,value) => value!==null && value!==undefined && value!=='' && Number.isFinite(Number(value)) ? u.esc(u.fmt(Number(value))) : '—';
function copySystem(u,row) {
  const system=text(row.system);
  if (!system) return '—';
  return '<span class="near-system"><button type="button" class="table-entity-link" data-near-action="system" data-system="'+u.esc(system)+'">'+u.esc(system)+'</button>'+
    '<button type="button" class="icon-button" data-near-action="copy" data-system="'+u.esc(system)+'" title="'+h(u,'Копировать систему')+
    '" aria-label="'+h(u,'Копировать систему')+'">'+u.icon('copy')+'</button></span>';
}
function comma(u,values) {
  return list(values).map(value=>typeof value==='string'?term(u,value):term(u,value?.name || value?.type || value?.state)).filter(Boolean).map(u.esc).join(' · ');
}
function itemList(u,row) {
  const items=list(row.items);
  if (!items.length) return '—';
  const rows=items.map(item=>{
    const title = item.class!=null || item.rating ? moduleLabel(u,item) : term(u,item.name || item.item);
    const discount = item.discounted && item.basePrice > 0 ? ' · −'+fmt(u,100-100*item.price/item.basePrice)+'%' : '';
    const amount = item.price==null?'':fmt(u,item.price)+' '+h(u,'кр.')+discount;
    return '<li><span>'+u.esc(title)+'</span>'+(amount?'<span>'+amount+'</span>':'')+'</li>';
  });
  return '<ul class="near-stock">'+rows.join('')+'</ul>';
}
function extras(u,row,key,query) {
  if (key==='services') {
    const types = [row.materialTrader,row.technologyBroker].filter(Boolean).map(value=>term(u,value));
    const services = list(row.services);
    const label = comma(u,services);
    return label || types.length ? '<details class="near-result-details"><summary>'+h(u,'Услуги')+
      (types.length ? ' · '+u.esc(types.join(' · ')) : '')+'</summary><span>'+label+'</span></details>' : '—';
  }
  if (key==='materials') {
    const wanted=list(query.filters?.materials);
    const materials=[...list(row.materials)];
    if (!materials.length) return '—';
    const rank = name => wanted.includes(name) ? wanted.indexOf(name) : wanted.length;
    materials.sort((a,b)=>rank(a.name)-rank(b.name));
    const labels=materials.map(item=>u.esc(term(u,item.name))+(Number.isFinite(Number(item.share))?' '+fmt(u,item.share)+'%':''));
    return labels.length > 4 ? '<details class="near-result-details"><summary>'+labels.slice(0,3).join(' · ')+'</summary>'+
      labels.slice(3).join(' · ')+'</details>' : labels.join(' · ');
  }
  if (key==='rings') return comma(u,[...list(row.rings),...list(row.belts)]) || '—';
  return '';
}
function cell(u,row,key,query) {
  const value=row[key];
  if (key==='system') return copySystem(u,row);
  if (key==='items') return itemList(u,row);
  const extra=extras(u,row,key,query);
  if (extra) return extra;
  if (key==='name' && row.referenceId) {
    return '<button type="button" class="table-entity-link" data-near-action="reference" data-id="'+u.esc(row.referenceId)+'">'+
      u.esc(!ru(u)&&row.nameEn||value)+'</button>';
  }
  if (key==='name' && /^https:\/\/spansh\.co\.uk\/station\/[0-9]+$/.test(row.url || '')) {
    return '<button type="button" class="table-entity-link" data-near-action="external" data-url="'+u.esc(row.url)+'">'+u.esc(value)+'</button>';
  }
  if (key==='updatedAt') {
    const date = Date.parse(value);
    return Number.isFinite(date)?u.esc(u.date?.(value,{dateStyle:'short',timeStyle:'short'}) || new Date(date).toLocaleDateString(u.locale())):'—';
  }
  if (key==='influence') return value!=null?fmt(u,Number(value)*100)+'%':'—';
  if (['distanceLy','distanceLs','population','gravity','surfaceConflicts'].includes(key)) return fmt(u,value);
  if (key==='states') return comma(u,value) || display(u,term(u,row.state || '—'));
  if (key==='name' && value && row.count != null && row.count !== 1) return display(u,value)+' <span class="muted">×'+fmt(u,row.count)+'</span>';
  return value!==null&&value!==undefined&&value!=='' ? u.esc(term(u,String(value))) : '—';
}
function table(u,s,rows) {
  const query=s.executed || {};
  const columns=tableColumns[s.id]||miscColumns[query.filters?.kind||s.values.kind]||commonColumns;
  const visible=columns.filter(([key])=>['name','system','distanceLy','type','items'].includes(key) || rows.some(row=>{
    const value=row[key];
    return Array.isArray(value)?value.length>0 : value!=null && value!=='';
  }));
  return '<div class="near-table panel"><table><thead><tr>'+visible.map(([,label])=>'<th scope="col">'+h(u,label)+'</th>').join('')+
    '</tr></thead><tbody>'+rows.map(row=>'<tr>'+visible.map(([key])=>'<td>'+cell(u,row,key,query)+'</td>').join('')+'</tr>').join('')+
    '</tbody></table></div>';
}
function result(u,s) {
  const data=s.result;
  if (!data&&!s.pending&&!s.error) return '';
  const rows=list(data?.rows);
  const title=s.pending?'Ищем ближайшие объекты…':s.error || {
    'unavailable':'Сервис поиска временно недоступен.',
    'invalid':'Проверьте исходную систему и параметры поиска.',
    'disabled':'Сетевой поиск отключён.',
  }[data?.status] || '';
  const note=data?.note;
  const link=data?.link && /^https:\/\/inara\.cz\/elite\//.test(data.link)?data.link:'';
  const external=link && (note||data?.status!=='ok'||s.id==='outfitting'&&!rows.length) ?
    button(u,'inara','Открыть поиск в Inara','arrow-up-right','data-url="'+u.esc(link)+'"'):'';
  const pager=data && (data.hasMore||s.page>0)?'<nav class="near-pager" aria-label="'+h(u,'Страницы результатов')+'">'+
    button(u,'previous','Назад','chevron-left',s.pending||!s.page?'disabled':'')+
    '<span>'+h(u,'Страница {page}',{page:u.fmt(s.page+1)})+'</span>'+
    button(u,'next','Далее','chevron-right',s.pending||!data.hasMore?'disabled':'')+'</nav>':'';
  const status=title?'<p role="status" class="near-status">'+h(u,title)+'</p>':
    (note?'<p role="status" class="near-status">'+h(u,note)+'</p>':'');
  const resultsNote=!title&&data?.status==='ok'&&!note&&!rows.length ? '<p role="status" class="near-status">'+h(u,'Ничего не найдено.')+'</p>':'';
  return '<section class="near-results" aria-label="'+h(u,'Результаты поиска')+'">'+
    '<header><h2>'+h(u,'Результаты поиска')+'</h2>'+external+'</header>'+status+
    (rows.length?table(u,s,rows):resultsNote)+pager+'</section>';
}
function content(C,u) {
  const s=session(C);
  const nav='<nav class="segmented near-tabs" aria-label="'+h(u,'Виды поиска')+'">'+sections.map(([id,title,icon])=>
    button(u,'section',title,icon,'data-section="'+id+'" aria-pressed="'+(s.id===id)+'"'+(s.id===id?' aria-current="page"':''))).join('')+'</nav>';
  return nav+form(u,s)+result(u,s);
}
function render(C,u) {
  return u.heading(u.t('Поиск ближайших'),'')+'<div id="nearest-workspace" class="nearest-workspace">'+content(C,u)+'</div>';
}
function draw(C,u,focus='') {
  const host=active(C)&&u.$('#nearest-workspace');
  if (!host) return;
  host.innerHTML=content(C,u);
  if (focus) host.querySelector(focus)?.focus({preventScroll:true});
}
function parameters(C,s) {
  const {origin,name='',region='',sort='distance',moduleName,moduleClass,moduleRating,moduleMount,...filters}=s.values;
  for (const key of ['radiusLy','maxArrivalLs','populationMin','populationMax']) {
    if (filters[key] !== '' && filters[key] != null) filters[key]=Number(filters[key]);
    else delete filters[key];
  }
  for (const key of Object.keys(filters)) if (filters[key]==='' || filters[key]===false) delete filters[key];
  if (!list(s.groups.services).includes('Material Trader')) delete filters.materialTrader;
  if (!list(s.groups.services).includes('Technology Broker')) delete filters.technologyBroker;
  if (region) delete filters.radiusLy;
  const pledged=C.state?.commander?.power || C.state?.pilot?.power;
  if (typeof pledged==='string') filters.pledgedPower=pledged;
  return {category:s.id,origin:text(origin),name:text(name),region:text(region),sort,filters:{...filters,...s.groups}};
}
function canSearch(s) {
  if (s.id!=='outfitting') return true;
  return Boolean(s.values.personalEquipment || list(s.groups.modules).length || list(s.groups.ships).length);
}
function cancel(s,u) {
  if (!s.pending) return;
  const id=s.pending;
  s.pending=null;
  if (!s.result) s.primed=false;
  void u.call('nearest.cancel',{sequence:id}).catch(()=>{});
}
async function search(C,u,s,page=0,repeat=false) {
  if (s.pending) cancel(s,u);
  s.primed=true;
  if (!canSearch(s)) {
    s.error='Выберите корабль, модуль или отдел персонального снаряжения.';
    s.result=null; draw(C,u);
    return;
  }
  const base=repeat&&s.executed?s.executed:parameters(C,s);
  if (!base.origin&&!base.region) {
    s.error='Укажите систему или регион Галактики.';
    s.result=null; draw(C,u);
    return;
  }
  const id=++sequence;
  s.error='';s.result=null;s.pending=id;s.page=page;s.executed=base;
  draw(C,u);
  try {
    const response=await u.call('nearest.search',{...base,page,sequence:id});
    if (s.pending===id) s.result=response;
  } catch (error) {
    if (s.pending===id) s.error=error.message || 'Сервис поиска временно недоступен.';
  } finally {
    if(s.pending===id) {
      s.pending=null;
      if (active(C)&&session(C)===s) draw(C,u);
    }
  }
}
function prime(C,u,s) {
  if (!s.primed&&s.id!=='outfitting') void search(C,u,s);
}
function leave(C,u) {
  if (!active(C)&&C.nearestUI) cancel(session(C),u);
}
function click(C,u,event) {
  const control=event.target.closest('[data-near-action]');
  if (!control) return;
  const s=session(C), {nearAction,section,group,index,system,id,url}=control.dataset;
  switch (nearAction) {
    case 'section': {
      if (!validPage.has(section)||section===s.id) return;
      cancel(s,u);
      nearbyModel(C).section=section;
      const next=session(C);
      draw(C,u,'[data-section="'+section+'"]');
      prime(C,u,next);
      return;
    }
    case 'previous': case 'next':
      void search(C,u,s,s.page+(nearAction==='next'?1:-1),true);return;
    case 'cancel':
      cancel(s,u);s.primed=true;draw(C,u);return;
    case 'clear':
      cancel(s,u);delete nearbyModel(C).sessions[s.id];draw(C,u);prime(C,u,session(C));return;
    case 'remove': case 'prioritize': {
      const items=list(s.groups[group]).slice(), offset=Number(index);
      if (!Number.isInteger(offset)||offset<0||offset>=items.length) return;
      if (nearAction==='remove') items.splice(offset,1);
      else if (offset>0) [items[offset-1],items[offset]]=[items[offset],items[offset-1]];
      s.groups[group]=items;draw(C,u);return;
    }
    case 'module': {
      const {moduleName, moduleClass='',moduleRating='',moduleMount=''}=s.values;
      if (!moduleName) return;
      const item={name:moduleName,...(moduleClass!==''?{class:Number(moduleClass)}:{}),
        ...(moduleRating?{rating:moduleRating}:{}),...(moduleMount?{mount:moduleMount}:{})};
      const items=list(s.groups.modules);
      if (items.length<40&&!items.some(known=>JSON.stringify(known)===JSON.stringify(item))) s.groups.modules=[...items,item];
      s.values.moduleName='';draw(C,u,'[name="moduleName"]');return;
    }
    case 'copy':
      void u.call('native',{action:'copy',text:system}).then(()=>u.toast(u.t('Система скопирована'))).catch(error=>u.toast(error.message,true));return;
    case 'inara': case 'external':
      void u.call('native',{action:'open',url}).catch(error=>u.toast(error.message,true));return;
    case 'system': case 'reference': {
      const entry=id || 'system/'+encodeURIComponent(system);
      const mode=nearAction==='system'?'Системы':'Инженеры';
      void u.navigate('encyclopedia',{mode}).then(()=>window.ReferenceUI.open(entry,C,u)).catch(error=>u.toast(error.message,true));
    }
  }
}
function edit(C,u,field,commit) {
  const s=session(C), key=field.name;
  if (!key) return;
  if (field.dataset.nearAdd) {
    if (!commit||!field.value) return;
    const group=field.dataset.nearAdd, items=list(s.groups[group]);
    if (!items.includes(field.value)&&items.length<(group==='ships'?40:15)) s.groups[group]=[...items,field.value];
    draw(C,u,'[name="'+key+'"]');return;
  }
  s.values[key]=field.type==='checkbox'?field.checked:field.value;
  if (!commit) return;
  if (key==='region') {
    s.values.origin=field.value?'':(s.values.origin || C.state?.commander?.system || 'Sol');
    const originField=field.form?.elements.origin;
    if (originField) originField.value=s.values.origin;
    const radius=field.form?.elements.radiusLy;
    if (radius) radius.disabled=Boolean(field.value);
  }
  if (key==='origin'&&field.value) {
    s.values.region='';
    const regionField=field.form?.elements.region;
    if (regionField) regionField.value='';
    const radius=field.form?.elements.radiusLy;
    if (radius) radius.disabled=false;
  }
  if (key==='moduleName') draw(C,u,'[name="moduleName"]');
}
function init(C,u) {
  if (!active(C)) return;
  const host=u.$('#nearest-workspace');
  if (!host||host.nearBound) return;
  host.nearBound=true;
  host.addEventListener('click',event=>click(C,u,event));
  host.addEventListener('input',event=>edit(C,u,event.target,false));
  host.addEventListener('change',event=>edit(C,u,event.target,true));
  host.addEventListener('submit',event=>{
    if (event.target.id!=='nearest-form') return;
    event.preventDefault();
    void search(C,u,session(C));
  });
  host.addEventListener('toggle',event=>{
    const key=event.target.dataset?.nearDisclosure;
    if (key && host.contains(event.target)) session(C).details[key]=event.target.open;
  },true);
  prime(C,u,session(C));
}

window.NearestUI={active,render,init,leave};
})();
