/* Screenshot and integration scenarios run only in the isolated smoke profile. */
(()=>{'use strict';
async function run(C,u){
 if(!window.COPILOT_SMOKE)throw new Error('Сценарий доступен только в изолированной проверке');
 const checks=[],check=(name,ok)=>{checks.push({name,ok:!!ok});if(!ok)throw new Error(name);};
 const capture=async name=>{u.$('#toasts')?.replaceChildren();await u.smokeCapture(name);};
 const ready=async()=>{const limit=Date.now()+15000;while(Date.now()<limit){const r=C.reference;if(r&&!r.loading&&!r.entryLoading&&(r.entryId?r.entry:r.list)){if(r.error||r.entryError)throw new Error(r.error||r.entryError);await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));return;}await new Promise(resolve=>setTimeout(resolve,30));}throw new Error('Справочник не завершил загрузку');};
 const category=async id=>{await u.navigate('encyclopedia');await window.ReferenceUI.handleClick('reference-category',{dataset:{id}},C,u);await ready();};
 const openFirst=async(category,query)=>{const list=await u.call('reference.list',{category,query,limit:24});const entry=list.entries?.[0];if(!entry)throw new Error('Запись справочника не найдена: '+query);await window.ReferenceUI.open(entry.id,C,u);await ready();return entry;};
 await u.call('window',{action:'fullscreen'});
 try{
  const windowState=await u.call('window',{action:'status'});check('Скриншоты README: окно развёрнуто на весь экран',windowState.displayMode==='fullscreen');
  await u.navigate('overview');await capture('readme-overview');
  check('Баланс командира без имени Status.json',!u.$('#page').textContent.includes('Status.json'));
  C.mode='Состояние';await u.loadView();check('Состояние командира оформлено карточками',!!u.$('.ov-facts')&&!u.$('#page table'));
  await u.navigate('carrier-trade');check('Демонстрационный рейс содержит рассчитанную прибыль',C.state.carrierTrade?.summary?.profit!=null&&Number.isFinite(Number(C.state.carrierTrade.summary.profit)));await capture('readme-carrier-trade');
  await category('ships');check('Корабли представлены карточками',!!u.$('.ref-grid-ships'));await capture('readme-ships');
  await openFirst('ships','Python');check('Корабль открывается на отдельной странице',!!u.$('[data-reference="entry"]')&&!u.$('.inspector'));await capture('readme-ship');
  await category('engineers');await openFirst('engineers','Фарсир');check('Инженер открывается на отдельной странице',!!u.$('[data-reference="entry"]')&&!u.$('.inspector'));check('Инженер имеет настоящее изображение',!!u.$('.ref-art-hero img'));await capture('readme-engineer');
  await category('blueprints');check('Чертежи сгруппированы в семейства',Number(C.reference.list?.total)>0&&Number(C.reference.list.total)<80);
  await category('experimentals');check('Экспериментальные эффекты сгруппированы',Number(C.reference.list?.total)>0&&Number(C.reference.list.total)<87);
  await category('commodities');await openFirst('commodities','Золото');check('Товар имеет панель рыночных предложений',!!u.$('.ref-trading')||u.$('#page').textContent.includes('Покуп')||u.$('#page').textContent.includes('Продаж'));
  await category('articles');await openFirst('articles','Тритий');check('Статья открывается для чтения',!!u.$('.ref-entry-content'));check('В справочнике нет блока источников',!u.$('#page').textContent.includes('Источники:'));await capture('readme-article');
  await u.navigate('colonization',{mode:'Строительство'});await capture('readme-colonization');
  await u.navigate('learning');await capture('readme-academy');
  await u.navigate('route');check('Настройка автоматического копирования следующей системы',!!u.$('#route-auto-copy'));
 }finally{await u.call('window',{action:'windowed'});}
 return checks;
}
window.V22Smoke={run};
})();
