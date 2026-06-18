package eval

import (
	"context"
	"strings"
)

type Case struct {
	CaseID              string         `json:"case_id"`
	Query               string         `json:"query"`
	Turns               []string       `json:"turns,omitempty"`
	Intent              string         `json:"intent"`
	Difficulty          string         `json:"difficulty"`
	ExpectedDocuments   []string       `json:"expected_docs"`
	ExpectedTools       []string       `json:"expected_tools"`
	ExpectedToolParams  map[string]any `json:"expected_tool_params"`
	StandardAnswer      string         `json:"standard_answer"`
	ShouldClarify       bool           `json:"should_clarify"`
	ShouldReject        bool           `json:"should_reject"`
	ExpectedEvidenceIDs []string       `json:"expected_evidence_ids"`
	Tags                []string       `json:"tags"`
}

func (c Case) ConversationTurns() []string {
	if len(c.Turns) == 0 {
		return []string{c.Query}
	}
	turns := make([]string, 0, len(c.Turns))
	for _, turn := range c.Turns {
		if turn = strings.TrimSpace(turn); turn != "" {
			turns = append(turns, turn)
		}
	}
	if len(turns) == 0 {
		return []string{c.Query}
	}
	return turns
}

func (c Case) EvaluationQuery() string {
	turns := c.ConversationTurns()
	return turns[len(turns)-1]
}

type AgentOutput struct {
	Intent              string         `json:"intent"`
	Documents           []string       `json:"documents"`
	Contexts            []string       `json:"contexts,omitempty"`
	Tools               []string       `json:"tools"`
	ToolParams          map[string]any `json:"tool_params"`
	ToolResults         []any          `json:"tool_results,omitempty"`
	Answer              string         `json:"answer"`
	EvidenceIDs         []string       `json:"evidence_ids"`
	LatencyMS           int64          `json:"latency_ms"`
	TokenCount          int            `json:"token_count"`
	StepCount           int            `json:"step_count"`
	SuccessfulToolCalls int            `json:"successful_tool_calls"`
	ReflectionAttempts  int            `json:"reflection_attempts"`
	ReflectionSucceeded bool           `json:"reflection_succeeded"`
	PlanRevisions       int            `json:"plan_revisions"`
}

type MetricResult struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Pass  bool    `json:"pass"`
}

type Evaluator interface {
	Evaluate(ctx context.Context, evalCase Case, output AgentOutput) ([]MetricResult, error)
}
