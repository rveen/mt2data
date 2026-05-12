package tables

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/parse"
	"github.com/rveen/mt2data/internal/provenance"
	"github.com/rveen/mt2data/internal/schema"
	"github.com/rveen/mt2data/internal/tables/units"
)

// reKeyValue matches a line of the form "Key name : value" or "Key name: value".
// The key may contain letters, digits, spaces, parentheses, and slashes.
// The value is everything after the first colon.
var reKeyValue = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9 ()/\-]{1,60}?)\s*:\s*(.+)$`)

// reImperative is a quick check to avoid mis-classifying requirement sentences.
var reImperative = regexp.MustCompile(`(?i)\bshall\b|\bmust\b|\bshould\b|\bmay not\b`)

// signalAttrKeys are key names that indicate a signal-interface definition block,
// not a physical parameter specification.
var signalAttrKeys = map[string]bool{
	"signal name":    true,
	"basic value":    true,
	"unit":           true,
	"description":    true,
	"attribute name": true,
}

// ExtractKeyValueParams scans the parsed block list for paragraphs that consist
// entirely (or mostly) of "key: value" lines and extracts them as Parameters.
//
// Two patterns are handled:
//  1. Multi-line block: a single paragraph where ≥2 lines are key:value pairs.
//  2. Single-line block: a single "key: value" line whose value parses as a quantity.
//
// Each extracted parameter inherits the ReqDocID from the preceding ReqID block (if any)
// and the clause from the blockClause map.
func ExtractKeyValueParams(
	doc *parse.Document,
	blockClause map[string]string,
	rep *issues.Reporter,
	seq *int,
) []schema.Parameter {
	var params []schema.Parameter
	blocks := doc.Blocks

	for i, blk := range blocks {
		if blk.Type != parse.TypeParagraph {
			continue
		}

		lines := nonEmptyLines(blk.Raw)
		if len(lines) == 0 {
			continue
		}

		// Reject if the block contains an imperative — it's a requirement, not a spec.
		if reImperative.MatchString(blk.Raw) {
			continue
		}

		kvLines := keyValueLines(lines)
		if len(kvLines) == 0 {
			continue
		}

		// For a single-line block require the value to parse as a quantity.
		if len(lines) == 1 {
			if units.Parse(kvLines[0].value) == nil {
				continue
			}
		}

		// Need at least 2 kv lines for a multi-line block.
		if len(lines) > 1 && len(kvLines) < 2 {
			continue
		}

		// Skip blocks where no value contains a digit — these are signal attribute
		// tables (Basic Value: OPEN, Range: OPEN/CLOSE) not physical parameters.
		if !anyValueHasDigit(kvLines) {
			continue
		}

		// Skip signal-interface definition blocks (Signal Name, Basic Value, etc.).
		if anyKeyIsSignalAttr(kvLines) {
			continue
		}

		// Look back for a ReqID to use as the parameter ID.
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

		clauseID := blockClause[blk.ID]
		src := provenance.Source{
			BlockID:   blk.ID,
			Clause:    clauseID,
			LineStart: blk.Source.LineStart,
			LineEnd:   blk.Source.LineEnd,
		}

		for _, kv := range kvLines {
			*seq++
			id := fmt.Sprintf("PARAM-%04d", *seq)
			// If docID is available and there's only one parameter in this block, use it.
			if docID != "" && len(kvLines) == 1 {
				id = docID
			}

			p := schema.Parameter{
				ID:   id,
				Name: kv.key,
				Source: provenance.Source{
					BlockID:   src.BlockID,
					Clause:    src.Clause,
					LineStart: src.LineStart,
					LineEnd:   src.LineEnd,
					OrigText:  kv.raw,
				},
			}

			// Parse the value.
			q := units.Parse(kv.value)
			if q != nil {
				p.Typ = &schema.Quantity{Value: q.Value, Unit: q.Unit, Raw: kv.value}
			} else {
				// Store unparsed value as a raw quantity with no unit.
				p.Typ = &schema.Quantity{Raw: kv.value}
			}

			params = append(params, p)
		}
		_ = rep
	}

	return params
}

type kvPair struct {
	key, value, raw string
}

// keyValueLines returns the subset of lines that match the key:value pattern.
func keyValueLines(lines []string) []kvPair {
	var out []kvPair
	for _, line := range lines {
		m := reKeyValue.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		out = append(out, kvPair{
			key:   strings.TrimSpace(m[1]),
			value: strings.TrimSpace(m[2]),
			raw:   strings.TrimSpace(line),
		})
	}
	return out
}

// anyKeyIsSignalAttr returns true if any key matches a known signal-interface attribute name.
func anyKeyIsSignalAttr(kvs []kvPair) bool {
	for _, kv := range kvs {
		if signalAttrKeys[strings.ToLower(kv.key)] {
			return true
		}
	}
	return false
}

// anyValueHasDigit returns true if at least one kv value contains a digit.
func anyValueHasDigit(kvs []kvPair) bool {
	for _, kv := range kvs {
		for _, r := range kv.value {
			if r >= '0' && r <= '9' {
				return true
			}
		}
	}
	return false
}

// nonEmptyLines splits text on newlines and returns non-empty lines.
func nonEmptyLines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
