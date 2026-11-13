'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const window={};vm.runInNewContext(fs.readFileSync(__dirname+'/navigation.js','utf8'),{window});const nav=window.NavigationUI;
test('every section belongs to exactly one semantic group and settings has a single rail entry',()=>{
 const tabs=nav.groups.flatMap(group=>Array.from(group.tabs));assert.equal(new Set(tabs).size,tabs.length);assert.equal(tabs.includes('settings'),false);
 assert.equal(nav.groupFor('account'),null);assert.equal(nav.groupFor('integrations'),null);
 assert.equal(nav.groupFor('commander'),'commander');assert.equal(nav.groupFor('ranks'),'commander');assert.equal(nav.groupFor('route'),'fleet');
 for(const tab of ['colonization','mining','engineering','exobiology']){
  assert.equal(nav.groupFor(tab),tab);assert.equal(nav.usesSidebarModes(tab),true);
 }
});
test('commander submenu opens independent sections and never redirects the avatar to Frontier',()=>{
 const list=nav.entries({tabs:[],nav:[]},'commander');assert.equal(list.length,8);assert.ok(list.some(item=>item.tab==='ranks'));assert.ok(list.some(item=>item.mode==='Финансы'));assert.equal(list.some(item=>item.tab==='account'||item.mode==='Репутация'),false);
});
test('a selected commander subsection has one active item',()=>{
 const C={tab:'commander',mode:'Инвентарь',tabs:[],nav:[]};assert.equal(nav.entries(C,'commander').filter(item=>nav.selected(C,item)).length,1);
});
test('trade opens its ready routes first and shows the related tools together',()=>{
 const C={tab:'deals',mode:'',group:'trade',nav:['sale'],tabs:[{ID:'sale',Title:'Рынки'},{ID:'deals',Title:'Рейсы'},{ID:'carrier-trade',Title:'Торговля флотоносцем'},{ID:'learning',Title:'Академия'}]};
 const html=nav.sidebar(C,{t:x=>x,esc:x=>x,icon:()=>'',tabIcons:{}});
 assert.ok(html.indexOf('data-nav="deals"')<html.indexOf('data-nav="sale"'));
 assert.match(html,/data-nav="carrier-trade"/);
 assert.doesNotMatch(html,/nav-additional|data-nav="learning"|data-nav="settings"/);
});
test('colony tools include system history without duplicate supply screens',()=>{
 const C={tab:'colonization',mode:'Строительство',tabs:[],nav:[]};
 assert.deepEqual(Array.from(nav.entries(C,'colonization'),item=>item.mode),['Обзор','Выбор системы','Строительство','Журнал снабжения','Бюджет','Динамика системы']);
 assert.equal(nav.entries(C,'colonization').filter(item=>nav.selected(C,item)).length,1);
});
test('Cartography appears in Galaxy and keeps the existing planner route',()=>{
 const C={tab:'riches',mode:'',group:'galaxy',tabs:[{ID:'riches',Title:'Картография'}],nav:[]};
 assert.equal(nav.groupFor('riches'),'galaxy');
 const html=nav.sidebar(C,{t:x=>x,esc:x=>x,icon:()=>'',tabIcons:{}});
 assert.match(html,/data-nav="riches"[^>]*aria-current="page"[^>]*><span>Картография<\/span>/);
 assert.equal(nav.entries(C,'work').some(item=>item.tab==='riches'),false);
});
test('surface coordinates is a distinct Galaxy tab next to ship navigation',()=>{
 const C={tab:'surface-navigation',mode:'',group:'galaxy',tabs:[{ID:'ship-route',Title:'Маршрут корабля'},{ID:'surface-navigation',Title:'Полёт по координатам'}],nav:[]};
 assert.equal(nav.groupFor(C.tab),'galaxy');
 const entries=nav.entries(C,'galaxy');
 assert.equal(entries.findIndex(item=>item.tab===C.tab),entries.findIndex(item=>item.tab==='ship-route')+1);
 assert.deepEqual(Array.from(entries.filter(item=>nav.selected(C,item)),item=>item.title),['Полёт по координатам']);
});
