package policy

import (
	"strings"
	"testing"

	"CleanCaregent/internal/intent"
)

func TestAllowedToolsForRouteMergesSecondaryIntents(t *testing.T) {
	tools := AllowedToolsForRoute(intent.Result{
		Secondary:        intent.PurchaseRecommendation,
		SecondaryIntents: []intent.Type{intent.PriceQuery, intent.InventoryQuery},
	})
	for _, expected := range []string{"price_query", "inventory_check"} {
		if !contains(tools, expected) {
			t.Fatalf("tools = %v, missing %s", tools, expected)
		}
	}
}

func TestStateChangingPoliciesDeclarePreconditions(t *testing.T) {
	for _, intentType := range []intent.Type{
		intent.CreateAfterSalesTicket,
		intent.ReturnEligibility,
		intent.HumanHandoff,
		intent.Troubleshooting,
	} {
		rule, ok := Rule(intentType)
		if !ok {
			t.Fatalf("missing policy for %s", intentType)
		}
		if len(rule.SideEffects) == 0 {
			t.Fatalf("policy %s missing side effects", intentType)
		}
		if !contains(rule.Preconditions, "explicit_confirmation_for_state_change") {
			t.Fatalf("policy %s preconditions = %v", intentType, rule.Preconditions)
		}
	}
}

func TestValidateToolExecutionBlocksUnconfirmedStateChange(t *testing.T) {
	err := ValidateToolExecution(
		intent.Result{Secondary: intent.CreateAfterSalesTicket},
		"create_after_sales_ticket",
		map[string]any{
			"order_no":    "CC100001",
			"description": "power failure",
		},
		"user-1",
		"cm-1",
	)
	if err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateToolExecutionBlocksMissingClientMessageIDForStateChange(t *testing.T) {
	err := ValidateToolExecution(
		intent.Result{Secondary: intent.CreateAfterSalesTicket},
		"create_after_sales_ticket",
		map[string]any{
			"order_no":    "CC100001",
			"description": "power failure",
			"confirmed":   true,
		},
		"user-1",
		"",
	)
	if err == nil || !strings.Contains(err.Error(), "client_message_id") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateToolExecutionAllowsReadOnlyUserHistoryWithoutOrderNo(t *testing.T) {
	err := ValidateToolExecution(
		intent.Result{Secondary: intent.ReturnEligibility},
		"user_purchase_history",
		map[string]any{"limit": 10},
		"user-1",
		"",
	)
	if err != nil {
		t.Fatalf("ValidateToolExecution() error = %v", err)
	}
}

func TestValidateToolExecutionBlocksToolOutsideRoute(t *testing.T) {
	err := ValidateToolExecution(
		intent.Result{Secondary: intent.PriceQuery},
		"create_after_sales_ticket",
		map[string]any{"confirmed": true},
		"user-1",
		"cm-1",
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAllAllowedToolsIncludesPolicyRegistry(t *testing.T) {
	tools := AllAllowedTools()
	if !contains(tools, "create_after_sales_ticket") || !sortStringsAreStable(tools) {
		t.Fatalf("tools = %v", tools)
	}
}

func sortStringsAreStable(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] > values[index] {
			return false
		}
	}
	return true
}
