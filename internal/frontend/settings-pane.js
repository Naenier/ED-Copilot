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
function canToggle(kind){return kind==='table'||kind==='settings';}
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
 if(!active(C))return mainHTML;
 return '<div class="settings-workspace'+(C.inspector?' has-help':'')+'" data-settings-workspace><div class="settings-main">'+mainHTML+'</div>'+(C.inspector?pane(C,u):'')+'</div>';
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
window.SettingsPaneUI={active,canToggle,render,init};
})();
