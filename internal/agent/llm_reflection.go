package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"CleanCaregent/internal/evidencefmt"
	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/prompt"
)

// llmReflectionResult mirrors the JSON structure returned by the LLM reflection check.
type llmReflectionResult struct {
	ClaimVerification []struct {
		Claim       string   `json:"claim"`
		Status      string   `json:"status"`
		EvidenceIDs []string `json:"evidence_ids"`
		Reason      string   `json:"reason"`
	} `json:"claim_verification"`
	RetrievalQuality struct {
		Score      string `json:"score"`
		NeedRerun  bool   `json:"need_rerun"`
		RerunQuery string `json:"rerun_query"`
	} `json:"retrieval_quality"`
	Completeness struct {
		SubQuestionStatus map[string]string `json:"sub_question_status"`
		AllCovered        bool              `json:"all_covered"`
		MissingTopics     []string          `json:"missing_topics"`
	} `json:"completeness"`
	FactualAccuracy struct {
		UnsupportedClaims  []string `json:"unsupported_claims"`
		UnitErrors         []string `json:"unit_errors"`
		AllNumericGrounded bool     `json:"all_numeric_grounded"`
	} `json:"factual_accuracy"`
	DataConflict struct {
		ConflictsFound    bool     `json:"conflicts_found"`
		ConflictsDetail   []string `json:"conflicts_detail"`
		ResolutionCorrect bool     `json:"resolution_correct"`
	} `json:"data_conflict"`
	ToolUtilization struct {
		ResultsUsed   []string `json:"results_used"`
		ResultsMissed []string `json:"results_missed"`
		ErrorsHandled bool     `json:"errors_handled"`
	} `json:"tool_utilization"`
	CitationIntegrity struct {
		AllKeyClaimsCited bool     `json:"all_key_claims_cited"`
		InvalidCitations  []string `json:"invalid_citations"`
		BareClaims        []string `json:"bare_claims"`
	} `json:"citation_integrity"`
	SafetyCompliance struct {
		Passed     bool     `json:"passed"`
		Violations []string `json:"violations"`
	} `json:"safety_compliance"`
	OverallVerdict string `json:"overall_verdict"`
	ActionIfFail   string `json:"action_if_fail"`
	VerdictReason  string `json:"verdict_reason"`
}

// LLMReflector performs LLM-based quality review of generated answers.
// It wraps a GroundingReflector for fast rule-based checks and adds
// semantic quality assessment via LLM.
type LLMReflector struct {
	llm     *llm.Client
	prompts *prompt.Registry
	ground  *GroundingReflector
}

// NewLLMReflector creates an LLM-backed reflector. If llmClient is nil,
// degrades to the rule-based GroundingReflector.
func NewLLMReflector(llmClient *llm.Client, prompts *prompt.Registry) *LLMReflector {
	return &LLMReflector{
		llm:     llmClient,
		prompts: prompts,
		ground:  NewGroundingReflector(),
	}
}

// Review performs a two-layer review: rule-based checks first, then LLM deep check
// for complex answers. The result merges both layers.
func (r *LLMReflector) Review(
	query string,
	intentType intent.Type,
	answer string,
	evidences []Evidence,
) ReflectionResult {
	return r.ReviewContext(context.Background(), ReflectionRequest{
		Query:     query,
		Intent:    intentType,
		Answer:    answer,
		Evidences: evidences,
	})
}

