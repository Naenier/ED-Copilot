'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
test('documentation capture cannot start on a real or ordinary runtime',async()=>{
 for(const flags of [{},{COPILOT_SMOKE:true},{COPILOT_README:true},{COPILOT_SMOKE:'true',COPILOT_README:true}]){
  const window={...flags};vm.runInNewContext(fs.readFileSync(path.join(__dirname,'readme-capture.js'),'utf8'),{window});
  await assert.rejects(window.ReadmeCapture.run({},{}),/isolated fixture/);
 }
});
