package value

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestJSONBoundaryPreservesIDsUnknownsAndIndependentCopies(t *testing.T) {
	original := M{"address": json.Number("18446744073709551615"), "missing": nil, "zero": 0, "nested": M{"name": "Колония Ёж"}, "rows": []M{{"commodity": "steel", "amount": 1}}}
	copy := Copy(original)
	if Str(copy["address"]) != "18446744073709551615" || copy["missing"] != nil || Int(copy["zero"]) != 0 {
		t.Fatal(copy)
	}
	Obj(copy["nested"])["name"] = "Изменено"
	Rows(copy["rows"])[0]["amount"] = 2
	if At(original, "nested", "name") != "Колония Ёж" || Int(Rows(original["rows"])[0]["amount"]) != 1 {
		t.Fatal("copy mutated source", original)
	}
	if Rows(nil) != nil || Rows("not an array") != nil || len(Rows([]any{1, M{"ok": true}, nil})) != 1 {
		t.Fatal("absent collection became a confirmed empty inventory")
	}
	if At(original, "unknown", "nested") != nil || len(Obj(nil)) != 0 || len(Copy(nil)) != 0 {
		t.Fatal("missing value contract")
	}
	if Str(M{"private": "value"}) != "" || Str([]M{{"value": 1}}) != "" {
		t.Fatal("raw objects escaped into display text")
	}
	if First(nil, "", M{"safe": true}) == nil || Text(nil, "", "Система") != "Система" {
		t.Fatal("fallback contract")
	}
}

func TestNumbersCannotConvertUnknownOrNonFiniteToKnownZero(t *testing.T) {
	for _, raw := range []any{nil, "", " ", "unknown", true, math.NaN(), math.Inf(1), json.Number("Infinity")} {
		if _, known := Number(raw); known {
			t.Fatalf("invalid number accepted: %#v", raw)
		}
	}
	for _, raw := range []any{0, int64(0), uint64(0), float32(0), float64(0), "0", json.Number("0")} {
		if n, known := Number(raw); !known || n != 0 {
			t.Fatalf("known zero lost: %#v", raw)
		}
	}
	if Nullable(0, false) != nil || Nullable(0, true) != float64(0) {
		t.Fatal("unknown and zero conflated")
	}
	if Bool("false") || !Bool(true) {
		t.Fatal("boolean strings must not become opt-in")
	}
	if Key("$Gold_Name;") != "gold" || Key("Колония Ёж-1") != "колонияёж1" {
		t.Fatal("resource normalization")
	}
	if len(Arr([]float64{1, 2, 3})) != 3 || len(Arr([]int{1, 2})) != 2 || len(Arr([]string{"a"})) != 1 {
		t.Fatal("typed coordinate arrays lost")
	}
	now := time.Date(2026, 11, 8, 1, 2, 3, 123, time.FixedZone("MSK", 3*60*60))
	if !Time(ISO(now)).Equal(now) || !Time("invalid").IsZero() {
		t.Fatal("timestamps")
	}
	if Clamp(-1, 0, 2) != 0 || Clamp(3, 0, 2) != 2 {
		t.Fatal("range clamp")
	}
	if id := ID(); len(id) != 32 || id == ID() {
		t.Fatal("independent profile row identifiers")
	}
}
