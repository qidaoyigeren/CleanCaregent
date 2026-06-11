package diagnosis

import (
	"errors"
	"strings"

	"CleanCaregent/internal/memory"
)

var ErrNoMatchingTree = errors.New("no matching diagnosis tree")

type SafetyLevel string

const (
	SafetyLow  SafetyLevel = "low"
	SafetyHigh SafetyLevel = "high"
)

type Node struct {
	ID            string
	ProductModel  string
	Symptom       string
	Question      string
	Guidance      string
	YesNext       string
	NoNext        string
	Resolution    string
	Terminal      bool
	NeedHuman     bool
	SafetyLevel   SafetyLevel
	EvidenceDocID string
}

type Decision struct {
	NodeID        string
	Question      string
	Guidance      string
	Resolution    string
	Terminal      bool
	NeedHuman     bool
	SafetyLevel   SafetyLevel
	EvidenceDocID string
	Understood    bool
}

type Engine struct {
	nodes  map[string]Node
	roots  map[string]string
	safety []string
}

func NewDefaultEngine() *Engine {
	nodes := []Node{
		{
			ID:            "t20_charge_power",
			ProductModel:  "T20",
			Symptom:       "无法充电",
			Question:      "充电座的电源指示灯现在是否亮着？",
			Guidance:      "请确认插座有电、适配器两端插紧，只观察指示灯，不要拆机。",
			YesNext:       "t20_charge_contact",
			NoNext:        "t20_charge_no_power",
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_t20_charge",
		},
		{
			ID:            "t20_charge_no_power",
			ProductModel:  "T20",
			Symptom:       "无法充电",
			Resolution:    "充电座未通电。请更换已确认有电的插座并重新插紧适配器；若指示灯仍不亮，应停止使用该适配器并联系售后检测。",
			Terminal:      true,
			NeedHuman:     true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_t20_charge",
		},
		{
			ID:            "t20_charge_contact",
			ProductModel:  "T20",
			Symptom:       "无法充电",
			Question:      "断电后清洁主机和充电座金属触点、重新对位，主机现在能开始充电吗？",
			Guidance:      "请先拔掉充电座电源，再用干燥软布清洁触点，复位后重新通电测试。",
			YesNext:       "t20_charge_resolved",
			NoNext:        "t20_charge_service",
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_t20_charge",
		},
		{
			ID:            "t20_charge_resolved",
			ProductModel:  "T20",
			Symptom:       "无法充电",
			Resolution:    "重新清洁触点并对位后已恢复充电。建议保持充电座周围无遮挡，并定期用干燥软布清洁金属触点。",
			Terminal:      true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_t20_charge",
		},
		{
			ID:            "t20_charge_service",
			ProductModel:  "T20",
			Symptom:       "无法充电",
			Resolution:    "充电座供电和触点状态已排除，问题可能涉及电池、充电模块或主板，需要售后检测。请不要自行拆机。",
			Terminal:      true,
			NeedHuman:     true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_t20_charge",
		},
		{
			ID:            "x20_tangle_poweroff",
			ProductModel:  "X20 Pro",
			Symptom:       "滚刷缠绕",
			Question:      "机器关机后，您是否已经取出滚刷并清理轴端毛发和异物？",
			Guidance:      "请先关机，再按说明书取出滚刷；不要在机器运行时接触滚刷。",
			YesNext:       "x20_tangle_noise",
			NoNext:        "x20_tangle_clean",
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_x20_tangle",
		},
		{
			ID:            "x20_tangle_clean",
			ProductModel:  "X20 Pro",
			Symptom:       "滚刷缠绕",
			Resolution:    "请在关机状态下清理滚刷和轴端异物，确认滚刷可自由转动后再装回测试。",
			Terminal:      true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_x20_tangle",
		},
		{
			ID:            "x20_tangle_noise",
			ProductModel:  "X20 Pro",
			Symptom:       "滚刷缠绕",
			Question:      "清理并重新安装滚刷后，机器是否仍有持续异响？",
			Guidance:      "请只进行短时间空地测试；若声音明显异常，立即关机。",
			YesNext:       "x20_tangle_service",
			NoNext:        "x20_tangle_resolved",
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_x20_tangle",
		},
		{
			ID:            "x20_tangle_service",
			ProductModel:  "X20 Pro",
			Symptom:       "滚刷缠绕",
			Resolution:    "清理滚刷后仍持续异响，可能涉及滚刷电机或传动部件，请停止使用并联系售后检测。",
			Terminal:      true,
			NeedHuman:     true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_x20_tangle",
		},
		{
			ID:            "x20_tangle_resolved",
			ProductModel:  "X20 Pro",
			Symptom:       "滚刷缠绕",
			Resolution:    "清理滚刷和轴端异物后故障已解除。养宠家庭建议提高滚刷检查频率。",
			Terminal:      true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_x20_tangle",
		},
	}
	engine := &Engine{
		nodes: make(map[string]Node, len(nodes)),
		roots: map[string]string{
			rootKey("T20", "无法充电"):     "t20_charge_power",
			rootKey("X20 Pro", "滚刷缠绕"): "x20_tangle_poweroff",
		},
		safety: []string{"漏电", "冒烟", "烧焦味", "起火", "电线破损", "漏水"},
	}
	for _, node := range nodes {
		engine.nodes[node.ID] = node
	}
	return engine
}

