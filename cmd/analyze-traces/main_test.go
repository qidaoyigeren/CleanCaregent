package main

import (
	"strings"
	"testing"

	"CleanCaregent/internal/trace"
)

func TestFormatTraceHighlightsAbnormalStep(t *testing.T) {
	output := formatTrace(trace.AgentTraceRecord{
		AgentTrace: trace.AgentTrace{TraceID: "tr_1", Intent: "price_query", RouteMode: "react"},
		Status:     "failed",
		Steps: []trace.Step{{
			Type: "retrieve", Status: "failed", Metadata: map[string]any{"error": "timeout"},
		}},
	})
	if !strings.Contains(output, "[异常]") || !strings.Contains(output, "timeout") {
		t.Fatalf("output = %s", output)
	}
}
