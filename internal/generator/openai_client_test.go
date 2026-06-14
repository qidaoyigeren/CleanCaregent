package generator

import (
	"strings"
	"testing"
	"unicode/utf8"

	"CleanCaregent/internal/rag"
)

func TestBuildEvidenceContextUsesDynamicStructuredCompression(t *testing.T) {
	content := "# 扫地机器人参数\n" +
		strings.Repeat("| T20 | 6000Pa | 常规清洁 |\n", 60) +
		"| X20 Pro | 8000Pa | 养宠和地毯 |"
	context := buildEvidenceContextForQuery([]rag.SearchResult{{
		DocumentID: "kb_params",
		Title:      "产品参数表",
		Content:    content,
	}}, "X20 Pro 吸力")

	if !strings.Contains(context, "X20 Pro") || !strings.Contains(context, "8000Pa") {
		t.Fatalf("focused evidence row was lost: %s", context)
	}
	if utf8.RuneCountInString(context) > 1400 {
		t.Fatalf("evidence context unexpectedly large: %d", utf8.RuneCountInString(context))
	}
}
