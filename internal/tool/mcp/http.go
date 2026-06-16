package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CleanCaregent/internal/tool"
)

const maxRPCBodyBytes = 4 << 20

type ListToolsResult struct {
	ResultType string `json:"resultType,omitempty"`
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
	TTLMs      int64  `json:"ttlMs,omitempty"`
	CacheScope string `json:"cacheScope,omitempty"`
}

type HTTPHandlerConfig struct {
	APIKey         string
	AllowedOrigins []string
}

func NewHTTPHandler(server *Server, config HTTPHandlerConfig) http.Handler {
	handler := &httpHandler{
		server:         server,
		apiKey:         strings.TrimSpace(config.APIKey),
		allowedOrigins: make(map[string]struct{}, len(config.AllowedOrigins)),
	}
	for _, origin := range config.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "*" {
			handler.allowAnyOrigin = true
			continue
		}
		if origin != "" {
			handler.allowedOrigins[origin] = struct{}{}
		}
	}
	return handler
}

type httpHandler struct {
	server         *Server
	apiKey         string
	allowedOrigins map[string]struct{}
	allowAnyOrigin bool
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		h.handleOptions(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "mcp endpoint accepts POST JSON-RPC requests", http.StatusMethodNotAllowed)
		return
	}
	if h.server == nil {
		writeRPCError(w, nil, -32603, "mcp server is not configured", http.StatusInternalServerError)
		return
	}
	if !h.originAllowed(r.Header.Get("Origin")) {
		writeRPCError(w, nil, -32003, "origin is not allowed", http.StatusForbidden)
		return
	}
	if !h.authenticated(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="mcp"`)
		writeRPCError(w, nil, -32001, "mcp authentication failed", http.StatusUnauthorized)
		return
	}
	var request rpcRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRPCBodyBytes))
	if err := decoder.Decode(&request); err != nil {
		writeRPCError(w, nil, -32700, "parse JSON-RPC request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if request.JSONRPC != "2.0" || len(request.ID) == 0 || strings.TrimSpace(request.Method) == "" {
		writeRPCError(w, request.ID, -32600, "invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	switch request.Method {
	case "tools/list":
		h.handleToolsList(w, r.Context(), request.ID)
	case "tools/call":
		h.handleToolsCall(w, r.Context(), request)
	default:
		writeRPCError(w, request.ID, -32601, "method not found", http.StatusOK)
	}
}

func (h *httpHandler) handleOptions(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if !h.originAllowed(origin) {
		http.Error(w, "origin is not allowed", http.StatusForbidden)
		return
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-MCP-API-Key")
	w.WriteHeader(http.StatusNoContent)
}

func (h *httpHandler) handleToolsList(w http.ResponseWriter, ctx context.Context, id json.RawMessage) {
	tools, err := h.server.ListTools(ctx)
	if err != nil {
		writeRPCError(w, id, -32603, err.Error(), http.StatusOK)
		return
	}
	writeRPCResult(w, id, ListToolsResult{
		ResultType: "complete",
		Tools:      tools,
		TTLMs:      int64((30 * time.Second) / time.Millisecond),
		CacheScope: "public",
	})
}

func (h *httpHandler) handleToolsCall(w http.ResponseWriter, ctx context.Context, request rpcRequest) {
	var params CallToolParams
	if len(request.Params) > 0 {
		if err := json.Unmarshal(request.Params, &params); err != nil {
			writeRPCError(w, request.ID, -32602, "decode tools/call params: "+err.Error(), http.StatusOK)
			return
		}
	}
	if strings.TrimSpace(params.Name) == "" {
		writeRPCError(w, request.ID, -32602, "tools/call params.name is required", http.StatusOK)
		return
	}
	result, err := h.server.CallTool(ctx, params)
	if err != nil {
		writeRPCError(w, request.ID, -32603, err.Error(), http.StatusOK)
		return
	}
	writeRPCResult(w, request.ID, result)
}

func (h *httpHandler) originAllowed(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" || h.allowAnyOrigin || len(h.allowedOrigins) == 0 {
		return true
	}
	_, ok := h.allowedOrigins[origin]
	return ok
}

func (h *httpHandler) authenticated(r *http.Request) bool {
	if h.apiKey == "" {
		return true
	}
	token := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[len("bearer "):])
	}
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("X-MCP-API-Key"))
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.apiKey)) == 1
}

type RemoteClientConfig struct {
	Endpoint       string
	APIKey         string
	Headers        map[string]string
	Timeout        time.Duration
	ListCacheTTL   time.Duration
	MaxRetries     int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
	HTTPClient     *http.Client
}

type RemoteClient struct {
	endpoint       string
	apiKey         string
	headers        map[string]string
	timeout        time.Duration
	listCacheTTL   time.Duration
	maxRetries     int
	retryBaseDelay time.Duration
	retryMaxDelay  time.Duration
	httpClient     *http.Client
	nextID         atomic.Uint64

	cacheMu      sync.RWMutex
	cachedTools  []tool.Definition
	cacheExpires time.Time
}

func NewRemoteClient(config RemoteClientConfig) (*RemoteClient, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		return nil, errors.New("mcp remote endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("mcp remote endpoint must be an absolute URL: %q", endpoint)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("mcp remote endpoint scheme must be http or https: %q", parsed.Scheme)
	}
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}
	if config.ListCacheTTL < 0 {
		config.ListCacheTTL = 0
	}
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = 100 * time.Millisecond
	}
	if config.RetryMaxDelay <= 0 {
		config.RetryMaxDelay = time.Second
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	headers := make(map[string]string, len(config.Headers))
	for key, value := range config.Headers {
		key = strings.TrimSpace(key)
		if key != "" {
			headers[key] = value
		}
	}
	return &RemoteClient{
		endpoint:       endpoint,
		apiKey:         strings.TrimSpace(config.APIKey),
		headers:        headers,
		timeout:        config.Timeout,
		listCacheTTL:   config.ListCacheTTL,
		maxRetries:     config.MaxRetries,
		retryBaseDelay: config.RetryBaseDelay,
		retryMaxDelay:  config.RetryMaxDelay,
		httpClient:     httpClient,
	}, nil
}

func (c *RemoteClient) ListTools(ctx context.Context) ([]tool.Definition, error) {
	if cached, ok := c.cachedToolDefinitions(true); ok {
		return cached, nil
	}
	var response ListToolsResult
	if err := c.rpc(ctx, "tools/list", map[string]any{}, &response); err != nil {
		if cached, ok := c.cachedToolDefinitions(false); ok {
			return cached, nil
		}
		return nil, fmt.Errorf("mcp tools/list remote: %w", err)
	}
	definitions := definitionsFromMCPTools(response.Tools)
	ttl := c.listCacheTTL
	if response.TTLMs > 0 {
		responseTTL := time.Duration(response.TTLMs) * time.Millisecond
		if ttl <= 0 || responseTTL < ttl {
			ttl = responseTTL
		}
	}
	c.storeToolDefinitions(definitions, ttl)
	return definitions, nil
}

func (c *RemoteClient) CallTool(ctx context.Context, call tool.Call) (tool.Result, error) {
	params := CallToolParams{
		Name:      call.Name,
		Arguments: call.Arguments,
		Meta: CallMeta{
			TraceID:        call.TraceID,
			CallID:         call.CallID,
			UserID:         call.UserID,
			ConversationID: call.ConversationID,
			IdempotencyKey: call.IdempotencyKey,
		},
	}
	var response CallToolResult
	if err := c.rpc(ctx, "tools/call", params, &response); err != nil {
		return tool.Result{CallID: call.CallID}, fmt.Errorf("mcp tools/call remote %s: %w", call.Name, err)
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

func (c *RemoteClient) rpc(ctx context.Context, method string, params any, result any) error {
	if c == nil {
		return errors.New("mcp remote client is nil")
	}
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		err := c.rpcOnce(ctx, method, params, result)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == c.maxRetries || !isRetryableRPCError(err) {
			break
		}
		timer := time.NewTimer(c.retryDelay(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func (c *RemoteClient) rpcOnce(ctx context.Context, method string, params any, result any) error {
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode mcp params: %w", err)
	}
	requestID := c.nextID.Add(1)
	payload, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(strconv.FormatUint(requestID, 10)),
		Method:  method,
		Params:  paramsRaw,
	})
	if err != nil {
		return fmt.Errorf("encode mcp request: %w", err)
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create mcp request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body := http.MaxBytesReader(nil, resp.Body, maxRPCBodyBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(body)
		return &httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(raw))}
	}
	envelope, err := decodeRPCResponse(resp.Header.Get("Content-Type"), body)
	if err != nil {
		return err
	}
	if envelope.Error != nil {
		return envelope.Error
	}
	if len(envelope.Result) == 0 {
		return errors.New("mcp response result is empty")
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("decode mcp response result: %w", err)
	}
	return nil
}

func (c *RemoteClient) retryDelay(attempt int) time.Duration {
	delay := c.retryBaseDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay >= c.retryMaxDelay {
			return c.retryMaxDelay
		}
	}
	if delay > c.retryMaxDelay {
		return c.retryMaxDelay
	}
	return delay
}

func (c *RemoteClient) cachedToolDefinitions(requireFresh bool) ([]tool.Definition, bool) {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	if len(c.cachedTools) == 0 {
		return nil, false
	}
	if requireFresh && time.Now().After(c.cacheExpires) {
		return nil, false
	}
	return cloneDefinitions(c.cachedTools), true
}

func (c *RemoteClient) storeToolDefinitions(definitions []tool.Definition, ttl time.Duration) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if ttl <= 0 {
		c.cachedTools = nil
		c.cacheExpires = time.Time{}
		return
	}
	c.cachedTools = cloneDefinitions(definitions)
	c.cacheExpires = time.Now().Add(ttl)
}

func definitionsFromMCPTools(values []Tool) []tool.Definition {
	definitions := make([]tool.Definition, 0, len(values))
	for _, value := range values {
		definitions = append(definitions, tool.Definition{
			Name:         value.Name,
			Description:  value.Description,
			ParamsSchema: append(json.RawMessage(nil), value.InputSchema...),
			SideEffect:   sideEffectFrom(value.Annotations),
		})
	}
	return definitions
}

func cloneDefinitions(values []tool.Definition) []tool.Definition {
	cloned := make([]tool.Definition, 0, len(values))
	for _, value := range values {
		value.ParamsSchema = append(json.RawMessage(nil), value.ParamsSchema...)
		cloned = append(cloned, value)
	}
	return cloned
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("mcp rpc error %d: %s", e.Code, e.Message)
}

type httpStatusError struct {
	StatusCode int
	Body       string
}

func (e *httpStatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("mcp http status %d", e.StatusCode)
	}
	return fmt.Sprintf("mcp http status %d: %s", e.StatusCode, e.Body)
}

func writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	writeJSON(w, http.StatusOK, rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  mustRaw(result),
	})
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string, status int) {
	writeJSON(w, status, rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func mustRaw(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return raw
}

func decodeRPCResponse(contentType string, body io.Reader) (rpcResponse, error) {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "text/event-stream" {
		return decodeSSERPCResponse(body)
	}
	var response rpcResponse
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		return rpcResponse{}, fmt.Errorf("decode mcp JSON-RPC response: %w", err)
	}
	return response, nil
}

func decodeSSERPCResponse(body io.Reader) (rpcResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRPCBodyBytes)
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if data.Len() == 0 {
				continue
			}
			var response rpcResponse
			if err := json.Unmarshal([]byte(data.String()), &response); err == nil && len(response.ID) > 0 {
				return response, nil
			}
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return rpcResponse{}, fmt.Errorf("read mcp SSE response: %w", err)
	}
	if data.Len() > 0 {
		var response rpcResponse
		if err := json.Unmarshal([]byte(data.String()), &response); err == nil && len(response.ID) > 0 {
			return response, nil
		}
	}
	return rpcResponse{}, errors.New("mcp SSE response did not contain JSON-RPC result")
}

func isRetryableRPCError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusRequestTimeout ||
			statusErr.StatusCode == http.StatusTooManyRequests ||
			statusErr.StatusCode >= 500
	}
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		return false
	}
	return true
}
