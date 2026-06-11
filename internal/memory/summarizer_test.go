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
