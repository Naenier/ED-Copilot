/* The rail selects a workspace; the sidebar contains only that workspace. */
(()=>{'use strict';
const groups=[
 {id:'work',title:'Рабочее место',icon:'house',tabs:['overview','targets','statistics','notifications','second-screen']},
 {id:'trade',title:'Торговля',icon:'arrow-left-right',tabs:['deals','sale','carrier-trade'],default:'deals'},
 {id:'colonization',title:'Колонизация',icon:'orbit',tabs:['colonization'],modes:[['Обзор','house'],['Выбор системы','search'],['Строительство','wrench'],['Журнал снабжения','package'],['Бюджет','wallet'],['Динамика системы','chart-no-axes-combined']]},
 {id:'mining',title:'Добыча',icon:'pickaxe',tabs:['mining'],modes:[['Текущий заход','pickaxe'],['История','history'],['Сравнение заходов','chart-no-axes-combined'],['Снаряжение','package'],['План','target']]},
 {id:'engineering',title:'Инженерия',icon:'wrench',tabs:['engineering'],modes:[['Прокачка','wrench'],['План','target'],['Поездка к инженерам','route'],['Оснащение корабля','ship'],['Снаряжение Odyssey','shield-check'],['Материалы','package'],['Инженеры','users']]},
 {id:'exobiology',title:'Экзобиология',icon:'leaf',tabs:['exobiology'],modes:[['Обзор','house'],['Планеты','orbit'],['Образцы','leaf'],['Радар поверхности','compass'],['Продажи','wallet'],['Находки','target'],['Места','map-pin'],['Статистика','chart-no-axes-combined']]},
 {id:'fleet',title:'Флотоносцы',icon:'ship',tabs:['carrier','route','carrier-calculator']},
 {id:'galaxy',title:'Галактика',icon:'orbit',tabs:['politics','riches','ship-route','exploration']},
 {id:'knowledge',title:'Библиотека',icon:'book-open',tabs:['encyclopedia','learning','help']},
 {id:'commander',title:'Командир',icon:'users',tabs:['commander','ranks']},
];
const commanderModes=[['Сводка','house'],['Ранги','shield-check'],['Финансы','wallet'],['Инвентарь','package'],['Имущество','layout-grid'],['Флот','ship'],['Статистика','chart-no-axes-combined'],['Журнал','history']];
function groupFor(tab,mode){return tab==='encyclopedia'&&mode==='Системы'?'galaxy':groups.find(group=>group.tabs.includes(tab))?.id||null;}
function group(id){return groups.find(item=>item.id===id)||groups[0];}
function entries(C,id){
 if(id==='commander')return commanderModes.map(([title,icon])=>({tab:title==='Ранги'?'ranks':'commander',mode:title==='Ранги'?'':title,title,icon}));
 if(id==='galaxy'){
  const politics=window.PoliticsUI?.navigationModes?.(C)||[['Политика','orbit'],['Цели сообщества','target'],['Пауэрплей','shield-check']].map(([title,icon])=>({tab:'politics',mode:title,title,icon}));
  const cartography=C.tabs.find(tab=>tab.ID==='riches');
  const riches=cartography?[{tab:cartography.ID,mode:'',title:cartography.Title,icon:'route'}]:[];
  const route=C.tabs.find(tab=>tab.ID==='ship-route');
  const exploration=C.tabs.some(tab=>tab.ID==='exploration')?[['Экспедиции','route'],['Находки','target'],['Обследования','layout-grid']].map(([title,icon])=>({tab:'exploration',mode:title,title,icon})):[];
  return [{tab:'encyclopedia',mode:'Системы',title:'Системы',icon:'compass'},...riches,...(route?[{tab:route.ID,mode:'',title:route.Title,icon:'route'}]:[]),...exploration,...politics];
 }
 const g=group(id);
 if(g.modes)return g.modes.map(([title,icon])=>({tab:g.tabs[0],mode:title,title,icon}));
 if(id==='trade')return g.tabs.filter(tab=>C.tabs.some(item=>item.ID===tab)).map(tab=>({tab,mode:'',title:C.tabs.find(item=>item.ID===tab).Title,icon:null}));
 return group(id).tabs.filter(tab=>C.tabs.some(item=>item.ID===tab)).sort((a,b)=>{const rank=x=>C.nav.includes(x)?C.nav.indexOf(x):C.nav.length+group(id).tabs.indexOf(x);return rank(a)-rank(b);}).map(tab=>({tab,mode:'',title:C.tabs.find(item=>item.ID===tab)?.Title||tab,icon:null}));
}
function selected(C,item){return C.tab===item.tab&&(!item.mode||C.mode===item.mode||!C.mode&&entries(C,groupFor(C.tab))[0]?.mode===item.mode);}
function usesSidebarModes(tab){const g=group(groupFor(tab));return !!g.modes||['galaxy','commander'].includes(g.id);}
function rail(C,u){return groups.filter(item=>item.id!=='commander').map(item=>'<button type="button" class="icon-button workspace-group '+(C.group===item.id&&C.tab!=='settings'?'selected':'')+'" data-group="'+item.id+'" title="'+u.esc(u.t(item.title))+'" aria-label="'+u.esc(u.t(item.title))+'" aria-pressed="'+(C.group===item.id&&C.tab!=='settings')+'">'+u.icon(item.icon)+'</button>').join('');}
function sidebar(C,u){
 const g=group(C.group),all=entries(C,g.id),visible=all.filter(item=>g.modes||['commander','trade','galaxy'].includes(g.id)||C.nav.includes(item.tab)||item.tab===C.tab||['help','notifications','second-screen'].includes(item.tab));
 const hidden=all.filter(item=>!visible.includes(item));
 const row=item=>'<button type="button" class="nav-item '+(selected(C,item)?'selected':'')+'" data-nav="'+u.esc(item.tab)+'" '+(item.mode?'data-nav-mode="'+u.esc(item.mode)+'"':'data-order="'+u.esc(item.tab)+'" draggable="true"')+' '+(selected(C,item)?'aria-current="page"':'')+'>'+u.icon(item.icon||u.tabIcons?.[item.tab]||'circle')+'<span>'+u.esc(u.t(item.title))+'</span></button>';
 return '<div class="nav-label">'+u.esc(u.t(g.title))+'</div>'+visible.map(row).join('')+(hidden.length?'<details class="nav-additional"><summary>'+u.esc(u.t('Другие разделы группы'))+'<span>'+hidden.length+'</span></summary>'+hidden.map(row).join('')+'</details>':'');
}
window.NavigationUI={groups,commanderModes,groupFor,group,entries,selected,usesSidebarModes,rail,sidebar};
})();
