package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"CleanCaregent/internal/embedding"
	"CleanCaregent/internal/model"
	"CleanCaregent/internal/platform/id"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/vectorstore"
)

var (
	ErrInvalidKnowledgeDocument = errors.New("invalid knowledge document")
	ErrKnowledgeUnavailable     = errors.New("knowledge service unavailable")
)

type KnowledgeService struct {
	repository repository.KnowledgeRepository
	vector     vectorstore.Store
	embedder   embedding.Embedder
	chunker    rag.Chunker
}

type IngestDocumentRequest struct {
	DocID         string
	Title         string
	Content       string
	Category      string
	Brand         string
	DocType       string
	Version       string
	EffectiveTime *time.Time
	ExpireTime    *time.Time
	Source        string
	IntentTags    []string
	Metadata      map[string]any
}

type IngestDocumentResult struct {
	DocID             string `json:"doc_id"`
	Version           string `json:"version"`
	Status            string `json:"status"`
	ChunkCount        int    `json:"chunk_count"`
	EmbeddingProvider string `json:"embedding_provider"`
	CleanupPending    bool   `json:"cleanup_pending,omitempty"`
}

func NewKnowledgeService(
	repository repository.KnowledgeRepository,
	vector vectorstore.Store,
	embedder embedding.Embedder,
	chunker rag.Chunker,
) *KnowledgeService {
	return &KnowledgeService{
		repository: repository,
		vector:     vector,
		embedder:   embedder,
		chunker:    chunker,
	}
}

func (s *KnowledgeService) Ingest(ctx context.Context, request IngestDocumentRequest) (IngestDocumentResult, error) {
	request = normalizeIngestRequest(request)
	if err := validateIngestRequest(request); err != nil {
		return IngestDocumentResult{}, err
	}
	if s.repository == nil || s.vector == nil || s.embedder == nil || s.chunker == nil {
		return IngestDocumentResult{}, ErrKnowledgeUnavailable
	}

	rawChunks := s.chunker.Split(request.DocType, request.Title, request.Content)
	if contextual, ok := s.chunker.(rag.ContextChunker); ok {
		rawChunks = contextual.SplitContext(ctx, request.DocType, request.Title, request.Content)
	}
	if len(rawChunks) == 0 {
		return IngestDocumentResult{}, fmt.Errorf("%w: document produced no chunks", ErrInvalidKnowledgeDocument)
	}

	texts := make([]string, len(rawChunks))
	chunks := make([]model.KnowledgeChunk, len(rawChunks))
	for index, rawChunk := range rawChunks {
		chunkID := fmt.Sprintf("%s:%s:%04d", request.DocID, request.Version, index+1)
		pointID := id.DeterministicUUID(chunkID)
		metadata := cloneMap(request.Metadata)
		metadata["category"] = request.Category
		metadata["brand"] = request.Brand
		metadata["doc_type"] = request.DocType
		metadata["version"] = request.Version
		metadata["embedding_provider"] = s.embedder.Name()
		chunks[index] = model.KnowledgeChunk{
			ChunkID:       chunkID,
			DocID:         request.DocID,
			Title:         request.Title,
			SectionPath:   rawChunk.SectionPath,
			Content:       rawChunk.Content,
			TokenCount:    rawChunk.TokenCount,
			IntentTags:    append([]string(nil), request.IntentTags...),
			Metadata:      metadata,
			VectorPointID: pointID,
			ContentHash:   contentHash(rawChunk.Content),
		}
		texts[index] = request.Title + "\n" + rawChunk.SectionPath + "\n" + rawChunk.Content
	}

	vectors, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return IngestDocumentResult{}, fmt.Errorf("embed knowledge document: %w", err)
	}
	if len(vectors) != len(chunks) {
		return IngestDocumentResult{}, fmt.Errorf("embedding count %d does not match chunk count %d", len(vectors), len(chunks))
	}

	document := model.KnowledgeDocument{
		DocID:         request.DocID,
		Title:         request.Title,
		Category:      request.Category,
		Brand:         request.Brand,
		DocType:       request.DocType,
		Version:       request.Version,
		EffectiveTime: request.EffectiveTime,
		ExpireTime:    request.ExpireTime,
		Source:        request.Source,
		Status:        model.KnowledgeStatusIndexing,
		ContentHash:   contentHash(request.Content),
		Metadata:      cloneMap(request.Metadata),
	}
	if err := s.repository.CreateDocument(ctx, document, chunks); err != nil {
		return IngestDocumentResult{}, err
	}

	points := make([]vectorstore.Point, len(chunks))
	for index, chunk := range chunks {
		points[index] = vectorstore.Point{
			ID:     chunk.VectorPointID,
			Vector: vectors[index],
			Payload: map[string]any{
				"chunk_id":     chunk.ChunkID,
				"doc_id":       chunk.DocID,
				"title":        chunk.Title,
				"section_path": chunk.SectionPath,
				"content":      chunk.Content,
				"category":     request.Category,
				"brand":        request.Brand,
				"doc_type":     request.DocType,
				"version":      request.Version,
				"intent_tags":  request.IntentTags,
				"metadata":     chunk.Metadata,
				"status":       model.KnowledgeStatusActive,
			},
		}
	}
	if err := s.vector.Upsert(ctx, points); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = s.repository.UpdateDocumentStatus(cleanupCtx, request.DocID, request.Version, model.KnowledgeStatusFailed)
		cancel()
		return IngestDocumentResult{}, fmt.Errorf("index knowledge vectors: %w", err)
	}
	stalePointIDs, err := s.repository.ActivateDocumentVersion(ctx, request.DocID, request.Version)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = s.vector.Delete(cleanupCtx, pointIDs(points))
		_ = s.repository.UpdateDocumentStatus(
			cleanupCtx,
			request.DocID,
			request.Version,
			model.KnowledgeStatusFailed,
		)
		cancel()
		return IngestDocumentResult{}, err
	}
	cleanupPending := false
	if len(stalePointIDs) > 0 {
		cleanupPending = s.vector.Delete(ctx, stalePointIDs) != nil
	}

	return IngestDocumentResult{
		DocID:             request.DocID,
		Version:           request.Version,
		Status:            model.KnowledgeStatusActive,
		ChunkCount:        len(chunks),
		EmbeddingProvider: s.embedder.Name(),
		CleanupPending:    cleanupPending,
	}, nil
}

