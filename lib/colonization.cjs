'use strict';

// This planner records a player's intentions and observations. It does not make
// claims, place facilities, or send deliveries to the game.
const { randomUUID, createHash } = require('node:crypto');
const { normalize, getResource } = require('./catalog.cjs');
const russianNames = require('./commodity-ru.json');
const englishNames = require('./commodity-names.json');

const SOURCES = [
  { id: 'frontier', title: 'Frontier — правила колонизации', url: 'https://store.steampowered.com/news/posts/?appgroupname=Elite%3A+Dangerous&appids=359320&enddate=1760615315&feed=steam_community_announcements' },
  { id: 'journal', title: 'EDCD / EDDI — события журнала колонизации', url: 'https://github.com/EDCD/EDDI/wiki/Events' },
  { id: 'spansh', title: 'Spansh — наблюдаемые системы и станции', url: 'https://spansh.co.uk/systems' },
  { id: 'mechan', title: 'CMDR Mechan и соавторы — Colonization Mega Guide 2.3.0', url: 'https://svbkresearchinstitute.com/colonization' },
];
const RULES = Object.freeze({ rangeLy: 15, claimCost: 25000000, beaconHours: 24, primaryDays: 28, maxConcurrent: 5 });
const PROJECT_STATUSES = new Set(['planned', 'claimed', 'building', 'complete', 'cancelled']);
const BUILDING_STATUSES = new Set(['planned', 'building', 'complete', 'cancelled']);
const ECONOMIES = [
  ['refinery', 'Переработка'], ['industrial', 'Промышленность'], ['extraction', 'Добыча'],
  ['agriculture', 'Сельское хозяйство'], ['hightech', 'Высокие технологии'],
  ['military', 'Военная'], ['tourism', 'Туризм'], ['service', 'Услуги'], ['mixed', 'Смешанная'],
].map(([id, name]) => ({ id, name }));

// Independently assembled factual type families; no commodity bills are inferred
// from the type. The live construction depot is the authority for its own bill.
const CATALOG = [];
function facility(id, name, tier, kind, economy = '', extra = {}) {
  CATALOG.push({ id, name, tier, kind, economy, prerequisites: [], source: 'mechan',
    pointCost: tier === 1 ? 0 : tier === 2 && kind !== 'port' ? 1 : null,
    pointReward: tier === 1 ? 1 : tier === 2 ? 1 : 0,
    ...extra });
}
for (const [id, name, economy] of [
  ['civilian', 'Гражданский', 'mixed'], ['commercial', 'Коммерческий', 'mixed'],
  ['industrial', 'Промышленный', 'industrial'], ['scientific', 'Научный', 'hightech'],
  ['military', 'Военный', 'military'], ['pirate', 'Пиратский', 'contraband'],
]) facility(`outpost-${id}`, `${name} орбитальный аванпост`, 1, 'port', economy, { location: 'orbital', canBePrimary: true });
facility('coriolis', 'Кориолис', 2, 'port', 'mixed', { location: 'orbital', canBePrimary: true });
facility('asteroid-base', 'Астероидная база', 2, 'port', 'extraction', { location: 'orbital', canBePrimary: true, prerequisites: ['Требуется подходящее место в кольце; проверьте карту системы.'] });
facility('orbis', 'Орбис', 3, 'port', 'mixed', { location: 'orbital', canBePrimary: true });
facility('ocellus', 'Оцеллус', 3, 'port', 'mixed', { location: 'orbital', canBePrimary: true });
facility('dodec', 'Додек — параметры по экрану архитектора', null, 'port', 'mixed', { location: 'orbital', canBePrimary: true, pointReward: null, source: 'frontier', note: 'Вариант и очки введите по данным игры; автоматическая оценка для этого типа не задана.' });
for (const [id, name, economy] of [['civilian', 'Гражданский', 'mixed'], ['industrial', 'Промышленный', 'industrial'], ['scientific', 'Научный', 'hightech']])
  facility(`surface-outpost-${id}`, `${name} планетарный аванпост`, 1, 'port', economy, { location: 'surface', canBePrimary: false });
facility('planetary-port', 'Большой планетарный порт', 3, 'port', 'mixed', { location: 'surface', canBePrimary: false });
for (const [id, name, tier] of [['small', 'Малое', 1], ['medium', 'Среднее', 1], ['large', 'Большое', 2]])
  facility(`settlement-${id}`, `${name} поселение`, tier, 'settlement', '', { location: 'surface', pointReward: tier === 2 ? 2 : 1, note: 'Специализацию поселения укажите в названии или примечании.' });
for (const [id, name, economy] of [
  ['extraction', 'Добывающий', 'extraction'], ['civilian', 'Гражданский', 'mixed'],
  ['exploration', 'Исследовательский', 'hightech'], ['outpost', 'Опорный', 'service'],
  ['scientific', 'Научный', 'hightech'], ['military', 'Военный', 'military'],
  ['refinery', 'Перерабатывающий', 'refinery'], ['hightech', 'Высокотехнологичный', 'hightech'],
  ['industrial', 'Промышленный', 'industrial'],
]) facility(`hub-${id}`, `${name} узел`, 2, 'hub', economy, { location: 'surface' });
// Installation tiers differ between variants. Keeping them unset is preferable
// to charging an unverified number of construction points.
for (const [id, name, economy] of [
  ['satellite', 'Спутниковая', ''], ['communications', 'Связная', ''], ['relay', 'Ретрансляционная', ''],
  ['farm', 'Космическая ферма', 'agriculture'], ['mining', 'Добывающая / промышленная', 'extraction'],
  ['research', 'Исследовательская', 'hightech'], ['military', 'Военная', 'military'],
  ['security', 'Охранная', ''], ['government', 'Правительственная', ''], ['medical', 'Медицинская', ''],
  ['tourist', 'Туристическая', 'tourism'], ['bar', 'Космический бар', 'service'], ['pirate', 'Пиратская база', 'contraband'],
]) facility(`installation-${id}`, name.includes('ферма') || name.includes('бар') || name.includes('база') ? name : `${name} установка`, null, 'installation', economy,
  { location: 'orbital', pointReward: null, note: 'Уровень, очки и зависимости конкретного варианта подтвердите в игре.' });
