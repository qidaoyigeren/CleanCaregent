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
	"CleanCaregent/internal/model"
	"CleanCaregent/internal/platform/id"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/skill"
	"CleanCaregent/internal/tool"
)

var (
	ErrSkillNotFound = errors.New("skill not found")
	orderPattern     = regexp.MustCompile(`(?i)\b(?:CC|ORDER)[0-9]{6,}\b`)
	modelPattern     = regexp.MustCompile(`(?i)\b[A-Z]+[0-9]+(?:\s*Pro)?\b`)
)

type DynamicExecutor struct {
	tools             *tool.Executor
	skills            *skill.Registry
	argumentExtractor ArgumentExtractor
	retriever         rag.Retriever
}

type Option func(*DynamicExecutor)

func WithArgumentExtractor(extractor ArgumentExtractor) Option {
	return func(executor *DynamicExecutor) {
		executor.argumentExtractor = extractor
	}
}

func WithKnowledgeRetriever(retriever rag.Retriever) Option {
	return func(executor *DynamicExecutor) {
		executor.retriever = retriever
	}
}

func NewDynamicExecutor(tools *tool.Executor, skills *skill.Registry, options ...Option) *DynamicExecutor {
	executor := &DynamicExecutor{tools: tools, skills: skills}
	for _, option := range options {
		option(executor)
	}
	return executor
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
	arguments := e.buildArguments(ctx, request)
	logicalToolName := tool.LogicalName(request.Step.ToolName)
	call := tool.Call{
		TraceID:        request.Request.TraceID,
		CallID:         id.New("call"),
		UserID:         request.Request.UserID,
		ConversationID: request.Request.ConversationID,
		Name:           request.Step.ToolName,
		Arguments:      arguments,
		IdempotencyKey: request.Request.TraceID + ":" + request.Step.ToolName,
	}
	whitelist := request.AllowedTools
	if len(whitelist) == 0 {
		whitelist = allowedTools(intent.Type(request.Intent))
	}
	result, err := e.tools.Execute(ctx, call, whitelist)
	if err != nil {
		if result.ErrorCode == "TOOL_TIMEOUT" || errors.Is(err, context.DeadlineExceeded) {
			return e.degradeToKnowledge(ctx, request, call, result)
		}
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
	if validationErr := tool.ValidateResult(call.Name, result.Data); validationErr != nil {
		return agent.DynamicExecutionResult{
			Answer: "实时业务数据返回异常，本次结果未用于回答。请稍后重试或联系人工客服核实。",
			Evidences: []agent.Evidence{{
				Kind:     "tool_error",
				SourceID: call.CallID,
				Title:    toolEvidenceTitle(logicalToolName),
				Content:  validationErr.Error(),
				Metadata: map[string]any{"error_code": "INVALID_TOOL_RESULT"},
			}},
			Metadata: map[string]any{"degraded": true, "tool_name": call.Name},
		}, nil
	}
	raw, _ := json.Marshal(result.Data)
	return agent.DynamicExecutionResult{
		Answer: formatToolAnswer(logicalToolName, result.Data, result.DataScope),
		Evidences: []agent.Evidence{{
			Kind:     "tool_result",
			SourceID: call.CallID,
			Title:    toolEvidenceTitle(logicalToolName),
			Content:  string(raw),
			Metadata: map[string]any{
				"tool_name":         call.Name,
				"logical_tool_name": logicalToolName,
				"data_scope":        result.DataScope,
				"finished_at":       result.FinishedAt,
			},
		}},
		Metadata: map[string]any{
			"tool_name":         call.Name,
			"logical_tool_name": logicalToolName,
			"data_scope":        result.DataScope,
		},
	}, nil
}

func formatToolAnswer(name string, data any, dataScope ...string) string {
	name = tool.LogicalName(name)
	raw, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	label := dynamicDataLabel(firstString(dataScope))
	switch name {
	case "price_query":
		var payload struct {
			Items []model.PriceQuote `json:"items"`
		}
		if json.Unmarshal(raw, &payload) != nil || len(payload.Items) == 0 {
			return ""
		}
		var builder strings.Builder
		fmt.Fprintf(&builder, "**%s价格**\n", label)
		for _, item := range payload.Items {
			fmt.Fprintf(
				&builder,
				"- %s（%s）：当前价 %s 元，优惠后预估 %s 元，库存 %d 件。\n",
				item.ProductName,
				item.SKUName,
				formatCents(item.CurrentPriceCents),
				formatCents(item.EstimatedFinalPriceCents),
				item.AvailableStock,
			)
		}
		return strings.TrimSpace(builder.String())
	case "inventory_check":
		var payload struct {
			Items []model.ProductSKU `json:"items"`
		}
		if json.Unmarshal(raw, &payload) != nil || len(payload.Items) == 0 {
			return ""
		}
		var builder strings.Builder
		fmt.Fprintf(&builder, "**%s库存**\n", label)
		for _, item := range payload.Items {
			fmt.Fprintf(
				&builder,
				"- %s（%s）：可售库存 %d 件，当前价 %s 元。\n",
				item.ProductName,
				item.SKUName,
				item.AvailableStock,
				formatCents(item.CurrentPriceCents),
			)
		}
		return strings.TrimSpace(builder.String())
	case "order_lookup":
		var order model.OrderDetail
		if json.Unmarshal(raw, &order) != nil || order.OrderNo == "" {
			return ""
		}
		productNames := make([]string, 0, len(order.Items))
		for _, item := range order.Items {
			productNames = append(productNames, item.ProductName)
		}
		return fmt.Sprintf(
			"**订单状态**\n订单 %s 当前状态：%s；商品：%s；订单金额：%s 元。",
			order.OrderNo,
			order.Status,
			strings.Join(productNames, "、"),
			formatCents(order.TotalAmountCents),
		)
	case "create_after_sales_ticket":
		var ticket model.AfterSalesTicket
		if json.Unmarshal(raw, &ticket) != nil || ticket.TicketNo == "" {
			return ""
		}
		return fmt.Sprintf(
			"**%s售后工单**\n已按您的明确确认创建售后工单。订单号：%s；工单号：%s；当前状态：%s。本次提交已使用幂等键防止重复建单。",
			label,
			ticket.OrderNo,
			ticket.TicketNo,
			ticket.Status,
		)
	default:
		return ""
	}
}

func dynamicDataLabel(dataScope string) string {
	switch strings.ToLower(strings.TrimSpace(dataScope)) {
	case "external":
		return "实时"
	case "sandbox":
		return "沙箱动态"
	default:
		return "模拟动态"
	}
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func formatCents(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
}

func (e *DynamicExecutor) executeSkill(
	ctx context.Context,
	request agent.DynamicExecutionRequest,
) (agent.DynamicExecutionResult, error) {
	if e.skills == nil {
		return agent.DynamicExecutionResult{}, ErrSkillNotFound
	}
	nextSkill := request.Step.SkillName
	nextArgs := cloneMap(request.Step.Params)
	visited := make(map[string]struct{}, 3)
	var (
		answer    string
		evidences []agent.Evidence
		metadata  = map[string]any{}
	)
	for chainIndex := 0; chainIndex < 3 && nextSkill != ""; chainIndex++ {
		if _, exists := visited[nextSkill]; exists {
			return agent.DynamicExecutionResult{}, fmt.Errorf("Skill 链路存在循环: %s", nextSkill)
		}
		visited[nextSkill] = struct{}{}
		value, ok := e.skills.Get(nextSkill)
		if !ok {
			return agent.DynamicExecutionResult{}, fmt.Errorf("%w: %s", ErrSkillNotFound, nextSkill)
		}
		targetIntent := intent.Type(request.Intent)
		rawTarget, _ := nextArgs["target_intent"].(string)
		if rawTarget = strings.TrimSpace(rawTarget); rawTarget != "" {
			targetIntent = intent.Type(rawTarget)
		}
		secondaryIntents := make([]intent.Type, 0, len(request.SecondaryIntents))
		for _, value := range request.SecondaryIntents {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				secondaryIntents = append(secondaryIntents, intent.Type(trimmed))
			}
		}
		entities := stringMap(nextArgs)
		delete(entities, "target_intent")
		intentResult := intent.Result{
			Secondary:        targetIntent,
			SecondaryIntents: secondaryIntents,
			Entities:         entities,
		}
		result, err := value.Run(ctx, skill.Request{
			TraceID:        request.Request.TraceID,
			UserID:         request.Request.UserID,
			ConversationID: request.Request.ConversationID,
			Query:          request.Step.Query,
			Intent:         intentResult,
			Entities:       nextArgs,
		})
		if err != nil {
			return agent.DynamicExecutionResult{}, err
		}
		evidences = append(evidences, result.Evidences...)
		for key, value := range result.Metadata {
			metadata[key] = value
		}
		metadata["skill_status"] = result.Status
		metadata["skill_chain_length"] = chainIndex + 1
		metadata["intentional_clarification"] =
			result.Status == "clarify" && result.NextQuestion != ""
		answer = result.AnswerDraft
		if result.NextQuestion != "" {
			answer = result.NextQuestion
		}
		if result.Status == "clarify" || result.NextSkill == "" {
			nextSkill = ""
			break
		}
		nextSkill = result.NextSkill
		nextArgs = cloneMap(result.NextSkillArgs)
		for key, value := range request.Step.Params {
			if _, exists := nextArgs[key]; !exists {
				nextArgs[key] = value
			}
		}
	}
	if nextSkill != "" {
		return agent.DynamicExecutionResult{}, errors.New("Skill 链路超过最大 3 步")
	}
	return agent.DynamicExecutionResult{
		Answer:    answer,
		Evidences: evidences,
		Metadata:  metadata,
	}, nil
}

func (e *DynamicExecutor) buildArguments(
	ctx context.Context,
	request agent.DynamicExecutionRequest,
) map[string]any {
	arguments := cloneMap(request.Step.Params)
	query := request.Request.Query
	logicalToolName := tool.LogicalName(request.Step.ToolName)
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
		models = productRefsForDynamicTool(logicalToolName, query, models)
		arguments["product_refs"] = models
		arguments["model"] = strings.Join(strings.Fields(models[0]), " ")
	}
	if orderNo != "" {
		arguments["order_no"] = orderNo
	}
	switch logicalToolName {
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
		arguments["confirmed"] = ticketConfirmationPresent(query)
	}
	arguments = normalizeToolArguments(arguments)
	if e.argumentExtractor != nil && needsArgumentExtraction(logicalToolName, arguments) {
		if extracted, err := e.argumentExtractor.Extract(ctx, logicalToolName, query); err == nil {
			for key, value := range normalizeExtractedArguments(extracted) {
				if _, exists := arguments[key]; !exists || arguments[key] == "" {
					arguments[key] = value
				}
			}
		}
	}
	return normalizeToolArguments(arguments)
}

