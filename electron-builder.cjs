"use strict";

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
    "docs/**/*", "README.md", "README-RU.txt",
    "API.md", "TRADE-API.md",
    "!**/*.log", "!**/*.tmp", "!**/node_modules/**/*",
  ],
  extraResources: [
    { from: "web/copilot.png", to: "native/copilot.png" },
  ],
  extraFiles: [
    { from: "README-RU.txt", to: "НАЧАТЬ ЗДЕСЬ.txt" },
    { from: "web/copilot.ico", to: "ED Copilot.ico" },
  ],
  win: { icon: "web/copilot.ico", target: "dir", signAndEditExecutable: true },
  publish: null,
};

