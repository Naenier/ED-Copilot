'use strict';

// The tiers express collection priority, not a guaranteed market price.
// Commodity prices must come from an observed market or an explicit user quote.
const sources = [
  { title: 'Frontier — Player Journal Manual v37', url: 'https://hosting.zaonce.net/community/journal/v37/Journal_Manual_v37.pdf' },
  { title: 'Frontier — Rhino and Anaconda Mk II update notes', url: 'https://store.steampowered.com/news/posts/?appids=359320' },
  { title: 'EDCD — Frontier commodity identifiers', url: 'https://github.com/EDCD/FDevIDs/blob/master/commodity.csv' },
  { title: 'EDCD — Raw material identifiers and grades', url: 'https://github.com/EDCD/FDevIDs/blob/master/material.csv' },
  { title: 'EDTools — Reported mining hotspots', url: 'https://edtools.cc/hotspot' },
  { title: 'EDTools — Current reported mineral prices', url: 'https://edtools.cc/multi' },
  { title: 'Elite Dangerous Field Manual — Mining methods', url: 'https://edfieldmanual.com/wiki/Mining_Laser' },
  { title: 'NEWP — Mining guide', url: 'https://newp.io/mining' },
  { title: 'EDSMT developer — Observed Rhino journal behaviour', url: 'https://github.com/TheAnnouncer/EDSMT/blob/main/journal.py' },
];

function normalize(value) {
  return String(value ?? '').replace(/^\$/, '').replace(/_name;?$/i, '').normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '').toLowerCase().replace(/[^a-z0-9]/g, '');
}

const ringTypes = {
  metallic: 'Металлическое',
  metalrich: 'Богатое металлами',
  rocky: 'Каменистое',
  icy: 'Ледяное',
};

const reserveLevels = {
  pristine: 'Нетронутые запасы',
  major: 'Крупные запасы',
  common: 'Обычные запасы',
  low: 'Низкие запасы',
  depleted: 'Истощённые запасы',
};

const methods = {
  laser: 'Лазерная добыча',
  core: 'Разработка ядра',
  subsurface: 'Подповерхностная добыча',
  surface: 'Поверхностные отложения астероида',
  'surface-planet': 'Планетарная добыча на Rhino',
  raw: 'Сбор сырьевых материалов',
};

const tierLabels = { high: 'Высокий приоритет', medium: 'Средний приоритет', low: 'Низкий приоритет' };
const ringAliases = {
  eringclassmetallic: 'metallic', eringclassmetalic: 'metallic', metallic: 'metallic', metalic: 'metallic',
  eringclassmetalrich: 'metalrich', metalrich: 'metalrich',
  eringclassrocky: 'rocky', rocky: 'rocky',
  eringclassicy: 'icy', icy: 'icy', ice: 'icy',
};
const reserveAliases = {
  pristineresources: 'pristine', pristine: 'pristine',
  majorresources: 'major', major: 'major',
  commonresources: 'common', common: 'common',
  lowresources: 'low', low: 'low',
  depletedresources: 'depleted', depleted: 'depleted',
};

