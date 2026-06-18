package agent

import (
	"context"
	"fmt"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/platform/id"
)

type RulePlanner struct{}

func NewRulePlanner() *RulePlanner {
	return &RulePlanner{}
}

func (p *RulePlanner) Plan(ctx context.Context, request PlanRequest) (*Plan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	maxSteps := request.MaxSteps
	if maxSteps <= 0 || maxSteps > 5 {
		maxSteps = 5
	}
	plan := &Plan{
		ID:          id.New("plan"),
		Intent:      request.Intent.Secondary,
		MaxSteps:    maxSteps,
		TokenBudget: request.TokenBudget,
		Confidence:  request.Intent.Confidence,
	}
	if request.Intent.NeedClarify || request.Intent.Secondary == intent.Clarification {
		plan.Mode = "clarify"
		plan.Steps = []PlanStep{newPlanStep(1, ActionClarify, "", "", request.Query, nil, "low_confidence_or_missing_entity")}
		return plan, nil
	}
	if request.Intent.Secondary == intent.Chitchat || request.Intent.Secondary == intent.OutOfScope {
		plan.Mode = "direct"
		plan.Steps = []PlanStep{newPlanStep(1, ActionAnswerDirect, "", "", request.Query, nil, string(request.Intent.Secondary))}
		return plan, nil
	}
	if request.Intent.NeedDecomposition && len(request.Intent.SecondaryIntents) > 0 {
		plan.Mode = "compound"
		plan.Steps = compoundPlan(request)
		return plan, nil
	}

	switch request.Intent.Secondary {
	case intent.Chitchat, intent.OutOfScope:
		plan.Mode = "direct"
		plan.Steps = []PlanStep{newPlanStep(1, ActionAnswerDirect, "", "", request.Query, nil, string(request.Intent.Secondary))}
	case intent.ProductParameter:
		plan.Mode = "naive_rag"
		plan.Steps = focusedRetrievalPlan(
			request.RewrittenQueries,
			[]string{"product_parameter", "product_detail"},
			3,
		)
	case intent.UsageInstruction:
		plan.Mode = "naive_rag"
		plan.Steps = focusedRetrievalPlan(
			request.RewrittenQueries,
			[]string{"user_manual", "faq", "product_parameter"},
			4,
		)
	case intent.ProductComparison:
		plan.Mode = "react"
		plan.Steps = skillPlan("product_comparison", request.Query, request.Intent.Entities)
	case intent.PurchaseRecommendation:
		plan.Mode = "react"
		plan.Steps = skillPlan("purchase_recommendation", request.Query, request.Intent.Entities)
	case intent.AccessoryCompatibility:
		plan.Mode = "react"
		plan.Steps = skillPlan("accessory_compatibility", request.Query, request.Intent.Entities)
	case intent.Troubleshooting:
		plan.Mode = "react"
		plan.Steps = skillPlan("fault_diagnosis", request.Query, request.Intent.Entities)
	case intent.ReturnEligibility, intent.WarrantyQuery:
		plan.Mode = "react"
		plan.Steps = skillPlan("after_sales_judgement", request.Query, request.Intent.Entities)
	case intent.PriceQuery:
		plan.Mode = "react"
		plan.Steps = dynamicPlan("price_query", request.Query, request.Intent.Entities)
	case intent.InventoryQuery:
		plan.Mode = "react"
		plan.Steps = dynamicPlan("inventory_check", request.Query, request.Intent.Entities)
	case intent.OrderQuery:
		plan.Mode = "react"
		toolName := "order_lookup"
		if request.Intent.Entities["order_no"] == "" {
			toolName = "user_purchase_history"
		}
		plan.Steps = dynamicPlan(toolName, request.Query, request.Intent.Entities)
	case intent.CreateAfterSalesTicket:
		plan.Mode = "react"
		plan.Steps = afterSalesTicketPlan(request.Query, request.Intent.Entities)
	default:
		plan.Mode = "clarify"
		plan.Steps = []PlanStep{newPlanStep(1, ActionClarify, "", "", request.Query, nil, "unsupported_intent")}
	}
	if len(plan.Steps) > maxSteps {
		plan.Steps = plan.Steps[:maxSteps]
	}
	return plan, nil
}

func (p *RulePlanner) CompletePlan(ctx context.Context, request PlanRequest) (*Plan, error) {
	plan, err := p.Plan(ctx, request)
	if err != nil {
		return nil, err
	}
	if plan.Mode == "react" {
		plan.Mode = "plan_execute"
	}
	addSequentialDependencies(plan.Steps)
	return plan, nil
}

