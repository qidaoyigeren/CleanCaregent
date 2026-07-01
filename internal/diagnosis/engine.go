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
		{
			ID:            "p400_noise_filter",
			ProductModel:  "P400",
			Symptom:       "异响",
			Question:      "滤芯塑封是否已拆除，并且进出风口没有被遮挡？",
			Guidance:      "请先断电，重新检查滤芯安装方向、塑封膜和进出风口，不要拆电机仓。",
			YesNext:       "p400_noise_service",
			NoNext:        "p400_noise_fix",
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_p400_noise",
		},
		{
			ID:            "p400_noise_fix",
			ProductModel:  "P400",
			Symptom:       "异响",
			Resolution:    "先拆除滤芯塑封、按箭头方向复装滤芯，并清理进出风口遮挡物；复装后短时试机。",
			Terminal:      true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_p400_noise",
		},
		{
			ID:            "p400_noise_service",
			ProductModel:  "P400",
			Symptom:       "异响",
			Resolution:    "滤芯和风道已排除后仍持续异响，可能涉及风机或电机部件。请停止使用并联系售后检测，不要自行拆机。",
			Terminal:      true,
			NeedHuman:     true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_p400_noise",
		},
		{
			ID:            "p500_sensor_clean",
			ProductModel:  "P500",
			Symptom:       "传感器异常",
			Question:      "传感器窗口是否已断电清洁并重启，数值仍然不变吗？",
			Guidance:      "请用干燥软布清洁传感器窗口，重启后观察 PM2.5 数值是否恢复变化。",
			YesNext:       "p500_sensor_service",
			NoNext:        "p500_sensor_fix",
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_p500_sensor",
		},
		{
			ID:            "p500_sensor_fix",
			ProductModel:  "P500",
			Symptom:       "传感器异常",
			Resolution:    "清洁传感器窗口并重启后继续观察；若环境变化时数值恢复波动，暂不需要拆机。",
			Terminal:      true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_p500_sensor",
		},
		{
			ID:            "p500_sensor_service",
			ProductModel:  "P500",
			Symptom:       "传感器异常",
			Resolution:    "清洁传感器窗口并重启后数值仍固定异常，可能涉及传感器模块。请记录环境、数值和错误码，联系售后检测，不要拆机。",
			Terminal:      true,
			NeedHuman:     true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_p500_sensor",
		},
		{
			ID:            "w300_leak_stop",
			ProductModel:  "W300",
			Symptom:       "漏水",
			Question:      "是否已经关闭进水阀并断开电源？",
			Guidance:      "先关闭进水阀、断电并擦干地面积水；不要带压拆卸接头。",
			YesNext:       "w300_leak_service",
			NoNext:        "w300_leak_stop_now",
			SafetyLevel:   SafetyHigh,
			EvidenceDocID: "kb_fault_w300_leak",
		},
		{
			ID:            "w300_leak_stop_now",
			ProductModel:  "W300",
			Symptom:       "漏水",
			Resolution:    "请立即关闭进水阀并断开电源，擦干地面积水；不要带压拆卸接头。",
			Terminal:      true,
			NeedHuman:     true,
			SafetyLevel:   SafetyHigh,
			EvidenceDocID: "kb_fault_w300_leak",
		},
		{
			ID:            "w300_leak_service",
			ProductModel:  "W300",
			Symptom:       "漏水",
			Resolution:    "已关阀断电后仍滴水，需停止使用并安排售后检测；请准备订单、安装照片和漏水位置照片。",
			Terminal:      true,
			NeedHuman:     true,
			SafetyLevel:   SafetyHigh,
			EvidenceDocID: "kb_fault_w300_leak",
		},
		{
			ID:            "t20_wifi_band",
			ProductModel:  "T20",
			Symptom:       "网络连接",
			Question:      "手机当前是否连接 2.4GHz Wi-Fi，并且主机已重新进入配网模式？",
			Guidance:      "T20 配网应使用 2.4GHz Wi-Fi；双频合一路由器需临时拆分频段或关闭 5GHz 后再重试。",
			YesNext:       "t20_wifi_router",
			NoNext:        "t20_wifi_set_band",
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_t20_wifi",
		},
		{
			ID:            "t20_wifi_set_band",
			ProductModel:  "T20",
			Symptom:       "网络连接",
			Resolution:    "请先把手机连到 2.4GHz Wi-Fi，并长按联网键让主机重新进入配网模式；双频合一时建议临时拆分 2.4GHz/5GHz。",
			Terminal:      true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_t20_wifi",
		},
		{
			ID:            "t20_wifi_router",
			ProductModel:  "T20",
			Symptom:       "网络连接",
			Resolution:    "若 2.4GHz 和配网模式已确认仍失败，请检查路由器 AP 隔离、黑名单和 App 错误码，再联系售后。",
			Terminal:      true,
			NeedHuman:     true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_t20_wifi",
		},
		{
			ID:            "x20_app_region",
			ProductModel:  "X20 Pro",
			Symptom:       "App配对",
			Question:      "账号地区是否与设备销售地区一致，并且 App 已允许蓝牙和定位权限？",
			Guidance:      "建议顺序：先核对账号地区，再开启蓝牙/定位权限，最后重新进入配网模式。",
			YesNext:       "x20_app_service",
			NoNext:        "x20_app_fix",
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_x20_app_pair",
		},
		{
			ID:            "x20_app_fix",
			ProductModel:  "X20 Pro",
			Symptom:       "App配对",
			Resolution:    "请按账号地区、蓝牙/定位权限、重新进入配网模式的顺序排查；不要反复绑定同一失败流程。",
			Terminal:      true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_x20_app_pair",
		},
		{
			ID:            "x20_app_service",
			ProductModel:  "X20 Pro",
			Symptom:       "App配对",
			Resolution:    "账号地区、权限和配网模式都确认后仍失败，请记录 App 错误码并联系售后。",
			Terminal:      true,
			NeedHuman:     true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_x20_app_pair",
		},
		{
			ID:            "r20_noise_check",
			ProductModel:  "R20",
			Symptom:       "异响",
			Question:      "是否已经关机检查主刷、边刷、万向轮和轴端毛发？",
			Guidance:      "请关机后清理主刷、边刷和万向轮异物；若是持续金属摩擦声，不要继续长时间试跑。",
			YesNext:       "r20_noise_service",
			NoNext:        "r20_noise_clean",
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_r20_noise",
		},
		{
			ID:            "r20_noise_clean",
			ProductModel:  "R20",
			Symptom:       "异响",
			Resolution:    "请先关机清理主刷、边刷、万向轮和轴端毛发，复装后只做短时验证。",
			Terminal:      true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_r20_noise",
		},
		{
			ID:            "r20_noise_service",
			ProductModel:  "R20",
			Symptom:       "异响",
			Resolution:    "清理毛发后仍有持续金属摩擦声，不建议继续跑一圈；请停止使用并联系售后检测。",
			Terminal:      true,
			NeedHuman:     true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_r20_noise",
		},
		{
			ID:            "h200_runtime_check",
			ProductModel:  "H200",
			Symptom:       "续航下降",
			Question:      "当前目标湿度、档位和水箱水位是否正常？是否发现漏水？",
			Guidance:      "先核对目标湿度和档位，再检查水箱实际容量、漏水和雾化组件清洁状态。",
			YesNext:       "h200_runtime_service",
			NoNext:        "h200_runtime_fix",
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_h200_runtime",
		},
		{
			ID:            "h200_runtime_fix",
			ProductModel:  "H200",
			Symptom:       "续航下降",
			Resolution:    "请先降低不必要的高档位，核对水箱水位并清洁雾化组件；记录满水后的实际运行时长。",
			Terminal:      true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_h200_runtime",
		},
		{
			ID:            "h200_runtime_service",
			ProductModel:  "H200",
			Symptom:       "续航下降",
			Resolution:    "档位、水箱和雾化组件都排除后仍续航明显变短，请记录运行时长并联系售后核验保修。",
			Terminal:      true,
			NeedHuman:     true,
			SafetyLevel:   SafetyLow,
			EvidenceDocID: "kb_fault_h200_runtime",
		},
	}
	engine := &Engine{
		nodes: make(map[string]Node, len(nodes)),
		roots: map[string]string{
			rootKey("T20", "无法充电"):      "t20_charge_power",
			rootKey("X20 Pro", "滚刷缠绕"):  "x20_tangle_poweroff",
			rootKey("P400", "异响"):       "p400_noise_filter",
			rootKey("P500", "传感器异常"):    "p500_sensor_clean",
			rootKey("W300", "漏水"):       "w300_leak_stop",
			rootKey("T20", "网络连接"):      "t20_wifi_band",
			rootKey("X20 Pro", "App配对"): "x20_app_region",
			rootKey("R20", "异响"):        "r20_noise_check",
			rootKey("H200", "续航下降"):     "h200_runtime_check",
		},
		safety: []string{"漏电", "冒烟", "烧焦味", "起火", "电线破损", "漏水"},
	}
	for _, node := range nodes {
		engine.nodes[node.ID] = node
	}
	return engine
}

