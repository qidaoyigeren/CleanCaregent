package generator

import (
	"context"

	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
)

// Generator produces answers grounded in retrieved evidence.
type Generator interface {
	Name() string
	// Generate produces an answer using the default (generic) template.
	Generate(ctx context.Context, query string, evidence []rag.SearchResult) (string, error)
	// GenerateWithScenario produces an answer using a scenario-specific template.
	// Supported scenarios: generate_generic, generate_compare, generate_diagnose, generate_policy.
	GenerateWithScenario(
		ctx context.Context,
		scenario prompt.Scenario,
		query string,
		evidence []rag.SearchResult,
		toolResults string,
		conversationSummary string,
		models string,
	) (string, error)
}
