package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"CleanCaregent/internal/generator"
	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/trace"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	ErrMaxStepsExceeded = errors.New("agent maximum steps exceeded")
	ErrRepeatedAction   = errors.New("agent repeated action detected")
)

// AgenticConfig holds configuration parameters for the AgenticRunner.
type AgenticConfig struct {
	MaxSteps      int
	TokenBudget   int
	DenseTopK     int
	KeywordTopK   int
	RerankTopK    int
	MinDenseScore float64
	// EnableLLMComponents toggles LLM-powered intent, rewrite, plan, and reflect.
	EnableLLMComponents bool
}

// AgenticRunner is the main agentic RAG pipeline runner.
// It supports both rule-based and LLM-powered components, degrading gracefully
// when LLM components are unavailable.
type AgenticRunner struct {
	router          intent.Router
	rewriter        QueryRewriter
	planner         Planner
	retriever       rag.Retriever
	generator       generator.Generator
	traceStore      trace.Store
	dynamicExecutor DynamicExecutor
	reflector       Reflector
	clarifier       *Clarifier
	prompts         *prompt.Registry
	config          AgenticConfig
}

// AgenticRunnerOption allows optional components to be injected.
type AgenticRunnerOption func(*AgenticRunner)

// WithLLMRouter overrides the default rule router with an LLM hybrid router.
func WithLLMRouter(router intent.Router) AgenticRunnerOption {
	return func(r *AgenticRunner) { r.router = router }
}

// WithLLMRewriter overrides the default rule rewriter with an LLM rewriter.
func WithLLMRewriter(rewriter QueryRewriter) AgenticRunnerOption {
	return func(r *AgenticRunner) { r.rewriter = rewriter }
}

// WithLLMPlanner overrides the default rule planner with an LLM planner.
func WithLLMPlanner(planner Planner) AgenticRunnerOption {
	return func(r *AgenticRunner) { r.planner = planner }
}

// WithLLMReflector overrides the default rule reflector with an LLM reflector.
func WithLLMReflector(reflector Reflector) AgenticRunnerOption {
	return func(r *AgenticRunner) { r.reflector = reflector }
}

// WithClarifier injects an intelligent clarification generator.
func WithClarifier(clarifier *Clarifier) AgenticRunnerOption {
	return func(r *AgenticRunner) { r.clarifier = clarifier }
}

// WithPromptRegistry injects a prompt template registry for scenario-based generation.
func WithPromptRegistry(prompts *prompt.Registry) AgenticRunnerOption {
	return func(r *AgenticRunner) { r.prompts = prompts }
}

// NewAgenticRunner creates an AgenticRunner with sensible defaults for missing dependencies.
func NewAgenticRunner(
	router intent.Router,
	rewriter QueryRewriter,
	planner Planner,
	retriever rag.Retriever,
	generator generator.Generator,
	traceStore trace.Store,
	dynamicExecutor DynamicExecutor,
	config AgenticConfig,
	opts ...AgenticRunnerOption,
) *AgenticRunner {
	r := &AgenticRunner{
		router:          router,
		rewriter:        rewriter,
		planner:         planner,
		retriever:       retriever,
		generator:       generator,
		traceStore:      traceStore,
		dynamicExecutor: dynamicExecutor,
		reflector:       NewGroundingReflector(),
		prompts:         prompt.NewRegistry(),
		config:          config,
	}
	for _, opt := range opts {
		opt(r)
	}
	// If no clarifier was injected, use a nil-backed one (degrades to rules).
	if r.clarifier == nil {
		r.clarifier = NewClarifier(nil, r.prompts)
	}
	return r
}

