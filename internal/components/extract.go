// Package components implements LLM-based product component and connection extraction.
package components

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/llm"
	"github.com/rveen/mt2data/internal/schema"
)

const systemPrompt = `You extract product component structure from automotive/systems engineering requirements.

Given a list of section titles (document structure) and functional requirements, identify:

1. **components**: distinct physical modules, sensors, actuators, software components, or external
   interfaces that are subjects or objects of requirements. These are real engineering entities —
   not document sections. Use short, canonical names (e.g. "HVDC Contactor", "Voltage Sensor",
   "Charging Manager", "PDU").
   - type: one of module | sensor | actuator | interface | software | other

2. **connections**: interfaces or dependencies between components inferred from requirement text.
   - interface: one of CAN | LIN | HVDC | HV-analog | LV-analog | digital-IO | thermal | mechanical | other

Return a single JSON object. No commentary.

Output format:
{
  "components": [
    {"name":"<name>","type":"<type>","description":"<one sentence>"},
    ...
  ],
  "connections": [
    {"from":"<component name>","to":"<component name>","interface":"<type>","description":"<one sentence>"},
    ...
  ]
}`

// maxReqs caps the number of requirements sent to the LLM to bound token usage.
const maxReqs = 200

// llmResult is the top-level JSON shape returned by the LLM.
type llmResult struct {
	Components  []schema.Component  `json:"components"`
	Connections []schema.Connection `json:"connections"`
}

// ExtractComponents calls the LLM once with a condensed document context and returns
// the identified product components and their connections.
// Returns nil slices (no error) when provider is nil.
func ExtractComponents(
	ctx context.Context,
	clauses []schema.Clause,
	reqs []schema.Requirement,
	provider llm.Provider,
	rep *issues.Reporter,
) ([]schema.Component, []schema.Connection) {
	if provider == nil {
		return nil, nil
	}

	userMsg := buildUserMessage(clauses, reqs)

	raw, err := provider.Call(ctx, systemPrompt, userMsg)
	if err != nil {
		rep.Add(issues.KindUnknownBlock, "llm-components", fmt.Sprintf("LLM call: %v", err))
		return nil, nil
	}

	raw = llm.StripCodeFence(raw)
	var result llmResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		rep.Add(issues.KindUnknownBlock, "llm-components", fmt.Sprintf("parse response: %v", err))
		return nil, nil
	}

	return result.Components, result.Connections
}

// buildUserMessage assembles the LLM input: condensed section titles + functional requirements.
func buildUserMessage(clauses []schema.Clause, reqs []schema.Requirement) string {
	var sb strings.Builder

	// Part 1: condensed clause titles (depth ≤ 4, skipping very deep numbering).
	sb.WriteString("## Document sections\n")
	for _, c := range clauses {
		if len(c.Path) > 4 {
			continue
		}
		indent := strings.Repeat("  ", len(c.Path)-1)
		fmt.Fprintf(&sb, "%s%s %s\n", indent, c.ID, c.Title)
	}

	// Part 2: functional requirements — explicit IDs, non-test, capped at maxReqs.
	sb.WriteString("\n## Functional requirements\n")
	type reqEntry struct {
		ID      string `json:"id"`
		Section string `json:"section"`
		Text    string `json:"text"`
	}
	var entries []reqEntry
	for _, r := range reqs {
		if r.IDIsAuto {
			continue
		}
		if strings.Contains(r.Domain, "test") {
			continue
		}
		entries = append(entries, reqEntry{ID: r.ID, Section: r.Section, Text: r.Text})
		if len(entries) >= maxReqs {
			break
		}
	}
	reqJSON, _ := json.Marshal(entries)
	sb.Write(reqJSON)

	return sb.String()
}
