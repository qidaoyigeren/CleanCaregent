package eval

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/platform/id"
	"CleanCaregent/internal/service"
	"CleanCaregent/internal/trace"
)

type Runner struct {
	store         Store
	evaluator     Evaluator
	conversations *service.ConversationService
	traces        trace.Store
	router        intent.Router
}

type RunRequest struct {
	UserID         string
	DatasetVersion string
	SystemVersion  string
	MaxCases       int
}

func NewRunner(
	store Store,
	evaluator Evaluator,
	conversations *service.ConversationService,
	traces trace.Store,
	router intent.Router,
) *Runner {
	return &Runner{store: store, evaluator: evaluator, conversations: conversations, traces: traces, router: router}
}

func (r *Runner) Run(ctx context.Context, request RunRequest) (Run, error) {
	run, cases, request, err := r.prepareRun(ctx, request)
	if err != nil {
		return Run{}, err
	}
	return r.executeRun(ctx, request, run, cases)
}

// Start persists the run synchronously and executes the cases in the
// background. The detached context keeps a client disconnect from cancelling
// a long LLM-backed evaluation.
func (r *Runner) Start(ctx context.Context, request RunRequest) (Run, error) {
	run, cases, request, err := r.prepareRun(ctx, request)
	if err != nil {
		return Run{}, err
	}
	go func() {
		runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Hour)
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				_ = r.store.FinishRun(
					context.Background(),
					run.RunNo,
					"failed",
					map[string]any{"error": fmt.Sprintf("panic: %v", recovered)},
					time.Now().UTC(),
				)
			}
		}()
		if _, executeErr := r.executeRun(runCtx, request, run, cases); executeErr != nil {
			_ = r.store.FinishRun(
				context.Background(),
				run.RunNo,
				"failed",
				map[string]any{"error": executeErr.Error()},
				time.Now().UTC(),
			)
		}
	}()
	return run, nil
}

func (r *Runner) prepareRun(
	ctx context.Context,
	request RunRequest,
) (Run, []Case, RunRequest, error) {
	if request.DatasetVersion == "" {
		request.DatasetVersion = "v2"
	}
	if request.SystemVersion == "" {
		request.SystemVersion = "agentic-local"
	}
	cases := DefaultCases()
	if request.MaxCases > 0 && request.MaxCases < len(cases) {
		cases = cases[:request.MaxCases]
	}
	if err := r.store.UpsertCases(ctx, request.DatasetVersion, DefaultCases()); err != nil {
		return Run{}, nil, request, fmt.Errorf("upsert eval cases: %w", err)
	}
	now := time.Now().UTC()
	run := Run{
		RunNo:          id.New("eval"),
		DatasetVersion: request.DatasetVersion,
		SystemVersion:  request.SystemVersion,
		Status:         "running",
		StartedAt:      &now,
	}
	if err := r.store.CreateRun(ctx, run); err != nil {
		return Run{}, nil, request, err
	}
	return run, cases, request, nil
}

