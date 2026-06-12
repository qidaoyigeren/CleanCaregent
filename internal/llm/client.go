// Package llm provides a unified OpenAI-compatible chat client used across
// all LLM-dependent components (generator, intent, rewrite, plan, reflect).
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"CleanCaregent/internal/observability"
)

// Client wraps an OpenAI-compatible chat completions endpoint.
type Client struct {
	endpoint          string
	apiKey            string
	model             string
	maxTokens         int
	temperature       float64
	httpClient        *http.Client
	breaker           *CircuitBreaker
	fallbacks         []*Client
	firstTokenTimeout time.Duration
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
	client := &Client{
		endpoint:          endpoint,
		apiKey:            apiKey,
		model:             model,
		maxTokens:         maxTokens,
		temperature:       temperature,
		httpClient:        &http.Client{Timeout: timeout},
		breaker:           NewCircuitBreaker(5, time.Minute),
		firstTokenTimeout: minDuration(5*time.Second, timeout),
	}
	DefaultCircuitManager.Register("chat:"+model, client.breaker)
	return client
}

// Name returns the model identifier.
func (c *Client) Name() string { return c.model }

// WithFallbacks configures ordered provider fallbacks for every completion
// mode, including JSON responses and function calling.
func (c *Client) WithFallbacks(fallbacks ...*Client) *Client {
	c.fallbacks = append([]*Client(nil), fallbacks...)
	return c
}

// WithCircuitBreaker configures provider-local failure isolation.
func (c *Client) WithCircuitBreaker(failureThreshold int, openTimeout time.Duration) *Client {
	c.breaker = NewCircuitBreaker(failureThreshold, openTimeout)
	DefaultCircuitManager.Register("chat:"+c.model, c.breaker)
	return c
}

// WithFirstTokenTimeout configures how long streaming calls wait for the
// first non-empty content delta before the provider is considered failed.
func (c *Client) WithFirstTokenTimeout(timeout time.Duration) *Client {
	if timeout > 0 {
		c.firstTokenTimeout = timeout
	}
	return c
}

// WithModel creates a model-specific client for lightweight routing while
// preserving the configured endpoint and fallback chain.
func (c *Client) WithModel(model string) *Client {
	if strings.TrimSpace(model) == "" || strings.TrimSpace(model) == c.model {
		return c
	}
	return (&Client{
		endpoint:          c.endpoint,
		apiKey:            c.apiKey,
		model:             strings.TrimSpace(model),
		maxTokens:         c.maxTokens,
		temperature:       c.temperature,
		httpClient:        &http.Client{Timeout: c.httpClient.Timeout},
		breaker:           NewCircuitBreaker(5, time.Minute),
		firstTokenTimeout: c.firstTokenTimeout,
	}).WithFallbacks(c.fallbacks...)
}

// UseModel creates a model-specific client while preserving provider fallbacks.
func (c *Client) UseModel(model string) *Client {
	return c.WithModel(model)
}

// ChatStream consumes an OpenAI-compatible SSE response. Fallback providers
// are attempted only before any content has been emitted.
func (c *Client) ChatStream(
	ctx context.Context,
	messages []map[string]string,
	onDelta func(string) error,
) error {
	if onDelta == nil {
		return fmt.Errorf("stream delta callback is required")
	}
	payload := map[string]any{
		"model":       c.model,
		"messages":    messages,
		"temperature": c.temperature,
		"max_tokens":  c.maxTokens,
		"stream":      true,
		"stream_options": map[string]bool{
			"include_usage": true,
		},
	}
	emitted, err := c.streamSingle(ctx, payload, onDelta)
	if err == nil {
		return nil
	}
	if emitted {
		return err
	}
	failures := []string{c.model + ": " + err.Error()}
	observability.DefaultPrometheusMetrics.RecordFallback("chat_stream", fallbackReason(err))
	for _, fallback := range c.fallbacks {
		if fallback == nil {
			continue
		}
		fallbackPayload := clonePayload(payload)
		fallbackPayload["model"] = fallback.model
		fallbackPayload["max_tokens"] = fallback.maxTokens
		fallbackPayload["temperature"] = fallback.temperature
		emitted, fallbackErr := fallback.streamSingle(ctx, fallbackPayload, onDelta)
		if fallbackErr == nil {
			return nil
		}
		if emitted {
			return fallbackErr
		}
		failures = append(failures, fallback.model+": "+fallbackErr.Error())
	}
	return fmt.Errorf("all streaming chat providers failed: %s", strings.Join(failures, "; "))
}

