# Trade and carrier extension

Authoritative activity is settings.activity = asteroid | surface | raw | trade | carrier.
POST /api/settings persists it. Default asteroid.

Market state

- state.trading = engine.tradingState from MarketSearch
- POST /api/trading/search accepts {mode:sell|buy,system,resourceKey,useCargo,radiusLy,maxAgeHours,carrierMaxAgeHours,minDemand,includeCarriers,minPad,maxArrivalLs,quantity,demandRatio}
- POST /api/trading/cancel
- POST /api/trading/destination {destination:resultRow|null}
- rows {id,system,station,carrier,distanceLy,distanceLs,pad,updatedAt,ageHours,quotes:[{key,name,price,demand,supply,quantity,subtotal,updatedAt,ageHours}],estimatedTotal,coveredTons,missingTons,coverageComplete,warnings,sourceUrl}
- state.trading.destination is selected row, persisted separately from ephemeral search results.
- state.goods contains all observed market/trade commodities plus mining catalog, {key,name,english,kind:'commodity',...}; raw materials excluded.

Trade ledger

state.tradeLedger

- totals {boughtTons,soldTons,purchaseCost,revenue,knownCost,knownRevenue,grossMargin,unknownCostTons,miningRevenue,internalTons,operatingCosts}
- inventory [{key,name,count,knownCostTons,unknownCostTons,unitCost:number|null,purchaseCost,minedTons}]
- goods [{key,name,english}]
- recent [{timestamp,kind:buy|sell|transfer|cost,name,count,value,cost:number|null,margin:number|null,internal,system,station}]
- daily [{date,purchaseCost,revenue,grossMargin,minedTons}]
- scope/timeBasis/help explain tracked purchase cost and no invented unknown cost.
- current goods quotation does not become realized profit.

Route calculator

- POST /api/trading/plan {key,quantity,buyPrice,sellPrice,source,destination}
- state.tradePlan {key,name,quantity,buyPrice,sellPrice,purchaseCost,revenue,grossMargin,source,destination,updatedAt}
- Local planning only, manual quote inputs, saved across restarts.

Carrier

state.carrier from CarrierTracker module, exact shape supplied by carrier agent to frontend.
POST /api/carrier/planning accepts module planning fields, persists for local planning.
POST /api/carrier/stock accepts module manual baseline fields. Local recorded warehouse baseline.
POST /api/carrier/rescan triggers ordinary journal reconstruction.
GET /api/trading/export.csv exports observed trade ledger.

Root will integrate the module into chronological replay and side-file Market/Cargo reconciliation.
Transactions with the user's own carrier are internal transfers and not external sales revenue.
Market timestamps are observations from community data, never relabeled as the instant of search.
