package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/tool"
)

var plannedProductRefPattern = regexp.MustCompile(`(?i)^[A-Z]{1,8}[0-9]{1,5}(?:PRO)?(?:-[A-Z0-9]{1,12})?$`)

// llmPlanAction is the JSON structure the LLM returns for a single ReAct step.
type llmPlanAction struct {
	Thought         string          `json:"thought"`
	Action          string          `json:"action"`
	ActionDetail    llmActionDetail `json:"action_detail"`
	EvidenceSummary string          `json:"evidence_summary"`
	RemainingSteps  int             `json:"remaining_steps"`
}

type llmActionDetail struct {
	ToolName        string                `json:"tool_name"`
	ToolArgs        map[string]any        `json:"tool_args"`
	SkillName       string                `json:"skill_name"`
	SearchQueries   []string              `json:"search_queries"`
	DocTypes        []string              `json:"doc_types"`
	ClarifyQuestion string                `json:"clarify_question"`
	Reason          string                `json:"reason"`
	SubActions      []llmCompletePlanStep `json:"sub_actions"`
}

type llmCompletePlan struct {
	Confidence float64               `json:"confidence"`
	Steps      []llmCompletePlanStep `json:"steps"`
}

type llmCompletePlanStep struct {
	Action        string                `json:"action"`
	ToolName      string                `json:"tool_name"`
	SkillName     string                `json:"skill_name"`
	Query         string                `json:"query"`
	SearchQueries []string              `json:"search_queries"`
	DocTypes      []string              `json:"doc_types"`
	Params        map[string]any        `json:"params"`
	Reason        string                `json:"reason"`
	SubActions    []llmCompletePlanStep `json:"sub_actions"`
}

// LLMPlanner uses an LLM to dynamically decide the next action in a ReAct loop.
// It wraps a RulePlanner for the initial plan structure; the LLM decides each
// subsequent step based on accumulated evidence.
type LLMPlanner struct {
	llm             *llm.Client
	prompts         *prompt.Registry
	rule            *RulePlanner
	toolDefinitions map[string]tool.Definition
}

// NewLLMPlanner creates an LLM-backed planner. If llmClient is nil, degrades
// to the pure rule-based planner.
func NewLLMPlanner(
	llmClient *llm.Client,
	prompts *prompt.Registry,
	definitions ...tool.Definition,
) *LLMPlanner {
	byName := make(map[string]tool.Definition, len(definitions))
	for _, definition := range definitions {
		byName[definition.Name] = definition
	}
	return &LLMPlanner{
		llm:             llmClient,
		prompts:         prompts,
		rule:            NewRulePlanner(),
		toolDefinitions: byName,
	}
}

// Plan creates an initial plan. For simple intents (parameter query, usage, chitchat)
// the rule planner is used directly. For complex intents, a hybrid approach is used:
// the rule planner creates a skeleton, and the LLM can override step details.
func (p *LLMPlanner) Plan(ctx context.Context, request PlanRequest) (*Plan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	maxSteps := request.MaxSteps
	if maxSteps <= 0 || maxSteps > 5 {
		maxSteps = 5
	}

	// Use rule planner for simple / deterministic intents.
	switch request.Intent.Secondary {
	case intent.Chitchat, intent.OutOfScope, intent.Clarification:
		return p.rule.Plan(ctx, request)
	case intent.ProductParameter, intent.UsageInstruction:
		return p.rule.Plan(ctx, request)
	}

	// For complex intents, start with rule-based skeleton then let LLM refine.
	plan, err := p.rule.Plan(ctx, request)
	if err != nil {
		return nil, err
	}

	// If no LLM available, return rule-based plan.
	if p.llm == nil || p.prompts == nil {
		return plan, nil
	}

	// ReAct plans are executed one action at a time through NextStep. The rule
	// skeleton remains available to the runner as a deterministic fallback.
	return plan, nil
}