type streamEvent struct {
	data string
	err  error
}

func (c *Client) streamSingle(
	ctx context.Context,
	payload map[string]any,
	onDelta func(string) error,
) (bool, error) {
	if c.breaker != nil && !c.breaker.Allow() {
		return false, ErrCircuitOpen
	}
	success := false
	defer func() {
		if c.breaker != nil {
			c.breaker.Record(success)
		}
	}()

	body, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("encode streaming chat request: %w", err)
	}
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("create streaming chat request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return false, fmt.Errorf("call streaming chat endpoint: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return false, fmt.Errorf(
			"streaming chat endpoint returned %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(raw)),
		)
	}

	events := make(chan streamEvent, 1)
	go scanSSE(response.Body, events)

	timer := time.NewTimer(c.firstTokenTimeout)
	defer timer.Stop()
	firstContent := false
	for {
		var timeout <-chan time.Time
		if !firstContent {
			timeout = timer.C
		}
		select {
		case <-ctx.Done():
			return firstContent, ctx.Err()
		case <-timeout:
			cancel()
			_ = response.Body.Close()
			return false, fmt.Errorf("streaming chat first token timeout after %s", c.firstTokenTimeout)
		case event, ok := <-events:
			if !ok {
				if !firstContent {
					return false, fmt.Errorf("streaming chat endpoint returned no content")
				}
				success = true
				return true, nil
			}
			if event.err != nil {
				return firstContent, event.err
			}
			if event.data == "[DONE]" {
				if !firstContent {
					return false, fmt.Errorf("streaming chat endpoint completed without content")
				}
				success = true
				return true, nil
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
				Usage Usage `json:"usage"`
			}
			if err := json.Unmarshal([]byte(event.data), &chunk); err != nil {
				return firstContent, fmt.Errorf("decode streaming chat event: %w", err)
			}
			if chunk.Usage.TotalTokens > 0 {
				chunk.Usage.Model = c.model
				chunk.Usage.CostUSD = EstimateCostUSD(
					c.model,
					chunk.Usage.PromptTokens,
					chunk.Usage.CompletionTokens,
				)
				collectUsage(ctx, chunk.Usage)
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content == "" {
					continue
				}
				if !firstContent {
					firstContent = true
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
				}
				if err := onDelta(choice.Delta.Content); err != nil {
					return true, fmt.Errorf("consume streaming chat delta: %w", err)
				}
			}
		}
	}
}

func scanSSE(reader io.Reader, events chan<- streamEvent) {
	defer close(events)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		events <- streamEvent{data: data}
	}
	if err := scanner.Err(); err != nil {
		events <- streamEvent{err: fmt.Errorf("read streaming chat response: %w", err)}
	}
}

func minDuration(left, right time.Duration) time.Duration {
	if right <= 0 || left < right {
		return left
	}
	return right
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
	observability.DefaultPrometheusMetrics.RecordFallback("chat", fallbackReason(err))
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

func fallbackReason(err error) string {
	if err == nil {
		return "unknown"
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrCircuitOpen):
		return "circuit_open"
	case strings.Contains(strings.ToLower(err.Error()), "first token"):
		return "first_token_timeout"
	default:
		return "provider_error"
	}
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
	usage := Usage{
		PromptTokens:     response.Usage.PromptTokens,
		CompletionTokens: response.Usage.CompletionTokens,
		TotalTokens:      response.Usage.TotalTokens,
		Model:            c.model,
	}
	usage.CostUSD = EstimateCostUSD(c.model, usage.PromptTokens, usage.CompletionTokens)
	collectUsage(ctx, usage)
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