facility('custom', 'Другой объект — ручной план', null, 'custom', '', { location: 'unknown', pointReward: null, source: 'manual' });
const TYPE_BY_ID = new Map(CATALOG.map(row => [row.id, row]));
const SPANSH = 'https://spansh.co.uk/api';
const clone = value => JSON.parse(JSON.stringify(value));
const text = (value, max = 200) => String(value ?? '').replace(/[\u0000-\u001f]/g, ' ').trim().slice(0, max);
const equal = (a, b) => String(a ?? '').trim().toLocaleLowerCase('en') === String(b ?? '').trim().toLocaleLowerCase('en');
const finite = value => value !== null && value !== '' && Number.isFinite(Number(value));
const natural = (value, max = 1000000000) => finite(value) ? Math.max(0, Math.min(max, Math.trunc(Number(value)))) : null;
const marketId = value => /^\d{1,20}$/.test(String(value ?? '')) ? String(value) : '';
const coordinate = value => value && ['x', 'y', 'z'].every(key => finite(value[key])) ? { x: Number(value.x), y: Number(value.y), z: Number(value.z) } : null;
const distance = (a, b) => a && b ? Math.hypot(a.x - b.x, a.y - b.y, a.z - b.z) : null;
const validDate = value => value && Number.isFinite(Date.parse(value)) ? new Date(value).toISOString() : null;
const factionName = value => text(value && typeof value === 'object' ? value.name || value.Name : value);
const colonisationService = value => ['systemcolonisation', 'systemcolonization', 'colonisation', 'colonization'].includes(normalize(value));
function commodityName(key, supplied) {
  return russianNames[`${key}name`] || getResource(key)?.name || text(supplied) || englishNames[key]?.english || key;
}
function resourceRows(rows) {
  if (!Array.isArray(rows) || rows.length > 500) throw new Error('Список ресурсов должен содержать не более 500 строк.');
  const combined = new Map();
  for (const row of rows) {
    const commodity = normalize(row.commodity || row.Name || row.name);
    const required = natural(row.required ?? row.RequiredAmount);
    if (!commodity || commodity.length > 100 || required === null || Number(row.required ?? row.RequiredAmount) < 0 || !Number.isInteger(Number(row.required ?? row.RequiredAmount))) throw new Error('Укажите товар и неотрицательное целое количество.');
    const previous = combined.get(commodity);
    combined.set(commodity, { commodity, name: commodityName(commodity, row.Name_Localised || row.name),
      required: Math.min(1000000000, required + (previous?.required || 0)) });
  }
  return [...combined.values()];
}
function initialSearch() {
  return { running: false, status: 'idle', error: '', origin: null, origins: [], factions: [], claimStations: [], results: [],
    options: { originSystem: '', faction: '', rangeLy: RULES.rangeLy },
    completeness: { complete: false, providerTotal: null, fetched: 0, pages: 0, reason: 'Поиск ещё не выполнялся.' }, warnings: [] };
}
function cleanBuilding(value = {}) {
  const type = TYPE_BY_ID.get(value.typeId) || TYPE_BY_ID.get('custom');
  return { id: text(value.id, 80) || randomUUID(), typeId: type.id, name: text(value.name) || type.name,
    status: BUILDING_STATUSES.has(value.status) ? value.status : 'planned',
    body: text(value.body), isPrimary: Boolean(value.isPrimary && type.canBePrimary), marketId: marketId(value.marketId), notes: text(value.notes, 4000),
    dependsOn: Array.isArray(value.dependsOn) ? [...new Set(value.dependsOn.map(x => text(x, 80)))].slice(0, 100) : [],
    tierOverride: [1, 2, 3].includes(Number(value.tierOverride)) ? Number(value.tierOverride) : null,
    pointCostOverride: natural(value.pointCostOverride, 100000), pointRewardOverride: natural(value.pointRewardOverride, 100000),
    resourcePlan: resourceRows(Array.isArray(value.resourcePlan) ? value.resourcePlan : []), sourceNote: text(value.sourceNote, 1000) };
}
function cleanProject(value = {}) {
  return { id: text(value.id, 80) || randomUUID(), name: text(value.name) || text(value.targetSystem) || 'Новая колония',
    originSystem: text(value.originSystem), faction: text(value.faction), targetSystem: text(value.targetSystem),
    coordinates: coordinate(value.coordinates), systemAddress: marketId(value.systemAddress),
    status: PROJECT_STATUSES.has(value.status) ? value.status : 'planned', goalEconomy: text(value.goalEconomy, 80),
    claimedAt: validDate(value.claimedAt), deadline: validDate(value.deadline), notes: text(value.notes, 4000),
    points: { tier2: finite(value.points?.tier2) ? Math.max(-100000, Math.min(100000, Math.trunc(Number(value.points.tier2)))) : null,
      tier3: finite(value.points?.tier3) ? Math.max(-100000, Math.min(100000, Math.trunc(Number(value.points.tier3)))) : null },
    externalPortCount: natural(value.externalPortCount, 1000) || 0,
    buildings: Array.isArray(value.buildings) ? value.buildings.slice(0, 200).map(cleanBuilding) : [],
    deliveries: Array.isArray(value.deliveries) ? value.deliveries.slice(-5000).map(row => ({
      id: text(row.id, 80) || randomUUID(), buildingId: text(row.buildingId, 80), commodity: normalize(row.commodity),
      count: natural(row.count) || 0, timestamp: validDate(row.timestamp), notes: text(row.notes, 1000), source: 'manual',
    })).filter(row => row.commodity && row.count > 0) : [] };
}

class ColonizationPlanner {
  constructor(saved = {}, options = {}) {
    this.fetchImpl = options.fetchImpl || globalThis.fetch;
    this.onUpdate = options.onUpdate || (() => {});
    this.onSave = options.onSave || (() => {});
    this.now = options.now || (() => Date.now());
    this.config = { originSystem: text(saved.originSystem), faction: text(saved.faction), cargoCapacity: natural(saved.cargoCapacity, 100000) };
    this.projects = Array.isArray(saved.projects) ? saved.projects.slice(0, 100).map(cleanProject) : [];
    this.selectedProjectId = this.projects.some(p => p.id === saved.selectedProjectId) ? saved.selectedProjectId : this.projects[0]?.id || '';
    this.search = initialSearch();
    this.search.options = { ...this.search.options, ...this.config };
    this.searchSerial = 0;
    this.controller = null;
    this.searchPromise = Promise.resolve();
    this.reset();
  }

  reset() {
    this.depots = new Map();
    this.contributions = [];
    this.claims = new Map();
    this.observedStations = new Map();
    this.seen = new Set();
    this.sequence = 0;
    this.currentCommander = '';
  }

  exportSettings() { return clone({ ...this.config, projects: this.projects, selectedProjectId: this.selectedProjectId }); }
  changed(save = true) { if (save) this.onSave(); this.onUpdate(); }
  configure(body) { return this.update({ ...body, action: 'configure' }); }
  project(id) {
    const result = this.projects.find(row => row.id === (id || this.selectedProjectId));
    if (!result) throw new Error('Проект не найден.');
    return result;
  }
  building(project, id) {
    const result = project.buildings.find(row => row.id === id);
    if (!result) throw new Error('Объект строительства не найден.');
    return result;
  }
  validateDependencies(project) {
    const ids = new Set(project.buildings.map(row => row.id));
    if (ids.size !== project.buildings.length) throw new Error('Идентификаторы объектов должны быть уникальны.');
    const visiting = new Set(), done = new Set();
    const visit = id => {
      if (visiting.has(id)) throw new Error('Зависимости объектов образуют цикл.');
      if (done.has(id)) return;
      visiting.add(id);
      const row = this.building(project, id);
      for (const dependency of row.dependsOn) {
        if (!ids.has(dependency)) throw new Error('Объект зависимости не найден в этом проекте.');
        visit(dependency);
      }
      visiting.delete(id); done.add(id);
    };
    for (const id of ids) visit(id);
  }
  assertUniqueDepot(id, exceptBuilding) {
    if (id && this.projects.some(p => p.buildings.some(b => b !== exceptBuilding && b.marketId === id)))
      throw new Error('Это депо уже связано с другим объектом. Сначала удалите прежнюю связь.');
  }

