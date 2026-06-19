package model

import "time"

type Product struct {
	ProductCode string         `json:"product_code"`
	Name        string         `json:"name"`
	Category    string         `json:"category"`
	Brand       string         `json:"brand"`
	Model       string         `json:"model"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	SKUs        []ProductSKU   `json:"skus,omitempty"`
}

type ProductSKU struct {
	SKUCode           string         `json:"sku_code"`
	ProductCode       string         `json:"product_code"`
	ProductName       string         `json:"product_name"`
	Model             string         `json:"model"`
	SKUName           string         `json:"sku_name"`
	Specs             map[string]any `json:"specs,omitempty"`
	ListPriceCents    int64          `json:"list_price_cents"`
	CurrentPriceCents int64          `json:"current_price_cents"`
	Currency          string         `json:"currency"`
	AvailableStock    int            `json:"available_stock"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type CouponBenefit struct {
	CouponCode          string `json:"coupon_code"`
	Name                string `json:"name"`
	DiscountType        string `json:"discount_type"`
	DiscountAmountCents int64  `json:"discount_amount_cents,omitempty"`
	DiscountBasisPoints int64  `json:"discount_basis_points,omitempty"`
}

type PriceQuote struct {
	ProductSKU
	EstimatedFinalPriceCents int64           `json:"estimated_final_price_cents"`
	AvailableCoupons         []CouponBenefit `json:"available_coupons"`
}

type PurchaseRecord struct {
	OrderNo        string    `json:"order_no"`
	Status         string    `json:"status"`
	ProductCode    string    `json:"product_code"`
	ProductName    string    `json:"product_name"`
	Model          string    `json:"model"`
	SKUCode        string    `json:"sku_code"`
	Quantity       int       `json:"quantity"`
	UnitPriceCents int64     `json:"unit_price_cents"`
	PaidAt         time.Time `json:"paid_at,omitempty"`
	DeliveredAt    time.Time `json:"delivered_at,omitempty"`
	OrderItemID    int64     `json:"-"`
	WarrantyMonths int       `json:"warranty_months"`
}

type OrderDetail struct {
	OrderNo          string           `json:"order_no"`
	UserID           string           `json:"user_id"`
	Status           string           `json:"status"`
	TotalAmountCents int64            `json:"total_amount_cents"`
	PaidAt           *time.Time       `json:"paid_at,omitempty"`
	DeliveredAt      *time.Time       `json:"delivered_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	Items            []PurchaseRecord `json:"items"`
}

type WarrantyStatus struct {
	OrderNo        string     `json:"order_no"`
	ProductName    string     `json:"product_name"`
	Model          string     `json:"model"`
	InWarranty     bool       `json:"in_warranty"`
	WarrantyMonths int        `json:"warranty_months"`
	WarrantyStart  *time.Time `json:"warranty_start,omitempty"`
	WarrantyEnd    *time.Time `json:"warranty_end,omitempty"`
	Reason         string     `json:"reason"`
	OrderItemID    int64      `json:"-"`
}

type AfterSalesTicket struct {
	TicketNo         string    `json:"ticket_no"`
	UserID           string    `json:"user_id"`
	OrderNo          string    `json:"order_no"`
	OrderItemID      int64     `json:"order_item_id"`
	IssueType        string    `json:"issue_type"`
	Description      string    `json:"description"`
	DiagnosisSummary string    `json:"diagnosis_summary,omitempty"`
	EvidenceIDs      []string  `json:"evidence_ids,omitempty"`
	Status           string    `json:"status"`
	IdempotencyKey   string    `json:"idempotency_key"`
	CreatedAt        time.Time `json:"created_at"`
}

type AfterSalesActionResult struct {
	Action        string            `json:"action"`
	Ticket        AfterSalesTicket  `json:"ticket"`
	QueuePosition int               `json:"queue_position,omitempty"`
	SLAHours      int               `json:"sla_hours,omitempty"`
	NextAction    string            `json:"next_action,omitempty"`
	Audit         map[string]string `json:"audit,omitempty"`
}

type AfterSalesProgress struct {
	TicketNo              string     `json:"ticket_no,omitempty"`
	OrderNo               string     `json:"order_no"`
	IssueType             string     `json:"issue_type"`
	Status                string     `json:"status"`
	Stage                 string     `json:"stage"`
	NextAction            string     `json:"next_action,omitempty"`
	RefundAmountCents     int64      `json:"refund_amount_cents,omitempty"`
	EstimatedCompletionAt *time.Time `json:"estimated_completion_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at,omitempty"`
	UpdatedAt             time.Time  `json:"updated_at,omitempty"`
}
