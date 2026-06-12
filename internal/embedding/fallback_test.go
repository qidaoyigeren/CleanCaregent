package embedding

import (
	"context"
	"testing"
)

type staticEmbedder struct {
	name    string
	vectors [][]float32
}

func (e staticEmbedder) Name() string   { return e.name }
func (e staticEmbedder) Dimension() int { return 2 }
func (e staticEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return e.vectors, nil
}

func TestFallbackUsesSecondaryForEmptyPrimaryVector(t *testing.T) {
	value, err := NewFallback(
		staticEmbedder{name: "primary", vectors: [][]float32{{0, 0}}},
		staticEmbedder{name: "secondary", vectors: [][]float32{{1, 0}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := value.Embed(context.Background(), []string{"T20"})
	if err != nil {
		t.Fatal(err)
	}
	if vectors[0][0] != 1 {
		t.Fatalf("vectors = %#v, want fallback vector", vectors)
	}
}
