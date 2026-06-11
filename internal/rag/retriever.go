package rag

import (
	"context"
	"time"
)

type SearchMode string

const (
	SearchDense   SearchMode = "dense"
	SearchKeyword SearchMode = "keyword"
	SearchHybrid  SearchMode = "hybrid"
)

type MetadataFilter struct {
	ProductIDs   []string   `json:"product_ids,omitempty"`
	SKUIDs       []string   `json:"sku_ids,omitempty"`
	Categories   []string   `json:"categories,omitempty"`
	Brands       []string   `json:"brands,omitempty"`
	Models       []string   `json:"models,omitempty"`
	DocTypes     []string   `json:"doc_types,omitempty"`
	IntentTags   []string   `json:"intent_tags,omitempty"`
	Version      string     `json:"version,omitempty"`
	EffectiveAt  *time.Time `json:"effective_at,omitempty"`
	FaultNodeIDs []string   `json:"fault_node_ids,omitempty"`
}

type SearchRequest struct {
	Query       string         `json:"query"`
	Mode        SearchMode     `json:"mode"`
	Filter      MetadataFilter `json:"filter,omitempty"`
	DenseTopK   int            `json:"dense_top_k"`
	KeywordTopK int            `json:"keyword_top_k"`
	RerankTopK  int            `json:"rerank_top_k"`
	MinScore    float64        `json:"min_score"`
	NeedRerank  bool           `json:"need_rerank"`
}

type SearchResult struct {
	ChunkID      string         `json:"chunk_id"`
	DocumentID   string         `json:"document_id"`
	Title        string         `json:"title"`
	Content      string         `json:"content"`
	DenseScore   float64        `json:"dense_score"`
	KeywordScore float64        `json:"keyword_score"`
	FusionScore  float64        `json:"fusion_score"`
	RerankScore  float64        `json:"rerank_score"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type Retriever interface {
	Search(ctx context.Context, req SearchRequest) ([]SearchResult, error)
}
