package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"CleanCaregent/internal/agent"
	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/platform/id"
	"CleanCaregent/internal/skill"
	"CleanCaregent/internal/tool"
)

var (
	ErrSkillNotFound = errors.New("skill not found")
	orderPattern     = regexp.MustCompile(`(?i)\b(?:CC|ORDER)[0-9]{6,}\b`)
	modelPattern     = regexp.MustCompile(`(?i)\b[A-Z]+[0-9]+(?:\s+Pro)?\b`)
)

type DynamicExecutor struct {
	tools  *tool.Executor
	skills *skill.Registry
}

func NewDynamicExecutor(tools *tool.Executor, skills *skill.Registry) *DynamicExecutor {
	return &DynamicExecutor{tools: tools, skills: skills}
}

func (e *DynamicExecutor) Execute(
	ctx context.Context,
	request agent.DynamicExecutionRequest,
) (agent.DynamicExecutionResult, error) {
	switch request.Step.Action {
	case agent.ActionCallTool:
		return e.executeTool(ctx, request)
	case agent.ActionRunSkill:
		return e.executeSkill(ctx, request)
	default:
		return agent.DynamicExecutionResult{}, fmt.Errorf("unsupported dynamic action %s", request.Step.Action)
	}
}

func (e *DynamicExecutor) executeTool(
	ctx context.Context,
	request agent.DynamicExecutionRequest,
) (agent.DynamicExecutionResult, error) {
	if e.tools == nil {
		return agent.DynamicExecutionResult{
			Answer:   "动态业务服务暂时不可用，请稍后重试。",
			Metadata: map[string]any{"degraded": true},
		}, nil
	}
	arguments := buildArguments(request)
	call := tool.Call{
		TraceID:        request.Request.TraceID,
		CallID:         id.New("call"),
		UserID:         request.Request.UserID,
		ConversationID: request.Request.ConversationID,
		Name:           request.Step.ToolName,
		Arguments:      arguments,
		IdempotencyKey: request.Request.TraceID + ":" + request.Step.ToolName,
	}
	result, err := e.tools.Execute(ctx, call, allowedTools(intent.Type(request.Intent)))
	if err != nil {
		return agent.DynamicExecutionResult{
			Answer: "实时数据查询失败，已保留本次调用记录。请核对型号或订单号后重试。",
			Evidences: []agent.Evidence{{
				Kind:     "tool_error",
				SourceID: call.CallID,
				Title:    call.Name,
				Content:  result.Message,
				Metadata: map[string]any{"error_code": result.ErrorCode},
			}},
			Metadata: map[string]any{"degraded": true},
		}, nil
	}
	if validationErr := validateToolData(call.Name, result.Data); validationErr != nil {
		return agent.DynamicExecutionResult{
			Answer: "实时业务数据返回异常，本次结果未用于回答。请稍后重试或联系人工客服核实。",
			Evidences: []agent.Evidence{{
				Kind:     "tool_error",
				SourceID: call.CallID,
				Title:    toolEvidenceTitle(call.Name),
				Content:  validationErr.Error(),
				Metadata: map[string]any{"error_code": "INVALID_TOOL_RESULT"},
			}},
			Metadata: map[string]any{"degraded": true, "tool_name": call.Name},
		}, nil
	}
	raw, _ := json.Marshal(result.Data)
	return agent.DynamicExecutionResult{
		// A successful tool result is an observation, not a user-facing answer.
		// The answer generator consumes this evidence at the finish step.
		Answer: "",
		Evidences: []agent.Evidence{{
			Kind:     "tool_result",
			SourceID: call.CallID,
			Title:    toolEvidenceTitle(call.Name),
			Content:  string(raw),
			Metadata: map[string]any{
				"tool_name":   call.Name,
				"finished_at": result.FinishedAt,
			},
		}},
		Metadata: map[string]any{"tool_name": call.Name},
	}, nil
}

func (e *DynamicExecutor) executeSkill(
	ctx context.Context,
	request agent.DynamicExecutionRequest,
) (agent.DynamicExecutionResult, error) {
	if e.skills == nil {
		return agent.DynamicExecutionResult{}, ErrSkillNotFound
	}
	value, ok := e.skills.Get(request.Step.SkillName)
	if !ok {
		return agent.DynamicExecutionResult{}, fmt.Errorf("%w: %s", ErrSkillNotFound, request.Step.SkillName)
	}
	intentResult := intent.Result{
		Secondary: intent.Type(request.Intent),
		Entities:  stringMap(request.Step.Params),
	}
	result, err := value.Run(ctx, skill.Request{
		TraceID:        request.Request.TraceID,
		UserID:         request.Request.UserID,
		ConversationID: request.Request.ConversationID,
		Query:          request.Step.Query,
		Intent:         intentResult,
		Entities:       request.Step.Params,
	})
	if err != nil {
		return agent.DynamicExecutionResult{}, err
	}
	answer := result.AnswerDraft
	if result.NextQuestion != "" {
		answer = result.NextQuestion
	}
	metadata := cloneMap(result.Metadata)
	metadata["skill_status"] = result.Status
	metadata["intentional_clarification"] =
		result.Status == "clarify" && result.NextQuestion != ""
	return agent.DynamicExecutionResult{
		Answer:    answer,
		Evidences: result.Evidences,
		Metadata:  metadata,
	}, nil
}

