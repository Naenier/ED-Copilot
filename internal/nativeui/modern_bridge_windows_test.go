//go:build windows

package nativeui

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/app"
	"github.com/Naenier/ED-Copilot/internal/frontend"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
	"github.com/lxn/win"
)

func newBridgeEngine(t *testing.T) *app.Engine {
	t.Helper()
	engine, err := app.New(app.Options{DataDir: t.TempDir(), JournalDir: t.TempDir(), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	return engine
}

func TestWebBridgeAllSectionsModesAndActionDescriptions(t *testing.T) {
	engine := newBridgeEngine(t)
	boot, err := dispatchWeb(engine, "boot", M{})
	if err != nil || len(boot["tabs"].([]p.Tab)) != len(p.Tabs) {
		t.Fatal("boot", boot, err)
	}
	for _, tab := range p.Tabs {
		first, err := dispatchWeb(engine, "view", M{"tab": tab.ID})
		if err != nil {
			t.Fatal(tab.ID, err)
		}
		page := first["page"].(p.Page)
		modes := page.Modes
		if len(modes) == 0 {
			modes = []string{""}
		}
		for _, mode := range modes {
			result, err := dispatchWeb(engine, "view", M{"tab": tab.ID, "mode": mode})
			if err != nil {
				t.Fatalf("%s/%s: %v", tab.ID, mode, err)
			}
			page := result["page"].(p.Page)
			referencePage := tab.ID == "encyclopedia" || (tab.ID == "engineering" || tab.ID == "targets") && mode == "Инженеры"
			progressionPage := tab.ID == "engineering" && mode == p.EngineeringProgression
			if page.Title == "" || len(page.Columns) == 0 && !referencePage && !progressionPage {
				t.Fatalf("empty %s/%s", tab.ID, mode)
			}
			if progressionPage {
				roadmap, err := dispatchWeb(engine, "engineering-roadmap", M{"tab": tab.ID})
				if err != nil || len(v.Rows(roadmap["engineers"])) != 38 {
					t.Fatalf("engineering view has no roadmap: %v", err)
				}
			}
			if _, err := json.Marshal(result); err != nil {
				t.Fatalf("non-JSON view %s/%s: %v", tab.ID, mode, err)
			}
			for _, action := range v.Rows(result["actions"]) {
				if action["id"] == "" || action["title"] == "" {
					t.Fatal("invalid action", action)
				}
				for _, field := range v.Rows(action["fields"]) {
					if field["kind"] == "password" && field["default"] != nil {
						t.Fatal("password default escaped into UI")
					}
				}
			}
		}
	}
	if _, err := dispatchWeb(engine, "view", M{"tab": "../../settings"}); err == nil {
		t.Fatal("invalid section accepted")
	}
	if _, err := dispatchWeb(engine, "raw", M{"path": "/api/frontier/login"}); err == nil {
		t.Fatal("arbitrary backend path exposed")
	}
	if _, err := dispatchWeb(engine, "action", M{"tab": "settings", "actionId": "/api/frontier/login"}); err == nil {
		t.Fatal("arbitrary action ID accepted")
	}
}

func TestWebBridgeNavigationHintsAndReadableDetails(t *testing.T) {
	engine := newBridgeEngine(t)
	result, err := dispatchWeb(engine, "action", M{"tab": "engineering", "mode": "Материалы", "actionId": "engineering.go-plan"})
	if err != nil || result["navigate"] != "engineering" || result["mode"] != "План" {
		t.Fatal("lost navigation mode", result, err)
	}
	view, err := dispatchWeb(engine, "view", M{"tab": "engineering", "mode": "Материалы"})
	if err != nil {
		t.Fatal(err)
	}
	page := view["page"].(p.Page)
	if len(page.Rows) == 0 {
		t.Fatal("no engineering materials")
	}
	selected := page.Rows[0].ID
	result, err = dispatchWeb(engine, "action", M{"tab": "engineering", "mode": "Материалы", "selected": selected, "actionId": "engineering.material-trade"})
	if err != nil || result["navigate"] != "engineering" || result["mode"] != "Обмен" {
		t.Fatal("exchange navigation", result, err)
	}
	if v.Str(v.At(engine.Snapshot(), "engineering", "options", "tradeTargetKey")) == "" {
		t.Fatal("exchange target was not configured")
	}
	for _, tab := range p.Tabs {
		candidate, err := dispatchWeb(engine, "view", M{"tab": tab.ID})
		if err != nil {
			t.Fatal(err)
		}
		page := candidate["page"].(p.Page)
		if len(page.Rows) == 0 {
			continue
		}
		for _, row := range page.Rows {
			actions := p.CoreActions(tab.ID, "", engine.Snapshot(), row.Data, M{})
			for _, action := range actions {
				if action.Kind != "details" || action.Disabled {
					continue
				}
				result, err := dispatchWeb(engine, "action", M{"tab": tab.ID, "selected": row.ID, "actionId": action.ID})
				if err != nil {
					t.Fatal(err)
				}
				if text, ok := result["details"].(string); !ok || text != row.Details {
					t.Fatal("details are not readable row text", result)
				}
				return
			}
		}
	}
	t.Fatal("no details action exercised")
}

func TestWebBridgeThemePreviewRequiresSeparateApply(t *testing.T) {
	engine := newBridgeEngine(t)
	before := v.Str(v.At(engine.Snapshot(), "theme", "palette", "accent"))
	file := filepath.Join(t.TempDir(), "GraphicsConfigurationOverride.xml")
	if err := os.WriteFile(file, []byte(`<GraphicsConfig><GUIColour><Default><MatrixRed>0,1,0</MatrixRed><MatrixGreen>0,0,1</MatrixGreen><MatrixBlue>1,0,0</MatrixBlue></Default></GUIColour></GraphicsConfig>`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := dispatchWeb(engine, "action", M{"tab": "settings", "actionId": "settings.import.palette", "values": M{"filePath": file}})
	if err != nil {
		t.Fatal(err)
	}
	preview := v.Obj(result["themePreview"])
	if len(v.Obj(preview["settings"])) == 0 || len(v.Obj(preview["palette"])) == 0 {
		t.Fatal("missing preview", result)
	}
	if v.Str(v.At(engine.Snapshot(), "theme", "palette", "accent")) != before {
		t.Fatal("preview changed saved palette")
	}
	if _, err = dispatchWeb(engine, "theme-apply", M{"values": preview["settings"]}); err != nil {
		t.Fatal(err)
	}
	if v.Str(v.At(engine.Snapshot(), "theme", "palette", "accent")) == before {
		t.Fatal("confirmed theme was not saved")
	}
	if _, err = dispatchWeb(engine, "action", M{"tab": "settings", "actionId": "settings.window", "values": M{"displayMode": "invalid"}}); err == nil {
		t.Fatal("invalid display mode reached backend")
	}
}

func TestWebRequestsRequireEmbeddedOriginAndBoundedSingleJSON(t *testing.T) {
	valid := `{"id":"123","op":"view","params":{"address":18446744073709551615}}`
	for _, source := range []string{"about:blank", "about:blank#lesson-anchor"} {
		request, err := decodeWebRequest(source, valid)
		if err != nil || v.Str(request.Params["address"]) != "18446744073709551615" {
			t.Fatal(request, err)
		}
	}
	for _, source := range []string{"", "https://example.com", "file:///C:/app.html", "http://localhost:7122", "about:blank?remote=1", "about:blank-evil", "https://example.com/#about:blank"} {
		if _, err := decodeWebRequest(source, valid); err == nil {
			t.Fatal("untrusted source", source)
		}
	}
	for _, message := range []string{`{}`, `{"id":"1"}`, `{"op":"view"}`, valid + `{}`, `{"id":"` + strings.Repeat("x", 81) + `","op":"view"}`, strings.Repeat(" ", 2*1024*1024+1)} {
		if _, err := decodeWebRequest("about:blank", message); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
}

func TestWebStateAndFormsDoNotExposeCredentials(t *testing.T) {
	state := M{"apiKey": "top-secret", "commander": M{"name": "Тест"}, "frontier": M{"connected": true, "access_token": "access-secret", "refreshToken": "refresh-secret"}, "integrations": M{"apiKeyConfigured": true, "apiKey": "inara-secret", "queue": []M{{"password": "hidden", "event": "CarrierStats"}}}}
	result := webState(state)
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "hidden") || !strings.Contains(string(encoded), "apiKeyConfigured") {
		t.Fatal(string(encoded))
	}
	if state["apiKey"] != "top-secret" || v.At(state, "frontier", "access_token") != "access-secret" {
		t.Fatal("redaction mutated backend")
	}
	actions := webActions([]p.Action{{ID: "key", Title: "Key", Fields: []p.Field{{Key: "apiKey", Kind: "password", Default: "do-not-send"}}}})
	if v.Rows(actions[0]["fields"])[0]["default"] != nil {
		t.Fatal("saved key rendered as password default")
	}
}

type countingPlanReader struct {
	reader io.Reader
	count  int
}

func (r *countingPlanReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.count += n
	return n, err
}
func TestNativeImportReadsAtMostTwoMiBAndRejectsWrongShape(t *testing.T) {
	reader := &countingPlanReader{reader: bytes.NewReader(bytes.Repeat([]byte{' '}, 3*1024*1024))}
	if _, err := readWebEngineeringPlan(reader); err == nil || reader.count > 2*1024*1024+1 {
		t.Fatal("unbounded import", reader.count, err)
	}
	for _, raw := range []string{`{}`, `{"items":null}`, `{"items":{}}`, `{"items":[]} {}`, `[]`, `not-json`} {
		if _, err := readWebEngineeringPlan(strings.NewReader(raw)); err == nil {
			t.Fatal("invalid plan", raw)
		}
	}
	plan, err := readWebEngineeringPlan(strings.NewReader(`{"items":[],"address":18446744073709551615}`))
	if err != nil || v.Str(plan["address"]) != "18446744073709551615" {
		t.Fatal(plan, err)
	}
}

func TestFrontendAssetsAreEmbeddedAndDoNotLoadRemoteUI(t *testing.T) {
	html := frontend.HTML("test-nonce-123", true)
	if !strings.Contains(html, "script-src 'nonce-test-nonce-123'") || !strings.Contains(html, `nonce="test-nonce-123"`) || !strings.Contains(html, "connect-src 'none'") || !strings.Contains(html, "frame-src 'none'") {
		t.Fatal("missing embedded-only policy")
	}
	for _, pattern := range []string{`(?i)<script[^>]+\bsrc\s*=`, `(?i)<link[^>]+href\s*=["']https?://`, `(?i)url\(\s*["']?https?://`, `(?i)@import\s`} {
		if regexp.MustCompile(pattern).MatchString(html) {
			t.Fatal("external UI dependency", pattern)
		}
	}
	if !strings.Contains(html, "COPILOT_SMOKE=true") || strings.Contains(html, "{{JS}}") || strings.Contains(html, "{{CSS}}") {
		t.Fatal("incomplete embedded assets")
	}
}

type desktopBridgeController struct{ platformController }

func (c *desktopBridgeController) Do(path string, body M) (M, error) {
	c.path = path
	c.last = v.Copy(body)
	if path == "/api/desktop" {
		desktop := v.Obj(c.state["desktop"])
		for key, value := range body {
			desktop[key] = value
		}
		c.state["desktop"] = desktop
	}
	return M{"ok": true}, nil
}
func TestHiddenModernWindowModesPersistAndRespectWorkArea(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer nativeTestControls(t)()
	main, err := walk.NewMainWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer main.Dispose()
	_ = main.SetLayout(walk.NewVBoxLayout())
	backend := &desktopBridgeController{platformController{state: M{"desktop": M{"displayMode": "windowed"}}}}
	u := &Window{main: main, backend: backend, state: backend.Snapshot(), modeValue: "windowed"}
	m := &modernShell{owner: u}
	u.modern = m
	if work, ok := monitorRect(main.Handle(), true); ok {
		_ = main.SetBoundsPixels(fitWindowedBounds(walk.Rectangle{X: 50, Y: 50, Width: 1000, Height: 720}, work))
	}
	for _, mode := range []string{"fullscreen", "borderless", "windowed"} {
		if _, err := m.windowAction(mode); err != nil {
			t.Fatal(err)
		}
		m.flushDesktop()
		if u.modeValue != mode || v.At(backend.state, "desktop", "displayMode") != mode {
			t.Fatal("mode not persisted", mode, u.modeValue, backend.state)
		}
	}
	if _, err := m.windowAction("toggle-fullscreen"); err != nil {
		t.Fatal(err)
	}
	m.flushDesktop()
	if u.modeValue != "fullscreen" {
		t.Fatal("F11 did not enter fullscreen")
	}
	if _, err := m.windowAction("toggle-fullscreen"); err != nil {
		t.Fatal(err)
	}
	m.flushDesktop()
	if u.modeValue != "windowed" {
		t.Fatal("F11 did not restore window")
	}
	if win.IsWindowVisible(main.Handle()) {
		t.Fatal("hidden shell test showed a window")
	}
}

func TestDesktopInstanceMutexIsSharedAndReleased(t *testing.T) {
	name := `Local\EDCopilot.InstallerSmoke.Unit-` + v.ID()
	first, acquired, err := acquireNamedDesktopInstance(name)
	if err != nil || !acquired {
		t.Fatal(acquired, err)
	}
	second, acquired, err := acquireNamedDesktopInstance(name)
	if err != nil || acquired {
		t.Fatal("duplicate instance accepted", acquired, err)
	}
	second()
	first()
	first()
	third, acquired, err := acquireNamedDesktopInstance(name)
	if err != nil || !acquired {
		t.Fatal("mutex leaked", acquired, err)
	}
	third()
}
