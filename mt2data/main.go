// Command mt2req extracts a structured requirements table from an MT document.
//
// Usage:
//
//	mt2req [flags] input.mt
//
// Flags:
//
//	-o string      output file (required)
//	-j             also write a JSON array alongside the TOON table
//	-model string  Claude model (default claude-sonnet-4-6)
//
// Requires ANTHROPIC_API_KEY in the environment.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	mt2req "github.com/rveen/mt2req"
)

var Usage = func() {
	fmt.Fprintf(flag.CommandLine.Output(), "MT to requirements table extractor\nUsage of %s:\n", os.Args[0])
	flag.PrintDefaults()
	fmt.Fprintln(flag.CommandLine.Output(), "\n  The ANTHROPIC_API_KEY must be set in the environment.")
}

func main() {
	var (
		outputFile = flag.String("o", "", "output file (required)")
		jsonOut    = flag.Bool("j", false, "also write a JSON array alongside the TOON table")
		model      = flag.String("model", "", "Claude model (default claude-sonnet-4-6)")
	)

	flag.Usage = Usage
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "MT to requirements table extractor\nUsage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\n  The ANTHROPIC_API_KEY must be set in the environment.")
		os.Exit(1)
	}

	if *outputFile == "" {
		fmt.Fprintln(os.Stderr, "Set an output file with -o (otherwise you could lose your money)")
		os.Exit(1)
	}

	mtPath := flag.Arg(0)
	opts := &mt2req.Options{
		Model:      *model,
		OutputFile: *outputFile,
		JSON:       *jsonOut,
	}

	_, err := mt2req.Extract(context.Background(), mtPath, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mt2req: %v\n", err)
		os.Exit(1)
	}
}
