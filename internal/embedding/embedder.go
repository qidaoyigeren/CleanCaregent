package embedding

import "context"

type Embedder interface {
	Name() string
	Dimension() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
