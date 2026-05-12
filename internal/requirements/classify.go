package requirements

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/llm"
	"github.com/rveen/mt2data/internal/schema"
)

const classifySystemPrompt = `You classify automotive and systems engineering requirements.

For each requirement assign:
- domain: comma-separated subset of {system, hardware, software, test}
  Use "test" when the requirement describes: a measurement specification, accuracy or resolution limit,
  sampling rate, diagnostic procedure, test coverage, or verification evidence. A requirement can
  be both functional and test (e.g. "system, test").
- verification: single letter — T (by test/measurement), A (by analysis), I (by inspection), D (by demonstration)
- title: concise 5-10 word noun phrase summarising what is required

Return a JSON array with exactly one entry per input requirement, in the same order.
Output format (no commentary, JSON only):
[
  {"id":"<id>","domain":"<domain>","verification":"<T|A|I|D>","title":"<title>"},
  ...
]`

const classifyBatchSize = 20

// llmClassifyResult is the JSON shape returned by the LLM for one requirement.
type llmClassifyResult struct {
	ID           string `json:"id"`
	Domain       string `json:"domain"`
	Verification string `json:"verification"`
	Title        string `json:"title"`
}

// ClassifyRequirements uses an LLM to populate Domain, Verification, and Title on each
// requirement. It returns the same slice with those fields filled in.
// If the provider is nil the slice is returned unchanged.
func ClassifyRequirements(
	ctx context.Context,
	reqs []schema.Requirement,
	provider llm.Provider,
	rep *issues.Reporter,
) []schema.Requirement {
	if provider == nil || len(reqs) == 0 {
		return reqs
	}

	// Build an id→index map for patching results back.
	idxByID := make(map[string]int, len(reqs))
	for i, r := range reqs {
		idxByID[r.ID] = i
	}

	for start := 0; start < len(reqs); start += classifyBatchSize {
		end := min(start+classifyBatchSize, len(reqs))
		batch := reqs[start:end]
		patchClassifications(ctx, batch, reqs, idxByID, provider, rep)
	}
	return reqs
}

// patchClassifications sends one batch and writes results back into the reqs slice.
func patchClassifications(
	ctx context.Context,
	batch []schema.Requirement,
	reqs []schema.Requirement,
	idxByID map[string]int,
	provider llm.Provider,
	rep *issues.Reporter,
) {
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
		rep.Add(issues.KindUnknownBlock, "llm-classify", fmt.Sprintf("marshal: %v", err))
		return
	}

	raw, err := provider.Call(ctx, classifySystemPrompt, string(userBytes))
	if err != nil {
		rep.Add(issues.KindUnknownBlock, "llm-classify", fmt.Sprintf("LLM call: %v", err))
		return
	}

	raw = llm.StripCodeFence(raw)
	var results []llmClassifyResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		rep.Add(issues.KindUnknownBlock, "llm-classify", fmt.Sprintf("parse response: %v", err))
		return
	}

	for _, res := range results {
		idx, ok := idxByID[res.ID]
		if !ok {
			rep.Add(issues.KindUnknownBlock, "llm-classify",
				fmt.Sprintf("unknown requirement ID %q in LLM response", res.ID))
			continue
		}
		if res.Domain != "" {
			reqs[idx].Domain = res.Domain
		}
		if res.Verification != "" {
			reqs[idx].Verification = res.Verification
		}
		if res.Title != "" {
			reqs[idx].Title = res.Title
		}
	}
}
