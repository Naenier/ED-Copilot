# Attribution and third-party material

ED Copilot is an unofficial companion application for Elite Dangerous. It is not affiliated with or endorsed by Frontier Developments.

## EDEXO 1.3.7

Required Notice: Copyright (c) 2026 bahuckel (https://bahuckel.com/projects/edexo-compare)

The Electron window/foreground helper modules and the Rust foreground watcher originated in EDEXO 1.3.7; some were adapted for ED Copilot. The EDEXO interface fonts under `web/fonts` also come from that project. The font metadata identifies Bahuckel as the author and explicitly licenses the font under the same PolyForm Noncommercial 1.0.0 terms and additional permissions as EDEXO.

The exact upstream EDEXO license, including its additional permissions and required copyright notice, is retained in [LICENSE](LICENSE). EDEXO versions 1.3.0 and later are not MIT-licensed. Earlier MIT releases do not change the terms of the 1.3.7 material used here.

EDEXO's biology photo collection and EDAstro-derived biology tables are not included in this source repository. The upstream license refers to those assets to explain their exclusions; their presence in that license is not a claim that this repository distributes them.

## Elite Dangerous reference data

Elite Dangerous, game identifiers, names, material and commodity labels, engineering recipes, ship specifications and associated game data remain the intellectual property of Frontier Developments plc and are subject to Frontier's terms. Rights to game data are not replaced by the license on application code.

Reference datasets use EDCD/FDevIDs and EDCD/coriolis-data. Their source links, capture dates, known limitations and attribution are retained in:

- [Engineering data notice](lib/ENGINEERING-DATA-NOTICE.txt)
- [Encyclopedia data notice](lib/ENCYCLOPEDIA-DATA-NOTICE.txt)
- `lib/catalog.cjs`, `lib/engineering-data.json` and the source links in the encyclopedia UI.

The commodity-name lookup was adapted from EDEXO's identifier/name mapping; the Russian labels and original explanatory articles were prepared for ED Copilot. A static ID/name pair does not include player identity, inventory, timestamps, location or any other player-journal record. No personal player journals are included.

References to Inara, EDMC, EDDN, Spansh, NEWP, EDTools and other services identify their respective services and factual information sources. Their names and trademarks belong to their owners. This repository does not claim rights to those services or include their application code unless a separately marked component states otherwise.

## Runtime dependencies

The four official EDDN JSON Schema documents in `test/fixtures/eddn` are distributed under EDDN's BSD 3-Clause license. Their exact upstream license and source notice are retained in that directory. They support offline tests with synthetic observations and contain no player data.

Electron and electron-builder are downloaded as declared development dependencies. Their third-party notices remain in the dependencies and generated runtime distributions; they are not checked into this source repository. The native foreground watcher has no external Rust crate dependencies.

The ED Copilot icon is an independently prepared SVG asset, with PNG and ICO exports in `web/`.