  update(body = {}) {
    const action = text(body.action, 80);
    if (action === 'configure') {
      if ('originSystem' in body) this.config.originSystem = text(body.originSystem);
      if ('faction' in body) this.config.faction = text(body.faction);
      if ('cargoCapacity' in body) {
        if (body.cargoCapacity !== null && body.cargoCapacity !== '' && natural(body.cargoCapacity, 100000) === null) throw new Error('Укажите вместимость грузового отсека в тоннах.');
        this.config.cargoCapacity = natural(body.cargoCapacity, 100000);
      }
    } else if (action === 'create-project') {
      if (this.projects.length >= 100) throw new Error('Достигнут предел 100 проектов.');
      const candidate = this.search.results.find(row => row.id === String(body.candidateId));
      const targetSystem = text(candidate?.system || body.targetSystem);
      if (!targetSystem) throw new Error('Выберите целевую систему.');
      if (candidate?.availability === 'unavailable') throw new Error('По последним данным система не подходит для новой колонии.');
      const project = cleanProject({ ...body, id: randomUUID(), targetSystem,
        originSystem: this.search.origin?.name || this.config.originSystem,
        faction: text(body.faction) || this.config.faction,
        coordinates: candidate?.coordinates || body.coordinates,
        systemAddress: candidate?.id64 || body.systemAddress,
        buildings: [], deliveries: [] });
      this.projects.push(project); this.selectedProjectId = project.id;
    } else if (action === 'select-project') {
      this.selectedProjectId = this.project(body.id).id;
    } else if (action === 'remove-project') {
      const project = this.project(body.id);
      this.projects = this.projects.filter(row => row !== project);
      if (this.selectedProjectId === project.id) this.selectedProjectId = this.projects[0]?.id || '';
    } else if (action === 'update-project') {
      const project = this.project(body.id || body.projectId);
      const next = cleanProject({ ...project, ...body, id: project.id, buildings: project.buildings, deliveries: project.deliveries });
      this.projects[this.projects.indexOf(project)] = next;
    } else if (action === 'record-delivery') {
      const project = this.project(body.projectId);
      this.building(project, body.buildingId);
      const commodity = normalize(body.commodity), count = natural(body.count);
      if (!commodity || commodity.length > 100 || count === null || count < 1 || !Number.isInteger(Number(body.count))) throw new Error('Укажите товар и положительное целое количество.');
      if (project.deliveries.length >= 5000) throw new Error('Достигнут предел ручных записей в проекте.');
      project.deliveries.push({ id: randomUUID(), buildingId: body.buildingId, commodity, count,
        timestamp: new Date(this.now()).toISOString(), notes: text(body.notes, 1000), source: 'manual' });
    } else if (action === 'remove-delivery') {
      const project = this.project(body.projectId);
      if (!project.deliveries.some(row => row.id === body.id)) throw new Error('Запись доставки не найдена.');
      project.deliveries = project.deliveries.filter(row => row.id !== body.id);
    } else if (action === 'add-building') {
      const project = this.project(body.projectId);
      if (project.buildings.length >= 200) throw new Error('Достигнут предел 200 объектов в проекте.');
      if (!TYPE_BY_ID.has(body.typeId)) throw new Error('Выберите тип объекта из каталога.');
      const building = cleanBuilding({ ...body, id: randomUUID(), resourcePlan: [] });
      this.assertUniqueDepot(building.marketId);
      if (building.isPrimary && project.buildings.some(row => row.isPrimary)) throw new Error('Первичный порт в проекте уже указан.');
      project.buildings.push(building);
      try { this.validateDependencies(project); } catch (error) { project.buildings.pop(); throw error; }
    } else {
      const project = this.project(body.projectId);
      if (action === 'reorder-buildings') {
        if (!Array.isArray(body.ids) || body.ids.length !== project.buildings.length || new Set(body.ids).size !== body.ids.length || body.ids.some(id => !project.buildings.some(row => row.id === id)))
          throw new Error('Новый порядок должен содержать каждый объект ровно один раз.');
        project.buildings = body.ids.map(id => this.building(project, id));
      } else {
        const building = this.building(project, body.buildingId || body.id);
        if (action === 'remove-building') {
          project.buildings = project.buildings.filter(row => row !== building);
          for (const row of project.buildings) row.dependsOn = row.dependsOn.filter(id => id !== building.id);
          project.deliveries = project.deliveries.filter(row => row.buildingId !== building.id);
        } else if (action === 'move-building') {
          const index = project.buildings.indexOf(building);
          if (!['up', 'down'].includes(body.direction)) throw new Error('Неизвестное направление перемещения.');
          const next = index + (body.direction === 'up' ? -1 : 1);
          if (next >= 0 && next < project.buildings.length) [project.buildings[index], project.buildings[next]] = [project.buildings[next], project.buildings[index]];
        } else if (action === 'set-resource-plan') {
          const rows = resourceRows(body.resources);
          const sourceNote = text(body.sourceNote, 1000);
          if (rows.length && !sourceNote) throw new Error('Укажите источник ручной оценки потребности.');
          building.resourcePlan = rows; building.sourceNote = sourceNote;
        } else if (action === 'link-depot') {
          const id = body.marketId === '' || body.marketId === null ? '' : marketId(body.marketId);
          if (body.marketId && !id) throw new Error('MarketID должен содержать только цифры.');
          this.assertUniqueDepot(id, building);
          const depot = this.depots.get(id);
          if (depot?.system && !equal(depot.system, project.targetSystem)) throw new Error('Депо находится в другой системе.');
          building.marketId = id;
        } else if (action === 'update-building') {
          const next = cleanBuilding({ ...building, ...body, id: building.id, typeId: building.typeId });
          this.assertUniqueDepot(next.marketId, building);
          if (next.isPrimary && project.buildings.some(row => row !== building && row.isPrimary)) throw new Error('Первичный порт в проекте уже указан.');
          const index = project.buildings.indexOf(building);
          project.buildings[index] = next;
          try { this.validateDependencies(project); } catch (error) { project.buildings[index] = building; throw error; }
        } else throw new Error('Неизвестное действие планировщика колонизации.');
      }
    }
    this.changed();
    return this.snapshot();
  }

