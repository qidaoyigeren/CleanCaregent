package llm

import (
	"context"
	"strings"
	"sync"
)

type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Calls            int     `json:"calls"`
	Model            string  `json:"model,omitempty"`
	CostUSD          float64 `json:"cost_usd"`
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
	c.usage.CostUSD += usage.CostUSD
	if usage.Model != "" {
		c.usage.Model = usage.Model
	}
}

type modelPrice struct {
	inputPerMillion  float64
	outputPerMillion float64
}

var modelPrices = map[string]modelPrice{
	"qwen-turbo":    {inputPerMillion: 0.05, outputPerMillion: 0.20},
	"qwen-plus":     {inputPerMillion: 0.11, outputPerMillion: 0.28},
	"deepseek-chat": {inputPerMillion: 0.27, outputPerMillion: 1.10},
	"gpt-4o-mini":   {inputPerMillion: 0.15, outputPerMillion: 0.60},
	"gpt-4.1-mini":  {inputPerMillion: 0.40, outputPerMillion: 1.60},
}

// EstimateCostUSD estimates token cost using the built-in model price table.
func EstimateCostUSD(model string, promptTokens, completionTokens int) float64 {
	lower := strings.ToLower(strings.TrimSpace(model))
	var price modelPrice
	for name, candidate := range modelPrices {
		if strings.Contains(lower, name) {
			price = candidate
			break
		}
	}
	return float64(promptTokens)*price.inputPerMillion/1_000_000 +
		float64(completionTokens)*price.outputPerMillion/1_000_000
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