// Availability is method-specific. A platinum hotspot in a metal-rich ring
// confirms a location where platinum may occur in cores, not laser platinum.
const commodityRows = [
  ['platinum', 'Платина', 'Platinum', 'high', { laser: ['metallic'], core: ['metalrich', 'metallic'], surface: ['metalrich', 'metallic'], subsurface: ['metallic'] },
    'Одна из основных целей для лазерной добычи. Для лазерной платины нужны металлические кольца.',
    ['Богатые металлами и металлические кольца — разные типы. В богатом металлами кольце ищите платиновое ядро.', 'Запустите геологоразведочный дрон до начала добычи. Приоритет зависит от доли платины и доступной цены продажи.']],
  ['painite', 'Пейнит', 'Painite', 'high', { laser: ['metallic'], core: ['metalrich', 'metallic'], surface: ['metalrich', 'metallic'], subsurface: ['metallic'] },
    'Ценный минерал металлических колец. В богатых металлами кольцах встречается в ядрах.',
    ['Для лазерной добычи выбирайте металлическое кольцо. Сначала проверьте состав дронами.', 'Сравнивайте свежие предложения покупателей и их спрос.']],
  ['lowtemperaturediamond', 'Низкотемпературные алмазы', 'Low Temperature Diamonds', 'high', { laser: ['icy'], core: ['icy'], surface: ['icy'], subsurface: ['icy'] },
    'Минерал ледяных колец, доступный несколькими способами разработки астероидов.',
    ['Проверьте долю алмазов в обычном составе либо наличие алмазного ядра.', 'Разные способы добычи имеют разную скорость. Цена за тонну не определяет доход в час.']],
  ['opal', 'Опал бездны', 'Void Opals', 'high', { core: ['icy'], surface: ['icy'] },
    'Ценный минерал ядер в ледяных кольцах.',
    ['Для доступа к ядру нужны сейсмические заряды. После подрыва используйте абразивный бластер на внутренних отложениях.', 'Лазер не извлекает содержимое опалового ядра.']],
  ['alexandrite', 'Александрит', 'Alexandrite', 'high', { core: ['rocky', 'metalrich', 'icy'], surface: ['rocky', 'metalrich', 'icy'] },
    'Драгоценный минерал астероидных ядер.',
    ['Ищите подходящие ядра импульсно-волновым анализатором и подтверждайте состав дронами.', 'Абразивная добыча нужна для доступных отложений после раскалывания ядра.']],
  ['benitoite', 'Бенитоит', 'Benitoite', 'high', { core: ['rocky'], surface: ['rocky'] },
    'Драгоценный минерал ядер каменистых колец.',
    ['Каменистое кольцо и сигнал бенитоита задают район поиска. Само ядро подтверждает геологоразведочный дрон.']],
  ['grandidierite', 'Грандидьерит', 'Grandidierite', 'high', { core: ['icy'], surface: ['icy'] },
    'Драгоценный минерал ядер ледяных колец.',
    ['Для разработки нужен набор для подрыва ядра и абразивный бластер.', 'Отсутствие сигнала грандидьерита не доказывает отсутствие отдельных ядер в кольце.']],
  ['monazite', 'Монацит', 'Monazite', 'high', { core: ['rocky', 'metalrich', 'metallic'], surface: ['rocky', 'metalrich', 'metallic'] },
    'Ценный минерал астероидных ядер в каменистых и металлических кольцах.',
    ['Проверьте сигнал месторождения с помощью DSS и затем исследуйте отдельные астероиды.']],
  ['musgravite', 'Мусгравит', 'Musgravite', 'high', { core: ['rocky'], surface: ['rocky'] },
    'Драгоценный минерал ядер каменистых колец.',
    ['Импульсно-волновый анализатор помогает искать астероиды с отложениями, а ядро мусгравита подтверждает дрон.']],
  ['rhodplumsite', 'Родплумсайт', 'Rhodplumsite', 'high', { core: ['metalrich', 'metallic'], surface: ['metalrich', 'metallic'] },
    'Ценный минерал астероидных ядер в металлических и богатых металлами кольцах.',
    ['Кольцо подходящего типа показывает возможную среду. Для подтверждения ресурса нужен сигнал либо исследование астероида.']],
  ['serendibite', 'Серендибит', 'Serendibite', 'high', { core: ['rocky', 'metalrich', 'metallic'], surface: ['rocky', 'metalrich', 'metallic'] },
    'Ценный минерал астероидных ядер.',
    ['Сравнивайте свежую цену продажи с другими найденными минералами перед заполнением трюма.']],
  ['osmium', 'Осмий', 'Osmium', 'medium', { laser: ['metalrich', 'metallic'], surface: ['metalrich', 'metallic'], subsurface: ['metalrich', 'metallic'] },
    'Металл для продажи и добывающих заданий. Отдельного осмиевого сигнала месторождения нет.',
    ['Ищите осмий геологоразведочными дронами в металлических и богатых металлами кольцах.', 'Заказ на добычу может изменить практическую ценность ресурса.']],
  ['bromellite', 'Бромеллит', 'Bromellite', 'medium', { laser: ['icy'], core: ['icy'], surface: ['icy'], subsurface: ['icy'] },
    'Минерал ледяных колец, полезный для продажи и отдельных заданий.',
    ['Оценивайте текущего покупателя. Базовый приоритет ниже основных драгоценных минералов.']],
  ['tritium', 'Тритий', 'Tritium', 'medium', { laser: ['icy'], surface: ['icy'], subsurface: ['icy'] },
    'Топливо флотских авианосцев, добываемое в ледяных кольцах.',
    ['Тритий полезен для снабжения собственного авианосца и заказов покупателей.', 'Отдельного тритиевого ядра нет. Проверяйте обычный состав и доступные отложения.']],
  ['palladium', 'Палладий', 'Palladium', 'medium', { laser: ['metallic'], surface: ['metallic'], subsurface: ['metallic'] },
    'Продаваемый металл металлических колец.', ['Подходит как попутный груз при хорошей концентрации и подтверждённой цене продажи.']],
  ['gold', 'Золото', 'Gold', 'medium', { laser: ['metalrich', 'metallic'], surface: ['metalrich', 'metallic'], subsurface: ['metalrich', 'metallic'] },
    'Продаваемый драгоценный металл.', ['Сопоставьте время добычи и занятую ёмкость с более ценными ресурсами.']],
  ['silver', 'Серебро', 'Silver', 'medium', { laser: ['metalrich', 'metallic'], surface: ['metalrich', 'metallic'], subsurface: ['metalrich', 'metallic'] },
    'Продаваемый металл металлических и богатых металлами колец.', ['Практический приоритет повышается при хорошем покупателе или задании.']],
  ['samarium', 'Самарий', 'Samarium', 'medium', { laser: ['rocky', 'metalrich', 'metallic'], surface: ['rocky', 'metalrich', 'metallic'], subsurface: ['rocky', 'metalrich', 'metallic'] },
    'Металл, встречающийся в обычном составе астероидов.', ['Оценивайте как попутную добычу либо ресурс для задания.']],
  ['bertrandite', 'Бертрандит', 'Bertrandite', 'medium', { laser: ['metalrich', 'metallic'], surface: ['metalrich', 'metallic'], subsurface: ['metalrich', 'metallic'] },
    'Промышленный минерал. Может быть нужен для добывающих заданий и торговли.', ['Для обычной продажи сравнивайте с более ценными ресурсами по свежим рыночным данным.']],
  ['praseodymium', 'Празеодим', 'Praseodymium', 'low', { laser: ['metalrich', 'metallic'], surface: ['metalrich', 'metallic'], subsurface: ['metalrich', 'metallic'] },
    'Промышленный металл с низким исходным приоритетом для целевой продажи.', ['Имеет смысл при конкретном заказе или личной потребности.']],
];

