package reranker

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"CleanCaregent/internal/rag"
)

type LocalLexical struct{}

func NewLocalLexical() *LocalLexical {
	return &LocalLexical{}
}

func (r *LocalLexical) Rerank(
	ctx context.Context,
	query string,
	documents []rag.SearchResult,
	topK int,
) ([]rag.SearchResult, error) {
	queryTokens := tokenSet(query)
	results := append([]rag.SearchResult(nil), documents...)
	for index := range results {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		documentTokens := tokenSet(results[index].Title + "\n" + results[index].Content)
		overlap := 0
		for token := range queryTokens {
			if _, ok := documentTokens[token]; ok {
				overlap++
			}
		}
		lexical := 0.0
		if len(queryTokens) > 0 {
			lexical = float64(overlap) / float64(len(queryTokens))
		}
		results[index].RerankScore = lexical*0.7 + results[index].FusionScore*0.3
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].RerankScore > results[j].RerankScore
	})
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func tokenSet(value string) map[string]struct{} {
	value = strings.ToLower(value)
	runes := []rune(value)
	result := make(map[string]struct{}, len(runes))
	var word []rune
	flush := func() {
		if len(word) > 0 {
			result[string(word)] = struct{}{}
			word = word[:0]
		}
	}
	var han []rune
	for _, current := range runes {
		if unicode.IsLetter(current) || unicode.IsNumber(current) {
			word = append(word, current)
			if unicode.Is(unicode.Han, current) {
				han = append(han, current)
			}
			continue
		}
		flush()
	}
	flush()
	for index := 0; index+2 <= len(han); index++ {
		result[string(han[index:index+2])] = struct{}{}
	}
	return result
}
