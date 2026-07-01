package generator

import (
	"context"
	"testing"
	"time"

	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
)

type countingGenerator struct {
	calls int
}

func (g *countingGenerator) Name() string { return "counting" }

func (g *countingGenerator) Generate(context.Context, string, []rag.SearchResult) (string, error) {
	g.calls++
	return "answer", nil
}

func (g *countingGenerator) GenerateWithScenario(
	context.Context,
	prompt.Scenario,
	string,
	[]rag.SearchResult,
	string,
	string,
	string,
) (string, error) {
	g.calls++
	return "answer", nil
}

func TestCachedGeneratorReusesSameGenerationContext(t *testing.T) {
	base := &countingGenerator{}
	cached := NewCached(base, time.Minute, 8)
	evidence := []rag.SearchResult{{
		ChunkID:    "chunk-1",
		DocumentID: "doc-1",
		Title:      "title",
		Content:    "grounded evidence",
		Metadata:   map[string]any{"model": "FD4"},
	}}

	for i := 0; i < 2; i++ {
		answer, err := cached.GenerateWithScenario(
			context.Background(),
			prompt.ScenarioGenerateCompare,
			"FD4 和 GB2 怎么选",
			evidence,
			"price_query: FD4",
			"",
			"FD4,GB2",
		)
		if err != nil {
			t.Fatal(err)
		}
		if answer != "answer" {
			t.Fatalf("answer = %q", answer)
		}
	}
	if base.calls != 1 {
		t.Fatalf("calls = %d, want 1", base.calls)
	}
}

func TestCachedGeneratorSeparatesToolContext(t *testing.T) {
	base := &countingGenerator{}
	cached := NewCached(base, time.Minute, 8)
	evidence := []rag.SearchResult{{ChunkID: "chunk-1", DocumentID: "doc-1", Content: "evidence"}}

	_, _ = cached.GenerateWithScenario(context.Background(), prompt.ScenarioGenerateCompare, "query", evidence, "price=1", "", "FD4")
	_, _ = cached.GenerateWithScenario(context.Background(), prompt.ScenarioGenerateCompare, "query", evidence, "price=2", "", "FD4")

	if base.calls != 2 {
		t.Fatalf("calls = %d, want 2", base.calls)
	}
}
