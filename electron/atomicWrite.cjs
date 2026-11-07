"use strict";
// Independently implemented file persistence for ED Copilot.
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");

function renameWithRetry(source, destination, tries = 5) {
  const attempts = Number.isInteger(tries) ? Math.max(1, Math.min(tries, 8)) : 5;
  for (let attempt = 0; attempt < attempts; attempt++) {
    try { fs.renameSync(source, destination); return; }
    catch (error) {
      if (!["EACCES", "EPERM", "EBUSY"].includes(error.code) || attempt === attempts - 1) throw error;
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 15 * (attempt + 1));
    }
  }
}

function writeFileAtomic(filename, data, options = "utf8") {
  const destination = path.resolve(filename);
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  const temporary = destination + "." + crypto.randomBytes(12).toString("hex") + ".tmp";
  let descriptor;
  try {
    descriptor = fs.openSync(temporary, "wx", 0o600);
    fs.writeFileSync(descriptor, data, options);
    fs.fsyncSync(descriptor);
    fs.closeSync(descriptor);
    descriptor = undefined;
    renameWithRetry(temporary, destination);
  } finally {
    if (descriptor !== undefined) try { fs.closeSync(descriptor); } catch {}
    try { fs.unlinkSync(temporary); } catch (error) { if (error.code !== "ENOENT") throw error; }
  }
}
module.exports = { writeFileAtomic, renameWithRetry };

