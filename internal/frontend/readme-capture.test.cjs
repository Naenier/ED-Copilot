'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
test('documentation capture cannot start on a real or ordinary runtime',async()=>{
 for(const flags of [{},{COPILOT_SMOKE:true},{COPILOT_README:true},{COPILOT_SMOKE:'true',COPILOT_README:true}]){
  const window={...flags};vm.runInNewContext(fs.readFileSync(path.join(__dirname,'readme-capture.js'),'utf8'),{window});
  await assert.rejects(window.ReadmeCapture.run({},{}),/isolated fixture/);
 }
});

function fixture(language='en',bodyText=language==='ru'?'Командир и его имущество':'Commander and property'){
 const C={collapsed:false,state:{settings:{language},commander:{fid:'FNATIVE-SYNTHETIC',name:language==='en'?'Demo Commander':'Тестовый командир'},exobiology:{alerts:[]}},reference:{loading:false,entryLoading:false,list:{entries:[]}}},calls=[],captures=[];
 const window={COPILOT_README:true,COPILOT_SMOKE:true,ReferenceUI:{async open(id){C.reference.entryId=id;C.reference.entry={id};}}};
 const document={documentElement:{lang:language},body:{innerText:bodyText},images:[]};
 const u={
  $(selector){if(selector==='#page-scroll')return {scrollTop:100};if(selector==='#toasts')return {replaceChildren(){}};if(selector.startsWith('[data-commander-section='))return C.tab==='commander'?{}:null;if(selector==='[data-click="sidebar"]')return {dataset:{click:'sidebar'}};return null;},
  $$(selector){if(selector==='[data-rank-card]')return Array.from({length:8},()=>({}));if(selector==='.rail [data-click="settings-nav"]')return [{}];if(selector==='.sidebar [data-nav="settings"]')return [];return [];},
  async call(op,body){calls.push({op,body});if(op==='window'&&body.action==='status')return {displayMode:'fullscreen'};if(op==='reference.list')return {entries:[{id:'article/example'}]};return {};},
  async navigate(tab,options={}){C.tab=tab;C.mode=options.mode||'';if(tab==='deals')C.dealsUI={};if(tab==='encyclopedia')C.reference={loading:false,entryLoading:false,list:{entries:[]}};},
  async handleClick(action){assert.equal(action,'sidebar');C.collapsed=!C.collapsed;calls.push({op:'sidebar',collapsed:C.collapsed});},
  renderPage(){},async loadView(){},async smokeCapture(name){captures.push(name);}
 };
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'readme-capture.js'),'utf8'),{window,document,innerWidth:1920,innerHeight:1080,requestAnimationFrame:fn=>fn(),setTimeout,Date});
 return {C,u,window,document,calls,captures,run:()=>window.ReadmeCapture.run(C,u),report:()=>calls.find(row=>row.op==='smoke-result')?.body};
}

test('true documentation flags still reject real commanders and invalid language before any action',async()=>{
 for(const change of [C=>C.state.commander.fid='REAL',C=>C.state.settings.language='de']){
  const f=fixture();change(f.C);await assert.rejects(f.run(),/synthetic commander/);assert.equal(f.calls.length,0);assert.equal(f.captures.length,0);
 }
});

test('English and Russian capture flows visit commander sections and restore compact navigation',async()=>{
 for(const language of ['en','ru']){
  const f=fixture(language);await f.run();const report=f.report();assert.equal(report.ok,true,JSON.stringify(report.problems));assert.equal(report.language,language);assert.equal(f.captures.length,17);assert.equal(new Set(f.captures).size,17);
  for(const name of ['readme-commander','readme-ranks','readme-fleet','readme-inventory','readme-navigation','readme-navigation-compact'])assert.ok(f.captures.includes(name));
  assert.deepEqual(f.calls.filter(row=>row.op==='sidebar').map(row=>row.collapsed),[true,false]);assert.equal(f.C.collapsed,false);assert.ok(report.pages.every(page=>page.language===language));
  const article=f.calls.find(row=>row.op==='reference.list'&&row.body.category==='articles');assert.equal(article.body.query,language==='en'?'Tritium':'Тритий');
  assert.equal(f.calls.filter(row=>row.op==='window'&&row.body.action==='fullscreen').length,1);
 }
});

test('only English photography rejects visible Cyrillic and a capture failure still restores the sidebar',async()=>{
 const en=fixture('en','Unexpected русский label');await en.run();assert.equal(en.report().ok,false);assert.ok(en.report().pages.every(page=>page.untranslated.includes('Unexpected русский label')));
 const ru=fixture('ru','Русский текст and canonical ship names');await ru.run();assert.equal(ru.report().ok,true);assert.ok(ru.report().pages.every(page=>page.untranslated.length===0));
 const failed=fixture();failed.u.smokeCapture=async name=>{if(name==='readme-navigation-compact')throw Error('Synthetic capture failure');};await failed.run();assert.equal(failed.C.collapsed,false);assert.equal(failed.report().ok,false);assert.ok(failed.report().problems.includes('Synthetic capture failure'));
});
