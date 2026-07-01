package agent

import (
	"fmt"
	"strings"

	"CleanCaregent/internal/evidencefmt"
	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/rag"
)

func deterministicGroundedAnswer(
	intentType intent.Type,
	query string,
	searchResults []rag.SearchResult,
) (string, bool) {
	if len(searchResults) == 0 {
		return "", false
	}
	var title string
	switch intentType {
	case intent.UsageInstruction:
		title = "使用说明"
	case intent.ProductParameter:
		title = "参数信息"
	default:
		return "", false
	}

	var builder strings.Builder
	builder.WriteString("**")
	builder.WriteString(title)
	builder.WriteString("**\n")
	builder.WriteString("- 根据当前知识库证据，建议按下面信息处理。\n")
	builder.WriteString("\n**依据**\n")
	count := 0
	seen := map[string]struct{}{}
	for index, item := range searchResults {
		content := strings.TrimSpace(evidencefmt.Compact(item.Content, 180, query, item.Title))
		if content == "" {
			continue
		}
		key := item.DocumentID + "\n" + content
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		builder.WriteString("- ")
		if strings.TrimSpace(item.Title) != "" {
			builder.WriteString(strings.TrimSpace(item.Title))
			builder.WriteString("：")
		}
		builder.WriteString(content)
		builder.WriteString(fmt.Sprintf(" [E%d]\n", index+1))
		count++
		if count >= 4 {
			break
		}
	}
	for _, hint := range deterministicAnswerHints(query, searchResults) {
		builder.WriteString("- ")
		builder.WriteString(hint)
		builder.WriteString("\n")
	}
	if count == 0 {
		return "", false
	}
	return strings.TrimSpace(builder.String()), true
}

func deterministicAnswerHints(query string, searchResults []rag.SearchResult) []string {
	lower := strings.ToLower(query)
	var hints []string
	for index, item := range searchResults {
		content := strings.ToLower(item.Content)
		citation := fmt.Sprintf("[E%d]", index+1)
		if strings.Contains(query, "水洗") && strings.Contains(content, `"washable":true`) {
			hints = append(hints, "结构化参数显示 washable=true，可理解为对应清洁件支持水洗。 "+citation)
		}
		if deterministicContainsAnyText(lower, "第一次", "多按", "按几次", "压力", "排出") &&
			(strings.Contains(item.Content, "按压 5") || strings.Contains(item.Content, "按压5") || strings.Contains(item.Content, "吸液压力")) {
			hints = append(hints, "新瓶首次多按几次是为了排出空气并建立吸液压力。 "+citation)
		}
		if strings.Contains(query, "水痕") &&
			(strings.Contains(item.Content, "干布收水") || strings.Contains(item.Content, "单独清洗")) {
			hints = append(hints, "减少水痕可先湿擦去污，再用干布收水；首次使用前建议单独清洗并晾干。 "+citation)
		}
		if strings.Contains(query, "柔顺剂") {
			hints = append(hints, "MC6 不建议与柔顺剂一起洗，柔顺剂会降低纤维吸水和抓尘能力。 "+citation)
		}
	}
	return compactDeterministicHints(hints)
}

func deterministicContainsAnyText(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func compactDeterministicHints(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
