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
    return { overlayEnabled: saved.overlayEnabled !== false, overlayHidden: saved.overlayHidden === true };
  } catch {
    return { overlayEnabled: true, overlayHidden: false };
  }
}
const desktop = readDesktopSettings();
function saveDesktopSettings() {
  try {
    writeFileAtomic(desktopFile, JSON.stringify(desktop, null, 2));
  } catch (error) {
    console.warn("Не удалось сохранить настройки окна:", error.message);
  }
}

let backend = null;
let mainWindow = null;
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
const watcherCandidate = app.isPackaged ? path.join(process.resourcesPath, "native", "edexo-fgwatch.exe") : path.join(__dirname, "../native/target/release/edexo-fgwatch.exe");
const watcherPath = fs.existsSync(watcherCandidate) ? watcherCandidate : null;

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
  const windows = mainOnly ? [mainWindow] : [mainWindow, overlayWindow];
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
  const remembered = windowState.readWindowState("app");
  const area = screen.getPrimaryDisplay().workArea;
  const window = new BrowserWindow({
    width: remembered?.width || Math.min(1380, area.width - 40),
    height: remembered?.height || Math.min(920, area.height - 40),
    ...(remembered ? { x: remembered.x, y: remembered.y } : {}),
    minWidth: 780,
    minHeight: 560,
    title: "ED Copilot — помощник пилота",
    backgroundColor: "#06111e",
    autoHideMenuBar: true,
    icon: iconPath,
    show: false,
    webPreferences: {
      preload: preloadPath,
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      spellcheck: false,
      backgroundThrottling: false,
      partition: "persist:ed-mining",
    },
  });
  mainWindow = window;
  guardWindow(window);
  windowState.trackWindowState("app", window);
  enableZoom(window);
  window.on("close", (event) => {
    if (quitRequested) return;
    event.preventDefault();
    window.hide();
  });
  window.on("closed", () => { mainWindow = null; });
  window.once("ready-to-show", () => {
    if (remembered?.maximized) window.maximize();
    window.show();
  });
  const icon = nativeImage.createFromPath(iconPath);
  tray = new Tray(icon.resize({ width: 20, height: 20 }));
  tray.setToolTip("ED Copilot — помощник пилота");
  tray.on("double-click", showMain);
  tray.on("click", showMain);
  shortcutAvailable = globalShortcut.register(SHORTCUT, toggleVisibility);
  registerIpc();
  await window.loadURL(backend.url + "/");
  unsubscribe = backend.engine.subscribe(synchronizeEngineState);
  state = backend.engine.getState();
  if (typeof state?.settings?.overlayEnabled === "boolean") desktop.overlayEnabled = state.settings.overlayEnabled;
  if (desktop.overlayEnabled) await createOverlay();
  synchronizeOverlay();
  foreground = watchForegroundOffMain(utilityProcess, path.join(__dirname, "foregroundHost.cjs"), (name, at) => {
    foregroundName = name;
    if (isGame(name) && at) gamePoint = at;
    synchronizeOverlay();
  }, watcherPath);
  screen.on("display-added", synchronizeOverlay);
  screen.on("display-removed", synchronizeOverlay);
  screen.on("display-metrics-changed", synchronizeOverlay);
}

async function shutdown() {
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
