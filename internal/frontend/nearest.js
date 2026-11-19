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
const maxOutfitItems = 40;

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
  ['navigation-beacon','Навигационный маяк','Nav Beacon','Орбитальные сигналы'],
  ['compromised-nav-beacon','Скомпрометированный навигационный маяк','Compromised Nav Beacon','Орбитальные сигналы'],
  ['any-resource-extraction','Места добычи ресурсов · все уровни','RES','Орбитальные сигналы'],
  ['resource-extraction-low','Место добычи ресурсов · низкая интенсивность','Low RES','Орбитальные сигналы'],
  ['resource-extraction','Место добычи ресурсов · обычная интенсивность','RES','Орбитальные сигналы'],
  ['resource-extraction-high','Место добычи ресурсов · высокая интенсивность','High RES','Орбитальные сигналы'],
  ['resource-extraction-haz','Место добычи ресурсов · опасное','Hazardous RES','Орбитальные сигналы'],
  ['orbital-tourist-beacon','Орбитальный туристический маяк','Tourist Beacon','Орбитальные сигналы'],
  ['colonization-beacon','Маяк колонизации','Colonisation Beacon','Орбитальные сигналы'],
  ['installation','Космический объект','Installation','Орбитальные сигналы'],
  ['megaship','Мегакорабль с посадкой','Megaship','Орбитальные сигналы'],
  ['scannable-megaship','Сканируемый мегакорабль','Scannable Megaship','Орбитальные сигналы'],
  ['listening-post','Пост прослушивания','Listening Post','Орбитальные сигналы'],
  ['unregistered-beacon','Незарегистрированный маяк связи','Number Station','Орбитальные сигналы'],
  ['stellar-phenomena','Звёздные феномены','Notable Stellar Phenomena','Орбитальные сигналы'],
  ['thargoid-titan','Сигнал титана таргоидов','Thargoid Titan','Орбитальные сигналы'],
  ['titan-wreck','Титаны таргоидов · обломки','Titan wreck','Орбитальные сигналы'],
  ['guardian-beacon','Маяк Стражей','Guardian Beacon','Сигналы Стражей и таргоидов'],
  ['guardian-ruin','Руины Стражей','Guardian Ruin','Сигналы Стражей и таргоидов'],
  ['guardian-structure','Структура Стражей','Guardian Structure','Сигналы Стражей и таргоидов'],
  ['thargoid-structure','Структура таргоидов','Thargoid Structure','Сигналы Стражей и таргоидов'],
  ['thargoid-spire','Шпиль таргоидов','Thargoid Spire','Сигналы Стражей и таргоидов'],
  ['thargoid-barnacle','Барнакл таргоидов','Thargoid Barnacle','Сигналы Стражей и таргоидов'],
  ['barnacle-matrix','Матрица барнаклов','Thargoid Barnacle Matrix','Сигналы Стражей и таргоидов'],
  ['tourist-beacon','Туристический маяк','Tourist Beacon','Поверхность'],
  ['wrecked-ship','Обломки корабля','Wrecked Ship','Поверхность'],
  ['crashed-ship','Разбившийся корабль','Crashed Ship','Поверхность'],
  ['abandoned-base','Заброшенная база','Abandoned Base','Поверхность'],
  ['planetary-mining','Участок планетарной добычи','Planetary Mining Location','Поверхность'],
  ['biological','Биологические сигналы','Biological','Поверхность'],
  ['geological','Геологические сигналы','Geological','Поверхность'],
  ['human','Сигналы людей','Human','Поверхность'],
  ['guardian','Сигналы Стражей','Guardian','Поверхность'],
  ['thargoid','Сигналы таргоидов','Thargoid','Поверхность'],
  ['major-anomaly','Крупная аномалия','Major Anomaly','Поверхность'],
];
const signalTypeLabels = {
  'Nav Beacon':'Навигационный маяк','Compromised Nav Beacon':'Скомпрометированный навигационный маяк',
  'Resource Extraction Site':'Место добычи ресурсов · обычная интенсивность','Low Resource Extraction Site':'Место добычи ресурсов · низкая интенсивность',
  'High Resource Extraction Site':'Место добычи ресурсов · высокая интенсивность','Hazardous Resource Extraction Site':'Место добычи ресурсов · опасное',
  'Orbital Tourist Beacon':'Орбитальный туристический маяк','Colonisation Beacon':'Маяк колонизации','Installation':'Космический объект',
  'Scannable Megaship':'Сканируемый мегакорабль','Listening Post':'Пост прослушивания','Unregistered Communications Beacon':'Незарегистрированный маяк связи',
  'Notable Stellar Phenomena':'Звёздные феномены','Thargoid Titan':'Сигнал титана таргоидов',
  'Guardian Beacon':'Маяк Стражей','Guardian Ruin':'Руины Стражей','Guardian Structure':'Структура Стражей',
  'Thargoid Structure':'Структура таргоидов','Thargoid Spires':'Шпиль таргоидов','Thargoid Barnacle':'Барнакл таргоидов',
  'Thargoid Barnacle Matrix':'Матрица барнаклов','Tourist Beacon':'Туристический маяк','Wrecked Ship':'Обломки корабля',
  'Crashed Ship':'Разбившийся корабль','Abandoned Base':'Заброшенная база','Planetary Mining Location':'Участок планетарной добычи',
  'Biological':'Биологические сигналы','Geological':'Геологические сигналы','Major Anomaly':'Крупная аномалия',
};

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
  return ru(u) ? options.moduleLabels?.[key] || options.stationTerms?.[key] || options.spaceLabels?.[key] || terms[key] || u.t(key) : key;
}
const optPair = (u,item) => Array.isArray(item)?[item[0],item[0]===item[1]?term(u,item[0]):u.t(item[1])]:[String(item),term(u,String(item))];
const field = (u,label,content) => '<label class="near-field"><span>'+h(u,label)+'</span>'+content+'</label>';
function option(u,key,label,selected,aliases) {
  const search = Array.isArray(aliases) ? aliases.join(' ') : aliases;
  return '<option value="'+u.esc(key)+'"'+(String(key)===String(selected)?' selected':'')+
    (search?' data-near-alias="'+u.esc(search)+'"':'')+'>'+u.esc(label)+'</option>';
}
function selectControl(u,key,items,value,blank,attrs='',label='') {
  const choices = list(items).map(item=>[...optPair(u,item), Array.isArray(item) ? item[2] : '', Array.isArray(item) ? item[3] : '']);
  if (blank!=null) choices.unshift(['',u.t(blank),'','']);
  let previousGroup = '', markup = '';
  for (const [id,title,aliases,group] of choices) {
    if (group!==previousGroup) {
      if (previousGroup) markup+='</optgroup>';
      if (group) markup+='<optgroup label="'+h(u,group)+'">';
      previousGroup=group;
    }
    markup+=option(u,id,title,value,aliases);
  }
  if (previousGroup) markup+='</optgroup>';
  const searchable=choices.length>=9 || key==='stationType';
  const selectMarkup='<select name="'+key+'" '+attrs+(searchable?' hidden':'')+'>'+markup+'</select>';
  if (!searchable) return selectMarkup;
  const selected=choices.find(([id])=>String(id)===String(value))?.[1] || choices[0]?.[1] || '';
  return '<span class="near-picker">'+selectMarkup+'<button type="button" data-near-picker data-near-label="'+h(u,label)+
    '" aria-haspopup="listbox" aria-expanded="false" aria-label="'+u.esc([u.t(label),selected].filter(Boolean).join(' · '))+
    '"><span>'+u.esc(selected)+'</span>'+u.icon('chevron-down')+'</button></span>';
}
function select(u,s,key,label,items,blank='Любое значение') {
  return field(u,label,selectControl(u,key,items,s.values[key],blank,'',label));
}
function effectiveSort(s, sort=s.values.sort||'distance') {
  if (sort==='arrival'&&isOrbitalStation(s)||sort==='materials'&&!list(s.groups.materials).length) sort='distance';
  return sort==='distance'&&s.values.region?'name':sort;
}
function sortSelect(u,s,items) {
  const choices=items.filter(([id])=>(id!=='distance'||!s.values.region)&&(id!=='materials'||list(s.groups.materials).length));
  return field(u,'Сортировать по',selectControl(u,'sort',choices,effectiveSort(s),null,'','Сортировать по'));
}
function input(u,s,key,label,attrs='') {
  const lookup = key==='owner'||key==='presenceFaction' ? 'faction' :
    key==='name' ? {stations:'station',outfitting:'station',factions:'faction',systems:'single',bodies:'body',signals:signalName(s)[1]}[s.id] : '';
  if (lookup) attrs+=' data-system-search="'+lookup+'"';
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
      ownerMode:'owner',ownerStateMode:'all',factionStateMode:'all',stateMode:'all',
      stronghold:'any',kind:'conflicts',signal:'tourist-beacon',minimumSources:'1',minimumStates:'2'},
    groups:{},details:{},page:0,primed:false,
  };
  return model.sessions[id];
}
const grid = (...fields) => '<div class="near-grid">'+fields.filter(Boolean).join('')+'</div>';
const span = html => '<div class="near-span">'+html+'</div>';
function chips(u,s,key,label,items) {
  const chosen=list(s.groups[key]);
  return '<div class="near-multiple"><label><span>'+h(u,label)+'</span>'+
    selectControl(u,'add-'+key,list(items).filter(item=>!chosen.includes(Array.isArray(item)?item[0]:item)),'','Добавить','data-near-add="'+key+'"',label)+
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
    input(u,s,'populationMin','Минимальное население','type="number" min="0" max="1000000000000000" step="1"'),
    input(u,s,'populationMax','Максимальное население','type="number" min="0" max="1000000000000000" step="1"'),
    showStates?select(u,s,'secondaryEconomy','Вторичная экономика',options.economy):'',
    showStates?select(u,s,'permit','Разрешение на вход',[['open','Без разрешения'],['required','Требуется разрешение']]):'',
    showStates?sortSelect(u,s,[['distance','Дистанция от системы'],['population','Население'],['name','Название']]):'',
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
    ['stations','factions','systems','bodies','signals','misc'].includes(s.id) ?
      input(u,s,'name',s.id==='signals'?signalName(s)[0]:'Название','type="search" maxlength="150"'):'');
}
function signalName(s) {
  if (s.values.signal==='megaship') return ['Название мегакорабля','station'];
  if (s.values.signal==='titan-wreck') return ['Имя титана или система',''];
  if (signals.find(item=>item[0]===s.values.signal)?.[3]==='Орбитальные сигналы') return ['Название сигнала или системы',''];
  return ['Название небесного тела','body'];
}
function stationArrival(u,s,label='До станции, не более, св. с.') {
  return input(u,s,'maxArrivalLs',label,'type="number" min="0" max="10000000" step="1"');
}
function stationLimits(u,s) {
  return [
    select(u,s,'minPad','Минимальная посадочная площадка',pad),
    stationArrival(u,s),
    select(u,s,'surface','Наземные станции',ground,null),
    check(u,s,'ignoreCarriers','Исключить флотоносцы'),
  ];
}
const stationEconomy = [...list(options.economy), 'Private Enterprise', 'Rescue', 'Prison'];
const stationGovernment = [...list(options.government), 'Engineer', 'Megaconstruction', 'Private Ownership'];
const stationAllegiance = [...list(options.allegiance), 'Frontline Solutions'];
const stationSort = [['distance','Дистанция от системы'],['arrival','Расстояние от звезды'],['name','По названию станции']];
const orbitalStationTypes = ['orbital-installations','scannable-megaships'];
const isOrbitalStation = s => orbitalStationTypes.includes(s.values.stationType);
const isTitan = s => s.values.stationType==='thargoid-titans';
const stationType = (u,s) => select(u,s,'stationType','Тип станции',options.stationTypes);
function stations(u,s) {
  if (isOrbitalStation(s) || isTitan(s)) {
    const orbital = isOrbitalStation(s);
    const sort = orbital ? stationSort.filter(([key])=>key!=='arrival') : stationSort;
    return grid(stationType(u,s),sortSelect(u,s,sort),!orbital?stationArrival(u,s,'От звезды, не более, св. с.'):'')+
      savedDisclosure(u,s,'systems','Свойства звёздной системы',properties(u,s));
  }
  const services=list(s.groups.services);
  return grid(
    select(u,s,'allegiance','Принадлежность станции',stationAllegiance),
    select(u,s,'government','Правительство станции',stationGovernment),
    select(u,s,'economy','Экономика станции',stationEconomy),
    stationType(u,s),
    ...stationLimits(u,s),sortSelect(u,s,stationSort),
    span(chips(u,s,'services','Услуги станции',options.services)),
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
const outfitCount = s => list(s.groups.ships).length + list(s.groups.modules).length;
function outfitting(u,s) {
  return grid(span(chips(u,s,'ships','Корабли',options.ships)),
    span('<fieldset class="near-modules"><legend>'+h(u,'Модули')+'</legend>'+
      '<div class="near-module-picker">'+select(u,s,'moduleName','Модуль',options.modules,'Выбрать модуль')+
      select(u,s,'moduleClass','Класс',[0,1,2,3,4,5,6,7,8])+
      select(u,s,'moduleRating','Рейтинг',['A','B','C','D','E','F','G','H','I'])+
      select(u,s,'moduleMount','Крепление',[['fixed','Фиксированное'],['gimballed','Карданное'],['turreted','Турельное']])+
      button(u,'module','Добавить','plus',s.values.moduleName&&outfitCount(s)<maxOutfitItems?'':'disabled')+'</div>'+
      '<div class="near-chips">'+list(s.groups.modules).map((item,index)=>chip(u,'modules',index,moduleLabel(u,item))).join('')+
      '</div></fieldset>'),
    input(u,s,'name','Название станции','type="search" maxlength="150"'),
    select(u,s,'stationType','Тип станции',list(options.stationTypes).filter(([key])=>key!=='thargoid-titans'&&!orbitalStationTypes.includes(key))),
    ...stationLimits(u,s),sortSelect(u,s,stationSort),
    s.values.ignoreCarriers?'':select(u,s,'stronghold','Флотоносцы оплота',[['any','Включить'],['exclude','Исключить'],['friendly','Только своей державы']],null),
    outfitCount(s)?check(u,s,'discountedOnly','Только со скидкой'):'',
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
  const compositions = [['any','Любой состав'],...options.rings];
  return grid(select(u,s,'ringType','Тип колец',compositions),
    select(u,s,'beltType','Тип астероидных поясов',compositions),
    select(u,s,'reserve','Запасы ресурсов',options.reserve),
    select(u,s,'bodyKind','Небесное тело',[['Star','Звезда'],['Planet','Планета или луна']]),
    s.values.bodyKind!=='Planet'?select(u,s,'starClass','Класс звезды',options.starTypes):'',
    s.values.bodyKind!=='Star'?select(u,s,'planetType','Тип планеты',options.planetTypes):'',
    select(u,s,'volcanism','Вулканизм',options.volcanismTypes),
    select(u,s,'atmosphere','Атмосфера',options.atmosphereTypes),
    select(u,s,'terraforming','Терраформирование',options.terraforming),
    check(u,s,'tidalLock','Синхронное вращение'),check(u,s,'landable','Только доступные для посадки'),
    chips(u,s,'materials','Материалы по приоритету',options.materials),
    sortSelect(u,s,[['distance','Дистанция от системы'],['arrival','Расстояние от звезды'],
      ['materials','Доля материалов'],['gravity','Гравитация, g'],['mapping','Награда за картографирование'],['name','Название']]),
    select(u,s,'power','Держава',options.power),
    input(u,s,'presenceFaction','Малая фракция в системе','maxlength="150"'))+
    savedDisclosure(u,s,'bodyLimits','Расстояние и условия на поверхности',grid(
      input(u,s,'maxArrivalLs','От звезды, не более, св. с.','type="number" min="0" max="100000000" step="1"'),
      input(u,s,'minGravity','Минимальная гравитация, g','type="number" min="0" max="100" step="0.01"'),
      input(u,s,'maxGravity','Максимальная гравитация, g','type="number" min="0" max="100" step="0.01"'),
      input(u,s,'minTemperature','Минимальная температура, К','type="number" min="0" max="10000000" step="1"'),
      input(u,s,'maxTemperature','Максимальная температура, К','type="number" min="0" max="10000000" step="1"'),
    ));
}
function form(u,s) {
  const missionOptions = () => {
    const kind = s.values.kind;
    return grid(select(u,s,'kind','Что искать',modes,null),
      ['massacre','mining'].includes(kind) ? select(u,s,'missionPad','Посадочная площадка',[['L','Только большая']],'Любая'):'',
      kind==='massacre' ? select(u,s,'minimumSources','Минимум систем выдачи',[1,2,3,4,5].map(n=>[String(n),String(n)]),null):'',
      kind==='multi-state' ? select(u,s,'minimumStates','Минимум активных состояний',[2,3,4,5].map(n=>[String(n),String(n)]),null):'');
  };
  const specific = {
    stations:()=>stations(u,s),outfitting:()=>outfitting(u,s),factions:()=>factions(u,s),
    systems:()=>properties(u,s,true),bodies:()=>bodies(u,s),
    signals:()=>grid(span(select(u,s,'signal','Сигнал',signals,null)))+
      savedDisclosure(u,s,'systems','Свойства звёздной системы',properties(u,s)),
    misc:missionOptions,
  }[s.id]();
  return '<form id="nearest-form" class="near-form panel">'+origin(u,s)+specific+
    '<div class="near-actions"><button type="submit" class="pill primary"'+(s.pending?' disabled':'')+'>'+
    u.icon('search')+h(u,'Поиск')+'</button>'+button(u,'clear','Сбросить фильтры')+
    button(u,'cancel','Отменить','',s.pending?'':'hidden')+'</div></form>';
}

const commonColumns = [['system','Система'],['distanceLy','Расстояние, св. л.']];
const tableColumns = {
  stations: [['name','Станция'],...commonColumns,['type','Тип станции'],['pad','Площадка'],['distanceLs','До станции, св. с.'],['faction','Владелец'],['economy','Экономика станции'],['services','Услуги станции']],
  outfitting: [['name','Станция'],...commonColumns,['type','Тип станции'],['pad','Площадка'],['distanceLs','До станции, св. с.'],['items','Оснащение'],['updatedAt','Наблюдение']],
  factions: [['name','Фракция'],...commonColumns,['allegiance','Принадлежность'],['government','Правительство'],['influence','Влияние'],['states','Состояния'],['controlling','Контролирует систему'],['updatedAt','Наблюдение']],
  systems: [['system','Система'],['distanceLy','Расстояние, св. л.'],['region','Регион Галактики'],['allegiance','Принадлежность'],['government','Правительство'],
    ['economy','Экономика'],['security','Безопасность'],['population','Население'],['faction','Контролирующая фракция'],['power','Держава'],
    ['powerState','Состояние Пауэрплей'],['permitRequired','Разрешение на вход'],['colonization','Колонизация']],
  bodies: [['name','Тело'],...commonColumns,['type','Тип'],['distanceLs','От звезды, св. с.'],['gravity','Гравитация, g'],['materials','Материалы'],
    ['rings','Кольца или пояса'],['atmosphere','Атмосфера'],['volcanism','Вулканизм'],['terraforming','Терраформирование'],
    ['landable','Посадка'],['tidalLock','Синхронное вращение'],['surfaceTemperature','Температура, К'],['estimatedMappingValue','Награда за картографирование']],
  signals: [['name','Сигнал'],...commonColumns,['body','Тело'],['type','Тип'],['distanceLs','От звезды, св. с.'],['latitude','Широта'],['longitude','Долгота'],['updatedAt','Наблюдение']],
};
const miscColumns={
  conflicts: [...commonColumns,['faction','Фракция'],['opponent','Соперник'],['state','Состояние'],['surfaceConflicts','Наземные конфликты']],
  'multi-state': [['name','Фракция'],...commonColumns,['states','Состояния']],
  engineers: [['name','Инженер'],...commonColumns,['station','База']],
  restore: [...commonColumns,['name','Поселение'],['distanceLs','От звезды, св. с.'],['faction','Владелец'],['states','Состояния'],['power','Держава']],
  mining: [...commonColumns,['name','Станция'],['distanceLs','От звезды, св. с.'],['pad','Площадка'],['economy','Экономика'],['type','Тип станции'],['states','Состояния']],
  massacre: [...commonColumns,['pad','Площадка'],['distanceLs','До станции, св. с.'],['destinationSystem','Целевая система'],['sourceSystems','Систем выдачи'],['huntingSites','Места боя']],
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
  if (key==='destinationSystem') return copySystem(u,{system:value});
  if (['controlling','landable','tidalLock'].includes(key) && typeof value==='boolean') return h(u,value?'Да':'Нет');
  if (key==='colonization') {
    if (row.construction===true) return h(u,'В процессе колонизации');
    if (row.colonized===true) return h(u,'Колонизирована');
    return row.colonized===false&&row.construction===false?h(u,'Неколонизирована'):'—';
  }
  if (query.category==='signals' && ru(u) && signalTypeLabels[value] && (key==='type' || key==='name')) return u.esc(signalTypeLabels[value]);
  if (key==='items') return itemList(u,row);
  const extra=extras(u,row,key,query);
  if (extra) return extra;
  if (key==='name' && row.referenceId) {
    return '<button type="button" class="table-entity-link" data-near-action="reference" data-id="'+u.esc(row.referenceId)+'">'+
      u.esc(!ru(u)&&row.nameEn||value)+'</button>';
  }
  if (key==='name' && /^https:\/\/spansh\.co\.uk\/(?:station|body)\/[0-9]+$/.test(row.url || '')) {
    return '<button type="button" class="table-entity-link" data-near-action="external" data-url="'+u.esc(row.url)+'">'+u.esc(value)+'</button>';
  }
  if (key==='updatedAt') {
    const date = Date.parse(value);
    return Number.isFinite(date)?u.esc(u.date?.(value,{dateStyle:'short',timeStyle:'short'}) || new Date(date).toLocaleDateString(u.locale())):'—';
  }
  if (key==='influence') return value!=null?fmt(u,Number(value)*100)+'%':'—';
  if (key==='estimatedMappingValue') return value==null?'—':fmt(u,value)+' '+h(u,'кр.');
  if (key==='permitRequired') return typeof value==='boolean'?h(u,value?'Требуется разрешение':'Без разрешения'):'—';
  if (key==='economy' && row.secondaryEconomy && row.secondaryEconomy!=='None' && row.secondaryEconomy!==value) {
    return comma(u,[value,row.secondaryEconomy]);
  }
  if (['distanceLy','distanceLs','population','gravity','surfaceTemperature','surfaceConflicts','latitude','longitude','sourceSystems'].includes(key)) return fmt(u,value);
  if (key==='states') return comma(u,value) || display(u,term(u,row.state || '—'));
  if (key==='name' && value && row.count != null && row.count !== 1) return display(u,value)+' <span class="muted">×'+fmt(u,row.count)+'</span>';
  return value!==null&&value!==undefined&&value!=='' ? u.esc(term(u,String(value))) : '—';
}
function table(u,s,rows) {
  const query=s.executed || {};
  const columns=tableColumns[s.id]||miscColumns[query.filters?.kind||s.values.kind]||commonColumns;
  const visible=columns.filter(([key])=>['name','system','distanceLy','type','items'].includes(key) || rows.some(row=>{
    if (key==='colonization') return typeof row.colonized==='boolean'||typeof row.construction==='boolean';
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
  const resultsNote=!title&&data?.status==='ok'&&!rows.length ? '<p role="status" class="near-status">'+h(u,'Ничего не найдено.')+'</p>':'';
  return '<section class="near-results" aria-label="'+h(u,'Результаты поиска')+'">'+
    '<header><h2>'+h(u,'Результаты поиска')+'</h2>'+external+'</header>'+status+
    (rows.length?table(u,s,rows):resultsNote)+pager+'</section>';
}
function content(C,u) {
  const s=session(C);
  const nav='<nav class="segmented near-tabs" aria-label="'+h(u,'Виды поиска')+'">'+sections.map(([id,title,icon])=>
    button(u,'section',title,icon,'data-section="'+id+'" aria-pressed="'+(s.id===id)+'"'+(s.id===id?' aria-current="page"':''))).join('')+'</nav>';
  return nav+form(u,s)+'<div data-near-results>'+result(u,s)+'</div>';
}
function render(C,u) {
  return u.heading(u.t('Поиск ближайших'),'')+'<div id="nearest-workspace" class="nearest-workspace">'+content(C,u)+'</div>';
}
function draw(C,u,focus='') {
  const host=active(C)&&u.$('#nearest-workspace');
  if (!host) return;
  window.NearestPicker.close();
  host.innerHTML=content(C,u);
  if (focus) {
    const field=host.querySelector(focus);
    (window.NearestPicker.trigger(field)||field)?.focus({preventScroll:true});
  }
}
function drawResults(C,u,s) {
  if (!active(C)||session(C)!==s) return;
  const host=u.$('#nearest-workspace');
  if (!host) return;
  host.querySelector('[data-near-results]').innerHTML=result(u,s);
  host.querySelector('#nearest-form [type=submit]').disabled=Boolean(s.pending);
  host.querySelector('[data-near-action=cancel]').hidden=!s.pending;
}
function parameters(C,s) {
  const {origin,name='',region='',sort='distance',moduleName,moduleClass,moduleRating,moduleMount,...filters}=s.values;
  for (const key of ['radiusLy','maxArrivalLs','populationMin','populationMax','minGravity','maxGravity','minTemperature','maxTemperature']) {
    if (filters[key] !== '' && filters[key] != null) filters[key]=Number(filters[key]);
    else delete filters[key];
  }
  for (const key of Object.keys(filters)) if (filters[key]==='' || filters[key]===false) delete filters[key];
  if (!list(s.groups.services).includes('Material Trader')) delete filters.materialTrader;
  if (!list(s.groups.services).includes('Technology Broker')) delete filters.technologyBroker;
  if (region) delete filters.radiusLy;
  const selected={...filters,...s.groups};
  if (s.id==='stations' && (isOrbitalStation(s)||isTitan(s))) {
    for (const key of ['allegiance','government','economy','minPad','surface','ignoreCarriers','services','materialTrader','technologyBroker',
      'owner','ownerMode','ownerAllegiance','ownerGovernment','ownerPlayerOnly','ownerControllingOnly','ownerStates','ownerStateMode']) delete selected[key];
    if (isOrbitalStation(s)) delete selected.maxArrivalLs;
  }
  if (s.id==='outfitting') {
    if (selected.ignoreCarriers) delete selected.stronghold;
    if (!outfitCount(s)) delete selected.discountedOnly;
    const pledged=C.state?.commander?.power || C.state?.pilot?.power;
    if (typeof pledged==='string' && selected.stronghold==='friendly') selected.pledgedPower=pledged;
  }
  const order=effectiveSort(s,sort);
  return {category:s.id,origin:region?'':text(origin),name:text(name),region:text(region),sort:order,filters:selected};
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
    s.result=null; drawResults(C,u,s);
    return;
  }
  const base=repeat&&s.executed?s.executed:parameters(C,s);
  if (!base.origin&&!base.region) {
    s.error='Укажите систему или регион Галактики.';
    s.result=null; drawResults(C,u,s);
    return;
  }
  if ([['populationMin','populationMax'],['minGravity','maxGravity'],['minTemperature','maxTemperature']].some(
    ([lower,upper])=>base.filters[lower]!=null && base.filters[upper]!=null && base.filters[lower]>base.filters[upper],
  )) {
    s.error='Минимальное значение превышает максимальное.';
    s.result=null; draw(C,u);
    return;
  }
  const id=++sequence, previous=s.result;
  s.error='';s.pending=id;s.executed=base;
  if (!repeat) {
    s.result=null;
    s.page=0;
  }
  drawResults(C,u,s);
  try {
    const response=await u.call('nearest.search',{...base,page,sequence:id});
    if (s.pending===id) {
      if (repeat && previous && response.status!=='ok') {
        s.error={
          unavailable:'Сервис поиска временно недоступен.',
          invalid:'Проверьте исходную систему и параметры поиска.',
          disabled:'Сетевой поиск отключён.',
        }[response.status] || 'Не удалось открыть страницу результатов.';
      } else {
        s.result=response;
        s.page=page;
      }
    }
  } catch (error) {
    if (s.pending===id) s.error=error.message || 'Сервис поиска временно недоступен.';
  } finally {
    if(s.pending===id) {
      s.pending=null;
      drawResults(C,u,s);
    }
  }
}
function prime(C,u,s) {
  if (!s.primed&&s.id!=='outfitting') void search(C,u,s);
}
function leave(C,u) {
  if (!active(C)&&C.nearestUI) {
    window.NearestPicker.close();
    cancel(session(C),u);
  }
}
function click(C,u,event) {
  const picker=event.target.closest('[data-near-picker]');
  if (picker) {
    event.preventDefault();
    window.NearestPicker.toggle(picker,u);
    return;
  }
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
      cancel(s,u);s.primed=true;drawResults(C,u,s);return;
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
      if (outfitCount(s)<maxOutfitItems&&!items.some(known=>JSON.stringify(known)===JSON.stringify(item))) s.groups.modules=[...items,item];
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
    if (!items.includes(field.value)&&(group==='ships'?outfitCount(s)<maxOutfitItems:items.length<15)) s.groups[group]=[...items,field.value];
    draw(C,u,'[name="'+key+'"]');return;
  }
  s.values[key]=field.type==='checkbox'?field.checked:field.value;
  if (!commit) return;
  if (key==='kind'||key==='signal'||key==='stationType'||s.id==='outfitting'&&key==='ignoreCarriers') {
    if (key==='stationType'&&isOrbitalStation(s)&&s.values.sort==='arrival') s.values.sort='distance';
    draw(C,u,'[name="'+key+'"]');return;
  }
  if (key==='region') {
    s.values.origin=field.value?'':(s.values.origin || C.state?.commander?.system || 'Sol');
    draw(C,u,'[name="region"]');return;
  }
  if (key==='origin'&&field.value&&s.values.region) {
    s.values.region='';
    draw(C,u,'[name="origin"]');return;
  }
  if (key==='moduleName') draw(C,u,'[name="moduleName"]');
  if (key==='bodyKind') {
    if (field.value==='Star') delete s.values.planetType;
    if (field.value==='Planet') delete s.values.starClass;
    draw(C,u,'[name="bodyKind"]');
  }
}
function init(C,u) {
  if (!active(C)) return;
  const host=u.$('#nearest-workspace');
  if (!host||host.nearBound) return;
  window.NearestPicker.sync(host);
  host.nearBound=true;
  host.addEventListener('click',event=>click(C,u,event));
  host.addEventListener('keydown',event=>{
    const picker=event.target.closest('[data-near-picker]');
    if (picker && (event.key==='ArrowDown'||event.key==='ArrowUp')) {
      event.preventDefault();
      window.NearestPicker.toggle(picker,u);
    }
  });
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
