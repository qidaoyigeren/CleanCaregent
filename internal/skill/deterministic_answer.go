package skill

import (
	"fmt"
	"strings"

	"CleanCaregent/internal/evidencefmt"
	"CleanCaregent/internal/rag"
)

func buildEvidenceDrivenSkillAnswer(
	skillName string,
	query string,
	models []string,
	searchData []rag.SearchResult,
	dynamicNotes []string,
) (string, bool) {
	if len(searchData) == 0 {
		return "", false
	}
	switch skillName {
	case ProductComparisonSkill, PurchaseRecommendationSkill:
	default:
		return "", false
	}

	var builder strings.Builder
	switch skillName {
	case ProductComparisonSkill:
		builder.WriteString("**对比结论**\n")
		if len(models) > 1 {
			builder.WriteString(fmt.Sprintf("- 当前按 %s 逐项对比；优先采用知识库证据和实时业务数据。\n", strings.Join(models, "、")))
		} else {
			builder.WriteString("- 当前资料更适合先按使用场景、关键参数、价格库存逐项比较。\n")
		}
	case PurchaseRecommendationSkill:
		builder.WriteString("**推荐结论**\n")
		if len(models) > 0 {
			builder.WriteString(fmt.Sprintf("- 当前可优先从 %s 中选择；最终取决于面积、预算、耗材和库存。\n", strings.Join(models, "、")))
		} else {
			builder.WriteString("- 当前可根据使用场景、预算、耗材和库存先缩小候选范围。\n")
		}
	}

	if len(dynamicNotes) > 0 {
		builder.WriteString("\n**实时数据**\n")
		for _, note := range firstNonEmpty(dynamicNotes, 4) {
			builder.WriteString("- ")
			builder.WriteString(strings.TrimSpace(note))
			builder.WriteString("\n")
		}
	}

	builder.WriteString("\n**知识依据**\n")
	for _, item := range topEvidenceSnippets(query, searchData, 4) {
		builder.WriteString("- ")
		if item.title != "" {
			builder.WriteString(item.title)
			builder.WriteString("：")
		}
		builder.WriteString(item.content)
		builder.WriteString(" ")
		builder.WriteString(item.citation)
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String()), true
}

type evidenceSnippet struct {
	title    string
	content  string
	citation string
}

func topEvidenceSnippets(query string, searchData []rag.SearchResult, limit int) []evidenceSnippet {
	if limit <= 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]evidenceSnippet, 0, limit)
	for index, item := range searchData {
		content := evidencefmt.Compact(item.Content, 160, query, item.Title)
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		key := item.DocumentID + "\n" + content
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, evidenceSnippet{
			title:    strings.TrimSpace(item.Title),
			content:  content,
			citation: evidenceCitation(index + 1),
		})
		if len(result) >= limit {
			break
		}
	}
	return result
}

func firstNonEmpty(values []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	result := make([]string, 0, limit)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
		if len(result) >= limit {
			break
		}
	}
	return result
}
