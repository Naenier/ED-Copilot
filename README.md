<p align="center"><img src="docs/assets/copilot-mark.png" width="76" alt="ED Copilot logo"></p>

# ED Copilot

**English** · [Русский](README.ru.md)

A desktop companion for Elite Dangerous. Look up star systems, plan engineering and colonisation, compare trading and mining opportunities, manage fleet carrier routes, and track exobiology.

**[Download for Windows](https://github.com/Naenier/ED-Copilot/releases/latest)** · [User guide (RU)](docs/USER-GUIDE.md) · [Installation and updates (RU)](docs/INSTALLATION.md)

![Commander overview in ED Copilot](docs/screenshots/en/overview.png)

*Screenshots use a demonstration profile. Commander data, balances, market quotes, and the sample mapping route are fictional.*

## Getting started

1. Install the **Windows Setup** from the latest release, or extract the portable archive and run `ED Copilot.exe`.
2. Start Elite Dangerous. Copilot reads the game journals; choose their folder in Settings if it is not detected automatically.
3. Choose a workspace using the icons on the far left, then open a tool from its sidebar. Use **View → Customise menu** to change the visible sections in Workspace, Fleet carriers, and Library.
4. Select an activity for the overlay and start working through your plan.

Windows 10/11 x64 is supported. The application window requires Microsoft Edge WebView2 Runtime; the installer offers to install it if needed. You do not need Go, Python or Node.js. You can choose the installation folder, including `C:\Games\ED Copilot`.

## Workspaces and your commander

The left rail groups tools into Workspace, Trading, Colonisation, Mining, Engineering, Exobiology, Fleet carriers, Galaxy, and Library. Collapse the sidebar to free up space while keeping the group icons available. **Settings** is at the bottom of the rail. **Ctrl+K** finds a tool across all groups.

Your avatar opens **Commander**, with pages for Summary, Ranks, Finances, Inventory, Assets, Fleet, Statistics, and Journal. Superpower reputation is part of Summary. The inventory distinguishes unknown stock from confirmed empty stock.

![Commander summary and grouped navigation](docs/screenshots/en/commander.png)

## Star system search

Open **Galaxy → Systems** to find a system by name or browse systems within 100 ly of a chosen reference system. Filter nearby results by population, allegiance, government, economy, and security. System pages load coordinates, permits, celestial bodies and materials, stations, and faction influence from EDSM. When nearby search falls back to Spansh, the results identify the provider and include up to 50 of the nearest systems.

Star system fields offer up to twenty suggestions from Spansh and EDSM. Type three characters and select a name with the mouse or ↑/↓ and Enter. In multiline route stops, selecting a suggestion replaces only the current line. You can also enter a name manually.

![Galaxy system search with nearby results and filters](docs/screenshots/en/galaxy.png)

![System page showing celestial bodies and surface materials](docs/screenshots/en/system.png)

## Road to Riches

Open **Galaxy → Road to Riches**, then enter your starting system, jump range and search radius. Set a destination or leave it blank to enable a return loop. Limit the number of systems and the distance from arrival to each planet, choose whether to rank targets by FSS or DSS reward, and skip known Thargoid systems. The minimum DSS reward defaults to **1,000,000 Cr**. Enter `0` to remove the minimum.

The results list water worlds and Earth-like worlds with their arrival distances and estimated FSS and DSS rewards. Use the signal diagrams to identify both planet types in the FSS. View the route diagram, mark completed planets or export the route as CSV or JSON.

Copilot copies the next system when the route is ready and after you arrive at each stop. Paste it into the in-game galaxy map with **Ctrl+V**. Overview and the flight overlay show your mapping target, next system, estimated payout and jump progress.

![Road to Riches search filters, FSS signals and mapping targets](docs/screenshots/en/road-to-riches.png)

## Exobiology

Choose planets by their biological signals, environmental conditions and the value of potential species. Track three genetic samples, the required separation and your distance from recorded points when live coordinates are available. A dedicated overlay activity supports surface exploration, while sample and sales screens distinguish data on board from confirmed Vista Genomics sales.

![Exobiology workspace](docs/screenshots/en/exobiology.png)

<details>
<summary>Planet search and sample tracking</summary>

![Search for survey planets by system or body name](docs/screenshots/en/exo-planets.png)

![Sample progress and data ready to sell](docs/screenshots/en/exo-samples.png)

</details>

The reference catalogue includes 126 species across 25 genera, habitat conditions, variants and 101 photographs. Planet bookmarks, surface waypoints, a public observation index and expedition statistics help organise your research. First footfall status, estimated payouts and unknown coordinates are identified separately. [Exobiology guide (RU)](docs/EXOBIOLOGY.md).

## Trading and mining

Find stations and fleet carriers where you can buy or sell a commodity. Compare price per tonne, demand, supply, landing pad size, distance and the age of each observation. Search for trade routes that fit your cargo hold and budget, keep a destination in the overlay and track your transactions.

Mining tools help you find valuable rings and planets, filter suitable targets, inspect prospected asteroids, check equipment and review mining sessions. Engineering materials and commodities for sale are tracked separately.

## Trade routes: from search to a usable plan

The route screen presents both stations on a wide card, including their systems, landing pads, arrival distances, buy and sell prices, supply, demand and observation times. A return load on an A → B → A route has its own commodity and calculation. Copy each system or station name independently, then paste it into the game.

Search takes available cargo space, your known credit balance, market volume, costs and personal limits into account. By default it compares all goods on the markets it checks. Enable **Filter by commodity** to pick specific goods, or set a minimum net profit for the complete trip, including any return load. Profit per hour uses the journey time you enter for one direction. Travel to the first station is excluded.

![A two-leg trade route](docs/screenshots/en/deals.png)

![Trade route search with the optional commodity picker and minimum net profit](docs/screenshots/en/deals-filters.png)

Select a route, set its buyer as the overlay destination or save a plan for a particular leg. Keep up to 40 complete routes, including both market quotes, in Saved. These are observation snapshots: run a new search before repeating a route to check current prices and available volume.

## Fleet carrier trading: the whole journey in one calculation

Find fully loaded runs with at least 400 million credits in estimated net profit. Copilot compares fresh regional markets and Rackham’s Peak, and can combine purchases across stations and systems. Each result includes separate estimates for jumping, loading, unloading and preparing tritium.

![Fleet carrier trading plan](docs/screenshots/en/carrier-trade.png)

Rackham’s Peak has a medium landing pad. The calculation uses a fresh market quote or a price you enter; confirm Public Holiday in the game.

A separate calculator compares the purchase and running costs of personal and squadron fleet carriers.

## Personal fleet carrier routes

Calculate jumps, fuel consumption, refuelling, the reserve on arrival and journey time. When your fuel stock is known, the form starts with that amount. Minimum starting fuel is a separate option for planning the required stock. Fuel cost remains unknown until you provide a tritium price.

![Personal fleet carrier route](docs/screenshots/en/route.png)

After a confirmed arrival, the next system can be copied to the clipboard automatically: press **Ctrl+V** in the galaxy map to paste it. Schedule jumps in the game.

## A reference library with complete entity pages

Browse by category, search by name and narrow results by group. Ships and engineers have illustrated catalogue cards; each entry opens a full page with properties, requirements and related entries.

The catalogue cards show top-view schematics. All 49 ship pages pair a Frontier screenshot or render with top and bottom schematics. [Ship artwork credits](docs/SHIP-ARTWORK.md)

Search for **Solo Thargoid hunting** for an illustrated AX guide with heart targeting, copyable hunting systems, ship fits, Guardian unlocks and ammunition synthesis.

![Ship catalogue](docs/screenshots/en/ships.png)

![Complete ship page](docs/screenshots/en/ship.png)

Engineer pages show invitation and unlock requirements, workshop locations, and supported upgrades. Module pages link to compatible blueprints and experimental effects. Material pages show known inventory, how to collect the material, trade ratios, and recipes that use it.

![Engineer page](docs/screenshots/en/engineer.png)

Read articles in a spacious layout with a table of contents. Commodity pages offer buy and sell views, show market offers and open trading with the commodity already selected.

![Reference article](docs/screenshots/en/article.png)

Use **Material farming** to find routes for collecting raw materials, manufactured components, encoded data, and Guardian components.

![Material farming articles in the reference catalogue](docs/screenshots/en/farming-catalog.png)

## Colonies

Manage multiple colonies independently. Choose a system using your preferred starting faction, plan the construction order and supply queue, then compare bills of materials with accepted deliveries. The overlay shows the next task for the selected colony.

Start with an origin system and its stations, then select the station and controlling faction. The results include systems that pass the available eligibility checks. **Supply log** records accepted deliveries, and **Construction** shows the selected project's materials directly below the build queue.

Enable **Include carrier warehouse** to include known personal carrier cargo. Enable **Squadron cargo is available to me** if you can use that stock. These settings are shared, while supply requirements and queues belong to each colony. The selected plan shows the sources separately with their callsigns and observation times. After sign-in, Frontier observations refresh automatically in the background. Stock aboard a carrier remains separate from cargo on your ship and accepted construction deliveries; it is not reserved between colony plans.

The **Budget** view values every commodity at its latest observed galactic average price, with full, remaining and procurement totals for all planned and active construction in the selected colony. Missing bills and prices remain explicit. The overlay stays focused on logistics: up to ten outstanding commodities, excluding requirements already covered by fresh accessible carrier stock when carrier accounting is enabled. Fully covered commodities remain visible in the application.

![Colonisation plan](docs/screenshots/en/colonization.png)

## Engineering

Use **Engineering → Progression** to follow unlock steps and filter engineers by module, blueprint, or experimental effect. You can start with a blueprint, then pick a compatible module to calculate its requirements. The plan links requirements to their reference pages and compares recipe costs with known inventory. **Engineering → Plan** combines requirements for multiple upgrades.

![Engineering progression and links to upgrade requirements](docs/screenshots/en/engineering.png)

Open **Engineering → Materials** to review stock or select **Calculate material trade**. The calculator compares exchange options for the selected material and shows which ones your available stock can cover.

![Materials inventory and exchange options](docs/screenshots/en/material-exchange.png)

![Material reference with collection sources and trade guidance](docs/screenshots/en/material.png)

## Commander ranks and galactic politics

Track all eight in-game rank disciplines with individual progress bars. Open a rank for advancement methods and a ladder of practical unlocks, including rank-gated ships, permits and discounts. Journal observations and promotions are dated separately; unknown progress remains unknown.

Career cards use the emblem for your observed rank, while naval cards use their faction’s crest. Unknown ranks have a neutral symbol.

![Commander ranks and practical unlocks](docs/screenshots/en/ranks.png)

Politics brings together public community goals, your recorded contribution, Powerplay powers and indexed systems, local faction influence and conflicts, and reported Thargoid states. Open observations refresh automatically, and the Powerplay table stays visible while a new page loads. Search for a system, inspect observation dates, or copy a destination into the galaxy map. Cached observations remain available offline.

![System politics with ruling faction, power control and local influence](docs/screenshots/en/politics.png)

## Academy

Nine courses and 27 lessons cover trading, group operations, getting started with Odyssey, fleet carriers and passenger runs. Follow practical Copilot workflows, worked fictional examples, decision checklists and recovery steps. All lessons are available in English and Russian, with saved answers, favourites and reading positions.

![ED Copilot Academy](docs/screenshots/en/academy.png)

## In-game guidance and controls

The overlay displays panels for the selected activity only while Elite Dangerous is focused. It adapts to ship, SRV/Nomad and on-foot play; switching to Copilot hides the game panels.

Compact panels fill the right side first, then continue on the left. In a ship or SRV, they stay above the lower cockpit instruments. The overlay starts with useful task information, without a permanent product or activity header. Location appears briefly within a relevant panel when needed. Empty plans and repeated information are hidden; an idle activity leaves the game clear. Your colonies are recovered from journal claims and become available in the overlay selector.

| Shortcut | Action |
| --- | --- |
| **Ctrl+K** | Jump to a section, including with a Russian keyboard layout. |
| **F11** | Enter or leave full screen. |
| **Ctrl+Alt+H** | Toggle the overlay. |

Customise the section order, window mode and colours. Import the overlay palette from the game and preview it before applying.

## Interface language

The application starts in **English** on first launch. Choose English or Russian in Settings; your choice is retained between sessions. System names, station names and personal notes are preserved. Academy lessons are available in both languages.

This README is also available [in Russian](README.ru.md).

## Connections and updates

In **Settings → Connections**, sign in to Frontier to refresh supported account information in the background. Under **Inara**, add your personal API key and enable event sending.

Automatic updates are enabled by default and can be switched off in Settings. Stable GitHub releases download quietly and are checked against published SHA256 digests. Installation waits until you fully exit Copilot; the next launch shows the changes. Setup provides separate Start Menu and desktop shortcut choices and preserves them during updates.

Plans, settings and progress are stored in `%LOCALAPPDATA%\ED Copilot`, separately from the application. Before updating, fully exit through the File menu or the system tray icon. The installer preserves your personal profile.

Embedded reference data, the Academy and local plans work offline. New market searches, system suggestions, route planning and synchronisation require an internet connection. A dash means a value is unknown; check quote timestamps before travelling. Purchases, orders, construction and jump scheduling take place in the game.

[Full user guide (RU)](docs/USER-GUIDE.md) · [Installation (RU)](docs/INSTALLATION.md) · [What's new in 2.8.7](docs/RELEASE-2.8.7.md) · [Build and delivery](docs/DELIVERY.md)
