package eval

import "testing"

func TestEfficiencyIsDiagnosticNotPassGate(t *testing.T) {
	if !nonBlockingMetric("efficiency_score") {
		t.Fatal("efficiency_score should remain visible without failing functional correctness")
	}
	if nonBlockingMetric("answer_correctness") {
		t.Fatal("answer_correctness must remain a pass gate")
	}
}
