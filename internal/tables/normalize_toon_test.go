package tables

import (
	"testing"

	"github.com/rveen/mt2data/internal/classify"
	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/parse"
)

// TestNormalize_conditionColumn verifies that a TOON table whose first column
// contains frequency ranges is treated as a condition column, not a name column,
// and that the group name comes from the preceding caption.
func TestNormalize_conditionColumn(t *testing.T) {
	input := "Table : Filter attenuation (CM and DM minimum)\n\nfilter[]{Frequency [MHz]|Att [db]}:\n  0,15 - 0,4 | 50\n  0,4 - 3 | 50\n  3 - 20 | 40\n  20 - 60 | 20\n"
	doc := parse.Parse(input)
	decisions := classify.Classify(doc)
	rep := &issues.Reporter{}
	res := Normalize(doc, decisions, map[string]string{}, rep)

	if len(res.Parameters) != 4 {
		t.Fatalf("want 4 parameters, got %d", len(res.Parameters))
	}
	for _, p := range res.Parameters {
		if p.Name == "0,15 - 0,4" || p.Name == "0,4 - 3" {
			t.Errorf("parameter name should not be a frequency range, got %q", p.Name)
		}
		if p.Name == "" {
			t.Error("parameter name must not be empty")
		}
		if len(p.Conditions) == 0 {
			t.Errorf("parameter %q should have a frequency condition", p.Name)
		}
		if p.Typ == nil {
			t.Errorf("parameter %q should have a Typ value", p.Name)
		}
	}
}
