package frontend

import (
	"embed"
	"encoding/base64"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
)

//go:embed *.html *.css *.js
var Files embed.FS

func HTML(nonce string, smoke bool) string {
	read := func(name string) string { b, _ := Files.ReadFile(name); return string(b) }
	flag := "false"
	if smoke {
		flag = "true"
	}
	brand, _ := data.Files.ReadFile("copilot.png")
	brandScript := "window.CopilotBrand='data:image/png;base64," + base64.StdEncoding.EncodeToString(brand) + "';\n"
	return strings.NewReplacer("{{NONCE}}", nonce, "{{CSS}}", read("app.css")+"\n"+read("academy.css")+"\n"+read("calculator.css")+"\n"+read("overview.css")+"\n"+read("reference.css")+"\n"+read("carrier-trade.css"), "{{JS}}", brandScript+read("icons.js")+"\n"+read("academy.js")+"\n"+read("calculator.js")+"\n"+read("overview.js")+"\n"+read("reference.js")+"\n"+read("carrier-trade.js")+"\n"+read("v22-smoke.js")+"\n"+read("app.js"), "{{SMOKE}}", flag).Replace(read("index.html"))
}
