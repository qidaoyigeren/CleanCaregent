package intent

import (
	"context"
	"regexp"
	"strings"
)

type RuleRouter struct{}

var (
	orderNumberPattern = regexp.MustCompile(`(?i)\b(?:CC|ORDER)[0-9]{6,}\b`)
	modelPattern       = regexp.MustCompile(`(?i)\b(?:T20|X20\s*Pro|R10|R20|P400|P500|W300|W500|H100|H200)\b`)
	accessoryPattern   = regexp.MustCompile(`(?i)\b(?:F|DB|RB|C)[0-9]{2,}[A-Z0-9-]*\b`)
)

func NewRuleRouter() *RuleRouter {
	return &RuleRouter{}
}

func (r *RuleRouter) Route(ctx context.Context, request RouteRequest) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	query := strings.TrimSpace(request.Query)
	lower := strings.ToLower(query)
	entities := extractEntities(query)

	result := Result{Entities: entities, Confidence: 0.93}
	switch {
	case query == "":
		result = Result{
			Primary: PrimaryFallback, Secondary: Clarification,
			Confidence: 1, NeedClarify: true, Entities: entities,
		}
	case containsAny(lower,
		"手机", "衣服", "食品", "生鲜", "空气炸锅", "增值税发票",
		"别的用户", "所有订单导出来", "system prompt", "所有指令",
		"黑一下", "隔壁牌子", "小米那款", "ignore all previous",
	):
		result.Primary, result.Secondary, result.Confidence = PrimaryFallback, OutOfScope, 0.98
	case entities["models"] == "" && entities["order_no"] == "" &&
		containsAny(lower, "它能", "这个够", "那俩", "那个耗材", "你说的那", "便宜的，别问"):
		result.Primary, result.Secondary, result.Confidence = PrimaryFallback, Clarification, 0.98
		result.NeedClarify = true
	case containsAny(lower, "你好", "您好", "谢谢", "再见", "随便聊聊") && len([]rune(query)) <= 16:
		result.Primary, result.Secondary, result.Confidence = PrimaryFallback, Chitchat, 0.98
	case containsAny(lower,
		"创建工单", "售后工单", "帮我报修", "申请维修", "转人工",
		"建维修工单", "建维修单", "建工单", "提售后", "建售后单", "提交",
	):
		result.Primary, result.Secondary = PrimaryAftersales, CreateAfterSalesTicket
	case containsAny(lower,
		"还能退", "能退吗", "能退不", "能退么", "退货", "换货", "无理由",
		"退款", "能不能退", "能换不", "想退",
	):
		result.Primary, result.Secondary = PrimaryAftersales, ReturnEligibility
	case containsAny(lower,
		"充不进电", "无法充电", "不充电", "异响", "故障", "报错", "不工作",
		"漏水", "冒烟", "充不上电", "配网失败", "绑不上", "连不上", "问题仍在",
		"下一步", "传感器异常", "数值异常", "还是0%", "咔咔响",
		"嗡嗡响", "pm2.5一直", "金属摩擦声", "续航变短", "拆开看看",
		"配网一直失败",
	):
		result.Primary, result.Secondary = PrimaryDiagnosis, Troubleshooting
	case entities["order_no"] != "" && containsAny(lower, "出水小", "出水越来越小", "续航变短"):
		result.Primary, result.Secondary = PrimaryDiagnosis, Troubleshooting
	case containsAny(lower, "保修", "在保", "保修期", "过保", "延保"):
		result.Primary, result.Secondary = PrimaryAftersales, WarrantyQuery
	case accessoryIntentRequested(lower, entities):
		result.Primary, result.Secondary = PrimaryPresales, AccessoryCompatibility
	case containsAny(lower, "滤芯", "尘袋", "滚刷", "边刷", "配件", "耗材", "换芯", "替换滤芯") &&
		containsAny(lower, "上周买", "之前买", "我买的", "上次买", "购买记录", "上礼拜", "买过") &&
		containsAny(lower, "多少钱", "价格", "售价", "优惠", "券"):
		result.Primary, result.Secondary = PrimaryPresales, AccessoryCompatibility
	case containsAny(lower, "各多少钱", "价格一起查", "优惠后价格", "只查价格", "先给今天的"):
		result.Primary, result.Secondary = PrimaryPresales, PriceQuery
	case containsAny(lower, "哪个有货", "各剩多少", "库存都看", "各几台库存", "只按库存结果"):
		result.Primary, result.Secondary = PrimaryPresales, InventoryQuery
	case strings.Contains(strings.ToUpper(entities["models"]), "W300") &&
		containsAny(lower, "四口之家", "五口人", "集中用水", "用水高峰") &&
		containsAny(lower, "够不够", "会不会不够", "够用"):
		result.Primary, result.Secondary = PrimaryPresales, ProductComparison
		entities["models"] = "W300,W500"
	case explicitComparisonRequested(lower, entities):
		result.Primary, result.Secondary = PrimaryPresales, ProductComparison
	case singleModelConfigurationQuestion(lower, entities):
		result.Primary, result.Secondary = PrimaryPresales, ProductParameter
	case purchaseHistoryRequested(lower) && !usageInstructionRequested(lower) && !troubleshootingProcedureRequested(lower):
		result.Primary, result.Secondary = PrimaryAftersales, OrderQuery
	case networkBandChoiceRequested(lower):
		result.Primary, result.Secondary = PrimaryPresales, UsageInstruction
	case usageInstructionRequested(lower) && !troubleshootingProcedureRequested(lower):
		result.Primary, result.Secondary = PrimaryPresales, UsageInstruction
	case troubleshootingProcedureRequested(lower):
		result.Primary, result.Secondary = PrimaryDiagnosis, Troubleshooting
	case containsAny(lower,
		"预算", "筛完", "先推荐", "满足流量后", "租房想装", "不够就换",
		"适合北方", "合适的话", "新房刚装", "五口人", "四口之家",
		"婴儿房", "花粉季", "老人自己用", "选多大",
	):
		result.Primary, result.Secondary = PrimaryPresales, PurchaseRecommendation
	case containsAny(lower, "对比", "比较", "区别", "差在哪", "放一张表", "哪个更", "哪个好", "值不值", "除了") ||
		modelCount(entities) >= 2 &&
			containsAny(lower, "还是", "跟", "和") &&
			!containsAny(
				lower,
				"预算", "想少加", "少折腾", "想要安静", "适合我家", "这档够不够",
			):
		result.Primary, result.Secondary = PrimaryPresales, ProductComparison
	case !networkBandChoiceRequested(lower) && (containsAny(lower,
		"推荐", "怎么选", "选哪", "选什么", "筛完", "预算", "租房想装",
		"哪款", "少折腾", "想要安静", "想少加", "适合我家", "这档够不够",
	) || recommendationPairingRequested(lower)):
		result.Primary, result.Secondary = PrimaryPresales, PurchaseRecommendation
	case containsAny(lower,
		"首次使用", "使用前", "刚拆箱", "重新安装", "重新联网", "怎么冲洗",
		"怎么清洁", "怎么洗", "怎么设", "咋设", "咋开", "怎么开",
		"应该加什么水", "睡眠模式", "夜间模式", "多久换", "滤芯寿命",
		"配网", "联网", "清洁地毯", "更换步骤", "步骤给我", "袋子", "撕掉",
		"白醋", "抬拖布", "开机前", "重装", "越来越小", "要先放水",
		"直接灌", "关灯", "自动控制", "寿命在哪", "自己调风量",
		"第一次开机", "第一次用", "刚到家", "重新装", "建图", "连网",
		"第一次冲洗", "咋拆", "水冲", "关阀泄压", "泡洗", "咋清理",
		"显示30%", "掉到10%", "滤芯显示", "加水", "纯净水", "自来水",
		"硬水", "软水",
	):
		result.Primary, result.Secondary = PrimaryPresales, UsageInstruction
	case containsAny(lower, "兼容", "能装", "适配", "该买", "装不进", "配件", "滤芯", "尘袋", "滚刷", "边刷") &&
		containsAny(lower,
			"对吧", "兼容", "能装", "适配", "该买", "装不进", "上次买", "购买记录",
			"行不行", "只配", "分别买什么", "对应哪些", "合适吗", "接口一样", "换芯该买",
			"能用吧", "能不能给", "直接装",
		):
		result.Primary, result.Secondary = PrimaryPresales, AccessoryCompatibility
	case containsAny(lower,
		"够不够", "够用不", "够用吗", "适合多大", "适用面积", "主机有啥区别",
		"覆盖得过来", "会不会中途没电", "会不会太小", "开一夜够吗",
		"合适不", "参数一样不", "换硬件", "参数表", "都列一下", "参数发我",
		"废水比", "滤芯配置", "加湿量", "缺水保护", "出水", "每分钟",
		"一分钟", "多少水", "400g", "600g", "800g", "1000g", "多少pa",
	):
		result.Primary, result.Secondary = PrimaryPresales, ProductParameter
	case purchaseHistoryRequested(lower):
		result.Primary, result.Secondary = PrimaryAftersales, OrderQuery
	case containsAny(lower, "库存", "现货", "有货", "还有几台", "能直接下单", "能下单几台", "当天发", "剩多少"):
		result.Primary, result.Secondary = PrimaryPresales, InventoryQuery
	case priceRequested(lower):
		result.Primary, result.Secondary = PrimaryPresales, PriceQuery
	case containsAny(lower, "怎么用", "如何使用", "重置", "联网", "安装", "清洁方法", "更换方法"):
		result.Primary, result.Secondary = PrimaryPresales, UsageInstruction
	case containsAny(lower, "兼容", "能装", "适配", "滤芯", "尘袋", "滚刷", "边刷", "配件"):
		result.Primary, result.Secondary = PrimaryPresales, AccessoryCompatibility
	case containsAny(lower,
		"吸力", "噪声", "噪音", "cadr", "面积", "水箱", "功率", "参数",
		"规格", "续航", "越障", "流量", "通量", "导航", "集尘",
		"出水", "每分钟", "一分钟", "多少水", "多少pa", "400g",
	):
		result.Primary, result.Secondary = PrimaryPresales, ProductParameter
	default:
		result.Primary, result.Secondary, result.Confidence = PrimaryFallback, Clarification, 0.52
		result.NeedClarify = true
	}

	result.SecondaryIntents = detectSecondaryIntents(lower, result.Secondary, entities)
	result.NeedDecomposition = len(result.SecondaryIntents) > 0
	result.RouteTrace = RouteTrace{
		Source:          "rule",
		MatchedKeywords: matchedKeywords(lower, result.Secondary),
		Reasoning:       ruleReasoning(result.Secondary, result.SecondaryIntents),
		ConfidenceBasis: confidenceBasis(result.Confidence, entities, result.NeedClarify),
	}

	if needsProduct(result.Secondary) && entities["models"] == "" &&
		result.Secondary != PurchaseRecommendation && result.Secondary != Troubleshooting {
		switch {
		case canResolveFromPurchaseHistory(lower, result.Secondary):
			result.NeedClarify = false
			result.Confidence = min(result.Confidence, 0.82)
		case referencesPriorProduct(lower) && hasPriorProduct(request):
			result.NeedClarify = false
			result.Confidence = min(result.Confidence, 0.72)
		default:
			result.NeedClarify = true
			result.Confidence = min(result.Confidence, 0.62)
		}
	}
	if result.Secondary == CreateAfterSalesTicket &&
		(entities["order_no"] == "" || !ticketConfirmationPresent(lower)) {
		result.NeedClarify = true
		result.Confidence = min(result.Confidence, 0.9)
	}
	annotateCompetitor(query, &result)
	return result, nil
}

