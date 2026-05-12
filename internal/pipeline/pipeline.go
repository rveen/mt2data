// Package pipeline orchestrates all extraction stages for one MT document.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rveen/mt2data/internal/classify"
	"github.com/rveen/mt2data/internal/components"
	"github.com/rveen/mt2data/internal/hierarchy"
	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/llm"
	"github.com/rveen/mt2data/internal/merge"
	"github.com/rveen/mt2data/internal/parse"
	"github.com/rveen/mt2data/internal/refs"
	"github.com/rveen/mt2data/internal/requirements"
	"github.com/rveen/mt2data/internal/schema"
	"github.com/rveen/mt2data/internal/tables"
	"github.com/rveen/mt2data/internal/toon"
)

// Options configures a pipeline run.
type Options struct {
	OutputBase string       // base path; writes <base>.md and <base>.json
	Provider   llm.Provider // if set, LLM extraction of params from requirements is enabled
}

// Run processes the MT file at mtPath through all pipeline stages and writes both outputs.
// It returns the final Document IR.
func Run(mtPath string, opts *Options) (*schema.Document, error) {
	ctx := context.Background()
	data, err := os.ReadFile(mtPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", mtPath, err)
	}
	text := string(data)

	rep := &issues.Reporter{}

	// Stage 1: parse
	parsed := parse.Parse(text)

	// Stage 2: hierarchy
	hier := hierarchy.Build(parsed, rep)

	// Stage 3: classify
	decisions := classify.Classify(parsed)

	// Stage 4: normalize tables (TOON tables) + key-value parameter paragraphs
	tableResult := tables.Normalize(parsed, decisions, hier.BlockClause, rep)
	kvSeq := 1000 // offset to avoid ID collisions with TOON-table params
	kvParams := tables.ExtractKeyValueParams(parsed, hier.BlockClause, rep, &kvSeq)
	tableResult.Parameters = append(tableResult.Parameters, kvParams...)

	// Stage 5: requirement extraction from prose and lists
	autoSeq := 0
	proseReqs := requirements.ExtractFromBlocks(parsed, hier.BlockClause, rep, &autoSeq)
	listReqs := requirements.ExtractFromList(parsed, hier.BlockClause, rep, &autoSeq)
	allReqs := append(proseReqs, listReqs...)

	// Stage 6: reference resolution
	allRefs := refs.Resolve(parsed, rep)

	// Stage 7: merge/dedup
	dedupedReqs := merge.Requirements(allReqs, rep)

	// Stage 7b: LLM classification — populate Domain, Verification, Title (optional).
	if opts != nil && opts.Provider != nil {
		dedupedReqs = requirements.ClassifyRequirements(ctx, dedupedReqs, opts.Provider, rep)
		testCount := 0
		for _, r := range dedupedReqs {
			if strings.Contains(r.Domain, "test") {
				testCount++
			}
		}
		fmt.Fprintf(os.Stderr, "mt2data: classified %d requirements (%d test)\n", len(dedupedReqs), testCount)
	}

	// Stage 7c: LLM component extraction (optional).
	var docComponents []schema.Component
	var docConnections []schema.Connection
	if opts != nil && opts.Provider != nil {
		docComponents, docConnections = components.ExtractComponents(
			ctx, hier.Clauses, dedupedReqs, opts.Provider, rep)
		fmt.Fprintf(os.Stderr, "mt2data: extracted %d components, %d connections\n",
			len(docComponents), len(docConnections))
	}

	// Stage 4b: LLM-based parameter extraction from requirement text (optional).
	if opts != nil && opts.Provider != nil {
		llmSeq := 2000 // offset to avoid ID collisions with TOON/kv params
		llmParams := tables.ExtractParamsFromRequirements(ctx, dedupedReqs, opts.Provider, rep, &llmSeq)
		tableResult.Parameters = append(tableResult.Parameters, llmParams...)
		fmt.Fprintf(os.Stderr, "mt2data: LLM extracted %d parameters from requirements\n", len(llmParams))
	}
	dedupedParams := merge.Parameters(tableResult.Parameters, rep)

	// Product structure tree (always built; no LLM needed).
	productTree := hierarchy.ProductTree(hier.Clauses, dedupedReqs)

	// Stage 8: assemble document IR
	allTrees := tableResult.Trees
	if productTree != nil {
		allTrees = append(allTrees, *productTree)
	}
	docID := strings.TrimSuffix(filepath.Base(mtPath), filepath.Ext(mtPath))
	doc := &schema.Document{
		ID:   docID,
		Kind: detectKind(parsed, dedupedReqs, dedupedParams),
		Metadata: schema.DocumentMeta{
			Title:      extractTitle(parsed),
			SourceFile: filepath.Base(mtPath),
		},
		Clauses:      hier.Clauses,
		Components:   docComponents,
		Connections:  docConnections,
		Parameters:   dedupedParams,
		Requirements: dedupedReqs,
		Trees:        allTrees,
		References:   allRefs,
		Issues:       rep.All(),
	}

	// Coverage metric
	unknown := countDecisionType(decisions, classify.Unknown)
	total := len(decisions)
	if total > 0 {
		pct := float64(unknown) / float64(total) * 100
		fmt.Fprintf(os.Stderr, "mt2data: %d blocks, %d unknown (%.1f%%)\n", total, unknown, pct)
	}

	// Write outputs
	if opts != nil && opts.OutputBase != "" {
		if err := writeOutputs(opts.OutputBase, doc); err != nil {
			return doc, err
		}
	}

	return doc, nil
}

