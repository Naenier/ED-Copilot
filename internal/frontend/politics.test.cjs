'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function fixture(language='en') {
  const at = new Date().toISOString();
  const meta = {known:true,source:'spansh',observedAt:at,fetchedAt:at};
  return {tab:'politics',mode:'Политика',state:{settings:{language},politics:{
    selectedSystem:'Fixture',currentSystem:'Current',systems:[{name:'Fixture'}],
    activeSystem:{
      name:'Fixture',address:'9007199254740999',detailsFetchedAt:at,info:{...meta,controllingFaction:'<script>bad()</script>',population:0},
      factions:{...meta,entries:[{name:'War Faction',influence:0,state:'War',activeStates:['War'],pendingStates:['Boom']}]},
      conflicts:{...meta,source:'journal',entries:[{
        type:'war',status:'active',faction1:{name:'War Faction',stake:'Port',wonDays:0},faction2:{name:'Other',wonDays:2},
      }]},
      powerplay:{...meta,controllingPower:'Power A',state:'Stronghold',controlProgress:0,reinforcement:0,undermining:null},
      thargoid:{...meta,state:'Invasion',progress:.12},
    },
    communityGoals:{...meta,source:'frontier-public',language,entries:[{
      id:'1',title:'<img onerror=bad()>',system:'Campaign System',station:'Campaign Port',objective:'Deliver <supplies>',
      currentTotal:0,target:100,description:'Reward <script>bad()</script>',
    }]},
    personalGoals:{...meta,source:'journal',entries:[{
      id:'1',title:'Joined goal',playerContribution:0,bonus:200,percentileBand:75,tier:'Tier 1',complete:false,
    }]},
    powerIndex:{...meta,powers:[{name:'Power A',indexedSystems:12},{name:'Power B',indexedSystems:0}],
      states:[{name:'Stronghold',indexedSystems:2},{name:'Fortified',indexedSystems:3}]},
    search:{...meta,kind:'powerplay',power:'Power A',state:'Contested',page:1,limit:10,total:22,entries:[{
      name:'Public result',powerplay:{...meta,controllingPower:'Power A',state:'Contested'},
    }]},
    status:{running:false},sources:[{name:'Spansh',url:'https://spansh.co.uk/systems'},{name:'bad',url:'javascript:bad()'}],
  }}};
}

