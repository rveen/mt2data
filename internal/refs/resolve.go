// Package refs implements Stage 6: reference resolution.
//
// Extracts and classifies cross-references from block text:
//   - Document refs: [SPEC NNN NNN], [STD 50123], [LOG SPEC], etc.
//   - Standard citations: ISO 16750-2:2012, IEC 61851-23, SAE J3400, UL 2202
//   - Section refs: "Chapter 4", "see 2.1.4.1.3"
//   - Signal names: [PDU_Xxx], [CMM_Xxx] — distinguished by UPPER_CASE_WITH_UNDERSCORES
package refs

import (
	"regexp"
	"strings"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/parse"
	"github.com/rveen/mt2data/internal/schema"
)

var (
	// [SPEC 310 001], [STD 50123], [LOG SPEC], [Van MGU], [EU Recommendation 2003/361]
	reBracketRef = regexp.MustCompile(`\[([A-Za-z][^\]]{1,60})\]`)

	// Signal names: start with an uppercase ECU prefix (PDU_, CMM_, CIC_, etc.)
	// followed by at least one underscore-separated component. Mixed case is allowed.
	reSignalName = regexp.MustCompile(`^(?:PDU|CMM|CIC|ECU|HVS|SZ)[_A-Za-z0-9]+$`)

	// ISO/IEC/SAE/UL standard citations in running text
	reStandardCitation = regexp.MustCompile(
		`(?i)\b(ISO|IEC|SAE|UL|DIN|EN|CISPR|IEEE)\s*(\d[\d\-/]*(?:[-_:]\d+)?)\s*(?::(\d{4}))?`)

	// Section references: "section 2.1.4", "Chapter 4", "see 2.1.4.1.3"
	reSectionRef = regexp.MustCompile(
		`(?i)\b(?:chapter|section|see|refer|clause)\s+(\d+(?:\.\d+)*)`)

)

// Resolve scans all blocks and returns structured references.
func Resolve(doc *parse.Document, rep *issues.Reporter) []schema.Reference {
	var refs []schema.Reference
	seen := make(map[string]bool)

	for _, blk := range doc.Blocks {
		extracted := extractRefs(blk.ID, blk.Raw)
		for _, r := range extracted {
			key := r.From + ":" + r.Raw
			if !seen[key] {
				seen[key] = true
				refs = append(refs, r)
				if !r.Resolved {
					rep.Add(issues.KindUnresolvedRef, r.From, "unresolved reference: "+r.Raw)
				}
			}
		}
	}
	return refs
}

func extractRefs(blockID, text string) []schema.Reference {
	var refs []schema.Reference

	// 1. Bracket references [...]
	for _, m := range reBracketRef.FindAllStringSubmatch(text, -1) {
		inner := strings.TrimSpace(m[1])
		if inner == "" {
			continue
		}

		// Signal name?
		if reSignalName.MatchString(inner) {
			refs = append(refs, schema.Reference{
				From:     blockID,
				Raw:      m[0],
				Kind:     schema.RefKindSignal,
				Resolved: false,
			})
			continue
		}

		// Document ref
		refs = append(refs, schema.Reference{
			From:     blockID,
			Raw:      m[0],
			Kind:     schema.RefKindDocument,
			Resolved: false,
		})
	}

	// 2. Standard citations in running text
	for _, m := range reStandardCitation.FindAllStringSubmatch(text, -1) {
		refs = append(refs, schema.Reference{
			From:     blockID,
			Raw:      strings.TrimSpace(m[0]),
			Kind:     schema.RefKindStandard,
			Norm:     m[1] + " " + m[2],
			Edition:  m[3],
			Resolved: false,
		})
	}

	// 3. Explicit section refs
	for _, m := range reSectionRef.FindAllStringSubmatch(text, -1) {
		refs = append(refs, schema.Reference{
			From:     blockID,
			Raw:      m[0],
			Kind:     schema.RefKindSection,
			Clause:   m[1],
			Resolved: false,
		})
	}

	return refs
}
