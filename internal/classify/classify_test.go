package classify

import (
	"testing"

	"github.com/rveen/mt2data/internal/parse"
)

func TestClassify_toonParameterTable(t *testing.T) {
	input := "filter[]{Frequency [MHz]|Att [db]}:\n  0,15 - 0,4 | 50\n  0,4 - 3 | 50\n"
	doc := parse.Parse(input)
	decisions := Classify(doc)
	for _, d := range decisions {
		if d.Semantic == ParameterTable {
			return
		}
	}
	t.Error("expected a ParameterTable decision for filter TOON table")
}

func TestClassify_toonTreeTable(t *testing.T) {
	input := "plan[]{Milestone,Payment,Acceptance criterion}:\n  B-sample, 20%, Parts delivery\n"
	doc := parse.Parse(input)
	decisions := Classify(doc)
	for _, d := range decisions {
		if d.Semantic == TreeTable {
			return
		}
	}
	t.Error("expected a TreeTable decision for plan TOON table")
}

func TestClassify_note(t *testing.T) {
	input := "Note: This is advisory.\n"
	doc := parse.Parse(input)
	decisions := Classify(doc)
	for _, d := range decisions {
		if d.Semantic == Note {
			return
		}
	}
	t.Error("expected a Note decision")
}

func TestClassify_figure(t *testing.T) {
	input := "(figure)\n"
	doc := parse.Parse(input)
	decisions := Classify(doc)
	for _, d := range decisions {
		if d.Semantic == FigureRef {
			return
		}
	}
	t.Error("expected a FigureRef decision")
}

func TestClassify_prose(t *testing.T) {
	input := "## 2.1 Section\n\nThe PDU shall close the contactors.\n"
	doc := parse.Parse(input)
	decisions := Classify(doc)
	for _, d := range decisions {
		if d.Semantic == Prose {
			return
		}
	}
	t.Error("expected at least one Prose decision")
}
