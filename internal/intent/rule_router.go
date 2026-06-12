package intent

import (
	"context"
	"regexp"
	"strings"
)

type RuleRouter struct{}

var (
	orderNumberPattern = regexp.MustCompile(`(?i)\b(?:CC|ORDER)[0-9]{6,}\b`)
	modelPattern       = regexp.MustCompile(`(?i)\b[A-Z]+[0-9]+(?:\s+Pro)?\b`)
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

	result := Result{Entities: entities, Confidence: 0.88}
	switch {
	case query == "":
		result = Result{Primary: PrimaryFallback, Secondary: Clarification, Confidence: 1, NeedClarify: true}
	case containsAny(lower, "手机", "衣服", "食品", "生鲜", "发票", "投诉"):
		result.Primary, result.Secondary, result.Confidence = PrimaryFallback, OutOfScope, 0.98
	case containsAny(lower, "你好", "您好", "谢谢", "再见") && len([]rune(query)) <= 12:
		result.Primary, result.Secondary, result.Confidence = PrimaryFallback, Chitchat, 0.98
	case containsAny(lower, "创建工单", "售后工单", "帮我报修", "申请维修", "转人工"):
		result.Primary, result.Secondary = PrimaryAftersales, CreateAfterSalesTicket
	case containsAny(lower, "充不进电", "无法充电", "不充电", "异响", "故障", "报错", "不工作", "漏水", "冒烟",
		"问题仍在", "下一步", "传感器", "数值异常"):
		result.Primary, result.Secondary = PrimaryDiagnosis, Troubleshooting
	case containsAny(lower, "还能退", "能退吗", "退货", "换货", "无理由"):
		result.Primary, result.Secondary = PrimaryAftersales, ReturnEligibility
	case containsAny(lower, "保修", "在保", "保修期"):
		result.Primary, result.Secondary = PrimaryAftersales, WarrantyQuery
	case containsAny(lower, "滤芯", "尘袋", "滚刷", "边刷", "配件") &&
		containsAny(lower, "上周买", "之前买", "我买的", "购买记录") &&
		containsAny(lower, "多少钱", "价格", "售价", "优惠", "券"):
		result.Primary, result.Secondary = PrimaryPresales, AccessoryCompatibility
	case containsAny(lower, "订单", "买的哪", "购买记录", "上周买", "什么时候买"):
		result.Primary, result.Secondary = PrimaryAftersales, OrderQuery
	case containsAny(lower, "库存", "现货", "有货"):
		result.Primary, result.Secondary = PrimaryPresales, InventoryQuery
	case containsAny(lower, "多少钱", "价格", "售价", "优惠", "券"):
		result.Primary, result.Secondary = PrimaryPresales, PriceQuery
	case containsAny(lower, "首次使用", "使用前", "重新安装", "重新联网", "怎么冲洗", "出水变小",
		"怎么清洁", "应该加什么水", "睡眠模式", "多久换", "滤芯寿命", "配网", "清洁地毯"):
		result.Primary, result.Secondary = PrimaryPresales, UsageInstruction
	case containsAny(lower, "兼容", "能装", "适配", "滤芯", "尘袋", "滚刷", "边刷", "配件"):
		result.Primary, result.Secondary = PrimaryPresales, AccessoryCompatibility
	case containsAny(lower, "对比", "比较", "区别", "哪个好", "哪个更"):
		result.Primary, result.Secondary = PrimaryPresales, ProductComparison
	case containsAny(lower, "推荐", "怎么选", "选哪", "选什么", "选多大", "适合我", "预算"):
		result.Primary, result.Secondary = PrimaryPresales, PurchaseRecommendation
	case containsAny(lower, "怎么用", "如何使用", "重置", "联网", "安装", "清洁方法", "更换方法"):
		result.Primary, result.Secondary = PrimaryPresales, UsageInstruction
	case containsAny(lower, "吸力", "噪声", "噪音", "cadr", "面积", "水箱", "功率", "参数", "规格"):
		result.Primary, result.Secondary = PrimaryPresales, ProductParameter
	default:
		result.Primary, result.Secondary, result.Confidence = PrimaryFallback, Clarification, 0.52
		result.NeedClarify = true
	}

	result.SecondaryIntents = detectSecondaryIntents(lower, result.Secondary)
	result.NeedDecomposition = len(result.SecondaryIntents) > 0
	result.RouteTrace = RouteTrace{
		Source:          "rule",
		MatchedKeywords: matchedKeywords(lower, result.Secondary),
		Reasoning:       ruleReasoning(result.Secondary, result.SecondaryIntents),
		ConfidenceBasis: confidenceBasis(result.Confidence, entities, result.NeedClarify),
	}

	if needsProduct(result.Secondary) && entities["models"] == "" &&
		result.Secondary != PurchaseRecommendation && result.Secondary != Troubleshooting {
		if canResolveFromPurchaseHistory(lower, result.Secondary) {
			result.NeedClarify = false
			result.Confidence = min(result.Confidence, 0.82)
		} else if referencesPriorProduct(lower) && hasPriorProduct(request) {
			result.NeedClarify = false
			result.Confidence = min(result.Confidence, 0.72)
		} else {
			result.NeedClarify = true
			result.Confidence = min(result.Confidence, 0.62)
		}
	}
	annotateCompetitor(query, &result)
	return result, nil
}

