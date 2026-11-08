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
function text(paragraphs,u){return paragraphs.map(value=>'<p>'+u.esc(value)+'</p>').join('');}
function content(C,u){
 const state=C.state||{},settings=state.settings||{},desktop=state.desktop||{},connection=state.connection||{},theme=state.theme||{};
 const topic=session(C).topic,mode={windowed:t("В окне"),fullscreen:t("На весь экран"),borderless:t("Окно без рамки")}[desktop.displayMode]||t("В окне");
 let body='';
 if(topic==='workspace')body=text([
  t("Оставьте в основном меню разделы, которыми пользуетесь чаще. Через «Настроить» можно добавить остальные и изменить их порядок; скрытый раздел остаётся доступен через поиск Ctrl+K."),
  t("Панель подробностей раскрывает выбранную строку таблицы. В настройках она объясняет выделенный блок. На небольшом окне панель открывается поверх содержимого; крестик возвращает всю ширину странице."),
 ],u)+facts([[t("Отображение"),mode],[t("Быстрый переход"),'Ctrl + K'],[t("Полноэкранный режим"),'F11']],u);
 if(topic==='overlay'){
  const activity=(C.boot?.activities||[]).find(item=>item.Value===settings.activity)?.Label||t("Выберите занятие в панели над разделом");
  body=text([
   t("Оверлей появляется только при активном окне Elite Dangerous. Когда вы переходите в Copilot или другое приложение, игровые панели скрываются."),
   t("Включите оверлей, затем выберите занятие над рабочей страницей. Состав панелей изменится под торговлю, добычу, маршрут или снабжение колонии. Для колонизации отдельно выбирается нужная колония."),
  ],u)+facts([[t("Панели в игре"),settings.overlayEnabled?t("Включены"):t("Выключены")],[t("Занятие"),activity],[t("Переключение"),'Ctrl + Alt + H']],u);
 }
 if(topic==='theme'){
  body=text([
   t("Готовая палитра меняет акцентные цвета. Матрица RGB позволяет приблизить их к вашему HUD. «Скопировать палитру из игры» сначала показывает предварительный результат; цвета сохранятся после применения."),
   t("Флажок «Игровая палитра в основном окне» переносит игровые цвета фона и текста в Copilot. Если его снять, основное окно использует спокойный фон, а выбранные цвета оверлея сохраняются."),
   t("Импорт читает конфигурацию интерфейса игры и не изменяет её файлы."),
  ],u)+facts([[t("Основное окно"),desktop.useGamePalette?t("Игровая палитра"):t("Стандартный фон")],[t("Палитра"),theme.settings?.mode==='game'?t("Из игры"):theme.settings?.mode==='matrix'?t("Матрица RGB"):theme.settings?.mode==='custom'?t("Пользовательская"):(theme.presets||[]).find(item=>item.id===theme.settings?.preset)?.name||t("Пользовательская")]],u);
  const colors=[['accent',t("Акцент")],['text',t("Текст")],['panel',t("Панель")]].filter(([key])=>/^#[0-9a-f]{6}$/i.test(theme.palette?.[key]||''));
  if(colors.length)body+='<div class="settings-help-colors">'+colors.map(([key,label])=>'<span><i style="background:'+theme.palette[key]+'"></i>'+u.esc(t(label))+'<code>'+u.esc(theme.palette[key])+'</code></span>').join('')+'</div>';
 }
 if(topic==='journal'){
  const status=connection.error?t("Журнал недоступен"):connection.scanning?t("Читаем журналы"):connection.lastUpdate||Number(connection.eventCount)>0?t("Журнал подключён"):connection.journalDir?t("Ожидаем события игры"):t("Папка не выбрана");
  body=text([
   t("Copilot читает журналы Elite Dangerous: положение, груз, материалы, сделки и игровые события. Выберите папку журналов, если автоматическое обнаружение не нашло её."),
   t("«Перечитать журналы» заново собирает данные из уже записанных событий. Это полезно после переноса журналов или восстановления подключения. Ваши личные планы сохраняются в профиле отдельно от файлов приложения."),
  ],u)+facts([[t("Подключение"),status],[t("Прочитано событий"),Number.isFinite(Number(connection.eventCount))&&connection.eventCount!==null&&connection.eventCount!==''?u.fmt(Number(connection.eventCount)):t("Нет данных")],[t("Папка"),connection.journalDir||t("Не выбрана")]],u);
  if(connection.error)body+='<p class="settings-help-warning">'+u.esc(connection.error)+'</p>';
 }
 if(topic==='mining'){
  const tier={low:t("Любая"),medium:t("Средняя и выше"),high:t("Высокая")}[settings.minTier]||t("Не задана");
  const method={all:t("Любой"),laser:t("Лазер"),core:t("Ядра"),subsurface:t("Подповерхностная"),surface:t("Поверхность астероида"),'surface-planet':t("Планетарная"),raw:t("Сырьё")}[settings.method]||t("Не задан");
  body=text([
   t("Начните с минимальной ценности и способа добычи. Затем ограничьте радиус поиска и путь от звезды: богатое кольцо может оказаться невыгодным из-за долгого перелёта."),
   t("Порог содержания и скрытие истощённых колец относятся к лазерной добыче. Для ядерной, подповерхностной и планетарной добычи используйте соответствующий способ и сравнивайте доступность нужного ресурса."),
   t("Фильтры помогают отобрать места. До вылета проверьте оснащение корабля, запас дронов и рынок сбыта в разделе торговли."),
  ],u)+facts([[t("Минимальная ценность"),tier],[t("Способ добычи"),method]],u);
 }
 if(topic==='prices')body=text([
  t("Справочная цена — необязательный личный ориентир в кредитах за тонну. Она нужна для приблизительной оценки груза и планов, когда свежая рыночная котировка ещё не найдена."),
  t("Выберите товар и задайте свою оценку. Пустое значение возвращает последнюю известную цену из игры. Ноль означает нулевую оценку, поэтому для сброса оставьте поле пустым."),
  t("Цена продажи на станции или флотоносце берётся из результатов поиска рынков. Справочный ориентир не меняет эти котировки и историю совершённых сделок. Перед рейсом сравните спрос, расстояние, площадку и давность цены."),
 ],u);
 return '<h3>'+u.esc(t(topics.find(item=>item.id===topic).title))+'</h3>'+body;
}
function pane(C,u){
 return `<aside class="inspector settings-help" aria-label="${th("Подробности настроек")}"><div class="inspector-title"><span>`+u.icon('circle-help')+`${th("Подробности настроек")}</span>`+u.btn('','inspector','x','quiet settings-help-close',`title="${th("Скрыть подробности")}" aria-label="${th("Скрыть подробности")}"`)+`</div><p class="settings-help-intro">${th("Выберите тему или перейдите к настройке — пояснение появится здесь.")}</p><nav class="settings-help-topics" aria-label="${th("Темы настроек")}">`+topics.map(topic=>'<button data-settings-topic="'+topic.id+'" aria-pressed="'+(session(C).topic===topic.id)+'">'+u.icon(topic.icon)+u.esc(t(topic.title))+'</button>').join('')+'</nav><div data-settings-help-content aria-live="polite">'+content(C,u)+'</div></aside>';
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
