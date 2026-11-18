package frontend

import (
	"regexp"
	"strings"
	"testing"
)

func TestDeferredWorkspacesAreAvailableOnlyThroughEmbeddedResources(t *testing.T) {
	html := HTML("1111111111111111111111111111111111111111", false)
	for path, module := range map[string]string{
		"/workspace/settings.js":      "SettingsPaneUI",
		"/workspace/trade-routes.js":  "DealsUI",
		"/workspace/carrier-route.js": "RouteWorkspaceUI",
		"/workspace/colonization.js":  "ColonyWorkspaceUI",
		"/workspace/academy.js":       "AcademyUI",
		"/workspace/calculator.js":    "CalculatorUI",
		"/workspace/engineering.js":   "EngineeringRoadmapUI",
	} {
		pattern := regexp.MustCompile(`window\.` + module + `\s*=`)
		bundle, ok := WorkspaceBundle(path)
		if !ok || !pattern.Match(bundle) || pattern.MatchString(html) {
			t.Errorf("workspace %s must be available on demand without loading at startup", module)
		}
		if !strings.Contains(string(bundle), "style.textContent=") {
			t.Errorf("workspace %s omits its stylesheet", module)
		}
	}
	for _, path := range []string{"/workspace/../app.js", "/workspace/academy.js?cache=1", "/workspace/unknown.js"} {
		if asset, ok := WorkspaceBundle(path); ok || asset != nil {
			t.Errorf("unknown workspace was served at %q", path)
		}
	}
}

func TestSmokeScriptsOnlyLoadForTheirIsolatedWorkflow(t *testing.T) {
	const nonce = "1111111111111111111111111111111111111111"
	smoke := regexp.MustCompile(`window\.V23Smoke\s*=`)
	capture := regexp.MustCompile(`window\.ReadmeCapture\s*=`)
	production := HTML(nonce, false)
	isolated := HTML(nonce, true)
	readme := HTMLMode(nonce, true, true)
	if smoke.MatchString(production) || capture.MatchString(production) || !smoke.MatchString(isolated) ||
		capture.MatchString(isolated) || smoke.MatchString(readme) || !capture.MatchString(readme) {
		t.Fatal("production, smoke and README capture need different startup assets")
	}
}
