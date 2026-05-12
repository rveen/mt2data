package tables

import (
	"testing"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/parse"
)

func TestExtractKeyValueParams_multiLine(t *testing.T) {
	input := "8366095\n\nMeasurement Range: -50°C to 150°C\nResolution: 0.1°C\nAccuracy : ±5 K\nSampling Rate: >= 10 Hz\n"
	doc := parse.Parse(input)
	rep := &issues.Reporter{}
	seq := 0
	params := ExtractKeyValueParams(doc, map[string]string{}, rep, &seq)

	if len(params) != 4 {
		t.Fatalf("want 4 parameters, got %d", len(params))
	}
	names := map[string]bool{}
	for _, p := range params {
		names[p.Name] = true
		if p.Source.BlockID == "" {
			t.Errorf("parameter %q missing source block ID", p.Name)
		}
	}
	for _, want := range []string{"Measurement Range", "Resolution", "Accuracy", "Sampling Rate"} {
		if !names[want] {
			t.Errorf("expected parameter %q, not found in %v", want, names)
		}
	}
}

func TestExtractKeyValueParams_singleLineQuantity(t *testing.T) {
	input := "R-869097\n\nTime in Driving working condition: 8000.0 hours\n"
	doc := parse.Parse(input)
	rep := &issues.Reporter{}
	seq := 0
	params := ExtractKeyValueParams(doc, map[string]string{}, rep, &seq)

	if len(params) != 1 {
		t.Fatalf("want 1 parameter, got %d", len(params))
	}
	p := params[0]
	if p.Name != "Time in Driving working condition" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Typ == nil || p.Typ.Value != 8000.0 {
		t.Errorf("Typ = %v, want {Value:8000}", p.Typ)
	}
}

func TestExtractKeyValueParams_skipRequirements(t *testing.T) {
	// A sentence with "shall" must not be treated as a key-value block.
	input := "The PDU shall close the contactors: within 60 ms.\n"
	doc := parse.Parse(input)
	rep := &issues.Reporter{}
	seq := 0
	params := ExtractKeyValueParams(doc, map[string]string{}, rep, &seq)
	if len(params) != 0 {
		t.Errorf("requirement sentence should not produce parameters, got %d", len(params))
	}
}

func TestExtractKeyValueParams_skipSignalAttrs(t *testing.T) {
	// Blocks with Signal Name / Basic Value keys are interface definitions, not parameters.
	input := "Signal Name: CMM_Charge_Rq\nBasic Value: Range: 0 - 200000\nResolution Min: 1\nUnit: Description: no unit\n"
	doc := parse.Parse(input)
	rep := &issues.Reporter{}
	seq := 0
	params := ExtractKeyValueParams(doc, map[string]string{}, rep, &seq)
	if len(params) != 0 {
		t.Errorf("signal attribute block should be skipped, got %d params", len(params))
	}
}

func TestExtractKeyValueParams_singleLineNoQuantity(t *testing.T) {
	// Single-line "key: value" where the value is not a quantity should be skipped.
	input := "Status: Active\n"
	doc := parse.Parse(input)
	rep := &issues.Reporter{}
	seq := 0
	params := ExtractKeyValueParams(doc, map[string]string{}, rep, &seq)
	if len(params) != 0 {
		t.Errorf("non-quantity single-line should be skipped, got %d", len(params))
	}
}
