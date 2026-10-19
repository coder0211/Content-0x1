package domain

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// WarningType classifies a post-generation warning (FR-CNT-06, FR-CNT-07).
type WarningType string

const (
	WarningForbiddenWord   WarningType = "forbidden_word"
	WarningUnverifiedClaim WarningType = "unverified_claim"
)

// Warning is shown inline in the editor; it never blocks saving.
type Warning struct {
	Type WarningType
	Text string
}

// ClaimKind is the kind of a numeric commercial claim.
type ClaimKind string

const (
	ClaimPercent ClaimKind = "percent"
	ClaimMoney   ClaimKind = "money"
)

// Claim is a money amount or percentage found in text.
type Claim struct {
	Kind     ClaimKind
	Value    float64
	Currency string // VND | USD for money
	Text     string // matched snippet, e.g. "giảm 50%"
}

func (c Claim) key() string {
	return string(c.Kind) + "|" + c.Currency + "|" + strconv.FormatFloat(math.Round(c.Value*100)/100, 'f', -1, 64)
}

const numPattern = `(\d{1,3}(?:[.,]\d{3})+|\d+(?:[.,]\d+)?)`

var (
	percentRe = regexp.MustCompile(`(?i)(?:(\p{L}+)\s+)?` + numPattern + `\s*(%|phần trăm)`)
	moneyRe   = regexp.MustCompile(`(?i)(?:(\p{L}+)\s+)?` + numPattern + `\s*(triệu đồng|nghìn đồng|ngàn đồng|triệu|nghìn|ngàn|tỷ|tr|k|đồng|vnđ|vnd|đ|₫|usd)`)
	dollarRe  = regexp.MustCompile(`(?i)(?:(\p{L}+)\s+)?(?:\$|us\$)\s*` + numPattern)
	thousands = regexp.MustCompile(`^\d{1,3}(?:[.,]\d{3})+$`)
)

// parseNumber reads Vietnamese/English formatted numbers: "2.990.000",
// "1,500" (thousand separators) or "2,5" / "2.5" (decimal).
func parseNumber(s string) (float64, bool) {
	if thousands.MatchString(s) {
		s = strings.NewReplacer(".", "", ",", "").Replace(s)
	} else {
		s = strings.ReplaceAll(s, ",", ".")
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

// followedByLetter reports whether text[end:] starts with a letter/digit,
// i.e. the unit match is a prefix of a longer word ("k" in "khóa").
func followedByLetter(text string, end int) bool {
	if end >= len(text) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text[end:])
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func snippet(text string, start, end int) string {
	return strings.Join(strings.Fields(text[start:end]), " ")
}

// ExtractClaims finds every percentage and money amount in text.
func ExtractClaims(text string) []Claim {
	var out []Claim
	for _, m := range percentRe.FindAllStringSubmatchIndex(text, -1) {
		if v, ok := parseNumber(text[m[4]:m[5]]); ok {
			out = append(out, Claim{Kind: ClaimPercent, Value: v, Text: snippet(text, m[0], m[1])})
		}
	}
	for _, m := range moneyRe.FindAllStringSubmatchIndex(text, -1) {
		if followedByLetter(text, m[1]) {
			continue
		}
		v, ok := parseNumber(text[m[4]:m[5]])
		if !ok {
			continue
		}
		unit := strings.ToLower(text[m[6]:m[7]])
		cur := "VND"
		switch {
		case strings.HasPrefix(unit, "triệu") || unit == "tr":
			v *= 1e6
		case strings.HasPrefix(unit, "nghìn") || strings.HasPrefix(unit, "ngàn") || unit == "k":
			v *= 1e3
		case unit == "tỷ":
			v *= 1e9
		case unit == "usd":
			cur = "USD"
		}
		out = append(out, Claim{Kind: ClaimMoney, Value: v, Currency: cur, Text: snippet(text, m[0], m[1])})
	}
	for _, m := range dollarRe.FindAllStringSubmatchIndex(text, -1) {
		if v, ok := parseNumber(text[m[4]:m[5]]); ok {
			out = append(out, Claim{Kind: ClaimMoney, Value: v, Currency: "USD", Text: snippet(text, m[0], m[1])})
		}
	}
	return out
}

// Facts is the set of verified commercial numbers a text may mention:
// product prices, valid offers, policies, proof and the user's brief.
type Facts struct {
	allowed map[string]bool
}

// NewFacts extracts claims from fact texts and adds explicit amounts.
func NewFacts(texts []string, amounts ...Claim) Facts {
	f := Facts{allowed: map[string]bool{}}
	for _, t := range texts {
		for _, c := range ExtractClaims(t) {
			f.allowed[c.key()] = true
		}
	}
	for _, c := range amounts {
		f.allowed[c.key()] = true
	}
	return f
}

// Allows reports whether claim is backed by a fact.
func (f Facts) Allows(c Claim) bool { return f.allowed[c.key()] }

// CheckUnverifiedClaims flags money/% in output that is not in facts.
func CheckUnverifiedClaims(texts []string, facts Facts) []Warning {
	var out []Warning
	seen := map[string]bool{}
	for _, t := range texts {
		for _, c := range ExtractClaims(t) {
			if facts.Allows(c) || seen[c.Text] {
				continue
			}
			seen[c.Text] = true
			out = append(out, Warning{Type: WarningUnverifiedClaim, Text: c.Text})
		}
	}
	return out
}

func normalizeSpace(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// containsPhrase finds phrase in text as a whole word/phrase (no letter or
// digit directly before or after).
func containsPhrase(text, phrase string) bool {
	for from := 0; from < len(text); {
		i := strings.Index(text[from:], phrase)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(phrase)
		before := start == 0
		if !before {
			r, _ := utf8.DecodeLastRuneInString(text[:start])
			before = !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}
		if before && !followedByLetter(text, end) {
			return true
		}
		_, size := utf8.DecodeRuneInString(text[start:])
		from = start + size
	}
	return false
}

// CheckForbiddenWords flags every brand forbidden word/phrase found
// (case-insensitive, whole words) in texts.
func CheckForbiddenWords(texts []string, forbidden []string) []Warning {
	joined := normalizeSpace(strings.Join(texts, "\n"))
	var out []Warning
	var seen []string
	for _, w := range forbidden {
		p := normalizeSpace(w)
		if p == "" || slices.Contains(seen, p) {
			continue
		}
		if containsPhrase(joined, p) {
			seen = append(seen, p)
			out = append(out, Warning{Type: WarningForbiddenWord, Text: strings.TrimSpace(w)})
		}
	}
	return out
}

// RunChecks runs every post-generation check on a variant.
func RunChecks(v Variant, forbidden []string, facts Facts) []Warning {
	texts := v.Texts()
	return append(CheckForbiddenWords(texts, forbidden), CheckUnverifiedClaims(texts, facts)...)
}
