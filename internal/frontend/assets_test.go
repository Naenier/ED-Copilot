package frontend

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"testing"
)

func embeddedAsset(t *testing.T, name string) string {
	t.Helper()
	data, err := Files.ReadFile(name)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		t.Fatalf("embedded asset %q is missing or empty: %v", name, err)
	}
	return string(data)
}

func TestHTMLIsCompleteOfflineDocument(t *testing.T) {
	const nonce = "0123456789abcdef0123456789abcdef01234567" // Same form as the trusted random-hex caller.
	for _, smoke := range []bool{false, true} {
		name, flag := "normal", "false"
		if smoke {
			name, flag = "smoke", "true"
		}
		t.Run(name, func(t *testing.T) {
			document := HTML(nonce, smoke)
			// WebView2 NavigateToString rejects documents over 2 MiB. Keep a
			// useful margin so adding embedded art cannot silently prevent startup.
			if len([]byte(document)) >= 1800*1024 {
				t.Fatalf("inline HTML exceeds the WebView2 startup budget: %d bytes", len([]byte(document)))
			}
			for _, required := range []string{"<!doctype html>", `<html lang="en">`, `id="app"`, `id="layers"`, `id="toasts"`, "</body></html>"} {
				if !strings.Contains(document, required) {
					t.Errorf("document is missing %s", required)
				}
			}
			if regexp.MustCompile(`\{\{[A-Z_]+\}\}`).MatchString(document) {
				t.Fatal("document contains an unresolved build placeholder")
			}
			if strings.Count(document, "window.COPILOT_SMOKE="+flag+";") != 1 {
				t.Fatalf("smoke flag is not an unquoted boolean %s", flag)
			}
			if strings.Count(document, nonce) != 2 || !strings.Contains(document, "script-src 'nonce-"+nonce+"'") || !strings.Contains(document, `<script nonce="`+nonce+`">`) {
				t.Fatal("CSP and the single executable script must share exactly the supplied nonce")
			}
			for _, directive := range []string{"default-src 'none'", "connect-src 'none'", "frame-src 'none'", "object-src 'none'", "base-uri 'none'", "form-action 'none'", "img-src data:", "font-src 'none'"} {
				if !strings.Contains(document, directive) {
					t.Errorf("offline document is missing CSP restriction %q", directive)
				}
			}
			// Styles also include controlled palette/progress attributes at runtime;
			// granting those must never grant arbitrary inline JavaScript or eval.
			csp := regexp.MustCompile(`http-equiv="Content-Security-Policy" content="([^"]+)"`).FindStringSubmatch(document)
			if len(csp) != 2 || strings.Contains(csp[1], "unsafe-eval") || strings.Contains(csp[1], "script-src 'unsafe-inline'") {
				t.Fatal("executable content is not confined to the nonce-protected bundle")
			}
			for _, pattern := range []string{`(?i)<script\b[^>]*\bsrc\s*=`, `(?i)<link\b[^>]*\bhref\s*=`, `(?i)<(?:iframe|object|base)\b`, `(?i)\b(?:src|href)\s*=\s*["'](?:https?:)?//`} {
				if regexp.MustCompile(pattern).MatchString(document) {
					t.Errorf("offline document requests an external runtime resource: %s", pattern)
				}
			}
		})
	}
	other := HTML("fedcba9876543210fedcba9876543210fedcba98", false)
	if strings.Contains(other, nonce) {
		t.Fatal("nonce from an earlier document leaked into a later document")
	}
}

func TestHTMLIncludesEveryRuntimeAssetWithoutBreakingRawTextElements(t *testing.T) {
	document := HTML("1111111111111111111111111111111111111111", false)
	script := regexp.MustCompile(`(?s)<script\b[^>]*>(.*?)</script>`).FindStringSubmatch(document)
	style := regexp.MustCompile(`(?s)<style\b[^>]*>(.*?)</style>`).FindStringSubmatch(document)
	if len(script) != 2 || len(style) != 2 {
		t.Fatal("complete executable and stylesheet elements are required")
	}
	previousScript, previousStyle := -1, -1
	for _, name := range []string{"icons.js", "academy.js", "calculator.js", "app.js", "app.css", "academy.css", "calculator.css"} {
		t.Run(name, func(t *testing.T) {
			asset := embeddedAsset(t, name)
			position := strings.Index(document, asset)
			if position < 0 || strings.Count(document, asset) != 1 {
				t.Fatal("asset is not included in the document exactly once")
			}
			if strings.HasSuffix(name, ".js") {
				if !strings.Contains(script[1], asset) {
					t.Fatal("JavaScript is outside its executable element")
				}
				if regexp.MustCompile(`(?i)</script[\t\n\f\r />]`).MatchString(asset) {
					t.Fatal("raw closing script tag would truncate the embedded executable bundle; escape its slash")
				}
				if position <= previousScript {
					t.Fatal("scripts must load icons and feature modules before the application boots")
				}
				previousScript = position
			} else {
				if !strings.Contains(style[1], asset) {
					t.Fatal("CSS is outside its stylesheet element")
				}
				if regexp.MustCompile(`(?i)</style[\t\n\f\r />]|@import\b|url\s*\(\s*["']?(?:https?:)?//`).MatchString(asset) {
					t.Fatal("stylesheet closes its raw-text element or requires an external runtime asset")
				}
				if position <= previousStyle {
					t.Fatal("feature styles must follow the shared application styles")
				}
				previousStyle = position
			}
		})
	}
	if strings.Count(strings.ToLower(document), "</script>") != 1 || strings.Count(strings.ToLower(document), "</style>") != 1 {
		t.Fatal("document must contain one complete inline script and stylesheet")
	}
	if strings.Contains(document, "node:test") || strings.Contains(document, "require('node:") {
		t.Fatal("development tests were included in the production document")
	}
}

