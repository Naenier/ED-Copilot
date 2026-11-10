(()=>{
'use strict';
const t=(value,vars)=>window.CopilotI18n?.t(value,vars)??String(value??'');
const th=(value,vars)=>t(value,vars).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const $=(s,r=document)=>r.querySelector(s), $$=(s,r=document)=>[...r.querySelectorAll(s)];
const esc=v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const icon=name=>(window.CopilotIcons?.[name]||window.CopilotIcons?.circle||'<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8"/></svg>').replace('<svg','<svg class="icon" aria-hidden="true"');
const locale=()=>window.CopilotI18n?.locale()||'ru-RU';
const fmt=v=>window.CopilotI18n?window.CopilotI18n.number(v,{maximumFractionDigits:2}):v===null||v===undefined||v===''?'—':new Intl.NumberFormat(locale(),{maximumFractionDigits:2}).format(Number(v));
const money=v=>v===null||v===undefined?'—':(window.CopilotI18n?window.CopilotI18n.number(v,{notation:'compact',maximumFractionDigits:1}):new Intl.NumberFormat(locale(),{notation:'compact',maximumFractionDigits:1}).format(Number(v)))+t(" кр.");
const tabIcons={commander:'users',ranks:'shield-check',politics:'users',exobiology:'leaf',overview:'house',targets:'target',sale:'arrow-left-right',deals:'route',carrier:'ship','carrier-trade':'arrow-left-right',route:'route',mining:'pickaxe',engineering:'wrench',colonization:'orbit',encyclopedia:'book-open',learning:'graduation-cap','carrier-calculator':'calculator',statistics:'chart-no-axes-combined',settings:'settings',account:'shield-check',integrations:'cloud-check',help:'circle-help'};
let sequence=0,viewEpoch=0;const pending=new Map();
const storage={get(key){try{return localStorage.getItem(key);}catch{return null;}},set(key,value){try{localStorage.setItem(key,value);}catch{}}};
const C={t,locale,tab:'overview',group:'work',groupLast:{},mode:'',query:'',selected:'',page:null,actions:[],state:{},tabs:[],nav:[],prefill:{},boot:null,collapsed:storage.get('sidebar-collapsed')==='true',inspector:storage.get('inspector')!=='false',focus:false,lesson:'',course:'',layer:false,lastFetch:0,lastSignature:'',lastRevision:null};
function currentLanguage(){return C.state.settings?.language||C.state.app?.language||'en';}
function captureInputs(){return $$('#page input,#page select,#page textarea').map(node=>({
 id:node.id,name:node.name,tag:node.tagName,type:node.type,value:node.value,checked:node.checked,
 data:Object.entries(node.dataset||{}).filter(([key])=>!['click','nav','mode'].includes(key)),
}));}
function restoreInputs(saved){const nodes=$$('#page input,#page select,#page textarea');for(const item of saved){
 const node=item.id?document.getElementById(item.id):nodes.find(node=>node.tagName===item.tag&&node.name===item.name&&node.type===item.type&&item.data.length&&item.data.every(([key,value])=>node.dataset?.[key]===value));
 if(!node||node.type!==item.type)continue;
 if(node.tagName!=='SELECT'||[...(node.options||[])].some(option=>option.value===item.value))node.value=item.value;
 if(item.type==='checkbox'||item.type==='radio')node.checked=item.checked;
}}
function applyLanguage(rebuild=false){
 const desired=currentLanguage(),i18n=window.CopilotI18n;
 if(!i18n||i18n.normalize(desired)===i18n.language)return false;
 if(C.layer){C.pendingLanguage=desired;return false;}
 const saved=rebuild&&C.page?captureView():null,drafts=saved?captureInputs():null,disclosures=saved?captureDisclosures():null;
 i18n.setLanguage(desired);delete C.pendingLanguage;
 // Lazy catalogue responses are language-specific; preserve query, selection and history.
 for(const session of Object.values(C.referenceSessions||{})){if(session===C.reference)session.localeScroll=$('#page-scroll')?.scrollTop||0;session.loadedKey='';session.requestKey='';session.listToken++;session.entryToken++;session.list=null;session.entry=null;session.listPromise=null;session.entryPromise=null;session.error='';session.entryError='';session.loading=false;session.entryLoading=false;}
 if(rebuild&&C.page){shell();renderPage();restoreInputs(drafts);restoreDisclosures(disclosures);restoreView(saved);}
 return true;
}
window.copilotReceive=message=>{
 if(message.event==='notice'){toast(message.data?.message||t("Не удалось сохранить изменение."),true);return;}
 if(message.event==='response'){const entry=pending.get(message.data.id);if(entry){pending.delete(message.data.id);clearTimeout(entry.timeout);message.data.error?entry.reject(new Error(message.data.error)):entry.resolve(message.data.result);}return;}
 if(message.event==='update'){const update=message.data;const changed=JSON.stringify(update.settings)!==JSON.stringify(C.state.settings),previousStatus=connectionStatus(),oldShip=shipLabel(),oldSystem=stateCmd().system,selectorChanged=updateOverlayColonies(update.overlayColonies,update.revision);C.state.settings=update.settings;C.state.connection=update.connection;C.state.commander=update.commander;applyLanguage(true);if(C.page&&!C.layer){if(changed||previousStatus!==connectionStatus()||oldShip!==shipLabel()||oldSystem!==stateCmd().system)renderChrome();else if(selectorChanged)renderActivity();}if(update.revision!==C.lastRevision){C.lastRevision=update.revision;C.needsRefresh=true;}}
};
function call(op,params={}){return new Promise((resolve,reject)=>{const id=String(++sequence);const timeout=setTimeout(()=>{pending.delete(id);reject(new Error(t("Ответ приложения задерживается. Повторите действие.")));},45000);pending.set(id,{resolve,reject,timeout});window.chrome.webview.postMessage(JSON.stringify({id,op,params}));});}
function toast(text,error=false){const item=document.createElement('div');item.className='toast';item.textContent=text;if(error)item.style.borderColor='#79504c';$('#toasts').append(item);setTimeout(()=>item.remove(),5500);}
function navTitle(id){return t(C.tabs.find(item=>item.ID===id)?.Title||'ED Copilot');}
function context(){return {tab:C.tab,mode:C.mode,query:C.query,selected:C.tab==='learning'&&C.lesson?C.lesson:C.selected,prefill:C.prefill};}
function applyPalette(){
 const p=C.state.theme?.palette||{},game=!!C.state.desktop?.useGamePalette,style=document.documentElement.style;
 const safe=value=>/^#[0-9a-f]{6}$/i.test(value||'')?value:null;
 for(const [role,key] of Object.entries({accent:'accent',good:'good',warn:'warn',danger:'danger'})){const value=safe(p[key]);value?style.setProperty('--'+role,value):style.removeProperty('--'+role);}
 const roles={app:'bg',sidebar:'panel',bg:'bg',panel:'panel',raised:'panelRaised',hover:'panelRaised',border:'line',line:'line',text:'text',muted:'muted',faint:'faint','accent-bg':'accentDim'};
 for(const [role,key] of Object.entries(roles)){const value=game?safe(p[key]):null;value?style.setProperty('--'+role,value):style.removeProperty('--'+role);}
 document.documentElement.classList.toggle('game-palette',game);
}
function stateCmd(){return C.state.commander||{};}
function connectionStatus(){const connection=C.state.connection||{};if(connection.error)return t("Журнал недоступен");if(connection.scanning)return t("Читаем журналы");if(connection.lastUpdate||Number(connection.eventCount)>0)return t("Журнал подключён");return connection.journalDir?t("Ожидаем события игры"):t("Журнал не настроен");}
function shipLabel(){const c=stateCmd();return [c.ship,c.shipName?'«'+c.shipName+'»':'',c.shipIdent].filter(Boolean).join(' · ')||t("Ожидание корабля");}
function btn(label,action,ico='',style='',attrs=''){return `<button ${/\btype\s*=/.test(attrs)?'':'type="button"'} class="pill ${style}" data-click="${action}" ${attrs}>${ico?icon(ico):''}${esc(t(label))}</button>`;}
function iconBtn(name,action,title,attrs=''){return `<button type="button" class="icon-button" data-click="${action}" title="${esc(t(title))}" aria-label="${esc(t(title))}" ${attrs}>${icon(name)}</button>`;}
function badge(text,kind=''){return `<span class="badge ${kind}">${esc(t(text))}</span>`;}
function metric(label,value,foot,ico=''){return `<div class="metric"><div class="metric-label">${ico?icon(ico):''}${esc(label)}</div><div class="metric-value">${esc(value)}</div><div class="metric-foot">${esc(foot||'')}</div></div>`;}
function shell(){
 C.shellLanguage=window.CopilotI18n?.language||currentLanguage();
 const cmd=stateCmd();const initial=(cmd.name||'CMDR').slice(0,2).toUpperCase();
 $('#app').innerHTML=`<header class="titlebar" data-drag><button class="icon-button" data-click="back" title="${th("Назад")}">${icon('arrow-left')}</button>${iconBtn('panel-left','sidebar',t("Показать или скрыть меню"))}<span class="brand-name">ED Copilot</span><button class="menu-text" data-click="file-menu">${th("Файл")}</button><button class="menu-text" data-click="view-menu">${th("Вид")}</button><button class="menu-text" data-click="navigate-help">${th("Справка")}</button><div class="spacer" data-drag></div><div class="window-actions"><button class="caption" data-window="minimize" data-tooltip="${th("Свернуть")}" aria-label="${th("Свернуть")}">${icon('minus')}</button><button class="caption" data-window="maximize" data-tooltip="${th("Развернуть или восстановить")}" aria-label="${th("Развернуть или восстановить")}">${icon('maximize-2')}</button><button class="caption close" data-window="hide" data-tooltip="${th("Закрыть в область уведомлений")}" aria-label="${th("Закрыть в область уведомлений")}">${icon('x')}</button></div></header><div class="app-layout"><nav class="rail" aria-label="${th("Группы разделов")}"><button class="icon-button logo" data-group="work" title="${th("Рабочее место")}" aria-label="${th("Рабочее место")}"><img class="brand-mark" src="${window.CopilotBrand||''}" alt=""></button>${iconBtn('search','command',t("Поиск разделов · Ctrl+K"))}<div class="rail-line"></div><div class="rail-groups" id="rail-groups"></div><div class="spacer"></div><span class="connection-dot" id="journal-indicator" role="img" tabindex="0"></span><div id="rail-overlay"></div>${iconBtn('settings','settings-nav',t("Настройки"))}<button class="icon-button avatar" data-nav="commander" title="${th("Командир")}" aria-label="${th("Командир")}">${esc(initial)}</button></nav><div class="workspace"><aside class="sidebar ${C.collapsed?'collapsed':''}" aria-label="${th("Разделы выбранной группы")}"><div class="sidebar-head"><strong id="sidebar-group-title">ED Copilot</strong>${iconBtn('ellipsis','customize',t("Настроить меню"))}</div><div class="sidebar-scroll" id="navigation"></div><div class="sidebar-footer"><div class="commander-mini"><div class="ship-dot">${icon('ship')}</div><div class="ellipsis"><strong id="sidebar-ship">${esc(cmd.ship||t("Ваш корабль"))}</strong><span id="sidebar-system">${esc(cmd.system||t("Ожидание журнала"))}</span></div></div><div id="sidebar-activity"></div></div></aside><main class="main"><div class="page-toolbar" id="page-toolbar"></div><div class="page-scroll" id="page-scroll"><div class="content-wrap" id="page"></div></div></main></div></div>`;
 C.lastNavigationSignature='';renderNavigation();renderChrome();bindDragNav();
}
function renderNavigation(){
 const navigation=window.NavigationUI;if(!navigation)return;
 const signature=JSON.stringify([C.group,C.tab,C.mode,C.nav,C.tabs,currentLanguage(),navigation.entries(C,C.group).map(item=>item.mode)]);if(C.lastNavigationSignature===signature)return;
 const node=$('#navigation');if(!node)return;
 C.lastNavigationSignature=signature;
 node.innerHTML=navigation.sidebar(C,{t,esc,icon,tabIcons});
 const rail=$('#rail-groups');if(rail)rail.innerHTML=navigation.rail(C,{t,esc,icon});
 const title=$('#sidebar-group-title');if(title)title.textContent=t(navigation.group(C.group).title);
 $('.rail .avatar')?.classList.toggle('selected',C.group==='commander'&&C.tab!=='settings');$('.rail [data-click="settings-nav"]')?.classList.toggle('selected',C.tab==='settings');
}
async function selectGroup(id){
 const navigation=window.NavigationUI;if(!navigation?.groups.some(group=>group.id===id))return;
 const previous=C.group;C.group=id;C.focus=false;document.querySelector('.workspace')?.classList.remove('reading-focus');C.collapsed=false;storage.set('sidebar-collapsed',false);$('.sidebar')?.classList.remove('collapsed');
 renderNavigation();
 if(previous!==id||navigation.groupFor(C.tab,C.mode)!==id){
  const entries=navigation.entries(C,id),remembered=C.groupLast?.[id],item=entries.find(item=>item.tab===remembered?.tab&&item.mode===(remembered?.mode||''))||(id==='trade'?entries[0]:entries.find(item=>C.nav.includes(item.tab)))||entries[0];
  if(item)await navigate(item.tab,{mode:item.mode});
 }
 await call('desktop',{...context(),values:{sidebarCollapsed:false}});
}
function rendererKind(){if(C.tab==='help'||C.tab==='statistics'&&C.mode==='Торговля')return 'custom';if(C.tab==='commander')return 'commander';if(C.tab==='exobiology')return 'exobiology';if(referenceActive())return 'reference';if(window.EngineeringRoadmapUI?.active(C))return 'engineering-roadmap';if(C.tab==='overview'&&(!C.mode||C.mode==="Обзор"))return 'overview';if(['learning','carrier-calculator','carrier-trade','deals','ranks','politics'].includes(C.tab))return C.tab;return C.tab==='settings'?'settings':'table';}
function breadcrumbHtml(){
 const group=C.tab==='settings'?t('Приложение'):t(window.NavigationUI?.group(C.group)?.title||'Рабочее место');
 const names=[group,C.tab==='encyclopedia'&&C.mode==='Системы'?t(C.mode):navTitle(C.tab)];
 if(C.tab==='commander'&&C.mode&&C.mode!=='Сводка')names.push(t(C.mode));
 if(C.mode&&window.NavigationUI?.usesSidebarModes(C.tab)&&C.tab!=='commander')names.push(t(C.mode));
 if(C.tab==='learning'&&C.lesson)names.push(t('Урок'));
 const path=names.filter((value,index,all)=>index===0||value!==all[index-1]);
 return path.map((value,index)=>(index?icon('chevron-right'):'')+'<span class="'+(index===path.length-1?'current':'')+'">'+esc(value)+'</span>').join('');
}
function renderChrome(){
 const toolbar=$('#page-toolbar');if(!toolbar){renderActivity();return;}
 const markup=`<div class="breadcrumb">${breadcrumbHtml()}</div><div class="spacer"></div>${C.tab!=='help'?iconBtn('circle-help','help',t("Как пользоваться разделом")):''}${!colonyBudgetActive()&&window.SettingsPaneUI?.canToggle(rendererKind())&&(C.tab==='settings'||C.page?.Rows?.length)&&!(C.tab==='colonization'&&(!C.mode||['Обзор','Журнал снабжения'].includes(C.mode)))?iconBtn('panel-left','inspector',t("Показать или скрыть подробности")):''}`;
 if(toolbar.copilotMarkup!==markup){toolbar.innerHTML=markup;toolbar.copilotMarkup=markup;}
 const journal=$('#journal-indicator'), status=connectionStatus(), connection=C.state.connection||{};
 if(journal){journal.setAttribute('aria-label',status);journal.setAttribute('data-tooltip',status);journal.dataset.status=connection.error?'error':connection.scanning?'scanning':connection.lastUpdate||Number(connection.eventCount)>0?'connected':'waiting';}
 const overlay=$('#rail-overlay'),enabled=!!C.state.settings?.overlayEnabled,overlayLabel=enabled?t("Выключить оверлей"):t("Включить оверлей");
 if(overlay&&overlay.copilotEnabled!==enabled){overlay.innerHTML=iconBtn(enabled?'eye':'eye-off','overlay',overlayLabel,`aria-pressed="${enabled}"`);overlay.copilotEnabled=enabled;}
 const ship=$('#sidebar-ship'),system=$('#sidebar-system');
 if(ship)ship.textContent=stateCmd().ship?shipLabel():t("Ваш корабль");
 if(system)system.textContent=stateCmd().system||t("Ожидание журнала");
 const avatar=$('.rail .avatar');if(avatar)avatar.textContent=(stateCmd().name||'CMDR').slice(0,2).toUpperCase();renderActivity();
}
function selectorRevision(value){return Number.isSafeInteger(value)&&value>=0?value:-1;}
function colonySelectorSource(){return C.overlayColonies&&selectorRevision(C.overlayColoniesRevision)>=0&&selectorRevision(C.overlayColoniesRevision)>=selectorRevision(C.state.revision)?C.overlayColonies:C.state.colonization||{};}
function updateOverlayColonies(value,revision){
 if(!value||!Array.isArray(value.projects)||selectorRevision(revision)<0||revision<Math.max(selectorRevision(C.overlayColoniesRevision),selectorRevision(C.state.revision)))return false;
 const signature=()=>{const model=colonySelectorModel();return JSON.stringify([model.selected,model.known,model.projects.map(item=>[item.id,item.name,item.targetSystem,item.status])]);},before=signature();
 const string=value=>typeof value==='string'?value:'';
 C.overlayColonies={projects:value.projects.filter(item=>item&&typeof item.id==='string').map(item=>({id:item.id,name:string(item.name),targetSystem:string(item.targetSystem),status:string(item.status)})),selectedProjectId:string(value.selectedProjectId),activeProject:{id:string(value.activeProject?.id)}};
 C.overlayColoniesRevision=revision;
 return signature()!==before;
}
function colonySelectorModel(){
 const colony=colonySelectorSource(),seen=new Set(),raw=Array.isArray(colony.projects)?colony.projects:colony.activeProject?.id?[colony.activeProject]:[];
 const projects=raw.filter(project=>{if(!project||typeof project.id!=='string'||!project.id.trim()||seen.has(project.id))return false;seen.add(project.id);return true;});
 const valid=id=>typeof id==='string'&&projects.some(project=>project.id===id),choice=C.colonySelection;
 if(choice&&!choice.pending&&colony.selectedProjectId===choice.id)delete C.colonySelection;
 const selected=valid(C.colonySelection?.id)?C.colonySelection.id:valid(colony.selectedProjectId)?colony.selectedProjectId:valid(colony.activeProject?.id)?colony.activeProject.id:projects.length===1?projects[0].id:'';
 return {projects,selected,known:Array.isArray(colony.projects)||!!colony.activeProject?.id};
}
function activityHtml(){
 const settings=C.state.settings||{};if(!settings.overlayEnabled)return '';
 const activities=(C.boot?.activities||[]).filter(item=>typeof item.Value==='string'&&item.Value&&item.Label),{projects,selected,known}=colonySelectorModel();
 const currentActivity=activities.some(item=>item.Value===settings.activity),options=activities.length?(currentActivity?'':`<option value="${esc(settings.activity||'')}" selected disabled>${th("Выберите занятие")}</option>`)+activities.map(item=>`<option value="${esc(item.Value)}" ${item.Value===settings.activity?'selected':''}>${esc(t(item.Label))}</option>`).join(''):`<option selected>${C.boot?t("Занятия недоступны"):t("Загружаем занятия…")}</option>`;
 const name=project=>{const title=String(project.name||'').trim(),system=String(project.targetSystem||'').trim();return title&&system&&title!==system?title+' · '+system:title||system||t("Колония без названия");};
 let colony='';
 if(settings.activity==='colonization'){
  colony=projects.length?`<label class="activity-field activity-colony"><span>${th("Колония для оверлея")}</span><select id="overlay-colony" aria-label="${th("Колония для оверлея")}" title="${th("Строительство и снабжение выбранной колонии")}">${selected?'':`<option value="" selected disabled>${th("Выберите колонию")}</option>`}${projects.map(project=>`<option value="${esc(project.id)}" ${project.id===selected?'selected':''}>${esc(name(project))}</option>`).join('')}</select></label>`:`<div class="activity-empty" role="status"><span>${C.state.connection?.scanning?t("Читаем журналы — ищем ваши колонии"):known?t("Колонии пока не найдены"):t("Список колоний обновляется")}</span>${btn(t("Открыть колонизацию"),'navigate-colonization','arrow-right','quiet')}</div>`;
 }
 return `<div class="activity-strip" id="activity-strip"><span class="activity-caption">${icon('layers')}${th("Панели в игре")}</span><div class="activity-fields"><label class="activity-field"><select id="activity-select" aria-label="${th("Занятие для оверлея")}" ${activities.length?'':'disabled'}>${options}</select></label>${colony}</div></div>`;
}
function renderActivity(){
 const host=$('#sidebar-activity');if(!host)return;
 if(C.state.settings?.overlayEnabled&&document.activeElement?.closest?.('#activity-strip')){C.activityRefreshPending=true;return;}
 const markup=activityHtml();if(host.copilotMarkup!==markup){host.innerHTML=markup;host.copilotMarkup=markup;}C.activityRefreshPending=false;
}
let colonySelectionSequence=0;
async function selectColony(id){
 const {projects}=colonySelectorModel();if(!projects.some(project=>project.id===id)||colonySelectorSource().selectedProjectId===id&&!C.colonySelection)return;
 const sequence=++colonySelectionSequence;C.colonySelection={id,pending:true,sequence};
 try{await call('select-colony',{...context(),id});if(C.colonySelection?.sequence===sequence)C.colonySelection.pending=false;await loadView();colonySelectorModel();}
 catch(error){if(C.colonySelection?.sequence===sequence){delete C.colonySelection;const confirmed=colonySelectorModel().selected;for(const name of ['overlay-colony','colony-select']){const control=document.getElementById(name);if(control)control.value=confirmed;}}renderActivity();throw error;}
}
function heading(title,description,extra='',search=''){return `<div class="page-heading${search?' search-heading':''}"><div><h1>${esc(title)}</h1>${description?`<p class="description">${esc(description)}</p>`:''}</div>${extra?`<div class="heading-actions">${extra}</div>`:''}${search}</div>`;}
function renderPage(){if(!C.page)return;applyPalette();renderChrome();renderNavigation();const container=$('#page');if(C.tab==='overview'&&(!C.mode||C.mode==="Обзор"))container.innerHTML=renderOverview();else if(C.tab==='commander')container.innerHTML=window.CommanderUI.render(C,uiContext());else if(C.tab==='ranks')container.innerHTML=window.RanksUI.render(C,uiContext());else if(C.tab==='politics')container.innerHTML=window.PoliticsUI.render(C,uiContext());else if(C.tab==='learning')container.innerHTML=renderLearning();else if(C.tab==='carrier-calculator')container.innerHTML=renderCalculator();else if(referenceActive())container.innerHTML=window.ReferenceUI.render(C,uiContext());else if(window.EngineeringRoadmapUI?.active(C))container.innerHTML=window.EngineeringRoadmapUI.render(C,uiContext());else if(C.tab==='carrier-trade')container.innerHTML=window.CarrierTradeUI.render(C,uiContext());else if(C.tab==='deals')container.innerHTML=window.DealsUI.render(C,uiContext());else if(C.tab==='exobiology')container.innerHTML=window.ExobiologyUI.render(C,uiContext());else if(C.tab==='help')container.innerHTML=renderHelp();else if(C.tab==='statistics'&&C.mode==='Торговля'&&window.TradeStatsUI)container.innerHTML=window.TradeStatsUI.render(C,uiContext());else if(C.tab==='settings')container.innerHTML=window.SettingsPaneUI.render(C,uiContext(),renderSettings());else container.innerHTML=renderGeneral();if(C.restoreScroll!==undefined){$('#page-scroll').scrollTop=C.restoreScroll;delete C.restoreScroll;}document.querySelector('.workspace')?.classList.toggle('reading-focus',C.focus&&C.tab==='learning'&&!!C.lesson);window.AcademyUI?.init?.(C,uiContext());window.CalculatorUI?.init?.(C,uiContext());if(referenceActive())window.ReferenceUI?.init?.(C,uiContext());window.EngineeringRoadmapUI?.init(C,uiContext());if(C.tab==='carrier-trade')window.CarrierTradeUI?.init?.(C,uiContext());if(C.tab==='deals')window.DealsUI?.init?.(C,uiContext());if(C.tab==='settings')window.SettingsPaneUI?.init?.(C,uiContext());if(C.tab==='route')window.RouteWorkspaceUI?.init?.(C,uiContext());if(C.tab==='colonization')window.ColonyWorkspaceUI?.init?.(C,uiContext());if(C.tab==='exobiology')window.ExobiologyUI?.init?.(C,uiContext());if(C.tab==='commander')window.CommanderUI?.init?.(C,uiContext());if(C.tab==='ranks')window.RanksUI?.init?.(C,uiContext());if(C.tab==='politics')window.PoliticsUI?.init?.(C,uiContext());window.SystemSearchUI?.init?.(C,uiContext());}
function referenceActive(){return C.tab==='encyclopedia'||(['engineering','targets'].includes(C.tab)&&C.mode==='Инженеры');}
function renderOverview(){return window.OverviewUI.render(C,{...uiContext(),renderModes});}
function referenceCell(row,column,text){
 const d=row.Data||{};let id='';
 const isColumn=label=>column===label||column===t(label);
 const system=/^(?:Система(?:\s|$)|System\b)/.test(column)?(d.system||(C.tab==='route'?d.name:'')):'';if(system)id='system/'+encodeURIComponent(system);
 else if(isColumn('Материал')&&d.key)id='material/'+d.key;
 else if(isColumn('Модуль')&&d.moduleKey)id='module/'+d.moduleKey;
 else if(isColumn('Чертёж')&&d.blueprintKey)id='blueprint/'+d.blueprintKey;
 else if(isColumn('Экспериментальный эффект')&&d.experimentalKey)id='experimental/'+d.experimentalKey;
 else if(isColumn('Товар')&&(d.commodityKey||d.key))id='commodity/'+(d.commodityKey||d.key);
 const link=id?`<button type="button" class="table-entity-link" data-click="reference-entity" data-reference-id="${esc(id)}">${esc(text)}</button>`:esc(text);return system?`<span class="table-entity-actions">${link}<button type="button" class="icon-button" data-click="table-copy-system" data-system="${esc(system)}" title="${th("Копировать систему")}" aria-label="${th("Копировать систему ")}${esc(system)}">${icon("copy")}</button></span>`:link;
}
function colonyBudgetActive(){return C.tab==='colonization'&&C.mode==='Бюджет';}
function renderEngineeringExchange(){
 const trade=C.state.engineeringTrade;
 if(C.tab!=='engineering'||!trade?.targetName)return '';
 const en=currentLanguage()==='en', rows=trade.offers||[];
 const link=(key,name)=>`<button type="button" class="table-entity-link" data-reference-id="material/${esc(key)}">${esc(name)}</button>`;
 const target=link(trade.targetKey,en&&trade.targetEnglish||trade.targetName);
 const columns=['Отдать','Количество','Доступно без резерва','Получить','Статус'];
 const offers=rows.map(row=>{
  const status=row.storageFits===false?'Мало места для материала':row.affordable===false?'Не хватает запаса':row.affordable===true&&row.storageFits===true?'Можно обменять':'Запас не подтверждён';
  return `<tr><td>${link(row.fromKey,en&&row.fromEnglish||row.fromName)}</td><td>${esc(fmt(row.requiredFrom))}</td><td>${row.available==null?'—':esc(fmt(row.available))}</td><td>${esc(fmt(row.received))}</td><td>${th(status)}</td></tr>`;
 }).join('');
 return `<section class="panel eng-exchange" aria-label="${th('Результат обмена')}"><h2>${th('Результат обмена')}</h2><p>${target} × ${esc(fmt(trade.targetAmount))}</p>${rows.length?`<div class="eng-exchange-scroll"><table><thead><tr>${columns.map(label=>`<th scope="col">${th(label)}</th>`).join('')}</tr></thead><tbody>${offers}</tbody></table></div>`:`<p>${th('Нет подходящих вариантов обмена для текущего запаса.')}</p>`}</section>`;
}
function renderGeneral(){
 const page=C.page,table=tableWindow(),budgetMode=colonyBudgetActive();
 const colony=C.tab==='colonization',colonyOverview=colony&&(!C.mode||C.mode==='Обзор'),colonySearch=colony&&C.mode==='Выбор системы',colonyLog=colony&&C.mode==='Журнал снабжения';
 if(colonyOverview)return heading(page.Title,'')+(window.ColonyWorkspaceUI?.render(C,uiContext())||'');
 const budgetActionIds=['colony.purchase','colony.resource-upsert','colony.export-csv'];
 const allowed=ids=>C.actions.filter(a=>ids.includes(a.id));
 const actions=budgetMode?renderActions(allowed(budgetActionIds),budgetActionIds):colonySearch?renderActions(allowed(['colony.choose-candidate','colony.cancel-search'])):colonyLog?renderActions(allowed(['colony.delivery-remove'])):colony?renderActions(C.actions.filter(a=>!['colony.create','colony.select','colony.carrier-stock','colony.edit','colony.copy-system'].includes(a.id)),['colony.add-building','colony.edit-building','colony.delivery-add']):renderActions();
 const scoped=window.NavigationUI?.usesSidebarModes(C.tab), title=scoped&&C.mode?t(C.mode):page.Title;
 const controls=modeControls(), showSearch=page.Rows.length||!!C.query||(C.tab!=='route'&&!colony);
 const intro=heading(title,'','',showSearch&&!controls?tableSearch():'')+renderModes(!showSearch||!controls,controls)+(C.tab==='route'?(window.RouteWorkspaceUI?.renderSummary(C,uiContext())||''):colony?(window.ColonyWorkspaceUI?.render(C,uiContext())||''):'')+(C.tab==='route'?'<label class="route-clipboard-option"><input id="route-auto-copy" type="checkbox" '+(C.state.settings?.routeAutoCopyNext!==false?'checked':'')+`><span>${th("Копировать следующую систему после прыжка")}<small>${th("В карте галактики нажмите Ctrl+V.")}</small></span></label>`:'')+actions+(!budgetMode&&!colonyLog&&page.Summary?`<div class="summary-note">${esc(page.Summary)}</div>`:'');
 if(!page.Rows.length&&((C.tab==='route'&&C.mode!=='Итоги')||colonySearch))return intro;
 const tablePanel=`<div class="table-panel"><div class="table-scroll"><table><thead><tr>${page.Columns.map(c=>`<th>${esc(c)}</th>`).join('')}</tr></thead><tbody>${table.rows.map(r=>`<tr data-row="${esc(r.ID)}" class="${r.ID===C.selected?'selected':''}" tabindex="0" aria-selected="${r.ID===C.selected}">${r.Cells.map((c,index)=>`<td>${referenceCell(r,page.Columns[index],c)}</td>`).join('')}</tr>`).join('')}</tbody></table>${page.Rows.length?'':renderEmptyState()}</div>${renderTablePager(table)}</div>`;
 const building=colony&&C.mode==='Строительство'?(window.ColonyWorkspaceUI?.renderBuilding(C,uiContext())||''):'';
 const inspector=C.inspector&&page.Rows.length&&!budgetMode&&!colonyLog?renderInspector():'';
 return intro+renderEngineeringExchange()+`<div class="data-split${building?' colony-construction-split':''}">${building?`<div class="colony-construction-main">${tablePanel}${building}</div>`:tablePanel}${inspector}</div>`;
}
function tableWindow(){
 const rows=C.page?.Rows||[],key=[C.tab,C.mode,C.query].join('\u0000'),size=100;
 if(C.tableContext!==key){C.tableContext=key;C.tablePage=0;}
 const pages=Math.max(1,Math.ceil(rows.length/size));C.tablePage=Math.max(0,Math.min(C.tablePage||0,pages-1));
 const start=C.tablePage*size;return {rows:rows.slice(start,start+size),total:rows.length,start,end:Math.min(start+size,rows.length),pages,page:C.tablePage};
}
function renderTablePager(table){if(table.pages<=1)return '';return `<div class="table-pager" aria-label="${th("Страницы таблицы")}"><span>`+fmt(table.start+1)+'–'+fmt(table.end)+t(" из ")+fmt(table.total)+'</span><span class="spacer"></span>'+btn(t("Назад"),'table-prev','chevron-left','quiet',table.page===0?'disabled':'')+'<span>'+(table.page+1)+' / '+table.pages+'</span>'+btn(t("Далее"),'table-next','chevron-right','quiet',table.page>=table.pages-1?'disabled':'')+'</div>';}
function emptyStateInfo(){
 const mode=C.mode||C.page?.Modes?.[0]||'',colony=C.state.colonization||{},project=colony.activeProject||{},buildings=project.buildings||[];
 if(C.query.trim())return {title:t("Нет совпадений"),hint:t("По запросу «")+C.query.trim()+t("» ничего не найдено. Очистите поиск или уточните запрос."),clear:true};
 if(C.tab==='colonization'){
  if(["Выбор системы",'selection',''].includes(mode))return colony.search?.running?{title:t("Ищем системы для колонизации"),hint:''}:{title:t("Выберите исходную систему"),hint:t("Укажите систему и выберите исходную станцию."),actionId:'colony.search'};
  if(!project.id)return {title:t("Выберите колонию"),hint:t("Добавьте колонию вручную или найдите систему."),actionId:'colony.create'};
  if(["Строительство",'construction'].includes(mode))return {title:t("В плане пока нет объектов"),hint:t("Добавьте постройку."),actionId:'colony.add-building'};
  if(mode!=='Журнал снабжения'&&!buildings.length)return {title:t("Сначала добавьте объект строительства"),hint:t("Добавьте объект на экране строительства."),mode:'Строительство',label:t("К строительству")};
  if(mode==='Очередь снабжения')return {title:t("Очередь снабжения пуста"),hint:t("Незавершённых объектов нет."),mode:'Строительство',label:t("К строительству")};
  if(mode==='Журнал снабжения')return {title:t("Поставок этой колонии пока нет"),hint:''};
  if(mode==='Поставки')return {title:t("Поставок этой колонии пока нет"),hint:t("Можно добавить личную отметку поставки."),actionId:'colony.delivery-add'};
 }
 if(C.tab==='engineering'){
  if(["План",''].includes(mode))return {title:t("План улучшений пуст"),hint:t("Добавьте модуль и чертёж."),actionId:'engineering.add'};
  if(mode.includes('плана'))return {title:t("Для плана пока нет материалов"),hint:t("Добавьте улучшение в план или проверьте выбранную категорию материалов."),mode:'План',label:t("К плану улучшений")};
  return {title:mode==='Инженеры'?t("Инженеры не найдены"):t("Материалы пока не отображаются"),hint:t("Проверьте категорию и подключение журналов.")};
 }
 if(C.tab==='statistics')return {title:mode==='Торговля'?t("Торговых сделок пока нет"):mode==='Колонии'?t("Поставок колониям пока нет"):t("За этот период пока нет событий"),hint:''};
 if(C.tab==='sale'&&mode==='Журнал сделок')return {title:t("Сделок пока нет"),hint:''};
 const pages={sale:{title:t("Найдите подходящий рынок"),hint:t("Укажите товар, исходную систему и ограничения перелёта."),actionId:'market.search'},targets:mode==='Места добычи'?{title:t("Найдите место добычи"),hint:t("Укажите исходную систему и ценный ресурс для поиска."),actionId:'mining.search'}:{title:t("Нет активных целей"),hint:'',actionId:'goal.add'},route:{title:t("Постройте маршрут флотоносца"),hint:t("Задайте системы назначения, загрузку и запас трития."),actionId:'route.calculate'},deals:{title:t("Подберите торговый рейс"),hint:t("Задайте исходную систему, трюм и бюджет для поиска выгодных перевозок."),actionId:'deals.search'}};
 return pages[C.tab]||{title:t("Нет записей"),hint:''};
}
function emptyHint(){return emptyStateInfo().hint;}
function renderEmptyState(){const info=emptyStateInfo();let control='';if(info.clear)control=btn(t("Очистить поиск"),'clear-query','x','quiet');else if(info.mode)control=`<button class="pill" data-mode="${esc(info.mode)}">${icon('arrow-right')}${esc(info.label)}</button>`;return `<div class="empty-state"><strong>${esc(info.title)}</strong>${info.hint?`<p>${esc(info.hint)}</p>`:''}${control}</div>`;}

function modeControls(){const modes=window.NavigationUI?.usesSidebarModes(C.tab)?[]:C.page.Modes||[],projects=C.state.colonization?.projects||[];return `${modes.length?`<div class="segmented" aria-label="${th("Экраны раздела")}">${modes.map(m=>`<button data-mode="${esc(m)}" class="${m===(C.mode||modes[0])?'selected':''}">${esc(t(m))}</button>`).join('')}</div>`:''}${C.tab==='colonization'&&projects.length>1&&['Строительство','Журнал снабжения','Бюджет'].includes(C.mode)?`<select id="colony-select" aria-label="${th("Выбранная колония")}">${projects.map(p=>`<option value="${esc(p.id)}" ${p.id===C.state.colonization.selectedProjectId?'selected':''}>${esc(p.name||p.targetSystem)}</option>`).join('')}</select>`:''}`;}
function tableSearch(){return `<label class="search-field">${icon('search')}<input id="table-query" value="${esc(C.query)}" placeholder="${th("Поиск в разделе")}" aria-label="${th("Поиск в разделе")}"></label>`;}
function renderModes(segmentsOnly=false,controls=modeControls()){const items=controls+(segmentsOnly?'':tableSearch());return items?`<div class="filters">${items}</div>`:'';}
function extraActions(anchor){const ids=new Set((anchor?.dataset?.extraActions||'').split(','));return C.actions.filter(action=>!action.disabled&&ids.has(action.id));}
function renderActions(actions=C.actions,primaryIds=[]){
 const enabled=actions.filter(a=>!a.disabled),primary=primaryIds.map(id=>enabled.find(a=>a.id===id)).filter(Boolean);
 const ordered=[...primary,...enabled.filter(a=>!primary.includes(a))],visible=ordered.slice(0,3),more=ordered.slice(3);
 if(!visible.length)return '';
 return '<div class="action-row">'+visible.map((a,i)=>btn(a.title,'action',i===0?'plus':'',i===0?'primary':'','data-action-id="'+esc(a.id)+'"')).join('')+
 (more.length?btn(t("Ещё"),'page-menu','ellipsis','quiet','data-extra-actions="'+esc(more.map(a=>a.id).join(','))+'" aria-haspopup="true"'):'')+'</div>';
}
function renderHelp(){
 const groups=new Map();
 for(const row of C.page.Rows){const [group,title]=row.Cells;if(!groups.has(group))groups.set(group,[]);groups.get(group).push({id:row.ID,title});}
 const selected=C.page.Rows.find(row=>row.ID===C.selected);
 return heading(t("Руководство"),'')+renderModes()+(!groups.size?renderEmptyState():
 '<div class="guide-workspace"><nav class="guide-index" aria-label="'+th("Темы руководства")+'">'+Array.from(groups,([group,rows])=>
 '<section><h2>'+esc(group)+'</h2>'+rows.map(row=>'<button type="button" data-guide-id="'+esc(row.id)+'" '+(row.id===C.selected?'aria-current="true"':'')+'>'+esc(row.title)+'</button>').join('')+'</section>').join('')+'</nav>'+
 (selected?'<article class="panel guide-article" tabindex="-1"><h2>'+esc(selected.Cells[1])+'</h2><div class="prose-small">'+esc(selected.Details)+'</div></article>':'')+'</div>');
}
function renderConnections(){
 const frontier=C.state.frontier||{},inara=C.state.integrations||{},connected=frontier.connected===true,keySet=inara.apiKeySet===true||inara.apiKeyConfigured===true;
 const frontierStatus=connected?t("Подключено"):t("Не подключено");
 const inaraStatus=keySet?(inara.enabled===true?t("Отправка включена"):t("Отправка выключена")):t("Ключ не добавлен");
 const signIn=connected?'frontier.logout':'frontier.login',action=C.actions.find(item=>item.id===signIn),keyAction=C.actions.find(item=>item.id==='inara.settings');
 return '<section class="connections-settings"><div class="section-heading"><h2>'+th("Подключения")+'</h2></div><div class="settings-grid">'+
 '<section class="settings-group"><h3>'+icon('shield-check')+'Frontier</h3><div class="setting-row"><span>'+th("Состояние подключения")+'</span>'+badge(frontierStatus,connected?'good':'')+'</div>'+
 (frontier.lastError?'<p class="info-note" role="status">'+esc(frontier.lastError)+'</p>':'')+
 (action?btn(action.title,'action',connected?'log-out':'external-link','quiet','data-action-id="'+esc(signIn)+'"'):'')+'</section>'+
 '<section class="settings-group"><h3>'+icon('cloud-check')+'Inara</h3><div class="setting-row"><span>'+th("Синхронизация")+'</span>'+badge(inaraStatus,keySet&&inara.enabled===true?'good':'')+'</div>'+
 (inara.lastError?'<p class="info-note" role="status">'+esc(inara.lastError)+'</p>':'')+
 (keyAction?'<form id="inara-connection-form" class="connection-form"><label><span>'+th("Личный API-ключ Inara")+'</span><input name="apiKey" type="password" autocomplete="off" '+(keySet?'placeholder="'+th("Ключ сохранён")+'"':'')+'></label>'+
 '<label class="connection-check"><input name="enabled" type="checkbox" '+(inara.enabled===true?'checked':'')+'>'+th("Отправлять события в Inara")+'</label>'+btn(t("Сохранить"),'connection-save','','quiet','type="submit"')+'</form>':'')+'</section></div></section>';
}
function renderInspector(){const row=C.page.Rows.find(r=>r.ID===C.selected);return `<aside class="inspector"><div class="inspector-title">${th("Подробности")}${iconBtn('x','inspector',t("Скрыть подробности"))}</div><div class="prose-small">${esc(row?.Details||t("Выберите строку, чтобы увидеть подробности, ограничения и источник данных."))}</div>${row?`<hr>${btn(t("Скопировать"),'copy-details','copy','quiet')}`:''}</aside>`;}
function renderSettings(){const theme=C.state.theme||{},d=C.state.desktop||{},settings=C.state.settings||{};const languageAction=C.actions.find(a=>a.id==='settings.language');const mode={windowed:t("В окне"),fullscreen:t("На весь экран"),borderless:t("Окно без рамки")}[d.displayMode]||t("В окне");const find=(pattern)=>C.actions.find(a=>a.id.includes(pattern));const actionButton=(pattern,label)=>{const a=find(pattern);return a?btn(label||a.title,'action','','','data-action-id="'+esc(a.id)+'"'):'';};return heading(t("Настройки"),'')+`<div class="settings-grid"><section class="settings-group"><h3>${icon('layout-grid')}${th("Рабочее пространство")}</h3><div class="setting-row"><span>${th("Язык интерфейса")}</span>${languageAction?btn(settings.language==='en'?'English':'Русский','action','','quiet','data-action-id="settings.language"'):esc(settings.language==='en'?'English':'Русский')}</div><div class="setting-row"><span>${th("Режим отображения")}</span><strong>${mode}</strong></div><div class="setting-row"><span>${th("Основное меню")}</span>${btn(t("Настроить"),'customize','ellipsis','quiet')}</div>${actionButton('window')||actionButton('display')||C.actions.filter(a=>a.fields.some(f=>f.key==='displayMode')).map(a=>btn(a.title,'action','','','data-action-id="'+esc(a.id)+'"')).join('')}</section><section class="settings-group"><h3>${icon('eye')}${th("Игровой оверлей")}</h3><div class="setting-row"><span>${th("Горячая клавиша")}</span><strong>Ctrl + Alt + H</strong></div></section><section class="settings-group"><h3>${icon('sun')}${th("Цветовая палитра")}</h3><p class="muted" style="font-size:12px;line-height:1.7">${th("Выберите готовую палитру, настройте матрицу RGB или скопируйте цвета из конфигурации игры.")}</p>${C.actions.filter(a=>/theme|palette|matrix/i.test(a.id)).map(a=>btn(a.title,'action','','quiet','data-action-id="'+esc(a.id)+'"')).join('')}<label class="check-label" style="display:flex;align-items:center;gap:10px;margin-top:16px;font-size:12px"><input type="checkbox" id="main-game-palette" ${d.useGamePalette?'checked':''}>${th("Игровая палитра в основном окне")}</label><div class="palette-sample" style="--game-accent:${esc(theme.palette?.accent||'#a6c7ef')};--game-panel:${esc(theme.palette?.panel||'#172534')};--game-text:${esc(theme.palette?.text||'#eee')}"><strong>${th("ED COPILOT · КУРС ПРОЛОЖЕН")}</strong>${th("Корабль готов к прыжку. Чистого неба, командир.")}</div></section><section class="settings-group"><h3>${icon('radio')}${th("Журнал и данные")}</h3><div class="prose-small">${esc(C.state.connection?.journalDir||t("Папка журналов не определена"))}</div><div class="divider"></div>${C.actions.filter(a=>/folder|journal|rescan/.test(a.kind+' '+a.id)).map(a=>btn(a.title,'action','','quiet','data-action-id="'+esc(a.id)+'"')).join('')}<p class="info-note">${th("Личные планы и профиль сохраняются отдельно от файлов приложения.")}</p></section>${window.UpdatesUI.renderSettings(C,uiContext())}${window.SettingsPaneUI.renderProfile(uiContext())}</div><div class="section-heading"><h2>${th("Добыча и справочные цены")}</h2></div><div class="panel subtle"><p class="muted" style="font-size:12px;line-height:1.8">${th("Фильтры помогают отсечь неподходящие места добычи. Справочная цена нужна для приблизительной оценки груза и планов, когда текущей рыночной котировки ещё нет. Это ваша оценка, а не обещание цены продажи.")}</p>${C.actions.filter(a=>a.id.startsWith('settings.')&&a.id!=='settings.language'&&!/(theme|palette|matrix|window|display|folder|journal|rescan)/i.test(a.kind+' '+a.id)).map(a=>btn(a.title,'action','','quiet','data-action-id="'+esc(a.id)+'"')).join('')}</div>`+renderConnections();}
function captureView(){
 const host=$('#page-scroll'),active=document.activeElement,inside=host?.contains(active);
 return {top:host?.scrollTop||0,scroll:$$('.table-scroll,.inspector,.sidebar-scroll').map(node=>({top:node.scrollTop,left:node.scrollLeft})),focus:inside?{id:active.id,name:active.name,tag:active.tagName,row:active.dataset?.row,calc:active.dataset?.calcInput,start:active.selectionStart,end:active.selectionEnd,direction:active.selectionDirection}:null};
}
function restoreView(saved){
 const host=$('#page-scroll');if(host)host.scrollTop=saved.top;
 $$('.table-scroll,.inspector,.sidebar-scroll').forEach((node,index)=>{const position=saved.scroll[index];if(position){node.scrollTop=position.top;node.scrollLeft=position.left;}});
 const f=saved.focus;if(!f)return;
 const target=f.id?document.getElementById(f.id):$$('input,select,textarea,button,[data-row]').find(node=>f.name?node.name===f.name&&node.tagName===f.tag:f.row?node.dataset.row===f.row:f.calc?node.dataset.calcInput===f.calc:false);
 if(!target||target.disabled)return;target.focus({preventScroll:true});if(f.start!==null&&f.start!==undefined&&target.setSelectionRange){try{target.setSelectionRange(f.start,f.end,f.direction||'none');}catch{}}
}
function captureDisclosures(){return $$('#page details').map((node,index)=>({index,kind:String(node.className||''),owner:node.closest?.('[data-deals-route]')?.dataset?.dealsRoute||'',open:node.open}));}
function restoreDisclosures(saved){const nodes=$$('#page details');for(const item of saved){const node=nodes[item.index];if(node&&String(node.className||'')===item.kind&&(node.closest?.('[data-deals-route]')?.dataset?.dealsRoute||'')===item.owner)node.open=item.open;}}
async function loadView(quiet=false,{smokeProbe=false}={}){
 if(quiet&&((window.COPILOT_SMOKE&&!smokeProbe)||document.hidden||C.layer||document.activeElement?.closest('input,select,textarea,#inara-connection-form')))return;
 const epoch=++viewEpoch;
 try{
  let result=await call('view',context());if(epoch!==viewEpoch)return;
  const page=result.page||{},rows=page.Rows||[],modes=page.Modes||[],oldContext=context();
  if(result.context&&typeof result.context.mode==='string'&&typeof result.context.selected==='string'){
   C.mode=result.context.mode;C.selected=result.context.selected;
  }else{
   if(C.mode===''&&modes.length)C.mode=modes[0];
   if(!rows.some(row=>row.ID===C.selected))C.selected=rows[0]?.ID||'';
   // Compatibility with view providers that don't return resolved action context.
   if(JSON.stringify(context())!==JSON.stringify(oldContext)){result=await call('view',context());if(epoch!==viewEpoch)return;}
  }
  if(quiet&&(C.layer||document.activeElement?.closest('input,select,textarea,#inara-connection-form')))return;
  const languageChanging=window.CopilotI18n&&window.CopilotI18n.normalize(result.state?.settings?.language||result.state?.app?.language)!==window.CopilotI18n.language;
  const saved=captureView(),drafts=languageChanging?captureInputs():null,disclosures=quiet||languageChanging?captureDisclosures():null;
  $('#page')?.removeAttribute?.('aria-busy');C.state=result.state||{};C.page=result.page||{};C.page.Rows=C.page.Rows||[];C.page.Modes=C.page.Modes||[];C.actions=result.actions||[];C.help=result.help;C.lastFetch=Date.now();C.needsRefresh=false;
  const languageChanged=applyLanguage();
  const signature=JSON.stringify([C.page,C.actions,C.state]);
  if(quiet&&signature===C.lastSignature){applyPalette();renderChrome();return;}
  C.lastSignature=signature;const explicitScroll=C.restoreScroll;
  if(languageChanged||C.shellLanguage&&C.shellLanguage!==currentLanguage())shell();renderPage();if(drafts)restoreInputs(drafts);if(disclosures)restoreDisclosures(disclosures);if(explicitScroll===undefined)restoreView(saved);window.UpdatesUI?.showNotice?.(C,uiContext());
 }catch(error){if(!quiet){toast(error.message,true);if(epoch===viewEpoch&&!C.page){const page=$('#page');if(page){page.removeAttribute?.('aria-busy');page.innerHTML=`<div class="empty-state"><h3>${th("Не удалось открыть раздел")}</h3><p>`+esc(error.message)+'</p>'+btn(t("Повторить"),'reload-page','refresh-cw')+'</div>';}}}}
}
function beginNavigation(){window.EngineeringRoadmapUI?.leave(C);window.SystemSearchUI?.close?.();++viewEpoch;C.page=null;C.actions=[];C.help='';const page=$('#page');if(page){page.innerHTML='<div class="page-loading" role="status">'+icon('refresh-cw')+`<span>${th("Открываю раздел…")}</span></div>`;page.setAttribute?.('aria-busy','true');}}
const history=[];
async function navigate(id,opts={}){if(['account','integrations'].includes(id))id='settings';if(id==='overview'&&['Планы','Состояние'].includes(opts.mode))opts={...opts,mode:'Обзор'};if(id==='commander'&&opts.mode==='Репутация')opts={...opts,mode:'Сводка'};if(id==='colonization'&&['Очередь снабжения','Поставки','Снабжение','supplies'].includes(opts.mode))opts={...opts,mode:opts.mode==='Очередь снабжения'?'Строительство':'Журнал снабжения'};if(!C.tabs.some(t=>t.ID===id))return;if(!opts.back)history.push({tab:C.tab,mode:C.mode});C.tab=id;C.mode=opts.mode||window.NavigationUI?.group(window.NavigationUI?.groupFor(id))?.modes?.[0]?.[0]||'';const group=window.NavigationUI?.groupFor(id,C.mode);if(group){C.group=group;C.groupLast[group]={tab:id,mode:C.mode};}C.query='';C.selected='';if(id!=='learning'){C.lesson='';C.course='';C.focus=false;}if(window.ReferenceUI?.navigate)window.ReferenceUI.navigate(id,opts,C);C.prefill=opts.prefill||{};beginNavigation();if(!C.nav.includes(id)&&['work','fleet','knowledge'].includes(group)&&id!=='help'){C.nav.push(id);await saveNavigation();}renderNavigation();renderChrome();$('#page-scroll').scrollTop=0;await loadView();}
async function saveNavigation(){await call('navigation',{tabs:C.nav});}
function closeLayer(){window.SystemSearchUI?.close?.();$('#layers').innerHTML='';C.layer=false;C.themePreview=null;if(C.pendingLanguage)applyLanguage(true);}
function dialog(title,body,footer='',wide=false){C.layer=true;$('#layers').innerHTML=`<div class="overlay-backdrop"><section class="dialog ${wide?'wide':''}" role="dialog" aria-modal="true" aria-label="${esc(title)}"><header class="dialog-head"><h2>${esc(title)}</h2><span class="spacer"></span>${iconBtn('x','close-dialog',t("Закрыть"))}</header><div class="dialog-body">${body}</div>${footer?`<footer class="dialog-foot">${footer}</footer>`:''}</section></div>`;window.SystemSearchUI?.init?.(C,uiContext());const panel=$('#layers .dialog');setTimeout(()=>{if(!C.layer||!panel||panel!==$('#layers .dialog')||panel.contains(document.activeElement))return;const first=$('.dialog-body input:not([type=checkbox]):not([type=hidden]):not([disabled]),.dialog-body select:not([disabled]),.dialog-body textarea:not([disabled])',panel)||$('.dialog-foot button:not([disabled])',panel)||$('.dialog-head button',panel);first?.focus();},30);}
function menu(items,anchor){$('.menu')?.remove();const bounds=anchor?.getBoundingClientRect()||{left:innerWidth/2,top:80,bottom:80};const node=document.createElement('div');node.className='menu';node.innerHTML=items.map(item=>item.separator?'<hr>':`<button ${item.action?'data-click="'+esc(item.action)+'"':''} ${item.nav?'data-nav="'+esc(item.nav)+'"':''} ${item.id?'data-action-id="'+esc(item.id)+'" data-click="action"':''} ${item.disabled?'disabled':''}>${icon(item.icon||'circle')}${esc(item.title)}</button>`).join('');$('#layers').append(node);const width=node.offsetWidth||230,height=node.offsetHeight||0;node.style.left=Math.max(8,Math.min(bounds.left,innerWidth-width-8))+'px';node.style.top=Math.max(8,bounds.bottom+height+13<=innerHeight?bounds.bottom+5:Math.min(bounds.top-height-5,innerHeight-height-8))+'px';}
function showCommand(){dialog(t("Быстрый переход"),`<label class="command-search">${icon('search')}<input id="command-query" placeholder="${th("Куда перейдём?")}" autocomplete="off"><span class="key">Esc</span></label><div class="command-results" id="command-results"></div>`);$('.dialog-body').style.padding='0';filterCommand('');}
function filterCommand(query){
 const norm=value=>String(value||'').toLocaleLowerCase().replaceAll('ё','е'),q=norm(query).trim();
 const nav=window.NavigationUI,items=C.tabs.flatMap(tab=>{
  const group=nav?.groupFor(tab.ID),modes=nav?.usesSidebarModes(tab.ID)?nav.entries(C,group).filter(item=>item.tab===tab.ID&&item.mode):[];
  return modes.length?modes.map(item=>({...item,section:tab.Title,subsection:item.title})):[{tab:tab.ID,title:tab.Title,icon:tabIcons[tab.ID]}];
 });
 if(C.tabs.some(tab=>tab.ID==='encyclopedia'))items.push({tab:'encyclopedia',mode:'Системы',title:'Системы',icon:'compass'});
 const label=item=>item.subsection?(item.section===item.subsection?t(item.section):t(item.section)+' · '+t(item.subsection)):t(item.title);
 $('#command-results').innerHTML=items.filter(item=>norm(label(item)).includes(q)).map(item=>'<button class="command-item" data-nav="'+esc(item.tab)+'"'+(item.mode?' data-nav-mode="'+esc(item.mode)+'"':'')+'>'+icon(item.icon)+esc(label(item))+'<span class="spacer"></span>'+icon('arrow-right')+'</button>').join('')||'<div class="empty-state">'+th("Разделов с таким названием нет")+'</div>';
}
function customize(){
 const ordered=[...C.nav,...C.tabs.map(t=>t.ID).filter(id=>!C.nav.includes(id))],groups=window.NavigationUI?.groups.filter(group=>['work','fleet','knowledge'].includes(group.id))||[];
 const row=id=>`<label class="customize-item">${icon(tabIcons[id])}<input type="checkbox" data-nav-check="${id}" ${C.nav.includes(id)||['commander','ranks'].includes(id)?'checked':''} ${['overview','commander','ranks'].includes(id)?'disabled':''}><span>${esc(navTitle(id))}</span><span class="item-arrows" ${['commander','ranks'].includes(id)?'hidden':''}>${iconBtn('arrow-up','nav-up',t("Выше"),'data-id="'+id+'"')}${iconBtn('arrow-down','nav-down',t("Ниже"),'data-id="'+id+'"')}</span></label>`;
 dialog(t("Ваше основное меню"),`<p class="muted" style="font-size:12px">${th("Выберите разделы в каждой группе. Остальные доступны через поиск и список дополнительных разделов.")}</p><div id="customize-list">${groups.map(group=>`<section class="customize-group"><h3>${esc(t(group.title))}</h3>${ordered.filter(id=>group.tabs.includes(id)&&!['settings','help'].includes(id)).map(row).join('')}</section>`).join('')}</div>`,btn(t("Готово"),'save-menu','','primary'));
}
function bindDragNav(){const nav=$('#navigation');nav.addEventListener('dragstart',event=>{const item=event.target.closest('[data-order]');if(item)event.dataTransfer.setData('text/plain',item.dataset.order);});nav.addEventListener('dragover',event=>{event.preventDefault();$$('.drag-over').forEach(x=>x.classList.remove('drag-over'));event.target.closest('[data-order]')?.classList.add('drag-over');});nav.addEventListener('drop',async event=>{event.preventDefault();const from=event.dataTransfer.getData('text/plain'),to=event.target.closest('[data-order]')?.dataset.order;if(!from||!to||from===to)return;C.nav=C.nav.filter(x=>x!==from);C.nav.splice(C.nav.indexOf(to),0,from);await saveNavigation();renderNavigation();});}
async function openAction(id){const a=C.actions.find(x=>x.id===id);if(!a||a.disabled)return;if(a.confirm){dialog(t("Подтвердите действие"),`<p class="prose-small">${esc(a.confirm)}</p>`,btn(t("Отмена"),'close-dialog')+btn(a.title,'confirm-action','','primary','data-action-id="'+esc(a.id)+'"'));return;}await actionForm(a);}
function datetimeInput(value){
 if(value===null||value===undefined||value==='')return '';
 const text=String(value),ru=text.match(/^(\d{2})\.(\d{2})\.(\d{4}) (\d{2}):(\d{2})$/);
 if(ru)return `${ru[3]}-${ru[2]}-${ru[1]}T${ru[4]}:${ru[5]}`;
 if(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(text))return text;
 const date=new Date(value);if(Number.isNaN(+date))return '';
 const pad=number=>String(number).padStart(2,'0');
 return `${date.getFullYear()}-${pad(date.getMonth()+1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}
function actionValues(action,values){
 const result={...values};
 for(const field of action?.fields||[]){if(['number','integer'].includes(field.kind)&&typeof result[field.key]==='string')result[field.key]=result[field.key].trim().replace(/[\s\u00a0\u202f]/g,'').replace(',', '.');}
 return result;
}
async function actionForm(a){if(!a.fields.length){await submitAction(a.id,{});return;}const field=f=>{const choices=f.choices||[];let input;if(f.kind==='bool')input=`<label class="check-label"><input name="${esc(f.key)}" type="checkbox" ${f.default?'checked':''}>${esc(f.label)}</label>`;else if(f.kind==='select'||f.kind==='combo'){const options=choices.map(c=>`<option value="${esc(c.Value)}" ${String(c.Value)===String(f.default??'')?'selected':''}>${esc(c.Label)}</option>`).join('');input=f.kind==='select'?`<select name="${esc(f.key)}">${options}</select>`:`<input name="${esc(f.key)}" value="${esc(f.default)}" list="choices-${esc(f.key)}"><datalist id="choices-${esc(f.key)}">${options}</datalist>`;}else if(f.kind==='multiline')input=`<textarea name="${esc(f.key)}" rows="3" ${f.required?'required':''}>${esc(f.default)}</textarea>`;else{const type=f.kind==='password'?'password':f.kind==='datetime'?'datetime-local':f.kind==='color'?'text':'text';let value=f.default??'';if(f.kind==='datetime')value=datetimeInput(value);input=`<input name="${esc(f.key)}" type="${type}" value="${esc(value)}" ${f.required?'required':''} ${['number','integer'].includes(f.kind)?'inputmode="decimal"':''} ${f.kind==='password'?'autocomplete="new-password"':''}>`;}return `<label class="field ${['bool','multiline'].includes(f.kind)?'full':''}">${f.kind==='bool'?'':`<span>${esc(f.label)}${f.required?' *':''}</span>`}${input}${f.help?`<span class="help-text">${esc(f.help)}</span>`:''}</label>`;};dialog(a.title,`<div id="form-error" class="form-error hidden" role="alert"></div><form id="action-form" data-id="${esc(a.id)}"><div class="field-grid">${a.fields.map(field).join('')}</div></form>`,btn(t("Отмена"),'close-dialog')+`<button class="pill primary" type="submit" form="action-form">${th("Применить")}</button>`,a.fields.length>8);if(a.fields.some(f=>f.key==='moduleKey'))await prepareEngineeringForm();}
async function prepareEngineeringForm(){try{const catalog=(await call('catalog',context())).engineering||{};const form=$('#action-form'),module=form.elements.moduleKey,bp=form.elements.blueprintKey,exp=form.elements.experimentalKey;if(!module||!bp)return;const rebuild=()=>{const m=(catalog.modules||[]).find(x=>x.key===module.value)||{};const old=bp.value;bp.innerHTML=(catalog.blueprints||[]).filter(x=>(m.blueprints||[]).includes(x.key)).map(x=>`<option value="${esc(x.key)}">${esc(x.name)}</option>`).join('');if([...bp.options].some(o=>o.value===old))bp.value=old;if(exp){const e=exp.value;exp.innerHTML=`<option value="">${th("Без эффекта")}</option>`+(catalog.experimentals||[]).filter(x=>(m.experimentals||[]).includes(x.key)).map(x=>`<option value="${esc(x.key)}">${esc(x.name)}</option>`).join('');if([...exp.options].some(o=>o.value===e))exp.value=e;}};module.addEventListener('change',rebuild);rebuild();}catch(e){toast(e.message,true);}}
function previewTheme(preview){
 C.themePreview=preview;const palette=preview.palette||{},labels={accent:t("Акцент"),text:t("Текст"),muted:t("Вторичный текст"),bg:t("Фон"),panel:t("Панели"),good:t("Подтверждение"),warn:t("Внимание"),danger:t("Ошибка")};
 const colors=Object.entries(labels).filter(([key])=>/^#[0-9a-f]{6}$/i.test(palette[key]||'')).map(([key,label])=>`<div class="setting-row"><span>${esc(label)}</span><span class="row"><span style="display:inline-block;width:18px;height:18px;border:1px solid #ffffff30;border-radius:5px;background:${esc(palette[key])}"></span><code>${esc(palette[key])}</code></span></div>`).join('');
 const source=typeof preview.source==='string'?preview.source:JSON.stringify(preview.source||{},null,2);
 const sample=Object.entries({accent:'accent',panel:'panel',text:'text'}).filter(([,key])=>/^#[0-9a-f]{6}$/i.test(palette[key]||'')).map(([role,key])=>`--game-${role}:${palette[key]}`).join(';');
 dialog(t("Палитра из интерфейса игры"),`<p class="muted">${th("Проверьте найденные цвета. Примените палитру, чтобы сохранить её для оверлея и выбранного оформления приложения.")}</p><div class="palette-sample" style="${esc(sample)}"><strong>${th("ED COPILOT · КУРС ПРОЛОЖЕН")}</strong>${th("Корабль готов к прыжку. Чистого неба, командир.")}</div><div style="margin-top:18px">${colors}</div>${source?`<details style="margin-top:18px"><summary>${th("Источник палитры")}</summary><div class="prose-small">${esc(source)}</div></details>`:''}`,btn(t("Отмена"),'close-dialog')+btn(t("Применить палитру"),'apply-theme','check','primary'));
}
async function submitAction(id,values){
 const action=C.actions.find(item=>item.id===id);values=actionValues(action,values);const result=await call('action',{...context(),actionId:id,values,...(id.startsWith('goal.')?{planWorkspace:C.state.planWorkspace?.key||''}:{})});
 if(result?.themePreview){previewTheme(result.themePreview);return;}
 if(result&&'details' in result){dialog(result.title||t("Подробности"),`<div class="prose-small">${esc(result.details)}</div>`,btn(t("Закрыть"),'close-dialog'));return;}
 if(result?.help){dialog(t("Помощь · ")+navTitle(C.tab),`<div class="prose-small">${esc(result.help)}</div>`);return;}
 let message=id==='engineering.trade'?t("Обмен рассчитан"):result?.result?.started||result?.started?t("Поиск запущен"):t("Изменения сохранены"),nativeResult=null;
 if(result?.url){nativeResult=await call('native',{action:'open',url:result.url});message=t("Ссылка открыта в браузере");}
 if(result&&'copy' in result){nativeResult=await call('native',{action:'copy',text:result.copy});message=t("Скопировано в буфер обмена");}
 if(result?.native){nativeResult=await call('native',{action:result.native});message=result.native==='folder'?t("Папка журналов выбрана"):t("План импортирован");}
 if(result?.export){const kind=result.export.includes('colonization')?'colonization':result.export.includes('engineering')?'engineering':result.export.includes('trading')?'trading':result.export.includes('route')?'route':'statistics';nativeResult=await call('native',{action:'export',kind,json:result.export.endsWith('.json'),projectId:result.projectId});message=nativeResult?.file?t("Файл сохранён: ")+nativeResult.file:t("Файл сохранён");}
 closeLayer();
 if(nativeResult?.cancelled){toast(t("Действие отменено"));return;}
 if(result?.navigate){await navigate(result.navigate,{mode:result.mode,prefill:result.prefill});if(result.prefill&&C.actions[0])await actionForm(C.actions[0]);return;}
 if(C.tab==='learning'&&/^learning\.(open|next|previous)$/.test(id)){
  await loadView();const library=C.state.learning||{},lesson=(library.lessons||[]).find(item=>item.id===library.selectedLessonId);
  if(!lesson)throw new Error(t("Урок не найден в библиотеке."));
  C.lesson=lesson.id;C.course=lesson.courseId;C.academy=C.academy||{category:'',drafts:{}};const position=lesson.position||{},ratio=Number(position.ratio);
  C.academy.pendingRestore={lessonId:lesson.id,ratio:Number.isFinite(ratio)?Math.max(0,Math.min(1,ratio)):0,sectionId:position.sectionId||''};C.restoreScroll=0;
  await loadView();return;
 }
 if(action?.fields.some(field=>field.key==='displayMode'))await call('window',{action:values.displayMode});
 await loadView();toast(message);
}
document.addEventListener('submit',async event=>{if(event.target.id==='inara-connection-form'){
 event.preventDefault();const form=event.target,button=form.querySelector('button[type=submit]');button.disabled=true;
 try{await submitAction('inara.settings',{apiKey:form.elements.apiKey.value,enabled:form.elements.enabled.checked});}catch(error){button.disabled=false;toast(error.message,true);}return;
 }if(event.target.id!=='action-form')return;event.preventDefault();const form=event.target;const values={};for(const element of form.elements){if(!element.name||element.disabled)continue;values[element.name]=element.type==='checkbox'?element.checked:element.value;}const button=$('button[form="action-form"]');button.disabled=true;try{await submitAction(form.dataset.id,values);}catch(error){const box=$('#form-error');if(box){box.textContent=error.message;box.classList.remove('hidden');}else toast(error.message,true);if(button)button.disabled=false;}});
document.addEventListener('click',async event=>{try{const guide=event.target.closest('[data-guide-id]');if(guide&&C.tab==='help'){C.selected=guide.dataset.guideId;renderPage();$('.guide-article')?.focus({preventScroll:true});return;}const external=event.target.closest('[data-external]');if(external){event.preventDefault();await call('native',{action:'open',url:external.dataset.external});return;}const copySystem=event.target.closest('[data-click="table-copy-system"]');if(copySystem){event.preventDefault();await handleClick('table-copy-system',copySystem,event);return;}const referenceButton=event.target.closest('[data-reference-id]');if(referenceButton){event.preventDefault();await handleClick('reference-entity',referenceButton,event);return;}const group=event.target.closest('[data-group]');if(group){closeLayer();await selectGroup(group.dataset.group);return;}const nav=event.target.closest('[data-nav]');if(nav){closeLayer();const avatar=nav.classList?.contains('avatar');if(avatar){C.collapsed=false;C.focus=false;storage.set('sidebar-collapsed',false);$('.sidebar')?.classList.remove('collapsed');}await navigate(nav.dataset.nav,{mode:nav.dataset.navMode||''});if(avatar)await call('desktop',{...context(),values:{sidebarCollapsed:false}});return;}const mode=event.target.closest('[data-mode]');if(mode){if(mode.dataset.mode!==C.mode)window.EngineeringRoadmapUI?.leave(C);C.mode=mode.dataset.mode;C.query='';C.selected='';await loadView();return;}const row=event.target.closest('[data-row]');if(row){C.selected=row.dataset.row;await loadView();return;}const caption=event.target.closest('[data-window]');if(caption){await call('window',{action:caption.dataset.window});return;}const button=event.target.closest('[data-click]');if(!button){if(!event.target.closest('.menu'))$('.menu')?.remove();return;}$('.menu')?.remove();await handleClick(button.dataset.click,button,event);}catch(error){toast(error.message,true);}});
async function handleClick(action,b,event){if(action==="profile-export"||action==="profile-import")return window.SettingsPaneUI.handleProfile(action,b,uiContext());if(action.startsWith('commander-'))return window.CommanderUI.handleClick(action,b,C,uiContext());if(action.startsWith('exo-'))return window.ExobiologyUI.handleClick(action,b,C,uiContext());if(action==='table-copy-system'){await call('native',{action:'copy',text:b.dataset.system});toast(t("Система скопирована"));return;}if(action.startsWith('route-workspace-'))return window.RouteWorkspaceUI.handle(action,b,C,uiContext());if(action.startsWith('colony-workspace-'))return window.ColonyWorkspaceUI.handleClick(action,b,C,uiContext());if(action==='reference-entity'){await navigate('encyclopedia');return window.ReferenceUI.open(b.dataset.referenceId,C,uiContext());}if(action.startsWith('ranks-'))return window.RanksUI.handleClick(action,b,C,uiContext());if(action.startsWith('politics-'))return window.PoliticsUI.handleClick(action,b,C,uiContext());if(action.startsWith('updates-'))return window.UpdatesUI.handleClick(action,b,C,uiContext());if(action.startsWith('reference-'))return window.ReferenceUI.handleClick(action,b,C,uiContext());if(action.startsWith('deals-'))return window.DealsUI.handle(action,b,C,uiContext());if(action.startsWith('carrier-trade-'))return (window.CarrierTradeUI.handleClick||window.CarrierTradeUI.handle)(action,b,C,uiContext());if(action==='trade-stats-ledger')return navigate('sale',{mode:'Журнал сделок'});if(action==='overview-navigation')return navigate('targets',{mode:'Навигация'});if(action==='overview-cargo'){C.mode='Корабль и груз';return loadView();}if(action.startsWith('navigate-'))return navigate(action.slice(9));switch(action){case 'reload-page':return loadView();case 'action':return openAction(b.dataset.actionId);case 'confirm-action':{const a=C.actions.find(x=>x.id===b.dataset.actionId);return actionForm(a);}case 'close-dialog':closeLayer();break;case 'table-prev':case 'table-next':{const table=tableWindow();C.tablePage=Math.max(0,Math.min(table.page+(action==='table-next'?1:-1),table.pages-1));C.selected=C.page.Rows[C.tablePage*100]?.ID||'';C.restoreScroll=$('#page-scroll').scrollTop;await loadView();$('.table-scroll')?.scrollTo?.({top:0});break;}case 'clear-query':C.query='';C.selected='';await loadView();break;case 'apply-theme':{if(!C.themePreview?.settings)throw new Error(t("Сначала загрузите палитру из игры."));b.disabled=true;try{await call('theme-apply',{values:C.themePreview.settings});closeLayer();await loadView();toast(t("Палитра применена"));}catch(error){b.disabled=false;throw error;}break;}case 'sidebar':C.collapsed=!C.collapsed;storage.set('sidebar-collapsed',C.collapsed);await call('desktop',{...context(),values:{sidebarCollapsed:C.collapsed}});$('.sidebar').classList.toggle('collapsed',C.collapsed);break;case 'inspector':C.inspector=!C.inspector;storage.set('inspector',C.inspector);await call('desktop',{...context(),values:{inspectorOpen:C.inspector}});C.restoreScroll=$('#page-scroll').scrollTop;renderPage();break;case 'command':case 'all-sections':showCommand();break;case 'customize':customize();break;case 'save-menu':C.nav=[...C.nav.filter(id=>!['work','fleet','knowledge'].includes(window.NavigationUI?.groupFor(id))&&id!=='settings'),...$$('[data-nav-check]:checked').map(x=>x.dataset.navCheck),'settings'];await saveNavigation();closeLayer();renderNavigation();toast(t("Меню сохранено"));break;case 'nav-up':case 'nav-down':{const row=b.closest('.customize-item');const sibling=action==='nav-up'?row.previousElementSibling:row.nextElementSibling;if(sibling){action==='nav-up'?sibling.before(row):sibling.after(row);}break;}case 'overlay':{const enabled=!C.state.settings?.overlayEnabled;await call('settings',{...context(),values:{overlayEnabled:enabled}});if(enabled&&C.collapsed){C.collapsed=false;storage.set('sidebar-collapsed',false);$('.sidebar')?.classList.remove('collapsed');await call('desktop',{...context(),values:{sidebarCollapsed:false}});}await loadView();break;}case 'settings-nav':await navigate('settings');break;case 'help':dialog(t("Как пользоваться · ")+navTitle(C.tab),`<div class="prose-small">${esc(C.help||t("Выберите раздел в основном меню."))}</div>`,btn(t("Игровые гайды"),'navigate-learning','graduation-cap'));break;case 'page-menu':{const extra=extraActions(b);if(extra.length)menu(extra.map(a=>({id:a.id,title:a.title,disabled:a.disabled,icon:a.kind==='export'?'external-link':'chevron-right'})),b);break;}case 'file-menu':menu([{title:t("Перечитать журналы"),action:'refresh',icon:'refresh-cw'},{separator:true},{title:t("Завершить ED Copilot"),action:'quit',icon:'log-out'}],b);break;case 'view-menu':menu([{title:t("Настроить меню"),action:'customize',icon:'ellipsis'},{title:t("Свернуть боковое меню"),action:'sidebar',icon:'panel-left'},{title:t("Полный экран"),action:'fullscreen',icon:'maximize-2'},{title:t("Вернуться в окно"),action:'windowed',icon:'layout-grid'}],b);break;case 'fullscreen':case 'windowed':await call('window',{action});break;case 'quit':await call('window',{action:'quit'});break;case 'refresh':{const previous=C.tab;await navigate('settings');const a=C.actions.find(a=>/rescan/.test(a.id));if(a)await submitAction(a.id,{});await navigate(previous);break;}case 'copy-details':await call('native',{action:'copy',text:C.page.Rows.find(r=>r.ID===C.selected)?.Details||''});toast(t("Подробности скопированы"));break;case 'back':{const h=history.pop();if(h)await navigate(h.tab,{mode:h.mode,back:true});break;}default:await handleLearningClick(action,b);}}
let queryTimer;document.addEventListener('input',event=>{if(window.PoliticsUI?.handleInput?.(event,C,uiContext()))return;if(window.ExobiologyUI?.handleInput?.(event,C,uiContext()))return;if(window.ReferenceUI?.handleInput?.(event,C,uiContext()))return;if(window.CarrierTradeUI?.handleInput?.(event,C,uiContext()))return;if(window.DealsUI?.handleInput?.(event,C,uiContext()))return;if(window.ColonyWorkspaceUI?.handleInput?.(event,C,uiContext()))return;if(event.target.id==='command-query')filterCommand(event.target.value);if(event.target.id==='table-query'){C.query=event.target.value;clearTimeout(queryTimer);queryTimer=setTimeout(async()=>{const position=event.target.selectionStart;await loadView();const input=$('#table-query');if(input){input.focus();input.setSelectionRange(position,position);}},230);}});
document.addEventListener('change',async event=>{try{if(C.tab==='politics'&&await window.PoliticsUI?.handleChange?.(event,C,uiContext()))return;if(event.target?.id==='auto-update-enabled'&&await window.UpdatesUI?.handleChange?.(event,C,uiContext()))return;if(await window.ExobiologyUI?.handleChange?.(event,C,uiContext()))return;if(await window.ReferenceUI?.handleChange?.(event,C,uiContext()))return;if(await window.CarrierTradeUI?.handleChange?.(event,C,uiContext()))return;if(await window.DealsUI?.handleChange?.(event,C,uiContext()))return;if(window.ColonyWorkspaceUI?.handleChange?.(event,C,uiContext()))return;if(event.target.id==='route-auto-copy'){await call('settings',{...context(),values:{routeAutoCopyNext:event.target.checked}});await loadView();return;}if(event.target.id==='main-game-palette'){await call('desktop',{...context(),values:{useGamePalette:event.target.checked}});await loadView();}if(event.target.id==='activity-select'){await call('settings',{...context(),values:{activity:event.target.value}});await loadView();}if(['colony-select','overlay-colony'].includes(event.target.id)){await selectColony(event.target.value);return;}if(event.target.matches('[data-calc-service]'))await calculatorChange(event.target.dataset.calcService,event.target.value);}catch(error){toast(error.message,true);}});
document.addEventListener('focusout',event=>{if(!C.activityRefreshPending||!event.target?.closest?.('#activity-strip'))return;Promise.resolve().then(()=>{if(C.activityRefreshPending&&!document.activeElement?.closest?.('#activity-strip'))renderActivity();});});
document.addEventListener('mousedown',event=>{if(event.button===0&&event.target.closest('[data-drag]')&&!event.target.closest('button,input,select,textarea')){event.preventDefault();call('window',{action:'drag'}).catch(()=>{});}});
document.addEventListener('dblclick',event=>{if(event.target.closest('[data-drag]')&&!event.target.closest('button'))call('window',{action:'maximize'});});
document.addEventListener('keydown',event=>{if(event.key==='Escape'){if(window.CSS?.supports?.('selector(select:open)')&&$('select:open'))return;closeLayer();$('.menu')?.remove();}if((event.ctrlKey||event.metaKey)&&(event.code==='KeyK'||event.key.toLowerCase()==='k')){event.preventDefault();showCommand();}if(event.key==='F11'){event.preventDefault();call('window',{action:'toggle-fullscreen'}).catch(error=>toast(error.message,true));}if(event.key==='Enter'&&event.ctrlKey&&C.lesson)handleLearningClick('lesson-complete',null);});
function smokePaintOpportunity(){return new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));}
async function smokeCapture(name){if(!window.COPILOT_SMOKE)return;await smokePaintOpportunity();await new Promise(resolve=>setTimeout(resolve,120));return call('smoke-capture',{name});}
async function smokeMeasure(timings,category,name,work){
 const start=performance.now();
 await work();
 timings.push({category,name,milliseconds:Math.round((performance.now()-start)*100)/100});
}
async function smokeInteractions(){
 const checks=[],timings=[];
 const measured=(name,work)=>smokeMeasure(timings,'window',name,work);
 const settled=async(before)=>{const deadline=performance.now()+6000;while(sequence===before||pending.size){if(performance.now()>deadline)throw new Error(t("Интерфейс не ответил на нажатие: ")+(timings.at(-1)?.name||t("кнопка")));await new Promise(resolve=>setTimeout(resolve,15));}};
 await call('window',{action:'restore'});
 for(let index=0;index<3;index++){
  await measured(t("Развернуть окно ")+(index+1),async()=>{const before=sequence;$('.titlebar [data-window="maximize"]').click();await settled(before);const state=await call('window',{action:'status'});if(!state.maximized)throw new Error(t("Кнопка разворачивания не развернула окно"));});
  await measured(t("Восстановить окно ")+(index+1),async()=>{const before=sequence;$('.titlebar [data-window="maximize"]').click();await settled(before);const state=await call('window',{action:'status'});if(state.maximized)throw new Error(t("Повторное нажатие не восстановило окно"));});
 }
 await measured(t("Свернуть и вернуть окно"),async()=>{await call('window',{action:'minimize'});const state=await call('window',{action:'status'});if(!state.minimized)throw new Error(t("Окно не свернулось"));await call('window',{action:'restore'});});
 for(const expected of ['fullscreen','windowed'])await measured('F11: '+expected,async()=>{const before=sequence;document.dispatchEvent(new KeyboardEvent('keydown',{key:'F11',bubbles:true}));await settled(before);const state=await call('window',{action:'status'});if(state.displayMode!==expected)throw new Error(t("F11 не переключил режим окна"));});
 for(let repeat=0;repeat<4;repeat++)for(const id of ['settings','learning','overview']){
  if(!$('[data-nav="'+id+'"]')&&window.NavigationUI?.groupFor(id))await selectGroup(window.NavigationUI.groupFor(id));
  const node=$('[data-nav="'+id+'"]')||(id==='settings'?$('.rail [data-click="settings-nav"]'):null);
  if(!node)throw new Error(t("Кнопка раздела отсутствует"));
  await smokeMeasure(timings,'navigation',id,async()=>{
   const before=sequence;node.click();await settled(before);await smokePaintOpportunity();
   if(C.tab!==id||!C.page||$('#page')?.hasAttribute('aria-busy'))throw new Error(t("Раздел не переключился"));
  });
 }
 checks.push({name:t("Настоящие обработчики кнопок: разворачивание, восстановление, сворачивание, F11 и разделы"),ok:true});
 checks.push({name:t("Оконные команды отвечают без зависания"),ok:timings.filter(row=>row.category==='window').every(row=>row.milliseconds<5000)});
 return {checks,timings};
}
function uiContext(){return {t,locale,plural:window.CopilotI18n?.plural,date:window.CopilotI18n?.date,esc,icon,fmt,money,metric,btn,badge,heading,renderModes,handleClick,call,toast,loadView,navigate,renderPage,dialog,closeLayer,$,$$,context,applyPalette,smokeCapture};}
function renderLearning(){return window.AcademyUI?window.AcademyUI.render(C,uiContext()):heading(t("Академия"),t("Учебная библиотека подготавливается."));}
function renderCalculator(){return window.CalculatorUI?window.CalculatorUI.render(C,uiContext()):renderGeneral();}
async function handleLearningClick(action,b){if(window.CalculatorUI&&await window.CalculatorUI.handle(action,b,C,uiContext()))return;if(window.AcademyUI)await window.AcademyUI.handle(action,b,C,uiContext());}
async function calculatorChange(id,value){if(window.CalculatorUI)await window.CalculatorUI.change(id,value,C,uiContext());}
async function boot(){
 try{
  await call('window',{action:'ready'});
  const data=await call('boot');
  C.boot=data;C.tabs=data.tabs.filter(tab=>!['account','integrations'].includes(tab.ID));
  C.nav=(data.navigation?.tabs||[]).filter(id=>C.tabs.some(tab=>tab.ID===id));
  C.state=data.state;applyLanguage();
  if(typeof C.state.desktop?.sidebarCollapsed==="boolean")C.collapsed=C.state.desktop.sidebarCollapsed;
  if(typeof C.state.desktop?.inspectorOpen==="boolean")C.inspector=C.state.desktop.inspectorOpen;
  shell();window.TooltipUI?.init?.();window.SystemSearchUI?.init?.(C,uiContext());window.AcademyUI?.init?.(C,uiContext());window.CalculatorUI?.init?.(C,uiContext());
  await loadView();
  if(window.COPILOT_SMOKE){
   if(!window.COPILOT_README&&C.page){await smokePaintOpportunity();await call('smoke-performance',{checkpoint:'firstView'});}
   setTimeout(()=>window.COPILOT_README?window.ReadmeCapture.run(C,uiContext()):smoke(),300);
  }else{
   setInterval(()=>{if(!C.layer&&!C.lesson&&(C.needsRefresh||Date.now()-C.lastFetch>4500))loadView(true);},1500);
  }
 }catch(error){
  $('#app').innerHTML=`<div class="boot"><strong>${th("Не удалось открыть рабочее место")}</strong><span>${esc(error.message)}</span><button class="pill" data-window="quit">${th("Закрыть приложение")}</button></div>`;
 }
}
async function smoke(){
 const scenarios=[],forms=[],interactionTimings=[],add=(name,ok)=>scenarios.push({name,ok:!!ok});
 let capturedForm=false;
 try{
  add('Initial workspace rendered',!!C.page&&!$('#page')?.hasAttribute('aria-busy'));
  const interactions=await smokeInteractions();scenarios.push(...interactions.checks);interactionTimings.push(...interactions.timings);
  await call('smoke-performance',{checkpoint:'afterInteractions'});
  for(const tab of C.tabs){
   await navigate(tab.ID);add(t("Раздел: ")+tab.Title,!!$('#page h1')||tab.ID==='learning');
   if(['overview','learning','colonization','settings'].includes(tab.ID))await smokeCapture(({learning:'academy'})[tab.ID]||tab.ID);
   const modes=[...(C.page.Modes?.length?C.page.Modes:[''])];
   for(const mode of modes){
    C.mode=mode;C.selected='';await loadView();add(tab.ID+' / '+(mode||'Основной экран'),referenceActive()||C.tab==='exobiology'||window.EngineeringRoadmapUI?.active(C)?!!$('#page h1'):!!C.page.Columns?.length);
    if(tab.ID==='colonization'&&mode==='Строительство'){
     const building=$('.colony-building-cost'),table=$('.colony-construction-main>.table-panel');
     if(building&&table){const gap=building.getBoundingClientRect().top-table.getBoundingClientRect().bottom;add('Construction requirements follow the queue without a large empty gap',gap>=0&&gap<=30);}
     await smokeCapture('construction');
    }
    if(tab.ID==='exobiology'&&mode==='Обзор'){const survey=$('.exo-system-progress'),cards=$('.exo-overview-grid');add('Exobiology survey card has visible spacing',!!survey&&!!cards&&cards.getBoundingClientRect().top-survey.getBoundingClientRect().bottom>=20);survey?.scrollIntoView({block:'start'});await smokeCapture('exobiology');$('#page-scroll').scrollTop=0;}
    if(tab.ID==='carrier-calculator'&&mode==='Сводка'){
     await smokeCapture('calculator');const original=C.state.carrierCalculator?.selectedType||'fleet';
     await call('calculator',{values:{action:'select',carrierType:'squadron'}});await loadView();await smokeCapture('calculator-squadron');
     await call('calculator',{values:{action:'select',carrierType:original}});await loadView();
    }
    if(tab.ID==='overview'&&mode==='Обзор')add(t("Переключение экранов обзора"),$$('#page [data-mode]').length===C.page.Modes.length&&!C.page.Modes.includes('Состояние'));
    const actions=[...C.actions];
    const more=$('#page [data-click="page-menu"]');
    if(more){const inline=new Set($$('#page [data-action-id]').map(node=>node.dataset.actionId));await handleClick('page-menu',more);const extra=$$('.menu [data-action-id]');
     add('Only additional actions: '+tab.ID+' / '+mode,extra.length>0&&extra.every(item=>!inline.has(item.dataset.actionId)));$('.menu')?.remove();}
    add('No duplicate toolbar menu: '+tab.ID+' / '+mode,!$('#page-toolbar [data-click="page-menu"]'));
    for(const action of actions){
     if(!action.fields?.length){if(action.confirm&&!action.disabled){await openAction(action.id);add(t("Подтверждение: ")+action.id,!!$('[data-click="confirm-action"]'));closeLayer();}continue;}
     const record={tab:tab.ID,mode,action:action.id,title:action.title,fieldKeys:action.fields.map(field=>field.key),disabled:!!action.disabled,opened:false,cancelled:false};forms.push(record);
     if(action.disabled){await openAction(action.id);add(t("Недоступная форма не открывается: ")+action.id,!$('#action-form'));continue;}
     if(action.confirm){await openAction(action.id);add(t("Подтверждение формы: ")+action.id,!!$('[data-click="confirm-action"]'));closeLayer();}
     await smokeMeasure(interactionTimings,'formOpen',action.id,async()=>{await actionForm(action);await smokePaintOpportunity();});const form=$('#action-form');record.opened=!!form;
     if(!capturedForm&&tab.ID==='colonization'&&mode==='Строительство'&&action.id==='colony.edit'){
      capturedForm=true;
      const input=$('input[type="text"]',form);
      if(!input)throw new Error('Synthetic construction form has no text field');
      input.focus();input.value='Synthetic construction draft';input.setSelectionRange(2,8,'backward');
      const before=sequence;
      C.needsRefresh=true;
      await loadView(true,{smokeProbe:true});
      add('Quiet refresh preserves open form, draft and selection',!document.hidden&&sequence===before&&C.needsRefresh&&C.layer&&$('#action-form')===form&&document.activeElement===input&&input.value==='Synthetic construction draft'&&input.selectionStart===2&&input.selectionEnd===8&&input.selectionDirection==='backward');
      input.value=input.defaultValue;
      await smokeCapture('form');
     }
     add(t("Поля формы: ")+tab.ID+' / '+mode+' / '+action.id,!!form&&action.fields.every(field=>[...form.elements].some(element=>element.name===field.key)));
     add(t("Отмена формы: ")+action.id,!!$('.dialog [data-click="close-dialog"]'));
     await smokeMeasure(interactionTimings,'formCancel',action.id,async()=>{$('.dialog [data-click="close-dialog"]')?.click();await smokePaintOpportunity();});record.cancelled=!$('#action-form')&&!C.layer;add(t("Форма закрыта: ")+action.id,record.cancelled);
    }
    const rows=C.page.Rows||[];
    if(rows.length>1){C.selected=rows[1].ID;await loadView();add(t("Выбор второй строки: ")+tab.ID+' / '+mode,C.selected===rows[1].ID);}
   }
   if(tab.ID==='deals'&&window.DealsUI?.smoke){scenarios.push(...await window.DealsUI.smoke(C,uiContext()));}if(tab.ID==='carrier-trade'&&window.CarrierTradeUI?.smoke){scenarios.push(...await window.CarrierTradeUI.smoke(C,uiContext()));}if(tab.ID==='learning'&&window.AcademyUI?.smoke){const extra=await window.AcademyUI.smoke(C,uiContext());scenarios.push(...extra);}
   if(tab.ID==='engineering'&&window.EngineeringRoadmapUI?.smoke)scenarios.push(...await window.EngineeringRoadmapUI.smoke(C,uiContext()));
   if(tab.ID==='carrier-calculator'&&window.CalculatorUI?.smoke){const extra=await window.CalculatorUI.smoke(C,uiContext());scenarios.push(...extra);}
  }
  customize();add(t("Редактор бокового меню"),$$('[data-nav-check]').length>0&&!$('[data-nav-check=account]')&&!$('[data-nav-check=integrations]'));closeLayer();
  showCommand();filterCommand(t("торг"));add(t("Поиск разделов"),$$('.command-item').length>0);closeLayer();
  const collapsed=C.collapsed;C.collapsed=true;$('.sidebar').classList.add('collapsed');add(t("Меню сворачивается"),$('.sidebar').classList.contains('collapsed'));C.collapsed=collapsed;$('.sidebar').classList.toggle('collapsed',collapsed);
  add(t("Встроенная верхняя панель"),!!$('.titlebar [data-window="maximize"]'));
  add(t("Скроллбары оформлены"),getComputedStyle($('#page-scroll')).scrollbarWidth==='thin');
  add(t("Нет внешних UI-зависимостей"),[...document.scripts].every(script=>!script.src));
  const originalTheme=JSON.stringify(C.state.theme);previewTheme({settings:{preset:'blue'},palette:{accent:'#28a0f0',text:'#eeeeee',panel:'#172534'},source:t("Проверка предварительного просмотра")});
  add(t("Палитра требует явного применения"),!!$('[data-click="apply-theme"]')&&JSON.stringify(C.state.theme)===originalTheme);closeLayer();
  if(window.V27Smoke)scenarios.push(...await window.V27Smoke.run(C,uiContext()));if(!window.V26Smoke)throw Error("Commander scenario missing");scenarios.push(...await window.V26Smoke.run(C,uiContext()));if(window.V24Smoke)scenarios.push(...await window.V24Smoke.run(C,uiContext()));if(window.V23Smoke)scenarios.push(...await window.V23Smoke.run(C,uiContext()));if(window.V22Smoke)scenarios.push(...await window.V22Smoke.run(C,uiContext()));
  await navigate('overview');
  try{await call('smoke-viewport',{compact:true});await smokeCapture('overview-900');}finally{await call('smoke-viewport',{compact:false});}
  await call('smoke-result',{ok:scenarios.every(item=>item.ok),scenarios,forms,interactionTimings,problems:scenarios.filter(item=>!item.ok).map(item=>item.name)});
 }catch(error){await call('smoke-result',{ok:false,scenarios,forms,interactionTimings,problems:[error.message]});}
}
window.addEventListener('error',event=>{if(window.COPILOT_SMOKE)call('smoke-result',{ok:false,scenarios:[],problems:[event.message]}).catch(()=>{});});
window.Copilot={state:C,call,navigate,loadView,openAction,smoke};
boot();
})();
