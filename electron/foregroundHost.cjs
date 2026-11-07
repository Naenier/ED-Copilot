"use strict";
// This process owns the focus monitor and relays only process identity and position.
const { watchForeground } = require("./foregroundWatch.cjs");
const port = process.parentPort;
let monitor;
if (port) {
  monitor = watchForeground((name, at) => port.postMessage({ type: "foreground", name, at }));
  port.on("message", message => {
    if (message?.data?.type === "stop") { monitor.stop(); process.exit(0); }
  });
}
process.once("exit", () => monitor?.stop());

