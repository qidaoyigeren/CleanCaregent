package agent

import (
	"testing"

	"CleanCaregent/internal/intent"
)

func TestShouldUseGroundingOnlyForSimpleGroundedIntent(t *testing.T) {
	if !shouldUseGroundingOnly(intent.ProductParameter, ReflectionResult{
		Answer: "T20 吸力为 6000Pa。[E1]",
	}) {
		t.Fatal("simple grounded parameter answer should skip semantic reflection")
	}
	if shouldUseGroundingOnly(intent.ProductComparison, ReflectionResult{
		Answer: "X20 Pro 更适合养宠家庭。[E1]",
	}) {
		t.Fatal("comparison should retain semantic reflection")
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
