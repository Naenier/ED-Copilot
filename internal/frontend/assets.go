package frontend

import (
	"embed"
	"strings"
)

//go:embed *.html *.css *.js
var Files embed.FS

func HTML(nonce string, smoke bool) string {
	read := func(name string) string { b, _ := Files.ReadFile(name); return string(b) }
	flag := "false"
	if smoke {
		flag = "true"
	}
	return strings.NewReplacer("{{NONCE}}", nonce, "{{CSS}}", read("app.css")+"\n"+read("academy.css")+"\n"+read("calculator.css"), "{{JS}}", read("icons.js")+"\n"+read("academy.js")+"\n"+read("calculator.js")+"\n"+read("app.js"), "{{SMOKE}}", flag).Replace(read("index.html"))
}
