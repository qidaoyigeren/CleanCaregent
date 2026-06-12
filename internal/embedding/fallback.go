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
	if err == nil && validVectors(vectors, len(texts), e.Dimension()) {
		return vectors, nil
	}
	if err == nil {
		err = fmt.Errorf("primary embedding returned empty or invalid vectors")
	}
	fallbackVectors, fallbackErr := e.secondary.Embed(ctx, texts)
	if fallbackErr != nil {
		return nil, fmt.Errorf("primary embedding failed: %v; fallback embedding failed: %w", err, fallbackErr)
	}
	if !validVectors(fallbackVectors, len(texts), e.Dimension()) {
		return nil, fmt.Errorf("primary embedding failed: %v; fallback embedding returned empty or invalid vectors", err)
	}
	return fallbackVectors, nil
}

func validVectors(vectors [][]float32, expectedCount, dimension int) bool {
	if len(vectors) != expectedCount {
		return false
	}
	for _, vector := range vectors {
		if len(vector) != dimension {
			return false
		}
		nonZero := false
		for _, value := range vector {
			if value != 0 {
				nonZero = true
				break
			}
		}
		if !nonZero {
			return false
		}
	}
	return true
}
