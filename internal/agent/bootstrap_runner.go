package agent

import (
	"context"
	"strings"
)

type BootstrapRunner struct {
	mode string
}

func NewBootstrapRunner(mode string) *BootstrapRunner {
	return &BootstrapRunner{mode: mode}
}

func (r *BootstrapRunner) Run(ctx context.Context, req Request, sink EventSink) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if r.mode != "bootstrap" {
		return Result{}, ErrNotConfigured
	}

	if sink != nil {
		if err := sink(Event{
			Type: "status",
			Data: map[string]any{
				"stage":    "bootstrap",
				"mode":     r.mode,
				"trace_id": req.TraceID,
			},
		}); err != nil {
			return Result{}, err
		}
	}

	answer := "当前运行的是 bootstrap 模式，HTTP、会话与 SSE 链路已经可用，真实知识库检索和模型生成尚未接入。"
	for _, chunk := range splitForStream(answer, 16) {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if sink != nil {
			if err := sink(Event{Type: "delta", Data: map[string]string{"content": chunk}}); err != nil {
				return Result{}, err
			}
		}
	}

	return Result{
		Answer: answer,
		Mode:   r.mode,
	}, nil
}

func splitForStream(value string, size int) []string {
	if size <= 0 || value == "" {
		return []string{value}
	}
	runes := []rune(strings.TrimSpace(value))
	chunks := make([]string, 0, (len(runes)+size-1)/size)
	for start := 0; start < len(runes); start += size {
		end := min(start+size, len(runes))
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}
