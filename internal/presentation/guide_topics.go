package presentation

import (
	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type guideTranslation struct {
	group string
	title string
	body  string
}

// Guide search uses both languages, while each response carries only the
// current language. The same topic ID survives language changes.
func guidePage(query, language string) Page {
	page := Page{Title: Title("help"), Columns: []string{"Раздел", "Тема"}, Rows: []Row{}}
	if language == "en" {
		page.Title = "Guide"
		page.Columns = []string{"Section", "Topic"}
	}
	for _, topic := range data.Array("help.json") {
		id := v.Str(topic["id"])
		en := englishGuideTopics[id]
		if !contains(query, id, topic["group"], topic["title"], topic["summary"], topic["body"], topic["keywords"], en.group, en.title, en.body) {
			continue
		}
		group, title, body := v.Str(topic["group"]), v.Str(topic["title"]), v.Str(topic["body"])
		if language == "en" && en.body != "" {
			group, title, body = en.group, en.title, en.body
		}
		page.Rows = append(page.Rows, Row{ID: id, Data: M{"id": id, "title": title}, Cells: []string{group, title}, Details: body})
	}
	return page
}

const (
	guideStart    = "Getting started"
	guideMining   = "Mining"
	guideTrade    = "Trading"
	guideCarrier  = "Fleet carriers"
	guideEngineer = "Engineering"
	guideLibrary  = "Reference and results"
	guideColony   = "Colonisation"
	guideSettings = "Settings and connections"
)

var englishGuideTopics = map[string]guideTranslation{
	"start": {guideStart, "First launch", `Start ED Copilot and Elite Dangerous. If the journal indicator above Settings shows a problem, select the game's journal folder in Settings. Copilot reads the journal as you play.

Use the icon rail to choose a workspace and the sidebar to open its tools. Ctrl+K searches for a section. Open Settings → Connections to sign in to Frontier or enter an Inara key.`},
	"overview": {guideStart, "Reading the overview", `Overview shows your system, ship, cargo, fuel, balance and active objectives. Open Ship and cargo for the latest inventory, Missions for accepted assignments or Journal for recent events.

A dash means the value is unknown. Cargo is measured in tonnes, engineering materials in units, interstellar distances in light-years and station approach distances in light-seconds.`},
	"targets": {guideStart, "Objectives, missions and navigation", `Add an objective with a title, activity, system and priority. Mark it complete when you finish. Saved mining locations retain their system and body names.

Navigation shows the ship route read from the in-game galaxy map. Carrier routing has its own tool. Missions come from the game journal.`},
	"ring-mining": {guideMining, "Mining in rings", `Choose a mining method and target resource in Mining → Plan. Check Equipment before travelling and use Targets to compare known ring sites.

Fire a prospector limpet and review the asteroid's minerals and concentration in Current run. Select Asteroid mining in the sidebar overlay to see the same measurements in flight.`},
	"surface-mining": {guideMining, "Planetary mining in a Rhino", `In Targets, choose a surface mining location and review its body, approach distance and observation. Confirm the location in the game before deploying your vehicle.

Select Planetary mining in the overlay. Refined commodities go into cargo and appear in the current run when the journal records them.`},
	"raw-materials": {guideMining, "Raw materials for engineering and synthesis", `Raw materials are stored in units in the engineering inventory. Check Engineering → Materials to compare your observed stock with plan requirements.

Use Targets to find a gathering site and select Raw material gathering in the overlay. Review Material Trader exchanges before spending materials reserved for your plan.`},
	"mining-analysis": {guideMining, "Mining runs, equipment and income", `Current run shows refined tonnage, observed rate and prospecting. Equipment checks the selected mining method against the known loadout. History lists past observations.

Open Compare mining runs and select up to four runs. Compare time between the first and last mining entries with the trip from undocking to docking. Confirmed revenue follows identifiable cargo through recorded market sales. Estimated remaining value uses reference prices, including for identifiable cargo held on a carrier. For asteroid prospecting, compare hit rate, the average of measured finds and the average across all scans.

Use Plan to estimate a run from cargo target, reference sale price, travel time and costs. Start a new run to track the next trip separately.`},
	"trade": {guideTrade, "Buying and selling commodities", `Open Markets and choose whether to buy a commodity or sell cargo. Set the quantity, starting system, pad size, radius and quote-age limit.

Compare station price, supply or demand, approach distance and observation time. Confirm the quote in the game before committing the cargo. You can set a result as the overlay destination.`},
	"deals": {guideTrade, "Finding profitable trade trips", `Trade opens with ready trips when market observations are available. The search uses your current system, available hold, budget and required landing pad. Adjust the limits if you want a wider search or a return cargo.

Compare the buying and selling stations, quantities, quote age and estimated profit. Copy a system name for the galaxy map or save a trip to revisit it later.`},
	"ledger": {guideTrade, "Revenue, acquisition cost and profit", `Statistics → Trading summarises sales and purchases during the selected tracking period. Trade history in Markets shows individual journal operations.

Sales consume the oldest known acquisition lots first. When a lot has no recorded cost, the full margin remains unknown. Internal transfers and colony deliveries are recorded separately from market sales.`},
	"carrier": {guideCarrier, "Managing a personal fleet carrier", `Open carrier management in the game to record your carrier's parameters. Signing in to Frontier adds automatic updates for supported finances, services and stock.

Check the tritium tank, free warehouse space and the time of each snapshot. Only mark an inventory complete when it includes every commodity. Use Carrier trading for trip profitability and Carrier route for jumps.`},
	"carrier-route": {guideCarrier, "Carrier route and tritium planning", `Choose a departure, destination, any intermediate stops and the reserve you want on arrival. Import your carrier's known mass, tank and warehouse tritium. Set a tritium price to estimate fuel cost.

After calculation, check the next jump, fuel use, transfers and refuelling stops. Copy the next system to the galaxy map. Queue times can extend the arrival estimate.`},
	"engineering": {guideEngineer, "Planning engineering upgrades", `Open Engineering → Progression to see engineer referrals, invitations, permits and workshop requirements in order. Select engineers, a module and a blueprint to compare providers and calculate the material list for one upgrade. Selecting observed equipment starts the calculation from its recorded modifications.

Use Engineering → Plan for a shopping list across several upgrades. Set the number of rolls per grade and select any experimental effect. Materials compares the list with recorded stock and opens the material trader calculation.`},
	"material-trader": {guideEngineer, "Calculating a Material Trader exchange", `Open Engineering → Material trading. Enter the material and the additional quantity you need, or start with a missing material from the Materials view.

Check each proposed exchange against trader categories, exchange rates, stock, storage limits and materials needed by the rest of your plan. Complete the trade in the game.`},
	"engineers": {guideEngineer, "Engineers and upgrade access", `Open an engineer to see their base, available blueprints and maximum grades. Compare those capabilities with your planned upgrades.

Check your observed access status before travelling. Personal notes help you plan a visit and can be updated when the journal records access.`},
	"encyclopedia": {guideLibrary, "Using the Encyclopedia", `Choose a category and use search to find ships, engineers, commodities, modules, materials, blueprints or effects. Open an entry to see specifications, recipes and related items.

Read the selected module variant and grade before planning an upgrade. Commodity entries can open a market search. Systems and stations use their observed game names.`},
	"statistics": {guideLibrary, "Statistics and CSV exports", `Choose a tracking period and review refined ore, collected materials, observed sales and mining results. Trading aggregates turnover and recorded profit. Colonisation deliveries remain in a separate view.

Use CSV export for further analysis. Beginning a new tracking period leaves the original journals in place.`},
	"colonization": {guideColony, "Choosing a colony and planning construction", `Overview lists all colonies and their next requirements. In System selection, enter an inhabited system and select an eligible station and its controlling faction. Confirm the claim in the game.

In Construction, choose a colony, add facilities and inspect each resource bill. Supplies is the delivery journal. Budget combines the galactic average cost, outstanding deliveries and outstanding procurement after cargo and eligible carrier stock are counted.`},
	"overlay": {guideSettings, "In-game overlay and activity", `Click the eye above Settings. Choose the in-game activity under the ship name in the sidebar. For colonisation, choose which colony to follow.

Ctrl+Alt+H toggles the overlay. The overlay appears when Elite Dangerous has focus. Change its opacity in Settings → Appearance and colours.`},
	"settings": {guideSettings, "Journal folder and local settings", `If there is no journal data, select the folder containing Journal files and game snapshots. Use Rescan journals after moving the files. Local plans and preferences are stored separately from the game.

Reference commodity prices apply to cargo and planning estimates. Markets and recorded deals retain their own prices.`},
	"interface": {guideSettings, "Sidebar groups and window modes", `Choose a workspace on the icon rail, then a tool or subsection in the sidebar. The menu editor arranges optional sections. Ctrl+K opens section search.

Use Settings to choose windowed, fullscreen or borderless mode. F11 switches fullscreen. Where a table has row details, use its details control to open or close the pane.`},
	"appearance": {guideSettings, "HUD colours and palettes", `In Settings → Appearance and colours, choose a preset, enter your own colours or set an RGB matrix. Import a game or EDHM palette, preview it and apply it.

Enable the game palette for the main window if you want Copilot and the overlay to share the same colours.`},
	"frontier": {guideSettings, "Connecting a Frontier account", `In Settings → Connections, select Sign in with Frontier. Sign in on the official Frontier website opened in your browser.

Supported commander, market and carrier data update automatically. Check snapshot dates when planning around inventory or finances. Sign out in the same settings card to remove the saved connection.`},
	"integrations": {guideSettings, "Connecting Inara", `Create a personal key in your Inara account's API settings. In Copilot, open Settings → Connections, paste the key and turn on event delivery.

A blank key field keeps the current key. The Inara card shows the current status. You can turn off delivery while keeping the key.`},
	"troubleshooting": {guideSettings, "When data is missing", `Check the journal indicator first. Confirm the journal folder and active commander, then read the journals again if you have moved them.

If a market or route is unavailable, check system names, filters and the connection. For an empty overlay, verify that it is enabled and that Elite Dangerous has focus.`},
	"sources": {guideSettings, "Reading observations and estimates", `Game journals provide local observations, Frontier supplies supported account snapshots, and public catalogues provide system, market and route observations.

Use a quote's timestamp to judge its age. Incomplete inventories and unobserved costs remain unknown. Manual inputs affect estimates and plans.`},
}
