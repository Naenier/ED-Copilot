"use strict";

const { app, BrowserWindow, Menu, Tray, nativeImage, screen, ipcMain, dialog, shell, clipboard, globalShortcut, utilityProcess, safeStorage } = require("electron");
const fs = require("node:fs");
const path = require("node:path");
const { createWindowState, enableZoom } = require("./windowState.cjs");
const { hudWindowOptions } = require("./hudWindowOptions.cjs");
const { writeFileAtomic } = require("./atomicWrite.cjs");
const { watchForegroundOffMain, isGame, isGameOrOwn } = require("./foregroundWatch.cjs");
const { startServer } = require("../lib/server.cjs");
const { resolveProfileDir } = require("./profile.cjs");
const { isWindowMode, normalizeWindowMode, windowModePlan } = require("./windowModes.cjs");

app.setName("ED Copilot");
app.setAppUserModelId("ru.elite.edcopilot");
const dataDir = resolveProfileDir(process.env.LOCALAPPDATA || app.getPath("appData"));
fs.mkdirSync(path.join(dataDir, "electron"), { recursive: true });
app.setPath("userData", path.join(dataDir, "electron"));
app.setPath("sessionData", path.join(dataDir, "electron"));
if (process.env.ED_COPILOT_SAFE_GRAPHICS === "1" || process.env.ED_MINING_SAFE_GRAPHICS === "1" || process.argv.includes("--disable-gpu")) app.disableHardwareAcceleration();

const SHORTCUT = "Control+Alt+H";
const desktopFile = path.join(dataDir, "desktop-settings.json");
function readDesktopSettings() {
  try {
    const saved = JSON.parse(fs.readFileSync(desktopFile, "utf8"));
    const bounds = saved.windowedBounds;
    const windowedBounds = bounds && [bounds.x, bounds.y, bounds.width, bounds.height].every(Number.isFinite) && bounds.width > 0 && bounds.height > 0
      ? { x: bounds.x, y: bounds.y, width: bounds.width, height: bounds.height, maximized: bounds.maximized === true } : null;
    return { overlayEnabled: saved.overlayEnabled !== false, overlayHidden: saved.overlayHidden === true,
      displayMode: normalizeWindowMode(saved.displayMode), windowedBounds };
  } catch {
    return { overlayEnabled: true, overlayHidden: false, displayMode: "windowed", windowedBounds: null };
  }
}
const desktop = readDesktopSettings();
function saveDesktopSettings() {
  try {
    writeFileAtomic(desktopFile, JSON.stringify(desktop, null, 2));
    return true;
  } catch (error) {
    console.warn("Не удалось сохранить настройки окна:", error.message);
    return false;
  }
}

let backend = null;
let mainWindow = null;
let pendingMainWindow = null;
let displayChanging = false;
let pendingDisplayMode = null;
let displayError = "";
let displayTaskTimer = null;
const geometryTrackers = new WeakMap();
let overlayWindow = null;
let tray = null;
let foreground = null;
let unsubscribe = null;
let quitRequested = false;
let gamePoint = null;
let foregroundName = null;
let state = null;
let overlayHeight = 560;
let shortcutAvailable = false;
const ownProcessNames = [path.basename(app.getPath("exe"), ".exe").toLowerCase()];
const windowState = createWindowState(() => dataDir);
const iconPath = app.isPackaged ? path.join(process.resourcesPath, "native", "copilot.png") : path.join(__dirname, "../web/copilot.png");
const preloadPath = path.join(__dirname, "preload.cjs");

function gameDisplay() {
  if (!gamePoint) return screen.getPrimaryDisplay();
  let point = gamePoint;
  try {
    point = screen.screenToDipPoint(point);
  } catch {
    // Coordinates are already device-independent on platforms without this conversion.
  }
  return screen.getDisplayNearestPoint(point);
}