func productRefsForDynamicTool(toolName, query string, refs []string) []string {
	switch tool.LogicalName(toolName) {
	case "price_query", "inventory_check":
	default:
		return refs
	}
	if !containsAny(query, "配件", "耗材", "滤芯", "滚刷", "尘袋", "边刷") {
		return refs
	}
	accessories := make([]string, 0, len(refs))
	for _, ref := range refs {
		normalized := normalizeProductRef(ref)
		if accessoryPattern.MatchString(normalized) {
			accessories = append(accessories, normalized)
		}
	}
	if len(accessories) > 0 {
		return accessories
	}
	return refs
}

func ticketConfirmationPresent(query string) bool {
	if containsAny(query, "没确认", "未确认", "没有确认", "不确认", "不要创建") {
		return false
	}
	return containsAny(query, "我确认", "确认创建", "确认提交", "确认给", "确认了")
}

func (e *DynamicExecutor) degradeToKnowledge(
	ctx context.Context,
	request agent.DynamicExecutionRequest,
	call tool.Call,
	result tool.Result,
) (agent.DynamicExecutionResult, error) {
	if e.retriever == nil {
		return agent.DynamicExecutionResult{
			Answer: "实时服务超时，当前无法确认最新动态数据。请稍后重试。",
			Evidences: []agent.Evidence{{
				Kind: "tool_error", SourceID: call.CallID, Title: toolEvidenceTitle(call.Name),
				Content: result.Message, Metadata: map[string]any{"error_code": result.ErrorCode},
			}},
			Metadata: map[string]any{"degraded": true, "degrade_strategy": "tool_timeout"},
		}, nil
	}
	items, err := e.retriever.Search(ctx, rag.SearchRequest{
		Query: request.Request.Query,
		Mode:  rag.SearchHybrid,
		Filter: rag.MetadataFilter{
			Models:   stringSliceArgument(call.Arguments["product_refs"], call.Arguments["model"]),
			DocTypes: fallbackDocTypes(call.Name),
		},
		DenseTopK: 8, KeywordTopK: 8, RerankTopK: 4, NeedRerank: true,
	})
	if err != nil {
		return agent.DynamicExecutionResult{}, fmt.Errorf("实时工具和知识库降级均失败: %w", err)
	}
	evidences := make([]agent.Evidence, 0, len(items)+1)
	evidences = append(evidences, agent.Evidence{
		Kind: "tool_error", SourceID: call.CallID, Title: toolEvidenceTitle(call.Name),
		Content: result.Message, Metadata: map[string]any{"error_code": result.ErrorCode},
	})
	for _, item := range items {
		evidences = append(evidences, agent.Evidence{
			Kind: "kb_chunk", SourceID: item.ChunkID, Title: item.Title, Content: item.Content,
			Metadata: item.Metadata,
		})
	}
	return agent.DynamicExecutionResult{
		Answer:     "",
		Evidences:  evidences,
		SearchData: items,
		Metadata: map[string]any{
			"degraded": true, "degrade_strategy": "tool_timeout_to_kb",
		},
	}, nil
}

func validateToolData(name string, data any) error {
	name = tool.LogicalName(name)
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
			if numberValue(item["current_price_cents"]) <= 0 ||
				numberValue(item["estimated_final_price_cents"]) <= 0 {
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
	switch tool.LogicalName(name) {
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
