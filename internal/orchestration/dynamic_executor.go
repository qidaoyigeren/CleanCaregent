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
	"CleanCaregent/internal/compatibility"
	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/model"
	"CleanCaregent/internal/platform/id"
	"CleanCaregent/internal/policy"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/skill"
	"CleanCaregent/internal/tool"
)

var (
	ErrSkillNotFound = errors.New("skill not found")
	orderPattern     = regexp.MustCompile(`(?i)\b(?:CC|ORDER)[0-9]{6,}\b`)
	modelPattern     = regexp.MustCompile(`(?i)\b[A-Z]+[0-9]+(?:\s*Pro)?(?:-[A-Z0-9]+)?\b`)
)

type DynamicExecutor struct {
	tools             *tool.Executor
	skills            *skill.Registry
	argumentExtractor ArgumentExtractor
	retriever         rag.Retriever
	compatibility     *compatibility.Matrix
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

func WithCompatibilityMatrix(matrix *compatibility.Matrix) Option {
	return func(executor *DynamicExecutor) {
		executor.compatibility = matrix
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
	route := executionRoute(request)
	call := tool.Call{
		TraceID:        request.Request.TraceID,
		CallID:         id.New("call"),
		UserID:         request.Request.UserID,
		ConversationID: request.Request.ConversationID,
		Name:           request.Step.ToolName,
		Arguments:      arguments,
		IdempotencyKey: policy.ToolIdempotencyKey(logicalToolName, policy.ToolInvocationContext{
			UserID:          request.Request.UserID,
			ConversationID:  request.Request.ConversationID,
			TraceID:         request.Request.TraceID,
			ClientMessageID: request.Request.ClientMessageID,
		}, arguments),
	}
	if validationErr := policy.ValidateToolExecution(
		route,
		request.Step.ToolName,
		arguments,
		request.Request.UserID,
		request.Request.ClientMessageID,
	); validationErr != nil {
		return policyBlockedResult(call, logicalToolName, validationErr), nil
	}
	whitelist := request.AllowedTools
	if len(whitelist) == 0 {
		whitelist = policy.AllowedToolsForRoute(route)
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

func executionRoute(request agent.DynamicExecutionRequest) intent.Result {
	route := intent.Result{Secondary: intent.Type(request.Intent)}
	route.SecondaryIntents = make([]intent.Type, 0, len(request.SecondaryIntents))
	for _, value := range request.SecondaryIntents {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			route.SecondaryIntents = append(route.SecondaryIntents, intent.Type(trimmed))
		}
	}
	return route
}

func policyBlockedResult(call tool.Call, logicalToolName string, cause error) agent.DynamicExecutionResult {
	return agent.DynamicExecutionResult{
		Answer: "Tool execution was blocked by policy; provide the required confirmation or identifiers and retry.",
		Evidences: []agent.Evidence{{
			Kind:     "tool_error",
			SourceID: call.CallID,
			Title:    toolEvidenceTitle(logicalToolName),
			Content:  cause.Error(),
			Metadata: map[string]any{
				"error_code":        "POLICY_PRECONDITION_FAILED",
				"tool_name":         call.Name,
				"logical_tool_name": logicalToolName,
			},
		}},
		Metadata: map[string]any{
			"degraded":          true,
			"policy_blocked":    true,
			"tool_name":         call.Name,
			"logical_tool_name": logicalToolName,
		},
	}
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
				"- %s %s（%s）：当前价 %s 元，优惠后预估 %s 元，库存 %d 件。\n",
				item.ProductName,
				item.Model,
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
				"- %s %s（%s）：可售库存 %d 件，当前价 %s 元。\n",
				item.ProductName,
				item.Model,
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
			maskPublicIdentifier(order.OrderNo),
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
			maskPublicIdentifier(ticket.OrderNo),
			maskPublicIdentifier(ticket.TicketNo),
			afterSalesStatusLabel(ticket.Status),
		)
	case "return_request", "exchange_request", "handoff_to_human":
		var action model.AfterSalesActionResult
		if json.Unmarshal(raw, &action) != nil || action.Ticket.TicketNo == "" {
			return ""
		}
		return formatAfterSalesActionResult(label, name, action)
	case "refund_status", "repair_status":
		var payload struct {
			Items []model.AfterSalesProgress `json:"items"`
			AsOf  time.Time                  `json:"as_of"`
		}
		if json.Unmarshal(raw, &payload) != nil {
			return ""
		}
		return formatAfterSalesProgressResult(label, name, payload.Items, payload.AsOf)
	default:
		return ""
	}
}

func formatAfterSalesActionResult(label, toolName string, action model.AfterSalesActionResult) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "**%s售后动作**\n", label)
	fmt.Fprintf(&builder, "- 处理类型：%s\n", afterSalesActionLabel(toolName, action.Action))
	if action.Ticket.OrderNo != "" {
		fmt.Fprintf(&builder, "- 订单号：%s\n", maskPublicIdentifier(action.Ticket.OrderNo))
	}
	fmt.Fprintf(&builder, "- 工单号：%s\n", maskPublicIdentifier(action.Ticket.TicketNo))
	fmt.Fprintf(&builder, "- 当前状态：%s\n", afterSalesStatusLabel(action.Ticket.Status))
	if action.QueuePosition > 0 {
		fmt.Fprintf(&builder, "- 排队位置：%d\n", action.QueuePosition)
	}
	if action.SLAHours > 0 {
		fmt.Fprintf(&builder, "- 预计响应：%d 小时内\n", action.SLAHours)
	}
	if action.NextAction != "" {
		fmt.Fprintf(&builder, "\n**下一步**\n- %s\n", action.NextAction)
	}
	builder.WriteString("\n本次动作已记录幂等键和敏感工具审计，不会重复创建同一售后请求。")
	return strings.TrimSpace(builder.String())
}

