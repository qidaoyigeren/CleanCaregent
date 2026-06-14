package generator

import (
	"context"
	"strings"
	"testing"

	"CleanCaregent/internal/rag"
)

func TestExtractiveIncludesEvidenceReferences(t *testing.T) {
	generator := NewExtractive(500)
	answer, err := generator.Generate(context.Background(), "T20 吸力", []rag.SearchResult{
		{Title: "参数表", Content: "T20 的额定吸力为 6000Pa。"},
		{Title: "FAQ", Content: "地毯模式会自动增压。"},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	for _, expected := range []string{"6000Pa", "[E1]", "[E2]", "实时价格"} {
		if !strings.Contains(answer, expected) {
			t.Fatalf("answer missing %q: %s", expected, answer)
		}
	}
}

func TestExtractivePreservesRelevantTailEvidence(t *testing.T) {
	generator := NewExtractive(500)
	content := "# 产品参数\n" +
		strings.Repeat("T20 常规参数说明。\n", 30) +
		"| 型号 | 吸力 | 场景 |\n|---|---:|---|\n| X20 Pro | 8000Pa | 养宠和地毯 |"
	answer, err := generator.Generate(context.Background(), "X20 Pro 吸力", []rag.SearchResult{
		{Title: "参数表", Content: content},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !strings.Contains(answer, "X20 Pro") || !strings.Contains(answer, "8000Pa") {
		t.Fatalf("relevant tail evidence was lost: %s", answer)
	}
}

func TestExtractiveScenarioPreservesQueryForEvidenceCompression(t *testing.T) {
	generator := NewExtractive(500)
	content := "# 产品参数\n" +
		strings.Repeat("T20 常规参数说明。\n", 30) +
		"| X20 Pro | 8000Pa | 养宠和地毯 |"
	answer, err := generator.GenerateWithScenario(
		context.Background(),
		"",
		"X20 Pro 吸力",
		[]rag.SearchResult{{Title: "参数表", Content: content}},
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("GenerateWithScenario() error = %v", err)
	}
	if !strings.Contains(answer, "X20 Pro") || !strings.Contains(answer, "8000Pa") {
		t.Fatalf("scenario query was not used for compression: %s", answer)
	}
}
