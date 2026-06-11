package retriever

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"CleanCaregent/internal/model"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/repository"
)

type ProductCatalog interface {
	FindProductByModel(ctx context.Context, modelName string) (model.Product, error)
}

// StructuredFirst prepends exact product attributes from MySQL before semantic
// knowledge results. Prices and inventory are intentionally excluded because
// they belong to dynamic tools.
type StructuredFirst struct {
	next    rag.Retriever
	catalog ProductCatalog
}

func NewStructuredFirst(next rag.Retriever, catalog ProductCatalog) *StructuredFirst {
	return &StructuredFirst{next: next, catalog: catalog}
}

func (r *StructuredFirst) Search(
	ctx context.Context,
	request rag.SearchRequest,
) ([]rag.SearchResult, error) {
	results, retrievalErr := r.next.Search(ctx, request)
	if r.catalog == nil || !supportsStructuredProductLookup(request.Filter.DocTypes) {
		return results, retrievalErr
	}

	structured := make([]rag.SearchResult, 0, len(request.Filter.Models))
	for _, modelName := range compactStrings(request.Filter.Models) {
		product, err := r.catalog.FindProductByModel(ctx, modelName)
		if errors.Is(err, repository.ErrProductNotFound) {
			continue
		}
		if err != nil {
			if retrievalErr != nil {
				return nil, retrievalErr
			}
			continue
		}
		content, _ := json.Marshal(map[string]any{
			"product_code": product.ProductCode,
			"name":         product.Name,
			"category":     product.Category,
			"brand":        product.Brand,
			"model":        product.Model,
			"attributes":   product.Attributes,
		})
		structured = append(structured, rag.SearchResult{
			ChunkID:     "structured:product:" + product.ProductCode,
			DocumentID:  "structured_products",
			Title:       product.Model + " 结构化商品参数",
			Content:     string(content),
			FusionScore: 1,
			RerankScore: 1,
			Metadata: map[string]any{
				"source_type": "mysql_structured_product",
				"doc_type":    "product_parameter",
				"model":       product.Model,
				"category":    product.Category,
				"brand":       product.Brand,
			},
		})
	}
	if len(structured) == 0 {
		return results, retrievalErr
	}
	return mergeStructuredResults(structured, results), nil
}

func supportsStructuredProductLookup(docTypes []string) bool {
	if len(docTypes) == 0 {
		return true
	}
	for _, docType := range docTypes {
		switch strings.TrimSpace(docType) {
		case "product_parameter", "product_detail", "product_comparison", "purchase_guide":
			return true
		}
	}
	return false
}

func mergeStructuredResults(first, second []rag.SearchResult) []rag.SearchResult {
	seen := make(map[string]struct{}, len(first)+len(second))
	result := make([]rag.SearchResult, 0, len(first)+len(second))
	for _, item := range append(first, second...) {
		if _, ok := seen[item.ChunkID]; ok {
			continue
		}
		seen[item.ChunkID] = struct{}{}
		result = append(result, item)
	}
	return result
}
