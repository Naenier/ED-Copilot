# Ship fitting bundle

From this directory, run `npm ci` and `node build.mjs`. Node downloads the pinned Coriolis, coriolis-data and Elite Dangerous Almanac source revisions using Git and regenerates `internal/frontend/ship-fit-engine.js` and the accompanying notices. The native app embeds the generated engine. Browsing and editing a fit work offline.

Coriolis supplies the calculation engine, its compact module encoding and the foundation of the catalogue. Almanac supplies corrected numerical hull attributes, journal slot keys and facts for hulls and modules released after the pinned Coriolis data. EDSY measurements supplement the newer hulls' boost parameters and the zero-cost Mk II hangar variants. License and data notices are embedded in the bundle. The application owns the interface and database.

An ED Copilot build stores the compact code, model, title and optional load scenario (fuel, cargo and distributor allocation) separately. Native export preserves those settings. Coriolis share links and Detailed JSON also carry the scenario, while Coriolis backups import each named fit. SLEF and Loadout use the station and game module symbols. A build with a supplemental hull or module can still be exchanged in native JSON and SLEF. Availability of those parts in another tool depends on its own catalogue.

The displayed maximum jump range uses the same empty operating reserve as the game's Loadout event. A jump calculated for current fuel and cargo includes the fuel in the operating reserve. Speed, boost and rotation also include this mass.

Run `node --test ../../internal/frontend/ship-fit-engine.test.cjs` here to exercise the generated engine.
