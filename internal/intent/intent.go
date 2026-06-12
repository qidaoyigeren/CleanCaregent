package intent

import (
	"context"

	"CleanCaregent/internal/model"
)

type Type string

// PrimaryType is the top-level business intent used for routing and metrics.
type PrimaryType string

const (
	PrimaryPresales   PrimaryType = "presales"
	PrimaryAftersales PrimaryType = "aftersales"
	PrimaryDiagnosis  PrimaryType = "diagnosis"
	PrimaryFallback   PrimaryType = "fallback"

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

// RouteTrace explains why the router selected an intent.
type RouteTrace struct {
	Source          string   `json:"source"`
	MatchedKeywords []string `json:"matched_keywords,omitempty"`
	Reasoning       string   `json:"reasoning,omitempty"`
	ConfidenceBasis string   `json:"confidence_basis,omitempty"`
}

type Result struct {
	Primary           PrimaryType       `json:"primary"`
	Secondary         Type              `json:"secondary"`
	SecondaryIntents  []Type            `json:"secondary_intents,omitempty"`
	Confidence        float64           `json:"confidence"`
	Entities          map[string]string `json:"entities,omitempty"`
	NeedClarify       bool              `json:"need_clarify"`
	NeedDecomposition bool              `json:"need_decomposition,omitempty"`
	CompetitorMention bool              `json:"competitor_mention,omitempty"`
	Competitors       []string          `json:"competitors,omitempty"`
	CompetitorPolicy  string            `json:"competitor_policy,omitempty"`
	RouteTrace        RouteTrace        `json:"route_trace"`
}

type RouteRequest struct {
	Query          string
	Summary        string
	RecentMessages []model.Message
	Primary        PrimaryType
}

type Router interface {
	Route(ctx context.Context, request RouteRequest) (Result, error)
}

// PrimaryFor returns the top-level intent that owns a secondary intent.
func PrimaryFor(value Type) PrimaryType {
	switch value {
	case ProductParameter, ProductComparison, PurchaseRecommendation,
		AccessoryCompatibility, UsageInstruction, PriceQuery, InventoryQuery:
		return PrimaryPresales
	case OrderQuery, WarrantyQuery, ReturnEligibility, CreateAfterSalesTicket:
		return PrimaryAftersales
	case Troubleshooting:
		return PrimaryDiagnosis
	default:
		return PrimaryFallback
	}
}
