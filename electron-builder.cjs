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
    "LICENSE", "NOTICE.md", "docs/**/*", "README.md", "README-RU.txt",
    "API.md", "TRADE-API.md",
    "!**/*.log", "!**/*.tmp", "!**/node_modules/**/*",
  ],
  extraResources: [
    { from: "web/copilot.png", to: "native/copilot.png" },
    { from: "LICENSE", to: "LICENSE-COPILOT.txt" },
    { from: "NOTICE.md", to: "NOTICE-COPILOT.md" },
  ],
  extraFiles: [
    { from: "edmc-plugin", to: "edmc-plugin", filter: ["**/*", "!**/__pycache__/**", "!**/*.pyc"] },
    { from: "README-RU.txt", to: "НАЧАТЬ ЗДЕСЬ.txt" },
    { from: "web/copilot.ico", to: "ED Copilot.ico" },
    { from: "LICENSE", to: "LICENSE-COPILOT.txt" },
    { from: "NOTICE.md", to: "NOTICE-COPILOT.txt" },
    { from: "lib/COMMODITY-DATA-NOTICE.txt", to: "NOTICE-COMMODITY-DATA.txt" },
    { from: "lib/ENGINEERING-DATA-NOTICE.txt", to: "NOTICE-ENGINEERING-DATA.txt" },
    { from: "lib/ENCYCLOPEDIA-DATA-NOTICE.txt", to: "NOTICE-ENCYCLOPEDIA-DATA.txt" },
  ],
  win: { icon: "web/copilot.ico", target: "dir", signAndEditExecutable: true },
  publish: null,
};

