package diagnosis

import "testing"

func TestChargeDiagnosisAdvancesAcrossTurns(t *testing.T) {
	engine := NewDefaultEngine()
	state, decision, err := engine.Start("T20", "扫地机器人充不进电")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Question == "" || state.FaultNodeID != "t20_charge_power" {
		t.Fatalf("state = %#v, decision = %#v", state, decision)
	}

	state, decision, err = engine.Advance(state, "充电座指示灯亮着")
	if err != nil {
		t.Fatal(err)
	}
	if state.FaultNodeID != "t20_charge_contact" || decision.Question == "" {
		t.Fatalf("state = %#v, decision = %#v", state, decision)
	}

	state, decision, err = engine.Advance(state, "清理后还是不能充电")
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Terminal || !decision.NeedHuman {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestDiagnosisStopsOnSafetyRisk(t *testing.T) {
	engine := NewDefaultEngine()
	_, decision, err := engine.Start("T20", "充电时有烧焦味")
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Terminal || decision.SafetyLevel != SafetyHigh {
		t.Fatalf("decision = %#v", decision)
	}
}
