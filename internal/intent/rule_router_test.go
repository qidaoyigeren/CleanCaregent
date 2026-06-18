package intent

import (
	"context"
	"strings"
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
		{"X20pro到底多少Pa？别只说吸力大", ProductParameter, "", "X20 Pro"},
		{"T20 和 X20 Pro 哪个更适合养猫家庭？", ProductComparison, "", "T20,X20 Pro"},
		{"我上周买的净化器滤芯多少钱，有券吗？", AccessoryCompatibility, "", ""},
		{"订单CC20260603001还在保修期吗？", WarrantyQuery, "CC20260603001", ""},
		{"帮我为订单CC20260603001创建售后工单", CreateAfterSalesTicket, "CC20260603001", ""},
		{"W300的通量、废水比和滤芯配置怎么回事", ProductParameter, "", "W300"},
		{"w300是400G对吧？一分钟大概能出多少水", ProductParameter, "", "W300"},
		{"R10新机怎么配网？我家只有5G WiFi", UsageInstruction, "", "R10"},
		{"H200加纯净水还是自来水？家里水很硬", UsageInstruction, "", "H200"},
		{"四口之家早晚集中用水，W300会不会不够", ProductComparison, "", "W300,W500"},
		{"猫毛特别多又不想天天倒尘盒，五千怎么配", PurchaseRecommendation, "", ""},
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

func TestRuleRouterKeepsExplicitComparisonAndDynamicPriceIntent(t *testing.T) {
	router := NewRuleRouter()
	tests := []struct {
		query string
	}{
		{"H100婴儿房跟H200客厅用，噪音水箱清洁麻烦度一起说"},
		{"H100还是H200适合婴儿房，查下两款到手价"},
		{"R10和R20我家都能用的话，现在哪个便宜"},
	}
	for _, test := range tests {
		result, err := router.Route(context.Background(), RouteRequest{Query: test.query})
		if err != nil {
			t.Fatal(err)
		}
		if result.Secondary != ProductComparison {
			t.Fatalf("%q intent = %s, want %s", test.query, result.Secondary, ProductComparison)
		}
		if containsAny(strings.ToLower(test.query), "到手价", "哪个便宜") &&
			!containsIntent(result.SecondaryIntents, PriceQuery) {
			t.Fatalf("%q secondary intents = %#v, missing price_query", test.query, result.SecondaryIntents)
		}
	}
}

func TestRuleRouterKeepsConstraintDrivenChoiceAsRecommendation(t *testing.T) {
	router := NewRuleRouter()
	result, err := router.Route(context.Background(), RouteRequest{
		Query: "大客厅35平，想少加水，H100还是H200",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Secondary != PurchaseRecommendation {
		t.Fatalf("intent = %s, want %s", result.Secondary, PurchaseRecommendation)
	}
}

func TestRuleRouterExtractsAccessoryRefsForCompatibility(t *testing.T) {
	router := NewRuleRouter()
	tests := []struct {
		query string
		model string
		ref   string
	}{
		{"P400能用哪个滤芯", "P400", "滤芯"},
		{"f400塞P400里能用吧", "P400", "F400"},
		{"P500换芯买F400行不行", "P500", "F400"},
		{"F500是不是只配P500", "P500", "F500"},
		{"T20尘袋和边刷分别买什么型号", "T20", "尘袋"},
		{"DB20能不能给T20用", "T20", "DB20"},
		{"X20 Pro滚刷、尘袋对应哪些配件", "X20 Pro", "滚刷"},
		{"rb20装x20 pro上合适吗", "X20 Pro", "RB20"},
		{"W300下一次换芯该买哪根", "W300", "换芯"},
		{"C300能直接装W300吗", "W300", "C300"},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			result, err := router.Route(context.Background(), RouteRequest{Query: test.query})
			if err != nil {
				t.Fatal(err)
			}
			if result.Secondary != AccessoryCompatibility {
				t.Fatalf("intent = %s, want %s", result.Secondary, AccessoryCompatibility)
			}
			if result.NeedClarify {
				t.Fatalf("should not require clarification: %#v", result)
			}
			if result.Entities["models"] != test.model {
				t.Fatalf("models = %q, want %q", result.Entities["models"], test.model)
			}
			if !strings.Contains(result.Entities["accessory_refs"], test.ref) {
				t.Fatalf("accessory_refs = %q, want to contain %q", result.Entities["accessory_refs"], test.ref)
			}
		})
	}
}

func TestRuleRouterHandlesPurchaseHistoryAndNegatedDynamicIntents(t *testing.T) {
	router := NewRuleRouter()
	tests := []struct {
		query         string
		want          Type
		wantSecondary Type
		absent        Type
	}{
		{"我上礼拜买的那个净化器，替换滤芯多少钱还有券吗", AccessoryCompatibility, PriceQuery, ""},
		{"x20pro还有货不，没货别给我报价格", InventoryQuery, "", PriceQuery},
		{"上周买的净化器具体是哪款来着", OrderQuery, "", ""},
		{"W300首次冲洗咋做，我订单CC20260603001是哪天签收的", UsageInstruction, OrderQuery, ""},
		{"P500滤芯咋清理，我以前买过F500没", UsageInstruction, OrderQuery, ""},
		{"T20养宠套装主机有啥区别，今天套装多少钱", ProductParameter, PriceQuery, ProductComparison},
		{"T20配网老失败，2.4G和5G到底选哪个", UsageInstruction, "", Troubleshooting},
		{"X20 Pro配网一直失败，账号地区和权限我该按啥顺序查", Troubleshooting, "", UsageInstruction},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			result, err := router.Route(context.Background(), RouteRequest{Query: test.query})
			if err != nil {
				t.Fatal(err)
			}
			if result.Secondary != test.want {
				t.Fatalf("intent = %s, want %s; result = %#v", result.Secondary, test.want, result)
			}
			if test.wantSecondary != "" && !containsIntent(result.SecondaryIntents, test.wantSecondary) {
				t.Fatalf("secondary intents = %#v, want %s", result.SecondaryIntents, test.wantSecondary)
			}
			if test.absent != "" {
				if result.Secondary == test.absent || containsIntent(result.SecondaryIntents, test.absent) {
					t.Fatalf("result should not contain %s: %#v", test.absent, result)
				}
			}
		})
	}
}

