package skill

import (
	"strings"
	"testing"
	"time"

	"CleanCaregent/internal/model"
	"CleanCaregent/internal/rag"
)

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
