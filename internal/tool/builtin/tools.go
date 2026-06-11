package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/tool"
)

var ErrInvalidArguments = errors.New("invalid tool arguments")

type PriceQuery struct{ repository repository.BusinessRepository }
type InventoryCheck struct{ repository repository.BusinessRepository }
type UserPurchaseHistory struct{ repository repository.BusinessRepository }
type OrderLookup struct{ repository repository.BusinessRepository }
type WarrantyCheck struct{ repository repository.BusinessRepository }
type CreateAfterSalesTicket struct{ repository repository.BusinessRepository }

func NewPriceQuery(repository repository.BusinessRepository) *PriceQuery {
	return &PriceQuery{repository: repository}
}
func NewInventoryCheck(repository repository.BusinessRepository) *InventoryCheck {
	return &InventoryCheck{repository: repository}
}
func NewUserPurchaseHistory(repository repository.BusinessRepository) *UserPurchaseHistory {
	return &UserPurchaseHistory{repository: repository}
}
func NewOrderLookup(repository repository.BusinessRepository) *OrderLookup {
	return &OrderLookup{repository: repository}
}
func NewWarrantyCheck(repository repository.BusinessRepository) *WarrantyCheck {
	return &WarrantyCheck{repository: repository}
}
func NewCreateAfterSalesTicket(repository repository.BusinessRepository) *CreateAfterSalesTicket {
	return &CreateAfterSalesTicket{repository: repository}
}

func (t *PriceQuery) Name() string { return "price_query" }
func (t *PriceQuery) Description() string {
	return "查询商品 SKU 的 mock 实时价格及当前用户可用优惠券"
}
func (t *PriceQuery) ParamsSchema() json.RawMessage {
	return schema(`{"type":"object","required":["product_refs"],"properties":{"product_refs":{"type":"array","items":{"type":"string"}}}}`)
}
func (t *PriceQuery) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	refs := stringSlice(call.Arguments["product_refs"])
	if len(refs) == 0 {
		return invalid(call, "product_refs is required")
	}
	items, err := t.repository.QueryPrices(ctx, call.UserID, refs)
	return result(call, map[string]any{"items": items, "as_of": time.Now().UTC()}), err
}

func (t *InventoryCheck) Name() string { return "inventory_check" }
func (t *InventoryCheck) Description() string {
	return "查询商品 SKU 的 mock 可售库存和库存更新时间"
}
func (t *InventoryCheck) ParamsSchema() json.RawMessage {
	return schema(`{"type":"object","required":["product_refs"],"properties":{"product_refs":{"type":"array","items":{"type":"string"}}}}`)
}
func (t *InventoryCheck) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	refs := stringSlice(call.Arguments["product_refs"])
	if len(refs) == 0 {
		return invalid(call, "product_refs is required")
	}
	items, err := t.repository.CheckInventory(ctx, refs)
	return result(call, map[string]any{"items": items}), err
}

func (t *UserPurchaseHistory) Name() string { return "user_purchase_history" }
func (t *UserPurchaseHistory) Description() string {
	return "按用户、品类、型号和时间范围查询历史购买记录"
}
func (t *UserPurchaseHistory) ParamsSchema() json.RawMessage {
	return schema(`{"type":"object","properties":{"category":{"type":"string"},"model":{"type":"string"},"since":{"type":"string","format":"date-time"},"until":{"type":"string","format":"date-time"},"limit":{"type":"integer","minimum":1,"maximum":50}}}`)
}
func (t *UserPurchaseHistory) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	filter := repository.PurchaseHistoryFilter{
		Category: stringValue(call.Arguments["category"]),
		Model:    stringValue(call.Arguments["model"]),
		Limit:    intValue(call.Arguments["limit"], 10),
	}
	if value, err := optionalTime(call.Arguments["since"]); err != nil {
		return invalid(call, "since must be RFC3339")
	} else {
		filter.Since = value
	}
	if value, err := optionalTime(call.Arguments["until"]); err != nil {
		return invalid(call, "until must be RFC3339")
	} else {
		filter.Until = value
	}
	items, err := t.repository.ListPurchaseHistory(ctx, call.UserID, filter)
	return result(call, map[string]any{"items": items}), err
}