// Run executes the full agentic RAG pipeline.
func (r *AgenticRunner) Run(ctx context.Context, request Request, sink EventSink) (result Result, runErr error) {
	startedAt := time.Now()
	usageCollector := &llm.UsageCollector{}
	ctx = llm.WithUsageCollector(ctx, usageCollector)
	ctx, span := otel.Tracer("clean-care-agent/agent").Start(ctx, "agent.run")
	span.SetAttributes(
		attribute.String("agent.trace_id", request.TraceID),
		attribute.String("agent.conversation_id", request.ConversationID),
	)
	defer func() {
		if runErr != nil {
			span.RecordError(runErr)
			span.SetStatus(codes.Error, runErr.Error())
		}
		span.End()
	}()

	// ---- Step 1: Intent Routing ----
	route, err := r.router.Route(ctx, intent.RouteRequest{
		Query:          request.Query,
		Summary:        request.Context.Summary,
		RecentMessages: request.Context.RecentMessages,
	})
	if err != nil {
		return Result{}, fmt.Errorf("route intent: %w", err)
	}
	route = continueDiagnosisRoute(route, request)
	span.SetAttributes(
		attribute.String("agent.intent", string(route.Secondary)),
		attribute.Float64("agent.intent_confidence", route.Confidence),
	)

	// ---- Step 2: Query Rewriting ----
	rewrite, err := r.rewriter.Rewrite(ctx, RewriteRequest{
		Query:          request.Query,
		Intent:         route,
		Summary:        request.Context.Summary,
		RecentMessages: request.Context.RecentMessages,
	})
	if err != nil {
		return Result{}, fmt.Errorf("rewrite query: %w", err)
	}
	route.Entities = rewrite.Entities

	// ---- Step 3: Planning ----
	toolWhitelist := allowedTools(route.Secondary)
	planRequest := PlanRequest{
		TraceID:        request.TraceID,
		UserID:         request.UserID,
		ConversationID: request.ConversationID,
		Query:          rewrite.Rewritten,
		RewrittenQueries: uniqueStrings(append(
			append([]string(nil), rewrite.SearchQueries...),
			rewrite.SubQuestions...,
		)),
		Intent:       route,
		AllowedTools: toolWhitelist,
		MaxSteps:     r.config.MaxSteps,
		TokenBudget:  r.config.TokenBudget,
		Deadline:     contextDeadline(ctx),
	}
	plan, err := r.planner.Plan(ctx, planRequest)
	if err != nil {
		return Result{}, fmt.Errorf("plan agent request: %w", err)
	}

	reactivePlanner, useReactivePlanner := r.planner.(ReactivePlanner)
	useReactivePlanner = useReactivePlanner && plan.Mode == "react" && r.config.EnableLLMComponents
	if useReactivePlanner {
		staticFallback := append([]PlanStep(nil), plan.Steps...)
		firstStep, nextErr := reactivePlanner.NextStep(ctx, planRequest, 0, nil, "", "")
		if nextErr != nil {
			useReactivePlanner = false
			plan.Steps = staticFallback
		} else if firstStep == nil {
			plan.Steps = []PlanStep{finishStep(1, "llm_finished_without_action")}
		} else {
			plan.Steps = []PlanStep{*firstStep}
		}
	}
	span.SetAttributes(
		attribute.String("agent.route_mode", plan.Mode),
		attribute.Int("agent.plan_steps", len(plan.Steps)),
	)

	// ---- Trace start ----
	r.startTrace(ctx, request, route, plan)
	var (
		searchResults            []rag.SearchResult
		evidences                []Evidence
		answer                   string
		intentionalClarification bool
		finalEvidenceIDs         []string
		outputTokens             int
	)
	inputTokens := estimateTokens(request.Query + "\n" + request.Context.Summary)
	defer func() {
		status, errorCode := "success", ""
		if runErr != nil {
			status, errorCode = "failed", traceErrorCode(runErr)
		}
		usage := usageCollector.Snapshot()
		if usage.PromptTokens > 0 {
			inputTokens = usage.PromptTokens
		}
		if usage.CompletionTokens > 0 {
			outputTokens = usage.CompletionTokens
		}
		r.finishTrace(
			context.WithoutCancel(ctx),
			request.TraceID,
			status,
			errorCode,
			finalEvidenceIDs,
			inputTokens,
			outputTokens,
			startedAt,
		)
	}()

	// ---- Emit status ----
	if sink != nil {
		if err := sink(Event{Type: "status", Data: map[string]any{
			"stage":      "planned",
			"mode":       plan.Mode,
			"intent":     route.Secondary,
			"confidence": route.Confidence,
			"trace_id":   request.TraceID,
		}}); err != nil {
			return Result{}, err
		}
	}

	// ---- Step 4: Execute plan steps ----
	seenActions := map[string]struct{}{}
	executedActions := 0
	for index := 0; index < len(plan.Steps); index++ {
		planStep := plan.Steps[index]
		if planStep.Action != ActionFinish && executedActions >= plan.MaxSteps {
			planStep = finishStep(index+1, "max_steps_reached")
			plan.Steps[index] = planStep
		}
		if planStep.Action != ActionFinish &&
			plan.TokenBudget > 0 &&
			runtimeTokenUsage(
				inputTokens,
				usageCollector.Snapshot(),
				searchResults,
				evidences,
			) >= plan.TokenBudget {
			planStep = finishStep(index+1, "token_budget_reached")
			plan.Steps[index] = planStep
		}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		signature := actionSignature(planStep)
		if _, exists := seenActions[signature]; exists {
			if !useReactivePlanner {
				return Result{}, ErrRepeatedAction
			}
			planStep = finishStep(index+1, "repeated_action_blocked")
			plan.Steps[index] = planStep
			signature = actionSignature(planStep)
		}
		seenActions[signature] = struct{}{}
		if planStep.Action != ActionFinish {
			executedActions++
		}

		stepStartedAt := time.Now()
		stepStatus := "success"
		stepMetadata := map[string]any{
			"action":      planStep.Action,
			"reason_code": planStep.ReasonCode,
		}

		switch planStep.Action {
		case ActionAnswerDirect:
			answer = r.directAnswerWithClarifier(ctx, route, request.Query)

		case ActionClarify:
			answer = r.generateClarification(ctx, route, request.Query, rewrite.Entities)

		case ActionRetrieve:
			items, retrieveErr := r.retrievePlanStep(ctx, planStep, rewrite)
			if retrieveErr != nil {
				stepStatus = "failed"
				r.appendTraceStep(ctx, request.TraceID, planStep, stepStatus, stepStartedAt, stepMetadata)
				return Result{}, fmt.Errorf("retrieve evidence: %w", retrieveErr)
			}
			searchResults = mergeSearchResults(searchResults, items)
			searchResults = trimSearchResults(searchResults, max(800, plan.TokenBudget-inputTokens))
			stepMetadata["result_count"] = len(items)

		case ActionCallTool, ActionRunSkill:
			if planStep.Action == ActionCallTool && !containsString(toolWhitelist, planStep.ToolName) {
				stepStatus = "blocked"
				r.appendTraceStep(ctx, request.TraceID, planStep, stepStatus, stepStartedAt, stepMetadata)
				return Result{}, fmt.Errorf("tool %s is not allowed for intent %s", planStep.ToolName, route.Secondary)
			}
			if r.dynamicExecutor == nil {
				answer = "该问题需要查询动态业务数据，但对应工具尚未启用。请稍后重试。"
				stepMetadata["degraded"] = true
				break
			}
			dynamicResult, executeErr := r.dynamicExecutor.Execute(ctx, DynamicExecutionRequest{
				Request: request,
				Intent:  string(route.Secondary),
				Step:    planStep,
			})
			if executeErr != nil {
				stepStatus = "failed"
				r.appendTraceStep(ctx, request.TraceID, planStep, stepStatus, stepStartedAt, stepMetadata)
				return Result{}, executeErr
			}
			answer = dynamicResult.Answer
			evidences = append(evidences, dynamicResult.Evidences...)
			searchResults = mergeSearchResults(searchResults, dynamicResult.SearchData)
			for key, value := range dynamicResult.Metadata {
				stepMetadata[key] = value
			}
			if metadataBool(dynamicResult.Metadata, "intentional_clarification") {
				intentionalClarification = true
			}

		case ActionReflect:
			stepMetadata["evidence_count"] = len(searchResults) + len(evidences)
			if len(searchResults) == 0 && len(evidences) == 0 && answer == "" {
				answer = "当前没有找到足够可靠的证据。请补充具体型号、订单号或故障现象。"
				stepMetadata["low_confidence"] = true
			}

		case ActionFinish:
			if answer == "" {
				evidences = mergeEvidences(evidences, searchResults)
				scenario := selectGenerateScenario(route.Secondary)
				generated, generateErr := r.generator.GenerateWithScenario(
					ctx,
					scenario,
					rewrite.Rewritten,
					searchResults,
					buildToolResultsSummary(evidences),
					request.Context.Summary,
					rewrite.Entities["models"],
				)
				if generateErr != nil {
					stepStatus = "failed"
					r.appendTraceStep(ctx, request.TraceID, planStep, stepStatus, stepStartedAt, stepMetadata)
					return Result{}, fmt.Errorf("generate grounded answer: %w", generateErr)
				}
				answer = generated
			}
		}
		r.appendTraceStep(ctx, request.TraceID, planStep, stepStatus, stepStartedAt, stepMetadata)

		// A Skill owns its internal retrieval/tool orchestration. Once it returns
		// a user-facing answer or the next diagnostic question, this turn is
		// complete and must not be followed by speculative LLM actions.
		if useReactivePlanner && planStep.Action == ActionRunSkill && answer != "" {
			plan.Steps = append(plan.Steps, finishStep(index+2, "skill_turn_complete"))
			continue
		}

		if useReactivePlanner &&
			planStep.Action != ActionFinish &&
			planStep.Action != ActionClarify &&
			planStep.Action != ActionAnswerDirect {
			currentEvidences := mergeEvidences(evidences, searchResults)
			nextStep, nextErr := reactivePlanner.NextStep(
				ctx,
				planRequest,
				executedActions,
				currentEvidences,
				summarizeSearchResults(searchResults),
				buildToolResultsSummary(currentEvidences),
			)
			if nextErr != nil {
				plan.Steps = append(plan.Steps, finishStep(index+2, "llm_next_step_failed"))
				continue
			}
			if nextStep == nil {
				plan.Steps = append(plan.Steps, finishStep(index+2, "llm_finish"))
				continue
			}
			plan.Steps = append(plan.Steps, *nextStep)
		}
	}

	// ---- Step 5: Final reflection ----
	evidences = mergeEvidences(evidences, searchResults)
	reflectionStartedAt := time.Now()
	reflectionRequest := ReflectionRequest{
		Query:                    request.Query,
		Intent:                   route.Secondary,
		Answer:                   answer,
		Evidences:                evidences,
		SubQuestions:             rewrite.SubQuestions,
		IntentionalClarification: intentionalClarification,
	}
	var reflection ReflectionResult
	if plan.TokenBudget > 0 &&
		runtimeTokenUsage(
			inputTokens,
			usageCollector.Snapshot(),
			searchResults,
			evidences,
		) >= plan.TokenBudget {
		reflection = NewGroundingReflector().Review(
			reflectionRequest.Query,
			reflectionRequest.Intent,
			reflectionRequest.Answer,
			reflectionRequest.Evidences,
		)
		reflection.Warnings = append(reflection.Warnings, "llm_reflection_skipped_token_budget")
	} else {
		reflection = r.reviewAnswer(ctx, reflectionRequest)
	}
	if reflection.Action == "rerun_retrieval" {
		recoveryStartedAt := time.Now()
		rerunQuery := strings.TrimSpace(reflection.RerunQuery)
		if rerunQuery == "" {
			rerunQuery = rewrite.Rewritten
		}
		recovered, recoveryErr := r.retrievePlanStep(ctx, PlanStep{
			StepID:     "step_reflection_retrieval",
			Action:     ActionRetrieve,
			Query:      rerunQuery,
			Params:     map[string]any{},
			ReasonCode: "llm_reflection_rerun",
		}, rewrite)
		recoveryStatus := "success"
		if recoveryErr != nil || len(recovered) == 0 {
			recoveryStatus = "failed"
			reflection.LowConfidence = true
			reflection.Warnings = append(reflection.Warnings, "reflection_retrieval_failed")
		} else {
			searchResults = mergeSearchResults(searchResults, recovered)
			searchResults = trimSearchResults(
				searchResults,
				max(800, plan.TokenBudget-inputTokens),
			)
			evidences = mergeEvidences(evidences, searchResults)
			regenerated, generateErr := r.generator.GenerateWithScenario(
				ctx,
				selectGenerateScenario(route.Secondary),
				rewrite.Rewritten,
				searchResults,
				buildToolResultsSummary(evidences),
				request.Context.Summary,
				rewrite.Entities["models"],
			)
			if generateErr != nil {
				recoveryStatus = "failed"
				reflection.LowConfidence = true
				reflection.Warnings = append(reflection.Warnings, "reflection_regeneration_failed")
			} else {
				answer = regenerated
				reflection = r.reviewAnswer(ctx, ReflectionRequest{
					Query:                    request.Query,
					Intent:                   route.Secondary,
					Answer:                   answer,
					Evidences:                evidences,
					SubQuestions:             rewrite.SubQuestions,
					IntentionalClarification: intentionalClarification,
				})
			}
		}
		r.appendTraceStep(ctx, request.TraceID, PlanStep{
			StepID:     "step_reflection_recovery",
			Action:     ActionRetrieve,
			Query:      rerunQuery,
			ReasonCode: "llm_reflection_recovery",
		}, recoveryStatus, recoveryStartedAt, map[string]any{
			"rerun_query":  rerunQuery,
			"result_count": len(recovered),
		})
	} else if reflection.Action == "regenerate" {
		if regenerated, generateErr := r.generator.GenerateWithScenario(
			ctx,
			selectGenerateScenario(route.Secondary),
			regenerationQuery(rewrite.Rewritten, reflection.UnsupportedClaims),
			searchResults,
			buildToolResultsSummary(evidences),
			request.Context.Summary,
			rewrite.Entities["models"],
		); generateErr == nil {
			answer = regenerated
			reflection = r.reviewAnswer(ctx, ReflectionRequest{
				Query:                    request.Query,
				Intent:                   route.Secondary,
				Answer:                   answer,
				Evidences:                evidences,
				SubQuestions:             rewrite.SubQuestions,
				IntentionalClarification: intentionalClarification,
			})
		}
	} else if reflection.Action == "clarify" && !intentionalClarification {
		answer = r.generateClarification(ctx, route, request.Query, rewrite.Entities)
		reflection.Answer = answer
	}
	answer = reflection.Answer
	if reflection.Action == "regenerate" && len(reflection.UnsupportedClaims) > 0 {
		answer = removeUnsupportedClaims(answer, reflection.UnsupportedClaims)
		reflection.Answer = answer
		reflection.Warnings = append(reflection.Warnings, "unsupported_claims_removed_after_regeneration")
	}
	if reflection.ShouldTransfer {
		answer += "\n\n当前结论置信度不足，建议转人工客服复核后再执行售后操作。"
	}
	r.appendTraceStep(ctx, request.TraceID, PlanStep{
		StepID:     "step_grounding_review",
		Action:     ActionReflect,
		ReasonCode: "final_grounding_review",
	}, reflectionStatus(reflection), reflectionStartedAt, map[string]any{
		"low_confidence":     reflection.LowConfidence,
		"should_transfer":    reflection.ShouldTransfer,
		"warnings":           reflection.Warnings,
		"unsupported_claims": reflection.UnsupportedClaims,
		"evidence_count":     len(evidences),
	})

	// ---- Step 6: Stream output ----
	finalEvidenceIDs = evidenceIDs(evidences)
	outputTokens = estimateTokens(answer)
	for _, evidence := range evidences {
		if sink != nil {
			if err := sink(Event{Type: "evidence", Data: evidence}); err != nil {
				return Result{}, err
			}
		}
	}
	for _, content := range splitForStream(answer, 24) {
		if sink != nil {
			if err := sink(Event{Type: "delta", Data: map[string]string{"content": content}}); err != nil {
				return Result{}, err
			}
		}
	}
	return Result{Answer: answer, Evidences: evidences, Mode: "agentic"}, nil
}

