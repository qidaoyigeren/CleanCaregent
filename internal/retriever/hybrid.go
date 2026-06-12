package retriever

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"CleanCaregent/internal/embedding"
	"CleanCaregent/internal/model"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/reranker"
	"CleanCaregent/internal/vectorstore"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type Hybrid struct {
	embedder   embedding.Embedder
	vector     vectorstore.Store
	repository repository.KnowledgeRepository
	reranker   reranker.Reranker
}

func NewHybrid(
	embedder embedding.Embedder,
	vector vectorstore.Store,
	repository repository.KnowledgeRepository,
	reranker reranker.Reranker,
) *Hybrid {
	return &Hybrid{
		embedder:   embedder,
		vector:     vector,
		repository: repository,
		reranker:   reranker,
	}
}

func (h *Hybrid) Search(ctx context.Context, request rag.SearchRequest) ([]rag.SearchResult, error) {
	ctx, span := otel.Tracer("clean-care-agent/retriever").Start(ctx, "retriever.hybrid_search")
	span.SetAttributes(
		attribute.String("retrieval.mode", string(request.Mode)),
		attribute.Int("retrieval.query_runes", len([]rune(request.Query))),
	)
	defer span.End()
	request = normalizeSearchRequest(request)
	if strings.TrimSpace(request.Query) == "" {
		span.SetStatus(codes.Error, "empty query")
		return nil, errors.New("search query is required")
	}

	var (
		denseResults   []rag.SearchResult
		keywordResults []rag.SearchResult
		denseErr       error
		keywordErr     error
		wg             sync.WaitGroup
	)

	if request.Mode == rag.SearchDense || request.Mode == rag.SearchHybrid {
		wg.Add(1)
		go func() {
			defer wg.Done()
			denseResults, denseErr = h.denseSearch(ctx, request)
		}()
	}
	if request.Mode == rag.SearchKeyword || request.Mode == rag.SearchHybrid {
		wg.Add(1)
		go func() {
			defer wg.Done()
			keywordResults, keywordErr = h.keywordSearch(ctx, request)
		}()
	}
	wg.Wait()

	if denseErr != nil && keywordErr != nil {
		span.SetStatus(codes.Error, "dense and keyword search failed")
		return nil, fmt.Errorf("dense search failed: %v; keyword search failed: %v", denseErr, keywordErr)
	}
	if request.Mode == rag.SearchDense && denseErr != nil {
		return nil, denseErr
	}
	if request.Mode == rag.SearchKeyword && keywordErr != nil {
		return nil, keywordErr
	}

	fused := reciprocalRankFusion(denseResults, keywordResults, 60)
	span.SetAttributes(
		attribute.Int("retrieval.dense_results", len(denseResults)),
		attribute.Int("retrieval.keyword_results", len(keywordResults)),
		attribute.Int("retrieval.fused_results", len(fused)),
	)
	if request.NeedRerank && h.reranker != nil && len(fused) > 0 {
		reranked, err := h.reranker.Rerank(ctx, request.Query, fused, request.RerankTopK)
		if err == nil {
			markFallbacks(reranked, denseErr, keywordErr, nil)
			return reranked, nil
		}
		markFallbacks(fused, denseErr, keywordErr, err)
	} else {
		markFallbacks(fused, denseErr, keywordErr, nil)
	}
	if len(fused) > request.RerankTopK {
		fused = fused[:request.RerankTopK]
	}
	return fused, nil
}

func markFallbacks(results []rag.SearchResult, denseErr, keywordErr, rerankErr error) {
	for index := range results {
		if results[index].Metadata == nil {
			results[index].Metadata = map[string]any{}
		}
		if denseErr != nil {
			results[index].Metadata["dense_fallback"] = true
		}
		if keywordErr != nil {
			results[index].Metadata["keyword_fallback"] = true
		}
		if rerankErr != nil {
			results[index].Metadata["rerank_fallback"] = true
		}
	}
}