func ticketConfirmationPresent(query string) bool {
	if containsAny(query, "没确认", "未确认", "没有确认", "不确认", "不要创建") {
		return false
	}
	return containsAny(query, "我确认", "确认创建", "确认提交", "确认给", "确认了")
}

func detectSecondaryIntents(query string, primary Type, entities map[string]string) []Type {
	var candidates []Type
	add := func(value Type, matched bool) {
		if !matched || value == primary {
			return
		}
		for _, current := range candidates {
			if current == value {
				return
			}
		}
		candidates = append(candidates, value)
	}
	add(Troubleshooting, containsAny(query,
		"充不进电", "无法充电", "异响", "漏水", "冒烟", "报错", "配网失败", "绑不上",
	))
	add(ProductComparison,
		!singleModelConfigurationQuestion(query, entities) &&
			containsAny(query, "对比", "比较", "区别", "差在哪", "哪个更", "哪个好", "放一张表"),
	)
	add(PurchaseRecommendation,
		!networkBandChoiceRequested(query) &&
			(containsAny(query, "推荐", "怎么选", "选哪", "预算", "筛完", "租房想装") ||
				recommendationPairingRequested(query)),
	)
	add(ProductParameter, containsAny(query,
		"吸力", "噪声", "噪音", "cadr", "面积", "水箱", "功率", "参数",
		"规格", "续航", "越障", "流量", "通量", "够不够", "够用不",
		"出水", "每分钟", "一分钟", "多少pa", "400g",
	))
	add(AccessoryCompatibility, accessoryIntentRequested(query, entities))
	add(UsageInstruction,
		(!troubleshootingProcedureRequested(query) || networkBandChoiceRequested(query)) &&
			containsAny(query,
				"怎么用", "如何", "安装", "清洁", "配网", "联网", "重置", "怎么设",
				"咋设", "咋开", "步骤", "冲洗", "白醋", "撕掉", "纯净水", "自来水",
				"硬水",
			),
	)
	add(PriceQuery, priceRequested(query))
	add(InventoryQuery, containsAny(query, "库存", "现货", "有货", "还有几台"))
	add(OrderQuery, purchaseHistoryRequested(query))
	add(WarrantyQuery, containsAny(query, "保修", "在保", "保修期"))
	add(ReturnEligibility, containsAny(query, "退货", "换货", "还能退", "能退吗"))
	add(CreateAfterSalesTicket, containsAny(query, "创建工单", "售后工单", "申请维修", "帮我报修"))
	return candidates
}

