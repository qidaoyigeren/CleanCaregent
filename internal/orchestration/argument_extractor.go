package orchestration

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/tool"
)

type ArgumentExtractor interface {
	Extract(ctx context.Context, toolName, query string) (map[string]any, error)
}

type LLMArgumentExtractor struct {
	client *llm.Client
}

func NewLLMArgumentExtractor(client *llm.Client) *LLMArgumentExtractor {
	return &LLMArgumentExtractor{client: client}
}

func (e *LLMArgumentExtractor) Extract(
	ctx context.Context,
	toolName string,
	query string,
) (map[string]any, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("LLM 参数提取器未配置")
	}
	messages := []map[string]string{
		{
			"role": "system",
			"content": `你是清洁工具销售场景的工具参数提取器。只输出 JSON 对象。
允许字段：product_refs(string[]), model(string), order_no(string), category(string), confirmed(boolean)。
不得猜测型号或订单号；无法确认的字段不要输出。`,
		},
		{
			"role":    "user",
			"content": fmt.Sprintf("工具：%s\n用户原话：%s", toolName, query),
		},
	}
	var output map[string]any
	if err := e.client.ChatJSON(ctx, messages, &output); err != nil {
		return nil, fmt.Errorf("LLM 参数提取失败: %w", err)
	}
	return output, nil
}

var accessoryPattern = regexp.MustCompile(`(?i)^(?:F|DB|RB|C)[0-9]{2,}[A-Z0-9-]*$`)
var productRefPattern = regexp.MustCompile(`(?i)^[A-Z]{1,8}[0-9]{1,5}(?:PRO)?(?:-[A-Z0-9]{1,12})?$`)

func needsArgumentExtraction(toolName string, arguments map[string]any) bool {
	switch tool.LogicalName(toolName) {
	case "price_query", "inventory_check":
		return len(stringSliceArgument(arguments["product_refs"], arguments["model"])) == 0
	case "order_lookup", "warranty_check", "create_after_sales_ticket":
		return strings.TrimSpace(fmt.Sprint(arguments["order_no"])) == ""
	default:
		return false
	}
}

func normalizeExtractedArguments(source map[string]any) map[string]any {
	result := make(map[string]any)
	knownProducts := map[string]struct{}{
		"T20": {}, "X20 PRO": {}, "R10": {}, "R20": {}, "P400": {},
		"P500": {}, "W300": {}, "W500": {}, "H100": {}, "H200": {},
	}
	var products []string
	for _, value := range stringSliceArgument(source["product_refs"], source["model"]) {
		normalized := normalizeProductRef(value)
		compact := strings.ToUpper(strings.Join(strings.Fields(normalized), ""))
		if _, ok := knownProducts[normalized]; ok || accessoryPattern.MatchString(normalized) || productRefPattern.MatchString(compact) {
			products = append(products, normalized)
		}
	}
	if len(products) > 0 {
		result["product_refs"] = products
		result["model"] = products[0]
	}
	if orderNo := strings.ToUpper(strings.TrimSpace(fmt.Sprint(source["order_no"]))); orderPattern.MatchString(orderNo) {
		result["order_no"] = orderNo
	}
	if category := strings.TrimSpace(fmt.Sprint(source["category"])); category != "" {
		result["category"] = category
	}
	return result
}

func normalizeProductRef(value string) string {
	normalized := strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(value)), ""))
	if normalized == "X20PRO" {
		return "X20 PRO"
	}
	return normalized
}

func normalizeToolArguments(source map[string]any) map[string]any {
	result := cloneMap(source)
	_, hasProducts := source["product_refs"]
	_, hasModel := source["model"]
	_, hasOrder := source["order_no"]
	normalized := normalizeExtractedArguments(source)
	if hasProducts {
		delete(result, "product_refs")
	}
	if hasModel {
		delete(result, "model")
	}
	if hasOrder {
		delete(result, "order_no")
	}
	for key, value := range normalized {
		result[key] = value
	}
	return result
}

func stringSliceArgument(values ...any) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, value := range values {
		switch typed := value.(type) {
		case []string:
			for _, item := range typed {
				for _, split := range splitArgumentText(item) {
					if _, ok := seen[split]; !ok {
						seen[split] = struct{}{}
						result = append(result, split)
					}
				}
			}
		case []any:
			for _, item := range typed {
				for _, split := range splitArgumentText(fmt.Sprint(item)) {
					if _, ok := seen[split]; !ok {
						seen[split] = struct{}{}
						result = append(result, split)
					}
				}
			}
		case string:
			for _, split := range splitArgumentText(typed) {
				if _, ok := seen[split]; !ok {
					seen[split] = struct{}{}
					result = append(result, split)
				}
			}
		}
	}
	return result
}

func splitArgumentText(value string) []string {
	var result []string
	for _, item := range strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', '，', '、', '/', '|', ';', '；', '\n', '\t':
			return true
		default:
			return false
		}
	}) {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func fallbackDocTypes(toolName string) []string {
	switch tool.LogicalName(toolName) {
	case "price_query", "inventory_check":
		return []string{"product_detail", "product_parameter", "faq"}
	case "order_lookup", "warranty_check", "create_after_sales_ticket":
		return []string{"after_sales_policy", "faq"}
	default:
		return []string{"faq"}
	}
}
