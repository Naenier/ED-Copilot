package frontend

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type readmeScreenshotManifest struct {
	ApplicationVersion string `json:"applicationVersion"`
	Language           string `json:"language"`
	Data               string `json:"data"`
	DisplayMode        string `json:"displayMode"`
	Images             []struct {
		File   string `json:"file"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
		SHA256 string `json:"sha256"`
	} `json:"images"`
}

func readmeRepoFile(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}

func checkedReadmeManifest(t *testing.T, english bool) map[string]bool {
	t.Helper()
	directory, name, language := "docs/screenshots", "ru-manifest.json", "ru"
	if english {
		directory, name, language = directory+"/en", "manifest.json", "en"
	}
	body, err := os.ReadFile(readmeRepoFile(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	var manifest readmeScreenshotManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Language != language || len(manifest.Images) == 0 || len(manifest.Images) > 32 {
		t.Fatalf("%s: expected %s and a bounded nonempty gallery, got %s and %d images", name, language, manifest.Language, len(manifest.Images))
	}
	// The original Russian gallery predates capture metadata. Any replacement
	// gallery declaring that metadata must meet the same contract as English.
	fullScreen := english || manifest.Data != "" || manifest.DisplayMode != "" || manifest.ApplicationVersion != ""
	if fullScreen && (manifest.Data != "synthetic" || manifest.DisplayMode != "fullscreen" || manifest.ApplicationVersion == "") {
		t.Fatal("Documentation gallery must identify its application version, synthetic data and full-screen capture")
	}
	images := map[string]bool{}
	for _, item := range manifest.Images {
		if item.File == "" || filepath.Base(item.File) != item.File || strings.ContainsAny(item.File, `/\\`) || filepath.Ext(item.File) != ".png" || images[item.File] {
			t.Fatalf("invalid or duplicate manifest image: %q", item.File)
		}
		images[item.File] = true
		data, err := os.ReadFile(readmeRepoFile(directory, item.File))
		if err != nil {
			t.Fatal(err)
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width <= 0 || config.Height <= 0 {
			t.Fatalf("%s is not a valid nonempty PNG: %v", item.File, err)
		}
		if fullScreen && (item.Width != 1920 || item.Height != 1080 || config.Width != item.Width || config.Height != item.Height) {
			t.Fatalf("%s %s: manifest and actual PNG must both be 1920 × 1080, got %d × %d and %d × %d", language, item.File, item.Width, item.Height, config.Width, config.Height)
		}
		expected, err := hex.DecodeString(item.SHA256)
		actual := sha256.Sum256(data)
		if err != nil || len(expected) != len(actual) || !bytes.Equal(expected, actual[:]) {
			t.Fatalf("%s no longer matches its published %s SHA256", item.File, name)
		}
	}
	baseline := []string{"overview.png", "deals.png", "carrier-trade.png", "route.png", "ships.png", "ship.png", "engineer.png", "article.png", "colonization.png", "academy.png", "exobiology.png", "galaxy.png", "system.png", "engineering.png", "material-exchange.png", "material.png"}
	for _, name := range baseline {
		if !images[name] {
			t.Fatalf("%s gallery lost a baseline documentation screenshot: %s", language, name)
		}
	}
	return images
}

func TestReadmeEnglishScreenshotManifest(t *testing.T) {
	checkedReadmeManifest(t, true)
}

func TestReadmeRussianScreenshotManifest(t *testing.T) {
	checkedReadmeManifest(t, false)
}

func TestReadmeImagesExistAndKeepLanguageGalleriesSeparate(t *testing.T) {
	markdownImage := regexp.MustCompile(`!\[[^\]]*\]\(([^\s)]+)(?:\s+"[^"]*")?\)`)
	htmlImage := regexp.MustCompile(`(?i)<img\b[^>]*\bsrc\s*=\s*["']([^"']+)["']`)
	for _, english := range []bool{true, false} {
		name, directory := "README.ru.md", "docs/screenshots/"
		if english {
			name, directory = "README.md", directory+"en/"
		}
		t.Run(name, func(t *testing.T) {
			manifest := checkedReadmeManifest(t, english)
			body, err := os.ReadFile(readmeRepoFile(name))
			if err != nil {
				t.Fatal(err)
			}
			referenced := map[string]bool{}
			for _, pattern := range []*regexp.Regexp{markdownImage, htmlImage} {
				for _, match := range pattern.FindAllStringSubmatch(string(body), -1) {
					link, err := url.Parse(match[1])
					if err != nil {
						t.Fatal("invalid README image URL", match[1], err)
					}
					if link.IsAbs() || link.Host != "" {
						continue
					}
					local := link.Path
					if stat, err := os.Stat(readmeRepoFile(local)); err != nil || stat.IsDir() || stat.Size() == 0 {
						t.Fatalf("README image does not exist or is empty: %s (%v)", local, err)
					}
					// The shared HTML product mark is deliberately outside either
					// screenshot gallery. Only screenshots are language-specific.
					if !strings.HasPrefix(local, "docs/screenshots/") {
						continue
					}
					if !strings.HasPrefix(local, directory) || strings.Contains(strings.TrimPrefix(local, directory), "/") {
						t.Fatalf("%s uses an image from the wrong language gallery: %s", name, local)
					}
					file := strings.TrimPrefix(local, directory)
					if !manifest[file] {
						t.Fatalf("%s image has no checked manifest entry: %s", name, file)
					}
					referenced[file] = true
				}
			}
			for _, file := range []string{"overview.png", "commander.png", "ranks.png", "colonization.png", "academy.png", "exobiology.png", "galaxy.png", "system.png", "engineering.png", "material-exchange.png", "material.png"} {
				if !referenced[file] {
					t.Fatalf("%s omits a required product workflow screenshot: %s", name, file)
				}
			}
		})
	}
}
