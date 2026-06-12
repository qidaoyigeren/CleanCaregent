package prompt

import (
	"context"
	"fmt"

	"CleanCaregent/internal/llm"
)

type EvaluationCase struct {
	ID       string `json:"case_id"`
	Query    string `json:"query"`
	Expected string `json:"expected"`
}

type VersionScore struct {
	Version      string            `json:"version"`
	CaseCount    int               `json:"case_count"`
	Faithfulness float64           `json:"faithfulness"`
	Correctness  float64           `json:"correctness"`
	Outputs      map[string]string `json:"outputs,omitempty"`
}

type VersionComparison struct {
	Scenario Scenario     `json:"scenario"`
	A        VersionScore `json:"version_a"`
	B        VersionScore `json:"version_b"`
	Winner   string       `json:"winner"`
}

type VersionEvaluator interface {
	Evaluate(
		ctx context.Context,
		template *Template,
		cases []EvaluationCase,
	) (VersionScore, error)
}

// CompareVersions evaluates two registered versions against the same cases.
func (r *Registry) CompareVersions(
	ctx context.Context,
	scenario Scenario,
	versionA string,
	versionB string,
	cases []EvaluationCase,
	evaluator VersionEvaluator,
) (VersionComparison, error) {
	if evaluator == nil {
		return VersionComparison{}, fmt.Errorf("Prompt 版本评估器未配置")
	}
	templateA, err := r.GetVersion(scenario, versionA)
	if err != nil {
		return VersionComparison{}, err
	}
	templateB, err := r.GetVersion(scenario, versionB)
	if err != nil {
		return VersionComparison{}, err
	}
	scoreA, err := evaluator.Evaluate(ctx, templateA, cases)
	if err != nil {
		return VersionComparison{}, fmt.Errorf("评估 Prompt 版本 %s 失败: %w", versionA, err)
	}
	scoreB, err := evaluator.Evaluate(ctx, templateB, cases)
	if err != nil {
		return VersionComparison{}, fmt.Errorf("评估 Prompt 版本 %s 失败: %w", versionB, err)
	}
	winner := "tie"
	totalA := scoreA.Faithfulness + scoreA.Correctness
	totalB := scoreB.Faithfulness + scoreB.Correctness
	if totalA > totalB {
		winner = versionA
	} else if totalB > totalA {
		winner = versionB
	}
	return VersionComparison{Scenario: scenario, A: scoreA, B: scoreB, Winner: winner}, nil
}

type LLMVersionEvaluator struct {
	client *llm.Client
}

// NewLLMVersionEvaluator creates a semantic Prompt version evaluator.
func NewLLMVersionEvaluator(client *llm.Client) *LLMVersionEvaluator {
	return &LLMVersionEvaluator{client: client}
}

func (e *LLMVersionEvaluator) Evaluate(
	ctx context.Context,
	template *Template,
	cases []EvaluationCase,
) (VersionScore, error) {
	if e == nil || e.client == nil {
		return VersionScore{}, fmt.Errorf("LLM Prompt 评估器未配置")
	}
	score := VersionScore{
		Version: template.Version, CaseCount: len(cases),
		Outputs: make(map[string]string, len(cases)),
	}
	for _, evalCase := range cases {
		params := commonEvaluationParams(evalCase)
		answer, err := e.client.Chat(ctx, template.BuildMessages(params))
		if err != nil {
			return VersionScore{}, err
		}
		var judged struct {
			Faithfulness float64 `json:"faithfulness"`
			Correctness  float64 `json:"correctness"`
		}
		err = e.client.ChatJSON(ctx, []map[string]string{
			{
				"role": "system",
				"content": `你是 Prompt A/B 评估器。根据参考事实评估答案，只输出 JSON：
{"faithfulness":0.0,"correctness":0.0}
两个分数范围均为 0 到 1。faithfulness 衡量答案是否只陈述参考事实支持的内容，correctness 衡量是否回答了问题。`,
			},
			{
				"role": "user",
				"content": fmt.Sprintf(
					"问题：%s\n参考事实：%s\n待评答案：%s",
					evalCase.Query,
					evalCase.Expected,
					answer,
				),
			},
		}, &judged)
		if err != nil {
			return VersionScore{}, err
		}
		score.Faithfulness += clampScore(judged.Faithfulness)
		score.Correctness += clampScore(judged.Correctness)
		score.Outputs[evalCase.ID] = answer
	}
	if len(cases) > 0 {
		score.Faithfulness /= float64(len(cases))
		score.Correctness /= float64(len(cases))
	}
	return score, nil
}

func commonEvaluationParams(evalCase EvaluationCase) map[string]string {
	return map[string]string{
		"query": evalCase.Query, "original_query": evalCase.Query,
		"rewritten_query": evalCase.Query, "evidence_context": evalCase.Expected,
		"context": evalCase.Expected, "draft_answer": "",
		"conversation_summary": "", "known_info": "", "missing_info": "",
		"sub_questions": "[]", "tool_calls": "[]", "tool_results": "",
		"intent_type": "", "intent_info": "{}", "model": "",
		"step_info": "0/1", "max_steps": "1", "tool_definitions": "[]",
		"evidence_summary": evalCase.Expected, "messages": evalCase.Query,
		"previous_summary": "",
	}
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
