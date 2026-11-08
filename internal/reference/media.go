package reference

import (
	"embed"
	"encoding/base64"
	"strings"
)

//go:embed media/*
var mediaFiles embed.FS

func (s *Store) image(key string, thumbnail bool, alt string) *Image {
	if key == "" {
		return nil
	}
	name := key + ".jpg"
	fit := "cover"
	if strings.HasPrefix(key, "ship-") {
		fit = "contain"
	}
	if thumbnail {
		name = "thumb-" + name
	}
	if found, ok := s.media.Load(name); ok {
		return &Image{Src: found.(string), Alt: alt, Fit: fit}
	}
	raw, err := mediaFiles.ReadFile("media/" + name)
	if err != nil {
		return nil
	}
	src := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)
	actual, _ := s.media.LoadOrStore(name, src)
	return &Image{Src: actual.(string), Alt: alt, Fit: fit}
}
