package skill

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/model"
	"CleanCaregent/internal/rag"
)

type routeCapturingRetriever struct {
	mu       sync.Mutex
	requests []rag.SearchRequest
}

func (r *routeCapturingRetriever) Search(_ context.Context, request rag.SearchRequest) ([]rag.SearchResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
	return []rag.SearchResult{{
		ChunkID:    strings.Join(request.Filter.DocTypes, ","),
		DocumentID: strings.Join(request.Filter.DocTypes, ","),
	}}, nil
}

func TestBuildReturnEligibilityAnswerUsesOrderFacts(t *testing.T) {
	deliveredAt := time.Date(2026, 5, 20, 10, 0, 0, 0, time.Local)
	paidAt := deliveredAt.Add(-48 * time.Hour)
	answer := buildReturnEligibilityAnswer(
		"订单CC20260518001买了超过7天，包装拆了还能退吗？",
		model.OrderDetail{
			OrderNo:     "CC20260518001",
			Status:      "delivered",
			PaidAt:      &paidAt,
			DeliveredAt: &deliveredAt,
			Items: []model.PurchaseRecord{
				{ProductName: "CleanCare T20 扫地机器人"},
			},
		},
		[]rag.SearchResult{
			{DocumentID: "kb_policy_return_7d"},
			{DocumentID: "kb_policy_quality_exchange"},
			{DocumentID: "kb_policy_warranty"},
		},
		"[E4]",
		"[E5]",
		time.Date(2026, 6, 11, 12, 0, 0, 0, time.Local),
	)

	for _, expected := range []string{
		"CC20260518001",
		"CleanCare T20 扫地机器人",
		"已超过 7 天",
		"包装已经拆开",
		"[E1]",
		"[E4]",
		"[E5]",
	} {
		if !strings.Contains(answer, expected) {
			t.Fatalf("answer missing %q:\n%s", expected, answer)
		}
	}
	if strings.Contains(answer, "未提供具体日期") {
		t.Fatalf("answer ignored the order facts:\n%s", answer)
	}
}

func TestReturnEligibilityDerivedEvidenceGroundsElapsedDays(t *testing.T) {
	deliveredAt := time.Date(2026, 5, 20, 10, 0, 0, 0, time.Local)
	evidence, ok := returnEligibilityDerivedEvidence(
		model.OrderDetail{
			OrderNo:     "CC20260518001",
			DeliveredAt: &deliveredAt,
		},
		time.Date(2026, 6, 11, 12, 0, 0, 0, time.Local),
	)
	if !ok {
		t.Fatal("expected derived evidence")
	}
	if evidence.Kind != "derived_fact" || !strings.Contains(evidence.Content, "22 天") {
		t.Fatalf("unexpected derived evidence: %#v", evidence)
	}
}

func TestBuildWarrantyAnswerUsesToolResult(t *testing.T) {
	start := time.Date(2026, 6, 5, 9, 0, 0, 0, time.Local)
	end := start.AddDate(1, 0, 0)
	answer := buildWarrantyAnswer(
		warrantyToolPayload{
			Items: []model.WarrantyStatus{
				{
					OrderNo:       "CC20260603001",
					ProductName:   "CleanCare X20 Pro 扫地机器人",
					Model:         "X20 Pro",
					InWarranty:    true,
					WarrantyStart: &start,
					WarrantyEnd:   &end,
					Reason:        "查询时间早于保修截止时间",
				},
			},
			CheckedAt: time.Date(2026, 6, 11, 12, 0, 0, 0, time.Local),
		},
		[]rag.SearchResult{{DocumentID: "kb_policy_warranty"}},
		"[E2]",
	)

	for _, expected := range []string{
		"CC20260603001",
		"X20 Pro",
		"在保修期内",
		"[E1]",
		"[E2]",
	} {
		if !strings.Contains(answer, expected) {
			t.Fatalf("answer missing %q:\n%s", expected, answer)
		}
	}
}