func (r *Runner) executeRun(
	ctx context.Context,
	request RunRequest,
	run Run,
	cases []Case,
) (Run, error) {
	metricTotals := map[string]float64{}
	metricCounts := map[string]int{}
	failureTypes := map[string]int{}
	intentGroups := map[string]*groupSummary{}
	difficultyGroups := map[string]*groupSummary{}
	pathGroups := map[string]*groupSummary{}
	var latencies []int64
	totalTokens, totalSteps, passed := 0, 0, 0
	for _, evalCase := range cases {
		caseResult := r.runCase(ctx, request.UserID, evalCase)
		if err := r.store.SaveResult(ctx, run.RunNo, run.DatasetVersion, caseResult); err != nil {
			_ = r.store.FinishRun(context.WithoutCancel(ctx), run.RunNo, "failed", map[string]any{
				"error": err.Error(),
			}, time.Now().UTC())
			return Run{}, err
		}
		for _, metric := range caseResult.Metrics {
			metricTotals[metric.Name] += metric.Value
			metricCounts[metric.Name]++
		}
		if caseResult.Passed {
			passed++
		} else {
			failureTypes[caseResult.ErrorType]++
		}
		recordGroup(intentGroups, evalCase.Intent, caseResult.Passed)
		recordGroup(difficultyGroups, evalCase.Difficulty, caseResult.Passed)
		recordGroup(pathGroups, evaluationPath(evalCase.Tags), caseResult.Passed)
		latencies = append(latencies, caseResult.LatencyMS)
		totalTokens += caseResult.TokenCount
		if steps := metricValue(caseResult.Metrics, "react_steps"); steps >= 0 {
			totalSteps += int(steps)
		}
	}

	metrics := make(map[string]float64, len(metricTotals))
	for name, total := range metricTotals {
		metrics[name] = total / float64(metricCounts[name])
	}
	summary := map[string]any{
		"total_cases":         len(cases),
		"passed_cases":        passed,
		"pass_rate":           ratio(passed, len(cases)),
		"metrics":             metrics,
		"failure_types":       failureTypes,
		"p95_latency_ms":      percentile95(latencies),
		"average_tokens":      ratio(totalTokens, len(cases)),
		"average_react_steps": ratio(totalSteps, len(cases)),
		"dataset_full_size":   len(DefaultCases()),
		"by_intent":           renderGroups(intentGroups),
		"by_difficulty":       renderGroups(difficultyGroups),
		"by_path":             renderGroups(pathGroups),
	}
	finishedAt := time.Now().UTC()
	if err := r.store.FinishRun(ctx, run.RunNo, "completed", summary, finishedAt); err != nil {
		return Run{}, err
	}
	run.Status = "completed"
	run.FinishedAt = &finishedAt
	run.Summary = summary
	return run, nil
}

type groupSummary struct {
	Total  int
	Passed int
}

func recordGroup(groups map[string]*groupSummary, key string, passed bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "unknown"
	}
	if groups[key] == nil {
		groups[key] = &groupSummary{}
	}
	groups[key].Total++
	if passed {
		groups[key].Passed++
	}
}

func renderGroups(groups map[string]*groupSummary) map[string]map[string]any {
	rendered := make(map[string]map[string]any, len(groups))
	for key, group := range groups {
		rendered[key] = map[string]any{
			"total":     group.Total,
			"passed":    group.Passed,
			"pass_rate": ratio(group.Passed, group.Total),
		}
	}
	return rendered
}

func evaluationPath(tags []string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "eval_group:") {
			return strings.TrimPrefix(tag, "eval_group:")
		}
	}
	return "unknown"
}