func (t *OrderLookup) Name() string { return "order_lookup" }
func (t *OrderLookup) Description() string {
	return "根据当前用户和订单号查询订单状态、购买商品及时间"
}
func (t *OrderLookup) ParamsSchema() json.RawMessage {
	return schema(`{"type":"object","required":["order_no"],"properties":{"order_no":{"type":"string"}}}`)
}
func (t *OrderLookup) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	orderNo := strings.ToUpper(stringValue(call.Arguments["order_no"]))
	if orderNo == "" {
		return invalid(call, "order_no is required")
	}
	order, err := t.repository.GetOrder(ctx, call.UserID, orderNo)
	return result(call, order), err
}

func (t *WarrantyCheck) Name() string { return "warranty_check" }
func (t *WarrantyCheck) Description() string {
	return "根据订单签收或支付时间及订单项保修月数判断是否在保"
}
func (t *WarrantyCheck) ParamsSchema() json.RawMessage {
	return schema(`{"type":"object","required":["order_no"],"properties":{"order_no":{"type":"string"},"model":{"type":"string"},"at":{"type":"string","format":"date-time"}}}`)
}
func (t *WarrantyCheck) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	orderNo := strings.ToUpper(stringValue(call.Arguments["order_no"]))
	if orderNo == "" {
		return invalid(call, "order_no is required")
	}
	at := time.Now().UTC()
	if value, err := optionalTime(call.Arguments["at"]); err != nil {
		return invalid(call, "at must be RFC3339")
	} else if value != nil {
		at = *value
	}
	items, err := t.repository.CheckWarranty(ctx, call.UserID, orderNo, stringValue(call.Arguments["model"]), at)
	return result(call, map[string]any{"items": items, "checked_at": at}), err
}

func (t *CreateAfterSalesTicket) Name() string { return "create_after_sales_ticket" }
func (t *CreateAfterSalesTicket) Description() string {
	return "在用户明确确认后，为指定订单创建幂等的售后工单"
}
func (t *CreateAfterSalesTicket) ParamsSchema() json.RawMessage {
	return schema(`{"type":"object","required":["order_no","issue_type","description","confirmed"],"properties":{"order_no":{"type":"string"},"order_item_id":{"type":"integer"},"issue_type":{"type":"string"},"description":{"type":"string"},"diagnosis_summary":{"type":"string"},"evidence_ids":{"type":"array","items":{"type":"string"}},"confirmed":{"type":"boolean"}}}`)
}
func (t *CreateAfterSalesTicket) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if !boolValue(call.Arguments["confirmed"]) {
		return invalid(call, "explicit user confirmation is required")
	}
	orderNo := strings.ToUpper(stringValue(call.Arguments["order_no"]))
	description := strings.TrimSpace(stringValue(call.Arguments["description"]))
	if orderNo == "" || description == "" {
		return invalid(call, "order_no and description are required")
	}
	key := call.IdempotencyKey
	if key == "" {
		key = call.TraceID + ":" + orderNo
	}
	ticket, err := t.repository.CreateAfterSalesTicket(ctx, repository.CreateTicketRequest{
		UserID:           call.UserID,
		OrderNo:          orderNo,
		OrderItemID:      int64(intValue(call.Arguments["order_item_id"], 0)),
		IssueType:        defaultString(stringValue(call.Arguments["issue_type"]), "repair"),
		Description:      description,
		DiagnosisSummary: stringValue(call.Arguments["diagnosis_summary"]),
		EvidenceIDs:      stringSlice(call.Arguments["evidence_ids"]),
		IdempotencyKey:   key,
	})
	return result(call, ticket), err
}

func result(call tool.Call, data any) tool.Result {
	return tool.Result{CallID: call.CallID, Data: data}
}

func invalid(call tool.Call, message string) (tool.Result, error) {
	return tool.Result{CallID: call.CallID, ErrorCode: "INVALID_TOOL_ARGUMENTS", Message: message},
		fmt.Errorf("%w: %s", ErrInvalidArguments, message)
}

func schema(value string) json.RawMessage { return json.RawMessage(value) }

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return nonEmpty(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, stringValue(item))
		}
		return nonEmpty(values)
	case string:
		return nonEmpty(strings.Split(typed, ","))
	default:
		return nil
	}
}

func intValue(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := strconv.Atoi(typed.String())
		return parsed
	case string:
		parsed, err := strconv.Atoi(typed)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, _ := strconv.ParseBool(typed)
		return parsed
	default:
		return false
	}
}

func optionalTime(value any) (*time.Time, error) {
	raw := stringValue(value)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
