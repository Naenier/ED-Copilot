# Shared implementation contract

GET /api/state and SSE /api/events ("state" event) return the same JSON.
POST routes accept JSON and return {ok:true} or {ok:false,error}.

State

- app {name:"ED Copilot", version:"1.8.0", profileDir, sources:[{title,url}]}
- connection {journalDir, scanning, filesRead, filesTotal, eventCount, lastEvent, lastUpdate, error, gameRunning:boolean|null}
- commander {name, fid, system, systemAddress, coordinates:[x,y,z]|null, body, ship, vehicle:"ship"|"srv"|"foot"|"unknown", docked, cargoCapacity:number|null, cargoTons:number, cargo:[{key,name,count}], materials:[{key,name,count,grade}]}
- settings {minTier:"medium"|"high"|"low", method:"all"|"laser"|"core"|"subsurface"|"surface"|"surface-planet"|"raw", radiusLy:50, maxArrivalLs:10000, minLaserPercent:15, hideDepleted:true, overlayEnabled:true}
- navigation {tabs:[string]} ordered list of open feature tabs; default ["overview","targets","sale","encyclopedia","statistics","settings"]. Overview and settings remain available.
- system {name, bodiesScanned, rings, surfaceTargets}
- targets [{id, kind:"ring"|"surface", system, systemAddress, body, bodyId, ring, ringType, reserve, tier, score, confidence:"confirmed"|"potential"|"unknown", distanceLy:number|null, distanceLs:number|null, gravity:number|null, landable, hotspots:[{key,name,count,tier}], materials:[{key,name,percent,grade,tier}], resources:[{key,name,tier,methods}], methods:[string], reasons:[string], recommendation, source:"journal"|"edsm", updatedAt}]
- prospect null|{timestamp, system, body, remaining:number|null, content, motherlode, decision:"mine"|"skip"|"unknown", reasons:[string], materials:[{key,name,percent,tier,price:number|null}], estimatedPerTonne:number|null}
- resources [{key,name,english,kind:"commodity"|"raw",tier,grade:number|null,methods:[string],ringTypes:[string],description,tips:[string],sources:[{title,url}],price:number|null,priceSource,priceAt,manualPrice:number|null,automaticPrice:number|null,automaticPriceSource,automaticPriceAt}]
- stats {since, total:{refinedTons,materialUnits,soldTons,revenue,miningRevenue,prospected,accepted,rejected,asteroidsCracked}, byResource:[{key,name,refinedTons,materialUnits,soldTons,revenue}], sessions:[{date,refinedTons,materialUnits,prospected,revenue}], recent:[{timestamp,kind,name,count,value,system}], elapsedSeconds, revenuePerHour:number|null}
- bookmarks [{id,targetId,label,system,body,ring,kind,addedAt,target:target|null}]
- market {station,system,timestamp,pricesCount,error}
- search {running,system,radiusLy,checked,total,status,error,finishedAt}
- notices [{id,timestamp,text,level:"info"|"warn"}]

Routes

- POST /api/settings partial settings object
- POST /api/navigation {tabs:[string]} persists the ordered feature list and emits state. Valid IDs: overview, targets, sale, deals, carrier, colonization, encyclopedia, statistics, mining, engineering, route, settings, account, integrations, help. Unknown IDs/non-array payloads are rejected; duplicates are removed. Missing overview/settings are inserted. Closing a tab changes only this navigation list, not feature data or availability.
- POST /api/search {system,radiusLy}; POST /api/search/cancel
- POST /api/rescan
- POST /api/bookmarks {action:"add",targetId} or {action:"remove",id}
- POST /api/session {action:"reset"}
- POST /api/market {prices:[{key,price}]} optional per-tonne reference prices for local cargo/asteroid estimates. Positive finite values up to 1,000,000,000 set an override; null or "" clears it. An empty override falls back to the most recent available in-game market observation; without either value the estimate is unknown. Market-search quotes, demand/supply and historical transaction amounts use their own source observations.
- POST /api/journal-dir {path}
- GET /api/export.csv statistics CSV in Russian, UTF8 BOM

window.edMining native bridge

openOverlay(), toggleOverlay(), showMain(), chooseJournalDir() -> path|null, quit(),
copyText(text), getOverlayState(), onOverlayState(cb).
chooseJournalDir returns a path only. UI persists through POST /api/journal-dir.

Native display-mode IPC (desktop preload only; no HTTP display-control route):

- getDisplayState() -> {mode:"windowed"|"fullscreen"|"borderless",changing:boolean,pendingMode:string|null,fullscreen:boolean,borderless:boolean,bounds:rect|null,windowedBounds:rect|null,error:string|null}.
- setDisplayMode(mode) applies one of the three modes and returns the resulting display state. Invalid modes are rejected. The main-process handler owns native window changes and persists the mode plus ordinary-window geometry independently of navigation tabs.
- onDisplayState(callback) subscribes to display state changes and returns an unsubscribe function. minimize() minimizes the main window; close() hides it in the tray; quit() exits the app.

Windowed mode uses a normal Windows frame. Fullscreen uses the display's complete bounds. Borderless is a frameless window filling the display work area, leaving the taskbar area available. F11 switches windowed/fullscreen and returns borderless to windowed; Escape returns fullscreen or borderless to windowed. Native mode controls are unavailable in a plain browser. Overlay visibility and geometry remain independent of the main display mode.

Backend startServer({dataDir,journalDir,port,host}) -> {url,close(),engine}.
engine.getState() synchronous, engine.subscribe(callback) returns unsubscribe.
engine.setDirectory(path) async when used by native code.

All raw journal IDs remain untranslated. RU display names are lookup-only.
No price means unknown sale value. Ring content and hotspot signals distinguish potential from confirmed.

Inara integration (opt-in, same-origin loopback JSON POST)

- /api/integrations/configure {enabled:boolean, apiKey?:string}. Empty key retains the saved key. Credentials and the retry queue use native encrypted Windows storage.
- /api/integrations/sync {} attempts eligible pending events and authoritative current snapshots. Historical journal replay is not published.
- State integrations contains status, queue counters and errors, without the API key. Live-only checks, commander identity binding, saved journal cursors and retry deadlines prevent unsafe resends.

Colonization

- POST /api/colonization action commands manage projects, ordered buildings, planned bills and manual delivery notes.
- POST /api/colonization/search {originSystem,faction?} starts a cancellable background search. POST /api/colonization/cancel {} stops it.
- State colonization contains the selected project, ordered build plan, candidate pages, observed depots, requirements and delivery totals.
- Journal depot requirements and accepted deliveries are authoritative observations. Planned quantities/manual notes and onboard cargo remain separate. Public candidate data does not prove that a claim can currently be purchased.

Appearance routes and state

- State theme:{settings,palette,presets}. Settings support mode preset/custom/matrix/game, preset blue/orange/green/purple, eight #RRGGBB roles, 3x3 coefficients from -2 to 2 and overlayOpacity from 0.4 to 1. Palette contains derived colors for all window/overlay CSS variables. Source metadata is local runtime data only.
- POST /api/theme/preview {settings} validates and returns {ok,theme} without saving or emitting state.
- POST /api/theme/import {} reads the connected game EDHM configuration or standard graphics XML and returns a preview theme. Optional filePath must name a supported configuration file. No game files are changed and no imported INI code is executed.
- POST /api/theme {settings} persists the selection in the local appearance preferences and emits state for all windows/overlays.