func NewEngine(nodes []Node, roots map[string]string, safety []string) *Engine {
	engine := &Engine{
		nodes:  map[string]Node{},
		roots:  map[string]string{},
		safety: append([]string(nil), safety...),
	}
	_ = engine.Merge(nodes, roots, nil)
	return engine
}

func (e *Engine) Merge(nodes []Node, roots map[string]string, safety []string) error {
	if e.nodes == nil {
		e.nodes = map[string]Node{}
	}
	if e.roots == nil {
		e.roots = map[string]string{}
	}
	e.safety = append(e.safety, safety...)
	for _, node := range nodes {
		node.ID = strings.TrimSpace(node.ID)
		node.ProductModel = strings.TrimSpace(node.ProductModel)
		node.Symptom = strings.TrimSpace(node.Symptom)
		if node.ID == "" {
			continue
		}
		e.nodes[node.ID] = node
	}
	for key, nodeID := range roots {
		key = strings.TrimSpace(key)
		nodeID = strings.TrimSpace(nodeID)
		if key == "" || nodeID == "" {
			continue
		}
		if _, ok := e.nodes[nodeID]; !ok {
			return ErrNoMatchingTree
		}
		e.roots[key] = nodeID
	}
	return nil
}

func (e *Engine) Start(modelName, query string) (memory.DiagnosisState, Decision, error) {
	if decision, ok := e.SafetyDecision(query); ok {
		return memory.DiagnosisState{}, decision, nil
	}
	symptom := normalizeSymptom(query)
	rootID := e.roots[rootKey(modelName, symptom)]
	if rootID == "" {
		rootID = e.matchRootByQuery(modelName, query)
	}
	if rootID == "" {
		return memory.DiagnosisState{}, Decision{}, ErrNoMatchingTree
	}
	rootID = e.advanceRootFromObservedSteps(rootID, query)
	node := e.nodes[rootID]
	state := memory.DiagnosisState{
		ProductModel: modelName,
		FaultNodeID:  rootID,
		Answers:      map[string]string{},
	}
	return state, decisionFromNode(node, false), nil
}

