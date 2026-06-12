package eval

import "testing"

func TestDefaultCasesDistribution(t *testing.T) {
	cases := DefaultCases()
	if len(cases) != 100 {
		t.Fatalf("case count = %d, want 100", len(cases))
	}
	paths := map[string]int{}
	ids := map[string]struct{}{}
	for _, item := range cases {
		if _, exists := ids[item.CaseID]; exists {
			t.Fatalf("duplicate case id %s", item.CaseID)
		}
		ids[item.CaseID] = struct{}{}
		if len(item.Tags) > 0 {
			paths[item.Tags[0]]++
		}
	}
	expected := map[string]int{
		"kb_single":            45,
		"kb_multi":             20,
		"kb_tool":              20,
		"diagnosis_multi_turn": 10,
		"reject_clarify":       5,
	}
	for path, want := range expected {
		if paths[path] != want {
			t.Fatalf("%s count = %d, want %d", path, paths[path], want)
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
	if colloquial < 30 {
		t.Fatalf("colloquial cases = %d, want at least 30", colloquial)
	}
}
