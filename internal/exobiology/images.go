package exobiology

import (
	"encoding/base64"
	"strings"
	"sync"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

var photoCache sync.Map

// SpeciesImage loads only a requested, exactly matched species photograph.
// Biological snapshots and the game overlay never carry image bytes.
func SpeciesImage(id string) M {
	id = strings.TrimPrefix(id, "exo/")
	if cached, ok := photoCache.Load(id); ok {
		return v.Copy(v.Obj(cached))
	}
	metadata := v.Obj(data.Object("exobiology-photos.json")[id])
	file := v.Str(metadata["file"])
	if file == "" || !strings.HasPrefix(file, "exobiology-photos/") || strings.Contains(file, "..") {
		return nil
	}
	bytes, err := data.Files.ReadFile(file)
	if err != nil || len(bytes) > 256*1024 {
		return nil
	}
	photo := M{"src": "data:image/webp;base64," + base64.StdEncoding.EncodeToString(bytes), "alt": metadata["alt"], "fit": "cover", "variant": metadata["variant"]}
	photoCache.Store(id, photo)
	return v.Copy(photo)
}