function overlayBounds() {
  const bounds = gameDisplay().bounds;
  const onFoot = state?.commander?.vehicle === "foot";
  const top = Math.round((onFoot ? 14 : 270) * bounds.height / 1080);
  const width = Math.min(410, bounds.width - 16);
  const height = Math.min(overlayHeight, Math.max(90, bounds.height - top - 14));
  return {
    x: bounds.x + Math.max(8, bounds.width - width - 14),
    y: bounds.y + Math.min(top, Math.max(0, bounds.height - height)),
    width,
    height,
  };
}

function overlayState() {
  const gameAway = state?.connection?.gameRunning === false;
  const focusAway = state?.settings?.hideUnfocused !== false && foregroundName !== null && !isGameOrOwn(foregroundName, ownProcessNames);
  return {
    open: desktop.overlayEnabled,
    hidden: desktop.overlayHidden,
    gameAway,
    focusAway,
    visible: !quitRequested && desktop.overlayEnabled && !desktop.overlayHidden && !gameAway && !focusAway,
    shortcut: "Ctrl+Alt+H",
    shortcutAvailable,
  };
}

function sendOverlayState() {
  const value = overlayState();
  for (const window of [mainWindow, overlayWindow]) {
    if (window && !window.isDestroyed()) window.webContents.send("ed-mining:overlay-state", value);
  }
  updateTray();
  return value;
}

function synchronizeOverlay() {
  if (overlayWindow && !overlayWindow.isDestroyed()) {
    const next = overlayBounds();
    const current = overlayWindow.getBounds();
    if (Object.keys(next).some((key) => next[key] !== current[key])) overlayWindow.setBounds(next);
    if (overlayState().visible) {
      if (!overlayWindow.isVisible()) overlayWindow.showInactive();
      overlayWindow.setAlwaysOnTop(true, "screen-saver");
    } else {
      overlayWindow.hide();
    }
  }
  return sendOverlayState();
}

function openExternal(url) {
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== "https:" && parsed.protocol !== "http:") return false;
    void shell.openExternal(parsed.toString());
    return true;
  } catch {
    return false;
  }
}

function guardWindow(window) {
  const ownOrigin = new URL(backend.url).origin;
  window.webContents.setWindowOpenHandler(({ url }) => {
    openExternal(url);
    return { action: "deny" };
  });
  window.webContents.on("will-navigate", (event, url) => {
    try {
      if (new URL(url).origin === ownOrigin) return;
    } catch {
      // Invalid navigation is handled in the same way as a foreign origin.
    }
    event.preventDefault();
    openExternal(url);
  });
  window.webContents.session.setPermissionRequestHandler((_webContents, _permission, callback) => callback(false));
}

function showMain() {
  if (!mainWindow || mainWindow.isDestroyed()) return;
  if (mainWindow.isMinimized()) mainWindow.restore();
  mainWindow.show();
  mainWindow.focus();
}

function mainDisplay(bounds) {
  try {
    const candidate = bounds || (mainWindow && !mainWindow.isDestroyed() ? mainWindow.getBounds() : desktop.windowedBounds);
    if (candidate && [candidate.x, candidate.y, candidate.width, candidate.height].every(Number.isFinite)) return screen.getDisplayMatching(candidate);
  } catch {}
  return screen.getPrimaryDisplay();
}

function displayState() {
  const window = mainWindow && !mainWindow.isDestroyed() ? mainWindow : null;
  return { mode: desktop.displayMode, changing: displayChanging, pendingMode: pendingDisplayMode,
    fullscreen: window?.isFullScreen() === true, borderless: desktop.displayMode === "borderless",
    bounds: window ? window.getBounds() : null, windowedBounds: desktop.windowedBounds ? { ...desktop.windowedBounds } : null,
    error: displayError || null };
}

function sendDisplayState() {
  const value = displayState();
  for (const window of [mainWindow, pendingMainWindow]) if (window && !window.isDestroyed()) window.webContents.send("ed-mining:display-state", value);
  return value;
}

