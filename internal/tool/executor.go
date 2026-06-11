package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	ErrToolNotFound     = errors.New("tool not found")
	ErrToolNotAllowed   = errors.New("tool not allowed")
	ErrRepeatedToolCall = errors.New("repeated tool call")
	ErrInvalidArguments = errors.New("tool arguments do not match schema")
)

type Executor struct {
	registry Registry
	logStore CallLogStore
	timeout  time.Duration
	mu       sync.Mutex
	seen     map[string]time.Time
	seenTTL  time.Duration
}

func NewExecutor(registry Registry, logStore CallLogStore, timeout time.Duration) *Executor {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Executor{
		registry: registry,
		logStore: logStore,
		timeout:  timeout,
		seen:     make(map[string]time.Time),
		seenTTL:  10 * time.Minute,
	}
}

func (e *Executor) Execute(ctx context.Context, call Call, allowed []string) (Result, error) {
	ctx, span := otel.Tracer("clean-care-agent/tool").Start(ctx, "tool."+call.Name)
	span.SetAttributes(
		attribute.String("tool.name", call.Name),
		attribute.String("agent.trace_id", call.TraceID),
	)
	defer span.End()
	startedAt := time.Now().UTC()
	result := Result{CallID: call.CallID, StartedAt: startedAt}
	if !contains(allowed, call.Name) {
		span.SetStatus(codes.Error, "tool not allowed")
		return e.failAndLog(ctx, call, result, "TOOL_NOT_ALLOWED", ErrToolNotAllowed)
	}
	value, ok := e.registry.Get(call.Name)
	if !ok {
		span.SetStatus(codes.Error, "tool not found")
		return e.failAndLog(ctx, call, result, "TOOL_NOT_FOUND", ErrToolNotFound)
	}
	if err := validateArguments(value.ParamsSchema(), call.Arguments); err != nil {
		span.SetStatus(codes.Error, "invalid tool arguments")
		return e.failAndLog(
			ctx,
			call,
			result,
			"INVALID_TOOL_ARGUMENTS",
			fmt.Errorf("%w: %v", ErrInvalidArguments, err),
		)
	}
	signature := toolCallSignature(call)
	e.mu.Lock()
	now := time.Now()
	for key, seenAt := range e.seen {
		if now.Sub(seenAt) > e.seenTTL {
			delete(e.seen, key)
		}
	}
	_, repeated := e.seen[signature]
	if !repeated {
		e.seen[signature] = now
	}
	e.mu.Unlock()
	if repeated {
		span.SetStatus(codes.Error, "repeated tool call")
		return e.failAndLog(ctx, call, result, "REPEATED_TOOL_CALL", ErrRepeatedToolCall)
	}

	runCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	executed, err := value.Execute(runCtx, call)
	if executed.CallID == "" {
		executed.CallID = call.CallID
	}
	if executed.StartedAt.IsZero() {
		executed.StartedAt = startedAt
	}
	executed.FinishedAt = time.Now().UTC()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		executed.Success = false
		if executed.ErrorCode == "" {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				executed.ErrorCode = "TOOL_TIMEOUT"
			} else {
				executed.ErrorCode = "TOOL_EXECUTION_FAILED"
			}
		}
		if executed.Message == "" {
			executed.Message = err.Error()
		}
		e.log(context.WithoutCancel(ctx), call, executed)
		return executed, fmt.Errorf("execute tool %s: %w", call.Name, err)
	}
	executed.Success = true
	span.SetAttributes(attribute.Int64("tool.latency_ms", executed.FinishedAt.Sub(executed.StartedAt).Milliseconds()))
	e.log(context.WithoutCancel(ctx), call, executed)
	return executed, nil
}

func (e *Executor) failAndLog(
	ctx context.Context,
	call Call,
	result Result,
	code string,
	err error,
) (Result, error) {
	result.Success = false
	result.ErrorCode = code
	result.Message = err.Error()
	result.FinishedAt = time.Now().UTC()
	e.log(context.WithoutCancel(ctx), call, result)
	return result, err
}

func (e *Executor) log(ctx context.Context, call Call, result Result) {
	if e.logStore == nil {
		return
	}
	logCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = e.logStore.SaveToolCall(logCtx, call, result)
}

func toolCallSignature(call Call) string {
	raw, _ := json.Marshal(call.Arguments)
	sum := sha256.Sum256(append([]byte(call.TraceID+"|"+call.Name+"|"), raw...))
	return hex.EncodeToString(sum[:])
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validateArguments(raw json.RawMessage, arguments map[string]any) error {
	if len(raw) == 0 {
		return nil
	}
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return fmt.Errorf("decode params schema: %w", err)
	}
	for _, name := range schema.Required {
		value, ok := arguments[name]
		if !ok || value == nil {
			return fmt.Errorf("missing required field %q", name)
		}
	}
	for name, value := range arguments {
		property, ok := schema.Properties[name]
		if !ok || property.Type == "" || value == nil {
			continue
		}
		if !matchesJSONType(value, property.Type) {
			return fmt.Errorf("field %q must be %s", name, property.Type)
		}
	}
	return nil
}

func matchesJSONType(value any, expected string) bool {
	switch expected {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		switch value.(type) {
		case []any, []string, []int, []int64, []float64:
			return true
		default:
			return false
		}
	case "integer":
		switch typed := value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
			return true
		case float64:
			return typed == float64(int64(typed))
		default:
			return false
		}
	case "number":
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
			return true
		default:
			return false
		}
	case "object":
		_, ok := value.(map[string]any)
		return ok
	default:
		return true
	}
}
