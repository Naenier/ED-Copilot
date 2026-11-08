'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const script=fs.readFileSync(path.join(__dirname,'rank-insignia.js'),'utf8'),manifest=JSON.parse(fs.readFileSync(path.join(__dirname,'../data/rank-insignia-source.json'),'utf8'));
function reply(entries){const images={};for(const row of entries){const proof=manifest.assets.find(p=>p.discipline===row.id&&(p.kind==='faction'||p.rankIndex===row.rankIndex));if(proof)images[proof.kind==='faction'?row.id:row.id+'/'+row.rankIndex]='data:image/png;base64,'+fs.readFileSync(path.join(__dirname,'../data',proof.file)).toString('base64');}return {images};}
function fixture(){const window={};vm.runInNewContext(script,{window});const calls=[],C={tab:'ranks',layer:false};let renders=0;const u={icon:id=>'<svg data-icon="'+id+'"></svg>',async call(op,body){calls.push({op,body});return reply(body.entries);},renderPage(){renders++;}};return {api:window.RankInsignia,u,C,calls,get renders(){return renders}};}

test('startup contains no rank bitmaps and a neutral placeholder becomes the exact requested offline image after loading',async()=>{
 assert.ok(Buffer.byteLength(script)<5000);assert.doesNotMatch(script,/iVBORw0KGgo|https?:\/\//);const f=fixture(),row={id:'combat',rankIndex:0};assert.equal(f.api.source(row),'');assert.match(f.api.render(row,f.u),/is-unknown/);assert.doesNotMatch(f.api.render(row,f.u),/<img/);
 await f.api.load([row],f.u,f.C);assert.equal(f.calls.length,1);assert.equal(f.calls[0].op,'rank.insignia');assert.equal(f.api.source(row),reply([row]).images['combat/0']);assert.match(f.api.render(row,f.u),/<img src="data:image\/png;base64,/);assert.equal(f.renders,1);assert.ok(Object.isFrozen(f.api));
 await f.api.load([row],f.u,f.C);assert.equal(f.calls.length,1);assert.equal(f.renders,1);
});

test('six career ladders and both naval crests are selected by exact bounded IDs and indices without an Elite fallback',async()=>{
 const f=fixture();for(const id of ['combat','trade','explore','cqc','soldier','exobiologist']){const rows=Array.from({length:14},(_,rankIndex)=>({id,rankIndex}));await f.api.load(rows,f.u,f.C);assert.equal(new Set(rows.map(row=>f.api.source(row))).size,14);}
 for(const id of ['federation','empire']){await f.api.load([{id,rankIndex:0},{id,rankIndex:14}],f.u,f.C);assert.equal(f.api.source({id,rankIndex:0}),f.api.source({id,rankIndex:14}));}
 for(const rankIndex of [null,undefined,'',false,true,-1,14,99,NaN,Infinity,1.5,'0'])assert.equal(f.api.source({id:'combat',rankIndex}),'');
 const count=f.calls.length;await f.api.load([{id:'../../private',rankIndex:0},{id:'combat',rankIndex:null}],f.u,f.C);assert.equal(f.calls.length,count);assert.equal(f.api.source({id:'unlisted',rankIndex:8}),'');
 assert.notEqual(f.api.source({id:'federation',rankIndex:0}),f.api.source({id:'empire',rankIndex:0}));
});

test('concurrent renders deduplicate requests, preserve different ranks, and do not repaint a different tab or an open dialog',async()=>{
 const f=fixture();let complete;f.u.call=async(op,body)=>{f.calls.push({op,body});return new Promise(resolve=>{complete=()=>resolve(reply(body.entries));});};
 const rows=[{id:'trade',rankIndex:0},{id:'trade',rankIndex:0},{id:'trade',rankIndex:8}],first=f.api.load(rows,f.u,f.C),second=f.api.load(rows,f.u,f.C);await Promise.resolve();assert.equal(f.calls.length,1);assert.equal(f.calls[0].body.entries.length,2);complete();await Promise.all([first,second]);assert.equal(f.renders,1);assert.notEqual(f.api.source(rows[0]),f.api.source(rows[2]));
 const late=f.api.load([{id:'combat',rankIndex:0}],f.u,f.C);await Promise.resolve();f.C.tab='settings';complete();await late;assert.equal(f.renders,1);
 f.C.tab='ranks';const modal=f.api.load([{id:'explore',rankIndex:0}],f.u,f.C);await Promise.resolve();f.C.layer=true;complete();await modal;assert.equal(f.renders,1);
});

test('failed loads can be retried and malicious or unsolicited response values cannot enter the renderer',async()=>{
 const f=fixture(),row={id:'combat',rankIndex:2};f.u.call=async()=>{throw Error('offline bridge unavailable');};await assert.rejects(f.api.load([row],f.u,f.C),/unavailable/);assert.equal(f.api.source(row),'');
 f.u.call=async()=>({images:{'combat/2':'data:image/png;base64,bad" onerror="bad()',empire:'https://external'}});await f.api.load([row],f.u,f.C);assert.equal(f.api.source(row),'');assert.doesNotMatch(f.api.render(row,f.u),/onerror|https?:/);
 f.u.call=async(op,body)=>reply(body.entries);await f.api.load([row],f.u,f.C);assert.ok(f.api.source(row));assert.match(f.api.render(row,f.u,{size:'hero" onclick="bad()'}),/rank-insignia-card/);
 for(const size of ['small','card','hero'])assert.match(f.api.render(row,f.u,{size}),new RegExp('rank-insignia-'+size));
});

test('late art preserves a different commander mode and focused inventory input while retaining the offline cache',async()=>{
 const f=fixture();f.C.tab='commander';f.C.mode='Сводка';let complete;f.u.call=async(op,body)=>new Promise(resolve=>{complete=()=>resolve(reply(body.entries));});
 const rows=[{id:'combat',rankIndex:3}],summary=f.api.load(rows,f.u,f.C);await Promise.resolve();f.C.mode='Инвентарь';complete();await summary;assert.equal(f.renders,0);assert.ok(f.api.source(rows[0]));
 f.C.mode='Сводка';const focused=f.api.load([{id:'trade',rankIndex:3}],f.u,f.C);await Promise.resolve();f.u.$=selector=>{assert.equal(selector,'#page input:focus,#page textarea:focus,#page select:focus');return {value:'my unsaved filter'};};complete();await focused;assert.equal(f.renders,0);assert.ok(f.api.source({id:'trade',rankIndex:3}));
});