func writeOutputs(base string, doc *schema.Document) error {
	dir := filepath.Dir(base)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
	}

	// Strip extension from base if given
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if ext == "" {
		stem = base
	}

	// TOON output: product structure → requirements (non-test) → tests → parameters.
	// Split requirements by domain so test specs go to their own table.
	var nonTestReqs, testReqs []schema.Requirement
	for _, r := range doc.Requirements {
		if strings.Contains(r.Domain, "test") {
			testReqs = append(testReqs, r)
		} else {
			nonTestReqs = append(nonTestReqs, r)
		}
	}

	var toonBuf strings.Builder
	for _, tree := range doc.Trees {
		if tree.ID == "product_structure" {
			toonBuf.WriteString(hierarchy.ProductTreeTOON(&tree))
			toonBuf.WriteByte('\n')
			break
		}
	}
	if compOut := toon.AssembleComponents(doc.Components); compOut != "" {
		toonBuf.WriteString(compOut)
		toonBuf.WriteByte('\n')
	}
	if connOut := toon.AssembleConnections(doc.Connections); connOut != "" {
		toonBuf.WriteString(connOut)
		toonBuf.WriteByte('\n')
	}
	toonBuf.WriteString(toon.AssembleRequirements(nonTestReqs))
	if testsOut := toon.AssembleTests(testReqs); testsOut != "" {
		toonBuf.WriteByte('\n')
		toonBuf.WriteString(testsOut)
	}
	if paramOut := toon.AssembleParameters(doc.Parameters); paramOut != "" {
		toonBuf.WriteByte('\n')
		toonBuf.WriteString(paramOut)
	}
	toonOut := toonBuf.String()
	if err := os.WriteFile(stem+".md", []byte(toonOut), 0o644); err != nil {
		return fmt.Errorf("write TOON: %w", err)
	}

	// JSON IR
	jsonBytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	if err := os.WriteFile(stem+".json", append(jsonBytes, '\n'), 0o644); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}

	return nil
}

func countDecisionType(decisions []classify.Decision, sem classify.SemanticType) int {
	n := 0
	for _, d := range decisions {
		if d.Semantic == sem {
			n++
		}
	}
	return n
}

// extractTitle returns the text of the first H1 heading in the document.
func extractTitle(doc *parse.Document) string {
	for _, blk := range doc.Blocks {
		if blk.Type == parse.TypeHeading && blk.Level == 1 {
			return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(blk.Raw), "#"))
		}
	}
	return ""
}

// detectKind heuristically classifies the document as rfq, norm, datasheet, or other.
//
//   - rfq:       many requirements with numeric OEM IDs (7+ digit integers)
//   - norm:      many requirements, mostly auto-assigned IDs (no OEM IDs)
//   - datasheet: few requirements, many parameters
//   - other:     everything else
func detectKind(_ *parse.Document, reqs []schema.Requirement, params []schema.Parameter) string {
	numericIDs := 0
	for _, r := range reqs {
		if !r.IDIsAuto && len(r.ID) >= 7 {
			allDigits := true
			for _, c := range r.ID {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				numericIDs++
			}
		}
	}
	if numericIDs >= 5 {
		return "rfq"
	}
	if len(reqs) >= 20 {
		return "norm"
	}
	if len(params) > len(reqs) {
		return "datasheet"
	}
	return "other"
}
