'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function fixture(language='en'){
 const window={CopilotTranslations:Object.assign({},...fs.readdirSync(path.join(__dirname,'../localization')).filter(name=>name.endsWith('_en.json')).sort().map(name=>JSON.parse(fs.readFileSync(path.join(__dirname,'../localization',name),'utf8'))))},scope={window,document:{documentElement:{}}};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'i18n.js'),'utf8'),scope);window.CopilotI18n.setLanguage(language);vm.runInNewContext(fs.readFileSync(path.join(__dirname,'ranks.js'),'utf8'),scope);
 const ids=['combat','trade','explore','soldier','exobiologist','cqc','federation','empire'],C={tab:'ranks',state:{settings:{language},ranks:{disciplines:ids.map(id=>({id,title:id,icon:'target',rankIndex:0,rankName:'First rank',rankNameEnglish:'First rank',nextRankIndex:1,nextRankName:'Next rank',progressPercent:0,maxed:false,rankUpdatedAt:'2026-11-08T12:00:00Z',progressUpdatedAt:'2026-11-08T12:01:00Z',rankSource:'journal',timeline:Array.from({length:14},(_,i)=>({rankIndex:i,rankName:'Rank '+i,status:i===0?'current':'locked',rewards:i===4?[{id:'sol',kind:'permit',title:'Sol permit',note:'Check the actual permit in the game.',entryId:'system/Sol',system:'Sol'}]:[]})),guide:{title:'A practical goal',sections:[{title:'Method',paragraphs:['An original practical method.']},{title:'Practice',paragraphs:['Compare observed progress without guessing XP.']}]}})),history:[]}}};
 const calls=[],toasts=[];let renders=0;
 const u={esc,t:window.CopilotI18n.t,fmt:window.CopilotI18n.number,date:()=> '08 Nov 2026',icon:name=>'<svg data-icon="'+esc(name)+'"></svg>',heading:(title,description)=>'<header><h1>'+esc(title)+'</h1><p>'+esc(description)+'</p></header>',btn:(label,action,icon,style,attrs)=>'<button data-click="'+esc(action)+'" '+(attrs||'')+'>'+esc(label)+'</button>',badge:(label,kind)=>'<span class="badge '+esc(kind)+'">'+esc(label)+'</span>',renderPage(){renders++},navigate:async id=>calls.push({op:'navigate',id}),call:async(op,params)=>calls.push({op,params}),toast:text=>toasts.push(text)};
 window.ReferenceUI={open:async(id)=>calls.push({op:'reference',id})};
 return {C,u,window,calls,toasts,api:window.RanksUI,render:()=>window.RanksUI.render(C,u),get renders(){return renders}};
}
test('rank overview renders eight English cards, real zero percentages and separate unknown state',()=>{
 const f=fixture();f.C.state.ranks.disciplines[1].rankIndex=null;f.C.state.ranks.disciplines[1].rankName=null;f.C.state.ranks.disciplines[1].progressPercent=null;
 const html=f.render();assert.equal((html.match(/data-rank-card=/g)||[]).length,8);assert.match(html,/Commander ranks/);assert.match(html,/0%/);assert.match(html,/Rank not yet known/);assert.match(html,/ranks-progress is-unknown/);assert.match(html,/rank requirements being met/);assert.doesNotMatch(html,/[А-Яа-яЁё]/);assert.equal(f.calls.length,0);f.api.init(f.C,f.u);assert.equal(f.calls.length,0);
});
test('rank selection and all-ranks controls keep canonical IDs and reveal the selected full guide',async()=>{
 const f=fixture();await f.api.handleClick('ranks-select',{dataset:{id:'federation'}},f.C,f.u);let html=f.render();assert.equal(f.C.rankWorkspace.selected,'federation');assert.match(html,/data-rank-current="federation"/);assert.match(html,/An original practical method/);assert.match(html,/Sol permit/);assert.ok((html.match(/class="ranks-level /g)||[]).length<14);await f.api.handleClick('ranks-all',null,f.C,f.u);html=f.render();assert.equal((html.match(/class="ranks-level /g)||[]).length,14);await f.api.handleClick('ranks-select',{dataset:{id:'unknown'}},f.C,f.u);assert.equal(f.C.rankWorkspace.selected,'federation');assert.equal(f.calls.length,0);
});
test('full naval progress does not silently become a promotion and missing dates are not fabricated',()=>{
 const f=fixture();f.C.rankWorkspace={selected:'federation',view:'overview',allRanks:false,page:0};const row=f.C.state.ranks.disciplines[6];row.rankName='Midshipman';row.nextRankName='Petty Officer';row.progressPercent=100;row.rankUpdatedAt='';row.progressUpdatedAt='';const html=f.render();assert.match(html,/Midshipman/);assert.match(html,/Petty Officer/);assert.match(html,/naval promotion mission/);assert.match(html,/Unknown/);assert.doesNotMatch(html,/Confirmed promotion/);
});
test('rank reward links and copy actions require an existing selected reward and preserve exact system text',async()=>{
 const f=fixture();f.C.state.ranks.disciplines[0].timeline[4].rewards[0].system='Настройки';await f.api.handleClick('ranks-reference',{dataset:{id:'system/Sol'}},f.C,f.u);assert.deepEqual(f.calls,[{op:'navigate',id:'encyclopedia'},{op:'reference',id:'system/Sol'}]);await f.api.handleClick('ranks-copy',{dataset:{system:'Настройки'}},f.C,f.u);assert.equal(f.calls.at(-1).params.text,'Настройки');const count=f.calls.length;await f.api.handleClick('ranks-reference',{dataset:{id:'javascript:evil()'}},f.C,f.u);await f.api.handleClick('ranks-copy',{dataset:{system:'Unlisted System'}},f.C,f.u);assert.equal(f.calls.length,count);await f.api.handleClick('ranks-tool',{dataset:{id:'exobiologist'}},f.C,f.u);assert.deepEqual(f.calls.at(-1),{op:'navigate',id:'exobiology'});
});
test('history is scoped to the selected discipline and mounts at most fifty events per page',async()=>{
 const f=fixture();f.C.state.ranks.history=Array.from({length:122},(_,i)=>({disciplineId:i<120?'combat':'trade',kind:i%2?'promotion':'observed',rankName:'Observed '+i,progressPercent:i===0?0:null,at:i===0?'': '2026-11-08T12:00:00Z',source:'journal'}));await f.api.handleClick('ranks-view',{dataset:{id:'history'}},f.C,f.u);
 const count=html=>(html.match(/<tbody>(.*?)<\/tbody>/s)?.[1].match(/<tr>/g)||[]).length;
 let html=f.render();assert.equal(count(html),50);assert.match(html,/first rank snapshot is not its earned date/);assert.match(html,/0%/);assert.match(html,/Unknown/);assert.doesNotMatch(html,/Observed 120/);await f.api.handleClick('ranks-page',{dataset:{direction:'next'}},f.C,f.u);assert.equal(count(f.render()),50);await f.api.handleClick('ranks-page',{dataset:{direction:'next'}},f.C,f.u);html=f.render();assert.equal(count(html),20);assert.match(html,/Observed 119/);await f.api.handleClick('ranks-page',{dataset:{direction:'next'}},f.C,f.u);assert.equal(count(f.render()),20);
});

