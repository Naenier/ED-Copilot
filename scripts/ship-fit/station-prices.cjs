// Regenerate after rebuilding the pinned ship fitter catalogue.
// Run node scripts/ship-fit/station-prices.cjs at the repository root.
const {writeFileSync} = require('node:fs');
const {resolve} = require('node:path');
require(resolve('internal/frontend/ship-fit-engine.js'));

const engine = globalThis.EDCopilotShipFitEngine;
const key = value => String(value).toLowerCase().replace(/[^a-z0-9]/g, '');
const sortKeys = object => Object.fromEntries(
  Object.entries(object).sort(([left], [right]) => left.localeCompare(right, 'en'))
);
const ships = {};
const modules = {};
for (const ship of engine.ships()) {
  const fit = engine.create(ship.id);
  ships[key(ship.name)] = Math.round(engine.stats(fit).cost);
  for (const slot of engine.slots(fit)) {
    for (const module of [slot.module, ...engine.candidates(fit, slot.key)]) {
      if (module?.symbol && module.cost && !module.preEngineered) {
        modules[key(module.symbol)] ??= Math.round(module.cost);
      }
    }
  }
}

writeFileSync('internal/nearest/catalog_prices.json',
  JSON.stringify({ships: sortKeys(ships), modules: sortKeys(modules)}) + '\n');
