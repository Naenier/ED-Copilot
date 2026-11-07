"use strict";

// Native frame style is a BrowserWindow constructor option on Windows.
// Crossing the framed/frameless boundary therefore recreates the main window;
// backend, tray and overlay lifetimes belong to the caller and stay separate.
// https://www.electronjs.org/docs/latest/tutorial/custom-window-styles
// https://www.electronjs.org/docs/latest/api/browser-window#winsetfullscreenflag

const WINDOW_MODES = Object.freeze(["windowed", "fullscreen", "borderless"]);
const MINIMUM = Object.freeze({ width: 780, height: 560 });
const DEFAULT_AREA = Object.freeze({ x: 0, y: 0, width: 1280, height: 800 });
const finite = value => typeof value === "number" && Number.isFinite(value);

class WindowModeError extends Error {
  constructor() { super("Выберите режим: в окне, полноэкранный или безрамочное окно."); this.name = "WindowModeError"; this.code = "WINDOW_MODE_INVALID"; }
}

function isWindowMode(value) { return typeof value === "string" && WINDOW_MODES.includes(value); }
function normalizeWindowMode(value) { return isWindowMode(value) ? value : "windowed"; }
function requireMode(value) { if (!isWindowMode(value)) throw new WindowModeError(); return value; }
function requiresFrameChange(previous, next) {
  return (normalizeWindowMode(previous) === "borderless") !== (requireMode(next) === "borderless");
}
function rectangle(value) {
  if (!value || ![value.x, value.y, value.width, value.height].every(finite) || value.width <= 0 || value.height <= 0) return null;
  return { x: Math.round(value.x), y: Math.round(value.y), width: Math.max(1, Math.round(value.width)), height: Math.max(1, Math.round(value.height)) };
}
function displayAreas(display = {}) {
  const workArea = rectangle(display.workArea) || rectangle(display.bounds) || { ...DEFAULT_AREA };
  const bounds = rectangle(display.bounds) || { ...workArea };
  return { workArea, bounds };
}
function normalRectangle(saved, workArea) {
  const value = saved && typeof saved === "object" ? saved : {};
  const minWidth = Math.min(MINIMUM.width, workArea.width), minHeight = Math.min(MINIMUM.height, workArea.height);
  const fallbackWidth = Math.min(1380, Math.max(minWidth, workArea.width - 40));
  const fallbackHeight = Math.min(920, Math.max(minHeight, workArea.height - 40));
  const width = Math.round(Math.max(minWidth, Math.min(workArea.width, finite(value.width) && value.width > 0 ? value.width : fallbackWidth)));
  const height = Math.round(Math.max(minHeight, Math.min(workArea.height, finite(value.height) && value.height > 0 ? value.height : fallbackHeight)));
  const x = finite(value.x) ? value.x : workArea.x + (workArea.width - width) / 2;
  const y = finite(value.y) ? value.y : workArea.y + (workArea.height - height) / 2;
  return {
    x: Math.round(Math.max(workArea.x, Math.min(workArea.x + workArea.width - width, x))),
    y: Math.round(Math.max(workArea.y, Math.min(workArea.y + workArea.height - height, y))),
    width, height, maximized: value.maximized === true,
  };
}

/**
 * Pure layout plan, in Electron device-independent screen coordinates.
 * The caller records getNormalBounds()/isMaximized() only while windowed.
 * Never pass a fullscreen or borderless rectangle as normalBounds.
 * `maximize` is applied after ready-to-show only for restored windowed mode.
 * Borderless fills workArea exactly rather than invoking native maximize;
 * fullscreen uses Electron's native fullscreen and the display's full bounds.
 */
function windowModePlan(value = "windowed", { previousMode = "windowed", normalBounds, display } = {}) {
  const mode = requireMode(value), areas = displayAreas(display);
  const normal = normalRectangle(normalBounds, areas.workArea);
  const borderless = mode === "borderless", fullscreen = mode === "fullscreen";
  const target = borderless ? areas.workArea : fullscreen ? areas.bounds : normal;
  const bounds = { x: target.x, y: target.y, width: target.width, height: target.height };
  return {
    mode,
    requiresRecreation: requiresFrameChange(previousMode, mode),
    bounds,
    normalBounds: normal,
    maximize: mode === "windowed" && normal.maximized,
    options: {
      ...bounds,
      frame: !borderless,
      fullscreen,
      fullscreenable: true,
      resizable: !borderless,
      movable: !borderless,
      maximizable: !borderless,
      minimizable: true,
      closable: true,
      thickFrame: !borderless,
      hasShadow: !borderless,
      roundedCorners: !borderless,
      minWidth: Math.min(MINIMUM.width, areas.workArea.width),
      minHeight: Math.min(MINIMUM.height, areas.workArea.height),
      skipTaskbar: false,
    },
  };
}

module.exports = { WINDOW_MODES, WindowModeError, isWindowMode, normalizeWindowMode, requiresFrameChange, windowModePlan };
