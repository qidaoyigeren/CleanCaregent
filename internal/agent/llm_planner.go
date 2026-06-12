package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/tool"
)

// llmPlanAction is the JSON structure the LLM returns for a single ReAct step.
type llmPlanAction struct {
	Thought         string          `json:"thought"`
	Action          string          `json:"action"`
	ActionDetail    llmActionDetail `json:"action_detail"`
	EvidenceSummary string          `json:"evidence_summary"`
	RemainingSteps  int             `json:"remaining_steps"`
}

type llmActionDetail struct {
	ToolName        string         `json:"tool_name"`
	ToolArgs        map[string]any `json:"tool_args"`
	SkillName       string         `json:"skill_name"`
	SearchQueries   []string       `json:"search_queries"`
	DocTypes        []string       `json:"doc_types"`
	ClarifyQuestion string         `json:"clarify_question"`
	Reason          string         `json:"reason"`
}

type llmCompletePlan struct {
	Confidence float64               `json:"confidence"`
	Steps      []llmCompletePlanStep `json:"steps"`
}

type llmCompletePlanStep struct {
	Action        string         `json:"action"`
	ToolName      string         `json:"tool_name"`
	SkillName     string         `json:"skill_name"`
	Query         string         `json:"query"`
	SearchQueries []string       `json:"search_queries"`
	DocTypes      []string       `json:"doc_types"`
	Params        map[string]any `json:"params"`
	Reason        string         `json:"reason"`
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
	plan, err := p.generateCompletePlan(ctx, request, "", "")
	if err != nil || len(plan.Steps) == 0 {
		return fallback, nil
	}
	return plan, nil
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
{"confidence":0.0,"steps":[{"action":"retrieve|call_tool|run_skill|clarify|reflect|finish","tool_name":"","skill_name":"","query":"","search_queries":[],"doc_types":[],"params":{},"reason":""}]}`,
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
			if !containsString(request.AllowedTools, step.ToolName) {
				return nil, fmt.Errorf("complete plan selected non-whitelisted tool %q", step.ToolName)
			}
		case ActionRunSkill:
			if !skillAllowedForIntent(request.Intent.Secondary, step.SkillName) {
				return nil, fmt.Errorf(
					"complete plan selected skill %q for incompatible intent %q",
					step.SkillName,
					request.Intent.Secondary,
				)
			}
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
	addSequentialDependencies(steps)
	return steps, nil
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
			if !containsString(request.AllowedTools, call.Name) {
				return nil, fmt.Errorf("llm selected non-whitelisted tool %q", call.Name)
			}
			return &PlanStep{
				StepID:     fmt.Sprintf("step_%02d", currentStep+1),
				Action:     ActionCallTool,
				ToolName:   call.Name,
				Params:     call.Arguments,
				ReasonCode: "llm_function_call",
			}, nil
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
		if !containsString(request.AllowedTools, llmOut.ActionDetail.ToolName) {
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
	}

	return step, nil
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

func (p *LLMPlanner) allowedToolDefinitions(allowed []string) string {
	definitions := make([]tool.Definition, 0, len(allowed))
	for _, name := range allowed {
		if definition, ok := p.toolDefinitions[name]; ok {
			definitions = append(definitions, definition)
			continue
		}
		definitions = append(definitions, tool.Definition{Name: name})
	}
	raw, _ := json.Marshal(definitions)
	return string(raw)
}

func (p *LLMPlanner) allowedLLMToolDefinitions(allowed []string) []llm.ToolDefinition {
	definitions := make([]llm.ToolDefinition, 0, len(allowed))
	for _, name := range allowed {
		definition, ok := p.toolDefinitions[name]
		if !ok {
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
