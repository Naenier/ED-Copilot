# Political observations

Verified against the providers on 2026-11-08. The implementation uses explicit
public JSON requests and game journal records. It does not scrape Inara or use
commander credentials.

- Frontier website feed: `GET https://api.orerve.net/2.0/website/initiatives/list?lang=en`
  with `Accept: application/json`. Its `activeInitiatives` records supply public
  titles, objectives, destination, progress, target, expiry and bulletin/reward
  text. This is distinct from authenticated CAPI `/communitygoals`. Primary
  service: <https://www.elitedangerous.com/community/goals>.
- Spansh website APIs, verified from the provider's own system-search application:
  `GET /api/systems/field_values/system_names?q=...` resolves an exact system to
  its id64; `GET /api/system/{id64}` supplies dated faction, power and Thargoid
  observations. The record's `updated_at` is retained; retrieval time does not
  become observation time. Bodies and stations are discarded.
- `GET /api/systems/field_values/controlling_power` and `/power_state` supply
  available values and indexed counts, not a census of the current galaxy.
  `POST /api/systems/search` uses provider filters, alphabetical sort, page
  and size 10. A `Contested` filter is accepted even when it has no indexed
  matches. Multiple names in `power` do not establish a contested state.
  Provider: <https://spansh.co.uk/systems>.
- `GET /api/systems/field_values/thargoid_war_state` supplies indexed state values.
  When only `None` exists, the interface describes that bounded index result,
  without asserting the galaxy has no Thargoids. DCoH's primary homepage now
  states that Overwatch is decommissioned and its API returns HTTP 410:
  <https://dcoh.watch/>. Its suggested future reporting site is
  <https://navbeacon.app/>; no unverified current API is assumed.
- Journal `FSDJump`, `Location` and `CarrierJump` supply the actually present
  faction/Powerplay/Thargoid fields. Journal `Conflicts` is the source for exact
  opponents, stakes and days won; matching faction states do not establish a
  pairing. `CommunityGoal` and reward/discard events supply personal progress.
  Frontier manual:
  <https://hosting.zaonce.net/community/journal/v37/Journal_Manual_v37.pdf>.

Requests have a 25-second context deadline, a 4 MiB response limit and no followed
redirects. Successful duplicate requests have a five-minute cache, failed
requests a one-minute backoff, one request runs at a time, and system history
holds at most 40 normalized records. Public failures retain successful dated
snapshots. Offline rendering, journal application and snapshots never fetch.