func (e *Engine) matchRootByQuery(modelName, query string) string {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	query = strings.ToLower(strings.TrimSpace(query))
	if modelName == "" || query == "" {
		return ""
	}
	for key, nodeID := range e.roots {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != modelName {
			continue
		}
		symptom := strings.ToLower(strings.TrimSpace(parts[1]))
		if symptom != "" && strings.Contains(query, symptom) {
			return nodeID
		}
	}
	return ""
}

func (e *Engine) InferModel(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	switch {
	case containsAny(query, "pm2.5", "传感器", "数值"):
		return "P500"
	case containsAny(query, "双频合一", "2.4g", "5g") && containsAny(query, "配网", "连不上", "连接"):
		return "T20"
	case containsAny(query, "账号地区", "权限", "app") && containsAny(query, "配网", "配对", "绑定"):
		return "X20 Pro"
	case containsAny(query, "金属摩擦", "万向轮"):
		return "R20"
	case containsAny(query, "毛发", "猫毛", "滚刷", "咔咔响"):
		return "X20 Pro"
	case containsAny(query, "滤芯", "风口", "嗡嗡", "响"):
		return "P400"
	case containsAny(query, "漏水", "滴水", "关阀"):
		return "W300"
	case containsAny(query, "续航", "水箱", "运行时长"):
		return "H200"
	case containsAny(query, "掸套", "除尘掸", "伸缩除尘", "伸缩杆"):
		return "FD4"
	case containsAny(query, "缝隙刷", "刷毛变形", "刷毛张开", "水槽底座缝"):
		return "GB2"
	case containsAny(query, "刷头停转", "刷头不转", "电动清洁刷"):
		return "SB3"
	default:
		return ""
	}
}