func (p *RulePlanner) RevisePlan(
	ctx context.Context,
	request PlanRequest,
	current *Plan,
	completed []PlanStep,
	evidences []Evidence,
	cause error,
) (*Plan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	remaining := max(1, request.MaxSteps-len(completed))
	plan := &Plan{
		ID:          id.New("plan"),
		Mode:        "plan_execute",
		Intent:      request.Intent.Secondary,
		MaxSteps:    request.MaxSteps,
		TokenBudget: request.TokenBudget,
		Confidence:  request.Intent.Confidence,
	}
	if len(evidences) > 0 {
		plan.Steps = []PlanStep{
			newPlanStep(1, ActionReflect, "", "", "", nil, "revision_use_available_evidence"),
			finishStep(2, "revision_grounded_finish"),
		}
	} else {
		plan.Steps = []PlanStep{
			newPlanStep(1, ActionClarify, "", "", request.Query, nil, "revision_missing_evidence"),
		}
	}
	if len(plan.Steps) > remaining {
		plan.Steps = plan.Steps[:remaining]
	}
	addSequentialDependencies(plan.Steps)
	return plan, nil
}

func focusedRetrievalPlan(
	queries []string,
	docTypes []string,
	maxResults int,
) []PlanStep {
	if len(queries) == 0 {
		queries = []string{""}
	}
	query := queries[0]
	steps := []PlanStep{
		newPlanStep(1, ActionRetrieve, "", "", query, map[string]any{
			"search_queries": []string{query},
			"doc_types":      docTypes,
			"max_results":    maxResults,
		}, "collect_focused_static_evidence"),
		newPlanStep(2, ActionReflect, "", "", "", nil, "check_evidence_coverage"),
		newPlanStep(3, ActionFinish, "", "", "", nil, "grounded_answer"),
	}
	addSequentialDependencies(steps)
	return steps
}

func dynamicPlan(toolName, query string, entities map[string]string) []PlanStep {
	steps := []PlanStep{
		newPlanStep(1, ActionCallTool, toolName, "", query, stringMapToAny(entities), "dynamic_data_required"),
		newPlanStep(2, ActionReflect, "", "", "", nil, "check_tool_evidence"),
		newPlanStep(3, ActionFinish, "", "", "", nil, "grounded_answer"),
	}
	addSequentialDependencies(steps)
	return steps
}

func afterSalesTicketPlan(query string, entities map[string]string) []PlanStep {
	ticketSteps := afterSalesTicketParallelSteps(query, entities)
	steps := []PlanStep{
		ticketSteps[0],
		ticketSteps[1],
		newPlanStep(3, ActionFinish, "", "", "", nil, "grounded_answer"),
	}
	addSequentialDependencies(steps)
	return steps
}

func afterSalesTicketParallelSteps(query string, entities map[string]string) []PlanStep {
	return []PlanStep{
		newPlanStep(1, ActionRetrieve, "", "", "售后工单创建规则 用户明确确认 订单 问题描述 幂等键", map[string]any{
			"search_queries":          []string{"售后工单创建规则 用户明确确认 订单 问题描述 幂等键"},
			"doc_types":               []string{"after_sales_policy"},
			"max_results":             4,
			"search_mode":             "keyword",
			"disable_entity_filters":  true,
			"disable_original_recall": true,
		}, "collect_after_sales_ticket_policy"),
		newPlanStep(2, ActionCallTool, "create_after_sales_ticket", "", query, stringMapToAny(entities), "confirmed_side_effect"),
	}
}

func skillPlan(skillName, query string, entities map[string]string) []PlanStep {
	steps := []PlanStep{
		newPlanStep(1, ActionRunSkill, "", skillName, query, stringMapToAny(entities), "complex_workflow_required"),
		newPlanStep(2, ActionReflect, "", "", "", nil, "check_skill_evidence"),
		newPlanStep(3, ActionFinish, "", "", "", nil, "grounded_answer"),
	}
	addSequentialDependencies(steps)
	return steps
}

