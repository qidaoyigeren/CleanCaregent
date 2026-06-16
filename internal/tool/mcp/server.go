package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"CleanCaregent/internal/tool"
)

var (
	ErrToolAlreadyAdded = errors.New("mcp tool already added")
	ErrToolNotFound     = errors.New("mcp tool not found")
)

type Tool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Annotations Annotations     `json:"annotations,omitempty"`
}

type Annotations struct {
	ReadOnlyHint    *bool `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool `json:"idempotentHint,omitempty"`
}

type CallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Meta      CallMeta       `json:"_meta,omitempty"`
}

type CallMeta struct {
	TraceID        string `json:"trace_id,omitempty"`
	CallID         string `json:"call_id,omitempty"`
	UserID         string `json:"user_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Data any    `json:"data,omitempty"`
}

type CallToolResult struct {
	ResultType        string    `json:"resultType,omitempty"`
	Content           []Content `json:"content"`
	StructuredContent any       `json:"structuredContent,omitempty"`
	IsError           bool      `json:"isError,omitempty"`
	ErrorCode         string    `json:"errorCode,omitempty"`
	Message           string    `json:"message,omitempty"`
	DataScope         string    `json:"dataScope,omitempty"`
	StartedAt         time.Time `json:"startedAt,omitempty"`
	FinishedAt        time.Time `json:"finishedAt,omitempty"`
}

type Server struct {
	mu    sync.RWMutex
	tools map[string]tool.Tool
}

func NewServer(values ...tool.Tool) (*Server, error) {
	server := &Server{tools: make(map[string]tool.Tool, len(values))}
	for _, value := range values {
		if err := server.AddTool(value); err != nil {
			return nil, err
		}
	}
	return server, nil
}

func (s *Server) AddTool(value tool.Tool) error {
	if value == nil {
		return errors.New("mcp tool is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tools[value.Name()]; exists {
		return ErrToolAlreadyAdded
	}
	s.tools[value.Name()] = value
	return nil
}

func (s *Server) ListTools(context.Context) ([]Tool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Tool, 0, len(s.tools))
	for _, value := range s.tools {
		result = append(result, Tool{
			Name:        value.Name(),
			Description: value.Description(),
			InputSchema: append(json.RawMessage(nil), value.ParamsSchema()...),
			Annotations: annotationsFor(tool.EffectOf(value)),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s *Server) CallTool(ctx context.Context, params CallToolParams) (CallToolResult, error) {
	s.mu.RLock()
	value, ok := s.tools[params.Name]
	s.mu.RUnlock()
	if !ok {
		return CallToolResult{IsError: true, ErrorCode: "TOOL_NOT_FOUND", Message: ErrToolNotFound.Error()}, nil
	}
	startedAt := time.Now().UTC()
	result, err := value.Execute(ctx, tool.Call{
		TraceID:        params.Meta.TraceID,
		CallID:         params.Meta.CallID,
		UserID:         params.Meta.UserID,
		ConversationID: params.Meta.ConversationID,
		Name:           params.Name,
		Arguments:      params.Arguments,
		IdempotencyKey: params.Meta.IdempotencyKey,
	})
	if result.StartedAt.IsZero() {
		result.StartedAt = startedAt
	}
	if result.FinishedAt.IsZero() {
		result.FinishedAt = time.Now().UTC()
	}
	response := CallToolResult{
		ResultType: "complete",
		DataScope:  result.DataScope,
		ErrorCode:  result.ErrorCode,
		Message:    result.Message,
		StartedAt:  result.StartedAt,
		FinishedAt: result.FinishedAt,
	}
	if err != nil {
		response.IsError = true
		if response.Message == "" {
			response.Message = err.Error()
		}
		if response.ErrorCode == "" {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				response.ErrorCode = "TOOL_TIMEOUT"
			} else {
				response.ErrorCode = "TOOL_EXECUTION_FAILED"
			}
		}
		response.Content = []Content{{Type: "text", Text: response.Message}}
		return response, nil
	}
	response.Content = []Content{{Type: "json", Data: result.Data}}
	response.StructuredContent = result.Data
	return response, nil
}

type InProcessClient struct {
	server *Server
}

func NewInProcessClient(server *Server) *InProcessClient {
	return &InProcessClient{server: server}
}

func (c *InProcessClient) ListTools(ctx context.Context) ([]tool.Definition, error) {
	if c == nil || c.server == nil {
		return nil, errors.New("mcp client has no server")
	}
	tools, err := c.server.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	definitions := make([]tool.Definition, 0, len(tools))
	for _, value := range tools {
		definitions = append(definitions, tool.Definition{
			Name:         value.Name,
			Description:  value.Description,
			ParamsSchema: append(json.RawMessage(nil), value.InputSchema...),
			SideEffect:   sideEffectFrom(value.Annotations),
		})
	}
	return definitions, nil
}

func (c *InProcessClient) CallTool(ctx context.Context, call tool.Call) (tool.Result, error) {
	if c == nil || c.server == nil {
		return tool.Result{}, errors.New("mcp client has no server")
	}
	response, err := c.server.CallTool(ctx, CallToolParams{
		Name:      call.Name,
		Arguments: call.Arguments,
		Meta: CallMeta{
			TraceID:        call.TraceID,
			CallID:         call.CallID,
			UserID:         call.UserID,
			ConversationID: call.ConversationID,
			IdempotencyKey: call.IdempotencyKey,
		},
	})
	if err != nil {
		return tool.Result{CallID: call.CallID}, err
	}
	result := tool.Result{
		CallID:     call.CallID,
		Data:       resultData(response),
		DataScope:  response.DataScope,
		ErrorCode:  response.ErrorCode,
		Message:    response.Message,
		StartedAt:  response.StartedAt,
		FinishedAt: response.FinishedAt,
	}
	if response.IsError {
		if result.Message == "" {
			result.Message = "mcp tool call failed"
		}
		return result, fmt.Errorf("mcp tools/call %s: %s", call.Name, result.Message)
	}
	return result, nil
}

func annotationsFor(effect tool.SideEffect) Annotations {
	trueValue := true
	falseValue := false
	switch effect {
	case tool.SideEffectStateChange:
		return Annotations{
			ReadOnlyHint:    &falseValue,
			DestructiveHint: &trueValue,
			IdempotentHint:  &trueValue,
		}
	case tool.SideEffectNone:
		return Annotations{
			ReadOnlyHint:    &trueValue,
			DestructiveHint: &falseValue,
		}
	default:
		return Annotations{
			ReadOnlyHint:    &trueValue,
			DestructiveHint: &falseValue,
		}
	}
}

func sideEffectFrom(annotations Annotations) tool.SideEffect {
	if annotations.DestructiveHint != nil && *annotations.DestructiveHint {
		return tool.SideEffectStateChange
	}
	if annotations.ReadOnlyHint != nil && *annotations.ReadOnlyHint {
		return tool.SideEffectReadOnly
	}
	return tool.SideEffectReadOnly
}

func resultData(response CallToolResult) any {
	if response.StructuredContent != nil {
		return response.StructuredContent
	}
	return firstJSONContent(response.Content)
}

func firstJSONContent(contents []Content) any {
	for _, content := range contents {
		if content.Type == "json" {
			return content.Data
		}
		if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
			var decoded any
			if err := json.Unmarshal([]byte(content.Text), &decoded); err == nil {
				return decoded
			}
			return content.Text
		}
	}
	return nil
}
