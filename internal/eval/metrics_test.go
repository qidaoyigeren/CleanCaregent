package eval

import "testing"

func TestPrecisionAtKUsesOnlyFirstFiveUniqueDocuments(t *testing.T) {
	got := precisionAtK(
		[]string{"kb_compat_p400_f400"},
		[]string{
			"kb_compat_p400_f400",
			"kb_detail_p400",
			"kb_params_p400",
			"kb_faq_p400_filter",
			"kb_manual_p400",
			"kb_guide_air_area",
		},
		5,
	)
	if got != 0.2 {
		t.Fatalf("precision@5 = %v, want 0.2", got)
	}
}

func TestClarificationDetectionRequiresClarificationFirst(t *testing.T) {
	if !isClarification("请补充具体型号，我才能继续查询。") {
		t.Fatal("leading clarification should be detected")
	}
	if isClarification("W300 适合三口之家。[E1]\n\n如需查询扫地机器人，请提供型号。") {
		t.Fatal("optional follow-up after a substantive answer is not a clarification")
	}
	if !isClarification("您好！为了帮您查询准确价格，我需要先确认产品型号。请问您要查哪款？") {
		t.Fatal("polite leading clarification should be detected")
	}
}

func TestRefusalDetectionAcceptsDomainBoundaryWording(t *testing.T) {
	for _, answer := range []string{
		"抱歉，我无法回答您的问题。您询问的是手机。",
		"很抱歉，我目前只支持扫地机器人、空气净化器、净水器和加湿器相关问题。",
	} {
		if !isRefusal(answer) {
			t.Fatalf("expected refusal: %s", answer)
		}
	}
}

func TestToolSelectionAllowsRequiredBusinessPrerequisites(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		actual   []string
		want     bool
	}{
		{
			name:     "ticket prerequisites",
			expected: []string{"create_after_sales_ticket"},
			actual:   []string{"order_lookup", "warranty_check", "create_after_sales_ticket"},
			want:     true,
		},
		{
			name:     "warranty order lookup",
			expected: []string{"warranty_check"},
			actual:   []string{"order_lookup", "warranty_check"},
			want:     true,
		},
		{
			name:     "unrelated extra tool",
			expected: []string{"price_query"},
			actual:   []string{"price_query", "inventory_check"},
			want:     false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := toolSelectionCorrect(normalizeSet(test.expected), normalizeSet(test.actual))
			if got != test.want {
				t.Fatalf("tool selection = %v, want %v", got, test.want)
			}
		})
	}
}

func TestToolGroundingChecksAnswerFactsAgainstToolResult(t *testing.T) {
	output := AgentOutput{
		Answer:              "T20 当前价 3599.00 元，优惠后预估 3499.00 元，库存 18 件。",
		SuccessfulToolCalls: 1,
		ToolResults: []any{
			map[string]any{
				"tool_name":                   "price_query",
				"model":                       "T20",
				"current_price_cents":         int64(359900),
				"estimated_final_price_cents": int64(349900),
				"available_stock":             18,
			},
		},
	}
	if got := toolGroundingRate(output); got != 1 {
		t.Fatalf("grounding rate = %v, want 1", got)
	}

	output.Answer = "T20 当前价 9999.00 元。"
	if got := toolGroundingRate(output); got >= 1 {
		t.Fatalf("unsupported price should reduce grounding, got %v", got)
	}
}

func TestComparisonDocumentAcceptsBothProductEvidenceSets(t *testing.T) {
	expected := []string{"kb_compare_t20_x20pro"}
	actual := []string{
		"kb_detail_t20_standard_cn",
		"kb_params_x20_pro",
		"kb_detail_x20_pro_pet_bundle",
		"kb_params_t20",
		"kb_guide_pet_home",
	}
	if got := hitAtK(expected, actual, 5); got != 1 {
		t.Fatalf("hit@5 = %v", got)
	}
	if got := setRecall(normalizeSet(expected), normalizeSet(actual)); got != 1 {
		t.Fatalf("recall = %v", got)
	}
	if got := precisionAtK(expected, actual, 5); got != 0.2 {
		t.Fatalf("precision@5 = %v", got)
	}
	if got := reciprocalRank(expected, actual); got != 0.5 {
		t.Fatalf("mrr = %v, want 0.5", got)
	}
}

