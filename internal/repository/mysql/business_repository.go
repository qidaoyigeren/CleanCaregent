package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"CleanCaregent/internal/model"
	"CleanCaregent/internal/platform/id"
	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/tool"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

type BusinessRepository struct {
	db *sql.DB
}

func NewBusinessRepository(db *sql.DB) *BusinessRepository {
	return &BusinessRepository{db: db}
}

func (r *BusinessRepository) ListProducts(ctx context.Context, category string, limit int) ([]model.Product, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := `
		SELECT product_code, name, category, brand, model, attributes_json
		FROM products
		WHERE status = 'active'`
	args := make([]any, 0, 2)
	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	query += " ORDER BY id LIMIT ?"
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	products := make([]model.Product, 0)
	for rows.Next() {
		product, scanErr := scanProduct(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}
	return products, nil
}

func (r *BusinessRepository) GetProduct(ctx context.Context, productCode string) (model.Product, error) {
	product, err := scanProduct(r.db.QueryRowContext(ctx, `
		SELECT product_code, name, category, brand, model, attributes_json
		FROM products
		WHERE product_code = ? AND status = 'active'
	`, productCode))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Product{}, repository.ErrProductNotFound
	}
	if err != nil {
		return model.Product{}, err
	}
	skus, err := r.querySKUs(ctx, []string{productCode})
	if err != nil {
		return model.Product{}, err
	}
	product.SKUs = skus
	return product, nil
}

func (r *BusinessRepository) FindProductByModel(
	ctx context.Context,
	modelName string,
) (model.Product, error) {
	product, err := scanProduct(r.db.QueryRowContext(ctx, `
		SELECT product_code, name, category, brand, model, attributes_json
		FROM products
		WHERE model = ? AND status = 'active'
		LIMIT 1
	`, strings.TrimSpace(modelName)))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Product{}, repository.ErrProductNotFound
	}
	if err != nil {
		return model.Product{}, fmt.Errorf("find product by model: %w", err)
	}
	return product, nil
}

