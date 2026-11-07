# Attribution and third-party material

Required Notice: Copyright (c) 2026 Naenier

ED Copilot is an independently implemented, unofficial companion application for Elite Dangerous. It is not affiliated with or endorsed by Frontier Developments. The application code is provided under the PolyForm Noncommercial License 1.0.0 in [LICENSE](LICENSE).

## Elite Dangerous reference data

Elite Dangerous, game identifiers, names, material and commodity labels, engineering recipes, ship specifications and associated game data remain the intellectual property of Frontier Developments plc and are subject to Frontier's terms. Rights to game data are not replaced by the license on application code.

Reference datasets use EDCD/FDevIDs and EDCD/coriolis-data. Their source links, capture dates, known limitations and attribution are retained in:

- [Engineering data notice](lib/ENGINEERING-DATA-NOTICE.txt)
- [Encyclopedia data notice](lib/ENCYCLOPEDIA-DATA-NOTICE.txt)
- The [commodity-name data notice](lib/COMMODITY-DATA-NOTICE.txt)
- `lib/catalog.cjs`, `lib/engineering-data.json` and source links in the encyclopedia

The commodity-name lookup is generated independently from EDCD/FDevIDs' published commodity identifiers and names. Russian labels and explanatory articles were prepared for ED Copilot. Static reference records do not include commander identity, inventory, timestamps or location. No personal player journals are distributed.

References to Inara, EDMC, EDDN, Spansh, NEWP, EDTools, EDHM and other services identify their respective services and factual information sources. Their names and trademarks belong to their owners. The protocol adapters and local EDMC bridge were implemented for ED Copilot; no application source from those services is included unless a separately marked component states otherwise.

## Runtime and interface

Electron and electron-builder are downloaded as pinned development dependencies. Their licenses and bundled third-party notices remain in dependency installations and generated runtime distributions.

The interface uses Windows system fonts, beginning with Segoe UI. No font files are redistributed. The ED Copilot icon is an independently prepared SVG asset with PNG and ICO exports in `web/`.

The window management, atomic file persistence, passive overlay configuration and foreground-process monitor are application implementations using Node.js, Electron and documented Win32 interfaces. The foreground monitor runs a hidden local PowerShell process; no separately supplied native binary is included.

Color import reads supported XML and applied EDHM INI configuration values without executing directives or modifying game files. It translates the observed color settings into application palette roles. It does not distribute EDHM shader or application code.

## Test protocol fixtures

The four official EDDN JSON Schema documents in `test/fixtures/eddn` are distributed under EDDN's BSD 3-Clause license. Their exact upstream license and source notice are retained in that directory. Tests use synthetic observations and mocked network responses.

OAuth behavior is based on Frontier's public documentation and standard PKCE. The setup guide cites the relevant Frontier and EDCD protocol references.

