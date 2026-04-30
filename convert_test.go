package mt2req

import (
	"encoding/json"
	"strings"
	"testing"
)

// ----------------------------------------------------------------- text block splitting

func TestSplitTextBlocks_basic(t *testing.T) {
	doc := `## Introduction

Some context about this document.

## Requirements

The system shall provide a power supply.
The software shall log all errors.
`
	blocks := splitTextBlocks(doc)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if blocks[0].section != "Introduction" {
		t.Errorf("blocks[0].section = %q, want Introduction", blocks[0].section)
	}
	if blocks[1].section != "Requirements" {
		t.Errorf("blocks[1].section = %q, want Requirements", blocks[1].section)
	}
	if !strings.Contains(blocks[1].text, "power supply") {
		t.Errorf("blocks[1].text should contain requirement text, got: %q", blocks[1].text)
	}
}

func TestSplitTextBlocks_deepHeading(t *testing.T) {
	doc := `### 4.3.2.1 Sub-sub-section

The hardware shall operate between -40 °C and +85 °C.
`
	blocks := splitTextBlocks(doc)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].section != "4.3.2.1 Sub-sub-section" {
		t.Errorf("section = %q, want 4.3.2.1 Sub-sub-section", blocks[0].section)
	}
}

func TestSplitTextBlocks_noHeadings(t *testing.T) {
	doc := "The system shall boot within 5 seconds.\nThe software shall log all errors.\n"
	blocks := splitTextBlocks(doc)
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks for heading-free text, want 1", len(blocks))
	}
	if blocks[0].section != "" {
		t.Errorf("section = %q, want empty for heading-free text", blocks[0].section)
	}
	if !strings.Contains(blocks[0].text, "boot") {
		t.Error("block text should contain document content")
	}
}

func TestSplitTextBlocks_preamble(t *testing.T) {
	doc := `Preamble text before any heading.

## Section 1

Requirement text here.
`
	blocks := splitTextBlocks(doc)
	// Preamble before first heading becomes a block with empty section.
	if len(blocks) < 2 {
		t.Fatalf("got %d blocks, want at least 2", len(blocks))
	}
	if blocks[0].section != "" {
		t.Errorf("preamble block section = %q, want empty", blocks[0].section)
	}
	if !strings.Contains(blocks[0].text, "Preamble") {
		t.Error("preamble block should contain preamble text")
	}
}

// ----------------------------------------------------------------- JSON array concatenation (multi-block LLM response)

func TestStripCodeFence_multipleArrays(t *testing.T) {
	// Simulate two JSON arrays concatenated — the LLM returning two content blocks.
	input := `[{"section":"4.1","item":"vehicle","title":"Vehicle shall start within 2 s","domain":"system","verb":"shall","verification":"T","compound":"no"}][{"section":"4.2","item":"software","title":"Software shall log errors","domain":"software","verb":"shall","verification":"I","compound":"no"}]`

	var reqs []requirement
	dec := json.NewDecoder(strings.NewReader(input))
	for dec.More() {
		var batch []requirement
		if err := dec.Decode(&batch); err != nil {
			t.Fatalf("unexpected decode error: %v", err)
		}
		reqs = append(reqs, batch...)
	}
	if len(reqs) != 2 {
		t.Errorf("got %d requirements, want 2", len(reqs))
	}
}

// ----------------------------------------------------------------- code fence stripping

