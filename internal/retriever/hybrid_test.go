package retriever

import (
	"context"
	"strings"
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
	for _, result := range results {
		if result.FusionScore <= 0 {
			t.Fatalf("result %s fusion score = %f", result.ChunkID, result.FusionScore)
		}
		if result.RerankScore <= 0 {
			t.Fatalf("result %s rerank score = %f", result.ChunkID, result.RerankScore)
		}
	}
}

func TestBuildVectorFilterCoversMetadataContract(t *testing.T) {
	filter := buildVectorFilter(rag.MetadataFilter{
		ProductIDs:   []string{"product-1"},
		SKUIDs:       []string{"sku-1"},
		Categories:   []string{"robot_vacuum"},
		Brands:       []string{"CleanCare"},
		Models:       []string{"T20"},
		DocTypes:     []string{"product_parameter"},
		IntentTags:   []string{"product_parameter"},
		Version:      "kb-v2",
		FaultNodeIDs: []string{"CHARGE-01"},
	})
	must, ok := filter["must"].([]map[string]any)
	if !ok {
		t.Fatalf("must filter = %#v", filter["must"])
	}
	keys := make(map[string]bool, len(must))
	modelFields := map[string]bool{}
	for _, condition := range must {
		key, _ := condition["key"].(string)
		keys[key] = true
		if should, ok := condition["should"].([]map[string]any); ok {
			for _, nested := range should {
				nestedKey, _ := nested["key"].(string)
				modelFields[nestedKey] = true
			}
		}
	}
	for _, key := range []string{
		"status",
		"category",
		"brand",
		"doc_type",
		"metadata.product_ids",
		"metadata.sku_ids",
		"intent_tags",
		"metadata.fault_node_ids",
		"version",
	} {
		if !keys[key] {
			t.Errorf("vector filter missing %q: %#v", key, must)
		}
	}
	if !modelFields["metadata.model"] || !modelFields["metadata.models"] {
		t.Fatalf("vector model filter does not cover scalar and array metadata: %#v", must)
	}
}

func TestBM25CandidatesRewardRareExactTerm(t *testing.T) {
	chunks := []model.KnowledgeChunk{
		{ChunkID: "exact", Title: "T20 充电故障", Content: strings.Repeat("充电 ", 4) + "CHARGE-01"},
		{ChunkID: "generic", Title: "清洁说明", Content: strings.Repeat("清洁 保养 ", 12)},
		{ChunkID: "other", Title: "P400 滤芯", Content: "滤芯寿命和更换方法"},
	}
	results := bm25Candidates(chunks, queryTerms("T20 充电"))
	if len(results) != 3 {
		t.Fatalf("result count = %d", len(results))
	}
	if results[0].chunk.ChunkID != "exact" || results[0].score <= results[1].score {
		t.Fatalf("BM25 results = %#v", results)
	}
}

func TestReciprocalRankFusionUsesDocumentSpecificConstantsAndNormalization(t *testing.T) {
	results := reciprocalRankFusion(
		[]rag.SearchResult{
			{ChunkID: "parameter", Metadata: map[string]any{"doc_type": "product_parameter"}},
			{ChunkID: "guide", Metadata: map[string]any{"doc_type": "purchase_guide"}},
		},
		nil,
	)
	if len(results) != 2 {
		t.Fatalf("results = %#v", results)
	}
	if results[0].ChunkID != "parameter" {
		t.Fatalf("first result = %s, want parameter document", results[0].ChunkID)
	}
	if results[0].FusionScore != 1 {
		t.Fatalf("top fusion score = %f, want normalized 1", results[0].FusionScore)
	}
	if results[1].FusionScore <= 0 || results[1].FusionScore >= 1 {
		t.Fatalf("guide fusion score = %f, want (0,1)", results[1].FusionScore)
	}
}

func TestRetrievalQualityChecksDocumentTypeAndModelCoverage(t *testing.T) {
	request := rag.SearchRequest{
		Filter: rag.MetadataFilter{
			DocTypes: []string{"product_parameter"},
			Models:   []string{"T20", "X20 Pro"},
		},
	}
	results := []rag.SearchResult{
		{RerankScore: 0.8, Metadata: map[string]any{"doc_type": "purchase_guide", "model": "T20"}},
		{RerankScore: 0.7, Metadata: map[string]any{"doc_type": "product_parameter"}},
		{RerankScore: 0.6, Metadata: map[string]any{"doc_type": "faq", "model": "X20 Pro"}},
		{RerankScore: 0.6, Metadata: map[string]any{"doc_type": "faq", "model": "X20 Pro"}},
	}
	issues := retrievalQualityIssues(request, results)
	if !stringInSlice("doc_type_fit_below_30_percent", issues) {
		t.Fatalf("issues = %#v, want doc type issue", issues)
	}
	if !stringInSlice("model_coverage_below_2:T20", issues) {
		t.Fatalf("issues = %#v, want T20 coverage issue", issues)
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
	calls   int
}

func (s *hybridVectorStore) Upsert(context.Context, []vectorstore.Point) error {
	return nil
}

func (s *hybridVectorStore) Search(
	_ context.Context,
	request vectorstore.SearchRequest,
) ([]vectorstore.SearchResult, error) {
	s.request = request
	s.calls++
	return s.results, nil
}

func TestHybridRetriesWithoutFilterForLowQualityResults(t *testing.T) {
	repo := &hybridRepository{
		active: []model.KnowledgeChunk{{
			ChunkID: "dense",
			DocID:   "doc1",
			Title:   "无关标题",
			Content: "完全不同的内容",
		}},
	}
	vector := &hybridVectorStore{results: []vectorstore.SearchResult{{
		Score:   0.1,
		Payload: map[string]any{"chunk_id": "dense"},
	}}}
	value := NewHybrid(embedding.NewLocalHash(16), vector, repo, reranker.NewLocalLexical())
	results, err := value.Search(context.Background(), rag.SearchRequest{
		Query:       "T20 吸力",
		Mode:        rag.SearchHybrid,
		Filter:      rag.MetadataFilter{DocTypes: []string{"product_parameter"}},
		DenseTopK:   5,
		KeywordTopK: 5,
		RerankTopK:  5,
		NeedRerank:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if vector.calls != 2 {
		t.Fatalf("vector search calls = %d, want 2", vector.calls)
	}
	if len(results) == 0 || results[0].Metadata["retry_without_filter"] != true {
		t.Fatalf("results = %#v, want retry metadata", results)
	}
}

func (s *hybridVectorStore) Delete(context.Context, []string) error {
	return nil
}
