/* The rail selects a workspace; the sidebar contains only that workspace. */
(()=>{'use strict';
const groups=[
 {id:'work',title:'Рабочее место',icon:'house',tabs:['overview','targets','statistics']},
 {id:'trade',title:'Торговля',icon:'arrow-left-right',tabs:['deals','sale','carrier-trade'],default:'deals'},
 {id:'colonization',title:'Колонизация',icon:'orbit',tabs:['colonization'],modes:[['Обзор','house'],['Выбор системы','search'],['Строительство','wrench'],['Снабжение','package'],['Бюджет','wallet']]},
 {id:'mining',title:'Добыча',icon:'pickaxe',tabs:['mining'],modes:[['Текущий заход','pickaxe'],['История','history'],['Снаряжение','package'],['План','target']]},
 {id:'engineering',title:'Инженерия',icon:'wrench',tabs:['engineering'],modes:[['План','target'],['Материалы','package'],['Обмен','arrow-left-right'],['Инженеры','users']]},
 {id:'exobiology',title:'Экзобиология',icon:'leaf',tabs:['exobiology'],modes:[['Обзор','house'],['Планеты','orbit'],['Образцы','leaf'],['Продажи','wallet'],['Находки','target'],['Места','map-pin'],['Статистика','chart-no-axes-combined']]},
 {id:'fleet',title:'Флотоносцы',icon:'ship',tabs:['carrier','route','carrier-calculator']},
 {id:'galaxy',title:'Галактика',icon:'orbit',tabs:['politics']},
 {id:'knowledge',title:'Библиотека',icon:'book-open',tabs:['encyclopedia','learning','help']},
 {id:'commander',title:'Командир',icon:'users',tabs:['commander','ranks']},
];
const commanderModes=[['Сводка','house'],['Ранги','shield-check'],['Финансы','wallet'],['Инвентарь','package'],['Имущество','layout-grid'],['Флот','ship'],['Статистика','chart-no-axes-combined'],['Журнал','history']];
function groupFor(tab){return groups.find(group=>group.tabs.includes(tab))?.id||null;}
function group(id){return groups.find(item=>item.id===id)||groups[0];}
function entries(C,id){
 if(id==='commander')return commanderModes.map(([title,icon])=>({tab:title==='Ранги'?'ranks':'commander',mode:title==='Ранги'?'':title,title,icon}));
 if(id==='galaxy')return window.PoliticsUI?.navigationModes?.(C)||[['Политика','orbit'],['Цели сообщества','target'],['Пауэрплей','shield-check']].map(([title,icon])=>({tab:'politics',mode:title,title,icon}));
 const g=group(id);
 if(g.modes)return g.modes.map(([title,icon])=>({tab:g.tabs[0],mode:title,title,icon}));
 if(id==='trade')return g.tabs.filter(tab=>C.tabs.some(item=>item.ID===tab)).map(tab=>({tab,mode:'',title:C.tabs.find(item=>item.ID===tab).Title,icon:null}));
 return group(id).tabs.filter(tab=>C.tabs.some(item=>item.ID===tab)).sort((a,b)=>{const rank=x=>C.nav.includes(x)?C.nav.indexOf(x):C.nav.length+group(id).tabs.indexOf(x);return rank(a)-rank(b);}).map(tab=>({tab,mode:'',title:C.tabs.find(item=>item.ID===tab)?.Title||tab,icon:null}));
}
function selected(C,item){return C.tab===item.tab&&(!item.mode||C.mode===item.mode||!C.mode&&entries(C,groupFor(C.tab))[0]?.mode===item.mode);}
function usesSidebarModes(tab){const g=group(groupFor(tab));return !!g.modes||['galaxy','commander'].includes(g.id);}
function rail(C,u){return groups.filter(item=>item.id!=='commander').map(item=>'<button type="button" class="icon-button workspace-group '+(C.group===item.id&&C.tab!=='settings'?'selected':'')+'" data-group="'+item.id+'" title="'+u.esc(u.t(item.title))+'" aria-label="'+u.esc(u.t(item.title))+'" aria-pressed="'+(C.group===item.id&&C.tab!=='settings')+'">'+u.icon(item.icon)+'</button>').join('');}
function sidebar(C,u){
 const g=group(C.group),all=entries(C,g.id),visible=all.filter(item=>g.modes||['commander','trade','galaxy'].includes(g.id)||C.nav.includes(item.tab)||item.tab===C.tab||item.tab==='help');
 const hidden=all.filter(item=>!visible.includes(item));
 const row=item=>'<button type="button" class="nav-item '+(selected(C,item)?'selected':'')+'" data-nav="'+u.esc(item.tab)+'" '+(item.mode?'data-nav-mode="'+u.esc(item.mode)+'"':'data-order="'+u.esc(item.tab)+'" draggable="true"')+' '+(selected(C,item)?'aria-current="page"':'')+'>'+u.icon(item.icon||u.tabIcons?.[item.tab]||'circle')+'<span>'+u.esc(u.t(item.title))+'</span></button>';
 return '<div class="nav-label">'+u.esc(u.t(g.title))+'</div>'+visible.map(row).join('')+(hidden.length?'<details class="nav-additional"><summary>'+u.esc(u.t('Другие разделы группы'))+'<span>'+hidden.length+'</span></summary>'+hidden.map(row).join('')+'</details>':'');
}
window.NavigationUI={groups,commanderModes,groupFor,group,entries,selected,usesSidebarModes,rail,sidebar};
})();