func accessoryIntentRequested(query string, entities map[string]string) bool {
	hasAccessory := containsAny(query, "滤芯", "尘袋", "滚刷", "边刷", "配件", "耗材", "换芯", "替换滤芯") ||
		strings.TrimSpace(entities["accessory_refs"]) != ""
	hasRelationship := containsAny(
		query,
		"兼容", "能装", "适配", "该买", "装不进", "行不行", "只配",
		"分别买什么", "对应哪些", "能用吧", "能不能给", "直接装",
		"换芯买", "换芯该买", "买什么型号", "哪个滤芯", "型号别给错", "合适吗",
		"能用哪个", "装上", "塞", "接口一样", "通用",
	)
	return hasAccessory && hasRelationship
}

func purchaseHistoryRequested(query string) bool {
	return containsAny(
		query,
		"订单", "买的啥", "购买记录", "上周买", "上礼拜买", "之前买", "以前买",
		"什么时候买", "哪天签收", "最近一单", "啥状态", "走到哪", "最近买过",
		"过去一年买", "订单号忘了", "商品明细", "型号和数量", "状态都查",
		"具体是哪款", "型号给我翻", "买过",
	)
}

func troubleshootingProcedureRequested(query string) bool {
	return containsAny(query, "接着怎么查", "怎么查", "排查", "下一步", "仍然", "还是响", "还不行") ||
		(containsAny(query, "配网", "连接", "连不上") &&
			containsAny(query, "一直失败", "老失败", "账号地区", "权限", "双频合一"))
}