func (r *BusinessRepository) QueryPrices(ctx context.Context, userID string, productRefs []string) ([]model.PriceQuote, error) {
	skus, err := r.querySKUs(ctx, productRefs)
	if err != nil {
		return nil, err
	}
	if len(skus) == 0 {
		return nil, repository.ErrProductNotFound
	}
	coupons, err := r.availableCoupons(ctx, userID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	quotes := make([]model.PriceQuote, 0, len(skus))
	for _, sku := range skus {
		finalPriceCents := sku.CurrentPriceCents
		for _, coupon := range coupons {
			switch coupon.DiscountType {
			case "amount":
				finalPriceCents -= coupon.DiscountAmountCents
			case "percent":
				finalPriceCents = (finalPriceCents*coupon.DiscountBasisPoints + 5000) / 10000
			}
		}
		if finalPriceCents < 0 {
			finalPriceCents = 0
		}
		quotes = append(quotes, model.PriceQuote{
			ProductSKU:               sku,
			EstimatedFinalPriceCents: finalPriceCents,
			AvailableCoupons:         coupons,
		})
	}
	return quotes, nil
}

func (r *BusinessRepository) CheckInventory(ctx context.Context, productRefs []string) ([]model.ProductSKU, error) {
	skus, err := r.querySKUs(ctx, productRefs)
	if err != nil {
		return nil, err
	}
	if len(skus) == 0 {
		return nil, repository.ErrProductNotFound
	}
	return skus, nil
}

func (r *BusinessRepository) ListPurchaseHistory(
	ctx context.Context,
	userID string,
	filter repository.PurchaseHistoryFilter,
) ([]model.PurchaseRecord, error) {
	if filter.Limit <= 0 || filter.Limit > 50 {
		filter.Limit = 10
	}
	conditions := []string{"u.user_no = ?"}
	args := []any{userID}
	if filter.Category != "" {
		conditions = append(conditions, "p.category = ?")
		args = append(args, filter.Category)
	}
	if filter.Model != "" {
		conditions = append(conditions, "p.model = ?")
		args = append(args, filter.Model)
	}
	if filter.Since != nil {
		conditions = append(conditions, "o.created_at >= ?")
		args = append(args, *filter.Since)
	}
	if filter.Until != nil {
		conditions = append(conditions, "o.created_at < ?")
		args = append(args, *filter.Until)
	}
	args = append(args, filter.Limit)

	rows, err := r.db.QueryContext(ctx, `
		SELECT o.order_no, o.status, p.product_code, p.name, p.model, s.sku_code,
		       oi.quantity, oi.unit_price, o.paid_at, o.delivered_at, oi.id, oi.warranty_months
		FROM orders o
		JOIN users u ON u.id = o.user_id
		JOIN order_items oi ON oi.order_id = o.id
		JOIN products p ON p.id = oi.product_id
		JOIN product_skus s ON s.id = oi.sku_id
		WHERE `+strings.Join(conditions, " AND ")+`
		ORDER BY o.created_at DESC, oi.id
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list purchase history: %w", err)
	}
	defer rows.Close()

	records := make([]model.PurchaseRecord, 0)
	for rows.Next() {
		record, scanErr := scanPurchaseRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate purchase history: %w", err)
	}
	return records, nil
}

func (r *BusinessRepository) GetOrder(ctx context.Context, userID, orderNo string) (model.OrderDetail, error) {
	var order model.OrderDetail
	var paidAt, deliveredAt sql.NullTime
	var totalAmountRaw string
	err := r.db.QueryRowContext(ctx, `
		SELECT o.order_no, u.user_no, o.status, o.total_amount, o.paid_at, o.delivered_at, o.created_at
		FROM orders o
		JOIN users u ON u.id = o.user_id
		WHERE u.user_no = ? AND o.order_no = ?
	`, userID, orderNo).Scan(
		&order.OrderNo, &order.UserID, &order.Status, &totalAmountRaw,
		&paidAt, &deliveredAt, &order.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.OrderDetail{}, repository.ErrOrderNotFound
	}
	if err != nil {
		return model.OrderDetail{}, fmt.Errorf("get order: %w", err)
	}
	order.TotalAmountCents, err = parseDecimalCents(totalAmountRaw)
	if err != nil {
		return model.OrderDetail{}, fmt.Errorf("parse order total amount: %w", err)
	}
	if paidAt.Valid {
		order.PaidAt = &paidAt.Time
	}
	if deliveredAt.Valid {
		order.DeliveredAt = &deliveredAt.Time
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT o.order_no, o.status, p.product_code, p.name, p.model, s.sku_code,
		       oi.quantity, oi.unit_price, o.paid_at, o.delivered_at, oi.id, oi.warranty_months
		FROM orders o
		JOIN users u ON u.id = o.user_id
		JOIN order_items oi ON oi.order_id = o.id
		JOIN products p ON p.id = oi.product_id
		JOIN product_skus s ON s.id = oi.sku_id
		WHERE u.user_no = ? AND o.order_no = ?
		ORDER BY oi.id
	`, userID, orderNo)
	if err != nil {
		return model.OrderDetail{}, fmt.Errorf("list order items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		item, scanErr := scanPurchaseRecord(rows)
		if scanErr != nil {
			return model.OrderDetail{}, scanErr
		}
		order.Items = append(order.Items, item)
	}
	if err := rows.Err(); err != nil {
		return model.OrderDetail{}, fmt.Errorf("iterate order items: %w", err)
	}
	return order, nil
}

func (r *BusinessRepository) CheckWarranty(
	ctx context.Context,
	userID, orderNo, modelName string,
	at time.Time,
) ([]model.WarrantyStatus, error) {
	order, err := r.GetOrder(ctx, userID, orderNo)
	if err != nil {
		return nil, err
	}
	results := make([]model.WarrantyStatus, 0, len(order.Items))
	for _, item := range order.Items {
		if modelName != "" && !strings.EqualFold(item.Model, modelName) {
			continue
		}
		status := model.WarrantyStatus{
			OrderNo:        order.OrderNo,
			ProductName:    item.ProductName,
			Model:          item.Model,
			WarrantyMonths: item.WarrantyMonths,
			OrderItemID:    item.OrderItemID,
		}
		start := order.DeliveredAt
		if start == nil {
			start = order.PaidAt
		}
		if start == nil {
			status.Reason = "订单尚无支付或签收时间，无法计算保修期"
		} else {
			end := start.AddDate(0, item.WarrantyMonths, 0)
			status.WarrantyStart = start
			status.WarrantyEnd = &end
			status.InWarranty = !at.Before(*start) && at.Before(end)
			if status.InWarranty {
				status.Reason = "当前时间在保修期限内"
			} else {
				status.Reason = "当前时间已超过保修期限"
			}
		}
		results = append(results, status)
	}
	if len(results) == 0 {
		return nil, repository.ErrOrderItemNotFound
	}
	return results, nil
}

func (r *BusinessRepository) CreateAfterSalesTicket(
	ctx context.Context,
	request repository.CreateTicketRequest,
) (model.AfterSalesTicket, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.AfterSalesTicket{}, fmt.Errorf("begin create after-sales ticket: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var userPK, orderPK int64
	if err := tx.QueryRowContext(ctx, `
		SELECT u.id, o.id
		FROM orders o
		JOIN users u ON u.id = o.user_id
		WHERE u.user_no = ? AND o.order_no = ?
	`, request.UserID, request.OrderNo).Scan(&userPK, &orderPK); errors.Is(err, sql.ErrNoRows) {
		return model.AfterSalesTicket{}, repository.ErrOrderNotFound
	} else if err != nil {
		return model.AfterSalesTicket{}, fmt.Errorf("resolve ticket order: %w", err)
	}
	if request.OrderItemID == 0 {
		if err := tx.QueryRowContext(ctx,
			"SELECT id FROM order_items WHERE order_id = ? ORDER BY id LIMIT 1",
			orderPK,
		).Scan(&request.OrderItemID); err != nil {
			return model.AfterSalesTicket{}, repository.ErrOrderItemNotFound
		}
	}
	var itemExists int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM order_items WHERE id = ? AND order_id = ?",
		request.OrderItemID, orderPK,
	).Scan(&itemExists); err != nil || itemExists == 0 {
		return model.AfterSalesTicket{}, repository.ErrOrderItemNotFound
	}

	evidenceRaw, _ := json.Marshal(request.EvidenceIDs)
	ticket := model.AfterSalesTicket{
		TicketNo:         id.New("AS"),
		UserID:           request.UserID,
		OrderNo:          request.OrderNo,
		OrderItemID:      request.OrderItemID,
		IssueType:        request.IssueType,
		Description:      request.Description,
		DiagnosisSummary: request.DiagnosisSummary,
		EvidenceIDs:      request.EvidenceIDs,
		Status:           initialAfterSalesStatus(request.IssueType),
		IdempotencyKey:   request.IdempotencyKey,
		CreatedAt:        time.Now().UTC(),
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO after_sales_tickets (
			ticket_no, user_id, order_id, order_item_id, issue_type, description,
			diagnosis_summary, evidence_ids_json, status, idempotency_key, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ticket.TicketNo, userPK, orderPK, ticket.OrderItemID, ticket.IssueType, ticket.Description,
		nullableString(ticket.DiagnosisSummary), evidenceRaw, ticket.Status, ticket.IdempotencyKey,
		ticket.CreatedAt, ticket.CreatedAt)
	if err != nil {
		var mysqlErr *mysqlDriver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return model.AfterSalesTicket{}, repository.ErrTicketConflict
		}
		return model.AfterSalesTicket{}, fmt.Errorf("insert after-sales ticket: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return model.AfterSalesTicket{}, fmt.Errorf("commit after-sales ticket: %w", err)
	}
	return ticket, nil
}

func (r *BusinessRepository) RequestAfterSalesAction(
	ctx context.Context,
	request repository.AfterSalesActionRequest,
) (model.AfterSalesActionResult, error) {
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	request.IssueType = strings.ToLower(strings.TrimSpace(request.IssueType))
	if request.IssueType == "" {
		request.IssueType = request.Action
	}
	if request.Description == "" {
		request.Description = request.Reason
	}
	if request.Description == "" {
		request.Description = "after-sales action requested"
	}
	ticket, err := r.CreateAfterSalesTicket(ctx, repository.CreateTicketRequest{
		UserID:           request.UserID,
		OrderNo:          request.OrderNo,
		OrderItemID:      request.OrderItemID,
		IssueType:        request.IssueType,
		Description:      request.Description,
		DiagnosisSummary: request.DiagnosisSummary,
		EvidenceIDs:      request.EvidenceIDs,
		IdempotencyKey:   request.IdempotencyKey,
	})
	if err != nil {
		return model.AfterSalesActionResult{}, err
	}
	slaHours := afterSalesSLAHours(request.IssueType)
	result := model.AfterSalesActionResult{
		Action:        request.Action,
		Ticket:        ticket,
		QueuePosition: estimatedQueuePosition(request.UserID, request.OrderNo, request.IssueType),
		SLAHours:      slaHours,
		NextAction:    afterSalesNextAction(request.IssueType),
		Audit: map[string]string{
			"scope":       "current_user_order",
			"side_effect": "state_change",
			"reason":      request.IssueType,
		},
	}
	return result, nil
}

func (r *BusinessRepository) GetAfterSalesProgress(
	ctx context.Context,
	filter repository.AfterSalesProgressFilter,
) ([]model.AfterSalesProgress, error) {
	filter.UserID = strings.TrimSpace(filter.UserID)
	filter.OrderNo = strings.ToUpper(strings.TrimSpace(filter.OrderNo))
	filter.TicketNo = strings.ToUpper(strings.TrimSpace(filter.TicketNo))
	filter.IssueType = strings.ToLower(strings.TrimSpace(filter.IssueType))
	if filter.Limit <= 0 || filter.Limit > 20 {
		filter.Limit = 10
	}
	conditions := []string{"u.user_no = ?"}
	args := []any{filter.UserID}
	if filter.OrderNo != "" {
		conditions = append(conditions, "o.order_no = ?")
		args = append(args, filter.OrderNo)
	}
	if filter.TicketNo != "" {
		conditions = append(conditions, "ast.ticket_no = ?")
		args = append(args, filter.TicketNo)
	}
	if filter.IssueType != "" {
		conditions = append(conditions, "ast.issue_type = ?")
		args = append(args, filter.IssueType)
	}
	args = append(args, filter.Limit)
	rows, err := r.db.QueryContext(ctx, `
		SELECT ast.ticket_no, o.order_no, ast.issue_type, ast.status, ast.created_at, ast.updated_at,
		       o.total_amount
		FROM after_sales_tickets ast
		JOIN users u ON u.id = ast.user_id
		JOIN orders o ON o.id = ast.order_id
		WHERE `+strings.Join(conditions, " AND ")+`
		ORDER BY ast.updated_at DESC, ast.id DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("query after-sales progress: %w", err)
	}
	defer rows.Close()
	items := make([]model.AfterSalesProgress, 0)
	for rows.Next() {
		var item model.AfterSalesProgress
		var totalAmountRaw string
		if err := rows.Scan(
			&item.TicketNo,
			&item.OrderNo,
			&item.IssueType,
			&item.Status,
			&item.CreatedAt,
			&item.UpdatedAt,
			&totalAmountRaw,
		); err != nil {
			return nil, fmt.Errorf("scan after-sales progress: %w", err)
		}
		item.Stage = afterSalesStage(item.IssueType, item.Status)
		item.NextAction = afterSalesNextAction(item.IssueType)
		if refundLikeIssue(item.IssueType) {
			item.RefundAmountCents, _ = parseDecimalCents(totalAmountRaw)
		}
		estimated := item.UpdatedAt.Add(time.Duration(afterSalesSLAHours(item.IssueType)) * time.Hour)
		item.EstimatedCompletionAt = &estimated
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate after-sales progress: %w", err)
	}
	if len(items) == 0 && filter.OrderNo != "" {
		order, err := r.GetOrder(ctx, filter.UserID, filter.OrderNo)
		if err != nil {
			return nil, err
		}
		issueType := filter.IssueType
		if issueType == "" {
			issueType = "repair"
		}
		items = append(items, model.AfterSalesProgress{
			OrderNo:    order.OrderNo,
			IssueType:  issueType,
			Status:     "not_created",
			Stage:      "no_after_sales_record",
			NextAction: "Ask the user whether to create an after-sales ticket after policy and order verification.",
			CreatedAt:  order.CreatedAt,
			UpdatedAt:  order.CreatedAt,
		})
	}
	return items, nil
}

func (r *BusinessRepository) SaveToolCall(ctx context.Context, call tool.Call, result tool.Result) error {
	argsRaw, _ := json.Marshal(maskToolArguments(call.Arguments))
	resultRaw, _ := json.Marshal(map[string]any{
		"data_scope": result.DataScope,
		"data":       redactToolData(result.Data),
		"audit": map[string]any{
			"tool_name":   call.Name,
			"sensitive":   sensitiveTool(call.Name),
			"side_effect": sensitiveToolSideEffect(call.Name),
			"scope":       "current_user",
		},
	})
	status := "success"
	if !result.Success {
		status = "failed"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tool_call_logs (
			trace_id, call_id, tool_name, args_masked_json, result_summary_json,
			status, error_code, latency_ms, idempotency_key, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(6))
	`, call.TraceID, call.CallID, call.Name, argsRaw, resultRaw, status,
		nullableString(result.ErrorCode), result.FinishedAt.Sub(result.StartedAt).Milliseconds(),
		nullableString(call.IdempotencyKey))
	if err != nil {
		return fmt.Errorf("save tool call log: %w", err)
	}
	return nil
}

func redactToolData(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return value
	}
	return redactAny(decoded)
}

func redactAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		masked := make(map[string]any, len(typed))
		for key, item := range typed {
			lower := strings.ToLower(key)
			switch lower {
			case "user_id", "user_no", "phone", "mobile", "contact", "address", "description", "diagnosis_summary", "idempotency_key":
				masked[key] = "[REDACTED]"
			case "order_no", "ticket_no":
				masked[key] = maskIdentifier(fmt.Sprint(item))
			default:
				masked[key] = redactAny(item)
			}
		}
		return masked
	case []any:
		masked := make([]any, 0, len(typed))
		for _, item := range typed {
			masked = append(masked, redactAny(item))
		}
		return masked
	default:
		return value
	}
}

func maskIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 6 {
		return "[REDACTED]"
	}
	return value[:2] + "****" + value[len(value)-4:]
}

func sensitiveTool(name string) bool {
	switch tool.LogicalName(name) {
	case "user_purchase_history", "order_lookup", "warranty_check",
		"create_after_sales_ticket", "return_request", "exchange_request",
		"refund_status", "repair_status", "handoff_to_human":
		return true
	default:
		return false
	}
}

func sensitiveToolSideEffect(name string) string {
	switch tool.LogicalName(name) {
	case "create_after_sales_ticket", "return_request", "exchange_request", "handoff_to_human":
		return string(tool.SideEffectStateChange)
	default:
		return string(tool.SideEffectReadOnly)
	}
}

func maskToolArguments(arguments map[string]any) map[string]any {
	masked := make(map[string]any, len(arguments))
	for key, value := range arguments {
		switch strings.ToLower(key) {
		case "description", "contact", "phone", "mobile", "address", "user_id", "user_no":
			masked[key] = "[REDACTED]"
		case "order_no", "ticket_no":
			masked[key] = maskIdentifier(fmt.Sprint(value))
		default:
			masked[key] = value
		}
	}
	return masked
}

func initialAfterSalesStatus(issueType string) string {
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "return":
		return "return_requested"
	case "exchange":
		return "exchange_requested"
	case "refund":
		return "refund_reviewing"
	case "human_handoff":
		return "human_queued"
	case "repair", "":
		return "repair_requested"
	default:
		return "created"
	}
}

