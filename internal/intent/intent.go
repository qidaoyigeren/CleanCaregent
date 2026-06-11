package intent

import (
	"context"

	"CleanCaregent/internal/model"
)

type Type string

const (
	ProductParameter       Type = "product_parameter"
	ProductComparison      Type = "product_comparison"
	PurchaseRecommendation Type = "purchase_recommendation"
	AccessoryCompatibility Type = "accessory_compatibility"
	UsageInstruction       Type = "usage_instruction"
	PriceQuery             Type = "price_query"
	InventoryQuery         Type = "inventory_query"
	OrderQuery             Type = "order_query"
	WarrantyQuery          Type = "warranty_query"
	ReturnEligibility      Type = "return_eligibility"
	Troubleshooting        Type = "troubleshooting"
	CreateAfterSalesTicket Type = "create_after_sales_ticket"
	Clarification          Type = "clarification"
	OutOfScope             Type = "out_of_scope"
	Chitchat               Type = "chitchat"
)

type Result struct {
	Primary     string            `json:"primary"`
	Secondary   Type              `json:"secondary"`
	Confidence  float64           `json:"confidence"`
	Entities    map[string]string `json:"entities,omitempty"`
	NeedClarify bool              `json:"need_clarify"`
}

type RouteRequest struct {
	Query          string
	Summary        string
	RecentMessages []model.Message
}

type Router interface {
	Route(ctx context.Context, request RouteRequest) (Result, error)
}
