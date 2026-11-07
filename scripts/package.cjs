"use strict";

const { build, Platform, Arch } = require("electron-builder");
const { buildNative } = require("./build-native.cjs");

async function main() {
  if (process.platform !== "win32") throw new Error("Build the Windows application on Windows.");
  buildNative({ optional: true });
  await build({
    projectDir: require("node:path").join(__dirname, ".."),
    config: require("../electron-builder.cjs"),
    targets: Platform.WINDOWS.createTarget(["dir"], Arch.x64),
    publish: "never",
  });
}

main().catch((error) => { console.error(error.message); process.exitCode = 1; });
