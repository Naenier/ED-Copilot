# ED Copilot 2.9.0

## Ship builder

- The editor shows the selected hull's diagram, filters slots by module group and keeps the module editor in view on wide layouts.
- **Load from fleet** opens a ship from the current commander's observed fleet. Switch to a stored ship in-game to record its loadout if the available observations contain only its identity and location.
- **Materials** opens three separate tables showing quantities needed, in stock and missing for raw, manufactured and encoded materials.
- Range, shield and weapon calculations account for power availability. The editor checks equipment limits and keeps the recorded module variants and engineering modifiers when loading a fit. The G3 materials estimate at 67% counts two applications.
- SLEF import and export preserve supported engineering values. Large libraries export in numbered files that can be imported independently.

## Search nearby

- Review stations, outfitting, minor factions, star systems, bodies, signals and other destinations with filters specific to each category.
- Searches accept name fragments, honour zero arrival distance and apply the selected radius to engineers. Carrier exclusion also covers stronghold carriers. Dockable megaships require a recorded docking service.
- Search outfitting by exact module class and mount. Body results include temperature, volcanism, landability and tidal locking where known. Search results draw on indexed observations and include notices for bounded queries.

## Settings and names

- **Settings** shows the installed version. **Application log** filters local diagnostics by component, level and text. Four weeks of logs are retained, up to 32 MiB.
- Russian in-game names are used for ship engineering materials and Odyssey resources. Existing reference searches and links continue to recognise older names.
- Updated the English and Russian README galleries using the application's synthetic demonstration profile.
