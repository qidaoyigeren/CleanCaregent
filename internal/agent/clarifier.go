package agent

import (
	"context"

	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/prompt"
)

// Clarifier generates context-aware clarification questions when the system
// needs more information from the user.
type Clarifier struct {
	llm     *llm.Client
	prompts *prompt.Registry
}

// NewClarifier creates a clarifier. If llmClient is nil, degrades to
// canned clarification messages (see clarifyWithRules).
func NewClarifier(llmClient *llm.Client, prompts *prompt.Registry) *Clarifier {
	return &Clarifier{
		llm:     llmClient,
		prompts: prompts,
	}
}

// Clarify generates a clarification question tailored to the user's intent
// and the specific information that is missing.
func (c *Clarifier) Clarify(
	ctx context.Context,
	query string,
	intentType intent.Type,
	knownEntities map[string]string,
	missingInfo []string,
) string {
	// Try LLM-based clarification.
	if c.llm != nil && c.prompts != nil {
		if result, err := c.clarifyWithLLM(ctx, query, intentType, knownEntities, missingInfo); err == nil {
			return result
		}
	}
	// Fallback to rules.
	return c.clarifyWithRules(intentType, knownEntities)
}

func (c *Clarifier) clarifyWithLLM(
	ctx context.Context,
	query string,
	intentType intent.Type,
	knownEntities map[string]string,
	missingInfo []string,
) (string, error) {
	tmpl, err := c.prompts.Get(prompt.ScenarioClarify)
	if err != nil {
		return "", err
	}

	knownJSON := ""
	if len(knownEntities) > 0 {
		knownJSON = "已知信息：\n"
		for key, value := range knownEntities {
			knownJSON += "- " + key + ": " + value + "\n"
		}
	} else {
		knownJSON = "(尚无已知信息)"
	}

	missingJSON := ""
	if len(missingInfo) > 0 {
		for _, m := range missingInfo {
			missingJSON += "- " + m + "\n"
		}
	} else {
		missingJSON = "关键信息（型号/订单号/约束条件）缺失"
	}

	params := map[string]string{
		"intent_type":  string(intentType),
		"known_info":   knownJSON,
		"missing_info": missingJSON,
		"query":        query,
	}
	messages := tmpl.BuildMessages(params)

	return c.llm.Chat(ctx, messages)
}

// clarifyWithRules provides canned clarification messages per intent type
// when LLM is unavailable.
func (c *Clarifier) clarifyWithRules(intentType intent.Type, knownEntities map[string]string) string {
	hasModel := knownEntities["models"] != ""
	hasOrder := knownEntities["order_no"] != ""

	switch intentType {
	case intent.ProductParameter, intent.PriceQuery, intent.InventoryQuery, intent.AccessoryCompatibility:
		if !hasModel {
			return "为了帮您查询准确的参数/兼容信息，我需要确认一下：您具体是指哪个型号呢？\n常见的型号比如扫地机器人有 T20、X20 Pro，空气净化器有 P400、A500。\n（产品型号通常在机器底部标签或包装盒上可以看到）"
		}
		return "请补充您想了解的具体信息，比如是想了解参数、价格还是兼容配件？"

	case intent.PurchaseRecommendation:
		return "帮您推荐合适的清洁方案，我需要多了解一点您家的情况：\n1. 您家的面积大概多大？（比如 80平以下 / 80-120平 / 120平以上）\n2. 家里有宠物吗？或者有对毛发过敏的家人吗？\n3. 您的预算大概是多少呢？\n这样我才能给出最适合您的推荐 😊"

	case intent.Troubleshooting:
		if !hasModel {
			return "了解您的机器出了问题，我先帮您初步判断一下：\n1. 您使用的是哪个型号的机器呢？\n2. 机器现在是什么状态？（比如指示灯是否亮、有没有报错提示音）\n3. 出问题前您做过什么操作吗？\n您可以描述一下具体的现象，我会一步步帮您排查。"
		}
		return "请描述一下具体的故障现象：\n1. 机器的指示灯现在是什么状态？（正常亮 / 闪烁 / 完全不亮）\n2. 有没有报错提示音或错误代码？\n3. 出问题前您做过什么操作吗？（比如刚清洗完、移动了位置等）"

	case intent.WarrantyQuery, intent.ReturnEligibility:
		if !hasOrder {
			return "关于退换货/保修的问题，我需要确认几个关键信息：\n1. 您的订单号是多少？（可以在 APP 的'我的订单'里找到，格式通常是 CC开头+数字）\n2. 商品目前的使用状态如何？（未拆封 / 已拆封未使用 / 已使用一段时间）\n有了这些信息，我才能根据最新政策为您准确判断。"
		}
		return "请补充说明：您退换货/保修的具体原因是什么？（比如质量问题、不想要了、配件缺失等）"

	case intent.OrderQuery:
		return "我需要确认一下：\n1. 您的订单号是多少？（可以在 APP 的'我的订单'里找到）\n2. 或者您可以告诉我大概什么时候购买的、是什么产品，我帮您查一下记录。"

	default:
		return "请补充具体型号、品类或您希望查询的内容，这样我才能更好地为您服务。"
	}
}
