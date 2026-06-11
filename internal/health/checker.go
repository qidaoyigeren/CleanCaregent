package health

import (
	"context"
	"sync"
	"time"
)

type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

type ComponentStatus struct {
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
}

type Report struct {
	Ready      bool                       `json:"ready"`
	Components map[string]ComponentStatus `json:"components"`
}

type Service struct {
	timeout  time.Duration
	checkers []Checker
}

func NewService(timeout time.Duration, checkers ...Checker) *Service {
	return &Service{timeout: timeout, checkers: checkers}
}

func (s *Service) Check(ctx context.Context) Report {
	report := Report{
		Ready:      true,
		Components: map[string]ComponentStatus{},
	}
	if len(s.checkers) == 0 {
		return report
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, checker := range s.checkers {
		checker := checker
		wg.Add(1)
		go func() {
			defer wg.Done()
			startedAt := time.Now()
			status := ComponentStatus{Status: "ready"}
			if err := checker.Check(ctx); err != nil {
				status.Status = "not_ready"
				status.Error = err.Error()
			}
			status.LatencyMS = time.Since(startedAt).Milliseconds()

			mu.Lock()
			report.Components[checker.Name()] = status
			if status.Status != "ready" {
				report.Ready = false
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return report
}

type FuncChecker struct {
	ComponentName string
	CheckFunc     func(context.Context) error
}

func (c FuncChecker) Name() string {
	return c.ComponentName
}

func (c FuncChecker) Check(ctx context.Context) error {
	return c.CheckFunc(ctx)
}
