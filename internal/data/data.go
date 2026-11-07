package data

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/value"
	"sync"
)

//go:embed *.json *.ico *.png
var Files embed.FS
var cache sync.Map

func Load(name string) (any, error) {
	if item, ok := cache.Load(name); ok {
		return item, nil
	}
	b, e := Files.ReadFile(name)
	if e != nil {
		return nil, e
	}
	var v any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if e = d.Decode(&v); e != nil {
		return nil, fmt.Errorf("%s: %w", name, e)
	}
	actual, _ := cache.LoadOrStore(name, v)
	return actual, nil
}
func Object(name string) value.M {
	v, e := Load(name)
	if e != nil {
		panic(e)
	}
	return value.Obj(v)
}
func Array(name string) []value.M {
	v, e := Load(name)
	if e != nil {
		panic(e)
	}
	return value.Rows(v)
}