func (e *Engine) advanceRootFromObservedSteps(rootID, query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	switch rootID {
	case "t20_charge_power":
		if containsAny(query, "灯是亮", "灯亮", "指示灯亮", "亮的") &&
			containsAny(query, "触点也擦", "触点擦", "清洁触点", "还是充不进", "仍然充不进") {
			return "t20_charge_service"
		}
		if containsAny(query, "灯是亮", "灯亮", "指示灯亮", "亮的") {
			return "t20_charge_contact"
		}
	case "x20_tangle_poweroff":
		if containsAny(query, "毛发清完", "清完", "清理") &&
			containsAny(query, "还异响", "还是响", "持续异响") {
			return "x20_tangle_service"
		}
	case "p400_noise_filter":
		if containsAny(query, "重装", "复装", "装过") &&
			containsAny(query, "还", "仍然", "不行") {
			return "p400_noise_service"
		}
	case "p500_sensor_clean":
		if containsAny(query, "擦过", "清洁过") &&
			containsAny(query, "还不变", "仍不变", "一直是0") {
			return "p500_sensor_service"
		}
	case "w300_leak_stop":
		if containsAny(query, "关了", "关阀", "断电") &&
			containsAny(query, "还是", "仍然", "滴水") {
			return "w300_leak_service"
		}
	case "r20_noise_check":
		if containsAny(query, "清完", "清理") &&
			containsAny(query, "金属摩擦", "还响", "仍然响") {
			return "r20_noise_service"
		}
	case "fd4_sleeve_slips":
		if containsAny(query, "松了", "松弛", "套不紧", "变形", "洗完", "老化", "要换", "是不是要换") {
			return "fd4_replace_sleeve"
		}
	}
	return rootID
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
				Resolution:  "请立即停止使用并断开电源；涉及漏水的设备同时关闭进水阀。不要拆机、不要拔内部线缆，也不要继续通电测试，请联系售后处理。",
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
	case containsAny(query, "充不进电", "充不进", "充不上电", "充不上", "无法充电", "不能充电"):
		return "无法充电"
	case containsAny(query, "配网", "连不上", "连接", "双频合一", "2.4g", "5g"):
		if containsAny(query, "账号地区", "权限", "app", "配对", "绑定") {
			return "App配对"
		}
		return "网络连接"
	case containsAny(query, "滚刷缠绕", "毛发缠绕", "滚刷异响", "滚刷卡住"):
		return "滚刷缠绕"
	case containsAny(query, "毛发", "猫毛", "咔咔响"):
		return "滚刷缠绕"
	case containsAny(query, "pm2.5", "pm 2.5", "传感器", "数值", "一直是0", "一直为0"):
		return "传感器异常"
	case containsAny(query, "漏水", "滴水", "关阀"):
		return "漏水"
	case containsAny(query, "续航", "运行时长", "水箱"):
		return "续航下降"
	case containsAny(query, "掸套", "套筒", "松紧口") &&
		containsAny(query, "滑落", "松了", "松弛", "套不紧", "变形", "洗完", "老化"):
		return "掸套滑落"
	case containsAny(query, "刷毛", "刷头") &&
		containsAny(query, "变形", "张开", "弯了", "进不去"):
		return "刷毛变形"
	case containsAny(query, "刷头停转", "刷头不转", "停转", "不转"):
		return "刷头停转"
	case containsAny(query, "异响", "嗡嗡", "响", "金属摩擦", "万向轮"):
		return "异响"
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
