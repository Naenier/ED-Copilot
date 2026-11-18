import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const [, , almanacArg, coriolisArg] = process.argv;
if (!almanacArg || !coriolisArg) {
  throw new Error("Usage: node scripts/ship-fit/localize-ru.mjs <Almanac-Core checkout> <coriolis-data checkout>");
}

const almanac = resolve(almanacArg);
const coriolis = resolve(coriolisArg);
const output = fileURLToPath(new URL("../../internal/frontend/ship-fit-ru.js", import.meta.url));

function readJSON(filename) {
  const text = readFileSync(filename, "utf8").replace(/^\s*\/\*[\s\S]*?\*\//, "");
  return JSON.parse(text);
}

function almanacData(area, name) {
  return readJSON(join(almanac, "data", area, name + ".jsonc"));
}

function coriolisData(folder) {
  return readdirSync(folder, { withFileTypes: true }).flatMap((entry) => {
    const filename = join(folder, entry.name);
    if (entry.isDirectory()) return coriolisData(filename);
    return entry.name.endsWith(".json") ? [readJSON(filename)] : [];
  });
}

function lookupKey(value) {
  return String(value || "").toLowerCase();
}

function shipKey(value) {
  return lookupKey(value).replace(/[^a-z0-9]/g, "");
}

function localizedRecords(table) {
  return Object.fromEntries(
    Object.entries(table).filter(([, value]) => value.ru).map(([key, value]) => [lookupKey(key), value.ru]),
  );
}

function requireName(name, kind, key) {
  if (!name || !/\p{Script=Cyrillic}/u.test(name)) {
    throw new Error("Missing Russian " + kind + " name for " + key);
  }
  return name;
}

function uniqueAliases(entries) {
  const names = new Map();
  for (const [key, name] of entries) {
    if (key) {
      if (!names.has(key)) names.set(key, new Set());
      names.get(key).add(name);
    }
  }
  return Object.fromEntries([...names].filter(([, labels]) => labels.size === 1).map(([key, labels]) => [key, [...labels][0]]));
}

const moduleNames = almanacData("i18n", "module-names");
const moduleNameKeys = Object.fromEntries(
  Object.entries(moduleNames.nameKeys).map(([key, value]) => [lookupKey(key), value]),
);
const modules = Object.fromEntries(Object.entries(moduleNameKeys).map(([key, sourceName]) => {
  const name = requireName(moduleNames.names[sourceName].ru, "module", key);
  return [key, key.endsWith("_free") ? name + " (Бесплатно)" : name];
}));
const moduleFamilies = almanacData("i18n", "module-family-names");
const gameModules = Object.fromEntries(
  ["core", "hardpoint", "internal", "utility"].flatMap((group) =>
    almanacData("ships", "modules-" + group).map((entry) => [lookupKey(entry.symbol), entry]),
  ),
);
const gameShips = almanacData("ships", "ships");
const shipByName = new Map(gameShips.map((ship) => [shipKey(ship.name), ship]));
const shipByFittingName = new Map([...shipByName, ["viper", shipByName.get("vipermkiii")]]);

const symbolsNotInGame = {
  int_missing_powerplant: "Неизвестная силовая установка",
  int_missing_engine: "Неизвестные маневровые двигатели",
  int_missing_hyperdrive: "Неизвестный двигатель FSD",
  int_missing_lifesupport: "Неизвестная система жизнеобеспечения",
  int_missing_powerdistributor: "Неизвестный распределитель питания",
  int_missing_sensors: "Неизвестные сенсоры",
  int_missing_fueltank: "Неизвестный топливный бак",
  hpt_missing_hardpoint: "Неизвестное оружие",
  hpt_missing_utility: "Неизвестное внешнее оборудование",
  int_missing_module: "Неизвестный модуль",
};

const variantNames = almanacData("i18n", "pre-engineered-variant-names");
const specialVariants = {
  "6C": { experimental: "special_feedback_cascade_cooled" },
  "6I": { experimental: "" },
  "6J": { experimental: "" },
};

function variantName(module, baseName) {
  const engineering = module.preEngineered;
  if (!engineering) return null;

  const experimental = specialVariants[module.id]?.experimental ?? engineering.experimentalEffects?.[0] ?? "";
  const acquisition = engineering.availability === "CG" ? "communityGoal" : "techBroker";
  for (const blueprint of engineering.blueprints) {
    const variantKey = [module.symbol, blueprint, experimental, acquisition].map(lookupKey).join("|");
    const result = variantsByKey[variantKey];
    if (result) return result;
  }
  return baseName;
}

const variantsByKey = Object.fromEntries(
  Object.entries(variantNames.nameKeys)
    .filter(([, name]) => variantNames.names[name].ru)
    .map(([key, name]) => [lookupKey(key), variantNames.names[name].ru]),
);
const variants = {};
const ships = Object.fromEntries(gameShips.map((ship) => [lookupKey(ship.symbol), ship.name]));
ships.lynx_highliner = ships.mediumtransport01;
ships.anaconda_mk_ii = ships.anacondamkii;
const moduleIDs = [];
const nameAliases = Object.entries(moduleNames.names).map(([name, terms]) => [name, terms.ru]);
const familyCandidates = new Map();
const groupIDs = new Set();

function addModule(entry, symbol = entry.symbol) {
  const key = lookupKey(symbol);
  const name = requireName(modules[key] || symbolsNotInGame[key], "Coriolis module", entry.id + " " + key);
  const variant = variantName(entry, name);
  if (variant) variants[entry.id] = variant;
  if (entry.edID) moduleIDs.push([String(entry.edID), name]);
  nameAliases.push([entry.name, variant || name], [entry.ukName, variant || name]);

  groupIDs.add(entry.grp);
  if (!familyCandidates.has(entry.grp)) familyCandidates.set(entry.grp, new Map());
  const candidates = familyCandidates.get(entry.grp);
  const family = moduleFamilies[gameModules[key]?.familyId]?.ru;
  if (!variant && !symbolsNotInGame[key]) {
    const category = family || name;
    candidates.set(category, (candidates.get(category) || 0) + 1);
  }
}

const coriolisModules = coriolisData(join(coriolis, "modules")).flatMap((groups) => Object.values(groups).flat());
for (const entry of coriolisModules) addModule(entry);

const fittingShips = coriolisData(join(coriolis, "ships")).flatMap(Object.entries);
const armourGrades = ["grade1", "grade2", "grade3", "mirrored", "reactive"];
for (const [shipId, ship] of fittingShips) {
  const model = shipByFittingName.get(shipKey(ship.properties.name));
  if (!model) throw new Error("Unmapped ship " + shipId);
  ships[lookupKey(shipId)] = model.name;
  ship.bulkheads.forEach((entry, index) => addModule(entry, entry.symbol || model.symbol + "_armour_" + armourGrades[index]));
}

const groupOverrides = {
  bh: "Броня",
  fsd: "Двигатели FSD",
  mh: "Неизвестное внешнее оборудование",
  mm: "Неизвестные модули",
};
const groups = Object.fromEntries([...groupIDs].map((id) => {
  const majority = [...familyCandidates.get(id)].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0], "ru"))[0]?.[0];
  return [id, requireName(groupOverrides[id] || majority, "module group", id)];
}));