func afterSalesSLAHours(issueType string) int {
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "human_handoff":
		return 2
	case "refund":
		return 48
	case "return", "exchange":
		return 24
	default:
		return 72
	}
}

func afterSalesNextAction(issueType string) string {
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "human_handoff":
		return "Human agent will review conversation context and contact the user in queue order."
	case "return":
		return "Wait for return eligibility review and keep product, accessories, packaging, and invoice materials."
	case "exchange":
		return "Wait for exchange eligibility review and keep product condition evidence."
	case "refund":
		return "Wait for refund audit; refund timing depends on payment channel after approval."
	default:
		return "Keep the product powered off if safety risk exists and wait for diagnosis or repair scheduling."
	}
}

func afterSalesStage(issueType, status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "not_created" {
		return "no_after_sales_record"
	}
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "human_handoff":
		return "human_queue"
	case "refund":
		if strings.Contains(status, "done") || strings.Contains(status, "completed") {
			return "refund_completed"
		}
		return "refund_review"
	case "return":
		return "return_review"
	case "exchange":
		return "exchange_review"
	default:
		return "repair_review"
	}
}

func refundLikeIssue(issueType string) bool {
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "refund", "return":
		return true
	default:
		return false
	}
}

func estimatedQueuePosition(userID, orderNo, issueType string) int {
	key := userID + "|" + orderNo + "|" + issueType
	sum := 0
	for _, current := range key {
		sum += int(current)
	}
	return sum%8 + 1
}

