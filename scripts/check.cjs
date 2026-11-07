"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const root = path.join(__dirname, "..");
let failed = false;
let count = 0;
function visit(dir) {
  for (const item of fs.readdirSync(dir, { withFileTypes: true })) {
    const file = path.join(dir, item.name);
    if (item.isDirectory()) visit(file);
    else if (/\.(cjs|js)$/.test(item.name)) {
      const result = spawnSync(process.execPath, ["--check", file], { encoding: "utf8", windowsHide: true });
      count++;
      if (result.error || result.status !== 0) {
        failed = true;
        console.error(path.relative(root, file), result.error?.message || result.stderr);
      }
    }
  }
}
for (const dir of ["electron", "lib", "web", "scripts"]) visit(path.join(root, dir));
for (const file of ["package.json", ...fs.readdirSync(path.join(root, "lib")).filter((name) => name.endsWith(".json")).map((name) => `lib/${name}`)]) {
  try { JSON.parse(fs.readFileSync(path.join(root, file), "utf8")); }
  catch (error) { failed = true; console.error(file, error.message); }
}
console.log(`Checked ${count} JavaScript files and static JSON data.`);
process.exitCode = failed ? 1 : 0;
