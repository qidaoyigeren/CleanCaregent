package generator

import (
	"context"
	"fmt"

	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
)

// Fallback wraps a primary and secondary generator. Calls to Generate
// and GenerateWithScenario are tried on the primary first; on failure
// the secondary is used.
type Fallback struct {
	primary   Generator
	secondary Generator
}

func NewFallback(primary, secondary Generator) *Fallback {
	return &Fallback{primary: primary, secondary: secondary}
}

func (g *Fallback) Name() string {
	return g.primary.Name() + "|fallback:" + g.secondary.Name()
}

func (g *Fallback) Generate(ctx context.Context, query string, evidence []rag.SearchResult) (string, error) {
	answer, err := g.primary.Generate(ctx, query, evidence)
	if err == nil {
		return answer, nil
	}
	fallbackAnswer, fallbackErr := g.secondary.Generate(ctx, query, evidence)
	if fallbackErr != nil {
		return "", fmt.Errorf("primary generator failed: %v; fallback generator failed: %w", err, fallbackErr)
	}
	return fallbackAnswer, nil
}

func (g *Fallback) GenerateWithScenario(
	ctx context.Context,
	scenario prompt.Scenario,
	query string,
	evidence []rag.SearchResult,
	toolResults string,
	conversationSummary string,
	models string,
) (string, error) {
	answer, err := g.primary.GenerateWithScenario(ctx, scenario, query, evidence, toolResults, conversationSummary, models)
	if err == nil {
		return answer, nil
	}
	fallbackAnswer, fallbackErr := g.secondary.GenerateWithScenario(ctx, scenario, query, evidence, toolResults, conversationSummary, models)
	if fallbackErr != nil {
		return "", fmt.Errorf("primary scenario generator failed: %v; fallback failed: %w", err, fallbackErr)
	}
	return fallbackAnswer, nil
}
