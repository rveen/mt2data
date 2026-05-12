package hierarchy

import (
	"os"
	"testing"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/parse"
)

func TestBuild_basic(t *testing.T) {
	doc := parse.Parse("# Introduction\n\nSome text.\n\n## 1.1 Background\n\nMore text.\n")
	rep := &issues.Reporter{}
	res := Build(doc, rep)

	if len(res.Clauses) != 2 {
		t.Fatalf("want 2 clauses, got %d", len(res.Clauses))
	}
	if res.Clauses[0].Title != "Introduction" {
		t.Errorf("clause[0].Title = %q, want Introduction", res.Clauses[0].Title)
	}
	if res.Clauses[1].ID != "1.1" {
		t.Errorf("clause[1].ID = %q, want 1.1", res.Clauses[1].ID)
	}
	if len(res.Clauses[1].Path) != 2 {
		t.Errorf("clause[1].Path len = %d, want 2", len(res.Clauses[1].Path))
	}
}

func TestBuild_blockAttachment(t *testing.T) {
	doc := parse.Parse("## 2.1 Section\n\n1867448\n\nThe PDU shall close.\n")
	rep := &issues.Reporter{}
	res := Build(doc, rep)

	if len(res.Clauses) == 0 {
		t.Fatal("no clauses built")
	}
	// All non-heading blocks should be attached to clause 2.1
	for _, blk := range doc.Blocks {
		if blk.Type == parse.TypeHeading {
			continue
		}
		clauseID, ok := res.BlockClause[blk.ID]
		if !ok {
			t.Errorf("block %s (type %s) not attached to any clause", blk.ID, blk.Type)
		} else if clauseID != "2.1" {
			t.Errorf("block %s attached to %q, want 2.1", blk.ID, clauseID)
		}
	}
}

func TestBuild_rfq(t *testing.T) {
	data, err := os.ReadFile("../../testdata/rfq.md")
	if err != nil {
		t.Skip("testdata not available:", err)
	}
	doc := parse.Parse(string(data))
	rep := &issues.Reporter{}
	res := Build(doc, rep)

	if len(res.Clauses) < 10 {
		t.Errorf("want >10 clauses from rfq.md, got %d", len(res.Clauses))
	}
}
