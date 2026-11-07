# Inara integration

Protocol sources, checked 7 November 2026:

- [Official developer guide](https://inara.cz/elite/inara-api-devguide/)
- [Official v1 event reference](https://inara.cz/elite/inara-api-docs/)
- [EDMC integration source](https://github.com/EDCD/EDMarketConnector/blob/main/plugins/inara.py), consulted for journal field correspondence.

ED Copilot uses its own application name. Inara must approve that name before uploads work. Users provide their personal Inara API key. A Frontier OAuth Client ID does not replace either requirement.

Uploads go to `https://inara.cz/inapi/v1/`. The header contains `appName`, `appVersion`, `isBeingDeveloped`, `APIkey`, `commanderName` and, when available, `commanderFrontierID`. Events contain `eventName`, the original `eventTimestamp`, `eventData` and a numeric `eventCustomID` for acknowledgement matching.

Inara codes 200 and 202 acknowledge an event. Codes 204 and 400 do not. Missing event acknowledgements remain pending. A header-level 400 pauses pending events for setup correction. Retryable network/server failures use increasing delays; they never trigger an immediate retry loop.

## Adapter and manager

`inara.cjs` exports pure journal/snapshot mapping, `validateLive`, stable `eventKey`, replaceable `snapshotKey`, and one-attempt `sendBatch`. `integrations.cjs` owns encrypted persistence, commander binding, replay cursors, batching and retries. Renderer status never contains a key or bridge token. Only the explicit pairing response returns a new EDMC token.

The manager accepts `secureStorage: {encrypt, decrypt}` callbacks compatible with Electron safeStorage wrappers. Without working encryption it refuses publishing. The binary profile file contains configuration, credentials, cursors and pending observations; do not publish it.

Only Live data qualifies. Journal replay and observations preceding opt-in do not generate history uploads. Commander changes cancel outstanding batches. EDMC and ED Copilot are mutually exclusive publishers; the local EDMC bridge displays connection status and never republishes its messages.

Inventory replacement requires complete data. Ship cargo uses the original full Inventory, including stolen and mission partitions; summary cargo is unsuitable. Materials require all three known categories. Manual engineer checkboxes are excluded. Credits require an authoritative observation and known loan value. Game statistics use the complete original Statistics payload.

Fleet-carrier travel is supported by `addCommanderTravelCarrierJump`. The documented API has no carrier ownership, fuel, balance, warehouse, station-market import or galaxy-price lookup event. Those must not be disguised as commander ship inventory. ED Copilot keeps private carrier management locally and submits supported public market observations through EDDN separately.

Tests use synthetic commanders and mocked transports only:

```text
node work/test-inara.cjs
node work/test-integration-manager.cjs
```
