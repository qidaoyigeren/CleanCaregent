package embedding

import (
	"context"
	"fmt"
	"time"

	"CleanCaregent/internal/llm"
)

type CircuitBreakerEmbedder struct {
	next    Embedder
	breaker *llm.CircuitBreaker
}

func WithCircuitBreaker(
	next Embedder,
	failureThreshold int,
	openTimeout time.Duration,
) *CircuitBreakerEmbedder {
	value := &CircuitBreakerEmbedder{
		next:    next,
		breaker: llm.NewCircuitBreaker(failureThreshold, openTimeout),
	}
	llm.DefaultCircuitManager.Register("embedding:"+next.Name(), value.breaker)
	return value
}

func (e *CircuitBreakerEmbedder) Name() string {
	return e.next.Name()
}

func (e *CircuitBreakerEmbedder) Dimension() int {
	return e.next.Dimension()
}

func (e *CircuitBreakerEmbedder) Embed(
	ctx context.Context,
	texts []string,
) (vectors [][]float32, err error) {
	if !e.breaker.Allow() {
		return nil, fmt.Errorf("embedding provider %s: %w", e.next.Name(), llm.ErrCircuitOpen)
	}
	defer func() {
		e.breaker.Record(err == nil)
	}()
	return e.next.Embed(ctx, texts)
}