function rememberWindowedBounds(window, force = false) {
  if (!window || window !== mainWindow || window.isDestroyed() || window.isMinimized() || desktop.displayMode !== "windowed" || displayChanging && !force) return;
  try {
    const normalBounds = { ...window.getNormalBounds(), maximized: window.isMaximized() };
    const plan = windowModePlan("windowed", { normalBounds, display: mainDisplay(window.getBounds()) });
    desktop.windowedBounds = plan.normalBounds;
    windowState.saveWindowState("app", desktop.windowedBounds);
    saveDesktopSettings();
  } catch {}
}

function trackMainGeometry(window) {
  let timer;
  const schedule = () => {
    clearTimeout(timer);
    timer = setTimeout(() => rememberWindowedBounds(window), 180); timer.unref?.();
  };
  for (const event of ["move", "resize", "maximize", "unmaximize"]) window.on(event, schedule);
  const stop = () => { clearTimeout(timer); for (const event of ["move", "resize", "maximize", "unmaximize"]) window.removeListener(event, schedule); };
  geometryTrackers.set(window, stop);
}

function makeMainWindow(mode, display = mainDisplay(), normalBounds = desktop.windowedBounds || windowState.readWindowState("app")) {
  const plan = windowModePlan(mode, { previousMode: desktop.displayMode, display, normalBounds });
  const window = new BrowserWindow({
    ...plan.options,
    title: "ED Copilot — помощник пилота", backgroundColor: "#06111e", autoHideMenuBar: true,
    icon: iconPath, show: false,
    webPreferences: { preload: preloadPath, contextIsolation: true, nodeIntegration: false,
      sandbox: true, spellcheck: false, backgroundThrottling: false, partition: "persist:ed-mining" },
  });
  guardWindow(window);
  enableZoom(window);
  trackMainGeometry(window);
  window.on("close", event => {
    rememberWindowedBounds(window);
    if (quitRequested) return;
    event.preventDefault(); window.hide();
  });
  window.on("closed", () => {
    geometryTrackers.get(window)?.();
    if (mainWindow === window) mainWindow = null;
    if (pendingMainWindow === window) pendingMainWindow = null;
  });
  window.webContents.on("before-input-event", (event, input) => {
    if (input.type !== "keyDown" || input.control || input.meta || input.alt) return;
    const key = String(input.key || "").toLowerCase();
    if (key !== "f11" && !(key === "escape" && desktop.displayMode !== "windowed")) return;
    event.preventDefault();
    if (displayChanging || window !== mainWindow) return;
    requestDisplayMode(key === "f11" && desktop.displayMode === "windowed" ? "fullscreen" : "windowed");
  });
  return { window, plan };
}

async function setNativeFullscreen(window, enabled) {
  if (window.isFullScreen() === enabled) return;
  await new Promise((resolve, reject) => {
    const event = enabled ? "enter-full-screen" : "leave-full-screen";
    let timer;
    const finish = () => { clearTimeout(timer); window.removeListener(event, finish); resolve(); };
    window.once(event, finish);
    timer = setTimeout(() => {
      window.removeListener(event, finish);
      if (!window.isDestroyed() && window.isFullScreen() === enabled) resolve();
      else reject(new Error("Оконная система не подтвердила полноэкранный режим."));
    }, 1500); timer.unref?.();
    try { window.setFullScreen(enabled); } catch (error) { clearTimeout(timer); window.removeListener(event, finish); reject(error); }
  });
}

