package toon

import (
	"strings"
	"testing"

	"github.com/rveen/mt2data/internal/provenance"
	"github.com/rveen/mt2data/internal/schema"
)

func TestAssembleRequirements_basic(t *testing.T) {
	reqs := []schema.Requirement{
		{
			ID: "8641256", Text: "The PDU shall receive via bus a signal.",
			Section: "2.1.4.1.3.3.2", Verb: "MUST", FunctionalSafety: "ASIL B",
			Source: provenance.Source{BlockID: "abc123"},
		},
	}
	out := AssembleRequirements(reqs)
	if !strings.HasPrefix(out, "requirements[1]{ID|") {
		t.Errorf("unexpected header: %q", out)
	}
	if !strings.Contains(out, "8641256") {
		t.Error("ID not in output")
	}
	if !strings.Contains(out, "ASIL B") {
		t.Error("FunctionalSafety not in output")
	}
}

func TestAssembleRequirements_sanitize(t *testing.T) {
	reqs := []schema.Requirement{
		{ID: "1", Text: "A | B shall\nwork.", Verb: "MUST", Source: provenance.Source{BlockID: "x"}},
	}
	out := AssembleRequirements(reqs)
	if !strings.Contains(out, "A / B") {
		t.Error("pipe not escaped in text")
	}
	if strings.Contains(out[:strings.Index(out, "\n")+1], "\n\n") {
		t.Error("newline inside field should be collapsed")
	}
	// The row itself must be a single line (no embedded newline before the trailing one)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 { // header + 1 data row
		t.Errorf("expected 2 lines (header + 1 row), got %d", len(lines))
	}
}

func TestAssembleRequirements_empty(t *testing.T) {
	out := AssembleRequirements(nil)
	if !strings.HasPrefix(out, "requirements[0]{") {
		t.Errorf("empty table header wrong: %q", out)
	}
}
