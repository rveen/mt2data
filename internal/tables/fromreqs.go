package tables

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/llm"
	"github.com/rveen/mt2data/internal/provenance"
	"github.com/rveen/mt2data/internal/schema"
	"github.com/rveen/mt2data/internal/tables/units"
)

const reqParamSystemPrompt = `You are a parameter extraction assistant for automotive and systems engineering requirement documents.

Given a list of requirements, extract any physical parameters (measurements, thresholds, ranges, tolerances) as structured data.

Rules:
- Only extract concrete numeric specifications (values, ranges, min/max limits).
- A parameter has a name, optional symbol, optional min, optional typ (typical/nominal), optional max, and a unit.
- If a requirement implies both a minimum and maximum (e.g. "range 0V to 1300V"), set both min and max as plain numbers.
- If only a single limit is stated (e.g. "at least 10 Hz"), set only min or max as appropriate.
- min/max/typ values must be plain numbers only (no unit, no text). Put the unit in the "unit" field.
- symbol: if the requirement or domain convention uses a short identifier for this parameter (e.g. Ubatt, Imax, tResponse, f_sw), set it; otherwise use empty string.
- Include relevant conditions or qualifications as a short "condition" string (empty string if none).
- Names should be concise noun phrases without the verb (e.g. "Voltage range", "Operating temperature", "Response time").
- If a requirement contains no extractable numeric parameter, return null for that entry.
- Return a JSON array with exactly one entry per input requirement, in the same order.

Output format (no commentary, JSON only):
[
  {"req_id":"<id>","name":"<name>","symbol":"<symbol or empty>","min":"<number or null>","max":"<number or null>","typ":"<number or null>","unit":"<unit>","condition":"<condition>"},
  null,
  ...
]`

// reHasNumericSpec matches text that likely contains a numeric specification.
var reHasNumericSpec = regexp.MustCompile(
	`\b\d[\d.,]*\s*(?:V|A|W|Hz|kHz|MHz|GHz|ms|µs|us|ns|s|m|km|mm|cm|°C|K|Ω|ohm|dB|%|bar|N|kg|g|rpm|Ah|kWh|lx)\b` +
		`|` +
		`(?i)\b(?:range|minimum|maximum|at least|at most|up to|no more than|not less than|no less than|between)\b.*\d`)

const batchSize = 20

// llmParamResult is the JSON shape returned by the LLM for one requirement.
type llmParamResult struct {
	ReqID     string  `json:"req_id"`
	Name      string  `json:"name"`
	Symbol    string  `json:"symbol"`
	Min       *string `json:"min"`
	Max       *string `json:"max"`
	Typ       *string `json:"typ"`
	Unit      string  `json:"unit"`
	Condition string  `json:"condition"`
}

// ExtractParamsFromRequirements uses an LLM to extract physical parameters that are
// embedded in requirement text (e.g. "shall measure 0V to 1300V DC").
//
// Only requirements whose text contains a likely numeric specification are sent to the LLM.
// Requests are batched (batchSize per call) to limit token usage.
func ExtractParamsFromRequirements(
	ctx context.Context,
	reqs []schema.Requirement,
	provider llm.Provider,
	rep *issues.Reporter,
	seq *int,
) []schema.Parameter {
	// Pre-filter: only requirements with numeric content.
	var candidates []schema.Requirement
	for _, r := range reqs {
		if reHasNumericSpec.MatchString(r.Text) {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	var params []schema.Parameter
	for start := 0; start < len(candidates); start += batchSize {
		end := min(start+batchSize, len(candidates))
		batch := candidates[start:end]
		extracted := callLLMBatch(ctx, batch, provider, rep, seq)
		params = append(params, extracted...)
	}
	return params
}

// callLLMBatch sends one batch to the LLM and parses the response.
func callLLMBatch(
	ctx context.Context,
	batch []schema.Requirement,
	provider llm.Provider,
	rep *issues.Reporter,
	seq *int,
) []schema.Parameter {
	// Build user message: JSON array of {id, text}.
	type reqInput struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	inputs := make([]reqInput, len(batch))
	for i, r := range batch {
		inputs[i] = reqInput{ID: r.ID, Text: r.Text}
	}
	userBytes, err := json.Marshal(inputs)
	if err != nil {
		rep.Add(issues.KindUnknownBlock, "llm-batch", fmt.Sprintf("marshal inputs: %v", err))
		return nil
	}

	raw, err := provider.Call(ctx, reqParamSystemPrompt, string(userBytes))
	if err != nil {
		rep.Add(issues.KindUnknownBlock, "llm-batch", fmt.Sprintf("LLM call: %v", err))
		return nil
	}

	raw = llm.StripCodeFence(raw)
	var results []*llmParamResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		rep.Add(issues.KindUnknownBlock, "llm-batch", fmt.Sprintf("parse LLM response: %v", err))
		return nil
	}

	// Build a quick lookup from req ID → requirement (for source + section).
	reqByID := make(map[string]schema.Requirement, len(batch))
	for _, r := range batch {
		reqByID[r.ID] = r
	}

	var params []schema.Parameter
	for i, res := range results {
		if res == nil || i >= len(batch) {
			continue
		}
		if res.Name == "" {
			continue
		}
		*seq++
		origReq := reqByID[res.ReqID]
		p := schema.Parameter{
			ID:     fmt.Sprintf("PARAM-%04d", *seq),
			Name:   res.Name,
			Symbol: res.Symbol,
			Source: provenance.Source{
				BlockID:  origReq.Source.BlockID,
				Clause:   origReq.Section,
				OrigText: batch[i].Text,
			},
		}
		if res.Condition != "" {
			p.Conditions = []schema.Condition{{Quantity: "condition", Raw: res.Condition}}
		}
		p.Min = parseQuantityStr(res.Min, res.Unit)
		p.Typ = parseQuantityStr(res.Typ, res.Unit)
		p.Max = parseQuantityStr(res.Max, res.Unit)
		params = append(params, p)
	}
	return params
}

// parseQuantityStr parses an optional string value + unit hint into a *schema.Quantity.
// val contains only the numeric part (e.g. "1300"); unitHint is the separate unit string.
func parseQuantityStr(val *string, unitHint string) *schema.Quantity {
	if val == nil {
		return nil
	}
	s := strings.TrimSpace(*val)
	if s == "" || s == "null" {
		return nil
	}
	// Try parsing the value as-is (handles "1300 V" if the LLM included the unit).
	q := units.Parse(s)
	if q != nil {
		unit := q.Unit
		if unit == "" {
			// Numeric parsed but no unit in the value string — use the hint directly.
			unit = canonicalUnit(unitHint)
		}
		return &schema.Quantity{Value: q.Value, Unit: unit, Raw: s}
	}
	// Numeric parse failed; store as raw text with the unit hint.
	return &schema.Quantity{Raw: s, Unit: canonicalUnit(unitHint)}
}

// canonicalUnit normalises a unit hint string.
// For multi-word hints like "V DC" it checks whether the first token is a known unit alias
// and uses it; otherwise the full hint is returned unchanged.
func canonicalUnit(hint string) string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return ""
	}
	// Try the full string first (handles single-token units like "V", "°C", "ms").
	if q := units.Parse("1 " + hint); q != nil && q.Unit != "" {
		return q.Unit
	}
	// Try just the first token (handles "V DC", "kHz rms", etc.).
	if first := strings.Fields(hint)[0]; first != hint {
		if q := units.Parse("1 " + first); q != nil && q.Unit != "" {
			return q.Unit
		}
	}
	return hint
}