func (r *Runner) runCase(ctx context.Context, userID string, evalCase Case) CaseResult {
	startedAt := time.Now()
	conversation, err := r.conversations.Create(ctx, userID, "Eval "+evalCase.CaseID)
	if err != nil {
		return failedCase(evalCase, "", "create_conversation", startedAt, err)
	}
	askResult, err := r.conversations.Ask(ctx, userID, conversation.ID, evalCase.Query, nil)
	if err != nil {
		return failedCase(evalCase, "", "agent_execution", startedAt, err)
	}
	traceRecord, err := r.traces.Get(ctx, askResult.Message.TraceID)
	if err != nil {
		return r.runCaseWithoutTrace(ctx, evalCase, askResult, startedAt)
	}
	output := AgentOutput{
		Intent:      traceRecord.Intent,
		Answer:      askResult.Result.Answer,
		EvidenceIDs: traceRecord.EvidenceIDs,
		LatencyMS:   traceRecord.LatencyMS,
		TokenCount:  traceRecord.InputTokens + traceRecord.OutputTokens,
		StepCount:   len(traceRecord.Steps),
		ToolParams:  map[string]any{},
	}
	for _, evidence := range askResult.Result.Evidences {
		if evidence.Kind != "kb_chunk" {
			continue
		}
		output.Documents = append(output.Documents, documentID(evidence.SourceID))
		output.Contexts = append(output.Contexts, evidence.Content)
	}
	for _, call := range traceRecord.ToolCalls {
		output.Tools = append(output.Tools, call.ToolName)
		output.ToolParams[call.ToolName] = call.Arguments
		output.ToolResults = append(output.ToolResults, call.ResultSummary)
		if call.Status != "failed" && call.ErrorCode == "" {
			output.SuccessfulToolCalls++
		}
	}
	for _, step := range traceRecord.Steps {
		switch {
		case strings.HasPrefix(step.StepID, "step_reflection_recovery_"):
			output.ReflectionAttempts++
			if step.Status != "failed" && step.Status != "blocked" {
				output.ReflectionSucceeded = true
			}
		case strings.HasPrefix(step.StepID, "revision_"):
			output.PlanRevisions++
			if step.Status != "failed" && step.Status != "blocked" {
				output.ReflectionSucceeded = true
			}
		case step.StepID == "step_grounding_review":
			action, _ := step.Metadata["action"].(string)
			if action != "" && action != "accept" {
				output.ReflectionAttempts++
				if step.Status != "failed" && step.Status != "blocked" {
					output.ReflectionSucceeded = true
				}
			}
		}
	}
	metrics, err := r.evaluator.Evaluate(ctx, evalCase, output)
	if err != nil {
		return failedCase(evalCase, askResult.Message.TraceID, "evaluation", startedAt, err)
	}
	metrics = append(metrics,
		MetricResult{Name: "latency_ms", Value: float64(output.LatencyMS), Pass: output.LatencyMS < 5000},
		MetricResult{Name: "token_count", Value: float64(output.TokenCount), Pass: output.TokenCount <= 6000},
		MetricResult{Name: "react_steps", Value: float64(output.StepCount), Pass: output.StepCount <= 6},
	)
	passed := true
	errorType := ""
	for _, metric := range metrics {
		if metric.Name == "latency_ms" || metric.Name == "token_count" || metric.Name == "react_steps" {
			continue
		}
		if !metric.Pass {
			passed = false
			if errorType == "" {
				errorType = classifyBadCaseWithTrace(metric.Name, traceRecord)
			}
		}
	}
	return CaseResult{
		CaseID:       evalCase.CaseID,
		TraceID:      askResult.Message.TraceID,
		ActualIntent: output.Intent,
		ActualTools:  output.Tools,
		Answer:       output.Answer,
		Metrics:      metrics,
		Passed:       passed,
		ErrorType:    errorType,
		LatencyMS:    output.LatencyMS,
		TokenCount:   output.TokenCount,
	}
}

func (r *Runner) runCaseWithoutTrace(
	ctx context.Context,
	evalCase Case,
	askResult service.AskResult,
	startedAt time.Time,
) CaseResult {
	route, err := r.router.Route(ctx, intent.RouteRequest{Query: evalCase.Query})
	if err != nil {
		return failedCase(evalCase, "", "intent_route", startedAt, err)
	}
	output := AgentOutput{
		Intent:     string(route.Secondary),
		Answer:     askResult.Result.Answer,
		ToolParams: map[string]any{},
		LatencyMS:  time.Since(startedAt).Milliseconds(),
		TokenCount: estimateTokens(evalCase.Query + askResult.Result.Answer),
		StepCount:  1,
	}
	for _, evidence := range askResult.Result.Evidences {
		output.EvidenceIDs = append(output.EvidenceIDs, evidence.ID)
		if evidence.Kind == "kb_chunk" {
			output.Documents = append(output.Documents, documentID(evidence.SourceID))
			output.Contexts = append(output.Contexts, evidence.Content)
		}
	}
	metrics, err := r.evaluator.Evaluate(ctx, evalCase, output)
	if err != nil {
		return failedCase(evalCase, "", "evaluation", startedAt, err)
	}
	metrics = append(metrics,
		MetricResult{Name: "latency_ms", Value: float64(output.LatencyMS), Pass: output.LatencyMS < 5000},
		MetricResult{Name: "token_count", Value: float64(output.TokenCount), Pass: output.TokenCount <= 6000},
		MetricResult{Name: "react_steps", Value: 1, Pass: true},
	)
	passed := true
	errorType := ""
	for _, metric := range metrics {
		if metric.Name == "latency_ms" || metric.Name == "token_count" || metric.Name == "react_steps" {
			continue
		}
		if !metric.Pass {
			passed = false
			if errorType == "" {
				errorType = classifyBadCase(metric.Name)
			}
		}
	}
	return CaseResult{
		CaseID:       evalCase.CaseID,
		ActualIntent: output.Intent,
		Answer:       output.Answer,
		Metrics:      metrics,
		Passed:       passed,
		ErrorType:    errorType,
		LatencyMS:    output.LatencyMS,
		TokenCount:   output.TokenCount,
	}
}

