package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"CleanCaregent/internal/compatibility"
	"CleanCaregent/internal/config"
	"CleanCaregent/internal/diagnosis"
	"CleanCaregent/internal/ingest"
	"CleanCaregent/internal/productpack"
	"CleanCaregent/internal/seed"
	"CleanCaregent/internal/service"
)

func main() {
	kbPathsFlag := flag.String("kb-paths", "", "comma or semicolon separated knowledge pack files/directories")
	productPackPathsFlag := flag.String("product-packs", "", "comma or semicolon separated product pack files/directories")
	builtinFlag := flag.Bool("builtin", true, "include built-in mock knowledge documents")
	flag.Parse()

	cfg, err := config.Load("")
	if err != nil {
		fail("load config", err)
	}

	seedPaths := cfg.Knowledge.SeedPaths
	if raw := strings.TrimSpace(os.Getenv("CLEANCARE_KB_SEED_PATHS")); raw != "" {
		seedPaths = splitPaths(raw)
	}
	if strings.TrimSpace(*kbPathsFlag) != "" {
		seedPaths = splitPaths(*kbPathsFlag)
	}
	productPackPaths := cfg.Knowledge.ProductPackPaths
	if raw := strings.TrimSpace(os.Getenv("CLEANCARE_PRODUCT_PACK_PATHS")); raw != "" {
		productPackPaths = splitPaths(raw)
	}
	if strings.TrimSpace(*productPackPathsFlag) != "" {
		productPackPaths = splitPaths(*productPackPathsFlag)
	}
	includeBuiltIn := cfg.Knowledge.SeedBuiltInMock
	if flagWasSet("builtin") {
		includeBuiltIn = *builtinFlag
	}

	var validationErrs []error
	packs, err := productpack.Load(productPackPaths...)
	if err != nil {
		validationErrs = append(validationErrs, err)
	} else {
		validationErrs = append(validationErrs, productpack.Validate(packs)...)
	}

	var documents []service.IngestDocumentRequest
	if includeBuiltIn {
		documents = append(documents, seed.DefaultKnowledgeDocuments()...)
	}
	externalDocuments, err := ingest.LoadKnowledgeDocuments(seedPaths...)
	if err != nil {
		validationErrs = append(validationErrs, err)
	} else {
		documents = append(documents, externalDocuments...)
	}
	packDocuments, err := productpack.KnowledgeDocuments(packs)
	if err != nil {
		validationErrs = append(validationErrs, err)
	} else {
		documents = append(documents, packDocuments...)
	}
	validationErrs = append(validationErrs, validateDocuments(documents)...)

	matrix := compatibility.NewDefaultMatrix()
	matrix.Merge(productpack.CompatibilityEntries(packs))
	engine := diagnosis.NewDefaultEngine()
	diagnosisData := productpack.DiagnosisEntries(packs)
	if err := engine.Merge(diagnosisData.Nodes, diagnosisData.Roots, diagnosisData.SafetyKeywords); err != nil {
		validationErrs = append(validationErrs, err)
	}

	if len(validationErrs) > 0 {
		for _, err := range validationErrs {
			fmt.Fprintf(os.Stderr, "validation error: %v\n", err)
		}
		os.Exit(1)
	}
	fmt.Printf(
		"kb validation passed: documents=%d external=%d pack_docs=%d product_packs=%d products=%d compatibility_entries=%d diagnosis_nodes=%d builtin=%t\n",
		len(documents),
		len(externalDocuments),
		len(packDocuments),
		len(packs),
		len(productpack.Registry(packs).Products()),
		len(matrix.Entries()),
		len(diagnosisData.Nodes),
		includeBuiltIn,
	)
}

func validateDocuments(documents []service.IngestDocumentRequest) []error {
	seen := map[string]struct{}{}
	var errs []error
	for _, document := range documents {
		if err := service.ValidateIngestDocument(document); err != nil {
			errs = append(errs, fmt.Errorf("%s@%s: %w", document.DocID, effectiveVersion(document.Version), err))
		}
		key := document.DocID + "@" + effectiveVersion(document.Version)
		if _, exists := seen[key]; exists {
			errs = append(errs, fmt.Errorf("duplicate knowledge document %s", key))
		}
		seen[key] = struct{}{}
	}
	return errs
}

func effectiveVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "1.0"
	}
	return version
}

func splitPaths(value string) []string {
	parts := strings.FieldsFunc(value, func(current rune) bool {
		return current == ',' || current == ';'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func flagWasSet(name string) bool {
	wasSet := false
	flag.Visit(func(item *flag.Flag) {
		if item.Name == name {
			wasSet = true
		}
	})
	return wasSet
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
}
