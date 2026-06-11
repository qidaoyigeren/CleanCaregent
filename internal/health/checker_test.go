package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceReportsFailedComponent(t *testing.T) {
	service := NewService(time.Second,
		FuncChecker{
			ComponentName: "healthy",
			CheckFunc: func(context.Context) error {
				return nil
			},
		},
		FuncChecker{
			ComponentName: "failed",
			CheckFunc: func(context.Context) error {
				return errors.New("dependency unavailable")
			},
		},
	)

	report := service.Check(context.Background())
	if report.Ready {
		t.Fatal("report.Ready = true")
	}
	if report.Components["healthy"].Status != "ready" {
		t.Fatalf("healthy status = %q", report.Components["healthy"].Status)
	}
	if report.Components["failed"].Status != "not_ready" {
		t.Fatalf("failed status = %q", report.Components["failed"].Status)
	}
}

func TestServiceAppliesTimeout(t *testing.T) {
	service := NewService(10*time.Millisecond, FuncChecker{
		ComponentName: "slow",
		CheckFunc: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})

	report := service.Check(context.Background())
	if report.Ready || report.Components["slow"].Error == "" {
		t.Fatalf("report = %#v", report)
	}
}
