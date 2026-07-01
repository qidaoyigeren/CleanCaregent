package policy

import (
	"fmt"
	"sort"
	"strings"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/tool"
)

type ToolPolicy struct {
	Intent               intent.Type
	Tools                []string
	Preconditions        []string
	SideEffects          map[string]string
	EvidenceRequirements []string
}

type ToolInvocationContext struct {
	UserID          string
	ConversationID  string
	TraceID         string
	ClientMessageID string
}

var toolPolicies = map[intent.Type]ToolPolicy{
	intent.PriceQuery: {
		Intent: intent.PriceQuery,
		Tools:  []string{"price_query"},
	},
	intent.InventoryQuery: {
		Intent: intent.InventoryQuery,
		Tools:  []string{"inventory_check"},
	},
	intent.OrderQuery: {
		Intent:        intent.OrderQuery,
		Tools:         []string{"user_purchase_history", "order_lookup"},
		Preconditions: []string{"authenticated_user"},
	},
	intent.PurchaseRecommendation: {
		Intent:               intent.PurchaseRecommendation,
		Tools:                []string{"price_query", "inventory_check"},
		EvidenceRequirements: []string{"product_candidates"},
	},
	intent.AccessoryCompatibility: {
		Intent:               intent.AccessoryCompatibility,
		Tools:                []string{"user_purchase_history", "price_query", "inventory_check"},
		EvidenceRequirements: []string{"host_model", "accessory_model", "compatibility_evidence"},
	},
	intent.WarrantyQuery: {
		Intent:               intent.WarrantyQuery,
		Tools:                []string{"user_purchase_history", "order_lookup", "warranty_check"},
		Preconditions:        []string{"authenticated_user", "order_no"},
		EvidenceRequirements: []string{"order_evidence", "warranty_policy"},
	},
	intent.ReturnEligibility: {
		Intent:        intent.ReturnEligibility,
		Tools:         []string{"user_purchase_history", "order_lookup", "warranty_check", "return_request", "exchange_request", "refund_status"},
		Preconditions: []string{"authenticated_user", "order_no", "explicit_confirmation_for_state_change"},
		SideEffects: map[string]string{
			"return_request":   "state_change",
			"exchange_request": "state_change",
		},
		EvidenceRequirements: []string{"order_evidence", "after_sales_policy"},
	},
	intent.AfterSalesStatus: {
		Intent:        intent.AfterSalesStatus,
		Tools:         []string{"user_purchase_history", "order_lookup", "refund_status", "repair_status"},
		Preconditions: []string{"authenticated_user", "order_no_or_ticket_no"},
	},
	intent.HumanHandoff: {
		Intent:        intent.HumanHandoff,
		Tools:         []string{"handoff_to_human"},
		Preconditions: []string{"explicit_confirmation_for_state_change"},
		SideEffects:   map[string]string{"handoff_to_human": "state_change"},
	},
	intent.Troubleshooting: {
		Intent:        intent.Troubleshooting,
		Tools:         []string{"user_purchase_history", "order_lookup", "warranty_check", "repair_status", "create_after_sales_ticket", "handoff_to_human"},
		Preconditions: []string{"explicit_confirmation_for_state_change"},
		SideEffects: map[string]string{
			"create_after_sales_ticket": "state_change",
			"handoff_to_human":          "state_change",
		},
		EvidenceRequirements: []string{"diagnosis_evidence", "order_evidence_for_state_change"},
	},
	intent.CreateAfterSalesTicket: {
		Intent:        intent.CreateAfterSalesTicket,
		Tools:         []string{"order_lookup", "warranty_check", "create_after_sales_ticket"},
		Preconditions: []string{"authenticated_user", "order_no", "issue_description", "client_message_id", "explicit_confirmation_for_state_change"},
		SideEffects:   map[string]string{"create_after_sales_ticket": "state_change"},
		EvidenceRequirements: []string{
			"order_evidence",
			"after_sales_policy",
		},
	},
}

