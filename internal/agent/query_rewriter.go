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
	contextEntities := recentUserRewriteEntities(request.RecentMessages, request.Summary)
	if shouldCarryRewriteContext(request, entities, contextEntities) {
		mergeMissingRewriteEntities(entities, contextEntities,
			"models", "category", "categories", "area", "budget", "pets", "has_carpet", "accessory_refs",
		)
	}
	if entities["models"] == "" && containsReference(query) {
		if modelName := latestMentionedModel(request.RecentMessages, request.Summary); modelName != "" {
			entities["models"] = modelName
			query = modelName + " " + query
		}
	}
	if request.Intent.Secondary == intent.PurchaseRecommendation {
		query = contextualRecommendationQuery(query, entities)
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

func recentUserRewriteEntities(messages []model.Message, summary string) map[string]string {
	result := map[string]string{}
	if strings.TrimSpace(summary) != "" {
		overwriteRewriteEntities(result, intent.ExtractContextEntities(summary),
			"models", "category", "categories", "area", "budget", "pets", "has_carpet", "accessory_refs",
		)
	}
	for _, message := range messages {
		if message.Role != "user" {
			continue
		}
		overwriteRewriteEntities(result, intent.ExtractContextEntities(message.Content),
			"models", "category", "categories", "area", "budget", "pets", "has_carpet", "accessory_refs",
		)
	}
	return result
}

func shouldCarryRewriteContext(request RewriteRequest, entities, contextEntities map[string]string) bool {
	if !hasRewriteSlot(contextEntities) {
		return false
	}
	if request.Intent.Secondary == intent.PurchaseRecommendation {
		return true
	}
	if hasRewriteConstraint(entities) {
		return true
	}
	query := strings.ToLower(strings.TrimSpace(request.Query))
	return len([]rune(query)) <= 24 &&
		containsAnyRewrite(query, "扫地", "净化器", "净水器", "加湿器", "预算", "地面", "家庭", "平", "地毯", "宠物")
}

func contextualRecommendationQuery(query string, entities map[string]string) string {
	parts := make([]string, 0, 5)
	if category := categoryLabel(entities["category"]); category != "" && !strings.Contains(query, category) {
		parts = append(parts, category)
	}
	if area := strings.TrimSpace(entities["area"]); area != "" && !strings.Contains(query, area) {
		parts = append(parts, area)
	}
	if budget := strings.TrimSpace(entities["budget"]); budget != "" && !strings.Contains(query, budget) {
		parts = append(parts, "预算"+budget+"元")
	}
	if strings.TrimSpace(entities["pets"]) != "" && !containsAnyRewrite(query, "宠物", "养猫", "猫毛", "养狗", "狗毛") {
		parts = append(parts, "宠物毛发")
	}
	if strings.TrimSpace(entities["has_carpet"]) != "" && !strings.Contains(query, "地毯") {
		parts = append(parts, "有地毯")
	}
	if len(parts) == 0 {
		return query
	}
	return strings.TrimSpace(query + " " + strings.Join(parts, " "))
}

func categoryLabel(category string) string {
	switch strings.TrimSpace(category) {
	case "robot_vacuum":
		return "扫地机器人"
	case "air_purifier":
		return "空气净化器"
	case "water_purifier":
		return "净水器"
	case "humidifier":
		return "加湿器"
	default:
		return ""
	}
}

func hasRewriteSlot(entities map[string]string) bool {
	if len(entities) == 0 {
		return false
	}
	for _, key := range []string{"models", "category", "categories", "area", "budget", "pets", "has_carpet", "accessory_refs"} {
		if strings.TrimSpace(entities[key]) != "" {
			return true
		}
	}
	return false
}

func hasRewriteConstraint(entities map[string]string) bool {
	for _, key := range []string{"area", "budget", "pets", "has_carpet"} {
		if strings.TrimSpace(entities[key]) != "" {
			return true
		}
	}
	return false
}

func mergeMissingRewriteEntities(target, source map[string]string, keys ...string) {
	if target == nil || source == nil {
		return
	}
	for _, key := range keys {
		if strings.TrimSpace(target[key]) == "" && strings.TrimSpace(source[key]) != "" {
			target[key] = source[key]
		}
	}
}

func overwriteRewriteEntities(target, source map[string]string, keys ...string) {
	if target == nil || source == nil {
		return
	}
	for _, key := range keys {
		if strings.TrimSpace(source[key]) != "" {
			target[key] = source[key]
		}
	}
}

func containsAnyRewrite(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
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