// directAnswerWithClarifier generates direct answers (chitchat, out-of-scope) using
// the clarifier for more natural responses when available.
func (r *AgenticRunner) directAnswerWithClarifier(ctx context.Context, route intent.Result, query string) string {
	switch route.Secondary {
	case intent.Chitchat:
		return "你好！我是 CleanCare 清洁电器智能客服助手，可以帮您查询扫地机器人、空气净化器、净水器、加湿器的参数、对比、选购推荐、故障排查和售后问题。请问有什么可以帮您的？"
	case intent.OutOfScope:
		return "很抱歉，我目前只支持扫地机器人、空气净化器、净水器和加湿器相关问题的咨询。如果您有这些品类的问题，我很乐意帮您解答。"
	default:
		return "请描述您的清洁电器问题，我会尽力帮您解决。"
	}
}

// generateClarification uses the Clarifier to generate a context-aware clarification.
func (r *AgenticRunner) generateClarification(
	ctx context.Context,
	route intent.Result,
	query string,
	entities map[string]string,
) string {
	missing := []string{}
	if entities["models"] == "" && needsModelForIntent(route.Secondary) {
		missing = append(missing, "产品型号")
	}
	if entities["order_no"] == "" && needsOrderForIntent(route.Secondary) {
		missing = append(missing, "订单号")
	}
	return r.clarifier.Clarify(ctx, query, route.Secondary, entities, missing)
}

