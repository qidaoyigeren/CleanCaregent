package reranker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"CleanCaregent/internal/rag"
)

type OpenAIClient struct {
	endpoint   string
	apiKey     string
	model      string
	httpClient *http.Client
}

type rerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
		Score          float64 `json:"score"`
	} `json:"results"`
}

func NewOpenAIClient(
	endpoint string,
	apiKey string,
	model string,
	timeout time.Duration,
) *OpenAIClient {
	return &OpenAIClient{
		endpoint: strings.TrimSpace(endpoint),
		apiKey:   strings.TrimSpace(apiKey),
		model:    strings.TrimSpace(model),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *OpenAIClient) Rerank(
	ctx context.Context,
	query string,
	documents []rag.SearchResult,
	topK int,
) ([]rag.SearchResult, error) {
	if len(documents) == 0 {
		return []rag.SearchResult{}, nil
	}
	if topK <= 0 || topK > len(documents) {
		topK = len(documents)
	}
	texts := make([]string, len(documents))
	for index, document := range documents {
		texts[index] = document.Title + "\n" + document.Content
	}
	body, err := json.Marshal(map[string]any{
		"model":            c.model,
		"query":            query,
		"documents":        texts,
		"top_n":            topK,
		"return_documents": false,
	})
	if err != nil {
		return nil, fmt.Errorf("encode rerank request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create rerank request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call rerank endpoint: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf(
			"rerank endpoint returned %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(raw)),
		)
	}
	var payload rerankResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode rerank response: %w", err)
	}
	if len(payload.Results) == 0 {
		return nil, fmt.Errorf("rerank endpoint returned no results")
	}

	seen := make(map[int]struct{}, len(payload.Results))
	result := make([]rag.SearchResult, 0, min(topK, len(payload.Results)))
	for _, item := range payload.Results {
		if item.Index < 0 || item.Index >= len(documents) {
			return nil, fmt.Errorf("rerank result index %d is out of range", item.Index)
		}
		if _, exists := seen[item.Index]; exists {
			return nil, fmt.Errorf("rerank result index %d is duplicated", item.Index)
		}
		seen[item.Index] = struct{}{}
		score := item.RelevanceScore
		if score == 0 && item.Score != 0 {
			score = item.Score
		}
		if math.IsNaN(score) || math.IsInf(score, 0) {
			return nil, fmt.Errorf("rerank result index %d has invalid score", item.Index)
		}
		document := documents[item.Index]
		document.RerankScore = score
		if document.Metadata == nil {
			document.Metadata = map[string]any{}
		}
		document.Metadata["rerank_provider"] = c.model
		result = append(result, document)
		if len(result) == topK {
			break
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].RerankScore > result[j].RerankScore
	})
	return result, nil
}

var _ Reranker = (*OpenAIClient)(nil)
