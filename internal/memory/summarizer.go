package memory

import (
	"context"
	"encoding/json"
	"strings"

	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/model"
	"CleanCaregent/internal/prompt"
)

type Summarizer interface {
	Summarize(ctx context.Context, previous string, messages []model.Message) (string, error)
}

type LLMSummarizer struct {
	client   *llm.Client
	prompts  *prompt.Registry
	fallback Summarizer
}

func NewLLMSummarizer(client *llm.Client, prompts *prompt.Registry) *LLMSummarizer {
	return &LLMSummarizer{
		client:   client,
		prompts:  prompts,
		fallback: NewExtractiveSummarizer(900),
	}
}

func (s *LLMSummarizer) Summarize(
	ctx context.Context,
	previous string,
	messages []model.Message,
) (string, error) {
	if s.client == nil || s.prompts == nil {
		return s.fallback.Summarize(ctx, previous, messages)
	}
	template, err := s.prompts.Get(prompt.ScenarioSummarize)
	if err != nil {
		return s.fallback.Summarize(ctx, previous, messages)
	}
	raw, _ := json.Marshal(messages)
	answer, err := s.client.Chat(ctx, template.BuildMessages(map[string]string{
		"previous_summary": previous,
		"messages":         string(raw),
	}))
	if err != nil {
		return s.fallback.Summarize(ctx, previous, messages)
	}
	return strings.TrimSpace(answer), nil
}

type ExtractiveSummarizer struct {
	maxRunes int
}

func NewExtractiveSummarizer(maxRunes int) *ExtractiveSummarizer {
	return &ExtractiveSummarizer{maxRunes: maxRunes}
}

func (s *ExtractiveSummarizer) Summarize(
	ctx context.Context,
	previous string,
	messages []model.Message,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var builder strings.Builder
	if strings.TrimSpace(previous) != "" {
		builder.WriteString(strings.TrimSpace(previous))
		builder.WriteString("\n")
	}
	for _, message := range messages {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		content := strings.Join(strings.Fields(message.Content), " ")
		if content == "" {
			continue
		}
		builder.WriteString(message.Role)
		builder.WriteString(": ")
		builder.WriteString(content)
		builder.WriteString("\n")
	}
	runes := []rune(strings.TrimSpace(builder.String()))
	if s.maxRunes > 0 && len(runes) > s.maxRunes {
		runes = runes[len(runes)-s.maxRunes:]
	}
	return string(runes), nil
}
