package reranker

import (
	"context"

	"CleanCaregent/internal/rag"
)

type Reranker interface {
	Rerank(ctx context.Context, query string, documents []rag.SearchResult, topK int) ([]rag.SearchResult, error)
}
