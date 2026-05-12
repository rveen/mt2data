package requirements

import (
	"context"
	"testing"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/schema"
)

// stubProvider returns a fixed JSON response for classification tests.
type stubProvider struct{ response string }

func (s *stubProvider) Call(_ context.Context, _, _ string) (string, error) {
	return s.response, nil
}

func TestClassifyRequirements_patchesFields(t *testing.T) {
	reqs := []schema.Requirement{
		{ID: "REQ-001", Text: "The sensor shall measure voltage with ±1% accuracy."},
		{ID: "REQ-002", Text: "The PDU shall open the contactor within 10 ms."},
	}

	provider := &stubProvider{response: `[
		{"id":"REQ-001","domain":"test","verification":"T","title":"Voltage sensor accuracy"},
		{"id":"REQ-002","domain":"system","verification":"T","title":"Contactor opening time"}
	]`}

	rep := &issues.Reporter{}
	got := ClassifyRequirements(context.Background(), reqs, provider, rep)

	if got[0].Domain != "test" {
		t.Errorf("REQ-001 domain = %q, want %q", got[0].Domain, "test")
	}
	if got[0].Verification != "T" {
		t.Errorf("REQ-001 verification = %q, want T", got[0].Verification)
	}
	if got[0].Title != "Voltage sensor accuracy" {
		t.Errorf("REQ-001 title = %q", got[0].Title)
	}
	if got[1].Domain != "system" {
		t.Errorf("REQ-002 domain = %q, want system", got[1].Domain)
	}
	if len(rep.All()) != 0 {
		t.Errorf("unexpected issues: %v", rep.All())
	}
}

func TestClassifyRequirements_nilProvider(t *testing.T) {
	reqs := []schema.Requirement{{ID: "REQ-001", Text: "Something."}}
	rep := &issues.Reporter{}
	got := ClassifyRequirements(context.Background(), reqs, nil, rep)
	if got[0].Domain != "" {
		t.Errorf("nil provider should leave Domain empty, got %q", got[0].Domain)
	}
}
