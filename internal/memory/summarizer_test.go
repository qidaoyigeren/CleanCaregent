package memory

import (
	"context"
	"strings"
	"testing"

	"CleanCaregent/internal/model"
)

func TestExtractiveSummarizerKeepsPreviousAndMessages(t *testing.T) {
	summarizer := NewExtractiveSummarizer(200)
	summary, err := summarizer.Summarize(
		context.Background(),
		"用户关注养猫家庭",
		[]model.Message{
			{Role: "user", Content: "我家 120 平，有地毯，预算 5000"},
			{Role: "assistant", Content: "已记录这些约束"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"养猫", "120 平", "预算 5000"} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("summary = %q, missing %q", summary, expected)
		}
	}
}

func TestSummaryPreservesProductOrderAndDiagnosisState(t *testing.T) {
	messages := []model.Message{
		{Role: "user", Content: "订单CC20260603001里的T20充不进电"},
		{Role: "user", Content: "充电座指示灯不亮，已换插座，仍然没有焦味"},
	}
	good := "用户订单CC20260603001中的T20充不进电；充电座指示灯不亮，已换插座，无焦味。"
	if !summaryPreservesEntities("", messages, good) {
		t.Fatal("complete summary was rejected")
	}
	if summaryPreservesEntities("", messages, "用户的设备存在充电问题。") {
		t.Fatal("summary missing key entities was accepted")
	}
}

func TestEnforceSummaryLimit(t *testing.T) {
	value := enforceSummaryLimit(strings.Repeat("旧", 20)+"T20充不进电", 10)
	if len([]rune(value)) != 10 || !strings.Contains(value, "T20") {
		t.Fatalf("limited summary = %q", value)
	}
}
