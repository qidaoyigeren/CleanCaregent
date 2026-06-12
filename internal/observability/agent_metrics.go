package observability

import (
	"sort"
	"sync"
	"time"
)

const defaultLatencyWindow = 2048

type AgentMetrics struct {
	mu                sync.RWMutex
	maxLatencySamples int
	latencies         []int64
	nextLatency       int
	requestCount      uint64
	failureCount      uint64
	promptTokens      uint64
	completionTokens  uint64
	costUSD           float64
}

type AgentMetricsSnapshot struct {
	RequestCount     uint64  `json:"request_count"`
	FailureCount     uint64  `json:"failure_count"`
	PromptTokens     uint64  `json:"prompt_tokens"`
	CompletionTokens uint64  `json:"completion_tokens"`
	TotalTokens      uint64  `json:"total_tokens"`
	P95LatencyMS     int64   `json:"p95_latency_ms"`
	LatencySamples   int     `json:"latency_samples"`
	CostUSD          float64 `json:"cost_usd"`
}

var DefaultAgentMetrics = NewAgentMetrics(defaultLatencyWindow)

func NewAgentMetrics(maxLatencySamples int) *AgentMetrics {
	if maxLatencySamples <= 0 {
		maxLatencySamples = defaultLatencyWindow
	}
	return &AgentMetrics{maxLatencySamples: maxLatencySamples}
}

func (m *AgentMetrics) Record(
	latency time.Duration,
	promptTokens, completionTokens int,
	failed bool,
) AgentMetricsSnapshot {
	return m.RecordWithCost(latency, promptTokens, completionTokens, 0, failed)
}

// RecordWithCost records latency, tokens, failure state, and estimated model cost.
func (m *AgentMetrics) RecordWithCost(
	latency time.Duration,
	promptTokens, completionTokens int,
	costUSD float64,
	failed bool,
) AgentMetricsSnapshot {
	if m == nil {
		return AgentMetricsSnapshot{}
	}
	m.mu.Lock()
	m.requestCount++
	if failed {
		m.failureCount++
	}
	if promptTokens > 0 {
		m.promptTokens += uint64(promptTokens)
	}
	if completionTokens > 0 {
		m.completionTokens += uint64(completionTokens)
	}
	if costUSD > 0 {
		m.costUSD += costUSD
	}
	latencyMS := latency.Milliseconds()
	if len(m.latencies) < m.maxLatencySamples {
		m.latencies = append(m.latencies, latencyMS)
	} else {
		m.latencies[m.nextLatency] = latencyMS
		m.nextLatency = (m.nextLatency + 1) % m.maxLatencySamples
	}
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	return snapshot
}

func (m *AgentMetrics) Snapshot() AgentMetricsSnapshot {
	if m == nil {
		return AgentMetricsSnapshot{}
	}
	m.mu.RLock()
	snapshot := m.snapshotLocked()
	m.mu.RUnlock()
	return snapshot
}

func (m *AgentMetrics) snapshotLocked() AgentMetricsSnapshot {
	latencies := append([]int64(nil), m.latencies...)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95 := int64(0)
	if len(latencies) > 0 {
		index := (95*len(latencies) + 99) / 100
		p95 = latencies[index-1]
	}
	return AgentMetricsSnapshot{
		RequestCount:     m.requestCount,
		FailureCount:     m.failureCount,
		PromptTokens:     m.promptTokens,
		CompletionTokens: m.completionTokens,
		TotalTokens:      m.promptTokens + m.completionTokens,
		P95LatencyMS:     p95,
		LatencySamples:   len(latencies),
		CostUSD:          m.costUSD,
	}
}
