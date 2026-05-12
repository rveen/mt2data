// Package requirements implements Stage 5: imperative extraction from prose.
package requirements

import "strings"

// rfc2119VerbMap maps lower-case source verb forms to canonical RFC2119 verbs.
var rfc2119VerbMap = map[string]string{
	"shall":        "MUST",
	"must":         "MUST",
	"required":     "MUST",
	"is required":  "MUST",
	"shall not":    "MUST NOT",
	"must not":     "MUST NOT",
	"is not permitted": "MUST NOT",
	"should":       "SHOULD",
	"recommended":  "SHOULD",
	"is recommended": "SHOULD",
	"should not":   "SHOULD NOT",
	"may":          "MAY",
	"optional":     "MAY",
}

// NormalizeVerb returns the canonical RFC2119 verb for v.
// If v is not in the RFC2119 map the original value (uppercased) is returned.
func NormalizeVerb(v string) string {
	if canonical, ok := rfc2119VerbMap[strings.ToLower(strings.TrimSpace(v))]; ok {
		return canonical
	}
	return strings.ToUpper(strings.TrimSpace(v))
}

// DetectVerb scans text for the first imperative verb and returns it normalized.
// Returns "" if no imperative is found.
func DetectVerb(text string) string {
	lower := strings.ToLower(text)
	// Test multi-word verbs first (longer matches win)
	multiWord := []string{
		"shall not", "must not", "should not",
		"is not permitted", "is required to", "is required",
		"shall be possible", "is recommended",
	}
	for _, mw := range multiWord {
		if strings.Contains(lower, mw) {
			return NormalizeVerb(mw)
		}
	}
	singleWord := []string{"shall", "must", "should", "may"}
	for _, sw := range singleWord {
		// Use word-boundary check to avoid matching "maybe" → "may"
		if containsWord(lower, sw) {
			return NormalizeVerb(sw)
		}
	}
	return ""
}

// containsWord checks that word appears in s as a standalone word (bounded by
// non-letter characters or string edges).
func containsWord(s, word string) bool {
	idx := 0
	for {
		pos := strings.Index(s[idx:], word)
		if pos < 0 {
			return false
		}
		abs := idx + pos
		before := abs == 0 || !isLetter(rune(s[abs-1]))
		after := abs+len(word) >= len(s) || !isLetter(rune(s[abs+len(word)]))
		if before && after {
			return true
		}
		idx = abs + 1
	}
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
