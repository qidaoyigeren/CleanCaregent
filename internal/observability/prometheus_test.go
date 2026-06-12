package observability

import (
	"strings"
	"testing"
	"time"
)

func TestPrometheusMetricsRender(t *testing.T) {
	metrics := NewPrometheusMetrics()
	metrics.RecordRequest("product_parameter", "success", 250*time.Millisecond, 100, 20, 3, 0.001)
	metrics.RecordToolDuration("price_query", "success", 25*time.Millisecond)
	metrics.RecordRetrieval(40 * time.Millisecond)
	metrics.RecordFallback("planner", "timeout")
	output := metrics.Render()
	for _, expected := range []string{
		`cleancare_requests_total{intent="product_parameter",status="success"} 1`,
		`cleancare_tool_calls_total{tool="price_query",status="success"} 1`,
		`cleancare_tokens_consumed{type="prompt"} 100`,
		`cleancare_tokens_by_intent_total{intent="product_parameter",type="prompt"} 100`,
		`cleancare_tool_duration_seconds_count{tool="price_query"} 1`,
		`cleancare_request_duration_by_intent_seconds_count{intent="product_parameter"} 1`,
		`cleancare_fallback_total{stage="planner",reason="timeout"} 1`,
		`cleancare_llm_cost_usd_total 0.001000000`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q in:\n%s", expected, output)
		}
	}
}
