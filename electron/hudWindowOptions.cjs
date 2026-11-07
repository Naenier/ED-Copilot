"use strict";
// A passive transparent window: the main process supplies screen position.
function hudWindowOptions(width, height, icon, preloadPath) {
  return {
    width: Math.max(1, Math.round(width)), height: Math.max(1, Math.round(height)),
    icon, show: false, transparent: true, backgroundColor: "#00000000",
    frame: false, hasShadow: false, roundedCorners: false, thickFrame: false,
    focusable: false, resizable: false, movable: false, minimizable: false,
    maximizable: false, fullscreenable: false, skipTaskbar: true, alwaysOnTop: true,
    webPreferences: {
      preload: preloadPath, sandbox: true, contextIsolation: true,
      nodeIntegration: false, backgroundThrottling: false, spellcheck: false,
    },
  };
}
module.exports = { hudWindowOptions };

