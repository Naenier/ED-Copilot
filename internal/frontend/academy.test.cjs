'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function fixture() {
 const courses=[{id:'first',title:'Первый курс',summary:'Понятный учебный путь',category:'Торговля',icon:'arrows',difficulty:'Начальный',minutes:18,completedLessons:0,totalLessons:3,percent:0,lessonIds:['first.basics','first.practice','first.checkpoint'],nextLessonId:'first.basics'},{id:'second',title:'Второй курс',summary:'Развитие пилота',category:'Бой',icon:'target',difficulty:'Средний',minutes:18,completedLessons:0,totalLessons:1,percent:0,lessonIds:['second.basics'],nextLessonId:'second.basics'}];
 const base={courseId:'first',courseTitle:'Первый курс',category:'Торговля',summary:'Краткий план изучения',difficulty:'Начальный',minutes:5,completed:false,bookmarked:false,status:'not-started',position:{},checklistState:{},sourceURLs:['https://pilotstradenetwork.com/guides/ptn-trade-alerts/'],quizResult:{answers:{},answered:0,total:0,correct:0,passed:false}};
 const lessons=[{...base,id:'first.basics',title:'Основы',kind:'reading',order:1,nextLessonId:'first.practice',sections:[{id:'mechanics',title:'Игровая схема',paragraphs:['Пример содержания без разметки.']} ]},{...base,id:'first.practice',title:'Практика',kind:'practice',order:2,previousLessonId:'first.basics',nextLessonId:'first.checkpoint',sections:[{id:'workflow',title:'Порядок',paragraphs:['Создайте свой план.'],appSection:'sale'},{id:'checklist',title:'Проверка',checklist:['Первый пункт','Второй пункт','Третий пункт','Четвёртый пункт','Пятый пункт']}]},{...base,id:'first.checkpoint',title:'Проверка знаний',kind:'checkpoint',order:3,previousLessonId:'first.practice',sections:[{id:'exercise',title:'Задание',paragraphs:['Решите учебную задачу.']},{id:'solution',title:'Разбор',paragraphs:['Условный ответ.']}],quiz:[{id:'one',prompt:'Выберите ответ',choices:['Первый','Второй'],correctIndex:1,explanation:'Верный второй ответ.'},{id:'two',prompt:'Другой вопрос',choices:['Да','Нет'],correctIndex:0,explanation:'Верно: да.'}],quizResult:{answers:{},answered:0,total:2,correct:0,passed:false}},{...base,id:'second.basics',courseId:'second',courseTitle:'Второй курс',category:'Бой',title:'Другая тема',kind:'reading',order:1,sections:[{id:'mechanics',title:'Схема',paragraphs:['Другая схема.']}]}];
 return {tab:'learning',lesson:'',course:'',focus:false,layer:false,state:{learning:{courses,lessons,filteredLessons:lessons,recent:[],progress:{completed:0,total:4,percent:0},options:{filter:'all',query:'',courseId:''},reviewedAt:'2026-11-08'}}};
}

