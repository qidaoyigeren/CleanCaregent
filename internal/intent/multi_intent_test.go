package intent

import (
	"context"
	"testing"
)

func TestRuleRouterBuildsPrimaryAndRouteTrace(t *testing.T) {
	result, err := NewRuleRouter().Route(context.Background(), RouteRequest{
		Query: "订单CC20260603001还在保修期吗？",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Primary != PrimaryAftersales {
		t.Fatalf("primary = %q, want %q", result.Primary, PrimaryAftersales)
	}
	if result.RouteTrace.Source != "rule" || len(result.RouteTrace.MatchedKeywords) == 0 {
		t.Fatalf("route trace = %#v", result.RouteTrace)
	}
}

func TestRuleRouterDetectsMultipleIntents(t *testing.T) {
	result, err := NewRuleRouter().Route(context.Background(), RouteRequest{
		Query: "T20吸力多大，现在多少钱还有货吗？",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.NeedDecomposition {
		t.Fatalf("result = %#v, want decomposition", result)
	}
	want := map[Type]bool{ProductParameter: true, PriceQuery: true, InventoryQuery: true}
	delete(want, result.Secondary)
	for _, secondary := range result.SecondaryIntents {
		delete(want, secondary)
	}
	if len(want) != 0 {
		t.Fatalf("missing intents: %#v in result %#v", want, result)
	}
}

func TestPrimaryForCoversIntentHierarchy(t *testing.T) {
	tests := map[Type]PrimaryType{
		ProductComparison:      PrimaryPresales,
		ReturnEligibility:      PrimaryAftersales,
		Troubleshooting:        PrimaryDiagnosis,
		Clarification:          PrimaryFallback,
		CreateAfterSalesTicket: PrimaryAftersales,
	}
	for secondary, want := range tests {
		if got := PrimaryFor(secondary); got != want {
			t.Fatalf("PrimaryFor(%s) = %s, want %s", secondary, got, want)
		}
	}
}
