package parse

import (
	"os"
	"testing"
)

func TestParse_headings(t *testing.T) {
	doc := Parse("# Title\n\nSome text.\n\n## 1.1 Sub (1867456)\n\nMore text.\n")
	headings := blocksOfType(doc, TypeHeading)
	if len(headings) != 2 {
		t.Fatalf("want 2 headings, got %d", len(headings))
	}
	if headings[0].Level != 1 {
		t.Errorf("first heading level want 1, got %d", headings[0].Level)
	}
	if headings[1].HeadID != "1867456" {
		t.Errorf("heading ID want 1867456, got %q", headings[1].HeadID)
	}
}

func TestParse_reqID(t *testing.T) {
	doc := Parse("1867448\n\nSome requirement text.\n\nR-869098\n\nAnother paragraph.\n")
	ids := blocksOfType(doc, TypeReqID)
	if len(ids) != 2 {
		t.Fatalf("want 2 req IDs, got %d", len(ids))
	}
	if ids[0].ReqDocID != "1867448" {
		t.Errorf("req ID want 1867448, got %q", ids[0].ReqDocID)
	}
	if ids[1].ReqDocID != "R-869098" {
		t.Errorf("req ID want R-869098, got %q", ids[1].ReqDocID)
	}
}

func TestParse_asil(t *testing.T) {
	doc := Parse("The PDU shall close.\n\nASIL = B(D)\n")
	asilBlocks := blocksOfType(doc, TypeASIL)
	if len(asilBlocks) != 1 {
		t.Fatalf("want 1 ASIL block, got %d", len(asilBlocks))
	}
	if asilBlocks[0].ASIL != "B(D)" {
		t.Errorf("ASIL want B(D), got %q", asilBlocks[0].ASIL)
	}
}

func TestParse_toonTable(t *testing.T) {
	input := "filter[]{Frequency [MHz]|Att [db]}:\n  0,15 - 0,4 | 50\n  0,4 - 3 | 50\n\n"
	doc := Parse(input)
	tables := blocksOfType(doc, TypeToonTable)
	if len(tables) != 1 {
		t.Fatalf("want 1 toon table, got %d", len(tables))
	}
	tt := doc.ToonTables[tables[0].ID]
	if tt == nil {
		t.Fatal("toon table not in map")
	}
	if tt.Name != "filter" {
		t.Errorf("table name want filter, got %q", tt.Name)
	}
	if len(tt.Columns) != 2 {
		t.Errorf("want 2 columns, got %d", len(tt.Columns))
	}
	if len(tt.Rows) != 2 {
		t.Errorf("want 2 rows, got %d", len(tt.Rows))
	}
}

func TestParse_noteHint(t *testing.T) {
	input := "Note: This is a note.\n\nHint:\nSome hint.\n"
	doc := Parse(input)
	notes := blocksOfType(doc, TypeNote)
	hints := blocksOfType(doc, TypeHint)
	if len(notes) != 1 {
		t.Errorf("want 1 note block, got %d", len(notes))
	}
	if len(hints) != 1 {
		t.Errorf("want 1 hint block, got %d", len(hints))
	}
}

func TestParse_rfq(t *testing.T) {
	data, err := os.ReadFile("../../testdata/rfq.md")
	if err != nil {
		t.Skip("testdata/rfq.md not available:", err)
	}
	doc := Parse(string(data))

	headings := blocksOfType(doc, TypeHeading)
	reqIDs := blocksOfType(doc, TypeReqID)
	asilBlocks := blocksOfType(doc, TypeASIL)
	toonTables := blocksOfType(doc, TypeToonTable)

	if len(doc.Blocks) < 200 {
		t.Errorf("expected >200 blocks from rfq.md, got %d", len(doc.Blocks))
	}
	if len(headings) < 20 {
		t.Errorf("expected >20 headings, got %d", len(headings))
	}
	if len(reqIDs) < 50 {
		t.Errorf("expected >50 req ID blocks, got %d", len(reqIDs))
	}
	if len(asilBlocks) < 10 {
		t.Errorf("expected >10 ASIL blocks, got %d", len(asilBlocks))
	}
	if len(toonTables) < 1 {
		t.Errorf("expected at least 1 TOON table, got %d", len(toonTables))
	}

	// All blocks must have non-empty IDs
	for _, blk := range doc.Blocks {
		if blk.ID == "" {
			t.Errorf("block at line %d has empty ID", blk.Source.LineStart)
		}
	}
}

// blocksOfType filters doc.Blocks by type.
func blocksOfType(doc *Document, t BlockType) []Block {
	var out []Block
	for _, b := range doc.Blocks {
		if b.Type == t {
			out = append(out, b)
		}
	}
	return out
}
