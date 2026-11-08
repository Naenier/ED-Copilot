package frontend

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image/png"
	"math"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/data"
)

func TestRankInsigniaReturnsOnlyTheVerifiedOfflineAssets(t *testing.T) {
	proof, err := data.Files.ReadFile("rank-insignia-source.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Assets []struct {
			Discipline string `json:"discipline"`
			RankIndex  int    `json:"rankIndex"`
			Kind       string `json:"kind"`
			File       string `json:"file"`
			SHA256     string `json:"sha256"`
			Bytes      int    `json:"bytes"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
		} `json:"assets"`
	}
	if err = json.Unmarshal(proof, &manifest); err != nil || len(manifest.Assets) != 86 {
		t.Fatal("rank art provenance is incomplete", err)
	}
	for _, asset := range manifest.Assets {
		input := map[string]any{"entries": []any{map[string]any{"id": asset.Discipline, "rankIndex": asset.RankIndex}}}
		result, err := RankInsignia(input)
		if err != nil {
			t.Fatal(asset.File, err)
		}
		images := result["images"].(map[string]any)
		if len(images) != 1 {
			t.Fatal("response included unsolicited art", asset.File)
		}
		for _, raw := range images {
			uri := raw.(string)
			if !strings.HasPrefix(uri, "data:image/png;base64,") {
				t.Fatal("art must remain local PNG", asset.File)
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "data:image/png;base64,"))
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(decoded)
			config, err := png.DecodeConfig(bytes.NewReader(decoded))
			if err != nil || len(decoded) != asset.Bytes || hex.EncodeToString(hash[:]) != asset.SHA256 || config.Width != asset.Width || config.Height != asset.Height {
				t.Fatal("extracted asset differs from the verified original", asset.File, err)
			}
			original, err := data.Files.ReadFile(asset.File)
			if err != nil || !bytes.Equal(original, decoded) {
				t.Fatal("response and embedded asset differ", asset.File, err)
			}
		}
	}
}

func TestRankInsigniaValidatesEveryEntryBeforeReadingOnlyWhitelistedFiles(t *testing.T) {
	entry := func(id string, index any) map[string]any { return map[string]any{"id": id, "rankIndex": index} }
	for _, input := range []map[string]any{
		nil, {}, {"entries": "combat/0"}, {"entries": []any{}}, {"entries": make([]any, 17)},
		{"entries": []any{"combat"}}, {"entries": []any{entry("../../copilot", 0)}},
		{"entries": []any{entry("Combat", 0)}}, {"entries": []any{entry("combat", "0")}},
		{"entries": []any{entry("combat", true)}}, {"entries": []any{entry("combat", nil)}},
		{"entries": []any{entry("combat", -1)}}, {"entries": []any{entry("combat", 14)}},
		{"entries": []any{entry("empire", 15)}}, {"entries": []any{entry("combat", 1.5)}},
		{"entries": []any{entry("combat", math.NaN())}}, {"entries": []any{entry("combat", math.Inf(1))}},
		{"entries": []any{entry("combat", 0), entry("copilot.png", 0)}},
	} {
		if result, err := RankInsignia(input); err == nil || result != nil {
			t.Fatal("untrusted insignia selection was accepted", input)
		}
	}
	for _, index := range []any{0, int64(0), float64(0), json.Number("0")} {
		result, err := RankInsignia(map[string]any{"entries": []map[string]any{entry("combat", index), entry("federation", 14)}})
		if err != nil || len(result["images"].(map[string]any)) != 2 {
			t.Fatal("valid typed numeric indices rejected", index, err)
		}
	}
	result, err := RankInsignia(map[string]any{"entries": []any{entry("empire", 0), entry("empire", 14)}})
	if err != nil || len(result["images"].(map[string]any)) != 1 {
		t.Fatal("naval crest should be shared across known faction ranks", err)
	}
}
