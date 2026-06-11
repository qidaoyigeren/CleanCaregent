package model

import "time"

const (
	KnowledgeStatusIndexing   = "indexing"
	KnowledgeStatusActive     = "active"
	KnowledgeStatusSuperseded = "superseded"
	KnowledgeStatusFailed     = "failed"
)

type KnowledgeDocument struct {
	ID            int64          `json:"-"`
	DocID         string         `json:"doc_id"`
	Title         string         `json:"title"`
	Category      string         `json:"category"`
	Brand         string         `json:"brand,omitempty"`
	DocType       string         `json:"doc_type"`
	Version       string         `json:"version"`
	EffectiveTime *time.Time     `json:"effective_time,omitempty"`
	ExpireTime    *time.Time     `json:"expire_time,omitempty"`
	Source        string         `json:"source"`
	Status        string         `json:"status"`
	ContentHash   string         `json:"content_hash"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type KnowledgeChunk struct {
	ID            int64          `json:"-"`
	ChunkID       string         `json:"chunk_id"`
	DocumentID    int64          `json:"-"`
	DocID         string         `json:"doc_id"`
	Title         string         `json:"title"`
	SectionPath   string         `json:"section_path,omitempty"`
	Content       string         `json:"content"`
	TokenCount    int            `json:"token_count"`
	IntentTags    []string       `json:"intent_tags,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	VectorPointID string         `json:"vector_point_id"`
	ContentHash   string         `json:"content_hash"`
}
