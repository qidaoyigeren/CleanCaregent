package generator

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

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
	_ string,
	evidence []rag.SearchResult,
	_ string,
	_ string,
	_ string,
) (string, error) {
	return g.Generate(ctx, "", evidence)
}

func (g *Extractive) Generate(ctx context.Context, _ string, evidence []rag.SearchResult) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(evidence) == 0 {
		return "当前知识库中没有找到足够相关的资料。请补充具体型号、品类或问题现象。", nil
	}

	var builder strings.Builder
	builder.WriteString("根据当前知识库检索结果：\n")
	for index, item := range evidence {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		content := compactWhitespace(item.Content)
		content = truncateRunes(content, 320)
		fmt.Fprintf(&builder, "%d. %s [E%d]\n", index+1, content, index+1)
		if g.maxRunes > 0 && utf8.RuneCountInString(builder.String()) >= g.maxRunes {
			break
		}
	}
	builder.WriteString("以上为知识库原文摘要；涉及实时价格、库存、订单或保修状态时，需要调用对应工具确认。")
	return truncateRunes(builder.String(), g.maxRunes), nil
}

func compactWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}
