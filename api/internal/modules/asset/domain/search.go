package domain

import (
	"encoding/json"
	"strings"
	"unicode"
)

// SearchText extracts the searchable text of a version's content_json
// (migration 0006 schema): content → title, hook, body_text, cta, hashtags;
// image/video → prompt, script. Unknown/invalid JSON yields "".
func SearchText(content json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(content, &m) != nil {
		return ""
	}
	var parts []string
	for _, k := range []string{"title", "hook", "body_text", "cta", "prompt", "script", "platform"} {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			parts = append(parts, s)
		}
	}
	if hs, ok := m["hashtags"].([]any); ok {
		for _, h := range hs {
			if s, ok := h.(string); ok {
				parts = append(parts, strings.TrimPrefix(s, "#"))
			}
		}
	}
	// body_html is a fallback when no plain body exists.
	if _, ok := m["body_text"]; !ok {
		if s, ok := m["body_html"].(string); ok {
			parts = append(parts, stripTags(s))
		}
	}
	return strings.Join(parts, "\n")
}

func stripTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
			b.WriteByte(' ')
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// MaxQueryTerms bounds the number of words in a library search.
const MaxQueryTerms = 8

// PrefixQuery turns free user input into a safe to_tsquery('simple', …)
// expression: words are reduced to letters/digits, lower-cased, AND-ed and
// prefix-matched ("ưu đãi hè" → "ưu:* & đãi:* & hè:*"). Accent removal
// happens in SQL (immutable_unaccent) on both sides. Returns "" when the
// input has no searchable word.
func PrefixQuery(q string) string {
	words := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r)
	})
	terms := make([]string, 0, len(words))
	seen := map[string]bool{}
	for _, w := range words {
		if seen[w] {
			continue
		}
		seen[w] = true
		terms = append(terms, w+":*")
		if len(terms) == MaxQueryTerms {
			break
		}
	}
	return strings.Join(terms, " & ")
}