func failedCase(evalCase Case, traceID, errorType string, startedAt time.Time, err error) CaseResult {
	return CaseResult{
		CaseID:    evalCase.CaseID,
		TraceID:   traceID,
		Answer:    err.Error(),
		Passed:    false,
		ErrorType: errorType,
		LatencyMS: time.Since(startedAt).Milliseconds(),
	}
}

func documentID(chunkID string) string {
	if index := strings.Index(chunkID, ":"); index > 0 {
		return chunkID[:index]
	}
	return chunkID
}

func percentile95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (95*len(sorted) + 99) / 100
	if index < 1 {
		index = 1
	}
	return sorted[index-1]
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func metricValue(metrics []MetricResult, name string) float64 {
	for _, metric := range metrics {
		if metric.Name == name {
			return metric.Value
		}
	}
	return -1
}

func classifyBadCase(metricName string) string {
	switch metricName {
	case "intent_accuracy":
		return "intent_error"
	case "hit_at_5", "mrr", "context_recall":
		return "retrieval_miss"
	case "context_precision":
		return "retrieval_noise"
	case "tool_decision_accuracy", "tool_selection_accuracy":
		return "tool_selection_error"
	case "tool_parameter_accuracy":
		return "tool_parameter_error"
	case "answer_faithfulness":
		return "hallucination_or_ungrounded"
	case "answer_grounding_rate":
		return "hallucination_or_ungrounded"
	case "answer_correctness", "multi_step_completion", "multi_step_completion_rate":
		return "answer_incomplete_or_incorrect"
	case "self_correction_success_rate":
		return "self_correction_failed"
	case "clarify_reject_accuracy", "clarify_accuracy", "reject_accuracy",
		"false_rejection_rate", "false_acceptance_rate":
		return "clarification_or_rejection_error"
	case "safety_compliance", "safety_violation_rate":
		return "safety_violation"
	case "tool_result_utilization":
		return "tool_result_not_used"
	case "efficiency_score":
		return "inefficient_execution"
	default:
		return "metric:" + metricName
	}
}

func classifyBadCaseWithTrace(metricName string, record trace.AgentTraceRecord) string {
	for _, call := range record.ToolCalls {
		if call.Status != "failed" && call.ErrorCode == "" {
			continue
		}
		code := strings.ToUpper(call.ErrorCode)
		if strings.Contains(code, "ARGUMENT") ||
			strings.Contains(code, "PARAM") ||
			strings.Contains(code, "INVALID_TOOL_RESULT") {
			return "tool_parameter_error"
		}
		return "tool_selection_error"
	}
	for _, step := range record.Steps {
		if step.Status != "failed" && step.Status != "blocked" {
			continue
		}
		switch strings.ToLower(step.Type) {
		case "retrieve":
			return "retrieval_miss"
		case "tool", "run_tool", "run_skill":
			return "tool_selection_error"
		case "clarify":
			return "clarification_or_rejection_error"
		case "generate", "answer_direct", "finish":
			return "answer_incomplete_or_incorrect"
		}
	}
	code := strings.ToUpper(record.ErrorCode)
	switch {
	case strings.Contains(code, "RETRIEV"):
		return "retrieval_miss"
	case strings.Contains(code, "TOOL"):
		return "tool_selection_error"
	case strings.Contains(code, "INTENT"):
		return "intent_error"
	}
	return classifyBadCase(metricName)
}

func estimateTokens(value string) int {
	count := len([]rune(value))
	if count == 0 {
		return 0
	}
	return (count + 2) / 3
}
