package tool

import (
	"context"
	"encoding/json"
	"time"
)

type Call struct {
	TraceID        string
	CallID         string
	UserID         string
	ConversationID string
	Name           string
	Arguments      map[string]any
	IdempotencyKey string
}

type Result struct {
	CallID     string
	Success    bool
	Data       any
	ErrorCode  string
	Message    string
	StartedAt  time.Time
	FinishedAt time.Time
}

type SideEffect string

const (
	SideEffectNone        SideEffect = "none"
	SideEffectReadOnly    SideEffect = "read_only"
	SideEffectStateChange SideEffect = "state_change"
)

type Definition struct {
	Name         string
	Description  string
	ParamsSchema json.RawMessage
	SideEffect   SideEffect
}

type Tool interface {
	Name() string
	Description() string
	ParamsSchema() json.RawMessage
	Execute(ctx context.Context, call Call) (Result, error)
}

// SideEffectTool lets tools declare mutation semantics without changing the
// existing Tool interface implemented by external integrations.
type SideEffectTool interface {
	SideEffect() SideEffect
}

func EffectOf(value Tool) SideEffect {
	if declared, ok := value.(SideEffectTool); ok {
		switch declared.SideEffect() {
		case SideEffectNone, SideEffectReadOnly, SideEffectStateChange:
			return declared.SideEffect()
		}
	}
	return SideEffectReadOnly
}

type Registry interface {
	Register(tool Tool) error
	Get(name string) (Tool, bool)
	ListAllowed(names []string) []Definition
}

type CallLogStore interface {
	SaveToolCall(ctx context.Context, call Call, result Result) error
}
