package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"CleanCaregent/internal/config"
	"CleanCaregent/internal/vectorstore"
)

var ErrCollectionNotFound = errors.New("qdrant collection not found")

type Client struct {
	baseURL    string
	apiKey     string
	collection string
	vectorSize int
	distance   string
	httpClient *http.Client
}

func NewClient(cfg config.QdrantConfig) *Client {
	return &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     cfg.APIKey,
		collection: cfg.Collection,
		vectorSize: cfg.VectorSize,
		distance:   normalizeDistance(cfg.Distance),
		httpClient: &http.Client{Timeout: cfg.RequestTimeout},
	}
}

func (c *Client) Health(ctx context.Context) error {
	response, err := c.do(ctx, http.MethodGet, "/healthz", nil)
	if err != nil {
		return fmt.Errorf("qdrant health: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseError("qdrant health", response)
	}
	return nil
}

func (c *Client) EnsureCollection(ctx context.Context) error {
	exists, err := c.collectionExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	body := map[string]any{
		"vectors": map[string]any{
			"size":     c.vectorSize,
			"distance": c.distance,
		},
	}
	response, err := c.do(ctx, http.MethodPut, "/collections/"+url.PathEscape(c.collection), body)
	if err != nil {
		return fmt.Errorf("create qdrant collection: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseError("create qdrant collection", response)
	}
	return nil
}

func (c *Client) Upsert(ctx context.Context, points []vectorstore.Point) error {
	if len(points) == 0 {
		return nil
	}
	for _, point := range points {
		if len(point.Vector) != c.vectorSize {
			return fmt.Errorf("point %v vector size %d does not match collection size %d", point.ID, len(point.Vector), c.vectorSize)
		}
	}

	path := "/collections/" + url.PathEscape(c.collection) + "/points?wait=true"
	response, err := c.do(ctx, http.MethodPut, path, map[string]any{"points": points})
	if err != nil {
		return fmt.Errorf("upsert qdrant points: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseError("upsert qdrant points", response)
	}
	return nil
}

func (c *Client) Search(ctx context.Context, request vectorstore.SearchRequest) ([]vectorstore.SearchResult, error) {
	if len(request.Vector) != c.vectorSize {
		return nil, fmt.Errorf("query vector size %d does not match collection size %d", len(request.Vector), c.vectorSize)
	}
	if request.Limit <= 0 {
		request.Limit = 10
	}
	request.WithPayload = true

	path := "/collections/" + url.PathEscape(c.collection) + "/points/search"
	response, err := c.do(ctx, http.MethodPost, path, request)
	if err != nil {
		return nil, fmt.Errorf("search qdrant points: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, responseError("search qdrant points", response)
	}

	var payload struct {
		Result []vectorstore.SearchResult `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode qdrant search response: %w", err)
	}
	return payload.Result, nil
}

func (c *Client) Delete(ctx context.Context, pointIDs []string) error {
	if len(pointIDs) == 0 {
		return nil
	}
	path := "/collections/" + url.PathEscape(c.collection) + "/points/delete?wait=true"
	response, err := c.do(ctx, http.MethodPost, path, map[string]any{"points": pointIDs})
	if err != nil {
		return fmt.Errorf("delete qdrant points: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseError("delete qdrant points", response)
	}
	return nil
}

func (c *Client) collectionExists(ctx context.Context) (bool, error) {
	response, err := c.do(ctx, http.MethodGet, "/collections/"+url.PathEscape(c.collection), nil)
	if err != nil {
		return false, fmt.Errorf("check qdrant collection: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, responseError("check qdrant collection", response)
	}
	return true, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode qdrant request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("create qdrant request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		request.Header.Set("api-key", c.apiKey)
	}
	return c.httpClient.Do(request)
}

func responseError(operation string, response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	return fmt.Errorf("%s returned status %d: %s", operation, response.StatusCode, strings.TrimSpace(string(body)))
}

func normalizeDistance(value string) string {
	switch strings.ToLower(value) {
	case "dot":
		return "Dot"
	case "euclid":
		return "Euclid"
	case "manhattan":
		return "Manhattan"
	default:
		return "Cosine"
	}
}

func (c *Client) Name() string {
	return "qdrant"
}

func (c *Client) Check(ctx context.Context) error {
	return c.Health(ctx)
}