func (e *Engine) Start(modelName, query string) (memory.DiagnosisState, Decision, error) {
	if decision, ok := e.SafetyDecision(query); ok {
		return memory.DiagnosisState{}, decision, nil
	}
	symptom := normalizeSymptom(query)
	rootID := e.roots[rootKey(modelName, symptom)]
	if rootID == "" {
		return memory.DiagnosisState{}, Decision{}, ErrNoMatchingTree
	}
	node := e.nodes[rootID]
	state := memory.DiagnosisState{
		ProductModel: modelName,
		FaultNodeID:  rootID,
		Answers:      map[string]string{},
	}
	return state, decisionFromNode(node, false), nil
}

func (e *Engine) Advance(
	state memory.DiagnosisState,
	answer string,
) (memory.DiagnosisState, Decision, error) {
	if decision, ok := e.SafetyDecision(answer); ok {
		return state, decision, nil
	}
	current, ok := e.nodes[state.FaultNodeID]
	if !ok {
		return state, Decision{}, ErrNoMatchingTree
	}
	yes, understood := classifyBoolean(answer)
	if !understood {
		return state, decisionFromNode(current, false), nil
	}
	if state.Answers == nil {
		state.Answers = map[string]string{}
	}
	state.Answers[current.ID] = strings.TrimSpace(answer)
	nextID := current.NoNext
	if yes {
		nextID = current.YesNext
	}
	next, ok := e.nodes[nextID]
	if !ok {
		return state, Decision{}, ErrNoMatchingTree
	}
	state.FaultNodeID = next.ID
	return state, decisionFromNode(next, true), nil
}

func (e *Engine) SafetyDecision(query string) (Decision, bool) {
	for _, keyword := range e.safety {
		if strings.Contains(query, keyword) {
			return Decision{
				NodeID:      "safety_stop",
				Resolution:  "请立即停止使用并断开电源；涉及漏水的设备同时关闭进水阀。不要拆机或继续通电测试，请联系售后处理。",
				Terminal:    true,
				NeedHuman:   true,
				SafetyLevel: SafetyHigh,
				Understood:  true,
			}, true
		}
	}
	return Decision{}, false
}

func decisionFromNode(node Node, understood bool) Decision {
	return Decision{
		NodeID:        node.ID,
		Question:      node.Question,
		Guidance:      node.Guidance,
		Resolution:    node.Resolution,
		Terminal:      node.Terminal,
		NeedHuman:     node.NeedHuman,
		SafetyLevel:   node.SafetyLevel,
		EvidenceDocID: node.EvidenceDocID,
		Understood:    understood,
	}
}

func rootKey(modelName, symptom string) string {
	return strings.ToLower(strings.TrimSpace(modelName)) + "|" + symptom
}

func normalizeSymptom(query string) string {
	switch {
	case containsAny(query, "充不进电", "无法充电", "不能充电", "充不上电"):
		return "无法充电"
	case containsAny(query, "滚刷缠绕", "毛发缠绕", "滚刷异响", "滚刷卡住"):
		return "滚刷缠绕"
	default:
		return strings.TrimSpace(query)
	}
}

func classifyBoolean(answer string) (bool, bool) {
	normalized := strings.ToLower(strings.TrimSpace(answer))
	if containsAny(normalized, "没有", "没亮", "不亮", "不能", "不行", "仍然", "还是不", "否", "no") {
		return false, true
	}
	if containsAny(normalized, "已经", "亮着", "亮了", "可以", "能", "好了", "正常", "是", "yes") {
		return true, true
	}
	return false, false
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
