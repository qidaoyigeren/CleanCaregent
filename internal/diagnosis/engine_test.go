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

func TestChargeDiagnosisUsesObservedStepsInInitialQuery(t *testing.T) {
	engine := NewDefaultEngine()
	state, decision, err := engine.Start("T20", "T20充电座灯亮，触点也擦了，还是充不进")
	if err != nil {
		t.Fatal(err)
	}
	if state.FaultNodeID != "t20_charge_service" || !decision.Terminal || !decision.NeedHuman {
		t.Fatalf("state = %#v, decision = %#v", state, decision)
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

func TestDiagnosisStartsAdditionalFaultFamilies(t *testing.T) {
	engine := NewDefaultEngine()
	tests := []struct {
		model     string
		query     string
		wantDoc   string
		terminal  bool
		needHuman bool
	}{
		{"P400", "滤芯重装过，P400还是响，接着怎么查", "kb_fault_p400_noise", true, true},
		{"P500", "P500显示的PM2.5一直是0，屋里明明有烟", "kb_fault_p500_sensor", false, false},
		{"W300", "阀门关了也断电了，W300接口还是滴水", "kb_fault_w300_leak", true, true},
		{"T20", "T20连不上家里网，路由器只有双频合一咋办", "kb_fault_t20_wifi", false, false},
		{"X20 Pro", "X20 Pro配网一直失败，账号地区和权限我该按啥顺序查", "kb_fault_x20_app_pair", false, false},
		{"R20", "R20清完毛还是金属摩擦声，能继续跑一圈试试吗", "kb_fault_r20_noise", true, true},
		{"H200", "H200续航变短，按档位水箱排查", "kb_fault_h200_runtime", false, false},
	}
	for _, test := range tests {
		_, decision, err := engine.Start(test.model, test.query)
		if err != nil {
			t.Fatalf("%s Start() error = %v", test.query, err)
		}
		if decision.EvidenceDocID != test.wantDoc ||
			decision.Terminal != test.terminal ||
			decision.NeedHuman != test.needHuman {
			t.Fatalf("%s decision = %#v", test.query, decision)
		}
	}
}

func TestDiagnosisInfersModelFromSpecificSymptoms(t *testing.T) {
	engine := NewDefaultEngine()
	tests := map[string]string{
		"毛发清完了还异响，下一步检查啥":     "X20 Pro",
		"传感器窗口擦过了数值还不变，能拆机看吗": "P500",
		"H200续航变短，水箱正常":       "H200",
		"掸套洗完松了，是不是要换":        "FD4",
		"缝隙刷刷毛张开，进不去缝里":       "GB2",
	}
	for query, want := range tests {
		if got := engine.InferModel(query); got != want {
			t.Fatalf("%q inferred model = %q, want %q", query, got, want)
		}
	}
}

func TestNormalizeCleaningToolSymptoms(t *testing.T) {
	tests := map[string]string{
		"FD4 掸套洗完松了，是不是要换": "掸套滑落",
		"GB2 刷毛张开后进不去缝里":   "刷毛变形",
		"SB3 刷头不转了":        "刷头停转",
	}
	for query, want := range tests {
		if got := normalizeSymptom(query); got != want {
			t.Fatalf("%q symptom = %q, want %q", query, got, want)
		}
	}
}

func TestFD4LooseSleeveSkipsToReplaceNode(t *testing.T) {
	engine := NewDefaultEngine()
	if got := engine.advanceRootFromObservedSteps("fd4_sleeve_slips", "FD4 掸套洗完松了，是不是要换？"); got != "fd4_replace_sleeve" {
		t.Fatalf("next node = %q, want fd4_replace_sleeve", got)
	}
}
