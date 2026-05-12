package components

import (
	"context"
	"testing"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/schema"
)

type stubProvider struct{ response string }

func (s *stubProvider) Call(_ context.Context, _, _ string) (string, error) {
	return s.response, nil
}

func TestExtractComponents_basic(t *testing.T) {
	clauses := []schema.Clause{
		{ID: "1", Title: "Introduction", Path: []string{"1"}},
		{ID: "2", Title: "Component Functions", Path: []string{"2"}},
	}
	reqs := []schema.Requirement{
		{ID: "REQ-001", Text: "The PDU shall close the contactor.", IDIsAuto: false},
		{ID: "REQ-002", Text: "The charging manager shall command the PDU.", IDIsAuto: false},
	}

	provider := &stubProvider{response: `{
		"components": [
			{"name":"PDU","type":"module","description":"Power distribution unit"},
			{"name":"Charging Manager","type":"software","description":"Controls charging"}
		],
		"connections": [
			{"from":"Charging Manager","to":"PDU","interface":"CAN","description":"Contactor commands"}
		]
	}`}

	rep := &issues.Reporter{}
	comps, conns := ExtractComponents(context.Background(), clauses, reqs, provider, rep)

	if len(comps) != 2 {
		t.Fatalf("want 2 components, got %d", len(comps))
	}
	if comps[0].Name != "PDU" {
		t.Errorf("comps[0].Name = %q, want PDU", comps[0].Name)
	}
	if len(conns) != 1 {
		t.Fatalf("want 1 connection, got %d", len(conns))
	}
	if conns[0].Interface != "CAN" {
		t.Errorf("conns[0].Interface = %q, want CAN", conns[0].Interface)
	}
	if len(rep.All()) != 0 {
		t.Errorf("unexpected issues: %v", rep.All())
	}
}

func TestExtractComponents_nilProvider(t *testing.T) {
	rep := &issues.Reporter{}
	comps, conns := ExtractComponents(context.Background(), nil, nil, nil, rep)
	if comps != nil || conns != nil {
		t.Error("nil provider should return nil slices")
	}
}
