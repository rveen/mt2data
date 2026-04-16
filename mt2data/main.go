// Command mt2data extracts a structured requirements table from an MT document.
//
// Usage:
//
//	mt2data [flags] input.mt
//
// Flags:
//
//	-o string          output file (required)
//	-j                 also write a JSON array alongside the TOON table
//	-provider string   LLM provider: claude (default) or openai
//	-model string      model override (default: claude-sonnet-4-6 / gpt-4o)
//
// Required environment variables:
//
//	ANTHROPIC_API_KEY  when using -provider claude (default)
//	OPENAI_API_KEY     when using -provider openai
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	mt2data "github.com/rveen/mt2data"
)

var Usage = func() {
	fmt.Fprintf(flag.CommandLine.Output(), "MT to requirements table extractor\nUsage of %s:\n", os.Args[0])
	flag.PrintDefaults()
	fmt.Fprintln(flag.CommandLine.Output(), "\n  Set ANTHROPIC_API_KEY (claude) or OPENAI_API_KEY (openai) in the environment.")
}

func main() {
	var (
		outputFile = flag.String("o", "", "output file (required)")
		jsonOut    = flag.Bool("j", false, "also write a JSON array alongside the TOON table")
		provider   = flag.String("provider", "claude", "LLM provider: claude or openai")
		model      = flag.String("model", "", "model override (default: claude-sonnet-4-6 / gpt-4o)")
	)

	flag.Usage = Usage
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "MT to requirements table extractor\nUsage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\n  Set ANTHROPIC_API_KEY (claude) or OPENAI_API_KEY (openai) in the environment.")
		os.Exit(1)
	}

	if *outputFile == "" {
		fmt.Fprintln(os.Stderr, "Set an output file with -o (otherwise you could lose your money)")
		os.Exit(1)
	}

	mtPath := flag.Arg(0)
	opts := &mt2data.Options{
		Provider:   *provider,
		Model:      *model,
		OutputFile: *outputFile,
		JSON:       *jsonOut,
	}

	_, err := mt2data.Extract(context.Background(), mtPath, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mt2data: %v\n", err)
		os.Exit(1)
	}
}
