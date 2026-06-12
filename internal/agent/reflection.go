package agent

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"CleanCaregent/internal/intent"
)

type ReflectionResult struct {
	Answer            string   `json:"answer"`
	LowConfidence     bool     `json:"low_confidence"`
	ShouldTransfer    bool     `json:"should_transfer"`
	UnsupportedClaims []string `json:"unsupported_claims,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
	Action            string   `json:"action,omitempty"`
	RerunQuery        string   `json:"rerun_query,omitempty"`
}

type Reflector interface {
	Review(query string, intentType intent.Type, answer string, evidences []Evidence) ReflectionResult
}

type ReflectionRequest struct {
	Query                    string
	Intent                   intent.Type
	Answer                   string
	Evidences                []Evidence
	SubQuestions             []string
	IntentionalClarification bool
}

type ContextualReflector interface {
	Reflector
	ReviewContext(ctx context.Context, request ReflectionRequest) ReflectionResult
}

type GroundingReflector struct{}

var (
	citationPattern     = regexp.MustCompile(`\[E([0-9]+)\]`)
	numericClaimPattern = regexp.MustCompile(`(?i)\b[0-9]+(?:\.[0-9]+)?\s*(?:pa|w|㎡|m²|平米|元|个月|天|%)`)
	numericPartsPattern = regexp.MustCompile(`(?i)\b([0-9]+(?:\.[0-9]+)?)\s*(pa|w|㎡|m²|平米|元|个月|天|%)`)
)

func NewGroundingReflector() *GroundingReflector {
	return &GroundingReflector{}
}

func (r *GroundingReflector) Review(
	query string,
	intentType intent.Type,
	answer string,
	evidences []Evidence,
) ReflectionResult {
	result := ReflectionResult{Answer: strings.TrimSpace(answer)}
	if result.Answer == "" {
		result.Answer = "当前没有形成可靠答案，请补充具体型号、订单号或故障现象。"
		result.LowConfidence = true
		result.Warnings = append(result.Warnings, "empty_answer")
		return result
	}

	evidenceText := normalizeGroundingText(query + "\n" + joinEvidence(evidences))
	toolEvidence, toolFailure := evidenceKinds(evidences)
	if low, scores := lowEvidenceRelevance(evidences); low {
		result.LowConfidence = true
		result.Action = "rerun_retrieval"
		result.RerunQuery = query
		result.Warnings = append(
			result.Warnings,
			fmt.Sprintf("low_evidence_relevance_top3:%v", scores),
		)
	}
	if requiresDynamicEvidence(intentType) && !toolEvidence {
		result.LowConfidence = true
		result.Warnings = append(result.Warnings, "missing_dynamic_tool_evidence")
	}
	if toolFailure {
		result.LowConfidence = true
		result.Warnings = append(result.Warnings, "tool_execution_failed")
	}

	for _, match := range citationPattern.FindAllStringSubmatch(result.Answer, -1) {
		index, _ := strconv.Atoi(match[1])
		if index < 1 || index > len(evidences) {
			result.Warnings = append(result.Warnings, "invalid_evidence_citation:"+match[0])
			result.LowConfidence = true
		}
	}
	for _, claim := range numericClaimPattern.FindAllString(result.Answer, -1) {
		if !numericClaimGrounded(claim, evidenceText, evidences) {
			result.UnsupportedClaims = appendUnique(result.UnsupportedClaims, claim)
		}
	}
	if len(result.UnsupportedClaims) > 0 {
		result.Answer = "当前回答包含无法从知识库或工具结果确认的数值，已停止输出该结论。请补充型号或由人工客服复核。"
		result.LowConfidence = true
		result.ShouldTransfer = isHighRiskIntent(intentType)
		result.Warnings = append(result.Warnings, "unsupported_numeric_claim")
		return result
	}

	for _, facet := range requestedFacets(query) {
		if !answerCoversFacet(result.Answer, facet) {
			result.Warnings = append(result.Warnings, "possibly_missing_subanswer:"+facet)
			result.LowConfidence = true
		}
	}
	if len(evidences) == 0 && intentType != intent.Chitchat && intentType != intent.OutOfScope &&
		intentType != intent.Clarification {
		result.LowConfidence = true
		result.Warnings = append(result.Warnings, "no_evidence")
	}
	if result.LowConfidence && isHighRiskIntent(intentType) {
		result.ShouldTransfer = true
	}
	return result
}

func lowEvidenceRelevance(evidences []Evidence) (bool, []float64) {
	scores := make([]float64, 0, 3)
	for _, evidence := range evidences {
		if evidence.Kind != "kb_chunk" || evidence.Metadata == nil {
			continue
		}
		score, ok := numericMetadata(evidence.Metadata["rerank_score"])
		if !ok {
			continue
		}
		scores = append(scores, score)
		if len(scores) == 3 {
			break
		}
	}
	if len(scores) == 0 {
		return false, nil
	}
	for _, score := range scores {
		if score >= 0.3 {
			return false, scores
		}
	}
	return true, scores
}

func numericMetadata(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func numericClaimGrounded(
	claim string,
	normalizedSources string,
	evidences []Evidence,
) bool {
	normalizedClaim := normalizeGroundingText(claim)
	if strings.Contains(normalizedSources, normalizedClaim) {
		return true
	}
	parts := numericPartsPattern.FindStringSubmatch(claim)
	if len(parts) != 3 {
		return false
	}
	number := normalizeGroundingText(parts[1])
	unit := normalizeGroundingText(parts[2])
	if !strings.Contains(normalizedSources, number) {
		return false
	}
	switch unit {
	case "元":
		hasTool, _ := evidenceKinds(evidences)
		return hasTool || strings.Contains(normalizedSources, number+"元")
	case "平米":
		return strings.Contains(normalizedSources, "平米") ||
			strings.Contains(normalizedSources, "area_m2")
	case "pa":
		return strings.Contains(normalizedSources, "pa") ||
			strings.Contains(normalizedSources, "suction_pa")
	default:
		return strings.Contains(normalizedSources, unit)
	}
}

func normalizeGroundingText(value string) string {
	value = strings.ToLower(value)
	value = strings.NewReplacer(
		"m²", "平米",
		"㎡", "平米",
	).Replace(value)
	return strings.Map(func(current rune) rune {
		if unicode.IsSpace(current) {
			return -1
		}
		return current
	}, value)
}

func (r *GroundingReflector) ReviewContext(
	_ context.Context,
	request ReflectionRequest,
) ReflectionResult {
	return r.Review(request.Query, request.Intent, request.Answer, request.Evidences)
}

func joinEvidence(evidences []Evidence) string {
	var builder strings.Builder
	for _, evidence := range evidences {
		fmt.Fprintf(&builder, "%s\n%s\n%s\n", evidence.Title, evidence.Content, evidence.SourceID)
	}
	return builder.String()
}

func evidenceKinds(evidences []Evidence) (hasTool bool, hasFailure bool) {
	for _, evidence := range evidences {
		switch evidence.Kind {
		case "tool_result":
			hasTool = true
		case "tool_error":
			hasFailure = true
		}
	}
	return
}

func requiresDynamicEvidence(intentType intent.Type) bool {
	switch intentType {
	case intent.PriceQuery, intent.InventoryQuery, intent.OrderQuery, intent.WarrantyQuery,
		intent.ReturnEligibility, intent.CreateAfterSalesTicket:
		return true
	default:
		return false
	}
}

func isHighRiskIntent(intentType intent.Type) bool {
	switch intentType {
	case intent.WarrantyQuery, intent.ReturnEligibility, intent.CreateAfterSalesTicket:
		return true
	default:
		return false
	}
}

func requestedFacets(query string) []string {
	facets := make([]string, 0, 4)
	for facet, keywords := range map[string][]string{
		"price":     {"多少钱", "价格", "售价"},
		"coupon":    {"券", "优惠"},
		"inventory": {"库存", "有货"},
		"warranty":  {"保修", "在保"},
		"return":    {"退货", "换货", "还能退"},
	} {
		for _, keyword := range keywords {
			if strings.Contains(query, keyword) {
				facets = append(facets, facet)
				break
			}
		}
	}
	return facets
}

func answerCoversFacet(answer, facet string) bool {
	keywords := map[string][]string{
		"price":     {"价格", "price", "current_price", "current_price_cents", "元"},
		"coupon":    {"优惠", "coupon", "券"},
		"inventory": {"库存", "stock", "有货"},
		"warranty":  {"保修", "warranty", "在保"},
		"return":    {"退", "换货", "政策"},
	}[facet]
	lower := strings.ToLower(answer)
	for _, keyword := range keywords {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
