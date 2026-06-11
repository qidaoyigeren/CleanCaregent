// Package llm provides a unified OpenAI-compatible chat client used across
// all LLM-dependent components (generator, intent, rewrite, plan, reflect).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client wraps an OpenAI-compatible chat completions endpoint.
type Client struct {
	endpoint    string
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
	breaker     *CircuitBreaker
	fallbacks   []*Client
}

// ToolDefinition is an OpenAI-compatible function tool declaration.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ToolCall is a model-selected function call with decoded JSON arguments.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type completionResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// NewClient creates a Client for the given endpoint and model.
func NewClient(
	endpoint string,
	apiKey string,
	model string,
	maxTokens int,
	temperature float64,
	timeout time.Duration,
) *Client {
	return &Client{
		endpoint:    endpoint,
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient:  &http.Client{Timeout: timeout},
		breaker:     NewCircuitBreaker(5, 30*time.Second),
	}
}

// Name returns the model identifier.
func (c *Client) Name() string { return c.model }

// WithFallbacks configures ordered provider fallbacks for every completion
// mode, including JSON responses and function calling.
func (c *Client) WithFallbacks(fallbacks ...*Client) *Client {
	c.fallbacks = append([]*Client(nil), fallbacks...)
	return c
}

// WithModel creates a model-specific client for lightweight routing while
// preserving the configured endpoint and fallback chain.
func (c *Client) WithModel(model string) *Client {
	if strings.TrimSpace(model) == "" || strings.TrimSpace(model) == c.model {
		return c
	}
	return (&Client{
		endpoint:    c.endpoint,
		apiKey:      c.apiKey,
		model:       strings.TrimSpace(model),
		maxTokens:   c.maxTokens,
		temperature: c.temperature,
		httpClient:  &http.Client{Timeout: c.httpClient.Timeout},
		breaker:     NewCircuitBreaker(5, 30*time.Second),
	}).WithFallbacks(c.fallbacks...)
}

// Chat sends messages and returns the assistant's response text.
func (c *Client) Chat(ctx context.Context, messages []map[string]string) (string, error) {
	return c.chat(ctx, messages, false)
}

func (c *Client) chat(ctx context.Context, messages []map[string]string, jsonMode bool) (string, error) {
	payload := map[string]any{
		"model":       c.model,
		"messages":    messages,
		"temperature": c.temperature,
		"max_tokens":  c.maxTokens,
		"stream":      false,
	}
	if jsonMode {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}
	response, err := c.complete(ctx, payload)
	if err != nil {
		return "", err
	}
	if len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("chat endpoint returned no answer")
	}
	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

// ChatWithTools lets the model select one of the supplied function tools.
// It returns assistant content when no tool was selected.
func (c *Client) ChatWithTools(
	ctx context.Context,
	messages []map[string]string,
	definitions []ToolDefinition,
) (string, []ToolCall, error) {
	tools := make([]map[string]any, 0, len(definitions))
	for _, definition := range definitions {
		parameters := any(map[string]any{"type": "object"})
		if len(definition.Parameters) > 0 {
			var decoded any
			if err := json.Unmarshal(definition.Parameters, &decoded); err != nil {
				return "", nil, fmt.Errorf("decode tool schema %s: %w", definition.Name, err)
			}
			parameters = decoded
		}
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        definition.Name,
				"description": definition.Description,
				"parameters":  parameters,
			},
		})
	}
	payload := map[string]any{
		"model":       c.model,
		"messages":    messages,
		"temperature": c.temperature,
		"max_tokens":  c.maxTokens,
		"stream":      false,
		"tools":       tools,
		"tool_choice": "auto",
	}
	response, err := c.complete(ctx, payload)
	if err != nil {
		return "", nil, err
	}
	if len(response.Choices) == 0 {
		return "", nil, fmt.Errorf("chat endpoint returned no choices")
	}
	message := response.Choices[0].Message
	calls := make([]ToolCall, 0, len(message.ToolCalls))
	for _, value := range message.ToolCalls {
		arguments := map[string]any{}
		if strings.TrimSpace(value.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(value.Function.Arguments), &arguments); err != nil {
				return "", nil, fmt.Errorf("decode tool call arguments for %s: %w", value.Function.Name, err)
			}
		}
		calls = append(calls, ToolCall{
			ID:        value.ID,
			Name:      value.Function.Name,
			Arguments: arguments,
		})
	}
	return strings.TrimSpace(message.Content), calls, nil
}

func (c *Client) complete(ctx context.Context, payload map[string]any) (completionResponse, error) {
	response, err := c.completeSingle(ctx, payload)
	if err == nil {
		return response, nil
	}
	failures := []string{c.model + ": " + err.Error()}
	for _, fallback := range c.fallbacks {
		if fallback == nil {
			continue
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return completionResponse{}, ctxErr
		}
		fallbackPayload := clonePayload(payload)
		fallbackPayload["model"] = fallback.model
		fallbackPayload["max_tokens"] = fallback.maxTokens
		fallbackPayload["temperature"] = fallback.temperature
		response, fallbackErr := fallback.completeSingle(ctx, fallbackPayload)
		if fallbackErr == nil {
			return response, nil
		}
		failures = append(failures, fallback.model+": "+fallbackErr.Error())
	}
	return completionResponse{}, fmt.Errorf("all chat providers failed: %s", strings.Join(failures, "; "))
}

func (c *Client) completeSingle(ctx context.Context, payload map[string]any) (completionResponse, error) {
	if c.breaker != nil && !c.breaker.Allow() {
		return completionResponse{}, ErrCircuitOpen
	}
	success := false
	defer func() {
		if c.breaker != nil {
			c.breaker.Record(success)
		}
	}()

	body, err := json.Marshal(payload)
	if err != nil {
		return completionResponse{}, fmt.Errorf("encode chat request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return completionResponse{}, fmt.Errorf("create chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return completionResponse{}, fmt.Errorf("call chat endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return completionResponse{}, fmt.Errorf("chat endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var response completionResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return completionResponse{}, fmt.Errorf("decode chat response: %w", err)
	}
	success = true
	collectUsage(ctx, Usage{
		PromptTokens:     response.Usage.PromptTokens,
		CompletionTokens: response.Usage.CompletionTokens,
		TotalTokens:      response.Usage.TotalTokens,
	})
	return response, nil
}

func clonePayload(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// ChatJSON sends messages and unmarshals the response into dest.
// The model is instructed to output only JSON via the system prompt.
func (c *Client) ChatJSON(ctx context.Context, messages []map[string]string, dest any) error {
	raw, err := c.chat(ctx, messages, true)
	if err != nil {
		return err
	}
	return DecodeJSON(raw, dest)
}

// DecodeJSON accepts plain JSON or a fenced JSON code block.
func DecodeJSON(raw string, dest any) error {
	// Strip markdown code fences if present.
	raw = stripCodeFences(raw)
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return fmt.Errorf("decode JSON response: %w\nraw: %.500s", err, raw)
	}
	return nil
}

func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		} else {
			s = strings.TrimPrefix(s, "```")
		}
		if strings.HasSuffix(s, "```") {
			s = strings.TrimSuffix(s, "```")
		}
	}
	return strings.TrimSpace(s)
}