  apply(event, context = {}) {
    if (!event || typeof event.event !== 'string') return;
    const commander = context.commander || {};
    if (event.event === 'LoadGame' && event.FID) {
      if (this.currentCommander && this.currentCommander !== event.FID) this.reset();
      this.currentCommander = event.FID;
    }
    if (!['ColonisationConstructionDepot', 'ColonisationContribution', 'ColonisationSystemClaim', 'ColonisationSystemClaimRelease', 'ColonisationBeaconDeployed', 'Docked', 'Location'].includes(event.event)) return;
    // Exact journal identity survives identical same-second deliveries. A caller
    // without a cursor must provide distinct events; do not guess at duplicates.
    const identity = context.file !== undefined && context.offset !== undefined ? `${context.file}:${context.offset}` : '';
    if (identity && this.seen.has(identity)) return;
    if (identity) this.seen.add(identity);
    const sequence = ++this.sequence;
    const timestamp = validDate(event.timestamp) || new Date(this.now()).toISOString();
    const system = text(event.StarSystem || commander.system || commander.starSystem);
    const station = text(event.StationName || commander.station);
    if (event.event === 'Docked' || event.event === 'Location') {
      const id = marketId(event.MarketID);
      if (id && station) this.observedStations.set(id, { marketId: id, name: station, system,
        faction: text(event.StationFaction?.Name), services: Array.isArray(event.StationServices) ? event.StationServices.map(String) : [], timestamp });
      return;
    }
    if (event.event === 'ColonisationConstructionDepot') {
      const id = marketId(event.MarketID);
      if (!id || !Array.isArray(event.ResourcesRequired)) return;
      const rows = [];
      for (const row of event.ResourcesRequired.slice(0, 500)) {
        const commodity = normalize(row.Name), required = natural(row.RequiredAmount), delivered = natural(row.ProvidedAmount);
        if (!commodity || required === null || delivered === null) continue;
        rows.push({ commodity, name: commodityName(commodity, row.Name_Localised), required, delivered,
          payment: natural(row.Payment), missing: Math.max(0, required - delivered), source: 'journal', authoritative: true });
      }
      this.depots.set(id, { marketId: id, system, station, timestamp, sequence,
        constructionProgress: finite(event.ConstructionProgress) ? Math.max(0, Math.min(1, Number(event.ConstructionProgress))) : null,
        complete: event.ConstructionComplete === true, failed: event.ConstructionFailed === true, requirements: rows });
    } else if (event.event === 'ColonisationContribution') {
      const id = marketId(event.MarketID);
      if (!id || !Array.isArray(event.Contributions)) return;
      const resources = event.Contributions.slice(0, 500).map(row => ({ commodity: normalize(row.Name), name: commodityName(normalize(row.Name), row.Name_Localised), count: natural(row.Amount) }))
        .filter(row => row.commodity && row.count > 0);
      if (!resources.length) return;
      this.contributions.push({ id: identity ? createHash('sha256').update(identity).digest('hex').slice(0, 32) : randomUUID(),
        marketId: id, system, timestamp, sequence, resources, source: 'journal' });
    } else {
      const target = text(event.SystemName || event.StarSystem || system);
      const address = marketId(event.SystemAddress || (event.event === 'ColonisationBeaconDeployed' ? commander.systemAddress : null));
      const key = address || target.toLocaleLowerCase('en');
      const previous = this.claims.get(key);
      if (key) this.claims.set(key, { ...previous, system: target, systemAddress: address, timestamp, sequence,
        ...(event.event === 'ColonisationSystemClaim' ? { claimedAt: timestamp, beaconAt: null, releasedAt: null }
          : event.event === 'ColonisationBeaconDeployed' ? { beaconAt: timestamp } : { releasedAt: timestamp }),
        status: event.event === 'ColonisationSystemClaimRelease' ? 'released' : event.event === 'ColonisationBeaconDeployed' ? 'beacon' : 'claimed' });
    }
    // Engine emits its state once after journal application. No callbacks here:
    // replay may involve millions of records and constructor state may be partial.
  }

  async request(path, options, signal) {
    const timeout = AbortSignal.timeout(25000);
    const requestSignal = signal ? AbortSignal.any([signal, timeout]) : timeout;
    const response = await this.fetchImpl(`${SPANSH}${path}`, { ...options, headers: { Accept: 'application/json', ...(options?.headers || {}) }, signal: requestSignal });
    if (!response.ok) throw new Error(`Spansh вернул HTTP ${response.status}. Повторите поиск позже.`);
    const declared = Number(response.headers?.get?.('content-length'));
    if (declared > 25000000) throw new Error('Ответ Spansh слишком велик.');
    const value = await response.json();
    if (value?.error) throw new Error(`Spansh: ${text(value.error, 300)}`);
    return value;
  }

  async startSearch(input = {}) {
    const originSystem = text(input.originSystem ?? this.config.originSystem);
    const faction = text(input.faction ?? this.config.faction);
    if (!originSystem) throw new Error('Введите систему происхождения.');
    this.cancel(false);
    const serial = ++this.searchSerial;
    const controller = new AbortController(); this.controller = controller;
    this.config.originSystem = originSystem; this.config.faction = faction;
    this.search = { ...initialSearch(), running: true, status: 'resolving', options: { originSystem, faction, rangeLy: RULES.rangeLy } };
    this.changed();
    this.searchPromise = this.runSearch(originSystem, faction, serial, controller.signal).catch(error => {
      if (serial !== this.searchSerial) return;
      this.search.running = false;
      this.search.status = controller.signal.aborted ? 'cancelled' : 'error';
      this.search.error = controller.signal.aborted ? '' : text(error.message, 600);
      this.search.completeness.reason = controller.signal.aborted ? 'Поиск отменён; показана только полученная часть.' : 'Поиск прерван; список неполный.';
      this.onUpdate();
    });
    return clone(this.search);
  }

