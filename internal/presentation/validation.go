package presentation

import (
	"errors"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"math"
	"regexp"
	"strings"
	"time"
)

func ParseField(field Field, raw any) (any, error) {
	text := strings.TrimSpace(v.Str(raw))
	switch field.Kind {
	case "bool":
		value, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("%s: требуется переключатель", field.Label)
		}
		if field.Required && !value {
			return nil, fmt.Errorf("%s: подтвердите отметку", field.Label)
		}
		return value, nil
	case "number", "integer":
		if text == "" && !field.Required {
			return nil, nil
		}
		n, ok := v.Number(raw)
		if !ok {
			return nil, fmt.Errorf("%s: укажите число", field.Label)
		}
		if field.Kind == "integer" && n != math.Trunc(n) {
			return nil, fmt.Errorf("%s: требуется целое число", field.Label)
		}
		if field.Max > field.Min && (n < field.Min || n > field.Max) {
			return nil, fmt.Errorf("%s: от %g до %g", field.Label, field.Min, field.Max)
		}
		return n, nil
	case "datetime":
		if text == "" && !field.Required {
			return nil, nil
		}
		for _, format := range []string{time.RFC3339Nano, "02.01.2006 15:04", "2006-01-02 15:04", "2006-01-02T15:04"} {
			var t time.Time
			var e error
			if format == time.RFC3339Nano {
				t, e = time.Parse(format, text)
			} else {
				t, e = time.ParseInLocation(format, text, time.Local)
			}
			if e == nil {
				return v.ISO(t), nil
			}
		}
		return nil, fmt.Errorf("%s: используйте ДД.ММ.ГГГГ ЧЧ:ММ", field.Label)
	case "color":
		if !regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`).MatchString(text) {
			return nil, fmt.Errorf("%s: цвет в формате #RRGGBB", field.Label)
		}
		return strings.ToLower(text), nil
	case "select":
		if text == "" && !field.Required {
			return "", nil
		}
		for _, c := range field.Choices {
			if c.Value == text {
				return text, nil
			}
		}
		return nil, fmt.Errorf("%s: выберите значение из списка", field.Label)
	default:
		if field.Required && text == "" {
			return nil, fmt.Errorf("Заполните поле «%s»", field.Label)
		}
		if len([]rune(text)) > 10000 {
			return nil, errors.New("Слишком длинный текст")
		}
		return text, nil
	}
}
func FieldText(field Field) string {
	if field.Kind == "datetime" {
		t := v.Time(field.Default)
		if !t.IsZero() {
			return t.Local().Format("02.01.2006 15:04")
		}
	}
	return v.Str(field.Default)
}
