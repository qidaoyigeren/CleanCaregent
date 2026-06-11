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
		result = Result{Primary: "fallback", Secondary: Clarification, Confidence: 1, NeedClarify: true}
	case containsAny(lower, "手机", "衣服", "食品", "生鲜", "发票", "投诉"):
		result.Primary, result.Secondary, result.Confidence = "fallback", OutOfScope, 0.98
	case containsAny(lower, "你好", "您好", "谢谢", "再见") && len([]rune(query)) <= 12:
		result.Primary, result.Secondary, result.Confidence = "fallback", Chitchat, 0.98
	case containsAny(lower, "创建工单", "售后工单", "帮我报修", "申请维修", "转人工"):
		result.Primary, result.Secondary = "aftersales", CreateAfterSalesTicket
	case containsAny(lower, "充不进电", "无法充电", "不充电", "异响", "故障", "报错", "不工作", "漏水", "冒烟",
		"问题仍在", "下一步", "传感器", "数值异常"):
		result.Primary, result.Secondary = "diagnosis", Troubleshooting
	case containsAny(lower, "还能退", "能退吗", "退货", "换货", "无理由"):
		result.Primary, result.Secondary = "aftersales", ReturnEligibility
	case containsAny(lower, "保修", "在保", "保修期"):
		result.Primary, result.Secondary = "aftersales", WarrantyQuery
	case containsAny(lower, "滤芯", "尘袋", "滚刷", "边刷", "配件") &&
		containsAny(lower, "上周买", "之前买", "我买的", "购买记录") &&
		containsAny(lower, "多少钱", "价格", "售价", "优惠", "券"):
		result.Primary, result.Secondary = "presales", AccessoryCompatibility
	case containsAny(lower, "订单", "买的哪", "购买记录", "上周买", "什么时候买"):
		result.Primary, result.Secondary = "aftersales", OrderQuery
	case containsAny(lower, "库存", "现货", "有货"):
		result.Primary, result.Secondary = "presales", InventoryQuery
	case containsAny(lower, "多少钱", "价格", "售价", "优惠", "券"):
		result.Primary, result.Secondary = "presales", PriceQuery
	case containsAny(lower, "首次使用", "使用前", "重新安装", "重新联网", "怎么冲洗", "出水变小",
		"怎么清洁", "应该加什么水", "睡眠模式", "多久换", "滤芯寿命", "配网", "清洁地毯"):
		result.Primary, result.Secondary = "presales", UsageInstruction
	case containsAny(lower, "兼容", "能装", "适配", "滤芯", "尘袋", "滚刷", "边刷", "配件"):
		result.Primary, result.Secondary = "presales", AccessoryCompatibility
	case containsAny(lower, "对比", "比较", "区别", "哪个好", "哪个更"):
		result.Primary, result.Secondary = "presales", ProductComparison
	case containsAny(lower, "推荐", "怎么选", "选哪", "选什么", "选多大", "适合我", "预算"):
		result.Primary, result.Secondary = "presales", PurchaseRecommendation
	case containsAny(lower, "怎么用", "如何使用", "重置", "联网", "安装", "清洁方法", "更换方法"):
		result.Primary, result.Secondary = "presales", UsageInstruction
	case containsAny(lower, "吸力", "噪声", "噪音", "cadr", "面积", "水箱", "功率", "参数", "规格"):
		result.Primary, result.Secondary = "presales", ProductParameter
	default:
		result.Primary, result.Secondary, result.Confidence = "fallback", Clarification, 0.52
		result.NeedClarify = true
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
	return result, nil
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