func needsModelForIntent(t intent.Type) bool {
	switch t {
	case intent.ProductParameter, intent.ProductComparison, intent.AccessoryCompatibility,
		intent.PriceQuery, intent.InventoryQuery, intent.UsageInstruction:
		return true
	}
	return false
}

func needsOrderForIntent(t intent.Type) bool {
	switch t {
	case intent.ReturnEligibility, intent.WarrantyQuery, intent.CreateAfterSalesTicket:
		return true
	}
	return false
}

func continueDiagnosisRoute(route intent.Result, request Request) intent.Result {
	state := request.Context.DiagnosisState
	if state == nil || state.FaultNodeID == "" {
		return route
	}
	if route.Secondary != intent.Troubleshooting &&
		!isDiagnosisFollowUp(request.Query, route.Secondary) {
		return route
	}
	route.Primary = "diagnosis"
	route.Secondary = intent.Troubleshooting
	route.NeedClarify = false
	if route.Confidence < 0.98 {
		route.Confidence = 0.98
	}
	if route.Entities == nil {
		route.Entities = map[string]string{}
	}
	if route.Entities["models"] == "" {
		route.Entities["models"] = state.ProductModel
	}
	return route
}

func isDiagnosisFollowUp(query string, routed intent.Type) bool {
	if routed != intent.Clarification && routed != intent.Chitchat {
		return false
	}
	query = strings.ToLower(strings.TrimSpace(query))
	for _, marker := range []string{
		"是", "否", "有", "没有", "亮", "不亮", "可以", "不能", "好了",
		"还是", "仍然", "异响", "报错", "闪烁", "正常",
	} {
		if strings.Contains(query, marker) {
			return true
		}
	}
	return false
}

