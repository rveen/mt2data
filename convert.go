// Package mt2req extracts a structured requirements table from an MT
// (Markdown+TOON) document produced by pdf2mt, using the Claude API.
//
// The output is a TOON block with one row per requirement:
//
//	requirements[N]{section|title|domain|verb|verification}:
//	  4.3.2.1 | System provides 12 V power supply | hardware | MUST | T
//
// Environment:
//
//	ANTHROPIC_API_KEY  — required
package mt2req

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Options controls extraction behaviour.
type Options struct {
	// Model is the Claude model (default "claude-sonnet-4-6").
	Model string
	// OutputFile, if set, writes the resulting table to this path.
	OutputFile string
	// JSON, if true, also writes a JSON array alongside the TOON table.
	// The JSON file is written to the same path as OutputFile with the
	// extension replaced by ".json" (e.g. "out.md" → "out.json").
	JSON bool
}

func (o *Options) model() string {
	if o == nil || o.Model == "" {
		return "claude-sonnet-4-6"
	}
	return o.Model
}

// requirement mirrors the JSON schema the LLM is asked to produce.
// ID is assigned by Extract after all blocks are processed.
// ID and Compound are retained in the structure for downstream use but are
// not included in the TOON printout.
type requirement struct {
	ID           string `json:"id"`
	Section      string `json:"section"`
	Item         string `json:"item"`
	Title        string `json:"title"`
	Domain       string `json:"domain"`
	Verb         string `json:"verb"`
	Verification string `json:"verification"`
	Compound     string `json:"compound"`
}

// textBlock is a contiguous slice of document text associated with a section heading.
type textBlock struct {
	section string // heading text (stripped of # markers), or empty if before any heading
	text    string // full text of the section including its header line
}

// rfc2119VerbMap is the hardcoded RFC2119 verb normalization table.
// Keys are lower-case; values are canonical upper-case verbs.
var rfc2119VerbMap = map[string]string{
	"shall":      "MUST",
	"must":       "MUST",
	"required":   "MUST",
	"should":     "SHOULD",
	"recommended": "SHOULD",
	"may":        "MAY",
	"optional":   "MAY",
	"must not":   "MUST NOT",
	"shall not":  "MUST NOT",
	"should not": "SHOULD NOT",
}

// Extract reads the MT file at mtPath, calls the Claude API once per clause
// block, and returns a TOON requirements table.
func Extract(ctx context.Context, mtPath string, opts *Options) (string, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}

	mtBytes, err := os.ReadFile(mtPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", mtPath, err)
	}

	blocks := splitTextBlocks(string(mtBytes))
	if len(blocks) == 0 {
		return "", fmt.Errorf("no content found in %s", mtPath)
	}

	var allReqs []requirement
	seq := 0

	for i, blk := range blocks {
		reqs, err := callClaudeForBlock(ctx, apiKey, opts.model(), blk)
		if err != nil {
			label := blk.section
			if label == "" {
				label = fmt.Sprintf("chunk %d", i+1)
			}
			fmt.Fprintf(os.Stderr, "mt2req: section %s: %v\n", label, err)
			continue
		}
		for _, r := range reqs {
			seq++
			r.ID = fmt.Sprintf("%d", seq)
			r.Verb = normalizeVerb(r.Verb)
			if r.Domain == "" {
				r.Domain = "system"
			}
			if r.Section == "" {
				r.Section = blk.section
			}
			allReqs = append(allReqs, r)
		}
	}

	result := assembleTOON(allReqs)

	if opts != nil && opts.OutputFile != "" {
		dir := filepath.Dir(opts.OutputFile)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return result, fmt.Errorf("mkdir: %w", err)
			}
		}
		if err := os.WriteFile(opts.OutputFile, []byte(result), 0o644); err != nil {
			return result, fmt.Errorf("write output: %w", err)
		}
		if opts.JSON {
			jsonPath := jsonOutputPath(opts.OutputFile)
			if err := os.WriteFile(jsonPath, []byte(assembleJSON(allReqs)), 0o644); err != nil {
				return result, fmt.Errorf("write JSON output: %w", err)
			}
		}
	}

	return result, nil
}

// ----------------------------------------------------------------- document parsing

// reHeading matches Markdown headings (1–6 #-signs) and captures the heading text.
var reHeading = regexp.MustCompile(`^#{1,6}\s+(.+)`)

// splitTextBlocks partitions the document at every Markdown heading.
// Each block contains the heading line and all subsequent lines up to
// (but not including) the next heading. Text before the first heading
// is collected into a block with an empty section name.
// If the document contains no headings the entire text is returned as
// a single block with an empty section name.
func splitTextBlocks(text string) []textBlock {
	lines := strings.Split(text, "\n")
	var blocks []textBlock
	var cur *textBlock

	for _, line := range lines {
		if m := reHeading.FindStringSubmatch(line); m != nil {
			// Flush previous block.
			if cur != nil {
				cur.text = strings.TrimSpace(cur.text)
				if cur.text != "" {
					blocks = append(blocks, *cur)
				}
			}
			cur = &textBlock{
				section: strings.TrimSpace(m[1]),
				text:    line + "\n",
			}
		} else {
			if cur == nil {
				// Text before any heading — start a preamble block.
				cur = &textBlock{section: ""}
			}
			cur.text += line + "\n"
		}
	}

	// Flush the last block.
	if cur != nil {
		cur.text = strings.TrimSpace(cur.text)
		if cur.text != "" {
			blocks = append(blocks, *cur)
		}
	}

	return blocks
}

// ----------------------------------------------------------------- verb helpers