func TestComparisonDocumentRequiresEvidenceForBothModels(t *testing.T) {
	actual := []string{"kb_detail_t20_standard_cn", "kb_params_t20"}
	if expectedDocumentSatisfied("kb_compare_t20_x20pro", actual) {
		t.Fatal("one-sided product evidence must not satisfy a comparison document")
	}
}

func TestParameterTableAcceptsSameModelProductDetail(t *testing.T) {
	actual := []string{"kb_detail_x20_pro_standard_cn"}
	if !expectedDocumentSatisfied("kb_params_x20_pro", actual) {
		t.Fatal("same-model product detail should satisfy a parameter-table expectation")
	}
	if expectedDocumentSatisfied("kb_params_x20_pro", []string{"kb_detail_t20"}) {
		t.Fatal("a different model must not satisfy the parameter expectation")
	}
	if got := reciprocalRank([]string{"kb_params_x20_pro"}, actual); got != 1 {
		t.Fatalf("mrr = %v", got)
	}
}

func TestCanonicalGuideAcceptsMoreSpecificScenarioGuide(t *testing.T) {
	tests := map[string]string{
		"kb_guide_pet_home":   "kb_guide_pet_large_family",
		"kb_guide_air_area":   "kb_guide_allergy_air",
		"kb_guide_large_home": "kb_guide_pet_large_family",
	}
	for expected, actual := range tests {
		if !expectedDocumentSatisfied(expected, []string{actual}) {
			t.Fatalf("%s should accept %s", expected, actual)
		}
	}
}

func TestGroundingNormalizesUnitsAndWhitespace(t *testing.T) {
	output := AgentOutput{
		Answer:      "W300 为 400GPD、1.05L/min，适合 3-4 人；P400 覆盖 55m²，CADR 有证据支持。",
		EvidenceIDs: []string{"E1"},
		Contexts: []string{
			"W300 参数：400G，1.05 L/min，适合 3-4 人。",
			"P400 建议面积 35-55㎡。",
		},
	}
	if got := answerGroundingRate(Case{
		Intent:            "product_comparison",
		ExpectedDocuments: []string{"kb_compare_w300_w500"},
	}, output); got != 1 {
		t.Fatalf("grounding = %v", got)
	}
}

func TestGroundingSeparatesEnglishComparisonMarker(t *testing.T) {
	output := AgentOutput{
		Answer:      "P400 推荐 55㎡ [E1] vs P500 推荐 75㎡ [E2]。",
		EvidenceIDs: []string{"E1", "E2"},
		Contexts: []string{
			"P400 建议面积 35-55㎡。",
			"P500 建议面积 50-75㎡。",
		},
	}
	if got := answerGroundingRate(Case{
		Intent:            "product_comparison",
		ExpectedDocuments: []string{"kb_compare_p400_p500"},
	}, output); got != 1 {
		t.Fatalf("grounding = %v", got)
	}
}

func TestGroundingMapsStructuredProductFieldsToDisplayUnits(t *testing.T) {
	output := AgentOutput{
		Answer:      "X20 Pro 为 8000Pa；W300 为 400GPD（1.05L/min），W500 为 600GPD（1.58L/min）。",
		EvidenceIDs: []string{"E1", "E2"},
		Contexts: []string{
			`{"model":"X20 Pro","attributes":{"suction_pa":8000}}`,
			`{"model":"W300","attributes":{"capacity_gpd":400,"flow_lpm":1.05}}`,
			`{"model":"W500","attributes":{"capacity_gpd":600,"flow_lpm":1.58}}`,
		},
	}
	if got := answerGroundingRate(Case{
		Intent:            "product_comparison",
		ExpectedDocuments: []string{"kb_compare_w300_w500"},
	}, output); got != 1 {
		t.Fatalf("grounding = %v", got)
	}
}
