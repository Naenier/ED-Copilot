// Package value provides bounded JSON values at the journal and profile boundary.
package value

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type M = map[string]any

func Obj(v any) M {
	if m, ok := v.(map[string]any); ok && m != nil {
		return m
	}
	return M{}
}
func Arr(v any) []any {
	switch a := v.(type) {
	case []any:
		return a
	case []M:
		r := make([]any, len(a))
		for i := range a {
			r[i] = a[i]
		}
		return r
	case []string:
		r := make([]any, len(a))
		for i := range a {
			r[i] = a[i]
		}
		return r
	case []float64:
		r := make([]any, len(a))
		for i := range a {
			r[i] = a[i]
		}
		return r
	case []int:
		r := make([]any, len(a))
		for i := range a {
			r[i] = a[i]
		}
		return r
	}
	return nil
}
func Rows(v any) []M {
	a := Arr(v)
	if a == nil {
		return nil
	}
	r := make([]M, 0, len(a))
	for _, v := range a {
		if m, ok := v.(M); ok {
			r = append(r, m)
		}
	}
	return r
}
func Str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	switch v.(type) {
	case map[string]any, []any, []M:
		return ""
	}
	return fmt.Sprint(v)
}
func String(m M, key string) string { return Str(m[key]) }
func Num(v any) float64             { n, _ := Number(v); return n }
func Number(v any) (float64, bool) {
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case float32:
		n = float64(x)
	case int:
		n = float64(x)
	case int64:
		n = float64(x)
	case uint64:
		n = float64(x)
	case json.Number:
		f, e := x.Float64()
		if e != nil {
			return 0, false
		}
		n = f
	case string:
		if strings.TrimSpace(x) == "" {
			return 0, false
		}
		f, e := strconv.ParseFloat(x, 64)
		if e != nil {
			return 0, false
		}
		n = f
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}
func Int(v any) int   { return int(Num(v)) }
func Bool(v any) bool { b, _ := v.(bool); return b }
func Key(v any) string {
	s := strings.TrimPrefix(Str(v), "$")
	s = strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(s), ";"), "_name")
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}
func First(v ...any) any {
	for _, x := range v {
		if x == nil {
			continue
		}
		if s, ok := x.(string); !ok || s != "" {
			return x
		}
	}
	return nil
}
func Text(v ...any) string {
	for _, x := range v {
		if s := Str(x); s != "" {
			return s
		}
	}
	return ""
}
func At(m M, path ...string) any {
	var v any = m
	for _, k := range path {
		v = Obj(v)[k]
	}
	return v
}
func Clone(v any) any {
	b, e := json.Marshal(v)
	if e != nil {
		return nil
	}
	var out any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if d.Decode(&out) != nil {
		return nil
	}
	return out
}
func Copy(m M) M { return Obj(Clone(m)) }
func Merge(dst M, src ...M) M {
	for _, m := range src {
		for k, v := range m {
			dst[k] = v
		}
	}
	return dst
}
func ISO(t time.Time) string            { return t.UTC().Format(time.RFC3339Nano) }
func Time(v any) time.Time              { t, _ := time.Parse(time.RFC3339Nano, Str(v)); return t }
func ID() string                        { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func Clamp(n, min, max float64) float64 { return math.Max(min, math.Min(max, n)) }
func Nullable(n float64, ok bool) any {
	if ok {
		return n
	}
	return nil
}
