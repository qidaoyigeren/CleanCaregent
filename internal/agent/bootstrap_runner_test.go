package agent

import (
	"context"
	"strings"
	"testing"
)

func TestBootstrapRunnerStreamsAndReturnsResult(t *testing.T) {
	runner := NewBootstrapRunner("bootstrap")
	var eventTypes []string
	var streamed strings.Builder

	result, err := runner.Run(context.Background(), Request{
		TraceID: "tr_test",
		Query:   "test",
	}, func(event Event) error {
		eventTypes = append(eventTypes, event.Type)
		if event.Type == "delta" {
			data := event.Data.(map[string]string)
			streamed.WriteString(data["content"])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Mode != "bootstrap" {
		t.Fatalf("Mode = %q", result.Mode)
	}
	if streamed.String() != result.Answer {
		t.Fatalf("streamed answer does not match result")
	}
	if len(eventTypes) < 2 || eventTypes[0] != "status" {
		t.Fatalf("event types = %v", eventTypes)
	}
}

func TestBootstrapRunnerRejectsUnknownMode(t *testing.T) {
	runner := NewBootstrapRunner("rag")
	_, err := runner.Run(context.Background(), Request{}, nil)
	if err != ErrNotConfigured {
		t.Fatalf("Run() error = %v, want %v", err, ErrNotConfigured)
	}
}
