package presentation

import (
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// These are the transport values produced by the modern frontend after a
// Russian user's input has been normalized, before normal Go validation.
func TestModernFrontendNormalizedFields(t *testing.T) {
	for _, test := range []struct {
		field Field
		raw   string
		want  any
	}{
		{Field{Kind: "number", Min: 0, Max: 1}, "0.75", float64(0.75)},
		{Field{Kind: "integer", Min: 0, Max: 25000}, "1000", float64(1000)},
		{Field{Kind: "number"}, "", nil},
	} {
		got, err := ParseField(test.field, test.raw)
		if err != nil || got != test.want {
			t.Fatalf("kind=%s raw=%q got=%v error=%v", test.field.Kind, test.raw, got, err)
		}
	}
	for _, day := range []int{7, 18} {
		want := time.Date(2026, time.November, day, 21, 45, 0, 0, time.Local)
		raw := want.Format("2006-01-02T15:04")
		got, err := ParseField(Field{Kind: "datetime"}, raw)
		if err != nil || !v.Time(got).Equal(want) {
			t.Fatalf("local datetime %q changed: got=%v error=%v", raw, got, err)
		}
	}
}
