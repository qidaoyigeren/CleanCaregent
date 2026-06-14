package agent

import (
	"strings"
	"testing"

	"CleanCaregent/internal/intent"
)

func TestShouldUseGroundingOnlyForSimpleGroundedIntent(t *testing.T) {
	if !shouldUseGroundingOnly(intent.ProductParameter, ReflectionResult{
		Answer: "T20 吸力为 6000Pa。[E1]",
	}) {
		t.Fatal("simple grounded parameter answer should skip semantic reflection")
	}
	if !shouldUseGroundingOnly(intent.ProductComparison, ReflectionResult{
		Answer: "X20 Pro 更适合养宠家庭。[E1]",
	}) {
		t.Fatal("grounded comparison should skip destructive semantic reflection")
	}
	if !shouldUseGroundingOnly(intent.AccessoryCompatibility, ReflectionResult{
		Answer: "F400 与 P400 兼容。[E1]",
	}) {
		t.Fatal("structured compatibility answer should skip destructive semantic reflection")
	}
	if !shouldUseGroundingOnly(intent.UsageInstruction, ReflectionResult{
		Answer: "首次开机前先拆除滤芯塑封。[E1]",
	}) {
		t.Fatal("grounded usage instructions should skip semantic reflection")
	}
	if !shouldUseGroundingOnly(intent.WarrantyQuery, ReflectionResult{
		Answer: "该订单仍在保修期内。[E1][E2]",
	}) {
		t.Fatal("grounded deterministic warranty answers should skip semantic reflection")
	}
	if shouldUseGroundingOnly(intent.ProductParameter, ReflectionResult{
		Answer:        "unknown",
		LowConfidence: true,
	}) {
		t.Fatal("low-confidence answer should retain semantic reflection")
	}
}

func TestExtractClaimsSplitsDeclarativeSentences(t *testing.T) {
	claims := extractClaims("T20 额定吸力为 6000Pa。[E1]\n它适合 80-120 平米家庭。[E2]\n需要我继续查价格吗？")
	if len(claims) != 2 {
		t.Fatalf("claims = %#v", claims)
	}
}

func TestReflectionEvidencePreservesAnswerRelatedPolicyException(t *testing.T) {
	content := "# 售后政策\n" +
		strings.Repeat("商品应保持包装及配件完整。\n", 40) +
		"例外：滤芯拆封使用后，不适用七天无理由退货。"
	context := buildEvidenceContextForReflectionWithFocus(
		[]Evidence{{ID: "E1", Kind: "kb_chunk", Title: "退货政策", Content: content}},
		"滤芯拆封后能退吗",
		"滤芯拆封后可能不支持无理由退货。[E1]",
	)
	if !strings.Contains(context, "例外") || !strings.Contains(context, "不适用七天无理由退货") {
		t.Fatalf("reflection evidence lost policy exception: %s", context)
	}
}
