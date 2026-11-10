(() => {
'use strict';

const mode = 'Прокачка';
const pageSize = 12;
const active = C => C.tab === 'engineering' && C.mode === mode;
const filters = () => ({engineerIDs: [], moduleKey: '', blueprintKey: '', grade: 0, experimentalKey: '', equipmentKey: ''});
const model = C => C.engineeringRoadmap ??= {filters: filters(), query: '', visible: pageSize, sequence: 0, open: new Set(), capabilitiesOpen: new Set()};
const labels = {
  locked: 'Закрыт', known: 'Обнаружен', invited: 'Приглашение получено', acquainted: 'Приглашение получено',
  unlocked: 'Открыт', unknown: 'Неизвестно', barred: 'Доступ ограничен',
  done: 'Выполнено', available: 'Доступно', pending: 'Требуется', blocked: 'Доступ ограничен',
  referral: 'Знакомство', discovery: 'Знакомство', meeting: 'Приглашение', invitation: 'Приглашение',
  unlock: 'Открытие', donation: 'Передача ресурсов', rank: 'Репутация', reputation: 'Репутация', permit: 'Разрешение'
};
const conditions={
  'equipment-unavailable':'Экземпляр больше не виден в журнале. Выберите другой экземпляр или новый предмет.',
  'unlock-engineer':'Сначала откройте доступ к мастерской.',
  'rank-engineer':'Поднимите репутацию инженера до целевой ступени.',
  'replaces-blueprint':'Новый чертёж заменит установленное улучшение.',
  'unmapped-blueprint':'Текущий чертёж не распознан. Расход рассчитан без зачёта этого улучшения.',
  'experimental-incompatible':'Экспериментальный эффект несовместим с выбранным экземпляром.',
  'requires-blueprint':'Сначала примените чертёж к выбранному экземпляру.',
  'weapon-incompatible':'Эта модификация несовместима с типом выбранного оружия.',
  'weapon-type-unknown':'Тип оружия не подтверждён журналом.',
  'live-only':'Доступно в режиме Live.',
  'slots-unknown':'Количество свободных слотов не подтверждено журналом.',
  'no-free-slot':'Нет свободного слота для модификации.',
  'upgrade-equipment':'Повысьте ступень снаряжения, чтобы открыть слот для модификации.'
};
const damageNames={kinetic:'Кинетический урон',laser:'Лазерный урон',plasma:'Плазменный урон'};
const array = value => Array.isArray(value) ? value : [];
const isKnown = value => value !== null && value !== undefined && value !== '' && Number.isFinite(Number(value));
const keyOf = item => String(item?.key ?? '');
const kindOf = item => ['odyssey','foot','onfoot','suit','weapon','ground'].includes(item?.kind) ? 'foot' : 'ship';
const lang = C => window.CopilotI18n?.language || C.state?.settings?.language || 'en';
const label = (row,C,u,field='name',english='english') =>
  lang(C) === 'en' && row?.[english] ? row[english] : u.t(row?.[field] || row?.[english] || '');
const number = (value,u) => isKnown(value) ? u.fmt(Number(value)) : '—';
const tr = (u,text,params) => u.esc(u.t(text,params));
const identity = C => JSON.stringify([C.state?.commander?.fid||'',C.state?.planWorkspace?.key||'',C.state?.planWorkspace?.branch||'']);
const requestKey = C => JSON.stringify([identity(C), model(C).filters]);
const current = C => {
  const r = model(C);
  return r.resultKey === requestKey(C) ? r.result : null;
};
const catalogue = C => {
  const r = model(C);
  return r.catalogueIdentity === identity(C) ? r.result?.filters || {} : {};
};

function select(u, name, caption, choices, chosen, empty, disabled = false) {
  const id = 'eng-roadmap-' + name;
  return `<label class="eng-field" for="${id}"><span>${tr(u,caption)}</span><select id="${id}" data-engineering-filter="${name}" ${disabled?'disabled':''}>
  <option value="">${tr(u,empty)}</option>${choices.map(({value,text}) => `<option value="${u.esc(value)}" ${String(chosen) === String(value)?'selected':''}>${u.esc(text)}</option>`).join('')}</select></label>`;
}

function blueprintLabel(item,C,u,field='name',english='english') {
  const damage=damageNames[item?.damageType];
  return label(item,C,u,field,english)+(damage?' · '+u.t(damage):'');
}

function equipmentLabel(item,C,u) {
  const name=label(item,C,u);
  if(item.planned)return u.t("Новый: {name}",{name});
  return [name,item.stored?u.t("На хранении"):item.slot,item.system].filter(Boolean).join(' · ');
}

function engineerPicker(C,u,engineers) {
  if (!engineers.length) return '';
  const r=model(C), selected=new Set(r.filters.engineerIDs);
  const summary = selected.size ? u.t("Выбрано инженеров: {count}",{count:u.fmt(selected.size)}) : u.t("Все инженеры");
  const groups = [['ship','Корабельные инженеры'],['foot','Инженеры Odyssey']];
  return `<details class="eng-picker" data-engineering-picker ${r.pickerOpen?'open':''}>
    <summary id="eng-roadmap-engineer-picker">${u.icon('users')}<span data-engineering-picker-summary>${u.esc(summary)}</span></summary>
    <div class="eng-picker-body">
      <label class="eng-field" for="eng-roadmap-engineer-query"><span class="eng-filter-label-hidden">${tr(u,"Поиск инженера")}</span><input id="eng-roadmap-engineer-query" type="search" data-engineering-query value="${u.esc(r.query)}" placeholder="${tr(u,"Имя или система")}" autocomplete="off"></label>
      <div class="eng-engineer-options">
        ${groups.map(([kind,title]) => {
          const rows=engineers.filter(row=>kindOf(row)===kind).sort((a,b)=>label(a,C,u).localeCompare(label(b,C,u),lang(C)));
          return rows.length ? `<fieldset><legend>${tr(u,title)}</legend>${rows.map(row=>{
            const id=keyOf(row), name=label(row,C,u);
            const search=[name,row.name,row.english,row.system,row.base].filter(Boolean).join(' ').toLocaleLowerCase();
            return `<label class="eng-engineer-option" data-engineering-option="${u.esc(search)}" ${r.query&&!search.includes(r.query.trim().toLocaleLowerCase())?'hidden':''}><input type="checkbox" id="eng-roadmap-engineer-${u.esc(id)}" data-engineering-engineer="${u.esc(id)}" ${selected.has(id)?'checked':''}><span>${u.esc(name)}</span></label>`;
          }).join('')}</fieldset>` : '';
        }).join('')}
      </div>
      <button class="pill quiet" type="button" data-engineering-action="clear-engineers" ${selected.size?'':'disabled'}>${tr(u,"Показать всех")}</button>
    </div>
  </details>`;
}

function filterMarkup(C,u) {
  const r=model(C), f=r.filters, options=catalogue(C);
  const choices = (rows,title=item=>label(item,C,u)) => array(rows).map(item => ({value:keyOf(item), text:title(item)})).sort((a,b)=>a.text.localeCompare(b.text,lang(C)));
  const blueprintOptions = array(options.blueprints).filter(item=>!f.moduleKey||array(item.moduleKeys).includes(f.moduleKey));
  const blueprint = blueprintOptions.find(item=>keyOf(item)===f.blueprintKey);
  const maxGrade = !f.moduleKey||blueprint?.kind==='odyssey'?0:Math.min(5, Math.max(0, Number(blueprint?.maxGrade)||0));
  const grades = Array.from({length:maxGrade}, (_,i) => ({value:String(i+1),text:'G'+(i+1)}));
  const equipment = array(options.equipment);
  const experiments = array(options.experimentals);
  return `<div class="eng-filters-primary">
    ${engineerPicker(C,u,array(options.engineers))}
    ${select(u,'moduleKey','Модуль или снаряжение',choices(options.modules),f.moduleKey,'Все модули')}
    ${select(u,'blueprintKey','Чертёж',choices(blueprintOptions,item=>blueprintLabel(item,C,u)),f.blueprintKey,'Все чертежи',!blueprintOptions.length)}
    ${grades.length?select(u,'grade','Целевая ступень',grades,f.grade,'Максимальная ступень'):''}
  </div>${f.moduleKey&&(equipment.length||experiments.length) ? `<div class="eng-filters-secondary">
    ${equipment.length?select(u,'equipmentKey','Экземпляр снаряжения',choices(equipment,item=>equipmentLabel(item,C,u)),f.equipmentKey,'Новый экземпляр'):''}
    ${experiments.length?select(u,'experimentalKey','Экспериментальный эффект',choices(experiments),f.experimentalKey,'Без эффекта'):''}
  </div>` : ''}`;
}

function statusTag(status,u) {
  const caption=labels[status];
  if (!caption) return '';
  const style = status==='unlocked'||status==='done'?'good':status==='unknown'?'muted':status==='invited'||status==='available'?'current':'neutral';
  return `<span class="eng-status ${style}">${tr(u,caption)}</span>`;
}

function materials(C,u,rows,{stock=true,combined=false}={}) {
  if (!array(rows).length) return '';
  const showDonated=rows.some(item=>isKnown(item.donated));
  return `<div class="eng-materials"><table><thead><tr>
  <th scope="col">${tr(u,"Материал")}</th>${combined?'':`<th scope="col">${tr(u,"Требуется")}</th>`}
  ${showDonated?`<th scope="col">${tr(u,"Передано")}</th>`:''}
  ${stock?`<th scope="col">${tr(u,"В наличии")}</th>${combined?'':`<th scope="col">${tr(u,"Не хватает")}</th>`}`:''}</tr></thead>
  <tbody>${rows.map(row=>`<tr><th scope="row">${referenceLink(row.entryId,label(row,C,u),u)}</th>${combined?'':`<td>${number(row.count,u)}</td>`}
    ${showDonated?`<td>${number(row.donated,u)}</td>`:''}${stock?`<td>${number(row.owned,u)}</td>${combined?'':`<td class="${Number(row.missing)>0?'eng-shortage':''}">${number(row.missing,u)}</td>`}`:''}</tr>`).join('')}</tbody></table></div>`;
}

function location(row,u) {
  if (!row?.system && !row?.base) return '';
  return `<div class="eng-location">${u.icon('map-pin')}<span>${u.esc([row.system,row.base].filter(Boolean).join(' · '))}</span>
  ${row.system ? `<button type="button" class="icon-button" data-click="table-copy-system" data-system="${u.esc(row.system)}" aria-label="${tr(u,"Копировать систему ")}${u.esc(row.system)}" title="${tr(u,"Копировать систему")}">${u.icon('copy')}</button>` : ''}</div>`;
}

function entityLink(kind,key,name,u) {
  return referenceLink(key ? `${kind}/${key}` : '',name,u);
}

function referenceLink(id,name,u) {
  return id ? `<button type="button" class="eng-entity-link" data-reference-id="${u.esc(id)}">${u.esc(name)}${u.icon('arrow-up-right')}</button>` : u.esc(name);
}

function stepText(step,C,u) {
  return label(step,C,u,'text') || u.t(labels[step.kind]||"Следующий шаг");
}

function blueprintButton(C,u,item,engineer) {
  const partial=Number(item.accessibleGrade)>0&&Number(item.accessibleGrade)<Number(item.maxGrade);
  const maximum=Number(item.maxGrade)>0&&!['suit','weapon'].includes(item.moduleKey)?`<small ${partial?'title="'+tr(u,"Доступная и максимальная ступени")+'"':''}>${partial?'G'+number(item.accessibleGrade,u)+' / ':''}G${number(item.maxGrade,u)}</small>`:'';
  return `<button type="button" class="eng-variant" data-engineering-action="select-blueprint" data-engineer="${u.esc(engineer)}" data-module="${u.esc(item.moduleKey)}" data-blueprint="${u.esc(item.blueprintKey)}"><span>${u.esc(blueprintLabel(item,C,u,'blueprintName','blueprintEnglish')||item.blueprintKey)}</span>${maximum}${u.icon('chevron-right')}</button>`;
}

function stepDetails(C,u,step,row) {
  const kind=labels[step.kind], heading=kind?u.t(kind):'', content=stepText(step,C,u);
  const counted=isKnown(step.target)&&isKnown(step.current);
  const prerequisite=label(step,C,u,'engineerName','engineerEnglish');
  const practice=step.practice;
  const reputation=step.kind==='rank'&&step.status!=='done'?array(row.reputation).slice(1):[];
  return `<li><div class="eng-step-body"><div class="eng-step-head">
    ${heading&&heading!==content?`<strong>${u.esc(heading)}</strong>`:''}
    ${step.status&&step.status!=='pending'?statusTag(step.status,u):''}
    ${counted?`<span class="eng-step-counter">${number(step.current,u)} / ${number(step.target,u)}</span>`:''}
  </div><p>${u.esc(content)}</p>
  ${prerequisite?`<div class="eng-step-location"><strong>${entityLink('engineer',step.engineerKey,prerequisite,u)}</strong>${location({system:step.engineerSystem,base:step.engineerBase},u)}</div>`:''}
  ${step.combined&&isKnown(step.target)?`<p class="eng-observed">${tr(u,"Всего передать: {count}",{count:number(step.target,u)})}${isKnown(step.missing)?' · '+tr(u,"Ещё собрать: {count}",{count:number(step.missing,u)}):''}</p>`:''}
  ${materials(C,u,step.items,{combined:step.combined===true})}
  ${reputation.length?`<ul class="eng-rank-methods">${reputation.map(item=>`<li>${u.esc(label(item,C,u,'text'))}</li>`).join('')}</ul>`:''}
  ${practice?.moduleKey&&practice.blueprintKey?`<div class="eng-practice"><small>${u.esc(label(practice,C,u,'moduleName','moduleEnglish'))}</small>${blueprintButton(C,u,practice,row.key)}</div>`:''}
  </div></li>`;
}

function capabilityMarkup(C,u,row) {
  const modules=new Map();
  for (const capability of array(row.capabilities)) {
    if (!capability.moduleKey || !capability.blueprintKey) continue;
    let module=modules.get(capability.moduleKey);
    if (!module) {
      module={name:label(capability,C,u,'moduleName','moduleEnglish'), variants:new Map()};
      modules.set(capability.moduleKey,module);
    }
    const previous=module.variants.get(capability.blueprintKey);
    if (!previous || Number(capability.maxGrade)>Number(previous.maxGrade)) module.variants.set(capability.blueprintKey,capability);
  }
  return Array.from(modules,([id,module])=>`<section><h4>${u.esc(module.name||id)}</h4><ul>
  ${Array.from(module.variants.values(),item=>`<li>${blueprintButton(C,u,item,row.key)}</li>`).join('')}</ul></section>`).join('');
}

function capabilityDetails(C,u,row) {
  const count=new Set(array(row.capabilities).map(item=>item.moduleKey).filter(Boolean)).size;
  if (!count) return '';
  const open=model(C).capabilitiesOpen.has(keyOf(row));
  return `<details class="eng-capabilities" data-engineering-disclosure="capabilities" ${open?'open':''}>
  <summary>${tr(u,"Модули и чертежи")} <span>${u.fmt(count)}</span></summary>
  <div class="eng-capability-grid" data-engineering-variants>${open?capabilityMarkup(C,u,row):''}</div></details>`;
}

function engineerCard(C,u,row,index) {
  const r=model(C), id=keyOf(row), steps=array(row.steps), first=steps.find(step=>step.status!=='done');
  const open=r.open.has(id)||(!r.closed?.has(id)&&r.filters.engineerIDs.length===1&&r.filters.engineerIDs[0]===id);
  const rank=kindOf(row)==='ship'&&Number(row.rank)>0?'G'+number(row.rank,u)+(Number(row.rank)<5&&isKnown(row.progress)?' · '+number(row.progress,u)+' %':''):'';
  return `<article class="eng-card" data-engineering-card="${u.esc(id)}">
  <div class="eng-card-top"><span class="eng-order" aria-hidden="true">${u.fmt(index+1)}</span>
  <div><h3>${entityLink('engineer',id,label(row,C,u),u)}</h3>${location(row,u)}</div>
  <div class="eng-card-badges">${row.dependency?`<span class="eng-status neutral">${tr(u,"Предшественник")}</span>`:''}${rank?`<span class="eng-status neutral">${u.esc(rank)}</span>`:''}${statusTag(row.status,u)}</div></div>
  ${first?`<p class="eng-next" data-engineering-preview ${open?'hidden':''}>${u.esc(stepText(first,C,u))}</p>`:''}
  ${steps.length ? `<details class="eng-engineer-steps" data-engineering-disclosure="steps" ${open?'open':''}>
    <summary>${tr(u,"Шаги прокачки")}</summary><ol class="eng-steps">${steps.map(step=>stepDetails(C,u,step,row)).join('')}</ol>
  </details>` : ''}
  ${capabilityDetails(C,u,row)}
  </article>`;
}

function confidenceText(confidence,u) {
  switch(confidence){
    case 'exact': return '';
    case 'estimate': return tr(u,"Число применений оценочное и зависит от репутации инженера.");
    default: return tr(u,"Точное число применений пока неизвестно. Показан расход по известным рецептам.");
  }
}

function gradeCard(C,u,row,{title='',oneApplication=false}={}) {
  const inputs=array(row.materials);
  const perRoll=oneApplication||(!inputs.length&&row.status!=='done'&&row.perRoll?.length);
  const effects=array(row.effects);
  return `<section><header><strong>${title||'G'+number(row.grade,u)}</strong>
    ${perRoll?`<small>${tr(u,"За одно применение")}</small>`:isKnown(row.rolls)&&Number(row.rolls)>0?`<small>${tr(u,"Применений: {count}",{count:number(row.rolls,u)})}</small>`:''}
    ${row.status==='done'?statusTag(row.status,u):''}
    ${Number(row.credits)>0?`<small>${number(row.credits,u)} ${tr(u,"кр.")}</small>`:''}
    </header>${effects.length?`<dl class="eng-grade-effects">${effects.map(item=>`<div><dt>${u.esc(item.label)}</dt><dd>${u.esc(item.value)}</dd></div>`).join('')}</dl>`:''}
    ${materials(C,u,perRoll?row.perRoll:inputs,{stock:false})}</section>`;
}

function recipe(C,u,row,options) {
  if (!row) return '';
  const option=(rows,key)=>array(rows).find(item=>keyOf(item)===key);
  const ship=kindOf(row)==='ship', description=label(row,C,u,'description','descriptionEnglish');
  const names = [
    entityLink('module',ship?row.moduleKey:'',label(option(options.modules,row.moduleKey),C,u)||row.moduleKey,u),
    entityLink('blueprint',row.blueprintKey,blueprintLabel(option(options.blueprints,row.blueprintKey),C,u)||row.blueprintKey,u),
    ship&&Number(row.grade)>0?'G'+number(row.grade,u):''
  ].filter(Boolean);
  const effect=label(option(options.experimentals,row.experimentalKey),C,u);
  const effectDescription=label(row,C,u,'experimentalDescription','experimentalDescriptionEnglish');
  const installedBlueprint=blueprintLabel(option(options.blueprints,row.current?.blueprintKey),C,u);
  const installedBy=label(row.current,C,u,'engineerName','engineerEnglish');
  const gradeRows=array(row.grades), stock=array(row.totals), engineer=row.engineer;
  const currency = row.credits;
  const notes=array(row.conditions).filter(id=>conditions[id]);
  const equipmentGrades=array(row.equipmentGrades);
  return `<section class="eng-recipe" aria-label="${tr(u,"Расчёт улучшения")}">
    <div class="eng-recipe-heading"><div><span class="eyebrow">${tr(u,"Расчёт улучшения")}</span><h2>${names.join(' · ')}</h2>${effect?`<p>${entityLink('experimental',row.experimentalKey,effect,u)} ${row.experimentalStatus==='done'?statusTag('done',u):''}</p>`:''}</div>
    ${engineer?`<div><strong>${entityLink('engineer',engineer.key,label(engineer,C,u),u)}</strong>${location(engineer,u)}</div>`:''}</div>
    ${description?`<p class="eng-observed">${u.esc(description)}</p>`:''}
    ${effectDescription?`<p class="eng-observed">${u.esc(effectDescription)}</p>`:''}
    ${row.current&&Number(row.current.grade)>0&&!row.current.planned?`<p class="eng-observed">${tr(u,"Установленная ступень")} <strong>G${number(row.current.grade,u)}</strong>${isKnown(row.current.quality)?` · ${number(Math.round(Number(row.current.quality)<=1?Number(row.current.quality)*100:Number(row.current.quality)),u)} %`:''}${installedBlueprint?' · '+entityLink('blueprint',row.current.blueprintKey,installedBlueprint,u):''}${installedBy?' · '+entityLink('engineer',row.current.engineerKey,installedBy,u):''}</p>`:''}
    ${row.confidence&&row.confidence!=='exact'?`<p class="eng-note">${confidenceText(row.confidence,u)}</p>`:''}
    ${notes.length?`<ul class="eng-recipe-notes">${notes.map(id=>`<li>${tr(u,conditions[id])}</li>`).join('')}</ul>`:''}
    ${stock.length?materials(C,u,stock):''}
    ${row.currencies?.length?`<div class="eng-currencies"><span>${tr(u,"Валюта")}</span> ${row.currencies.map(item=>u.esc(label(item,C,u))+' × '+number(item.count,u)).join(' · ')}</div>`:''}
    ${isKnown(currency)&&Number(currency)>0?`<p class="eng-observed">${tr(u,"Кредиты")} <strong>${number(currency,u)}</strong></p>`:''}
    ${gradeRows.length||row.experimentalMaterials?.length?`<details class="eng-grade-breakdown" data-engineering-disclosure="recipe" ${(model(C).recipeOpen??(!stock.length&&row.confidence==='unknown'))?'open':''}>
      <summary>${tr(u,"Расход по ступеням")}</summary>
      <div class="eng-grade-grid">${gradeRows.map(grade=>gradeCard(C,u,grade,{title:!ship?tr(u,"Модификация"):''})).join('')}
      ${row.equipmentUpgrade?gradeCard(C,u,row.equipmentUpgrade,{title:tr(u,"Ступень снаряжения G{grade}",{grade:number(row.equipmentUpgrade.grade,u)})}):''}
      ${row.experimentalMaterials?.length?`<section><header><strong>${u.esc(effect||u.t("Экспериментальный эффект"))}</strong></header>${materials(C,u,row.experimentalMaterials,{stock:false})}</section>`:''}</div>
    </details>`:''}
    ${equipmentGrades.length>1||equipmentGrades.length&&!row.equipmentUpgrade?`<details class="eng-grade-breakdown">
      <summary>${tr(u,"Повышение ступени снаряжения")}</summary><div class="eng-grade-grid">${equipmentGrades.map(item=>gradeCard(C,u,item)).join('')}</div>
    </details>`:''}
  </section>`;
}

function specialRecipes(C,u,rows) {
  if(!array(rows).length)return '';
  return `<details class="eng-recipe eng-special" data-engineering-disclosure="special" ${model(C).specialOpen?'open':''}>
    <summary>${tr(u,"Особые модули")} <span>${u.fmt(rows.length)}</span></summary>
    ${rows.map(row=>`<article class="eng-special-item"><h3>${u.esc(label(row,C,u))}</h3>
      ${row.acquisition==='mercenary'?`<p class="eng-observed">${tr(u,"Покупка за монеты из «Операций»")}</p>`:''}
      ${row.liveOnly&&C.state?.planWorkspace?.branch==='legacy'?`<p class="eng-note">${tr(u,"Доступно в режиме Live.")}</p>`:''}
      ${row.variants?.length?`<ul>${row.variants.map(item=>`<li><strong>${u.esc(label(item,C,u)||item.itemSymbol)}</strong> ${Number(item.initialGrade)>0?' · G'+number(item.initialGrade,u):''}
      ${Number(item.mercCoins)>0?' · '+tr(u,"Монет наёмника: {count}",{count:number(item.mercCoins,u)}):''}</li>`).join('')}</ul>`:''}
      <div class="eng-grade-grid">${array(row.grades).map(item=>gradeCard(C,u,item,{oneApplication:true})).join('')}</div></article>`).join('')}
  </details>`;
}

function results(C,u) {
  const r=model(C), data=current(C);
  if (!data) {
    if (r.error) return `<div class="eng-message" role="alert"><strong>${tr(u,"Не удалось построить план")}</strong><p>${u.esc(r.error)}</p><button type="button" class="pill" data-engineering-action="retry">${tr(u,"Повторить")}</button></div>`;
    return `<div class="page-loading" role="status">${u.icon('refresh-cw')}<span>${tr(u,"Составляем план инженеров…")}</span></div>`;
  }
  const summary=data.summary||{}, engineers=array(data.engineers), total=engineers.length;
  const metrics=[["Инженеров в плане",summary.total??total],["Открыто",summary.unlocked],["С полной репутацией",summary.maxRank]].filter(([,value])=>isKnown(value));
  return `${metrics.length?`<dl class="eng-metrics">${metrics.map(([name,value])=>`<div><dt>${tr(u,name)}</dt><dd>${number(value,u)}</dd></div>`).join('')}</dl>`:''}
    ${recipe(C,u,data.recipe,data.filters||{})}
    ${specialRecipes(C,u,data.specialRecipes)}
    <section aria-label="${tr(u,"Порядок открытия инженеров")}" class="eng-order-list">
      <h2>${tr(u,"Порядок открытия инженеров")}</h2>
      ${total?engineers.slice(0,r.visible).map((row,index)=>engineerCard(C,u,row,index)).join(''):`<p class="eng-message">${tr(u,"Нет инженеров для выбранных условий.")}</p>`}
      ${total>r.visible?`<button type="button" class="pill" data-engineering-action="more">${tr(u,"Показать ещё ({count})",{count:u.fmt(total-r.visible)})}</button>`:''}
    </section>`;
}

function render(C,u) {
  const r=model(C);
  r.renderedFilters=filterMarkup(C,u);
  r.renderedResults=results(C,u);
  return u.heading(u.t("Прокачка инженеров"),u.t("Порядок открытия инженеров и материалы для выбранных улучшений."))+
    `<div data-engineering-roadmap class="engineering-roadmap">
      <section class="eng-filters" data-engineering-filters aria-label="${tr(u,"Фильтры инженерии")}">${r.renderedFilters}</section>
      <div class="eng-results" data-engineering-results>${r.renderedResults}</div>
    </div>`;
}

function replaceBlock(node, html) {
  if (!node || node.engineeringMarkup===html) return;
  const field=document.activeElement, focus=node.contains(field)?field.id:'';
  const selection=focus?[field.selectionStart,field.selectionEnd,field.selectionDirection]:null;
  node.innerHTML=html;
  node.engineeringMarkup=html;
  const next=focus&&document.getElementById(focus);
  if (next&&!next.disabled) {
    next.focus({preventScroll:true});
    if(selection[0]!==null&&selection[0]!==undefined&&next.setSelectionRange)next.setSelectionRange(...selection);
  }
}

function paint(C,u,options=false) {
  if (!active(C)) return;
  if (options) replaceBlock(u.$('[data-engineering-filters]'),filterMarkup(C,u));
  replaceBlock(u.$('[data-engineering-results]'),results(C,u));
}

async function fetchPlan(C,u,force=false) {
  if (!active(C)) return;
  const r=model(C), selectionKey=requestKey(C), language=lang(C), key=JSON.stringify([selectionKey,language]);
  const engineeringRevision=C.state?.engineeringRoadmapRevision;
  const revision=engineeringRevision??C.state?.revision, now=Date.now();
  if (r.inFlight===key || (!force&&r.lastKey===key&&(r.lastRevision===revision||
      (engineeringRevision==null&&now-r.lastRequestedAt<10000)))) return;
  const token=++r.sequence;
  r.inFlight=key;r.lastKey=key;r.lastRevision=revision;r.lastRequestedAt=now;r.error='';
  if (!current(C)) paint(C,u);
  try {
    const params={...r.filters,engineerIDs:[...r.filters.engineerIDs]};
    if(typeof C.state?.planWorkspace?.key==='string')params.expectedPlanWorkspace=C.state.planWorkspace.key;
    const response=await u.call('engineering-roadmap',params);
    if (!active(C)||r.sequence!==token||requestKey(C)!==selectionKey||lang(C)!==language) return;
    r.result=response;r.resultKey=selectionKey;r.catalogueIdentity=identity(C);
    paint(C,u,true);
  } catch(error) {
    if (!active(C)||r.sequence!==token||requestKey(C)!==selectionKey||lang(C)!==language) return;
    r.error=error?.message||String(error);
    paint(C,u);
  } finally {
    if (r.sequence===token) r.inFlight='';
  }
}

function applyFilters(C,u,next) {
  const r=model(C);
  r.filters={...r.filters,...next};
  r.visible=pageSize;r.error='';r.lastKey='';
  paint(C,u,true);
  clearTimeout(r.timer);
  r.timer=setTimeout(()=>{
    r.timer=null;
    fetchPlan(C,u);
  },160);
}

function change(event,C,u) {
  const input=event.target,d=input.dataset||{}, r=model(C);
  if (d.engineeringEngineer) {
    const selection=new Set(r.filters.engineerIDs);
    if(input.checked)selection.add(d.engineeringEngineer);else selection.delete(d.engineeringEngineer);
    applyFilters(C,u,{engineerIDs:[...selection].sort()});
    return;
  }
  switch (d.engineeringFilter) {
    case 'moduleKey': {
      const blueprint=array(catalogue(C).blueprints).find(item=>keyOf(item)===r.filters.blueprintKey);
      const compatible=!input.value||array(blueprint?.moduleKeys).includes(input.value);
      applyFilters(C,u,{moduleKey:input.value,blueprintKey:compatible?r.filters.blueprintKey:'',experimentalKey:'',equipmentKey:'',grade:0});break;
    }
    case 'blueprintKey': applyFilters(C,u,{blueprintKey:input.value,grade:0});break;
    case 'grade': applyFilters(C,u,{grade:Number(input.value)||0});break;
    case 'experimentalKey': applyFilters(C,u,{experimentalKey:input.value});break;
    case 'equipmentKey': applyFilters(C,u,{equipmentKey:input.value});break;
  }
}

function activate(event,C,u) {
  const button=event.target.closest?.('[data-engineering-action]');
  if (!button || button.disabled) return;
  const r=model(C),data=button.dataset;
  switch(data.engineeringAction) {
    case 'retry': fetchPlan(C,u,true);break;
    case 'clear-engineers': applyFilters(C,u,{engineerIDs:[]});break;
    case 'more': r.visible+=pageSize;paint(C,u);break;
    case 'select-blueprint':
      applyFilters(C,u,{engineerIDs:data.engineer?[data.engineer]:[],moduleKey:data.module,blueprintKey:data.blueprint,grade:0,experimentalKey:'',equipmentKey:''});
      document.getElementById('eng-roadmap-moduleKey')?.focus();
      break;
  }
}

function init(C,u) {
  if (!active(C)) return;
  const host=u.$('[data-engineering-roadmap]'), r=model(C);
  if (!host) return;
  if (!host.engineeringBound) {
    host.engineeringBound=true;
    const filterHost=u.$('[data-engineering-filters]'),resultsHost=u.$('[data-engineering-results]');
    if(filterHost)filterHost.engineeringMarkup=r.renderedFilters;
    if(resultsHost)resultsHost.engineeringMarkup=r.renderedResults;
    host.addEventListener('change',event=>change(event,C,u));
    host.addEventListener('click',event=>{
      const picker=host.querySelector?.('[data-engineering-picker]');
      if(picker?.open&&!picker.contains(event.target))picker.open=false;
      activate(event,C,u);
    });
    host.addEventListener('keydown',event=>{
      if(event.key!=='Escape')return;
      const picker=event.target.closest?.('[data-engineering-picker][open]');
      if(!picker)return;
      event.preventDefault();
      picker.open=false;
      picker.querySelector('summary')?.focus();
    });
    host.addEventListener('input',event=>{
      if(event.target.dataset?.engineeringQuery===undefined)return;
      r.query=event.target.value;
      const query=r.query.trim().toLocaleLowerCase();
      for(const option of host.querySelectorAll('[data-engineering-option]')) option.hidden=!option.dataset.engineeringOption.includes(query);
    });
    host.addEventListener('toggle',event=>{
      const node=event.target;
      if(node.isConnected===false)return;
      if(node.hasAttribute?.('data-engineering-picker'))r.pickerOpen=node.open;
      const section=node.dataset?.engineeringDisclosure;
      if(section==='recipe'){r.recipeOpen=node.open;return;}
      if(section==='special'){r.specialOpen=node.open;return;}
      if(!section)return;
      const id=node.closest('[data-engineering-card]')?.dataset.engineeringCard;
      if(!id)return;
      if(section==='capabilities'){
        if(node.open)r.capabilitiesOpen.add(id);else r.capabilitiesOpen.delete(id);
        const row=array(current(C)?.engineers).find(item=>keyOf(item)===id), variants=node.querySelector('[data-engineering-variants]');
        if(row&&variants)replaceBlock(variants,node.open?capabilityMarkup(C,u,row):'');
        return;
      }
      const preview=node.closest('[data-engineering-card]')?.querySelector('[data-engineering-preview]');
      if(preview)preview.hidden=node.open;
      r.closed??=new Set();
      (node.open?r.open:r.closed).add(id);(node.open?r.closed:r.open).delete(id);
    },true);
  }
  if (!r.timer) fetchPlan(C,u);
}

function leave(C) {
  const r=C.engineeringRoadmap;
  if (!r) return;
  ++r.sequence;r.inFlight='';r.lastKey='';r.error='';
  clearTimeout(r.timer);r.timer=null;
}

function preselect(C,engineer) {
  const r=model(C);
  r.filters={...filters(),engineerIDs:engineer?[String(engineer)]:[]};
  r.visible=pageSize;r.open.clear();r.closed?.clear();r.capabilitiesOpen.clear();
}

async function smoke(C,u) {
  if (window.COPILOT_SMOKE!==true) return [];
  const checks=[], add=(name,ok)=>checks.push({name,ok:!!ok});
  const original=C.engineeringRoadmap;
  const wait=async predicate=>{
    const until=Date.now()+7000;
    while(Date.now()<until){
      if(predicate())return true;
      if(model(C).error)throw Error(model(C).error);
      await new Promise(resolve=>setTimeout(resolve,35));
    }
    return false;
  };
  const choose=(name,value)=>{
    const field=document.getElementById('eng-roadmap-'+name);
    if (!field||![...field.options].some(option=>option.value===value))return false;
    field.value=value;
    field.dispatchEvent(new Event('change',{bubbles:true}));
    return true;
  };
  try {
    delete C.engineeringRoadmap;
    await u.navigate('engineering',{mode});
    const loaded=await wait(()=>!!current(C));
    const initial=current(C);
    add('Engineering progression loads the offline roster',loaded&&initial?.filters?.engineers?.length>20&&!!u.$('.eng-card h3'));
    const engineer=array(initial?.engineers).find(item=>kindOf(item)==='ship'&&array(item.capabilities).some(cap=>Number(cap.maxGrade)>1)&&item.system&&item.base);
    if(!engineer)throw Error('The engineering roster did not contain a selectable ship engineer');
    const id=keyOf(engineer), checkbox=document.getElementById('eng-roadmap-engineer-'+id);
    checkbox?.click();
    const filtered=await wait(()=>array(current(C)?.engineers).some(item=>keyOf(item)===id));
    const engineerCard=u.$$('[data-engineering-card]').find(card=>card.dataset.engineeringCard===id);
    add('Engineering progression filters a commander plan and renders workshop location',filtered&&model(C).filters.engineerIDs.join()===id&&engineerCard?.textContent.includes(engineer.system)&&engineerCard?.textContent.includes(engineer.base));
    const capability=engineer.capabilities.find(item=>item.moduleKey&&item.blueprintKey&&Number(item.maxGrade)>1);
    const blueprintField=document.getElementById('eng-roadmap-blueprintKey');
    const blueprintChanged=blueprintField&&!blueprintField.disabled&&choose('blueprintKey',capability.blueprintKey);
    const blueprintLoaded=blueprintChanged&&await wait(()=>!!current(C));
    add('Engineering progression accepts a blueprint before selecting a module',blueprintLoaded&&model(C).filters.moduleKey===''&&array(current(C)?.engineers).some(row=>array(row.capabilities).some(item=>item.blueprintKey===capability.blueprintKey)));
    const moduleChanged=blueprintLoaded&&choose('moduleKey',capability.moduleKey);
    const recipeLoaded=moduleChanged&&await wait(()=>current(C)?.recipe?.blueprintKey===capability.blueprintKey);
    const shown=u.$('.eng-recipe');
    add('Engineering progression calculates real materials for a selected blueprint',recipeLoaded&&!!shown?.querySelector('.eng-materials tbody tr'));
  } catch(error) {
    add('Engineering progression smoke: '+error.message,false);
  } finally {
    leave(C);
    if(original)C.engineeringRoadmap=original;else delete C.engineeringRoadmap;
  }
  return checks;
}

window.EngineeringRoadmapUI={active,render,init,leave,preselect,fetchPlan,smoke};
})();
