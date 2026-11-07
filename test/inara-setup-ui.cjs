"use strict";
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const assert = require("node:assert/strict");
const handlers = new Map();
let copied = "", apiCalls = 0;
const h = value => String(value ?? "").replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
const window = {
  MiningUI: { h, n: value => String(value ?? 0), dateTime: value => value || "—", pill: label => '<span>' + h(label) + '</span>', copyText: async value => { copied = value; } },
  addEventListener() {},
};
const document = {
  getElementById: () => null,
  querySelector: () => null,
  addEventListener(type, fn) { const list = handlers.get(type) || []; list.push(fn); handlers.set(type, list); },
};
const sandbox = vm.createContext({ window, document, location: { hash: "#help" }, requestAnimationFrame: () => 0, queueMicrotask });
const source = file => fs.readFileSync(path.join(__dirname, "../web", file), "utf8");
vm.runInContext(source("integrations.js"), sandbox);
vm.runInContext(source("help.js"), sandbox);
const state = {
  app: { version: "1.7.0" }, commander: { name: "Private Pilot", fid: "FPRIVATE" },
  privateApiKey: "should-never-be-copied", frontier: { clientId: "private-client-id" },
  integrations: { publisher: "copilot", storageAvailable: true, identityMatches: true, live: { ok: true },
    commander: { name: "Private Pilot", fid: "FPRIVATE" }, inara: { enabled: true, keySet: true, appRegistered: false, setupRequired: true },
    eddn: { enabled: false }, queue: { total: 0 } },
};
async function run() {
  const ui = window.CopilotIntegrations;
  const html = ui.render(state);
  assert(html.includes('https://inara.cz/elite/cmdr-settings-api/'));
  assert(html.includes('https://inara.cz/elite/cmdr/1/'));
  assert(html.includes('Нужно разрешение Inara для ED Copilot'));
  assert(html.includes('Она не отправляет заявку и не выдаёт разрешение'));
  assert(!html.includes('Последняя отправка подтверждена'));
  ui.init({ getState: () => state, render() {}, showToast() {}, perform: async () => { apiCalls++; throw new Error("No API call expected"); } });
  const button = { disabled: false, dataset: { syncAction: "copy-inara-request" } };
  await handlers.get("click")[0]({ target: { closest: () => button } });
  assert(copied.includes('header.appName: ED Copilot'));
  assert(copied.includes('Current version: 1.7.0'));
  assert(copied.includes('https://github.com/Naenier/ED-Copilot'));
  assert(copied.includes('live-account export still needs validation'));
  for (const secret of ['should-never-be-copied', 'private-client-id', 'Private Pilot', 'FPRIVATE']) assert(!copied.includes(secret));
  assert.equal(apiCalls, 0, 'Copying a request must not send an API call or an administrator message');
  window.MiningHelp.open('integrations');
  const help = window.MiningHelp.render(state);
  assert(help.includes('href="https://inara.cz/elite/cmdr/1/"'));
  assert(help.includes('href="https://inara.cz/elite/cmdr-settings-api/"'));
  assert(help.includes('data-sync-action="copy-inara-request"'));
  assert(!help.includes('&lt;a href=&quot;https://inara.cz/elite/cmdr/1/'));
  window.MiningHelp.open('carrier-route');
  assert(window.MiningHelp.render(state).includes('Открыть: Маршрут флотоносца'));
  console.log('Inara setup UI: personal-key vs application approval, live request copy without secrets or network calls, real help links and fleet-carrier navigation passed.');
}
run().catch(error => { console.error(error); process.exitCode = 1; });
