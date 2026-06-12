package rag

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStructureAwareChunkerPreservesTableHeader(t *testing.T) {
	chunker := NewStructureAwareChunker(100, 10)
	content := `# 核心参数
| 型号 | 吸力 | 适用面积 |
|---|---:|---:|
| T20 | 6000Pa | 80-120㎡ |
| X20 Pro | 8000Pa | 100-150㎡ |
| S10 | 5000Pa | 60-100㎡ |
| A1 | 4000Pa | 50-80㎡ |`

	chunks := chunker.Split("product_parameter", "扫地机器人参数", content)
	if len(chunks) < 2 {
		t.Fatalf("chunk count = %d, want at least 2", len(chunks))
	}
	for _, chunk := range chunks {
		if !strings.Contains(chunk.Content, "| 型号 | 吸力 |") ||
			!strings.Contains(chunk.Content, "|---|---:|---:|") {
			t.Fatalf("table header missing from chunk: %s", chunk.Content)
		}
	}
}

func TestStructureAwareChunkerSplitsFAQPairs(t *testing.T) {
	chunker := NewStructureAwareChunker(500, 20)
	chunks := chunker.Split("faq", "常见问题", `
Q: T20 如何重新联网？
A: 长按联网键进入配网状态。

Q: P400 多久更换滤芯？
A: 结合滤芯寿命提示判断。`)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v", chunks)
	}
	if !strings.Contains(chunks[0].Content, "T20") ||
		!strings.Contains(chunks[1].Content, "P400") {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestStructureAwareChunkerSplitsFaultNodesAndPolicyClauses(t *testing.T) {
	chunker := NewStructureAwareChunker(500, 20)
	faultChunks := chunker.Split("troubleshooting", "充电故障树", `
node_id: charge_power
question: 充电座指示灯是否亮？
yes_next: charge_contact

node_id: charge_contact
question: 清洁触点后能否充电？
no_next: service`)
	if len(faultChunks) != 2 {
		t.Fatalf("fault chunks = %#v", faultChunks)
	}

	policyChunks := chunker.Split("after_sales_policy", "退换货政策", `
第一条 适用范围
签收后七天内且商品完好。

第二条 质量问题
经检测确认后按质量问题流程处理。`)
	if len(policyChunks) != 2 {
		t.Fatalf("policy chunks = %#v", policyChunks)
	}
}

func TestStructureAwareChunkerUsesHeadingsAndSizeLimit(t *testing.T) {
	chunker := NewStructureAwareChunker(120, 20)
	content := "# 充电故障\n" + strings.Repeat("检查底座供电。", 40) + "\n## 触点检查\n清洁充电触点。"

	chunks := chunker.Split("troubleshooting", "T20 故障手册", content)
	if len(chunks) < 3 {
		t.Fatalf("chunk count = %d", len(chunks))
	}
	for _, chunk := range chunks {
		if utf8.RuneCountInString(chunk.Content) > 120 {
			t.Fatalf("chunk exceeds limit: %d", utf8.RuneCountInString(chunk.Content))
		}
		if !strings.HasPrefix(chunk.SectionPath, "T20 故障手册") {
			t.Fatalf("section path = %q", chunk.SectionPath)
		}
	}
}

func TestStructureAwareChunkerUsesDocumentProfiles(t *testing.T) {
	chunker := NewProfiledStructureAwareChunker(500, 50, map[string]ChunkProfile{
		"faq": {MaxRunes: 80, Overlap: 0},
	})
	content := "Q: 问题\nA: " + strings.Repeat("这是一个完整回答。", 20)
	chunks := chunker.Split("faq", "常见问题", content)
	if len(chunks) < 2 {
		t.Fatalf("chunk count = %d, want profile to split content", len(chunks))
	}
	for _, chunk := range chunks {
		if utf8.RuneCountInString(chunk.Content) > 80 {
			t.Fatalf("profile size was not applied: %d", utf8.RuneCountInString(chunk.Content))
		}
	}
}