  async runSearch(originSystem, faction, serial, signal) {
    const names = await this.request(`/systems/field_values/system_names?q=${encodeURIComponent(originSystem)}`, {}, signal);
    const exact = (Array.isArray(names.min_max) ? names.min_max : []).find(row => equal(row.name, originSystem));
    if (!exact?.id64) throw new Error('Spansh не нашёл точного совпадения системы. Укажите полное название.');
    const response = await this.request(`/system/${encodeURIComponent(exact.id64)}`, {}, signal);
    const record = response.record;
    const coords = coordinate(record);
    if (!record || !coords || !equal(record.name, originSystem)) throw new Error('Spansh не вернул координаты выбранной системы.');
    if (serial !== this.searchSerial) return;
    this.search.origin = { name: record.name, id64: String(record.id64), coordinates: coords, population: natural(record.population), updatedAt: text(record.updated_at), source: 'spansh' };
    this.search.origins = [this.search.origin];
    this.search.factions = (record.minor_faction_presences || []).map(row => ({ name: text(row.name), influence: finite(row.influence) ? Number(row.influence) : null, government: text(row.government), allegiance: text(row.allegiance) })).filter(row => row.name);
    const stations = (record.stations || []).filter(row => !/carrier|megaship/i.test(String(row.type))).map(row => ({
      marketId: marketId(row.market_id), name: text(row.name), system: record.name, type: text(row.type),
      faction: factionName(row.controlling_minor_faction),
      hasColonisationContact: Array.isArray(row.services) && row.services.some(colonisationService) ? true : null,
      distanceLs: finite(row.distance_to_arrival) ? Number(row.distance_to_arrival) : null, updatedAt: text(record.updated_at), source: 'spansh',
    }));
    for (const observation of this.observedStations.values()) {
      if (!equal(observation.system, record.name)) continue;
      const known = stations.find(row => row.marketId === observation.marketId);
      const observed = { marketId: observation.marketId, name: observation.name, system: observation.system, faction: observation.faction,
        hasColonisationContact: observation.services.some(colonisationService) ? true : null,
        updatedAt: observation.timestamp, source: 'journal' };
      if (known) Object.assign(known, observed); else stations.push(observed);
    }
    this.search.claimStations = stations.filter(row => !faction || equal(row.faction, faction));
    for (const station of stations) if (station.faction && !this.search.factions.some(row => equal(row.name, station.faction))) this.search.factions.push({ name: station.faction, influence: null });
    if (faction && !this.search.claimStations.length) this.search.warnings.push('У выбранной фракции не найдено станции в системе происхождения. Одного присутствия фракции в системе недостаточно.');
    if (!this.search.claimStations.some(row => row.hasColonisationContact)) this.search.warnings.push('Наличие контакта колонизации не подтверждено. Проверьте службу и контролирующую фракцию станции в игре.');
    if (record.needs_permit) this.search.warnings.push('Система происхождения требует разрешение. Убедитесь, что можете посетить выбранную станцию.');
    this.search.warnings.push('База сообщества не гарантирует отсутствие свежей заявки. Окончательную доступность показывает контакт колонизации в игре.');
    this.search.status = 'searching'; this.onUpdate();
    const collected = new Map(), providerIds = new Set(); let providerTotal = null, pages = 0, providerRows = 0;
    for (let page = 0; page < 100; page += 1) {
      const data = await this.request('/systems/search', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({
        filters: { distance: { min: 0, max: RULES.rangeLy } },
        sort: [{ distance: { direction: 'asc' } }], size: 100, page, reference_coords: coords,
      }) }, signal);
      if (serial !== this.searchSerial) return;
      if (!Array.isArray(data.results) || !finite(data.count)) throw new Error('Неизвестный формат ответа Spansh. Список не считается полным.');
      const total = natural(data.count, 1000000000);
      if (providerTotal !== null && providerTotal !== total) this.search.warnings.push('Во время поиска база обновилась; число систем изменилось. Для воспроизводимого полного списка повторите поиск.');
      providerTotal = total; pages += 1; providerRows += data.results.length;
      for (const row of data.results) {
        providerIds.add(String(row.id64 || row.id || row.name));
        if (equal(row.name, record.name)) continue;
        const candidate = classifyCandidate(row, coords, new Date(this.now()).toISOString());
        if (candidate.distanceLy !== null && candidate.distanceLy > RULES.rangeLy + 0.000001) continue;
        collected.set(candidate.id, candidate);
      }
      this.search.results = [...collected.values()].sort((a, b) => (a.distanceLy ?? Infinity) - (b.distanceLy ?? Infinity) || a.name.localeCompare(b.name));
      this.search.completeness = { complete: false, providerTotal, fetched: providerRows, pages, reason: 'Загружаются страницы Spansh.' };
      this.onUpdate();
      if (providerRows >= providerTotal) {
        if (providerIds.size < providerTotal) throw new Error('Страницы Spansh содержат повторяющиеся записи; полнота списка не подтверждена.');
        this.search.completeness.complete = !this.search.warnings.some(row => row.startsWith('Во время поиска база'));
        this.search.completeness.reason = this.search.completeness.complete ? 'Получены все страницы систем, известных Spansh в радиусе 15 св. лет. Неизвестные базе системы не включены.' : 'База изменилась при поиске; повторите запрос.';
        break;
      }
      if (data.results.length === 0) throw new Error('Spansh вернул пустую страницу до конца списка.');
    }
    if (!this.search.completeness.complete && this.search.completeness.pages >= 100) this.search.completeness.reason = 'Достигнут предел 10 000 записей. Полученный список неполный.';
    this.search.running = false; this.search.status = 'complete'; this.controller = null; this.onUpdate();
  }

  cancel(notify = true) {
    this.searchSerial += 1;
    this.controller?.abort(); this.controller = null;
    if (this.search.running) {
      this.search.running = false; this.search.status = 'cancelled';
      this.search.completeness.complete = false; this.search.completeness.reason = 'Поиск отменён; список неполный.';
      if (notify) this.onUpdate();
    }
  }

  snapshot(context = {}) {
    const cargo = availableCargo(context.commander);
    const warehouse = availableWarehouse(context.carrier);
    const projects = this.projects.map(project => this.projectSnapshot(project, cargo, warehouse));
    const activeProject = projects.find(row => row.id === this.selectedProjectId) || null;
    const aggregateSupplies = activeProject?.supplies || [];
    const search = clone(this.search);
    search.originFactions = search.factions;
    return { ...clone(this.config), config: clone(this.config), selectedProjectId: this.selectedProjectId, projects, activeProject,
      catalog: clone(CATALOG), economies: clone(ECONOMIES), rules: { ...RULES }, sources: clone(SOURCES),
      search, aggregateSupplies,
      depots: [...this.depots.values()].map(row => ({ ...clone(row), stationName: row.station, linked: this.projects.some(p => p.buildings.some(b => b.marketId === row.marketId)) })),
      claims: [...this.claims.values()].map(clone),
      cargoKnown: cargo !== null, carrierWarehouseComplete: warehouse?.complete ?? false,
      journalContributionCount: this.contributions.length,
      limitations: ['Список кандидатов отражает только наблюдения Spansh; заявки и разрешения проверяются в игре.',
        'Точная потребность загружается из журнала при посещении строительного депо. Ручные планы помечены как оценки.',
        'Баланс очков задаётся по экрану архитектора. Экономические направления не являются прогнозом объёмов рынка.'] };
  }

  projectSnapshot(project, cargo, warehouse) {
    const points = { ...project.points };
    let portsStarted = project.externalPortCount;
    // Costs escalate as soon as a non-primary T2/T3 port is started, regardless
    // of the position of that already-started port in the editable future queue.
    for (const row of project.buildings) {
      const type = TYPE_BY_ID.get(row.typeId);
      const tier = row.tierOverride || type.tier;
      if (type.kind === 'port' && tier >= 2 && !row.isPrimary && ['building', 'complete'].includes(this.statusOf(row))) portsStarted += 1;
    }
    const warnings = [];
    const buildings = project.buildings.map((building, index) => {
      const type = TYPE_BY_ID.get(building.typeId);
      const tier = building.tierOverride || type.tier;
      const status = this.statusOf(building);
      const depot = this.depots.get(building.marketId);
      const manual = new Map();
      for (const row of project.deliveries) if (row.buildingId === building.id) manual.set(row.commodity, (manual.get(row.commodity) || 0) + row.count);
      const own = this.contributions.filter(row => row.marketId === building.marketId);
      const resources = depot ? depot.requirements.map(row => {
        const pendingOwn = own.filter(contribution => contribution.sequence > depot.sequence).reduce((sum, contribution) => sum + contribution.resources.filter(r => r.commodity === row.commodity).reduce((n, r) => n + r.count, 0), 0);
        const delivered = row.delivered + pendingOwn;
        return { ...row, observedDelivered: row.delivered, pendingOwn, delivered, missing: Math.max(0, row.required - delivered), manualDelivered: manual.get(row.commodity) || 0,
          source: pendingOwn ? 'journal+contribution' : 'journal', updatedAt: depot.timestamp };
      }) : building.resourcePlan.map(row => ({ ...row, delivered: manual.get(row.commodity) || 0, manualDelivered: manual.get(row.commodity) || 0,
        missing: Math.max(0, row.required - (manual.get(row.commodity) || 0)), source: 'manual', sourceNote: building.sourceNote, authoritative: false }));
      for (const row of resources) {
        row.cargoAvailable = cargo ? cargo.get(row.commodity) || 0 : null;
        row.carrierAvailable = warehouse ? warehouse.items.has(row.commodity) ? warehouse.items.get(row.commodity) : warehouse.complete ? 0 : null : null;
      }
      const ownWarnings = [];
      if (status === 'failed') ownWarnings.push('Журнал сообщает о провале строительства.');
      if (!resources.length && status !== 'complete' && status !== 'cancelled') ownWarnings.push('Потребность в товарах неизвестна. Посетите депо или добавьте ручную оценку с источником.');
      if (resources.some(row => !row.authoritative)) ownWarnings.push('План ресурсов — ручная оценка; он будет заменён требованиями связанного депо.');
      if (building.marketId && !depot) ownWarnings.push('Депо связано, но его требования ещё не прочитаны из журнала.');
      if (tier === null) ownWarnings.push('Уровень и строительные очки этого варианта требуют ручного подтверждения.');
      for (const id of building.dependsOn) {
        const dependencyIndex = project.buildings.findIndex(row => row.id === id);
        const dependency = project.buildings[dependencyIndex];
        if (!dependency) ownWarnings.push('Зависимый объект отсутствует.');
        else if (dependencyIndex > index && !['complete', 'building'].includes(this.statusOf(dependency))) ownWarnings.push(`Сначала нужен объект «${dependency.name}», стоящий позже в очереди.`);
        else if (['cancelled', 'failed'].includes(this.statusOf(dependency))) ownWarnings.push(`Зависимый объект «${dependency.name}» отменён или строительство провалено.`);
      }
      let pointCost = building.pointCostOverride;
      if (pointCost === null) pointCost = building.isPrimary ? 0 : type.kind === 'port' && tier === 2 ? 3 + Math.max(0, portsStarted - 1) * 2 : type.kind === 'port' && tier === 3 ? 6 * Math.max(1, portsStarted) : type.pointCost;
      const pointReward = building.pointRewardOverride ?? type.pointReward;
      const pointsBefore = { ...points };
      if (status === 'planned') {
        if (tier === 2 || tier === 3) {
          const key = tier === 2 ? 'tier2' : 'tier3';
          if (pointCost === 0) { /* Primary port does not require construction points. */ }
          else if (pointCost === null || points[key] === null) ownWarnings.push(`Баланс очков T${tier} неизвестен. Введите текущий баланс по экрану архитектора.`);
          else {
            if (points[key] < pointCost) ownWarnings.push(`Не хватает ${pointCost - points[key]} очк. T${tier} для начала строительства.`);
            points[key] -= pointCost;
          }
        }
        if (type.kind === 'port' && tier >= 2 && !building.isPrimary) portsStarted += 1;
      }
      if (['planned', 'building'].includes(status)) {
        const key = tier === 1 ? 'tier2' : tier === 2 ? 'tier3' : null;
        if (key && points[key] !== null && pointReward !== null) points[key] += pointReward;
      }
      return { ...clone(building), manualStatus: building.status, tier, kind: type.kind, location: type.location, economy: type.economy, status, effectiveStatus: status,
        pointCost, pointReward, pointsBefore, pointsAfter: { ...points }, pointSource: building.pointCostOverride !== null || building.pointRewardOverride !== null ? 'manual' : type.source,
        requirements: resources, authoritative: Boolean(depot), billSource: depot ? 'journal' : resources.length ? 'manual' : 'unknown', warnings: ownWarnings, prerequisites: [...type.prerequisites],
        progress: depot?.constructionProgress ?? null, depotUpdatedAt: depot?.timestamp || null,
        ownDelivered: own.reduce((sum, row) => sum + row.resources.reduce((n, r) => n + r.count, 0), 0) };
    });
    if (project.points.tier2 === null || project.points.tier3 === null) warnings.push('Укажите текущие доступные очки T2/T3. Уже построенные объекты включены в этот баланс и повторно очки не начисляют.');
    const concurrent = buildings.filter(row => row.status === 'building').length;
    if (concurrent > RULES.maxConcurrent) warnings.push('В плане более пяти одновременных строек. Проверьте фактическое ограничение у архитектора.');
    const claim = [...this.claims.values()].find(row => project.systemAddress && row.systemAddress === project.systemAddress || row.system && equal(row.system, project.targetSystem));
    const claimedAt = project.claimedAt || claim?.claimedAt || null;
    // A beacon observation does not contain the claim timestamp. Only an explicit
    // claim record or manual date can supply this planning estimate.
    const deadline = project.deadline || (claimedAt ? new Date(Date.parse(claimedAt) + RULES.primaryDays * 86400000).toISOString() : null);
    const primaryComplete = buildings.some(row => row.isPrimary && row.status === 'complete');
    if (deadline && !primaryComplete && !['complete', 'cancelled'].includes(project.status) && Date.parse(deadline) < this.now()) warnings.push('Расчётный срок первичного порта истёк. Актуальный таймер проверьте в игре.');
    const supplies = aggregateResources(buildings, cargo, warehouse, this.config.cargoCapacity);
    let status = project.status;
    if (status === 'planned' && claim && claim.status !== 'released') status = claim.status === 'beacon' ? 'building' : 'claimed';
    if (status === 'planned' && buildings.some(row => ['building', 'complete'].includes(row.status))) status = 'building';
    if (!['complete', 'cancelled'].includes(status) && buildings.some(row => row.status === 'complete') && buildings.every(row => ['complete', 'cancelled'].includes(row.status))) status = 'complete';
    if (claim?.status === 'released') warnings.push('Журнал содержит отказ от заявки. Проверьте действительность проекта в игре.');
    return { ...clone(project), status, manualStatus: project.status, buildings, supplies, aggregateSupplies: supplies,
      claimedAt, deadline, deadlineSource: project.deadline ? 'manual' : claimedAt ? 'claim-estimate' : null,
      beaconDeadline: claimedAt ? new Date(Date.parse(claimedAt) + RULES.beaconHours * 3600000).toISOString() : null,
      claimObservation: claim ? clone(claim) : null, pointsProjected: points, warnings,
      concurrent, unknownBills: buildings.filter(row => !['complete', 'cancelled'].includes(row.status) && !row.requirements.length).length,
      totalRequired: supplies.reduce((sum, row) => sum + row.required, 0),
      totalMissing: supplies.reduce((sum, row) => sum + row.missing, 0),
      manualDeliveries: clone(project.deliveries),
      journalDeliveries: this.contributions.filter(row => project.buildings.some(b => b.marketId && b.marketId === row.marketId)).map(clone),
      economyPlan: economyPlan(project.goalEconomy, buildings),
      buildAdvice: buildAdvice(project, buildings),
      nextTask: nextTask({ ...project, status }, buildings, supplies, deadline) };
  }

  statusOf(building) {
    const depot = this.depots.get(building.marketId);
    return depot?.failed ? 'failed' : depot?.complete ? 'complete' : depot ? 'building' : building.status;
  }
}