func metadataBool(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func regenerationQuery(query string, unsupportedClaims []string) string {
	if len(unsupportedClaims) == 0 {
		return query
	}
	return query + "\n\n重新生成要求：删除以下无证据声明，不要改写成同义推测：" +
		strings.Join(unsupportedClaims, "；")
}

func removeUnsupportedClaims(answer string, unsupportedClaims []string) string {
	for _, claim := range unsupportedClaims {
		claim = strings.TrimSpace(claim)
		if claim == "" {
			continue
		}
		answer = strings.ReplaceAll(answer, claim, "")
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "当前证据不足，无法形成可靠结论。请补充具体型号或由人工客服复核。"
	}
	return answer
}

// selectGenerateScenario maps intent types to the appropriate generation scenario.
func selectGenerateScenario(intentType intent.Type) prompt.Scenario {
	switch intentType {
	case intent.ProductComparison:
		return prompt.ScenarioGenerateCompare
	case intent.Troubleshooting:
		return prompt.ScenarioGenerateDiagnose
	case intent.ReturnEligibility, intent.WarrantyQuery, intent.CreateAfterSalesTicket:
		return prompt.ScenarioGeneratePolicy
	default:
		return prompt.ScenarioGenerateGeneric
	}
}

func (r *AgenticRunner) reviewAnswer(
	ctx context.Context,
	request ReflectionRequest,
) ReflectionResult {
	if contextual, ok := r.reflector.(ContextualReflector); ok {
		return contextual.ReviewContext(ctx, request)
	}
	return r.reflector.Review(
		request.Query,
		request.Intent,
		request.Answer,
		request.Evidences,
	)
}

func (r *AgenticRunner) retrievePlanStep(
	ctx context.Context,
	step PlanStep,
	rewrite RewriteResult,
) ([]rag.SearchResult, error) {
	queries := stringSliceParam(step.Params["search_queries"])
	if len(queries) == 0 {
		for _, value := range strings.Split(step.Query, "\n") {
			if value = strings.TrimSpace(value); value != "" {
				queries = append(queries, value)
			}
		}
	}
	if len(queries) == 0 {
		queries = []string{rewrite.Rewritten}
	}
	if len(queries) > 4 {
		queries = queries[:4]
	}
	docTypes := stringSliceParam(step.Params["doc_types"])
	effectiveAt := time.Now().UTC()

	results := make([][]rag.SearchResult, len(queries))
	errorsByQuery := make([]error, len(queries))
	var waitGroup sync.WaitGroup
	for index, query := range queries {
		index, query := index, query
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			results[index], errorsByQuery[index] = r.retriever.Search(ctx, rag.SearchRequest{
				Query: query,
				Mode:  rag.SearchHybrid,
				Filter: rag.MetadataFilter{
					Models:      splitCSV(rewrite.Entities["models"]),
					DocTypes:    docTypes,
					EffectiveAt: &effectiveAt,
				},
				DenseTopK:   r.config.DenseTopK,
				KeywordTopK: r.config.KeywordTopK,
				RerankTopK:  r.config.RerankTopK,
				MinScore:    r.config.MinDenseScore,
				NeedRerank:  true,
			})
		}()
	}
	waitGroup.Wait()

	var (
		merged      []rag.SearchResult
		failedCount int
		lastErr     error
	)
	for index := range results {
		if errorsByQuery[index] != nil {
			failedCount++
			lastErr = errorsByQuery[index]
			continue
		}
		merged = mergeSearchResults(merged, results[index])
	}
	if failedCount == len(queries) {
		return nil, lastErr
	}
	if maxResults := intParam(step.Params["max_results"]); maxResults > 0 && len(merged) > maxResults {
		merged = merged[:maxResults]
	}
	return merged, nil
}

