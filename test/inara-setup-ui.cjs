"use strict";
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const assert = require("node:assert/strict");
const handlers = new Map(), calls = [];
let currentInput = null;
const h = value => String(value ?? "").replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
const window = {
  MiningUI: { h, n: value => String(value ?? 0), dateTime: value => value || "—", credits: value => String(value), duration: value => String(value),
    pill: label => "<span>" + h(label) + "</span>" },
  addEventListener() {}
};
const document = {
  activeElement: null,
  getElementById: id => id === "sync-inara-key" ? currentInput : null,
  querySelector: () => null,
  addEventListener(type, fn) { const list = handlers.get(type) || []; list.push(fn); handlers.set(type, list); }
};
const sandbox = vm.createContext({ window, document, location: { hash: "#help" }, requestAnimationFrame: () => 0, queueMicrotask });
const source = file => fs.readFileSync(path.join(__dirname, "../web", file), "utf8");
for (const file of ["integrations.js", "expedition.js", "help.js"]) vm.runInContext(source(file), sandbox);
const state = { app: { version: "1.8.0" }, commander: { name: "Private Pilot", fid: "FPRIVATE" },
  integrations: { enabled: false, storageAvailable: true, apiKeySet: true, queued: 0, lastSuccess: null },
  frontier: { storageAvailable: true, connected: false } };
async function run() {
  const ui = window.CopilotIntegrations;
  let html = ui.render(state);
  assert(html.includes('type="password"'));
  assert(html.includes("Отправлять события в Inara"));
  assert(html.includes("https://inara.cz/elite/cmdr-settings-api/"));
  assert(!html.includes("data-sync-action"));
  assert(!html.includes("Последняя отправка подтверждена"));
  assert.deepEqual(Object.keys(ui).sort(), ["init", "render"]);
  ui.init({ getState: () => state, render() {}, showToast() {}, perform: async (url, body) => {
    assert.equal(currentInput.value, "", "Password is cleared before awaiting a response");
    calls.push({ url, body: { ...body }, original: body });
    return { ok: true };
  } });
  const input = { type: "password", value: "synthetic-private-key", disabled: false, placeholder: "", focus() {}, setSelectionRange() {} };
  currentInput = input;
  const form = { id: "integrations-form", elements: { enabled: { checked: true } }, querySelector: () => input };
  await handlers.get("submit")[0]({ target: form, preventDefault() {} });
  assert.equal(calls[0].url, "/api/integrations/configure");
  assert.equal(calls[0].body.apiKey, "synthetic-private-key");
  assert.equal(calls[0].body.enabled, true);
  assert.equal(Object.hasOwn(calls[0].original, "apiKey"), false, "The request object does not retain a key after saving");
  assert(!ui.render(state).includes("synthetic-private-key"));
  await handlers.get("submit")[0]({ target: form, preventDefault() {} });
  assert.equal(Object.hasOwn(calls[1].body, "apiKey"), false, "An empty password field does not replace a saved key");

  // Frequent state events must not discard an unsubmitted password.
  input.value = "unsubmitted-synthetic-key"; currentInput = input;
  ui.render(state);
  currentInput = { type: "password", disabled: false, placeholder: "replacement", replaceWith(node) { currentInput = node; } };
  await Promise.resolve();
  assert.equal(currentInput, input);
  assert.equal(currentInput.value, "unsubmitted-synthetic-key");
  currentInput = null;
  html = ui.render({ ...state, integrations: { ...state.integrations, lastError: '<script>bad</script>' } });
  assert(!html.includes("<script>"));
  assert(html.includes("&lt;script&gt;"));

  const account = window.MiningExpedition.renderAccount(state);
  assert(account.includes("Войти через Frontier"));
  assert(!account.includes("frontier-form"));
  assert(!/Client\s?ID|Developer Zone|PKCE|OAuth/.test(account));
  const connected = window.MiningExpedition.renderAccount({ ...state, frontier: { ...state.frontier, connected: true,
    summary: { profile: { commander: "Private Pilot", credits: 100, system: "Sol", ship: "Python" } } } });
  assert(connected.includes("Обновить данные") && connected.includes("Выйти") && connected.includes("Private Pilot"));
  window.MiningHelp.open("integrations");
  const help = window.MiningHelp.render(state);
  assert(help.includes('href="https://inara.cz/elite/cmdr-settings-api/"'));
  assert(help.includes("Отправлять события в Inara"));
  assert(!help.includes("copy-inara-request"));
  window.MiningHelp.open("frontier");
  assert(!/Client\s?ID|Developer Zone|PKCE/.test(window.MiningHelp.render(state)));
  window.MiningHelp.open("carrier-route");
  assert(window.MiningHelp.render(state).includes("Открыть: Маршрут флотоносца"));
  console.log("Account UI: simple Frontier sign-in, Inara key+checkbox, safe password retention/save, escaped errors and matching help passed.");
}
run().catch(error => { console.error(error); process.exitCode = 1; });
