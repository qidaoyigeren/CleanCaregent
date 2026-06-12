package eval

import (
	"context"
	"testing"
	"time"
)

type fixedRunExecutor struct {
	run Run
}

func TestComparisonRunnerStartsInBackground(t *testing.T) {
	runner := NewComparisonRunner(
		fixedRunExecutor{run: Run{Summary: map[string]any{"pass_rate": 0.5}}},
		fixedRunExecutor{run: Run{Summary: map[string]any{"pass_rate": 0.7}}},
	)
	started, err := runner.Start(context.Background(), ComparisonRequest{MaxCases: 1})
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != "running" || started.ComparisonID == "" {
		t.Fatalf("started comparison = %#v", started)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, ok := runner.Get(started.ComparisonID)
		if ok && current.Status == "completed" {
			if current.Report == nil || current.Report.Deltas["pass_rate"] < 0.19 {
				t.Fatalf("completed comparison = %#v", current)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("comparison did not complete")
}

func (e fixedRunExecutor) Run(context.Context, RunRequest) (Run, error) {
	return e.run, nil
}

func TestComparisonRunnerBuildsImprovementDeltas(t *testing.T) {
	runner := NewComparisonRunner(
		fixedRunExecutor{run: Run{Summary: map[string]any{
			"pass_rate":      0.60,
			"p95_latency_ms": int64(1200),
			"average_tokens": 300.0,
			"metrics":        map[string]float64{"answer_correctness": 0.65},
		}}},
		fixedRunExecutor{run: Run{Summary: map[string]any{
			"pass_rate":      0.78,
			"p95_latency_ms": int64(1500),
			"average_tokens": 420.0,
			"metrics":        map[string]float64{"answer_correctness": 0.82},
		}}},
	)
	report, err := runner.Run(context.Background(), ComparisonRequest{DatasetVersion: "v2"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Deltas["pass_rate"] < 0.17 ||
		report.Deltas["metric.answer_correctness"] < 0.16 {
		t.Fatalf("deltas = %#v", report.Deltas)
	}
	if report.Deltas["p95_latency_ms"] != -300 || report.Deltas["average_tokens"] != -120 {
		t.Fatalf("cost deltas = %#v", report.Deltas)
	}
}
