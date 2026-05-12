// Command mt2data extracts structured data from an MT (Markdown+TOON) document.
//
// Usage:
//
//	mt2data -o <output-base> input.mt
//
// Always writes:
//
//	<output-base>.md   — TOON requirements table (human-readable)
//	<output-base>.json — full document IR (machine-readable)
//
// LLM parameter extraction is enabled automatically when ANTHROPIC_API_KEY or
// OPENAI_API_KEY is set (Claude is preferred). Use -model to override the model.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rveen/mt2data/internal/llm"
	"github.com/rveen/mt2data/internal/pipeline"
)

func main() {
	var (
		outputBase = flag.String("o", "", "output base path (required); writes <path>.md and <path>.json")
		modelName  = flag.String("model", "", "LLM model override (default: claude-sonnet-4-6 or gpt-4o)")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -o <output-base> [flags] input.mt\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	if *outputBase == "" {
		fmt.Fprintln(os.Stderr, "mt2data: -o output base path is required")
		os.Exit(1)
	}

	p, err := llm.NewAutoProvider(*modelName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mt2data: LLM init: %v\n", err)
		os.Exit(1)
	}
	if p == nil {
		fmt.Fprintln(os.Stderr, "mt2data: no ANTHROPIC_API_KEY or OPENAI_API_KEY set; skipping LLM parameter extraction")
	}

	mtPath := flag.Arg(0)
	_, err = pipeline.Run(mtPath, &pipeline.Options{OutputBase: *outputBase, Provider: p})
	if err != nil {
		fmt.Fprintf(os.Stderr, "mt2data: %v\n", err)
		os.Exit(1)
	}
}
