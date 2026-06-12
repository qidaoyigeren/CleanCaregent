package reranker

import (
	"context"
	"errors"
	"testing"
	"time"

	"CleanCaregent/internal/rag"
)

type countingReranker struct {
	calls int
	err   error
}

func (r *countingReranker) Rerank(
	_ context.Context,
	_ string,
	documents []rag.SearchResult,
	_ int,
) ([]rag.SearchResult, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return documents, nil
}

func TestCircuitBreakerRerankerSkipsOpenProvider(t *testing.T) {
	primary := &countingReranker{err: errors.New("down")}
	secondary := &countingReranker{}
	fallback := NewFallback(
		WithCircuitBreaker(primary, 1, time.Minute),
		secondary,
	)
	for range 2 {
		if _, err := fallback.Rerank(context.Background(), "T20", []rag.SearchResult{{ChunkID: "1"}}, 1); err != nil {
			t.Fatal(err)
		}
	}
	if primary.calls != 1 || secondary.calls != 2 {
		t.Fatalf("calls primary=%d secondary=%d", primary.calls, secondary.calls)
	}
}