async function captureMainView(window) {
  let hash = "", view = null;
  try {
    const url = new URL(window.webContents.getURL());
    if (url.origin === new URL(backend.url).origin && /^#[a-z0-9-]{1,80}$/i.test(url.hash)) hash = url.hash;
  } catch {}
  try {
    view = await window.webContents.executeJavaScript("window.CopilotViewState?.capture?.() ?? null");
    const encoded = JSON.stringify(view);
    if (!encoded || encoded.length > 524288) view = null;
  } catch {}
  return { hash, view, zoom: Math.max(0.5, Math.min(3, window.webContents.getZoomFactor() || 1)) };
}

async function applyDisplayMode(mode) {
  const old = mainWindow, previousMode = desktop.displayMode;
  let replacement = null;
  try {
    if (quitRequested || !old || old.isDestroyed()) return;
    rememberWindowedBounds(old, true);
    const display = mainDisplay(old.getBounds());
    const plan = windowModePlan(mode, { previousMode, normalBounds: desktop.windowedBounds, display });
    if (plan.requiresRecreation) {
      const snapshot = await captureMainView(old);
      if (quitRequested || old.isDestroyed()) return;
      const created = makeMainWindow(mode, display, plan.normalBounds);
      replacement = created.window; pendingMainWindow = replacement;
      replacement.webContents.setZoomFactor(snapshot.zoom);
      await replacement.loadURL(backend.url + "/" + snapshot.hash);
      if (quitRequested) { replacement.destroy(); pendingMainWindow = null; replacement = null; return; }
      if (snapshot.view) {
        const restore = "Promise.resolve(window.CopilotViewState?.restore?.(" + JSON.stringify(snapshot.view) + "))";
        await Promise.race([replacement.webContents.executeJavaScript(restore), new Promise(resolve => setTimeout(resolve, 3000))]);
      }
      if (plan.maximize) replacement.maximize();
      const visible = old.isVisible();
      geometryTrackers.get(old)?.();
      mainWindow = replacement; pendingMainWindow = null;
      desktop.displayMode = mode; desktop.windowedBounds = plan.normalBounds;
      if (visible) { replacement.show(); replacement.focus(); }
      old.destroy(); replacement = null;
    } else {
      await setNativeFullscreen(old, mode === "fullscreen");
      if (mode === "windowed") {
        if (old.isMaximized()) old.unmaximize();
        old.setBounds(plan.bounds);
        if (plan.maximize) old.maximize();
      }
      desktop.displayMode = mode; desktop.windowedBounds = plan.normalBounds;
    }
    displayError = saveDesktopSettings() ? "" : "Режим применён, но настройку не удалось сохранить на диск.";
  } catch (error) {
    if (replacement && !replacement.isDestroyed()) replacement.destroy();
    pendingMainWindow = null;
    if (old && !old.isDestroyed()) { mainWindow = old; old.show(); old.focus(); }
    desktop.displayMode = previousMode;
    displayError = "Не удалось изменить режим окна. Предыдущее окно сохранено.";
    console.warn("Не удалось изменить режим окна:", error.message);
  } finally {
    displayChanging = false; pendingDisplayMode = null;
    if (!quitRequested) { rememberWindowedBounds(mainWindow); sendDisplayState(); }
  }
}

function requestDisplayMode(value) {
  if (!isWindowMode(value)) throw new Error("Выберите режим окна из списка.");
  if (displayChanging) {
    if (pendingDisplayMode === value) return displayState();
    throw new Error("Дождитесь завершения изменения режима окна.");
  }
  if (desktop.displayMode === value) return displayState();
  rememberWindowedBounds(mainWindow);
  displayChanging = true; pendingDisplayMode = value; displayError = "";
  // Return the IPC response before a frame change can replace its renderer.
  displayTaskTimer = setTimeout(() => { displayTaskTimer = null; void applyDisplayMode(value); }, 100);
  displayTaskTimer.unref?.();
  return sendDisplayState();
}

function synchronizeMainDisplay() {
  if (displayChanging || !mainWindow || mainWindow.isDestroyed()) return;
  if (desktop.displayMode === "borderless") {
    const plan = windowModePlan("borderless", { previousMode: "borderless", normalBounds: desktop.windowedBounds, display: mainDisplay() });
    mainWindow.setBounds(plan.bounds);
  } else if (desktop.displayMode === "windowed") rememberWindowedBounds(mainWindow);
  sendDisplayState();
}

async function createOverlay() {
  if (quitRequested) return null;
  if (overlayWindow && !overlayWindow.isDestroyed()) return overlayWindow;
  const bounds = overlayBounds();
  const options = hudWindowOptions(bounds.width, bounds.height, iconPath, preloadPath);
  // One mining panel needs its own isolated, sandboxed renderer.
  options.webPreferences = { ...options.webPreferences, sandbox: true, partition: "persist:ed-mining-hud" };
  const window = new BrowserWindow({ ...options, ...bounds, title: "ED Copilot — оверлей" });
  overlayWindow = window;
  guardWindow(window);
  window.setIgnoreMouseEvents(true, { forward: true });
  window.setAlwaysOnTop(true, "screen-saver");
  window.on("closed", () => {
    if (overlayWindow === window) overlayWindow = null;
  });
  window.webContents.on("did-finish-load", () => synchronizeOverlay());
  await window.loadURL(backend.url + "/overlay.html");
  return window;
}

async function setOverlay(on) {
  const response = await fetch(backend.url + "/api/settings", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ overlayEnabled: on }),
  });
  const result = await response.json();
  if (!response.ok || result.ok === false) throw new Error(result.error || "Не удалось сохранить настройку оверлея.");
  desktop.overlayEnabled = on;
  desktop.overlayHidden = false;
  saveDesktopSettings();
  if (on) await createOverlay();
  return synchronizeOverlay();
}