func (r *BusinessRepository) querySKUs(ctx context.Context, refs []string) ([]model.ProductSKU, error) {
	conditions := []string{"p.status = 'active'", "s.status = 'active'"}
	args := make([]any, 0, len(refs)*3)
	if len(refs) > 0 {
		refConditions := make([]string, 0, len(refs))
		for _, ref := range refs {
			refConditions = append(refConditions, "(p.model = ? OR p.product_code = ? OR s.sku_code = ?)")
			args = append(args, ref, ref, ref)
		}
		conditions = append(conditions, "("+strings.Join(refConditions, " OR ")+")")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.sku_code, p.product_code, p.name, p.model, s.sku_name, s.specs_json,
		       s.list_price, COALESCE(i.current_price, s.list_price),
		       COALESCE(i.currency, 'CNY'), COALESCE(i.available_stock - i.reserved_stock, 0),
		       COALESCE(i.updated_at, s.updated_at)
		FROM product_skus s
		JOIN products p ON p.id = s.product_id
		LEFT JOIN product_inventory i ON i.sku_id = s.id
		WHERE `+strings.Join(conditions, " AND ")+`
		ORDER BY p.id, s.id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("query product skus: %w", err)
	}
	defer rows.Close()
	skus := make([]model.ProductSKU, 0)
	for rows.Next() {
		var sku model.ProductSKU
		var specsRaw []byte
		var listPriceRaw, currentPriceRaw string
		if err := rows.Scan(
			&sku.SKUCode, &sku.ProductCode, &sku.ProductName, &sku.Model, &sku.SKUName,
			&specsRaw, &listPriceRaw, &currentPriceRaw, &sku.Currency,
			&sku.AvailableStock, &sku.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan product sku: %w", err)
		}
		sku.ListPriceCents, err = parseDecimalCents(listPriceRaw)
		if err != nil {
			return nil, fmt.Errorf("parse product sku list price: %w", err)
		}
		sku.CurrentPriceCents, err = parseDecimalCents(currentPriceRaw)
		if err != nil {
			return nil, fmt.Errorf("parse product sku current price: %w", err)
		}
		if len(specsRaw) > 0 {
			_ = json.Unmarshal(specsRaw, &sku.Specs)
		}
		skus = append(skus, sku)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product skus: %w", err)
	}
	return skus, nil
}

