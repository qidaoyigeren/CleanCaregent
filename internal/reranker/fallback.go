package reranker

import (
	"context"

	"CleanCaregent/internal/rag"
)

type Fallback struct {
	primary   Reranker
	secondary Reranker
}

func NewFallback(primary, secondary Reranker) *Fallback {
	return &Fallback{primary: primary, secondary: secondary}
}

func (r *Fallback) Rerank(
	ctx context.Context,
	query string,
	documents []rag.SearchResult,
	topK int,
) ([]rag.SearchResult, error) {
	result, err := r.primary.Rerank(ctx, query, documents, topK)
	if err == nil {
		return result, nil
	}
	result, fallbackErr := r.secondary.Rerank(ctx, query, documents, topK)
	if fallbackErr != nil {
		return nil, fallbackErr
	}
	for index := range result {
		if result[index].Metadata == nil {
			result[index].Metadata = map[string]any{}
		}
		result[index].Metadata["rerank_fallback"] = true
	}
	return result, nil
}

var _ Reranker = (*Fallback)(nil)
