// Package localization owns the immutable UI dictionaries shared by Go and WebView.
// A language is always explicit: simultaneous snapshots never share mutable locale.
package localization

import (
	"embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

//go:embed *_en.json
var files embed.FS

var once sync.Once
var dictionary map[string]string
var folded map[string]string

type phraseNode struct {
	children map[byte]*phraseNode
	value    string
	source   string
}

var root phraseNode
var catalog string

type formatPattern struct {
	expression *regexp.Regexp
	anchor     string
	literals   []string
}

var templates []formatPattern
var printfDirective = regexp.MustCompile(`%(?:\[[0-9]+\])?[+#0 -]*[0-9]*(?:\.[0-9]+)?[sdvfwg]`)

func Normalize(language string) string {
	if strings.EqualFold(strings.TrimSpace(language), "en") {
		return "en"
	}
	return "ru"
}

func load() {
	once.Do(func() {
		dictionary, folded = map[string]string{}, map[string]string{}
		entries, err := files.ReadDir(".")
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			body, err := files.ReadFile(entry.Name())
			if err != nil {
				panic(err)
			}
			var values map[string]string
			if err := json.Unmarshal(body, &values); err != nil {
				panic(err)
			}
			for source, translation := range values {
				if source == "" || translation == "" {
					panic("empty UI translation: " + entry.Name())
				}
				dictionary[source] = translation
			}
		}
		keys := make([]string, 0, len(dictionary))
		for key := range dictionary {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		root.children = map[byte]*phraseNode{}
		for _, source := range keys {
			translation := dictionary[source]
			folded[strings.ToLower(source)] = translation
			if !containsCyrillic(source) {
				continue
			}
			if parts := printfDirective.Split(source, -1); len(parts) > 1 {
				translatedParts := printfDirective.Split(translation, -1)
				if len(parts) == len(translatedParts) {
					expression, anchor := "^", ""
					for i, part := range parts {
						expression += regexp.QuoteMeta(part)
						if i < len(parts)-1 {
							expression += `(.+?)`
						}
						if len(part) > len(anchor) && containsCyrillic(part) {
							anchor = part
						}
					}
					if len(anchor) >= 4 {
						templates = append(templates, formatPattern{regexp.MustCompile(expression + "$"), anchor, translatedParts})
					}
				}
			}
			node := &root
			for i := 0; i < len(source); i++ {
				if node.children == nil {
					node.children = map[byte]*phraseNode{}
				}
				next := node.children[source[i]]
				if next == nil {
					next = &phraseNode{}
					node.children[source[i]] = next
				}
				node = next
			}
			node.value, node.source = translation, source
		}
		body, err := json.Marshal(dictionary)
		if err != nil {
			panic(err)
		}
		catalog = string(body)
	})
}

func CatalogJSON() string { load(); return catalog }

// T translates an explicit UI string; it does not inspect arbitrary model data.
func T(language, source string) string {
	if Normalize(language) != "en" || source == "" {
		return source
	}
	load()
	if translated, ok := dictionary[source]; ok {
		return translated
	}
	trimmed := strings.TrimSpace(source)
	if translated, ok := dictionary[trimmed]; ok {
		return source[:len(source)-len(strings.TrimLeftFunc(source, unicode.IsSpace))] + translated + source[len(strings.TrimRightFunc(source, unicode.IsSpace)):]
	}
	if translated, ok := folded[strings.ToLower(trimmed)]; ok && strings.ToUpper(trimmed) == trimmed {
		return strings.ToUpper(translated)
	}
	return source
}

// Text translates composed UI prose using bounded phrase replacement. Callers
// must use TextProtected when prose also contains commander-entered identities.
func Text(language, source string) string {
	if Normalize(language) != "en" || source == "" {
		return source
	}
	if translated := T(language, source); translated != source {
		return translated
	}
	if !containsCyrillic(source) {
		return source
	}
	load()
	for _, pattern := range templates {
		if !strings.Contains(source, pattern.anchor) {
			continue
		}
		if groups := pattern.expression.FindStringSubmatch(source); groups != nil {
			var result strings.Builder
			for i, literal := range pattern.literals {
				result.WriteString(literal)
				if i+1 < len(groups) {
					result.WriteString(groups[i+1])
				}
			}
			return result.String()
		}
	}
	// Work in a single pass, matching the longest phrase first. Generated English
	// is never re-translated, and Latin star/station names remain untouched.
	var out strings.Builder
	for pos := 0; pos < len(source); {
		node, end, translation := &root, pos, ""
		for i := pos; i < len(source); i++ {
			node = node.children[source[i]]
			if node == nil {
				break
			}
			if node.value != "" && phraseBoundary(source, pos, i+1, node.source) {
				end, translation = i+1, node.value
			}
		}
		if end > pos {
			out.WriteString(translation)
			pos = end
		} else {
			out.WriteByte(source[pos])
			pos++
		}
	}
	return out.String()
}

func containsCyrillic(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func phraseBoundary(source string, first, end int, phrase string) bool {
	startRune, _ := utf8.DecodeRuneInString(phrase)
	endRune, _ := utf8.DecodeLastRuneInString(phrase)
	if first > 0 && (unicode.IsLetter(startRune) || unicode.IsDigit(startRune)) {
		previous, _ := utf8.DecodeLastRuneInString(source[:first])
		if unicode.IsLetter(previous) || unicode.IsDigit(previous) {
			return false
		}
	}
	if end < len(source) && (unicode.IsLetter(endRune) || unicode.IsDigit(endRune)) {
		next, _ := utf8.DecodeRuneInString(source[end:])
		if unicode.IsLetter(next) || unicode.IsDigit(next) {
			return false
		}
	}
	return true
}

func TextProtected(language, source string, identities []string) string {
	if Normalize(language) != "en" {
		return source
	}
	values := append([]string(nil), identities...)
	sort.SliceStable(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	marker := "\ue000"
	for strings.Contains(source, marker) {
		marker += "\ue000"
	}
	var tokens []string
	for _, value := range values {
		if value == "" || !strings.Contains(source, value) {
			continue
		}
		body, _ := json.Marshal(len(tokens))
		token := marker + string(body) + marker
		source = strings.ReplaceAll(source, value, token)
		tokens = append(tokens, value)
	}
	source = Text(language, source)
	for i, value := range tokens {
		body, _ := json.Marshal(i)
		source = strings.ReplaceAll(source, marker+string(body)+marker, value)
	}
	return source
}
