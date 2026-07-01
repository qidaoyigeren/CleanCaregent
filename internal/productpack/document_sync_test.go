package productpack

import (
	"testing"

	"CleanCaregent/internal/service"
)

func TestBusinessProductsFromKnowledgeDocuments(t *testing.T) {
	products := BusinessProductsFromKnowledgeDocuments([]service.IngestDocumentRequest{{
		DocID:    "doc-fd4",
		Title:    "FD4 静电除尘掸",
		Category: "duster",
		Brand:    "CleanCare",
		Metadata: map[string]any{
			"product_code":        "P-FD4",
			"model":               "FD4",
			"sku_code":            "SKU-FD4-BASE",
			"current_price_cents": int64(12900),
			"list_price_cents":    int64(15900),
			"available_stock":     42,
			"warranty_months":     12,
		},
	}})
	if len(products) != 1 {
		t.Fatalf("products = %d, want 1", len(products))
	}
	product := products[0]
	if product.ProductCode != "P-FD4" || product.Model != "FD4" || product.Category != "duster" {
		t.Fatalf("product = %#v", product)
	}
	if len(product.SKUs) != 1 {
		t.Fatalf("skus = %d, want 1", len(product.SKUs))
	}
	sku := product.SKUs[0]
	if sku.SKUCode != "SKU-FD4-BASE" || sku.CurrentPriceCents != 12900 || sku.AvailableStock != 42 {
		t.Fatalf("sku = %#v", sku)
	}
}

func TestBusinessProductsFromKnowledgeDocumentsSkipsStaticOnlyDocs(t *testing.T) {
	products := BusinessProductsFromKnowledgeDocuments([]service.IngestDocumentRequest{{
		DocID: "doc-static",
		Title: "只包含说明",
		Metadata: map[string]any{
			"model": "FD4",
		},
	}})
	if len(products) != 0 {
		t.Fatalf("products = %#v, want none", products)
	}
}
