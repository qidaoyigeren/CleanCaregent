package tool

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidResult = errors.New("tool returned invalid result")

// ValidateResult rejects semantically impossible or incomplete successful
// responses before they can be used as answer evidence.
func ValidateResult(name string, data any) error {
	name = LogicalName(name)
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("%w: encode result: %v", ErrInvalidResult, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("%w: decode result: %v", ErrInvalidResult, err)
	}

	var validationErr error
	switch name {
	case "price_query":
		validationErr = validatePriceResult(payload)
	case "inventory_check":
		validationErr = validateInventoryResult(payload)
	case "user_purchase_history":
		validationErr = validatePurchaseHistoryResult(payload)
	case "order_lookup":
		validationErr = validateOrderResult(payload)
	case "warranty_check":
		validationErr = validateWarrantyResult(payload)
	case "create_after_sales_ticket":
		validationErr = validateTicketResult(payload)
	}
	if validationErr != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResult, validationErr)
	}
	return nil
}

func validatePriceResult(payload map[string]any) error {
	items, err := requiredItems(payload)
	if err != nil {
		return err
	}
	for index, item := range items {
		if blank(item["sku_code"]) || blank(item["model"]) {
			return fmt.Errorf("price item %d is missing sku_code or model", index)
		}
		if number(item["list_price_cents"]) <= 0 ||
			number(item["current_price_cents"]) <= 0 ||
			number(item["estimated_final_price_cents"]) <= 0 {
			return fmt.Errorf("price item %d contains a non-positive price", index)
		}
		if blank(item["currency"]) {
			return fmt.Errorf("price item %d is missing currency", index)
		}
	}
	return nil
}

func validateInventoryResult(payload map[string]any) error {
	items, err := requiredItems(payload)
	if err != nil {
		return err
	}
	for index, item := range items {
		if blank(item["sku_code"]) {
			return fmt.Errorf("inventory item %d is missing sku_code", index)
		}
		if number(item["available_stock"]) < 0 {
			return fmt.Errorf("inventory item %d contains negative stock", index)
		}
		if raw := strings.TrimSpace(fmt.Sprint(item["updated_at"])); raw != "" {
			updatedAt, parseErr := time.Parse(time.RFC3339Nano, raw)
			if parseErr != nil {
				return fmt.Errorf("inventory item %d has invalid updated_at", index)
			}
			if updatedAt.After(time.Now().UTC().Add(5 * time.Minute)) {
				return fmt.Errorf("inventory item %d has future updated_at", index)
			}
		}
	}
	return nil
}

func validatePurchaseHistoryResult(payload map[string]any) error {
	items, ok := payload["items"].([]any)
	if !ok {
		return errors.New("purchase history result is missing items")
	}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			return fmt.Errorf("purchase history item %d is not an object", index)
		}
		if blank(item["order_no"]) || blank(item["model"]) || blank(item["sku_code"]) {
			return fmt.Errorf("purchase history item %d is missing order or product identity", index)
		}
		if number(item["quantity"]) <= 0 || number(item["unit_price_cents"]) <= 0 {
			return fmt.Errorf("purchase history item %d has invalid quantity or unit price", index)
		}
	}
	return nil
}

func validateOrderResult(payload map[string]any) error {
	if blank(payload["order_no"]) || blank(payload["status"]) {
		return errors.New("order result is missing order_no or status")
	}
	if number(payload["total_amount_cents"]) <= 0 {
		return errors.New("order result contains a non-positive total_amount_cents")
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) == 0 {
		return errors.New("order result contains no items")
	}
	return nil
}

func validateWarrantyResult(payload map[string]any) error {
	items, err := requiredItems(payload)
	if err != nil {
		return err
	}
	for index, item := range items {
		if blank(item["order_no"]) || blank(item["model"]) {
			return fmt.Errorf("warranty item %d is missing order_no or model", index)
		}
		if number(item["warranty_months"]) <= 0 {
			return fmt.Errorf("warranty item %d has invalid warranty_months", index)
		}
		startRaw := strings.TrimSpace(fmt.Sprint(item["warranty_start"]))
		endRaw := strings.TrimSpace(fmt.Sprint(item["warranty_end"]))
		if startRaw != "" && endRaw != "" {
			start, startErr := time.Parse(time.RFC3339Nano, startRaw)
			end, endErr := time.Parse(time.RFC3339Nano, endRaw)
			if startErr != nil || endErr != nil || !end.After(start) {
				return fmt.Errorf("warranty item %d has invalid warranty period", index)
			}
		}
	}
	return nil
}

func validateTicketResult(payload map[string]any) error {
	for _, field := range []string{"ticket_no", "order_no", "status", "idempotency_key"} {
		if blank(payload[field]) {
			return fmt.Errorf("ticket result is missing %s", field)
		}
	}
	return nil
}

func requiredItems(payload map[string]any) ([]map[string]any, error) {
	rawItems, ok := payload["items"].([]any)
	if !ok || len(rawItems) == 0 {
		return nil, errors.New("result contains no items")
	}
	items := make([]map[string]any, 0, len(rawItems))
	for index, rawItem := range rawItems {
		item, ok := rawItem.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("item %d is not an object", index)
		}
		items = append(items, item)
	}
	return items, nil
}

func blank(value any) bool {
	if value == nil {
		return true
	}
	raw := strings.TrimSpace(fmt.Sprint(value))
	return raw == "" || raw == "<nil>"
}

func number(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}
