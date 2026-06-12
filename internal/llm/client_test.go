package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestChatStreamFallsBackOnFirstTokenTimeout(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(100 * time.Millisecond)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer primary.Close()

	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["stream"] != true || payload["model"] != "secondary-model" {
			t.Fatalf("payload = %#v", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"fallback \"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"stream\"}}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2,\"total_tokens\":4}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer secondary.Close()

	client := NewClient(primary.URL, "", "primary-model", 100, 0, time.Second).
		WithFirstTokenTimeout(20 * time.Millisecond).
		WithFallbacks(
			NewClient(secondary.URL, "", "secondary-model", 100, 0, time.Second).
				WithFirstTokenTimeout(20 * time.Millisecond),
		)
	collector := &UsageCollector{}
	var answer strings.Builder
	err := client.ChatStream(
		WithUsageCollector(context.Background(), collector),
		[]map[string]string{{"role": "user", "content": "hello"}},
		func(delta string) error {
			answer.WriteString(delta)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if answer.String() != "fallback stream" {
		t.Fatalf("answer = %q", answer.String())
	}
	if usage := collector.Snapshot(); usage.TotalTokens != 4 || usage.Calls != 1 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestChatStreamDoesNotFallbackAfterContent(t *testing.T) {
	var fallbackCalls int
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: not-json\n\n")
	}))
	defer primary.Close()
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackCalls++
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer secondary.Close()

	client := NewClient(primary.URL, "", "primary", 100, 0, time.Second).
		WithFallbacks(NewClient(secondary.URL, "", "secondary", 100, 0, time.Second))
	var answer strings.Builder
	err := client.ChatStream(context.Background(), nil, func(delta string) error {
		answer.WriteString(delta)
		return nil
	})
	if err == nil {
		t.Fatal("ChatStream() expected decode error")
	}
	if answer.String() != "partial" || fallbackCalls != 0 {
		t.Fatalf("answer = %q, fallback calls = %d", answer.String(), fallbackCalls)
	}
}
