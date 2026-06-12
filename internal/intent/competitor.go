package intent

import "strings"

var competitorNames = []string{"小米", "追觅", "石头", "科沃斯", "戴森"}

func annotateCompetitor(query string, result *Result) {
	for _, name := range competitorNames {
		if strings.Contains(query, name) {
			result.Competitors = append(result.Competitors, name)
		}
	}
	if len(result.Competitors) == 0 {
		return
	}
	result.CompetitorMention = true
	switch {
	case containsAny(query, "垃圾", "很差", "不行", "拉踩", "贬低"):
		result.CompetitorPolicy = "refuse_disparagement"
	case containsAny(query, "对比", "比较", "哪个好", "哪个更", "区别"):
		result.CompetitorPolicy = "neutral_comparison"
	default:
		result.CompetitorPolicy = "own_product_only"
	}
}
