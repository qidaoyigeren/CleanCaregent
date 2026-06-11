package retriever

import (
	"context"
	"testing"
	"time"

	"CleanCaregent/internal/embedding"
	"CleanCaregent/internal/model"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/reranker"
	"CleanCaregent/internal/vectorstore"
)

func TestHybridCombinesDenseAndKeywordResults(t *testing.T) {
	repo := &hybridRepository{
		active: []model.KnowledgeChunk{
			{ChunkID: "dense", DocID: "doc1", Title: "T20 参数", Content: "T20 吸力 6000Pa"},
		},
		keyword: []model.KnowledgeChunk{
			{ChunkID: "keyword", DocID: "doc2", Title: "吸力 FAQ", Content: "吸力单位为 Pa"},
		},
	}
	vector := &hybridVectorStore{results: []vectorstore.SearchResult{
		{Score: 0.9, Payload: map[string]any{"chunk_id": "dense"}},
		{Score: 0.8, Payload: map[string]any{"chunk_id": "orphan"}},
	}}
	retriever := NewHybrid(
		embedding.NewLocalHash(16),
		vector,
		repo,
		reranker.NewLocalLexical(),
	)

	results, err := retriever.Search(context.Background(), rag.SearchRequest{
		Query:       "T20 吸力",
		Mode:        rag.SearchHybrid,
		DenseTopK:   10,
		KeywordTopK: 10,
		RerankTopK:  5,
		NeedRerank:  true,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("result count = %d, results = %#v", len(results), results)
	}
	for _, result := range results {
		if result.ChunkID == "orphan" {
			t.Fatal("inactive/orphan dense result was not filtered")
		}
	}
	if vector.request.Filter == nil {
		t.Fatal("vector filter was not applied")
	}
}

type hybridRepository struct {
	active  []model.KnowledgeChunk
	keyword []model.KnowledgeChunk
}

func (r *hybridRepository) CreateDocument(context.Context, model.KnowledgeDocument, []model.KnowledgeChunk) error {
	return nil
}

func (r *hybridRepository) UpdateDocumentStatus(context.Context, string, string, string) error {
	return nil
}

func (r *hybridRepository) ActivateDocumentVersion(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func (r *hybridRepository) KeywordSearch(
	context.Context,
	repository.KnowledgeSearchRequest,
) ([]model.KnowledgeChunk, error) {
	return r.keyword, nil
}

func (r *hybridRepository) FindActiveChunks(context.Context, []string, time.Time) ([]model.KnowledgeChunk, error) {
	return r.active, nil
}

type hybridVectorStore struct {
	request vectorstore.SearchRequest
	results []vectorstore.SearchResult
}

func (s *hybridVectorStore) Upsert(context.Context, []vectorstore.Point) error {
	return nil
}

func (s *hybridVectorStore) Search(
	_ context.Context,
	request vectorstore.SearchRequest,
) ([]vectorstore.SearchResult, error) {
	s.request = request
	return s.results, nil
}

func (s *hybridVectorStore) Delete(context.Context, []string) error {
	return nil
}
