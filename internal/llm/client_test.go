package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestChatJSONRequestsJSONObjectResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		format, _ := body["response_format"].(map[string]any)
		if format["type"] != "json_object" {
			t.Fatalf("response_format = %#v", body["response_format"])
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{
				map[string]any{"message": map[string]any{"content": `{"ok":true}`}},
			},
			"usage": map[string]any{
				"prompt_tokens":     11,
				"completion_tokens": 7,
				"total_tokens":      18,
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "", "test", 100, 0, time.Second)
	var result struct {
		OK bool `json:"ok"`
	}
	collector := &UsageCollector{}
	ctx := WithUsageCollector(context.Background(), collector)
	if err := client.ChatJSON(ctx, []map[string]string{
		{"role": "user", "content": "return json"},
	}, &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	if usage := collector.Snapshot(); usage.PromptTokens != 11 ||
		usage.CompletionTokens != 7 ||
		usage.Calls != 1 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestChatWithToolsDecodesFunctionCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		tools, _ := body["tools"].([]any)
		if len(tools) != 1 {
			t.Fatalf("tools = %#v", body["tools"])
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{
				map[string]any{"message": map[string]any{
					"content": "",
					"tool_calls": []any{
						map[string]any{
							"id": "call_1",
							"function": map[string]any{
								"name":      "price_query",
								"arguments": `{"product_refs":["T20"]}`,
							},
						},
					},
				}},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "", "test", 100, 0, time.Second)
	_, calls, err := client.ChatWithTools(
		context.Background(),
		[]map[string]string{{"role": "user", "content": "T20多少钱"}},
		[]ToolDefinition{{
			Name:        "price_query",
			Description: "query price",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Name != "price_query" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestClientFallsBackAcrossProviders(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer primary.Close()
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "secondary-model" {
			t.Fatalf("model = %#v", payload["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"fallback answer"}}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`))
	}))
	defer secondary.Close()

	client := NewClient(primary.URL, "", "primary-model", 100, 0, time.Second).
		WithFallbacks(NewClient(secondary.URL, "", "secondary-model", 80, 0.2, time.Second))
	answer, err := client.Chat(context.Background(), []map[string]string{{
		"role": "user", "content": "hello",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "fallback answer" {
		t.Fatalf("answer = %q", answer)
	}
}

func TestClientWithModelUsesOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "intent-model" {
			t.Fatalf("model = %#v", payload["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", "main-model", 100, 0, time.Second)
	answer, err := client.WithModel("intent-model").Chat(
		context.Background(),
		[]map[string]string{{"role": "user", "content": "classify"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "ok" {
		t.Fatalf("answer = %q", answer)
	}
}
