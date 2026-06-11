package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/prompt"
)

// llmIntentResult mirrors the JSON structure the LLM returns for intent classification.
type llmIntentResult struct {
	Primary         string         `json:"primary"`
	Secondary       string         `json:"secondary"`
	Confidence      float64        `json:"confidence"`
	Entities        map[string]any `json:"entities"`
	NeedClarify     bool           `json:"need_clarify"`
	ClarifyQuestion string         `json:"clarify_question"`
	Reason          string         `json:"reason"`
}

// HybridRouter combines rule-based fast-path filtering with LLM deep classification.
// The rule layer catches clear-cut cases (chitchat, out-of-scope, exact keyword matches
// with high confidence); ambiguous or complex queries fall through to the LLM.
type HybridRouter struct {
	rule    *RuleRouter
	llm     *llm.Client
	prompts *prompt.Registry
	// MinRuleConfidence is the threshold above which rule results are trusted without LLM fallback.
	MinRuleConfidence float64
}

// NewHybridRouter creates a HybridRouter.
// If llmClient is nil, the router degrades to rule-only behavior.
func NewHybridRouter(llmClient *llm.Client, prompts *prompt.Registry) *HybridRouter {
	return &HybridRouter{
		rule:              NewRuleRouter(),
		llm:               llmClient,
		prompts:           prompts,
		MinRuleConfidence: 0.85,
	}
}

// Route classifies the query using rule-first, LLM-fallback strategy.
func (r *HybridRouter) Route(ctx context.Context, request RouteRequest) (Result, error) {
	// Step 1: Rule-based fast path.
	ruleResult, err := r.rule.Route(ctx, request)
	if err != nil {
		return Result{}, err
	}

	// For out-of-scope and chitchat, rules are very reliable — skip LLM.
	if ruleResult.Secondary == OutOfScope || ruleResult.Secondary == Chitchat {
		return ruleResult, nil
	}

	// If rule confidence is high enough and no clarification is needed, use rule result.
	if ruleResult.Confidence >= r.MinRuleConfidence && !ruleResult.NeedClarify {
		return ruleResult, nil
	}

	// Step 2: Fall through to LLM for deeper classification.
	if r.llm == nil || r.prompts == nil {
		// No LLM available — return rule result with clarification flag if low confidence.
		if ruleResult.Confidence < r.MinRuleConfidence {
			ruleResult.NeedClarify = true
		}
		return ruleResult, nil
	}

	llmResult, err := r.classifyWithLLM(ctx, request, ruleResult)
	if err != nil {
		// LLM call failed — degrade gracefully to rule result.
		if ruleResult.Confidence < r.MinRuleConfidence {
			ruleResult.NeedClarify = true
		}
		return ruleResult, nil
	}

	return llmResult, nil
}

func (r *HybridRouter) classifyWithLLM(ctx context.Context, request RouteRequest, ruleResult Result) (Result, error) {
	tmpl, err := r.prompts.Get(prompt.ScenarioIntent)
	if err != nil {
		return Result{}, fmt.Errorf("get intent prompt: %w", err)
	}

	recentJSON, _ := json.Marshal(request.RecentMessages)
	params := map[string]string{
		"summary":         request.Summary,
		"recent_messages": string(recentJSON),
		"query":           request.Query,
	}
	messages := tmpl.BuildMessages(params)

	var llmOut llmIntentResult
	if err := r.llm.ChatJSON(ctx, messages, &llmOut); err != nil {
		return Result{}, fmt.Errorf("llm intent classification: %w", err)
	}

	// Validate and normalize the LLM output.
	secondary := normalizeIntentType(llmOut.Secondary)
	if secondary == "" {
		secondary = Clarification
	}
	primary := llmOut.Primary
	if primary == "" {
		primary = "fallback"
	}

	entities := make(map[string]string)
	if raw, ok := llmOut.Entities["models"]; ok {
		entities["models"] = flattenStringSlice(raw)
	}
	if raw, ok := llmOut.Entities["categories"]; ok {
		entities["categories"] = flattenStringSlice(raw)
	}
	if raw, ok := llmOut.Entities["accessory_refs"]; ok {
		entities["accessory_refs"] = flattenStringSlice(raw)
	}
	if raw, ok := llmOut.Entities["order_numbers"]; ok {
		entities["order_no"] = flattenStringSlice(raw)
	}
	if raw, ok := llmOut.Entities["attributes"].(map[string]any); ok {
		for key, value := range raw {
			if normalized := scalarString(value); normalized != "" {
				entities[key] = normalized
			}
		}
	}

	// Merge rule entities as fallback (rule is better at regex-based model/order extraction).
	if entities["models"] == "" && ruleResult.Entities["models"] != "" {
		entities["models"] = ruleResult.Entities["models"]
	}
	if entities["order_no"] == "" && ruleResult.Entities["order_no"] != "" {
		entities["order_no"] = ruleResult.Entities["order_no"]
	}

	confidence := llmOut.Confidence
	if confidence <= 0 {
		confidence = ruleResult.Confidence
	}
	// Clamp.
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	needClarify := llmOut.NeedClarify || confidence < 0.65
	if enoughEntitiesForIntent(secondary, entities) {
		needClarify = false
	}
	return Result{
		Primary:     primary,
		Secondary:   secondary,
		Confidence:  confidence,
		Entities:    entities,
		NeedClarify: needClarify,
	}, nil
}

func normalizeIntentType(raw string) Type {
	raw = strings.TrimSpace(strings.ToLower(raw))
	known := map[string]Type{
		"product_parameter":         ProductParameter,
		"product_comparison":        ProductComparison,
		"purchase_recommendation":   PurchaseRecommendation,
		"accessory_compatibility":   AccessoryCompatibility,
		"usage_instruction":         UsageInstruction,
		"price_query":               PriceQuery,
		"inventory_query":           InventoryQuery,
		"order_query":               OrderQuery,
		"warranty_query":            WarrantyQuery,
		"return_eligibility":        ReturnEligibility,
		"troubleshooting":           Troubleshooting,
		"create_after_sales_ticket": CreateAfterSalesTicket,
		"clarification":             Clarification,
		"out_of_scope":              OutOfScope,
		"chitchat":                  Chitchat,
	}
	if t, ok := known[raw]; ok {
		return t
	}
	return ""
}

func flattenStringSlice(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	case []string:
		return strings.Join(v, ",")
	}
	return ""
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", typed), "0"), ".")
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func enoughEntitiesForIntent(intentType Type, entities map[string]string) bool {
	models := strings.Split(entities["models"], ",")
	switch intentType {
	case ProductParameter, UsageInstruction, PriceQuery, InventoryQuery, Troubleshooting:
		return strings.TrimSpace(entities["models"]) != ""
	case ProductComparison:
		count := 0
		for _, modelName := range models {
			if strings.TrimSpace(modelName) != "" {
				count++
			}
		}
		return count >= 2
	case AccessoryCompatibility:
		return strings.TrimSpace(entities["models"]) != "" &&
			strings.TrimSpace(entities["accessory_refs"]) != ""
	case WarrantyQuery, ReturnEligibility, CreateAfterSalesTicket:
		return strings.TrimSpace(entities["order_no"]) != ""
	default:
		return false
	}
}
