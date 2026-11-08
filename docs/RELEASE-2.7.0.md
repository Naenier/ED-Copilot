# ED Copilot 2.7.0

## Commander workspace and grouped navigation

- The narrow navigation rail selects a workspace. Its collapsible sidebar contains that workspace's sections, including optional sections in a separate disclosure. Search stays on the rail; Settings has a single navigation button.
- The commander avatar opens a dedicated profile with overview, finances, inventory, assets, fleet, game statistics, reputation and recent activity. Ranks are part of the same workspace. Frontier sign-in remains in Connections.
- Career ranks use the corresponding in-game insignia, bundled for offline use. Naval careers use their faction emblems alongside the observed rank name.

## Colony supply and carrier cargo

- Optional carrier warehouse accounting reduces the amount still to buy using observed personal carrier cargo and, when available and accessible, squadron carrier cargo.
- Cargo aboard, warehouse stock and delivered construction supplies remain separate. Missing or outdated inventory is marked unknown, and the procurement estimate does not double count a transfer.
- Supply previews and tables show carrier stock and the amount to buy. Shared cargo requires the user to confirm access; the squadron bank is not treated as cargo.
- Squadron access follows the observed commander and carrier identity. An older Frontier response cannot overwrite a more recent cargo transfer.

## Interface improvements

- Restored vertical spacing between overview summaries and detail cards, with shared spacing for other affected summaries.
- Removed the application/activity heading from the in-game overlay, leaving room for useful activity panels.
- Updated both README versions, user help and full-screen screenshots with isolated demonstration data.
- Current ship location follows confirmed ship events, separately from walking, taxis and multicrew. Unknown inventory and rank progress remain explicitly unknown.

Existing profiles and account credentials remain in the user's local profile. No registration of an OAuth application or additional developer setup is required.
