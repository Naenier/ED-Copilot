'use strict';
const assert = require('node:assert/strict');
const test = require('node:test');
const fs = require('node:fs');
const vm = require('node:vm');

const window = {};
vm.runInNewContext(fs.readFileSync(__dirname+'/colony-history.js','utf8'),{window});
const history = window.ColonyHistoryUI;
const esc = value => String(value ?? '').replace(/[&<>"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));
const ui = {t:x=>x,esc,fmt:x=>String(Math.round(x*100)/100),heading:(title,detail,controls='')=>`<h1>${esc(title)}</h1>${esc(detail)}${controls}`,locale:()=> 'ru-RU'};
const view = (samples,milestones=[])=>({tab:'colonization',mode:'Динамика системы',state:{colonyHistory:{
  system:'Alpha',selectedProjectId:'a',projects:[{id:'a',name:'Alpha'}],samples,milestones
}}});

test('actual colony history charts show signed changes, faction influence and confirmed milestones',()=>{
  const input = view([
    {at:'2026-11-01T12:00:00Z',population:120,security:'Low',factions:[{name:'Test <faction>',influence:.4}]},
    {at:'2026-11-03T12:00:00Z',population:100,security:'High',factions:[{name:'Test <faction>',influence:.6}]}
  ],[
    {at:'2026-11-02T12:00:00Z',kind:'completion',name:'Test & Port'},
    {at:'2026-11-02T14:00:00Z',kind:'first-visit',name:'ARX port'}
  ]);
  const html = history.render(input,ui);
  assert.match(html,/График населения/);
  assert.match(html,/>-20</);
  assert.match(html,/>-10</);
  assert.match(html,/\+20 п. п\./);
  assert.match(html,/Низкая → Высокая/);
  assert.match(html,/colony-history-marker.*Завершение подтверждено · Test &amp; Port/);
  assert.match(html,/Первое посещение объекта/);
  assert.match(html,/Test &lt;faction&gt;/);
  assert.doesNotMatch(html,/Test <faction>/);
});

test('missing observations are not fabricated or graphed as zero',()=>{
  const empty=history.render(view([]),ui);
  assert.match(empty,/Пока нет наблюдений/);
  const one=history.render(view([{at:'2026-11-01T12:00:00Z',population:0}]),ui);
  assert.match(one,/>0</);
  assert.match(one,/Для графика нужно минимум два наблюдения/);
  assert.doesNotMatch(one,/<svg/);
  const missing=history.render(view([{at:'2026-11-01T12:00:00Z',security:'Low'}]),ui);
  assert.doesNotMatch(missing,/<h2>Население/);
});

test('period filtering is relative to the latest actual observation',()=>{
  const rows = [{at:'2026-10-01T12:00:00Z'},{at:'2026-10-10T12:00:00Z'},{at:'2026-10-12T12:00:00Z'}];
  assert.equal(history.samplesInRange(rows,'week').length,2);
  assert.equal(history.samplesInRange(rows,'all').length,3);
});
