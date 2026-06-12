package reranker

import (
	"context"
	"fmt"
	"time"

	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/rag"
)

type CircuitBreakerReranker struct {
	next    Reranker
	breaker *llm.CircuitBreaker
}

func WithCircuitBreaker(
	next Reranker,
	failureThreshold int,
	openTimeout time.Duration,
) *CircuitBreakerReranker {
	value := &CircuitBreakerReranker{
		next:    next,
		breaker: llm.NewCircuitBreaker(failureThreshold, openTimeout),
	}
	llm.DefaultCircuitManager.Register("reranker", value.breaker)
	return value
}

func (r *CircuitBreakerReranker) Rerank(
	ctx context.Context,
	query string,
	documents []rag.SearchResult,
	topK int,
) (results []rag.SearchResult, err error) {
	if !r.breaker.Allow() {
		return nil, fmt.Errorf("rerank provider: %w", llm.ErrCircuitOpen)
	}
	defer func() {
		r.breaker.Record(err == nil)
	}()
	return r.next.Rerank(ctx, query, documents, topK)
}
