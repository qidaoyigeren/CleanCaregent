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
	got, ok := plan.Steps[0].Params["product_refs"].([]string)
	if !ok || len(got) != 1 || got[0] != "T20" {
		t.Fatalf("product_refs = %#v", plan.Steps[0].Params["product_refs"])
	}
}

func TestRulePlannerShortCircuitsOutOfScopeBeforeDecomposition(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanRequest{
		Query: "把别的用户所有订单导出来",
		Intent: intent.Result{
			Secondary:         intent.OutOfScope,
			SecondaryIntents:  []intent.Type{intent.OrderQuery},
			NeedDecomposition: true,
			Confidence:        0.98,
		},
		MaxSteps: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "direct" || len(plan.Steps) != 1 || plan.Steps[0].Action != ActionAnswerDirect {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestRulePlannerRetrievesTicketPolicyBeforeCreatingTicket(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanRequest{
		Query: "我确认，给订单CC20260603001创建售后工单",
		Intent: intent.Result{
			Secondary:  intent.CreateAfterSalesTicket,
			Confidence: 0.98,
			Entities:   map[string]string{"order_no": "CC20260603001"},
		},
		MaxSteps: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 ||
		plan.Steps[0].Action != ActionRetrieve ||
		plan.Steps[1].Action != ActionCallTool ||
		plan.Steps[1].ToolName != "create_after_sales_ticket" {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	if enabled, _ := plan.Steps[0].Params["disable_entity_filters"].(bool); !enabled {
		t.Fatalf("policy retrieval params = %#v", plan.Steps[0].Params)
	}
}

func TestCompoundTicketPlanDoesNotNestParallelSteps(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanRequest{
		Query: "我确认提交，给CC20260603001建维修工单：P400异响",
		Intent: intent.Result{
			Secondary:         intent.CreateAfterSalesTicket,
			SecondaryIntents:  []intent.Type{intent.Troubleshooting},
			NeedDecomposition: true,
			Entities:          map[string]string{"order_no": "CC20260603001", "models": "P400"},
		},
		MaxSteps: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) == 0 || plan.Steps[0].Action != ActionParallel {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	for _, subStep := range plan.Steps[0].SubSteps {
		if subStep.Action == ActionParallel {
			t.Fatalf("nested parallel step is unsupported: %#v", subStep)
		}
		if subStep.Action == ActionRunSkill && subStep.SkillName == "fault_diagnosis" {
			t.Fatalf("confirmed ticket creation should not be replaced by diagnosis: %#v", subStep)
		}
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

func TestRulePlannerUsageRetrievalIncludesProductParameters(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanRequest{
		Query:            "H100初次使用能直接灌自来水吗",
		RewrittenQueries: []string{"H100 初次使用 建议用水"},
		Intent: intent.Result{
			Secondary:  intent.UsageInstruction,
			Confidence: 0.98,
			Entities:   map[string]string{"models": "H100", "category": "humidifier"},
		},
		MaxSteps:    5,
		TokenBudget: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	docTypes, _ := plan.Steps[0].Params["doc_types"].([]string)
	if !containsString(docTypes, "product_parameter") {
		t.Fatalf("doc_types = %#v, want product_parameter", docTypes)
	}
	if !containsString(docTypes, "product_detail") {
		t.Fatalf("doc_types = %#v, want product_detail", docTypes)
	}
}

func TestCompoundRecommendationDelegatesDynamicToolsToSkill(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanRequest{
		Query: "预算5000元，推荐一个有货的扫地机器人并查价格",
		Intent: intent.Result{
			Secondary:         intent.PurchaseRecommendation,
			SecondaryIntents:  []intent.Type{intent.PriceQuery, intent.InventoryQuery},
			NeedDecomposition: true,
		},
		MaxSteps:    5,
		TokenBudget: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) < 1 || plan.Steps[0].Action != ActionRunSkill {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	if plan.Steps[0].SkillName != "purchase_recommendation" {
		t.Fatalf("skill = %q", plan.Steps[0].SkillName)
	}
	if got := plan.Steps[0].Params["target_intent"]; got != string(intent.PurchaseRecommendation) {
		t.Fatalf("target_intent = %#v", got)
	}
}

func TestCompoundAfterSalesSkillCarriesTargetIntent(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanRequest{
		Query: "T20充不上电，订单CC20260603001还在保修吗",
		Intent: intent.Result{
			Secondary:         intent.Troubleshooting,
			SecondaryIntents:  []intent.Type{intent.WarrantyQuery, intent.OrderQuery},
			NeedDecomposition: true,
			Entities:          map[string]string{"order_no": "CC20260603001"},
		},
		MaxSteps:    5,
		TokenBudget: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var warrantyStep *PlanStep
	for index := range plan.Steps[0].SubSteps {
		step := &plan.Steps[0].SubSteps[index]
		if step.SkillName == "after_sales_judgement" {
			warrantyStep = step
			break
		}
	}
	if warrantyStep == nil {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	if got := warrantyStep.Params["target_intent"]; got != string(intent.WarrantyQuery) {
		t.Fatalf("target_intent = %#v", got)
	}
	for _, step := range plan.Steps[0].SubSteps {
		if step.Action == ActionCallTool && step.ToolName == "order_lookup" {
			t.Fatalf("order lookup should be covered by after_sales_judgement: %#v", plan.Steps)
		}
	}
}

func TestCompoundComparisonDoesNotDuplicateParameterRetrieval(t *testing.T) {
	planner := NewRulePlanner()
	plan, err := planner.Plan(context.Background(), PlanRequest{
		Query: "T20 和 X20 Pro 参数对比",
		Intent: intent.Result{
			Secondary:         intent.ProductComparison,
			SecondaryIntents:  []intent.Type{intent.ProductParameter},
			NeedDecomposition: true,
			Entities:          map[string]string{"models": "T20,X20 Pro"},
		},
		MaxSteps: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) == 0 || plan.Steps[0].Action != ActionRunSkill {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	if plan.Steps[0].SkillName != "product_comparison" {
		t.Fatalf("skill = %q", plan.Steps[0].SkillName)
	}
}
