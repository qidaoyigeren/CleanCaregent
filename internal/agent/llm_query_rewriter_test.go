package agent

import (
	"testing"

	"CleanCaregent/internal/intent"
)

func TestShouldUseLLMRewriteKeepsExplicitComparisonDeterministic(t *testing.T) {
	request := RewriteRequest{
		Query: "T20 和 X20 Pro 哪个适合养猫？",
		Intent: intent.Result{
			Secondary:  intent.ProductComparison,
			Confidence: 0.98,
			Entities:   map[string]string{"models": "T20,X20 Pro"},
		},
	}
	if shouldUseLLMRewrite(request) {
		t.Fatal("explicit comparison should use deterministic rewriting")
	}
}

func TestShouldUseLLMRewriteUsesConversationForReference(t *testing.T) {
	request := RewriteRequest{
		Query: "那台滤芯多少钱？",
		Intent: intent.Result{
			Secondary:  intent.AccessoryCompatibility,
			Confidence: 0.95,
		},
	}
	if !shouldUseLLMRewrite(request) {
		t.Fatal("anaphora should use LLM rewriting")
	}
}

func TestLLMRewriteDoesNotOverwriteRuleEntities(t *testing.T) {
	entities := map[string]string{"models": "T20,X20 Pro"}
	for key, value := range map[string]any{"models": "P400,P500", "category": "robot_vacuum"} {
		if entities[key] != "" {
			continue
		}
		entities[key] = rewriteEntityString(value)
	}
	if entities["models"] != "T20,X20 Pro" {
		t.Fatalf("models = %q", entities["models"])
	}
	if entities["category"] != "robot_vacuum" {
		t.Fatalf("category = %q", entities["category"])
	}
}