const resources = commodityRows.map(([key, name, english, tier, availability, description, tips]) => ({
  key, id: key, name, english, kind: 'commodity', tier, grade: null,
  methods: Object.keys(availability), ringTypes: [...new Set(Object.values(availability).flat())],
  availability: Object.entries(availability).map(([method, rings]) => ({ method, ringTypes: rings })),
  description, tips, sources: [sources[2], sources[4], sources[5], sources[6], sources[7]],
  price: null, priceSource: null, priceAt: null,
}));

// Frontier's September 2026 commodity list. Planet and deposit distribution
// have no published deterministic rule, so these profiles do not invent one.
const planetaryRows = [
  ['iridium', 'Иридий', 'Iridium'],
  ['helium', 'Гелий', 'Helium'],
  ['helium3', 'Гелий-3', 'Helium-3'],
  ['bastnasite', 'Бастнезит', 'Bastnäsite'],
  ['deuterium', 'Дейтерий', 'Deuterium'],
  ['thortveitite', 'Тортвейтит', 'Thortveitite'],
  ['quartzpyroxenite', 'Кварцевый пироксенит', 'Quartz Pyroxenite'],
  ['olivine', 'Оливин', 'Olivine'],
  ['periclasedunite', 'Периклазовый дунит', 'Periclase Dunite'],
  ['sapphire', 'Сапфир', 'Sapphire'],
  ['diamond', 'Алмаз', 'Diamond'],
  ['ruby', 'Рубин', 'Ruby'],
  ['magnesite', 'Магнезит', 'Magnesite'],
];

for (const [key, name, english] of planetaryRows) {
  resources.push({
    key, id: key, name, english, kind: 'commodity', tier: 'medium', grade: null,
    methods: ['surface-planet'], ringTypes: [], availability: [{ method: 'surface-planet', ringTypes: [] }],
    description: 'Товар планетарной добычи, добавленный с Rhino в сентябре 2026 года. Добывается установками на поверхности доступного для посадки тела.',
    tips: [
      'FSS и DSS сообщают число участков для планетарной добычи. Этот счётчик не определяет конкретный ресурс или плотность залежи.',
      'Ищите залежи планетарным минеральным сканером Rhino. Плотность и состояние залежи проверяйте в игре.',
      'Средний приоритет отражает доступность этого направления для поиска. Доходность уточняется по свежей цене и фактической скорости добычи.',
    ],
    sources: [sources[1], sources[2], sources[8]],
    price: null, priceSource: null, priceAt: null,
  });
}

