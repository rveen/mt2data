// Package tables implements Stage 4: table normalization.
//
// For each classified TOON table it produces typed records:
//   - parameter_table → []schema.Parameter
//   - requirement_table → []schema.Requirement (basic, no prose extraction)
//   - tree_table → schema.Tree
package tables

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rveen/mt2data/internal/classify"
	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/parse"
	"github.com/rveen/mt2data/internal/provenance"
	"github.com/rveen/mt2data/internal/schema"
	"github.com/rveen/mt2data/internal/tables/units"
)

// Result holds all records extracted from tables.
type Result struct {
	Parameters   []schema.Parameter
	Requirements []schema.Requirement
	Trees        []schema.Tree
}

// Normalize processes all TOON tables in the document, guided by classification decisions.
// It iterates blocks in document order to collect caption context for each table.
func Normalize(
	doc *parse.Document,
	decisions []classify.Decision,
	hierarchy map[string]string, // block ID → clause ID
	rep *issues.Reporter,
) *Result {
	decisionByID := make(map[string]classify.SemanticType, len(decisions))
	for _, d := range decisions {
		decisionByID[d.BlockID] = d.Semantic
	}

	res := &Result{}
	paramSeq := 0

	// Iterate blocks in order so we can collect the preceding caption.
	var prevCaption string
	for _, blk := range doc.Blocks {
		// Track the most recent heading or paragraph text as a caption hint.
		if blk.Type == parse.TypeHeading {
			prevCaption = headingText(blk.Raw)
			continue
		}
		if blk.Type == parse.TypeParagraph || blk.Type == parse.TypeNote {
			prevCaption = strings.TrimSpace(blk.Raw)
			continue
		}
		if blk.Type != parse.TypeToonTable {
			continue
		}

		tt := doc.ToonTables[blk.ID]
		if tt == nil {
			continue
		}
		sem := decisionByID[blk.ID]
		src := provenance.Source{
			BlockID:   blk.ID,
			Clause:    hierarchy[blk.ID],
			LineStart: blk.Source.LineStart,
			LineEnd:   blk.Source.LineEnd,
		}

		switch sem {
		case classify.ParameterTable:
			params := normalizeParameterTable(tt, prevCaption, src, &paramSeq, rep)
			res.Parameters = append(res.Parameters, params...)

		case classify.RequirementTable:
			tree := extractTree(tt, blk.ID, src)
			if tree != nil {
				res.Trees = append(res.Trees, *tree)
			}

		case classify.TreeTable:
			tree := extractTree(tt, blk.ID, src)
			if tree != nil {
				res.Trees = append(res.Trees, *tree)
			}

		case classify.Unknown:
			rep.Add(issues.KindUnknownBlock, blk.ID,
				fmt.Sprintf("TOON table %q not classified", tt.Name))
		}

		prevCaption = ""
	}

	return res
}

// headingText strips the leading # characters from a Markdown heading line.
func headingText(raw string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(raw), "#"))
}

// reRange matches a value that looks like a range ("0,15 - 0,4", "3..20", "0,4-3").
var reRange = regexp.MustCompile(`^\s*[\d.,]+\s*[-–.]{1,2}\s*[\d.,]+\s*$`)

// normalizeParameterTable converts a parameter TOON table to []schema.Parameter.
//
// When the first column contains range-like values (e.g. "0,15 - 0,4") instead of names
// it is treated as a condition column. The parameter group name is taken from the
// preceding heading/caption text passed in via caption.
func normalizeParameterTable(
	tt *parse.ToonTable,
	caption string,
	src provenance.Source,
	seq *int,
	rep *issues.Reporter,
) []schema.Parameter {
	cols := normalizeCols(tt.Columns)
	minIdx, typIdx, maxIdx := colIndex(cols, "min"), colIndex(cols, "typ"), colIndex(cols, "max")
	unitIdx := colIndex(cols, "unit")
	nameIdx := colIndex(cols, "name")
	nameIdx = max(nameIdx, 0)

	// Detect whether the name column actually holds condition ranges rather than names.
	conditionColIdx := -1
	conditionColName := ""
	if nameIdx >= 0 && isRangeColumn(tt.Rows, nameIdx) {
		conditionColIdx = nameIdx
		conditionColName = tt.Columns[nameIdx]
		nameIdx = -1 // name will come from group context
	}

	// Derive the group name from the caption (e.g. "Table : Filter attenuation (CM and DM minimum)")
	// or from the TOON table name as a fallback.
	groupName := cleanCaption(caption)
	if groupName == "" {
		groupName = tt.Name
	}

	var params []schema.Parameter
	for _, row := range tt.Rows {
		if len(row) == 0 {
			continue
		}
		*seq++
		p := schema.Parameter{
			ID:     fmt.Sprintf("PARAM-%04d", *seq),
			Source: src,
		}
		p.Source.OrigText = strings.Join(row, " | ")

		if nameIdx >= 0 && nameIdx < len(row) {
			p.Name = strings.TrimSpace(row[nameIdx])
		} else {
			p.Name = groupName
		}

		unitHint := ""
		if unitIdx >= 0 && unitIdx < len(row) {
			unitHint = strings.TrimSpace(row[unitIdx])
		}

		// Propagate the condition column value as a Condition.
		if conditionColIdx >= 0 && conditionColIdx < len(row) {
			raw := strings.TrimSpace(row[conditionColIdx])
			if raw != "" {
				cond := schema.Condition{Quantity: conditionColName, Raw: raw}
				if q := units.Parse(raw); q != nil {
					cond.Value = &schema.Quantity{Value: q.Value, Unit: q.Unit, Raw: raw}
				}
				p.Conditions = append(p.Conditions, cond)
			}
		}

		p.Min = parseCell(row, minIdx, unitHint, rep, p.ID)
		p.Typ = parseCell(row, typIdx, unitHint, rep, p.ID)
		p.Max = parseCell(row, maxIdx, unitHint, rep, p.ID)

		// If no min/typ/max columns found, scan remaining numeric columns.
		if p.Min == nil && p.Typ == nil && p.Max == nil {
			for i, col := range cols {
				if i == nameIdx || i == conditionColIdx || i == unitIdx {
					continue
				}
				if col == "value" || col == "att" || col == "attenuation" || isValueCol(col) {
					if i < len(row) {
						raw := strings.TrimSpace(row[i])
						// Prefer unit from the column header bracket over explicit unit column.
						colUnitHint := extractColUnit(tt.Columns[i])
						hint := colUnitHint
						if hint == "" {
							hint = unitHint
						}
						q := units.Parse(raw)
						if q == nil && hint != "" {
							q = units.Parse(raw + " " + hint)
						}
						if q != nil {
							p.Typ = &schema.Quantity{Value: q.Value, Unit: q.Unit, Raw: raw}
						}
					}
					break
				}
			}
		}

		params = append(params, p)
	}
	_ = rep
	return params
}

