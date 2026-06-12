package llm

import "testing"

func TestEstimateCostUSDAndCollector(t *testing.T) {
	cost := EstimateCostUSD("qwen-plus", 1_000_000, 1_000_000)
	if cost != 0.39 {
		t.Fatalf("cost = %f, want 0.39", cost)
	}
	collector := &UsageCollector{}
	collector.Add(Usage{
		PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
		Model: "qwen-plus", CostUSD: 0.001,
	})
	snapshot := collector.Snapshot()
	if snapshot.CostUSD != 0.001 || snapshot.Calls != 1 || snapshot.Model != "qwen-plus" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}
