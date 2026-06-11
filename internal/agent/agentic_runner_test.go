package agent

import (
	"context"
	"testing"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
)

type fixedRouter struct{}

func (fixedRouter) Route(context.Context, intent.RouteRequest) (intent.Result, error) {
	return intent.Result{
		Primary:    "presales",
		Secondary:  intent.ProductComparison,
		Confidence: 0.95,
		Entities:   map[string]string{"models": "T20,X20 Pro"},
	}, nil
}

type fixedRewriter struct{}

func (fixedRewriter) Rewrite(context.Context, RewriteRequest) (RewriteResult, error) {
	return RewriteResult{
		Original:     "compare",
		Rewritten:    "compare T20 and X20 Pro",
		SubQuestions: []string{"compare pet hair handling"},
		Entities:     map[string]string{"models": "T20,X20 Pro"},
	}, nil
}

type observingPlanner struct {
	calls                int
	sawSearchObservation bool
}

func (p *observingPlanner) Plan(context.Context, PlanRequest) (*Plan, error) {
	return &Plan{
		ID:          "plan_1",
		Mode:        "react",
		Intent:      intent.ProductComparison,
		Steps:       []PlanStep{finishStep(1, "fallback")},
		MaxSteps:    5,
		TokenBudget: 2000,
		Confidence:  0.95,
	}, nil
}

func (p *observingPlanner) NextStep(
	_ context.Context,
	_ PlanRequest,
	currentStep int,
	_ []Evidence,
	searchResults string,
	_ string,
) (*PlanStep, error) {
	p.calls++
	if currentStep == 0 {
		return &PlanStep{
			StepID: "step_01",
			Action: ActionRetrieve,
			Query:  "T20 X20 Pro pet hair",
			Params: map[string]any{
				"doc_types": []string{"product_comparison"},
			},
		}, nil
	}
	p.sawSearchObservation = searchResults != ""
	return nil, nil
}

type fixedRetriever struct{}

func (fixedRetriever) Search(context.Context, rag.SearchRequest) ([]rag.SearchResult, error) {
	return []rag.SearchResult{{
		ChunkID:    "chunk_1",
		DocumentID: "doc_1",
		Title:      "T20 与 X20 Pro 对比",
		Content:    "X20 Pro 使用防缠绕胶刷，T20 使用双主刷。",
	}}, nil
}

type fixedGenerator struct{}

func (fixedGenerator) Name() string { return "fixed" }
func (fixedGenerator) Generate(context.Context, string, []rag.SearchResult) (string, error) {
	return "X20 Pro 更适合重视防缠绕的养宠家庭。[E1]", nil
}
func (fixedGenerator) GenerateWithScenario(
	ctx context.Context,
	_ prompt.Scenario,
	query string,
	evidence []rag.SearchResult,
	_ string,
	_ string,
	_ string,
) (string, error) {
	return fixedGenerator{}.Generate(ctx, query, evidence)
}

func TestAgenticRunnerUsesReactivePlannerAfterObservation(t *testing.T) {
	planner := &observingPlanner{}
	runner := NewAgenticRunner(
		fixedRouter{},
		fixedRewriter{},
		planner,
		fixedRetriever{},
		fixedGenerator{},
		nil,
		nil,
		AgenticConfig{
			MaxSteps:            5,
			TokenBudget:         2000,
			DenseTopK:           5,
			KeywordTopK:         5,
			RerankTopK:          3,
			EnableLLMComponents: true,
		},
	)

	result, err := runner.Run(context.Background(), Request{
		TraceID:        "tr_1",
		UserID:         "u_1",
		ConversationID: "cv_1",
		Query:          "T20和X20 Pro哪个适合养猫",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if planner.calls != 2 {
		t.Fatalf("NextStep calls = %d, want 2", planner.calls)
	}
	if !planner.sawSearchObservation {
		t.Fatal("planner did not receive retrieval observation")
	}
	if len(result.Evidences) != 1 || result.Evidences[0].ID != "E1" {
		t.Fatalf("evidences = %#v", result.Evidences)
	}
}
