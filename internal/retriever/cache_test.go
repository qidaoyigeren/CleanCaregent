package retriever

import (
	"context"
	"testing"
	"time"

	"CleanCaregent/internal/rag"
	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

type countingRetriever struct {
	calls int
}

func (r *countingRetriever) Search(
	context.Context,
	rag.SearchRequest,
) ([]rag.SearchResult, error) {
	r.calls++
	return []rag.SearchResult{{ChunkID: "kb_t20"}}, nil
}

func TestCachedRetrieverUsesStableRequestKey(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	next := &countingRetriever{}
	cached := NewCached(next, client, 5*time.Minute)
	request := rag.SearchRequest{
		Query: "T20 吸力",
		Mode:  rag.SearchHybrid,
		Filter: rag.MetadataFilter{
			Models:   []string{"T20"},
			DocTypes: []string{"product_parameter"},
		},
	}
	first, err := cached.Search(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cached.Search(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if next.calls != 1 {
		t.Fatalf("wrapped retriever calls = %d, want 1", next.calls)
	}
	if first[0].Metadata["retrieval_cache_hit"] != false {
		t.Fatalf("first metadata = %#v", first[0].Metadata)
	}
	if second[0].Metadata["retrieval_cache_hit"] != true {
		t.Fatalf("second metadata = %#v", second[0].Metadata)
	}
}
