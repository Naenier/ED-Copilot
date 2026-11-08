'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const catalogue=Object.assign({},...fs.readdirSync(path.join(__dirname,'../localization')).filter(name=>name.endsWith('_en.json')).sort().map(name=>JSON.parse(fs.readFileSync(path.join(__dirname,'../localization',name),'utf8'))));
const i18nSource=fs.readFileSync(path.join(__dirname,'i18n.js'),'utf8');
function inject(context,language='en'){
 context.window||={};context.window.CopilotTranslations=catalogue;
 vm.runInNewContext(i18nSource,context);context.window.CopilotI18n.setLanguage(language);return context.window.CopilotI18n;
}
// Reuse the meaningful domain fixtures, but inject the real translator into each
// sandbox before loading a production module. Registering their tests is disabled.
function domainFixture(name,exportName='fixture'){
 const realVM=require('node:vm'),contexts=[];
 const adapted={...realVM,createContext(context,...args){inject(context);contexts.push(context);return realVM.createContext(context,...args);},runInNewContext(source,context,...args){inject(context);contexts.push(context);return realVM.runInNewContext(source,context,...args);}};
 const module={exports:{}};
 const sandbox={module,exports:module.exports,__dirname,setTimeout,clearTimeout,Date,Intl,require:id=>id==='node:test'?()=>{}:id==='node:vm'?adapted:require(id)};
 realVM.runInNewContext(fs.readFileSync(path.join(__dirname,name+'.test.cjs'),'utf8')+'\nmodule.exports='+exportName+';',sandbox);
 const result=module.exports();return {...result,contexts};
}
test('explicit translation defaults to English and preserves unknown player text and interpolation values',()=>{
 const context={window:{CopilotTranslations:catalogue},document:{documentElement:{}}};vm.runInNewContext(i18nSource,context);const api=context.window.CopilotI18n;
 assert.equal(api.language,'en');assert.equal(api.t('Настройки'),'Settings');assert.equal(api.setLanguage('en'),false);assert.equal(context.document.documentElement.lang,'en');
 assert.equal(api.t('Настройки'),'Settings');assert.equal(api.t('CMDR Журнал: <private>'),'CMDR Журнал: <private>');
 context.window.CopilotTranslations['Осталось {count}: {name}']='Remaining {count}: {name}';
 assert.equal(api.t('Осталось {count}: {name}',{count:5,name:'Система <Sol>'}),'Remaining 5: Система <Sol>');
 assert.equal(api.t('Неизвестный ключ {missing}'),'Неизвестный ключ {missing}');
 api.setLanguage('unsupported');assert.equal(api.language,'ru');assert.equal(context.document.documentElement.lang,'ru');
});
test('cached Intl formatting follows the active locale and handles zero and unknown values honestly',()=>{
 const api=inject({window:{}});assert.equal(api.number(null),'—');assert.equal(api.number(''),'—');assert.equal(api.number(0),'0');assert.equal(api.number(12345.5),'12,345.5');
 const forms={ru:{one:'{count} урок',few:'{count} урока',many:'{count} уроков',other:'{count} урока'},en:{one:'{count} lesson',other:'{count} lessons'}};
 assert.equal(api.plural(1,forms),'1 lesson');assert.equal(api.plural(2,forms),'2 lessons');api.setLanguage('ru');assert.equal(api.plural(1,forms),'1 урок');assert.equal(api.plural(2,forms),'2 урока');assert.equal(api.plural(5,forms),'5 уроков');assert.match(api.number(12345.5),/12\s345,5/);
 assert.equal(api.date('not-a-date'),'—');api.setLanguage('en');assert.equal(api.date('2026-11-08T12:00:00Z',{timeZone:'UTC',year:'numeric',month:'2-digit',day:'2-digit'}),'08/11/2026');
});
test('all explicit static frontend phrases have an English catalogue entry',()=>{
 const names=['app','academy','calculator','overview','reference','carrier-trade','settings-pane','deals','route-workspace','colonization-workspace'];
 const missing=[];
 for(const name of names){const source=fs.readFileSync(path.join(__dirname,name+'.js'),'utf8');for(const match of source.matchAll(/\b(?:t|th)\(("(?:[^"\\]|\\.)*")\)/g)){const key=JSON.parse(match[1]);if(/[А-Яа-яЁё]/.test(key)&&!Object.hasOwn(catalogue,key))missing.push(name+': '+key);}}
 assert.deepEqual(missing,[]);
 for(const[key,value]of Object.entries(JSON.parse(fs.readFileSync(path.join(__dirname,'../localization/frontend_en.json'),'utf8'))))assert.doesNotMatch(value,/[А-Яа-яЁё]/,key);
});
test('calculator and carrier trading translate labels while mode IDs and numeric drafts stay stable',()=>{
 const calc=domainFixture('calculator');let html=calc.api.render(calc.C,calc.ui);assert.match(html,/Personal carrier/);assert.match(html,/Save parameters/);assert.match(html,/Weekly jumps/);assert.match(html,/data-mode="Сводка"/);assert.doesNotMatch(html,/data-mode="Summary"/);
 calc.C.calculatorDrafts.fleet.values.tritiumPrice='12 345,5';calc.C.calculatorDrafts.fleet.dirty=true;html=calc.api.render(calc.C,calc.ui);assert.match(html,/value="12 345,5"/);assert.equal(calc.C.mode,'Сводка');
 const carrier=domainFixture('carrier-trade');html=carrier.api.render(carrier.C,carrier.ui);assert.match(html,/Where to buy/);assert.match(html,/Where to sell/);assert.match(html,/Carrier trading/);assert.match(html,/data-mode="Рейс"/);assert.doesNotMatch(html,/data-mode="Trip"/);assert.match(html,/Synthetic Origin/);
});
test('trade search, colony workflow and settings help render English controls without translating player values',()=>{
 const deals=domainFixture('deals');deals.C.dealsUI=null;deals.C.state.commander.system='Система Настройки <A>';let html=deals.api.render(deals.C,deals.u);assert.match(html,/Free hold, t/);assert.match(html,/Find trips/);assert.match(html,/Система Настройки &lt;A&gt;/);assert.match(html,/data-copy="system"/);assert.match(html,/title="Copy system/);
 const colony=domainFixture('colonization-workspace');colony.C.actions.push({id:'colony.search'});colony.C.state.commander.system='Торговля';html=colony.api.render(colony.C,colony.ui);assert.match(html,/Plan a new colony/);assert.match(html,/Check system/);assert.match(html,/value="Торговля"/);
 const settings=domainFixture('settings-pane');html=settings.api.render(settings.C,settings.ui,'<div>controls</div>');assert.match(html,/Settings details/);assert.match(html,/Workspace/);assert.doesNotMatch(html,/Подробности настроек|Рабочее пространство/);
});
test('Academy translates reader navigation and labels while preserving lesson content',()=>{
 const academy=domainFixture('academy','runtime');let html=academy.api.render(academy.C,academy.ui);assert.match(html,/All courses/);assert.match(html,/Bookmarks/);assert.match(html,/Find a guide/);assert.match(html,/Первый курс/);
 academy.C.lesson='first.practice';html=academy.api.render(academy.C,academy.ui);assert.match(html,/Course lessons/);assert.match(html,/Practice/);assert.match(html,/Continue practising in the app/);assert.match(html,/Создайте свой план/);assert.match(html,/data-id="sale"/);
});
test('reference navigation translates category labels and loading errors without touching entity names',async()=>{
 const ref=domainFixture('reference');let html=ref.api.render(ref.C,ref.ui);assert.match(html,/Search this section/);assert.match(html,/Ships/);await ref.api.init(ref.C,ref.ui);html=ref.api.render(ref.C,ref.ui);assert.match(html,/Тестовый корабль/);assert.match(html,/Open page/);
 ref.setRPC(async()=>{throw new Error('Synthetic network failure');});await ref.api.open('ship/other',ref.C,ref.ui);html=ref.api.render(ref.C,ref.ui);assert.match(html,/Could not open the page/);assert.match(html,/Try again/);assert.match(html,/Synthetic network failure/);
});
test('English table captions retain entity links and option values remain exact player data',async()=>{
 const f=domainFixture('app'),a=f.api;
 a.C.state={settings:{language:'en'}};a.C.tab='route';
 const system='Настройки <System>',data={system,name:system,key:'carbon',moduleKey:'am',blueprintKey:'AFM_Shielded',experimentalKey:'test'};
 for(const[column,id]of [['System','system/'+encodeURIComponent(system)],['Material','material/carbon'],['Module','module/am'],['Blueprint','blueprint/AFM_Shielded'],['Experimental effect','experimental/test'],['Commodity','commodity/carbon']])assert.ok(a.referenceCell({Data:data},column,'Настройки').includes('data-reference-id="'+id+'"'),column);
 assert.match(a.referenceCell({Data:data},'System',system),/Настройки &lt;System&gt;/);
 a.C.actions=[{id:'settings.language',fields:[{key:'language',kind:'select',label:'Interface language',choices:[{Value:'ru',Label:'Русский'},{Value:'en',Label:'English'}]}]}];
 assert.match(a.renderSettings(),/data-action-id="settings.language"/);
 await a.actionForm({id:'goal.edit',title:'Edit objective',fields:[{key:'goalId',kind:'select',label:'Objective',default:'goal-1',choices:[{Value:'goal-1',Label:'Настройки'}]},{key:'title',kind:'text',label:'Name',default:'Настройки'}]});
 assert.match(f.dialogs.at(-1).body,/<option value="goal-1"[^>]*>Настройки<\/option>/);assert.match(f.dialogs.at(-1).body,/value="Настройки"/);
});
test('live language switching preserves typed drafts, focus, selection and independent module sessions',()=>{
 const f=domainFixture('app'),a=f.api,i18n=f.window.CopilotI18n;i18n.setLanguage('ru');
 Object.assign(a.C,{tab:'deals',mode:'Поиск',query:'Настройки',selected:'saved-player-route',page:{Rows:[]},state:{settings:{language:'ru'}}});
 const node={id:'table-query',name:'',tagName:'INPUT',type:'text',value:'12 345,5 · Настройки',checked:false,dataset:{},inPage:true,selectionStart:3,selectionEnd:8,selectionDirection:'forward',focus(){this.focused=true;},setSelectionRange(start,end){this.range=[start,end];}};
 f.nodes.set('table-query',node);f.selectors.set('#page input,#page select,#page textarea',[node]);f.document.activeElement=node;
 let shells=0,renders=0;a.setShell(()=>{shells++;node.value='reset';});a.setRender(()=>{renders++;node.value='reset';});
 a.C.referenceSessions={library:{query:'user query',group:'custom',entryId:'ship/test',loadedKey:'old',listToken:2,entryToken:3,list:{},entry:{},listPromise:Promise.resolve({})}};
 f.window.copilotReceive({event:'update',data:{settings:{language:'en'},connection:{},commander:{},revision:2}});
 assert.equal(i18n.language,'en');assert.equal(f.document.documentElement.lang,'en');assert.equal(shells,1);assert.equal(renders,1);assert.equal(node.value,'12 345,5 · Настройки');assert.deepEqual(node.range,[3,8]);assert.equal(node.focused,true);
 assert.equal(a.C.mode,'Поиск');assert.equal(a.C.query,'Настройки');assert.equal(a.C.selected,'saved-player-route');assert.equal(a.C.referenceSessions.library.query,'user query');assert.equal(a.C.referenceSessions.library.entryId,'ship/test');assert.equal(a.C.referenceSessions.library.loadedKey,'');assert.equal(a.C.referenceSessions.library.listPromise,null);
});
test('an incoming language change waits for the open form to close',()=>{
 const f=domainFixture('app'),a=f.api,i18n=f.window.CopilotI18n;i18n.setLanguage('ru');a.C.state={settings:{language:'ru'}};a.C.page={};a.C.layer=true;let rebuilds=0;a.setShell(()=>rebuilds++);a.setRender(()=>{});
 f.window.copilotReceive({event:'update',data:{settings:{language:'en'},connection:{},commander:{},revision:3}});
 assert.equal(i18n.language,'ru');assert.equal(rebuilds,0);assert.equal(a.C.pendingLanguage,'en');a.closeLayer();assert.equal(i18n.language,'en');assert.equal(rebuilds,1);assert.equal(a.C.layer,false);
});
test('English overview and route workspace keep commander, objective and system names unchanged',async()=>{
 const f=domainFixture('app'),a=f.api;Object.assign(a.C,{tab:'overview',mode:'Планы',page:{Modes:['Планы','Состояние'],Rows:[]},state:{settings:{language:'en'},commander:{name:'Настройки',shipName:'Торговля',system:'Справка',ship:'Python',docked:false},objectives:[{title:'Обзор',system:'Колонизация'}],carrier:{name:'Цели',owned:true}}});
 let html=a.renderOverview();assert.match(html,/What shall we do today/);assert.match(html,/CMDR Настройки/);assert.match(html,/>Справка<\/h2>/);assert.match(html,/>Обзор<\/strong>/);assert.match(html,/«Торговля»/);assert.match(html,/data-mode="Планы"/);assert.match(html,/>Plans<\/button>/);
 const route=domainFixture('route-workspace');route.C.state.carrierRoute={status:'ready',route:{sourceSystem:'Настройки',destinationSystem:'Обзор',stops:[{name:'Настройки',visited:true},{name:'Торговля',visited:false,distanceLy:400}]},summary:{totalJumps:1,completedJumps:0,remainingJumps:1,totalMinutes:20,estimatedArrival:null}};
 html=route.render();assert.match(html,/Next jump/);assert.match(html,/>Торговля<\/h2>/);assert.match(html,/Copy system/);assert.match(html,/Unknown/);assert.doesNotMatch(html,/Unknown — Unknown/);await route.api.handle('route-workspace-copy',null,route.C,route.u);assert.equal(route.calls[0].values.text,'Торговля');
});
test('the exobiology library uses photos and the bounded page size returned by Go without skipping species',async()=>{
 const ref=domainFixture('reference');let offset=0;
 ref.setRPC(async(op,p)=>{if(op==='reference.entry')return {entry:{id:p.id,category:'exobiology',title:'Bacterium Cerbrus',image:{src:'data:image/webp;base64,YQ==',alt:'Bacterium'},sections:[]}};offset=p.offset;return {categories:[{id:'exobiology',title:'Экзобиология'}],entries:Array.from({length:12},(_,i)=>({id:'exo/'+(p.offset+i),category:'exobiology',title:'Species '+(p.offset+i),image:{src:'data:image/webp;base64,YQ==',alt:'Species photo'}})),total:25,offset:p.offset,limit:12};});
 ref.api.navigate('encyclopedia',{referenceCategory:'exobiology'},ref.C);await ref.api.init(ref.C,ref.ui);let html=ref.api.render(ref.C,ref.ui);assert.match(html,/Exobiology/);assert.match(html,/ref-visual-card/);assert.match(html,/data:image\/webp/);assert.match(html,/1–12/);
 await ref.api.handleClick('reference-next',null,ref.C,ref.ui);assert.equal(offset,12);html=ref.api.render(ref.C,ref.ui);assert.match(html,/13–24/);
 await ref.api.open('exo/12',ref.C,ref.ui);html=ref.api.render(ref.C,ref.ui);assert.match(html,/<h1>Bacterium Cerbrus<\/h1>/);assert.match(html,/loading="eager"/);
});
