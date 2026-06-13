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
	"CleanCaregent/internal/observability"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/reranker"
	"CleanCaregent/internal/vectorstore"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
	startedAt := time.Now()
	ctx, span := otel.Tracer("clean-care-agent/retriever").Start(ctx, "retriever.hybrid_search")
	span.SetAttributes(
		attribute.String("retrieval.mode", string(request.Mode)),
		attribute.Int("retrieval.query_runes", len([]rune(request.Query))),
	)
	defer func() {
		observability.DefaultPrometheusMetrics.RecordRetrieval(time.Since(startedAt))
		span.SetAttributes(attribute.Int64("retrieval.duration_ms", time.Since(startedAt).Milliseconds()))
		span.End()
	}()
	request = normalizeSearchRequest(request)
	if strings.TrimSpace(request.Query) == "" {
		span.SetStatus(codes.Error, "empty query")
		return nil, errors.New("search query is required")
	}

	results, err := h.searchOnce(ctx, request, span)
	if err != nil {
		return nil, err
	}
	qualityIssues := retrievalQualityIssues(request, results)
	if broadFilter, relaxed := relaxBusinessFilter(request.Filter); len(qualityIssues) > 0 && relaxed {
		broadRequest := request
		broadRequest.Filter = broadFilter
		broadResults, broadErr := h.searchOnce(ctx, broadRequest, span)
		if broadErr == nil && (len(results) == 0 || betterRetrievalQuality(broadResults, results)) {
			results = broadResults
		}
		for index := range results {
			if results[index].Metadata == nil {
				results[index].Metadata = map[string]any{}
			}
			results[index].Metadata["low_quality_retrieval"] = true
			results[index].Metadata["retry_with_relaxed_filter"] = true
			results[index].Metadata["quality_issues"] = append([]string(nil), qualityIssues...)
		}
		span.SetAttributes(attribute.Bool("retrieval.retry_with_relaxed_filter", true))
	}
	span.SetAttributes(
		attribute.Bool("retrieval.low_quality", len(qualityIssues) > 0),
		attribute.StringSlice("retrieval.quality_issues", qualityIssues),
	)
	return results, nil
}

// SearchWithFallbackEmbedder executes hybrid search. Empty or malformed
// primary vectors are handled by embedding.Fallback before vector search.
func (h *Hybrid) SearchWithFallbackEmbedder(
	ctx context.Context,
	request rag.SearchRequest,
) ([]rag.SearchResult, error) {
	return h.Search(ctx, request)
}

func (h *Hybrid) searchOnce(
	ctx context.Context,
	request rag.SearchRequest,
	span trace.Span,
) ([]rag.SearchResult, error) {
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

	fused := reciprocalRankFusion(denseResults, keywordResults)
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
	searchRequest := repository.KnowledgeSearchRequest{
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
	}
	var fulltextChunks []model.KnowledgeChunk
	var fulltextErr error
	if fulltext, ok := h.repository.(repository.FulltextKnowledgeRepository); ok {
		fulltextChunks, fulltextErr = fulltext.FulltextSearch(ctx, searchRequest)
	}
	applicationChunks, applicationErr := h.repository.KeywordSearch(ctx, searchRequest)
	if fulltextErr != nil && applicationErr != nil {
		return nil, fmt.Errorf(
			"fulltext search failed: %v; application keyword search failed: %v",
			fulltextErr,
			applicationErr,
		)
	}

	resultsByChunk := make(map[string]rag.SearchResult, len(fulltextChunks)+len(applicationChunks))
	if fulltextErr == nil {
		for index, chunk := range fulltextChunks {
			score, _ := chunk.Metadata["fulltext_score"].(float64)
			if score <= 0 {
				score = 1 / float64(index+1)
			}
			result := toSearchResult(chunk, 0, score)
			result.Metadata["keyword_backend"] = "mysql_fulltext"
			resultsByChunk[chunk.ChunkID] = result
		}
	}
	if applicationErr == nil {
		for _, candidate := range bm25Candidates(applicationChunks, queryTerms(request.Query)) {
			current, exists := resultsByChunk[candidate.chunk.ChunkID]
			if !exists {
				current = toSearchResult(candidate.chunk, 0, candidate.score)
				current.Metadata["keyword_backend"] = "application_bm25"
			} else {
				current.KeywordScore += candidate.score
				current.Metadata["keyword_backend"] = "mysql_fulltext+application_bm25"
			}
			resultsByChunk[candidate.chunk.ChunkID] = current
		}
	}
	results := make([]rag.SearchResult, 0, len(resultsByChunk))
	for _, result := range resultsByChunk {
		results = append(results, result)
	}
	normalizeKeywordScores(results)
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].KeywordScore > results[j].KeywordScore
	})
	if len(results) > request.KeywordTopK {
		results = results[:request.KeywordTopK]
	}
	return results, nil
}

