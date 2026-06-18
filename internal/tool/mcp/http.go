package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
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

const (
	maxRPCBodyBytes       = 4 << 20
	defaultHTTPSessionTTL = 30 * time.Minute
)

type ListToolsResult struct {
	ResultType string `json:"resultType,omitempty"`
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
	TTLMs      int64  `json:"ttlMs,omitempty"`
	CacheScope string `json:"cacheScope,omitempty"`
}

type HTTPHandlerConfig struct {
	APIKey               string
	AllowedOrigins       []string
	StreamResponses      bool
	RequireSession       bool
	SessionTTL           time.Duration
	AuthorizationServers []string
	Scopes               []string
}

func NewHTTPHandler(server *Server, config HTTPHandlerConfig) http.Handler {
	handler := &httpHandler{
		server:               server,
		apiKey:               strings.TrimSpace(config.APIKey),
		allowedOrigins:       make(map[string]struct{}, len(config.AllowedOrigins)),
		streamResponses:      config.StreamResponses,
		requireSession:       config.RequireSession,
		sessionTTL:           config.SessionTTL,
		authorizationServers: cleanStrings(config.AuthorizationServers),
		scopes:               cleanStrings(config.Scopes),
		sessions:             make(map[string]*SessionInfo),
	}
	if handler.sessionTTL <= 0 {
		handler.sessionTTL = defaultHTTPSessionTTL
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
	server               *Server
	apiKey               string
	allowedOrigins       map[string]struct{}
	allowAnyOrigin       bool
	streamResponses      bool
	requireSession       bool
	sessionTTL           time.Duration
	authorizationServers []string
	scopes               []string

	sessionMu sync.RWMutex
	sessions  map[string]*SessionInfo
	nextEvent atomic.Uint64
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.cleanupExpiredSessions(time.Now().UTC())
	if r.Method == http.MethodOptions {
		h.handleOptions(w, r)
		return
	}
	if r.Method == http.MethodGet {
		h.handleSSEStream(w, r)
		return
	}
	if r.Method == http.MethodDelete {
		h.handleSessionDelete(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, GET, DELETE, OPTIONS")
		http.Error(w, "mcp endpoint accepts POST JSON-RPC requests and GET SSE streams", http.StatusMethodNotAllowed)
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
		w.Header().Set("WWW-Authenticate", h.wwwAuthenticate(r))
		writeRPCError(w, nil, -32001, "mcp authentication failed", http.StatusUnauthorized)
		return
	}
	var request rpcRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRPCBodyBytes))
	if err := decoder.Decode(&request); err != nil {
		writeRPCError(w, nil, -32700, "parse JSON-RPC request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if request.JSONRPC != "2.0" || strings.TrimSpace(request.Method) == "" {
		writeRPCError(w, request.ID, -32600, "invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	if len(request.ID) == 0 {
		if err := h.validateLifecycle(r, request.Method, false); err != nil {
			writeRPCError(w, nil, err.Code, err.Message, h.lifecycleStatus(r, err))
			return
		}
		if err := h.server.HandleNotification(r.Context(), request.Method, request.Params); err != nil {
			writeRPCError(w, nil, err.Code, err.Message, http.StatusBadRequest)
			return
		}
		if request.Method == "notifications/initialized" {
			h.markSessionInitialized(r)
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if err := h.validateLifecycle(r, request.Method, true); err != nil {
		writeRPCError(w, request.ID, err.Code, err.Message, h.lifecycleStatus(r, err))
		return
	}
	result, rpcErr := h.server.HandleRequest(r.Context(), request.Method, request.Params)
	if rpcErr != nil {
		writeRPCError(w, request.ID, rpcErr.Code, rpcErr.Message, http.StatusOK)
		return
	}
	if request.Method == "initialize" {
		session := h.createSession(result, request.Params)
		w.Header().Set("Mcp-Session-Id", session.ID)
		w.Header().Set("MCP-Protocol-Version", session.ProtocolVersion)
	}
	if h.streamResponses && accepts(r, "text/event-stream") {
		writeRPCResultSSE(w, request.ID, result, h.nextEventID())
		return
	}
	writeRPCResult(w, request.ID, result)
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
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-MCP-API-Key, Mcp-Session-Id, MCP-Protocol-Version, Last-Event-ID")
	w.WriteHeader(http.StatusNoContent)
}

func (h *httpHandler) handleSSEStream(w http.ResponseWriter, r *http.Request) {
	if h.server == nil {
		writeRPCError(w, nil, -32603, "mcp server is not configured", http.StatusInternalServerError)
		return
	}
	if !h.originAllowed(r.Header.Get("Origin")) {
		writeRPCError(w, nil, -32003, "origin is not allowed", http.StatusForbidden)
		return
	}
	if !h.authenticated(r) {
		w.Header().Set("WWW-Authenticate", h.wwwAuthenticate(r))
		writeRPCError(w, nil, -32001, "mcp authentication failed", http.StatusUnauthorized)
		return
	}
	if h.requireSession {
		session, ok := h.sessionFromRequest(r)
		if !ok || !session.Initialized {
			writeRPCError(w, nil, -32004, "mcp session is not initialized", http.StatusBadRequest)
			return
		}
	}
	if !accepts(r, "text/event-stream") {
		http.Error(w, "mcp SSE stream requires Accept: text/event-stream", http.StatusNotAcceptable)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	lastEventID, replayRequested := parseLastEventID(r.Header.Get("Last-Event-ID"))
	notifications, cancel := h.server.SubscribeNotifications()
	defer cancel()
	fmt.Fprintf(w, "retry: 1000\n")
	fmt.Fprintf(w, "data:\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	lastSentID := lastEventID
	if replayRequested {
		for _, event := range h.server.ReplayNotifications(lastEventID) {
			writeSSENotification(w, strconv.FormatUint(event.ID, 10), event.Notification)
			lastSentID = maxUint64(lastSentID, event.ID)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-notifications:
			if !ok {
				return
			}
			if event.ID <= lastSentID {
				continue
			}
			writeSSENotification(w, strconv.FormatUint(event.ID, 10), event.Notification)
			lastSentID = event.ID
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func (h *httpHandler) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	if !h.authenticated(r) {
		w.Header().Set("WWW-Authenticate", h.wwwAuthenticate(r))
		writeRPCError(w, nil, -32001, "mcp authentication failed", http.StatusUnauthorized)
		return
	}
	sessionID := strings.TrimSpace(r.Header.Get("Mcp-Session-Id"))
	if sessionID == "" {
		writeRPCError(w, nil, -32004, "Mcp-Session-Id is required", http.StatusBadRequest)
		return
	}
	h.sessionMu.Lock()
	delete(h.sessions, sessionID)
	h.sessionMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (h *httpHandler) validateLifecycle(r *http.Request, method string, isRequest bool) *RPCError {
	if method == "initialize" || method == "ping" {
		return nil
	}
	if !h.requireSession {
		if session, ok := h.sessionFromRequest(r); ok {
			h.validateProtocolHeader(r, session)
		}
		return nil
	}
	session, ok := h.sessionFromRequest(r)
	if !ok {
		return &RPCError{Code: -32004, Message: "Mcp-Session-Id is required after initialize"}
	}
	if session.ProtocolVersion != "" {
		if err := h.validateProtocolHeader(r, session); err != nil {
			return err
		}
	}
	if !session.Initialized && method != "notifications/initialized" {
		if isRequest {
			return &RPCError{Code: -32004, Message: "mcp session is not initialized"}
		}
	}
	return nil
}

func (h *httpHandler) lifecycleStatus(r *http.Request, err *RPCError) int {
	if err != nil && err.Code == -32004 && strings.TrimSpace(r.Header.Get("Mcp-Session-Id")) != "" {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

func (h *httpHandler) validateProtocolHeader(r *http.Request, session *SessionInfo) *RPCError {
	if session == nil || session.ProtocolVersion == "" {
		return nil
	}
	version := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version"))
	if version == "" {
		return &RPCError{Code: -32005, Message: "MCP-Protocol-Version header is required"}
	}
	if version != session.ProtocolVersion {
		return &RPCError{Code: -32005, Message: "MCP-Protocol-Version header does not match initialized session"}
	}
	return nil
}

func (h *httpHandler) sessionFromRequest(r *http.Request) (*SessionInfo, bool) {
	sessionID := strings.TrimSpace(r.Header.Get("Mcp-Session-Id"))
	if sessionID == "" {
		return nil, false
	}
	h.sessionMu.Lock()
	defer h.sessionMu.Unlock()
	session, ok := h.sessions[sessionID]
	if !ok {
		return nil, false
	}
	now := time.Now().UTC()
	if h.sessionExpired(session, now) {
		delete(h.sessions, sessionID)
		return nil, false
	}
	session.LastSeenAt = now
	return session, true
}

func (h *httpHandler) createSession(result any, params json.RawMessage) *SessionInfo {
	session := &SessionInfo{
		ID:              newSessionID(),
		ProtocolVersion: ProtocolVersion,
		CreatedAt:       time.Now().UTC(),
		LastSeenAt:      time.Now().UTC(),
	}
	if initialized, ok := result.(InitializeResult); ok {
		session.ProtocolVersion = initialized.ProtocolVersion
	}
	var request InitializeParams
	if err := json.Unmarshal(rawOrEmpty(params), &request); err == nil {
		session.ClientInfo = request.ClientInfo
	}
	h.sessionMu.Lock()
	h.sessions[session.ID] = session
	h.sessionMu.Unlock()
	return session
}

func (h *httpHandler) markSessionInitialized(r *http.Request) {
	sessionID := strings.TrimSpace(r.Header.Get("Mcp-Session-Id"))
	if sessionID == "" {
		return
	}
	h.sessionMu.Lock()
	defer h.sessionMu.Unlock()
	if session, ok := h.sessions[sessionID]; ok {
		session.Initialized = true
		session.LastSeenAt = time.Now().UTC()
	}
}

func (h *httpHandler) cleanupExpiredSessions(now time.Time) {
	if h == nil || h.sessionTTL <= 0 {
		return
	}
	h.sessionMu.Lock()
	defer h.sessionMu.Unlock()
	for id, session := range h.sessions {
		if h.sessionExpired(session, now) {
			delete(h.sessions, id)
		}
	}
}

func (h *httpHandler) sessionExpired(session *SessionInfo, now time.Time) bool {
	if h == nil || h.sessionTTL <= 0 || session == nil || session.LastSeenAt.IsZero() {
		return false
	}
	return now.Sub(session.LastSeenAt) > h.sessionTTL
}

func (h *httpHandler) wwwAuthenticate(r *http.Request) string {
	if len(h.authorizationServers) == 0 {
		return `Bearer realm="mcp"`
	}
	return fmt.Sprintf(`Bearer realm="mcp", resource_metadata=%q, scope=%q`, protectedResourceMetadataURL(r), strings.Join(h.scopes, " "))
}

func (h *httpHandler) nextEventID() string {
	return strconv.FormatUint(h.nextEvent.Add(1), 10)
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

type ProtectedResourceMetadataConfig struct {
	Resource             string   `json:"-"`
	AuthorizationServers []string `json:"-"`
	Scopes               []string `json:"-"`
	ResourceName         string   `json:"-"`
}

type ProtectedResourceMetadata struct {
	Resource                 string   `json:"resource"`
	ResourceName             string   `json:"resource_name,omitempty"`
	AuthorizationServers     []string `json:"authorization_servers,omitempty"`
	ScopesSupported          []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported   []string `json:"bearer_methods_supported,omitempty"`
	ResourceDocumentation    string   `json:"resource_documentation,omitempty"`
	ResourceSigningAlgValues []string `json:"resource_signing_alg_values_supported,omitempty"`
}

func NewProtectedResourceMetadataHandler(config ProtectedResourceMetadataConfig) http.Handler {
	metadata := ProtectedResourceMetadata{
		Resource:               strings.TrimSpace(config.Resource),
		ResourceName:           strings.TrimSpace(config.ResourceName),
		AuthorizationServers:   cleanStrings(config.AuthorizationServers),
		ScopesSupported:        cleanStrings(config.Scopes),
		BearerMethodsSupported: []string{"header"},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		value := metadata
		if value.Resource == "" {
			value.Resource = requestBaseURL(r) + "/mcp"
		}
		writeJSON(w, http.StatusOK, value)
	})
}

func protectedResourceMetadataURL(r *http.Request) string {
	return requestBaseURL(r) + "/.well-known/oauth-protected-resource"
}

func requestBaseURL(r *http.Request) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := r.Host
	if forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	return scheme + "://" + host
}

func accepts(r *http.Request, mediaType string) bool {
	header := r.Header.Get("Accept")
	if header == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		value := strings.TrimSpace(strings.Split(part, ";")[0])
		if value == mediaType || value == "*/*" {
			return true
		}
	}
	return false
}

func parseLastEventID(value string) (uint64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func maxUint64(left, right uint64) uint64 {
	if left > right {
		return left
	}
	return right
}

func cleanStrings(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func newSessionID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
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

	lifecycleMu     sync.Mutex
	initialized     bool
	protocolVersion string
	sessionID       string
	serverInfo      ImplementationInfo
	serverCaps      ServerCapabilities

	cacheMu      sync.RWMutex
	cachedTools  []tool.Definition
	cacheExpires time.Time

	notificationMu          sync.Mutex
	lastNotificationEventID string
}

type NotificationHandler func(Notification)

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
		endpoint:        endpoint,
		apiKey:          strings.TrimSpace(config.APIKey),
		headers:         headers,
		timeout:         config.Timeout,
		listCacheTTL:    config.ListCacheTTL,
		maxRetries:      config.MaxRetries,
		retryBaseDelay:  config.RetryBaseDelay,
		retryMaxDelay:   config.RetryMaxDelay,
		httpClient:      httpClient,
		protocolVersion: ProtocolVersion,
	}, nil
}

func (c *RemoteClient) ListTools(ctx context.Context) ([]tool.Definition, error) {
	if err := c.ensureInitialized(ctx); err != nil {
		return nil, err
	}
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
	if err := c.ensureInitialized(ctx); err != nil {
		return tool.Result{CallID: call.CallID}, err
	}
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

func (c *RemoteClient) ensureInitialized(ctx context.Context) error {
	if c == nil {
		return errors.New("mcp remote client is nil")
	}
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.initialized {
		return nil
	}
	c.sessionID = ""
	c.protocolVersion = ProtocolVersion
	var response InitializeResult
	if err := c.rpc(ctx, "initialize", InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    ClientCapabilities{},
		ClientInfo: ImplementationInfo{
			Name:        "cleancare-agent",
			Title:       "CleanCare Agent",
			Version:     "1.0.0",
			Description: "CleanCare Agent MCP tool client",
		},
	}, &response); err != nil {
		return fmt.Errorf("mcp initialize remote: %w", err)
	}
	if strings.TrimSpace(response.ProtocolVersion) == "" {
		response.ProtocolVersion = ProtocolVersion
	}
	c.protocolVersion = response.ProtocolVersion
	c.serverInfo = response.ServerInfo
	c.serverCaps = response.Capabilities
	if err := c.notify(ctx, "notifications/initialized", nil); err != nil {
		return fmt.Errorf("mcp initialized notification remote: %w", err)
	}
	c.initialized = true
	return nil
}

func (c *RemoteClient) resetLifecycle() {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	c.initialized = false
	c.sessionID = ""
	c.clearToolDefinitions()
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
		if isSessionExpired(err) {
			c.resetLifecycle()
			if method != "initialize" {
				if initErr := c.ensureInitialized(ctx); initErr != nil {
					return initErr
				}
				err = c.rpcOnce(ctx, method, params, result)
				if err == nil {
					return nil
				}
			}
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

func (c *RemoteClient) notify(ctx context.Context, method string, params any) error {
	if c == nil {
		return errors.New("mcp remote client is nil")
	}
	return c.notifyOnce(ctx, method, params)
}

func (c *RemoteClient) WatchNotifications(ctx context.Context, handler NotificationHandler) error {
	if c == nil {
		return errors.New("mcp remote client is nil")
	}
	if handler == nil {
		return errors.New("mcp notification handler is nil")
	}
	for attempt := 0; ; attempt++ {
		if err := c.ensureInitialized(ctx); err != nil {
			return err
		}
		err := c.watchNotificationsOnce(ctx, handler)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if isSessionExpired(err) {
			c.resetLifecycle()
		}
		timer := time.NewTimer(c.retryDelay(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *RemoteClient) watchNotificationsOnce(ctx context.Context, handler NotificationHandler) error {
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return fmt.Errorf("create mcp notification stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if lastID := c.lastEventID(); lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	if c.sessionID != "" && c.protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", c.protocolVersion)
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
	if mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mediaType != "text/event-stream" {
		return fmt.Errorf("mcp notification stream returned content-type %q", resp.Header.Get("Content-Type"))
	}
	return readSSENotifications(body, handler, c.storeLastEventID)
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
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	if c.sessionID != "" && c.protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", c.protocolVersion)
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
	if sessionID := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); sessionID != "" {
		c.sessionID = sessionID
	}
	if version := strings.TrimSpace(resp.Header.Get("MCP-Protocol-Version")); version != "" {
		c.protocolVersion = version
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

func (c *RemoteClient) notifyOnce(ctx context.Context, method string, params any) error {
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	request := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
	}
	if params != nil {
		request.Params = mustRaw(params)
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode mcp notification: %w", err)
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create mcp notification: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	if c.sessionID != "" && c.protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", c.protocolVersion)
	}
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(http.MaxBytesReader(nil, resp.Body, maxRPCBodyBytes))
		return &httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(raw))}
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

func (c *RemoteClient) clearToolDefinitions() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.cachedTools = nil
	c.cacheExpires = time.Time{}
}

func (c *RemoteClient) ClearToolDefinitions() {
	c.clearToolDefinitions()
}

func (c *RemoteClient) lastEventID() string {
	c.notificationMu.Lock()
	defer c.notificationMu.Unlock()
	return c.lastNotificationEventID
}

func (c *RemoteClient) storeLastEventID(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	c.notificationMu.Lock()
	c.lastNotificationEventID = id
	c.notificationMu.Unlock()
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

func writeRPCResultSSE(w http.ResponseWriter, id json.RawMessage, result any, eventID string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	writeSSEPayload(w, eventID, rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  mustRaw(result),
	})
}

func writeSSENotification(w io.Writer, eventID string, notification Notification) {
	payload := rpcRequest{
		JSONRPC: "2.0",
		Method:  notification.Method,
	}
	if notification.Params != nil {
		payload.Params = mustRaw(notification.Params)
	}
	writeSSEPayload(w, eventID, payload)
}

func writeSSEPayload(w io.Writer, eventID string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if eventID != "" {
		fmt.Fprintf(w, "id: %s\n", eventID)
	}
	fmt.Fprintf(w, "event: message\n")
	fmt.Fprintf(w, "data: %s\n\n", raw)
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

func readSSENotifications(body io.Reader, handler NotificationHandler, eventIDHandler func(string)) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRPCBodyBytes)
	var data strings.Builder
	var eventID string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if data.Len() == 0 {
				eventID = ""
				continue
			}
			var request rpcRequest
			if err := json.Unmarshal([]byte(data.String()), &request); err == nil &&
				len(request.ID) == 0 &&
				request.JSONRPC == "2.0" &&
				strings.TrimSpace(request.Method) != "" {
				var params any
				if len(request.Params) > 0 {
					params = json.RawMessage(append([]byte(nil), request.Params...))
				}
				if eventIDHandler != nil {
					eventIDHandler(eventID)
				}
				handler(Notification{Method: request.Method, Params: params})
			}
			data.Reset()
			eventID = ""
			continue
		}
		if strings.HasPrefix(line, "id:") {
			eventID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
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
		return fmt.Errorf("read mcp SSE notifications: %w", err)
	}
	return io.EOF
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

func isSessionExpired(err error) bool {
	var statusErr *httpStatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound
}
