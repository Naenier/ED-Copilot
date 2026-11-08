package frontend

import (
	"regexp"
	"testing"
)

func TestCustomWorkspacesAreIncludedInTheOfflineBundle(t *testing.T) {
	html := HTML("1111111111111111111111111111111111111111", false)
	for _, module := range []string{"SettingsPaneUI", "DealsUI", "RouteWorkspaceUI", "ColonyWorkspaceUI", "ReferenceUI", "V23Smoke"} {
		if !regexp.MustCompile(`window\.` + module + `\s*=`).MatchString(html) {
			t.Fatalf("Module %s is absent from the built application", module)
		}
	}
}