func usageInstructionRequested(query string) bool {
	return containsAny(
		query,
		"首次", "咋做", "怎么做", "怎么用", "如何", "冲洗", "清理",
		"更换", "清洁", "配网", "联网", "安装", "模式咋", "怎么设",
	)
}

func priceRequested(query string) bool {
	if containsAny(query, "别给我报价格", "不要报价", "不查价格", "别报价格", "不用报价格") {
		return false
	}
	return containsAny(
		query,
		"多少钱", "价格", "售价", "优惠", "券后", "报价", "到手价",
		"查价", "今天价格", "实时价", "总价", "哪个便宜",
	)
}

func matchedKeywords(query string, intentType Type) []string {
	keywords := map[Type][]string{
		ProductParameter:       {"吸力", "噪声", "噪音", "cadr", "面积", "水箱", "功率", "参数", "规格", "续航", "越障", "流量"},
		ProductComparison:      {"对比", "比较", "区别", "差在哪", "哪个好", "哪个更", "放一张表"},
		PurchaseRecommendation: {"推荐", "怎么选", "选哪", "预算", "筛完", "租房想装"},
		AccessoryCompatibility: {"兼容", "能装", "适配", "该买", "装不进", "滤芯", "尘袋", "滚刷", "边刷"},
		UsageInstruction:       {"怎么用", "如何", "安装", "清洁", "配网", "联网", "重置", "怎么设", "步骤", "冲洗"},
		PriceQuery:             {"多少钱", "价格", "售价", "优惠", "券后", "报价"},
		InventoryQuery:         {"库存", "现货", "有货", "还有几台"},
		OrderQuery:             {"订单", "购买记录", "什么时候买", "哪天签收", "最近一单"},
		WarrantyQuery:          {"保修", "在保", "保修期"},
		ReturnEligibility:      {"退货", "换货", "还能退", "能退吗"},
		Troubleshooting:        {"充不进电", "无法充电", "异响", "漏水", "冒烟", "报错", "配网失败", "绑不上"},
		CreateAfterSalesTicket: {"创建工单", "售后工单", "建售后单", "建工单", "申请维修", "帮我报修"},
		OutOfScope:             {"手机", "衣服", "食品", "生鲜", "发票", "投诉"},
		Chitchat:               {"你好", "您好", "谢谢", "再见"},
	}
	var matched []string
	for _, keyword := range keywords[intentType] {
		if strings.Contains(query, keyword) {
			matched = append(matched, keyword)
		}
	}
	if intentType == PurchaseRecommendation && recommendationPairingRequested(query) {
		matched = append(matched, "怎么配")
	}
	return matched
}