func (r *BusinessRepository) availableCoupons(
	ctx context.Context,
	userID string,
	at time.Time,
) ([]model.CouponBenefit, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.coupon_code, c.name, c.discount_type, c.discount_value
		FROM user_coupons uc
		JOIN users u ON u.id = uc.user_id
		JOIN coupons c ON c.id = uc.coupon_id
		WHERE u.user_no = ? AND uc.status = 'available' AND c.status = 'active'
		  AND c.start_at <= ? AND c.end_at > ?
		ORDER BY c.discount_value DESC
	`, userID, at, at)
	if err != nil {
		return nil, fmt.Errorf("query available coupons: %w", err)
	}
	defer rows.Close()
	coupons := make([]model.CouponBenefit, 0)
	for rows.Next() {
		var coupon model.CouponBenefit
		var discountRaw string
		if err := rows.Scan(&coupon.CouponCode, &coupon.Name, &coupon.DiscountType, &discountRaw); err != nil {
			return nil, fmt.Errorf("scan coupon: %w", err)
		}
		switch coupon.DiscountType {
		case "amount":
			coupon.DiscountAmountCents, err = parseDecimalCents(discountRaw)
		case "percent":
			coupon.DiscountBasisPoints, err = parseDecimalBasisPoints(discountRaw)
		default:
			err = fmt.Errorf("unsupported discount type %q", coupon.DiscountType)
		}
		if err != nil {
			return nil, fmt.Errorf("parse coupon %s discount: %w", coupon.CouponCode, err)
		}
		coupons = append(coupons, coupon)
	}
	return coupons, rows.Err()
}

type productScanner interface {
	Scan(dest ...any) error
}

func scanProduct(scanner productScanner) (model.Product, error) {
	var product model.Product
	var attributesRaw []byte
	if err := scanner.Scan(
		&product.ProductCode, &product.Name, &product.Category, &product.Brand,
		&product.Model, &attributesRaw,
	); err != nil {
		return model.Product{}, err
	}
	if len(attributesRaw) > 0 {
		_ = json.Unmarshal(attributesRaw, &product.Attributes)
	}
	return product, nil
}

func scanPurchaseRecord(scanner productScanner) (model.PurchaseRecord, error) {
	var record model.PurchaseRecord
	var paidAt, deliveredAt sql.NullTime
	var unitPriceRaw string
	if err := scanner.Scan(
		&record.OrderNo, &record.Status, &record.ProductCode, &record.ProductName,
		&record.Model, &record.SKUCode, &record.Quantity, &unitPriceRaw,
		&paidAt, &deliveredAt, &record.OrderItemID, &record.WarrantyMonths,
	); err != nil {
		return model.PurchaseRecord{}, fmt.Errorf("scan purchase record: %w", err)
	}
	var err error
	record.UnitPriceCents, err = parseDecimalCents(unitPriceRaw)
	if err != nil {
		return model.PurchaseRecord{}, fmt.Errorf("parse purchase unit price: %w", err)
	}
	if paidAt.Valid {
		record.PaidAt = paidAt.Time
	}
	if deliveredAt.Valid {
		record.DeliveredAt = deliveredAt.Time
	}
	return record, nil
}
