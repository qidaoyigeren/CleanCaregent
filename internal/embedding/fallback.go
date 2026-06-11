package embedding

import (
	"context"
	"fmt"
)

type Fallback struct {
	primary   Embedder
	secondary Embedder
}

func NewFallback(primary, secondary Embedder) (*Fallback, error) {
	if primary.Dimension() != secondary.Dimension() {
		return nil, fmt.Errorf("embedding fallback dimensions differ: %d != %d", primary.Dimension(), secondary.Dimension())
	}
	return &Fallback{primary: primary, secondary: secondary}, nil
}

func (e *Fallback) Name() string {
	return e.primary.Name() + "|fallback:" + e.secondary.Name()
}

func (e *Fallback) Dimension() int {
	return e.primary.Dimension()
}

func (e *Fallback) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors, err := e.primary.Embed(ctx, texts)
	if err == nil {
		return vectors, nil
	}
	fallbackVectors, fallbackErr := e.secondary.Embed(ctx, texts)
	if fallbackErr != nil {
		return nil, fmt.Errorf("primary embedding failed: %v; fallback embedding failed: %w", err, fallbackErr)
	}
	return fallbackVectors, nil
}
