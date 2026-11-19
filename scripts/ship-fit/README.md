# Ship fitting bundle

From this directory, run `npm ci` and `node build.mjs`. Node downloads the pinned Coriolis, coriolis-data and Elite Dangerous Almanac source revisions using Git and regenerates `internal/frontend/ship-fit-engine.js` and the accompanying notices. The native app embeds the generated engine. Browsing and editing a fit work offline.

Coriolis supplies the calculation engine, its compact module encoding and the foundation of the catalogue. Almanac supplies corrected numerical hull attributes, journal slot keys and facts for hulls and modules released after the pinned Coriolis data. EDSY measurements supplement the newer hulls' boost parameters and the journal catalogue distinguishes the original and Mk II hangar variants. License and data notices are embedded in the bundle. The application owns the interface and database.

An ED Copilot build stores the compact code, model, title and optional load scenario (fuel, cargo and distributor allocation) separately. Native export preserves those settings. Coriolis share links and Detailed JSON also carry the scenario, while Coriolis backups import each named fit. SLEF and Loadout use the station and game module symbols. A build with a supplemental hull or module can still be exchanged in native JSON and SLEF. Availability of those parts in another tool depends on its own catalogue.

The desktop passes an observed fleet loadout to the same importer used for journal `Loadout` JSON. The importer requires a complete core loadout and checks for duplicate or unknown slots. It retains supported observed engineering values. The desktop stores saved builds in its local `ship-fits.db` and exports the library as individually importable JSON parts when required.

The displayed maximum jump range uses the same empty operating reserve as the game's Loadout event. A jump calculated for current fuel and cargo includes the fuel in the operating reserve. Speed, boost and rotation also include this mass. The jump calculation uses retracted power and combat figures use deployed power.

Run `node --test ../../internal/frontend/ship-fit-engine.test.cjs ../../internal/frontend/ship-fits.test.cjs` here to exercise the generated engine and editor.

The preset library uses [Coriolis share builds by EDAstro](https://edastro.com/exploration/), [Pilots Trade Network](https://pilotstradenetwork.com/ship-builds/anti-xeno/) and [E:D Black Box](https://edblackbox.com/guides/ships/ship-dossiers/panther-clipper-mk-ii-trading.html), alongside original roles for other hulls. The downloaded share URLs used by the generator are pinned in `preset-sources.json`. Presets carry their provenance and the exact share URL when adapted. Starting from an adapted build may mean different equipment or power priorities from the source.

Run `node presets.mjs` from this directory after changing the engine or the fitting recipes. It reads only the local catalogue and pinned share URLs and writes the lazy-loaded `internal/frontend/ship-fit-presets.js`. Run `node presets.mjs --check` and `node --test ../../internal/frontend/ship-fit-presets.test.cjs` to verify the generated dataset and the required equipment for each role.