func Rule(intentType intent.Type) (ToolPolicy, bool) {
	rule, ok := toolPolicies[intentType]
	if !ok {
		return ToolPolicy{}, false
	}
	return clonePolicy(rule), true
}

func AllowedTools(intentType intent.Type) []string {
	rule, ok := Rule(intentType)
	if !ok {
		return nil
	}
	return rule.Tools
}

func AllowedToolsForRoute(route intent.Result) []string {
	if route.Secondary == intent.OutOfScope ||
		route.Secondary == intent.Chitchat ||
		route.Secondary == intent.Clarification {
		return AllowedTools(route.Secondary)
	}
	result := AllowedTools(route.Secondary)
	for _, secondary := range route.SecondaryIntents {
		for _, toolName := range AllowedTools(secondary) {
			if !contains(result, toolName) {
				result = append(result, toolName)
			}
		}
	}
	return result
}

func AllAllowedTools() []string {
	seen := make(map[string]struct{})
	for _, rule := range toolPolicies {
		for _, toolName := range rule.Tools {
			toolName = strings.TrimSpace(toolName)
			if toolName == "" {
				continue
			}
			seen[toolName] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for toolName := range seen {
		result = append(result, toolName)
	}
	sort.Strings(result)
	return result
}

func ValidateToolExecution(
	route intent.Result,
	toolName string,
	arguments map[string]any,
	userID string,
	clientMessageID string,
) error {
	logicalToolName := tool.LogicalName(toolName)
	if !tool.NameAllowed(AllowedToolsForRoute(route), logicalToolName) {
		return fmt.Errorf("tool %s is not allowed for intent %s", logicalToolName, route.Secondary)
	}
	matched := false
	for _, rule := range policiesForRoute(route) {
		if !tool.NameAllowed(rule.Tools, logicalToolName) {
			continue
		}
		matched = true
		if err := validatePreconditions(rule, logicalToolName, arguments, userID, clientMessageID); err != nil {
			return err
		}
	}
	if !matched {
		return fmt.Errorf("tool %s has no execution policy for intent %s", logicalToolName, route.Secondary)
	}
	return nil
}

func ToolIdempotencyKey(toolName string, context ToolInvocationContext, arguments map[string]any) string {
	toolName = tool.LogicalName(toolName)
	userID := idempotencyPart(context.UserID)
	switch toolName {
	case "create_after_sales_ticket":
		return strings.Join([]string{
			"tool",
			toolName,
			userID,
			idempotencyPart(idempotencyArgumentString(arguments, "order_no")),
			idempotencyPart(idempotencyArgumentIntString(arguments, "order_item_id")),
			idempotencyPart(idempotencyArgumentString(arguments, "issue_type")),
		}, ":")
	case "return_request", "exchange_request":
		return strings.Join([]string{
			"tool",
			toolName,
			userID,
			idempotencyPart(idempotencyArgumentString(arguments, "order_no")),
			idempotencyPart(idempotencyArgumentIntString(arguments, "order_item_id")),
			idempotencyPart(idempotencyArgumentString(arguments, "reason")),
		}, ":")
	case "handoff_to_human":
		return strings.Join([]string{
			"tool",
			toolName,
			userID,
			idempotencyPart(context.ConversationID),
			idempotencyPart(firstNonEmptyString(context.ClientMessageID, context.TraceID)),
		}, ":")
	default:
		return strings.Join([]string{
			"tool",
			toolName,
			userID,
			idempotencyPart(context.ConversationID),
			idempotencyPart(firstNonEmptyString(context.ClientMessageID, context.TraceID)),
		}, ":")
	}
}

func All() []ToolPolicy {
	result := make([]ToolPolicy, 0, len(toolPolicies))
	for _, rule := range toolPolicies {
		result = append(result, clonePolicy(rule))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Intent < result[j].Intent
	})
	return result
}

func policiesForRoute(route intent.Result) []ToolPolicy {
	values := make([]ToolPolicy, 0, len(route.SecondaryIntents)+1)
	if rule, ok := Rule(route.Secondary); ok {
		values = append(values, rule)
	}
	for _, secondary := range route.SecondaryIntents {
		if secondary == route.Secondary {
			continue
		}
		if rule, ok := Rule(secondary); ok {
			values = append(values, rule)
		}
	}
	return values
}

func validatePreconditions(
	rule ToolPolicy,
	toolName string,
	arguments map[string]any,
	userID string,
	clientMessageID string,
) error {
	stateChange := rule.SideEffects[toolName] == "state_change"
	if stateChange {
		if !boolArgument(arguments, "confirmed") {
			return fmt.Errorf("tool %s requires explicit confirmation", toolName)
		}
		if strings.TrimSpace(clientMessageID) == "" {
			return fmt.Errorf("tool %s requires client_message_id for idempotency", toolName)
		}
	}
	for _, precondition := range rule.Preconditions {
		switch precondition {
		case "authenticated_user":
			if strings.TrimSpace(userID) == "" {
				return fmt.Errorf("tool %s requires authenticated user", toolName)
			}
		case "order_no":
			if toolRequiresOrderNo(toolName) && stringArgument(arguments, "order_no") == "" {
				return fmt.Errorf("tool %s requires order_no", toolName)
			}
		case "order_no_or_ticket_no":
			if toolRequiresOrderOrTicketNo(toolName) &&
				stringArgument(arguments, "order_no") == "" &&
				stringArgument(arguments, "ticket_no") == "" {
				return fmt.Errorf("tool %s requires order_no or ticket_no", toolName)
			}
		case "issue_description":
			if toolRequiresIssueDescription(toolName) && stringArgument(arguments, "description") == "" {
				return fmt.Errorf("tool %s requires issue description", toolName)
			}
		case "client_message_id":
			if strings.TrimSpace(clientMessageID) == "" {
				return fmt.Errorf("tool %s requires client_message_id", toolName)
			}
		case "explicit_confirmation_for_state_change":
			if stateChange && !boolArgument(arguments, "confirmed") {
				return fmt.Errorf("tool %s requires explicit confirmation", toolName)
			}
		}
	}
	return nil
}

func toolRequiresOrderNo(toolName string) bool {
	switch toolName {
	case "order_lookup", "warranty_check", "create_after_sales_ticket", "return_request", "exchange_request":
		return true
	default:
		return false
	}
}

func toolRequiresOrderOrTicketNo(toolName string) bool {
	switch toolName {
	case "order_lookup", "refund_status", "repair_status":
		return true
	default:
		return false
	}
}

func toolRequiresIssueDescription(toolName string) bool {
	return toolName == "create_after_sales_ticket"
}

func clonePolicy(rule ToolPolicy) ToolPolicy {
	rule.Tools = append([]string(nil), rule.Tools...)
	rule.Preconditions = append([]string(nil), rule.Preconditions...)
	rule.EvidenceRequirements = append([]string(nil), rule.EvidenceRequirements...)
	if rule.SideEffects != nil {
		copy := make(map[string]string, len(rule.SideEffects))
		for key, value := range rule.SideEffects {
			copy[key] = value
		}
		rule.SideEffects = copy
	}
	return rule
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func stringArgument(arguments map[string]any, key string) string {
	if arguments == nil {
		return ""
	}
	switch value := arguments[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func boolArgument(arguments map[string]any, key string) bool {
	if arguments == nil {
		return false
	}
	switch value := arguments[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

func idempotencyArgumentString(arguments map[string]any, key string) string {
	return strings.ToUpper(stringArgument(arguments, key))
}

func idempotencyArgumentIntString(arguments map[string]any, key string) string {
	value := idempotencyArgumentString(arguments, key)
	if value == "" || value == "0" {
		return "default"
	}
	return value
}

func idempotencyPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "none"
	}
	replacer := strings.NewReplacer(":", "_", "|", "_", " ", "_", "\t", "_", "\n", "_")
	return replacer.Replace(value)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