// buildToolResultsSummary creates a summary string from tool result evidences.
func buildToolResultsSummary(evidences []Evidence) string {
	var builder strings.Builder
	for _, ev := range evidences {
		if ev.Kind == "tool_result" || ev.Kind == "tool_error" {
			fmt.Fprintf(&builder, "[%s] %s: %s\n", ev.ID, ev.Title, ev.Content)
		}
	}
	if builder.Len() == 0 {
		return "(无工具调用结果)"
	}
	return builder.String()
}

// ---------------------------------------------------------------------------
// Trace helpers
// ---------------------------------------------------------------------------

func (r *AgenticRunner) startTrace(ctx context.Context, request Request, route intent.Result, plan *Plan) {
	if r.traceStore == nil {
		return
	}
	_ = r.traceStore.Start(ctx, trace.AgentTrace{
		TraceID:        request.TraceID,
		ConversationID: request.ConversationID,
		Intent:         string(route.Secondary),
		RouteMode:      plan.Mode,
		Plan:           plan,
		StartedAt:      time.Now().UTC(),
	})
}

func (r *AgenticRunner) appendTraceStep(
	ctx context.Context,
	traceID string,
	planStep PlanStep,
	status string,
	startedAt time.Time,
	metadata map[string]any,
) {
	if r.traceStore == nil {
		return
	}
	_ = r.traceStore.AppendStep(ctx, trace.Step{
		TraceID:    traceID,
		StepID:     planStep.StepID,
		Type:       string(planStep.Action),
		Status:     status,
		DurationMS: time.Since(startedAt).Milliseconds(),
		Metadata:   metadata,
	})
}

