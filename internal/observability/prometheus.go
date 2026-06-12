package observability

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type PrometheusMetrics struct {
	mu               sync.RWMutex
	requests         map[string]uint64
	toolCalls        map[string]uint64
	fallbacks        map[string]uint64
	requestByIntent  map[string]histogram
	toolLatency      map[string]histogram
	tokensByIntent   map[string]uint64
	requestDuration  histogram
	reactSteps       histogram
	retrievalLatency histogram
	promptTokens     uint64
	completionTokens uint64
	costUSD          float64
}

type histogram struct {
	bounds  []float64
	buckets []uint64
	count   uint64
	sum     float64
}

var DefaultPrometheusMetrics = NewPrometheusMetrics()

// NewPrometheusMetrics creates an in-process Prometheus metric registry.
func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		requests:        make(map[string]uint64),
		toolCalls:       make(map[string]uint64),
		fallbacks:       make(map[string]uint64),
		requestByIntent: make(map[string]histogram),
		toolLatency:     make(map[string]histogram),
		tokensByIntent:  make(map[string]uint64),
		requestDuration: newHistogram(
			0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30,
		),
		reactSteps:       newHistogram(1, 2, 3, 4, 5, 8, 13),
		retrievalLatency: newHistogram(0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5),
	}
}

// RecordRequest records an Agent request and its token/cost footprint.
func (m *PrometheusMetrics) RecordRequest(
	intent, status string,
	duration time.Duration,
	promptTokens, completionTokens, steps int,
	costUSD float64,
) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.requests[labelKey(intent, status)]++
	m.requestDuration.observe(duration.Seconds())
	intentDuration := m.requestByIntent[intent]
	if intentDuration.bounds == nil {
		intentDuration = newHistogram(0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30)
	}
	intentDuration.observe(duration.Seconds())
	m.requestByIntent[intent] = intentDuration
	m.reactSteps.observe(float64(steps))
	if promptTokens > 0 {
		m.promptTokens += uint64(promptTokens)
		m.tokensByIntent[labelKey(intent, "prompt")] += uint64(promptTokens)
	}
	if completionTokens > 0 {
		m.completionTokens += uint64(completionTokens)
		m.tokensByIntent[labelKey(intent, "completion")] += uint64(completionTokens)
	}
	if costUSD > 0 {
		m.costUSD += costUSD
	}
	m.mu.Unlock()
}

// RecordTool records one completed tool call.
func (m *PrometheusMetrics) RecordTool(name, status string) {
	m.RecordToolDuration(name, status, 0)
}

// RecordToolDuration records one completed tool call and its latency.
func (m *PrometheusMetrics) RecordToolDuration(name, status string, duration time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.toolCalls[labelKey(name, status)]++
	if duration > 0 {
		latency := m.toolLatency[name]
		if latency.bounds == nil {
			latency = newHistogram(0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5)
		}
		latency.observe(duration.Seconds())
		m.toolLatency[name] = latency
	}
	m.mu.Unlock()
}

// RecordFallback records a provider fallback by stage and reason.
func (m *PrometheusMetrics) RecordFallback(stage, reason string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.fallbacks[labelKey(stage, reason)]++
	m.mu.Unlock()
}

// RecordRetrieval records one retrieval duration.
func (m *PrometheusMetrics) RecordRetrieval(duration time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.retrievalLatency.observe(duration.Seconds())
	m.mu.Unlock()
}

