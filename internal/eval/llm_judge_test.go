package eval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/prompt"
)

func TestCompositeEvaluatorOverridesSemanticMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{
				map[string]any{"message": map[string]any{
					"content": `{"answer_faithfulness":0.91,"answer_correctness":0.83,"reason":"grounded"}`,
				}},
			},
		})
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "", "judge", 200, 0, time.Second)
	evaluator := NewCompositeEvaluator(
		NewRuleEvaluator(),
		NewLLMJudgeEvaluator(client, prompt.NewRegistry()),
	)
	metrics, err := evaluator.Evaluate(context.Background(), Case{
		Query:          "T20吸力多大",
		Intent:         "product_parameter",
		StandardAnswer: "6000Pa",
	}, AgentOutput{
		Intent:    "product_parameter",
		Answer:    "T20吸力为6000Pa。[E1]",
		Contexts:  []string{"T20额定吸力6000Pa"},
		Documents: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if value := metricValue(metrics, "answer_faithfulness"); value != 0.91 {
		t.Fatalf("faithfulness = %v", value)
	}
	if value := metricValue(metrics, "answer_correctness"); value != 0.83 {
		t.Fatalf("correctness = %v", value)
	}
}

func TestClassifyBadCase(t *testing.T) {
	if got := classifyBadCase("tool_parameter_accuracy"); got != "tool_parameter_error" {
		t.Fatalf("classification = %q", got)
	}
}
