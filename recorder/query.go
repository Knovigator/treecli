package recorder

import (
	"strings"
	"unicode"
)

// memdb hands queries straight to SQLite FTS5 MATCH, where a bare phrase
// means "every token must appear" and punctuation is syntax. Free-form prompts
// therefore need to become an OR of their significant words before they can
// find anything.

var ftsStopwords = map[string]bool{}

func init() {
	for _, word := range strings.Fields(`a about again all also an and any are as at be been being but by can could did do does doing done down each few for from further get got had has have having he her here hers him his how i if in into is it its just let lets like made make me might more most much must my need no not now of off on once only or other our ours out over own please said same she should so some such than that the their theirs them then there these they this those through to too under until up us use used using very want was we were what when where which while who whom why will with would yes you your yours`) {
		ftsStopwords[word] = true
	}
}

// FTSQuery turns free text into a memdb/FTS5 query: significant words joined
// with OR. Text that already uses FTS operators or quotes is passed through
// untouched so callers can be precise. An empty result means "nothing worth
// searching for".
func FTSQuery(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if looksLikeFTSSyntax(trimmed) {
		return trimmed
	}
	seen := map[string]bool{}
	terms := []string{}
	for _, token := range strings.FieldsFunc(strings.ToLower(trimmed), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
	}) {
		if len([]rune(token)) < 3 || ftsStopwords[token] || seen[token] {
			continue
		}
		seen[token] = true
		terms = append(terms, token)
		if len(terms) == 12 {
			break
		}
	}
	return strings.Join(terms, " OR ")
}

func looksLikeFTSSyntax(text string) bool {
	if strings.ContainsAny(text, `"*^`) {
		return true
	}
	upper := " " + strings.ToUpper(text) + " "
	return strings.Contains(upper, " OR ") || strings.Contains(upper, " AND ") || strings.Contains(upper, " NOT ") || strings.Contains(upper, " NEAR(")
}
