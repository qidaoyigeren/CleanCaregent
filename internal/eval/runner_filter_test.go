package eval

import "testing"

func TestSelectCasesKeepsDatasetOrder(t *testing.T) {
	cases := []Case{
		{CaseID: "EVAL-001"},
		{CaseID: "EVAL-002"},
		{CaseID: "EVAL-003"},
	}
	selected, err := selectCases(cases, []string{"EVAL-003", "EVAL-001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 ||
		selected[0].CaseID != "EVAL-001" ||
		selected[1].CaseID != "EVAL-003" {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestSelectCasesRejectsUnknownIDs(t *testing.T) {
	_, err := selectCases(
		[]Case{{CaseID: "EVAL-001"}},
		[]string{"EVAL-999"},
	)
	if err == nil {
		t.Fatal("expected an unknown case id error")
	}
}