function runtime(C=fixture()) {
  const listeners = new Map();
  const scope = vm.createContext({window:{},document:{addEventListener:(name,fn) => listeners.set(name,fn)}});
  for (const file of ['freshness.js','politics.js']) {
    vm.runInContext(fs.readFileSync(path.join(__dirname,file),'utf8'),scope);
  }
  const api = scope.window.PoliticsUI;
  const calls = [];
  const esc = value => String(value??'').replace(/[&<>"']/g,ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  const u = {
    esc, fmt:n => String(n), heading:title => `<h1>${esc(title)}</h1>`,
    call:async(op,body) => {calls.push({op,body:JSON.parse(JSON.stringify(body))});return {};},
    loadView:async() => {calls.push({op:'loadView'});api.init(C,u);},
    renderPage:() => api.init(C,u),
    navigate:async(tab,opts) => {C.tab=tab;C.mode=opts?.mode || '';api.init(C,u);},
    toast:() => {}, $:() => null,
  };
  return {C,u,api,calls,listeners,window:scope.window,document:scope.document};
}
const flush = () => new Promise(resolve => setImmediate(resolve));
const requests = r => r.calls.filter(item => item.op === 'politics-refresh').map(item => item.body.values);

test('Galaxy has distinct views and hides Factions without actual rows', () => {
  const r = runtime();
  assert.deepEqual(Array.from(r.api.navigationModes(r.C),x => x.mode),
    ['Политика','Цели сообщества','Пауэрплей','Фракции и конфликты']);
  const html = r.api.render(r.C,r.u);
  assert.match(html,/<h1>Politics<\/h1>/);
  assert.match(html,/data-system-search="single"/);
  assert.match(html,/&lt;script&gt;bad/);
  assert.match(html,/>0<\/dd>/);
  assert.doesNotMatch(html,/9007199254740999|Thargoid|javascript:bad|<script>|pol-tabs/);
  r.C.state.politics.activeSystem.factions.entries = [];
  r.C.state.politics.activeSystem.conflicts.entries = [{faction1:{name:'Incomplete record'}}];
  assert.deepEqual(Array.from(r.api.navigationModes(r.C),x => x.mode),['Политика','Цели сообщества','Пауэрплей']);
});

test('Opening each Galaxy view automatically requests missing data for its scope', async () => {
  for (const [mode,expected] of [
    ['Политика',{kind:'system',system:'Fixture',keepSelection:true}],
    ['Цели сообщества',{kind:'goals',language:'ru'}],
    ['Пауэрплей',{kind:'powerplay',power:'',state:'',page:0}],
  ]) {
    const C = fixture('ru');
    C.mode = mode;
    C.state.politics.activeSystem = {name:'Fixture'};
    delete C.state.politics.communityGoals;
    delete C.state.politics.search;
    const r = runtime(C);
    assert.match(r.api.render(C,r.u),mode === 'Пауэрплей' ? /Загружаем страницу 1/ : /Обновляем данные/);
    r.api.init(C,r.u);
    r.api.init(C,r.u);
    await flush();
    assert.deepEqual(requests(r),[expected]);
  }
});

test('fresh data is shown without a duplicate network call, and isolated mode never fetches', async () => {
  const r = runtime();
  for (const mode of ['Политика','Цели сообщества','Пауэрплей','Фракции и конфликты']) {
    await r.u.navigate('politics',{mode});
    await flush();
  }
  assert.deepEqual(requests(r),[]);
  r.window.COPILOT_SMOKE = true;
  r.C.state.politics.search = {};
  await r.u.navigate('politics',{mode:'Пауэрплей'});
  await flush();
  assert.deepEqual(requests(r),[]);
});

test('a recent Powerplay index row still receives full system details when opened', async () => {
  const C = fixture();
  delete C.state.politics.activeSystem.detailsFetchedAt;
  delete C.state.politics.activeSystem.conflicts;
  const r = runtime(C);
  r.api.init(C,r.u);
  await flush();
  assert.deepEqual(requests(r),[{kind:'system',system:'Fixture',keepSelection:true}]);
});

test('rapid navigation waits for the active public request before loading another tab', async () => {
  const C = fixture();
  C.state.politics.activeSystem = {name:'Fixture'};
  delete C.state.politics.communityGoals;
  const r = runtime(C);
  let unblock;
  const oldCall = r.u.call;
  r.u.call = async(op,body) => {
    await oldCall(op,body);
    if (op==='politics-refresh' && body.values.kind==='system') {
      await new Promise(resolve => {unblock=resolve;});
    }
  };
  await r.u.navigate('politics',{mode:'Политика'});
  await flush();
  await r.u.navigate('politics',{mode:'Цели сообщества'});
  await flush();
  assert.equal(requests(r).length,1);
  unblock();
  await flush();
  assert.deepEqual(requests(r).map(item => item.kind),['system','goals']);
});

test('public and journal goals retain progress, zero contribution and escaped descriptions', () => {
  const r = runtime();
  r.C.mode = 'Цели сообщества';
  const html = r.api.render(r.C,r.u);
  assert.match(html,/Public community goals/);
  assert.match(html,/Your community goals/);
  assert.match(html,/Your contribution<\/dt><dd>0<\/dd>/);
  assert.match(html,/&lt;img onerror=bad/);
  assert.match(html,/Deliver &lt;supplies&gt;/);
  assert.match(html,/aria-valuenow="0"/);
  assert.doesNotMatch(html,/<img|<script|politics-refresh|Refresh public/);
});

test('Powerplay opens cached figures, auto-applies filters and bounds pagination', async () => {
  const r = runtime();
  r.C.mode = 'Пауэрплей';
  let html = r.api.render(r.C,r.u);
  assert.match(html,/Controlled systems/);
  assert.match(html,/Power B/);
  assert.match(html,/Public result/);
  assert.doesNotMatch(html,/Search index|Refresh powers|<th>Source/);
  await r.api.handleChange({target:{dataset:{politicsField:'state'},value:'Fortified'}},r.C,r.u);
  await flush();
  assert.deepEqual(requests(r)[0],{kind:'powerplay',power:'Power A',state:'Fortified',page:0});
  assert.doesNotMatch(r.api.render(r.C,r.u),/Public result/);
  await r.api.handleClick('politics-page',{dataset:{page:'101'}},r.C,r.u);
  await flush();
  assert.equal(requests(r).length,1);
  await r.api.handleClick('politics-page',{dataset:{page:'2'}},r.C,r.u);
  await flush();
  assert.deepEqual(requests(r)[1],{kind:'powerplay',power:'Power A',state:'Fortified',page:2});
});

test('Powerplay keeps the displayed table while the next page is loading', async () => {
  const r = runtime();
  r.C.mode = 'Пауэрплей';
  const p = r.C.state.politics;
  const previous = p.search;
  let release;
  const call = r.u.call;
  r.u.call = async (op,body) => {
    await call(op,body);
    if (op === 'politics-refresh') await new Promise(resolve => {release = resolve;});
  };
  assert.match(r.api.render(r.C,r.u),/Public result/);
  await r.api.handleClick('politics-page',{dataset:{page:'2'}},r.C,r.u);
  await flush();
  let html = r.api.render(r.C,r.u);
  assert.match(html,/aria-busy="true"><table/);
  assert.match(html,/Public result/);
  assert.match(html,/Page 2/);
  assert.match(html,/Loading page 3/);
  assert.match(html,/data-page="2" disabled/);
  assert.deepEqual(requests(r),[{kind:'powerplay',power:'Power A',state:'Contested',page:2}]);

  r.C.state.politicsNetwork = {running:true,options:requests(r)[0]};
  release();
  await flush();
  html = r.api.render(r.C,r.u);
  assert.match(html,/aria-busy="true"><table/);
  assert.match(html,/Public result/);

  p.search = {...previous,page:2,entries:[{name:'Third page system',powerplay:{}}]};
  r.C.state.politicsNetwork.running = false;
  await r.u.loadView(true);
  html = r.api.render(r.C,r.u);
  assert.match(html,/aria-busy="false"><table/);
  assert.match(html,/Page 3/);
  assert.match(html,/Third page system/);
  assert.doesNotMatch(html,/Public result|Loading page/);
});

test('politics refresh waits for visibility and runs when the focused view ages', async () => {
  const r = runtime();
  const scheduled = [];
  r.window.setTimeout = callback => scheduled.push(callback);
  r.document.hidden = true;
  r.C.state.politics.limits = {cacheMinutes:5};
  r.C.state.politics.activeSystem.detailsFetchedAt = new Date(Date.now()-301000).toISOString();
  r.api.init(r.C,r.u);
  await flush();
  assert.deepEqual(requests(r),[]);
  assert.equal(scheduled.length,1);
  r.document.hidden = false;
  scheduled.shift()();
  await flush();
  assert.deepEqual(requests(r),[{kind:'system',system:'Fixture',keepSelection:true}]);
  scheduled.shift()();
  await flush();
  assert.equal(requests(r).length,1);
  r.C.tab = 'overview';
  scheduled.shift()();
  assert.equal(scheduled.length,0);
});

test('faction influence and exact conflict pairings appear as separate cards', () => {
  const r = runtime();
  r.C.mode = 'Фракции и конфликты';
  const html = r.api.render(r.C,r.u);
  assert.match(html,/War Faction/);
  assert.match(html,/Observed conflicts/);
  assert.match(html,/Days won<\/dt><dd>0<\/dd>/);
  assert.match(html,/aria-valuenow="0"/);
  assert.doesNotMatch(html,/<table|<th>Source/);
  delete r.C.state.politics.activeSystem.conflicts;
  assert.doesNotMatch(r.api.render(r.C,r.u),/Observed conflicts/);
});

test('selecting a suggested or cached system requests that context and can copy its name', async () => {
  const r = runtime();
  const previous = r.u.call;
  r.u.call = async(op,body) => {
    const result = await previous(op,body);
    if (op === 'politics' && body.values.system) {
      r.C.state.politics.selectedSystem = body.values.system;
      r.C.state.politics.activeSystem = {name:body.values.system};
    }
    return result;
  };
  r.api.init(r.C,r.u);
  await r.api.handleChange({isTrusted:false,target:{dataset:{politicsField:'system'},value:'Typed System'}},r.C,r.u);
  await flush();
  assert.deepEqual(r.calls.find(c => c.op==='politics').body,{values:{action:'select-system',system:'Typed System'}});
  assert.deepEqual(requests(r),[{kind:'system',system:'Typed System',keepSelection:true}]);
  await r.api.handleClick('politics-copy',{dataset:{system:'Typed System'}},r.C,r.u);
  assert.deepEqual(r.calls.at(-1),{op:'native',body:{action:'copy',text:'Typed System'}});
});

test('failed refresh keeps cached observations and avoids duplicate attempts', async () => {
  const r = runtime();
  r.C.mode = 'Цели сообщества';
  r.C.state.politics.communityGoals.fetchedAt = '2024-01-01T00:00:00Z';
  const previous = r.u.call;
  r.u.call = async(op,body) => {
    await previous(op,body);
    if (op === 'politics-refresh') throw Error('network failure');
  };
  r.api.init(r.C,r.u);
  await flush();
  r.api.init(r.C,r.u);
  await flush();
  const html = r.api.render(r.C,r.u);
  assert.equal(requests(r).length,1);
  assert.match(html,/Could not update these observations/);
  assert.match(html,/Deliver &lt;supplies&gt;/);
});

test('Politics distinguishes provider failure, empty results and partial cached data', async () => {
  const r = runtime();
  r.C.mode = 'Цели сообщества';
  const p = r.C.state.politics;
  const age = '2026-10-01T12:00:00Z';
  p.communityGoals = {...p.communityGoals, entries:[], fetchedAt:age,
    quality:{state:'stale',coverage:'partial',observedAt:age,fetchedAt:'2026-10-01T13:00:00Z'}};
  p.status = {kind:'goals',parameters:{language:'en'},reason:'rate-limited',retryAt:new Date(Date.now()+60000).toISOString()};
  let html = r.api.render(r.C,r.u);
  assert.match(html,/provider is limiting requests/);
  assert.match(html,/<summary>Incomplete<\/summary>/);
  assert.doesNotMatch(html,/Outdated|Устарело/);
  assert.match(html,/<dt>Observed<\/dt>/);
  assert.match(html,/<dt>Retrieved<\/dt>/);
  assert.match(html,/No usable community goals/);
  assert.doesNotMatch(html,/No active community goals/);
  r.api.init(r.C,r.u);
  await flush();
  assert.deepEqual(requests(r),[]);
  p.status = {kind:'goals',parameters:{language:'ru'},reason:'offline'};
  p.communityGoals.quality = {state:'fresh',coverage:'empty',fetchedAt:age};
  r.C.state.settings.language = 'ru';
  r.window.COPILOT_SMOKE = true;
  html = r.api.render(r.C,r.u);
  assert.match(html,/Нет соединения с источником данных/);
  assert.match(html,/<summary>Пустой результат<\/summary>/);
  assert.match(html,/В последнем обновлении нет активных целей/);
});

test('Politics shows observed and retrieved dates without judging their age', () => {
  const r = runtime(fixture('ru'));
  const data = r.C.state.politics.activeSystem.info;
  data.quality = {state:'stale',coverage:'complete',observedAt:'2025-01-01',fetchedAt:'2025-01-02'};
  const html = r.api.render(r.C,r.u);
  assert.match(html,/<summary>Даты данных<\/summary>/);
  assert.match(html,/<dt>Наблюдение<\/dt>/);
  assert.match(html,/<dt>Получение<\/dt>/);
  assert.doesNotMatch(html,/Устарело|Актуально|is-warning/);
});

test('canonical and legacy government and security labels follow the interface language', () => {
  for (const [government,security] of [['Democracy','Medium'],['$government_Democracy;','$SYSTEM_SECURITY_medium;'],['Демократия','Средняя']]) {
    const C = fixture();
    C.state.politics.activeSystem.info = {known:true,government,security,controllingFaction:'Фракция Демократия'};
    const r = runtime(C);
    let html = r.api.render(C,r.u);
    assert.match(html,/>Democracy<\/dd>/);
    assert.match(html,/>Medium<\/dd>/);
    assert.match(html,/Фракция Демократия/);
    C.state.settings.language = 'ru';
    html = r.api.render(C,r.u);
    assert.match(html,/>Демократия<\/dd>/);
    assert.match(html,/>Средняя<\/dd>/);
  }
});
