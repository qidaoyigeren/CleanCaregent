package qdrant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"CleanCaregent/internal/config"
	"CleanCaregent/internal/vectorstore"
)

func TestEnsureCollectionAndSearch(t *testing.T) {
	var collectionCreated bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "secret" {
			t.Fatalf("api-key header = %q", r.Header.Get("api-key"))
		}
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/collections/test_collection":
			if !collectionCreated {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"result":{"status":"green"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/collections/test_collection":
			var body struct {
				Vectors struct {
					Size     int    `json:"size"`
					Distance string `json:"distance"`
				} `json:"vectors"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create collection: %v", err)
			}
			if body.Vectors.Size != 3 || body.Vectors.Distance != "Cosine" {
				t.Fatalf("create collection body = %#v", body)
			}
			collectionCreated = true
			_, _ = w.Write([]byte(`{"result":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/collections/test_collection/points/search":
			_, _ = w.Write([]byte(`{"result":[{"id":"point-1","score":0.91,"payload":{"chunk_id":"chunk-1"}}]}`))
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewClient(config.QdrantConfig{
		BaseURL:        server.URL,
		APIKey:         "secret",
		Collection:     "test_collection",
		VectorSize:     3,
		Distance:       "cosine",
		RequestTimeout: time.Second,
	})
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if err := client.EnsureCollection(context.Background()); err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}
	if !collectionCreated {
		t.Fatal("collection was not created")
	}

	results, err := client.Search(context.Background(), vectorstore.SearchRequest{
		Vector: []float32{0.1, 0.2, 0.3},
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 || results[0].Payload["chunk_id"] != "chunk-1" {
		t.Fatalf("results = %#v", results)
	}
}

func TestUpsertRejectsWrongVectorSize(t *testing.T) {
	client := NewClient(config.QdrantConfig{
		BaseURL:        "http://127.0.0.1",
		Collection:     "test",
		VectorSize:     3,
		RequestTimeout: time.Second,
	})
	err := client.Upsert(context.Background(), []vectorstore.Point{{ID: "point-1", Vector: []float32{0.1}}})
	if err == nil || !strings.Contains(err.Error(), "vector size") {
		t.Fatalf("Upsert() error = %v", err)
	}
}

func TestDeletePoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost ||
			r.URL.Path != "/collections/test_collection/points/delete" ||
			r.URL.Query().Get("wait") != "true" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var body struct {
			Points []string `json:"points"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Points) != 2 || body.Points[0] != "point-1" {
			t.Fatalf("points = %#v", body.Points)
		}
		_, _ = w.Write([]byte(`{"result":{"status":"completed"}}`))
	}))
	defer server.Close()

	client := NewClient(config.QdrantConfig{
		BaseURL:        server.URL,
		Collection:     "test_collection",
		VectorSize:     3,
		RequestTimeout: time.Second,
	})
	if err := client.Delete(context.Background(), []string{"point-1", "point-2"}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}