const rawRows = [
  ['carbon', 'Углерод', 'Carbon', 1], ['phosphorus', 'Фосфор', 'Phosphorus', 1],
  ['sulphur', 'Сера', 'Sulphur', 1], ['iron', 'Железо', 'Iron', 1], ['nickel', 'Никель', 'Nickel', 1],
  ['rhenium', 'Рений', 'Rhenium', 1], ['lead', 'Свинец', 'Lead', 1],
  ['vanadium', 'Ванадий', 'Vanadium', 2], ['chromium', 'Хром', 'Chromium', 2],
  ['manganese', 'Марганец', 'Manganese', 2], ['zinc', 'Цинк', 'Zinc', 2],
  ['germanium', 'Германий', 'Germanium', 2], ['arsenic', 'Мышьяк', 'Arsenic', 2], ['zirconium', 'Цирконий', 'Zirconium', 2],
  ['niobium', 'Ниобий', 'Niobium', 3], ['molybdenum', 'Молибден', 'Molybdenum', 3],
  ['cadmium', 'Кадмий', 'Cadmium', 3], ['tin', 'Олово', 'Tin', 3],
  ['tungsten', 'Вольфрам', 'Tungsten', 3], ['mercury', 'Ртуть', 'Mercury', 3], ['boron', 'Бор', 'Boron', 3],
  ['yttrium', 'Иттрий', 'Yttrium', 4], ['technetium', 'Технеций', 'Technetium', 4],
  ['ruthenium', 'Рутений', 'Ruthenium', 4], ['selenium', 'Селен', 'Selenium', 4],
  ['tellurium', 'Теллур', 'Tellurium', 4], ['polonium', 'Полоний', 'Polonium', 4], ['antimony', 'Сурьма', 'Antimony', 4],
];

for (const [key, name, english, grade] of rawRows) {
  resources.push({
    key, id: key, name, english, kind: 'raw', grade,
    tier: grade === 4 ? 'high' : grade === 3 ? 'medium' : 'low',
    methods: ['raw'], ringTypes: [], availability: [{ method: 'raw', ringTypes: [] }],
    description: `Сырьевой материал категории ${grade}. Используется для синтеза, инженерных улучшений и обмена у торговца сырьевыми материалами.`,
    tips: [
      'Доля в Scan.Materials показывает наличие элемента на теле. Она не задаёт координаты месторождения или ожидаемые кредиты.',
      'Собирается в отдельный запас материалов. Для продажи на товарном рынке не предназначен.',
      'Категории 3–4 получают повышенный приоритет по редкости. Личная потребность и заполненность запаса могут изменить выбор.',
    ],
    sources: [sources[0], sources[3]],
    price: null, priceSource: null, priceAt: null,
  });
}

const resourceAliases = {
  voidopal: 'opal', voidopals: 'opal', lowtemperaturediamonds: 'lowtemperaturediamond',
  ltd: 'lowtemperaturediamond', grandiderite: 'grandidierite', sulfur: 'sulphur',
};
const resourceByKey = new Map(resources.map((resource) => [resource.key, resource]));

function getResource(value) {
  const key = normalize(value);
  return resourceByKey.get(resourceAliases[key] ?? key) ?? null;
}

function normalizeRingType(value) {
  return ringAliases[normalize(value)] ?? null;
}

function normalizeReserveLevel(value) {
  return reserveAliases[normalize(value)] ?? null;
}

const qualityGuidance = {
  minLaserPercent: 15,
  highLaserPercent: 30,
  description: 'Порог 15 % — настраиваемый начальный фильтр для выбранного продаваемого ресурса. 30 % — усиленное выделение богатого астероида. Это правила интерфейса, а не игровые границы качества или гарантии прибыли.',
  contentDescription: 'ProspectedAsteroid.Content оценивает сырьевые материалы. Решение о товарной добыче опирается на Materials[].Proportion, наличие ядра и нужный метод.',
  reserveDescription: 'Уровень запасов влияет на обычную лазерную добычу. Он не подтверждает отсутствие ценных ядер и не измеряет плотность конкретной планетарной залежи.',
  hotspotDescription: 'Count — число сигналов данного ресурса в кольце. По этому числу нельзя вычислить перекрытие зон, концентрацию каждого астероида или оставшийся тоннаж.',
};

module.exports = {
  resources, ringTypes, reserveLevels, methods, tierLabels, sources, qualityGuidance,
  resourceAliases, resourceByKey, normalize, getResource, normalizeRingType, normalizeReserveLevel,
  planetarySignalType: '$PlanetaryMiningLocation_Name;',
  dataAsOf: '2026-11-07',
};
