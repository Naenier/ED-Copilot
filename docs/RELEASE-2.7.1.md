# ED Copilot 2.7.1

## Polished tooltips

- Replaced native Windows hints with rounded, dark tooltips that match the application, including a subtle border and shadow.
- Tooltips appear beside navigation icons and below other controls, stay inside the window and work with keyboard focus. Escape dismisses the hint before closing a surrounding dialog.
- Background refresh preserves unchanged toolbar controls and their focus. Hints disappear when their control is removed, the page scrolls or the window loses focus.

## Automatic Frontier updates

- Frontier data refreshes in the background after sign-in and when a saved account starts. Profile and carrier requests respect Frontier's individual cooldowns.
- Commander observations, station information and verified carrier cargo update automatically. Removed the manual refresh and carrier-import buttons.
- Temporary failures preserve the last observations and retry with backoff. Logout, identity changes and application shutdown cancel pending work.

## Colony supply overlay

- Supply rows distinguish remaining construction deliveries, cargo aboard the ship and available carrier stock. A covered requirement is marked clearly without treating warehouse cargo as a completed delivery.
- The compact list prioritizes loaded cargo, a carrier-covered requirement and procurement needs. Its footer shows covered requirements and how many additional commodities are outside the preview.
- Physical stock remains visible when warehouse procurement accounting is off, with a short notice. Unread, stale and inaccessible squadron stock are handled separately from a confirmed zero.