const blueprints = localizedRecords(almanacData("i18n", "blueprint-names"));
const experimentals = localizedRecords(almanacData("i18n", "experimental-effect-names"));
const effectAliases = {
  special_feedback_cascade: "special_feedback_cascade_cooled",
  special_ion_disruptor: "special_choke_canister",
  special_mass_lock_munition: "special_mass_lock",
  special_penetrator_payload: "special_deep_cut_payload",
  special_plasma_slug_pa: "special_plasma_slug",
};
for (const [oldId, currentId] of Object.entries(effectAliases)) {
  experimentals[oldId] = requireName(experimentals[currentId], "experimental effect", currentId);
}

const blueprintRows = Object.entries(readJSON(join(coriolis, "modifications", "blueprints.json")));
for (const [key, blueprint] of blueprintRows) {
  const name = requireName(blueprints[lookupKey(blueprint.fdname || key)], "Coriolis blueprint", key);
  blueprints[lookupKey(key)] = name;
}
const experimentalRows = Object.entries(readJSON(join(coriolis, "modifications", "specials.json")));
for (const [key, experimental] of experimentalRows) {
  const name = requireName(experimentals[lookupKey(experimental.edname || key)], "Coriolis experimental effect", key);
  experimentals[lookupKey(key)] = name;
}

const bundle = {
  modules: { ...modules, ...symbolsNotInGame },
  moduleIds: uniqueAliases(moduleIDs),
  names: uniqueAliases(nameAliases),
  variants,
  blueprints,
  experimentals,
  groups,
  ships,
};
for (const key of Object.keys(bundle)) {
  bundle[key] = Object.fromEntries(Object.keys(bundle[key]).sort().map((name) => [name, bundle[key][name]]));
}

writeFileSync(output, "// Generated by scripts/ship-fit/localize-ru.mjs. See docs/SHIP-FIT-LOCALIZATION.md.\n"
  + "globalThis.EdCopilotShipFitRU = " + JSON.stringify(bundle, null, 2) + ";\n");

const real = coriolisModules.filter((entry) => !symbolsNotInGame[lookupKey(entry.symbol)]).length;
console.log("Russian coverage: %d modules plus %d armour rows, %d hulls, %d blueprints, %d experimental effects, %d named pre-engineered rows.",
  real, fittingShips.reduce((sum, [, ship]) => sum + ship.bulkheads.length, 0), fittingShips.length,
  blueprintRows.length, experimentalRows.length, Object.keys(variants).length);
