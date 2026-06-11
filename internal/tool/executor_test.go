package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type fakeTool struct {
	name   string
	schema json.RawMessage
}

func (t fakeTool) Name() string        { return t.name }
func (t fakeTool) Description() string { return "fake" }
func (t fakeTool) ParamsSchema() json.RawMessage {
	if len(t.schema) > 0 {
		return t.schema
	}
	return json.RawMessage(`{"type":"object"}`)
}
func (t fakeTool) Execute(context.Context, Call) (Result, error) {
	return Result{Data: map[string]any{"ok": true}}, nil
}

func TestExecutorValidatesArgumentsAgainstSchema(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(fakeTool{
		name: "price_query",
		schema: json.RawMessage(
			`{"type":"object","required":["product_refs"],"properties":{"product_refs":{"type":"array"}}}`,
		),
	})
	executor := NewExecutor(registry, nil, time.Second)
	_, err := executor.Execute(context.Background(), Call{
		TraceID:   "tr_schema",
		CallID:    "call_schema",
		Name:      "price_query",
		Arguments: map[string]any{"product_refs": "T20"},
	}, []string{"price_query"})
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("error = %v, want ErrInvalidArguments", err)
	}
}

func TestExecutorWhitelistAndRepeatedCall(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(fakeTool{name: "price_query"}); err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor(registry, nil, time.Second)
	call := Call{TraceID: "tr_1", CallID: "call_1", Name: "price_query", Arguments: map[string]any{"model": "T20"}}
	if _, err := executor.Execute(context.Background(), call, []string{"price_query"}); err != nil {
		t.Fatal(err)
	}
	call.CallID = "call_2"
	if _, err := executor.Execute(context.Background(), call, []string{"price_query"}); !errors.Is(err, ErrRepeatedToolCall) {
		t.Fatalf("error = %v, want repeated call", err)
	}
}

func TestExecutorRejectsNonWhitelistedTool(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(fakeTool{name: "price_query"})
	executor := NewExecutor(registry, nil, time.Second)
	_, err := executor.Execute(context.Background(), Call{
		TraceID: "tr_1", CallID: "call_1", Name: "price_query",
	}, []string{"order_lookup"})
	if !errors.Is(err, ErrToolNotAllowed) {
		t.Fatalf("error = %v, want not allowed", err)
	}
}