test('mercenary practice opens general targets instead of the ship engineering workspace',async()=>{
 const f=fixture();await f.api.handleClick('ranks-select',{dataset:{id:'soldier'}},f.C,f.u);
 const html=f.render();assert.match(html,/data-rank-current="soldier"/);assert.match(html,/data-click="ranks-tool" data-id="soldier"/);
 await f.api.handleClick('ranks-tool',{dataset:{id:'soldier'}},f.C,f.u);assert.deepEqual(f.calls,[{op:'navigate',id:'targets'}]);
 await f.api.handleClick('ranks-tool',{dataset:{id:'combat'}},f.C,f.u);assert.deepEqual(f.calls.at(-1),{op:'navigate',id:'engineering'});
});
test('all game text, guide content, rewards and history are escaped without translating identities',async()=>{
 const f=fixture(),r=f.C.state.ranks.disciplines[0];r.title='<img src=x onerror=bad()>';r.rankName='Настройки';r.guide.sections[0].paragraphs=['<script>bad()</script>'];r.timeline[4].rewards[0].title='<iframe>';r.progressPercent='0;position:fixed';let html=f.render();assert.match(html,/&lt;img/);assert.match(html,/&lt;script&gt;/);assert.match(html,/&lt;iframe&gt;/);assert.match(html,/>Настройки<\/strong>/);assert.doesNotMatch(html,/<script|<iframe|<img/);assert.doesNotMatch(html,/width:0;position/);assert.match(html,/Percentage not yet observed/);
 f.C.state.ranks.history=[{disciplineId:'combat',kind:'observed',rankName:'<private>',at:'',source:'journal'}];await f.api.handleClick('ranks-view',{dataset:{id:'history'}},f.C,f.u);html=f.render();assert.match(html,/&lt;private&gt;/);
});
test('Russian and English controls change while selected discipline, page and rank values stay stable',()=>{
 const f=fixture('ru');f.C.rankWorkspace={selected:'empire',view:'history',allRanks:true,page:0};let html=f.render();assert.match(html,/Ранги командира/);assert.match(html,/История наблюдений/);f.window.CopilotI18n.setLanguage('en');html=f.render();assert.match(html,/Commander ranks/);assert.match(html,/Observation history/);assert.equal(f.C.rankWorkspace.selected,'empire');assert.equal(f.C.rankWorkspace.view,'history');assert.equal(f.C.state.ranks.disciplines[7].rankIndex,0);
});
test('rank smoke cannot run against a normal profile or bypass the root click handler',async()=>{
 const f=fixture();for(const flag of [false,undefined,1,'true']){f.window.COPILOT_SMOKE=flag;await assert.rejects(f.api.smoke(f.C,f.u),/isolated profile/);}f.window.COPILOT_SMOKE=true;await assert.rejects(f.api.smoke(f.C,f.u),/root click handler/);assert.equal(f.calls.length,0);
});
