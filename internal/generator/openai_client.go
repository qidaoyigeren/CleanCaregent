package generator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
)

// OpenAIClient generates answers using an OpenAI-compatible chat API with
// scenario-aware prompt templates from the prompt registry.
type OpenAIClient struct {
	client  *llm.Client
	prompts *prompt.Registry
}

// NewOpenAIClient creates a generator backed by an LLM client and prompt templates.
func NewOpenAIClient(
	endpoint string,
	apiKey string,
	model string,
	maxTokens int,
	temperature float64,
	timeout time.Duration,
	prompts *prompt.Registry,
) *OpenAIClient {
	return &OpenAIClient{
		client:  llm.NewClient(endpoint, apiKey, model, maxTokens, temperature, timeout),
		prompts: prompts,
	}
}

func NewOpenAIClientFromClient(
	client *llm.Client,
	prompts *prompt.Registry,
) *OpenAIClient {
	return &OpenAIClient{client: client, prompts: prompts}
}

// Name returns the model identifier.
func (c *OpenAIClient) Name() string {
	return c.client.Name()
}

// Generate produces an answer using the generic generation template.
// Use GenerateWithScenario for intent-specific templates.
func (c *OpenAIClient) Generate(ctx context.Context, query string, evidence []rag.SearchResult) (string, error) {
	return c.GenerateWithScenario(ctx, prompt.ScenarioGenerateGeneric, query, evidence, "", "", "")
}

// GenerateWithScenario produces an answer using a scenario-specific template.
// scenario selects the prompt template (generic, compare, diagnose, policy).
// extra fields (toolResults, conversationSummary, models) provide additional context.
func (c *OpenAIClient) GenerateWithScenario(
	ctx context.Context,
	scenario prompt.Scenario,
	query string,
	evidence []rag.SearchResult,
	toolResults string,
	conversationSummary string,
	models string,
) (string, error) {
	tmpl, err := c.prompts.Get(scenario)
	if err != nil {
		return "", fmt.Errorf("get prompt template %s: %w", scenario, err)
	}

	evidenceContext := buildEvidenceContext(evidence)
	if evidenceContext == "" {
		evidenceContext = "(无证据)"
	}
	if toolResults == "" {
		toolResults = "(无工具调用结果)"
	}
	if conversationSummary == "" {
		conversationSummary = "(新会话)"
	}

	params := map[string]string{
		"query":                query,
		"evidence_context":     evidenceContext,
		"tool_results":         toolResults,
		"conversation_summary": conversationSummary,
		"models":               models,
		"model":                models,
		"concerns":             "",
		"symptom":              "",
		"diagnosis_state":      "",
		"current_node":         "",
		"order_info":           "",
		"warranty_info":        "",
	}

	messages := tmpl.BuildMessages(params)
	var answer strings.Builder
	if err := c.client.ChatStream(ctx, messages, func(delta string) error {
		answer.WriteString(delta)
		return nil
	}); err != nil {
		return "", err
	}
	if strings.TrimSpace(answer.String()) == "" {
		return "", fmt.Errorf("streaming generation returned no answer")
	}
	return strings.TrimSpace(answer.String()), nil
}

// buildEvidenceContext formats search results into the [EN] evidence format.
func buildEvidenceContext(evidence []rag.SearchResult) string {
	var builder strings.Builder
	for index, item := range evidence {
		fmt.Fprintf(&builder, "[E%d] 标题：%s\n文档：%s\n内容：%s\n\n",
			index+1,
			item.Title,
			item.DocumentID,
			item.Content,
		)
	}
	return builder.String()
}
