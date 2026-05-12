package tables

import (
	"testing"

	"github.com/rveen/mt2data/internal/classify"
	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/parse"
)

func TestNormalize_parameterTable(t *testing.T) {
	input := "filter[]{Frequency [MHz]|Att [db]}:\n  0,15 - 0,4 | 50\n  0,4 - 3 | 50\n  3 - 20 | 40\n"
	doc := parse.Parse(input)
	decisions := classify.Classify(doc)
	rep := &issues.Reporter{}
	res := Normalize(doc, decisions, map[string]string{}, rep)

	if len(res.Parameters) != 3 {
		t.Errorf("want 3 parameters, got %d", len(res.Parameters))
	}
}

func TestNormalize_treeTable(t *testing.T) {
	input := "plan[]{Milestone,Payment,Acceptance}:\n  B-sample, 20%, Parts delivery\n  C-sample, 30%, Parts in A-Vehicle\n"
	doc := parse.Parse(input)
	decisions := classify.Classify(doc)
	rep := &issues.Reporter{}
	res := Normalize(doc, decisions, map[string]string{}, rep)

	if len(res.Trees) != 1 {
		t.Errorf("want 1 tree, got %d", len(res.Trees))
	}
	if len(res.Trees[0].Root.Children) != 2 {
		t.Errorf("want 2 tree children, got %d", len(res.Trees[0].Root.Children))
	}
}

func TestNormalize_units(t *testing.T) {
	input := "filter[]{Frequency [MHz]|Att [db]}:\n  0,15 - 0,4 | 50\n"
	doc := parse.Parse(input)
	decisions := classify.Classify(doc)
	rep := &issues.Reporter{}
	res := Normalize(doc, decisions, map[string]string{}, rep)

	if len(res.Parameters) == 0 {
		t.Fatal("no parameters")
	}
	// Typ should be parsed from the "Att" column
	p := res.Parameters[0]
	if p.Typ == nil {
		t.Error("expected Typ to be parsed")
	} else if p.Typ.Value != 50 {
		t.Errorf("Typ.Value = %v, want 50", p.Typ.Value)
	}
}