func lowQualityRetrieval(results []rag.SearchResult) bool {
	if len(results) == 0 {
		return true
	}
	for _, result := range results {
		if result.RerankScore > 0.5 {
			return false
		}
	}
	return true
}

func retrievalQualityIssues(request rag.SearchRequest, results []rag.SearchResult) []string {
	var issues []string
	if len(results) == 0 {
		return []string{"empty_results"}
	}
	if request.MinScore > 0 && effectiveResultScore(results[0]) < request.MinScore {
		issues = append(issues, "top1_below_min_score")
	}
	if lowQualityRetrieval(results) {
		issues = append(issues, "rerank_confidence_low")
	}
	if len(request.Filter.DocTypes) > 0 {
		matched := 0
		for _, result := range results {
			if stringInSlice(resultDocType(result), request.Filter.DocTypes) {
				matched++
			}
		}
		if float64(matched)/float64(len(results)) < 0.30 {
			issues = append(issues, "doc_type_fit_below_30_percent")
		}
	}
	if len(request.Filter.Models) >= 2 {
		coverage := make(map[string]int, len(request.Filter.Models))
		for _, result := range results {
			for _, modelName := range resultModels(result) {
				if stringInSlice(modelName, request.Filter.Models) {
					coverage[modelName]++
				}
			}
		}
		for _, modelName := range request.Filter.Models {
			if coverage[modelName] < 2 {
				issues = append(issues, "model_coverage_below_2:"+modelName)
			}
		}
	}
	return compactStrings(issues)
}

func relaxBusinessFilter(filter rag.MetadataFilter) (rag.MetadataFilter, bool) {
	relaxed := filter
	changed := len(filter.ProductIDs) > 0 ||
		len(filter.SKUIDs) > 0 ||
		len(filter.Brands) > 0 ||
		len(filter.Models) > 0 ||
		len(filter.FaultNodeIDs) > 0
	relaxed.ProductIDs = nil
	relaxed.SKUIDs = nil
	relaxed.Brands = nil
	relaxed.Models = nil
	relaxed.FaultNodeIDs = nil
	return relaxed, changed
}

func betterRetrievalQuality(candidate, current []rag.SearchResult) bool {
	best := func(values []rag.SearchResult) float64 {
		score := 0.0
		for _, value := range values {
			if value.RerankScore > score {
				score = value.RerankScore
			}
		}
		return score
	}
	return len(candidate) > 0 && (best(candidate) > best(current) || len(current) == 0)
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

func reciprocalRankFusion(dense, keyword []rag.SearchResult) []rag.SearchResult {
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
			current.score += 1 / (rrfRankConstant(result) + float64(index+1))
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
	normalizeFusionScores(output)
	sort.SliceStable(output, func(i, j int) bool {
		return output[i].FusionScore > output[j].FusionScore
	})
	return output
}

func rrfRankConstant(result rag.SearchResult) float64 {
	switch resultDocType(result) {
	case "product_parameter":
		return 10
	case "purchase_guide":
		return 60
	default:
		return 40
	}
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

func normalizeKeywordScores(results []rag.SearchResult) {
	maxScore := 0.0
	for _, result := range results {
		if result.KeywordScore > maxScore {
			maxScore = result.KeywordScore
		}
	}
	if maxScore <= 0 {
		return
	}
	for index := range results {
		results[index].KeywordScore /= maxScore
	}
}

func normalizeFusionScores(results []rag.SearchResult) {
	maxScore := 0.0
	for _, result := range results {
		if result.FusionScore > maxScore {
			maxScore = result.FusionScore
		}
	}
	if maxScore <= 0 {
		return
	}
	for index := range results {
		results[index].FusionScore /= maxScore
	}
}

func effectiveResultScore(result rag.SearchResult) float64 {
	for _, score := range []float64{
		result.RerankScore,
		result.FusionScore,
		result.DenseScore,
		result.KeywordScore,
	} {
		if score > 0 {
			return score
		}
	}
	return 0
}

func resultDocType(result rag.SearchResult) string {
	value, _ := result.Metadata["doc_type"].(string)
	return strings.TrimSpace(value)
}

func resultModels(result rag.SearchResult) []string {
	if result.Metadata == nil {
		return nil
	}
	var models []string
	if value, ok := result.Metadata["model"].(string); ok {
		models = append(models, value)
	}
	switch values := result.Metadata["models"].(type) {
	case []string:
		models = append(models, values...)
	case []any:
		for _, value := range values {
			if text, ok := value.(string); ok {
				models = append(models, text)
			}
		}
	}
	return compactStrings(models)
}

func stringInSlice(value string, values []string) bool {
	for _, candidate := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
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
