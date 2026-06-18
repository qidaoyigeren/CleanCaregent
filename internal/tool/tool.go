package tool

import (
	"context"
	"encoding/json"
	"strings"
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
	DataScope  string
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

// Client is the MCP-facing tool client used by the executor. Implementations
// are expected to discover tools through MCP tools/list and execute through
// MCP tools/call.
type Client interface {
	ListTools(ctx context.Context) ([]Definition, error)
	CallTool(ctx context.Context, call Call) (Result, error)
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

type CallLogStore interface {
	SaveToolCall(ctx context.Context, call Call, result Result) error
}

// LogicalName returns the tool identity used by the agent policy layer.
// Aggregated MCP tools are exposed as "server/tool"; the logical name remains
// the remote tool name so existing intent policies can stay transport-agnostic.
func LogicalName(name string) string {
	name = strings.TrimSpace(name)
	if server, remote, ok := strings.Cut(name, "/"); ok && strings.TrimSpace(server) != "" {
		return strings.TrimSpace(remote)
	}
	return name
}

// NamesMatch compares exact tool names and aggregated "server/tool" aliases.
func NamesMatch(allowedName, candidateName string) bool {
	allowedName = strings.TrimSpace(allowedName)
	candidateName = strings.TrimSpace(candidateName)
	if allowedName == "" || candidateName == "" {
		return false
	}
	return allowedName == candidateName ||
		LogicalName(allowedName) == candidateName ||
		allowedName == LogicalName(candidateName) ||
		LogicalName(allowedName) == LogicalName(candidateName)
}

// NameAllowed reports whether a candidate tool is permitted by a policy list.
func NameAllowed(allowed []string, candidateName string) bool {
	for _, name := range allowed {
		if NamesMatch(name, candidateName) {
			return true
		}
	}
	return false
}
