/* This scenario uses only the native launcher's guarded offline fixtures. */
(()=>{'use strict';
async function run(C,u){
 if(window.COPILOT_SMOKE!==true)throw Error('Commander smoke requires an isolated launcher');
 const checks=[],original={tab:C.tab,mode:C.mode,language:C.state.settings?.language,updates:C.state.settings?.autoUpdateEnabled};
 const check=(name,ok)=>checks.push({name,ok:!!ok});
 const waitForUpdateToggle=async enabled=>{
  const until=Date.now()+6000;
  while(Date.now()<until){
   const input=u.$('#auto-update-enabled');
   if(input&&!input.disabled&&input.checked===enabled&&C.state.settings?.autoUpdateEnabled===enabled)return;
   await new Promise(resolve=>setTimeout(resolve,20));
  }
  throw Error('Update checkbox did not settle at '+(enabled?'enabled':'disabled'));
 };
 try{
  for(const language of ['en','ru']){
   await u.call('settings',{values:{language}});
   await u.navigate('ranks');
   const ranks=C.state.ranks?.disciplines||[];
   check(language+': all eight rank observations reached the renderer',ranks.length===8&&ranks.every(row=>Number.isFinite(row.rankIndex)));
   check(language+': known zero progress remains zero',ranks.find(row=>row.id==='exobiologist')?.progressPercent===0);
   check(language+': rank page uses a progress bar for every discipline',u.$$('#page [role="progressbar"]').length>=8);
   check(language+': rank guide module is available',typeof window.RanksUI?.smoke==='function');
   if(window.RanksUI?.smoke)checks.push(...await window.RanksUI.smoke(C,u));
   if(language==='en')check('English rank reader has no Russian prose',!/[\p{Script=Cyrillic}]/u.test(u.$('#page').innerText));
   await u.smokeCapture('ranks-'+language);
   await u.navigate('politics');
   check(language+': politics fixture remains separate from commander location',C.state.politics?.activeSystem?.name==='Demo Political System'&&C.state.commander?.system!=='Demo Political System');
   check(language+': politics module is available',typeof window.PoliticsUI?.smoke==='function');
   if(window.PoliticsUI?.smoke)checks.push(...await window.PoliticsUI.smoke(C,u));
   if(language==='en')check('English politics has no Russian interface prose',!/[\p{Script=Cyrillic}]/u.test(u.$('#page').innerText));
   await u.smokeCapture('politics-'+language);
   if(language==='en'){
    for(const [mode,name]of [['Фракции и конфликты','politics-factions'],['Цели сообщества','politics-goals']]){
     const button=u.$('#navigation [data-nav="politics"][data-nav-mode="'+mode+'"]');if(!button)throw Error('Galaxy sidebar view is missing');
     await u.navigate(button.dataset.nav,{mode:button.dataset.navMode});await u.smokeCapture(name);
    }
   }
  }
  await u.navigate('settings');
  const input=u.$('#auto-update-enabled');check('Automatic updates are enabled in a fresh profile',input?.checked===true);
  input.click();await waitForUpdateToggle(false);
  check('Update opt-out reaches the actual Go settings store',(await u.call('view',{tab:'settings'})).state?.settings?.autoUpdateEnabled===false);
  let refused=false;try{await u.call('updates-check');}catch{refused=true;}
  check('Isolated update checks cannot contact GitHub',refused);
  u.$('#auto-update-enabled').click();await waitForUpdateToggle(true);
  check('Automatic updates can be enabled again',u.$('#auto-update-enabled')?.checked===true);
 }finally{
  await u.call('settings',{values:{language:original.language,autoUpdateEnabled:original.updates!==false}});
  await u.navigate(original.tab,{mode:original.mode});
 }
 return checks;
}
window.V26Smoke={run};
})();