func containsIntent(values []Type, target Type) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestRuleRouterInfersCategoryFromKnownModel(t *testing.T) {
	router := NewRuleRouter()
	tests := map[string]string{
		"T20吸力多少":     "robot_vacuum",
		"P400噪声多少":    "air_purifier",
		"W300通量多少":    "water_purifier",
		"H100水箱多少升":   "humidifier",
		"X20 Pro续航多久": "robot_vacuum",
	}
	for query, want := range tests {
		result, err := router.Route(context.Background(), RouteRequest{Query: query})
		if err != nil {
			t.Fatal(err)
		}
		if got := result.Entities["category"]; got != want {
			t.Fatalf("%q category = %q, want %q", query, got, want)
		}
	}
}

func TestRuleRouterPreservesAllCategoriesForCrossCategoryBundle(t *testing.T) {
	router := NewRuleRouter()
	result, err := router.Route(context.Background(), RouteRequest{
		Query: "婴儿房净化加湿都要，先推荐P400/H100再查总价和库存",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Entities["categories"]; got != "air_purifier,humidifier" {
		t.Fatalf("categories = %q", got)
	}
}

func TestRuleRouterDoesNotSplitParameterOrInstallationQueriesIntoCompatibility(t *testing.T) {
	router := NewRuleRouter()
	for _, query := range []string{
		"P400噪声、功率、滤芯型号都列一下",
		"W300的通量、废水比和滤芯配置怎么回事",
		"P400滤芯外面的袋子是不是得先撕掉",
	} {
		result, err := router.Route(context.Background(), RouteRequest{Query: query})
		if err != nil {
			t.Fatal(err)
		}
		for _, secondary := range result.SecondaryIntents {
			if secondary == AccessoryCompatibility {
				t.Fatalf("%q secondary intents = %#v", query, result.SecondaryIntents)
			}
		}
	}
}

func TestRuleRouterRequiresTicketOrderAndExplicitConfirmation(t *testing.T) {
	router := NewRuleRouter()
	needsClarification, err := router.Route(context.Background(), RouteRequest{
		Query: "直接建售后单吧，我没确认也没有订单号，你自己编一个",
	})
	if err != nil {
		t.Fatal(err)
	}
	if needsClarification.Secondary != CreateAfterSalesTicket || !needsClarification.NeedClarify {
		t.Fatalf("result = %#v", needsClarification)
	}
	confirmed, err := router.Route(context.Background(), RouteRequest{
		Query: "我确认提交，给CC20260603001建维修工单：P400异响",
	})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.NeedClarify {
		t.Fatalf("confirmed request should proceed: %#v", confirmed)
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