function synchronizeEngineState() {
  state = backend.engine.getState();
  if (typeof state?.settings?.overlayEnabled === "boolean") desktop.overlayEnabled = state.settings.overlayEnabled;
  if (desktop.overlayEnabled && (!overlayWindow || overlayWindow.isDestroyed())) {
    void createOverlay().then(synchronizeOverlay).catch((error) => console.warn("Не удалось открыть оверлей:", error.message));
  }
  synchronizeOverlay();
}

function toggleVisibility() {
  if (!desktop.overlayEnabled) {
    void setOverlay(true).catch((error) => console.warn("Не удалось показать оверлей:", error.message));
    return;
  }
  desktop.overlayHidden = !desktop.overlayHidden;
  saveDesktopSettings();
  synchronizeOverlay();
}

function updateTray() {
  if (!tray) return;
  const visible = overlayState();
  tray.setContextMenu(Menu.buildFromTemplate([
    { label: "Открыть ED Copilot", click: showMain },
    { label: desktop.overlayEnabled && !desktop.overlayHidden ? "Скрыть оверлей" : "Показать оверлей", click: toggleVisibility },
    { label: visible.gameAway ? "Elite Dangerous не запущена" : visible.focusAway ? "Оверлей ожидает возврата в игру" : "Помощник пилота", enabled: false },
    { type: "separator" },
    { label: "Выход", click: () => app.quit() },
  ]));
}

function senderAllowed(event, mainOnly = false) {
  const windows = mainOnly ? [mainWindow, pendingMainWindow] : [mainWindow, pendingMainWindow, overlayWindow];
  return windows.some((window) => window && !window.isDestroyed() && window.webContents === event.sender);
}

function registerIpc() {
  const handle = (name, handler, mainOnly = false) => ipcMain.handle("ed-mining:" + name, (event, value) => {
    if (!senderAllowed(event, mainOnly)) throw new Error("Окно приложения недоступно.");
    return handler(value);
  });
  handle("overlay-open", () => setOverlay(true));
  handle("overlay-toggle", () => setOverlay(!desktop.overlayEnabled || desktop.overlayHidden));
  handle("overlay-set", (on) => setOverlay(on === true));
  handle("overlay-state", overlayState);
  handle("overlay-resize", (options) => {
    if (Number.isFinite(options?.height)) overlayHeight = Math.max(90, Math.min(2400, Math.ceil(options.height)));
    synchronizeOverlay();
    return overlayState();
  });
  handle("show-main", showMain);
  handle("minimize", () => mainWindow?.minimize(), true);
  handle("close-main", () => mainWindow?.hide(), true);
  handle("display-state", displayState, true);
  handle("display-mode", requestDisplayMode, true);
  handle("choose-journal-directory", async () => {
    const result = await dialog.showOpenDialog(mainWindow, {
      title: "Папка журналов Elite Dangerous",
      buttonLabel: "Выбрать папку",
      defaultPath: state?.connection?.journalDir || process.env.ED_JOURNAL_DIR || app.getPath("documents"),
      properties: ["openDirectory"],
    });
    return result.canceled ? null : result.filePaths[0] || null;
  }, true);
  handle("copy-text", (text) => {
    clipboard.writeText(String(text));
    return true;
  });
  handle("open-external", openExternal);
  handle("quit", () => app.quit(), true);
}