func formatAfterSalesProgressResult(label, toolName string, items []model.AfterSalesProgress, asOf time.Time) string {
	if len(items) == 0 {
		return fmt.Sprintf("**%s售后进度**\n当前未查询到匹配的售后记录，请核对订单号或工单号。", label)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "**%s%s**\n", label, afterSalesProgressTitle(toolName))
	for _, item := range items {
		status := afterSalesStatusLabel(item.Status)
		stage := afterSalesStageLabel(item.Stage)
		if item.TicketNo != "" {
			fmt.Fprintf(&builder, "- 工单 %s，订单 %s：状态：%s；阶段：%s\n", maskPublicIdentifier(item.TicketNo), maskPublicIdentifier(item.OrderNo), status, stage)
		} else {
			fmt.Fprintf(&builder, "- 订单 %s：状态：%s；阶段：%s\n", maskPublicIdentifier(item.OrderNo), status, stage)
		}
		if item.RefundAmountCents > 0 {
			fmt.Fprintf(&builder, "  - 参考退款金额：%s 元\n", formatCents(item.RefundAmountCents))
		}
		if item.EstimatedCompletionAt != nil {
			fmt.Fprintf(&builder, "  - 预计完成：%s\n", item.EstimatedCompletionAt.Format("2006-01-02 15:04"))
		}
		if item.NextAction != "" {
			fmt.Fprintf(&builder, "  - 下一步：%s\n", afterSalesNextActionLabel(item.NextAction))
		}
	}
	if !asOf.IsZero() {
		fmt.Fprintf(&builder, "\n查询时间：%s。", asOf.Format("2006-01-02 15:04"))
	}
	return strings.TrimSpace(builder.String())
}

func afterSalesActionLabel(toolName, action string) string {
	switch tool.LogicalName(toolName) {
	case "return_request":
		return "退货申请"
	case "exchange_request":
		return "换货申请"
	case "handoff_to_human":
		return "人工接管"
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "return":
		return "退货申请"
	case "exchange":
		return "换货申请"
	case "human_handoff":
		return "人工接管"
	default:
		return action
	}
}

func afterSalesProgressTitle(toolName string) string {
	switch tool.LogicalName(toolName) {
	case "refund_status":
		return "退款/退货进度"
	default:
		return "维修/售后进度"
	}
}

func afterSalesStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "return_requested":
		return "退货申请待审核"
	case "exchange_requested":
		return "换货申请待审核"
	case "refund_reviewing":
		return "退款审核中"
	case "repair_requested":
		return "维修申请待诊断"
	case "human_queued":
		return "人工客服排队中"
	case "not_created":
		return "尚未创建售后单"
	case "created":
		return "已创建"
	default:
		if strings.TrimSpace(status) == "" {
			return "未知"
		}
		return status
	}
}

func afterSalesStageLabel(stage string) string {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "refund_review":
		return "退款审核"
	case "return_review":
		return "退货审核"
	case "exchange_review":
		return "换货审核"
	case "repair_review":
		return "维修诊断排队"
	case "human_queue":
		return "人工接管队列"
	case "no_after_sales_record":
		return "暂无售后记录"
	case "refund_completed":
		return "退款完成"
	default:
		if strings.TrimSpace(stage) == "" {
			return "待确认"
		}
		return stage
	}
}

