package agent

import (
	"context"
	"strings"
	"testing"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/model"
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

func TestRuleRewriteCarriesRecommendationContextIntoSearchQueries(t *testing.T) {
	rewriter := NewRuleQueryRewriter()
	result, err := rewriter.Rewrite(context.Background(), RewriteRequest{
		Query: "扫地机器人",
		Intent: intent.Result{
			Secondary:  intent.PurchaseRecommendation,
			Confidence: 0.88,
			Entities:   map[string]string{"category": "robot_vacuum"},
		},
		RecentMessages: []model.Message{
			{Role: "user", Content: "家庭地面，100平"},
			{Role: "user", Content: "预算5000"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"category": "robot_vacuum",
		"area":     "100平",
		"budget":   "5000",
	} {
		if got := result.Entities[key]; got != want {
			t.Fatalf("%s = %q, want %q; entities = %#v", key, got, want, result.Entities)
		}
	}
	for _, want := range []string{"扫地机器人", "100平", "5000"} {
		if !strings.Contains(result.Rewritten, want) {
			t.Fatalf("rewritten query %q missing %q", result.Rewritten, want)
		}
	}
	if len(result.SearchQueries) == 0 || !strings.Contains(result.SearchQueries[0], "100平") {
		t.Fatalf("search queries did not preserve carried constraints: %#v", result.SearchQueries)
	}
}
