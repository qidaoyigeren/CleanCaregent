package eval

import "testing"

func TestDefaultCasesDistribution(t *testing.T) {
	cases := DefaultCases()
	if len(cases) != 200 {
		t.Fatalf("case count = %d, want 200", len(cases))
	}
	groups := map[string]int{}
	difficulties := map[string]int{}
	ids := map[string]struct{}{}
	for _, item := range cases {
		if _, exists := ids[item.CaseID]; exists {
			t.Fatalf("duplicate case id %s", item.CaseID)
		}
		ids[item.CaseID] = struct{}{}
		difficulties[item.Difficulty]++
		for _, tag := range item.Tags {
			if len(tag) > len("eval_group:") && tag[:len("eval_group:")] == "eval_group:" {
				groups[tag]++
			}
		}
		if item.CaseID >= "EVAL-101" && len(item.Tags) < 4 {
			t.Fatalf("%s tags = %d, want at least 4 including eval group", item.CaseID, len(item.Tags))
		}
	}
	expected := map[string]int{
		"eval_group:pure_kb":      100,
		"eval_group:pure_tool":    40,
		"eval_group:kb_tool":      40,
		"eval_group:reject_guide": 20,
	}
	for group, want := range expected {
		if groups[group] != want {
			t.Fatalf("%s count = %d, want %d", group, groups[group], want)
		}
	}
	wantDifficulties := map[string]int{"simple": 80, "medium": 70, "hard": 50}
	for difficulty, want := range wantDifficulties {
		if difficulties[difficulty] != want {
			t.Fatalf("%s difficulty count = %d, want %d", difficulty, difficulties[difficulty], want)
		}
	}
	colloquial := 0
	for _, item := range cases {
		for _, tag := range item.Tags {
			if tag == "口语化" || tag == "省略" || tag == "错别字" || tag == "歧义" {
				colloquial++
				break
			}
		}
	}
	if colloquial < 130 {
		t.Fatalf("colloquial cases = %d, want at least 130", colloquial)
	}
}

func TestCaseConversationTurnsFallsBackToSingleQuery(t *testing.T) {
	item := Case{Query: "T20 吸力多大"}
	turns := item.ConversationTurns()
	if len(turns) != 1 || turns[0] != item.Query {
		t.Fatalf("turns = %#v", turns)
	}
	if item.EvaluationQuery() != item.Query {
		t.Fatalf("evaluation query = %q", item.EvaluationQuery())
	}
}

func TestCaseConversationTurnsUsesFinalTurnForEvaluation(t *testing.T) {
	item := Case{
		Query: "扫地机器人",
		Turns: []string{
			"家庭地面，100平",
			"预算5000",
			"扫地机器人",
		},
	}
	turns := item.ConversationTurns()
	if len(turns) != 3 {
		t.Fatalf("turns = %#v", turns)
	}
	if item.EvaluationQuery() != "扫地机器人" {
		t.Fatalf("evaluation query = %q", item.EvaluationQuery())
	}
}
