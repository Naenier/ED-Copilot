"use strict";
// Window geometry belongs to the local profile and is safe to discard.
const fs = require("node:fs");
const path = require("node:path");
const { writeFileAtomic } = require("./atomicWrite.cjs");
const WINDOW_MIN = Object.freeze({ app: { w: 780, h: 560 } });
const validNumber = value => typeof value === "number" && Number.isFinite(value);
const safeName = name => typeof name === "string" && /^[a-z][a-z0-9_-]{0,59}$/i.test(name) && !["constructor", "prototype", "__proto__"].includes(name);
function plain(value) { return value && typeof value === "object" && !Array.isArray(value) ? value : {}; }
function areaOf(display) {
  const area = display?.workArea || display?.bounds;
  return area && [area.x, area.y, area.width, area.height].every(validNumber) && area.width > 0 && area.height > 0 ? area : null;
}
function clampBounds(saved, displays, minimum = WINDOW_MIN.app) {
  if (!saved || ![saved.x, saved.y, saved.width, saved.height].every(validNumber) || saved.width <= 0 || saved.height <= 0) return null;
  const areas = displays.map(areaOf).filter(Boolean);
  if (!areas.length) return null;
  const overlap = area => Math.max(0, Math.min(saved.x + saved.width, area.x + area.width) - Math.max(saved.x, area.x)) *
    Math.max(0, Math.min(saved.y + saved.height, area.y + area.height) - Math.max(saved.y, area.y));
  const ranked = areas.map(area => ({ area, overlap: overlap(area) })).sort((a, b) => b.overlap - a.overlap);
  const match = ranked[0];
  const area = match.area;
  const width = Math.round(Math.min(area.width, Math.max(Math.min(minimum.w, area.width), saved.width)));
  const height = Math.round(Math.min(area.height, Math.max(Math.min(minimum.h, area.height), saved.height)));
  const x = match.overlap > 0 ? saved.x : area.x + (area.width - width) / 2;
  const y = match.overlap > 0 ? saved.y : area.y + (area.height - height) / 2;
  return {
    x: Math.round(Math.max(area.x, Math.min(area.x + area.width - width, x))),
    y: Math.round(Math.max(area.y, Math.min(area.y + area.height - height, y))),
    width, height, maximized: saved.maximized === true,
  };
}
function createWindowState(stateDir, dependencies = {}) {
  const screen = dependencies.screen || require("electron").screen;
  const folder = () => typeof stateDir === "function" ? stateDir() : stateDir;
  const windowStatePath = () => path.join(folder(), "window-state.json");
  function readWindowStates() {
    try {
      if (fs.statSync(windowStatePath()).size > 1024 * 1024) return {};
      return plain(JSON.parse(fs.readFileSync(windowStatePath(), "utf8")));
    } catch { return {}; }
  }
  function readWindowState(name) {
    if (!safeName(name)) return null;
    let saved = readWindowStates()[name];
    if (!saved && name === "app") {
      try { saved = JSON.parse(fs.readFileSync(path.join(folder(), "app-window.json"), "utf8")); } catch {}
    }
    const displays = screen.getAllDisplays?.() || [screen.getPrimaryDisplay()];
    return clampBounds(saved, displays, WINDOW_MIN[name] || WINDOW_MIN.app);
  }
  function saveWindowState(name, value) {
    if (!safeName(name)) return false;
    let bounds;
    try {
      if (typeof value?.getBounds === "function") {
        if (value.isDestroyed?.() || value.isMinimized?.()) return false;
        bounds = { ...(value.getNormalBounds?.() || value.getBounds()), maximized: value.isMaximized?.() === true };
      } else bounds = value;
      if (!bounds || ![bounds.x, bounds.y, bounds.width, bounds.height].every(validNumber) || bounds.width <= 0 || bounds.height <= 0) return false;
      const states = readWindowStates();
      states[name] = { x: Math.round(bounds.x), y: Math.round(bounds.y), width: Math.round(bounds.width), height: Math.round(bounds.height), maximized: bounds.maximized === true };
      writeFileAtomic(windowStatePath(), JSON.stringify(states, null, 2));
      return true;
    } catch { return false; }
  }
  function trackWindowState(name, win) {
    let timer = null;
    const schedule = () => { clearTimeout(timer); timer = setTimeout(() => { timer = null; saveWindowState(name, win); }, 180); timer.unref?.(); };
    const flush = () => { clearTimeout(timer); timer = null; saveWindowState(name, win); };
    const events = ["move", "resize", "maximize", "unmaximize"];
    for (const event of events) win.on(event, schedule);
    win.on("close", flush);
    return () => { clearTimeout(timer); for (const event of events) win.removeListener(event, schedule); win.removeListener("close", flush); };
  }
  return { readWindowState, readWindowStates, saveWindowState, trackWindowState, windowStatePath };
}
function enableZoom(win) {
  const contents = win.webContents;
  contents.on("before-input-event", (event, input) => {
    if (input.type && input.type !== "keyDown" || !(input.control || input.meta) || input.alt) return;
    const key = String(input.key || "").toLowerCase();
    let value;
    if (["0", "num0"].includes(key) || input.code === "Digit0" || input.code === "Numpad0") value = 1;
    else if (["+", "=", "add"].includes(key) || input.code === "NumpadAdd") value = Math.min(2.5, (contents.getZoomFactor?.() || 1) + 0.1);
    else if (["-", "_", "subtract"].includes(key) || input.code === "NumpadSubtract") value = Math.max(0.7, (contents.getZoomFactor?.() || 1) - 0.1);
    else return;
    event.preventDefault();
    contents.setZoomFactor(Math.round(value * 100) / 100);
  });
}
module.exports = { WINDOW_MIN, createWindowState, enableZoom, clampBounds };

