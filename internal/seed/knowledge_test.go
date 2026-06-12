package seed

import (
	"strings"
	"testing"
)

func TestDefaultKnowledgeDocuments(t *testing.T) {
	documents := DefaultKnowledgeDocuments()
	if len(documents) != 143 {
		t.Fatalf("document count = %d, want 143", len(documents))
	}
	counts := map[string]int{}
	ids := map[string]struct{}{}
	productParameterRows := map[string]int{}
	for _, document := range documents {
		counts[document.DocType]++
		if _, exists := ids[document.DocID]; exists {
			t.Fatalf("duplicate doc_id %s", document.DocID)
		}
		ids[document.DocID] = struct{}{}
		if !strings.HasPrefix(document.Source, "mock://") {
			t.Fatalf("%s source = %q, want mock:// prefix", document.DocID, document.Source)
		}
		if strings.TrimSpace(anyString(document.Metadata["structural_difficulty"])) == "" {
			t.Fatalf("%s is missing structural_difficulty metadata", document.DocID)
		}
		if document.DocType == "product_parameter" {
			model, _ := document.Metadata["model"].(string)
			productParameterRows[model] = strings.Count(document.Content, "\n| ")
		}
		switch document.DocType {
		case "product_comparison":
			if rows := strings.Count(document.Content, "\n| "); rows < 10 {
				t.Fatalf("%s comparison rows = %d, want at least 10", document.DocID, rows)
			}
		case "purchase_guide":
			if !strings.Contains(document.Content, "## 决策流程") || len([]rune(document.Content)) < 250 {
				t.Fatalf("%s purchase guide is too sparse", document.DocID)
			}
		case "user_manual":
			if strings.Count(document.Content, "## 任务") < 5 {
				t.Fatalf("%s manual does not preserve task sections", document.DocID)
			}
		case "troubleshooting":
			if strings.Count(document.Content, "node_id:") < 5 ||
				!strings.Contains(document.Content, "parent_node_id:") {
				t.Fatalf("%s troubleshooting tree is not structured", document.DocID)
			}
		case "after_sales_policy":
			if strings.Count(document.Content, "条：") < 4 {
				t.Fatalf("%s policy clauses are incomplete", document.DocID)
			}
		case "faq":
			if !strings.Contains(document.Content, "Q:") || !strings.Contains(document.Content, "A:") {
				t.Fatalf("%s faq is not a complete question-answer pair", document.DocID)
			}
		}
	}
	expected := map[string]int{
		"product_detail":          50,
		"product_parameter":       10,
		"product_comparison":      5,
		"purchase_guide":          15,
		"accessory_compatibility": 5,
		"user_manual":             25,
		"troubleshooting":         10,
		"after_sales_policy":      15,
		"faq":                     8,
	}
	for docType, want := range expected {
		if counts[docType] != want {
			t.Fatalf("%s count = %d, want %d", docType, counts[docType], want)
		}
	}
	for _, product := range defaultProducts() {
		if rows := productParameterRows[product.Model]; rows < 15 {
			t.Fatalf("%s parameter rows = %d, want at least 15", product.Model, rows)
		}
	}
}

func anyString(value any) string {
	text, _ := value.(string)
	return text
}