func TestRequestedRecommendationTools(t *testing.T) {
	tests := []struct {
		name  string
		input Request
		want  []string
	}{
		{
			name: "pure recommendation",
			input: Request{
				Query:  "120平两只猫，预算5000元推荐扫地机器人",
				Intent: intent.Result{Secondary: intent.PurchaseRecommendation},
			},
		},
		{
			name: "secondary intents",
			input: Request{
				Query: "推荐适合大户型的型号",
				Intent: intent.Result{
					Secondary:        intent.PurchaseRecommendation,
					SecondaryIntents: []intent.Type{intent.PriceQuery, intent.InventoryQuery},
				},
			},
			want: []string{"price_query", "inventory_check"},
		},
		{
			name: "query fallback",
			input: Request{
				Query:  "推荐一个有货的，顺便看今天价格",
				Intent: intent.Result{Secondary: intent.PurchaseRecommendation},
			},
			want: []string{"price_query", "inventory_check"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := requestedRecommendationTools(test.input)
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("tools = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRequestedRecommendationToolsRecognizesCommercePhrases(t *testing.T) {
	for _, query := range []string{
		"先推荐P400/H100再查总价和库存",
		"W300和W500满足流量后再比实时价和库存",
		"R10和R20现在哪个便宜",
	} {
		got := requestedRecommendationTools(Request{Query: query})
		found := false
		for _, toolName := range got {
			if toolName == "price_query" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%q tools = %#v, missing price_query", query, got)
		}
	}
}

func TestWarrantyModelMismatchNoteUsesOrderFacts(t *testing.T) {
	note := warrantyModelMismatchNote(
		"T20",
		warrantyToolPayload{
			Items: []model.WarrantyStatus{
				{OrderNo: "CC20260603001", Model: "P400"},
			},
		},
		"[E2]",
	)
	for _, expected := range []string{"T20", "P400", "订单记录为准", "[E2]"} {
		if !strings.Contains(note, expected) {
			t.Fatalf("note missing %q: %s", expected, note)
		}
	}
}

func TestPurchaseOrderNosFallsBackToAvailableRecords(t *testing.T) {
	records := []model.PurchaseRecord{
		{OrderNo: "CC20260603001", Model: "P400"},
		{OrderNo: "CC20250522008", Model: "T20"},
	}
	got := purchaseOrderNos(records, []string{"P500"})
	if strings.Join(got, ",") != "CC20260603001,CC20250522008" {
		t.Fatalf("order nos = %#v", got)
	}
}

func TestPurchaseWarrantyModelMismatchNote(t *testing.T) {
	note := purchaseWarrantyModelMismatchNote("P500", []model.WarrantyStatus{
		{OrderNo: "CC20260603001", Model: "P400"},
	})
	if !strings.Contains(note, "P500") || !strings.Contains(note, "未找到") {
		t.Fatalf("note = %q", note)
	}
}

func TestRefersToPurchaseRecognizesInWarrantyPhrases(t *testing.T) {
	for _, query := range []string{
		"确认在保后我再决定建不建单",
		"这台还在保吗",
		"看看是不是保内",
	} {
		if !refersToPurchase(query) {
			t.Fatalf("%q should refer to purchase context", query)
		}
	}
}

func TestAccessorySkillUsesCompatibilityDocumentsOnly(t *testing.T) {
	got := skillDocTypes(AccessoryCompatibilitySkill)
	if len(got) != 1 || got[0] != "accessory_compatibility" {
		t.Fatalf("doc types = %#v", got)
	}
}

func TestQueryModelsFallsBackToModelsInRawQuery(t *testing.T) {
	got := compactValues(queryModels("两只长毛猫加地毯，t20还是X20 Pro"))
	if strings.Join(got, ",") != "T20,X20 Pro" {
		t.Fatalf("models = %#v", got)
	}
}

func TestComparisonRetrievalPrioritizesScenarioDocuments(t *testing.T) {
	retriever := &routeCapturingRetriever{}
	workflow := newWorkflow(
		ProductComparisonSkill,
		[]intent.Type{intent.ProductComparison},
		retriever,
		nil,
		nil,
		WorkflowConfig{DenseTopK: 8, KeywordTopK: 8, RerankTopK: 4},
	)

	results, err := workflow.retrieveInitial(
		context.Background(),
		Request{
			Query: "两只长毛猫加地毯，T20还是X20 Pro",
			Intent: intent.Result{
				Secondary: intent.ProductComparison,
				Entities:  map[string]string{"category": "robot_vacuum"},
			},
		},
		compactValues(queryModels("两只长毛猫加地毯，T20还是X20 Pro")),
		skillDocTypes(ProductComparisonSkill),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].DocumentID != "product_comparison,purchase_guide" {
		t.Fatalf("first merged result = %#v", results)
	}
	if len(retriever.requests) != 3 {
		t.Fatalf("request count = %d, want 3", len(retriever.requests))
	}
	for _, request := range retriever.requests {
		if len(request.Filter.DocTypes) == 2 &&
			request.Filter.DocTypes[0] == "product_comparison" &&
			len(request.Filter.Models) != 0 {
			t.Fatalf("comparison scenario route should not apply single-model metadata filter: %#v", request.Filter)
		}
		if len(request.Filter.DocTypes) == 2 &&
			request.Filter.DocTypes[0] == "product_comparison" &&
			(len(request.Filter.Categories) != 1 || request.Filter.Categories[0] != "robot_vacuum") {
			t.Fatalf("comparison scenario route should infer category from models: %#v", request.Filter)
		}
		if request.Filter.DocTypes[0] == "product_comparison" && request.RerankTopK != 6 {
			t.Fatalf("scenario rerank top k = %d, want 6", request.RerankTopK)
		}
		if request.Filter.DocTypes[0] == "product_detail" && request.RerankTopK != 3 {
			t.Fatalf("model rerank top k = %d, want 3", request.RerankTopK)
		}
	}
}

func TestRecommendationScenarioRetrievalDoesNotFilterBySingleModel(t *testing.T) {
	retriever := &routeCapturingRetriever{}
	workflow := newWorkflow(
		PurchaseRecommendationSkill,
		[]intent.Type{intent.PurchaseRecommendation},
		retriever,
		nil,
		nil,
		WorkflowConfig{DenseTopK: 8, KeywordTopK: 8, RerankTopK: 4},
	)

	_, err := workflow.retrieveInitial(
		context.Background(),
		Request{
			Query: "R10适合六十平不",
			Intent: intent.Result{
				Secondary: intent.PurchaseRecommendation,
				Entities:  map[string]string{"models": "R10", "category": "robot_vacuum"},
			},
		},
		[]string{"R10"},
		skillDocTypes(PurchaseRecommendationSkill),
	)
	if err != nil {
		t.Fatal(err)
	}

	var guideRequest *rag.SearchRequest
	for index := range retriever.requests {
		request := &retriever.requests[index]
		if strings.Join(request.Filter.DocTypes, ",") == "purchase_guide,product_comparison" {
			guideRequest = request
			break
		}
	}
	if guideRequest == nil {
		t.Fatalf("requests = %#v", retriever.requests)
	}
	if len(guideRequest.Filter.Models) != 0 {
		t.Fatalf("scenario guide request models = %#v, want no single-model filter", guideRequest.Filter.Models)
	}
}

func TestScenarioGuideExpansionCoversCrossCategoryNurseryBundle(t *testing.T) {
	got := scenarioGuideQueryExpansion("婴儿房净化加湿都要")
	for _, expected := range []string{"婴儿房", "过敏", "空气净化"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expansion = %q, missing %q", got, expected)
		}
	}
}

func TestCategoriesForModels(t *testing.T) {
	got := categoriesForModels([]string{"t20", "X20 Pro", "P400"})
	if strings.Join(got, ",") != "robot_vacuum,air_purifier" {
		t.Fatalf("categories = %#v", got)
	}
}

func TestCategoriesForRecommendationQuery(t *testing.T) {
	for query, expected := range map[string]string{
		"猫毛多又不想天天倒尘盒": "robot_vacuum",
		"客厅50平而且有人过敏": "air_purifier",
		"五口之家早晚用水高峰":  "water_purifier",
	} {
		got := categoriesForQuery(query)
		if len(got) != 1 || got[0] != expected {
			t.Fatalf("%q categories = %#v", query, got)
		}
	}
}

func TestRecommendationQueryExpansionAddsHardRequirementTerms(t *testing.T) {
	got := recommendationQueryExpansion("猫毛特别多，不想天天倒尘盒，客厅有地毯")
	for _, expected := range []string{"自动集尘", "防缠绕", "拖布抬升"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expansion missing %q: %s", expected, got)
		}
	}
}

func TestRecommendationCandidateModelsFollowHardConstraints(t *testing.T) {
	tests := []struct {
		query      string
		categories []string
		want       string
	}{
		{"猫毛多又不想天天倒尘盒", []string{"robot_vacuum"}, "X20 Pro,R20"},
		{"120平预算2500，能接受手动倒尘", []string{"robot_vacuum"}, "T20,R10"},
		{"客厅50平而且有人过敏", []string{"air_purifier"}, "P400,P500"},
	}
	for _, test := range tests {
		got := recommendationCandidateModels(test.query, test.categories)
		if strings.Join(got, ",") != test.want {
			t.Fatalf("%q candidates = %#v, want %s", test.query, got, test.want)
		}
	}
}

func TestBuildAccessoryLookupAnswerUsesRetrievedCompatibilityEvidence(t *testing.T) {
	answer := buildAccessoryLookupAnswer(
		"W300",
		[]string{"C300"},
		[]rag.SearchResult{{
			Title:   "W300 配件兼容表",
			Content: "W300 兼容 C300 滤芯。",
		}},
	)
	for _, expected := range []string{"W300", "C300", "[E1]"} {
		if !strings.Contains(answer, expected) {
			t.Fatalf("answer missing %q: %s", expected, answer)
		}
	}
}

func TestAccessoryDynamicRequestDoesNotUseStaticOnlyAnswer(t *testing.T) {
	if !requestsAccessoryDynamicData(Request{
		Query: "P400 滤芯多少钱，还有库存吗",
		Intent: intent.Result{
			SecondaryIntents: []intent.Type{intent.PriceQuery, intent.InventoryQuery},
		},
	}) {
		t.Fatal("price or inventory request should continue through dynamic tools")
	}
}
