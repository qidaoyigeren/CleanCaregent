package intent

import (
	"context"
	"testing"
)

func TestRuleRouterAnnotatesCompetitorPolicy(t *testing.T) {
	result, err := NewRuleRouter().Route(
		context.Background(),
		RouteRequest{Query: "T20 和石头哪个更适合养猫？"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CompetitorMention ||
		len(result.Competitors) != 1 ||
		result.Competitors[0] != "石头" ||
		result.CompetitorPolicy != "neutral_comparison" {
		t.Fatalf("result=%+v", result)
	}
}
