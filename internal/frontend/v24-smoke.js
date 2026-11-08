/* Only --smoke loads these probes against a disposable offline profile. */
(()=>{
'use strict';
const clone=value=>value==null?value:JSON.parse(JSON.stringify(value));
const equal=(a,b)=>JSON.stringify(a)===JSON.stringify(b);
async function run(C,u){
 if(window.COPILOT_SMOKE!==true)throw new Error('V24 requires an isolated smoke profile');
 if(typeof u.handleClick!=='function'||typeof u.call!=='function'||typeof u.navigate!=='function')throw new Error('V24 requires the real root interface handlers');
 const checks=[],check=(name,ok)=>{checks.push({name:'V24 · '+name,ok:!!ok});if(!ok)throw new Error(name);};
 const original={tab:C.tab,mode:C.mode,query:C.query,selected:C.selected,prefill:clone(C.prefill),nav:[...(C.nav||[])],focus:C.focus,lesson:C.lesson,course:C.course,exo:C.exoWorkspace,reference:C.reference,references:C.referenceSessions};
 const initialLanguage=C.state?.settings?.language||'ru';let initialExo,failure;const cleanup=[];
 const visit=(tab,opts={})=>u.navigate(tab,{...opts,back:true});
 const fresh=async()=>((await u.call('view',{tab:'exobiology',mode:'Обзор',query:'',selected:''})).state||{}).exobiology||{};
 const wait=async(predicate,label,diagnostics)=>{const end=Date.now()+12000;while(Date.now()<end){const error=u.$('#form-error');if(error&&!error.classList.contains('hidden')&&error.textContent)throw new Error(error.textContent);if(predicate())return;await new Promise(resolve=>setTimeout(resolve,20));}throw new Error(label+(diagnostics?' · '+JSON.stringify(diagnostics()):''));};
 const click=async selector=>{const node=u.$(selector);if(!node||node.disabled)throw new Error('Unavailable smoke control: '+selector);await u.handleClick(node.dataset.click,node);return node;};
 const language=async value=>{
  await visit('settings');await click('[data-action-id="settings.language"]');
  const form=u.$('#action-form');check('language uses the actual settings form',form?.dataset.id==='settings.language'&&!!form.elements.language);
  form.elements.language.value=value;form.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));
  await wait(()=>!C.layer&&C.state.settings?.language===value&&window.CopilotI18n.language===value,'Language form did not finish');
 };
 try{
  initialExo=clone(await fresh());
  check('synthetic journal contains multiple survey bodies',initialExo.bodies?.length>=2);
  check('synthetic journal contains both unsold and sold data',initialExo.pending?.length===1&&initialExo.sales?.length===1);
  const tracker=initialExo.tracker,anchors=tracker?.anchors||[];
  check('synthetic tracker retains two measured sample anchors',tracker?.sampleCount===2&&anchors.length===2&&anchors.every(a=>Number.isFinite(a.latitude)&&Number.isFinite(a.longitude))&&(anchors[0].latitude!==anchors[1].latitude||anchors[0].longitude!==anchors[1].longitude));
  // The initial synthetic Status can be older than the live-fix lease after
  // the earlier form probes. Saved sample anchors remain valid, but a stale
  // position must never be presented as a currently measured distance.
  check('tracker distinguishes a fresh distance from an expired live position',tracker.positionKnown===true?Number.isFinite(tracker.nearestSampleDistanceM)&&Number.isFinite(tracker.remainingDistanceM):tracker.positionKnown===false&&tracker.nearestSampleDistanceM===null&&tracker.remainingDistanceM===null&&tracker.nearestSampleMeetsMin===null);
  await language('en');
  check('English language is persisted by the backend',(await u.call('view',{tab:'settings'})).state?.settings?.language==='en');
  check('document language and menu label follow English',document.documentElement.lang==='en'&&C.t('Настройки')==='Settings');
  check('internal mode tokens retain their canonical values',C.t('Продажи')==='Sales'&&C.t('Вид')==='View');
  for(const mode of ['Обзор','Планеты','Образцы','Продажи','Находки','Места','Статистика']){
   const started=performance.now();await visit('exobiology',{mode});
   check('English exobiology screen renders: '+mode,u.$('[data-exobiology]')?.dataset.exobiology===mode&&u.$('[data-exobiology] h1')?.textContent==='Exobiology');
   check('screen remains responsive: '+mode,performance.now()-started<10000);
  }
  await visit('exobiology',{mode:'Обзор'});
  check('sample tracker exposes all three distinct steps',u.$$('[data-exo-tracker] .exo-sample-steps li').length===3&&u.$$('[data-exo-tracker] .exo-sample-steps li.complete').length===2);
  const bodyKey=C.state.exobiology.selectedBodyKey,other=C.state.exobiology.bodies.find(body=>body.key!==bodyKey);
  const select=u.$('[data-exo-body]');select.value=other.key;select.dispatchEvent(new Event('change',{bubbles:true}));
  await wait(()=>C.state.exobiology.selectedBodyKey===other.key,'Body selection did not persist');
  check('body selector loads the selected complete body card',u.$('.exo-body-title h2')?.textContent===other.name);
  await u.call('exobiology',{values:{action:'select-body',bodyKey}});await u.loadView();
  const radius=u.$('[data-exo-config="radiusLy"]'),newRadius=Number(initialExo.settings.radiusLy)===125?126:125;radius.value=String(newRadius);radius.dispatchEvent(new Event('input',{bubbles:true}));await click('[data-click="exo-configure"]');
  check('inline criteria save reaches Go settings',(await fresh()).settings.radiusLy===newRadius);
  check('criteria save re-enables its actual button',!u.$('[data-click="exo-configure"]').disabled);
  if(!(initialExo.bookmarks||[]).some(b=>b.bodyKey===bodyKey)){
   await click('[data-click="exo-bookmark"]');check('bookmark survives a new backend snapshot',(await fresh()).bookmarks.some(b=>b.bodyKey===bodyKey));
   await u.call('exobiology',{values:{action:'bookmark',bodyKey}});check('repeated bookmark save is idempotent',(await fresh()).bookmarks.filter(b=>b.bodyKey===bodyKey).length===1);
  }
  await visit('exobiology',{mode:'Места'});await click('[data-click="exo-waypoint"]');
  const form=u.$('#exo-waypoint-form'),label='V24 synthetic waypoint';form.elements.label.value=label;form.elements.latitude.value='12.5';form.elements.longitude.value='-45.25';await click('[data-click="exo-waypoint-save"]');
  const waypoint=(await fresh()).surfaceMarks.find(m=>m.kind==='waypoint'&&m.label===label);
  check('waypoint stores exactly the submitted coordinates',waypoint?.latitude===12.5&&waypoint.longitude===-45.25);
  check('waypoint dialog closes after a successful save',!C.layer&&!u.$('#exo-waypoint-form'));
  check('surface marks distinguish samples from scanner origins',u.$('.exo-marks')?.textContent.includes('Recorded position')&&!u.$('.exo-marks')?.textContent.includes('undefined'));
  const siteRadius=u.$('[data-exo-site="radiusLy"]');siteRadius.value='0';siteRadius.dispatchEvent(new Event('input',{bubbles:true}));await click('[data-click="exo-sites-search"]');
  const sites=C.exoWorkspace.sites.result;
  check('public sites are a bounded local observation list',sites?.entries?.length>0&&sites.entries.length<=12&&sites.live===false);
  check('public sites never invent a precise distance',sites.entries.every(e=>e.coordinatesKnown===false&&e.distanceLy==null)&&u.$('.exo-sites').textContent.includes('approximate'));
  C.referenceSessions={};delete C.reference;await visit('encyclopedia',{referenceCategory:'exobiology'});
  await wait(()=>C.reference?.list&&!C.reference.loading,'Exobiology catalogue did not finish');
  check('species catalogue respects the twelve-photo page size',C.reference.list.limit===12&&u.$$('.ref-card').length===12&&u.$$('[data-reference-image]').length===12);
  check('species catalogue images are embedded offline assets',u.$$('[data-reference-image]').every(img=>img.src.startsWith('data:image/')));
  const firstID=u.$('.ref-card').dataset.id;await click('[data-click="reference-next"]');await wait(()=>C.reference.offset===12&&!C.reference.loading,'Second species page did not finish');
  check('species pagination advances without skipping a page',C.reference.list.offset===12&&u.$('.ref-card').dataset.id!==firstID);
  await click('.ref-card');await wait(()=>C.reference?.entry&&!C.reference.entryLoading,'Full species page did not finish');
  check('species opens a full illustrated reference page',C.reference.entry.id.startsWith('exo/')&&!!u.$('[data-reference="entry"] [data-reference-image]')&&!u.$('.inspector'));
  await visit('route');await click('[data-action-id="route.calculate"]');
  const system=u.$('#action-form').elements.sourceSystem;
  check('route origin is recognized as a system-name field',window.SystemSearchUI.fieldMode(system)==='single');
  system.value='Syn';system.focus();system.dispatchEvent(new Event('input',{bubbles:true}));
  await wait(()=>u.$('#copilot-system-suggestions-status')?.textContent.includes('disabled'),'Offline combobox did not disclose its disabled network',()=>({popupPresent:!!u.$('#copilot-system-suggestions'),popupHidden:u.$('#copilot-system-suggestions')?.hidden,status:u.$('#copilot-system-suggestions-status')?.textContent,focusedField:document.activeElement?.name||document.activeElement?.tagName,inputConnected:system.isConnected,inputRole:system.getAttribute('role')}));
  check('offline system combobox preserves manually typed names',system.value==='Syn'&&system.getAttribute('role')==='combobox');
  system.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));
  check('Escape closes suggestions without discarding the form',u.$('#copilot-system-suggestions').hidden&&!!u.$('#action-form'));u.closeLayer();
  await language('ru');check('Russian interface is restored through the same real form',document.documentElement.lang==='ru'&&C.t('Настройки')==='Настройки');
 }catch(error){failure=error;}
 finally{
  const restore=async(name,work)=>{try{await work();}catch(error){cleanup.push(name+': '+(error.message||error));}};
  u.closeLayer();
  if(initialExo)await restore('exobiology',async()=>{
   await u.call('exobiology',{values:{action:'configure',...initialExo.settings}});
   const current=await fresh(),bookmarks=new Set((initialExo.bookmarks||[]).map(b=>b.bodyKey)),marks=new Set((initialExo.surfaceMarks||[]).map(m=>m.id));
   for(const b of current.bookmarks||[])if(!bookmarks.has(b.bodyKey))await u.call('exobiology',{values:{action:'remove-bookmark',bodyKey:b.bodyKey}});
   for(const m of current.surfaceMarks||[])if(m.kind==='waypoint'&&!marks.has(m.id))await u.call('exobiology',{values:{action:'remove-waypoint',id:m.id}});
   if(initialExo.selectedBodyKey)await u.call('exobiology',{values:{action:'select-body',bodyKey:initialExo.selectedBodyKey}});
   const restored=await fresh();if(!equal(restored.settings,initialExo.settings)||restored.selectedBodyKey!==initialExo.selectedBodyKey)throw new Error('criteria or body selection changed');
  });
  await restore('language',async()=>{if(C.state?.settings?.language!==initialLanguage){await u.call('settings',{values:{language:initialLanguage}});await u.loadView();}});
  window.ReferenceUI?.navigate?.('overview',{},C);C.referenceSessions=original.references;C.reference=original.reference;C.exoWorkspace=original.exo;
  await restore('navigation',async()=>{if(!equal(C.nav,original.nav)){await u.call('navigation',{tabs:original.nav});C.nav=original.nav;}});
  await restore('view',async()=>{await visit(original.tab,{mode:original.mode,prefill:original.prefill});C.query=original.query;C.selected=original.selected;C.focus=original.focus;C.lesson=original.lesson;C.course=original.course;await u.loadView();});
 }
 if(failure)throw new Error(failure.message+(cleanup.length?' · Cleanup: '+cleanup.join('; '):''));
 if(cleanup.length)throw new Error('V24 cleanup failed: '+cleanup.join('; '));
 return checks;
}
window.V24Smoke={run};
})();
