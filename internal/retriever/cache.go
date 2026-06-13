package retriever

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"CleanCaregent/internal/rag"

	goredis "github.com/redis/go-redis/v9"
)

const retrievalCacheVersion = "v3"

type Cached struct {
	next   rag.Retriever
	client goredis.UniversalClient
	ttl    time.Duration
}

// NewCached wraps a Retriever with a best-effort Redis result cache.
func NewCached(
	next rag.Retriever,
	client goredis.UniversalClient,
	ttl time.Duration,
) rag.Retriever {
	if next == nil || client == nil || ttl <= 0 {
		return next
	}
	return &Cached{next: next, client: client, ttl: ttl}
}

// Search returns a cached result when available and falls back to the wrapped
// retriever on cache misses, malformed entries, or Redis failures.
func (c *Cached) Search(
	ctx context.Context,
	request rag.SearchRequest,
) ([]rag.SearchResult, error) {
	key := retrievalCacheKey(request)
	if raw, err := c.client.Get(ctx, key).Bytes(); err == nil {
		var results []rag.SearchResult
		if json.Unmarshal(raw, &results) == nil {
			markCacheState(results, true)
			return results, nil
		}
	}
	results, err := c.next.Search(ctx, request)
	if err != nil {
		return nil, err
	}
	markCacheState(results, false)
	if raw, marshalErr := json.Marshal(results); marshalErr == nil {
		_ = c.client.Set(context.WithoutCancel(ctx), key, raw, c.ttl).Err()
	}
	return results, nil
}

func retrievalCacheKey(request rag.SearchRequest) string {
	raw, _ := json.Marshal(request)
	sum := sha256.Sum256(raw)
	return "cleancare:retrieval:" + retrievalCacheVersion + ":" + hex.EncodeToString(sum[:])
}

func markCacheState(results []rag.SearchResult, hit bool) {
	for index := range results {
		if results[index].Metadata == nil {
			results[index].Metadata = map[string]any{}
		}
		results[index].Metadata["retrieval_cache_hit"] = hit
	}
}

var _ rag.Retriever = (*Cached)(nil)