func recommendationPairingRequested(query string) bool {
	return strings.Contains(query, "怎么配") && !strings.Contains(query, "怎么配网")
}

func networkBandChoiceRequested(query string) bool {
	return containsAny(query, "2.4g", "5g", "双频合一") &&
		containsAny(query, "配网", "联网", "连接", "连不上") &&
		containsAny(query, "选哪", "选哪个", "怎么选", "到底选")
}

func singleModelConfigurationQuestion(query string, entities map[string]string) bool {
	return modelCount(entities) == 1 &&
		containsAny(query, "套装", "主机", "配置") &&
		containsAny(query, "区别", "差异", "有什么不同", "有啥区别")
}

func explicitComparisonRequested(query string, entities map[string]string) bool {
	if modelCount(entities) < 2 {
		return false
	}
	return containsAny(
		query,
		"对比", "比较", "区别", "差啥", "差在哪", "放一起比", "一起比",
		"一起说", "两款到手价", "两款价格", "各自价格", "哪个便宜",
	)
}

func ruleReasoning(primary Type, secondary []Type) string {
	if len(secondary) == 0 {
		return "规则关键词、实体和上下文共同指向单一意图"
	}
	values := make([]string, len(secondary))
	for index := range secondary {
		values[index] = string(secondary[index])
	}
	return "检测到复合诉求，主意图为 " + string(primary) + "，其余意图为 " + strings.Join(values, ",")
}

func confidenceBasis(confidence float64, entities map[string]string, needClarify bool) string {
	switch {
	case needClarify:
		return "意图关键词明确，但关键实体缺失或指代无法解析"
	case confidence >= 0.9:
		return "高区分度关键词命中且关键实体充分"
	case entities["models"] != "" || entities["order_no"] != "":
		return "关键词命中并提取到型号或订单实体"
	default:
		return "关键词命中但实体信息有限"
	}
}

func canResolveFromPurchaseHistory(query string, intentType Type) bool {
	return intentType == AccessoryCompatibility &&
		containsAny(query, "上周买", "之前买", "我买的", "上次买", "购买记录", "上礼拜", "买过")
}

