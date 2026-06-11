package eval

import (
	"context"
	"time"
)

type Run struct {
	RunNo          string         `json:"run_no"`
	DatasetVersion string         `json:"dataset_version"`
	SystemVersion  string         `json:"system_version"`
	Status         string         `json:"status"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	Summary        map[string]any `json:"summary,omitempty"`
	Results        []CaseResult   `json:"results,omitempty"`
}

type CaseResult struct {
	CaseID       string         `json:"case_id"`
	TraceID      string         `json:"trace_id,omitempty"`
	ActualIntent string         `json:"actual_intent"`
	ActualTools  []string       `json:"actual_tools"`
	Answer       string         `json:"answer"`
	Metrics      []MetricResult `json:"metrics"`
	Passed       bool           `json:"passed"`
	ErrorType    string         `json:"error_type,omitempty"`
	LatencyMS    int64          `json:"latency_ms"`
	TokenCount   int            `json:"token_count"`
}

type Store interface {
	UpsertCases(ctx context.Context, version string, cases []Case) error
	CreateRun(ctx context.Context, run Run) error
	SaveResult(ctx context.Context, runNo, datasetVersion string, result CaseResult) error
	FinishRun(ctx context.Context, runNo, status string, summary map[string]any, finishedAt time.Time) error
	GetRun(ctx context.Context, runNo string, includeFailures bool) (Run, error)
}