function runtime(C=fixture()) {
 const timers=new Map();let timerID=0;
 const scope=vm.createContext({window:{},setTimeout:(fn,ms)=>{const id=++timerID;timers.set(id,{fn,ms});return id;},clearTimeout:id=>timers.delete(id)});
 vm.runInContext(fs.readFileSync(path.join(__dirname,'academy.js'),'utf8'),scope,{filename:'academy.js'});
 const calls=[],toasts=[],listeners=new Map(),scrollCalls=[];
 const host={scrollTop:120,scrollHeight:1000,clientHeight:400,getBoundingClientRect:()=>({top:0}),addEventListener:(name,fn)=>{if(listeners.has(name))throw Error('duplicate listener');listeners.set(name,fn);}};
 const nodes=new Map([['#page-scroll',host]]),collections=new Map();
 const esc=value=>String(value??'').replace(/[&<>"']/g,ch=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
 const ui={esc,icon:name=>`<svg data-icon="${esc(name)}"></svg>`,fmt:value=>String(value),money:value=>String(value),badge:(label,kind)=>`<span class="badge ${esc(kind||'')}">${esc(label)}</span>`,btn:(label,action,icon,style,attrs)=>`<button data-click="${esc(action)}" ${attrs||''}>${esc(label)}</button>`,heading:(title,description,actions)=>`<header><h1>${esc(title)}</h1><p>${esc(description)}</p>${actions||''}</header>`,call:async(op,body)=>{calls.push({op,body:JSON.parse(JSON.stringify(body))});return{};},toast:(message,error)=>toasts.push({message,error}),loadView:async()=>{calls.push({op:'loadView'});},navigate:async id=>{calls.push({op:'navigate',id});},renderPage:()=>{calls.push({op:'renderPage'});},dialog:(title,body,foot)=>{calls.push({op:'dialog',title,body,foot});},closeLayer:()=>{},$:selector=>nodes.get(selector)||null,$$:(selector,root)=>collections.get(selector)||[]};
 return {C,ui,api:scope.window.AcademyUI,calls,toasts,host,nodes,collections,listeners,scrollCalls,timers,async runTimers(ms){const ready=[...timers].filter(([,timer])=>ms===undefined||timer.ms===ms);for(const[id,timer]of ready){timers.delete(id);await timer.fn();}}};
}

test('catalogue renders courses, progress, filters and categories using escaped data',()=>{
 const r=runtime();const s=r.C.state.learning;
 s.courses[0].title='<img src=x onerror="bad()">';s.courses[0].summary='<script>bad()</script>';s.options.query='"/><svg onload=bad()>';
 const html=r.api.render(r.C,r.ui);
 assert.match(html,/data-academy="catalog"/);assert.equal((html.match(/class="course-card"/g)||[]).length,2);
 assert.match(html,/academy-query/);assert.match(html,/data-click="academy-category"/);assert.match(html,/aria-valuemax="100"/);
 assert.ok(html.includes('&lt;img'));assert.ok(html.includes('&lt;script&gt;'));assert.ok(!html.includes('<img'));assert.ok(!html.includes('<script>'));
 s.options.filter='bookmarked';s.lessons[0].bookmarked=true;s.filteredLessons=[s.lessons[0]];
 const favorites=r.api.render(r.C,r.ui);assert.match(favorites,/academy-library-lesson/);assert.ok(!favorites.includes('Другая тема'));
});

test('reader shows curriculum, structured sections, persisted checklist and concealed solution',()=>{
 const r=runtime();r.C.lesson='first.practice';r.C.course='first';r.C.state.learning.lessons[1].checklistState={'checklist:2':true};
 let html=r.api.render(r.C,r.ui);
 assert.match(html,/reader-layout/);assert.match(html,/reader-toc/);assert.equal((html.match(/data-academy-check=/g)||[]).length,5);
 assert.match(html,/data-index="2"[^>]* checked/);assert.match(html,/data-click="academy-tool" data-id="sale"/);
 r.C.lesson='first.checkpoint';html=r.api.render(r.C,r.ui);
 assert.equal((html.match(/class="academy-question"/g)||[]).length,2);assert.match(html,/<details class="academy-solution">/);assert.ok(!html.includes('quiz-feedback'));
});

test('opening a course or lesson delegates selection and preserves a resumable cursor',async()=>{
 const r=runtime();r.C.state.learning.lessons[0].position={sectionId:'mechanics',ratio:0.6};
 await r.api.handle('academy-course',{dataset:{id:'first'}},r.C,r.ui);
 assert.equal(r.C.lesson,'first.basics');assert.equal(r.C.course,'first');assert.equal(r.C.academy.pendingRestore.ratio,0.6);
 assert.deepEqual(r.calls[0],{op:'learning',body:{values:{action:'select',lessonId:'first.basics'}}});
 assert.equal(r.calls[1].op,'loadView');
 r.api.init(r.C,r.ui);await r.runTimers(0);assert.equal(r.host.scrollTop,360);
 const count=r.calls.length;await r.api.handle('academy-open',{dataset:{id:'missing'}},r.C,r.ui);assert.equal(r.calls.length,count);assert.equal(r.toasts.at(-1).error,true);
});

test('completion, bookmark, filters, focus and app links keep explicit IDs',async()=>{
 const r=runtime();r.C.lesson='first.practice';
 await r.api.handle('lesson-complete',null,r.C,r.ui);
 assert.deepEqual(r.calls[0].body,{values:{action:'complete',lessonId:'first.practice',completed:true}});
 await r.api.handle('academy-bookmark',{dataset:{id:'first.basics'}},r.C,r.ui);
 assert.deepEqual(r.calls.find(call=>call.body?.values.action==='bookmark').body,{values:{action:'bookmark',lessonId:'first.basics',bookmarked:true}});
 await r.api.handle('academy-filter',{dataset:{id:'bookmarked'}},r.C,r.ui);
 assert.ok(r.calls.some(call=>call.body?.values.filter==='bookmarked'));
 await r.api.handle('academy-focus',null,r.C,r.ui);assert.equal(r.C.focus,true);assert.equal(r.C.restoreScroll,120);
 await r.api.handle('academy-tool',{dataset:{id:'sale'}},r.C,r.ui);assert.ok(r.calls.some(call=>call.op==='navigate'&&call.id==='sale'));
 const count=r.calls.length;await r.api.handle('academy-tool',{dataset:{id:'javascript:bad()'}},r.C,r.ui);assert.equal(r.calls.length,count);
 assert.equal(await r.api.handle('unrelated-root-action',null,r.C,r.ui),false);
});

test('quiz sends chosen answers to Go and uses only the backend result',async()=>{
 const r=runtime();r.C.lesson='first.checkpoint';r.nodes.set('[data-academy-quiz]',{});
 r.collections.set('[data-academy-answer]',[{dataset:{academyAnswer:'one'},checked:true,value:'0'},{dataset:{academyAnswer:'two'},checked:true,value:'1'}]);
 r.ui.loadView=async()=>{r.C.state.learning.lessons[2].quizResult={passed:false,correct:0,total:2,answered:2,answers:{one:0,two:1}};};
 await r.api.handle('academy-check',{dataset:{id:'first.checkpoint'}},r.C,r.ui);
 const answers=r.calls.filter(call=>call.op==='learning').map(call=>call.body.values);
 assert.deepEqual(answers,[{action:'answer',lessonId:'first.checkpoint',questionId:'one',choiceIndex:0},{action:'answer',lessonId:'first.checkpoint',questionId:'two',choiceIndex:1}]);
 assert.match(r.toasts.at(-1).message,/Верных ответов: 0 из 2/);
 const html=r.api.render(r.C,r.ui);assert.match(html,/Ответ пока неверный/);assert.ok(!html.includes('Все ответы верны'));
 r.collections.set('[data-academy-answer]',[]);const count=r.calls.length;
 await r.api.handle('academy-check',{dataset:{id:'first.checkpoint'}},r.C,r.ui);assert.equal(r.calls.length,count);assert.match(r.toasts.at(-1).message,/каждый вопрос/);
});

test('owned event handlers bind once and persist a checklist without implicit completion',async()=>{
 const r=runtime();r.C.lesson='first.practice';r.api.init(r.C,r.ui);r.api.init(r.C,r.ui);
 assert.deepEqual([...r.listeners.keys()].sort(),['change','click','input','keydown','scroll']);
 const input={dataset:{academyCheck:'checklist',index:'1',lesson:'first.practice'},checked:true,disabled:false,matches:selector=>selector==='[data-academy-check]'};
 await r.listeners.get('change')({target:input});
 assert.deepEqual(r.calls[0].body,{values:{action:'checklist',lessonId:'first.practice',sectionId:'checklist',index:1,checked:true}});
 assert.equal(r.C.state.learning.lessons[1].checklistState['checklist:1'],true);assert.equal(input.disabled,false);
 assert.ok(!r.calls.some(call=>call.body?.values.action==='complete'));
 let prevented=false;r.listeners.get('click')({target:{closest:()=>({})},preventDefault:()=>{prevented=true;}});assert.equal(prevented,true);
});

test('scroll persistence captures the original lesson and Escape leaves focus mode',async()=>{
 const r=runtime();r.C.lesson='first.practice';r.C.focus=true;
 const section={dataset:{academySection:'workflow'},getBoundingClientRect:()=>({top:50})};
 r.collections.set('[data-academy-section]',[section]);r.api.init(r.C,r.ui);r.listeners.get('scroll')();
 r.C.lesson='second.basics';await r.runTimers(650);
 assert.deepEqual(r.calls[0].body,{values:{action:'position',lessonId:'first.practice',sectionId:'workflow',ratio:0.2}});
 let prevented=false;r.listeners.get('keydown')({key:'Escape',preventDefault:()=>{prevented=true;}});assert.equal(r.C.focus,false);assert.equal(prevented,true);
});

test('search debounce saves a query through the learning API',async()=>{
 const r=runtime();r.api.init(r.C,r.ui);
 const input={id:'academy-query',value:'Робиго',selectionStart:6,focus:()=>{},setSelectionRange:()=>{}};r.nodes.set('#academy-query',input);
 r.listeners.get('input')({target:input});input.value='Робиго пассажиры';r.listeners.get('input')({target:input});
 await r.runTimers(220);
 const updates=r.calls.filter(call=>call.op==='learning');assert.equal(updates.length,1);assert.deepEqual(updates[0].body,{values:{action:'configure',query:'Робиго пассажиры'}});
});