func (p *LLMPlanner) CompletePlan(ctx context.Context, request PlanRequest) (*Plan, error) {
	fallback, err := p.rule.CompletePlan(ctx, request)
	if err != nil {
		return nil, err
	}
	if p.llm == nil || p.prompts == nil || fallback.Mode != "plan_execute" {
		return fallback, nil
	}
	if preferRuleCompletePlan(request.Intent.Secondary) {
		return fallback, nil
	}
	plan, err := p.generateCompletePlan(ctx, request, "", "")
	if err != nil || len(plan.Steps) == 0 {
		return fallback, nil
	}
	evaluation := evaluatePlan(request, plan)
	if evaluation.Score < 4 {
		return fallback, nil
	}
	return plan, nil
}

func preferRuleCompletePlan(intentType intent.Type) bool {
	switch intentType {
	case intent.ProductComparison,
		intent.PurchaseRecommendation,
		intent.AccessoryCompatibility,
		intent.Troubleshooting,
		intent.WarrantyQuery,
		intent.ReturnEligibility,
		intent.AfterSalesStatus,
		intent.HumanHandoff,
		intent.CreateAfterSalesTicket:
		return true
	default:
		return false
	}
}

func (p *LLMPlanner) RevisePlan(
	ctx context.Context,
	request PlanRequest,
	current *Plan,
	completed []PlanStep,
	evidences []Evidence,
	cause error,
) (*Plan, error) {
	if p.llm == nil || p.prompts == nil {
		return p.rule.RevisePlan(ctx, request, current, completed, evidences, cause)
	}
	evidenceRaw, _ := json.Marshal(evidences)
	causeText := ""
	if cause != nil {
		causeText = cause.Error()
	}
	plan, err := p.generateCompletePlan(ctx, request, string(evidenceRaw), causeText)
	if err != nil || len(plan.Steps) == 0 {
		return p.rule.RevisePlan(ctx, request, current, completed, evidences, cause)
	}
	if evaluation := evaluatePlan(request, plan); evaluation.Score < 4 {
		return p.rule.RevisePlan(ctx, request, current, completed, evidences, cause)
	}
	return plan, nil
}

func (p *LLMPlanner) generateCompletePlan(
	ctx context.Context,
	request PlanRequest,
	evidenceSummary string,
	cause string,
) (*Plan, error) {
	tmpl, err := p.prompts.Get(prompt.ScenarioPlan)
	if err != nil {
		return nil, err
	}
	if evidenceSummary == "" {
		evidenceSummary = "(尚未执行步骤)"
	}
	contextRaw, _ := json.Marshal(map[string]any{
		"query":             request.Query,
		"intent":            request.Intent,
		"rewritten_queries": request.RewrittenQueries,
		"allowed_tools":     request.AllowedTools,
		"max_steps":         min(request.MaxSteps, 5),
		"observations":      evidenceSummary,
		"revision_cause":    cause,
	})
	messages := []map[string]string{
		{
			"role": "system",
			"content": tmpl.System + `

# Plan-and-Execute 补充规则
你现在必须先输出完整计划，而不是只输出下一步。步骤总数不超过 5。
已知复杂业务 Skill 应优先作为受控步骤；工具必须来自 allowed_tools。
输出 JSON：
{"confidence":0.0,"steps":[{"action":"retrieve|call_tool|run_skill|parallel|clarify|reflect|finish","tool_name":"","skill_name":"","query":"","search_queries":[],"doc_types":[],"params":{},"sub_actions":[],"reason":""}]}`,
		},
		{
			"role":    "user",
			"content": "请根据以下上下文生成完整可执行计划：\n" + string(contextRaw),
		},
	}
	var output llmCompletePlan
	if err := p.llm.ChatJSON(ctx, messages, &output); err != nil {
		return nil, fmt.Errorf("generate complete plan: %w", err)
	}
	steps, err := p.validateCompleteSteps(request, output.Steps)
	if err != nil {
		return nil, err
	}
	return &Plan{
		ID:          "llm_complete_" + request.TraceID,
		Mode:        "plan_execute",
		Intent:      request.Intent.Secondary,
		Steps:       steps,
		MaxSteps:    min(max(1, request.MaxSteps), 5),
		TokenBudget: request.TokenBudget,
		Confidence:  output.Confidence,
	}, nil
}

