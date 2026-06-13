package agent

import (
	"context"
	"errors"

	"CleanCaregent/internal/memory"
	"CleanCaregent/internal/rag"
)

var ErrNotConfigured = errors.New("agent pipeline is not configured")

type Request struct {
	TraceID        string
	UserID         string
	ConversationID string
	Query          string
	Context        memory.ConversationContext
}

type Evidence struct {
	ID       string         `json:"evidence_id"`
	Kind     string         `json:"kind"`
	SourceID string         `json:"source_id"`
	Title    string         `json:"title"`
	Content  string         `json:"content,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Result struct {
	Answer    string     `json:"answer"`
	Evidences []Evidence `json:"evidences"`
	Mode      string     `json:"mode"`
}

type EventSink func(Event) error

type Runner interface {
	Run(ctx context.Context, req Request, sink EventSink) (Result, error)
}

type DynamicExecutionRequest struct {
	Request          Request
	Intent           string
	SecondaryIntents []string
	AllowedTools     []string
	Step             PlanStep
}

type DynamicExecutionResult struct {
	Answer     string
	Evidences  []Evidence
	SearchData []rag.SearchResult
	Metadata   map[string]any
}

type DynamicExecutor interface {
	Execute(ctx context.Context, request DynamicExecutionRequest) (DynamicExecutionResult, error)
}
