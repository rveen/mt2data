// Package toon assembles a TOON requirements table from extracted requirements.
package toon

import (
	"fmt"
	"strings"

	"github.com/rveen/mt2data/internal/schema"
)

// sanitize normalises a field value for TOON output: collapses all whitespace
// (including newlines) to single spaces and escapes the pipe separator.
func sanitize(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(s, " | ", " / ")
}

// AssembleRequirements builds a TOON requirements table.
// The ID column is included since IDs may now be document-native (e.g. "8641256").
func AssembleRequirements(reqs []schema.Requirement) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "requirements[%d]{ID|Requirement|Section|Domain|Verb|Verification|FunctionalSafety}:\n",
		len(reqs))
	for _, r := range reqs {
		row := fmt.Sprintf("  %s | %s | %s | %s | %s | %s | %s",
			sanitize(r.ID),
			sanitize(r.Text),
			sanitize(r.Section),
			sanitize(r.Domain),
			sanitize(r.Verb),
			sanitize(r.Verification),
			sanitize(r.FunctionalSafety),
		)
		sb.WriteString(row)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// AssembleComponents builds a TOON components table.
func AssembleComponents(comps []schema.Component) string {
	if len(comps) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "components[%d]{Component|Type|Description}:\n", len(comps))
	for _, c := range comps {
		fmt.Fprintf(&sb, "  %s | %s | %s\n",
			sanitize(c.Name),
			sanitize(c.Type),
			sanitize(c.Description),
		)
	}
	return sb.String()
}

// AssembleConnections builds a TOON connections table.
func AssembleConnections(conns []schema.Connection) string {
	if len(conns) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "connections[%d]{From|To|Interface|Description}:\n", len(conns))
	for _, c := range conns {
		fmt.Fprintf(&sb, "  %s | %s | %s | %s\n",
			sanitize(c.From),
			sanitize(c.To),
			sanitize(c.Interface),
			sanitize(c.Description),
		)
	}
	return sb.String()
}

// AssembleTests builds a TOON tests table from requirements whose domain contains "test".
// Returns an empty string when no test requirements are present.
func AssembleTests(reqs []schema.Requirement) string {
	var testReqs []schema.Requirement
	for _, r := range reqs {
		if strings.Contains(r.Domain, "test") {
			testReqs = append(testReqs, r)
		}
	}
	if len(testReqs) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "tests[%d]{ID|Test|Section|Domain|Verb|Verification|FunctionalSafety}:\n",
		len(testReqs))
	for _, r := range testReqs {
		row := fmt.Sprintf("  %s | %s | %s | %s | %s | %s | %s",
			sanitize(r.ID),
			sanitize(r.Text),
			sanitize(r.Section),
			sanitize(r.Domain),
			sanitize(r.Verb),
			sanitize(r.Verification),
			sanitize(r.FunctionalSafety),
		)
		sb.WriteString(row)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// AssembleParameters builds a TOON parameters table.
// Columns: ID | Symbol | Name | Min | Typ | Max | Unit | Conditions
// Unit is extracted from whichever of Min/Typ/Max is present (first non-empty wins).
func AssembleParameters(params []schema.Parameter) string {
	if len(params) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "parameters[%d]{ID|Symbol|Name|Min|Typ|Max|Unit|Conditions}:\n", len(params))
	for _, p := range params {
		unit := quantityUnit(p.Min, p.Typ, p.Max)
		row := fmt.Sprintf("  %s | %s | %s | %s | %s | %s | %s | %s",
			sanitize(p.ID),
			sanitize(p.Symbol),
			sanitize(p.Name),
			sanitize(quantityRaw(p.Min)),
			sanitize(quantityRaw(p.Typ)),
			sanitize(quantityRaw(p.Max)),
			sanitize(unit),
			sanitize(conditionSummary(p.Conditions)),
		)
		sb.WriteString(row)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// quantityRaw returns the raw text of a quantity, or empty string if nil.
func quantityRaw(q *schema.Quantity) string {
	if q == nil {
		return ""
	}
	if q.Raw != "" {
		return q.Raw
	}
	if q.Unit != "" {
		return fmt.Sprintf("%g %s", q.Value, q.Unit)
	}
	return fmt.Sprintf("%g", q.Value)
}

// quantityUnit returns the unit from the first non-nil quantity.
func quantityUnit(qs ...*schema.Quantity) string {
	for _, q := range qs {
		if q != nil && q.Unit != "" {
			return q.Unit
		}
	}
	return ""
}

// conditionSummary renders a slice of Conditions as a compact string.
func conditionSummary(conds []schema.Condition) string {
	if len(conds) == 0 {
		return ""
	}
	parts := make([]string, 0, len(conds))
	for _, c := range conds {
		if c.Raw != "" {
			parts = append(parts, c.Quantity+": "+c.Raw)
		}
	}
	return strings.Join(parts, "; ")
}
