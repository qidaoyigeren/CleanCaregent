package observability

import (
	"testing"
	"time"
)

func TestAgentMetricsTracksTokensFailuresAndP95(t *testing.T) {
	metrics := NewAgentMetrics(100)
	for index := 1; index <= 100; index++ {
		metrics.Record(time.Duration(index)*time.Millisecond, index, 1, index == 100)
	}
	snapshot := metrics.Snapshot()
	if snapshot.RequestCount != 100 || snapshot.FailureCount != 1 {
		t.Fatalf("counts = %#v", snapshot)
	}
	if snapshot.PromptTokens != 5050 || snapshot.CompletionTokens != 100 {
		t.Fatalf("tokens = %#v", snapshot)
	}
	if snapshot.P95LatencyMS != 95 {
		t.Fatalf("p95 latency = %d", snapshot.P95LatencyMS)
	}
}
