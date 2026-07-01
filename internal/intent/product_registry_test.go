package intent

import (
	"context"
	"testing"

	"CleanCaregent/internal/productregistry"
)

func TestRuleRouterUsesProductRegistryForModelsAndCategories(t *testing.T) {
	registry := productregistry.New([]productregistry.Product{{
		ProductCode: "P-MOP-BM-M1",
		Model:       "BM-M1",
		Category:    "floor_mop",
		Brand:       "BrightClean",
		Aliases:     []string{"Breeze Mop M1"},
	}})
	result, err := NewRuleRouter(WithProductRegistry(registry)).Route(context.Background(), RouteRequest{
		Query: "Does Breeze Mop M1 support replacement pads?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Entities["models"] != "BM-M1" {
		t.Fatalf("models = %#v", result.Entities)
	}
	if result.Entities["category"] != "floor_mop" || result.Entities["categories"] != "floor_mop" {
		t.Fatalf("categories = %#v", result.Entities)
	}
}
