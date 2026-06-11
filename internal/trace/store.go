package trace

import (
	"context"
	"time"
)

type AgentTrace struct {
	TraceID        string    `json:"trace_id"`
	ConversationID string    `json:"conversation_id"`
	Intent         string    `json:"intent"`
	RouteMode      string    `json:"route_mode"`
	Plan           any       `json:"plan,omitempty"`
	StartedAt      time.Time `json:"started_at"`
}

type Step struct {
	TraceID    string         `json:"trace_id"`
	StepID     string         `json:"step_id"`
	Type       string         `json:"type"`
	Status     string         `json:"status"`
	DurationMS int64          `json:"duration_ms"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type Result struct {
	Status       string    `json:"status"`
	ErrorCode    string    `json:"error_code,omitempty"`
	EvidenceIDs  []string  `json:"evidence_ids,omitempty"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	LatencyMS    int64     `json:"latency_ms"`
	FinishedAt   time.Time `json:"finished_at"`
}

type Store interface {
	Start(ctx context.Context, trace AgentTrace) error
	AppendStep(ctx context.Context, step Step) error
	Finish(ctx context.Context, traceID string, result Result) error
	Get(ctx context.Context, traceID string) (AgentTraceRecord, error)
}

type AgentTraceRecord struct {
	AgentTrace
	Steps        []Step     `json:"steps"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	Status       string     `json:"status"`
	ErrorCode    string     `json:"error_code,omitempty"`
	EvidenceIDs  []string   `json:"evidence_ids,omitempty"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
	LatencyMS    int64      `json:"latency_ms"`
	FinishedAt   time.Time  `json:"finished_at,omitempty"`
}

type ToolCall struct {
	CallID        string         `json:"call_id"`
	ToolName      string         `json:"tool_name"`
	Arguments     map[string]any `json:"arguments,omitempty"`
	ResultSummary any            `json:"result_summary,omitempty"`
	Status        string         `json:"status"`
	ErrorCode     string         `json:"error_code,omitempty"`
	LatencyMS     int64          `json:"latency_ms"`
	CreatedAt     time.Time      `json:"created_at"`
}
