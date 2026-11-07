# Inara protocol adapter

The implementation follows the [official Inara API documentation](https://inara.cz/elite/inara-api/) and the [Frontier journal reference](https://hosting.zaonce.net/community/journal/v37/Journal_Manual_v37.pdf).

The application uses its own name, ED Copilot. A personal API key stays in Windows-protected local storage and is never included in renderer status, exported data or source control. The API response determines whether a submission succeeds.

## Local contract

The configuration endpoint accepts enabled and an optional apiKey. An empty key retains the saved value. Disabling stops sending while retaining queued observations. Status exposes counters, last successful submission, errors and retry time without credentials.

The version 2 encrypted state imports only Inara configuration and observations from version 1, preserving the key, commander binding, journal cursors and acknowledged-event hashes.

## Delivery

Only current Live-game events for the bound commander qualify. Journal replay does not upload history. Snapshots are coalesced, events are deduplicated, batches are limited and network failures use service-wide backoff. Switching commanders or disabling cancels an outstanding batch without acknowledging it for the wrong account.

Cargo snapshots use the raw ship inventory. An unknown inventory or loan is never replaced by a fabricated zero. Credits, ships/loadouts, cargo, materials, ranks, reputation, missions and supported travel events use the documented event types.

Fleet-carrier travel uses addCommanderTravelCarrierJump. Private carrier ownership, fuel, accounts and warehouse management remain local; the commander profile API does not represent them.
