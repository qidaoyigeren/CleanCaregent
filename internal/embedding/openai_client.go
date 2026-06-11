package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type OpenAIClient struct {
	endpoint   string
	apiKey     string
	model      string
	dimension  int
	batchSize  int
	httpClient *http.Client
}

func NewOpenAIClient(
	endpoint string,
	apiKey string,
	model string,
	dimension int,
	batchSize int,
	timeout time.Duration,
) *OpenAIClient {
	return &OpenAIClient{
		endpoint:   endpoint,
		apiKey:     apiKey,
		model:      model,
		dimension:  dimension,
		batchSize:  batchSize,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *OpenAIClient) Name() string {
	return c.model
}

func (c *OpenAIClient) Dimension() int {
	return c.dimension
}

func (c *OpenAIClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	result := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += c.batchSize {
		end := min(start+c.batchSize, len(texts))
		batch, err := c.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		result = append(result, batch...)
	}
	return result, nil
}

func (c *OpenAIClient) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{
		"model": c.model,
		"input": texts,
	})
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call embedding endpoint: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("embedding endpoint returned %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}

	var payload struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(payload.Data) != len(texts) {
		return nil, fmt.Errorf("embedding response count %d does not match request count %d", len(payload.Data), len(texts))
	}
	sort.Slice(payload.Data, func(i, j int) bool {
		return payload.Data[i].Index < payload.Data[j].Index
	})
	vectors := make([][]float32, len(payload.Data))
	for index, item := range payload.Data {
		if len(item.Embedding) != c.dimension {
			return nil, fmt.Errorf("embedding dimension %d does not match configured dimension %d", len(item.Embedding), c.dimension)
		}
		vectors[index] = item.Embedding
	}
	return vectors, nil
}
