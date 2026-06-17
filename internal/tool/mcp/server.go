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
	ErrResourceNotFound = errors.New("mcp resource not found")
	ErrPromptNotFound   = errors.New("mcp prompt not found")
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
	mu        sync.RWMutex
	tools     map[string]tool.Tool
	resources map[string]staticResource
	prompts   map[string]staticPrompt

	notificationMu sync.RWMutex
	subscribers    map[chan Notification]struct{}
}

func NewServer(values ...tool.Tool) (*Server, error) {
	server := &Server{
		tools:       make(map[string]tool.Tool, len(values)),
		resources:   make(map[string]staticResource),
		prompts:     make(map[string]staticPrompt),
		subscribers: make(map[chan Notification]struct{}),
	}
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
	go s.publish("notifications/tools/list_changed", nil)
	return nil
}

func (s *Server) AddResource(resource Resource, contents ...ResourceContent) error {
	if strings.TrimSpace(resource.URI) == "" {
		return errors.New("mcp resource uri is required")
	}
	if strings.TrimSpace(resource.Name) == "" {
		resource.Name = resource.URI
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := make([]ResourceContent, 0, len(contents))
	for _, content := range contents {
		if content.URI == "" {
			content.URI = resource.URI
		}
		cloned = append(cloned, content)
	}
	s.resources[resource.URI] = staticResource{definition: resource, contents: cloned}
	go s.publish("notifications/resources/list_changed", nil)
	return nil
}

func (s *Server) AddPrompt(prompt Prompt, result GetPromptResult) error {
	if strings.TrimSpace(prompt.Name) == "" {
		return errors.New("mcp prompt name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts[prompt.Name] = staticPrompt{definition: prompt, result: result}
	go s.publish("notifications/prompts/list_changed", nil)
	return nil
}

func (s *Server) Initialize(_ context.Context, params InitializeParams) InitializeResult {
	version := strings.TrimSpace(params.ProtocolVersion)
	if version == "" {
		version = ProtocolVersion
	}
	if version != ProtocolVersion {
		version = ProtocolVersion
	}
	return InitializeResult{
		ProtocolVersion: version,
		Capabilities:    s.Capabilities(),
		ServerInfo: ImplementationInfo{
			Name:        "cleancare-mcp",
			Title:       "CleanCare MCP Tool Server",
			Version:     "1.0.0",
			Description: "MCP server exposing CleanCare business tools",
		},
		Instructions: "Use tools/list to discover CleanCare tools and tools/call to execute them. Dynamic business data is mock unless data_scope says otherwise.",
	}
}

func (s *Server) Capabilities() ServerCapabilities {
	capabilities := ServerCapabilities{
		Tools: &ListChangedCapability{ListChanged: true},
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.resources) > 0 {
		capabilities.Resources = &ResourceCapability{ListChanged: true}
	}
	if len(s.prompts) > 0 {
		capabilities.Prompts = &ListChangedCapability{ListChanged: true}
	}
	return capabilities
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

func (s *Server) ListResources(context.Context) ([]Resource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Resource, 0, len(s.resources))
	for _, value := range s.resources {
		result = append(result, value.definition)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].URI < result[j].URI })
	return result, nil
}

func (s *Server) ReadResource(_ context.Context, uri string) ([]ResourceContent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.resources[uri]
	if !ok {
		return nil, ErrResourceNotFound
	}
	return append([]ResourceContent(nil), value.contents...), nil
}

func (s *Server) ListPrompts(context.Context) ([]Prompt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Prompt, 0, len(s.prompts))
	for _, value := range s.prompts {
		result = append(result, value.definition)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s *Server) GetPrompt(_ context.Context, name string) (GetPromptResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.prompts[name]
	if !ok {
		return GetPromptResult{}, ErrPromptNotFound
	}
	return value.result, nil
}

func (s *Server) SubscribeNotifications() (<-chan Notification, func()) {
	ch := make(chan Notification, 16)
	s.notificationMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.notificationMu.Unlock()
	cancel := func() {
		s.notificationMu.Lock()
		if _, ok := s.subscribers[ch]; ok {
			delete(s.subscribers, ch)
			close(ch)
		}
		s.notificationMu.Unlock()
	}
	return ch, cancel
}

func (s *Server) publish(method string, params any) {
	s.notificationMu.RLock()
	defer s.notificationMu.RUnlock()
	for ch := range s.subscribers {
		select {
		case ch <- Notification{Method: method, Params: params}:
		default:
		}
	}
}

type staticResource struct {
	definition Resource
	contents   []ResourceContent
}

type staticPrompt struct {
	definition Prompt
	result     GetPromptResult
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