// Render returns Prometheus text exposition format.
func (m *PrometheusMetrics) Render() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var builder strings.Builder
	builder.WriteString("# TYPE cleancare_requests_total counter\n")
	writeLabeledCounter(&builder, "cleancare_requests_total", m.requests, "intent", "status")
	writeHistogram(&builder, "cleancare_request_duration_seconds", m.requestDuration)
	writeLabeledHistograms(
		&builder,
		"cleancare_request_duration_by_intent_seconds",
		"intent",
		m.requestByIntent,
	)
	builder.WriteString("# TYPE cleancare_tool_calls_total counter\n")
	writeLabeledCounter(&builder, "cleancare_tool_calls_total", m.toolCalls, "tool", "status")
	writeLabeledHistograms(
		&builder,
		"cleancare_tool_duration_seconds",
		"tool",
		m.toolLatency,
	)
	builder.WriteString("# TYPE cleancare_fallback_total counter\n")
	writeLabeledCounter(&builder, "cleancare_fallback_total", m.fallbacks, "stage", "reason")
	writeHistogram(&builder, "cleancare_react_steps", m.reactSteps)
	fmt.Fprintf(&builder, "# TYPE cleancare_tokens_consumed counter\ncleancare_tokens_consumed{type=\"prompt\"} %d\n", m.promptTokens)
	fmt.Fprintf(&builder, "cleancare_tokens_consumed{type=\"completion\"} %d\n", m.completionTokens)
	builder.WriteString("# TYPE cleancare_tokens_by_intent_total counter\n")
	writeLabeledCounter(
		&builder,
		"cleancare_tokens_by_intent_total",
		m.tokensByIntent,
		"intent",
		"type",
	)
	writeHistogram(&builder, "cleancare_retrieval_latency_seconds", m.retrievalLatency)
	fmt.Fprintf(&builder, "# TYPE cleancare_llm_cost_usd_total counter\ncleancare_llm_cost_usd_total %.9f\n", m.costUSD)
	return builder.String()
}

func writeLabeledHistograms(
	builder *strings.Builder,
	name string,
	label string,
	values map[string]histogram,
) {
	fmt.Fprintf(builder, "# TYPE %s histogram\n", name)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		for index, bound := range value.bounds {
			fmt.Fprintf(
				builder,
				"%s_bucket{%s=\"%s\",le=\"%g\"} %d\n",
				name,
				label,
				escapeLabel(key),
				bound,
				value.buckets[index],
			)
		}
		fmt.Fprintf(
			builder,
			"%s_bucket{%s=\"%s\",le=\"+Inf\"} %d\n",
			name,
			label,
			escapeLabel(key),
			value.buckets[len(value.buckets)-1],
		)
		fmt.Fprintf(
			builder,
			"%s_sum{%s=\"%s\"} %.9f\n%s_count{%s=\"%s\"} %d\n",
			name,
			label,
			escapeLabel(key),
			value.sum,
			name,
			label,
			escapeLabel(key),
			value.count,
		)
	}
}

func newHistogram(bounds ...float64) histogram {
	return histogram{bounds: bounds, buckets: make([]uint64, len(bounds)+1)}
}

func (h *histogram) observe(value float64) {
	h.count++
	h.sum += value
	for index, bound := range h.bounds {
		if value <= bound {
			h.buckets[index]++
		}
	}
	h.buckets[len(h.buckets)-1]++
}

func writeHistogram(builder *strings.Builder, name string, value histogram) {
	fmt.Fprintf(builder, "# TYPE %s histogram\n", name)
	for index, bound := range value.bounds {
		fmt.Fprintf(builder, "%s_bucket{le=\"%g\"} %d\n", name, bound, value.buckets[index])
	}
	fmt.Fprintf(builder, "%s_bucket{le=\"+Inf\"} %d\n", name, value.buckets[len(value.buckets)-1])
	fmt.Fprintf(builder, "%s_sum %.9f\n%s_count %d\n", name, value.sum, name, value.count)
}

func writeLabeledCounter(
	builder *strings.Builder,
	name string,
	values map[string]uint64,
	firstLabel string,
	secondLabel string,
) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts := strings.SplitN(key, "\x00", 2)
		fmt.Fprintf(
			builder,
			"%s{%s=\"%s\",%s=\"%s\"} %d\n",
			name,
			firstLabel,
			escapeLabel(parts[0]),
			secondLabel,
			escapeLabel(parts[1]),
			values[key],
		)
	}
}

func labelKey(left, right string) string {
	return left + "\x00" + right
}

func escapeLabel(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(value)
}