func detectSecondaryIntents(query string, primary Type) []Type {
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
	add(Troubleshooting, containsAny(query, "充不进电", "无法充电", "异响", "漏水", "冒烟", "报错"))
	add(ProductComparison, containsAny(query, "对比", "比较", "区别", "哪个好", "哪个更"))
	add(PurchaseRecommendation, containsAny(query, "推荐", "怎么选", "选哪", "预算"))
	add(ProductParameter, containsAny(query, "吸力", "噪声", "噪音", "cadr", "面积", "水箱", "功率", "参数", "规格"))
	add(AccessoryCompatibility, containsAny(query, "兼容", "能装", "适配", "滤芯", "尘袋", "滚刷", "边刷"))
	add(UsageInstruction, containsAny(query, "怎么用", "如何", "安装", "清洁", "配网", "联网", "重置"))
	add(PriceQuery, containsAny(query, "多少钱", "价格", "售价", "优惠", "券"))
	add(InventoryQuery, containsAny(query, "库存", "现货", "有货"))
	add(OrderQuery, containsAny(query, "订单", "购买记录", "什么时候买"))
	add(WarrantyQuery, containsAny(query, "保修", "在保", "保修期"))
	add(ReturnEligibility, containsAny(query, "退货", "换货", "还能退", "能退吗"))
	add(CreateAfterSalesTicket, containsAny(query, "创建工单", "售后工单", "申请维修", "帮我报修"))
	return candidates
}

func matchedKeywords(query string, intentType Type) []string {
	keywords := map[Type][]string{
		ProductParameter:       {"吸力", "噪声", "噪音", "cadr", "面积", "水箱", "功率", "参数", "规格"},
		ProductComparison:      {"对比", "比较", "区别", "哪个好", "哪个更"},
		PurchaseRecommendation: {"推荐", "怎么选", "选哪", "预算"},
		AccessoryCompatibility: {"兼容", "能装", "适配", "滤芯", "尘袋", "滚刷", "边刷"},
		UsageInstruction:       {"怎么用", "如何", "安装", "清洁", "配网", "联网", "重置"},
		PriceQuery:             {"多少钱", "价格", "售价", "优惠", "券"},
		InventoryQuery:         {"库存", "现货", "有货"},
		OrderQuery:             {"订单", "购买记录", "什么时候买"},
		WarrantyQuery:          {"保修", "在保", "保修期"},
		ReturnEligibility:      {"退货", "换货", "还能退", "能退吗"},
		Troubleshooting:        {"充不进电", "无法充电", "异响", "漏水", "冒烟", "报错"},
		CreateAfterSalesTicket: {"创建工单", "售后工单", "申请维修", "帮我报修"},
		OutOfScope:             {"手机", "衣服", "食品", "生鲜", "发票", "投诉"},
		Chitchat:               {"你好", "您好", "谢谢", "再见"},
	}
	var matched []string
	for _, keyword := range keywords[intentType] {
		if strings.Contains(query, keyword) {
			matched = append(matched, keyword)
		}
	}
	return matched
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
		return "关键信息缺失或指代无法解析，降低置信度"
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
		containsAny(query, "上周买", "之前买", "我买的", "购买记录")
}

func extractEntities(query string) map[string]string {
	entities := map[string]string{}
	orderNo := strings.ToUpper(orderNumberPattern.FindString(query))
	if models := modelPattern.FindAllString(query, -1); len(models) > 0 {
		filtered := models[:0]
		for index := range models {
			models[index] = strings.Join(strings.Fields(models[index]), " ")
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
	return entities
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
