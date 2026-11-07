"use strict";

const { contextBridge, ipcRenderer } = require("electron");

const invoke = (method, value) => ipcRenderer.invoke("ed-mining:" + method, value);
contextBridge.exposeInMainWorld("edMining", {
  openOverlay: () => invoke("overlay-open"),
  toggleOverlay: () => invoke("overlay-toggle"),
  setOverlay: (on) => invoke("overlay-set", on === true),
  getOverlayState: () => invoke("overlay-state"),
  onOverlayState: (callback) => {
    const listener = (_event, state) => callback(state);
    ipcRenderer.on("ed-mining:overlay-state", listener);
    return () => ipcRenderer.removeListener("ed-mining:overlay-state", listener);
  },
  resizeOverlay: (options) => invoke("overlay-resize", { height: options?.height }),
  showMain: () => invoke("show-main"),
  minimize: () => invoke("minimize"),
  close: () => invoke("close-main"),
  getDisplayState: () => invoke("display-state"),
  setDisplayMode: (mode) => invoke("display-mode", mode),
  onDisplayState: (callback) => {
    const listener = (_event, state) => callback(state);
    ipcRenderer.on("ed-mining:display-state", listener);
    return () => ipcRenderer.removeListener("ed-mining:display-state", listener);
  },
  chooseJournalDir: () => invoke("choose-journal-directory"),
  copyText: (text) => invoke("copy-text", String(text)),
  openExternal: (url) => invoke("open-external", String(url)),
  quit: () => invoke("quit"),
});
