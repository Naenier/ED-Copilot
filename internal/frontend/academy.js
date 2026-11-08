/* Academy: the Go learning model owns content, answers and personal progress. */
(()=>{
'use strict';
const t=(value,vars)=>window.CopilotI18n?.t(value,vars)??String(value??'');
const th=(value,vars)=>t(value,vars).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const courseIcons={arrows:'arrow-left-right',package:'package',compass:'compass',target:'target',person:'person-standing',users:'users',carrier:'ship',chart:'chart-no-axes-combined',route:'route'};
const filters=[['all','Все курсы'],['in-progress','Продолжить'],['bookmarked','Избранное'],['completed','Изучено']];
const kinds={reading:'Основы',practice:'Практикум',checkpoint:'Самопроверка'};
const sectionNames={sale:'Торговля',overview:'Обзор',route:'Маршрут флотоносца',engineering:'Инженерия',targets:'Цели',deals:'Рейсы','carrier-calculator':'Калькулятор флотоносца','carrier-trade':'Торговля флотоносцем'};
const categoryGroups=[['trading','Торговля',['trade-alerts','wmm']],['expeditions','Экспедиции',['booze-cruise']],['combat','Бой',['afk-combat','operations']],['getting-started','Начало игры',['odyssey']],['fleet-carriers','Флотоносец',['carrier-basics','carrier-trade']],['passengers','Пассажирские рейсы',['robigo']]];
const categoryKey=course=>course?.categoryId||categoryGroups.find(group=>group[2].includes(course?.courseId||course?.id))?.[0]||course?.category||'';
const lessonCount=count=>window.CopilotI18n?.plural(count,{ru:{one:'{count} урок',few:'{count} урока',many:'{count} уроков',other:'{count} урока'},en:{one:'{count} lesson',other:'{count} lessons'}})||count+' уроков';
const bound=(n,min,max)=>Math.max(min,Math.min(max,Number.isFinite(Number(n))?Number(n):min));
const domID=value=>'academy-'+String(value).replace(/[^a-zA-Z0-9_-]/g,'-');
const model=C=>C.state?.learning||{courses:[],lessons:[],filteredLessons:[],recent:[],progress:{},options:{}};
function local(C){return C.academy||(C.academy={category:'',drafts:{},pendingRestore:null,readingID:'',positionTimer:null,searchTimer:null,suppressPosition:false});}
function lessonByID(C,id){return (model(C).lessons||[]).find(item=>item.id===id);}
function courseByID(C,id){return (model(C).courses||[]).find(item=>item.id===id);}
function progress(value,label,ui){const percent=bound(value,0,100);return `<div class="progress-bar" role="progressbar" aria-label="${ui.esc(t(label))}" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${percent}"><span style="width:${percent}%"></span></div>`;}
function status(lesson){return lesson.completed?t("Изучено"):lesson.status==='in-progress'?t("В процессе"):t("Не начато");}
function dateLabel(value){const match=String(value||'').match(/^(\d{4})-(\d{2})-(\d{2})/);if(!match)return '';return window.CopilotI18n?.date?window.CopilotI18n.date(match[0]+'T12:00:00Z',{day:'2-digit',month:'short',year:'numeric',timeZone:'UTC'}):`${match[3]}.${match[2]}.${match[1]}`;}
function lessonsFor(C,courseID){return (model(C).lessons||[]).filter(item=>item.courseId===courseID).sort((a,b)=>Number(a.order)-Number(b.order));}
function nextFor(C,course){const recent=(model(C).recent||[]).find(item=>item.courseId===course.id&&!item.completed);return recent?.id||course.nextLessonId||course.firstLessonId||course.lessonIds?.[0];}
function iconButton(ui,name,action,title,extra=''){return `<button class="icon-button" type="button" data-click="${ui.esc(action)}" title="${ui.esc(title)}" aria-label="${ui.esc(title)}" ${extra}>${ui.icon(name)}</button>`;}

function courseCard(C,course,ui){
 const completed=Number(course.completedLessons)||0,total=Number(course.totalLessons)||0;
 const label=completed===total&&total>0?t("Повторить курс"):completed>0?t("Продолжить курс"):t("Открыть курс");
 return `<button class="course-card" type="button" data-click="academy-course" data-id="${ui.esc(course.id)}" aria-label="${ui.esc(label+': '+course.title)}"><span class="course-top"><span class="course-icon">${ui.icon(courseIcons[course.icon]||'book-open')}</span>${ui.badge(t(course.difficulty||"Учебный курс"),completed===total&&total>0?'good':'')}</span><span class="academy-course-name">${ui.esc(course.title)}</span><span class="academy-course-summary">${ui.esc(course.summary)}</span><span class="course-footer">${ui.icon('clock')} ${ui.esc(course.minutes)}${th(" мин")}<span class="spacer"></span>${ui.esc(completed)} / ${ui.esc(lessonCount(total))}</span>${progress(course.percent,t("Прогресс курса ")+course.title,ui)}<span class="academy-card-cta">${ui.esc(t(label))}${ui.icon('arrow-right')}</span></button>`;
}

function catalog(C,ui){
 const s=model(C),a=local(C),options=s.options||{},filter=options.filter||'all',courses=s.courses||[],lessons=s.lessons||[];
 const currentQuery=typeof a.query==='string'?a.query:(options.query||'');
 const categories=[];for(const course of courses){const key=categoryKey(course);if(key&&course.category&&!categories.some(item=>item[0]===key))categories.push([key,course.category]);}
 if(a.category&&!categories.some(item=>item[0]===a.category)){const old=categoryGroups.find(group=>group[1]===a.category)?.[0]||categoryKey(courses.find(course=>course.category===a.category));a.category=categories.some(item=>item[0]===old)?old:'';}
 const inCategory=lesson=>!a.category||categoryKey(courseByID(C,lesson.courseId)||lesson)===a.category;
 const allowed=new Set((s.filteredLessons||lessons).map(lesson=>lesson.id));
 const shown=courses.filter(course=>(!a.category||categoryKey(course)===a.category)&&lessons.some(lesson=>lesson.courseId===course.id&&allowed.has(lesson.id)));
 const latest=(s.recent||[]).find(lesson=>!lesson.completed)||(s.recent||[])[0];
 const completed=Number(s.progress?.completed)||0,total=Number(s.progress?.total)||lessons.length;
 const bannerTitle=latest?(latest.completed?t("Вернуться к прочитанному"):t("Продолжить с того же места")):'';
 const bannerText=latest?latest.title:'';
 const banner= latest ? `<section class="learning-banner" aria-label="${th("Ваше обучение")}"><div class="large-icon">${ui.icon('graduation-cap')}</div><div class="academy-banner-copy"><h2>${ui.esc(bannerTitle)}</h2><p>${ui.esc(bannerText)}</p><div class="academy-progress-caption">${th("Изучено ")}${ui.esc(completed)}${th(" из ")}${ui.esc(total)}${th(" уроков")}</div>${progress(s.progress?.percent,t("Общий прогресс обучения"),ui)}</div><span class="spacer"></span>${latest?ui.btn(latest.completed?t("Повторить"):t("Продолжить"),'academy-open','arrow-right','primary',`data-id="${ui.esc(latest.id)}"`):ui.btn(t("С чего начать"),'academy-course','arrow-right','primary','data-id="odyssey"')}</section>` : '';
 const tools=`<div class="filters academy-filters"><div class="segmented" role="group" aria-label="${th("Фильтр обучения")}">${filters.map(([id,label])=>`<button type="button" data-click="academy-filter" data-id="${id}" class="${filter===id?'selected':''}" aria-pressed="${filter===id}">${ui.esc(t(label))}</button>`).join('')}</div><label class="search-field" for="academy-query">${ui.icon('search')}<input id="academy-query" type="search" placeholder="${th("Найти гайд, тему или инструмент")}" aria-label="${th("Поиск по гайдам")}" autocomplete="off" value="${ui.esc(currentQuery)}"></label></div>`;
 const topics=`<nav class="academy-topics" aria-label="${th("Темы гайдов")}"><button type="button" data-click="academy-category" data-id="" class="${!a.category?'selected':''}" aria-pressed="${!a.category}">${th("Все темы")}</button>${categories.map(([key,title])=>`<button type="button" data-click="academy-category" data-id="${ui.esc(key)}" class="${a.category===key?'selected':''}" aria-pressed="${a.category===key}">${ui.esc(t(title))}</button>`).join('')}</nav>`;
 const activeCourse=options.courseId?courseByID(C,options.courseId):null;
 const fixedFilter=activeCourse?`<div class="academy-active-filter">${th("Курс: ")}${ui.esc(activeCourse.title)} ${ui.btn(t("Показать все"),'academy-clear','x','quiet')}</div>`:'';
 let body='';
 if(filter==='bookmarked'||filter==='completed'||filter==='in-progress'){
  let list=(s.filteredLessons||[]).filter(inCategory);
  if(filter==='in-progress')list=[...list].sort((x,y)=>String(y.openedAt||'').localeCompare(String(x.openedAt||'')));
  body=list.length?`<div class="academy-lesson-list">${list.map(lesson=>`<button type="button" class="academy-library-lesson" data-click="academy-open" data-id="${ui.esc(lesson.id)}"><span class="academy-list-icon">${ui.icon(lesson.completed?'circle-check':lesson.kind==='checkpoint'?'circle-help':'book-open')}</span><span class="academy-list-copy"><strong>${ui.esc(lesson.title)}</strong><span>${ui.esc(lesson.courseTitle)} · ${ui.esc(t(kinds[lesson.kind]||"Урок"))} · ${ui.esc(lesson.minutes)}${th(" мин")}</span></span>${lesson.bookmarked?ui.icon('bookmark'):''}${ui.badge(status(lesson),lesson.completed?'good':'')}${ui.icon('chevron-right')}</button>`).join('')}</div>`:empty(filter==='bookmarked'?t("В избранном пока нет уроков"):filter==='completed'?t("Изученные уроки появятся здесь"):t("Начните любой курс"),filter==='bookmarked'?t("Откройте урок и нажмите на закладку — он останется под рукой."):t("Выберите тему и откройте первый урок. Прогресс сохраняется отдельно для каждого курса."),ui);
 }else body=shown.length?`<div class="course-grid">${shown.map(course=>courseCard(C,course,ui)).join('')}</div>`:empty(t("Ничего не найдено"),t("Попробуйте другую тему или более короткий поисковый запрос."),ui,true);
 return `<div data-academy="catalog">${ui.heading(t("Академия"),t("Сосредоточьтесь на занятии, которое хотите освоить. Учитесь в своём темпе."),ui.btn(t("Мой прогресс"),'academy-progress','chart-no-axes-combined','quiet'))}${banner}${tools}${topics}${fixedFilter}<div class="section-heading"><h2>${filter==='all'?t("Учебные маршруты"):t(filters.find(item=>item[0]===filter)?.[1]||"Уроки")}</h2><span class="muted academy-count">${ui.esc(filter==='all'?shown.length:(s.filteredLessons||[]).filter(inCategory).length)} ${filter==='all'?t("курсов"):t("уроков")}</span></div>${body}<div class="academy-library-foot">${th("Материалы доступны без интернета · ")}${ui.esc(dateLabel(s.reviewedAt))}${th(" · Практикумы ED Copilot и дополнительные материалы PTN")}</div></div>`;
}

function empty(title,text,ui,clear=false){return `<div class="empty-state academy-empty">${ui.icon('book-open')}<strong>${ui.esc(title)}</strong><p>${ui.esc(text)}</p>${ui.btn(clear?t("Сбросить фильтры"):t("Все курсы"),clear?'academy-clear':'academy-all','arrow-left','quiet')}</div>`;}

function quizHTML(C,lesson,ui){
 const quiz=lesson.quiz||[];if(!quiz.length)return '';
 const a=local(C),saved=lesson.quizResult?.answers||{},draft=a.drafts[lesson.id]||{};
 return `<section class="quiz" data-academy-quiz="${ui.esc(lesson.id)}" id="${domID(lesson.id+'--quiz')}" data-academy-section="quiz"><div class="academy-quiz-heading">${ui.icon('circle-help')}<h2>${th("Проверьте себя")}</h2><span class="spacer"></span>${lesson.quizResult?.passed?ui.badge(t("Все ответы верны"),'good'):ui.badge(`${lesson.quizResult?.answered||0} / ${quiz.length}`)}</div><p class="academy-quiz-intro">${th("Выберите ответы и нажмите «Проверить». Отметка «Изучено» ставится отдельно.")}</p>${quiz.map((question,index)=>{
  const chosen=Object.hasOwn(draft,question.id)?Number(draft[question.id]):Object.hasOwn(saved,question.id)?Number(saved[question.id]):-1;
  const checked=Object.hasOwn(saved,question.id)&&chosen===Number(saved[question.id]);
  const correct=checked&&Number(saved[question.id])===Number(question.correctIndex);
  return `<fieldset class="academy-question"><legend>${index+1}. ${ui.esc(question.prompt)}</legend>${(question.choices||[]).map((choice,n)=>`<label><input type="radio" name="${domID(lesson.id+'--'+question.id)}" data-academy-answer="${ui.esc(question.id)}" data-lesson="${ui.esc(lesson.id)}" value="${n}" ${chosen===n?'checked':''}><span>${ui.esc(choice)}</span></label>`).join('')}${checked?`<div class="quiz-feedback ${correct?'':'incorrect'}" aria-live="polite"><strong>${correct?t("Верно"):t("Неверно")}</strong><p>${ui.esc(question.explanation)}</p></div>`:''}</fieldset>`;
 }).join('')}<div class="academy-quiz-actions">${ui.btn(t("Проверить ответы"),'academy-check','circle-check','primary',`data-id="${ui.esc(lesson.id)}"`)}</div></section>`;
}

function reader(C,lesson,ui){
 const course=courseByID(C,lesson.courseId)||{},siblings=lessonsFor(C,lesson.courseId);
 const curriculum=`<aside class="curriculum" aria-label="${th("Уроки курса")}">${ui.btn(t("Все курсы"),'academy-library','arrow-left','quiet')}<div class="course-name">${ui.esc(course.title||lesson.courseTitle)}</div><div class="academy-course-progress">${ui.esc(course.completedLessons||0)} / ${ui.esc(lessonCount(course.totalLessons||siblings.length))}${progress(course.percent,t("Прогресс курса"),ui)}</div><nav>${siblings.map(item=>`<button class="lesson-item ${item.id===lesson.id?'selected':''}" type="button" data-click="academy-open" data-id="${ui.esc(item.id)}" ${item.id===lesson.id?'aria-current="page"':''}>${ui.icon(item.completed?'circle-check':item.kind==='checkpoint'?'circle-help':item.kind==='practice'?'wrench':'book-open')}<span>${ui.esc(item.title)}<small>${ui.esc(t(kinds[item.kind]||"Урок"))} · ${ui.esc(item.minutes)}${th(" мин")}</small></span></button>`).join('')}</nav></aside>`;
 const readerTools=`<div class="academy-reader-tools"><span class="academy-step">${th("Урок ")}${ui.esc(lesson.order)}${th(" из ")}${ui.esc(siblings.length)}</span><span class="spacer"></span>${iconButton(ui,lesson.bookmarked?'bookmark-check':'bookmark','academy-bookmark',lesson.bookmarked?t("Убрать из избранного"):t("Добавить в избранное"),`data-id="${ui.esc(lesson.id)}" aria-pressed="${!!lesson.bookmarked}"`)}${ui.btn(C.focus?t("Обычный вид"):t("Режим чтения"),'academy-focus',C.focus?'minimize-2':'maximize-2','quiet',`aria-pressed="${!!C.focus}"`)}</div>`;
 const sections=(lesson.sections||[]).map(section=>{
  const isSolution=section.id==='solution';
  const paragraphs=(section.paragraphs||[]).map(text=>`<p>${ui.esc(text)}</p>`).join('');
  const body=isSolution?`<details class="academy-solution"><summary>${ui.icon('chevron-down')}${th("Показать разбор")}</summary><div>${paragraphs}</div></details>`:paragraphs;
  const checks=section.checklist?.length?`<div class="lesson-checklist">${section.checklist.map((text,index)=>`<label><input type="checkbox" data-academy-check="${ui.esc(section.id)}" data-index="${index}" data-lesson="${ui.esc(lesson.id)}" ${lesson.checklistState?.[section.id+':'+index]?'checked':''}><span>${ui.esc(text)}</span></label>`).join('')}</div>`:'';
  const appLink=section.appSection?`<div class="academy-app-link"><span>${ui.icon('compass')}${th("Продолжить практику в приложении")}</span>${ui.btn(t(sectionNames[section.appSection]||"Открыть раздел"),'academy-tool','arrow-up-right','',`data-id="${ui.esc(section.appSection)}"`)}</div>`:'';
  return `<section class="academy-section" id="${domID(lesson.id+'--'+section.id)}" data-academy-section="${ui.esc(section.id)}"><h2>${ui.esc(section.title)}</h2>${body}${checks}${appLink}</section>`;
 }).join('');
 const sources=`<section class="academy-sources" aria-label="${th("Дополнительные материалы")}"><h2>${th("Дополнительное чтение")}</h2>${(lesson.sourceURLs||[]).map(url=>`<a href="#" data-external="${ui.esc(url)}">${ui.icon('external-link')}${th("Читать далее на PTN")}</a>`).join('')}</section>`;
 const complete=ui.btn(lesson.completed?t("Урок изучен"):t("Отметить изученным"),'academy-complete',lesson.completed?'circle-check':'check',lesson.completed?'accent':'primary',`data-id="${ui.esc(lesson.id)}" aria-pressed="${!!lesson.completed}"`);
 const previous=lesson.previousLessonId?ui.btn(t("Предыдущий"),'academy-open','arrow-left','quiet',`data-id="${ui.esc(lesson.previousLessonId)}"`):'';
 const next=lesson.nextLessonId?ui.btn(t("Следующий урок"),'academy-open','arrow-right','',`data-id="${ui.esc(lesson.nextLessonId)}"`):ui.btn(t("К библиотеке"),'academy-library','layout-grid','quiet');
 const footer=`<footer class="reader-footer">${complete}<span class="spacer"></span>${previous}${next}</footer>`;
 const toc=`<aside class="reader-toc" aria-label="${th("Оглавление урока")}"><h3>${th("В этом уроке")}</h3>${(lesson.sections||[]).map(section=>`<a href="#${domID(lesson.id+'--'+section.id)}" data-click="academy-section" data-id="${ui.esc(section.id)}">${ui.esc(section.title)}</a>`).join('')}${lesson.quiz?.length?'<a href="#'+domID(lesson.id+'--quiz')+`" data-click="academy-section" data-id="quiz">${th("Самопроверка")}</a>`:''}<div class="academy-reader-meta-foot">${lesson.completed?ui.badge(t("Изучено"),'good'):ui.badge(status(lesson))}</div></aside>`;
 return `<div class="reader-layout" data-academy="reader" data-lesson="${ui.esc(lesson.id)}">${curriculum}<article class="reader-article">${readerTools}<div class="eyebrow">${ui.esc(t(course.category||lesson.category||"Учебный маршрут"))}</div><h1>${ui.esc(lesson.title)}</h1><div class="reader-meta"><span>${ui.icon('clock')} ${ui.esc(lesson.minutes)}${th(" мин")}</span><span>${ui.esc(t(kinds[lesson.kind]||"Урок"))}</span><span>${ui.esc(t(lesson.difficulty||''))}</span></div><p class="academy-lead">${ui.esc(lesson.summary)}</p>${sections}${quizHTML(C,lesson,ui)}${sources}${footer}</article>${toc}</div>`;
}

function render(C,ui){
 const lesson=C.lesson?lessonByID(C,C.lesson):null;
 if(C.lesson&&!lesson){C.lesson='';C.course='';C.focus=false;}
 return lesson?reader(C,lesson,ui):catalog(C,ui);
}

async function update(C,ui,values,refresh=true){
 await ui.call('learning',{values});
 if(refresh){const host=ui.$('#page-scroll');C.restoreScroll=host?.scrollTop||0;await ui.loadView();}
}

async function openLesson(C,id,ui){
 const lesson=lessonByID(C,id);if(!lesson){ui.toast(t("Урок не найден"),true);return;}
 await ui.call('learning',{values:{action:'select',lessonId:id}});
 C.lesson=id;C.course=lesson.courseId;
 const position=lesson.position||{};
 local(C).pendingRestore={lessonId:id,ratio:bound(position.ratio,0,1),sectionId:position.sectionId||''};
 C.restoreScroll=0;
 await ui.loadView();
}

async function clearFilters(C,ui){local(C).category='';local(C).query='';await update(C,ui,{action:'configure',courseId:'',query:'',filter:'all'});}

async function handle(action,button,C,ui){
 const a=local(C),id=button?.dataset?.id||C.lesson;
 switch(action){
 case 'academy-course':{const course=courseByID(C,id);if(course)await openLesson(C,nextFor(C,course),ui);else ui.toast(t("Курс не найден"),true);return true;}
 case 'academy-open':await openLesson(C,id,ui);return true;
 case 'academy-library':case 'academy-all':C.lesson='';C.course='';C.focus=false;C.restoreScroll=0;if(action==='academy-all')await clearFilters(C,ui);else await ui.loadView();return true;
 case 'academy-clear':C.lesson='';C.course='';C.focus=false;await clearFilters(C,ui);return true;
 case 'academy-filter':await update(C,ui,{action:'configure',filter:button.dataset.id});return true;
 case 'academy-category':a.category=button.dataset.id||'';ui.renderPage();return true;
 case 'academy-complete':case 'lesson-complete':{const lesson=lessonByID(C,id);if(!lesson)return true;await update(C,ui,{action:'complete',lessonId:lesson.id,completed:!lesson.completed});ui.toast(lesson.completed?t("Урок возвращён в обучение"):t("Прогресс сохранён"));return true;}
 case 'academy-bookmark':{const lesson=lessonByID(C,id);if(!lesson)return true;await update(C,ui,{action:'bookmark',lessonId:lesson.id,bookmarked:!lesson.bookmarked});ui.toast(lesson.bookmarked?t("Закладка удалена"):t("Урок добавлен в избранное"));return true;}
 case 'academy-focus':C.focus=!C.focus;C.restoreScroll=ui.$('#page-scroll')?.scrollTop||0;ui.renderPage();return true;
 case 'academy-tool':if(Object.hasOwn(sectionNames,id))await ui.navigate(id);return true;
 case 'academy-section':{const node=ui.$('#'+domID(C.lesson+'--'+id));node?.scrollIntoView({behavior:'smooth',block:'start'});return true;}
 case 'academy-check':{
  const lesson=lessonByID(C,id);if(!lesson)return true;
  const scope=ui.$('[data-academy-quiz]');if(!scope){ui.toast(t("Сначала откройте самопроверку"));return true;}const inputs=ui.$$('[data-academy-answer]',scope);
  const answers=(lesson.quiz||[]).map(question=>({question,node:inputs.find(input=>input.dataset.academyAnswer===question.id&&input.checked)}));
  if(answers.some(answer=>!answer.node)){ui.toast(t("Выберите ответ на каждый вопрос"));return true;}
  const host=ui.$('#page-scroll');C.restoreScroll=host?.scrollTop||0;
  for(const {question,node} of answers)await ui.call('learning',{values:{action:'answer',lessonId:lesson.id,questionId:question.id,choiceIndex:Number(node.value)}});
  delete a.drafts[lesson.id];await ui.loadView();
  const result=lessonByID(C,lesson.id)?.quizResult;
  ui.toast(result?.passed?t("Все ответы верны. Отличная работа."):`${t("Верных ответов: ")}${result?.correct||0}${t(" из ")}${result?.total||answers.length}${t(". Посмотрите разбор.")}`);return true;
 }
 case 'academy-progress':{
  const s=model(C),courses=s.courses||[];
  ui.dialog(t("Мой прогресс"),`<p class="academy-dialog-intro">${th("Изучено ")}${ui.esc(s.progress?.completed||0)}${th(" из ")}${ui.esc(s.progress?.total||0)}${th(" уроков. Избранное и позиция чтения сохраняются между запусками.")}</p><div class="academy-progress-list">${courses.map(course=>`<div><strong>${ui.esc(course.title)}</strong><span>${ui.esc(course.completedLessons||0)} / ${ui.esc(course.totalLessons||0)}</span>${progress(course.percent,t("Прогресс курса"),ui)}</div>`).join('')}</div>`,ui.btn(t("Вернуться к обучению"),'close-dialog','arrow-left'));return true;
 }
 default:return false;
 }
}

const boundHosts=new WeakSet();
function init(C,ui){
 const host=ui.$('#page-scroll');if(!host)return;
 const a=local(C);
 if(!boundHosts.has(host)){
  boundHosts.add(host);
  host.addEventListener('click',event=>{if(event.target.closest?.('[data-click="academy-section"]'))event.preventDefault();});
  host.addEventListener('input',event=>{
   if(event.target.id!=='academy-query')return;
   a.query=event.target.value;clearTimeout(a.searchTimer);
   a.searchTimer=setTimeout(async()=>{try{const start=event.target.selectionStart,query=a.query;await update(C,ui,{action:'configure',query});if(a.query===query)delete a.query;const input=ui.$('#academy-query');if(input){input.focus();try{input.setSelectionRange(start,start);}catch(_){}}}catch(error){ui.toast(error.message,true);}},220);
  });
  host.addEventListener('change',async event=>{
   try{
    const input=event.target;
    if(input.matches('[data-academy-answer]')){const drafts=a.drafts[input.dataset.lesson]||(a.drafts[input.dataset.lesson]={});drafts[input.dataset.academyAnswer]=Number(input.value);return;}
    if(input.matches('[data-academy-check]')){
     input.disabled=true;
     try{await update(C,ui,{action:'checklist',lessonId:input.dataset.lesson,sectionId:input.dataset.academyCheck,index:Number(input.dataset.index),checked:input.checked},false);const lesson=lessonByID(C,input.dataset.lesson);if(lesson){lesson.checklistState=lesson.checklistState||{};lesson.checklistState[input.dataset.academyCheck+':'+input.dataset.index]=input.checked;}}
     catch(error){input.checked=!input.checked;throw error;}finally{input.disabled=false;}
    }
   }catch(error){ui.toast(error.message,true);}
  });
  host.addEventListener('keydown',event=>{if(event.key==='Escape'&&C.focus&&C.lesson&&!C.layer){event.preventDefault();C.focus=false;C.restoreScroll=host.scrollTop;ui.renderPage();}});
  host.addEventListener('scroll',()=>{
   if(C.tab!=='learning'||!C.lesson||a.suppressPosition)return;
   const lesson=lessonByID(C,C.lesson);if(!lesson)return;
   const sections=ui.$$('[data-academy-section]',host),top=host.getBoundingClientRect().top+72;
   let current=sections[0]?.dataset.academySection||'';
   for(const section of sections){if(section.getBoundingClientRect().top<=top)current=section.dataset.academySection;}
   for(const link of ui.$$('.reader-toc [data-click="academy-section"]',host))link.classList.toggle('selected',link.dataset.id===current);
   if(current==='quiz')current=lesson.sections?.at(-1)?.id||'';
   const max=host.scrollHeight-host.clientHeight,ratio=max>0?bound(host.scrollTop/max,0,1):0;
   const body={action:'position',lessonId:lesson.id,sectionId:current,ratio};
   clearTimeout(a.positionTimer);a.positionTimer=setTimeout(async()=>{try{await update(C,ui,body,false);const item=lessonByID(C,body.lessonId);if(item)item.position={sectionId:body.sectionId,ratio:body.ratio};}catch(error){ui.toast(error.message,true);}},650);
  },{passive:true});
 }
 const pending=a.pendingRestore;
 if(pending&&pending.lessonId===C.lesson){
  a.pendingRestore=null;a.suppressPosition=true;
  setTimeout(()=>{if(C.tab==='learning'&&C.lesson===pending.lessonId){if(pending.ratio>0)host.scrollTop=Math.max(0,host.scrollHeight-host.clientHeight)*pending.ratio;else if(pending.sectionId)ui.$('#'+domID(C.lesson+'--'+pending.sectionId))?.scrollIntoView({block:'start'});}a.suppressPosition=false;},0);
 }
}

async function smoke(C,ui){
 const checks=[],add=(name,ok)=>checks.push({name,ok:!!ok});
 const s=model(C);add(t("Академия: девять курсов"),s.courses?.length===9);add(t("Академия: 27 уроков"),s.lessons?.length===27);
 const previous={lesson:C.lesson,course:C.course,focus:C.focus};
 try{
  C.lesson='';C.course='';C.focus=false;ui.renderPage();
  add(t("Каталог курсов и поиск"),ui.$$('.course-card').length===9&&!!ui.$('#academy-query'));
  const practice=(s.lessons||[]).find(lesson=>lesson.kind==='practice');
  if(practice){await openLesson(C,practice.id,ui);add(t("Читатель и оглавление"),!!ui.$('.reader-article')&&!!ui.$('.reader-toc'));add(t("Практический чек-лист"),ui.$$('[data-academy-check]').length>=5);await ui.smokeCapture?.('reader');C.focus=true;ui.renderPage();add(t("Режим чтения"),!!ui.$('.reading-focus'));}
  const quiz=(s.lessons||[]).find(lesson=>lesson.kind==='checkpoint');
  if(quiz){await openLesson(C,quiz.id,ui);add(t("Вопросы самопроверки"),ui.$$('.academy-question').length===quiz.quiz.length);}
 }finally{C.lesson=previous.lesson;C.course=previous.course;C.focus=previous.focus;await ui.loadView();}
 return checks;
}

window.AcademyUI={render,handle,init,smoke};
})();
