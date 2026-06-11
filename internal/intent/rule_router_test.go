package intent

import (
	"context"
	"testing"

	"CleanCaregent/internal/model"
)

func TestRuleRouterRoutesCoreScenarios(t *testing.T) {
	router := NewRuleRouter()
	tests := []struct {
		query      string
		wantIntent Type
		wantOrder  string
		wantModels string
	}{
		{"T20 吸力多大？", ProductParameter, "", "T20"},
		{"T20 和 X20 Pro 哪个更适合养猫家庭？", ProductComparison, "", "T20,X20 Pro"},
		{"我上周买的净化器滤芯多少钱，有券吗？", AccessoryCompatibility, "", ""},
		{"订单CC20260603001还在保修期吗？", WarrantyQuery, "CC20260603001", ""},
		{"帮我为订单CC20260603001创建售后工单", CreateAfterSalesTicket, "CC20260603001", ""},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			result, err := router.Route(context.Background(), RouteRequest{Query: test.query})
			if err != nil {
				t.Fatal(err)
			}
			if result.Secondary != test.wantIntent {
				t.Fatalf("intent = %s, want %s", result.Secondary, test.wantIntent)
			}
			if result.Entities["order_no"] != test.wantOrder {
				t.Fatalf("order_no = %q, want %q", result.Entities["order_no"], test.wantOrder)
			}
			if result.Entities["models"] != test.wantModels {
				t.Fatalf("models = %q, want %q", result.Entities["models"], test.wantModels)
			}
		})
	}
}

func TestRuleRouterOnlyResolvesReferenceWithConversationContext(t *testing.T) {
	router := NewRuleRouter()
	withoutContext, err := router.Route(context.Background(), RouteRequest{Query: "这个多少钱？"})
	if err != nil {
		t.Fatal(err)
	}
	if !withoutContext.NeedClarify {
		t.Fatal("reference without prior model should clarify")
	}
	withContext, err := router.Route(context.Background(), RouteRequest{
		Query:          "这个多少钱？",
		RecentMessages: []model.Message{{Role: "user", Content: "T20 吸力多大？"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if withContext.NeedClarify {
		t.Fatal("reference with prior model should not clarify")
	}
}
