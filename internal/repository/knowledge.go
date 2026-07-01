package repository

import (
	"context"
	"errors"
	"time"

	"CleanCaregent/internal/model"
)

var (
	ErrKnowledgeDocumentConflict = errors.New("knowledge document version already exists")
	ErrKnowledgeDocumentNotFound = errors.New("knowledge document not found")
)

type KnowledgeSearchRequest struct {
	Query        string
	Terms        []string
	ProductIDs   []string
	SKUIDs       []string
	Categories   []string
	Brands       []string
	DocTypes     []string
	Models       []string
	IntentTags   []string
	Version      string
	FaultNodeIDs []string
	EffectiveAt  time.Time
	Limit        int
}

type KnowledgeRepository interface {
	CreateDocument(ctx context.Context, document model.KnowledgeDocument, chunks []model.KnowledgeChunk) error
	UpdateDocumentStatus(ctx context.Context, docID, version, status string) error
	ActivateDocumentVersion(ctx context.Context, docID, version string) ([]string, error)
	KeywordSearch(ctx context.Context, request KnowledgeSearchRequest) ([]model.KnowledgeChunk, error)
	FindActiveChunks(ctx context.Context, chunkIDs []string, effectiveAt time.Time) ([]model.KnowledgeChunk, error)
}

type KnowledgeDocumentVersion struct {
	DocID          string
	Version        string
	Status         string
	Category       string
	Brand          string
	DocType        string
	ContentHash    string
	Metadata       map[string]any
	VectorPointIDs []string
}

type KnowledgeVersionRepository interface {
	GetDocumentVersion(ctx context.Context, docID, version string) (KnowledgeDocumentVersion, error)
	DeleteDocumentVersion(ctx context.Context, docID, version string) ([]string, error)
}

const (
	KnowledgeVectorActionUpsert = "upsert_vectors"
	KnowledgeVectorActionDelete = "delete_vectors"
)

type KnowledgeVectorOutboxEvent struct {
	ID        int64
	DocID     string
	Version   string
	Action    string
	PointIDs  []string
	Attempts  int
	LastError string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type KnowledgeVectorOutboxRepository interface {
	PendingKnowledgeVectorOutbox(ctx context.Context, limit int) ([]KnowledgeVectorOutboxEvent, error)
	CompleteKnowledgeVectorOutbox(ctx context.Context, docID, version, action string) error
	CompleteKnowledgeVectorOutboxEvent(ctx context.Context, eventID int64) error
	FailKnowledgeVectorOutboxEvent(ctx context.Context, eventID int64, cause string) error
}

type KnowledgeChunkVersionRepository interface {
	GetDocumentChunks(ctx context.Context, docID, version string) ([]model.KnowledgeChunk, error)
}

// FulltextKnowledgeRepository is an optional capability for repositories that
// can execute indexed keyword search instead of scanning LIKE candidates.
type FulltextKnowledgeRepository interface {
	FulltextSearch(ctx context.Context, request KnowledgeSearchRequest) ([]model.KnowledgeChunk, error)
}
