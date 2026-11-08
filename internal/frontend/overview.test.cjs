'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const read=name=>fs.readFileSync(path.join(__dirname,name),'utf8');
const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function fixture(mode){
 const window={};vm.runInNewContext(read('overview.js'),{window});
 const C={mode,actions:[],state:{commander:{name:'Synthetic Commander',system:'Alpha',ship:'Synthetic Ship',shipName:'Personal name',vehicle:'ship',cargoTons:0,cargoCapacity:64},pilot:{credits:1000,fuel:{mainTons:12,mainCapacityTons:16},session:{jumps:2,distanceLy:90}},learning:{progress:{completed:4,total:27}},objectives:[]}};
 const u={esc,icon:id=>'<svg data-icon="'+esc(id)+'"></svg>',fmt:value=>String(value??0),money:value=>String(value??'—'),badge:value=>'<span class="badge">'+esc(value)+'</span>',heading:(title,description)=>'<div class="page-heading"><h1>'+esc(title)+'</h1><p>'+esc(description)+'</p></div>',btn:(label,action,icon,kind,attrs)=>'<button data-click="'+esc(action)+'" '+(attrs||'')+'>'+esc(label)+'</button>',renderModes:()=>'<div class="filters"><button data-mode="Состояние">Состояние</button></div>'};
 return {C,u,render:()=>window.OverviewUI.render(C,u)};
}
function rule(css,selector){const escaped=selector.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');const match=css.match(new RegExp('(?:^|})\\s*'+escaped+'\\s*\\{([^}]+)\\}'));assert.ok(match,'Missing layout rule '+selector);return match[1];}

test('commander state keeps four KPI cards and the following two panels in one spaced vertical layout',()=>{
 const f=fixture('Состояние'),html=f.render();
 assert.ok(html.startsWith('<div class="overview-page">'));assert.equal((html.match(/class="metric"/g)||[]).length,4);assert.equal((html.match(/class="panel ov-card"/g)||[]).length,2);
 assert.ok(html.indexOf('class="metrics ov-metrics"')<html.indexOf('class="ov-pair"'));
 assert.match(html,/data-click="overview-navigation"/);assert.match(html,/data-click="overview-cargo"/);
 assert.match(html,/>0 т</);assert.match(html,/Personal name/);
});

test('shared KPI spacing and overview stack provide a positive section gap without per-card margins',()=>{
 const shared=read('app.css'),overview=read('overview.css'),root=rule(shared,':root'),gap=Number(root.match(/--section-gap:\s*(\d+)px/)?.[1]);
 assert.ok(gap>=18&&gap<=32,'Section spacing should stay readable without consuming a large viewport band');
 assert.match(rule(shared,'.metrics'),/margin:\s*15px\s+0\s+var\(--section-gap\)/,'A KPI grid must leave space before a subsequent panel on any screen');
 const stack=rule(overview,'.overview-page');assert.match(stack,/display:\s*flex/);assert.match(stack,/flex-direction:\s*column/);assert.match(stack,/gap:\s*var\(--section-gap\)/);
 assert.match(rule(overview,'.overview-page>*'),/margin-block:\s*0/,'Child component margins must not double the shared stack gap');
 assert.match(overview,/@media\(max-width:1000px\)\{\.ov-pair\{grid-template-columns:1fr\}/,'The existing narrow-screen panel stack remains available');
});

test('overview plans preserve workflow destinations and escaped commander data inside the same stack',()=>{
 const f=fixture('Планы');f.C.state.commander.system='<img src=x onerror=bad()> ';f.C.state.objectives=[{title:'<private goal>',system:'Настройки',status:'active'}];
 const html=f.render();assert.ok(html.startsWith('<div class="overview-page">'));assert.equal((html.match(/class="metric"/g)||[]).length,4);assert.equal((html.match(/class="workflow-card"/g)||[]).length,4);
 for(const id of ['sale','carrier-trade','mining','colonization'])assert.match(html,new RegExp('data-nav="'+id+'"'));
 assert.match(html,/&lt;img src=x onerror=bad\(\)&gt;/);assert.match(html,/&lt;private goal&gt;/);assert.match(html,/Настройки/);assert.doesNotMatch(html,/<img|<private/);
});
