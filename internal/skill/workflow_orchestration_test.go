package skill

import (
	"context"
	"sync"
	"testing"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
)

type recordingRetriever struct {
	mu       sync.Mutex
	requests []rag.SearchRequest
}

func (r *recordingRetriever) Search(
	_ context.Context,
	request rag.SearchRequest,
) ([]rag.SearchResult, error) {
	r.mu.Lock()
	r.requests = append(r.requests, request)
	index := len(r.requests)
	r.mu.Unlock()
	return []rag.SearchResult{{
		ChunkID:     request.Filter.Models[0] + request.Filter.DocTypes[0],
		DocumentID:  "doc",
		Title:       "evidence",
		Content:     "grounded evidence",
		FusionScore: float64(index),
	}}, nil
}

type workflowGenerator struct{}

func (workflowGenerator) Name() string { return "test" }
func (workflowGenerator) Generate(context.Context, string, []rag.SearchResult) (string, error) {
	return "answer", nil
}
func (workflowGenerator) GenerateWithScenario(
	context.Context,
	prompt.Scenario,
	string,
	[]rag.SearchResult,
	string,
	string,
	string,
) (string, error) {
	return "answer", nil
}

func TestProductComparisonRetrievesEachModelAndScenarioGuide(t *testing.T) {
	retriever := &recordingRetriever{}
	workflow := NewProductComparison(
		retriever,
		workflowGenerator{},
		nil,
		WorkflowConfig{DenseTopK: 5, KeywordTopK: 5, RerankTopK: 3},
	)
	result, err := workflow.Run(context.Background(), Request{
		TraceID: "tr_compare",
		Query:   "T20和X20 Pro哪个适合养猫",
		Intent: intent.Result{
			Secondary: intent.ProductComparison,
			Entities:  map[string]string{"models": "T20,X20 Pro"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.AnswerDraft == "" {
		t.Fatal("comparison returned empty answer")
	}
	if len(retriever.requests) != 3 {
		t.Fatalf("retrieval routes = %d, want 3", len(retriever.requests))
	}
	modelRoutes := map[string]bool{}
	hasGuideRoute := false
	for _, request := range retriever.requests {
		if len(request.Filter.Models) == 1 {
			modelRoutes[request.Filter.Models[0]] = true
		}
		for _, docType := range request.Filter.DocTypes {
			if docType == "purchase_guide" {
				hasGuideRoute = true
			}
		}
	}
	if !modelRoutes["T20"] || !modelRoutes["X20 Pro"] || !hasGuideRoute {
		t.Fatalf("requests = %#v", retriever.requests)
	}
}
