'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const window={};vm.runInNewContext(fs.readFileSync(__dirname+'/navigation.js','utf8'),{window});const nav=window.NavigationUI;
test('every section belongs to exactly one semantic group and settings has a single rail entry',()=>{
 const tabs=nav.groups.flatMap(group=>Array.from(group.tabs));assert.equal(new Set(tabs).size,tabs.length);assert.equal(tabs.includes('settings'),false);
 assert.equal(nav.groupFor('account'),'connections');assert.equal(nav.groupFor('commander'),'commander');assert.equal(nav.groupFor('ranks'),'commander');assert.equal(nav.groupFor('route'),'fleet');
});
test('commander submenu opens independent sections and never redirects the avatar to Frontier',()=>{
 const list=nav.entries({tabs:[],nav:[]},'commander');assert.equal(list.length,9);assert.ok(list.some(item=>item.tab==='ranks'));assert.ok(list.some(item=>item.mode==='Финансы'));assert.equal(list.some(item=>item.tab==='account'),false);
});
test('a selected commander subsection has one active item',()=>{
 const C={tab:'commander',mode:'Инвентарь',tabs:[],nav:[]};assert.equal(nav.entries(C,'commander').filter(item=>nav.selected(C,item)).length,1);
});
test('sidebar shows only its selected group and exposes hidden sections in a disclosure',()=>{
 const C={tab:'sale',mode:'',group:'trade',nav:['sale'],tabs:[{ID:'sale',Title:'Торговля'},{ID:'deals',Title:'Рейсы'},{ID:'carrier-trade',Title:'Торговля флотоносцем'},{ID:'learning',Title:'Академия'}]};
 const html=nav.sidebar(C,{t:x=>x,esc:x=>x,icon:()=>'',tabIcons:{}});assert.ok(html.includes('data-nav="sale"'));assert.ok(html.includes('data-nav="deals"'));assert.ok(html.includes('nav-additional'));assert.equal(html.includes('data-nav="learning"'),false);assert.equal(html.includes('data-nav="settings"'),false);
});
