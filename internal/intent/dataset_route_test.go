package intent

import (
	"context"
	"encoding/json"
	"testing"

	evaldata "CleanCaregent/docs/eval"
)

func TestRuleRouterCoversCanonicalEvaluationIntents(t *testing.T) {
	var cases []struct {
		CaseID string `json:"case_id"`
		Query  string `json:"query"`
		Intent Type   `json:"intent"`
	}
	if err := json.Unmarshal(evaldata.CasesV2(), &cases); err != nil {
		t.Fatal(err)
	}

	router := NewRuleRouter()
	matched := 0
	var mismatches []string
	for _, evalCase := range cases {
		result, err := router.Route(context.Background(), RouteRequest{Query: evalCase.Query})
		if err != nil {
			t.Fatal(err)
		}
		if result.Secondary == evalCase.Intent {
			matched++
			continue
		}
		mismatches = append(mismatches, evalCase.CaseID+":"+string(result.Secondary)+"!="+string(evalCase.Intent))
	}
	accuracy := float64(matched) / float64(len(cases))
	if accuracy < 0.80 {
		t.Fatalf("rule intent coverage = %.2f, mismatches = %v", accuracy, mismatches)
	}
}

func TestReconcileRuleAndLLMKeepsExplicitBusinessIntent(t *testing.T) {
	result := reconcileRuleAndLLM(
		Result{
			Primary:     PrimaryPresales,
			Secondary:   PriceQuery,
			Confidence:  0.62,
			NeedClarify: true,
			Entities:    map[string]string{},
			RouteTrace: RouteTrace{
				Source:          "rule",
				MatchedKeywords: []string{"多少钱"},
				Reasoning:       "明确价格查询",
			},
		},
		Result{
			Primary:     PrimaryFallback,
			Secondary:   Clarification,
			Confidence:  0.91,
			NeedClarify: true,
			Entities:    map[string]string{},
			RouteTrace:  RouteTrace{Source: "llm", Reasoning: "缺少型号"},
		},
	)
	if result.Secondary != PriceQuery || !result.NeedClarify {
		t.Fatalf("result = %#v", result)
	}
}

func TestReconcileKeepsMandatoryTicketClarification(t *testing.T) {
	result := reconcileRuleAndLLM(
		Result{
			Primary:     PrimaryAftersales,
			Secondary:   CreateAfterSalesTicket,
			Confidence:  0.9,
			NeedClarify: true,
			Entities:    map[string]string{},
			RouteTrace: RouteTrace{
				Source:          "rule",
				MatchedKeywords: []string{"建售后单"},
			},
		},
		Result{
			Primary:     PrimaryAftersales,
			Secondary:   CreateAfterSalesTicket,
			Confidence:  0.85,
			NeedClarify: false,
			Entities:    map[string]string{},
		},
	)
	if !result.NeedClarify {
		t.Fatalf("result = %#v", result)
	}
}
