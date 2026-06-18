package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"CleanCaregent/internal/embedding"
	"CleanCaregent/internal/generator"
	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/observability"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
	toolpkg "CleanCaregent/internal/tool"
	"CleanCaregent/internal/trace"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
)

var (
	ErrMaxStepsExceeded = errors.New("agent maximum steps exceeded")
	ErrRepeatedAction   = errors.New("agent repeated action detected")
	ErrPlanDependency   = errors.New("agent plan dependency not satisfied")
)

// AgenticConfig holds configuration parameters for the AgenticRunner.
type AgenticConfig struct {
	MaxSteps      int
	TokenBudget   int
	PlanningMode  string
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
	metricsLogger   *zap.Logger
	stepEmbedder    embedding.Embedder
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

func WithMetricsLogger(logger *zap.Logger) AgenticRunnerOption {
	return func(r *AgenticRunner) { r.metricsLogger = logger }
}

func WithStepEmbedder(embedder embedding.Embedder) AgenticRunnerOption {
	return func(r *AgenticRunner) { r.stepEmbedder = embedder }
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
	routeCtx, routeSpan := otel.Tracer("clean-care-agent/agent").Start(ctx, "intent.route")
	route, err := r.router.Route(routeCtx, intent.RouteRequest{
		Query:          request.Query,
		Summary:        request.Context.Summary,
		RecentMessages: request.Context.RecentMessages,
	})
	if err != nil {
		routeSpan.RecordError(err)
		routeSpan.SetStatus(codes.Error, err.Error())
		routeSpan.End()
		return Result{}, fmt.Errorf("route intent: %w", err)
	}
	routeSpan.SetAttributes(
		attribute.String("intent.primary", string(route.Primary)),
		attribute.String("intent.secondary", string(route.Secondary)),
		attribute.String("intent.route_source", route.RouteTrace.Source),
		attribute.Float64("intent.confidence", route.Confidence),
		attribute.Bool("intent.need_decomposition", route.NeedDecomposition),
	)
	routeSpan.End()
	route = continueDiagnosisRoute(route, request)
	span.SetAttributes(
		attribute.String("agent.intent", string(route.Secondary)),
		attribute.Float64("agent.intent_confidence", route.Confidence),
	)

	// ---- Step 2: Query Rewriting ----
	rewriteCtx, rewriteSpan := otel.Tracer("clean-care-agent/agent").Start(ctx, "query.rewrite")
	rewrite, err := r.rewriter.Rewrite(rewriteCtx, RewriteRequest{
		Query:          request.Query,
		Intent:         route,
		Summary:        request.Context.Summary,
		RecentMessages: request.Context.RecentMessages,
	})
	if err != nil {
		rewriteSpan.RecordError(err)
		rewriteSpan.SetStatus(codes.Error, err.Error())
		rewriteSpan.End()
		return Result{}, fmt.Errorf("rewrite query: %w", err)
	}
	rewriteSpan.SetAttributes(
		attribute.Int("query.search_queries", len(rewrite.SearchQueries)),
		attribute.Int("query.sub_questions", len(rewrite.SubQuestions)),
	)
	rewriteSpan.End()
	route.Entities = rewrite.Entities

	// ---- Step 3: Planning ----
	planCtx, planSpan := otel.Tracer("clean-care-agent/agent").Start(ctx, "planner.plan")
	toolWhitelist := allowedToolsForRoute(route)
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
	plan, err := r.planner.Plan(planCtx, planRequest)
	if err != nil {
		planSpan.RecordError(err)
		planSpan.SetStatus(codes.Error, err.Error())
		planSpan.End()
		return Result{}, fmt.Errorf("plan agent request: %w", err)
	}
	planExecutePlanner, usePlanExecute := r.planner.(PlanAndExecutePlanner)
	usePlanExecute = usePlanExecute && shouldUsePlanExecute(r.config.PlanningMode, route.Secondary)
	if usePlanExecute {
		completePlan, completeErr := planExecutePlanner.CompletePlan(planCtx, planRequest)
		if completeErr == nil && completePlan != nil && len(completePlan.Steps) > 0 {
			plan = completePlan
		} else {
			usePlanExecute = false
		}
	}

	reactivePlanner, useReactivePlanner := r.planner.(ReactivePlanner)
	useReactivePlanner = useReactivePlanner &&
		!usePlanExecute &&
		plan.Mode == "react" &&
		shouldUseReactivePlanning(route.Secondary) &&
		r.config.EnableLLMComponents
	if useReactivePlanner {
		staticFallback := append([]PlanStep(nil), plan.Steps...)
		firstStep, nextErr := reactivePlanner.NextStep(planCtx, planRequest, 0, nil, "", "")
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
	planSpan.SetAttributes(
		attribute.String("planner.mode", plan.Mode),
		attribute.Int("planner.steps", len(plan.Steps)),
		attribute.Bool("planner.plan_execute", usePlanExecute),
	)
	planSpan.End()

	// ---- Trace start ----
	r.startTrace(ctx, request, route, plan)
	r.appendTraceStep(ctx, request.TraceID, PlanStep{
		StepID:     "step_intent_route",
		Action:     ActionReflect,
		ReasonCode: "intent_route_explanation",
	}, "success", startedAt, map[string]any{
		"primary":            route.Primary,
		"secondary":          route.Secondary,
		"secondary_intents":  route.SecondaryIntents,
		"need_decomposition": route.NeedDecomposition,
		"source":             route.RouteTrace.Source,
		"matched_keywords":   route.RouteTrace.MatchedKeywords,
		"reasoning":          route.RouteTrace.Reasoning,
		"confidence_basis":   route.RouteTrace.ConfidenceBasis,
		"confidence":         route.Confidence,
	})
	var (
		searchResults            []rag.SearchResult
		evidences                []Evidence
		answer                   string
		intentionalClarification bool
		committedSideEffect      bool
		finalEvidenceIDs         []string
		outputTokens             int
		executedActions          int
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
		metrics := observability.DefaultAgentMetrics.RecordWithCost(
			time.Since(startedAt),
			inputTokens,
			outputTokens,
			usage.CostUSD,
			runErr != nil,
		)
		observability.DefaultPrometheusMetrics.RecordRequest(
			string(route.Secondary),
			status,
			time.Since(startedAt),
			inputTokens,
			outputTokens,
			executedActions,
			usage.CostUSD,
		)
		span.SetAttributes(
			attribute.Int("agent.input_tokens", inputTokens),
			attribute.Int("agent.output_tokens", outputTokens),
			attribute.Int64("agent.latency_ms", time.Since(startedAt).Milliseconds()),
			attribute.Int64("agent.p95_latency_ms", metrics.P95LatencyMS),
		)
		if r.metricsLogger != nil {
			r.metricsLogger.Info("agent runtime metrics",
				zap.String("trace_id", request.TraceID),
				zap.Int("input_tokens", inputTokens),
				zap.Int("output_tokens", outputTokens),
				zap.Int("total_tokens", inputTokens+outputTokens),
				zap.Int64("latency_ms", time.Since(startedAt).Milliseconds()),
				zap.Int64("p95_latency_ms", metrics.P95LatencyMS),
				zap.Float64("cost_usd", usage.CostUSD),
				zap.Bool("failed", runErr != nil),
			)
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
	completedStatuses := map[string]string{}
	completedSteps := make([]PlanStep, 0, len(plan.Steps))
	revisionCount := 0
	reviseRemaining := func(index int, cause error) bool {
		if !usePlanExecute || revisionCount >= 2 {
			return false
		}
		revised, reviseErr := planExecutePlanner.RevisePlan(
			ctx,
			planRequest,
			plan,
			append([]PlanStep(nil), completedSteps...),
			mergeEvidences(evidences, searchResults),
			cause,
		)
		if reviseErr != nil || revised == nil || len(revised.Steps) == 0 {
			return false
		}
		remainingBudget := plan.MaxSteps - executedActions
		if remainingBudget <= 0 {
			return false
		}
		steps := revised.Steps
		if len(steps) > remainingBudget {
			steps = steps[:remainingBudget]
		}
		lastCompleted := ""
		if len(completedSteps) > 0 {
			lastCompleted = completedSteps[len(completedSteps)-1].StepID
		}
		steps = rebasePlanSteps(steps, index+2, lastCompleted)
		plan.Steps = append(plan.Steps[:index+1], steps...)
		revisionCount++
		r.appendTraceStep(ctx, request.TraceID, PlanStep{
			StepID:     fmt.Sprintf("revision_%02d", revisionCount),
			Action:     ActionReflect,
			ReasonCode: "plan_revision",
		}, "success", time.Now(), map[string]any{
			"revision_count":  revisionCount,
			"revision_reason": errorText(cause),
			"remaining_steps": len(steps),
		})
		return true
	}
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
		if planStep.Action == ActionParallel &&
			executedActions+len(planStep.SubSteps) > plan.MaxSteps {
			planStep = finishStep(index+1, "parallel_step_budget_exceeded")
			plan.Steps[index] = planStep
		}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if missing := unsatisfiedDependencies(planStep, completedStatuses); len(missing) > 0 {
			stepStartedAt := time.Now()
			r.appendTraceStep(ctx, request.TraceID, planStep, "blocked", stepStartedAt, map[string]any{
				"missing_dependencies": missing,
			})
			return Result{}, fmt.Errorf("%w for %s: %s", ErrPlanDependency, planStep.StepID, strings.Join(missing, ","))
		}
		signature := actionSignature(planStep)
		if _, exists := seenActions[signature]; exists {
			repeatedStartedAt := time.Now()
			r.appendTraceStep(ctx, request.TraceID, planStep, "blocked", repeatedStartedAt, map[string]any{
				"reason":    ErrRepeatedAction.Error(),
				"recovered": true,
			})
			if reviseRemaining(index, ErrRepeatedAction) {
				continue
			}
			planStep = finishStep(index+1, "repeated_action_recovered_with_finish")
			planStep.Params["blocked_signature"] = signature
			plan.Steps[index] = planStep
			signature = actionSignature(planStep)
		}
		seenActions[signature] = struct{}{}
		if planStep.Action != ActionFinish {
			if planStep.Action == ActionParallel {
				executedActions += len(planStep.SubSteps)
			} else {
				executedActions++
			}
		}

		stepStartedAt := time.Now()
		stepTokenBefore := runtimeTokenUsage(inputTokens, usageCollector.Snapshot(), searchResults, evidences)
		stepStatus := "success"
		stepMetadata := map[string]any{
			"action":      planStep.Action,
			"reason_code": planStep.ReasonCode,
			"input": map[string]any{
				"query":      planStep.Query,
				"params":     planStep.Params,
				"tool_name":  planStep.ToolName,
				"skill_name": planStep.SkillName,
			},
			"token_before": stepTokenBefore,
		}

		switch planStep.Action {
		case ActionAnswerDirect:
			answer = r.directAnswerWithClarifier(ctx, route, request.Query)

		case ActionClarify:
			answer = r.generateClarification(ctx, route, request.Query, rewrite.Entities)

		case ActionRetrieve:
			beforeCount := len(searchResults)
			items, retrieveErr := r.retrievePlanStep(ctx, planStep, rewrite)
			if retrieveErr != nil {
				stepStatus = "failed"
				r.appendTraceStep(ctx, request.TraceID, planStep, stepStatus, stepStartedAt, stepMetadata)
				completedStatuses[planStep.StepID] = stepStatus
				if reviseRemaining(index, retrieveErr) {
					continue
				}
				return Result{}, fmt.Errorf("retrieve evidence: %w", retrieveErr)
			}
			searchResults = mergeSearchResults(searchResults, items)
			searchResults = trimSearchResults(searchResults, max(800, plan.TokenBudget-inputTokens))
			stepMetadata["result_count"] = len(items)
			stepMetadata["redundant_information"] = len(searchResults)-beforeCount < len(items)
			stepMetadata["output"] = map[string]any{"new_evidence_count": len(searchResults) - beforeCount}

		case ActionCallTool, ActionRunSkill:
			if planStep.Action == ActionCallTool && !toolpkg.NameAllowed(toolWhitelist, planStep.ToolName) {
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
				Request:          request,
				Intent:           string(route.Secondary),
				SecondaryIntents: intentTypeStrings(route.SecondaryIntents),
				AllowedTools:     toolWhitelist,
				Step:             planStep,
			})
			if executeErr != nil {
				stepStatus = "failed"
				r.appendTraceStep(ctx, request.TraceID, planStep, stepStatus, stepStartedAt, stepMetadata)
				completedStatuses[planStep.StepID] = stepStatus
				if reviseRemaining(index, executeErr) {
					continue
				}
				return Result{}, executeErr
			}
			answer = dynamicResult.Answer
			evidences = append(evidences, dynamicResult.Evidences...)
			if planStep.ToolName == "create_after_sales_ticket" &&
				hasToolEvidence(dynamicResult.Evidences, planStep.ToolName) {
				committedSideEffect = true
			}
			searchResults = mergeSearchResults(searchResults, dynamicResult.SearchData)
			for key, value := range dynamicResult.Metadata {
				stepMetadata[key] = value
			}
			if metadataBool(dynamicResult.Metadata, "intentional_clarification") {
				intentionalClarification = true
				plan.Steps = plan.Steps[:index+1]
			}
			stepMetadata["output"] = map[string]any{
				"answer_present": dynamicResult.Answer != "",
				"evidence_count": len(dynamicResult.Evidences),
			}

		case ActionParallel:
			parallelResult, parallelErr := r.executeParallelStep(
				ctx, request, route, rewrite, planStep, toolWhitelist,
			)
			if parallelErr != nil {
				stepStatus = "failed"
				r.appendTraceStep(ctx, request.TraceID, planStep, stepStatus, stepStartedAt, stepMetadata)
				completedStatuses[planStep.StepID] = stepStatus
				if reviseRemaining(index, parallelErr) {
					continue
				}
				return Result{}, parallelErr
			}
			searchResults = mergeSearchResults(searchResults, parallelResult.SearchData)
			evidences = append(evidences, parallelResult.Evidences...)
			if hasToolEvidence(parallelResult.Evidences, "create_after_sales_ticket") {
				committedSideEffect = true
			}
			if parallelResult.Answer != "" {
				answer = parallelResult.Answer
			}
			for key, value := range parallelResult.Metadata {
				stepMetadata[key] = value
			}
			if metadataBool(parallelResult.Metadata, "intentional_clarification") {
				intentionalClarification = true
				plan.Steps = plan.Steps[:index+1]
			}
			stepMetadata["output"] = map[string]any{
				"evidence_count": len(parallelResult.Evidences) + len(parallelResult.SearchData),
				"answer_present": parallelResult.Answer != "",
			}

		case ActionReflect:
			stepMetadata["evidence_count"] = len(searchResults) + len(evidences)
			if len(searchResults) == 0 && len(evidences) == 0 && answer == "" {
				answer = "当前没有找到足够可靠的证据。请补充具体型号、订单号或故障现象。"
				stepMetadata["low_confidence"] = true
			}

		case ActionFinish:
			if answer == "" ||
				(!committedSideEffect &&
					route.NeedDecomposition &&
					!answerCoversRequestedFacets(answer, request.Query)) {
				evidences = mergeEvidences(evidences, searchResults)
				scenario := selectGenerateScenario(route.Secondary)
				generateCtx, generateSpan := otel.Tracer("clean-care-agent/agent").Start(ctx, "llm.generate")
				generated, generateErr := r.generator.GenerateWithScenario(
					generateCtx,
					scenario,
					rewrite.Rewritten,
					searchResults,
					buildToolResultsSummary(evidences),
					request.Context.Summary,
					rewrite.Entities["models"],
				)
				if generateErr != nil {
					generateSpan.RecordError(generateErr)
					generateSpan.SetStatus(codes.Error, generateErr.Error())
					generateSpan.End()
					stepStatus = "failed"
					r.appendTraceStep(ctx, request.TraceID, planStep, stepStatus, stepStartedAt, stepMetadata)
					return Result{}, fmt.Errorf("generate grounded answer: %w", generateErr)
				}
				generateSpan.SetAttributes(
					attribute.String("generation.scenario", string(scenario)),
					attribute.Int("generation.evidence_count", len(searchResults)),
				)
				generateSpan.End()
				answer = generated
			}
		}
		stepTokenAfter := runtimeTokenUsage(inputTokens, usageCollector.Snapshot(), searchResults, evidences)
		stepMetadata["token_after"] = stepTokenAfter
		stepMetadata["token_consumed"] = max(0, stepTokenAfter-stepTokenBefore)
		stepMetadata["status"] = stepStatus
		r.appendTraceStep(ctx, request.TraceID, planStep, stepStatus, stepStartedAt, stepMetadata)
		completedStatuses[planStep.StepID] = stepStatus
		completedSteps = append(completedSteps, planStep)

		if usePlanExecute && planStep.Action != ActionFinish {
			needsRevision := metadataBool(stepMetadata, "degraded")
			if count, ok := stepMetadata["result_count"].(int); ok && count == 0 {
				needsRevision = true
			}
			if needsRevision && reviseRemaining(index, errors.New("plan observation requires revision")) {
				continue
			}
		}

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
			nextStep = r.validateOrFallbackNextStep(ctx, planRequest, *nextStep, completedSteps)
			plan.Steps = append(plan.Steps, *nextStep)
		}
	}

	// ---- Step 5: Final reflection ----
	evidences = mergeEvidences(evidences, searchResults)
	reflectionStartedAt := time.Now()
	reflectionCtx, reflectionSpan := otel.Tracer("clean-care-agent/agent").Start(ctx, "reflection.check")
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
		reflection = r.reviewAnswer(reflectionCtx, reflectionRequest)
	}
	if reflection.Action == "rerun_retrieval" {
		for attempt := 1; attempt <= 3 && reflection.Action == "rerun_retrieval"; attempt++ {
			recoveryStartedAt := time.Now()
			rerunQuery := strings.TrimSpace(reflection.RerunQuery)
			if rerunQuery == "" {
				rerunQuery = rewrite.Rewritten
			}
			params := map[string]any{}
			strategy := "same_query_and_filters"
			switch attempt {
			case 2:
				params["disable_filters"] = true
				strategy = "remove_metadata_filters"
			case 3:
				params["disable_filters"] = true
				params["search_mode"] = string(rag.SearchKeyword)
				strategy = "keyword_only"
			}
			recovered, recoveryErr := r.retrievePlanStep(reflectionCtx, PlanStep{
				StepID:     fmt.Sprintf("step_reflection_retrieval_%02d", attempt),
				Action:     ActionRetrieve,
				Query:      rerunQuery,
				Params:     params,
				ReasonCode: "reflection_strategy_" + strategy,
			}, rewrite)
			recoveryStatus := "success"
			if recoveryErr != nil || len(recovered) == 0 {
				recoveryStatus = "failed"
				reflection.LowConfidence = true
				reflection.Warnings = append(
					reflection.Warnings,
					fmt.Sprintf("reflection_retrieval_failed_attempt_%d", attempt),
				)
			} else {
				searchResults = mergeSearchResults(searchResults, recovered)
				searchResults = trimSearchResults(
					searchResults,
					max(800, plan.TokenBudget-inputTokens),
				)
				evidences = mergeEvidences(evidences, searchResults)
				regenerated, generateErr := r.generator.GenerateWithScenario(
					reflectionCtx,
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
					reflection.Warnings = append(
						reflection.Warnings,
						fmt.Sprintf("reflection_regeneration_failed_attempt_%d", attempt),
					)
				} else {
					answer = regenerated
					reflection = r.reviewAnswer(reflectionCtx, ReflectionRequest{
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
				StepID:     fmt.Sprintf("step_reflection_recovery_%02d", attempt),
				Action:     ActionRetrieve,
				Query:      rerunQuery,
				ReasonCode: "reflection_strategy_" + strategy,
			}, recoveryStatus, recoveryStartedAt, map[string]any{
				"rerun_query":  rerunQuery,
				"result_count": len(recovered),
				"attempt":      attempt,
				"strategy":     strategy,
			})
		}
	} else if reflection.Action == "regenerate" {
		if regenerated, generateErr := r.generator.GenerateWithScenario(
			reflectionCtx,
			selectGenerateScenario(route.Secondary),
			regenerationQuery(rewrite.Rewritten, reflection.UnsupportedClaims),
			searchResults,
			buildToolResultsSummary(evidences),
			request.Context.Summary,
			rewrite.Entities["models"],
		); generateErr == nil {
			answer = regenerated
			reflection = r.reviewAnswer(reflectionCtx, ReflectionRequest{
				Query:                    request.Query,
				Intent:                   route.Secondary,
				Answer:                   answer,
				Evidences:                evidences,
				SubQuestions:             rewrite.SubQuestions,
				IntentionalClarification: intentionalClarification,
			})
		}
	} else if reflection.Action == "clarify" && !intentionalClarification {
		answer = r.generateClarification(reflectionCtx, route, request.Query, rewrite.Entities)
		reflection.Answer = answer
	}
	answer = reflection.Answer
	if (reflection.Action == "regenerate" || reflection.Action == "remove_unsupported") &&
		len(reflection.UnsupportedClaims) > 0 {
		answer = removeUnsupportedClaims(answer, reflection.UnsupportedClaims)
		reflection.Answer = answer
		reflection.Warnings = append(reflection.Warnings, "unsupported_claims_removed")
	}
	if reflection.ShouldTransfer {
		answer += "\n\n当前结论置信度不足，建议转人工客服复核后再执行售后操作。"
	}
	reflectionSpan.SetAttributes(
		attribute.String("reflection.action", reflection.Action),
		attribute.Bool("reflection.low_confidence", reflection.LowConfidence),
		attribute.Int("reflection.unsupported_claims", len(reflection.UnsupportedClaims)),
	)
	reflectionSpan.End()
	r.appendTraceStep(ctx, request.TraceID, PlanStep{
		StepID:     "step_grounding_review",
		Action:     ActionReflect,
		ReasonCode: "final_grounding_review",
	}, reflectionStatus(reflection), reflectionStartedAt, map[string]any{
		"action":             reflection.Action,
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
	if route.Secondary == intent.CreateAfterSalesTicket {
		if strings.TrimSpace(entities["order_no"]) == "" {
			return "请提供订单号和具体问题描述。创建售后工单还需要您明确回复“确认创建售后工单”；在信息完整并确认前，我不会执行创建。"
		}
		if !ticketConfirmationPresent(query) {
			return "请核对订单号和问题描述后，明确回复“确认创建售后工单”；未确认前我不会执行创建。"
		}
	}
	missing := []string{}
	if entities["models"] == "" && needsModelForIntent(route.Secondary) {
		if route.Secondary == intent.ProductComparison {
			missing = append(missing, "比较型号")
		} else {
			missing = append(missing, "产品型号")
		}
	}
	if entities["order_no"] == "" && needsOrderForIntent(route.Secondary) {
		missing = append(missing, "订单号")
	}
	if route.Secondary == intent.Clarification && len(missing) == 0 {
		missing = append(missing, "意图")
	}
	if route.Secondary == intent.CreateAfterSalesTicket &&
		!ticketConfirmationPresent(query) {
		missing = append(missing, "用户确认")
	}
	if strings.Contains(query, "够不够用") || strings.Contains(query, "够用不") {
		missing = append(missing, "参数含义")
	}
	return r.clarifier.Clarify(ctx, query, route.Secondary, entities, missing)
}

func ticketConfirmationPresent(query string) bool {
	query = strings.TrimSpace(query)
	for _, marker := range []string{"没确认", "未确认", "没有确认", "不确认", "不要创建"} {
		if strings.Contains(query, marker) {
			return false
		}
	}
	for _, marker := range []string{"我确认", "确认创建", "确认提交", "确认给", "确认了"} {
		if strings.Contains(query, marker) {
			return true
		}
	}
	return false
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
	route.Primary = intent.PrimaryDiagnosis
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
	claims := make([]string, 0, len(unsupportedClaims))
	for _, claim := range unsupportedClaims {
		if normalized := normalizeClaimForRemoval(claim); normalized != "" {
			claims = append(claims, normalized)
		}
	}
	lines := strings.Split(strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(answer), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		normalizedLine := normalizeClaimForRemoval(line)
		remove := false
		for _, claim := range claims {
			if strings.Contains(normalizedLine, claim) ||
				(len([]rune(normalizedLine)) >= 6 && strings.Contains(claim, normalizedLine)) {
				remove = true
				break
			}
		}
		if remove {
			continue
		}
		kept = append(kept, line)
	}
	answer = strings.TrimSpace(strings.Join(kept, "\n"))
	if !hasSubstantiveContent(answer) {
		return "当前证据不足，无法形成可靠结论。请补充具体型号或由人工客服复核。"
	}
	return answer
}

func normalizeClaimForRemoval(value string) string {
	value = citationPattern.ReplaceAllString(value, "")
	value = strings.ToLower(value)
	return strings.Map(func(current rune) rune {
		if unicode.IsLetter(current) || unicode.IsNumber(current) {
			return current
		}
		return -1
	}, value)
}

func hasSubstantiveContent(value string) bool {
	for _, current := range value {
		if unicode.IsLetter(current) || unicode.IsNumber(current) {
			return true
		}
	}
	return false
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
	searchMode := rag.SearchHybrid
	needRerank := true
	if strings.EqualFold(strings.TrimSpace(fmt.Sprint(step.Params["search_mode"])), string(rag.SearchKeyword)) {
		searchMode = rag.SearchKeyword
		needRerank = false
	}
	models := splitCSV(rewrite.Entities["models"])
	categories := splitCSV(rewrite.Entities["category"])
	if boolParam(step.Params["disable_filters"]) {
		models = nil
		categories = nil
		docTypes = nil
	} else if boolParam(step.Params["disable_entity_filters"]) {
		models = nil
		categories = nil
	}

	type retrievalTask struct {
		query  string
		filter rag.MetadataFilter
		route  string
	}
	tasks := make([]retrievalTask, 0, len(queries)+1)
	for _, query := range queries {
		tasks = append(tasks, retrievalTask{
			query: query,
			filter: rag.MetadataFilter{
				Models:      models,
				Categories:  categories,
				DocTypes:    docTypes,
				EffectiveAt: &effectiveAt,
			},
			route: "precision",
		})
	}
	originalQuery := strings.TrimSpace(rewrite.Original)
	if !boolParam(step.Params["disable_original_recall"]) &&
		originalQuery != "" && !containsString(queries, originalQuery) {
		tasks = append(tasks, retrievalTask{
			query: originalQuery,
			filter: rag.MetadataFilter{
				Models:      models,
				Categories:  categories,
				EffectiveAt: &effectiveAt,
			},
			route: "recall",
		})
	}

	results := make([][]rag.SearchResult, len(tasks))
	errorsByQuery := make([]error, len(tasks))
	var waitGroup sync.WaitGroup
	for index, task := range tasks {
		index, task := index, task
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			results[index], errorsByQuery[index] = r.retriever.Search(ctx, rag.SearchRequest{
				Query:       task.query,
				Mode:        searchMode,
				Filter:      task.filter,
				DenseTopK:   r.config.DenseTopK,
				KeywordTopK: r.config.KeywordTopK,
				RerankTopK:  r.config.RerankTopK,
				MinScore:    r.config.MinDenseScore,
				NeedRerank:  needRerank,
			})
			tagRetrievalResults(results[index], task.route, 0)
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
	if failedCount == len(tasks) {
		return nil, lastErr
	}

	hopQueries := stringSliceParam(step.Params["hop_queries"])
	if len(hopQueries) > 3 {
		hopQueries = hopQueries[:3]
	}
	for index, hopQuery := range hopQueries {
		hopQuery = expandHopQuery(hopQuery, merged)
		if strings.TrimSpace(hopQuery) == "" {
			continue
		}
		hopResults, hopErr := r.retriever.Search(ctx, rag.SearchRequest{
			Query: hopQuery,
			Mode:  searchMode,
			Filter: rag.MetadataFilter{
				Models:      models,
				Categories:  categories,
				DocTypes:    docTypes,
				EffectiveAt: &effectiveAt,
			},
			DenseTopK:   r.config.DenseTopK,
			KeywordTopK: r.config.KeywordTopK,
			RerankTopK:  r.config.RerankTopK,
			MinScore:    r.config.MinDenseScore,
			NeedRerank:  needRerank,
		})
		if hopErr != nil {
			lastErr = hopErr
			continue
		}
		tagRetrievalResults(hopResults, "multi_hop", index+1)
		merged = mergeSearchResults(merged, hopResults)
	}
	if maxResults := intParam(step.Params["max_results"]); maxResults > 0 && len(merged) > maxResults {
		merged = merged[:maxResults]
	}
	return merged, nil
}

func (r *AgenticRunner) executeParallelStep(
	ctx context.Context,
	request Request,
	route intent.Result,
	rewrite RewriteResult,
	parent PlanStep,
	toolWhitelist []string,
) (DynamicExecutionResult, error) {
	type subResult struct {
		step      PlanStep
		startedAt time.Time
		result    DynamicExecutionResult
		err       error
	}
	results := make([]subResult, len(parent.SubSteps))
	var waitGroup sync.WaitGroup
	for index, configured := range parent.SubSteps {
		index, configured := index, configured
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			configured.StepID = fmt.Sprintf("%s.%02d", parent.StepID, index+1)
			results[index].step = configured
			results[index].startedAt = time.Now()
			switch configured.Action {
			case ActionRetrieve:
				items, err := r.retrievePlanStep(ctx, configured, rewrite)
				results[index].result.SearchData = items
				results[index].err = err
			case ActionCallTool, ActionRunSkill:
				if configured.Action == ActionCallTool &&
					!toolpkg.NameAllowed(toolWhitelist, configured.ToolName) {
					results[index].err = fmt.Errorf(
						"并行工具 %s 不允许用于意图 %s",
						configured.ToolName,
						route.Secondary,
					)
					return
				}
				if r.dynamicExecutor == nil {
					results[index].err = errors.New("动态执行器未配置")
					return
				}
				results[index].result, results[index].err = r.dynamicExecutor.Execute(
					ctx,
					DynamicExecutionRequest{
						Request:          request,
						Intent:           string(route.Secondary),
						SecondaryIntents: intentTypeStrings(route.SecondaryIntents),
						AllowedTools:     toolWhitelist,
						Step:             configured,
					},
				)
			case ActionReflect:
				results[index].result.Metadata = map[string]any{"reflected": true}
			default:
				results[index].err = fmt.Errorf("不支持的并行子动作 %s", configured.Action)
			}
		}()
	}
	waitGroup.Wait()

	merged := DynamicExecutionResult{
		Metadata: map[string]any{"parallel_substep_count": len(results)},
	}
	committedSideEffectAnswer := ""
	for _, current := range results {
		status := "success"
		metadata := map[string]any{
			"parent_step_id": parent.StepID,
			"reason_code":    current.step.ReasonCode,
		}
		if current.err != nil {
			status = "failed"
			metadata["error"] = current.err.Error()
		}
		r.appendTraceStep(
			ctx,
			request.TraceID,
			current.step,
			status,
			current.startedAt,
			metadata,
		)
		if current.err != nil {
			return DynamicExecutionResult{}, current.err
		}
		merged.SearchData = mergeSearchResults(merged.SearchData, current.result.SearchData)
		merged.Evidences = append(merged.Evidences, current.result.Evidences...)
		if metadataBool(current.result.Metadata, "intentional_clarification") {
			merged.Metadata["intentional_clarification"] = true
		}
		if current.result.Answer != "" {
			if current.step.Action == ActionCallTool &&
				isCommittedSideEffectTool(current.step.ToolName) {
				committedSideEffectAnswer = current.result.Answer
			}
			if current.step.Action == ActionCallTool {
				merged.Answer = removeStaleDynamicClaims(merged.Answer, current.step.ToolName)
			}
			if merged.Answer != "" {
				merged.Answer += "\n\n"
			}
			merged.Answer += current.result.Answer
			if current.step.Action == ActionRunSkill {
				merged.Evidences = append(merged.Evidences, Evidence{
					Kind:     "skill_result",
					SourceID: "skill_result:" + current.step.StepID,
					Title:    "子任务结论：" + current.step.SkillName,
					Content:  current.result.Answer,
					Metadata: map[string]any{
						"skill_name": current.step.SkillName,
						"step_id":    current.step.StepID,
					},
				})
			}
		}
	}
	if committedSideEffectAnswer != "" {
		merged.Answer = committedSideEffectAnswer
	}
	return merged, nil
}

func isCommittedSideEffectTool(toolName string) bool {
	return toolpkg.LogicalName(toolName) == "create_after_sales_ticket"
}

func removeStaleDynamicClaims(answer, toolName string) string {
	if strings.TrimSpace(answer) == "" {
		return answer
	}
	var subject string
	switch toolpkg.LogicalName(toolName) {
	case "price_query":
		subject = "价格"
	case "inventory_check":
		subject = "库存"
	default:
		return answer
	}
	lines := strings.Split(answer, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		lower := strings.ToLower(line)
		stale := strings.Contains(line, subject) &&
			(strings.Contains(line, "未收录") ||
				strings.Contains(line, "无法") ||
				strings.Contains(line, "暂未") ||
				strings.Contains(line, "官方渠道") ||
				strings.Contains(lower, "unavailable"))
		if !stale {
			filtered = append(filtered, line)
		}
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func (r *AgenticRunner) validateOrFallbackNextStep(
	ctx context.Context,
	request PlanRequest,
	candidate PlanStep,
	recent []PlanStep,
) *PlanStep {
	var validationErr error
	if validator, ok := r.planner.(NextStepValidator); ok {
		validationErr = validator.ValidateNextStep(ctx, request, candidate, recent)
	}
	if validationErr == nil {
		var similar bool
		similar, validationErr = r.isSemanticDuplicate(ctx, candidate, recent)
		if validationErr == nil && similar {
			validationErr = errors.New("下一步与最近三步语义重复度超过 0.85")
		}
	}
	if validationErr == nil {
		return &candidate
	}
	fallback, err := NewRulePlanner().Plan(ctx, request)
	if err == nil && fallback != nil {
		for _, step := range fallback.Steps {
			duplicate := false
			for _, previous := range recent {
				if actionSignature(previous) == actionSignature(step) {
					duplicate = true
					break
				}
			}
			if !duplicate && step.Action != ActionFinish {
				step.StepID = fmt.Sprintf("step_%02d", len(recent)+1)
				step.ReasonCode = "rule_fallback_after_invalid_llm_step:" + validationErr.Error()
				return &step
			}
		}
	}
	step := finishStep(len(recent)+1, "invalid_llm_step_fallback_finish")
	step.Params["validation_error"] = validationErr.Error()
	return &step
}

func (r *AgenticRunner) isSemanticDuplicate(
	ctx context.Context,
	candidate PlanStep,
	recent []PlanStep,
) (bool, error) {
	if r.stepEmbedder == nil || len(recent) == 0 {
		return false, nil
	}
	start := max(0, len(recent)-3)
	texts := []string{semanticStepText(candidate)}
	for _, step := range recent[start:] {
		texts = append(texts, semanticStepText(step))
	}
	vectors, err := r.stepEmbedder.Embed(ctx, texts)
	if err != nil {
		return false, err
	}
	if len(vectors) != len(texts) {
		return false, errors.New("步骤语义向量数量不匹配")
	}
	for _, vector := range vectors[1:] {
		if cosineSimilarity(vectors[0], vector) > 0.85 {
			return true, nil
		}
	}
	return false, nil
}

func semanticStepText(step PlanStep) string {
	raw, err := json.Marshal(step.Params)
	if err != nil {
		raw = []byte("{}")
	}
	return strings.Join([]string{
		string(step.Action), step.ToolName, step.SkillName, step.Query, string(raw),
	}, "\n")
}

func cosineSimilarity(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for index := range left {
		l := float64(left[index])
		r := float64(right[index])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func tagRetrievalResults(results []rag.SearchResult, route string, hopIndex int) {
	for index := range results {
		if results[index].Metadata == nil {
			results[index].Metadata = map[string]any{}
		}
		results[index].Metadata["retrieval_route"] = route
		if hopIndex > 0 {
			results[index].Metadata["hop_index"] = hopIndex
		}
	}
}

func expandHopQuery(query string, previous []rag.SearchResult) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	entities := extractHopEntities(previous)
	if strings.Contains(query, "{previous_entities}") {
		return strings.ReplaceAll(query, "{previous_entities}", strings.Join(entities, " "))
	}
	if len(entities) > 0 {
		return query + " " + strings.Join(entities, " ")
	}
	return query
}

var accessoryReferencePattern = regexp.MustCompile(`(?i)\b(?:F|DB|RB|C)[0-9]+\b`)

func extractHopEntities(results []rag.SearchResult) []string {
	var entities []string
	for index, result := range results {
		if index >= 5 {
			break
		}
		text := result.Title + "\n" + result.Content
		entities = append(entities, productModelPattern.FindAllString(text, -1)...)
		entities = append(entities, accessoryReferencePattern.FindAllString(text, -1)...)
	}
	return uniqueStrings(entities)
}

// buildToolResultsSummary creates a summary string from tool result evidences.
func buildToolResultsSummary(evidences []Evidence) string {
	var builder strings.Builder
	for _, ev := range evidences {
		if ev.Kind == "tool_result" || ev.Kind == "tool_error" || ev.Kind == "skill_result" {
			fmt.Fprintf(&builder, "[%s] %s: %s\n", ev.ID, ev.Title, ev.Content)
		}
	}
	if builder.Len() == 0 {
		return "(无工具调用结果)"
	}
	return builder.String()
}

func answerCoversRequestedFacets(answer, query string) bool {
	for _, facet := range requestedFacets(query) {
		if !answerCoversFacet(answer, facet) {
			return false
		}
	}
	return true
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
	for _, item := range existing {
		if _, ok := seen[item.SourceID]; ok {
			continue
		}
		seen[item.SourceID] = struct{}{}
		result = append(result, item)
	}
	for _, item := range searchResults {
		if _, ok := seen[item.ChunkID]; ok {
			continue
		}
		seen[item.ChunkID] = struct{}{}
		metadata := cloneAnyMap(item.Metadata)
		metadata["dense_score"] = item.DenseScore
		metadata["keyword_score"] = item.KeywordScore
		metadata["fusion_score"] = item.FusionScore
		metadata["rerank_score"] = item.RerankScore
		result = append(result, Evidence{
			Kind:     "kb_chunk",
			SourceID: item.ChunkID,
			Title:    item.Title,
			Content:  item.Content,
			Metadata: metadata,
		})
	}
	for index := range result {
		result[index].ID = fmt.Sprintf("E%d", index+1)
	}
	return result
}

func actionSignature(step PlanStep) string {
	raw, _ := json.Marshal(map[string]any{
		"params":    step.Params,
		"sub_steps": step.SubSteps,
	})
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

func allowedToolsForRoute(route intent.Result) []string {
	result := append([]string(nil), allowedTools(route.Secondary)...)
	for _, secondary := range route.SecondaryIntents {
		for _, toolName := range allowedTools(secondary) {
			if !containsString(result, toolName) {
				result = append(result, toolName)
			}
		}
	}
	return result
}

func intentTypeStrings(values []intent.Type) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, string(value))
		}
	}
	return result
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
	case errors.Is(err, ErrPlanDependency):
		return "PLAN_DEPENDENCY"
	default:
		return "AGENT_FAILED"
	}
}

func shouldUsePlanExecute(mode string, intentType intent.Type) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "plan_execute":
		return true
	case "auto":
		switch intentType {
		case intent.ProductComparison,
			intent.PurchaseRecommendation,
			intent.Troubleshooting,
			intent.ReturnEligibility,
			intent.WarrantyQuery,
			intent.CreateAfterSalesTicket:
			return true
		}
	}
	return false
}

func shouldUseReactivePlanning(intentType intent.Type) bool {
	switch intentType {
	case intent.AccessoryCompatibility:
		return false
	default:
		return true
	}
}

func unsatisfiedDependencies(step PlanStep, statuses map[string]string) []string {
	var missing []string
	for _, dependency := range step.DependsOn {
		if statuses[dependency] != "success" && statuses[dependency] != "warning" {
			missing = append(missing, dependency)
		}
	}
	return missing
}

func rebasePlanSteps(steps []PlanStep, startIndex int, firstDependency string) []PlanStep {
	result := append([]PlanStep(nil), steps...)
	previous := firstDependency
	for index := range result {
		result[index].StepID = fmt.Sprintf("step_%02d", startIndex+index)
		result[index].DependsOn = nil
		if previous != "" {
			result[index].DependsOn = []string{previous}
		}
		previous = result[index].StepID
	}
	return result
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

func hasToolEvidence(evidences []Evidence, toolName string) bool {
	for _, evidence := range evidences {
		if evidence.Kind != "tool_result" || evidence.Metadata == nil {
			continue
		}
		if name, _ := evidence.Metadata["tool_name"].(string); toolpkg.NamesMatch(toolName, name) {
			return true
		}
	}
	return false
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
	_ llm.Usage,
	searchResults []rag.SearchResult,
	evidences []Evidence,
) int {
	// TokenBudget limits the active context assembled for the next step. The
	// usage collector is cumulative billing telemetry across all prior LLM
	// calls, so adding it here can exhaust the budget before any skill runs.
	return currentTokenUsage(inputTokens, searchResults, evidences)
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

func boolParam(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
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
