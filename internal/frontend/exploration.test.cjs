'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const translations = {
 ...require('../localization/frontend_en.json'),
 ...require('../localization/exobiology_ui_en.json'),
 ...require('../localization/exploration_ui_en.json')
};
const window = {};
vm.runInNewContext(fs.readFileSync(path.join(__dirname,'exploration.js'),'utf8'),{window});
const explorer = window.ExplorationUI;
const esc = value => String(value??'').replace(/[&<>"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'})[c]);
function ui(lang='en') {
 return {t:(key,vars)=>String(lang==='en'?translations[key]||key:key).replace(/\{(\w+)\}/g,(_,name)=>vars?.[name]??''),
  esc,fmt:String,locale:()=>lang,heading:title=>'<h1>'+esc(title)+'</h1>',renderModes:()=>''};
}
function setup(mode,key,data){
 return {tab:'exploration',mode,state:{planWorkspace:{key:'owner'}},explorationUI:{owner:'owner',sequence:0,queries:{},data:{[key]:data},errors:{}}};
}

test('expedition keeps actual and estimated credits distinct and renders the body tree',()=>{
 const C=setup('Экспедиции','expeditions',{
  expedition:{id:'one',name:'Reach',returnSystem:'Sol',path:[{system:'A',coordinates:[0,0,0]},{system:'B',coordinates:[10,0,10]}],travelCount:2},
  expeditions:[{id:'one',name:'Reach'}],wallet:{onboardEstimate:100000,confirmedCredits:200000,lostEstimate:0},
  systems:{rows:[{system:'Reach <One>',address:'18446744073709551614',scanned:2,mapped:1}],total:50,page:0,pageSize:30},
  system:{system:'Reach <One>',address:'18446744073709551614'},
  bodies:{rows:[{name:'Reach 2',id:'2',systemAddress:'18446744073709551614',depth:1,parent:'Reach 1',kind:'earthlike',mappedAt:'2026-11-13T00:00:00Z',dataState:'sold'}]}
 });
 const en=explorer.render(C,ui());
 assert.match(en,/<h1>Expeditions<\/h1>/);
 assert.match(en,/Data aboard, estimated/);
 assert.match(en,/Confirmed payout/);
 assert.match(en,/data-address="18446744073709551614"/);
 assert.match(en,/Reach &lt;One&gt;/);
 assert.match(en,/class="explore-depth-1"/);
 assert.match(en,/id="explore-path"/);
 assert.match(en,/data-explore="page"/);
 assert.match(explorer.render(C,ui('ru')),/Подтверждённая выплата/);
});

test('surveys label tentative boxel candidates and allow reordering via editable source list',()=>{
 const target={system:'Example AA-A d0',candidate:true,progress:{},note:'Test'};
 const C=setup('Обследования','surveys',{survey:{id:'one',name:'Survey',criterion:'fss',targets:[target],next:target,
  completed:0,results:{rows:[target],total:1,page:0,pageSize:30},boxel:{prefix:'Example AA-A d',widthLy:80}},
  surveys:[{id:'one',name:'Survey'}],worlds:{rows:[]}});
 const en=explorer.render(C,ui());
 assert.match(en,/Verify that each system exists in the game/);
 assert.match(en,/Candidate/);
 assert.match(en,/data-explore-import/);
 assert.match(en,/data-explore="export"/);
 assert.match(en,/data-explore="skip-target"/);
 assert.match(explorer.render(C,ui('ru')),/Следующая система/);
});

test('findings show the evidence in the interface language and open the exact body',()=>{
 const C=setup('Находки','findings',{profile:{id:'default'},profiles:[{id:'default'}],findings:{total:1,page:0,pageSize:30,rows:[
  {title:'Землеподобный мир',englishTitle:'Earth-like world',detail:'Класс тела: землеподобный',englishDetail:'Body class: Earth-like',
   system:'Here',body:'Here 2',bodyId:'2',systemAddress:'9007199254740993'}
 ]}});
 const en=explorer.render(C,ui());
 assert.match(en,/Body class: Earth-like/);
 assert.match(en,/data-address="9007199254740993" data-body="2"/);
 assert.doesNotMatch(en,/Класс тела: землеподобный/);
 assert.match(explorer.render(C,ui('ru')),/Класс тела: землеподобный/);
});

test('a notification prefill opens the requested body once',()=>{
 const C=setup('Находки','findings',{});
 C.prefill={systemName:'Reach',systemAddress:'18446744073709551614',bodyId:'0'};
 explorer.render(C,ui());
 const query=C.explorationUI.queries.findings;
 assert.equal(query.systemAddress,'18446744073709551614');
 assert.equal(query.systemName,'Reach');
 assert.equal(query.bodyId,'0');
 delete query.systemAddress;
 delete query.bodyId;
 explorer.render(C,ui());
 assert.equal(query.bodyId,undefined);
 C.prefill={...C.prefill,bodyId:'15'};
 explorer.render(C,ui());
 assert.equal(query.bodyId,'15');
});