// normalizeVerb returns the canonical RFC2119 verb for v.
// If v is not in the RFC2119 map the original value (uppercased) is returned.
func normalizeVerb(v string) string {
	if canonical, ok := rfc2119VerbMap[strings.ToLower(v)]; ok {
		return canonical
	}
	return strings.ToUpper(v)
}

// ----------------------------------------------------------------- Claude API

const systemPrompt = `You are a requirements extraction tool for systems engineering documents.

Extract every normative requirement from the text supplied by the user.
A normative requirement is any sentence containing a modal verb such as shall, must, should, may,
must not, or should not. Non-normative sentences (descriptions, rationale, notes, examples) must
be ignored.

Output a JSON array — one object per requirement. If there are no requirements, output [].
Output ONLY the JSON array — no prose, no code fences.

## Output schema (one JSON object per requirement)

{
  "section":      "nearest section heading in the supplied text, if discernible; otherwise empty string",
  "item":         "the element that is the object or target of the requirement, extracted from the text (e.g. vehicle, electronic module); empty string if not discernible",
  "title":        "one-sentence summary of the requirement in active voice, including any condition (≤20 words)",
  "domain":       "one or more of: system, hardware, software, test (comma-separated if multiple)",
  "verb":         "the source modal verb exactly as written in the text (e.g. shall, must, should)",
  "verification": "implied or stated verification method: T (test), A (analysis), I (inspection), D (demonstration)",
  "compound":     "yes if this row was split from a compound source sentence, no if it was already atomic"
}

## Domain taxonomy

- system   — not yet allocated to HW or SW; or a cross-cutting constraint
- hardware — allocated to or constrained by hardware implementation
- software — allocated to or constrained by software implementation
- test     — a requirement on the test process, test coverage, or test evidence

## Rules

- Extract ONLY normative requirements (sentences containing shall, must, should, may, must not, etc.).
- If a sentence contains multiple independent constraints (compound requirement), split it into one
  JSON object per atomic constraint. Set compound: "yes" on every object that came from a split.
  Atomic requirements that were never compound get compound: "no".
- Summarize the full requirement — including any condition ("when X", "if Y") — into the "title"
  field in plain active-voice English. Do not omit the condition from the summary.
- For the "section" field, use the nearest heading present in the supplied text.`

// callClaudeForBlock sends a single text block to the Claude API and returns
// the parsed requirements. A JSON parse failure is non-fatal (nil, nil).
func callClaudeForBlock(ctx context.Context, apiKey, model string, blk textBlock) ([]requirement, error) {
	header := ""
	if blk.section != "" {
		header = fmt.Sprintf("Section: %s\n\n", blk.section)
	}
	userMsg := header + blk.text

	body := map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"system":     systemPrompt,
		"messages": []map[string]any{
			{"role": "user", "content": userMsg},
		},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude API %d: %s", resp.StatusCode, respBytes)
	}

	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, err
	}

	var text string
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	text = stripCodeFence(strings.TrimSpace(text))

	// The LLM may return multiple JSON arrays (one per content block when the
	// response is streamed or split). Decode them all and merge into one slice.
	var reqs []requirement
	dec := json.NewDecoder(strings.NewReader(text))
	for dec.More() {
		var batch []requirement
		if err := dec.Decode(&batch); err != nil {
			fmt.Fprintf(os.Stderr, "mt2req: section %s: cannot parse LLM response as JSON: %v\n", blk.section, err)
			return nil, nil
		}
		reqs = append(reqs, batch...)
	}
	return reqs, nil
}

// stripCodeFence removes a leading ```json or ``` fence and its closing ``` from
// an LLM response so the remainder can be parsed as plain JSON.
func stripCodeFence(s string) string {
	// Remove opening fence (```json or ```)
	if after, ok := strings.CutPrefix(s, "```json"); ok {
		s = strings.TrimSpace(after)
	} else if after, ok := strings.CutPrefix(s, "```"); ok {
		s = strings.TrimSpace(after)
	} else {
		return s
	}
	// Remove closing fence
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	return s
}

// ----------------------------------------------------------------- TOON assembly

// pipeEscape replaces literal " | " sequences inside a field value so they do
// not break the pipe-separated TOON row format. The replacement " / " is chosen
// to be visually similar and unambiguous.
func pipeEscape(s string) string {
	return strings.ReplaceAll(s, " | ", " / ")
}

// assembleTOON builds the final TOON requirements table from all extracted
// requirements in document order. IDs must already be set on each requirement.
func assembleTOON(reqs []requirement) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "requirements[%d]{Requirement|Section|Item|Domain|Verb|Verification}:\n", len(reqs))

	for _, r := range reqs {
		row := fmt.Sprintf("  %s | %s | %s | %s | %s | %s",
		    pipeEscape(r.Title),
			pipeEscape(r.Section),
			pipeEscape(r.Item),
			pipeEscape(r.Domain),
			pipeEscape(r.Verb),
			pipeEscape(r.Verification),
		)
		sb.WriteString(row)
		sb.WriteByte('\n')
	}

	return sb.String()
}

// jsonOutputPath derives the JSON output path from the TOON output path by
// replacing the file extension with ".json" (e.g. "out.md" → "out.json").
func jsonOutputPath(toonPath string) string {
	ext := filepath.Ext(toonPath)
	return strings.TrimSuffix(toonPath, ext) + ".json"
}

// assembleJSON serialises the requirements as a pretty-printed JSON array.
// IDs must already be set on each requirement.
func assembleJSON(reqs []requirement) string {
	if reqs == nil {
		reqs = []requirement{}
	}
	b, err := json.MarshalIndent(reqs, "", "  ")
	if err != nil {
		// Should never happen with this type.
		return "[]\n"
	}
	return string(b) + "\n"
}
