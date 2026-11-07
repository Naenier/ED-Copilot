'use strict';
const { isGame } = require('./foregroundWatch.cjs');
function overlayVisibility({ enabled, hidden, quitting, gameRunning, foregroundName } = {}) {
  const gameAway = gameRunning === false;
  const focusAway = !isGame(foregroundName);
  return { gameAway, focusAway, visible: enabled === true && hidden !== true && quitting !== true && !gameAway && !focusAway };
}
module.exports = { overlayVisibility };
