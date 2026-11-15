package controls

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type M = v.M

type node struct {
	name                    xml.Name
	attrs                   []xml.Attr
	children                []*node
	start, open, close, end int
}

func (n *node) attr(name string) string {
	for _, attr := range n.attrs {
		if attr.Name.Space == "" && attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func (n *node) child(name string) *node {
	for _, child := range n.children {
		if child.name.Space == "" && child.name.Local == name {
			return child
		}
	}
	return nil
}

func parseXML(data []byte) (*node, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var stack []*node
	var root *node
	for {
		start := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("Не удалось прочитать XML профиля управления.")
		}
		end := int(decoder.InputOffset())
		switch t := token.(type) {
		case xml.StartElement:
			n := &node{name: t.Name, attrs: t.Attr, start: start, open: end}
			if len(stack) == 0 {
				if root != nil || n.name.Space != "" || n.name.Local != "Root" {
					return nil, errors.New("Неверный корневой элемент профиля управления.")
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			n := stack[len(stack)-1]
			n.close, n.end = start, end
			stack = stack[:len(stack)-1]
		case xml.Directive:
			return nil, errors.New("Директивы XML в профиле управления не поддерживаются.")
		case xml.CharData:
			if len(stack) == 0 && strings.TrimSpace(strings.TrimPrefix(string(t), "\ufeff")) != "" {
				return nil, errors.New("Лишние данные за пределами профиля управления.")
			}
		}
	}
	if root == nil || len(stack) != 0 || !validName(root.attr("PresetName")) {
		return nil, errors.New("В профиле отсутствует допустимое имя.")
	}
	return root, nil
}

type keyBinding struct {
	device, key string
}

func (n *node) binding() keyBinding {
	return keyBinding{n.attr("Device"), n.attr("Key")}
}

var slotTags = []struct{ id, tag string }{
	{"primary", "Primary"}, {"secondary", "Secondary"}, {"axis", "Binding"},
}

func actionViews(root *node) []M {
	actions := make([]M, 0, len(root.children))
	for _, a := range root.children {
		if a.name.Space != "" {
			continue
		}
		slots := []M{}
		kind := "button"
		for _, slot := range slotTags {
			n := a.child(slot.tag)
			if n == nil {
				continue
			}
			if slot.id == "axis" {
				kind = "axis"
			}
			b := n.binding()
			mods := []M{}
			for _, child := range n.children {
				if child.name.Space == "" && child.name.Local == "Modifier" {
					mod := child.binding()
					mods = append(mods, M{"device": mod.device, "key": mod.key})
				}
			}
			row := M{"id": slot.id, "device": b.device, "key": b.key,
				"modifiers": mods, "isSet": b.device != "{NoDevice}" && b.device != "" && b.key != ""}
			if hold := n.child("Hold"); hold != nil {
				row["hold"] = hold.attr("Value") == "1"
			}
			slots = append(slots, row)
		}
		if len(slots) == 0 {
			continue
		}
		row := M{"id": a.name.Local, "kind": kind, "slots": slots}
		if n := a.child("Inverted"); n != nil {
			row["inverted"] = n.attr("Value") == "1"
		}
		if n := a.child("Deadzone"); n != nil {
			if value, ok := v.Number(n.attr("Value")); ok {
				row["deadzone"] = value
			}
		}
		if n := a.child("ToggleOn"); n != nil {
			row["toggle"] = n.attr("Value") == "1"
		}
		actions = append(actions, row)
	}
	return actions
}

var attrToken = regexp.MustCompile(`[\t\r\n ]+([^\t\r\n =<>/]+)[\t\r\n ]*=[\t\r\n ]*("[^"]*"|'[^']*')`)

func escaped(text string) []byte {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(text))
	return out.Bytes()
}

func patchTag(raw []byte, attrs ...string) []byte {
	out := append([]byte(nil), raw...)
	for i := 0; i < len(attrs); i += 2 {
		name, value := attrs[i], escaped(attrs[i+1])
		found := false
		for _, match := range attrToken.FindAllSubmatchIndex(out, -1) {
			if string(out[match[2]:match[3]]) == name {
				out = replaceBytes(out, match[4]+1, match[5]-1, value)
				found = true
				break
			}
		}
		if !found {
			end := len(out) - 1
			if out[end-1] == '/' {
				end--
			}
			add := append([]byte(" "+name+"=\""), value...)
			out = replaceBytes(out, end, end, append(add, '"'))
		}
	}
	return out
}

func replaceBytes(data []byte, start, end int, value []byte) []byte {
	out := make([]byte, 0, len(data)-(end-start)+len(value))
	out = append(out, data[:start]...)
	out = append(out, value...)
	return append(out, data[end:]...)
}

type byteEdit struct {
	start, end int
	value      []byte
}

func applyEdits(data []byte, edits []byteEdit) []byte {
	if len(edits) == 0 {
		return data
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	out := make([]byte, 0, len(data))
	pos := 0
	for _, edit := range edits {
		out = append(out, data[pos:edit.start]...)
		out = append(out, edit.value...)
		pos = edit.end
	}
	return append(out, data[pos:]...)
}

func rewriteName(data []byte, root *node, name string, builtin bool) []byte {
	attrs := []string{"PresetName", name}
	if builtin {
		attrs = append(attrs, "MajorVersion", "4", "MinorVersion", "2")
	}
	return replaceBytes(data, root.start, root.open, patchTag(data[root.start:root.open], attrs...))
}

func patchProfile(data []byte, root *node, changes []M) ([]byte, error) {
	if len(changes) > 2500 {
		return nil, errors.New("Слишком много изменений управления.")
	}
	actions := map[string]*node{}
	for _, a := range root.children {
		if a.name.Space != "" {
			continue
		}
		if _, exists := actions[a.name.Local]; exists {
			actions[a.name.Local] = nil
		} else {
			actions[a.name.Local] = a
		}
	}
	seenSlots, seenTuning := map[*node]bool{}, map[*node]string{}
	edits := []byteEdit{}
	for _, change := range changes {
		action := actions[v.Str(change["action"])]
		if action == nil {
			return nil, errors.New("Действие управления не найдено в профиле.")
		}
		slotID := v.Str(change["slot"])
		var n *node
		for _, slot := range slotTags {
			if slot.id == slotID {
				n = action.child(slot.tag)
				break
			}
		}
		if n == nil || seenSlots[n] {
			return nil, errors.New("Некорректный или повторный слот управления.")
		}
		seenSlots[n] = true
		slotEdits, err := patchSlot(data, n, change)
		if err != nil {
			return nil, err
		}
		edits = append(edits, slotEdits...)
		for _, t := range []struct{ field, tag string }{
			{"inverted", "Inverted"}, {"deadzone", "Deadzone"}, {"toggle", "ToggleOn"},
		} {
			raw, supplied := change[t.field]
			if !supplied {
				continue
			}
			setting := action.child(t.tag)
			if setting == nil || (t.field == "toggle") == (slotID == "axis") {
				return nil, errors.New("Эта настройка недоступна для выбранного слота.")
			}
			value, ok := tune(t.field, raw)
			if !ok {
				return nil, errors.New("Некорректное значение настройки управления.")
			}
			if previous, seen := seenTuning[setting]; seen {
				if previous != value {
					return nil, errors.New("Для настройки указаны разные значения.")
				}
				continue
			}
			seenTuning[setting] = value
			edits = append(edits, byteEdit{setting.start, setting.open,
				patchTag(data[setting.start:setting.open], "Value", value)})
		}
	}
	return applyEdits(data, edits), nil
}

func tune(field string, raw any) (string, bool) {
	if field == "deadzone" {
		n, ok := v.Number(raw)
		return strconv.FormatFloat(n, 'f', 8, 64), ok && n >= 0 && n <= 1
	}
	flag, ok := raw.(bool)
	if flag {
		return "1", ok
	}
	return "0", ok
}

func validBinding(b keyBinding) bool {
	if b.device == "{NoDevice}" && b.key == "" {
		return true
	}
	safe := func(text string) bool {
		return text != "" && len(text) <= 160 && strings.TrimSpace(text) == text &&
			!strings.ContainsAny(text, "\x00\r\n\t<>")
	}
	return b.device != "{NoDevice}" && safe(b.device) && safe(b.key)
}

func patchSlot(data []byte, n *node, change M) ([]byteEdit, error) {
	b := n.binding()
	device, hasDevice := change["device"]
	key, hasKey := change["key"]
	if hasDevice != hasKey {
		return nil, errors.New("Укажите устройство и клавишу вместе.")
	}
	if hasDevice {
		var deviceOK, keyOK bool
		b.device, deviceOK = device.(string)
		b.key, keyOK = key.(string)
		if !deviceOK || !keyOK {
			return nil, errors.New("Некорректное устройство или клавиша.")
		}
		if b.device == "" && b.key == "" {
			b.device = "{NoDevice}"
		}
		if !validBinding(b) {
			return nil, errors.New("Некорректное устройство или клавиша.")
		}
	}
	open := data[n.start:n.open]
	tag := open
	if hasDevice && b != n.binding() {
		tag = patchTag(open, "Device", b.device, "Key", b.key)
	}
	edits := []byteEdit{{n.start, n.open, tag}}
	raw, explicit := change["modifiers"]
	mods := v.Rows(raw)
	if explicit && (v.Arr(raw) == nil || len(mods) != len(v.Arr(raw)) || len(mods) > 6 || b.device == "{NoDevice}" && len(mods) != 0) {
		return nil, errors.New("Некорректный список модификаторов.")
	}
	var items bytes.Buffer
	if explicit || hasDevice && b.device == "{NoDevice}" {
		existing := []*node{}
		for _, child := range n.children {
			if child.name.Space == "" && child.name.Local == "Modifier" {
				existing = append(existing, child)
			}
		}
		for i, mod := range mods {
			m := keyBinding{v.Str(mod["device"]), v.Str(mod["key"])}
			if !validBinding(m) || m.device == "{NoDevice}" {
				return nil, errors.New("Некорректный модификатор управления.")
			}
			if i < len(existing) {
				child := existing[i]
				if m != child.binding() {
					edits = append(edits, byteEdit{child.start, child.open,
						patchTag(data[child.start:child.open], "Device", m.device, "Key", m.key)})
				}
			} else {
				fmt.Fprintf(&items, "<Modifier Device=\"%s\" Key=\"%s\" />", escaped(m.device), escaped(m.key))
			}
		}
		for _, child := range existing[min(len(mods), len(existing)):] {
			edits = append(edits, byteEdit{child.start, child.end, nil})
		}
	}
	if value, present := change["hold"]; present {
		hold, ok := value.(bool)
		if !ok || n.name.Local == "Binding" || hold && (b.device == "{NoDevice}" || b.key == "") {
			return nil, errors.New("Удержание недоступно для выбранной привязки.")
		}
		if child := n.child("Hold"); child != nil {
			state, _ := tune("hold", hold)
			edits = append(edits, byteEdit{child.start, child.open, patchTag(data[child.start:child.open], "Value", state)})
		} else if hold {
			items.WriteString(`<Hold Value="1" />`)
		}
	}
	if items.Len() == 0 {
		return edits, nil
	}
	if n.close == n.open && n.end == n.open {
		tag = append(append([]byte(nil), bytes.TrimSpace(tag[:len(tag)-2])...), '>')
		tag = append(tag, items.Bytes()...)
		tag = append(tag, ("</" + n.name.Local + ">")...)
		edits[0].value = tag
	} else {
		edits = append(edits, byteEdit{n.close, n.close, items.Bytes()})
	}
	return edits, nil
}