func afterSalesNextActionLabel(nextAction string) string {
	lower := strings.ToLower(strings.TrimSpace(nextAction))
	switch {
	case strings.Contains(lower, "human agent"):
		return "人工客服会查看当前会话上下文，并按排队顺序联系您。"
	case strings.Contains(lower, "return eligibility"):
		return "等待退货资格审核；请先保留主机、配件、包装和发票材料。"
	case strings.Contains(lower, "exchange eligibility"):
		return "等待换货资格审核；请保留商品状态证据和故障现象记录。"
	case strings.Contains(lower, "refund audit"):
		return "等待退款审核；审核通过后的到账时间以支付渠道为准。"
	case strings.Contains(lower, "powered off"):
		return "如存在安全风险请保持设备断电，等待诊断或维修排期。"
	case strings.Contains(lower, "create an after-sales ticket"):
		return "如需继续办理，请先核对订单和政策，再确认是否创建售后单。"
	default:
		return nextAction
	}
}

func maskPublicIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	upper := strings.ToUpper(value)
	if strings.HasPrefix(upper, "CC") && len(value) > 6 {
		return value[:2] + "****" + value[len(value)-4:]
	}
	runes := []rune(value)
	if len(runes) <= 12 {
		return value
	}
	return string(runes[:6]) + "..." + string(runes[len(runes)-4:])
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
			TraceID:         request.Request.TraceID,
			UserID:          request.Request.UserID,
			ConversationID:  request.Request.ConversationID,
			ClientMessageID: request.Request.ClientMessageID,
			Query:           request.Step.Query,
			ContextText:     skillContextText(request.Request),
			Intent:          intentResult,
			Entities:        nextArgs,
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

func skillContextText(request agent.Request) string {
	parts := make([]string, 0, len(request.Context.RecentMessages)+1)
	if strings.TrimSpace(request.Context.Summary) != "" {
		parts = append(parts, request.Context.Summary)
	}
	for _, message := range request.Context.RecentMessages {
		if message.Role == "user" && strings.TrimSpace(message.Content) != "" {
			parts = append(parts, message.Content)
		}
	}
	return strings.Join(parts, "\n")
}