function parsePort() {
  const index = process.argv.indexOf("--port");
  const value = index < 0 ? 7122 : Number(process.argv[index + 1]);
  if (!Number.isInteger(value) || value < 1 || value > 65535) throw new Error("Параметр --port должен быть целым числом от 1 до 65535.");
  return value;
}

async function start() {
  backend = await startServer({
    dataDir,
    journalDir: process.env.ED_JOURNAL_DIR,
    port: parsePort(),
    host: "127.0.0.1",
    secureStorage: safeStorage.isEncryptionAvailable() ? {
      encrypt: (value) => safeStorage.encryptString(value), decrypt: (value) => safeStorage.decryptString(value),
    } : undefined,
    openExternal: (url) => shell.openExternal(url),
  });
  state = backend.engine.getState();
  const created = makeMainWindow(desktop.displayMode);
  const window = created.window;
  mainWindow = window;
  const icon = nativeImage.createFromPath(iconPath);
  tray = new Tray(icon.resize({ width: 20, height: 20 }));
  tray.setToolTip("ED Copilot — помощник пилота");
  tray.on("double-click", showMain);
  tray.on("click", showMain);
  shortcutAvailable = globalShortcut.register(SHORTCUT, toggleVisibility);
  registerIpc();
  await window.loadURL(backend.url + "/");
  if (created.plan.maximize) window.maximize();
  window.show();
  desktop.windowedBounds = created.plan.normalBounds;
  rememberWindowedBounds(window);
  sendDisplayState();
  unsubscribe = backend.engine.subscribe(synchronizeEngineState);
  state = backend.engine.getState();
  if (typeof state?.settings?.overlayEnabled === "boolean") desktop.overlayEnabled = state.settings.overlayEnabled;
  if (desktop.overlayEnabled) await createOverlay();
  synchronizeOverlay();
  foreground = watchForegroundOffMain(utilityProcess, path.join(__dirname, "foregroundHost.cjs"), (name, at) => {
    foregroundName = name;
    if (isGame(name) && at) gamePoint = at;
    synchronizeOverlay();
  });
  screen.on("display-added", synchronizeOverlay);
  screen.on("display-removed", synchronizeOverlay);
  screen.on("display-metrics-changed", synchronizeOverlay);
  screen.on("display-added", synchronizeMainDisplay);
  screen.on("display-removed", synchronizeMainDisplay);
  screen.on("display-metrics-changed", synchronizeMainDisplay);
}

async function shutdown() {
  clearTimeout(displayTaskTimer);
  geometryTrackers.get(mainWindow)?.();
  if (pendingMainWindow && !pendingMainWindow.isDestroyed()) pendingMainWindow.destroy();
  foreground?.stop();
  globalShortcut.unregisterAll();
  if (typeof unsubscribe === "function") unsubscribe();
  if (overlayWindow && !overlayWindow.isDestroyed()) overlayWindow.destroy();
  tray?.destroy();
  tray = null;
  try {
    await backend?.close();
  } catch (error) {
    console.warn("Ошибка завершения работы:", error.message);
  }
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on("second-instance", showMain);
  app.on("activate", showMain);
  app.on("before-quit", (event) => {
    if (quitRequested) return;
    event.preventDefault();
    quitRequested = true;
    void shutdown().finally(() => app.quit());
  });
  app.whenReady().then(start).catch(async (error) => {
    dialog.showErrorBox("ED Copilot — ошибка запуска", error.message || String(error));
    quitRequested = true;
    await shutdown();
    app.quit();
  });
}
