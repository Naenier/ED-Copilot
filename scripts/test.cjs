"use strict";

// Every file in test/ uses synthetic fixtures and injected/mock transports.
// This runner never enables the application's live integrations.
const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const root = path.join(__dirname, "..");
const tests = fs.readdirSync(path.join(root, "test")).filter((name) => name.endsWith(".cjs")).sort();
for (const file of tests) {
  const result = spawnSync(process.execPath, [path.join(root, "test", file)], { cwd: root, stdio: "inherit", windowsHide: true });
  if (result.error || result.status !== 0) {
    console.error(result.error?.message || `Failed: ${file}`);
    process.exitCode = 1;
    break;
  }
}