func (p *LLMPlanner) validateCompleteSteps(
	request PlanRequest,
	values []llmCompletePlanStep,
) ([]PlanStep, error) {
	maxSteps := min(max(1, request.MaxSteps), 5)
	if len(values) == 0 {
		return nil, fmt.Errorf("complete plan returned no steps")
	}
	if len(values) > maxSteps {
		values = values[:maxSteps]
	}
	steps := make([]PlanStep, 0, len(values)+1)
	for _, value := range values {
		action := normalizeAction(value.Action)
		if action == "" {
			return nil, fmt.Errorf("complete plan returned unsupported action %q", value.Action)
		}
		step := PlanStep{
			StepID:     fmt.Sprintf("step_%02d", len(steps)+1),
			Action:     action,
			Query:      strings.TrimSpace(value.Query),
			ToolName:   strings.TrimSpace(value.ToolName),
			SkillName:  strings.TrimSpace(value.SkillName),
			Params:     cloneAnyMap(value.Params),
			ReasonCode: strings.TrimSpace(value.Reason),
		}
		switch action {
		case ActionRetrieve:
			queries := compactStrings(value.SearchQueries)
			if len(queries) == 0 && step.Query != "" {
				queries = []string{step.Query}
			}
			step.Params["search_queries"] = queries
			step.Params["doc_types"] = allowedDocTypes(value.DocTypes)
		case ActionCallTool:
			if !tool.NameAllowed(request.AllowedTools, step.ToolName) {
				return nil, fmt.Errorf("complete plan selected non-whitelisted tool %q", step.ToolName)
			}
			if !toolRequestedForPlan(request, step.ToolName) {
				return nil, fmt.Errorf("complete plan selected unrequested tool %q", step.ToolName)
			}
			if err := validateKnownProductRefs(step.Params); err != nil {
				return nil, err
			}
		case ActionRunSkill:
			if !skillAllowedForIntent(request.Intent.Secondary, step.SkillName) {
				return nil, fmt.Errorf(
					"complete plan selected skill %q for incompatible intent %q",
					step.SkillName,
					request.Intent.Secondary,
				)
			}
		case ActionParallel:
			subSteps, err := p.convertParallelActions(request, value.SubActions)
			if err != nil {
				return nil, err
			}
			step.SubSteps = subSteps
		}
		steps = append(steps, step)
		if action == ActionFinish || action == ActionClarify {
			break
		}
	}
	if len(steps) < maxSteps &&
		steps[len(steps)-1].Action != ActionFinish &&
		steps[len(steps)-1].Action != ActionClarify {
		steps = append(steps, finishStep(len(steps)+1, "complete_plan_finish"))
	}
	if steps[len(steps)-1].Action != ActionFinish &&
		steps[len(steps)-1].Action != ActionClarify {
		return nil, errors.New("complete plan has no terminal step")
	}
	addSequentialDependencies(steps)
	return steps, nil
}

func toolRequestedForPlan(request PlanRequest, toolName string) bool {
	switch tool.LogicalName(toolName) {
	case "price_query":
		return request.Intent.Secondary == intent.PriceQuery ||
			containsIntent(request.Intent.SecondaryIntents, intent.PriceQuery) ||
			containsAnyFold(request.Query, "价格", "多少钱", "售价", "到手价", "查价", "报价")
	case "inventory_check":
		return request.Intent.Secondary == intent.InventoryQuery ||
			containsIntent(request.Intent.SecondaryIntents, intent.InventoryQuery) ||
			containsAnyFold(request.Query, "库存", "有货", "现货", "没货", "几台")
	default:
		return true
	}
}