func TestStripCodeFence(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Plain JSON — untouched
		{`[{"id":"1"}]`, `[{"id":"1"}]`},
		// ```json fence
		{"```json\n[{\"id\":\"1\"}]\n```", `[{"id":"1"}]`},
		// ``` fence (no language tag)
		{"```\n[{\"id\":\"1\"}]\n```", `[{"id":"1"}]`},
		// Extra whitespace around fences
		{"```json\n\n[]\n\n```", `[]`},
		// Empty array inside fence
		{"```json\n[]\n```", `[]`},
	}
	for _, c := range cases {
		got := stripCodeFence(c.in)
		if got != c.want {
			t.Errorf("stripCodeFence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ----------------------------------------------------------------- verb normalization

func TestNormalizeVerb(t *testing.T) {
	cases := []struct{ in, want string }{
		// RFC2119 MUST equivalents
		{"shall", "MUST"},
		{"Shall", "MUST"},
		{"SHALL", "MUST"},
		{"must", "MUST"},
		{"required", "MUST"},
		// RFC2119 SHOULD equivalents
		{"should", "SHOULD"},
		{"recommended", "SHOULD"},
		// RFC2119 MAY equivalents
		{"may", "MAY"},
		{"optional", "MAY"},
		// RFC2119 MUST NOT equivalents
		{"must not", "MUST NOT"},
		{"shall not", "MUST NOT"},
		// RFC2119 SHOULD NOT
		{"should not", "SHOULD NOT"},
		// Unknown verb — uppercased as-is
		{"unknown", "UNKNOWN"},
	}
	for _, c := range cases {
		if got := normalizeVerb(c.in); got != c.want {
			t.Errorf("normalizeVerb(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ----------------------------------------------------------------- TOON assembly

func TestAssembleTOON_count(t *testing.T) {
	reqs := []requirement{
		{ID: "1", Section: "4.1", Item: "power supply", Title: "System provides 12 V ± 0.5 V power supply", Domain: "hardware", Verb: "MUST", Verification: "T", Compound: "no"},
		{ID: "2", Section: "4.2", Item: "software", Title: "Software logs all errors when one occurs", Domain: "software", Verb: "MUST", Verification: "I", Compound: "no"},
	}
	out := assembleTOON(reqs)

	if !strings.HasPrefix(out, "requirements[2]{Requirement|Section|Item|Domain|Verb|Verification|FunctionalSafety}:") {
		t.Errorf("unexpected header: %q", out)
	}
	// ID must NOT appear in the printout.
	if strings.Contains(out, " 1 ") || strings.Contains(out, " 2 ") {
		t.Error("ID should not appear in TOON printout")
	}
	if !strings.Contains(out, "12 V") {
		t.Error("title content not found in first row")
	}
}

func TestAssembleTOON_pipeEscape(t *testing.T) {
	reqs := []requirement{
		{ID: "1", Section: "5.1", Item: "system", Title: "System A | B shall operate", Domain: "system", Verb: "MUST", Verification: "A", Compound: "no"},
	}
	out := assembleTOON(reqs)
	// The literal " | " in a field must be escaped to " / ".
	if strings.Count(out, " | ") != 6 { // exactly the 6 column separators
		t.Errorf("unexpected number of ' | ' separators in row: %q", out)
	}
	if !strings.Contains(out, "A / B") {
		t.Error("escaped ' / ' not found in TOON row")
	}
}

func TestAssembleTOON_empty(t *testing.T) {
	out := assembleTOON(nil)
	if !strings.HasPrefix(out, "requirements[0]{Requirement|Section|Item|Domain|Verb|Verification|FunctionalSafety}:") {
		t.Errorf("empty table should start with requirements[0]{, got: %q", out)
	}
}

func TestAssembleJSON(t *testing.T) {
	reqs := []requirement{
		{ID: "1", Section: "3.1", Item: "system", Title: "System boots within 5 seconds", Domain: "software", Verb: "MUST", Verification: "T", Compound: "no"},
	}
	out := assembleJSON(reqs)
	if !strings.Contains(out, `"id": "1"`) {
		t.Errorf("JSON output missing id field, got: %s", out)
	}
	if !strings.Contains(out, `"section": "3.1"`) {
		t.Errorf("JSON output missing section field")
	}
	if !strings.Contains(out, `"item": "system"`) {
		t.Errorf("JSON output missing item field")
	}
	// compound and functional_safety must still be present in JSON even though not printed
	if !strings.Contains(out, `"compound"`) {
		t.Error("compound field missing from JSON output")
	}
	if !strings.Contains(out, `"functional_safety"`) {
		t.Error("functional_safety field missing from JSON output")
	}
}

func TestAssembleJSON_empty(t *testing.T) {
	out := assembleJSON(nil)
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("empty JSON should be [], got: %q", out)
	}
}

// ----------------------------------------------------------------- test-mode: TOON assembly

func TestAssembleTestTOON_count(t *testing.T) {
	tests := []testCase{
		{ID: "1", Section: "5.1", Title: "Apply 12 V for 5 s and verify no fault", Item: "power supply", TestType: "functional", Method: "T", Domain: "hardware"},
		{ID: "2", Section: "5.2", Title: "Inspect wiring harness for chafing", Item: "wiring harness", TestType: "functional", Method: "I", Domain: "hardware"},
	}
	out := assembleTestTOON(tests)

	if !strings.HasPrefix(out, "tests[2]{Test|Section|Item|Type|Method|Domain}:") {
		t.Errorf("unexpected header: %q", out)
	}
	if !strings.Contains(out, "12 V") {
		t.Error("first test title not found in output")
	}
	if !strings.Contains(out, "wiring harness") {
		t.Error("second test item not found in output")
	}
}

func TestAssembleTestTOON_empty(t *testing.T) {
	out := assembleTestTOON(nil)
	if !strings.HasPrefix(out, "tests[0]{") {
		t.Errorf("empty table should start with tests[0]{, got: %q", out)
	}
}

func TestAssembleTestTOON_pipeEscape(t *testing.T) {
	tests := []testCase{
		{ID: "1", Section: "6.1", Title: "Check A | B interface", Item: "interface", TestType: "interface", Method: "T", Domain: "system"},
	}
	out := assembleTestTOON(tests)
	// exactly 5 column separators in the data row
	if strings.Count(out, " | ") != 5 {
		t.Errorf("unexpected number of ' | ' separators: %q", out)
	}
	if !strings.Contains(out, "A / B") {
		t.Error("escaped ' / ' not found in TOON row")
	}
}

// ----------------------------------------------------------------- test-mode: JSON assembly

func TestAssembleTestJSON(t *testing.T) {
	tests := []testCase{
		{ID: "1", Section: "5.1", Title: "Apply 12 V for 5 s", Item: "power supply", TestType: "functional", Method: "T", Domain: "hardware"},
	}
	out := assembleTestJSON(tests)
	if !strings.Contains(out, `"id": "1"`) {
		t.Errorf("JSON output missing id field, got: %s", out)
	}
	if !strings.Contains(out, `"test_type": "functional"`) {
		t.Errorf("JSON output missing test_type field")
	}
	if !strings.Contains(out, `"pass_criterion"`) {
		t.Errorf("JSON output missing pass_criterion field")
	}
}

func TestAssembleTestJSON_empty(t *testing.T) {
	out := assembleTestJSON(nil)
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("empty JSON should be [], got: %q", out)
	}
}

// ----------------------------------------------------------------- test-mode: JSON parsing

func TestParseTestCases_basic(t *testing.T) {
	input := `[{"section":"5.1","title":"Apply 12 V for 5 s and verify no fault","item":"power supply","test_type":"functional","method":"T","preconditions":"DUT powered off","pass_criterion":"no fault flag set","domain":"hardware"}]`
	cases := parseTestCases("5.1", input)
	if len(cases) != 1 {
		t.Fatalf("got %d test cases, want 1", len(cases))
	}
	if cases[0].TestType != "functional" {
		t.Errorf("test_type = %q, want functional", cases[0].TestType)
	}
	if cases[0].Method != "T" {
		t.Errorf("method = %q, want T", cases[0].Method)
	}
	if cases[0].PassCriterion != "no fault flag set" {
		t.Errorf("pass_criterion = %q, want 'no fault flag set'", cases[0].PassCriterion)
	}
}

func TestParseTestCases_empty(t *testing.T) {
	cases := parseTestCases("5.1", "[]")
	if len(cases) != 0 {
		t.Errorf("got %d cases for empty array, want 0", len(cases))
	}
}

func TestParseTestCases_withCodeFence(t *testing.T) {
	input := "```json\n[{\"section\":\"5.1\",\"title\":\"Verify boot time\",\"item\":\"ECU\",\"test_type\":\"performance\",\"method\":\"T\",\"preconditions\":\"\",\"pass_criterion\":\"boot within 2 s\",\"domain\":\"system\"}]\n```"
	cases := parseTestCases("5.1", input)
	if len(cases) != 1 {
		t.Fatalf("got %d cases with code fence, want 1", len(cases))
	}
	if cases[0].TestType != "performance" {
		t.Errorf("test_type = %q, want performance", cases[0].TestType)
	}
}
