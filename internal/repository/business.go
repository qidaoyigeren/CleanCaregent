package repository

import (
	"context"
	"errors"
	"time"

	"CleanCaregent/internal/model"
)

var (
	ErrProductNotFound   = errors.New("product not found")
	ErrOrderNotFound     = errors.New("order not found")
	ErrOrderItemNotFound = errors.New("order item not found")
	ErrTicketConflict    = errors.New("after-sales ticket already exists")
)

type PurchaseHistoryFilter struct {
	Category string
	Model    string
	Since    *time.Time
	Until    *time.Time
	Limit    int
}

type CreateTicketRequest struct {
	UserID           string
	OrderNo          string
	OrderItemID      int64
	IssueType        string
	Description      string
	DiagnosisSummary string
	EvidenceIDs      []string
	IdempotencyKey   string
}

type AfterSalesActionRequest struct {
	UserID           string
	OrderNo          string
	OrderItemID      int64
	Action           string
	IssueType        string
	Reason           string
	Description      string
	DiagnosisSummary string
	EvidenceIDs      []string
	IdempotencyKey   string
}

type AfterSalesProgressFilter struct {
	UserID    string
	OrderNo   string
	TicketNo  string
	IssueType string
	Limit     int
}

type BusinessRepository interface {
	ListProducts(ctx context.Context, category string, limit int) ([]model.Product, error)
	GetProduct(ctx context.Context, productCode string) (model.Product, error)
	FindProductByModel(ctx context.Context, modelName string) (model.Product, error)
	QueryPrices(ctx context.Context, userID string, productRefs []string) ([]model.PriceQuote, error)
	CheckInventory(ctx context.Context, productRefs []string) ([]model.ProductSKU, error)
	ListPurchaseHistory(ctx context.Context, userID string, filter PurchaseHistoryFilter) ([]model.PurchaseRecord, error)
	GetOrder(ctx context.Context, userID, orderNo string) (model.OrderDetail, error)
	CheckWarranty(ctx context.Context, userID, orderNo, modelName string, at time.Time) ([]model.WarrantyStatus, error)
	CreateAfterSalesTicket(ctx context.Context, request CreateTicketRequest) (model.AfterSalesTicket, error)
	RequestAfterSalesAction(ctx context.Context, request AfterSalesActionRequest) (model.AfterSalesActionResult, error)
	GetAfterSalesProgress(ctx context.Context, filter AfterSalesProgressFilter) ([]model.AfterSalesProgress, error)
}
