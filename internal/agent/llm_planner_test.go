package agent

import (
	"context"
	"testing"

	"CleanCaregent/internal/intent"
)

func TestLLMPlannerUsesGuardedSkillForKnownComplexIntent(t *testing.T) {
	planner := NewLLMPlanner(nil, nil)
	step, err := planner.NextStep(context.Background(), PlanRequest{
		Query: "T20 充不进电怎么办",
		Intent: intent.Result{
			Secondary: intent.Troubleshooting,
			Entities:  map[string]string{"models": "T20"},
		},
		MaxSteps: 5,
	}, 0, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if step == nil {
		t.Fatal("expected a guarded skill step")
	}
	if step.Action != ActionRunSkill || step.SkillName != "fault_diagnosis" {
		t.Fatalf("step = %#v", step)
	}
	if step.ReasonCode != "guarded_skill_entry" {
		t.Fatalf("reason = %q", step.ReasonCode)
	}
}

func TestLLMPlannerDoesNotForceGuardedSkillAfterFirstStep(t *testing.T) {
	planner := NewLLMPlanner(nil, nil)
	step, err := planner.NextStep(context.Background(), PlanRequest{
		Query:  "T20 充不进电怎么办",
		Intent: intent.Result{Secondary: intent.Troubleshooting},
	}, 1, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if step != nil {
		t.Fatalf("step = %#v, want nil without an LLM after the first step", step)
	}
}
