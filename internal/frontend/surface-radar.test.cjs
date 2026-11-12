const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const context = {window: {}};
vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'surface-radar.js'), 'utf8'), context);
const ui = context.window.SurfaceRadarUI;
const translations = require('../localization/ship_route_radar_en.json');
const u = {
  esc: value => String(value).replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;'),
  t: value => translations[value] || {' м': ' m'}[value] || value,
  fmt: value => String(value),
  heading: value => '<h1>' + value + '</h1>',
  renderModes: () => ''
};

test('surface radar identifies estimated contacts and hides directions when the position ages',()=>{
  const target={id:'found',target:true,kind:'codex',label:'Stratum <contact>',uncertaintyM:20,distanceM:300,bearing:75,x:150,y:-260};
  const C={tab:'exobiology',mode:'Радар поверхности',state:{surfaceRadar:{
    body:'Synthetic A 1',available:true,scaleM:500,headingUp:true,heading:80,target,points:[target],trail:[{x:0,y:0},{x:20,y:30}]
  }}};
  assert.equal(ui.active(C),true);
  const html=ui.render(C,u);
  assert.match(html,/search-area/);
  assert.match(html,/Stratum &lt;contact&gt;/);
  assert.match(html,/≈ 300 m · 75°/);
  assert.match(html,/Mark my position/);
  C.state.surfaceRadar.stale=true;
  const stale=ui.render(C,u);
  assert.match(stale,/Position is stale/);
  assert.doesNotMatch(stale,/75°|Mark my position/);
});
