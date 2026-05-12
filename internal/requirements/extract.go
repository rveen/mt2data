package requirements

import (
	"fmt"
	"strings"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/parse"
	"github.com/rveen/mt2data/internal/provenance"
	"github.com/rveen/mt2data/internal/schema"
)

// ExtractFromBlocks walks the block list and extracts Requirement records from
// prose paragraphs and lists. It attaches:
//   - The requirement DB ID from the immediately preceding ReqID block
//   - The ASIL level from the immediately following ASIL block
//   - The enclosing clause from the blockClause map
//
// autoSeq is a counter for auto-assigned IDs; pass a pointer initialised to 0.
func ExtractFromBlocks(
	doc *parse.Document,
	blockClause map[string]string,
	rep *issues.Reporter,
	autoSeq *int,
) []schema.Requirement {
	var reqs []schema.Requirement
	blocks := doc.Blocks

	for i, blk := range blocks {
		if blk.Type != parse.TypeParagraph && blk.Type != parse.TypeList {
			continue
		}

		// Scan sentences for imperatives
		sentences := segmentSentences(blk.Raw)
		verb := ""
		for _, s := range sentences {
			if v := DetectVerb(s); v != "" {
				verb = v
				break
			}
		}
		if verb == "" {
			continue
		}

		// Look back for a ReqID block
		docID := ""
		for j := i - 1; j >= 0 && j >= i-3; j-- {
			if blocks[j].Type == parse.TypeReqID {
				docID = blocks[j].ReqDocID
				break
			}
			if blocks[j].Type != parse.TypeBlank {
				break
			}
		}

		// Look ahead for an ASIL block
		asil := ""
		for j := i + 1; j < len(blocks) && j <= i+3; j++ {
			if blocks[j].Type == parse.TypeASIL {
				asil = blocks[j].ASIL
				break
			}
			if blocks[j].Type != parse.TypeBlank && blocks[j].Type != parse.TypeReqID {
				break
			}
		}

		// Build the requirement ID
		reqID := docID
		isAuto := false
		if reqID == "" {
			*autoSeq++
			reqID = fmt.Sprintf("REQ-AUTO-%04d", *autoSeq)
			isAuto = true
		}

		clauseID := blockClause[blk.ID]

		r := schema.Requirement{
			ID:               reqID,
			IDIsAuto:         isAuto,
			Text:             strings.TrimSpace(blk.Raw),
			Section:          clauseID,
			Verb:             verb,
			FunctionalSafety: asilString(asil),
			Source: provenance.Source{
				BlockID:   blk.ID,
				Clause:    clauseID,
				LineStart: blk.Source.LineStart,
				LineEnd:   blk.Source.LineEnd,
				OrigText:  strings.TrimSpace(blk.Raw),
			},
		}

		reqs = append(reqs, r)
	}

	return reqs
}

// asilString formats the ASIL value for the FunctionalSafety field.
func asilString(asil string) string {
	if asil == "" {
		return ""
	}
	return "ASIL " + asil
}

// segmentSentences splits text into sentences on ". " and ";" boundaries,
// respecting common abbreviations and signal names.
func segmentSentences(text string) []string {
	// Simple split on ". " and newlines — good enough for requirements prose.
	// Avoid splitting on abbreviations like "e.g.", "i.e.", "vs."
	text = strings.ReplaceAll(text, "e.g.", "eg")
	text = strings.ReplaceAll(text, "i.e.", "ie")
	text = strings.ReplaceAll(text, "\n", " ")

	var sentences []string
	parts := strings.Split(text, ". ")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			// Further split on semicolons
			for _, sub := range strings.Split(p, ";") {
				sub = strings.TrimSpace(sub)
				if sub != "" {
					sentences = append(sentences, sub)
				}
			}
		}
	}
	if len(sentences) == 0 && text != "" {
		sentences = []string{text}
	}
	return sentences
}

// ExtractFromList walks the block list and extracts requirements from list items
// that contain imperatives. Used to catch list-based requirements like
// "- The PDU shall enter Sleep Mode..."
func ExtractFromList(
	doc *parse.Document,
	blockClause map[string]string,
	rep *issues.Reporter,
	autoSeq *int,
) []schema.Requirement {
	var reqs []schema.Requirement
	blocks := doc.Blocks

	for i, blk := range blocks {
		if blk.Type != parse.TypeList {
			continue
		}
		items := strings.Split(blk.Raw, "\n")
		for _, item := range items {
			item = strings.TrimSpace(item)
			// Strip list marker
			item = strings.TrimLeft(item, "-*•0123456789. ")
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			verb := DetectVerb(item)
			if verb == "" {
				continue
			}

			// Look back for ReqID
			docID := ""
			for j := i - 1; j >= 0 && j >= i-3; j-- {
				if blocks[j].Type == parse.TypeReqID {
					docID = blocks[j].ReqDocID
					break
				}
				if blocks[j].Type != parse.TypeBlank {
					break
				}
			}

			reqID := docID
			isAuto := false
			if reqID == "" {
				*autoSeq++
				reqID = fmt.Sprintf("REQ-AUTO-%04d", *autoSeq)
				isAuto = true
			}

			clauseID := blockClause[blk.ID]
			r := schema.Requirement{
				ID:       reqID,
				IDIsAuto: isAuto,
				Text:     item,
				Section:  clauseID,
				Verb:     verb,
				Source: provenance.Source{
					BlockID:   blk.ID,
					LineStart: blk.Source.LineStart,
					LineEnd:   blk.Source.LineEnd,
					OrigText:  item,
				},
			}
			reqs = append(reqs, r)
		}
	}
	_ = rep
	return reqs
}
