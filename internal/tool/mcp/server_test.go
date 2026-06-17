package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"CleanCaregent/internal/tool"
)

type fakeTool struct{}

func (fakeTool) Name() string        { return "price_query" }
func (fakeTool) Description() string { return "fake price query" }
func (fakeTool) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","required":["product_refs"],"properties":{"product_refs":{"type":"array"}}}`)
}
func (fakeTool) SideEffect() tool.SideEffect { return tool.SideEffectReadOnly }
func (fakeTool) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	return tool.Result{
		CallID: call.CallID,
		Data: map[string]any{"items": []any{map[string]any{
			"sku_code":                    "SKU-T20",
			"model":                       "T20",
			"list_price_cents":            399900,
			"current_price_cents":         359900,
			"estimated_final_price_cents": 349900,
			"currency":                    "CNY",
		}}},
	}, nil
}

func TestInProcessClientListsAndCallsTools(t *testing.T) {
	server, err := NewServer(fakeTool{})
	if err != nil {
		t.Fatal(err)
	}
	client := NewInProcessClient(server)

	definitions, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].Name != "price_query" {
		t.Fatalf("definitions = %#v", definitions)
	}
	if definitions[0].SideEffect != tool.SideEffectReadOnly {
		t.Fatalf("side effect = %q", definitions[0].SideEffect)
	}

	result, err := client.CallTool(context.Background(), tool.Call{
		CallID:    "call_mcp",
		Name:      "price_query",
		Arguments: map[string]any{"product_refs": []string{"T20"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CallID != "call_mcp" || result.Data == nil {
		t.Fatalf("result = %#v", result)
	}
}

func TestRemoteClientListsAndCallsHTTPToolsWithAuth(t *testing.T) {
	server, err := NewServer(fakeTool{})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(NewHTTPHandler(server, HTTPHandlerConfig{APIKey: "secret"}))
	defer httpServer.Close()

	client, err := NewRemoteClient(RemoteClientConfig{
		Endpoint:       httpServer.URL,
		APIKey:         "secret",
		Timeout:        time.Second,
		ListCacheTTL:   time.Second,
		RetryBaseDelay: time.Millisecond,
		RetryMaxDelay:  time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	definitions, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].Name != "price_query" {
		t.Fatalf("definitions = %#v", definitions)
	}

	result, err := client.CallTool(context.Background(), tool.Call{
		CallID:    "call_remote_mcp",
		Name:      "price_query",
		Arguments: map[string]any{"product_refs": []string{"T20"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CallID != "call_remote_mcp" || result.Data == nil {
		t.Fatalf("result = %#v", result)
	}
}

func TestHTTPHandlerRejectsMissingAuth(t *testing.T) {
	server, err := NewServer(fakeTool{})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHTTPHandler(server, HTTPHandlerConfig{APIKey: "secret"})
	request := httptest.NewRequest(
		http.MethodPost,
		"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`),
	)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestRemoteClientRetriesTransientHTTPFailure(t *testing.T) {
	server, err := NewServer(fakeTool{})
	if err != nil {
		t.Fatal(err)
	}
	mcpHandler := NewHTTPHandler(server, HTTPHandlerConfig{})
	listAttempts := 0
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var request rpcRequest
		_ = json.Unmarshal(raw, &request)
		if request.Method == "tools/list" {
			listAttempts++
		}
		if request.Method == "tools/list" && listAttempts == 1 {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()

	client, err := NewRemoteClient(RemoteClientConfig{
		Endpoint:       httpServer.URL,
		Timeout:        time.Second,
		MaxRetries:     1,
		RetryBaseDelay: time.Millisecond,
		RetryMaxDelay:  time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || listAttempts != 2 {
		t.Fatalf("definitions = %#v, listAttempts = %d", definitions, listAttempts)
	}
}

func TestRemoteClientInitializesSessionAndReadsSSEResponse(t *testing.T) {
	server, err := NewServer(fakeTool{})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(NewHTTPHandler(server, HTTPHandlerConfig{
		RequireSession:  true,
		StreamResponses: true,
	}))
	defer httpServer.Close()

	client, err := NewRemoteClient(RemoteClientConfig{
		Endpoint:       httpServer.URL,
		Timeout:        time.Second,
		RetryBaseDelay: time.Millisecond,
		RetryMaxDelay:  time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].Name != "price_query" {
		t.Fatalf("definitions = %#v", definitions)
	}
}

func TestRemoteClientWatchesNotificationsOverSSE(t *testing.T) {
	server, err := NewServer(fakeTool{})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(NewHTTPHandler(server, HTTPHandlerConfig{
		RequireSession: true,
	}))
	defer httpServer.Close()
	client, err := NewRemoteClient(RemoteClientConfig{
		Endpoint:       httpServer.URL,
		Timeout:        time.Second,
		RetryBaseDelay: time.Millisecond,
		RetryMaxDelay:  time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	received := make(chan string, 1)
	go func() {
		_ = client.WatchNotifications(ctx, func(notification Notification) {
			received <- notification.Method
			cancel()
		})
	}()
	time.Sleep(25 * time.Millisecond)
	if err := server.AddResource(Resource{URI: "cleancare://test", Name: "test"}, ResourceContent{Text: "test"}); err != nil {
		t.Fatal(err)
	}
	select {
	case method := <-received:
		if method != "notifications/resources/list_changed" {
			t.Fatalf("notification = %q", method)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for SSE notification")
	}
}

func TestServeStdioInitializesAndListsTools(t *testing.T) {
	server, err := NewServer(fakeTool{})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		"",
	}, "\n"))
	var output bytes.Buffer
	if err := ServeStdio(context.Background(), server, input, &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdio responses = %q", output.String())
	}
	var initialize rpcResponse
	if err := json.Unmarshal([]byte(lines[0]), &initialize); err != nil {
		t.Fatal(err)
	}
	var initializeResult InitializeResult
	if err := json.Unmarshal(initialize.Result, &initializeResult); err != nil {
		t.Fatal(err)
	}
	if initializeResult.ProtocolVersion != ProtocolVersion || initializeResult.Capabilities.Tools == nil {
		t.Fatalf("initialize result = %#v", initializeResult)
	}
	var list rpcResponse
	if err := json.Unmarshal([]byte(lines[1]), &list); err != nil {
		t.Fatal(err)
	}
	var listResult ListToolsResult
	if err := json.Unmarshal(list.Result, &listResult); err != nil {
		t.Fatal(err)
	}
	if len(listResult.Tools) != 1 || listResult.Tools[0].Name != "price_query" {
		t.Fatalf("list result = %#v", listResult)
	}
}

func TestServerResourcesAndPrompts(t *testing.T) {
	server, err := NewServer(fakeTool{})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.AddResource(Resource{
		URI:      "cleancare://catalog",
		Name:     "catalog",
		MimeType: "application/json",
	}, ResourceContent{Text: `{"ok":true}`}); err != nil {
		t.Fatal(err)
	}
	if err := server.AddPrompt(Prompt{Name: "answer"}, GetPromptResult{
		Messages: []PromptMessage{{Role: "user", Content: PromptContent{Type: "text", Text: "answer with evidence"}}},
	}); err != nil {
		t.Fatal(err)
	}
	capabilities := server.Capabilities()
	if capabilities.Resources == nil || capabilities.Prompts == nil {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	resources, err := server.ListResources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].URI != "cleancare://catalog" {
		t.Fatalf("resources = %#v", resources)
	}
	prompt, err := server.GetPrompt(context.Background(), "answer")
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt.Messages) != 1 {
		t.Fatalf("prompt = %#v", prompt)
	}
}

func TestAggregateClientPrefixesMultiServerTools(t *testing.T) {
	serverA, err := NewServer(fakeTool{})
	if err != nil {
		t.Fatal(err)
	}
	serverB, err := NewServer(fakeTool{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewAggregateClient([]NamedClient{
		{Name: "a", Client: NewInProcessClient(serverA)},
		{Name: "b", Client: NewInProcessClient(serverB)},
	})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 2 || definitions[0].Name != "a/price_query" || definitions[1].Name != "b/price_query" {
		t.Fatalf("definitions = %#v", definitions)
	}
	result, err := client.CallTool(context.Background(), tool.Call{
		CallID:    "call_aggregate",
		Name:      "b/price_query",
		Arguments: map[string]any{"product_refs": []string{"T20"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CallID != "call_aggregate" || result.Data == nil {
		t.Fatalf("result = %#v", result)
	}
}
