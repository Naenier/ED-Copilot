// Package theme reads HUD settings without evaluating INI directives or writing
// game files. The colour transform preserves Elite's input-row matrix order.
package theme

import (
	"encoding/json"
	"errors"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ColorKeys = []string{"accent", "text", "muted", "bg", "panel", "good", "warn", "danger"}
var identity = [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
var presets = []v.M{
	{"id": "blue", "name": "Синий космос", "colors": v.M{"accent": "#28a0f0", "text": "#e0e0e0", "muted": "#8898a8", "bg": "#07101a", "panel": "#0a1622", "good": "#63d8bd", "warn": "#edbd66", "danger": "#fa8181"}},
	{"id": "orange", "name": "Оранжевый HUD", "colors": v.M{"accent": "#ff8a24", "text": "#f0e3d6", "muted": "#b7a18d", "bg": "#160d07", "panel": "#201208", "good": "#a2d572", "warn": "#ffd36a", "danger": "#ff7770"}},
	{"id": "green", "name": "Изумруд", "colors": v.M{"accent": "#42d5a4", "text": "#e0eee8", "muted": "#8eaaa0", "bg": "#06130f", "panel": "#0a1d17", "good": "#83e8ad", "warn": "#e7c66a", "danger": "#f57f8c"}},
	{"id": "purple", "name": "Фиолетовый", "colors": v.M{"accent": "#b78aff", "text": "#eae2f4", "muted": "#a396b4", "bg": "#100a19", "panel": "#190f25", "good": "#78d8c4", "warn": "#eac57a", "danger": "#fb87a9"}},
}

func Presets() []v.M {
	out := []v.M{}
	for _, row := range presets {
		next := v.Copy(row)
		next["matrix"] = identity
		out = append(out, v.Copy(next))
	}
	return out
}
func Default() v.M { m, _ := Normalize(nil); return m }
func matrix(raw any) ([][]float64, error) {
	if raw == nil {
		return [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var rows [][]float64
	if json.Unmarshal(b, &rows) != nil || len(rows) != 3 {
		return nil, errors.New("Матрица должна содержать девять чисел от −2 до 2.")
	}
	for _, row := range rows {
		if len(row) != 3 {
			return nil, errors.New("Неполная цветовая матрица.")
		}
		for _, n := range row {
			if math.IsNaN(n) || math.IsInf(n, 0) || n < -2 || n > 2 {
				return nil, errors.New("Коэффициенты матрицы должны быть от −2 до 2.")
			}
		}
	}
	return rows, nil
}
func Normalize(input v.M) (v.M, error) {
	mode, preset := v.Text(input["mode"], "preset"), v.Text(input["preset"], "blue")
	if mode != "preset" && mode != "custom" && mode != "matrix" && mode != "game" {
		return nil, errors.New("Неизвестный способ настройки палитры.")
	}
	var colors v.M
	for _, row := range presets {
		if row["id"] == preset {
			colors = v.Copy(v.Obj(row["colors"]))
		}
	}
	if colors == nil {
		return nil, errors.New("Неизвестная готовая палитра.")
	}
	valid := regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	for _, key := range ColorKeys {
		if raw, ok := v.Obj(input["colors"])[key]; ok {
			if !valid.MatchString(v.Str(raw)) {
				return nil, errors.New("Цвет должен быть записан как #RRGGBB.")
			}
			colors[key] = strings.ToLower(v.Str(raw))
		}
	}
	opacity := 0.82
	if raw, ok := input["overlayOpacity"]; ok {
		var valid bool
		opacity, valid = v.Number(raw)
		if _, stringInput := raw.(string); stringInput {
			valid = false
		}
		if !valid || opacity < 0.4 || opacity > 1 {
			return nil, errors.New("Плотность фона оверлея должна быть от 40 до 100 процентов.")
		}
	}
	coefficients, err := matrix(input["matrix"])
	if err != nil {
		return nil, err
	}
	var source any
	if mode == "game" {
		source = v.Copy(v.Obj(input["source"]))
	}
	return v.Copy(v.M{"mode": mode, "preset": preset, "colors": colors, "matrix": coefficients, "overlayOpacity": math.Round(opacity*100) / 100, "source": source}), nil
}
func rgb(hex string) []float64 {
	out := make([]float64, 3)
	if len(hex) != 7 {
		return out
	}
	for i := range out {
		n, _ := strconv.ParseUint(hex[1+i*2:3+i*2], 16, 8)
		out[i] = float64(n)
	}
	return out
}
func colour(rows []float64) string {
	out := "#"
	for _, n := range rows {
		out += fmt.Sprintf("%02x", int(math.Round(v.Clamp(n, 0, 255))))
	}
	return out
}
func Mix(a, b string, amount float64) string {
	left, right := rgb(a), rgb(b)
	for i := range left {
		left[i] = left[i]*(1-amount) + right[i]*amount
	}
	return colour(left)
}
func Transform(hex string, coefficients [][]float64) string {
	input := rgb(hex)
	out := make([]float64, 3)
	for column := 0; column < 3; column++ {
		for row := 0; row < 3; row++ {
			out[column] += input[row] * coefficients[row][column]
		}
	}
	return colour(out)
}
func matrixColors(coefficients [][]float64) v.M {
	accent := Transform("#ff7f00", coefficients)
	return v.M{"accent": accent, "text": "#e0e0e0", "muted": Mix("#a8b1ba", accent, 0.14), "bg": Mix("#07090c", accent, 0.055), "panel": Mix("#0a0d12", accent, 0.085), "good": Transform("#63d8bd", coefficients), "warn": Transform("#edbd66", coefficients), "danger": Transform("#fa8181", coefficients)}
}
func Palette(input v.M) (v.M, error) {
	settings, err := Normalize(input)
	if err != nil {
		return nil, err
	}
	colors := v.Copy(v.Obj(settings["colors"]))
	if settings["mode"] == "preset" {
		for _, row := range presets {
			if row["id"] == settings["preset"] {
				colors = v.Copy(v.Obj(row["colors"]))
			}
		}
	} else if settings["mode"] == "matrix" {
		m, _ := matrix(settings["matrix"])
		colors = matrixColors(m)
	}
	accent, bg := v.String(colors, "accent"), v.String(colors, "bg")
	v.Merge(colors, v.M{"accentBright": Mix(accent, "#ffffff", 0.33), "accentDim": Mix(accent, bg, 0.67), "faint": Mix(v.String(colors, "muted"), bg, 0.4), "panelRaised": Mix(v.String(colors, "panel"), accent, 0.07), "line": Mix(accent, bg, 0.78), "overlayOpacity": settings["overlayOpacity"]})
	return colors, nil
}
func ParseXML(raw string) ([][]float64, error) {
	if len(raw) > 1024*1024 {
		return nil, errors.New("Слишком большой файл матрицы.")
	}
	raw = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(raw, "")
	if regexp.MustCompile(`(?i)<!DOCTYPE|<!ENTITY`).MatchString(raw) {
		return nil, errors.New("XML-объявления не поддерживаются.")
	}
	for _, tag := range []string{"GUIColour", "Default"} {
		matches := regexp.MustCompile(`(?is)<` + tag + `\b[^>]*>(.*?)</` + tag + `>`).FindStringSubmatch(raw)
		if len(matches) > 1 {
			raw = matches[1]
		}
	}
	if !regexp.MustCompile(`(?i)<Matrix(?:Red|Green|Blue)\b`).MatchString(raw) {
		return nil, nil
	}
	rows := [][]float64{}
	for _, channel := range []string{"Red", "Green", "Blue"} {
		matches := regexp.MustCompile(`(?is)<Matrix` + channel + `\b[^>]*>([^<]+)</Matrix` + channel + `>`).FindStringSubmatch(raw)
		if len(matches) != 2 {
			return nil, errors.New("Неполная цветовая матрица.")
		}
		parts := regexp.MustCompile(`[\s,]+`).Split(strings.TrimSpace(matches[1]), -1)
		row := []float64{}
		for _, part := range parts {
			n, err := strconv.ParseFloat(part, 64)
			if err != nil {
				return nil, errors.New("Некорректный коэффициент матрицы.")
			}
			row = append(row, n)
		}
		rows = append(rows, row)
	}
	return matrix(rows)
}
func parseINI(raw string) map[string]float64 {
	result := map[string]float64{}
	inConstants := false
	assignment := regexp.MustCompile(`(?i)^\$?([xyzw]\d+)\s*=\s*([-+]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[-+]?\d+)?)\s*$`)
	for _, line := range strings.Split(strings.TrimPrefix(raw, "\ufeff"), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, ";", 2)[0])
		if strings.HasPrefix(line, "[") {
			inConstants = strings.EqualFold(strings.TrimSpace(strings.SplitN(strings.TrimPrefix(line, "["), "]", 2)[0]), "constants")
			continue
		}
		if !inConstants {
			continue
		}
		matches := assignment.FindStringSubmatch(line)
		if len(matches) == 3 {
			n, err := strconv.ParseFloat(matches[2], 64)
			if err == nil && !math.IsInf(n, 0) && !math.IsNaN(n) {
				result[strings.ToLower(matches[1])] = n
			}
		}
	}
	return result
}
func ParseEDHMMatrix(raw string) ([][]float64, error) {
	data := parseINI(raw)
	rows := [][]float64{}
	anyFound := false
	for i := 150; i <= 152; i++ {
		row := []float64{}
		for _, axis := range []string{"x", "y", "z"} {
			n, found := data[axis+strconv.Itoa(i)]
			anyFound = anyFound || found
			if !found {
				n = math.NaN()
			}
			row = append(row, n)
		}
		rows = append(rows, row)
	}
	if !anyFound {
		return nil, nil
	}
	return matrix(rows)
}
func LinearToSRGB(n float64) float64 {
	n = v.Clamp(n, 0, 1)
	if n <= 0.0031308 {
		return n * 12.92
	}
	return 1.055*math.Pow(n, 1/2.4) - 0.055
}
func iniColor(data map[string]float64, id int) (string, bool) {
	result := []float64{}
	for _, axis := range []string{"x", "y", "z"} {
		n, ok := data[axis+strconv.Itoa(id)]
		if !ok || n < 0 || n > 8 {
			return "", false
		}
		result = append(result, LinearToSRGB(n)*255)
	}
	return colour(result), true
}
func ParseEDHMColors(startupRaw, advancedRaw string, coefficients [][]float64) (v.M, []string) {
	if coefficients == nil {
		coefficients = identity
	}
	startup, advanced := parseINI(startupRaw), parseINI(advancedRaw)
	colors := matrixColors(coefficients)
	imported := []string{}
	take := func(role string, enabled bool, id int) bool {
		color, valid := iniColor(advanced, id)
		if enabled && valid {
			colors[role] = color
			imported = append(imported, role)
			return true
		}
		return false
	}
	if !take("accent", startup["w206"] == 100, 208) {
		take("accent", startup["x137"] == 100, 232)
	}
	take("text", startup["w75"] == 100, 77)
	if startup["y78"] == 100 {
		if secondary, ok := iniColor(advanced, 82); ok {
			colors["muted"] = Mix(secondary, v.String(colors, "bg"), 0.26)
			imported = append(imported, "muted")
		}
	}
	take("warn", startup["y104"] == 100, 172)
	take("good", startup["y139"] == 100, 246)
	take("danger", startup["y139"] == 100, 247)
	colors["bg"] = Mix("#07090c", v.String(colors, "accent"), 0.055)
	colors["panel"] = Mix("#0a0d12", v.String(colors, "accent"), 0.085)
	return colors, imported
}

type ImportOptions struct {
	FilePath, HomeDir, GameDir string
	Now                        func() time.Time
}
type configFile struct {
	Path, Raw string
	Modified  time.Time
}

func readConfig(file string, optional bool) (*configFile, error) {
	info, err := os.Stat(file)
	if err != nil {
		if optional && (os.IsNotExist(err) || os.IsPermission(err)) {
			return nil, nil
		}
		return nil, errors.New("Не удалось прочитать файл палитры.")
	}
	if !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return nil, errors.New("Файл палитры недоступен или превышает 1 МБ.")
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, errors.New("Не удалось прочитать файл палитры.")
	}
	return &configFile{file, strings.TrimPrefix(string(b), "\ufeff"), info.ModTime()}, nil
}
func importEDHM(dir string, now time.Time) (v.M, error) {
	profile, err := readConfig(filepath.Join(dir, "XML-Profile.ini"), true)
	if err != nil || profile == nil {
		return nil, err
	}
	m, err := ParseEDHMMatrix(profile.Raw)
	if err != nil || m == nil {
		return nil, err
	}
	startup, err := readConfig(filepath.Join(dir, "Startup-Profile.ini"), true)
	if err != nil {
		return nil, err
	}
	advanced, err := readConfig(filepath.Join(dir, "Advanced.ini"), true)
	if err != nil {
		return nil, err
	}
	selected, _ := readConfig(filepath.Join(dir, "ThemeSettings.json"), true)
	name := "Цвета EDHM"
	if selected != nil {
		var d v.M
		if json.Unmarshal([]byte(selected.Raw), &d) == nil {
			name = v.Text(v.At(d, "credits", "theme"), name)
		}
	}
	colors, imported := matrixColors(m), []string{}
	if startup != nil && advanced != nil {
		colors, imported = ParseEDHMColors(startup.Raw, advanced.Raw, m)
	}
	paths := []string{profile.Path}
	modified := profile.Modified
	for _, file := range []*configFile{startup, advanced} {
		if file != nil {
			paths = append(paths, file.Path)
			if modified.Before(file.Modified) {
				modified = file.Modified
			}
		}
	}
	note := "Прочитана матрица EDHM. Индивидуальные цвета элементов не найдены."
	if len(imported) > 0 {
		note = "Основные цвета прочитаны из EDHM; фон адаптирован для читаемости."
	}
	source := v.M{"kind": "edhm", "name": name, "path": dir, "files": paths, "modifiedAt": v.ISO(modified), "importedAt": v.ISO(now), "notes": []string{note, "После смены темы игры повторите импорт цветов."}}
	settings, err := Normalize(v.M{"mode": "game", "preset": "blue", "colors": colors, "matrix": m, "source": source})
	if err != nil {
		return nil, err
	}
	palette, _ := Palette(settings)
	return v.M{"settings": settings, "palette": palette, "source": source}, nil
}
func importXML(file string, now time.Time, optional bool) (v.M, error) {
	data, err := readConfig(file, optional)
	if err != nil || data == nil {
		return nil, err
	}
	m, err := ParseXML(data.Raw)
	if err != nil || m == nil {
		return nil, err
	}
	source := v.M{"kind": "xml", "name": "Матрица HUD Elite Dangerous", "path": file, "files": []string{file}, "modifiedAt": v.ISO(data.Modified), "importedAt": v.ISO(now), "notes": []string{"Основной цвет рассчитан по матрице HUD. Текст и фон адаптированы для читаемости."}}
	settings, err := Normalize(v.M{"mode": "game", "preset": "blue", "colors": matrixColors(m), "matrix": m, "source": source})
	if err != nil {
		return nil, err
	}
	palette, _ := Palette(settings)
	return v.M{"settings": settings, "palette": palette, "source": source}, nil
}
func gameDirectories(home string) []string {
	steamRoots := []string{}
	for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), "C:/Program Files (x86)"} {
		if base != "" {
			steamRoots = append(steamRoots, filepath.Join(base, "Steam"))
		}
	}
	libraries := append([]string{}, steamRoots...)
	for _, root := range steamRoots {
		if file, _ := readConfig(filepath.Join(root, "steamapps/libraryfolders.vdf"), true); file != nil {
			for _, match := range regexp.MustCompile(`"path"\s+"([^"\r\n]+)"`).FindAllStringSubmatch(file.Raw, 64) {
				libraries = append(libraries, strings.ReplaceAll(match[1], `\\`, `\`))
			}
		}
	}
	out := []string{}
	for _, library := range libraries {
		for _, product := range []string{"elite-dangerous-odyssey-64", "elite-dangerous-64"} {
			out = append(out, filepath.Join(library, "steamapps/common/Elite Dangerous/Products", product))
		}
	}
	for _, root := range []string{filepath.Join(home, "AppData/Local/Frontier_Developments/Products"), "C:/Program Files (x86)/Frontier/EDLaunch/Products", "C:/Program Files/Frontier/EDLaunch/Products"} {
		for _, product := range []string{"elite-dangerous-odyssey-64", "elite-dangerous-64"} {
			out = append(out, filepath.Join(root, product))
		}
	}
	return out
}
func ReadGamePalette(filePath string) (v.M, error) { return Import(ImportOptions{FilePath: filePath}) }
func Import(options ImportOptions) (v.M, error) {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.HomeDir == "" {
		options.HomeDir, _ = os.UserHomeDir()
	}
	now := options.Now()
	if options.FilePath != "" {
		file, err := filepath.Abs(options.FilePath)
		if err != nil {
			return nil, err
		}
		base := strings.ToLower(filepath.Base(file))
		var result v.M
		switch base {
		case "xml-profile.ini", "advanced.ini", "startup-profile.ini", "themesettings.json":
			result, err = importEDHM(filepath.Dir(file), now)
		case "graphicsconfigurationoverride.xml", "graphicsconfiguration.xml":
			result, err = importXML(file, now, false)
		default:
			return nil, errors.New("Выберите XML-Profile.ini из EDHM или GraphicsConfigurationOverride.xml.")
		}
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, errors.New("В выбранном файле не найдена полная матрица HUD.")
		}
		return result, nil
	}
	dirs := gameDirectories(options.HomeDir)
	if options.GameDir != "" {
		dirs = []string{options.GameDir}
	}
	for _, dir := range dirs {
		result, err := importEDHM(filepath.Join(dir, "EDHM-ini"), now)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil
		}
	}
	for _, local := range []string{filepath.Join(options.HomeDir, "AppData/Local"), os.Getenv("LOCALAPPDATA")} {
		if local == "" {
			continue
		}
		result, err := importXML(filepath.Join(local, "Frontier Developments/Elite Dangerous/Options/Graphics/GraphicsConfigurationOverride.xml"), now, true)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil
		}
	}
	for _, dir := range dirs {
		result, err := importXML(filepath.Join(dir, "GraphicsConfiguration.xml"), now, true)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil
		}
	}
	return nil, errors.New("Матрица игры не найдена. Укажите GraphicsConfigurationOverride.xml или XML-Profile.ini вручную.")
}
