"use strict";

const path = require("node:path");
const { spawnSync } = require("node:child_process");

function buildNative({ optional = false } = {}) {
  if (process.platform !== "win32") {
    if (optional) return false;
    throw new Error("The foreground-window helper is built on Windows.");
  }
  const probe = spawnSync("cargo", ["--version"], { windowsHide: true, encoding: "utf8" });
  if (probe.error || probe.status !== 0) {
    if (!optional) throw new Error("Install Rust and the Visual Studio C++ build tools, then run npm run build:native.");
    console.log("Rust is unavailable: the application will use its built-in hidden PowerShell foreground watcher.");
    return false;
  }
  const result = spawnSync("cargo", ["build", "--release", "--locked", "--manifest-path", path.join(__dirname, "..", "native", "Cargo.toml")], {
    stdio: "inherit",
    windowsHide: true,
  });
  if (result.error || result.status !== 0) throw result.error || new Error(`Native helper build failed (${result.status}).`);
  return true;
}

module.exports = { buildNative };
if (require.main === module) {
  try { buildNative(); } catch (error) { console.error(error.message); process.exitCode = 1; }
}
