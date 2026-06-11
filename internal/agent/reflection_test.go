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
			Content: `{"model":"X20 Pro","estimated_final_price":4599,"currency":"CNY"}`,
		}},
	)
	if len(result.UnsupportedClaims) > 0 {
		t.Fatalf("unsupported claims = %v", result.UnsupportedClaims)
	}
}
