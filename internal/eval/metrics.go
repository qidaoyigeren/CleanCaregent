package eval

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode"
)

type RuleEvaluator struct{}

func NewRuleEvaluator() *RuleEvaluator {
	return &RuleEvaluator{}
}

func (e *RuleEvaluator) Evaluate(_ context.Context, evalCase Case, output AgentOutput) ([]MetricResult, error) {
	expectedDocs := normalizeSet(evalCase.ExpectedDocuments)
	actualDocs := normalizeSet(output.Documents)
	expectedTools := normalizeSet(evalCase.ExpectedTools)
	actualTools := normalizeSet(output.Tools)

	hitAt5 := hitAtK(evalCase.ExpectedDocuments, output.Documents, 5)
	mrr := reciprocalRank(evalCase.ExpectedDocuments, output.Documents)
	contextRecall := setRecall(expectedDocs, actualDocs)
	contextPrecision := setPrecision(expectedDocs, actualDocs)
	toolSelection := setExact(expectedTools, actualTools)
	toolDecision := (len(expectedTools) == 0) == (len(actualTools) == 0)
	paramAccuracy := parameterAccuracy(evalCase.ExpectedToolParams, output.ToolParams)
	faithfulness := 1.0
	if needsEvidence(evalCase) && len(output.EvidenceIDs) == 0 {
		faithfulness = 0
	}
	correctness := answerSimilarity(evalCase.StandardAnswer, output.Answer)
	multiStep := contextRecall == 1 && toolSelection
	if len(expectedDocs) == 0 {
		multiStep = toolSelection
	}
	clarifyCorrect := !evalCase.ShouldClarify || containsAny(output.Answer, "请补充", "请提供", "需要具体")
	rejectCorrect := !evalCase.ShouldReject || containsAny(output.Answer, "只支持", "超出", "不提供")

	return []MetricResult{
		{Name: "intent_accuracy", Value: boolValue(output.Intent == evalCase.Intent), Pass: output.Intent == evalCase.Intent},
		{Name: "hit_at_5", Value: hitAt5, Pass: hitAt5 >= 1},
		{Name: "mrr", Value: mrr, Pass: len(expectedDocs) == 0 || mrr > 0},
		{Name: "context_recall", Value: contextRecall, Pass: contextRecall >= 0.8},
		{Name: "context_precision", Value: contextPrecision, Pass: len(expectedDocs) == 0 || contextPrecision >= 0.2},
		{Name: "tool_decision_accuracy", Value: boolValue(toolDecision), Pass: toolDecision},
		{Name: "tool_selection_accuracy", Value: boolValue(toolSelection), Pass: toolSelection},
		{Name: "tool_parameter_accuracy", Value: paramAccuracy, Pass: paramAccuracy >= 0.8},
		{Name: "answer_faithfulness", Value: faithfulness, Pass: faithfulness >= 1},
		{Name: "answer_correctness", Value: correctness, Pass: correctness >= 0.15},
		{Name: "multi_step_completion", Value: boolValue(multiStep), Pass: multiStep},
		{Name: "clarify_reject_accuracy", Value: boolValue(clarifyCorrect && rejectCorrect), Pass: clarifyCorrect && rejectCorrect},
	}, nil
}

func hitAtK(expected, actual []string, k int) float64 {
	if len(expected) == 0 {
		return 1
	}
	expectedSet := normalizeSet(expected)
	if len(actual) > k {
		actual = actual[:k]
	}
	for _, item := range actual {
		if matchesExpected(item, expectedSet) {
			return 1
		}
	}
	return 0
}

func reciprocalRank(expected, actual []string) float64 {
	if len(expected) == 0 {
		return 1
	}
	expectedSet := normalizeSet(expected)
	for index, item := range actual {
		if matchesExpected(item, expectedSet) {
			return 1 / float64(index+1)
		}
	}
	return 0
}

func setRecall(expected, actual map[string]struct{}) float64 {
	if len(expected) == 0 {
		return 1
	}
	hits := 0
	for item := range expected {
		if matchesExpected(item, actual) {
			hits++
		}
	}
	return float64(hits) / float64(len(expected))
}

func setPrecision(expected, actual map[string]struct{}) float64 {
	if len(expected) == 0 {
		if len(actual) == 0 {
			return 1
		}
		return 0
	}
	if len(actual) == 0 {
		return 0
	}
	hits := 0
	for item := range actual {
		if matchesExpected(item, expected) {
			hits++
		}
	}
	return float64(hits) / float64(len(actual))
}

func setExact(expected, actual map[string]struct{}) bool {
	if len(expected) != len(actual) {
		return false
	}
	for item := range expected {
		if _, ok := actual[item]; !ok {
			return false
		}
	}
	return true
}

func parameterAccuracy(expected, actual map[string]any) float64 {
	if len(expected) == 0 {
		return 1
	}
	raw, _ := json.Marshal(actual)
	actualText := strings.ToLower(string(raw))
	hits := 0
	for key, value := range expected {
		valueRaw, _ := json.Marshal(value)
		if strings.Contains(actualText, strings.ToLower(key)) &&
			containsJSONValue(actualText, strings.ToLower(string(valueRaw))) {
			hits++
		}
	}
	return float64(hits) / float64(len(expected))
}

func containsJSONValue(actual, expected string) bool {
	if strings.Contains(actual, expected) {
		return true
	}
	expected = strings.Trim(expected, "[]")
	for _, part := range strings.Split(expected, ",") {
		if !strings.Contains(actual, strings.TrimSpace(part)) {
			return false
		}
	}
	return true
}

func answerSimilarity(expected, actual string) float64 {
	if strings.TrimSpace(expected) == "" {
		return 1
	}
	expectedTokens := tokenSet(expected)
	actualTokens := tokenSet(actual)
	if len(expectedTokens) == 0 {
		return boolValue(strings.Contains(strings.ToLower(actual), strings.ToLower(expected)))
	}
	hits := 0
	for token := range expectedTokens {
		if _, ok := actualTokens[token]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(expectedTokens))
}

func tokenSet(value string) map[string]struct{} {
	value = strings.ToLower(value)
	var runes []rune
	for _, current := range value {
		if unicode.IsLetter(current) || unicode.IsNumber(current) {
			runes = append(runes, current)
		}
	}
	result := map[string]struct{}{}
	for index := 0; index < len(runes); index++ {
		end := min(index+2, len(runes))
		result[string(runes[index:end])] = struct{}{}
	}
	return result
}

func normalizeSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func matchesExpected(item string, expected map[string]struct{}) bool {
	item = strings.ToLower(strings.TrimSpace(item))
	if _, ok := expected[item]; ok {
		return true
	}
	keys := make([]string, 0, len(expected))
	for key := range expected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.HasPrefix(item, key+":") || strings.HasPrefix(item, key) || strings.HasPrefix(key, item) {
			return true
		}
	}
	return false
}

func needsEvidence(item Case) bool {
	return !item.ShouldClarify && !item.ShouldReject && item.Intent != "chitchat"
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func boolValue(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
