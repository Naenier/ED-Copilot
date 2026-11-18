'use strict';
const assert = require('node:assert/strict');
const test = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function fixture() {
  const requests = [], window = {CopilotI18n:{t:text=>text}};
  const document = {
    currentScript: {nonce:'isolated'},
    head: {append:script=>requests.push(script)},
    createElement: () => ({remove(){this.removed=true;}}),
  };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname,'workspace-assets.js'),'utf8'),{window,document});
  return {requests,load:window.WorkspaceAssets.load};
}

test('opening overview only loads the shell; related sections share a single embedded request', async () => {
  const {requests,load}=fixture();
  assert.equal(load('overview','Обзор'),undefined);
  assert.equal(load('statistics','Колонии'),undefined);
  assert.equal(load('engineering','Конструктор кораблей'),undefined);
  assert.equal(requests.length,0);
  const first=load('notifications');
  assert.equal(first,load('second-screen'));
  assert.equal(requests.length,1);
  assert.equal(requests[0].src,'workspace/section-tools.js');
  assert.equal(requests[0].nonce,'isolated');
  requests[0].onload();
  await first;
  assert.equal(requests[0].removed,true);
  assert.equal(first,load('notifications'));
  assert.equal(requests.length,1);
});

test('an unavailable local workspace can be opened again', async () => {
  const {requests,load}=fixture();
  const first=load('settings');
  assert.equal(requests.length,1);
  requests[0].onerror();
  await assert.rejects(first,/Не удалось загрузить раздел/);
  const retry=load('settings');
  assert.equal(requests.length,2);
  requests[1].onload();
  await retry;
  assert.equal(requests[1].removed,true);
});
