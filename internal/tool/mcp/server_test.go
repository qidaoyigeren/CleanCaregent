package mcp

import (
	"context"
	"encoding/json"
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
	attempts := 0
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
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
	if len(definitions) != 1 || attempts != 2 {
		t.Fatalf("definitions = %#v, attempts = %d", definitions, attempts)
	}
}