func (e *DynamicExecutor) buildArguments(
	ctx context.Context,
	request agent.DynamicExecutionRequest,
) map[string]any {
	arguments := cloneMap(request.Step.Params)
	query := request.Request.Query
	logicalToolName := tool.LogicalName(request.Step.ToolName)
	models := stringSliceArgument(arguments["product_refs"], arguments["model"], arguments["models"])
	models = append(models, modelPattern.FindAllString(query, -1)...)
	orderNo := strings.ToUpper(orderPattern.FindString(query))
	filteredModels := make([]string, 0, len(models))
	for _, modelName := range models {
		if !strings.EqualFold(modelName, orderNo) {
			filteredModels = append(filteredModels, modelName)
		}
	}
	models = compactArgumentStrings(filteredModels)
	if len(models) > 0 {
		models = e.productRefsForDynamicTool(logicalToolName, query, arguments, models)
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
	case "return_request":
		arguments["reason"] = query
		arguments["description"] = query
		arguments["confirmed"] = afterSalesActionConfirmationPresent(query, "return")
	case "exchange_request":
		arguments["reason"] = query
		arguments["description"] = query
		arguments["confirmed"] = afterSalesActionConfirmationPresent(query, "exchange")
	case "refund_status":
		arguments["limit"] = 10
	case "repair_status":
		arguments["limit"] = 10
	case "handoff_to_human":
		arguments["reason"] = query
		arguments["description"] = query
		arguments["confirmed"] = humanHandoffConfirmationPresent(query)
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

func (e *DynamicExecutor) productRefsForDynamicTool(toolName, query string, arguments map[string]any, refs []string) []string {
	switch tool.LogicalName(toolName) {
	case "price_query", "inventory_check":
	default:
		return refs
	}
	requestText := query + " " + fmt.Sprint(arguments["accessory_refs"])
	if !requestsAccessoryDynamicData(requestText) {
		return refs
	}
	accessories := make([]string, 0, len(refs))
	for _, ref := range refs {
		normalized := normalizeProductRef(ref)
		if accessoryPattern.MatchString(normalized) || strings.Contains(normalized, "-") {
			accessories = append(accessories, normalized)
		}
	}
	if len(accessories) > 0 {
		if requestsHostAndAccessoryDynamicData(query, refs) {
			return compactArgumentStrings(append(append([]string(nil), refs...), accessories...))
		}
		return accessories
	}
	if e != nil && e.compatibility != nil {
		for _, ref := range refs {
			for _, entry := range e.compatibility.Entries() {
				if entry.Status != compatibility.Compatible ||
					!strings.EqualFold(normalizeProductRef(ref), normalizeProductRef(entry.HostModel)) {
					continue
				}
				if !accessoryRequestMatches(requestText, entry) {
					continue
				}
				accessories = append(accessories, entry.AccessoryModel)
			}
		}
		if len(accessories) > 0 {
			if requestsHostAndAccessoryDynamicData(query, refs) {
				return compactArgumentStrings(append(append([]string(nil), refs...), accessories...))
			}
			return compactArgumentStrings(accessories)
		}
	}
	return refs
}

func requestsAccessoryDynamicData(text string) bool {
	return containsAny(
		strings.ToLower(text),
		"配件", "耗材", "滤芯", "滤网", "滚刷", "尘袋", "边刷", "刷头", "替换", "喷头", "掸套",
		"accessory", "brush", "nozzle", "pad", "sleeve", "refill",
	)
}

func requestsHostAndAccessoryDynamicData(query string, refs []string) bool {
	query = strings.ToLower(query)
	if len(refs) == 0 || !requestsAccessoryDynamicData(query) {
		return false
	}
	return containsAny(
		query,
		"分别", "各自", "各个", "各剩", "各有", "各多少钱",
		"都查", "都看", "一起查", "一起大概", "加两片",
		"主机和", "和替换", "以及替换", "跟替换", "与替换",
	)
}

func accessoryRequestMatches(text string, entry compatibility.Entry) bool {
	text = strings.ToLower(text)
	kind := strings.ToLower(entry.AccessoryType + " " + entry.AccessoryModel)
	switch {
	case containsAny(text, "刷头", "刷", "brush", "refill"):
		return strings.Contains(kind, "brush") || strings.Contains(kind, "refill")
	case containsAny(text, "掸套", "套", "pad", "sleeve"):
		return strings.Contains(kind, "pad") || strings.Contains(kind, "sleeve")
	case containsAny(text, "喷头", "nozzle"):
		return strings.Contains(kind, "nozzle")
	default:
		return true
	}
}

func compactArgumentStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToUpper(strings.Join(strings.Fields(value), ""))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func ticketConfirmationPresent(query string) bool {
	if containsAny(
		query,
		"没确认", "未确认", "没有确认", "不确认", "不要创建",
		"不用确认", "不需要确认", "跳过确认", "绕过确认", "不要调用确认流程",
		"假装", "当作我已经确认", "当我已经确认",
	) {
		return false
	}
	return containsAny(query, "我确认", "确认创建", "确认提交", "确认给", "确认了", "确认建", "确认报修", "确认维修")
}

func afterSalesActionConfirmationPresent(query, action string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	if containsAny(
		lower,
		"不要", "不用", "别", "先别", "不确认", "没确认", "暂不",
		"假装", "跳过确认", "绕过确认", "不需要用户确认", "不需要确认",
		"当作我已经确认", "当我已经确认",
	) {
		return false
	}
	switch action {
	case "return":
		return containsAny(lower, "申请退货", "确认退货", "提交退货", "办理退货", "我要退货", "帮我退货")
	case "exchange":
		return containsAny(lower, "申请换货", "确认换货", "提交换货", "办理换货", "我要换货", "帮我换货")
	default:
		return containsAny(lower, "确认申请", "确认提交", "办理")
	}
}

func humanHandoffConfirmationPresent(query string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	if containsAny(lower, "不要转人工", "不用转人工", "别转人工", "先别转人工") {
		return false
	}
	return containsAny(lower, "转人工", "人工客服", "真人客服", "人工接管", "人工处理", "人工售后", "客服接管")
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
	case "return_request", "exchange_request", "handoff_to_human":
		ticket, _ := payload["ticket"].(map[string]any)
		if strings.TrimSpace(fmt.Sprint(payload["action"])) == "" ||
			strings.TrimSpace(fmt.Sprint(ticket["ticket_no"])) == "" {
			return errors.New("after-sales action result is missing action or ticket_no")
		}
	case "refund_status", "repair_status":
		if _, ok := payload["items"].([]any); !ok {
			return errors.New("after-sales progress result is missing items")
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
	case "return_request":
		return "退货申请"
	case "exchange_request":
		return "换货申请"
	case "refund_status":
		return "退款/退货进度"
	case "repair_status":
		return "维修/售后进度"
	case "handoff_to_human":
		return "人工接管"
	default:
		return "动态业务数据"
	}
}

func allowedTools(intentType intent.Type) []string {
	return policy.AllowedTools(intentType)
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