func (r *LLMReflector) ReviewContext(
	ctx context.Context,
	request ReflectionRequest,
) ReflectionResult {
	// Step 1: Always run rule-based grounding checks first.
	groundResult := r.ground.Review(
		request.Query,
		request.Intent,
		request.Answer,
		request.Evidences,
	)

	// If rule-based check found critical issues, return immediately.
	if groundResult.ShouldTransfer {
		return groundResult
	}
	if groundResult.Action == "remove_unsupported" {
		return groundResult
	}
	// A diagnosis Skill deliberately asks one discriminating question per turn.
	// Do not let the semantic reviewer misclassify that controlled intermediate
	// response as an incomplete final answer.
	if request.IntentionalClarification {
		return groundResult
	}
	if shouldUseGroundingOnly(request.Intent, groundResult) {
		return groundResult
	}

	// Step 2: For non-trivial answers, run LLM-based quality assessment.
	if r.llm == nil || r.prompts == nil || request.Answer == "" || len(request.Answer) < 20 {
		return groundResult
	}

	llmResult, err := r.reviewWithLLM(ctx, request)
	if err != nil {
		// LLM review failed — use rule-based result.
		return groundResult
	}

	// Merge LLM findings with rule-based result.
	return r.mergeResults(groundResult, llmResult, request.Intent)
}

func shouldUseGroundingOnly(intentType intent.Type, ground ReflectionResult) bool {
	if ground.LowConfidence || ground.ShouldTransfer || len(ground.UnsupportedClaims) > 0 {
		return false
	}
	switch intentType {
	case intent.ProductParameter, intent.AccessoryCompatibility,
		intent.ProductComparison, intent.PurchaseRecommendation,
		intent.UsageInstruction,
		intent.Troubleshooting,
		intent.PriceQuery, intent.InventoryQuery, intent.OrderQuery,
		intent.WarrantyQuery, intent.ReturnEligibility, intent.CreateAfterSalesTicket,
		intent.Chitchat, intent.OutOfScope, intent.Clarification:
		return true
	default:
		return false
	}
}

func (r *LLMReflector) reviewWithLLM(
	ctx context.Context,
	request ReflectionRequest,
) (llmReflectionResult, error) {
	tmpl, err := r.prompts.Get(prompt.ScenarioReflect)
	if err != nil {
		return llmReflectionResult{}, err
	}

	evidenceContext := buildEvidenceContextForReflectionWithFocus(
		request.Evidences,
		request.Query,
		request.Answer,
	)
	subQuestions := "[]"
	if raw, err := json.Marshal(request.SubQuestions); err == nil {
		subQuestions = string(raw)
	}

	params := map[string]string{
		"original_query":   request.Query,
		"sub_questions":    subQuestions,
		"draft_answer":     request.Answer,
		"evidence_context": evidenceContext,
		"tool_calls":       buildToolResultsSummary(request.Evidences),
	}
	messages := tmpl.BuildMessages(params)
	claims := extractClaims(request.Answer)
	claimsRaw, err := json.Marshal(claims)
	if err != nil {
		return llmReflectionResult{}, fmt.Errorf("编码待核验主张失败: %w", err)
	}
	if len(messages) > 0 {
		messages[len(messages)-1]["content"] +=
			"\n\n# 待逐条核验的 claims\n" + string(claimsRaw) +
				"\n请在 claim_verification 中逐条返回 claim、status(supported|unsupported)、evidence_ids 和 reason。"
	}

	var llmOut llmReflectionResult
	if err := r.llm.ChatJSON(ctx, messages, &llmOut); err != nil {
		return llmReflectionResult{}, fmt.Errorf("llm reflection: %w", err)
	}
	return llmOut, nil
}

