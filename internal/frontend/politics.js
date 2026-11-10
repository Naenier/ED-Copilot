/* Galaxy views use dated public observations and the commander's game journal. */
(() => {
'use strict';

const views = [
  {id:'overview', mode:'Политика', en:'Politics', icon:'orbit'},
  {id:'goals', mode:'Цели сообщества', en:'Community goals', icon:'target'},
  {id:'powerplay', mode:'Пауэрплей', en:'Powerplay', icon:'shield-check'},
  {id:'factions', mode:'Фракции и конфликты', en:'Factions & conflicts', icon:'users'},
];
const terms = {
  None:['None','Нет'], War:['War','Война'], CivilWar:['Civil war','Гражданская война'],
  Election:['Election','Выборы'], Elections:['Elections','Выборы'], CivilLiberty:['Civil liberty','Гражданская свобода'],
  CivilUnrest:['Civil unrest','Гражданские беспорядки'], Boom:['Boom','Бум'], Bust:['Bust','Спад'],
  Expansion:['Expansion','Экспансия'], Retreat:['Retreat','Отступление'], Famine:['Famine','Голод'],
  Outbreak:['Outbreak','Эпидемия'], Lockdown:['Lockdown','Блокада'], Drought:['Drought','Засуха'],
  InfrastructureFailure:['Infrastructure failure','Отказ инфраструктуры'], Investment:['Investment','Инвестиции'],
  PublicHoliday:['Public holiday','Праздник'], Stronghold:['Stronghold','Оплот'], Fortified:['Fortified','Укреплённая'],
  Exploited:['Exploited','Эксплуатируемая'], Unoccupied:['Unoccupied','Незанятая'], Contested:['Contested','Оспариваемая'],
  Controlled:['Controlled','Контролируемая'], active:['Active','Активен'], pending:['Pending','Ожидается'],
  resolved:['Resolved','Завершён'], Federation:['Federation','Федерация'], Empire:['Empire','Империя'],
  Alliance:['Alliance','Альянс'], Independent:['Independent','Независимая'], PilotsFederation:['Pilots Federation','Федерация пилотов'],
  Anarchy:['Anarchy','Анархия'], Confederacy:['Confederacy','Конфедерация'], Cooperative:['Cooperative','Кооператив'],
  Corporate:['Corporate','Корпорация'], Communism:['Communism','Коммунизм'], Democracy:['Democracy','Демократия'],
  Dictatorship:['Dictatorship','Диктатура'], Feudal:['Feudal','Феодализм'], Patronage:['Patronage','Патронаж'],
  PrisonColony:['Prison colony','Тюремная колония'], Theocracy:['Theocracy','Теократия'],
  High:['High','Высокая'], Medium:['Medium','Средняя'], Low:['Low','Низкая'], Lawless:['Lawless','Беззаконие'],
};
const model = C => C.state?.politics || {};
const language = C => (C.state?.settings?.language || window.CopilotI18n?.language) === 'ru' ? 'ru' : 'en';
const text = (C,en,ru) => language(C) === 'ru' ? ru : en;
const list = raw => Array.isArray(raw) ? raw : [];
const known = value => (typeof value === 'number' || typeof value === 'string' && value.trim() !== '') && Number.isFinite(Number(value));
const present = value => value !== undefined && value !== null && value !== '';
const termKey = value => String(value).replace(/^\$(?:government_|system_security_|galaxy_map_info_state_)/i,'').replace(/;$/,'').replace(/[\s_-]/g,'').toLowerCase();
const termPairs = Object.fromEntries(Object.entries(terms).flatMap(([key,value]) => [[termKey(key),value],[termKey(value[1]),value]]));
const term = (C,value) => termPairs[termKey(value)] ? text(C,...termPairs[termKey(value)]) : value;
const powerName = name => name === 'A. Lavigny-Duval' ? 'Arissa Lavigny-Duval' : name;
const esc = (u,value) => u.esc(String(value ?? ''));
const label = (C,u,en,ru) => esc(u,text(C,en,ru));
const num = (u,value) => known(value) ? esc(u,u.fmt ? u.fmt(Number(value)) : value) : '—';

function percent(C,u,value) {
  if (!known(value)) return '—';
  return esc(u,new Intl.NumberFormat(language(C),{style:'percent',maximumFractionDigits:2}).format(Number(value)));
}
function date(C,u,value) {
  if (!Number.isFinite(Date.parse(value))) return '';
  return esc(u,new Date(value).toLocaleString(language(C),{dateStyle:'medium',timeStyle:'short'}));
}
function button(C,u,en,ru,action,attrs='',disabled=false) {
  return `<button type="button" class="pill" data-click="politics-${action}" ${attrs} ${disabled?'disabled':''}>${label(C,u,en,ru)}</button>`;
}
function systemButton(u,name) {
  if (!name) return '';
  return `<button type="button" class="pol-link" data-click="politics-system" data-system="${esc(u,name)}">${esc(u,name)}</button>`;
}
function empty(C,u,en,ru) {
  return `<p class="pol-empty">${label(C,u,en,ru)}</p>`;
}
function timestamp(C,u,data={}) {
  if (data.quality && window.FreshnessUI) return window.FreshnessUI.observation(data.quality,language(C),u.esc);
  const value = date(C,u,data.observedAt || data.fetchedAt);
  if (!value) return '';
  const title = data.observedAt ? label(C,u,'Observed','Наблюдение') : label(C,u,'Updated','Обновлено');
  return `<div class="pol-timestamp${data.stale?' pol-stale':''}">${title} · ${value}</div>`;
}
function panel(C,u,title,body,data={},classes='') {
  return `<section class="panel pol-panel ${classes}"><header class="pol-panel-head"><h2>${esc(u,title)}</h2>${timestamp(C,u,data)}</header>${body}</section>`;
}
function facts(C,u,rows) {
  const visible = rows.filter(([, ,value]) => present(value) && value !== '—');
  if (!visible.length) return '';
  return `<dl class="pol-facts">${visible.map(([en,ru,value]) =>
    `<div><dt>${label(C,u,en,ru)}</dt><dd>${value}</dd></div>`
  ).join('')}</dl>`;
}
function progress(C,u,value,subject) {
  if (!known(value)) return '';
  const scaled = Math.max(0,Math.min(100,Number(value)*100));
  return `<div class="pol-progress" role="progressbar" aria-label="${esc(u,subject)}" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${scaled}"><span style="width:${scaled}%"></span></div>`;
}
function factionRows(s) {
  return list(s?.factions?.entries).filter(row => row?.name);
}
function conflictRows(s) {
  return list(s?.conflicts?.entries).filter(row => row?.faction1?.name && row?.faction2?.name);
}
function hasFactions(C) {
  const snapshot = C.state?.politics;
  if (!snapshot) return Boolean(C.politicsUI?.hasFactions);
  return factionRows(snapshot.activeSystem).length > 0 || conflictRows(snapshot.activeSystem).length > 0;
}
function navigationModes(C) {
  return views.filter(item => item.id !== 'factions' || hasFactions(C)).map(item => ({
    title:item.mode, mode:item.mode, icon:item.icon, tab:'politics',
  }));
}
function currentView(C) {
  const view = views.find(item => item.mode === C.mode);
  return view?.id === 'factions' && !hasFactions(C) ? views[0] : view || views[0];
}
function local(C) {
  if (C.politicsUI) return C.politicsUI;
  const snapshot = model(C);
  const powerSearch = snapshot.search?.kind === 'powerplay' ? snapshot.search : {};
  return C.politicsUI = {
    system:snapshot.selectedSystem || snapshot.currentSystem || '',
    systemContext:snapshot.selectedSystem || snapshot.currentSystem || '',
    power:powerSearch.power || '', state:powerSearch.state || '', page:Number(powerSearch.page) || 0,
    pending:null, dispatching:false, attempted:new Map(), errors:new Set(),
  };
}
function selectedSystem(C) {
  return model(C).selectedSystem || model(C).currentSystem || '';
}
function searchForm(C,u) {
  const draft = local(C);
  const system = selectedSystem(C);
  if (draft.systemContext !== system) {
    draft.system = system;
    draft.systemContext = system;
  }
  return `<form id="politics-system-form" class="pol-system-form">
    <label><span>${label(C,u,'Star system','Звёздная система')}</span>
      <input id="politics-system" name="system" data-politics-field="system" data-system-search="single" autocomplete="off" maxlength="200"
        value="${esc(u,draft.system)}" placeholder="${label(C,u,'System name','Название системы')}"></label>
    <button class="pill primary" type="submit">${label(C,u,'Find system','Найти систему')}</button>
    ${model(C).currentSystem ? button(C,u,'Current system','Текущая система','current') : ''}
    ${system ? button(C,u,'Copy name','Скопировать название','copy',`data-system="${esc(u,system)}"`) : ''}
  </form>`;
}
function powerDetails(C,u,p) {
  const items = [
    ['Controlling power','Контролирующая держава',esc(u,powerName(p.controllingPower))],
    ['Status','Состояние',esc(u,term(C,p.state))],
    ['Control progress','Прогресс контроля',percent(C,u,p.controlProgress)],
    ['Reinforcement','Укрепление',num(u,p.reinforcement)],
    ['Undermining','Подрыв',num(u,p.undermining)],
  ];
  if (list(p.powers).length > 1) {
    items.push(['Powers present','Державы в системе',list(p.powers).map(name => esc(u,powerName(name))).join(' · ')]);
  }
  return facts(C,u,items);
}
function factionItem(C,u,row,controller,compact=false) {
  const active = [...new Set([...list(row.activeStates),row.state])]
    .filter(value => value && termKey(value) !== 'none');
  const extra = [row.allegiance,row.government].filter(Boolean).map(value => esc(u,term(C,value))).join(' · ');
  const states = [
    ['Active','Действуют',active],
    ['Pending','Ожидаются',list(row.pendingStates)],
    ['Recovering','Завершаются',list(row.recoveringStates)],
  ].filter(([, ,values]) => values.length > 0);
  return `<article class="pol-faction"><header><div><strong>${esc(u,row.name)}</strong>
    ${row.name===controller ? `<span class="pol-chip">${label(C,u,'Controlling','Правящая')}</span>` : ''}
    ${!compact && extra ? `<small>${extra}</small>` : ''}</div>
    ${known(row.influence) ? `<strong class="pol-faction-value">${percent(C,u,row.influence)}</strong>` : ''}</header>
    ${progress(C,u,row.influence,text(C,'Faction influence','Влияние фракции'))}
    ${!compact ? facts(C,u,states.map(([en,ru,values]) => [en,ru,values.map(x => esc(u,term(C,x))).join(' · ')])) : ''}
  </article>`;
}
function overview(C,u) {
  const snapshot = model(C);
  const system = snapshot.activeSystem || {};
  const info = system.info || {};
  const p = system.powerplay || {};
  const factsHTML = facts(C,u,[
    ['Controlling faction','Правящая фракция',esc(u,info.controllingFaction)],
    ['Faction state','Состояние фракции',esc(u,term(C,info.state))],
    ['Allegiance','Принадлежность',esc(u,term(C,info.allegiance))],
    ['Government','Правительство',esc(u,term(C,info.government))],
    ['Security','Безопасность',esc(u,term(C,info.security))],
    ['Population','Население',num(u,info.population)],
  ]);
  const politics = factsHTML ? panel(C,u,system.name,factsHTML,info) : '';
  const power = powerDetails(C,u,p);
  const factions = factionRows(system);
  const conflicts = conflictRows(system);
  const factionsCard = factions.length || conflicts.length ? panel(C,u,text(C,'Local factions','Местные фракции'),
    factions.slice(0,3).map(row => factionItem(C,u,row,info.controllingFaction,true)).join('') +
    `<div class="pol-footer-row">${button(C,u,'Factions & conflicts','Фракции и конфликты','mode','data-view="factions"')}</div>`,system.factions) : '';
  const recent = list(snapshot.systems).filter(row => row.name && row.name !== system.name)
    .sort((a,b) => String(b.updatedAt || '').localeCompare(String(a.updatedAt || ''))).slice(0,8);
  const summary = [politics,power ? panel(C,u,text(C,'Power control','Контроль державы'),power,p) : '',factionsCard].filter(Boolean).join('');
  const overviewEmpty = !summary && !loading(C) ? empty(C,u,system.name?'No political observations for this system.':'Choose a star system.',system.name?'Для этой системы нет политических данных.':'Выберите звёздную систему.') : '';
  return searchForm(C,u) + notice(C,u) + `<div class="pol-overview">${summary}</div>` + overviewEmpty +
    (recent.length ? panel(C,u,text(C,'Recent systems','Недавние системы'),`<div class="pol-system-list">${recent.map(row => systemButton(u,row.name)).join('')}</div>`) : '');
}
function goalCards(C,u,goals,personal) {
  return `<div class="pol-goals">${goals.map(goal => {
    const fraction = known(goal.currentTotal) && Number(goal.target) > 0 ? Number(goal.currentTotal)/Number(goal.target) : null;
    const progressValue = fraction === null ? '' : `${num(u,goal.currentTotal)} / ${num(u,goal.target)} · ${percent(C,u,fraction)}`;
    const location = [systemButton(u,goal.system),esc(u,goal.station)].filter(Boolean).join(' · ');
    const status = personal && goal.discarded ? text(C,'Discarded','Отказ') :
      goal.complete === true ? text(C,'Complete','Завершена') :
      goal.expiry && Date.parse(goal.expiry) < Date.now() ? text(C,'Ended','Окончена') :
      goal.expiry || goal.complete === false ? text(C,'In progress','Выполняется') : '';
    return `<article class="panel pol-goal">
      <header class="pol-goal-head"><div><h2>${esc(u,goal.title || goal.id)}</h2>${location ? `<div class="pol-goal-location">${location}</div>` : ''}</div>
        ${status ? `<span class="pol-chip">${esc(u,status)}</span>` : ''}</header>
      ${goal.objective ? `<p class="pol-objective">${esc(u,goal.objective)}</p>` : ''}
      ${goal.commodities ? `<p>${esc(u,goal.commodities)}</p>` : ''}
      ${progress(C,u,fraction,text(C,'Goal progress','Ход цели'))}
      ${facts(C,u,[
        ['Contributions','Внесено',progressValue || num(u,goal.currentTotal)],
        ['Ends','Окончание',date(C,u,goal.expiry)],
        ...personal ? [
          ['Your contribution','Ваш вклад',num(u,goal.playerContribution)],
          ['Tier','Ступень',esc(u,goal.tier)],
          ['Percentile band','Группа участников',known(goal.percentileBand)?percent(C,u,Number(goal.percentileBand)/100):''],
          ['Contributors','Участники',num(u,goal.contributors)],
          ['Bonus','Бонус',num(u,goal.bonus)],
          ['Reward received','Награда получена',num(u,goal.rewardReceived)],
        ] : [],
      ])}
      ${goal.description ? `<details><summary>${label(C,u,'Objective and rewards','Условия и награды')}</summary><p class="pol-description">${esc(u,goal.description)}</p></details>` : ''}
    </article>`;
  }).join('')}</div>`;
}
function goals(C,u) {
  const m = model(C);
  const published = list(m.communityGoals?.entries);
  const joined = list(m.personalGoals?.entries);
  const goalsQuality = m.communityGoals?.quality;
  const publicHTML = published.length || m.communityGoals?.known ?
    `<div class="pol-section-heading"><h2>${label(C,u,'Public community goals','Цели галактики')}</h2>${timestamp(C,u,m.communityGoals)}</div>`+
    (published.length ? goalCards(C,u,published,false) : goalsQuality?.coverage === 'partial' ?
      empty(C,u,'No usable community goals in this response.','В ответе нет пригодных данных о целях.') :
      empty(C,u,'No active community goals in the latest update.','В последнем обновлении нет активных целей.')) :
    loading(C) ? '' : empty(C,u,'No public community goal data.','Данных о целях сообщества пока нет.');
  const joinedHTML = joined.length ? `<div class="pol-section-heading"><h2>${label(C,u,'Your community goals','Ваши цели')}</h2>${timestamp(C,u,m.personalGoals)}</div>${goalCards(C,u,joined,true)}` : '';
  return notice(C,u) + publicHTML + joinedHTML;
}
function factions(C,u) {
  const system = model(C).activeSystem || {};
  const entries = factionRows(system);
  const conflicts = conflictRows(system);
  const influences = entries.length ? panel(C,u,system.name || text(C,'Faction influence','Влияние фракций'),
    `<div class="pol-factions">${entries.map(row => factionItem(C,u,row,system.info?.controllingFaction)).join('')}</div>`,system.factions) : '';
  const conflictCards = conflicts.length ? panel(C,u,text(C,'Observed conflicts','Наблюдаемые конфликты'),
    `<div class="pol-conflicts">${conflicts.map(row => {
      const title = [row.type,row.status].filter(Boolean).map(value => esc(u,term(C,value))).join(' · ');
      return `<article class="pol-conflict">${title ? `<h3>${title}</h3>` : ''}<div class="pol-conflict-sides">
        ${[row.faction1,row.faction2].map(side => `<section><strong>${esc(u,side.name)}</strong>${facts(C,u,[
          ['Days won','Дней выиграно',num(u,side.wonDays)],
          ['Stake','На кону',esc(u,side.stake)],
        ])}</section>`).join('')}
      </div></article>`;
    }).join('')}</div>`,system.conflicts) : '';
  return searchForm(C,u) + notice(C,u) + influences + conflictCards;
}
function options(u,entries,selected) {
  return entries.map(([value,name]) => `<option value="${esc(u,value)}" ${value===selected?'selected':''}>${esc(u,name)}</option>`).join('');
}
function powerplay(C,u) {
  const m = model(C);
  const a = local(C);
  const index = m.powerIndex || {};
  const powers = list(index.powers);
  const rows = [...powers].sort((x,y) => (known(y.indexedSystems)?Number(y.indexedSystems):-1)-(known(x.indexedSystems)?Number(x.indexedSystems):-1));
  const states = list(index.states);
  const summary = powers.length ? panel(C,u,text(C,'Controlled systems','Контролируемые системы'),`<div class="pol-power-grid">
    ${rows.map(power => `<button type="button" class="pol-power" aria-pressed="${a.power===power.name}" data-click="politics-power" data-power="${esc(u,power.name)}">
      <strong>${esc(u,powerName(power.name))}</strong><span>${num(u,power.indexedSystems)}</span></button>`).join('')}
  </div>${facts(C,u,states.filter(state => ['Stronghold','Fortified','Exploited'].includes(state.name)).map(state =>
    [...terms[state.name],num(u,state.indexedSystems)]
  ))}`,index) : '';
  const pledge = m.pledge || {};
  const pledgeCard = pledge.pledged && pledge.power ? panel(C,u,text(C,'Your power','Ваша держава'),facts(C,u,[
    ['Pledge','Присяга',esc(u,powerName(pledge.power))],
    ['Merits','Заслуги',num(u,pledge.merits)],
    ['Rank','Ранг',num(u,pledge.rank)],
  ]),pledge) : '';
  const results = m.search || {};
  const matching = results.kind === 'powerplay' && (results.scope === 'controlled' || a.power || a.state) &&
    (results.power || '') === a.power && (results.state || '') === a.state && Number(results.page || 0) === a.page;
  const filters = powers.length ? `<div class="pol-filter-row">
    <label><span>${label(C,u,'Power','Держава')}</span><select id="politics-power-filter" data-politics-field="power">${options(u,[['',text(C,'All powers','Все державы')],...powers.map(power => [power.name,powerName(power.name)])],a.power)}</select></label>
    <label><span>${label(C,u,'System state','Состояние системы')}</span><select id="politics-state-filter" data-politics-field="state">${options(u,[['',text(C,'All controlled states','Все состояния контроля')],...[...new Set([...states.map(state => state.name),'Contested'])].map(value => [value,term(C,value)])],a.state)}</select></label>
  </div>` : '';
  return notice(C,u) + pledgeCard + summary + filters +
    (matching ? panel(C,u,text(C,'Systems','Системы'),powerResults(C,u,results),results) :
      loading(C) || summary ? '' : empty(C,u,'No Powerplay data available.','Данные Пауэрплея пока недоступны.'));
}
function powerResults(C,u,result) {
  const rows = list(result.entries);
  const page = Number(result.page) || 0;
  const next = known(result.total) && (page+1)*(Number(result.limit)||10) < Number(result.total) && page < 100;
  const paging = page > 0 || next ? `<div class="pol-pagination">
    ${button(C,u,'Previous','Назад','page',`data-page="${page-1}"`,page===0)}
    <span>${label(C,u,'Page','Страница')} ${num(u,page+1)}</span>
    ${button(C,u,'Next','Далее','page',`data-page="${page+1}"`,!next)}
  </div>` : '';
  if (!rows.length) return (result.quality?.coverage === 'partial' ?
    empty(C,u,'No usable system records in this response.','В ответе нет пригодных записей о системах.') :
    empty(C,u,'No systems match these filters.','Нет систем с такими параметрами.')) + paging;
  return `<div class="table-scroll"><table class="pol-table"><thead><tr>
    ${[['System','Система'],['Power','Держава'],['Status','Состояние'],['Control','Контроль'],
      ['Reinforcement','Укрепление'],['Undermining','Подрыв']].map(names => `<th>${label(C,u,...names)}</th>`).join('')}
    </tr></thead><tbody>${rows.map(row => {
      const p = row.powerplay || {};
      return `<tr><td>${systemButton(u,row.name)}${timestamp(C,u,p)}</td>
        <td>${esc(u,powerName(p.controllingPower)) || '—'}</td><td>${esc(u,term(C,p.state)) || '—'}</td>
        <td>${percent(C,u,p.controlProgress)}</td><td>${num(u,p.reinforcement)}</td><td>${num(u,p.undermining)}</td></tr>`;
    }).join('')}</tbody></table></div>${paging}`;
}

function request(C) {
  const view = currentView(C).id;
  const draft = local(C);
  if (view === 'goals') return {kind:'goals',language:language(C)};
  if (view === 'powerplay') return {kind:'powerplay',power:draft.power,state:draft.state,page:draft.page};
  const system = selectedSystem(C);
  return system ? {kind:'system',system,keepSelection:true} : null;
}
function requestKey(values) {
  if (!values) return '';
  if (values.kind === 'system') return JSON.stringify(['system',String(values.system || '').trim().toLowerCase()]);
  if (values.kind === 'goals') return JSON.stringify(['goals',values.language || 'en']);
  return JSON.stringify(['powerplay',values.power || '',values.state || '',Number(values.page) || 0]);
}
function sameStatus(status,values) {
  return Boolean(values && status?.kind === values.kind && requestKey({kind:status.kind,...status.parameters}) === requestKey(values));
}
function busy(C) {
  return Boolean(local(C).dispatching || model(C).status?.running || C.state?.politicsNetwork?.running);
}
function freshness(C,values) {
  const m = model(C);
  if (values.kind === 'goals') {
    return m.communityGoals?.known && (m.communityGoals.language || 'en') === values.language ? m.communityGoals.fetchedAt : '';
  }
  if (values.kind === 'powerplay') {
    const search = m.search;
    const applicable = search?.scope === 'controlled' || values.power || values.state;
    return search?.known && search.kind === 'powerplay' && applicable && requestKey(search) === requestKey(values) ? search.fetchedAt : '';
  }
  const system = m.activeSystem;
  if (requestKey({kind:'system',system:system?.name}) !== requestKey(values)) return '';
  const observations = [system?.info,system?.factions,system?.powerplay].filter(data => data?.known && data.source === 'journal');
  return [system.detailsFetchedAt,...observations.map(data => data.observedAt)]
    .filter(value => Number.isFinite(Date.parse(value))).sort((a,b) => Date.parse(b)-Date.parse(a))[0];
}
function shouldLoad(C,values) {
  if (!values || window.COPILOT_SMOKE || model(C).status?.offline || !C.state?.politics) return false;
  const interval = 60000*(Number(model(C).limits?.cacheMinutes) || 5);
  const stamp = Date.parse(freshness(C,values));
  if (Number.isFinite(stamp) && Date.now()-stamp < interval) return false;
  const a = local(C);
  const key = requestKey(values);
  const status = model(C).status;
  if (sameStatus(status,values) && Date.parse(status.retryAt) > Date.now()) return false;
  const retryDelay = a.errors.has(key) || sameStatus(status,values) && status.error ? 60000 : interval;
  return !a.attempted.has(key) || Date.now()-a.attempted.get(key) >= retryDelay;
}
function loading(C) {
  const values = request(C);
  const key = requestKey(values);
  const status = model(C).status;
  return Boolean(key && (local(C).pending === key || sameStatus(status,values) && status.running || shouldLoad(C,values)));
}
function notice(C,u) {
  const values = request(C);
  const status = model(C).status;
  if (loading(C)) {
    return `<p class="pol-status" role="status">${label(C,u,'Updating…','Обновляем данные…')}</p>`;
  }
  if (sameStatus(status,values) && status.reason && window.FreshnessUI) {
    return `<p class="notice pol-status" role="status">${esc(u,window.FreshnessUI.reason(status.reason,language(C)))}</p>`;
  }
  if (local(C).errors.has(requestKey(values)) || sameStatus(status,values) && status.error) {
    return `<p class="notice pol-status" role="status">${label(C,u,'Could not update these observations.','Не удалось обновить данные.')}</p>`;
  }
  return '';
}
async function autoLoad(C,u) {
  if (C.tab !== 'politics' || busy(C)) return;
  const values = request(C);
  if (!shouldLoad(C,values)) return;
  const a = local(C);
  const key = requestKey(values);
  a.dispatching = true;
  a.pending = key;
  a.attempted.delete(key);
  a.attempted.set(key,Date.now());
  if (a.attempted.size > 128) {
    const oldest = a.attempted.keys().next().value;
    a.attempted.delete(oldest);
    a.errors.delete(oldest);
  }
  try {
    await u.call('politics-refresh',{values});
    a.errors.delete(key);
  } catch {
    a.errors.add(key);
  } finally {
    a.pending = null;
    try {
      if (C.tab === 'politics') await u.loadView();
    } finally {
      a.dispatching = false;
    }
  }
  if (C.tab === 'politics' && requestKey(request(C)) !== key) await autoLoad(C,u);
}
function render(C,u) {
  const view = currentView(C);
  const title = text(C,view.en,view.mode);
  const renderView = {overview,goals,powerplay,factions}[view.id];
  return `<div class="politics-page">${u.heading(title,'')}${renderView(C,u)}</div>`;
}
function openMode(C,u,view) {
  const item = views.find(candidate => candidate.id === view);
  if (!item || view === 'factions' && !hasFactions(C)) return;
  return u.navigate('politics',{mode:item.mode});
}
async function selectSystem(C,u,system,current=false) {
  const next = String(system || '').trim();
  if (!next) return;
  const state = local(C);
  state.system = next;
  state.systemContext = next;
  if (!current && next === selectedSystem(C) && ['overview','factions'].includes(currentView(C).id)) return;
  await u.call('politics',{values:current?{action:'select-current'}:{action:'select-system',system:next}});
  if (C.tab !== 'politics') return;
  if (['powerplay','goals'].includes(currentView(C).id)) await openMode(C,u,'overview');
  else await u.loadView();
}
async function handleClick(action,element,C,u) {
  if (!action.startsWith('politics-')) return false;
  const a = local(C);
  switch (action) {
    case 'politics-mode':
      await openMode(C,u,element.dataset.view);
      return true;
    case 'politics-system':
      await selectSystem(C,u,element.dataset.system);
      return true;
    case 'politics-current':
      await selectSystem(C,u,model(C).currentSystem,true);
      return true;
    case 'politics-copy':
      if (element.dataset.system) await u.call('native',{action:'copy',text:element.dataset.system});
      return true;
    case 'politics-power':
      a.power = a.power === element.dataset.power ? '' : element.dataset.power;
      a.page = 0;
      u.renderPage();
      return true;
    case 'politics-page': {
      const page = Number(element.dataset.page);
      if (Number.isInteger(page) && page >= 0 && page <= 100) {
        a.page = page;
        u.renderPage();
      }
      return true;
    }
    default:
      return false;
  }
}
function handleInput(event,C) {
  if (C.tab !== 'politics' || event.target?.dataset?.politicsField !== 'system') return false;
  local(C).system = event.target.value;
  return true;
}
async function handleChange(event,C,u) {
  const field = event.target?.dataset?.politicsField;
  if (C.tab !== 'politics' || !field) return false;
  if (field === 'system') {
    // The shared system autocomplete dispatches an explicit change on choice.
    // A native blur alone leaves the typed draft available for form submit.
    if (event.isTrusted === false) await selectSystem(C,u,event.target.value);
    return true;
  }
  if (field !== 'power' && field !== 'state') return false;
  const a = local(C);
  if (a[field] !== event.target.value) {
    a[field] = event.target.value;
    a.page = 0;
    u.renderPage();
  }
  return true;
}
let latest, bound = false, timer = null;
function schedule() {
  if (timer !== null || !window.setTimeout || window.COPILOT_SMOKE || model(latest.C).status?.offline) return;
  timer = window.setTimeout(() => {
    timer = null;
    if (latest.C.tab !== 'politics') return;
    autoLoad(latest.C,latest.u).catch(() => {});
    schedule();
  },30000);
}
function init(C,u) {
  latest = {C,u};
  local(C).hasFactions = hasFactions(C);
  if (typeof document !== 'undefined' && !bound) {
    bound = true;
    document.addEventListener('submit',event => {
      if (event.target.id !== 'politics-system-form' || latest.C.tab !== 'politics') return;
      event.preventDefault();
      selectSystem(latest.C,latest.u,local(latest.C).system).catch(error => latest.u.toast(error.message,true));
    });
  }
  schedule();
  Promise.resolve().then(() => autoLoad(C,u)).catch(() => {});
}
async function smoke(C,u) {
  if (window.COPILOT_SMOKE !== true) throw new Error('Politics smoke requires an isolated synthetic profile');
  const before = C.mode, checks = [];
  const check = (name,ok) => checks.push({name,ok:!!ok});
  try {
    check('Politics system input uses the shared autocomplete',u.$('#politics-system')?.dataset.systemSearch==='single');
    check('Galaxy sidebar has distinct politics, goals and Powerplay entries',navigationModes(C).slice(0,3).every(item => u.$(`#navigation [data-nav="politics"][data-nav-mode="${item.mode}"]`)));
    await openMode(C,u,'goals');
    const details = u.$('.pol-goal details');
    if (details) details.open = true;
    check('Community-goal card expands objective and rewards',details?.open && details.querySelector('p')?.textContent.trim());
    check('Public and personal community goals stay separate',u.$$('.pol-section-heading').length===2);
    await openMode(C,u,'powerplay');
    check('Powerplay shows power counts from the cached index',u.$$('.pol-power').length>0);
    await openMode(C,u,'factions');
    check('Faction view shows observed influence and conflict opponents',u.$$('.pol-faction').length>0 && !!u.$('.pol-conflict-sides'));
    await openMode(C,u,'overview');
    check('Galaxy does not expose obsolete Thargoid controls',!u.$('[data-view="thargoid"]'));
    const copy = u.$('[data-click="politics-copy"]');
    check('Politics system name can be copied',!!copy && !copy.disabled);
    if (copy) await handleClick('politics-copy',copy,C,u);
 } finally {
    await u.navigate('politics',{mode:before,back:true});
 }
 return checks;
}
window.PoliticsUI = {render,init,handleClick,handleInput,handleChange,navigationModes,smoke};
})();
