package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"CleanCaregent/internal/embedding"
	"CleanCaregent/internal/model"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/vectorstore"
)

func TestKnowledgeServiceIngestsAndActivatesDocument(t *testing.T) {
	repo := &fakeKnowledgeRepository{}
	vector := &fakeVectorStore{}
	service := NewKnowledgeService(
		repo,
		vector,
		embedding.NewLocalHash(32),
		rag.NewStructureAwareChunker(200, 20),
	)

	result, err := service.Ingest(context.Background(), IngestDocumentRequest{
		DocID:    "doc_t20_parameters",
		Title:    "T20 参数",
		Content:  validKnowledgeContent("T20 的额定吸力为 6000Pa"),
		Category: "robot_vacuum",
		Brand:    "MockClean",
		DocType:  "product_parameter",
		Version:  "1.0",
		Metadata: map[string]any{"model": "T20"},
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if result.Status != model.KnowledgeStatusActive || result.ChunkCount != 1 {
		t.Fatalf("result = %#v", result)
	}
	if repo.document.Status != model.KnowledgeStatusIndexing {
		t.Fatalf("initial status = %q", repo.document.Status)
	}
	if len(vector.points) != 1 || len(vector.points[0].Vector) != 32 {
		t.Fatalf("points = %#v", vector.points)
	}
	if repo.activatedDocID != "doc_t20_parameters" || repo.activatedVersion != "1.0" {
		t.Fatalf("activated = %s@%s", repo.activatedDocID, repo.activatedVersion)
	}
}

func TestKnowledgeServiceMarksFailedWhenVectorIndexFails(t *testing.T) {
	repo := &fakeKnowledgeRepository{}
	vector := &fakeVectorStore{upsertErr: errors.New("qdrant unavailable")}
	service := NewKnowledgeService(
		repo,
		vector,
		embedding.NewLocalHash(16),
		rag.NewStructureAwareChunker(200, 20),
	)

	_, err := service.Ingest(context.Background(), IngestDocumentRequest{
		DocID:    "doc_t20_parameters",
		Title:    "T20 参数",
		Content:  validKnowledgeContent("T20 的额定吸力为 6000Pa"),
		Category: "robot_vacuum",
		DocType:  "product_parameter",
	})
	if err == nil {
		t.Fatal("Ingest() expected error")
	}
	if len(repo.statuses) != 1 || repo.statuses[0] != model.KnowledgeStatusFailed {
		t.Fatalf("statuses = %v", repo.statuses)
	}
}

type fakeKnowledgeRepository struct {
	document         model.KnowledgeDocument
	chunks           []model.KnowledgeChunk
	statuses         []string
	stalePointIDs    []string
	activatedDocID   string
	activatedVersion string
}

func (r *fakeKnowledgeRepository) CreateDocument(
	_ context.Context,
	document model.KnowledgeDocument,
	chunks []model.KnowledgeChunk,
) error {
	r.document = document
	r.chunks = chunks
	return nil
}

func (r *fakeKnowledgeRepository) UpdateDocumentStatus(_ context.Context, _, _, status string) error {
	r.statuses = append(r.statuses, status)
	return nil
}

func (r *fakeKnowledgeRepository) ActivateDocumentVersion(
	_ context.Context,
	docID string,
	version string,
) ([]string, error) {
	r.activatedDocID = docID
	r.activatedVersion = version
	return append([]string(nil), r.stalePointIDs...), nil
}

func (r *fakeKnowledgeRepository) KeywordSearch(
	context.Context,
	repository.KnowledgeSearchRequest,
) ([]model.KnowledgeChunk, error) {
	return nil, nil
}

func (r *fakeKnowledgeRepository) FindActiveChunks(context.Context, []string, time.Time) ([]model.KnowledgeChunk, error) {
	return nil, nil
}

type fakeVectorStore struct {
	points     []vectorstore.Point
	upsertErr  error
	deletedIDs []string
	deleteErr  error
}

func validKnowledgeContent(prefix string) string {
	return prefix + "。该文档同时说明导航方式、续航时间、适用面积、地毯能力、宠物毛发处理、噪声、尘盒、水箱和越障能力，供测试完整入库流程使用。"
}

func (s *fakeVectorStore) Upsert(_ context.Context, points []vectorstore.Point) error {
	s.points = points
	return s.upsertErr
}

func (s *fakeVectorStore) Search(
	context.Context,
	vectorstore.SearchRequest,
) ([]vectorstore.SearchResult, error) {
	return nil, nil
}

func (s *fakeVectorStore) Delete(_ context.Context, pointIDs []string) error {
	s.deletedIDs = append(s.deletedIDs, pointIDs...)
	return s.deleteErr
}

func TestKnowledgeServiceDeletesSupersededVectors(t *testing.T) {
	repo := &fakeKnowledgeRepository{stalePointIDs: []string{"old-1", "old-2"}}
	vector := &fakeVectorStore{}
	service := NewKnowledgeService(
		repo,
		vector,
		embedding.NewLocalHash(16),
		rag.NewStructureAwareChunker(200, 20),
	)

	result, err := service.Ingest(context.Background(), IngestDocumentRequest{
		DocID:    "doc_t20_parameters",
		Title:    "T20 参数",
		Content:  validKnowledgeContent("T20 的额定吸力为 6500Pa"),
		Category: "robot_vacuum",
		DocType:  "product_parameter",
		Version:  "2.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanupPending {
		t.Fatal("cleanup should have completed")
	}
	if len(vector.deletedIDs) != 2 || vector.deletedIDs[0] != "old-1" {
		t.Fatalf("deleted IDs = %v", vector.deletedIDs)
	}
}

func TestKnowledgeServiceReportsDeferredVectorCleanup(t *testing.T) {
	repo := &fakeKnowledgeRepository{stalePointIDs: []string{"old-1"}}
	vector := &fakeVectorStore{deleteErr: errors.New("qdrant delete failed")}
	service := NewKnowledgeService(
		repo,
		vector,
		embedding.NewLocalHash(16),
		rag.NewStructureAwareChunker(200, 20),
	)

	result, err := service.Ingest(context.Background(), IngestDocumentRequest{
		DocID:    "doc_t20_parameters",
		Title:    "T20 参数",
		Content:  validKnowledgeContent("T20 的额定吸力为 6500Pa"),
		Category: "robot_vacuum",
		DocType:  "product_parameter",
		Version:  "2.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.CleanupPending {
		t.Fatal("cleanup_pending should expose stale vector cleanup failure")
	}
}
