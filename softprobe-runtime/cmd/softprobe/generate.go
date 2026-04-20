package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func runGenerate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "jest-session":
		return runGenerateJestSession(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return 2
	}
}

func runGenerateJestSession(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("generate jest-session", flag.ContinueOnError)
	fs.SetOutput(stderr)
	casePath := fs.String("case", "", "case file path")
	outPath := fs.String("out", "", "output TypeScript module path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *casePath == "" {
		_, _ = fmt.Fprintln(stderr, "generate jest-session requires --case")
		return 2
	}

	caseBytes, err := os.ReadFile(*casePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "generate jest-session failed: %v\n", err)
		return 1
	}

	var doc caseDocument
	if err := json.Unmarshal(caseBytes, &doc); err != nil {
		_, _ = fmt.Fprintf(stderr, "generate jest-session failed: invalid case: %v\n", err)
		return 1
	}

	if *outPath == "" {
		base := strings.TrimSuffix(filepath.Base(*casePath), filepath.Ext(*casePath))
		*outPath = filepath.Join(filepath.Dir(*casePath), base+".replay.session.ts")
	}

	moduleSource, err := generateJestSessionModule(*casePath, *outPath, doc)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "generate jest-session failed: %v\n", err)
		return 1
	}

	if err := os.WriteFile(*outPath, []byte(moduleSource), 0o600); err != nil {
		_, _ = fmt.Fprintf(stderr, "generate jest-session failed: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "wrote %s\n", *outPath)
	return 0
}
