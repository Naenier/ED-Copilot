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

type phraseNode struct {
	prefix, value string
	children      []phraseNode
}

type phraseIndex struct {
	first     [256]*phraseNode
	templates []formatPattern
}

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
		dictionary = map[string]string{}
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
	})
}

var foldedCatalogue = sync.OnceValue(func() map[string]string {
	load()
	keys := make([]string, 0, len(dictionary))
	for key := range dictionary {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	folded := make(map[string]string, len(keys))
	for _, source := range keys {
		folded[strings.ToLower(source)] = dictionary[source]
	}
	return folded
})

var catalogueJSON = sync.OnceValue(func() string {
	load()
	body, err := json.Marshal(dictionary)
	if err != nil {
		panic(err)
	}
	return string(body)
})

func CatalogJSON() string { return catalogueJSON() }

var composedPhrases = sync.OnceValue(buildPhraseIndex)

func buildPhraseIndex() *phraseIndex {
	load()
	sources := make([]string, 0, len(dictionary))
	for source := range dictionary {
		if containsCyrillic(source) {
			sources = append(sources, source)
		}
	}
	sort.Strings(sources)
	root := buildPhraseNode("", sources, 0)
	index := &phraseIndex{}
	for i := range root.children {
		child := &root.children[i]
		index.first[child.prefix[0]] = child
	}
	for _, source := range sources {
		parts := printfDirective.Split(source, -1)
		if len(parts) <= 1 {
			continue
		}
		translatedParts := printfDirective.Split(dictionary[source], -1)
		if len(parts) != len(translatedParts) {
			continue
		}
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
			index.templates = append(index.templates, formatPattern{regexp.MustCompile(expression + "$"), anchor, translatedParts})
		}
	}
	return index
}

// Each node covers a shared prefix of sorted dictionary entries. Storing
// complete shared fragments avoids a child map and allocation for every byte.
func buildPhraseNode(prefix string, sources []string, offset int) phraseNode {
	node := phraseNode{prefix: prefix}
	if len(sources) > 0 && len(sources[0]) == offset {
		node.value = dictionary[sources[0]]
		sources = sources[1:]
	}
	for first := 0; first < len(sources); {
		last := first + 1
		a := sources[first]
		for last < len(sources) && sources[last][offset] == a[offset] {
			last++
		}
		b := sources[last-1]
		common := offset + 1
		for common < min(len(a), len(b)) && a[common] == b[common] {
			common++
		}
		node.children = append(node.children, buildPhraseNode(a[offset:common], sources[first:last], common))
		first = last
	}
	return node
}

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
	if !strings.ContainsFunc(trimmed, func(r rune) bool { return unicode.ToUpper(r) != r }) {
		if lower := strings.ToLower(trimmed); lower != trimmed {
			if translated, ok := foldedCatalogue()[lower]; ok {
				return strings.ToUpper(translated)
			}
		}
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
	phrases := composedPhrases()
	for _, pattern := range phrases.templates {
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
		end, translation := pos, ""
		for i, node := pos, phrases.first[source[pos]]; node != nil && strings.HasPrefix(source[i:], node.prefix); {
			i += len(node.prefix)
			if node.value != "" && phraseBoundary(source, pos, i, source[pos:i]) {
				end, translation = i, node.value
			}
			if i == len(source) {
				break
			}
			node = node.child(source[i])
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

func (node *phraseNode) child(firstByte byte) *phraseNode {
	for i := range node.children {
		child := &node.children[i]
		if child.prefix[0] == firstByte {
			return child
		}
		if child.prefix[0] > firstByte {
			break
		}
	}
	return nil
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
