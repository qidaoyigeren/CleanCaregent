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

func TestFilterCasesBySplit(t *testing.T) {
	cases := []Case{
		{CaseID: "EVAL-001"},
		{CaseID: "EVAL-201", Tags: []string{"split:tuning"}},
		{CaseID: "EVAL-276", Tags: []string{"split:holdout"}},
	}
	regression, err := filterCasesBySplit(cases, "regression")
	if err != nil {
		t.Fatal(err)
	}
	if len(regression) != 1 || regression[0].CaseID != "EVAL-001" {
		t.Fatalf("regression = %#v", regression)
	}
	tuning, err := filterCasesBySplit(cases, "tuning")
	if err != nil {
		t.Fatal(err)
	}
	if len(tuning) != 1 || tuning[0].CaseID != "EVAL-201" {
		t.Fatalf("tuning = %#v", tuning)
	}
}

func TestFilterCasesBySplitRejectsUnknownSplit(t *testing.T) {
	if _, err := filterCasesBySplit([]Case{{CaseID: "EVAL-001"}}, "shadow"); err == nil {
		t.Fatal("expected unsupported split error")
	}
}
