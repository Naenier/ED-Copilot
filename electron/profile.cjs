"use strict";

const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");

function resolveProfileDir(baseDir, environment = process.env) {
  const override = environment.ED_COPILOT_DATA_DIR || environment.ED_MINING_DATA_DIR;
  if (override) return path.resolve(override);
  const current = path.resolve(baseDir, "ED Copilot");
  const legacy = path.resolve(baseDir, "ED Mining");
  if (!fs.existsSync(current) && fs.existsSync(legacy)) {
    // Copy the entire Electron profile too: Windows safeStorage uses its existing Local State.
    // Keep the original profile intact, and publish the new directory only after a full copy.
    const temporary = path.join(path.resolve(baseDir), ".ed-copilot-migration-" + crypto.randomUUID());
    try {
      fs.cpSync(legacy, temporary, { recursive: true, errorOnExist: true, force: false });
      fs.renameSync(temporary, current);
    } catch (error) {
      if (!fs.existsSync(current)) {
        console.warn("Не удалось перенести профиль ED Copilot; используется существующий профиль:", error.message);
        return legacy;
      }
    }
  }
  return current;
}

module.exports = { resolveProfileDir };
