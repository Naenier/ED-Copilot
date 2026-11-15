'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

test('control action labels cover exact XML keys from each game context', () => {
  const { window } = vm.runInNewContext(
    fs.readFileSync(path.join(__dirname, 'controls-action-labels.js'), 'utf8') + '\n({ window })',
    { window: {} }
  );
  const labels = window.ControlActionLabels;
  assert.equal(labels.OrderRequestDock, 'Приказать истребителю вернуться в ангар');
  assert.equal(labels.ToggleButtonUpInput, 'Переключить тихий ход');
  for (const id of [
    'UI_Toggle', 'CamTranslateZHold', 'PitchAxisAlternate', 'YawAxis_Landing',
    'BuggyTurretYawAxisRaw', 'HumanoidClearAuthorityLevel', 'HumanoidPulseScanButton',
    'ExplorationFSSTarget', 'ExplorationSAANextGenus', 'PhotoCameraToggle_Humanoid',
    'PlaceSettlement', 'RotateSettlementRight', 'TriggerColonisationModule'
  ]) {
    assert.match(labels[id] || '', /[а-яё]/i, id);
  }
  for (const [id, label] of Object.entries(labels)) {
    assert.match(label, /[а-яё]/i, id);
  }
});
