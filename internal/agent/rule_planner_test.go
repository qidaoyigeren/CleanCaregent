package agent

import (
	"context"
	"testing"

	"CleanCaregent/internal/intent"
)

func TestRulePlannerUsesSkillsForComplexIntents(t *testing.T) {
	planner := NewRulePlanner()
	tests := []struct {
		intent    intent.Type
		wantSkill string
	}{
		{intent.ProductComparison, "product_comparison"},
		{intent.PurchaseRecommendation, "purchase_recommendation"},
		{intent.AccessoryCompatibility, "accessory_compatibility"},
		{intent.Troubleshooting, "fault_diagnosis"},
		{intent.ReturnEligibility, "after_sales_judgement"},
	}
	for _, test := range tests {
		plan, err := planner.Plan(context.Background(), PlanRequest{
			Query:       "test",
			Intent:      intent.Result{Secondary: test.intent, Confidence: 0.9, Entities: map[string]string{"models": "T20"}},
			MaxSteps:    5,
			TokenBudget: 1000,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Steps) == 0 || plan.Steps[0].Action != ActionRunSkill || plan.Steps[0].SkillName != test.wantSkill {
			t.Fatalf("intent %s planned %#v", test.intent, plan.Steps)
		}
	}
}

func TestRulePlannerPassesEntitiesToTool(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanRequest{
		Query:       "T20多少钱",
		Intent:      intent.Result{Secondary: intent.PriceQuery, Confidence: 0.9, Entities: map[string]string{"models": "T20"}},
		MaxSteps:    5,
		TokenBudget: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Steps[0].Params["models"]; got != "T20" {
		t.Fatalf("models = %#v", got)
	}
}

func TestRulePlannerUsesFocusedRetrievalForParameterQuery(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanRequest{
		Query:            "T20核心参数",
		RewrittenQueries: []string{"T20 核心参数", "T20 规格参数"},
		Intent: intent.Result{
			Secondary:  intent.ProductParameter,
			Confidence: 0.98,
			Entities:   map[string]string{"models": "T20"},
		},
		MaxSteps:    5,
		TokenBudget: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[0].Action != ActionRetrieve {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	if got := plan.Steps[0].Params["max_results"]; got != 3 {
		t.Fatalf("max_results = %#v", got)
	}
	docTypes, _ := plan.Steps[0].Params["doc_types"].([]string)
	if len(docTypes) != 2 || docTypes[0] != "product_parameter" {
		t.Fatalf("doc_types = %#v", docTypes)
	}
}
