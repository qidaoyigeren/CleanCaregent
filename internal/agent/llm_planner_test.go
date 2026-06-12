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