func buildArguments(request agent.DynamicExecutionRequest) map[string]any {
	arguments := cloneMap(request.Step.Params)
	query := request.Request.Query
	models := modelPattern.FindAllString(query, -1)
	orderNo := strings.ToUpper(orderPattern.FindString(query))
	filteredModels := models[:0]
	for _, modelName := range models {
		if !strings.EqualFold(modelName, orderNo) {
			filteredModels = append(filteredModels, modelName)
		}
	}
	models = filteredModels
	if len(models) > 0 {
		arguments["product_refs"] = models
		arguments["model"] = strings.Join(strings.Fields(models[0]), " ")
	}
	if orderNo != "" {
		arguments["order_no"] = orderNo
	}
	switch request.Step.ToolName {
	case "user_purchase_history":
		arguments["limit"] = 10
		if strings.Contains(query, "上周") || strings.Contains(query, "最近") {
			arguments["since"] = time.Now().UTC().AddDate(0, 0, -14).Format(time.RFC3339)
		}
		if strings.Contains(query, "净化器") {
			arguments["category"] = "air_purifier"
		}
	case "create_after_sales_ticket":
		arguments["issue_type"] = "repair"
		arguments["description"] = query
		arguments["confirmed"] = containsAny(query, "确认", "创建", "报修", "申请维修", "提交工单")
	}
	return arguments
}

func formatToolAnswer(name string, data any) string {
	raw, _ := json.MarshalIndent(data, "", "  ")
	switch name {
	case "price_query":
		return "实时价格与可用优惠如下：\n```json\n" + string(raw) + "\n```"
	case "inventory_check":
		return "当前 mock 库存如下：\n```json\n" + string(raw) + "\n```"
	case "user_purchase_history":
		return "查询到的购买记录如下：\n```json\n" + string(raw) + "\n```"
	case "order_lookup":
		return "订单信息如下：\n```json\n" + string(raw) + "\n```"
	case "warranty_check":
		return "保修判断如下：\n```json\n" + string(raw) + "\n```"
	case "create_after_sales_ticket":
		return "售后工单已创建：\n```json\n" + string(raw) + "\n```"
	default:
		return string(raw)
	}
}

func validateToolData(name string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode tool result: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode tool result: %w", err)
	}
	switch name {
	case "price_query":
		items, _ := payload["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if numberValue(item["current_price"]) <= 0 ||
				numberValue(item["estimated_final_price"]) <= 0 {
				return errors.New("price result contains a non-positive price")
			}
		}
	case "inventory_check":
		items, _ := payload["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if numberValue(item["available_stock"]) < 0 {
				return errors.New("inventory result contains negative stock")
			}
		}
	case "order_lookup":
		if strings.TrimSpace(fmt.Sprint(payload["order_no"])) == "" {
			return errors.New("order result is missing order_no")
		}
	case "create_after_sales_ticket":
		if strings.TrimSpace(fmt.Sprint(payload["ticket_no"])) == "" {
			return errors.New("ticket result is missing ticket_no")
		}
	}
	return nil
}

func numberValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}

func toolEvidenceTitle(name string) string {
	switch name {
	case "price_query":
		return "实时价格查询"
	case "inventory_check":
		return "实时库存查询"
	case "user_purchase_history":
		return "用户购买记录"
	case "order_lookup":
		return "订单信息"
	case "warranty_check":
		return "保修状态"
	case "create_after_sales_ticket":
		return "售后工单"
	default:
		return "动态业务数据"
	}
}

func allowedTools(intentType intent.Type) []string {
	switch intentType {
	case intent.PriceQuery:
		return []string{"price_query"}
	case intent.InventoryQuery:
		return []string{"inventory_check"}
	case intent.OrderQuery:
		return []string{"user_purchase_history", "order_lookup"}
	case intent.PurchaseRecommendation:
		return []string{"price_query", "inventory_check"}
	case intent.AccessoryCompatibility:
		return []string{"user_purchase_history", "price_query"}
	case intent.WarrantyQuery:
		return []string{"order_lookup", "warranty_check"}
	case intent.ReturnEligibility:
		return []string{"order_lookup"}
	case intent.Troubleshooting:
		return []string{"warranty_check", "create_after_sales_ticket"}
	case intent.CreateAfterSalesTicket:
		return []string{"order_lookup", "warranty_check", "create_after_sales_ticket"}
	default:
		return nil
	}
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+4)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func stringMap(source map[string]any) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		switch typed := value.(type) {
		case string:
			result[key] = typed
		case []string:
			result[key] = strings.Join(typed, ",")
		}
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
