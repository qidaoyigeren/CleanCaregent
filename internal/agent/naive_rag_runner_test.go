package agent

import (
	"context"
	"testing"

	"CleanCaregent/internal/generator"
	"CleanCaregent/internal/rag"
)

func TestNaiveRAGRunnerExtractsProductModelFilter(t *testing.T) {
	retriever := &capturingRetriever{}
	runner := NewNaiveRAGRunner(
		retriever,
		generator.NewExtractive(500),
		NaiveRAGConfig{DenseTopK: 10, KeywordTopK: 10, RerankTopK: 5},
	)

	_, err := runner.Run(context.Background(), Request{Query: "T20 和 X20 Pro 哪个适合养猫？"}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(retriever.request.Filter.Models) != 2 ||
		retriever.request.Filter.Models[0] != "T20" ||
		retriever.request.Filter.Models[1] != "X20 Pro" {
		t.Fatalf("model filters = %v", retriever.request.Filter.Models)
	}
}

type capturingRetriever struct {
	request rag.SearchRequest
}

func (r *capturingRetriever) Search(_ context.Context, request rag.SearchRequest) ([]rag.SearchResult, error) {
	r.request = request
	return []rag.SearchResult{{
		ChunkID:    "chunk",
		DocumentID: "doc",
		Title:      "title",
		Content:    "content",
	}}, nil
}