// isRangeColumn returns true if the majority of values in column idx look like ranges.
func isRangeColumn(rows [][]string, idx int) bool {
	if len(rows) == 0 {
		return false
	}
	hits := 0
	for _, row := range rows {
		if idx < len(row) && reRange.MatchString(row[idx]) {
			hits++
		}
	}
	return hits > len(rows)/2
}

// isValueCol returns true for column names that are likely to hold a scalar value.
func isValueCol(col string) bool {
	for _, kw := range []string{"value", "att", "attenuation", "level", "limit", "spec"} {
		if strings.Contains(col, kw) {
			return true
		}
	}
	return false
}

// cleanCaption strips leading "Table :", "Figure :" prefixes and parenthesised IDs from
// a caption string, leaving a clean parameter group name.
var reCleanCaption = regexp.MustCompile(`(?i)^(?:table|figure)\s*:\s*`)
var reTrailingParens = regexp.MustCompile(`\s*\([^)]{0,40}\)\s*$`)

func cleanCaption(s string) string {
	s = strings.TrimSpace(s)
	s = reCleanCaption.ReplaceAllString(s, "")
	s = reTrailingParens.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func parseCell(row []string, idx int, unitHint string, _ *issues.Reporter, _ string) *schema.Quantity {
	if idx < 0 || idx >= len(row) {
		return nil
	}
	raw := strings.TrimSpace(row[idx])
	if raw == "" || raw == "-" {
		return nil
	}
	// Try parsing as-is first, then with appended unit hint
	q := units.Parse(raw)
	if q == nil && unitHint != "" {
		q = units.Parse(raw + " " + unitHint)
	}
	if q == nil {
		return nil
	}
	return &schema.Quantity{Value: q.Value, Unit: q.Unit, Raw: raw}
}

// extractTree converts a tree/plan TOON table into a schema.Tree.
func extractTree(tt *parse.ToonTable, blockID string, src provenance.Source) *schema.Tree {
	if len(tt.Rows) == 0 {
		return nil
	}
	root := schema.TreeNode{ID: blockID, Label: tt.Name}
	for i, row := range tt.Rows {
		if len(row) == 0 {
			continue
		}
		child := schema.TreeNode{
			ID:    fmt.Sprintf("%s-row%d", blockID, i),
			Label: strings.Join(row, " | "),
		}
		root.Children = append(root.Children, child)
	}
	return &schema.Tree{
		ID:     blockID,
		Source: src.BlockID,
		Root:   root,
	}
}

// normalizeCols lowercases and trims column names for matching.
func normalizeCols(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = strings.ToLower(strings.TrimSpace(c))
		// Strip bracketed units from column name: "Frequency [MHz]" → "frequency"
		if idx := strings.Index(out[i], "["); idx > 0 {
			out[i] = strings.TrimSpace(out[i][:idx])
		}
	}
	return out
}

// extractColUnit returns the unit hint embedded in a column header bracket,
// e.g. "Att [db]" → "dB", "Frequency [MHz]" → "MHz". Returns "" if none.
func extractColUnit(colRaw string) string {
	start := strings.Index(colRaw, "[")
	end := strings.Index(colRaw, "]")
	if start < 0 || end <= start {
		return ""
	}
	raw := strings.TrimSpace(colRaw[start+1 : end])
	// Reuse the unit normaliser: wrap in a dummy "1 <unit>" parse.
	q := units.Parse("1 " + raw)
	if q != nil && q.Unit != "" {
		return q.Unit
	}
	return raw
}

// colIndex returns the index of the first column whose normalised name contains sub.
func colIndex(cols []string, sub string) int {
	for i, c := range cols {
		if strings.Contains(c, sub) {
			return i
		}
	}
	return -1
}
