package agent

import (
	"testing"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/memory"
)

func TestContinueDiagnosisRouteUsesActiveStateForShortReply(t *testing.T) {
	route := continueDiagnosisRoute(intent.Result{
		Primary:     "fallback",
		Secondary:   intent.Clarification,
		Confidence:  0.55,
		NeedClarify: true,
		Entities:    map[string]string{},
	}, Request{
		Query: "充电座指示灯亮着",
		Context: memory.ConversationContext{
			DiagnosisState: &memory.DiagnosisState{
				ProductModel: "T20",
				FaultNodeID:  "t20_charge_power",
			},
		},
	})

	if route.Secondary != intent.Troubleshooting || route.NeedClarify {
		t.Fatalf("route = %#v", route)
	}
	if route.Entities["models"] != "T20" {
		t.Fatalf("models = %q", route.Entities["models"])
	}
}

func TestContinueDiagnosisRouteAllowsExplicitTopicSwitch(t *testing.T) {
	original := intent.Result{
		Primary:    "presales",
		Secondary:  intent.PriceQuery,
		Confidence: 0.96,
	}
	route := continueDiagnosisRoute(original, Request{
		Query: "T20 现在多少钱",
		Context: memory.ConversationContext{
			DiagnosisState: &memory.DiagnosisState{
				ProductModel: "T20",
				FaultNodeID:  "t20_charge_power",
			},
		},
	})
	if route.Secondary != intent.PriceQuery {
		t.Fatalf("route = %#v", route)
	}
}
