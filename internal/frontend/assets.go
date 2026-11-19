package frontend

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"

	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/localization"
)

//go:embed *.html *.css *.js
var Files embed.FS

func read(name string) string {
	data, err := Files.ReadFile(name)
	if err != nil {
		panic("Required frontend asset missing: " + name)
	}
	return string(data)
}

func joinAssets(names []string) string {
	var bundle strings.Builder
	for _, name := range names {
		bundle.WriteString(read(name))
		bundle.WriteByte('\n')
	}
	return bundle.String()
}

var shellCSS = sync.OnceValue(func() string {
	return joinAssets([]string{
		"app.css", "tooltips.css", "navigation.css", "commander.css", "ranks.css", "politics.css",
		"freshness.css", "updates.css", "overview.css", "reference.css", "galaxy.css",
		"workspace-fixes.css", "system-search.css", "surface-navigation.css",
	})
})

var shellJS = sync.OnceValue(func() string {
	brand, _ := data.Files.ReadFile("copilot.png")
	return "window.CopilotBrand='data:image/png;base64," + base64.StdEncoding.EncodeToString(brand) + "';\n" +
		"window.CopilotTranslations=" + strings.ReplaceAll(localization.CatalogJSON(), "<", "\\u003c") + ";\n" +
		joinAssets([]string{
			"i18n.js", "icons.js", "tooltips.js", "rank-insignia.js", "navigation.js", "commander.js",
			"ranks.js", "freshness.js", "politics.js", "updates.js", "overview.js", "galaxy.js", "reference.js",
			"system-search.js", "surface-navigation.js", "workspace-assets.js", "ship-fits-lazy.js",
		})
})

var smokeJS = sync.OnceValue(func() string {
	return joinAssets([]string{"v27-smoke.js", "v26-smoke.js", "v24-smoke.js", "v23-smoke.js", "v22-smoke.js"})
})

var workspaceHTML = sync.OnceValue(func() string { return read("index.html") })
var controllerJS = sync.OnceValue(func() string { return read("app.js") })

func HTML(nonce string, smoke bool) string {
	return HTMLMode(nonce, smoke, false)
}

func HTMLMode(nonce string, smoke, readme bool) string {
	flag, extras := "false", ""
	if smoke {
		flag = "true"
		if readme {
			extras = "window.COPILOT_README=true;\n" + read("readme-capture.js")
		} else {
			extras = smokeJS()
		}
	}
	return strings.NewReplacer(
		"{{NONCE}}", nonce, "{{CSS}}", shellCSS(),
		"{{JS}}", shellJS()+extras+controllerJS(), "{{SMOKE}}", flag,
	).Replace(workspaceHTML())
}

const ShipFitsAssetPath = "/workspace/ship-fits.js"
const NearestAssetPath = "/workspace/nearest.js"

var NearestBundle = makeBundle([]string{"nearest.css"}, "nearest-options.js", "nearest-strings.js", "nearest-picker.js", "nearest.js")

// ShipFitsBundle keeps the fitting catalogue out of the WebView startup document.
var ShipFitsBundle = makeBundle([]string{"ship-fits.css"}, "ship-fit-ru.js", "ship-fit-engine.js", "ship-fit-presets.js", "ship-fits.js")

var workspaceBundles = map[string]func() []byte{
	ShipFitsAssetPath:                ShipFitsBundle,
	NearestAssetPath:                 NearestBundle,
	"/workspace/academy.js":          makeBundle([]string{"academy.css"}, "academy.js"),
	"/workspace/calculator.js":       makeBundle([]string{"calculator.css"}, "calculator.js"),
	"/workspace/settings.js":         makeBundle([]string{"settings-pane.css", "controls.css"}, "settings-pane.js", "controls-action-labels.js", "controls.js"),
	"/workspace/trade-routes.js":     makeBundle([]string{"deals.css"}, "deals.js"),
	"/workspace/carrier-trade.js":    makeBundle([]string{"carrier-trade.css"}, "carrier-trade.js"),
	"/workspace/trade-statistics.js": makeBundle([]string{"trade-statistics.css"}, "trade-statistics.js"),
	"/workspace/engineering.js": makeBundle(
		[]string{"engineering-roadmap.css", "odyssey-workspace.css", "ship-builds.css", "ship-engineering.css", "engineering-itinerary.css"},
		"engineering-roadmap.js", "odyssey-workspace.js", "ship-builds.js", "ship-engineering.js", "engineering-itinerary.js",
	),
	"/workspace/colonization.js":  makeBundle([]string{"colonization-workspace.css", "colony-history.css"}, "colonization-workspace.js", "colony-history.js"),
	"/workspace/exobiology.js":    makeBundle([]string{"exobiology.css", "surface-radar.css"}, "exobiology.js", "surface-radar.js"),
	"/workspace/mining.js":        makeBundle([]string{"mining-comparison.css"}, "mining-comparison.js"),
	"/workspace/carrier-route.js": makeBundle([]string{"route-workspace.css"}, "route-workspace.js"),
	"/workspace/ship-route.js":    makeBundle([]string{"ship-route.css"}, "ship-route.js"),
	"/workspace/exploration.js":   makeBundle([]string{"exploration.css"}, "exploration.js"),
	"/workspace/cartography.js":   makeBundle([]string{"riches.css"}, "riches.js"),
	"/workspace/section-tools.js": makeBundle([]string{"section-seven-tools.css"}, "section-seven-tools.js"),
}

// WorkspaceBundle returns only explicitly registered embedded assets.
func WorkspaceBundle(path string) ([]byte, bool) {
	bundle, ok := workspaceBundles[path]
	if !ok {
		return nil, false
	}
	return bundle(), true
}

func makeBundle(styles []string, scripts ...string) func() []byte {
	return sync.OnceValue(func() []byte {
		style, _ := json.Marshal(joinAssets(styles))
		var script strings.Builder
		script.WriteString("(()=>{const style=document.createElement('style');style.textContent=")
		script.Write(style)
		script.WriteString(";document.head.append(style);})();\n")
		script.WriteString(joinAssets(scripts))
		return []byte(script.String())
	})
}