func compoundPlan(request PlanRequest) []PlanStep {
	intents := append([]intent.Type{request.Intent.Secondary}, request.Intent.SecondaryIntents...)
	subSteps := make([]PlanStep, 0, len(intents)+1)
	seen := map[string]struct{}{}
	for _, intentType := range intents {
		if request.Intent.Secondary == intent.PurchaseRecommendation &&
			(intentType == intent.PriceQuery || intentType == intent.InventoryQuery) {
			continue
		}
		if (request.Intent.Secondary == intent.ProductComparison ||
			request.Intent.Secondary == intent.PurchaseRecommendation) &&
			intentType == intent.ProductParameter {
			continue
		}
		if request.Intent.Secondary == intent.ReturnEligibility && intentType == intent.OrderQuery {
			continue
		}
		if intentType == intent.OrderQuery && containsIntent(intents, intent.WarrantyQuery) {
			continue
		}
		if request.Intent.Secondary == intent.CreateAfterSalesTicket &&
			intentType == intent.Troubleshooting {
			continue
		}
		if intentType == intent.CreateAfterSalesTicket {
			for _, step := range afterSalesTicketParallelSteps(request.Query, request.Intent.Entities) {
				signature := actionSignature(step)
				if _, exists := seen[signature]; exists {
					continue
				}
				seen[signature] = struct{}{}
				subSteps = append(subSteps, step)
			}
			continue
		}
		step, ok := compoundSubStep(intentType, request)
		if !ok {
			continue
		}
		signature := actionSignature(step)
		if _, exists := seen[signature]; exists {
			continue
		}
		seen[signature] = struct{}{}
		subSteps = append(subSteps, step)
	}
	if len(subSteps) == 0 {
		return []PlanStep{
			newPlanStep(1, ActionClarify, "", "", request.Query, nil, "compound_plan_has_no_supported_action"),
		}
	}
	if len(subSteps) == 1 {
		subSteps[0].StepID = "step_01"
		return []PlanStep{subSteps[0], finishStep(2, "compound_answer")}
	}
	for index := range subSteps {
		subSteps[index].StepID = fmt.Sprintf("step_01.%02d", index+1)
	}
	steps := []PlanStep{
		newPlanStep(1, ActionParallel, "", "", request.Query, nil, "compound_intents_parallel"),
		finishStep(2, "compound_answer"),
	}
	steps[0].SubSteps = subSteps
	addSequentialDependencies(steps)
	return steps
}

func compoundSubStep(intentType intent.Type, request PlanRequest) (PlanStep, bool) {
	params := stringMapToAny(request.Intent.Entities)
	switch intentType {
	case intent.ProductParameter:
		params["doc_types"] = []string{"product_parameter", "product_detail"}
		return newPlanStep(1, ActionRetrieve, "", "", request.Query, params, "compound_product_parameter"), true
	case intent.UsageInstruction:
		params["doc_types"] = []string{"user_manual", "faq", "product_parameter"}
		return newPlanStep(1, ActionRetrieve, "", "", request.Query, params, "compound_usage_instruction"), true
	case intent.ProductComparison:
		params["target_intent"] = string(intentType)
		return newPlanStep(1, ActionRunSkill, "", "product_comparison", request.Query, params, "compound_product_comparison"), true
	case intent.PurchaseRecommendation:
		params["target_intent"] = string(intentType)
		return newPlanStep(1, ActionRunSkill, "", "purchase_recommendation", request.Query, params, "compound_purchase_recommendation"), true
	case intent.AccessoryCompatibility:
		params["target_intent"] = string(intentType)
		return newPlanStep(1, ActionRunSkill, "", "accessory_compatibility", request.Query, params, "compound_accessory_compatibility"), true
	case intent.Troubleshooting:
		params["target_intent"] = string(intentType)
		return newPlanStep(1, ActionRunSkill, "", "fault_diagnosis", request.Query, params, "compound_fault_diagnosis"), true
	case intent.WarrantyQuery, intent.ReturnEligibility:
		params["target_intent"] = string(intentType)
		return newPlanStep(1, ActionRunSkill, "", "after_sales_judgement", request.Query, params, "compound_after_sales"), true
	case intent.PriceQuery:
		return newPlanStep(1, ActionCallTool, "price_query", "", request.Query, params, "compound_price"), true
	case intent.InventoryQuery:
		return newPlanStep(1, ActionCallTool, "inventory_check", "", request.Query, params, "compound_inventory"), true
	case intent.OrderQuery:
		toolName := "order_lookup"
		if request.Intent.Entities["order_no"] == "" {
			toolName = "user_purchase_history"
		}
		return newPlanStep(1, ActionCallTool, toolName, "", request.Query, params, "compound_order"), true
	case intent.CreateAfterSalesTicket:
		return newPlanStep(1, ActionCallTool, "create_after_sales_ticket", "", request.Query, params, "compound_after_sales_ticket"), true
	default:
		return PlanStep{}, false
	}
}

func addSequentialDependencies(steps []PlanStep) {
	for index := 1; index < len(steps); index++ {
		if len(steps[index].DependsOn) == 0 {
			steps[index].DependsOn = []string{steps[index-1].StepID}
		}
	}
}

func newPlanStep(
	index int,
	action ActionType,
	toolName string,
	skillName string,
	query string,
	params map[string]any,
	reason string,
) PlanStep {
	if params == nil {
		params = map[string]any{}
	}
	return PlanStep{
		StepID:     fmt.Sprintf("step_%02d", index),
		Action:     action,
		SkillName:  skillName,
		ToolName:   toolName,
		Query:      query,
		ReasonCode: reason,
		Params:     params,
	}
}

func stringMapToAny(source map[string]string) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

var _ PlanAndExecutePlanner = (*RulePlanner)(nil)
