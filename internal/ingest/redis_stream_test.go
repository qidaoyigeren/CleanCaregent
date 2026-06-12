package ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CleanCaregent/internal/service"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

type recordingIngester struct {
	request service.IngestDocumentRequest
	err     error
}

func (i *recordingIngester) Ingest(
	_ context.Context,
	request service.IngestDocumentRequest,
) (service.IngestDocumentResult, error) {
	i.request = request
	return service.IngestDocumentResult{DocID: request.DocID, ChunkCount: 2}, i.err
}

func TestNormalizeContentSupportsJSONMarkdownAndText(t *testing.T) {
	value, err := NormalizeContent("json", `{"型号":"T20","吸力":6000}`)
	if err != nil || value == "" {
		t.Fatalf("NormalizeContent(json) = %q, %v", value, err)
	}
	for _, format := range []string{"markdown", "text"} {
		if _, err := NormalizeContent(format, "T20 content"); err != nil {
			t.Fatalf("NormalizeContent(%s): %v", format, err)
		}
	}
	if _, err := NormalizeContent("xml", "<doc/>"); err == nil {
		t.Fatal("NormalizeContent(xml) expected error")
	}
}

func TestRedisStreamEnqueueAndProcess(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	ingester := &recordingIngester{}
	stream := NewRedisStream(client, ingester, StreamConfig{
		Stream:     "kb:ingest",
		Group:      "workers",
		Consumer:   "worker-1",
		DeadLetter: "kb:ingest:dead",
		Block:      10 * time.Millisecond,
		ClaimIdle:  time.Minute,
		BatchSize:  1,
		MaxRetries: 2,
	}, nil)
	ctx := context.Background()
	if err := stream.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	queued, err := stream.Enqueue(ctx, service.IngestDocumentRequest{
		DocID: "kb_test",
		Title: "T20",
	})
	if err != nil || queued.JobID == "" {
		t.Fatalf("Enqueue() = %#v, %v", queued, err)
	}
	values, err := client.XRange(ctx, "kb:ingest", "-", "+").Result()
	if err != nil || len(values) != 1 {
		t.Fatalf("XRange() = %#v, %v", values, err)
	}
	var job streamJob
	if err := json.Unmarshal([]byte(values[0].Values["payload"].(string)), &job); err != nil {
		t.Fatal(err)
	}
	stream.processMessage(ctx, values[0])
	if ingester.request.DocID != "kb_test" {
		t.Fatalf("ingested request = %#v", ingester.request)
	}
}