func (r *AgenticRunner) finishTrace(
	ctx context.Context,
	traceID string,
	status string,
	errorCode string,
	evidenceIDs []string,
	inputTokens int,
	outputTokens int,
	startedAt time.Time,
) {
	if r.traceStore == nil {
		return
	}
	_ = r.traceStore.Finish(ctx, traceID, trace.Result{
		Status:       status,
		ErrorCode:    errorCode,
		EvidenceIDs:  evidenceIDs,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		LatencyMS:    time.Since(startedAt).Milliseconds(),
		FinishedAt:   time.Now().UTC(),
	})
}

// ---------------------------------------------------------------------------
// Utility functions (unchanged from original)
// ---------------------------------------------------------------------------

func mergeSearchResults(existing, added []rag.SearchResult) []rag.SearchResult {
	seen := make(map[string]struct{}, len(existing)+len(added))
	result := make([]rag.SearchResult, 0, len(existing)+len(added))
	for _, item := range append(existing, added...) {
		if _, ok := seen[item.ChunkID]; ok {
			continue
		}
		seen[item.ChunkID] = struct{}{}
		result = append(result, item)
	}
	return result
}

func mergeEvidences(existing []Evidence, searchResults []rag.SearchResult) []Evidence {
	seen := make(map[string]struct{}, len(existing)+len(searchResults))
	result := make([]Evidence, 0, len(existing)+len(searchResults))
	for _, item := range searchResults {
		if _, ok := seen[item.ChunkID]; ok {
			continue
		}
		seen[item.ChunkID] = struct{}{}
		result = append(result, Evidence{
			Kind:     "kb_chunk",
			SourceID: item.ChunkID,
			Title:    item.Title,
			Content:  item.Content,
			Metadata: item.Metadata,
		})
	}
	for _, item := range existing {
		if _, ok := seen[item.SourceID]; ok {
			continue
		}
		seen[item.SourceID] = struct{}{}
		result = append(result, item)
	}
	for index := range result {
		result[index].ID = fmt.Sprintf("E%d", index+1)
	}
	return result
}

