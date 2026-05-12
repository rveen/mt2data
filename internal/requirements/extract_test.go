package requirements

import (
	"testing"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/parse"
)

func TestExtractFromBlocks_basic(t *testing.T) {
	input := "## 2.1 Section\n\n8641256\n\nThe PDU shall receive via bus communication a signal.\n\nASIL = B\n"
	doc := parse.Parse(input)
	blockClause := map[string]string{}
	// Attach all blocks to clause "2.1"
	for _, blk := range doc.Blocks {
		blockClause[blk.ID] = "2.1"
	}
	rep := &issues.Reporter{}
	seq := 0
	reqs := ExtractFromBlocks(doc, blockClause, rep, &seq)

	if len(reqs) == 0 {
		t.Fatal("expected at least one requirement")
	}
	r := reqs[0]
	if r.Verb != "MUST" {
		t.Errorf("verb = %q, want MUST", r.Verb)
	}
	if r.ID != "8641256" {
		t.Errorf("ID = %q, want 8641256", r.ID)
	}
	if r.FunctionalSafety != "ASIL B" {
		t.Errorf("FunctionalSafety = %q, want ASIL B", r.FunctionalSafety)
	}
	if r.Source.BlockID == "" {
		t.Error("Source.BlockID must be set")
	}
}

func TestExtractFromBlocks_autoID(t *testing.T) {
	input := "The system shall boot within 5 seconds.\n"
	doc := parse.Parse(input)
	rep := &issues.Reporter{}
	seq := 0
	reqs := ExtractFromBlocks(doc, map[string]string{}, rep, &seq)

	if len(reqs) == 0 {
		t.Fatal("expected a requirement")
	}
	if !reqs[0].IDIsAuto {
		t.Error("expected IDIsAuto = true when no ReqID block present")
	}
}

func TestExtractFromBlocks_noImperative(t *testing.T) {
	input := "This is a purely descriptive sentence with no modal verb.\n"
	doc := parse.Parse(input)
	rep := &issues.Reporter{}
	seq := 0
	reqs := ExtractFromBlocks(doc, map[string]string{}, rep, &seq)
	if len(reqs) != 0 {
		t.Errorf("expected 0 requirements for descriptive text, got %d", len(reqs))
	}
}
