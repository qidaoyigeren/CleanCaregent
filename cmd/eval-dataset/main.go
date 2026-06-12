package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"CleanCaregent/internal/eval"
)

func main() {
	output := flag.String("output", "docs/eval/eval-cases-v2.json", "output JSON path")
	flag.Parse()
	raw, err := json.MarshalIndent(eval.DefaultCases(), "", "  ")
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(*output, append(raw, '\n'), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %d eval cases to %s\n", len(eval.DefaultCases()), *output)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
