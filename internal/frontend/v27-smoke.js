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
   const warehouseBefore=C.state.colonization?.includeCarrierWarehouse===true;
   const warehouse=u.$('#colony-include-carrier-stock');check(language+': colony warehouse option is available',!!warehouse);warehouse.click();
   await wait(()=>C.state.colonization?.includeCarrierWarehouse===!warehouseBefore&&!u.$('#colony-include-carrier-stock')?.disabled);
   check(language+': colony warehouse option saves through the real backend',u.$('#colony-include-carrier-stock')?.checked===!warehouseBefore);
   if(!warehouseBefore)check(language+': unread squadron warehouse requires an observed carrier',u.$('#colony-squadron-stock-access')?.disabled===true);
   u.$('#colony-include-carrier-stock').click();await wait(()=>C.state.colonization?.includeCarrierWarehouse===warehouseBefore&&!u.$('#colony-include-carrier-stock')?.disabled);
  }
  await u.handleClick('sidebar',u.$('[data-click="sidebar"]'));check('Workspace sidebar can collapse',C.collapsed===true);
  u.$('.rail [data-group="trade"]').click();await wait(()=>C.group==='trade'&&!C.collapsed&&!!C.page);check('Choosing a group reopens its own sidebar',!u.$('.sidebar').classList.contains('collapsed'));
 }finally{await u.call('settings',{values:{language:before.language}});await u.navigate(before.tab,{mode:before.mode});if(C.collapsed!==before.collapsed)await u.handleClick('sidebar',u.$('[data-click="sidebar"]'));}
 return results;
}
window.V27Smoke={run};
})();
