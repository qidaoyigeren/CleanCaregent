package eval

import "context"

type Case struct {
	CaseID              string         `json:"case_id"`
	Query               string         `json:"query"`
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

type AgentOutput struct {
	Intent      string         `json:"intent"`
	Documents   []string       `json:"documents"`
	Contexts    []string       `json:"contexts,omitempty"`
	Tools       []string       `json:"tools"`
	ToolParams  map[string]any `json:"tool_params"`
	Answer      string         `json:"answer"`
	EvidenceIDs []string       `json:"evidence_ids"`
	LatencyMS   int64          `json:"latency_ms"`
	TokenCount  int            `json:"token_count"`
	StepCount   int            `json:"step_count"`
}

type MetricResult struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Pass  bool    `json:"pass"`
}

type Evaluator interface {
	Evaluate(ctx context.Context, evalCase Case, output AgentOutput) ([]MetricResult, error)
}
