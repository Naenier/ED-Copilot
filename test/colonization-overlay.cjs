'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const root = { innerHTML: '', getBoundingClientRect: () => ({ height: 500 }) };
let update;
const sandbox = { window: {}, document: { getElementById: () => root }, requestAnimationFrame: callback => callback(), setInterval: () => 1, clearInterval() {} };
vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/common.js'),'utf8'),sandbox);
sandbox.window.MiningUI.subscribe = callback => { update = callback; return {close(){}}; };
sandbox.window.CopilotAppearance = {apply(){}};
sandbox.window.addEventListener = () => {};
vm.runInContext(fs.readFileSync(path.join(__dirname,'../web/overlay.js'),'utf8'),sandbox);
const project = { name:'Synthetic colony', targetSystem:'Synthetic target',status:'building',totalMissing:100,concurrent:1,unknownBills:0,supplies:[],
 buildings:[{id:'b1',name:'Synthetic depot',requirements:[{name:'Сталь',commodity:'steel',missing:100,cargoAvailable:10,authoritative:true}]}],nextTask:{buildingId:'b1',text:'Доставить сталь'} };
const state = { settings:{activity:'colonization'},commander:{vehicle:'ship',cargoCapacity:50},colonization:{activeProject:project,cargoCapacity:50} };
update(state);
assert.match(root.innerHTML,/Сталь/); assert.match(root.innerHTML,/Доставить сталь/); assert.match(root.innerHTML,/На борту для поставки/);
for(const status of ['cancelled','complete']) {update({...state,colonization:{...state.colonization,activeProject:{...project,status,nextTask:null}}});assert(!root.innerHTML.includes('Ещё 100'));assert(!root.innerHTML.includes('РЕЙСОВ'));}
update({...state,colonization:{activeProject:{...project,unknownBills:1,totalMissing:0,buildings:[],nextTask:{text:'Посетите стройплощадку'}}}});
assert.match(root.innerHTML,/тоннаж ещё не включён/); assert(!root.innerHTML.includes('>0 <em>т'));
console.log('Colonization HUD: current delivery, unknown bills, and inactive-project supply suppression passed.');