func (h *Hybrid) denseSearch(ctx context.Context, request rag.SearchRequest) ([]rag.SearchResult, error) {
	vectors, err := h.embedder.Embed(ctx, []string{request.Query})
	if err != nil {
		return nil, fmt.Errorf("embed search query: %w", err)
	}
	if len(vectors) != 1 {
		return nil, errors.New("query embedding returned unexpected vector count")
	}
	results, err := h.vector.Search(ctx, vectorstore.SearchRequest{
		Vector:         vectors[0],
		Limit:          request.DenseTopK,
		ScoreThreshold: request.MinScore,
		Filter:         buildVectorFilter(request.Filter),
		WithPayload:    true,
	})
	if err != nil {
		return nil, err
	}

	chunkIDs := make([]string, 0, len(results))
	scores := make(map[string]float64, len(results))
	for _, result := range results {
		chunkID, _ := result.Payload["chunk_id"].(string)
		if chunkID == "" {
			continue
		}
		chunkIDs = append(chunkIDs, chunkID)
		scores[chunkID] = result.Score
	}
	chunks, err := h.repository.FindActiveChunks(
		ctx,
		chunkIDs,
		effectiveAt(request.Filter.EffectiveAt),
	)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]model.KnowledgeChunk, len(chunks))
	for _, chunk := range chunks {
		byID[chunk.ChunkID] = chunk
	}

	output := make([]rag.SearchResult, 0, len(chunkIDs))
	for _, chunkID := range chunkIDs {
		chunk, ok := byID[chunkID]
		if !ok {
			continue
		}
		output = append(output, toSearchResult(chunk, scores[chunkID], 0))
	}
	return output, nil
}

func (h *Hybrid) keywordSearch(ctx context.Context, request rag.SearchRequest) ([]rag.SearchResult, error) {
	chunks, err := h.repository.KeywordSearch(ctx, repository.KnowledgeSearchRequest{
		Query:        request.Query,
		Terms:        queryTerms(request.Query),
		ProductIDs:   request.Filter.ProductIDs,
		SKUIDs:       request.Filter.SKUIDs,
		Categories:   request.Filter.Categories,
		Brands:       request.Filter.Brands,
		DocTypes:     request.Filter.DocTypes,
		Models:       request.Filter.Models,
		IntentTags:   request.Filter.IntentTags,
		Version:      request.Filter.Version,
		FaultNodeIDs: request.Filter.FaultNodeIDs,
		EffectiveAt:  effectiveAt(request.Filter.EffectiveAt),
		Limit:        request.KeywordTopK,
	})
	if err != nil {
		return nil, err
	}
	scored := bm25Candidates(chunks, queryTerms(request.Query))
	results := make([]rag.SearchResult, len(scored))
	for index, candidate := range scored {
		results[index] = toSearchResult(candidate.chunk, 0, candidate.score)
	}
	return results, nil
}

func normalizeSearchRequest(request rag.SearchRequest) rag.SearchRequest {
	if request.Mode == "" {
		request.Mode = rag.SearchHybrid
	}
	if request.DenseTopK <= 0 {
		request.DenseTopK = 20
	}
	if request.KeywordTopK <= 0 {
		request.KeywordTopK = 20
	}
	if request.RerankTopK <= 0 {
		request.RerankTopK = 6
	}
	return request
}

func buildVectorFilter(filter rag.MetadataFilter) map[string]any {
	must := []map[string]any{
		{"key": "status", "match": map[string]any{"value": model.KnowledgeStatusActive}},
	}
	addMatchAny := func(key string, values []string) {
		values = compactStrings(values)
		if len(values) == 0 {
			return
		}
		must = append(must, map[string]any{
			"key": key,
			"match": map[string]any{
				"any": values,
			},
		})
	}
	addMatchAny("category", filter.Categories)
	addMatchAny("brand", filter.Brands)
	addMatchAny("doc_type", filter.DocTypes)
	if models := compactStrings(filter.Models); len(models) > 0 {
		must = append(must, map[string]any{
			"should": []map[string]any{
				{"key": "metadata.model", "match": map[string]any{"any": models}},
				{"key": "metadata.models", "match": map[string]any{"any": models}},
			},
		})
	}
	addMatchAny("metadata.product_ids", filter.ProductIDs)
	addMatchAny("metadata.sku_ids", filter.SKUIDs)
	addMatchAny("intent_tags", filter.IntentTags)
	addMatchAny("metadata.fault_node_ids", filter.FaultNodeIDs)
	if version := strings.TrimSpace(filter.Version); version != "" {
		must = append(must, map[string]any{
			"key":   "version",
			"match": map[string]any{"value": version},
		})
	}
	return map[string]any{"must": must}
}

type bm25Candidate struct {
	chunk model.KnowledgeChunk
	score float64
}