function classifyCandidate(row, originCoordinates, checkedAt) {
  const coords = coordinate(row), distanceLy = distance(coords, originCoordinates);
  const reasons = [], unknown = [];
  const population = natural(row.population);
  const permanent = (Array.isArray(row.stations) ? row.stations : []).filter(station => !/carrier/i.test(String(station.type)));
  if (population > 0) reasons.push('Есть население.');
  if (population === null) unknown.push('Население неизвестно.');
  if (row.needs_permit === true) reasons.push('Требуется разрешение.');
  if (row.is_colonised === true) reasons.push('Система уже колонизирована.');
  if (row.is_being_colonised === true) reasons.push('Отмечена активная колонизация.');
  if (permanent.length) reasons.push('В базе есть постоянные станции или поселения.');
  if (row.needs_permit !== true && row.needs_permit !== false) unknown.push('Разрешение не проверено.');
  if (row.is_colonised !== true && row.is_colonised !== false) unknown.push('Статус колонии не наблюдался.');
  if (row.is_being_colonised !== true && row.is_being_colonised !== false) unknown.push('Свежая заявка не проверена.');
  if (!coords) unknown.push('Нет координат.');
  if (distanceLy !== null && distanceLy > RULES.rangeLy + 0.000001) reasons.push('Дальше 15 св. лет от системы происхождения.');
  const availability = reasons.length ? 'unavailable' : unknown.length ? 'unknown' : 'candidate';
  return { id: String(row.id64 || row.id || row.name), id64: String(row.id64 || row.id || ''), system: text(row.name), name: text(row.name),
    coordinates: coords, distanceLy, population, permit: typeof row.needs_permit === 'boolean' ? row.needs_permit : null,
    isColonised: typeof row.is_colonised === 'boolean' ? row.is_colonised : null,
    isBeingColonised: typeof row.is_being_colonised === 'boolean' ? row.is_being_colonised : null,
    availability, reasons: [...reasons, ...unknown], factions: (row.minor_faction_presences || []).map(f => text(f.name)),
    permanentStationCount: permanent.length, updatedAt: text(row.updated_at), checkedAt, source: 'spansh',
    url: row.id64 || row.id ? `https://spansh.co.uk/system/${encodeURIComponent(row.id64 || row.id)}` : null };
}

