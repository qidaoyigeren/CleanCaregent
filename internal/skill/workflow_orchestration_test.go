package skill

import (
	"context"
	"strings"
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
	modelName := "scenario"
	if len(request.Filter.Models) > 0 {
		modelName = request.Filter.Models[0]
	}
	return []rag.SearchResult{{
		ChunkID:     modelName + request.Filter.DocTypes[0],
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

type countingWorkflowGenerator struct {
	calls int
}

func (g *countingWorkflowGenerator) Name() string { return "counting" }
func (g *countingWorkflowGenerator) Generate(context.Context, string, []rag.SearchResult) (string, error) {
	g.calls++
	return "generated", nil
}
func (g *countingWorkflowGenerator) GenerateWithScenario(
	context.Context,
	prompt.Scenario,
	string,
	[]rag.SearchResult,
	string,
	string,
	string,
) (string, error) {
	g.calls++
	return "generated", nil
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

func TestProductComparisonUsesEvidenceDrivenAnswerBeforeGenerator(t *testing.T) {
	retriever := &recordingRetriever{}
	generator := &countingWorkflowGenerator{}
	workflow := NewProductComparison(
		retriever,
		generator,
		nil,
		WorkflowConfig{DenseTopK: 5, KeywordTopK: 5, RerankTopK: 3},
	)
	result, err := workflow.Run(context.Background(), Request{
		TraceID: "tr_compare_deterministic",
		Query:   "FD4 和 GB2 哪个适合大面积清洁",
		Intent: intent.Result{
			Secondary: intent.ProductComparison,
			Entities:  map[string]string{"models": "FD4,GB2"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if generator.calls != 0 {
		t.Fatalf("generator calls = %d, want 0", generator.calls)
	}
	if result.AnswerDraft == "" || !strings.Contains(result.AnswerDraft, "知识依据") {
		t.Fatalf("unexpected deterministic answer: %q", result.AnswerDraft)
	}
}

func TestAfterSalesPolicyRetrievalIgnoresProductEntityFilters(t *testing.T) {
	retriever := &recordingRetriever{}
	workflow := NewAfterSalesJudgement(
		retriever,
		workflowGenerator{},
		nil,
		WorkflowConfig{DenseTopK: 5, KeywordTopK: 5, RerankTopK: 6},
	)
	_, err := workflow.Run(context.Background(), Request{
		TraceID: "tr_after_sales",
		Query:   "CC20260603001这单的P400保修到哪天",
		Intent: intent.Result{
			Secondary: intent.WarrantyQuery,
			Entities: map[string]string{
				"order_no": "CC20260603001",
				"models":   "P400",
				"category": "air_purifier",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(retriever.requests) != 1 {
		t.Fatalf("retrieval requests = %d", len(retriever.requests))
	}
	request := retriever.requests[0]
	if len(request.Filter.Models) != 0 || len(request.Filter.Categories) != 0 {
		t.Fatalf("policy retrieval leaked entity filters: %#v", request.Filter)
	}
	if len(request.Filter.DocTypes) == 0 || request.Filter.DocTypes[0] != "after_sales_policy" {
		t.Fatalf("policy doc types = %#v", request.Filter.DocTypes)
	}
}
