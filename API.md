# Shared implementation contract

GET /api/state and SSE /api/events ("state" event) return the same JSON.
POST routes accept JSON and return {ok:true} or {ok:false,error}.

State

- app {name:"ED Copilot", version:"1.4.0", profileDir, sources:[{title,url}]}
- connection {journalDir, scanning, filesRead, filesTotal, eventCount, lastEvent, lastUpdate, error, gameRunning:boolean|null}
- commander {name, fid, system, systemAddress, coordinates:[x,y,z]|null, body, ship, vehicle:"ship"|"srv"|"foot"|"unknown", docked, cargoCapacity:number|null, cargoTons:number, cargo:[{key,name,count}], materials:[{key,name,count,grade}]}
- settings {minTier:"medium"|"high"|"low", method:"all"|"laser"|"core"|"subsurface"|"surface"|"surface-planet"|"raw", radiusLy:50, maxArrivalLs:10000, minLaserPercent:15, hideDepleted:true, overlayEnabled:true, hideUnfocused:true}
- system {name, bodiesScanned, rings, surfaceTargets}
- targets [{id, kind:"ring"|"surface", system, systemAddress, body, bodyId, ring, ringType, reserve, tier, score, confidence:"confirmed"|"potential"|"unknown", distanceLy:number|null, distanceLs:number|null, gravity:number|null, landable, hotspots:[{key,name,count,tier}], materials:[{key,name,percent,grade,tier}], resources:[{key,name,tier,methods}], methods:[string], reasons:[string], recommendation, source:"journal"|"edsm", updatedAt}]
- prospect null|{timestamp, system, body, remaining:number|null, content, motherlode, decision:"mine"|"skip"|"unknown", reasons:[string], materials:[{key,name,percent,tier,price:number|null}], estimatedPerTonne:number|null}
- resources [{key,name,english,kind:"commodity"|"raw",tier,grade:number|null,methods:[string],ringTypes:[string],description,tips:[string],sources:[{title,url}],price:number|null,priceSource,priceAt}]
- stats {since, total:{refinedTons,materialUnits,soldTons,revenue,miningRevenue,prospected,accepted,rejected,asteroidsCracked}, byResource:[{key,name,refinedTons,materialUnits,soldTons,revenue}], sessions:[{date,refinedTons,materialUnits,prospected,revenue}], recent:[{timestamp,kind,name,count,value,system}], elapsedSeconds, revenuePerHour:number|null}
- bookmarks [{id,targetId,label,system,body,ring,kind,addedAt,target:target|null}]
- market {station,system,timestamp,pricesCount,error}
- search {running,system,radiusLy,checked,total,status,error,finishedAt}
- notices [{id,timestamp,text,level:"info"|"warn"}]

Routes

- POST /api/settings partial settings object
- POST /api/search {system,radiusLy}; POST /api/search/cancel
- POST /api/rescan
- POST /api/bookmarks {action:"add",targetId} or {action:"remove",id}
- POST /api/session {action:"reset"}
- POST /api/market {prices:[{key,price}]} manual reference prices (optional)
- POST /api/journal-dir {path}
- GET /api/export.csv statistics CSV in Russian, UTF8 BOM

window.edMining native bridge

openOverlay(), toggleOverlay(), showMain(), chooseJournalDir() -> path|null, quit(),
copyText(text), getOverlayState(), onOverlayState(cb).
chooseJournalDir returns a path only. UI persists through POST /api/journal-dir.

Backend startServer({dataDir,journalDir,port,host}) -> {url,close(),engine}.
engine.getState() synchronous, engine.subscribe(callback) returns unsubscribe.
engine.setDirectory(path) async when used by native code.

All raw journal IDs remain untranslated. RU display names are lookup-only.
No price means unknown sale value. Ring content and hotspot signals distinguish potential from confirmed.

Optional integration routes (JSON POST, same-origin loopback only)

- /api/integrations/configure {publisher:"off"|"copilot"|"edmc", inaraEnabled?:boolean, eddnEnabled?:boolean, inaraAppRegistered?:boolean, inaraApiKey?:string, clearInaraKey?:boolean}. Binds to the current journal FID. Secrets and pending uploads are encrypted through native secureStorage callbacks; browser-only servers cannot enable uploads.
- /api/integrations/sync {}. Queues only authoritative current Inara snapshots and attempts eligible pending uploads. Retry deadlines still apply. Does not upload journal history.
- /api/integrations/pair {} returns {ok,token,url}. Explicitly rotates the EDMC bridge token; it is never included in /api/state or SSE.
- /api/integrations/edmc receives {type:"heartbeat"|"journal"|"capi", commander,isBeta,pluginVersion,observedAt,entry?|data?} with X-EDCopilot-Token. Requires publisher=edmc, a matching local token and loopback socket, and rejects browser Origin headers. It updates connection status. Fresh CAPI market prices may update the local market only when current commander, system, station and MarketID agree. EDMC data is never republished by Copilot in this mode.

State includes integrations with service counters, last success/error, pending counts, next retry, current/bound identity and Live gate. It never contains API keys or the pairing token. External publishing is opt-in and disabled by default. Exactly one publisher should be enabled across Copilot and EDMC. Inara additionally requires app-name approval and the commander's own API key. EDDN accepts public observations only; personal inventory, balance and missions are excluded. Both uploads reject beta/Legacy sessions. Replay and persisted journal byte cursors prevent historical resends. Network failures remain queued with backoff; permanent validation failures are reported without infinite retries.
