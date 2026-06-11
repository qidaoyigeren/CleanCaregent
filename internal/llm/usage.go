package llm

import (
	"context"
	"sync"
)

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	Calls            int `json:"calls"`
}

type UsageCollector struct {
	mu    sync.Mutex
	usage Usage
}

func (c *UsageCollector) Add(usage Usage) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.usage.PromptTokens += usage.PromptTokens
	c.usage.CompletionTokens += usage.CompletionTokens
	c.usage.TotalTokens += usage.TotalTokens
	c.usage.Calls++
}

func (c *UsageCollector) Snapshot() Usage {
	if c == nil {
		return Usage{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.usage
}

type usageCollectorKey struct{}

func WithUsageCollector(ctx context.Context, collector *UsageCollector) context.Context {
	return context.WithValue(ctx, usageCollectorKey{}, collector)
}

func collectUsage(ctx context.Context, usage Usage) {
	collector, _ := ctx.Value(usageCollectorKey{}).(*UsageCollector)
	collector.Add(usage)
}
