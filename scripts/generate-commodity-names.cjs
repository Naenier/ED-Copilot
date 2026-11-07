"use strict";

// Independent generator for the public FDev identifier tables. It never reads
// the existing output dictionary or another application's identifier mapping.
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const root = path.resolve(__dirname, "..");
const SOURCE_ROOT = "https://raw.githubusercontent.com/EDCD/FDevIDs/master/";
const SOURCES = ["commodity.csv", "rare_commodity.csv"];
const normalize = value => String(value || "").normalize("NFKD").replace(/[\u0300-\u036f]/g, "").toLowerCase().replace(/[^a-z0-9]/g, "");

function parseCsv(input) {
  const rows = []; let row = [], field = "", quoted = false;
  const source = input.replace(/^\uFEFF/, "");
  for (let i = 0; i < source.length; i++) {
    const char = source[i];
    if (quoted) {
      if (char === '"' && source[i + 1] === '"') { field += '"'; i++; }
      else if (char === '"') quoted = false;
      else field += char;
    } else if (char === '"' && field === "") quoted = true;
    else if (char === ",") { row.push(field); field = ""; }
    else if (char === "\n") { row.push(field); if (row.some(Boolean)) rows.push(row); row = []; field = ""; }
    else if (char !== "\r") field += char;
  }
  if (quoted) throw new Error("Unterminated quoted CSV field");
  if (field || row.length) { row.push(field); rows.push(row); }
  const header = rows.shift();
  if (!header || !["id", "symbol", "name"].every(name => header.includes(name))) throw new Error("Unexpected FDev commodity CSV header");
  return rows.map((values, index) => {
    if (values.length !== header.length) throw new Error(`CSV row ${index + 2} has an unexpected column count`);
    const entry = Object.fromEntries(header.map((key, column) => [key, values[column]]));
    if (!/^\d+$/.test(entry.id) || !entry.symbol || !entry.name) throw new Error(`CSV row ${index + 2} has incomplete identity fields`);
    return entry;
  });
}

function buildMap(sourceRows, planetaryResources = []) {
  const mapping = new Map(), rows = sourceRows.map(row => ({ symbol: row.symbol, name: row.name }));
  let supplemental = 0;
  for (const resource of planetaryResources) {
    if (resource.kind !== "commodity" || !resource.methods?.includes("surface-planet")) continue;
    if (!rows.some(row => normalize(row.symbol) === normalize(resource.key))) {
      rows.push({ symbol: resource.key, name: resource.english }); supplemental++;
    }
  }
  // Journal symbols have precedence over friendly-name aliases.
  for (const row of rows) {
    const key = normalize(row.symbol), previous = mapping.get(key);
    if (!key || !row.name) throw new Error("Empty commodity key or English name");
    if (previous && previous[1] !== row.symbol) throw new Error(`Conflicting canonical commodity symbol: ${row.symbol}`);
    mapping.set(key, [row.name, row.symbol]);
  }
  const skippedAliases = [];
  for (const row of rows) {
    const alias = normalize(row.name), previous = mapping.get(alias);
    if (!alias) throw new Error("Empty commodity name alias");
    if (!previous) mapping.set(alias, [row.name, row.symbol]);
    else if (previous[1] !== row.symbol) skippedAliases.push(alias);
  }
  return { mapping: Object.fromEntries([...mapping].sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0)), supplemental, skippedAliases };
}

async function generate(args = process.argv.slice(2)) {
  const option = name => { const index = args.indexOf(name); return index < 0 ? undefined : args[index + 1]; };
  const sourceDir = option("--source-dir"), output = path.resolve(option("--output") || path.join(root, "lib/commodity-names.json"));
  const notice = path.resolve(option("--notice") || path.join(path.dirname(output), "COMMODITY-DATA-NOTICE.txt"));
  const catalogPath = path.resolve(option("--catalog") || path.join(root, "lib/catalog.cjs"));
  const allRows = [], provenance = [];
  for (const file of SOURCES) {
    let bytes;
    if (sourceDir) bytes = fs.readFileSync(path.join(sourceDir, file));
    else {
      const response = await fetch(SOURCE_ROOT + file, { redirect: "error", signal: AbortSignal.timeout(30000) });
      if (!response.ok) throw new Error(`FDevIDs ${file}: HTTP ${response.status}`);
      bytes = Buffer.from(await response.arrayBuffer());
    }
    const rows = parseCsv(bytes.toString("utf8"));
    allRows.push(...rows);
    provenance.push({ file, rows: rows.length, sha256: crypto.createHash("sha256").update(bytes).digest("hex") });
  }
  const result = buildMap(allRows, require(catalogPath).resources);
  fs.mkdirSync(path.dirname(output), { recursive: true });
  fs.writeFileSync(output, JSON.stringify(result.mapping, null, 2) + "\n", "utf8");
  const lines = ["ED Copilot commodity identifiers and English names", "", "Generated independently from the EDCD/FDevIDs primary CSV tables:",
    ...provenance.flatMap(row => [`${SOURCE_ROOT}${row.file}`, `Rows: ${row.rows}; SHA256: ${row.sha256}`]), "",
    "Source README: https://github.com/EDCD/FDevIDs/blob/master/README.md",
    "The source describes these as Companion API IDs and in-game English names; it warns that the collections can be incomplete and that names may differ from CAPI/EDDN labels.",
    "Names and identifiers describe Elite Dangerous game content. Elite Dangerous and its game data remain the intellectual property of Frontier Developments plc, subject to Frontier's terms. This notice does not claim an MIT or other application-code license for the game data.",
    "https://www.frontierstore.net/terms-and-conditions/", "",
    "Generator: scripts/generate-commodity-names.cjs. Inputs are the two CSV tables above and missing planetary commodity entries from ED Copilot's own catalog.cjs. The generator never uses the existing output dictionary as input.",
    "Keys are normalized FDev symbols and normalized English-name aliases. Values are [English name, FDev symbol]. Canonical symbols take precedence over conflicting name aliases. Russian display labels are maintained separately.",
    `Generated: ${new Date().toISOString().slice(0, 10)}; source rows: ${allRows.length}; supplemental planetary rows: ${result.supplemental}; lookup entries: ${Object.keys(result.mapping).length}; conflicting aliases skipped: ${result.skippedAliases.length}.`, "",
    "No player identity, inventory, location, timestamps from journals, or account credentials are included.", ""];
  fs.writeFileSync(notice, lines.join("\n"), "utf8");
  return { output, sourceRows: allRows.length, entries: Object.keys(result.mapping).length, supplemental: result.supplemental, skippedAliases: result.skippedAliases };
}

module.exports = { parseCsv, buildMap, generate, normalize };
if (require.main === module) generate().then(result => console.log(JSON.stringify(result))).catch(error => { console.error(error.message); process.exitCode = 1; });
