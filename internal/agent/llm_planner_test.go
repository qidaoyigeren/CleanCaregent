package agent

import (
	"context"
	"encoding/json"
	"testing"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/tool"
)

func TestLLMPlannerUsesGuardedSkillForKnownComplexIntent(t *testing.T) {
	planner := NewLLMPlanner(nil, nil)
	step, err := planner.NextStep(context.Background(), PlanRequest{
		Query: "T20 充不进电怎么办",
		Intent: intent.Result{
			Secondary: intent.Troubleshooting,
			Entities:  map[string]string{"models": "T20"},
		},
		MaxSteps: 5,
	}, 0, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if step == nil {
		t.Fatal("expected a guarded skill step")
	}
	if step.Action != ActionRunSkill || step.SkillName != "fault_diagnosis" {
		t.Fatalf("step = %#v", step)
	}
	if step.ReasonCode != "guarded_skill_entry" {
		t.Fatalf("reason = %q", step.ReasonCode)
	}
}

func TestEvaluatePlanRejectsFinishFirstAndCycles(t *testing.T) {
	request := PlanRequest{Intent: intent.Result{Secondary: intent.ProductComparison}}
	plan := &Plan{Steps: []PlanStep{
		{StepID: "step_01", Action: ActionFinish, DependsOn: []string{"step_02"}},
		{StepID: "step_02", Action: ActionRetrieve, DependsOn: []string{"step_01"}},
	}}
	evaluation := evaluatePlan(request, plan)
	if evaluation.Score >= 4 {
		t.Fatalf("evaluation = %#v", evaluation)
	}
}

func TestValidateNextStepChecksToolSchema(t *testing.T) {
	planner := NewLLMPlanner(nil, nil, tool.Definition{
		Name: "price_query",
		ParamsSchema: json.RawMessage(
			`{"type":"object","required":["product_refs"],"properties":{"product_refs":{"type":"array"}}}`,
		),
	})
	err := planner.ValidateNextStep(context.Background(), PlanRequest{
		Intent:       intent.Result{Secondary: intent.PriceQuery},
		AllowedTools: []string{"price_query"},
	}, PlanStep{
		Action: ActionCallTool, ToolName: "price_query",
		Params: map[string]any{"product_refs": "T20"},
	}, nil)
	if err == nil {
		t.Fatal("expected schema validation error")
	}
}

func TestValidateNextStepAcceptsAggregatedToolDefinition(t *testing.T) {
	planner := NewLLMPlanner(nil, nil, tool.Definition{
		Name: "primary/price_query",
		ParamsSchema: json.RawMessage(
			`{"type":"object","required":["product_refs"],"properties":{"product_refs":{"type":"array"}}}`,
		),
	})
	err := planner.ValidateNextStep(context.Background(), PlanRequest{
		Intent:       intent.Result{Secondary: intent.PriceQuery},
		AllowedTools: []string{"price_query"},
	}, PlanStep{
		Action:   ActionCallTool,
		ToolName: "primary/price_query",
		Params:   map[string]any{"product_refs": []string{"T20"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	definitions := planner.allowedLLMToolDefinitions([]string{"price_query"})
	if len(definitions) != 1 || definitions[0].Name != "primary/price_query" {
		t.Fatalf("definitions = %#v", definitions)
	}
}

func TestValidateNextStepAcceptsParallelRetrievalAndTool(t *testing.T) {
	planner := NewLLMPlanner(nil, nil, tool.Definition{
		Name: "price_query",
		ParamsSchema: json.RawMessage(
			`{"type":"object","required":["product_refs"],"properties":{"product_refs":{"type":"array"}}}`,
		),
	})
	err := planner.ValidateNextStep(context.Background(), PlanRequest{
		Intent:       intent.Result{Secondary: intent.PriceQuery},
		AllowedTools: []string{"price_query"},
	}, PlanStep{
		Action: ActionParallel,
		SubSteps: []PlanStep{
			{Action: ActionRetrieve, Query: "T20 参数", Params: map[string]any{}},
			{Action: ActionCallTool, ToolName: "price_query", Params: map[string]any{"product_refs": []string{"T20"}}},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLLMPlannerDoesNotForceGuardedSkillAfterFirstStep(t *testing.T) {
	planner := NewLLMPlanner(nil, nil)
	step, err := planner.NextStep(context.Background(), PlanRequest{
		Query:  "T20 充不进电怎么办",
		Intent: intent.Result{Secondary: intent.Troubleshooting},
	}, 1, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if step != nil {
		t.Fatalf("step = %#v, want nil without an LLM after the first step", step)
	}
}

func TestLLMPlannerUsesGuardedToolForDynamicIntent(t *testing.T) {
	planner := NewLLMPlanner(nil, nil)
	step, err := planner.NextStep(context.Background(), PlanRequest{
		Query: "P400多少钱",
		Intent: intent.Result{
			Secondary: intent.PriceQuery,
			Entities:  map[string]string{"models": "P400"},
		},
		AllowedTools: []string{"price_query"},
		MaxSteps:     5,
	}, 0, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if step == nil || step.Action != ActionCallTool || step.ToolName != "price_query" {
		t.Fatalf("step = %#v", step)
	}
}

func TestValidateCompleteStepsRejectsUnrequestedRecommendationTools(t *testing.T) {
	planner := NewLLMPlanner(nil, nil)
	_, err := planner.validateCompleteSteps(PlanRequest{
		Query:        "120平两只猫预算5000，推荐扫地机器人",
		Intent:       intent.Result{Secondary: intent.PurchaseRecommendation},
		AllowedTools: []string{"price_query", "inventory_check"},
		MaxSteps:     5,
	}, []llmCompletePlanStep{
		{
			Action:   "call_tool",
			ToolName: "price_query",
			Params:   map[string]any{"product_refs": []string{"T20"}},
		},
		{Action: "finish"},
	})
	if err == nil {
		t.Fatal("expected unrequested price tool to be rejected")
	}
}

func TestValidateCompleteStepsRequiresTerminalStep(t *testing.T) {
	planner := NewLLMPlanner(nil, nil)
	steps := make([]llmCompletePlanStep, 5)
	for index := range steps {
		steps[index] = llmCompletePlanStep{
			Action:        "retrieve",
			Query:         "候选产品",
			SearchQueries: []string{"候选产品"},
		}
	}
	_, err := planner.validateCompleteSteps(PlanRequest{
		Intent:   intent.Result{Secondary: intent.PurchaseRecommendation},
		MaxSteps: 5,
	}, steps)
	if err == nil {
		t.Fatal("expected a plan without a terminal step to be rejected")
	}
}

func TestSkillOwnedIntentsPreferRuleCompletePlan(t *testing.T) {
	for _, intentType := range []intent.Type{
		intent.ProductComparison,
		intent.PurchaseRecommendation,
		intent.AccessoryCompatibility,
		intent.Troubleshooting,
		intent.WarrantyQuery,
		intent.ReturnEligibility,
		intent.CreateAfterSalesTicket,
	} {
		if !preferRuleCompletePlan(intentType) {
			t.Fatalf("%s should use the deterministic complete plan", intentType)
		}
	}
	if preferRuleCompletePlan(intent.ProductParameter) {
		t.Fatal("simple retrieval intent does not need this skill-plan guard")
	}
}