func bm25Candidates(chunks []model.KnowledgeChunk, terms []string) []bm25Candidate {
	if len(chunks) == 0 {
		return nil
	}
	terms = compactStrings(terms)
	if len(terms) == 0 {
		result := make([]bm25Candidate, len(chunks))
		for index, chunk := range chunks {
			result[index] = bm25Candidate{chunk: chunk}
		}
		return result
	}

	const (
		k1 = 1.2
		b  = 0.75
	)
	tokenized := make([][]string, len(chunks))
	documentFrequency := make(map[string]int, len(terms))
	totalLength := 0
	for index, chunk := range chunks {
		tokens := queryTerms(chunk.Title + "\n" + chunk.Content)
		tokenized[index] = tokens
		totalLength += len(tokens)
		present := make(map[string]struct{}, len(tokens))
		for _, token := range tokens {
			present[token] = struct{}{}
		}
		for _, term := range terms {
			if _, ok := present[term]; ok {
				documentFrequency[term]++
			}
		}
	}
	averageLength := float64(totalLength) / float64(len(chunks))
	if averageLength < 1 {
		averageLength = 1
	}

	result := make([]bm25Candidate, len(chunks))
	documentCount := float64(len(chunks))
	for index, chunk := range chunks {
		frequencies := make(map[string]int, len(tokenized[index]))
		for _, token := range tokenized[index] {
			frequencies[token]++
		}
		documentLength := float64(len(tokenized[index]))
		score := 0.0
		for _, term := range terms {
			tf := float64(frequencies[term])
			if tf == 0 {
				continue
			}
			df := float64(documentFrequency[term])
			idf := math.Log(1 + (documentCount-df+0.5)/(df+0.5))
			denominator := tf + k1*(1-b+b*documentLength/averageLength)
			score += idf * (tf * (k1 + 1) / denominator)
		}
		result[index] = bm25Candidate{chunk: chunk, score: score}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].score > result[j].score
	})
	return result
}

func reciprocalRankFusion(dense, keyword []rag.SearchResult, rankConstant float64) []rag.SearchResult {
	type item struct {
		result rag.SearchResult
		score  float64
	}
	items := make(map[string]*item, len(dense)+len(keyword))
	add := func(results []rag.SearchResult, denseList bool) {
		for index, result := range results {
			current, ok := items[result.ChunkID]
			if !ok {
				current = &item{result: result}
				items[result.ChunkID] = current
			}
			current.score += 1 / (rankConstant + float64(index+1))
			if denseList {
				current.result.DenseScore = result.DenseScore
			} else {
				current.result.KeywordScore = result.KeywordScore
			}
		}
	}
	add(dense, true)
	add(keyword, false)

	output := make([]rag.SearchResult, 0, len(items))
	for _, current := range items {
		current.result.FusionScore = current.score
		output = append(output, current.result)
	}
	sort.SliceStable(output, func(i, j int) bool {
		return output[i].FusionScore > output[j].FusionScore
	})
	return output
}

func toSearchResult(chunk model.KnowledgeChunk, denseScore, keywordScore float64) rag.SearchResult {
	metadata := cloneMetadata(chunk.Metadata)
	metadata["section_path"] = chunk.SectionPath
	metadata["vector_point_id"] = chunk.VectorPointID
	return rag.SearchResult{
		ChunkID:      chunk.ChunkID,
		DocumentID:   chunk.DocID,
		Title:        chunk.Title,
		Content:      chunk.Content,
		DenseScore:   denseScore,
		KeywordScore: keywordScore,
		Metadata:     metadata,
	}
}

func queryTerms(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	var words []string
	var current []rune
	var han []rune
	flush := func() {
		if len(current) > 0 {
			words = append(words, string(current))
			current = current[:0]
		}
	}
	for _, value := range []rune(query) {
		if unicode.IsLetter(value) || unicode.IsNumber(value) {
			current = append(current, value)
			if unicode.Is(unicode.Han, value) {
				han = append(han, value)
			}
			continue
		}
		flush()
	}
	flush()
	for index := 0; index+2 <= len(han); index++ {
		words = append(words, string(han[index:index+2]))
	}
	terms := compactStrings(words)
	if len(terms) > 32 {
		terms = terms[:32]
	}
	return terms
}

func compactStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	output := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		output = append(output, value)
	}
	return output
}

func effectiveAt(value *time.Time) time.Time {
	if value != nil {
		return *value
	}
	return time.Now().UTC()
}

func cloneMetadata(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+2)
	for key, value := range source {
		result[key] = value
	}
	return result
}
