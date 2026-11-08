# ED Copilot 2.7.2

## Colony logistics overlay

- Shows up to ten outstanding commodities, with a count of additional items. Compact supply sections can flow into the left safe band while keeping cockpit instruments clear.
- When carrier stock accounting is enabled, requirements already fully covered by fresh accessible carrier cargo disappear from the overlay. They remain in the application. Cargo aboard a ship still requiring delivery remains actionable.
- Procurement and delivery actions stay separate from construction completion. Unknown, stale or inaccessible stock does not hide a requirement.

## Colony resource budget

- Added a dedicated Budget view, galactic average prices and commodity values in colony supplies.
- The selected colony has separate totals for the full resource bill of planned and active construction, resources already delivered, outstanding deliveries and remaining procurement after available stock.
- Estimates use game-reported galactic averages, not local station offers or actual expenditure. Missing prices and unknown bills remain explicit, with partial totals identified clearly.

## Carrier cargo updates

- Local cargo transfers update the ship, carrier and colony views immediately, including journal events that identify the carrier through the docking context.
- A lagging Frontier response no longer overwrites locally corrected carrier quantities. Matching observations confirm the changes without counting them twice.
- Pending reconciliation survives a restart. Conflicting provider observations do not extend a stock snapshot's freshness period. Background requests continue to respect Frontier's cooldowns.