func validateKnownProductRefs(params map[string]any) error {
	raw, exists := params["product_refs"]
	if !exists {
		return nil
	}
	refs := stringSliceValue(raw)
	known := map[string]struct{}{
		"T20": {}, "X20 PRO": {}, "R10": {}, "R20": {}, "P400": {},
		"P500": {}, "W300": {}, "W500": {}, "H100": {}, "H200": {},
	}
	for _, ref := range refs {
		normalized := strings.ToUpper(strings.Join(strings.Fields(ref), " "))
		compact := strings.ToUpper(strings.Join(strings.Fields(ref), ""))
		if _, ok := known[normalized]; !ok && !plannedProductRefPattern.MatchString(compact) {
			return fmt.Errorf("complete plan selected unknown product ref %q", ref)
		}
	}
	return nil
}

func containsIntent(values []intent.Type, expected intent.Type) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func containsAnyFold(value string, candidates ...string) bool {
	value = strings.ToLower(value)
	for _, candidate := range candidates {
		if strings.Contains(value, strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}

func stringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func cloneAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// NextStep asks the LLM to decide the next action in a ReAct loop based on
// the current evidence state. Returns nil if the LLM decides to finish.
func (p *LLMPlanner) NextStep(
	ctx context.Context,
	request PlanRequest,
	currentStep int,
	evidences []Evidence,
	searchResults string,
	toolResults string,
) (*PlanStep, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Known multi-step business flows enter through a guarded Skill. This keeps
	// safety and business invariants deterministic while the LLM remains free to
	// choose tools and retrieval steps for non-guarded intents.
	if currentStep == 0 {
		if skillName := guardedSkillForIntent(request.Intent.Secondary); skillName != "" {
			return &PlanStep{
				StepID:     "step_01",
				Action:     ActionRunSkill,
				SkillName:  skillName,
				Query:      request.Query,
				Params:     stringMapToAny(request.Intent.Entities),
				ReasonCode: "guarded_skill_entry",
			}, nil
		}
		if toolName := guardedToolForIntent(request.Intent); toolName != "" {
			return &PlanStep{
				StepID:     "step_01",
				Action:     ActionCallTool,
				ToolName:   toolName,
				Query:      request.Query,
				Params:     stringMapToAny(request.Intent.Entities),
				ReasonCode: "guarded_tool_entry",
			}, nil
		}
	}
	if p.llm == nil || p.prompts == nil {
		return nil, nil
	}

	tmpl, err := p.prompts.Get(prompt.ScenarioPlan)
	if err != nil {
		return nil, err
	}

	subQJSON, _ := json.Marshal(request.RewrittenQueries)
	intentJSON, _ := json.Marshal(map[string]any{
		"primary":    request.Intent.Primary,
		"secondary":  string(request.Intent.Secondary),
		"confidence": request.Intent.Confidence,
	})

	evidenceSummary := searchResults
	if toolResults != "" {
		evidenceSummary += "\n工具结果：\n" + toolResults
	}
	if evidenceSummary == "" {
		evidenceSummary = "(尚未收集任何证据)"
	}

	params := map[string]string{
		"query":            request.Query,
		"intent_info":      string(intentJSON),
		"sub_questions":    string(subQJSON),
		"evidence_summary": evidenceSummary,
		"step_info":        fmt.Sprintf("%d/%d", currentStep, request.MaxSteps),
		"max_steps":        fmt.Sprintf("%d", request.MaxSteps),
		"tool_definitions": p.allowedToolDefinitions(request.AllowedTools),
	}
	messages := tmpl.BuildMessages(params)

	var llmOut llmPlanAction
	var decoded bool
	if definitions := p.allowedLLMToolDefinitions(request.AllowedTools); len(definitions) > 0 {
		content, calls, toolErr := p.llm.ChatWithTools(ctx, messages, definitions)
		if toolErr == nil && len(calls) > 0 {
			call := calls[0]
			step := PlanStep{
				StepID:     fmt.Sprintf("step_%02d", currentStep+1),
				Action:     ActionCallTool,
				ToolName:   call.Name,
				Params:     call.Arguments,
				ReasonCode: "llm_function_call",
			}
			if err := p.ValidateNextStep(ctx, request, step, nil); err != nil {
				return nil, err
			}
			return &step, nil
		}
		if toolErr == nil && content != "" && llm.DecodeJSON(content, &llmOut) == nil {
			decoded = true
		}
	}
	if !decoded {
		if err := p.llm.ChatJSON(ctx, messages, &llmOut); err != nil {
			return nil, fmt.Errorf("llm plan next step: %w", err)
		}
	}

	action := normalizeAction(llmOut.Action)
	if action == "" || action == ActionFinish {
		return nil, nil // Signal to finish.
	}

	step := &PlanStep{
		StepID:     fmt.Sprintf("step_%02d", currentStep+1),
		Action:     action,
		ReasonCode: llmOut.ActionDetail.Reason,
	}

	switch action {
	case ActionRetrieve:
		queries := compactStrings(llmOut.ActionDetail.SearchQueries)
		step.Query = strings.Join(queries, "\n")
		if step.Query == "" {
			step.Query = request.Query
		}
		step.Params = map[string]any{
			"search_queries": queries,
			"doc_types":      allowedDocTypes(llmOut.ActionDetail.DocTypes),
			"thought":        llmOut.Thought,
		}
	case ActionCallTool:
		if !tool.NameAllowed(request.AllowedTools, llmOut.ActionDetail.ToolName) {
			return nil, fmt.Errorf("llm selected non-whitelisted tool %q", llmOut.ActionDetail.ToolName)
		}
		step.ToolName = llmOut.ActionDetail.ToolName
		step.Params = llmOut.ActionDetail.ToolArgs
		if step.Params == nil {
			step.Params = map[string]any{}
		}
		step.Params["thought"] = llmOut.Thought
	case ActionRunSkill:
		if !skillAllowedForIntent(request.Intent.Secondary, llmOut.ActionDetail.SkillName) {
			return nil, fmt.Errorf(
				"llm selected skill %q for incompatible intent %q",
				llmOut.ActionDetail.SkillName,
				request.Intent.Secondary,
			)
		}
		step.SkillName = llmOut.ActionDetail.SkillName
		step.Params = stringMapToAny(request.Intent.Entities)
		step.Params["thought"] = llmOut.Thought
	case ActionClarify:
		step.Query = llmOut.ActionDetail.ClarifyQuestion
		step.Params = map[string]any{"thought": llmOut.Thought}
	case ActionReflect:
		step.Params = map[string]any{"thought": llmOut.Thought}
	case ActionParallel:
		subSteps, err := p.convertParallelActions(request, llmOut.ActionDetail.SubActions)
		if err != nil {
			return nil, err
		}
		step.SubSteps = subSteps
		step.Params = map[string]any{"thought": llmOut.Thought}
	}

	if err := p.ValidateNextStep(ctx, request, *step, nil); err != nil {
		return nil, err
	}
	return step, nil
}

func (p *LLMPlanner) ValidateNextStep(
	ctx context.Context,
	request PlanRequest,
	candidate PlanStep,
	recent []PlanStep,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch candidate.Action {
	case ActionRetrieve, ActionReflect, ActionClarify, ActionFinish:
	case ActionCallTool:
		if !tool.NameAllowed(request.AllowedTools, candidate.ToolName) {
			return fmt.Errorf("下一步选择了非白名单工具 %q", candidate.ToolName)
		}
		if definition, ok := p.toolDefinition(candidate.ToolName); ok {
			if err := tool.ValidateArguments(definition.ParamsSchema, candidate.Params); err != nil {
				return fmt.Errorf("下一步工具参数不符合 Schema: %w", err)
			}
		}
	case ActionRunSkill:
		if !skillAllowedForIntent(request.Intent.Secondary, candidate.SkillName) {
			return fmt.Errorf("下一步 Skill %q 与意图 %q 不兼容", candidate.SkillName, request.Intent.Secondary)
		}
	case ActionParallel:
		if len(candidate.SubSteps) < 2 {
			return errors.New("并行动作至少需要两个子步骤")
		}
		for _, subStep := range candidate.SubSteps {
			if subStep.Action == ActionParallel || subStep.Action == ActionFinish ||
				subStep.Action == ActionClarify || subStep.Action == ActionAnswerDirect {
				return fmt.Errorf("并行动作包含不支持的子动作 %q", subStep.Action)
			}
			if err := p.ValidateNextStep(ctx, request, subStep, nil); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("不支持的下一步动作 %q", candidate.Action)
	}
	signature := actionSignature(candidate)
	start := max(0, len(recent)-3)
	for _, previous := range recent[start:] {
		if actionSignature(previous) == signature {
			return ErrRepeatedAction
		}
	}
	return nil
}

func guardedSkillForIntent(intentType intent.Type) string {
	switch intentType {
	case intent.ProductComparison:
		return "product_comparison"
	case intent.PurchaseRecommendation:
		return "purchase_recommendation"
	case intent.AccessoryCompatibility:
		return "accessory_compatibility"
	case intent.Troubleshooting:
		return "fault_diagnosis"
	case intent.ReturnEligibility, intent.WarrantyQuery:
		return "after_sales_judgement"
	default:
		return ""
	}
}

func guardedToolForIntent(result intent.Result) string {
	switch result.Secondary {
	case intent.PriceQuery:
		return "price_query"
	case intent.InventoryQuery:
		return "inventory_check"
	case intent.OrderQuery:
		if result.Entities["order_no"] == "" {
			return "user_purchase_history"
		}
		return "order_lookup"
	case intent.AfterSalesStatus:
		if result.Entities["after_sales_status_type"] == "refund" {
			return "refund_status"
		}
		return "repair_status"
	case intent.HumanHandoff:
		return "handoff_to_human"
	default:
		return ""
	}
}

func (p *LLMPlanner) enhancePlan(ctx context.Context, request PlanRequest, plan *Plan) (*Plan, error) {
	// Use the LLM to generate better search queries for the first retrieve step.
	tmpl, err := p.prompts.Get(prompt.ScenarioPlan)
	if err != nil {
		return plan, nil
	}

	subQJSON, _ := json.Marshal(request.RewrittenQueries)
	intentJSON, _ := json.Marshal(map[string]any{
		"secondary": string(request.Intent.Secondary),
	})

	params := map[string]string{
		"query":            request.Query,
		"intent_info":      string(intentJSON),
		"sub_questions":    string(subQJSON),
		"evidence_summary": "(初始步骤，尚未收集证据)",
		"step_info":        fmt.Sprintf("0/%d", request.MaxSteps),
		"max_steps":        fmt.Sprintf("%d", request.MaxSteps),
		"tool_definitions": p.allowedToolDefinitions(request.AllowedTools),
	}
	messages := tmpl.BuildMessages(params)

	var llmOut llmPlanAction
	if err := p.llm.ChatJSON(ctx, messages, &llmOut); err != nil {
		return plan, nil
	}

	// If the LLM suggests better search queries, update the first retrieve step.
	if llmOut.ActionDetail.Reason != "" {
		for i := range plan.Steps {
			if plan.Steps[i].Action == ActionRetrieve {
				newReason := "llm_enhanced:" + llmOut.ActionDetail.Reason
				plan.Steps[i].ReasonCode = newReason
				if len(llmOut.ActionDetail.SearchQueries) > 0 {
					plan.Steps[i].Query = strings.Join(llmOut.ActionDetail.SearchQueries, "\n")
				}
				plan.Steps[i].Params = map[string]any{
					"search_queries": llmOut.ActionDetail.SearchQueries,
					"doc_types":      llmOut.ActionDetail.DocTypes,
				}
				break
			}
		}
	}

	return plan, nil
}

func normalizeAction(raw string) ActionType {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "retrieve":
		return ActionRetrieve
	case "call_tool":
		return ActionCallTool
	case "run_skill":
		return ActionRunSkill
	case "parallel":
		return ActionParallel
	case "clarify":
		return ActionClarify
	case "reflect":
		return ActionReflect
	case "finish":
		return ActionFinish
	default:
		return ""
	}
}

func (p *LLMPlanner) convertParallelActions(
	request PlanRequest,
	values []llmCompletePlanStep,
) ([]PlanStep, error) {
	if len(values) < 2 || len(values) > 4 {
		return nil, fmt.Errorf("并行动作数量必须在 2 到 4 之间")
	}
	result := make([]PlanStep, 0, len(values))
	for index, value := range values {
		action := normalizeAction(value.Action)
		step := PlanStep{
			StepID: fmt.Sprintf("parallel_%02d", index+1),
			Action: action, ToolName: strings.TrimSpace(value.ToolName),
			SkillName: strings.TrimSpace(value.SkillName), Query: strings.TrimSpace(value.Query),
			Params: cloneAnyMap(value.Params), ReasonCode: strings.TrimSpace(value.Reason),
		}
		if action == ActionRetrieve {
			step.Params["search_queries"] = compactStrings(value.SearchQueries)
			step.Params["doc_types"] = allowedDocTypes(value.DocTypes)
		}
		if err := p.ValidateNextStep(context.Background(), request, step, nil); err != nil {
			return nil, err
		}
		result = append(result, step)
	}
	return result, nil
}

func evaluatePlan(request PlanRequest, plan *Plan) PlanEvaluation {
	evaluation := PlanEvaluation{Score: 5}
	if plan == nil || len(plan.Steps) == 0 {
		return PlanEvaluation{Score: 1, Warnings: []string{"empty_plan"}}
	}
	if plan.Steps[0].Action == ActionFinish &&
		request.Intent.Secondary != intent.Chitchat &&
		request.Intent.Secondary != intent.OutOfScope {
		evaluation.Score -= 2
		evaluation.Warnings = append(evaluation.Warnings, "finish_as_first_action")
	}
	terminalIndex := -1
	seenIDs := make(map[string]struct{}, len(plan.Steps))
	for index, step := range plan.Steps {
		if _, exists := seenIDs[step.StepID]; exists {
			evaluation.Score -= 2
			evaluation.Warnings = append(evaluation.Warnings, "duplicate_step_id")
		}
		seenIDs[step.StepID] = struct{}{}
		if step.Action == ActionFinish || step.Action == ActionClarify {
			if terminalIndex < 0 {
				terminalIndex = index
			}
			continue
		}
		if terminalIndex >= 0 {
			evaluation.Score -= 2
			evaluation.Warnings = append(evaluation.Warnings, "action_after_terminal")
		}
	}
	if terminalIndex < 0 {
		evaluation.Score -= 2
		evaluation.Warnings = append(evaluation.Warnings, "missing_terminal_action")
	}
	if hasDependencyCycle(plan.Steps) {
		evaluation.Score -= 2
		evaluation.Warnings = append(evaluation.Warnings, "dependency_cycle")
	}
	if evaluation.Score < 1 {
		evaluation.Score = 1
	}
	return evaluation
}

func hasDependencyCycle(steps []PlanStep) bool {
	graph := make(map[string][]string, len(steps))
	for _, step := range steps {
		graph[step.StepID] = append([]string(nil), step.DependsOn...)
	}
	state := make(map[string]uint8, len(graph))
	var visit func(string) bool
	visit = func(node string) bool {
		if state[node] == 1 {
			return true
		}
		if state[node] == 2 {
			return false
		}
		state[node] = 1
		for _, dependency := range graph[node] {
			if _, exists := graph[dependency]; exists && visit(dependency) {
				return true
			}
		}
		state[node] = 2
		return false
	}
	for node := range graph {
		if visit(node) {
			return true
		}
	}
	return false
}

func (p *LLMPlanner) allowedToolDefinitions(allowed []string) string {
	definitions := p.definitionsForAllowed(allowed)
	raw, _ := json.Marshal(definitions)
	return string(raw)
}

func (p *LLMPlanner) allowedLLMToolDefinitions(allowed []string) []llm.ToolDefinition {
	allowedDefinitions := p.definitionsForAllowed(allowed)
	definitions := make([]llm.ToolDefinition, 0, len(allowedDefinitions))
	for _, definition := range allowedDefinitions {
		if len(definition.ParamsSchema) == 0 {
			continue
		}
		definitions = append(definitions, llm.ToolDefinition{
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  definition.ParamsSchema,
		})
	}
	return definitions
}

func (p *LLMPlanner) toolDefinition(name string) (tool.Definition, bool) {
	if p == nil {
		return tool.Definition{}, false
	}
	if definition, ok := p.toolDefinitions[name]; ok {
		return definition, true
	}
	for _, definition := range p.toolDefinitions {
		if tool.NamesMatch(name, definition.Name) {
			return definition, true
		}
	}
	return tool.Definition{}, false
}

func (p *LLMPlanner) definitionsForAllowed(allowed []string) []tool.Definition {
	if p == nil {
		return nil
	}
	definitions := make([]tool.Definition, 0, len(p.toolDefinitions)+len(allowed))
	seen := make(map[string]struct{}, len(p.toolDefinitions)+len(allowed))
	for _, allowedName := range allowed {
		matched := false
		if definition, ok := p.toolDefinitions[allowedName]; ok {
			definitions = appendDefinitionOnce(definitions, seen, definition)
			matched = true
		}
		for _, definition := range p.toolDefinitions {
			if tool.NameAllowed([]string{allowedName}, definition.Name) {
				definitions = appendDefinitionOnce(definitions, seen, definition)
				matched = true
			}
		}
		if !matched {
			definitions = appendDefinitionOnce(definitions, seen, tool.Definition{Name: allowedName})
		}
	}
	return definitions
}

func appendDefinitionOnce(
	definitions []tool.Definition,
	seen map[string]struct{},
	definition tool.Definition,
) []tool.Definition {
	if _, ok := seen[definition.Name]; ok {
		return definitions
	}
	seen[definition.Name] = struct{}{}
	return append(definitions, definition)
}

func allowedDocTypes(values []string) []string {
	allowed := map[string]struct{}{
		"product_detail":          {},
		"product_parameter":       {},
		"product_comparison":      {},
		"purchase_guide":          {},
		"accessory_compatibility": {},
		"user_manual":             {},
		"troubleshooting":         {},
		"after_sales_policy":      {},
		"faq":                     {},
	}
	result := make([]string, 0, len(values))
	for _, value := range compactStrings(values) {
		if _, ok := allowed[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

func compactStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func skillAllowedForIntent(intentType intent.Type, skillName string) bool {
	expected := map[intent.Type]string{
		intent.ProductComparison:      "product_comparison",
		intent.PurchaseRecommendation: "purchase_recommendation",
		intent.AccessoryCompatibility: "accessory_compatibility",
		intent.Troubleshooting:        "fault_diagnosis",
		intent.ReturnEligibility:      "after_sales_judgement",
		intent.WarrantyQuery:          "after_sales_judgement",
	}
	return expected[intentType] != "" && expected[intentType] == skillName
}

// Ensure LLMPlanner implements the interface.
var _ Planner = (*LLMPlanner)(nil)
var _ ReactivePlanner = (*LLMPlanner)(nil)
var _ PlanAndExecutePlanner = (*LLMPlanner)(nil)
var _ NextStepValidator = (*LLMPlanner)(nil)
