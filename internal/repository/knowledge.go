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
	Query       string
	Terms       []string
	Categories  []string
	Brands      []string
	DocTypes    []string
	Models      []string
	EffectiveAt time.Time
	Limit       int
}

type KnowledgeRepository interface {
	CreateDocument(ctx context.Context, document model.KnowledgeDocument, chunks []model.KnowledgeChunk) error
	UpdateDocumentStatus(ctx context.Context, docID, version, status string) error
	ActivateDocumentVersion(ctx context.Context, docID, version string) ([]string, error)
	KeywordSearch(ctx context.Context, request KnowledgeSearchRequest) ([]model.KnowledgeChunk, error)
	FindActiveChunks(ctx context.Context, chunkIDs []string, effectiveAt time.Time) ([]model.KnowledgeChunk, error)
}