func actionSignature(step PlanStep) string {
	raw, _ := json.Marshal(step.Params)
	return string(step.Action) + "|" + step.ToolName + "|" + step.SkillName + "|" + step.Query + "|" + string(raw)
}

func allowedTools(intentType intent.Type) []string {
	switch intentType {
	case intent.PriceQuery:
		return []string{"price_query"}
	case intent.InventoryQuery:
		return []string{"inventory_check"}
	case intent.OrderQuery:
		return []string{"user_purchase_history", "order_lookup"}
	case intent.PurchaseRecommendation:
		return []string{"price_query", "inventory_check"}
	case intent.AccessoryCompatibility:
		return []string{"user_purchase_history", "price_query"}
	case intent.WarrantyQuery:
		return []string{"order_lookup", "warranty_check"}
	case intent.ReturnEligibility:
		return []string{"order_lookup"}
	case intent.Troubleshooting:
		return []string{"warranty_check", "create_after_sales_ticket"}
	case intent.CreateAfterSalesTicket:
		return []string{"order_lookup", "warranty_check", "create_after_sales_ticket"}
	default:
		return nil
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func contextDeadline(ctx context.Context) time.Time {
	if value, ok := ctx.Deadline(); ok {
		return value
	}
	return time.Now().Add(20 * time.Second)
}

func traceErrorCode(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "AGENT_TIMEOUT"
	case errors.Is(err, ErrRepeatedAction):
		return "REPEATED_ACTION"
	case errors.Is(err, ErrMaxStepsExceeded):
		return "MAX_STEPS_EXCEEDED"
	default:
		return "AGENT_FAILED"
	}
}

func reflectionStatus(result ReflectionResult) string {
	if result.LowConfidence {
		return "warning"
	}
	return "success"
}

func evidenceIDs(evidences []Evidence) []string {
	result := make([]string, 0, len(evidences))
	for _, evidence := range evidences {
		if evidence.ID != "" {
			result = append(result, evidence.ID)
		}
	}
	return result
}

func estimateTokens(value string) int {
	runes := len([]rune(value))
	if runes == 0 {
		return 0
	}
	return max(1, (runes+2)/3)
}

func currentTokenUsage(inputTokens int, searchResults []rag.SearchResult, evidences []Evidence) int {
	used := inputTokens
	for _, item := range searchResults {
		used += estimateTokens(item.Title + "\n" + item.Content)
	}
	for _, item := range evidences {
		used += estimateTokens(item.Title + "\n" + item.Content)
	}
	return used
}

func runtimeTokenUsage(
	inputTokens int,
	usage llm.Usage,
	searchResults []rag.SearchResult,
	evidences []Evidence,
) int {
	return currentTokenUsage(inputTokens, searchResults, evidences) + usage.TotalTokens
}

func summarizeSearchResults(items []rag.SearchResult) string {
	if len(items) == 0 {
		return ""
	}
	var builder strings.Builder
	for index, item := range items {
		content := []rune(strings.TrimSpace(item.Content))
		if len(content) > 240 {
			content = content[:240]
		}
		fmt.Fprintf(&builder, "[R%d] %s: %s\n", index+1, item.Title, string(content))
	}
	return builder.String()
}

func stringSliceParam(value any) []string {
	switch typed := value.(type) {
	case []string:
		return compactStringValues(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
		return compactStringValues(values)
	case string:
		return compactStringValues(strings.Split(typed, ","))
	default:
		return nil
	}
}

func intParam(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func compactStringValues(values []string) []string {
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

func finishStep(index int, reason string) PlanStep {
	return PlanStep{
		StepID:     fmt.Sprintf("step_%02d", index),
		Action:     ActionFinish,
		Params:     map[string]any{},
		ReasonCode: reason,
	}
}

func trimSearchResults(items []rag.SearchResult, budget int) []rag.SearchResult {
	if budget <= 0 {
		return nil
	}
	used := 0
	result := make([]rag.SearchResult, 0, len(items))
	for _, item := range items {
		cost := estimateTokens(item.Title + "\n" + item.Content)
		if len(result) > 0 && used+cost > budget {
			break
		}
		result = append(result, item)
		used += cost
	}
	return result
}
