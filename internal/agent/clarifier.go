package agent

import (
	"context"
	"strings"

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
	return c.clarifyWithRules(query, intentType, knownEntities, missingInfo)
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
func (c *Clarifier) clarifyWithRules(
	query string,
	intentType intent.Type,
	knownEntities map[string]string,
	missingInfo []string,
) string {
	hasModel := knownEntities["models"] != ""
	hasOrder := knownEntities["order_no"] != ""

	if containsMissing(missingInfo, "比较型号") {
		return "请问您想对比哪两款产品？例如 T20 和 X20 Pro。也请告诉我最关注的是价格、清洁能力、噪声还是维护成本。"
	}
	if containsMissing(missingInfo, "参数含义") {
		return "您说的“够不够用”具体是指清洁面积、单次续航、净化能力、出水速度还是加湿时长？确认维度后我才能按对应参数判断。"
	}
	if containsMissing(missingInfo, "意图") {
		return "您是想查产品参数、比较型号、获取选购推荐，还是处理故障或售后问题？请告诉我最想先解决的一项。"
	}
	if containsMissing(missingInfo, "用户确认") {
		return "创建售后工单会写入一条新的售后记录。请核对订单号和问题描述后，明确回复“确认创建售后工单”；未确认前我不会执行创建。"
	}
	if containsMissing(missingInfo, "产品型号") {
		if containsAmbiguousReference(query) {
			return "您提到的“那款/这台”具体是哪款产品？例如扫地机器人是 T20 还是 X20 Pro，净化器是 P400 还是 P500。型号不同，参数和配件不能混用。"
		}
		return "请问您使用的是哪款产品？目前型号包括 T20、X20 Pro、R10、R20、P400、P500、W300、W500、H100、H200。型号通常可在机身铭牌或订单中找到。"
	}

	switch intentType {
	case intent.ProductParameter, intent.PriceQuery, intent.InventoryQuery, intent.AccessoryCompatibility:
		if !hasModel {
			return "请问您使用的是哪款产品？目前型号包括 T20、X20 Pro、R10、R20、P400、P500、W300、W500、H100、H200。型号通常可在机身铭牌或订单中找到。"
		}
		return "请补充您想了解的具体信息，比如是想了解参数、价格还是兼容配件？"

	case intent.PurchaseRecommendation:
		return "为了筛选合适产品，请补充使用面积、预算，以及宠物、地毯、噪声敏感或安装空间等至少一项关键条件。"

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

	case intent.AfterSalesStatus:
		if !hasOrder {
			return "查询退款、退货、换货或维修进度前，请提供订单号。拿到订单号后，我会只查询当前账号下的售后状态。"
		}
		return "请说明您想查的是退款/退货进度，还是维修/售后工单进度。"

	case intent.HumanHandoff:
		return "可以为您转人工。请明确回复“转人工客服”，我会把当前对话上下文排入人工接管队列。"

	case intent.OrderQuery:
		return "我需要确认一下：\n1. 您的订单号是多少？（可以在 APP 的'我的订单'里找到）\n2. 或者您可以告诉我大概什么时候购买的、是什么产品，我帮您查一下记录。"

	case intent.CreateAfterSalesTicket:
		if !hasOrder {
			return "创建售后工单前请提供订单号和具体问题描述。信息核对完成后，还需要您明确确认创建。"
		}
		return "请核对订单号和问题描述后，明确回复“确认创建售后工单”；未确认前我不会执行创建。"

	default:
		return "请补充具体型号、品类或您希望查询的内容，这样我才能更好地为您服务。"
	}
}

func containsMissing(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsAmbiguousReference(query string) bool {
	for _, marker := range []string{"那个", "那款", "这个", "这台", "它"} {
		if strings.Contains(query, marker) {
			return true
		}
	}
	return false
}
