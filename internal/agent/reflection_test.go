package agent

import (
	"testing"

	"CleanCaregent/internal/intent"
)

func TestGroundingReflectorRejectsUnsupportedNumber(t *testing.T) {
	reflector := NewGroundingReflector()
	result := reflector.Review(
		"T20 吸力多大",
		intent.ProductParameter,
		"T20 吸力为 9000Pa。",
		[]Evidence{{ID: "E1", Kind: "kb_chunk", Content: "T20 额定吸力为 6000Pa。"}},
	)
	if !result.LowConfidence || len(result.UnsupportedClaims) == 0 {
		t.Fatalf("result = %#v", result)
	}
	if result.Action != "remove_unsupported" {
		t.Fatalf("action = %q", result.Action)
	}
	if result.Answer != "T20 吸力为 9000Pa。" {
		t.Fatalf("low-risk answer should be preserved for local repair: %q", result.Answer)
	}
}

func TestGroundingReflectorAcceptsToolBackedWarranty(t *testing.T) {
	reflector := NewGroundingReflector()
	result := reflector.Review(
		"订单是否在保修期",
		intent.WarrantyQuery,
		`warranty_check: {"in_warranty":true}`,
		[]Evidence{{ID: "E1", Kind: "tool_result", Content: `{"in_warranty":true}`}},
	)
	if result.LowConfidence {
		t.Fatalf("result = %#v", result)
	}
}

func TestGroundingReflectorTrustsValidatedTicketToolOverWeakKnowledgeScores(t *testing.T) {
	reflector := NewGroundingReflector()
	result := reflector.Review(
		"我确认创建售后工单",
		intent.CreateAfterSalesTicket,
		"已按您的明确确认创建售后工单，工单号 AS_TEST_001，本次提交使用幂等键防重复。",
		[]Evidence{
			{
				ID:      "E1",
				Kind:    "kb_chunk",
				Content: "售后政策",
				Metadata: map[string]any{
					"rerank_score": 0.01,
				},
			},
			{
				ID:      "E2",
				Kind:    "tool_result",
				Content: `{"ticket_no":"AS_TEST_001","status":"created"}`,
				Metadata: map[string]any{
					"tool_name": "create_after_sales_ticket",
				},
			},
		},
	)
	if result.LowConfidence || result.Action == "rerun_retrieval" || result.ShouldTransfer {
		t.Fatalf("result = %#v", result)
	}
}

func TestGroundingReflectorRequiresToolForDynamicIntent(t *testing.T) {
	reflector := NewGroundingReflector()
	result := reflector.Review(
		"T20 多少钱",
		intent.PriceQuery,
		"请以商品页为准。",
		[]Evidence{{ID: "E1", Kind: "kb_chunk", Content: "T20 商品介绍"}},
	)
	if !result.LowConfidence {
		t.Fatalf("result = %#v", result)
	}
}

func TestGroundingReflectorNormalizesWhitespaceAndAreaUnits(t *testing.T) {
	reflector := NewGroundingReflector()
	result := reflector.Review(
		"T20适合多大面积",
		intent.ProductParameter,
		"T20 适合 80-120平米。[E1]",
		[]Evidence{{
			ID:      "E1",
			Title:   "T20参数",
			Content: "建议面积为 80-120 ㎡。",
		}},
	)
	if len(result.UnsupportedClaims) > 0 {
		t.Fatalf("unsupported claims = %v", result.UnsupportedClaims)
	}
	if result.LowConfidence {
		t.Fatalf("result = %#v", result)
	}
}

func TestGroundingReflectorAcceptsUserBudgetAndToolPrice(t *testing.T) {
	reflector := NewGroundingReflector()
	result := reflector.Review(
		"预算5000元，推荐扫地机器人",
		intent.PurchaseRecommendation,
		"X20 Pro 券后 4599元，在您的 5000元预算内。[E1]",
		[]Evidence{{
			ID:      "E1",
			Kind:    "tool_result",
			Content: `{"model":"X20 Pro","estimated_final_price_cents":459900,"currency":"CNY"}`,
		}},
	)
	if len(result.UnsupportedClaims) > 0 {
		t.Fatalf("unsupported claims = %v", result.UnsupportedClaims)
	}
}

func TestGroundingReflectorAcceptsThousandsPriceAndWarrantyMonths(t *testing.T) {
	reflector := NewGroundingReflector()
	result := reflector.Review(
		"查订单状态",
		intent.OrderQuery,
		"订单商品价格为 2,099.00元，整机保修 12个月。[E1]",
		[]Evidence{{
			ID:   "E1",
			Kind: "tool_result",
			Content: `{
				"unit_price_cents": 209900,
				"warranty_months": 12
			}`,
		}},
	)
	if len(result.UnsupportedClaims) > 0 {
		t.Fatalf("unsupported claims = %v", result.UnsupportedClaims)
	}
}

func TestGroundingReflectorAcceptsUserAreaAndBudgetWithoutRepeatedUnits(t *testing.T) {
	reflector := NewGroundingReflector()
	result := reflector.Review(
		"120平两只猫预算5000，推荐扫地机器人",
		intent.PurchaseRecommendation,
		"按 120㎡ 和 5000元预算筛选。[E1]",
		[]Evidence{{
			ID:      "E1",
			Kind:    "kb_chunk",
			Content: "养宠家庭应关注防缠绕、续航和地毯策略。",
		}},
	)
	if len(result.UnsupportedClaims) > 0 {
		t.Fatalf("unsupported claims = %v", result.UnsupportedClaims)
	}
}

func TestGroundingReflectorRerunsWhenTopEvidenceIsIrrelevant(t *testing.T) {
	reflector := NewGroundingReflector()
	result := reflector.Review(
		"T20 适合养猫吗",
		intent.ProductComparison,
		"T20 可用于日常清扫。[E1]",
		[]Evidence{
			{ID: "E1", Kind: "kb_chunk", Content: "无关内容", Metadata: map[string]any{"rerank_score": 0.12}},
			{ID: "E2", Kind: "kb_chunk", Content: "无关内容", Metadata: map[string]any{"rerank_score": 0.18}},
			{ID: "E3", Kind: "kb_chunk", Content: "无关内容", Metadata: map[string]any{"rerank_score": 0.29}},
		},
	)
	if result.Action != "rerun_retrieval" || !result.LowConfidence {
		t.Fatalf("result = %#v", result)
	}
}

func TestGroundingReflectorUsesRemoteRerankerScoreCalibration(t *testing.T) {
	reflector := NewGroundingReflector()
	result := reflector.Review(
		"猫毛多怎么选",
		intent.PurchaseRecommendation,
		"优先考虑防缠绕和自动集尘。[E1]",
		[]Evidence{{
			ID: "E1", Kind: "kb_chunk", Content: "养宠家庭选购指南",
			Metadata: map[string]any{
				"rerank_score":    0.15,
				"rerank_provider": "BAAI/bge-reranker-v2-m3",
			},
		}},
	)
	if result.LowConfidence || result.Action == "rerun_retrieval" {
		t.Fatalf("result = %#v", result)
	}
}
