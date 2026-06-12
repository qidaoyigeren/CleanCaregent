package embedding

import (
	"context"
	"errors"
	"testing"
	"time"
)

type countingEmbedder struct {
	calls int
	err   error
}

func (e *countingEmbedder) Name() string   { return "counting" }
func (e *countingEmbedder) Dimension() int { return 2 }
func (e *countingEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	e.calls++
	if e.err != nil {
		return nil, e.err
	}
	return [][]float32{{1, 0}}, nil
}

func TestCircuitBreakerEmbedderSkipsOpenProvider(t *testing.T) {
	primary := &countingEmbedder{err: errors.New("down")}
	secondary := &countingEmbedder{}
	fallback, err := NewFallback(
		WithCircuitBreaker(primary, 1, time.Minute),
		secondary,
	)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := fallback.Embed(context.Background(), []string{"T20"}); err != nil {
			t.Fatal(err)
		}
	}
	if primary.calls != 1 || secondary.calls != 2 {
		t.Fatalf("calls primary=%d secondary=%d", primary.calls, secondary.calls)
	}
}
