package retriever

import (
	"context"
	"testing"

	"CleanCaregent/internal/model"
	"CleanCaregent/internal/rag"
)

type structuredRetriever struct{}

func (structuredRetriever) Search(context.Context, rag.SearchRequest) ([]rag.SearchResult, error) {
	return []rag.SearchResult{{
		ChunkID:    "kb_1",
		DocumentID: "doc_1",
		Title:      "T20 参数文档",
		Content:    "文档补充说明",
	}}, nil
}

type structuredCatalog struct{}

func (structuredCatalog) FindProductByModel(context.Context, string) (model.Product, error) {
	return model.Product{
		ProductCode: "P-RV-T20",
		Name:        "CleanCare T20",
		Category:    "robot_vacuum",
		Brand:       "CleanCare",
		Model:       "T20",
		Attributes:  map[string]any{"suction_pa": 6000},
	}, nil
}

func TestStructuredFirstPrependsExactProductAttributes(t *testing.T) {
	retriever := NewStructuredFirst(structuredRetriever{}, structuredCatalog{})
	results, err := retriever.Search(context.Background(), rag.SearchRequest{
		Query:  "T20吸力多大",
		Filter: rag.MetadataFilter{Models: []string{"T20"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].ChunkID != "structured:product:P-RV-T20" {
		t.Fatalf("results = %#v", results)
	}
	if results[0].Metadata["source_type"] != "mysql_structured_product" {
		t.Fatalf("metadata = %#v", results[0].Metadata)
	}
}
