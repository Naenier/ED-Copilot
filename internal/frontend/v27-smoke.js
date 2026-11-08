/* Actual grouped navigation and commander pages, in the guarded offline runtime. */
(()=>{'use strict';
async function run(C,u){
 if(window.COPILOT_SMOKE!==true)throw Error('Workspace smoke requires the isolated launcher');
 const results=[],before={tab:C.tab,mode:C.mode,language:C.state.settings?.language,collapsed:C.collapsed};
 const check=(name,ok)=>{results.push({name,ok:!!ok});if(!ok)throw Error(name);};
 const wait=async predicate=>{const until=Date.now()+12000;while(Date.now()<until){if(predicate())return;await new Promise(resolve=>setTimeout(resolve,25));}throw Error('Workspace did not settle');};
 try{
  for(const language of ['en','ru']){
   await u.call('settings',{values:{language}});await u.navigate('overview');
   check(language+': titlebar follows language changes arriving during navigation',u.$('.titlebar [data-click="file-menu"]')?.textContent===u.t('Файл'));
   const tipAnchor=u.$('.rail [data-group="trade"]');
   tipAnchor.dispatchEvent(new PointerEvent('pointerover',{bubbles:true,pointerType:'mouse'}));
   await wait(()=>!!u.$('#copilot-tooltip')&&!u.$('#copilot-tooltip').hidden);
   const tooltip=u.$('#copilot-tooltip'),tipBox=tooltip.getBoundingClientRect(),tipStyle=getComputedStyle(tooltip);
   check(language+': custom tooltip replaces the native title',!tipAnchor.hasAttribute('title')&&tooltip.textContent===u.t('Торговля'));
   check(language+': tooltip is a rounded body portal above navigation',tooltip.parentElement===document.body&&tipStyle.position==='fixed'&&parseFloat(tipStyle.borderRadius)===8&&tipStyle.backgroundColor==='rgb(48, 48, 48)');
   check(language+': tooltip remains inside the viewport',tipBox.left>=8&&tipBox.top>=8&&tipBox.right<=innerWidth-7&&tipBox.bottom<=innerHeight-7);
   await new Promise(resolve=>setTimeout(resolve,150));await u.smokeCapture('tooltip-'+language);
   document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));
   check(language+': Escape dismisses the tooltip',tooltip.hidden&&!tipAnchor.getAttribute('aria-describedby')?.includes('copilot-tooltip'));
   check(language+': settings appears once in navigation',u.$$('.rail [data-click="settings-nav"]').length===1&&u.$$('.sidebar [data-nav="settings"]').length===0);
   for(const group of window.NavigationUI.groups.filter(x=>x.id!=='commander')){
    const button=u.$('.rail [data-group="'+group.id+'"]');check(language+': group rail button '+group.id,!!button);button.click();
    await wait(()=>C.group===group.id&&C.page&&window.NavigationUI.groupFor(C.tab)===group.id);
    const expected=new Set(group.tabs);check(language+': sidebar contains only '+group.id,u.$$('.sidebar [data-nav]').every(node=>expected.has(node.dataset.nav)));
   }
   const avatar=u.$('.rail .avatar');avatar.click();await wait(()=>C.tab==='commander'&&!!u.$('[data-commander-section]'));
   check(language+': commander avatar opens commander instead of OAuth',C.group==='commander'&&C.tab==='commander'&&C.state.commanderProfile?.commanderFID==='FNATIVE-SYNTHETIC');
   for(const mode of ['Сводка','Финансы','Инвентарь','Имущество','Флот','Статистика','Репутация','Журнал']){
    const button=u.$('.sidebar [data-nav="commander"][data-nav-mode="'+mode+'"]');check(language+': commander section '+mode+' exists',!!button);button.click();
    await wait(()=>C.tab==='commander'&&C.mode===mode&&u.$('[data-commander-section]')?.dataset.commanderSection===mode);
    check(language+': commander section renders '+mode,!!u.$('#page h1')&&!u.$('#page .table-layout'));
    if(language==='en')check('English commander '+mode+' has no untranslated prose',!/[\p{Script=Cyrillic}]/u.test(u.$$('.page-heading,.commander-panel h2,.commander-empty').map(node=>node.innerText).join(' ')));
   }
   await u.navigate('ranks');await window.RankInsignia?.load?.(C.state.ranks?.disciplines||[],u,C);check(language+': rank pages use embedded game insignia',u.$$('.ranks-card .rank-insignia img').length===8);
   await u.navigate('colonization',{mode:'Снабжение'});
   const budget=u.$('[data-colony-budget]');
   check(language+': colony resource budget belongs to the active project',!!budget&&budget.dataset.colonyBudget===C.state.colonization?.activeProject?.id&&!!C.state.colonization?.activeProject?.costSummary);
   check(language+': budget shows three distinct totals and a collapsed detailed breakdown',budget.querySelectorAll(':scope>.colony-budget-metrics>.colony-summary-metric').length===3&&budget.querySelector('.colony-budget-breakdown')?.open===false);
   if(language==='en')check('English colony budget has no untranslated prose',!/[\p{Script=Cyrillic}]/u.test(budget.innerText));
   await u.navigate('colonization',{mode:'Бюджет'});
   check(language+': detailed budget table includes observed average prices and commodity totals',C.page.Columns.includes(u.t('Средняя по галактике, кр./т'))&&C.page.Columns.includes(u.t('Полная стоимость, кр.'))&&C.page.Columns.includes(u.t('Цена получена'))&&C.page.Rows.length===(C.state.colonization?.activeProject?.supplies||[]).length);
   check(language+': budget gives the resource table room without a side inspector',!u.$('.data-split .inspector')&&u.$('.table-scroll thead')?.getBoundingClientRect().top<innerHeight*.75);
   await u.smokeCapture('colony-budget-'+language);
   await u.navigate('colonization',{mode:'Снабжение'});
   const warehouseBefore=C.state.colonization?.includeCarrierWarehouse===true;
   const warehouse=u.$('#colony-include-carrier-stock');check(language+': colony warehouse option is available',!!warehouse);warehouse.click();
   await wait(()=>C.state.colonization?.includeCarrierWarehouse===!warehouseBefore&&!u.$('#colony-include-carrier-stock')?.disabled);
   check(language+': colony warehouse option saves through the real backend',u.$('#colony-include-carrier-stock')?.checked===!warehouseBefore);
   if(!warehouseBefore){
    check(language+': unread squadron warehouse requires an observed carrier',u.$('#colony-squadron-stock-access')?.disabled===true);
    const cards=u.$$('.colony-supply-items>div');check(language+': enabled warehouse supply cards are present',cards.length>0);
    check(language+': enabled warehouse metrics have separate readable rows',cards.every(card=>{
     const rows=[...card.querySelectorAll('small')],box=card.getBoundingClientRect();
     return rows.length===3&&rows.every((row,index)=>{
      const bounds=row.getBoundingClientRect(),previous=index?rows[index-1].getBoundingClientRect():null;
      return bounds.width>0&&bounds.height>0&&bounds.left>=box.left&&bounds.right<=box.right+1&&row.scrollWidth<=row.clientWidth+1&&(!previous||bounds.top-previous.bottom>=4);
     });
    }));
    const stockGroup=u.$('.colony-warehouse-options');check(language+': warehouse controls remain an independent visible group',stockGroup?.tagName==='DIV'&&stockGroup.getAttribute('role')==='group'&&!stockGroup.closest('summary,.summary-trigger,.section-summary-trigger'));
   }
   u.$('#colony-include-carrier-stock').click();await wait(()=>C.state.colonization?.includeCarrierWarehouse===warehouseBefore&&!u.$('#colony-include-carrier-stock')?.disabled);
  }
  await u.handleClick('sidebar',u.$('[data-click="sidebar"]'));check('Workspace sidebar can collapse',C.collapsed===true);
  u.$('.rail [data-group="trade"]').click();await wait(()=>C.group==='trade'&&!C.collapsed&&!!C.page);check('Choosing a group reopens its own sidebar',!u.$('.sidebar').classList.contains('collapsed'));
 }finally{await u.call('settings',{values:{language:before.language}});await u.navigate(before.tab,{mode:before.mode});if(C.collapsed!==before.collapsed)await u.handleClick('sidebar',u.$('[data-click="sidebar"]'));}
 return results;
}
window.V27Smoke={run};
})();
