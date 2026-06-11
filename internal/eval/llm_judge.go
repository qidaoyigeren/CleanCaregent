package eval

import (
	"context"
	"encoding/json"
	"fmt"

	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/prompt"
)

type CompositeEvaluator struct {
	base  Evaluator
	judge Evaluator
}

func NewCompositeEvaluator(base, judge Evaluator) *CompositeEvaluator {
	return &CompositeEvaluator{base: base, judge: judge}
}

func (e *CompositeEvaluator) Evaluate(
	ctx context.Context,
	evalCase Case,
	output AgentOutput,
) ([]MetricResult, error) {
	baseMetrics, err := e.base.Evaluate(ctx, evalCase, output)
	if err != nil || e.judge == nil {
		return baseMetrics, err
	}
	judgeMetrics, judgeErr := e.judge.Evaluate(ctx, evalCase, output)
	if judgeErr != nil {
		return baseMetrics, nil
	}
	return replaceMetrics(baseMetrics, judgeMetrics), nil
}

type LLMJudgeEvaluator struct {
	client  *llm.Client
	prompts *prompt.Registry
}

func NewLLMJudgeEvaluator(
	client *llm.Client,
	prompts *prompt.Registry,
) *LLMJudgeEvaluator {
	return &LLMJudgeEvaluator{client: client, prompts: prompts}
}

func (e *LLMJudgeEvaluator) Evaluate(
	ctx context.Context,
	evalCase Case,
	output AgentOutput,
) ([]MetricResult, error) {
	if e.client == nil || e.prompts == nil {
		return nil, fmt.Errorf("llm judge is not configured")
	}
	template, err := e.prompts.Get(prompt.ScenarioEvalJudge)
	if err != nil {
		return nil, err
	}
	contexts, _ := json.Marshal(output.Contexts)
	var result struct {
		Faithfulness float64 `json:"answer_faithfulness"`
		Correctness  float64 `json:"answer_correctness"`
	}
	if err := e.client.ChatJSON(ctx, template.BuildMessages(map[string]string{
		"query":           evalCase.Query,
		"standard_answer": evalCase.StandardAnswer,
		"contexts":        string(contexts),
		"actual_answer":   output.Answer,
	}), &result); err != nil {
		return nil, err
	}
	result.Faithfulness = clampScore(result.Faithfulness)
	result.Correctness = clampScore(result.Correctness)
	return []MetricResult{
		{
			Name:  "answer_faithfulness",
			Value: result.Faithfulness,
			Pass:  result.Faithfulness >= 0.8,
		},
		{
			Name:  "answer_correctness",
			Value: result.Correctness,
			Pass:  result.Correctness >= 0.7,
		},
	}, nil
}

func replaceMetrics(base, overrides []MetricResult) []MetricResult {
	byName := make(map[string]MetricResult, len(overrides))
	for _, metric := range overrides {
		byName[metric.Name] = metric
	}
	result := make([]MetricResult, 0, len(base)+len(overrides))
	for _, metric := range base {
		if override, ok := byName[metric.Name]; ok {
			result = append(result, override)
			delete(byName, metric.Name)
			continue
		}
		result = append(result, metric)
	}
	for _, metric := range byName {
		result = append(result, metric)
	}
	return result
}

func clampScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
