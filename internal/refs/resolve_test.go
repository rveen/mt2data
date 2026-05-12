package refs

import (
	"testing"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/parse"
	"github.com/rveen/mt2data/internal/schema"
)

func TestResolve_bracketDocRef(t *testing.T) {
	input := "The contractor shall comply with [STD 50123] requirements.\n"
	doc := parse.Parse(input)
	rep := &issues.Reporter{}
	refs := Resolve(doc, rep)
	found := findByKind(refs, schema.RefKindDocument)
	if len(found) == 0 {
		t.Error("expected a document reference for [STD 50123]")
	}
}

func TestResolve_signalName(t *testing.T) {
	input := "The signal [PDU_Charge_Stat] shall be set.\n"
	doc := parse.Parse(input)
	rep := &issues.Reporter{}
	refs := Resolve(doc, rep)
	found := findByKind(refs, schema.RefKindSignal)
	if len(found) == 0 {
		t.Error("expected a signal reference for [PDU_Charge_Stat]")
	}
}

func TestResolve_standardCitation(t *testing.T) {
	input := "Compliant with ISO 16750-2:2012 and IEC 61851-23.\n"
	doc := parse.Parse(input)
	rep := &issues.Reporter{}
	refs := Resolve(doc, rep)
	found := findByKind(refs, schema.RefKindStandard)
	if len(found) < 2 {
		t.Errorf("expected 2 standard references, got %d", len(found))
	}
}

func TestResolve_sectionRef(t *testing.T) {
	input := "See chapter 2.1.4.1 for details.\n"
	doc := parse.Parse(input)
	rep := &issues.Reporter{}
	refs := Resolve(doc, rep)
	found := findByKind(refs, schema.RefKindSection)
	if len(found) == 0 {
		t.Error("expected a section reference")
	}
}

func findByKind(refs []schema.Reference, kind schema.RefKind) []schema.Reference {
	var out []schema.Reference
	for _, r := range refs {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}
