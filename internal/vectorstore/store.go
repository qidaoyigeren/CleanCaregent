package vectorstore

import "context"

type Point struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type SearchRequest struct {
	Vector         []float32      `json:"vector"`
	Limit          int            `json:"limit"`
	ScoreThreshold float64        `json:"score_threshold,omitempty"`
	Filter         map[string]any `json:"filter,omitempty"`
	WithPayload    bool           `json:"with_payload"`
}

type SearchResult struct {
	ID      any            `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload"`
}

type Store interface {
	Upsert(ctx context.Context, points []Point) error
	Search(ctx context.Context, request SearchRequest) ([]SearchResult, error)
	Delete(ctx context.Context, pointIDs []string) error
}