func pointIDs(points []vectorstore.Point) []string {
	result := make([]string, 0, len(points))
	for _, point := range points {
		if point.ID != "" {
			result = append(result, point.ID)
		}
	}
	return result
}

func normalizeIngestRequest(request IngestDocumentRequest) IngestDocumentRequest {
	request.DocID = strings.TrimSpace(request.DocID)
	request.Title = strings.TrimSpace(request.Title)
	request.Content = strings.TrimSpace(request.Content)
	request.Category = strings.TrimSpace(request.Category)
	request.Brand = strings.TrimSpace(request.Brand)
	request.DocType = strings.TrimSpace(request.DocType)
	request.Version = strings.TrimSpace(request.Version)
	request.Source = strings.TrimSpace(request.Source)
	if request.Version == "" {
		request.Version = "1.0"
	}
	if request.Source == "" {
		request.Source = "manual-upload"
	}
	if request.Metadata == nil {
		request.Metadata = map[string]any{}
	}
	return request
}

func validateIngestRequest(request IngestDocumentRequest) error {
	if request.DocID == "" || request.Title == "" || request.Content == "" ||
		request.Category == "" || request.DocType == "" {
		return fmt.Errorf("%w: doc_id, title, content, category and doc_type are required", ErrInvalidKnowledgeDocument)
	}
	if len(request.DocID) > 128 || len([]rune(request.Title)) > 255 || len(request.Version) > 64 {
		return fmt.Errorf("%w: document identifiers are too long", ErrInvalidKnowledgeDocument)
	}
	if len([]rune(request.Content)) > 2_000_000 {
		return fmt.Errorf("%w: document content is too large", ErrInvalidKnowledgeDocument)
	}
	if err := validateContentQuality(request.Content); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidKnowledgeDocument, err)
	}
	if !allowedDocType(request.DocType) {
		return fmt.Errorf("%w: unsupported doc_type %q", ErrInvalidKnowledgeDocument, request.DocType)
	}
	if request.EffectiveTime != nil && request.ExpireTime != nil && !request.ExpireTime.After(*request.EffectiveTime) {
		return fmt.Errorf("%w: expire_time must be after effective_time", ErrInvalidKnowledgeDocument)
	}
	return nil
}

func validateContentQuality(content string) error {
	var (
		effective []rune
		counts    = make(map[rune]int)
	)
	for _, current := range []rune(content) {
		if unicode.IsSpace(current) || unicode.IsPunct(current) || unicode.IsSymbol(current) {
			continue
		}
		effective = append(effective, current)
		counts[current]++
	}
	if len(effective) < 50 {
		return errors.New("文档内容过短，有效正文必须不少于 50 个字符")
	}
	maxCount := 0
	for _, count := range counts {
		if count > maxCount {
			maxCount = count
		}
	}
	if len(effective) > 0 && float64(maxCount)/float64(len(effective)) > 0.8 {
		return errors.New("文档包含超过 80% 的重复字符")
	}
	return nil
}

func allowedDocType(docType string) bool {
	switch docType {
	case "product_detail",
		"product_parameter",
		"product_comparison",
		"purchase_guide",
		"accessory_compatibility",
		"user_manual",
		"troubleshooting",
		"after_sales_policy",
		"faq":
		return true
	default:
		return false
	}
}

func contentHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+6)
	for key, value := range source {
		result[key] = value
	}
	return result
}