function availableCargo(commander) {
  if (!commander || commander.vehicle && commander.vehicle !== 'ship' || !Array.isArray(commander.cargoForSync)) return null;
  const result = new Map();
  for (const row of commander.cargoForSync) {
    if (row.MissionID && Number(row.MissionID) !== 0) continue;
    const key = normalize(row.Name), count = natural(row.Count);
    if (!key || count === null) continue;
    const legal = Math.max(0, count - (natural(row.Stolen) || 0));
    result.set(key, (result.get(key) || 0) + legal);
  }
  return result;
}
function availableWarehouse(carrier) {
  if (!carrier?.owned || !carrier.warehouse || !Array.isArray(carrier.warehouse.items)) return null;
  const items = new Map();
  for (const row of carrier.warehouse.items) {
    const key = normalize(row.key || row.commodity || row.name), count = natural(row.count);
    if (key && count !== null) items.set(key, (items.get(key) || 0) + count);
  }
  return { items, complete: carrier.warehouse.complete === true };
}
function aggregateResources(buildings, cargo, warehouse, capacity) {
  const rows = new Map();
  for (const building of buildings) {
    if (['complete', 'cancelled', 'failed'].includes(building.status)) continue;
    for (const resource of building.requirements) {
      let row = rows.get(resource.commodity);
      if (!row) { row = { commodity: resource.commodity, name: resource.name, required: 0, delivered: 0, missing: 0, manualDelivered: 0, authoritative: true, sources: [], buildings: [] }; rows.set(resource.commodity, row); }
      row.required += resource.required; row.delivered += resource.delivered; row.missing += resource.missing; row.manualDelivered += resource.manualDelivered || 0;
      row.authoritative = row.authoritative && resource.authoritative; row.sources.push(resource.source);
      row.buildings.push({ id: building.id, name: building.name, missing: resource.missing });
    }
  }
  return [...rows.values()].map(row => {
    const cargoAvailable = cargo ? cargo.get(row.commodity) || 0 : null;
    const carrierAvailable = warehouse ? warehouse.items.has(row.commodity) ? warehouse.items.get(row.commodity) : warehouse.complete ? 0 : null : null;
    const toLoad = cargoAvailable === null ? null : Math.max(0, row.missing - cargoAvailable);
    const toBuy = toLoad === null || carrierAvailable === null ? null : Math.max(0, toLoad - carrierAvailable);
    return { ...row, source: [...new Set(row.sources)].join('+'), cargoAvailable, carrierAvailable, toLoad, toBuy,
      trips: capacity > 0 ? Math.ceil(row.missing / capacity) : null,
      coveredByCargo: cargoAvailable === null ? null : Math.min(row.missing, cargoAvailable),
      carrierComplete: warehouse?.complete ?? false };
  }).sort((a, b) => b.missing - a.missing || a.name.localeCompare(b.name, 'ru'));
}
function economyPlan(goal, buildings) {
  const selected = ECONOMIES.find(row => row.id === goal);
  const relevant = buildings.filter(row => row.economy === goal);
  return { goal: selected?.id || '', name: selected?.name || 'Не задана',
    supportingBuildings: relevant.map(row => ({ id: row.id, name: row.name, body: row.body })),
    guidance: goal === 'refinery' ? 'Перерабатывающий узел — профильная поддержка. Проверяйте связи с портом на том же небесном теле и итоговые пропорции экономик в игре.'
      : selected ? 'Размещайте профильные объекты с учётом связей с портом и свойств небесного тела. Проверяйте итоговую экономику по данным игры.' : 'Выберите направление экономики для плана развития.',
    exactSimulation: false, warning: 'Направление объектов не предсказывает точные доли экономик, производство, население или доход. Необходимые слоты и зависимости подтверждаются на карте системы.' };
}
function buildAdvice(project, buildings) {
  const advice = [];
  const primary = buildings.find(row => row.isPrimary && row.status !== 'cancelled');
  if (primary?.status === 'complete') {
    advice.push({ title: 'Сохраните роль первичного порта', text: 'Первичный порт уже построен: его нельзя снести или заменить. Дальнейшее развитие планируйте дополнительными объектами и проверяйте связи с портами.', typeIds: [primary.typeId] });
  } else {
    advice.push({ title: 'Сначала выберите первичный порт', text: 'Для первого шага и дальнейшего расширения рассмотрите компактный орбитальный аванпост. Если важен приём больших грузовых кораблей, рассмотрите Кориолис с большими площадками. Сравните фактическую потребность в товарах: размер очереди перевозок определяет скорость стройки. После завершения первичный порт нельзя заменить.', typeIds: ['outpost-civilian', 'outpost-commercial', 'coriolis'] });
  }
  advice.push({ title: 'Стройте ступенями: T1 → T2 → T3', text: 'Объекты T1 дают очки T2. За них строятся узлы и большие поселения T2, которые дают очки T3 для порта T3. Большое поселение T2 даёт два очка T3, обычный узел T2 — одно. Дополнительные начатые порты T2/T3 повышают стоимость следующих портов; первичный в счётчик не входит. Если нужны порты T3, сравните очередь с их постройкой раньше дополнительных портов T2.', typeIds: ['settlement-medium', 'settlement-large', 'hub-industrial', 'orbis', 'ocellus'] });
  const directions = {
    refinery: { title: 'Развитие переработки', text: 'Рассмотрите перерабатывающий узел и порт у того же небесного тела. Проверьте свойства тела и связи на карте системы: один профильный объект не гарантирует чистую перерабатывающую экономику.', typeIds: ['hub-refinery', 'coriolis'] },
    industrial: { title: 'Промышленная инфраструктура', text: 'Рассмотрите промышленный узел и промышленный аванпост. Для поселений выберите промышленную специализацию в игре. Проверьте, какие объекты будут связаны с выбранным портом.', typeIds: ['hub-industrial', 'outpost-industrial', 'surface-outpost-industrial', 'settlement-large'] },
    extraction: { title: 'Добывающая инфраструктура', text: 'Рассмотрите добывающий узел и добывающую установку. Для подходящего места в кольце возможна астероидная база. Размещение и доступность конкретного варианта подтвердите на карте системы.', typeIds: ['hub-extraction', 'installation-mining', 'asteroid-base'] },
    agriculture: { title: 'Сельскохозяйственная инфраструктура', text: 'Рассмотрите космическую ферму и поселение с сельскохозяйственной специализацией. Выбирайте подходящие тела и связи с портом; потребность и производство проверьте по итоговой экономике в игре.', typeIds: ['installation-farm', 'settlement-medium', 'settlement-large'] },
    hightech: { title: 'Высокотехнологичная инфраструктура', text: 'Рассмотрите высокотехнологичный или научный узел, научный аванпост и исследовательскую установку. Проверяйте местные связи с портом и совместимость итоговых экономик.', typeIds: ['hub-hightech', 'hub-scientific', 'outpost-scientific', 'installation-research'] },
    military: { title: 'Военная инфраструктура', text: 'Рассмотрите военный узел, военный аванпост и военную установку; для поселения выберите военную специализацию. Доступные услуги и итоговые экономики подтвердите в игре.', typeIds: ['hub-military', 'outpost-military', 'installation-military', 'settlement-large'] },
    tourism: { title: 'Туристическая инфраструктура', text: 'Рассмотрите туристическую установку и туристическое поселение. Учитывайте свойства тела и местные связи с портом. Точный доход по этому направлению заранее не рассчитывается.', typeIds: ['installation-tourist', 'settlement-medium', 'settlement-large'] },
    service: { title: 'Инфраструктура услуг', text: 'Рассмотрите опорный узел и космический бар, затем проверьте доступные услуги у связанных портов. Наличие конкретного объекта само по себе не подтверждает требуемую экономику порта.', typeIds: ['hub-outpost', 'installation-bar'] },
    mixed: { title: 'Смешанное развитие', text: 'Сначала определите, какие услуги и товары нужны вашим маршрутам. Выберите профильные узлы и поселения, затем сравните связи с портом. Чем больше направлений смешано, тем важнее проверять фактические доли экономик в игре.', typeIds: ['hub-refinery', 'hub-industrial', 'hub-hightech'] },
  };
  const direction = directions[project.goalEconomy];
  if (direction) advice.push(direction);
  else advice.push({ title: 'Выберите цель развития', text: 'Задайте направление экономики в параметрах проекта. Тогда план предложит подходящие семейства объектов. Пропорции экономик, доступные слоты и рыночный результат подтверждаются в игре.' });

  const deficits = buildings.filter(row => row.status === 'planned' && [2, 3].includes(row.tier)
    && row.pointCost > 0 && row.pointsBefore[`tier${row.tier}`] !== null
    && row.pointsBefore[`tier${row.tier}`] < row.pointCost);
  for (const row of deficits.slice(0, 3)) {
    const missing = row.pointCost - row.pointsBefore[`tier${row.tier}`];
    advice.push({ title: `Подготовьте очки перед «${row.name}»`, text: row.tier === 2
      ? `По текущей очереди перед этим этапом не хватает ${missing} очк. T2. Поставьте раньше подходящие объекты T1 и завершите их. Затем обновите текущий баланс по экрану архитектора. Не расходуйте одни и те же очки на параллельные стройки.`
      : `По текущей очереди перед этим этапом не хватает ${missing} очк. T3. Поставьте раньше нужные узлы или большие поселения T2 и завершите их; сначала обеспечьте их очками T2. После этого обновите баланс по экрану архитектора.`,
      typeIds: row.tier === 2 ? ['settlement-medium', 'outpost-civilian'] : ['settlement-large', 'hub-industrial', 'hub-refinery'] });
  }
  if (deficits.length > 3) advice.push({ title: 'Проверьте оставшуюся очередь', text: `Ещё ${deficits.length - 3} этапов имеют расчётный дефицит очков. Начните с первого предупреждения в очереди и пересчитайте план после изменения порядка.` });
  if (!deficits.length && (project.points.tier2 === null || project.points.tier3 === null))
    advice.push({ title: 'Уточните исходный баланс очков', text: 'Введите доступные очки T2 и T3 по экрану архитектора. Это текущий остаток, уже учитывающий существующие здания и начатые стройки. Пока баланс неизвестен, возможность начать следующий этап не подтверждена.' });
  return advice;
}
function nextTask(project, buildings, supplies, deadline) {
  if (['complete', 'cancelled'].includes(project.status)) return null;
  if (project.status === 'planned') return { kind: 'claim', text: `Проверьте заявку на ${project.targetSystem} у контакта колонизации.`, system: project.originSystem };
  const current = buildings.find(row => ['building', 'planned'].includes(row.status));
  if (!current) return { kind: 'plan', text: 'Добавьте следующий объект в план строительства.', system: project.targetSystem };
  const needed = current.requirements.filter(row => row.missing > 0).sort((a, b) => b.missing - a.missing)[0];
  return { kind: needed ? 'delivery' : current.requirements.length ? 'complete' : 'inspect-depot',
    text: needed ? `${current.name}: доставить ${needed.name} — ${needed.missing} т` : current.requirements.length ? `${current.name}: проверьте завершение строительства в игре.` : `${current.name}: посетите депо для точного списка товаров.`,
    system: project.targetSystem, buildingId: current.id, marketId: current.marketId || null, deadline,
    commodity: needed?.commodity || null, count: needed?.missing ?? null, authoritative: needed?.authoritative ?? false };
}

module.exports = { ColonizationPlanner, CATALOG, SOURCES, RULES, classifyCandidate, availableCargo, availableWarehouse };
