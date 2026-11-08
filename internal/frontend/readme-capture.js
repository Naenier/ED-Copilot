/* Documentation photographs use the real renderer and a disposable offline profile. */
(()=>{'use strict';
async function run(C,u){
 if(window.COPILOT_README!==true||window.COPILOT_SMOKE!==true)throw Error('Documentation capture requires the isolated fixture');
 const language=C.state?.settings?.language;
 if(!['en','ru'].includes(language)||C.state?.commander?.fid!=='FNATIVE-SYNTHETIC')throw Error('Documentation capture requires the synthetic commander in English or Russian');
 const scenarios=[],pages=[],problems=[];
 const check=(name,ok)=>{scenarios.push({name,ok:!!ok});if(!ok)problems.push(name);};
 const ready=async()=>{const until=Date.now()+20000;while(Date.now()<until){const r=C.reference;if(r?.error||r?.entryError)throw Error(r.error||r.entryError);if(r&&!r.loading&&!r.entryLoading&&(r.entryId?r.entry:r.list))return;await new Promise(resolve=>setTimeout(resolve,30));}throw Error('Reference page timed out');};
 const capture=async name=>{
  window.TooltipUI?.hide?.();
  if(C.tab==='ranks'||C.tab==='commander'&&C.mode==='Сводка')await window.RankInsignia?.load?.(C.state.ranks?.disciplines||[],u,C);
  u.$('#toasts')?.replaceChildren();u.$('#page-scroll').scrollTop=0;
  const visibleImages=[...document.images||[]].filter(img=>{const r=img.getBoundingClientRect();return r.bottom>0&&r.top<innerHeight&&r.right>0&&r.left<innerWidth;});
  await Promise.all(visibleImages.map(async img=>{if(!img.complete&&typeof img.decode==='function')await img.decode();if(img.complete&&img.naturalWidth===0)throw Error('A visible documentation image failed to load');}));
  await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
  const text=document.body.innerText,untranslated=language==='en'?[...new Set(text.split('\n').map(x=>x.trim()).filter(x=>/[\p{Script=Cyrillic}]/u.test(x)))].slice(0,100):[];
  const state=await u.call('window',{action:'status'});
  check(name+' is full screen',state.displayMode==='fullscreen');
  check(name+' uses '+language,document.documentElement.lang===language);
  if(language==='en')check(name+' contains no Cyrillic text',untranslated.length===0);
  pages.push({name,language,untranslated,viewport:{width:innerWidth,height:innerHeight},displayMode:state.displayMode});
  await u.smokeCapture(name);
 };
 const category=async id=>{await u.navigate('encyclopedia',{referenceCategory:id});await ready();};
 const entry=async(cat,query)=>{await category(cat);const list=await u.call('reference.list',{category:cat,query,limit:12});const item=list.entries?.[0];if(!item)throw Error('Reference entry not found: '+query);await window.ReferenceUI.open(item.id,C,u);await ready();};
 try{
  check('Synthetic commander is used',C.state.commander?.name===(language==='en'?'Demo Commander':'Тестовый командир'));
  check('Requested documentation language is active',document.documentElement.lang===language);
  await u.call('window',{action:'fullscreen'});
  await u.navigate('overview');await capture('readme-overview');
  await u.navigate('commander',{mode:'Сводка'});check('Commander summary is available',!!u.$('[data-commander-section="Сводка"]'));await capture('readme-commander');
  await u.navigate('ranks');check('All eight rank cards are available',u.$$('[data-rank-card]').length===8);await capture('readme-ranks');
  await u.navigate('commander',{mode:'Флот'});check('Commander fleet is available',!!u.$('[data-commander-section="Флот"]'));await capture('readme-fleet');
  await u.navigate('commander',{mode:'Инвентарь'});check('Commander inventory is available',!!u.$('[data-commander-section="Инвентарь"]'));await capture('readme-inventory');
  await u.navigate('overview',{mode:'Состояние'});check('Navigation has one permanent settings control',u.$$('.rail [data-click="settings-nav"]').length===1&&u.$$('.sidebar [data-nav="settings"]').length===0);await capture('readme-navigation');
  const wasCollapsed=C.collapsed;
  try{if(!C.collapsed)await u.handleClick('sidebar',u.$('[data-click="sidebar"]'));check('Compact navigation collapses only the sidebar',C.collapsed===true);await capture('readme-navigation-compact');}
  finally{if(C.collapsed!==wasCollapsed)await u.handleClick('sidebar',u.$('[data-click="sidebar"]'));}
  await u.navigate('deals',{mode:'Поиск'});C.dealsUI.view='results';C.dealsUI.query='';C.dealsUI.page=0;u.renderPage();await capture('readme-deals');
  await u.navigate('carrier-trade');await capture('readme-carrier-trade');
  await u.navigate('route');await capture('route-details');
  await category('ships');await capture('readme-ships');
  await entry('ships','Python');await capture('readme-ship');
  await entry('engineers','Felicity Farseer');await capture('readme-engineer');
  await entry('articles',language==='en'?'Tritium':'Тритий');await capture('readme-article');
  await u.navigate('colonization',{mode:'Строительство'});await capture('readme-colonization');
  await u.navigate('learning');await capture('readme-academy');
  await u.navigate('exobiology',{mode:'Обзор'});
  for(const alert of C.state.exobiology?.alerts||[])await u.call('exobiology',{values:{action:'dismiss-alert',id:alert.id}});
  await u.loadView();await capture('exobiology');
  await u.call('smoke-result',{ok:problems.length===0,scenarios,pages,problems,documentation:true,language});
 }catch(error){await u.call('smoke-result',{ok:false,scenarios,pages,problems:[...problems,error.message],documentation:true,language});}
}
window.ReadmeCapture={run};
})();
