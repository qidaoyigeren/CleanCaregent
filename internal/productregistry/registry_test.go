package productregistry

import "testing"

func TestProductsFromDocumentsExposeKBModelsAndAliases(t *testing.T) {
	products := ProductsFromDocuments([]Document{{
		DocID:    "kb-fd4",
		Title:    "FD4 静电除尘掸说明",
		Content:  "产品型号：FD4。替换掸套型号：FD4-PAD。",
		Category: "duster",
		Brand:    "CleanCare",
		Metadata: map[string]any{
			"product_code": "P-FD4",
			"model":        "FD4",
			"aliases":      []any{"静电掸", "长柄除尘"},
		},
	}})
	registry := New(products)

	matches := registry.MatchModels("静电掸的 FD4-PAD 能给 FD4 用吗")
	if !containsModel(matches, "FD4") || !containsModel(matches, "FD4-PAD") {
		t.Fatalf("matches = %#v", matches)
	}
	if matches[0] != "FD4" {
		t.Fatalf("first match = %q, want host product FD4 before accessory matches: %#v", matches[0], matches)
	}
	if category := registry.CategoryForModel("FD4"); category != "duster" {
		t.Fatalf("category = %q", category)
	}
}

func TestProductsFromDocumentsCanInferLabeledAccessoryModel(t *testing.T) {
	products := ProductsFromDocuments([]Document{{
		DocID:   "kb-gb2-br",
		Title:   "滚刷配件",
		Content: "配件型号：GB2-BR\n适配主机：GB2",
		Metadata: map[string]any{
			"category": "accessory",
			"brand":    "CleanCare",
		},
	}})
	registry := New(products)

	matches := registry.MatchModels("GB2-BR 是否兼容")
	if len(matches) == 0 || matches[0] != "GB2-BR" {
		t.Fatalf("matches = %#v", matches)
	}
}

func containsModel(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
