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
