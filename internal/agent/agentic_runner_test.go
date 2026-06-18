package agent

import (
	"context"
	"strings"
	"testing"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/llm"
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

type emptyRetriever struct{}

func (emptyRetriever) Search(context.Context, rag.SearchRequest) ([]rag.SearchResult, error) {
	return nil, nil
}

type repeatedStaticPlanner struct{}

func (repeatedStaticPlanner) Plan(context.Context, PlanRequest) (*Plan, error) {
	return &Plan{
		ID:   "repeated",
		Mode: "plan_execute",
		Steps: []PlanStep{
			{
				StepID: "step_01", Action: ActionRetrieve, Query: "T20",
				Params: map[string]any{"doc_types": []string{"product_parameter"}},
			},
			{
				StepID: "step_02", Action: ActionRetrieve, Query: "T20",
				Params:    map[string]any{"doc_types": []string{"product_parameter"}},
				DependsOn: []string{"step_01"},
			},
			{
				StepID: "step_03", Action: ActionFinish,
				Params: map[string]any{}, DependsOn: []string{"step_02"},
			},
		},
		MaxSteps:    5,
		TokenBudget: 2000,
	}, nil
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

type planExecuteTestPlanner struct {
	completeCalls int
	reviseCalls   int
}

func (p *planExecuteTestPlanner) Plan(context.Context, PlanRequest) (*Plan, error) {
	return &Plan{
		ID:          "initial",
		Mode:        "react",
		Steps:       []PlanStep{finishStep(1, "fallback")},
		MaxSteps:    5,
		TokenBudget: 2000,
	}, nil
}

func (p *planExecuteTestPlanner) CompletePlan(context.Context, PlanRequest) (*Plan, error) {
	p.completeCalls++
	return &Plan{
		ID:   "complete",
		Mode: "plan_execute",
		Steps: []PlanStep{
			{
				StepID: "step_01",
				Action: ActionRetrieve,
				Query:  "T20 X20 Pro",
				Params: map[string]any{"doc_types": []string{"product_comparison"}},
			},
			{
				StepID:    "step_02",
				Action:    ActionFinish,
				Params:    map[string]any{},
				DependsOn: []string{"step_01"},
			},
		},
		MaxSteps:    5,
		TokenBudget: 2000,
	}, nil
}

func (p *planExecuteTestPlanner) RevisePlan(
	context.Context,
	PlanRequest,
	*Plan,
	[]PlanStep,
	[]Evidence,
	error,
) (*Plan, error) {
	p.reviseCalls++
	return &Plan{
		ID:   "revised",
		Mode: "plan_execute",
		Steps: []PlanStep{{
			StepID: "step_01",
			Action: ActionClarify,
			Query:  "请补充希望重点比较的维度。",
			Params: map[string]any{},
		}},
		MaxSteps:    5,
		TokenBudget: 2000,
	}, nil
}

func TestAgenticRunnerPlanAndExecuteRevisesAfterZeroRecall(t *testing.T) {
	planner := &planExecuteTestPlanner{}
	runner := NewAgenticRunner(
		fixedRouter{},
		fixedRewriter{},
		planner,
		emptyRetriever{},
		fixedGenerator{},
		nil,
		nil,
		AgenticConfig{
			MaxSteps:     5,
			TokenBudget:  2000,
			PlanningMode: "plan_execute",
			DenseTopK:    5,
			KeywordTopK:  5,
			RerankTopK:   3,
		},
	)

	result, err := runner.Run(context.Background(), Request{
		TraceID:        "tr_plan_execute",
		UserID:         "u_1",
		ConversationID: "cv_1",
		Query:          "T20和X20 Pro怎么选",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if planner.completeCalls != 1 || planner.reviseCalls != 1 {
		t.Fatalf("complete calls = %d, revise calls = %d", planner.completeCalls, planner.reviseCalls)
	}
	if result.Answer == "" {
		t.Fatal("plan-and-execute returned an empty answer")
	}
}

func TestCompatibilityUsesDeterministicSkillPlanning(t *testing.T) {
	if shouldUsePlanExecute("auto", intent.AccessoryCompatibility) {
		t.Fatal("compatibility should not use open-ended plan-and-execute in auto mode")
	}
	if shouldUseReactivePlanning(intent.AccessoryCompatibility) {
		t.Fatal("compatibility should keep the rule-planned deterministic skill")
	}
}

func TestRemoveUnsupportedClaimsRejectsPunctuationOnlyAnswer(t *testing.T) {
	got := removeUnsupportedClaims("错误结论。。", []string{"错误结论"})
	if !strings.Contains(got, "证据不足") {
		t.Fatalf("answer = %q", got)
	}
}

func TestRemoveUnsupportedClaimsDropsWholeMarkdownLines(t *testing.T) {
	answer := `**回答**
P500 红灯一定代表污染。[E1]

**补充信息**
- 使用额定电源。[E2]
- 等待10分钟后会自动恢复。[E1]

**建议**
若仍异常，请联系售后。`
	got := removeUnsupportedClaims(answer, []string{
		"P500 红灯一定代表污染",
		"等待10分钟后会自动恢复",
	})
	if strings.Contains(got, "一定代表污染") || strings.Contains(got, "10分钟") {
		t.Fatalf("unsupported lines remain:\n%s", got)
	}
	for _, want := range []string{"使用额定电源", "联系售后"} {
		if !strings.Contains(got, want) {
			t.Fatalf("answer missing %q:\n%s", want, got)
		}
	}
}

func TestAgenticRunnerRecoversFromRepeatedStaticAction(t *testing.T) {
	runner := NewAgenticRunner(
		fixedRouter{},
		fixedRewriter{},
		repeatedStaticPlanner{},
		fixedRetriever{},
		fixedGenerator{},
		nil,
		nil,
		AgenticConfig{
			MaxSteps:     5,
			TokenBudget:  2000,
			PlanningMode: "react",
			DenseTopK:    5,
			KeywordTopK:  5,
			RerankTopK:   3,
		},
	)

	result, err := runner.Run(context.Background(), Request{
		TraceID:        "tr_repeated",
		UserID:         "u_1",
		ConversationID: "cv_1",
		Query:          "T20吸力多大",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer == "" {
		t.Fatal("repeated action recovery returned an empty answer")
	}
}

func TestAllowedToolsForRouteIncludesSecondaryIntents(t *testing.T) {
	tools := allowedToolsForRoute(intent.Result{
		Secondary:        intent.PurchaseRecommendation,
		SecondaryIntents: []intent.Type{intent.PriceQuery, intent.InventoryQuery},
	})
	for _, expected := range []string{"price_query", "inventory_check"} {
		if !containsString(tools, expected) {
			t.Fatalf("tools = %v, missing %s", tools, expected)
		}
	}
}

func TestRuntimeTokenUsageDoesNotTreatCumulativeBillingAsContext(t *testing.T) {
	got := runtimeTokenUsage(
		100,
		llm.Usage{PromptTokens: 9000, CompletionTokens: 1000, TotalTokens: 10000},
		nil,
		nil,
	)
	if got != 100 {
		t.Fatalf("runtime token usage = %d, want active context size 100", got)
	}
}

func TestMergeEvidencesPreservesExistingCitationOrder(t *testing.T) {
	existing := []Evidence{{
		ID:       "E1",
		Kind:     "kb_chunk",
		SourceID: "comparison:chunk-1",
		Title:    "comparison",
	}}
	merged := mergeEvidences(existing, []rag.SearchResult{{
		ChunkID: "parameter:chunk-1",
		Title:   "parameter",
	}})

	if len(merged) != 2 {
		t.Fatalf("evidence count = %d, want 2", len(merged))
	}
	if merged[0].SourceID != "comparison:chunk-1" || merged[0].ID != "E1" {
		t.Fatalf("existing evidence moved or renumbered incorrectly: %#v", merged)
	}
	if merged[1].SourceID != "parameter:chunk-1" || merged[1].ID != "E2" {
		t.Fatalf("search evidence was not appended: %#v", merged)
	}
}

func TestTicketConfirmationPresentRequiresExplicitPositiveConfirmation(t *testing.T) {
	for _, query := range []string{
		"没确认，先不要创建",
		"我没有确认提交售后工单",
		"帮我看看怎么报修",
	} {
		if ticketConfirmationPresent(query) {
			t.Fatalf("query %q must not authorize ticket creation", query)
		}
	}
	if !ticketConfirmationPresent("订单信息正确，我确认提交售后工单") {
		t.Fatal("explicit ticket confirmation was not recognized")
	}
}

func TestHasToolEvidenceRecognizesCommittedTicket(t *testing.T) {
	evidences := []Evidence{{
		Kind: "tool_result",
		Metadata: map[string]any{
			"tool_name": "create_after_sales_ticket",
		},
	}}
	if !hasToolEvidence(evidences, "create_after_sales_ticket") {
		t.Fatal("validated ticket tool evidence was not recognized")
	}
	if hasToolEvidence(evidences, "price_query") {
		t.Fatal("unrelated tool evidence was incorrectly recognized")
	}
}

func TestCommittedSideEffectToolClassification(t *testing.T) {
	if !isCommittedSideEffectTool("create_after_sales_ticket") {
		t.Fatal("ticket creation must own the final answer")
	}
	if isCommittedSideEffectTool("order_lookup") {
		t.Fatal("read-only order lookup must not replace a committed side-effect answer")
	}
}
