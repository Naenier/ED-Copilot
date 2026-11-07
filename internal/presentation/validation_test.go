package presentation

import (
	"math"
	"strings"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestNativeFieldValidation(t *testing.T) {
	tests := []struct {
		name      string
		field     Field
		raw, want any
		bad       bool
	}{
		{"number", Field{Kind: "number", Min: 1, Max: 100}, "12.5", 12.5, false},
		{"optional number", Field{Kind: "number"}, "", nil, false},
		{"required number", Field{Kind: "number", Required: true}, "", nil, true},
		{"minimum", Field{Kind: "number", Min: 1, Max: 100}, "0", nil, true},
		{"maximum", Field{Kind: "number", Min: 1, Max: 100}, 101, nil, true},
		{"infinity", Field{Kind: "number"}, math.Inf(1), nil, true},
		{"NaN", Field{Kind: "number"}, "NaN", nil, true},
		{"integer", Field{Kind: "integer", Min: 0, Max: 100}, "42", float64(42), false},
		{"fractional integer", Field{Kind: "integer", Min: 0, Max: 100}, "1.5", nil, true},
		{"true switch", Field{Kind: "bool"}, true, true, false},
		{"false switch", Field{Kind: "bool"}, false, false, false},
		{"required consent", Field{Kind: "bool", Required: true}, false, nil, true},
		{"typed switch", Field{Kind: "bool"}, "true", nil, true},
		{"choice ID", Field{Kind: "select", Choices: ChoicePairs("L", "Большая")}, "L", "L", false},
		{"choice label rejected", Field{Kind: "select", Choices: ChoicePairs("L", "Большая")}, "Большая", nil, true},
		{"optional choice", Field{Kind: "select"}, "", "", false},
		{"required choice", Field{Kind: "select", Required: true}, "", nil, true},
		{"editable combo", Field{Kind: "combo", Required: true}, "  Новая фракция  ", "Новая фракция", false},
		{"color", Field{Kind: "color"}, " #Aa12FF ", "#aa12ff", false},
		{"short color", Field{Kind: "color"}, "#fff", nil, true},
		{"named color", Field{Kind: "color"}, "orange", nil, true},
		{"required title", Field{Kind: "text", Required: true}, "  ", nil, true},
		{"Russian title", Field{Kind: "text", Required: true}, "  Перевезти тритий  ", "Перевезти тритий", false},
		{"multiline", Field{Kind: "multiline"}, "A\r\nB", "A\r\nB", false},
		{"rune limit", Field{Kind: "text"}, strings.Repeat("Я", 10000), strings.Repeat("Я", 10000), false},
		{"rune overflow", Field{Kind: "text"}, strings.Repeat("Я", 10001), nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseField(tc.field, tc.raw)
			if (err != nil) != tc.bad {
				t.Fatalf("value=%v error=%v", got, err)
			}
			if !tc.bad && got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestNativeDateTimeAndDisplayRoundTrip(t *testing.T) {
	for _, raw := range []string{"07.11.2026 21:45", "2026-11-07 21:45", "2026-11-07T21:45"} {
		got, err := ParseField(Field{Kind: "datetime"}, raw)
		want := time.Date(2026, 11, 7, 21, 45, 0, 0, time.Local)
		if err != nil || !v.Time(got).Equal(want) {
			t.Fatal(raw, got, err)
		}
		if display := FieldText(Field{Kind: "datetime", Default: got}); display != "07.11.2026 21:45" {
			t.Fatal(display)
		}
	}
	stamp := "2026-11-07T21:45:10.123Z"
	got, err := ParseField(Field{Kind: "datetime"}, stamp)
	if err != nil || !v.Time(got).Equal(v.Time(stamp)) {
		t.Fatal(got, err)
	}
	for _, raw := range []string{"31.02.2026 21:45", "2026-11-07", "nonsense"} {
		if _, err := ParseField(Field{Kind: "datetime"}, raw); err == nil {
			t.Fatal("invalid date accepted", raw)
		}
	}
	if got, err := ParseField(Field{Kind: "datetime"}, ""); err != nil || got != nil {
		t.Fatal(got, err)
	}
	if _, err := ParseField(Field{Kind: "datetime", Required: true}, ""); err == nil {
		t.Fatal("missing required date accepted")
	}
	if FieldText(Field{Kind: "number", Default: 2.5}) != "2.5" || FieldText(Field{Default: nil}) != "" {
		t.Fatal("field defaults corrupted")
	}
}
