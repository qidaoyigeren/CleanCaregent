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

type Definition struct {
	Name         string
	Description  string
	ParamsSchema json.RawMessage
}

type Tool interface {
	Name() string
	Description() string
	ParamsSchema() json.RawMessage
	Execute(ctx context.Context, call Call) (Result, error)
}

type Registry interface {
	Register(tool Tool) error
	Get(name string) (Tool, bool)
	ListAllowed(names []string) []Definition
}

type CallLogStore interface {
	SaveToolCall(ctx context.Context, call Call, result Result) error
}
