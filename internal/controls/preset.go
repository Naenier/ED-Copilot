package controls

import (
	"bytes"
	"errors"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type presets struct {
	names    [contextCount]string
	sep      string
	count    int
	bom      bool
	trailing bool
}

func parsePresets(data []byte) (presets, error) {
	p := presets{sep: "\n"}
	if len(data) == 0 {
		return p, nil
	}
	bom := []byte{0xef, 0xbb, 0xbf}
	p.bom = bytes.HasPrefix(data, bom)
	value := string(bytes.TrimPrefix(data, bom))
	if strings.Contains(value, "\r\n") {
		p.sep = "\r\n"
	}
	p.trailing = strings.HasSuffix(value, p.sep)
	lines := strings.Split(strings.TrimSuffix(value, p.sep), p.sep)
	p.count = len(lines)
	if p.count != 1 && p.count != contextCount {
		return presets{}, errors.New("Не удалось прочитать выбор профилей Elite Dangerous.")
	}
	for i := range p.names {
		line := lines[i%p.count]
		if !validName(line) {
			return presets{}, errors.New("Не удалось прочитать имя выбранного профиля Elite Dangerous.")
		}
		p.names[i] = line
	}
	return p, nil
}

func (p presets) encode() []byte {
	lines := p.names[:]
	if p.count == 1 && p.names[0] == p.names[1] && p.names[0] == p.names[2] && p.names[0] == p.names[3] {
		lines = lines[:1]
	}
	result := strings.Join(lines, p.sep)
	if p.trailing {
		result += p.sep
	}
	if p.bom {
		result = "\uFEFF" + result
	}
	return []byte(result)
}

func activationChanges(items inventory, p *controlProfile, input M) (map[string][]byte, error) {
	if items.presetErr != nil {
		return nil, items.presetErr
	}
	var name string
	if p != nil {
		if items.matchPreset(p.name) != p {
			return nil, errors.New("Игра выбирает другую версию профиля с таким именем. Создайте копию с новым именем.")
		}
		name = p.name
	} else if v.Str(input["profileId"]) == "" {
		candidate := v.Str(input["preset"])
		if available := items.matchPreset(candidate); available != nil {
			name = available.name
		} else {
			for _, preset := range items.selected.names {
				if strings.EqualFold(candidate, preset) && validName(preset) {
					name = preset
					break
				}
			}
		}
	}
	if name == "" {
		return nil, errors.New("Профиль управления не найден.")
	}
	current := items.selected
	context := v.Str(input["context"])
	if context == "" || context == "all" {
		for i := range current.names {
			current.names[i] = name
		}
		return map[string][]byte{startFile: current.encode()}, nil
	}
	for i, available := range contexts {
		if context == available {
			if current.count == 0 {
				return nil, errors.New("Выберите профиль для всех режимов: предыдущий выбор игры отсутствует.")
			}
			current.names[i] = name
			return map[string][]byte{startFile: current.encode()}, nil
		}
	}
	return nil, errors.New("Неизвестный режим управления.")
}