func (r *LLMReflector) mergeResults(
	ground ReflectionResult,
	llm llmReflectionResult,
	intentType intent.Type,
) ReflectionResult {
	result := ground
	diagnosisAdvisory := intentType == intent.Troubleshooting

	// If LLM found the answer is a fail, escalate.
	if llm.OverallVerdict == "fail" {
		result.LowConfidence = true

		// Map LLM action to concrete behavior.
		switch llm.ActionIfFail {
		case "transfer_human":
			result.ShouldTransfer = true
			result.Action = "transfer_human"
		case "rerun_retrieval":
			result.Warnings = append(result.Warnings, "llm_rerun_retrieval")
			result.Action = "rerun_retrieval"
			result.RerunQuery = llm.RetrievalQuality.RerunQuery
		case "clarify":
			result.Warnings = append(result.Warnings, "llm_clarify_needed")
			result.Action = "clarify"
		case "regenerate":
			if diagnosisAdvisory {
				result.Warnings = append(result.Warnings, "llm_regenerate_advisory")
			} else {
				result.Warnings = append(result.Warnings, "llm_regenerate_needed")
				result.Action = "regenerate"
			}
		}
	}

	// Merge warnings.
	if llm.OverallVerdict == "degraded" {
		result.Warnings = append(result.Warnings, "llm_degraded:"+llm.VerdictReason)
	}

	// Merge unsupported claims.
	unsupportedBefore := len(result.UnsupportedClaims)
	for _, verification := range llm.ClaimVerification {
		if !strings.EqualFold(verification.Status, "unsupported") {
			continue
		}
		if diagnosisAdvisory {
			result.Warnings = append(result.Warnings, "llm_unsupported_advisory:"+verification.Claim)
			continue
		}
		if !containsStringInSlice(result.UnsupportedClaims, verification.Claim) {
			result.UnsupportedClaims = append(result.UnsupportedClaims, verification.Claim)
		}
	}
	for _, claim := range llm.FactualAccuracy.UnsupportedClaims {
		if diagnosisAdvisory {
			result.Warnings = append(result.Warnings, "llm_unsupported_advisory:"+claim)
			continue
		}
		if !containsStringInSlice(result.UnsupportedClaims, claim) {
			result.UnsupportedClaims = append(result.UnsupportedClaims, claim)
		}
	}
	if len(result.UnsupportedClaims) > unsupportedBefore {
		result.LowConfidence = true
		if result.Action == "" {
			result.Action = "regenerate"
		}
		result.Warnings = append(result.Warnings, "llm_unsupported_claims")
	}

	// Add safety violations.
	for _, violation := range llm.SafetyCompliance.Violations {
		result.Warnings = append(result.Warnings, "safety:"+violation)
		if !llm.SafetyCompliance.Passed {
			result.ShouldTransfer = true
		}
	}

	// Add citation issues.
	for _, bare := range llm.CitationIntegrity.BareClaims {
		result.Warnings = append(result.Warnings, "bare_claim:"+bare)
	}

	return result
}

func extractClaims(answer string) []string {
	answer = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(answer)
	parts := strings.FieldsFunc(answer, func(current rune) bool {
		switch current {
		case '。', '！', '？', '\n', ';', '；':
			return true
		default:
			return false
		}
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimLeft(part, "-*#0123456789. "))
		if len([]rune(part)) < 6 || strings.HasSuffix(part, "吗") || strings.HasSuffix(part, "？") {
			continue
		}
		if !containsStringInSlice(result, part) {
			result = append(result, part)
		}
		if len(result) == 20 {
			break
		}
	}
	return result
}

func buildEvidenceContextForReflection(evidences []Evidence) string {
	return buildEvidenceContextForReflectionWithFocus(evidences, "", "")
}

func buildEvidenceContextForReflectionWithFocus(
	evidences []Evidence,
	query string,
	answer string,
) string {
	var builder strings.Builder
	if len(evidences) > 8 {
		evidences = evidences[:8]
	}
	perItemLimit := reflectionEvidenceItemLimit(len(evidences))
	for _, item := range evidences {
		fmt.Fprintf(&builder, "[%s] 类型：%s\n标题：%s\n内容：%s\n\n",
			item.ID,
			item.Kind,
			item.Title,
			evidencefmt.Compact(item.Content, perItemLimit, query, answer, item.Title),
		)
	}
	if builder.Len() == 0 {
		return "(无证据)"
	}
	return builder.String()
}

func truncateReflectionEvidence(value string, maxRunes int) string {
	return evidencefmt.Compact(value, maxRunes)
}

func reflectionEvidenceItemLimit(count int) int {
	if count <= 0 {
		return 1200
	}
	limit := 6400 / count
	if limit < 600 {
		return 600
	}
	if limit > 1200 {
		return 1200
	}
	return limit
}

func containsStringInSlice(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// Ensure LLMReflector implements the interface.
var _ Reflector = (*LLMReflector)(nil)
var _ ContextualReflector = (*LLMReflector)(nil)
