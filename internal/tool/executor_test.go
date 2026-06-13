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
	data   any
	effect SideEffect
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
	if t.data != nil {
		return Result{Data: t.data}, nil
	}
	if t.name == "price_query" {
		return Result{Data: map[string]any{"items": []any{map[string]any{
			"sku_code":                    "SKU-T20",
			"model":                       "T20",
			"list_price_cents":            399900,
			"current_price_cents":         359900,
			"estimated_final_price_cents": 359900,
			"currency":                    "CNY",
		}}}}, nil
	}
	return Result{Data: map[string]any{"ok": true}}, nil
}
func (t fakeTool) SideEffect() SideEffect {
	if t.effect == "" {
		return SideEffectReadOnly
	}
	return t.effect
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

func TestExecutorAnnotatesConfiguredDataScope(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(fakeTool{name: "price_query"}); err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor(registry, nil, time.Second).WithDataScope("sandbox")
	result, err := executor.Execute(context.Background(), Call{
		TraceID: "tr_scope", CallID: "call_scope", Name: "price_query",
	}, []string{"price_query"})
	if err != nil {
		t.Fatal(err)
	}
	if result.DataScope != "sandbox" {
		t.Fatalf("data scope = %q", result.DataScope)
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

func TestExecutorRejectsInvalidSuccessfulToolResult(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(fakeTool{
		name: "price_query",
		data: map[string]any{"items": []any{map[string]any{
			"sku_code":                    "SKU-T20",
			"model":                       "T20",
			"list_price_cents":            399900,
			"current_price_cents":         0,
			"estimated_final_price_cents": 0,
			"currency":                    "CNY",
		}}},
	})
	executor := NewExecutor(registry, nil, time.Second)
	result, err := executor.Execute(context.Background(), Call{
		TraceID: "tr_invalid_result",
		CallID:  "call_invalid_result",
		Name:    "price_query",
	}, []string{"price_query"})
	if !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("error = %v, want ErrInvalidResult", err)
	}
	if result.Success || result.ErrorCode != "INVALID_TOOL_RESULT" {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecutorRequiresConfirmationAndIdempotencyForStateChange(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(fakeTool{
		name:   "state_change",
		effect: SideEffectStateChange,
		schema: json.RawMessage(`{"type":"object","required":["confirmed"],"properties":{"confirmed":{"type":"boolean"}}}`),
	})
	executor := NewExecutor(registry, nil, time.Second)

	_, err := executor.Execute(context.Background(), Call{
		TraceID: "tr_state", CallID: "call_state_1", Name: "state_change",
		Arguments: map[string]any{"confirmed": false},
	}, []string{"state_change"})
	if !errors.Is(err, ErrIdempotencyRequired) {
		t.Fatalf("error = %v, want ErrIdempotencyRequired", err)
	}

	_, err = executor.Execute(context.Background(), Call{
		TraceID: "tr_state", CallID: "call_state_2", Name: "state_change",
		Arguments: map[string]any{"confirmed": false}, IdempotencyKey: "idem-1",
	}, []string{"state_change"})
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("error = %v, want ErrConfirmationRequired", err)
	}
}
