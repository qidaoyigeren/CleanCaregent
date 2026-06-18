package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/prompt"
)

// llmRewriteResult mirrors the JSON structure the LLM returns for query rewriting.
type llmRewriteResult struct {
	Original         string            `json:"original"`
	Rewritten        string            `json:"rewritten"`
	SearchQueries    []string          `json:"search_queries"`
	SubQuestions     []llmSubQuestion  `json:"sub_questions"`
	ResolvedEntities map[string]any    `json:"resolved_entities"`
	UnresolvedSlots  []string          `json:"unresolved_slots"`
	NeedToolCalls    []string          `json:"need_tool_calls"`
	TermsMapping     map[string]string `json:"terms_mapping"`
}

type llmSubQuestion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Type string `json:"type"`
}

// LLMQueryRewriter uses an LLM to rewrite queries with anaphora resolution,
// sub-question decomposition, and term normalization.
type LLMQueryRewriter struct {
	llm     *llm.Client
	prompts *prompt.Registry
	// Fallback to rule-based rewriting when LLM is unavailable.
	fallback *RuleQueryRewriter
}

// NewLLMQueryRewriter creates an LLM-backed query rewriter.
// If llmClient is nil, degrades to the rule-based rewriter.
func NewLLMQueryRewriter(llmClient *llm.Client, prompts *prompt.Registry) *LLMQueryRewriter {
	return &LLMQueryRewriter{
		llm:      llmClient,
		prompts:  prompts,
		fallback: NewRuleQueryRewriter(),
	}
}

// Rewrite performs LLM-based query rewriting with rule-based fallback.
func (r *LLMQueryRewriter) Rewrite(ctx context.Context, request RewriteRequest) (RewriteResult, error) {
	if r.llm == nil || r.prompts == nil || !shouldUseLLMRewrite(request) {
		return r.fallback.Rewrite(ctx, request)
	}

	tmpl, err := r.prompts.Get(prompt.ScenarioRewrite)
	if err != nil {
		return r.fallback.Rewrite(ctx, request)
	}

	knownEntitiesJSON, _ := json.Marshal(request.Intent.Entities)
	recentMessagesJSON, _ := json.Marshal(request.RecentMessages)
	params := map[string]string{
		"summary":         request.Summary,
		"recent_messages": string(recentMessagesJSON),
		"known_entities":  string(knownEntitiesJSON),
		"intent_type":     string(request.Intent.Secondary),
		"query":           request.Query,
	}
	messages := tmpl.BuildMessages(params)

	var llmOut llmRewriteResult
	if err := r.llm.ChatJSON(ctx, messages, &llmOut); err != nil {
		// Degrade to rule-based on LLM failure.
		return r.fallback.Rewrite(ctx, request)
	}

	// Merge with rule-based entities (rule is better at regex extraction).
	entities := cloneStringMap(request.Intent.Entities)
	for key, value := range llmOut.ResolvedEntities {
		if strings.TrimSpace(entities[key]) != "" {
			continue
		}
		if normalized := rewriteEntityString(value); normalized != "" {
			entities[key] = normalized
		}
	}

	rewritten := llmOut.Rewritten
	if rewritten == "" {
		rewritten = request.Query
	}

	subQuestions := make([]string, 0, len(llmOut.SubQuestions))
	for _, sq := range llmOut.SubQuestions {
		if sq.Text != "" {
			subQuestions = append(subQuestions, sq.Text)
		}
	}
	if len(subQuestions) == 0 {
		subQuestions = []string{rewritten}
	}

	return RewriteResult{
		Original:        request.Query,
		Rewritten:       rewritten,
		SearchQueries:   uniqueStrings(llmOut.SearchQueries),
		SubQuestions:    subQuestions,
		Entities:        entities,
		UnresolvedSlots: uniqueStrings(llmOut.UnresolvedSlots),
		NeedToolCalls:   uniqueStrings(llmOut.NeedToolCalls),
		TermsMapping:    llmOut.TermsMapping,
	}, nil
}

func shouldUseLLMRewrite(request RewriteRequest) bool {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return false
	}
	if containsReference(query) {
		return true
	}
	if len(request.Intent.Entities) > 0 && request.Intent.Confidence >= 0.75 {
		return false
	}
	switch request.Intent.Secondary {
	case intent.ProductComparison, intent.PurchaseRecommendation:
		return false
	case intent.Chitchat, intent.OutOfScope, intent.Clarification:
		return false
	}
	return request.Intent.Confidence < 0.9 || request.Intent.NeedClarify
}

func rewriteEntityString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case bool:
		return fmt.Sprint(typed)
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", typed), "0"), ".")
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if normalized := rewriteEntityString(item); normalized != "" {
				values = append(values, normalized)
			}
		}
		return strings.Join(values, ",")
	default:
		return ""
	}
}

// Ensure LLMQueryRewriter implements the interface.
var _ QueryRewriter = (*LLMQueryRewriter)(nil)
