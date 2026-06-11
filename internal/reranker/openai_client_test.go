package reranker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"CleanCaregent/internal/rag"
)

func TestOpenAIClientReranksByRemoteScore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"index": 1, "relevance_score": 0.95},
				{"index": 0, "relevance_score": 0.41}
			]
		}`))
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "test-key", "bge-reranker", time.Second)
	result, err := client.Rerank(context.Background(), "养猫", []rag.SearchResult{
		{ChunkID: "c1", Content: "普通清扫"},
		{ChunkID: "c2", Content: "宠物毛发防缠绕"},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result[0].ChunkID != "c2" || result[0].RerankScore != 0.95 {
		t.Fatalf("result = %#v", result)
	}
	if result[0].Metadata["rerank_provider"] != "bge-reranker" {
		t.Fatalf("metadata = %#v", result[0].Metadata)
	}
}

func TestFallbackUsesLocalReranker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	fallback := NewFallback(
		NewOpenAIClient(server.URL, "", "remote", time.Second),
		NewLocalLexical(),
	)
	result, err := fallback.Rerank(context.Background(), "宠物毛发", []rag.SearchResult{
		{ChunkID: "c1", Content: "基础清扫"},
		{ChunkID: "c2", Content: "宠物毛发防缠绕"},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].ChunkID != "c2" {
		t.Fatalf("result = %#v", result)
	}
	if result[0].Metadata["rerank_fallback"] != true {
		t.Fatalf("metadata = %#v", result[0].Metadata)
	}
}
