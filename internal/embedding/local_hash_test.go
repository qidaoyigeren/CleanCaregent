package embedding

import (
	"context"
	"math"
	"testing"
)

func TestLocalHashIsDeterministicAndNormalized(t *testing.T) {
	embedder := NewLocalHash(64)
	vectors, err := embedder.Embed(context.Background(), []string{
		"T20 吸力 6000Pa",
		"T20 吸力 6000Pa",
	})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 2 || len(vectors[0]) != 64 {
		t.Fatalf("vector shape = %d x %d", len(vectors), len(vectors[0]))
	}
	var norm float64
	for index := range vectors[0] {
		if vectors[0][index] != vectors[1][index] {
			t.Fatalf("vectors differ at %d", index)
		}
		norm += float64(vectors[0][index] * vectors[0][index])
	}
	if math.Abs(norm-1) > 0.0001 {
		t.Fatalf("vector norm squared = %f", norm)
	}
}
