package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

func (s *Server) HandleRequest(ctx context.Context, method string, params json.RawMessage) (any, *RPCError) {
	switch strings.TrimSpace(method) {
	case "initialize":
		var request InitializeParams
		if err := json.Unmarshal(rawOrEmpty(params), &request); err != nil {
			return nil, rpcInvalidParams("decode initialize params: " + err.Error())
		}
		return s.Initialize(ctx, request), nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		tools, err := s.ListTools(ctx)
		if err != nil {
			return nil, rpcInternal(err.Error())
		}
		return ListToolsResult{
			ResultType: "complete",
			Tools:      tools,
			TTLMs:      int64((30 * 1000)),
			CacheScope: "public",
		}, nil
	case "tools/call":
		var request CallToolParams
		if err := json.Unmarshal(rawOrEmpty(params), &request); err != nil {
			return nil, rpcInvalidParams("decode tools/call params: " + err.Error())
		}
		if strings.TrimSpace(request.Name) == "" {
			return nil, rpcInvalidParams("tools/call params.name is required")
		}
		result, err := s.CallTool(ctx, request)
		if err != nil {
			return nil, rpcInternal(err.Error())
		}
		return result, nil
	case "resources/list":
		resources, err := s.ListResources(ctx)
		if err != nil {
			return nil, rpcInternal(err.Error())
		}
		return ListResourcesResult{Resources: resources}, nil
	case "resources/read":
		var request ReadResourceParams
		if err := json.Unmarshal(rawOrEmpty(params), &request); err != nil {
			return nil, rpcInvalidParams("decode resources/read params: " + err.Error())
		}
		contents, err := s.ReadResource(ctx, request.URI)
		if errors.Is(err, ErrResourceNotFound) {
			return nil, rpcInvalidParams(err.Error())
		}
		if err != nil {
			return nil, rpcInternal(err.Error())
		}
		return ReadResourceResult{Contents: contents}, nil
	case "resources/templates/list":
		return map[string]any{"resourceTemplates": []any{}}, nil
	case "prompts/list":
		prompts, err := s.ListPrompts(ctx)
		if err != nil {
			return nil, rpcInternal(err.Error())
		}
		return ListPromptsResult{Prompts: prompts}, nil
	case "prompts/get":
		var request GetPromptParams
		if err := json.Unmarshal(rawOrEmpty(params), &request); err != nil {
			return nil, rpcInvalidParams("decode prompts/get params: " + err.Error())
		}
		result, err := s.GetPrompt(ctx, request.Name)
		if errors.Is(err, ErrPromptNotFound) {
			return nil, rpcInvalidParams(err.Error())
		}
		if err != nil {
			return nil, rpcInternal(err.Error())
		}
		return result, nil
	default:
		return nil, &RPCError{Code: -32601, Message: "method not found"}
	}
}

func (s *Server) HandleNotification(_ context.Context, method string, _ json.RawMessage) *RPCError {
	switch strings.TrimSpace(method) {
	case "notifications/initialized", "notifications/cancelled":
		return nil
	default:
		return nil
	}
}

func rpcInvalidParams(message string) *RPCError {
	return &RPCError{Code: -32602, Message: message}
}

func rpcInternal(message string) *RPCError {
	return &RPCError{Code: -32603, Message: message}
}
