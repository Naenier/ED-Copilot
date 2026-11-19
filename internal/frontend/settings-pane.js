(()=>{
'use strict';
const t=(value,vars)=>window.CopilotI18n?.t(value,vars)??String(value??'');
const th=(value,vars)=>t(value,vars).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
// This module renders contextual help; the controller owns inspector visibility
// and its existing desktop/localStorage persistence.
const topics=[
 {id:'workspace',title:'Рабочее пространство',icon:'layout-grid'},
 {id:'overlay',title:'Игровой оверлей',icon:'eye'},
 {id:'theme',title:'Цветовая палитра',icon:'sun'},
 {id:'journal',title:'Журнал и данные',icon:'radio'},
 {id:'mining',title:'Фильтры добычи',icon:'pickaxe'},
 {id:'prices',title:'Справочные цены',icon:'arrow-left-right'},
];
const bindings=new WeakMap();
function active(C){return C?.tab==='settings';}
const logsActive=C=>active(C)&&C.controlsUI?.tab==='diagnostics';
function canToggle(kind,C){return kind==='table'||kind==='settings'&&(!C||!logsActive(C)&&!window.ControlsUI?.active(C));}
function session(C){
 if(!C.settingsPane||!topics.some(topic=>topic.id===C.settingsPane.topic))C.settingsPane={topic:'workspace'};
 return C.settingsPane;
}
function facts(rows,u){return '<dl class="settings-help-facts">'+rows.map(([label,value])=>'<div><dt>'+u.esc(t(label))+'</dt><dd>'+u.esc(value)+'</dd></div>').join('')+'</dl>';}
function text(paragraphs,u){return paragraphs.filter(Boolean).map(value=>'<p>'+u.esc(value)+'</p>').join('');}
function content(C,u){
 const state=C.state||{},settings=state.settings||{},desktop=state.desktop||{},connection=state.connection||{},theme=state.theme||{};
 const topic=session(C).topic,mode={windowed:t("В окне"),fullscreen:t("На весь экран"),borderless:t("Окно без рамки")}[desktop.displayMode]||t("В окне");
 let body='';
 if(topic==='workspace')body=text([
  t("«Настроить» меняет состав и порядок меню. Ctrl+K ищет также скрытые разделы."),
  t("Панель подробностей показывает данные выбранной строки. Её можно скрыть кнопкой в верхней панели."),
 ],u)+facts([[t("Отображение"),mode],[t("Быстрый переход"),'Ctrl + K'],[t("Полноэкранный режим"),'F11']],u);
 if(topic==='overlay'){
  const activity=(C.boot?.activities||[]).find(item=>item.Value===settings.activity)?.Label||t("Выберите занятие в боковом меню");
  body=text([
   t("Оверлей виден только при активном окне Elite Dangerous."),
   t("Нажмите значок глаза и выберите занятие под названием корабля. Для снабжения выберите колонию."),
  ],u)+facts([[t("Панели в игре"),settings.overlayEnabled?t("Включены"):t("Выключены")],[t("Занятие"),activity],[t("Переключение"),'Ctrl + Alt + H']],u);
 }
 if(topic==='theme'){
  body=text([
   t("Выберите палитру, настройте матрицу RGB или скопируйте цвета игры. Импорт сначала открывает предпросмотр."),
   t("«Игровая палитра в основном окне» применяет её также к Copilot; без флажка она действует только на оверлей."),
   t(""),
  ],u)+facts([[t("Основное окно"),desktop.useGamePalette?t("Игровая палитра"):t("Стандартный фон")],[t("Палитра"),theme.settings?.mode==='game'?t("Из игры"):theme.settings?.mode==='matrix'?t("Матрица RGB"):theme.settings?.mode==='custom'?t("Пользовательская"):(theme.presets||[]).find(item=>item.id===theme.settings?.preset)?.name||t("Пользовательская")]],u);
  const colors=[['accent',t("Акцент")],['text',t("Текст")],['panel',t("Панель")]].filter(([key])=>/^#[0-9a-f]{6}$/i.test(theme.palette?.[key]||''));
  if(colors.length)body+='<div class="settings-help-colors">'+colors.map(([key,label])=>'<span><i style="background:'+theme.palette[key]+'"></i>'+u.esc(t(label))+'<code>'+u.esc(theme.palette[key])+'</code></span>').join('')+'</div>';
 }
 if(topic==='journal'){
  const status=connection.error?t("Журнал недоступен"):connection.scanning?t("Читаем журналы"):connection.lastUpdate||Number(connection.eventCount)>0?t("Журнал подключён"):connection.journalDir?t("Ожидаем события игры"):t("Папка не выбрана");
  body=text([
   t("Если журнал не подключён, выберите папку журналов Elite Dangerous."),
   t("После переноса журналов нажмите «Перечитать журналы». Личные планы сохраняются."),
  ],u)+facts([[t("Подключение"),status],[t("Прочитано событий"),Number.isFinite(Number(connection.eventCount))&&connection.eventCount!==null&&connection.eventCount!==''?u.fmt(Number(connection.eventCount)):t("Нет данных")],[t("Папка"),connection.journalDir||t("Не выбрана")]],u);
  if(connection.error)body+='<p class="settings-help-warning">'+u.esc(connection.error)+'</p>';
 }
 if(topic==='mining'){
  const tier={low:t("Любая"),medium:t("Средняя и выше"),high:t("Высокая")}[settings.minTier]||t("Не задана");
  const method={all:t("Любой"),laser:t("Лазер"),core:t("Ядра"),subsurface:t("Подповерхностная"),surface:t("Поверхность астероида"),'surface-planet':t("Планетарная"),raw:t("Сырьё")}[settings.method]||t("Не задан");
  body=text([
   t("Укажите способ добычи, ценность ресурса, радиус поиска и расстояние от звезды."),
   t("Порог содержания и скрытие истощённых колец применяются к лазерной добыче."),
   t(""),
  ],u)+facts([[t("Минимальная ценность"),tier],[t("Способ добычи"),method]],u);
 }
 if(topic==='prices')body=text([
  t("Справочная цена используется для оценки груза и планов без свежей рыночной котировки."),
  t("Пустое поле сбрасывает вашу цену к последней известной цене игры. Ноль задаёт нулевую оценку."),
  t("Котировки рынков и история сделок используют собственные цены."),
 ],u);
 return '<h3>'+u.esc(t(topics.find(item=>item.id===topic).title))+'</h3>'+body;
}
function pane(C,u){
 return `<aside class="inspector settings-help" aria-label="${th("Подробности настроек")}"><div class="inspector-title"><span>`+u.icon('circle-help')+`${th("Подробности настроек")}</span>`+u.btn('','inspector','x','quiet settings-help-close',`title="${th("Скрыть подробности")}" aria-label="${th("Скрыть подробности")}"`)+`</div><nav class="settings-help-topics" aria-label="${th("Темы настроек")}">`+topics.map(topic=>'<button data-settings-topic="'+topic.id+'" aria-pressed="'+(session(C).topic===topic.id)+'">'+u.icon(topic.icon)+u.esc(t(topic.title))+'</button>').join('')+'</nav><div data-settings-help-content aria-live="polite">'+content(C,u)+'</div></aside>';
}
function render(C,u,mainHTML){
 const general=()=>typeof mainHTML==='function'?mainHTML():mainHTML;
 if(!active(C))return general();
 const version=C.state?.app?.version;
 const header=u.heading(t("Настройки"),version?t("Версия приложения {version}",{version}):'')+(window.ControlsUI?.tabs(C,u)||'');
 if(logsActive(C))return '<div class="settings-workspace" data-settings-workspace><div class="settings-main">'+header+renderLogs(u)+'</div></div>';
 if(window.ControlsUI?.active(C))return '<div class="settings-workspace" data-settings-workspace><div class="settings-main">'+header+window.ControlsUI.render(C,u)+'</div></div>';
 return '<div class="settings-workspace'+(C.inspector?' has-help':'')+'" data-settings-workspace><div class="settings-main">'+header+general()+'</div>'+(C.inspector?pane(C,u):'')+'</div>';
}
const logBindings=new WeakSet();
const logLevels={debug:'Отладка',info:'Информация',warn:'Предупреждение',error:'Ошибка'};
const logSubsystems={
 app:'Приложение',journal:'Журналы игры',ui:'Интерфейс',overlay:'Игровой оверлей',network:'Сеть',settings:'Настройки',
 updates:'Обновления',frontier:'Frontier',inara:'Inara',edsm:'EDSM',spansh:'Spansh',github:'GitHub',edtools:'EDTools',
 trade:'Торговля',carrier:'Флотоносцы',colony:'Колонизация',engineering:'Инженерия',fits:'Сборки кораблей',
 controls:'Управление игрой',galaxy:'Галактика',search:'Поиск',exploration:'Исследования',exobiology:'Экзобиология',
 materials:'Материалы',reference:'Энциклопедия',profile:'Профиль',mining:'Добыча',powerplay:'Игра сил',
 surface:'Полёт по координатам',
};
const logLabel=(labels,key)=>Object.hasOwn(labels,key)?t(labels[key]):key;
function logState(C){
 return C.diagnosticsUI??={draft:{level:'',subsystem:'',search:''},filters:{},subsystems:[],cursors:[''],page:null,request:0,loading:false,failed:false};
}
function renderLogs(u){
 const levels=Object.entries(logLevels).map(([id,label])=>`<option value="${id}">${th(label)}</option>`).join('');
 return `<section class="settings-group settings-logs" data-settings-logs aria-label="${th("Журнал приложения")}">
  <form class="settings-logs-filters">
   <label for="settings-logs-level">${th("Уровень записи")}
    <select id="settings-logs-level" name="level"><option value="">${th("Все уровни")}</option>${levels}</select>
   </label>
   <label for="settings-logs-subsystem">${th("Компонент")}
    <select id="settings-logs-subsystem" name="subsystem"><option value="">${th("Все компоненты")}</option></select>
   </label>
   <label for="settings-logs-search">${th("Поиск по журналу")}
    <input id="settings-logs-search" name="search" type="search" maxlength="120" autocomplete="off">
   </label>
   <div class="settings-logs-actions">
    <button type="submit" class="pill primary">${th("Применить")}</button>
    <button type="button" class="pill quiet" data-logs-action="refresh">${u.icon('refresh-cw')}${th("Обновить")}</button>
   </div>
  </form>
  <div class="settings-logs-status" data-logs-status role="status"></div>
  <div data-logs-results></div>
  <nav class="settings-logs-paging" data-logs-paging aria-label="${th("Страницы журнала")}" hidden>
   <button type="button" class="pill" data-logs-action="previous">${th("Более новые")}</button>
   <span data-logs-page></span>
   <button type="button" class="pill" data-logs-action="next">${th("Более ранние")}</button>
  </nav>
 </section>`;
}
function logTable(entries,u){
 const rows=entries.map(entry=>{
  const time=u.date?.(entry.timestamp,{dateStyle:'short',timeStyle:'medium'})||entry.timestamp;
  const severity=Object.hasOwn(logLevels,entry.level)?entry.level:'';
  return `<tr>
   <td><time datetime="${u.esc(entry.timestamp)}" title="${u.esc(entry.timestamp)}">${u.esc(time)}</time></td>
   <td><span class="badge log-level ${severity}">${u.esc(logLabel(logLevels,entry.level))}</span></td>
   <td title="${u.esc(entry.subsystem)}">${u.esc(logLabel(logSubsystems,entry.subsystem))}</td>
   <td>${th(entry.message)}<code class="log-event">${u.esc(entry.event)}</code></td>
  </tr>`;
 }).join('');
 return `<div class="table-scroll settings-logs-table"><table>
  <thead><tr><th scope="col">${th("Время")}</th><th scope="col">${th("Уровень записи")}</th><th scope="col">${th("Компонент")}</th><th scope="col">${th("Сообщение журнала")}</th></tr></thead>
  <tbody>${rows}</tbody>
 </table></div>`;
}
function paintLogs(C,u){
 if(!logsActive(C))return;
 const host=u.$('[data-settings-logs]');
 if(!host)return;
 const state=logState(C);
 const select=u.$('#settings-logs-subsystem',host);
 const subsystems=new Set(state.subsystems);
 if(state.draft.subsystem)subsystems.add(state.draft.subsystem);
 const options=[...subsystems].sort((a,b)=>logLabel(logSubsystems,a).localeCompare(logLabel(logSubsystems,b),u.locale?.()));
 const optionHTML=`<option value="">${th("Все компоненты")}</option>`+options.map(id=>`<option value="${u.esc(id)}">${u.esc(logLabel(logSubsystems,id))}</option>`).join('');
 if(select.dataset.options!==optionHTML){
  select.innerHTML=optionHTML;
  select.dataset.options=optionHTML;
  select.value=state.draft.subsystem;
 }
 const results=u.$('[data-logs-results]',host),status=u.$('[data-logs-status]',host);
 const entries=state.page?.entries||[];
 results.setAttribute('aria-busy',String(state.loading));
 results.innerHTML=entries.length?logTable(entries,u):'';
 status.classList.toggle('is-error',state.failed);
 let message='';
 if(state.loading)message=t("Загружаем журнал…");
 else if(state.failed)message=t("Не удалось загрузить журнал приложения.");
 else if(!entries.length){
  if(state.cursors.length>1)message=t("На этой странице нет записей.");
  else if(Object.values(state.filters).some(Boolean))message=t("Записей по этим фильтрам нет.");
  else message=t("В журнале пока нет записей.");
 }
 status.textContent=message;
 const paging=u.$('[data-logs-paging]',host);
 paging.hidden=!(state.page?.next||state.cursors.length>1);
 u.$('[data-logs-page]',host).textContent=t("Страница {page}",{page:state.cursors.length});
 u.$('[data-logs-action="previous"]',host).disabled=state.loading||state.cursors.length<=1;
 u.$('[data-logs-action="next"]',host).disabled=state.loading||!state.page?.next;
 u.$('[data-logs-action="refresh"]',host).disabled=state.loading;
}
async function loadLogs(C,u,cursors){
 const state=logState(C),request=++state.request;
 state.loading=true;
 state.failed=false;
 paintLogs(C,u);
 try {
  const page=await u.call('diagnostics.logs',{...state.filters,before:cursors.at(-1),limit:75});
  if(request!==state.request)return;
  state.page=page;
  state.subsystems=page.subsystems||[];
  state.cursors=cursors;
 }catch{
  if(request===state.request)state.failed=true;
 }finally{
  if(request===state.request){state.loading=false;paintLogs(C,u);}
 }
}
function initLogs(C,u){
 const host=u.$('[data-settings-logs]');
 if(!host||logBindings.has(host))return;
 logBindings.add(host);
 const state=logState(C),form=u.$('form',host);
 for(const field of ['level','search'])form.elements.namedItem(field).value=state.draft[field];
 form.addEventListener('input',event=>{
  const {name,value}=event.target;
  if(Object.hasOwn(state.draft,name))state.draft[name]=value;
 });
 form.addEventListener('submit',event=>{
  event.preventDefault();
  const {level,subsystem,search}=state.draft;
  state.filters={level,subsystem,search:search.trim()};
  state.cursors=[''];
  state.page=null;
  void loadLogs(C,u,state.cursors);
 });
 host.addEventListener('click',event=>{
  const action=event.target?.closest?.('[data-logs-action]')?.dataset.logsAction;
  if(state.loading)return;
  if(action==='refresh')void loadLogs(C,u,['']);
  if(action==='previous'&&state.cursors.length>1)void loadLogs(C,u,state.cursors.slice(0,-1));
  if(action==='next'&&state.page?.next)void loadLogs(C,u,[...state.cursors,state.page.next]);
 });
 const language=window.CopilotI18n?.language;
 if(!state.request||state.language!==language){
  state.language=language;
  state.cursors=[''];
  state.page=null;
  void loadLogs(C,u,state.cursors);
 }else paintLogs(C,u);
}
function renderProfile(u){
 return '<section class="settings-group"><h3>'+u.icon('shield-check')+th("Резервная копия профиля")+'</h3>'+
  '<p class="info-note">'+th("В копию входят настройки, планы и прогресс. Восстановление создаёт отдельный профиль в пустой папке.")+'</p>'+
  u.btn("Сохранить копию профиля",'profile-export','','quiet')+' '+
  u.btn("Восстановить из копии",'profile-import','','quiet')+'</section>';
}
async function handleProfile(action,button,u){
 button.disabled=true;
 try {
  const result=await u.call('native',{action});
  if(result?.cancelled)return;
  if(action==='profile-export'){u.toast(t("Копия профиля сохранена"));return;}
  u.dialog(t("Профиль восстановлен"),'<div class="prose-small"><p>'+u.esc(result.folder)+'</p><p>'+
   th("Для запуска этого профиля укажите его папку в параметре --data-dir.")+'</p></div>');
 }finally{
  button.disabled=false;
 }
}
function topicFor(target,C,u,host){
 const control=target?.closest?.('button,input,select,textarea');
 if(!control)return '';
 if(control.id==='main-game-palette')return 'theme';
 const id=control.dataset?.actionId||'',action=(C.actions||[]).find(item=>item.id===id);
 if(/prices/i.test(id))return 'prices';
 if(/theme|palette|matrix/i.test(id))return 'theme';
 if(/journal|folder|rescan/i.test(id+' '+(action?.kind||'')))return 'journal';
 if(/filters/i.test(id))return 'mining';
 if(/window|display/i.test(id)||['customize','inspector'].includes(control.dataset?.click))return 'workspace';
 if(control.dataset?.click==='overlay')return 'overlay';
 const group=control.closest?.('.settings-group');
 if(group)return ['workspace','overlay','theme','journal'][u.$$('.settings-group',host).indexOf(group)]||'';
 return '';
}
function init(C,u){
 if(logsActive(C)){initLogs(C,u);return;}
 if(window.ControlsUI?.active(C)){window.ControlsUI.init(C,u);return;}
 if(!active(C)||!C.inspector)return;
 const host=u.$('[data-settings-workspace]');
 if(!host)return;
 const current=bindings.get(host);
 if(current){current.C=C;current.u=u;return;}
 const binding={C,u};bindings.set(host,binding);
 const select=topic=>{
  const {C,u}=binding;
  if(!topics.some(item=>item.id===topic)||session(C).topic===topic)return;
  session(C).topic=topic;
  const body=u.$('[data-settings-help-content]',host);if(body)body.innerHTML=content(C,u);
  u.$$('[data-settings-topic]',host).forEach(node=>node.setAttribute('aria-pressed',String(node.dataset.settingsTopic===topic)));
 };
 host.addEventListener('click',event=>{
  const button=event.target?.closest?.('[data-settings-topic]');
  if(button){select(button.dataset.settingsTopic);return;}
  select(topicFor(event.target,binding.C,binding.u,host));
 });
 host.addEventListener('focusin',event=>select(topicFor(event.target,binding.C,binding.u,host)));
}
window.SettingsPaneUI={active,canToggle,render,renderProfile,handleProfile,init};
})();
