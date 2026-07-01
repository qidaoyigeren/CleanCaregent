package eval

import (
	"context"
	"fmt"
	"sync"
	"time"

	"CleanCaregent/internal/platform/id"
)

type RunExecutor interface {
	Run(ctx context.Context, request RunRequest) (Run, error)
}

type ComparisonRunner struct {
	baseline  RunExecutor
	candidate RunExecutor
	mu        sync.RWMutex
	runs      map[string]ComparisonRun
}

type ComparisonRequest struct {
	UserID         string
	DatasetVersion string
	MaxCases       int
	Split          string
}

type ComparisonReport struct {
	DatasetVersion string             `json:"dataset_version"`
	Baseline       Run                `json:"baseline"`
	Candidate      Run                `json:"candidate"`
	Deltas         map[string]float64 `json:"deltas"`
	Split          string             `json:"split,omitempty"`
}

type ComparisonRun struct {
	ComparisonID string            `json:"comparison_id"`
	Status       string            `json:"status"`
	Report       *ComparisonReport `json:"report,omitempty"`
	Error        string            `json:"error,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	FinishedAt   *time.Time        `json:"finished_at,omitempty"`
}

func NewComparisonRunner(baseline, candidate RunExecutor) *ComparisonRunner {
	return &ComparisonRunner{
		baseline:  baseline,
		candidate: candidate,
		runs:      make(map[string]ComparisonRun),
	}
}

func (r *ComparisonRunner) Start(
	ctx context.Context,
	request ComparisonRequest,
) (ComparisonRun, error) {
	if r == nil || r.baseline == nil || r.candidate == nil {
		return ComparisonRun{}, fmt.Errorf("comparison runner is not configured")
	}
	if request.DatasetVersion == "" {
		request.DatasetVersion = "v2"
	}
	run := ComparisonRun{
		ComparisonID: id.New("cmp"),
		Status:       "running",
		StartedAt:    time.Now().UTC(),
	}
	r.mu.Lock()
	r.runs[run.ComparisonID] = run
	r.mu.Unlock()

	go func() {
		runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Hour)
		defer cancel()
		report, err := r.Run(runCtx, request)
		finishedAt := time.Now().UTC()
		r.mu.Lock()
		current := r.runs[run.ComparisonID]
		current.FinishedAt = &finishedAt
		if err != nil {
			current.Status = "failed"
			current.Error = err.Error()
		} else {
			current.Status = "completed"
			current.Report = &report
		}
		r.runs[run.ComparisonID] = current
		r.mu.Unlock()
	}()
	return run, nil
}

func (r *ComparisonRunner) Get(comparisonID string) (ComparisonRun, bool) {
	if r == nil {
		return ComparisonRun{}, false
	}
	r.mu.RLock()
	run, ok := r.runs[comparisonID]
	r.mu.RUnlock()
	return run, ok
}

func (r *ComparisonRunner) Run(
	ctx context.Context,
	request ComparisonRequest,
) (ComparisonReport, error) {
	if r == nil || r.baseline == nil || r.candidate == nil {
		return ComparisonReport{}, fmt.Errorf("comparison runner is not configured")
	}
	if request.DatasetVersion == "" {
		request.DatasetVersion = "v2"
	}
	baseline, err := r.baseline.Run(ctx, RunRequest{
		UserID:         request.UserID,
		DatasetVersion: request.DatasetVersion,
		SystemVersion:  "naive-rag-baseline",
		MaxCases:       request.MaxCases,
		Split:          request.Split,
	})
	if err != nil {
		return ComparisonReport{}, fmt.Errorf("run naive baseline: %w", err)
	}
	candidate, err := r.candidate.Run(ctx, RunRequest{
		UserID:         request.UserID,
		DatasetVersion: request.DatasetVersion,
		SystemVersion:  "agentic-rag-candidate",
		MaxCases:       request.MaxCases,
		Split:          request.Split,
	})
	if err != nil {
		return ComparisonReport{}, fmt.Errorf("run agentic candidate: %w", err)
	}
	return ComparisonReport{
		DatasetVersion: request.DatasetVersion,
		Baseline:       baseline,
		Candidate:      candidate,
		Deltas:         comparisonDeltas(baseline.Summary, candidate.Summary),
		Split:          request.Split,
	}, nil
}

func comparisonDeltas(baseline, candidate map[string]any) map[string]float64 {
	deltas := map[string]float64{
		"pass_rate":      summaryNumber(candidate, "pass_rate") - summaryNumber(baseline, "pass_rate"),
		"p95_latency_ms": summaryNumber(baseline, "p95_latency_ms") - summaryNumber(candidate, "p95_latency_ms"),
		"average_tokens": summaryNumber(baseline, "average_tokens") - summaryNumber(candidate, "average_tokens"),
	}
	baselineMetrics, _ := baseline["metrics"].(map[string]float64)
	candidateMetrics, _ := candidate["metrics"].(map[string]float64)
	if baselineMetrics == nil {
		baselineMetrics = anyFloatMap(baseline["metrics"])
	}
	if candidateMetrics == nil {
		candidateMetrics = anyFloatMap(candidate["metrics"])
	}
	for name, candidateValue := range candidateMetrics {
		deltas["metric."+name] = candidateValue - baselineMetrics[name]
	}
	return deltas
}

func summaryNumber(summary map[string]any, key string) float64 {
	if summary == nil {
		return 0
	}
	switch value := summary[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}

func anyFloatMap(value any) map[string]float64 {
	source, _ := value.(map[string]any)
	result := make(map[string]float64, len(source))
	for key, value := range source {
		result[key] = summaryNumber(map[string]any{"value": value}, "value")
	}
	return result
}
