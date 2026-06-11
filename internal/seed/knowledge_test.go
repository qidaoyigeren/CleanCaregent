package seed

import "testing"

func TestDefaultKnowledgeDocuments(t *testing.T) {
	documents := DefaultKnowledgeDocuments()
	if len(documents) != 57 {
		t.Fatalf("document count = %d, want 57", len(documents))
	}
	counts := map[string]int{}
	ids := map[string]struct{}{}
	for _, document := range documents {
		counts[document.DocType]++
		if _, exists := ids[document.DocID]; exists {
			t.Fatalf("duplicate doc_id %s", document.DocID)
		}
		ids[document.DocID] = struct{}{}
	}
	expected := map[string]int{
		"product_detail":          10,
		"product_parameter":       5,
		"product_comparison":      5,
		"purchase_guide":          5,
		"accessory_compatibility": 5,
		"user_manual":             8,
		"troubleshooting":         6,
		"after_sales_policy":      5,
		"faq":                     8,
	}
	for docType, want := range expected {
		if counts[docType] != want {
			t.Fatalf("%s count = %d, want %d", docType, counts[docType], want)
		}
	}
}
