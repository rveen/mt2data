// Package merge implements Stage 7: conservative deduplication of requirements and parameters.
//
// Only exact-key matches are merged. Near-matches are flagged as issues, not silently combined.
package merge

import (
	"fmt"
	"strings"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/schema"
)

// Requirements deduplicates req by exact {Text, Section, FunctionalSafety, Verb} key.
// Duplicate IDs (same explicit ID from the document) are flagged and the second copy dropped.
// Near-matches (same Text, different other fields) are flagged but kept.
func Requirements(reqs []schema.Requirement, rep *issues.Reporter) []schema.Requirement {
	type key struct{ Text, Section, ASIL, Verb string }

	seen := make(map[key]int)       // key → index in output
	seenID := make(map[string]bool) // explicit doc IDs
	var out []schema.Requirement

	for _, r := range reqs {
		// Duplicate explicit ID check
		if !r.IDIsAuto {
			if seenID[r.ID] {
				rep.Add(issues.KindDuplicateID, r.Source.BlockID,
					fmt.Sprintf("duplicate requirement ID %q", r.ID))
				continue
			}
			seenID[r.ID] = true
		}

		k := key{
			Text:    normalizeText(r.Text),
			Section: r.Section,
			ASIL:    r.FunctionalSafety,
			Verb:    r.Verb,
		}

		if _, exists := seen[k]; exists {
			// Exact duplicate — drop silently (already recorded under its ID)
			continue
		}

		// Near-match: same normalized text but different section/ASIL/verb
		for ek, ei := range seen {
			if ek.Text == k.Text && (ek.Section != k.Section || ek.ASIL != k.ASIL || ek.Verb != k.Verb) {
				rep.Add(issues.KindNearDuplicate, r.Source.BlockID,
					fmt.Sprintf("near-duplicate of requirement at block %s", out[ei].Source.BlockID))
				break
			}
		}

		seen[k] = len(out)
		out = append(out, r)
	}
	return out
}

// Parameters deduplicates params by exact {Name, Symbol} key within the same conditions set.
func Parameters(params []schema.Parameter, rep *issues.Reporter) []schema.Parameter {
	type key struct{ Name, Symbol string }
	seen := make(map[key]bool)
	var out []schema.Parameter

	for _, p := range params {
		k := key{Name: p.Name, Symbol: p.Symbol}
		if seen[k] {
			rep.Add(issues.KindNearDuplicate, p.Source.BlockID,
				fmt.Sprintf("near-duplicate parameter %q", p.Name))
			out = append(out, p) // keep both, flagged
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	return out
}

// normalizeText lowercases and collapses whitespace for comparison.
func normalizeText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
