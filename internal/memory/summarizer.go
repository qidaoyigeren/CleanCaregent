package memory

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
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
		fallback: NewExtractiveSummarizer(600),
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
	summary := strings.TrimSpace(answer)
	if !summaryPreservesEntities(previous, messages, summary) {
		summary, err = s.fallback.Summarize(ctx, previous, messages)
		if err != nil {
			return "", err
		}
	}
	return enforceSummaryLimit(summary, 600), nil
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

var memoryEntityPattern = regexp.MustCompile(
	`(?i)\b(?:T20|X20\s*Pro|R10|R20|P400|P500|W300|W500|H100|H200|CC\d{8,})\b`,
)

func summaryPreservesEntities(previous string, messages []model.Message, summary string) bool {
	var source strings.Builder
	source.WriteString(previous)
	for _, message := range messages {
		source.WriteByte('\n')
		source.WriteString(message.Content)
	}
	sourceText := source.String()
	required := memoryEntityPattern.FindAllString(sourceText, -1)
	for _, keyword := range []string{
		"漏水", "焦味", "冒烟", "发热", "无法充电", "充不进电",
		"指示灯不亮", "已换插座", "已清洁触点",
	} {
		if strings.Contains(sourceText, keyword) {
			required = append(required, keyword)
		}
	}
	for _, entity := range uniqueFolded(required) {
		if !strings.Contains(strings.ToLower(summary), strings.ToLower(entity)) {
			return false
		}
	}
	return strings.TrimSpace(summary) != ""
}

func enforceSummaryLimit(summary string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(summary))
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[len(runes)-maxRunes:])
}

func uniqueFolded(values []string) []string {
	seen := make(map[string]string, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; !exists {
			seen[key] = value
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, seen[key])
	}
	return result
}