func TestAllReferencedIconsAreBundledSelfContainedSVG(t *testing.T) {
	encoded := strings.TrimSpace(embeddedAsset(t, "icons.js"))
	const assignment = "window.CopilotIcons="
	if !strings.HasPrefix(encoded, assignment) {
		t.Fatal("icon registry must be a locally embedded data assignment")
	}
	var icons map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(encoded, assignment), ";")), &icons); err != nil || len(icons) == 0 {
		t.Fatalf("invalid or empty SVG registry: %v", err)
	}
	for name, svg := range icons {
		t.Run(name, func(t *testing.T) {
			decoder, roots, drawing := xml.NewDecoder(strings.NewReader(svg)), 0, false
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("invalid SVG: %v", err)
				}
				start, ok := token.(xml.StartElement)
				if !ok {
					continue
				}
				if roots == 0 && (start.Name.Local != "svg" || start.Name.Space != "http://www.w3.org/2000/svg") {
					t.Fatal("icon has no SVG root")
				}
				roots++
				if start.Name.Local == "script" || start.Name.Local == "foreignObject" || start.Name.Local == "image" || start.Name.Local == "use" {
					t.Fatalf("icon contains an executable or linked element: %s", start.Name.Local)
				}
				if start.Name.Local != "svg" {
					drawing = true
				}
				for _, attr := range start.Attr {
					if strings.HasPrefix(strings.ToLower(attr.Name.Local), "on") || attr.Name.Local == "href" || strings.Contains(strings.ToLower(attr.Value), "url(") {
						t.Fatalf("icon has an active or external attribute: %s", attr.Name.Local)
					}
				}
			}
			if !drawing {
				t.Fatal("icon contains no drawing")
			}
		})
	}

	// Derive references from the UI's icon-bearing calls and data registries.
	// This catches the silent circle fallback when a requested icon is omitted.
	references := map[string]string{"circle": "registry fallback"}
	callPattern := regexp.MustCompile(`\b(icon|iconBtn|iconButton|btn|metric)\s*\(`)
	literals := regexp.MustCompile(`['"]([a-z][a-z0-9-]*)['"]`)
	for _, name := range []string{"app.js", "academy.js", "calculator.js"} {
		asset := embeddedAsset(t, name)
		for _, match := range callPattern.FindAllStringSubmatchIndex(asset, -1) {
			kind := asset[match[2]:match[3]]
			args := jsCallArguments(asset, match[1])
			argument := map[string]int{"icon": 0, "iconBtn": 0, "iconButton": 1, "btn": 2, "metric": 3}[kind]
			if kind == "metric" && name == "calculator.js" {
				argument = 4
			}
			if argument >= len(args) {
				continue
			}
			for _, value := range literals.FindAllStringSubmatchIndex(args[argument], -1) {
				// A literal used as a ternary condition (kind==='checkpoint'?)
				// is a lesson kind, while its branch values name the icons.
				if strings.HasPrefix(strings.TrimSpace(args[argument][value[1]:]), "?") {
					continue
				}
				references[args[argument][value[2]:value[3]]] = name
			}
		}
		for _, registry := range []string{"tabIcons", "courseIcons", "serviceIcons"} {
			match := regexp.MustCompile(`\b` + registry + `\s*=\s*\{([^}]+)\}`).FindStringSubmatch(asset)
			if len(match) == 2 {
				for _, value := range regexp.MustCompile(`:\s*['"]([^'"]+)['"]`).FindAllStringSubmatch(match[1], -1) {
					references[value[1]] = name + "/" + registry
				}
			}
		}
		for _, value := range regexp.MustCompile(`\bicon:\s*['"]([^'"]+)['"]`).FindAllStringSubmatch(asset, -1) {
			references[value[1]] = name + "/icon property"
		}
	}
	for name, source := range references {
		if strings.TrimSpace(icons[name]) == "" {
			t.Errorf("%s references unbundled icon %q", source, name)
		}
	}
	for _, essential := range []string{"x", "minus", "maximize-2", "ellipsis", "graduation-cap", "bookmark-check", "person-standing", "circle-help", "calculator"} {
		if references[essential] == "" {
			t.Errorf("reference scanner missed essential chrome or Academy icon %q", essential)
		}
	}
}

// Split call arguments while ignoring quoted strings and nested expressions.
// This is deliberately limited to locating static icon references, not a JS parser.
func jsCallArguments(source string, from int) []string {
	var args []string
	start, parentheses, brackets, braces := from, 1, 0, 0
	var quote byte
	for i := from; i < len(source); i++ {
		c := source[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '(':
			parentheses++
		case ')':
			parentheses--
			if parentheses == 0 {
				return append(args, source[start:i])
			}
		case '[':
			brackets++
		case ']':
			brackets--
		case '{':
			braces++
		case '}':
			braces--
		case ',':
			if parentheses == 1 && brackets == 0 && braces == 0 {
				args = append(args, source[start:i])
				start = i + 1
			}
		}
	}
	return nil
}
