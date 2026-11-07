"use strict";

const fs = require("node:fs");
const path = require("node:path");
const watcher = "native/target/release/edexo-fgwatch.exe";

module.exports = {
  appId: "ru.elite.edcopilot",
  productName: "ED Copilot",
  executableName: "ED Copilot",
  electronVersion: "44.9.3",
  asar: true,
  npmRebuild: false,
  directories: { output: "dist" },
  files: [
    "package.json", "electron/**/*", "lib/**/*", "web/**/*",
    "LICENSE", "NOTICE.md", "docs/**/*", "README.md", "README-RU.txt",
    "API.md", "TRADE-API.md",
    "!**/*.log", "!**/*.tmp", "!**/node_modules/**/*"
  ],
  extraResources: [
    { from: "web/copilot.png", to: "native/copilot.png" },
    ...(fs.existsSync(path.join(__dirname, watcher)) ? [{ from: watcher, to: "native/edexo-fgwatch.exe" }] : []),
    { from: "LICENSE", to: "LICENSE-EDEXO.txt" },
    { from: "NOTICE.md", to: "NOTICE.md" }
  ],
  extraFiles: [
    { from: "edmc-plugin", to: "edmc-plugin", filter: ["**/*", "!**/__pycache__/**", "!**/*.pyc"] }
  ],
  win: {
    icon: "web/copilot.ico",
    target: "dir",
    signAndEditExecutable: true
  },
  publish: null
};
