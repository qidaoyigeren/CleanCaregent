package generator

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"CleanCaregent/internal/evidencefmt"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
)

type Extractive struct {
	maxRunes int
}

func NewExtractive(maxRunes int) *Extractive {
	return &Extractive{maxRunes: maxRunes}
}

func (g *Extractive) Name() string {
	return "extractive"
}

func (g *Extractive) GenerateWithScenario(
	ctx context.Context,
	_ prompt.Scenario,
	query string,
	evidence []rag.SearchResult,
	_ string,
	_ string,
	_ string,
) (string, error) {
	return g.Generate(ctx, query, evidence)
}

func (g *Extractive) Generate(ctx context.Context, query string, evidence []rag.SearchResult) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(evidence) == 0 {
		return "当前知识库中没有找到足够相关的资料。请补充具体型号、品类或问题现象。", nil
	}

	var builder strings.Builder
	builder.WriteString("根据当前知识库检索结果：\n")
	perItemLimit := extractiveEvidenceItemLimit(g.maxRunes, len(evidence))
	for index, item := range evidence {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		content := evidencefmt.Compact(item.Content, perItemLimit, query, item.Title)
		fmt.Fprintf(&builder, "%d. %s [E%d]\n", index+1, content, index+1)
		if g.maxRunes > 0 && utf8.RuneCountInString(builder.String()) >= g.maxRunes {
			break
		}
	}
	builder.WriteString("以上为知识库原文摘要；涉及实时价格、库存、订单或保修状态时，需要调用对应工具确认。")
	return truncateRunes(builder.String(), g.maxRunes), nil
}

func truncateRunes(value string, limit int) string {
	return evidencefmt.Compact(value, limit)
}

func extractiveEvidenceItemLimit(answerLimit, evidenceCount int) int {
	if evidenceCount <= 0 {
		return 480
	}
	if answerLimit <= 0 {
		return 480
	}
	available := answerLimit - 120
	if available < 160 {
		return 160
	}
	limit := available / evidenceCount
	if limit < 160 {
		return 160
	}
	if limit > 480 {
		return 480
	}
	return limit
}
