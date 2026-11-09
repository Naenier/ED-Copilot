package presentation

import "github.com/Naenier/ED-Copilot/internal/localization"

// HelpForLanguage selects explicit user-facing help. It does not change the
// existing Russian help or infer a shared mutable application language.
func HelpForLanguage(tab, language string) string {
	if localization.Normalize(language) != "en" {
		return HelpFor(tab)
	}
	if text := englishSectionHelp[tab]; text != "" {
		return text
	}
	return "Choose a section, use its search and action buttons, and open an entry for details. Plans are saved in your personal Windows profile. Names entered by you and observed in the game are preserved."
}

var englishSectionHelp = map[string]string{
	"commander": `Commander centre

Use the views to inspect observed finances, inventory, possessions, ships, statistics, reputation and recent journal activity. A dash means no confirmed observation is available; zero means a known zero value.

The current ship is separate from an owned-ship inventory. Visiting a shipyard provides a stored-ship list. After backpack or storage changes, an older inventory may require a replacement snapshot; an unknown total is never presented as exact. Ship engineering materials follow the existing collection and expense ledger. Frontier can supplement observations only for the matching active commander.`,
	"ranks": `Commander ranks

Eight cards show your in-game ranks and the observed percentage towards the next rank. Open a discipline to read advancement methods and rank-specific rewards. Progress comes from journal Progress events and promotions from Promotion events; first observation is not an achievement date. Frontier can supplement a known rank without inventing an unknown percentage.

A dash means no observation is available. Start the game or reread journals to update your record. Ship and system unlocks appear beside their relevant rank.`,
	"politics": `Galaxy

Politics shows the controlling faction and power in the selected system. Search for another system or return to your current one. Factions and conflicts appear when observations are available for the selected system.

Community goals and Powerplay update automatically. Personal goal progress and conflict scores come from the game journal.`,
	"overview": `Commander dashboard

Use this page to check your current location, ship, cargo and the tasks relevant to your chosen activity. Journal observations update as you play; a missing value means the application has not observed it yet.

The activity selector chooses the in-game panels. Enable the overlay to show it while Elite Dangerous has focus. When Colonisation is selected, choose the colony whose construction and supply queue you want to follow.

Use the sidebar to open a tool. Its menu lets you add, remove or reorder optional sections.`,
	"targets": `Objectives and locations

Keep personal tasks separate from observed mining locations. Add an objective with a title, activity, system and priority, then mark it complete when you have finished it. Saved locations retain their system and body names for later use.

Mining-location search uses known public observations and your filters. Check the resource, method, distance and evidence before travelling. A signal or predicted resource is not a guarantee of a profitable deposit.

The navigation view shows the ship route read from the in-game galaxy map. Carrier routing has its own section.`,
	"sale": `Buying and selling

Choose whether you want to buy a commodity or sell cargo. Enter a starting system and set the commodity, quantity, landing pad, radius and quote-age limits. System fields offer online name suggestions after three characters; you can also enter a name manually.

Compare station prices, supply or demand, arrival distance and observation time. Fleet carrier quotes may change quickly. Unknown stock is not zero, and a quote does not reserve goods.

Select a suitable market to use it as the overlay destination. Confirm the price and cargo quantity in the game before completing a transaction.`,
	"deals": `Trade trips

Enter a starting system, free cargo capacity, available budget and the commodities to compare. Filters control landing pads, planetary stations, carriers, market age, route distance and required supply or demand. Enable a return cargo when you want a two-leg plan.

Each result shows the purchase and sale stations, their systems, quote times, available quantity and estimated economics. Copy a system or station name without switching away from the application. Select a result, set a destination, save a hauling leg, or save the full route for later.

Saved quotes are snapshots. Refresh the search before flying. Time estimates use the entered assumptions and exclude the approach to the first station.`,
	"carrier": `Personal carrier

Open carrier management in the game to record observations. After you sign in with Frontier, supported ownership, finances, services and inventory are refreshed automatically in the background. Check each snapshot's time. An incomplete inventory does not establish zero stock for missing commodities.

Use planning settings to record your weekly upkeep and desired reserves. Warehouse corrections are personal records; they do not place market orders or move cargo in the game. Only mark an inventory complete if it really contains every commodity.

Use Carrier Trading for trip profitability, Carrier Calculator for purchase and upkeep, and Carrier Route for jump planning.`,
	"carrier-calculator": `Carrier cost calculator

Choose a personal or squadron carrier. Each type keeps a separate plan. Set optional services to active, suspended or absent; suspension reduces upkeep but does not release capacity. Mandatory services cannot be removed.

Enter planned jumps per week, average tritium consumption, fuel price, planned cargo, bank balance and desired reserve. Unknown fuel inputs remain unknown rather than becoming free fuel.

Compare purchase cost, occupied capacity, weekly upkeep and budget projections. The 30-day and 365-day estimates are proportional planning periods. Use Carrier Route for fuel on a specific journey. Calculator changes do not alter in-game services or finances.`,
	"carrier-trade": `Carrier trading

Compare wine and liquor suppliers for Rackham's Peak in HIP 58832. The planner uses the built-in corridor from Sol and works out the connection from your departure system.

Plan service space and the fuel for both directions. The cargo hold keeps enough tritium for the round trip plus 10%, after accounting for the separate tank. For a squadron carrier, enter the expected fuel burn in each direction.

Check the quote age, supply, demand and landing pad before loading. Rackham's Peak takes small and medium ships. Confirm Public Holiday and carrier parking in the game.`,
	"route": `Carrier route

Enter the departure and destination systems, occupied capacity, tank and warehouse tritium, arrival reserve and timing assumptions. Waypoint and refuelling lists accept one system per line; autocomplete replaces only the line containing the caret.

Use actual starting inventory when planning your own departure. Minimum starting fuel is a hypothetical mass-dependent calculation, not a reading of your carrier. Enter a tritium price if you need a fuel-cost estimate.

After calculation, check the next system, fuel consumption, required transfers and replenishment stops. Copy the next system to the galaxy map. Confirm visited stages or let supported carrier-jump observations advance them. Queues and unplanned waiting can extend the estimated journey.`,
	"mining": `Mining run and income plan

Review the current mining run, refined tonnage, measured rate and observed equipment. Select the mining method and target commodity. The prospector result helps decide whether the current asteroid meets your configured quality threshold.

Set target cargo, reference sale price, travel time and operating costs to estimate a run. If a price or rate is unknown, the corresponding estimate cannot be treated as guaranteed income.

Start a new run to separate sessions. Statistics use supported journal events; cargo transfers, purchased goods and colony deliveries are not automatically mining profit. Choose asteroid, planetary or raw-material activity to display the relevant overlay panels.`,
	"exobiology": `Exobiology

Use FSS and DSS observations to identify planets with biological signals. The application records observed species, possible candidates, sample progress and completed sample sets. Predictions help narrow a search; they do not prove a species is present.

Select a planet to inspect its signals and estimated value range. Sampling requires three genetically diverse samples of one species. When current coordinates are available, the overlay shows the distance to previous samples and the remaining minimum separation.

First footfall and bonuses require confirmation. Unsold values are estimates; Vista Genomics sales confirm the actual payment. Save useful planets and compare your finds and sales in the statistics views.`,
	"engineering": `Engineering

Add the modules, blueprints, experimental effects and planned rolls you need. Materials are calculated for the specified number of attempts, not an assumed guaranteed completion. Zero rolls skip a completed grade.

Compare the plan with observed raw, manufactured and encoded stocks. Unknown inventory remains unknown. Material Trader calculations respect trading families, storage limits and protected materials required by the plan. Some materials cannot be traded at ordinary material traders.

The engineer reference lists locations, access requirements, supported modules and maximum grades. Your personal access mark can be replaced with journal-derived status. Save or load a plan without changing your actual ship loadout.`,
	"colonization": `Colonisation

First check an inhabited origin system, choose an eligible station and its controlling faction, then search nearby candidate systems. Public observations can be incomplete; confirm claim availability and restrictions in the game. Adding a personal plan does not purchase a claim.

Overview lists your colonies. Open Construction to add facilities, see their resource bills and change construction order or supply priority. Visiting a depot loads its observed bill; manual bills remain estimates.

Supplies is the delivery log: date, commodity and quantity. Budget lists each commodity's full requirement value, remaining delivery value and procurement cost. It combines all planned and active structures in the selected colony.

The overlay groups cargo to deliver, stock to unload and goods to buy. With carrier accounting enabled, the unload card requires fresh, accessible stock and a confirmed carrier location in the colony system. Long lists show how many rows remain. Journal-observed transfers update a known warehouse immediately.

Cargo aboard is ready for delivery but has not yet been accepted by the depot. A colonisation ship is a supply vessel, not a completed facility. Confirmed permanent stations in your own claim can establish completed construction without inventing a variant or an unobserved resource bill.`,
	"encyclopedia": `Encyclopedia

Choose a category, then use groups and search to find articles, ships, engineers, commodities, modules, materials, blueprints or effects. Open an entry for specifications, recipes and related entries. IDs and game locations remain stable when you change the interface language.

Recipes show material amounts and grade-specific modifiers. Read the selected module variant: a blueprint family does not give every module identical effects. Commodity entries link to market searches, where you can obtain price, supply, demand and timestamped observations.

Systems and stations use their observed game names. Long articles or descriptions may retain their original content language; interface captions and known canonical game names are localised.`,
	"learning": `Academy

Choose a course to study its basics, work through the Copilot practice and check your understanding. The left contents panel moves between lessons; the article contents move between sections. Reading mode hides the main navigation so you can focus.

Bookmark useful lessons and mark a lesson Studied when you have finished it. Answers alone do not mark it complete. Your bookmarks, progress, answers and reading position are saved locally in your profile.

Tool links open the corresponding application section. Lessons are available offline; external reference links open in your browser. Check current game conditions before carrying out an operation.`,
	"statistics": `Statistics

Review the selected tracking period, mining runs, refined commodities, gathered materials and observed sales. Revenue is not profit. Trade history shows acquisition cost and margin only when they are known.

Colony deliveries and internal cargo transfers are tracked separately from market sales. Missing observations can limit totals; a blank value does not mean a confirmed zero.

Start a new tracking period when you need a fresh comparison. The original game journals remain in place. Export the available statistics to CSV for your own analysis.`,
	"settings": `Settings

Choose Russian or English for the interface. Names entered by you and actual system, station and ship identities are retained. Use windowed, fullscreen or borderless mode to suit your display.

Choose a palette, edit colours or preview a palette imported from the game before applying it. The optional game palette for the main window is separate from the overlay. Select the journal folder if the application cannot locate game observations.

Reference commodity prices are optional personal planning inputs. They help value cargo; market searches and recorded transactions keep their own prices and timestamps. Configure the overlay and select the activity whose panels you want while the game has focus.`,
	"account": `Frontier account

In Settings → Connections, select Sign in with Frontier to open the official sign-in page in your browser.

After sign-in, ED Copilot automatically refreshes supported commander, ship, current station and carrier information in the background. Companion API snapshots may lag behind journal events, so check their time before using inventory or finances for a plan. If the connection fails, saved observations remain dated and unknown information stays unknown.

Sign out when you want to remove the saved connection. You can still use local journal observations and offline tools without being signed in.`,
	"integrations": `Inara synchronisation

Obtain your personal API key in your Inara account settings. In Copilot, open Settings → Connections, enter the key and enable event delivery. Leaving the field empty retains the saved key.

Connection status appears next to the key. Re-reading old journals does not resend their history.

Service restrictions or temporary network failures may delay delivery. A saved key does not by itself prove that the service has accepted an event. You can disable sending while keeping the saved key.`,
	"help": `Using ED Copilot

Choose a tool from the sidebar. Its menu lets you show optional sections and arrange them around the activities you use. Search and action buttons are specific to the current section; select an entry to view its details.

The overlay appears only while Elite Dangerous has focus. Choose an activity to keep its panels relevant. For deeper explanations, use the Academy; use the Encyclopedia to look up equipment, engineers, commodities and recipes.

Plans, reading progress and preferences are stored in your Windows profile. Observations, estimates and unknown values have different meanings: check timestamps and confirm the final transaction, construction or navigation action in the game.`,
}
