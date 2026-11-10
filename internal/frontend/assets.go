package frontend

import (
	"embed"
	"encoding/base64"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/localization"
)

//go:embed *.html *.css *.js
var Files embed.FS

func HTML(nonce string, smoke bool) string {
	return HTMLMode(nonce, smoke, false)
}

func HTMLMode(nonce string, smoke, readme bool) string {
	read := func(name string) string {
		b, err := Files.ReadFile(name)
		if err != nil {
			panic("Required frontend asset missing: " + name)
		}
		return string(b)
	}
	flag := "false"
	if smoke {
		flag = "true"
	}
	brand, _ := data.Files.ReadFile("copilot.png")
	brandScript := "window.CopilotBrand='data:image/png;base64," + base64.StdEncoding.EncodeToString(brand) + "';\n"
	brandScript += "window.CopilotTranslations=" + strings.ReplaceAll(localization.CatalogJSON(), "<", "\\u003c") + ";\n" + read("i18n.js") + "\n"
	if readme && smoke {
		brandScript += "window.COPILOT_README=true;\n"
	}
	return strings.NewReplacer("{{NONCE}}", nonce, "{{CSS}}", read("app.css")+"\n"+read("tooltips.css")+"\n"+read("navigation.css")+"\n"+read("commander.css")+"\n"+read("ranks.css")+"\n"+read("politics.css")+"\n"+read("freshness.css")+"\n"+read("updates.css")+"\n"+read("academy.css")+"\n"+read("calculator.css")+"\n"+read("overview.css")+"\n"+read("reference.css")+"\n"+read("galaxy.css")+"\n"+read("engineering-roadmap.css")+"\n"+read("carrier-trade.css")+"\n"+read("settings-pane.css")+"\n"+read("deals.css")+"\n"+read("trade-statistics.css")+"\n"+read("route-workspace.css")+"\n"+read("colonization-workspace.css")+"\n"+read("workspace-fixes.css")+"\n"+read("system-search.css")+"\n"+read("exobiology.css"), "{{JS}}", brandScript+read("icons.js")+"\n"+read("tooltips.js")+"\n"+read("rank-insignia.js")+"\n"+read("navigation.js")+"\n"+read("commander.js")+"\n"+read("ranks.js")+"\n"+read("freshness.js")+"\n"+read("politics.js")+"\n"+read("updates.js")+"\n"+read("academy.js")+"\n"+read("calculator.js")+"\n"+read("overview.js")+"\n"+read("galaxy.js")+"\n"+read("reference.js")+"\n"+read("engineering-roadmap.js")+"\n"+read("carrier-trade.js")+"\n"+read("settings-pane.js")+"\n"+read("deals.js")+"\n"+read("trade-statistics.js")+"\n"+read("route-workspace.js")+"\n"+read("colonization-workspace.js")+"\n"+read("readme-capture.js")+"\n"+read("v27-smoke.js")+"\n"+read("v26-smoke.js")+"\n"+read("v24-smoke.js")+"\n"+read("v23-smoke.js")+"\n"+read("v22-smoke.js")+"\n"+read("exobiology.js")+"\n"+read("system-search.js")+"\n"+read("app.js"), "{{SMOKE}}", flag).Replace(read("index.html"))
}
