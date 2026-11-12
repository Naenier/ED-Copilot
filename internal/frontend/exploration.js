(() => {
'use strict';

const modes = {'Экспедиции':'expeditions','Находки':'findings','Обследования':'surveys'};
const active = C => C.tab === 'exploration';
const rows = value => Array.isArray(value) ? value : [];
const owner = C => C.state?.planWorkspace?.key || '';
const mode = C => modes[C.mode] || 'expeditions';
const q = (u,key,vars) => u.esc(u.t(key,vars));
const number = (u,n) => n == null ? '—' : u.esc(u.fmt(n));
const date = (u,stamp) => stamp ? u.esc(u.date?.(stamp,{dateStyle:'short',timeStyle:'short'}) || stamp) : '—';
const credits = (u,n) => number(u,n) + (n == null ? '' : q(u,' кр.'));
const locale = (u,ru,en) => /^en/i.test(u.locale?.() || '') ? en || ru : ru || en;
const known = (u,yes) => yes ? q(u,'Да') : '—';
const worldLabels = {earthlike:'Землеподобный мир',water:'Водный мир',ammonia:'Аммиачный мир',terraformable:'Терраформируемый мир',all:'Все планеты'};
const planetLabels = {'icy body':'Ледяное тело','rocky body':'Каменистое тело','rocky ice body':'Каменисто-ледяное тело',
 'metal rich body':'Тело, богатое металлами','high metal content body':'Тело с высоким содержанием металлов',
 'water giant':'Водный гигант','water giant with life':'Водный гигант с жизнью','gas giant with water based life':'Газовый гигант с жизнью на водной основе',
 'gas giant with ammonia based life':'Газовый гигант с жизнью на аммиачной основе','helium rich gas giant':'Газовый гигант, богатый гелием'};
const recordLabels = {hot:'Самая высокая температура',cold:'Самая низкая температура',gravity:'Самая высокая гравитация','low-gravity':'Самая низкая гравитация',radius:'Самый большой радиус','small-radius':'Самый малый радиус',mass:'Наибольшая масса',eccentric:'Наибольший эксцентриситет'};
const criteria = {visit:'Посещение',fss:'Полное сканирование',mapping:'Картографирование'};

function model(C) {
 const key = owner(C);
 if (!C.explorationUI || C.explorationUI.owner !== key) C.explorationUI = {owner:key,sequence:0,queries:{},data:{},errors:{}};
 const r = C.explorationUI;
 const query=r.queries[mode(C)] ||= {mode:mode(C),page:0,worldPage:0,bodyPage:0};
 const prefill=C.prefill;
 if(prefill && r.appliedPrefill!==prefill && (prefill.systemAddress||prefill.systemName)){
  r.appliedPrefill=prefill;
  query.systemAddress=String(prefill.systemAddress||'');
  query.systemName=String(prefill.systemName||'');
  query.bodyId=prefill.bodyId==null?'':String(prefill.bodyId);
  query.bodyPage=0;
  r.signature='';
  r.sequence++;
  r.loading=false;
 }
 return [r,query];
}
function button(u,action,label,data='',style='quiet') {
 return '<button type="button" class="pill '+style+'" data-explore="'+action+'" '+data+'>'+q(u,label)+'</button>';
}
function attr(u,key,value) {
 return 'data-'+key+'="'+u.esc(value??'')+'"';
}
function choice(u,key,items,value,label) {
 if (!items.length) return '';
 return '<label class="explore-choice"><span>'+q(u,label)+'</span><select data-explore-query="'+key+'">'+items.map(([id,name])=>
  '<option value="'+u.esc(id)+'" '+(String(id)===String(value??'')?'selected':'')+'>'+u.esc(name)+'</option>').join('')+'</select></label>';
}
function section(u,title,body,aside='') {
 return '<section class="panel explore-panel"><header class="explore-header"><h2>'+q(u,title)+'</h2>'+aside+'</header>'+body+'</section>';
}
function table(u,labels,body) {
 return '<div class="explore-table"><table><thead><tr>'+labels.map(label=>'<th scope="col">'+q(u,label)+'</th>').join('')+
  '</tr></thead><tbody>'+body.map(row=>'<tr>'+row.map(cell=>'<td>'+cell+'</td>').join('')+'</tr>').join('')+'</tbody></table></div>';
}
function pages(u,key,data) {
 if (!data || data.total<=data.pageSize) return '';
 const current = Number(data.page)||0, count = Math.ceil(data.total/data.pageSize);
 return '<div class="explore-pages">'+
  button(u,'page','Назад',attr(u,'field',key)+' '+attr(u,'value',current-1)+(current===0?' disabled':''))+
  '<span>'+number(u,current+1)+' / '+number(u,count)+'</span>'+
  button(u,'page','Далее',attr(u,'field',key)+' '+attr(u,'value',current+1)+(current>=count-1?' disabled':''))+'</div>';
}
function empty(u,label) { return '<p class="explore-empty">'+q(u,label)+'</p>'; }
function systemLink(u,name,id,body) {
 return id ? '<button type="button" class="table-entity-link" data-explore="open-system" '+attr(u,'address',id)+' '+attr(u,'body',body)+'>'+u.esc(name)+'</button>' : u.esc(name || '—');
}
function copy(u,name) { return name ? button(u,'copy','Копировать систему',attr(u,'system',name)) : ''; }
function note(u,name,id,body) { return button(u,'note','Заметка',attr(u,'system',name)+' '+attr(u,'address',id)+' '+attr(u,'body',body)); }
function worldType(u,body) {
 const kind=body.kind||'', planet=body.planetClass||'';
 const label=worldLabels[kind]||planetLabels[planet.toLowerCase()];
 if(label)return q(u,label);
 const giant=planet.match(/^(?:Sudarsky )?class ([IV]+) gas giant$/i);
 if(giant)return q(u,'Газовый гигант класса {class}',{class:giant[1].toUpperCase()});
 return u.esc(planet||body.starClass||'—');
}

function bodyTable(u,entries) {
 return table(u,['Тело','Тип','Посещено','Скан','DSS','Данные','Оценка DSS'],entries.map(b=>{
  const status = {onboard:'На борту',sold:'Продано',lost:'Потеряно'}[b.dataState];
  return ['<div class="explore-depth-'+Math.max(0,Math.min(5,Number(b.depth)||0))+'">'+systemLink(u,b.name,b.systemAddress,b.id)+(b.parent?'<small>'+u.esc(b.parent)+'</small>':'')+'</div>',
   worldType(u,b),known(u,b.visitedAt),known(u,b.scannedAt),known(u,b.mappedAt),
   status?q(u,status):'—',credits(u,b.mappingEstimate)];
 }));
}
function detail(u,data) {
 const sy=data.system;
 if (!sy) return '';
 const body=data.body;
 const actions=copy(u,sy.system)+note(u,sy.system,sy.address,'')+button(u,'close-system','Закрыть');
 let content=sy.note?'<p class="explore-note">'+u.esc(sy.note)+'</p>':'';
 if (body) {
  const values=[['Температура',body.temperature,' K'],['Гравитация',body.gravity==null?null:body.gravity/9.80665,' g'],['Радиус',body.radius==null?null:body.radius/1000,' км'],['От точки прибытия',body.distanceLs,' св. с.']];
  content+='<article class="explore-body-detail"><header><h3>'+u.esc(body.name)+'</h3>'+note(u,sy.system,sy.address,body.id)+'</header>'+
   '<div class="explore-metrics">'+values.filter(([,v])=>v!=null).map(([label,v,unit])=>'<div><small>'+q(u,label)+'</small><strong>'+number(u,v)+q(u,unit)+'</strong></div>').join('')+'</div>'+
   (body.note?'<p class="explore-note">'+u.esc(body.note)+'</p>':'')+'</article>';
 }
 const bodies=rows(data.bodies?.rows);
 const valuable=rows(data.valuableBodies);
 if(valuable.length)content+='<h3>'+q(u,'Цели для картографирования')+'</h3>'+table(u,['Тело','Тип','Оценка DSS'],valuable.map(b=>[
  systemLink(u,b.name,b.systemAddress,b.id),worldType(u,b),credits(u,b.mappingEstimate)]));
 return '<section class="panel explore-panel explore-system"><header class="explore-header"><h2>'+u.esc(sy.system)+'</h2><div class="explore-actions">'+actions+'</div></header>'+
  content+(bodies.length?bodyTable(u,bodies)+pages(u,'bodyPage',data.bodies):empty(u,'Тела ещё не зарегистрированы.'))+'</section>';
}

function expeditions(u,data,query) {
 const exp=data.expedition, wallet=data.wallet||{}, isActive=exp?.startedAt&&!exp?.endedAt;
 const select=choice(u,'expeditionId',[['all',u.t('Вся история')],...rows(data.expeditions).map(x=>[x.id,x.name])],query.expeditionId??exp?.id??'all','Экспедиция');
 let result='<div class="explore-toolbar">'+select+button(u,'new-expedition','Новая экспедиция','','primary')+'</div>';
 if (exp) {
  const dateText=exp.startedAt?date(u,exp.startedAt)+(exp.endedAt?' → '+date(u,exp.endedAt):''):q(u,'Запланировано');
  const actions=button(u,'edit-expedition','Изменить')+(!exp.startedAt?button(u,'start-expedition','Начать экспедицию'):isActive?button(u,'finish-expedition','Завершить экспедицию'):'')+
   button(u,'remove-expedition','Удалить');
  let plan='<p class="explore-caption">'+dateText+(exp.days?' · '+q(u,'Дней по плану')+' '+number(u,exp.days):'')+'</p>'+
   (exp.goal?'<p>'+u.esc(exp.goal)+'</p>':'')+(exp.notes?'<p class="explore-note">'+u.esc(exp.notes)+'</p>':'');
 if(exp.returnSystem)plan+='<div class="explore-return"><span>'+q(u,'Система возвращения')+'</span><strong>'+u.esc(exp.returnSystem)+'</strong>'+copy(u,exp.returnSystem)+'</div>';
  const stops=rows(exp.stops);
  if(stops.length)plan+='<h3>'+q(u,'Запланированные остановки')+'</h3>'+table(u,['Система','Посещено','Заметка','Действия'],stops.map(s=>[systemLink(u,s.system,s.address),known(u,s.visitedAt),u.esc(s.note||'—'),copy(u,s.system)]));
  const path=rows(exp.path);
  if(path.length>1)plan+='<h3>'+q(u,'Личный путь')+'</h3><canvas id="explore-path" height="220" role="img" aria-label="'+q(u,'Проекция маршрута в плоскости XZ')+'"></canvas>'+
   '<p class="explore-caption">'+u.esc(path[0].system)+' → '+u.esc(path.at(-1).system)+' · '+number(u,exp.travelCount)+'</p>';
  result+=section(u,'План экспедиции',plan,'<div class="explore-actions">'+actions+'</div>');
 }
 const metrics=[['Данные на борту, оценка',wallet.onboardEstimate],['Подтверждённая выплата',wallet.confirmedCredits],['Потерянные данные, оценка',wallet.lostEstimate]];
 result+='<div class="explore-metrics explore-wallet">'+metrics.map(([label,value])=>'<div class="panel"><small>'+q(u,label)+'</small><strong>'+credits(u,value)+'</strong></div>').join('')+'</div>';
 if(wallet.unpricedBodies>0)result+='<p class="explore-caption">'+q(u,'Для некоторых тел оценка недоступна.')+'</p>';
 const systems=rows(data.systems?.rows);
 result+=section(u,'История систем',systems.length?table(u,['Система','Посещено','Полное сканирование','Скан / DSS','На борту','Действия'],
  systems.map(s=>[systemLink(u,s.system,s.address),date(u,s.visitedAt),known(u,s.fssAt),number(u,s.scanned)+' / '+number(u,s.mapped),credits(u,s.estimated),copy(u,s.system)]))+pages(u,'page',data.systems):
  empty(u,'В журнале этой экспедиции пока нет систем.'),
  '<label class="explore-search"><span class="sr-only">'+q(u,'Поиск системы')+'</span><input type="search" data-explore-filter placeholder="'+q(u,'Поиск системы')+'" value="'+u.esc(query.filter||'')+'"></label>');
 const receipts=rows(data.receipts);
 if(receipts.length)result+=section(u,'Продажи и потери',table(u,['Дата','Событие','Системы','Оценка','Выплата'],receipts.map(r=>[
  date(u,r.at),q(u,r.kind==='loss'?'Потеря данных':'Продажа данных'),u.esc(rows(r.systems).join(', ')||'—'),credits(u,r.estimated),credits(u,r.credits)])));
 return result;
}

function findings(u,data,query) {
 const profile=data.profile||{}, rules=rows(data.rules), list=rows(data.findings?.rows);
 const chooser=choice(u,'findingProfile',rows(data.profiles).map(p=>[p.id,p.name||u.t('Основной профиль')]),profile.id,'Профиль находок');
 const picker=choice(u,'rule',[['',u.t('Все правила')],...rules.map(r=>[r.key,locale(u,r.title,r.english)])],query.rule,'Правило');
 let result='<div class="explore-toolbar">'+chooser+button(u,'edit-profile','Правила и уведомления')+button(u,'new-profile','Новый профиль')+
  (rows(data.profiles).length>1?button(u,'remove-profile','Удалить'):'')+'</div>';
 result+=section(u,'Журнал находок',list.length?table(u,['Дата','Правило','Тело','Почему отмечено','Действия'],list.map(f=>[
  date(u,f.observedAt),u.esc(locale(u,f.title,f.englishTitle)),systemLink(u,f.body,f.systemAddress,f.bodyId)+'<small>'+u.esc(f.system)+'</small>',
  u.esc(locale(u,f.detail,f.englishDetail)),note(u,f.system,f.systemAddress,f.bodyId)+(rows(data.expeditions).length?button(u,'stop','В экспедицию',attr(u,'system',f.system)+' '+attr(u,'address',f.systemAddress)):'')
 ]))+pages(u,'page',data.findings):empty(u,'Находок по выбранным правилам пока нет.'),picker);
 const records=rows(data.records);
 if(records.length)result+=section(u,'Личные рекорды',table(u,['Показатель','Значение','Тело','Дата'],records.map(r=>[
  q(u,recordLabels[r.key]||r.key),number(u,r.value)+(r.unit?' '+u.esc(r.unit):''),systemLink(u,r.body,r.systemAddress,r.bodyId)+'<small>'+u.esc(r.system)+'</small>',date(u,r.observedAt)])));
 return result;
}

function surveys(u,data,query) {
 const p=data.survey;
 let result='<div class="explore-toolbar">'+choice(u,'surveyId',rows(data.surveys).map(x=>[x.id,x.name]),p?.id,'Обследование')+
  button(u,'new-survey','Новое обследование','','primary')+
  '<label class="pill quiet explore-file">'+q(u,'Импорт JSON')+'<input type="file" accept=".json,application/json" data-explore-import></label></div>';
 if(p){
  const next=p.next;
  const actions=button(u,'edit-survey','Изменить')+button(u,'export','Экспорт JSON')+button(u,'remove-survey','Удалить');
  let summary='<div class="explore-survey-summary"><span>'+q(u,criteria[p.criterion]||criteria.visit)+'</span><strong>'+number(u,p.completed)+' / '+number(u,rows(p.targets).length)+'</strong>'+
   (p.skipped?' · '+q(u,'Пропущено')+' '+number(u,p.skipped):'')+'</div>'+(p.notes?'<p class="explore-note">'+u.esc(p.notes)+'</p>':'');
  if(p.boxel)summary+='<p class="explore-caption">'+u.esc(p.boxel.prefix)+' · '+number(u,p.boxel.widthLy)+q(u,' св. л.')+' · '+q(u,'Номера дают кандидатов. Существование системы проверяется в игре.')+
   (p.boxel.grid?' · '+q(u,'Индексы ячейки')+' '+u.esc(p.boxel.grid.join(' / ')):'')+'</p>';
  if(next)summary+='<div class="explore-return"><span>'+q(u,'Следующая система')+'</span><strong>'+u.esc(next.system)+'</strong>'+copy(u,next.system)+'</div>';
  const targets=rows(p.results?.rows);
  if(targets.length)summary+=table(u,['Система','Посещение','Полное сканирование','Картографирование','Заметка','Действия'],targets.map(t=>[
   systemLink(u,t.system,t.address)+(t.candidate?'<small>'+q(u,'Кандидат')+'</small>':t.skipped?'<small>'+q(u,'Пропущено')+'</small>':''),
   known(u,t.progress?.visitedAt),known(u,t.progress?.fssAt),known(u,t.progress?.mappedAt),u.esc(t.note||'—'),
   copy(u,t.system)+button(u,'skip-target',t.skip?'Вернуть в план':'Пропустить',attr(u,'system',t.system)+' '+attr(u,'skip',!t.skip))]))+pages(u,'page',p.results);
  result+=section(u,'Ход обследования',summary,'<div class="explore-actions">'+actions+'</div>');
 }
 const worlds=rows(data.worlds?.rows);
 const filter=choice(u,'worldFilter',[['',u.t('Все миры')],['earthlike',u.t('Землеподобный мир')],['terraformable',u.t('Терраформируемый мир')]],query.worldFilter,'Тип мира');
 result+=section(u,'Личный каталог миров',worlds.length?bodyTable(u,worlds)+pages(u,'worldPage',data.worlds):empty(u,'Таких миров в журнале пока нет.'),filter);
 return result;
}

function field(u,label,name,value,{multiline=false,type='text',min,max,required=false,placeholder=''}={}) {
 const attrs=' name="'+name+'" '+(required?'required ':'')+(min==null?'':'min="'+min+'" ')+(max==null?'':'max="'+max+'" ');
 const control=multiline?'<textarea '+attrs+' rows="'+(name==='targetText'?6:3)+'" placeholder="'+q(u,placeholder)+'">'+u.esc(value??'')+'</textarea>':
  '<input '+attrs+' type="'+type+'" value="'+u.esc(value??'')+'">';
 return '<label><span>'+q(u,label)+'</span>'+control+'</label>';
}
const lineText = targets => rows(targets).map(t=>[t.system,t.address||'',t.note||'',t.skip?'skip':''].join(' | ').replace(/(?: \| )+$/,'')).join('\n');
function readLines(value) {
 return String(value||'').split(/\r?\n/).map(s=>s.trim()).filter(Boolean).map(line=>{
  const [system,address,note,skip]=line.split('|').map(x=>x.trim());return {system,address,note,skip:skip==='skip'};
 });
}
function optionSet(u,values,value) {
 return Object.entries(values).map(([id,label])=>'<option value="'+u.esc(id)+'" '+(id===value?'selected':'')+'>'+q(u,label)+'</option>').join('');
}
function editor(u,r,data) {
 const edit=r.edit, item=edit.item||{};
 let label='',html='';
 if(edit.type==='expedition'){
  label=item.id?'Изменить экспедицию':'Новая экспедиция';
  html=field(u,'Название','name',item.name,{required:true})+field(u,'Цель','goal',item.goal)+field(u,'Дней по плану','days',item.days,{type:'number',min:0,max:3650})+
   field(u,'Система возвращения','returnSystem',item.returnSystem)+field(u,'Заметки','notes',item.notes,{multiline:true})+
   field(u,'Остановки, по одной на строку','targetText',edit.rawTargets??lineText(item.stops),{multiline:true,placeholder:'Система | ID64 | заметка'});
 }else if(edit.type==='survey'){
  label=item.id?'Изменить обследование':'Новое обследование';
  html=field(u,'Название','name',item.name,{required:true})+
   '<label><span>'+q(u,'Критерий завершения')+'</span><select name="criterion">'+optionSet(u,criteria,item.criterion||'visit')+'</select></label>'+
   '<fieldset><legend>'+q(u,'Типы тел для DSS')+'</legend>'+Object.entries(worldLabels).map(([key,label])=>'<label class="explore-check"><input type="checkbox" name="mappingTypes" value="'+key+'" '+(rows(item.mappingTypes).includes(key)?'checked':'')+'>'+q(u,label)+'</label>').join('')+'</fieldset>'+
   '<label class="explore-check"><input name="skipVisited" type="checkbox" '+(item.skipVisited?'checked':'')+'>'+q(u,'Пропускать посещённые системы')+'</label>'+
   field(u,'Системы, по одной на строку','targetText',edit.rawTargets??lineText(item.targets),{multiline:true,placeholder:'Система | ID64 | заметка'})+
   '<details class="explore-boxel" '+(item.boxelSeed?'open':'')+'><summary>'+q(u,'Диапазон boxel')+'</summary>'+
   field(u,'Пример процедурной системы','boxelSeed',item.boxelSeed)+field(u,'Первый номер','sequenceFrom',item.sequenceFrom??0,{type:'number',min:0,max:16777215})+
   field(u,'Последний номер','sequenceTo',item.sequenceTo??0,{type:'number',min:0,max:16777215})+
   '<small>'+q(u,'Оставьте список систем пустым, чтобы заполнить его номерами диапазона.')+'</small></details>'+
   field(u,'Заметки','notes',item.notes,{multiline:true});
 }else if(edit.type==='profile'){
  label='Правила и уведомления';
  html=field(u,'Название профиля','name',item.name,{required:!item.id})+rows(data.rules).map(rule=>{
   const settings=item.rules?.[rule.key]||{mode:'history',threshold:rule.default,cooldown:15};
   return '<fieldset class="explore-rule"><legend>'+u.esc(locale(u,rule.title,rule.english))+'</legend>'+
    '<label><span>'+q(u,'Действие')+'</span><select name="'+rule.key+'-mode">'+optionSet(u,{off:'Выключено',history:'Только в журнал',notify:'Уведомлять'},settings.mode)+'</select></label>'+
    (rule.max?field(u,'Порог',''+rule.key+'-threshold',settings.threshold,{type:'number',min:0,max:rule.max}).replace('type="number"','type="number" step="any"'):'')+
    (rule.key==='material'?field(u,'Материал','material',settings.material):'')+
    field(u,'Пауза между уведомлениями, мин',''+rule.key+'-cooldown',settings.cooldown,{type:'number',min:0,max:1440})+'</fieldset>';
  }).join('');
 }else if(edit.type==='note'){
  label='Заметка';
  html=field(u,'Заметка','note',item.note,{multiline:true});
 }else if(edit.type==='stop'){
  label='Добавить остановку';
  html='<label><span>'+q(u,'Экспедиция')+'</span><select name="expeditionId">'+rows(data.expeditions).map(e=>'<option value="'+u.esc(e.id)+'">'+u.esc(e.name)+'</option>').join('')+'</select></label>'+
   field(u,'Заметка','note',item.note,{multiline:true});
 }else{
  label='Удалить';
  html='<p>'+u.esc(item.name||'')+'</p>';
 }
 const actions='<div class="explore-actions">'+button(u,'cancel','Отмена')+
  '<button type="submit" class="pill primary" '+(r.writing?'disabled':'')+'>'+q(u,edit.type==='delete'?'Удалить':'Сохранить')+'</button></div>';
 return section(u,label,'<form class="explore-form" data-explore-form>'+html+actions+'</form>');
}

function content(C,u) {
 const [r,query]=model(C), data=r.data[query.mode], error=r.errors[query.mode];
 const issue=error?'<p class="explore-error" role="alert">'+u.esc(error)+'</p>':'';
 if(!data)return issue+(r.loading?empty(u,'Загрузка истории…'):button(u,'reload','Повторить'));
 if(!r.owner)return issue+empty(u,'Для исследований загрузите командира из журнала игры.');
 if(r.edit)return issue+editor(u,r,data);
 const view={expeditions,findings,surveys}[query.mode]||expeditions;
 return issue+detail(u,data)+view(u,data,query);
}
function render(C,u){
 return u.heading(u.t(C.mode||'Экспедиции'),'')+u.renderModes(true)+'<div id="exploration-workspace" class="exploration-workspace">'+content(C,u)+'</div>';
}
function draw(C,u) {
 if(!active(C))return;
 const host=u.$('#exploration-workspace');
 if(!host)return;
 const html=content(C,u);
 if(host.exploreMarkup!==html){
  const focused=host.ownerDocument?.activeElement;
  const field=focused?.dataset?.exploreQuery;
  host.innerHTML=html;host.exploreMarkup=html;
  if(field)host.querySelector('[data-explore-query="'+field+'"]')?.focus({preventScroll:true});
 }
 const [r,qry]=model(C);
 drawPath(host.querySelector('#explore-path'),r.data[qry.mode]?.expedition?.path);
}
function drawPath(canvas,raw) {
 const points=rows(raw).map(x=>x.coordinates).filter(x=>Array.isArray(x)&&x.length===3);
 if(!canvas||points.length<2)return;
 const width=Math.round(canvas.getBoundingClientRect().width)||1, height=220, ratio=Math.min(window.devicePixelRatio||1,2);
 const stamp=[width,height,ratio,JSON.stringify(points)].join('|');
 if(canvas.exploreStamp===stamp)return;
 canvas.exploreStamp=stamp;canvas.width=width*ratio;canvas.height=height*ratio;
 const ctx=canvas.getContext('2d');if(!ctx)return;
 ctx.scale(ratio,ratio);
 const xs=points.map(p=>p[0]), zs=points.map(p=>p[2]), xmin=Math.min(...xs),xmax=Math.max(...xs),zmin=Math.min(...zs),zmax=Math.max(...zs);
 const scale=Math.min((width-40)/Math.max(xmax-xmin,1),(height-40)/Math.max(zmax-zmin,1));
 const plot=([x,,z])=>[width/2+(x-(xmin+xmax)/2)*scale,height/2-(z-(zmin+zmax)/2)*scale];
 ctx.strokeStyle=getComputedStyle(canvas).color;ctx.globalAlpha=.66;ctx.lineWidth=2;ctx.beginPath();
 points.forEach((p,i)=>{const [x,y]=plot(p);if(i)ctx.lineTo(x,y);else ctx.moveTo(x,y);});ctx.stroke();
 ctx.globalAlpha=1;ctx.fillStyle=ctx.strokeStyle;
 for(const point of [points[0],points.at(-1)]){const [x,y]=plot(point);ctx.beginPath();ctx.arc(x,y,4,0,2*Math.PI);ctx.fill();}
}

async function load(C,u,force=false) {
 if(!active(C))return;
 const [r,query]=model(C), key=query.mode;
 const signature=JSON.stringify([query,C.state?.explorationRevision??C.state?.revision]);
 if(!force&&(r.loading||r.signature===signature))return;
 const token=++r.sequence;
 r.loading=true;r.signature=signature;
 try{
  const result=await u.call('explorationQuery',{...query,expectedPlanWorkspace:r.owner});
  if(active(C)&&model(C)[0]===r&&r.sequence===token){r.data[key]=result;r.errors[key]='';}
 }catch(err){if(r.sequence===token)r.errors[key]=u.t(err?.message||'Не удалось загрузить историю.');}
 finally{
  if(r.sequence===token){
   r.loading=false;
   if(active(C)&&model(C)[0]===r&&!r.edit)draw(C,u);
  }
 }
}
async function mutate(C,u,input) {
 const [r,query]=model(C);
 if(r.writing)return;
 const token=++r.sequence;r.loading=false;r.writing=true;
 const key=query.mode;
 try{
  const result=await u.call('exploration.update',{...query,...input,expectedPlanWorkspace:r.owner});
  if(model(C)[0]!==r||r.sequence!==token)return;
  r.data[key]=result;r.errors[key]='';r.edit=null;
  if(result.expedition?.id&&input.action!=='remove-expedition')query.expeditionId=result.expedition.id;
  if(result.survey?.id&&input.action!=='remove-survey')query.surveyId=result.survey.id;
 }catch(err){
  if(r.sequence===token)r.errors[key]=u.t(err?.message||'Не удалось сохранить изменения.');
 }finally{
  if(r.sequence===token){r.writing=false;draw(C,u);}
 }
}
function formInput(form,edit,data) {
 const fields=new FormData(form), get=key=>fields.get(key), item=edit.item||{};
 switch(edit.type){
 case 'expedition':return {action:'save-expedition',id:item.id,name:get('name'),goal:get('goal'),days:Number(get('days')||0),returnSystem:get('returnSystem'),notes:get('notes'),stops:readLines(get('targetText'))};
 case 'survey':return {action:'save-survey',id:item.id,name:get('name'),notes:get('notes'),criterion:get('criterion'),mappingTypes:fields.getAll('mappingTypes'),skipVisited:fields.has('skipVisited'),
  targets:readLines(get('targetText')),boxelSeed:get('boxelSeed'),sequenceFrom:Number(get('sequenceFrom')),sequenceTo:Number(get('sequenceTo'))};
 case 'profile':return {action:'save-finding-profile',id:item.id,name:get('name'),rules:Object.fromEntries(rows(data.rules).map(({key})=>[key,{mode:get(key+'-mode'),threshold:Number(get(key+'-threshold')||0),material:key==='material'?get('material'):'',cooldown:Number(get(key+'-cooldown')||0)}]))};
 case 'note':return {action:'save-note',systemAddress:item.address,bodyId:item.body,note:get('note')};
 case 'stop':return {action:'add-expedition-stop',id:get('expeditionId'),systemAddress:item.address,note:get('note')};
 default:return {action:edit.action,id:item.id};
 }
}
function init(C,u){
 if(!active(C))return;
 const host=u.$('#exploration-workspace'),[r,query]=model(C);
 if(!host)return;
 if(!host.exploreBound){
  host.exploreBound=true;
  host.addEventListener('submit',event=>{
   if(!event.target.matches('[data-explore-form]'))return;
   event.preventDefault();
   if(r.edit){
    const input=formInput(event.target,r.edit,r.data[query.mode]);
    r.edit.item={...r.edit.item,...input};
    const target=event.target.elements?.targetText;
    if(target)r.edit.rawTargets=target.value;
    mutate(C,u,input);
   }
  });
  host.addEventListener('change',async event=>{
   const field=event.target.dataset.exploreQuery;
   if(field){
    if(field==='findingProfile'){await mutate(C,u,{action:'select-finding-profile',id:event.target.value});return;}
    if(field==='surveyId'){query.page=0;await mutate(C,u,{action:'select-survey',id:event.target.value});return;}
    query[field]=event.target.value;
    if(field==='expeditionId'&&!query[field])query.expeditionId='all';
    query[field==='worldFilter'?'worldPage':'page']=0;
    await load(C,u,true);return;
   }
   if(event.target.matches('[data-explore-import]')){
    const file=event.target.files?.[0];
    if(!file)return;
    if(file.size>524288){r.errors[query.mode]=u.t('Файл обследования слишком большой.');draw(C,u);return;}
    try{await mutate(C,u,{action:'import-survey',text:await file.text()});}
    catch(err){r.errors[query.mode]=u.t(err?.message||'Не удалось открыть файл.');draw(C,u);}
   }
  });
  let delay;
  host.addEventListener('input',event=>{
   if(!event.target.matches('[data-explore-filter]'))return;
   clearTimeout(delay);const value=event.target.value;
   delay=setTimeout(()=>{query.filter=value;query.page=0;load(C,u,true);},250);
  });
  host.addEventListener('click',event=>{
   const btn=event.target.closest('[data-explore]');if(!btn)return;
   void handle(btn,C,u).catch(err=>{r.errors[query.mode]=u.t(err?.message||'Не удалось выполнить действие.');draw(C,u);});
  });
 }
 draw(C,u);
 if(!r.edit)load(C,u);
}
async function handle(btn,C,u){
 const [r,query]=model(C), data=r.data[query.mode]||{};
 const action=btn.dataset.explore, {address,body,system}=btn.dataset;
 switch(action){
 case 'reload':return load(C,u,true);
 case 'page':query[btn.dataset.field]=Number(btn.dataset.value);return load(C,u,true);
 case 'copy':await u.call('native',{action:'copy',text:system});u.toast(u.t('Система скопирована'));return;
 case 'open-system':query.systemAddress=address;delete query.systemName;query.bodyId=body||'';query.bodyPage=0;return load(C,u,true);
 case 'close-system':delete query.systemAddress;delete query.systemName;delete query.bodyId;return load(C,u,true);
 case 'new-expedition':case 'edit-expedition':r.edit={type:'expedition',item:action==='edit-expedition'?data.expedition:{}};break;
 case 'new-survey':case 'edit-survey':r.edit={type:'survey',item:action==='edit-survey'?data.survey:{}};break;
 case 'new-profile':case 'edit-profile':r.edit={type:'profile',item:action==='edit-profile'?data.profile:{rules:data.profile?.rules}};break;
 case 'note':r.edit={type:'note',item:{address,body,note:body?(data.body?.id===body?data.body.note:rows(data.findings?.rows).find(f=>f.bodyId===body&&f.systemAddress===address)?.note):data.system?.note}};break;
 case 'stop':r.edit={type:'stop',item:{address}};break;
 case 'skip-target':return mutate(C,u,{action:'skip-survey-target',id:data.survey?.id,systemName:system,skip:btn.dataset.skip==='true'});
 case 'remove-profile':r.edit={type:'delete',action:'remove-finding-profile',item:data.profile};break;
 case 'remove-expedition':case 'remove-survey':r.edit={type:'delete',action,item:action==='remove-expedition'?data.expedition:data.survey};break;
 case 'start-expedition':case 'finish-expedition':return mutate(C,u,{action,id:data.expedition?.id});
 case 'export':{
  const result=await u.call('exploration.export',{id:data.survey?.id,expectedPlanWorkspace:r.owner});
  const saved=await u.call('native',{action:'save-json',name:result.name,text:result.text});
  if(saved?.ok)u.toast(u.t('Файл сохранён'));
  return;
 }
 case 'cancel':r.edit=null;r.errors[query.mode]='';break;
 }
 draw(C,u);
 u.$('#exploration-workspace [data-explore-form] input')?.focus({preventScroll:true});
}
function leave(C){
 const r=C.explorationUI;if(!r)return;
 r.sequence++;r.loading=false;r.writing=false;r.signature='';r.edit=null;
}
window.ExplorationUI={active,render,init,leave};
})();