func modelCount(entities map[string]string) int {
	count := 0
	for _, model := range strings.Split(entities["models"], ",") {
		if strings.TrimSpace(model) != "" {
			count++
		}
	}
	return count
}

func extractEntities(query string) map[string]string {
	entities := map[string]string{}
	lower := strings.ToLower(query)
	orderNo := strings.ToUpper(orderNumberPattern.FindString(query))
	if models := modelPattern.FindAllString(query, -1); len(models) > 0 {
		filtered := models[:0]
		for index := range models {
			models[index] = normalizeModelName(models[index])
			if strings.EqualFold(models[index], orderNo) {
				continue
			}
			filtered = append(filtered, models[index])
		}
		if len(filtered) > 0 {
			entities["models"] = strings.Join(filtered, ",")
		}
	}
	if orderNo != "" {
		entities["order_no"] = orderNo
	}
	var accessoryRefs []string
	for _, ref := range accessoryPattern.FindAllString(query, -1) {
		accessoryRefs = append(accessoryRefs, normalizeAccessoryRef(ref))
	}
	for _, ref := range []string{"滤芯", "尘袋", "滚刷", "边刷", "配件", "耗材", "换芯"} {
		if strings.Contains(lower, ref) {
			accessoryRefs = append(accessoryRefs, ref)
		}
	}
	if accessoryRefs = compactStrings(accessoryRefs); len(accessoryRefs) > 0 {
		entities["accessory_refs"] = strings.Join(accessoryRefs, ",")
	}
	switch {
	case containsAny(lower, "扫地机器人", "扫地机", "扫拖机器人"):
		entities["category"] = "robot_vacuum"
	case containsAny(lower, "空气净化器", "净化器"):
		entities["category"] = "air_purifier"
	case containsAny(lower, "净水器"):
		entities["category"] = "water_purifier"
	case containsAny(lower, "加湿器"):
		entities["category"] = "humidifier"
	}
	modelCategories := categoriesForModelCSV(entities["models"])
	allCategories := append([]string(nil), modelCategories...)
	if entities["category"] != "" {
		allCategories = append(allCategories, entities["category"])
	}
	allCategories = compactStrings(allCategories)
	if len(allCategories) > 0 {
		entities["categories"] = strings.Join(allCategories, ",")
	}
	if entities["category"] == "" && len(modelCategories) == 1 {
		entities["category"] = modelCategories[0]
	}
	return entities
}

func normalizeModelName(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if strings.EqualFold(strings.ReplaceAll(value, " ", ""), "X20Pro") {
		return "X20 Pro"
	}
	return strings.ToUpper(value)
}

func normalizeAccessoryRef(value string) string {
	return strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(value)), ""))
}

func categoryForModels(models string) string {
	categories := categoriesForModelCSV(models)
	if len(categories) > 0 {
		return categories[0]
	}
	return ""
}

func categoriesForModelCSV(models string) []string {
	var categories []string
	for _, modelName := range strings.Split(models, ",") {
		var category string
		switch strings.ToUpper(strings.Join(strings.Fields(modelName), " ")) {
		case "T20", "X20 PRO", "R10", "R20":
			category = "robot_vacuum"
		case "P400", "P500":
			category = "air_purifier"
		case "W300", "W500":
			category = "water_purifier"
		case "H100", "H200":
			category = "humidifier"
		}
		if category != "" {
			categories = append(categories, category)
		}
	}
	return compactStrings(categories)
}

func compactStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func referencesPriorProduct(query string) bool {
	return containsAny(query, "那个", "这个", "这台", "它", "我买的", "上周买的", "之前买的")
}

func hasPriorProduct(request RouteRequest) bool {
	for index := len(request.RecentMessages) - 1; index >= 0; index-- {
		if len(modelPattern.FindAllString(request.RecentMessages[index].Content, -1)) > 0 {
			return true
		}
	}
	return len(modelPattern.FindAllString(request.Summary, -1)) > 0
}

func needsProduct(intent Type) bool {
	switch intent {
	case ProductParameter, ProductComparison, AccessoryCompatibility, UsageInstruction,
		PriceQuery, InventoryQuery:
		return true
	default:
		return false
	}
}
