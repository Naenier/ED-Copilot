"use strict";

const http = require("node:http");
const fs = require("node:fs");
const path = require("node:path");
const { MiningEngine } = require("./engine.cjs");
const { MiningSearch } = require("./search.cjs");
const { MarketSearch } = require("./markets.cjs");
const { FleetRoutePlanner } = require("./carrier-route.cjs");
const { FrontierClient, marketSnapshot } = require("./frontier.cjs");
const { TradeRouteFinder } = require("./trade-routes.cjs");
const { IntegrationManager } = require("./integrations.cjs");
const { normalizeTheme, readGamePalette } = require("./theme.cjs");

const TYPES = { ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8", ".woff2": "font/woff2", ".png": "image/png", ".ico": "image/x-icon", ".svg": "image/svg+xml" };
const CSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'";

function json(response, status, value) {
  response.writeHead(status, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" });
  response.end(JSON.stringify(value));
}

async function readBody(request, limit = 65536) {
  let size = 0;
  const chunks = [];
  for await (const chunk of request) {
    size += chunk.length;
    if (size > limit) throw new Error("Запрос слишком большой.");
    chunks.push(chunk);
  }
  try { return JSON.parse(Buffer.concat(chunks).toString("utf8") || "{}"); }
  catch { throw new Error("Некорректный JSON."); }
}

function csv(state) {
  const escape = (value) => '"' + String(value ?? "").replaceAll('"', '""') + '"';
  const rows = [["Ресурс", "Произведено, т", "Сырьё, ед.", "Продано, т", "Выручка, кр."]];
  for (const row of state.stats.byResource) rows.push([row.name, row.refinedTons, row.materialUnits, row.soldTons, row.revenue]);
  return "\uFEFF" + rows.map((row) => row.map(escape).join(";")).join("\r\n");
}

function tradingCsv(state, transactions) {
  const escape = (value) => '"' + String(value ?? "").replaceAll('"', '""') + '"';
  const rows = [["Время", "Операция", "Товар", "Тонны", "Сумма, кр.", "Маржа по известной себестоимости", "Внутренний перевод", "Система", "Станция"]];
  const labels = { buy: "Покупка", sell: "Продажа", transfer: "Перевод", cost: "Расход" };
  for (const item of transactions) rows.push([item.timestamp, labels[item.kind] ?? item.kind,
    item.name, item.count, item.value, item.margin, item.internal ? "Да" : "Нет", item.system, item.station]);
  return "\uFEFF" + rows.map((row) => row.map(escape).join(";")).join("\r\n");
}

function edmcMarket(body, commander) {
  const profile = body?.data;
  const at = Date.parse(body?.observedAt);
  if (body?.type !== "capi" || body.isBeta !== false || !Number.isFinite(at) ||
    Date.now() - at > 120000 || at - Date.now() > 30000 || !commander.docked ||
    !commander.name || body.commander?.toLowerCase() !== commander.name.toLowerCase() ||
    profile?.commander?.name?.toLowerCase() !== commander.name.toLowerCase() ||
    profile?.lastSystem?.name !== commander.system || profile?.lastStarport?.name !== commander.station ||
    String(profile?.lastStarport?.id) !== String(commander.marketId)) return null;
  const market = marketSnapshot({ status: { connected: true, commander: commander.name },
    data: { profile, market: profile.lastStarport },
    metadata: { source: "Frontier CAPI", observedAt: { market: body.observedAt } } });
  return market ? { ...market, Source: "EDMC · Frontier CAPI" } : null;
}

async function startServer({ dataDir, journalDir, port = 7122, host = "127.0.0.1", secureStorage, openExternal }) {
  fs.mkdirSync(dataDir, { recursive: true });
  const engine = new MiningEngine({ dataDir, journalDir });
  const search = new MiningSearch(engine);
  const markets = new MarketSearch(engine);
  const carrierRoute = new FleetRoutePlanner(engine);
  const tradeRoutes = new TradeRouteFinder(engine);
  let frontier;
  let integrations;
  const clients = new Set();
  const webRoot = path.resolve(__dirname, "../web");
  let baseUrl;
  const server = http.createServer(async (request, response) => {
    response.setHeader("X-Content-Type-Options", "nosniff");
    response.setHeader("Referrer-Policy", "no-referrer");
    const origin = request.headers.origin;
    const foreign = origin && origin !== baseUrl;
    const oauthCallback = request.method === "GET" && request.url?.split("?")[0] === "/api/frontier/callback";
    const callbackHost = baseUrl && "localhost:" + new URL(baseUrl).port;
    const localCallback = oauthCallback && request.headers.host === callbackHost && ["127.0.0.1", "::1", "::ffff:127.0.0.1"].includes(request.socket.remoteAddress);
    const wrongHost = baseUrl && request.headers.host !== new URL(baseUrl).host && !localCallback;
    if (wrongHost || (!oauthCallback && (foreign || request.headers["sec-fetch-site"] === "cross-site"))) {
      json(response, 403, { ok: false, error: "Доступ разрешён только локальному приложению." });
      return;
    }
    const url = new URL(request.url, baseUrl || "http://127.0.0.1");
    try {
      if (oauthCallback) {
        let message = "Вход Frontier завершён. Можно вернуться в ED Copilot.";
        let status = 200;
        try { await frontier.callback(url.searchParams); }
        catch (error) { status = 400; message = error.message; }
        const escaped = message.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
        response.writeHead(status, { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store", "Content-Security-Policy": CSP });
        response.end('<!doctype html><html lang="ru"><meta charset="utf-8"><title>ED Copilot · Frontier</title><style>body{background:#07101a;color:#e0e0e0;font:16px "Segoe UI",Tahoma,sans-serif;max-width:760px;margin:60px auto;padding:24px;line-height:1.6}a{color:#71c8ff}</style><body><main><h1>Frontier</h1><p>' + escaped + '</p><a href="' + baseUrl + '/#account">Вернуться в ED Copilot</a></main></body></html>');
        return;
      }
      if (request.method === "GET" && url.pathname === "/api/state") {
        json(response, 200, engine.getState());
        return;
      }
      if (request.method === "GET" && url.pathname === "/api/events") {
        response.writeHead(200, { "Content-Type": "text/event-stream; charset=utf-8", "Cache-Control": "no-cache", Connection: "keep-alive" });
        response.write("event: state\ndata: " + JSON.stringify(engine.getState()) + "\n\n");
        clients.add(response);
        request.on("close", () => clients.delete(response));
        return;
      }
      if (request.method === "GET" && url.pathname === "/api/export.csv") {
        response.writeHead(200, { "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": 'attachment; filename="ed-copilot-statistics.csv"' });
        response.end(csv(engine.getState()));
        return;
      }
      if (request.method === "GET" && url.pathname === "/api/trading/export.csv") {
        response.writeHead(200, { "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": 'attachment; filename="ed-copilot-trading.csv"' });
        response.end(tradingCsv(engine.getState(), engine.tradeLedger.transactions));
        return;
      }
      if (request.method === "GET" && url.pathname === "/api/carrier/route.csv") {
        response.writeHead(200, { "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": 'attachment; filename="ed-copilot-carrier-route.csv"' });
        response.end(carrierRoute.exportCsv());
        return;
      }
      if (request.method === "GET" && url.pathname === "/api/engineering/plan.json") {
        response.writeHead(200, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store", "Content-Disposition": 'attachment; filename="ed-copilot-engineering-plan.json"' });
        response.end(JSON.stringify(engine.engineeringPlanner.exportPlan(), null, 2));
        return;
      }
      if (request.method === "GET" && url.pathname === "/api/engineering/materials.csv") {
        const escape = (value) => '"' + String(value ?? "").replaceAll('"', '""') + '"';
        const labels = { raw: "Сырьё", manufactured: "Промышленные материалы", encoded: "Данные" };
        const rows = [["Материал", "Тип", "Уровень", "Есть", "Нужно", "Не хватает"]];
        for (const row of engine.engineeringPlanner.snapshot().materials.filter((m) => m.needed > 0)) {
          rows.push([row.name, labels[row.category] || row.category, row.grade, row.owned, row.needed, row.missing]);
        }
        response.writeHead(200, { "Content-Type": "text/csv; charset=utf-8", "Content-Disposition": 'attachment; filename="ed-copilot-engineering-materials.csv"' });
        response.end("\uFEFF" + rows.map((row) => row.map(escape).join(";")).join("\r\n"));
        return;
      }
      if (request.method === "POST" && url.pathname.startsWith("/api/")) {
        if (request.headers["content-type"]?.split(";")[0] !== "application/json") {
          json(response, 415, { ok: false, error: "Требуется application/json." });
          return;
        }
        const body = await readBody(request, ["/api/engineering", "/api/integrations/edmc"].includes(url.pathname) ? 524288 : 65536);
        switch (url.pathname) {
          case "/api/theme": engine.setTheme(body.settings); break;
          case "/api/theme/preview": {
            json(response, 200, { ok: true, theme: engine.themeState(normalizeTheme(body.settings)) }); return;
          }
          case "/api/theme/import": {
            const result = await readGamePalette({ filePath: body.filePath });
            result.settings.overlayOpacity = engine.appearance.overlayOpacity;
            json(response, 200, { ok: true, theme: engine.themeState(result.settings) }); return;
          }
          case "/api/integrations/configure": integrations.configure(body); break;
          case "/api/integrations/sync": await integrations.sync(); break;
          case "/api/integrations/pair": {
            const result = integrations.pair();
            json(response, 200, { ok: true, ...result, url: baseUrl + "/api/integrations/edmc" }); return;
          }
          case "/api/integrations/edmc": {
            if (origin || !["127.0.0.1", "::1", "::ffff:127.0.0.1"].includes(request.socket.remoteAddress)) {
              json(response, 403, { ok: false, error: "Мост принимает запросы только от локального EDMC." }); return;
            }
            const result = integrations.acceptEdmc(body, request.headers["x-edcopilot-token"]);
            const market = integrations.status({ commander: engine.commander }).identityMatches ? edmcMarket(body, engine.commander) : null;
            if (market) { engine.applyMarket(market); engine.emit(); }
            json(response, 200, result); return;
          }
          case "/api/settings": engine.settings(body); break;
          case "/api/rescan": engine.rescanRequested = true; void engine.poll(); break;
          case "/api/journal-dir": await engine.setDirectory(body.path); break;
          case "/api/bookmarks": engine.bookmark(body.action, body.targetId ?? body.id); break;
          case "/api/session":
            if (body.action !== "reset") throw new Error("Неизвестное действие.");
            engine.resetSession();
            break;
          case "/api/market": engine.setPrices(body.prices); break;
          case "/api/search": await search.start(body); break;
          case "/api/search/cancel": search.cancel(); break;
          case "/api/trading/search": await markets.start(body); break;
          case "/api/trading/cancel": markets.cancel(); break;
          case "/api/trading/destination": engine.setTradingDestination(body.destination); break;
          case "/api/trading/plan": engine.setTradePlan(body); break;
          case "/api/carrier/planning": engine.setCarrierPlanning(body); break;
          case "/api/carrier/stock": engine.setCarrierStock(body); break;
          case "/api/carrier/rescan": engine.rescanRequested = true; void engine.poll(); break;
          case "/api/carrier/route": await carrierRoute.start(body); break;
          case "/api/carrier/route/cancel": carrierRoute.cancel(); break;
          case "/api/carrier/route/visited": carrierRoute.markVisited(body.index); break;
          case "/api/trading/routes": await tradeRoutes.start(body); break;
          case "/api/trading/routes/cancel": tradeRoutes.cancel(); break;
          case "/api/mining/plan": engine.setMiningPlan(body); break;
          case "/api/mining/reset": engine.resetMiningRun(); break;
          case "/api/engineering": engine.updateEngineering(body); break;
          case "/api/objectives": engine.updateObjective(body); break;
          case "/api/frontier/configure": frontier.configure(body); break;
          case "/api/frontier/login": {
            const login = frontier.startLogin();
            if (openExternal) await openExternal(login.url);
            json(response, 200, { ok: true, url: login.url }); return;
          }
          case "/api/frontier/sync": {
            const result = await frontier.sync({ carrier: body.carrier !== false, shipyard: true });
            integrations.observeCapi(result);
            const market = marketSnapshot(result);
            if (market && engine.commander.docked && String(engine.commander.marketId) === String(market.MarketID)) {
              engine.applyMarket(market); engine.emit();
            }
            break;
          }
          case "/api/frontier/warehouse": {
            const account = frontier.status();
            const actual = account.summary?.carrier;
            const own = engine.carrierTracker.getState();
            if (!account.connected || !actual?.owned || !own.owned || !actual.callsign || actual.callsign !== own.callsign || !account.lastCarrierSync) {
              throw new Error("Нужен подтверждённый снимок вашего флотоносца Frontier и совпадающий позывной в журнале игры.");
            }
            const stock = new Map();
            for (const item of actual.cargo) stock.set(item.name, (stock.get(item.name) || 0) + item.count);
            engine.setCarrierStock({ carrierId: own.id, timestamp: account.lastCarrierSync,
              items: [...stock].map(([name, count]) => ({ name, count })) });
            break;
          }
          case "/api/frontier/logout": frontier.logout(); break;
          default: json(response, 404, { ok: false, error: "Метод не найден." }); return;
        }
        json(response, 200, { ok: true });
        return;
      }
      if (request.method !== "GET" && request.method !== "HEAD") {
        json(response, 405, { ok: false, error: "Метод не поддерживается." });
        return;
      }
      let relative;
      try { relative = decodeURIComponent(url.pathname === "/" ? "/index.html" : url.pathname); }
      catch { json(response, 400, { ok: false, error: "Некорректный путь." }); return; }
      const filename = path.resolve(webRoot, "." + relative);
      const type = TYPES[path.extname(filename)];
      if (!type || !filename.startsWith(webRoot + path.sep)) {
        json(response, 404, { ok: false, error: "Файл не найден." });
        return;
      }
      const stat = await fs.promises.stat(filename).catch(() => null);
      if (!stat?.isFile()) { json(response, 404, { ok: false, error: "Файл не найден." }); return; }
      response.writeHead(200, { "Content-Type": type, "Cache-Control": "no-store", "Content-Security-Policy": CSP });
      if (request.method === "HEAD") response.end();
      else fs.createReadStream(filename).on("error", () => response.destroy()).pipe(response);
    } catch (error) {
      if (!response.headersSent) json(response, Number.isInteger(error.status) && error.status >= 400 && error.status <= 503 ? error.status : 400, { ok: false, error: error.message });
      else response.end();
    }
  });
  engine.subscribe((state) => {
    carrierRoute.observeCarrier(state.carrier);
    const message = "event: state\ndata: " + JSON.stringify(state) + "\n\n";
    for (const client of clients) {
      if (client.destroyed || client.writableLength > 2000000) { clients.delete(client); client.destroy(); }
      else client.write(message);
    }
  });
  const heartbeat = setInterval(() => {
    for (const client of clients) client.write(": heartbeat\n\n");
  }, 15000);
  heartbeat.unref?.();
  try {
    await new Promise((resolve, reject) => {
      server.once("error", reject);
      server.listen(port, host, resolve);
    });
  } catch (error) { engine.close(); clearInterval(heartbeat); throw error; }
  baseUrl = "http://" + host + ":" + server.address().port;
  frontier = new FrontierClient({ dataDir, redirectUri: "http://localhost:" + server.address().port + "/api/frontier/callback",
    encrypt: secureStorage?.encrypt, decrypt: secureStorage?.decrypt,
    getCommander: () => engine.commander,
    onUpdate: (state) => { engine.frontierState = state; engine.emit(); } });
  engine.frontierState = frontier.status();
  engine.frontierStatus = () => frontier.status();
  integrations = new IntegrationManager({ dataDir, secureStorage,
    bridgeUrl: baseUrl + "/api/integrations/edmc",
    getState: () => engine.getState(), onUpdate: () => engine.emit() });
  engine.integrationsStatus = (context) => integrations.status(context);
  const stopJournal = engine.observeJournal((event, meta) => integrations.observeJournal(event, meta));
  const stopSide = engine.observeSide((event, meta) => integrations.observeSide(event, meta));
  engine.emit();
  return { url: baseUrl, engine, frontier, carrierRoute, integrations, close: async () => {
    stopJournal(); stopSide(); integrations.close();
    search.cancel();
    markets.cancel();
    carrierRoute.dispose();
    tradeRoutes.cancel();
    engine.close();
    clearInterval(heartbeat);
    for (const client of clients) client.end();
    await new Promise((resolve) => {
      server.close(resolve);
      // Chromium can keep an EventSource or idle HTTP connection open until its window closes.
      server.closeAllConnections();
    });
  } };
}

module.exports = { startServer, edmcMarket };
