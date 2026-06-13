package agent

import (
	"context"
	"regexp"
	"strings"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/model"
)

type RewriteRequest struct {
	Query          string
	Intent         intent.Result
	Summary        string
	RecentMessages []model.Message
}

type RewriteResult struct {
	Original        string            `json:"original"`
	Rewritten       string            `json:"rewritten"`
	SearchQueries   []string          `json:"search_queries,omitempty"`
	SubQuestions    []string          `json:"sub_questions"`
	Entities        map[string]string `json:"entities"`
	UnresolvedSlots []string          `json:"unresolved_slots,omitempty"`
	NeedToolCalls   []string          `json:"need_tool_calls,omitempty"`
	TermsMapping    map[string]string `json:"terms_mapping,omitempty"`
}

type QueryRewriter interface {
	Rewrite(ctx context.Context, request RewriteRequest) (RewriteResult, error)
}

type RuleQueryRewriter struct{}

var constraintSplitPattern = regexp.MustCompile(`[，,；;。?？]+`)

func NewRuleQueryRewriter() *RuleQueryRewriter {
	return &RuleQueryRewriter{}
}

func (r *RuleQueryRewriter) Rewrite(ctx context.Context, request RewriteRequest) (RewriteResult, error) {
	if err := ctx.Err(); err != nil {
		return RewriteResult{}, err
	}
	query := strings.TrimSpace(request.Query)
	entities := cloneStringMap(request.Intent.Entities)
	if entities["models"] == "" && containsReference(query) {
		if modelName := latestMentionedModel(request.RecentMessages, request.Summary); modelName != "" {
			entities["models"] = modelName
			query = modelName + " " + query
		}
	}

	subQuestions := []string{query}
	if request.Intent.Secondary == intent.ProductComparison {
		models := splitCSV(entities["models"])
		subQuestions = subQuestions[:0]
		for _, modelName := range models {
			subQuestions = append(subQuestions, modelName+" 核心参数与适用场景")
		}
		subQuestions = append(subQuestions, query+" 对比维度")
	}
	if request.Intent.Secondary == intent.PurchaseRecommendation {
		parts := constraintSplitPattern.Split(query, -1)
		subQuestions = []string{query + " 选购约束与候选商品"}
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				subQuestions = append(subQuestions, part)
			}
		}
	}
	return RewriteResult{
		Original:      request.Query,
		Rewritten:     query,
		SearchQueries: uniqueStrings(subQuestions),
		SubQuestions:  uniqueStrings(subQuestions),
		Entities:      entities,
	}, nil
}

func containsReference(query string) bool {
	for _, value := range []string{
		"那个", "这个", "那台", "这台", "那款", "这款", "前者", "后者", "它", "我买的",
	} {
		if strings.Contains(query, value) {
			return true
		}
	}
	return false
}

func latestMentionedModel(messages []model.Message, summary string) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if matches := productModelPattern.FindAllString(messages[index].Content, -1); len(matches) > 0 {
			return strings.Join(strings.Fields(matches[len(matches)-1]), " ")
		}
	}
	if matches := productModelPattern.FindAllString(summary, -1); len(matches) > 0 {
		return strings.Join(strings.Fields(matches[len(matches)-1]), " ")
	}
	return ""
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	return uniqueStrings(strings.Split(value, ","))
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
