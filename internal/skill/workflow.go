package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"CleanCaregent/internal/agent"
	"CleanCaregent/internal/compatibility"
	"CleanCaregent/internal/diagnosis"
	"CleanCaregent/internal/generator"
	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/memory"
	"CleanCaregent/internal/model"
	"CleanCaregent/internal/platform/id"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/tool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	ProductComparisonSkill      = "product_comparison"
	PurchaseRecommendationSkill = "purchase_recommendation"
	AccessoryCompatibilitySkill = "accessory_compatibility"
	FaultDiagnosisSkill         = "fault_diagnosis"
	AfterSalesJudgementSkill    = "after_sales_judgement"
)

type WorkflowConfig struct {
	DenseTopK     int     `mapstructure:"dense_top_k"`
	KeywordTopK   int     `mapstructure:"keyword_top_k"`
	RerankTopK    int     `mapstructure:"rerank_top_k"`
	MinDenseScore float64 `mapstructure:"min_dense_score"`
}

type Workflow struct {
	name            string
	intents         map[intent.Type]struct{}
	retriever       rag.Retriever
	generator       generator.Generator
	tools           *tool.Executor
	config          WorkflowConfig
	diagnosisStore  memory.Store
	diagnosisEngine *diagnosis.Engine
	compatibility   *compatibility.Matrix
	docTypes        []string
	nextSkill       string
	nextSkillArgs   map[string]any
}

var accessoryModelPattern = regexp.MustCompile(`(?i)\b(?:F|DB|RB)[0-9]{2,}[A-Z0-9-]*\b`)

func NewProductComparison(
	retriever rag.Retriever,
	generator generator.Generator,
	tools *tool.Executor,
	config WorkflowConfig,
) *Workflow {
	return newWorkflow(ProductComparisonSkill, []intent.Type{intent.ProductComparison}, retriever, generator, tools, config)
}

func NewPurchaseRecommendation(
	retriever rag.Retriever,
	generator generator.Generator,
	tools *tool.Executor,
	config WorkflowConfig,
) *Workflow {
	return newWorkflow(PurchaseRecommendationSkill, []intent.Type{intent.PurchaseRecommendation}, retriever, generator, tools, config)
}

func NewAccessoryCompatibility(
	retriever rag.Retriever,
	generator generator.Generator,
	tools *tool.Executor,
	config WorkflowConfig,
) *Workflow {
	workflow := newWorkflow(
		AccessoryCompatibilitySkill,
		[]intent.Type{intent.AccessoryCompatibility},
		retriever,
		generator,
		tools,
		config,
	)
	workflow.compatibility = compatibility.NewDefaultMatrix()
	return workflow
}

func NewFaultDiagnosis(
	retriever rag.Retriever,
	generator generator.Generator,
	tools *tool.Executor,
	config WorkflowConfig,
	stores ...memory.Store,
) *Workflow {
	workflow := newWorkflow(
		FaultDiagnosisSkill,
		[]intent.Type{intent.Troubleshooting},
		retriever,
		generator,
		tools,
		config,
	)
	workflow.diagnosisEngine = diagnosis.NewDefaultEngine()
	if len(stores) > 0 {
		workflow.diagnosisStore = stores[0]
	}
	return workflow
}

func NewAfterSalesJudgement(
	retriever rag.Retriever,
	generator generator.Generator,
	tools *tool.Executor,
	config WorkflowConfig,
) *Workflow {
	return newWorkflow(
		AfterSalesJudgementSkill,
		[]intent.Type{intent.ReturnEligibility, intent.WarrantyQuery},
		retriever,
		generator,
		tools,
		config,
	)
}

func newWorkflow(
	name string,
	intents []intent.Type,
	retriever rag.Retriever,
	generator generator.Generator,
	tools *tool.Executor,
	config WorkflowConfig,
) *Workflow {
	handled := make(map[intent.Type]struct{}, len(intents))
	for _, intentType := range intents {
		handled[intentType] = struct{}{}
	}
	return &Workflow{name: name, intents: handled, retriever: retriever, generator: generator, tools: tools, config: config}
}

func (s *Workflow) Name() string { return s.name }

func (s *Workflow) CanHandle(intentType intent.Type) bool {
	_, ok := s.intents[intentType]
	return ok
}

func (s *Workflow) Run(ctx context.Context, request Request) (*Result, error) {
	ctx, span := otel.Tracer("clean-care-agent/skill").Start(ctx, "skill."+s.name)
	span.SetAttributes(
		attribute.String("skill.name", s.name),
		attribute.String("agent.trace_id", request.TraceID),
		attribute.String("intent.secondary", string(request.Intent.Secondary)),
	)
	defer span.End()
	docTypes := s.docTypes
	if len(docTypes) == 0 {
		docTypes = skillDocTypes(s.name)
	}
	models := entityStrings(request.Intent.Entities["models"])
	if s.name == FaultDiagnosisSkill && len(models) == 0 && s.diagnosisStore != nil {
		if state, err := s.diagnosisStore.LoadDiagnosisState(ctx, request.ConversationID); err == nil &&
			state != nil &&
			state.ProductModel != "" {
			models = []string{state.ProductModel}
			request.Intent.Entities["models"] = state.ProductModel
		}
	}
	searchData, err := s.retrieveInitial(ctx, request, models, docTypes)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("run %s retrieval: %w", s.name, err)
	}
	span.SetAttributes(attribute.Int("skill.evidence_count", len(searchData)))

	evidences := searchEvidences(searchData)
	if s.name == FaultDiagnosisSkill {
		diagnosisResult, handled, diagnosisErr := s.runFaultDiagnosis(
			ctx,
			request,
			models,
			searchData,
			evidences,
		)
		if diagnosisErr != nil {
			return nil, diagnosisErr
		}
		if handled {
			return diagnosisResult, nil
		}
	}
	if s.name == AccessoryCompatibilitySkill {
		if compatibilityResult, handled := s.runCompatibilityCheck(
			request,
			searchData,
			evidences,
		); handled {
			return compatibilityResult, nil
		}
	}
	var (
		dynamicNotes        []string
		deterministicAnswer string
	)
	switch s.name {
	case PurchaseRecommendationSkill:
		if len(models) == 0 {
			models = candidateModels(searchData)
		}
		if len(models) > 0 {
			for _, toolName := range []string{"price_query", "inventory_check"} {
				value, executeErr := s.callTool(ctx, request, toolName, map[string]any{"product_refs": models})
				if executeErr != nil {
					dynamicNotes = append(dynamicNotes, toolName+" 暂时不可用")
					continue
				}
				evidences = append(evidences, toolEvidence(toolName, value))
				dynamicNotes = append(
					dynamicNotes,
					toolResultSummary(toolName, value.Data, evidenceCitation(len(evidences))),
				)
			}
		}
	case AfterSalesJudgementSkill:
		orderNo := request.Intent.Entities["order_no"]
		if orderNo == "" {
			return &Result{
				Status:       "clarify",
				NextQuestion: "请提供订单号，以便结合购买时间和售后政策判断。",
				Evidences:    evidences,
			}, nil
		}
		toolName := "order_lookup"
		if request.Intent.Secondary == intent.WarrantyQuery {
			toolName = "warranty_check"
		}
		value, executeErr := s.callTool(ctx, request, toolName, map[string]any{
			"order_no": orderNo,
			"model":    first(models),
		})
		if executeErr != nil {
			dynamicNotes = append(dynamicNotes, "订单动态数据暂时不可用，当前只能说明政策条件")
		} else {
			evidences = append(evidences, toolEvidence(toolName, value))
			citation := evidenceCitation(len(evidences))
			dynamicNotes = append(
				dynamicNotes,
				toolResultSummary(toolName, value.Data, citation),
			)
			switch toolName {
			case "order_lookup":
				var order model.OrderDetail
				if decodeToolData(value.Data, &order) == nil && order.OrderNo != "" {
					now := time.Now()
					derivedCitation := citation
					if derivedEvidence, ok := returnEligibilityDerivedEvidence(order, now); ok {
						evidences = append(evidences, derivedEvidence)
						derivedCitation = evidenceCitation(len(evidences))
					}
					deterministicAnswer = buildReturnEligibilityAnswer(
						request.Query,
						order,
						searchData,
						citation,
						derivedCitation,
						now,
					)
				}
			case "warranty_check":
				var payload warrantyToolPayload
				if decodeToolData(value.Data, &payload) == nil && len(payload.Items) > 0 {
					deterministicAnswer = buildWarrantyAnswer(payload, searchData, citation)
				}
			}
		}
	case AccessoryCompatibilitySkill:
		if len(models) == 0 && refersToPurchase(request.Query) {
			value, executeErr := s.callTool(ctx, request, "user_purchase_history", map[string]any{
				"category": "air_purifier",
				"limit":    5,
			})
			if executeErr != nil {
				dynamicNotes = append(dynamicNotes, "购买记录暂时不可用，请补充已购商品型号")
				break
			}
			evidences = append(evidences, toolEvidence("user_purchase_history", value))
			purchasedModels := purchaseModels(value.Data)
			if len(purchasedModels) == 0 {
				dynamicNotes = append(dynamicNotes, "未找到可用于兼容判断的净化器购买记录")
				break
			}
			more, searchErr := s.retriever.Search(ctx, rag.SearchRequest{
				Query:       purchasedModels[0] + " 滤芯 配件兼容",
				Mode:        rag.SearchHybrid,
				Filter:      rag.MetadataFilter{Models: purchasedModels, DocTypes: skillDocTypes(s.name)},
				DenseTopK:   s.config.DenseTopK,
				KeywordTopK: s.config.KeywordTopK,
				RerankTopK:  s.config.RerankTopK,
				MinScore:    s.config.MinDenseScore,
				NeedRerank:  true,
			})
			if searchErr == nil {
				searchData = mergeSearchData(searchData, more)
				evidences = searchEvidences(searchData)
				evidences = append(evidences, toolEvidence("user_purchase_history", value))
			}
			accessoryRefs := accessoryModels(searchData)
			if len(accessoryRefs) == 0 {
				dynamicNotes = append(dynamicNotes, "已找到购买记录，但兼容表未给出明确配件型号")
				break
			}
			price, priceErr := s.callTool(ctx, request, "price_query", map[string]any{"product_refs": accessoryRefs})
			if priceErr != nil {
				dynamicNotes = append(dynamicNotes, "兼容配件已识别，但实时价格暂时不可用")
				break
			}
			evidences = append(evidences, toolEvidence("price_query", price))
			dynamicNotes = append(
				dynamicNotes,
				toolResultSummary("price_query", price.Data, evidenceCitation(len(evidences))),
			)
		}
	}

	answer := deterministicAnswer
	if answer == "" {
		generateCtx, generateSpan := otel.Tracer("clean-care-agent/skill").Start(ctx, "skill.generate")
		answer, err = s.generator.GenerateWithScenario(
			generateCtx,
			generationScenario(s.name),
			request.Query,
			searchData,
			strings.Join(dynamicNotes, "\n"),
			"",
			strings.Join(models, ", "),
		)
		if err != nil {
			generateSpan.RecordError(err)
			generateSpan.SetStatus(codes.Error, err.Error())
			generateSpan.End()
			return nil, fmt.Errorf("generate %s answer: %w", s.name, err)
		}
		generateSpan.SetAttributes(attribute.Int("skill.generation_evidence_count", len(searchData)))
		generateSpan.End()
	}
	if answer == "" {
		if len(dynamicNotes) > 0 {
			answer = "动态数据已查询，但当前无法生成可靠的自然语言结论。请稍后重试。"
		} else {
			answer = "当前证据不足，请补充具体型号、订单号或故障现象。"
		}
	}
	result := &Result{
		Status:        "success",
		AnswerDraft:   answer,
		Evidences:     evidences,
		NextSkill:     s.nextSkill,
		NextSkillArgs: cloneAnyMap(s.nextSkillArgs),
		Metadata: map[string]any{
			"skill":              s.name,
			"knowledge_evidence": len(searchData),
		},
	}
	return result, nil
}

func (s *Workflow) retrieveInitial(
	ctx context.Context,
	request Request,
	models []string,
	docTypes []string,
) ([]rag.SearchResult, error) {
	ctx, span := otel.Tracer("clean-care-agent/skill").Start(ctx, "skill.retrieve")
	span.SetAttributes(
		attribute.String("skill.name", s.name),
		attribute.Int("skill.route_count", 1),
	)
	defer span.End()
	type route struct {
		query    string
		models   []string
		docTypes []string
	}
	routes := []route{{query: request.Query, models: models, docTypes: docTypes}}
	switch s.name {
	case ProductComparisonSkill:
		routes = routes[:0]
		for _, modelName := range compactValues(models) {
			routes = append(routes, route{
				query:    modelName + " " + request.Query,
				models:   []string{modelName},
				docTypes: []string{"product_detail", "product_parameter"},
			})
		}
		routes = append(routes, route{
			query:    request.Query + " 使用场景 选购取舍",
			models:   models,
			docTypes: []string{"product_comparison", "purchase_guide"},
		})
	case PurchaseRecommendationSkill:
		routes = []route{
			{
				query:    request.Query + " 候选产品 参数",
				models:   models,
				docTypes: []string{"product_detail", "product_parameter"},
			},
			{
				query:    request.Query + " 选购场景 硬约束",
				models:   models,
				docTypes: []string{"purchase_guide", "product_comparison"},
			},
		}
	}
	span.SetAttributes(attribute.Int("skill.route_count", len(routes)))

	results := make([][]rag.SearchResult, len(routes))
	errs := make([]error, len(routes))
	var waitGroup sync.WaitGroup
	for index, current := range routes {
		index, current := index, current
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			results[index], errs[index] = s.retriever.Search(ctx, rag.SearchRequest{
				Query: current.query,
				Mode:  rag.SearchHybrid,
				Filter: rag.MetadataFilter{
					Models:   current.models,
					DocTypes: current.docTypes,
				},
				DenseTopK:   s.config.DenseTopK,
				KeywordTopK: s.config.KeywordTopK,
				RerankTopK:  s.config.RerankTopK,
				MinScore:    s.config.MinDenseScore,
				NeedRerank:  true,
			})
		}()
	}
	waitGroup.Wait()

	var (
		merged      []rag.SearchResult
		failedCount int
		lastErr     error
	)
	for index := range results {
		if errs[index] != nil {
			failedCount++
			lastErr = errs[index]
			continue
		}
		merged = mergeSearchData(merged, results[index])
	}
	if failedCount == len(routes) {
		if lastErr != nil {
			span.RecordError(lastErr)
			span.SetStatus(codes.Error, lastErr.Error())
		}
		return nil, lastErr
	}
	span.SetAttributes(
		attribute.Int("skill.retrieval_result_count", len(merged)),
		attribute.Int("skill.retrieval_failed_routes", failedCount),
	)
	return merged, nil
}

func (s *Workflow) runCompatibilityCheck(
	request Request,
	searchData []rag.SearchResult,
	evidences []agent.Evidence,
) (*Result, bool) {
	if s.compatibility == nil {
		return nil, false
	}
	modelCandidates := entityStrings(request.Intent.Entities["models"])
	accessoryCandidates := entityStrings(request.Intent.Entities["accessory_refs"])
	for _, match := range accessoryModelPattern.FindAllString(request.Query, -1) {
		accessoryCandidates = append(accessoryCandidates, strings.ToUpper(match))
	}
	modelCandidates = compactValues(modelCandidates)
	accessoryCandidates = compactValues(accessoryCandidates)

	var hostModel string
	for _, candidate := range modelCandidates {
		if !accessoryModelPattern.MatchString(candidate) {
			hostModel = candidate
			break
		}
	}
	var accessoryModel string
	for _, candidate := range accessoryCandidates {
		if accessoryModelPattern.MatchString(candidate) {
			accessoryModel = strings.ToUpper(candidate)
			break
		}
	}
	if hostModel == "" || accessoryModel == "" {
		return nil, false
	}
	result := s.compatibility.Check(hostModel, accessoryModel)
	raw, _ := json.Marshal(result)
	evidences = append(evidences, agent.Evidence{
		Kind:     "structured_compatibility",
		SourceID: "compatibility:" + hostModel + ":" + accessoryModel,
		Title:    "配件兼容矩阵",
		Content:  string(raw),
		Metadata: map[string]any{
			"host_model":      hostModel,
			"accessory_model": accessoryModel,
			"status":          result.Status,
		},
	})
	citation := fmt.Sprintf("[E%d]", len(searchData)+1)
	switch result.Status {
	case compatibility.Compatible:
		return &Result{
			Status:      "success",
			AnswerDraft: fmt.Sprintf("%s 与 %s 兼容。%s %s", accessoryModel, hostModel, result.Reason, citation),
			Evidences:   evidences,
			Metadata:    map[string]any{"compatibility_status": result.Status},
		}, true
	case compatibility.Incompatible:
		return &Result{
			Status:      "success",
			AnswerDraft: fmt.Sprintf("%s 与 %s 不兼容。%s %s", accessoryModel, hostModel, result.Reason, citation),
			Evidences:   evidences,
			Metadata:    map[string]any{"compatibility_status": result.Status},
		}, true
	default:
		return &Result{
			Status: "success",
			AnswerDraft: fmt.Sprintf(
				"当前兼容矩阵未收录 %s 与 %s 的关系，因此不能确认兼容。请核对主机铭牌和配件完整型号，或联系售后确认。%s",
				accessoryModel,
				hostModel,
				citation,
			),
			Evidences: evidences,
			Metadata:  map[string]any{"compatibility_status": result.Status},
		}, true
	}
}

func (s *Workflow) runFaultDiagnosis(
	ctx context.Context,
	request Request,
	models []string,
	searchData []rag.SearchResult,
	evidences []agent.Evidence,
) (*Result, bool, error) {
	if s.diagnosisEngine == nil {
		return nil, false, nil
	}
	if safety, ok := s.diagnosisEngine.SafetyDecision(request.Query); ok {
		return diagnosisSkillResult(safety, searchData, evidences), true, nil
	}

	var state *memory.DiagnosisState
	if s.diagnosisStore != nil {
		loaded, err := s.diagnosisStore.LoadDiagnosisState(ctx, request.ConversationID)
		if err == nil {
			state = loaded
		}
	}

	var (
		nextState memory.DiagnosisState
		decision  diagnosis.Decision
		err       error
	)
	if state == nil {
		if len(models) == 0 {
			return &Result{
				Status:       "clarify",
				NextQuestion: "为了进入对应的故障排查流程，请先告诉我产品型号，并描述指示灯、错误码或异常现象。",
				Evidences:    evidences,
			}, true, nil
		}
		nextState, decision, err = s.diagnosisEngine.Start(models[0], request.Query)
		nextState.ConversationID = request.ConversationID
	} else {
		nextState, decision, err = s.diagnosisEngine.Advance(*state, request.Query)
		nextState.ConversationID = request.ConversationID
	}
	if errors.Is(err, diagnosis.ErrNoMatchingTree) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, fmt.Errorf("advance fault diagnosis: %w", err)
	}
	if s.diagnosisStore != nil && nextState.FaultNodeID != "" {
		_ = s.diagnosisStore.SaveDiagnosisState(ctx, nextState)
	}
	return diagnosisSkillResult(decision, searchData, evidences), true, nil
}

func diagnosisSkillResult(
	decision diagnosis.Decision,
	searchData []rag.SearchResult,
	evidences []agent.Evidence,
) *Result {
	citation := diagnosisCitation(decision.EvidenceDocID, searchData)
	metadata := map[string]any{
		"skill":         FaultDiagnosisSkill,
		"fault_node_id": decision.NodeID,
		"safety_level":  decision.SafetyLevel,
		"need_human":    decision.NeedHuman,
		"terminal":      decision.Terminal,
		"answer_parsed": decision.Understood,
	}
	if decision.Terminal {
		answer := strings.TrimSpace(decision.Resolution)
		if citation != "" {
			answer += " " + citation
		}
		if decision.NeedHuman {
			answer += "\n\n需要我在您确认订单信息后帮您创建售后工单吗？"
		}
		return &Result{
			Status:      "success",
			AnswerDraft: answer,
			Evidences:   evidences,
			Metadata:    metadata,
		}
	}

	nextQuestion := strings.TrimSpace(decision.Guidance)
	if citation != "" && nextQuestion != "" {
		nextQuestion += " " + citation
	}
	if nextQuestion != "" {
		nextQuestion += "\n\n"
	}
	nextQuestion += decision.Question
	if !decision.Understood && decision.Question != "" {
		nextQuestion += "\n请直接回复“是/否”，也可以描述您看到的指示灯或声音。"
	}
	return &Result{
		Status:       "clarify",
		NextQuestion: nextQuestion,
		Evidences:    evidences,
		Metadata:     metadata,
	}
}

func diagnosisCitation(docID string, searchData []rag.SearchResult) string {
	if docID == "" {
		return ""
	}
	for index, item := range searchData {
		if item.DocumentID == docID {
			return fmt.Sprintf("[E%d]", index+1)
		}
	}
	return ""
}

func (s *Workflow) callTool(
	ctx context.Context,
	request Request,
	name string,
	arguments map[string]any,
) (tool.Result, error) {
	if s.tools == nil {
		return tool.Result{}, fmt.Errorf("tool executor is unavailable")
	}
	ctx, span := otel.Tracer("clean-care-agent/skill").Start(ctx, "skill.tool")
	span.SetAttributes(
		attribute.String("skill.name", s.name),
		attribute.String("tool.name", name),
	)
	defer span.End()
	result, err := s.tools.Execute(ctx, tool.Call{
		TraceID:        request.TraceID,
		CallID:         id.New("call"),
		UserID:         request.UserID,
		ConversationID: request.ConversationID,
		Name:           name,
		Arguments:      arguments,
	}, []string{name})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

func skillDocTypes(name string) []string {
	switch name {
	case ProductComparisonSkill:
		return []string{"product_detail", "product_parameter", "product_comparison", "purchase_guide"}
	case PurchaseRecommendationSkill:
		return []string{"product_detail", "product_parameter", "purchase_guide", "product_comparison"}
	case AccessoryCompatibilitySkill:
		return []string{"accessory_compatibility", "product_detail", "faq"}
	case FaultDiagnosisSkill:
		return []string{"troubleshooting", "user_manual", "faq"}
	case AfterSalesJudgementSkill:
		return []string{"after_sales_policy", "faq"}
	default:
		return nil
	}
}

func generationScenario(name string) prompt.Scenario {
	switch name {
	case ProductComparisonSkill:
		return prompt.ScenarioGenerateCompare
	case FaultDiagnosisSkill:
		return prompt.ScenarioGenerateDiagnose
	case AfterSalesJudgementSkill:
		return prompt.ScenarioGeneratePolicy
	default:
		return prompt.ScenarioGenerateGeneric
	}
}

func searchEvidences(items []rag.SearchResult) []agent.Evidence {
	result := make([]agent.Evidence, 0, len(items))
	for _, item := range items {
		metadata := cloneAnyMap(item.Metadata)
		metadata["dense_score"] = item.DenseScore
		metadata["keyword_score"] = item.KeywordScore
		metadata["fusion_score"] = item.FusionScore
		metadata["rerank_score"] = item.RerankScore
		result = append(result, agent.Evidence{
			Kind:     "kb_chunk",
			SourceID: item.ChunkID,
			Title:    item.Title,
			Content:  item.Content,
			Metadata: metadata,
		})
	}
	return result
}

func cloneAnyMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func toolEvidence(name string, value tool.Result) agent.Evidence {
	raw, _ := json.Marshal(value.Data)
	return agent.Evidence{
		Kind:     "tool_result",
		SourceID: value.CallID,
		Title:    name,
		Content:  string(raw),
		Metadata: map[string]any{
			"tool_name":   name,
			"success":     value.Success,
			"finished_at": value.FinishedAt.Format(time.RFC3339Nano),
		},
	}
}

func toolResultSummary(name string, data any, citation string) string {
	raw, _ := json.Marshal(data)
	switch name {
	case "price_query":
		var payload struct {
			Items []model.PriceQuote `json:"items"`
			AsOf  time.Time          `json:"as_of"`
		}
		if json.Unmarshal(raw, &payload) == nil && len(payload.Items) > 0 {
			parts := make([]string, 0, len(payload.Items))
			for _, item := range payload.Items {
				parts = append(parts, fmt.Sprintf(
					"%s 当前价 %s 元，优惠后预估 %s 元",
					item.Model,
					formatCents(item.CurrentPriceCents),
					formatCents(item.EstimatedFinalPriceCents),
				))
			}
			return fmt.Sprintf(
				"实时价格（查询时间 %s）：%s %s",
				formatFactTime(payload.AsOf),
				strings.Join(parts, "；"),
				citation,
			)
		}
	case "inventory_check":
		var payload struct {
			Items []model.ProductSKU `json:"items"`
		}
		if json.Unmarshal(raw, &payload) == nil && len(payload.Items) > 0 {
			parts := make([]string, 0, len(payload.Items))
			for _, item := range payload.Items {
				parts = append(parts, fmt.Sprintf("%s 可售库存 %d 台", item.Model, item.AvailableStock))
			}
			return "实时库存：" + strings.Join(parts, "；") + " " + citation
		}
	case "user_purchase_history":
		var payload struct {
			Items []model.PurchaseRecord `json:"items"`
		}
		if json.Unmarshal(raw, &payload) == nil && len(payload.Items) > 0 {
			parts := make([]string, 0, len(payload.Items))
			for _, item := range payload.Items {
				parts = append(parts, fmt.Sprintf(
					"订单 %s 购买 %s（%s）",
					item.OrderNo,
					item.ProductName,
					formatFactTime(item.PaidAt),
				))
			}
			return "购买记录：" + strings.Join(parts, "；") + " " + citation
		}
	case "order_lookup":
		var order model.OrderDetail
		if json.Unmarshal(raw, &order) == nil && order.OrderNo != "" {
			productNames := make([]string, 0, len(order.Items))
			for _, item := range order.Items {
				productNames = append(productNames, item.ProductName)
			}
			return fmt.Sprintf(
				"订单事实：订单号 %s，状态 %s，支付时间 %s，签收时间 %s，商品 %s。%s",
				order.OrderNo,
				order.Status,
				formatOptionalFactTime(order.PaidAt),
				formatOptionalFactTime(order.DeliveredAt),
				strings.Join(productNames, "、"),
				citation,
			)
		}
	case "warranty_check":
		var payload struct {
			Items     []model.WarrantyStatus `json:"items"`
			CheckedAt time.Time              `json:"checked_at"`
		}
		if json.Unmarshal(raw, &payload) == nil && len(payload.Items) > 0 {
			parts := make([]string, 0, len(payload.Items))
			for _, item := range payload.Items {
				status := "不在保修期"
				if item.InWarranty {
					status = "在保修期"
				}
				parts = append(parts, fmt.Sprintf(
					"%s %s，保修截止 %s",
					item.Model,
					status,
					formatOptionalFactTime(item.WarrantyEnd),
				))
			}
			return "保修状态：" + strings.Join(parts, "；") + " " + citation
		}
	}
	return "动态业务数据已查询 " + citation
}

func formatCents(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
}

type warrantyToolPayload struct {
	Items     []model.WarrantyStatus `json:"items"`
	CheckedAt time.Time              `json:"checked_at"`
}

func decodeToolData(data any, target any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func buildReturnEligibilityAnswer(
	query string,
	order model.OrderDetail,
	searchData []rag.SearchResult,
	toolCitation string,
	derivedCitation string,
	now time.Time,
) string {
	productNames := make([]string, 0, len(order.Items))
	for _, item := range order.Items {
		if item.ProductName != "" {
			productNames = append(productNames, item.ProductName)
		}
	}
	if len(productNames) == 0 {
		productNames = append(productNames, "订单商品")
	}

	returnPolicyCitation := evidenceCitationByDocument(searchData, "kb_policy_return_7d")
	qualityPolicyCitation := evidenceCitationByDocument(searchData, "kb_policy_quality_exchange")
	warrantyPolicyCitation := evidenceCitationByDocument(searchData, "kb_policy_warranty")
	opened := containsAny(query, "拆封", "包装拆", "已使用", "用过")
	qualityIssue := containsAny(query, "质量", "故障", "坏", "异常", "漏水", "冒烟", "无法")

	var builder strings.Builder
	builder.WriteString("**您的订单情况**\n")
	fmt.Fprintf(&builder, "- 订单号：%s\n", order.OrderNo)
	fmt.Fprintf(&builder, "- 商品：%s\n", strings.Join(productNames, "、"))
	fmt.Fprintf(&builder, "- 支付时间：%s\n", formatOptionalFactTime(order.PaidAt))
	fmt.Fprintf(&builder, "- 签收时间：%s\n", formatOptionalFactTime(order.DeliveredAt))
	fmt.Fprintf(&builder, "- 订单状态：%s %s\n\n", order.Status, toolCitation)

	builder.WriteString("**退换货判断**\n")
	switch {
	case order.DeliveredAt == nil:
		fmt.Fprintf(
			&builder,
			"- 当前订单缺少签收时间，无法直接判断是否处于 7 天无理由期限内；具体情况需要售后核验。%s\n",
			returnPolicyCitation,
		)
	default:
		elapsedDays := elapsedCalendarDays(*order.DeliveredAt, now)
		if elapsedDays > 7 {
			fmt.Fprintf(
				&builder,
				"- 从签收到现在约 %d 天，已超过 7 天无理由退货期限。%s %s\n",
				elapsedDays,
				derivedCitation,
				returnPolicyCitation,
			)
		} else {
			fmt.Fprintf(
				&builder,
				"- 从签收到现在约 %d 天，仍在 7 天时间窗口内；是否可申请还需同时满足商品、附件和包装完好且不影响二次销售。%s %s\n",
				elapsedDays,
				derivedCitation,
				returnPolicyCitation,
			)
		}
	}
	if opened {
		fmt.Fprintf(
			&builder,
			"- 您说明包装已经拆开，是否影响二次销售需要售后验收，不能仅凭“已拆封”直接承诺可退。%s\n",
			returnPolicyCitation,
		)
	}
	if qualityIssue {
		fmt.Fprintf(
			&builder,
			"- 如果诉求来自产品质量故障，仍可申请质量检测，并根据检测结论进入换货、维修或保修流程。%s %s\n",
			qualityPolicyCitation,
			warrantyPolicyCitation,
		)
	} else {
		fmt.Fprintf(
			&builder,
			"- 如果商品存在质量故障，可改走质量检测或保修流程；无质量问题时，以上无理由退货条件是主要判断依据。%s %s\n",
			qualityPolicyCitation,
			warrantyPolicyCitation,
		)
	}

	builder.WriteString("\n**还需要确认**\n")
	builder.WriteString("- 请确认退货原因是“不想要了”还是“商品存在质量故障”，两种情况适用的处理路径不同。\n\n")
	builder.WriteString("**下一步**\n")
	builder.WriteString("- 若存在故障，请描述具体现象，我可以继续排查；需要报修时，在您确认订单和问题描述后可以创建售后工单。")
	return builder.String()
}

func returnEligibilityDerivedEvidence(
	order model.OrderDetail,
	now time.Time,
) (agent.Evidence, bool) {
	if order.DeliveredAt == nil {
		return agent.Evidence{}, false
	}
	elapsedDays := elapsedCalendarDays(*order.DeliveredAt, now)
	return agent.Evidence{
		Kind:     "derived_fact",
		SourceID: "derived:return_elapsed_days:" + order.OrderNo,
		Title:    "退货时效计算",
		Content: fmt.Sprintf(
			"订单 %s 从签收时间 %s 到评估时间 %s 共 %d 天。",
			order.OrderNo,
			formatFactTime(*order.DeliveredAt),
			formatFactTime(now),
			elapsedDays,
		),
		Metadata: map[string]any{
			"order_no":      order.OrderNo,
			"delivered_at":  order.DeliveredAt.Format(time.RFC3339Nano),
			"evaluated_at":  now.Format(time.RFC3339Nano),
			"elapsed_days":  elapsedDays,
			"derivation":    "calendar_day_difference",
			"source_kind":   "tool_result",
			"source_tool":   "order_lookup",
			"deterministic": true,
		},
	}, true
}

func buildWarrantyAnswer(
	payload warrantyToolPayload,
	searchData []rag.SearchResult,
	toolCitation string,
) string {
	policyCitation := evidenceCitationByDocument(searchData, "kb_policy_warranty")
	var builder strings.Builder
	builder.WriteString("**保修状态**\n")
	for _, item := range payload.Items {
		status := "不在保修期"
		if item.InWarranty {
			status = "在保修期内"
		}
		fmt.Fprintf(
			&builder,
			"- 订单 %s，商品 %s（%s）：%s；保修起止时间为 %s 至 %s。%s\n",
			item.OrderNo,
			item.ProductName,
			item.Model,
			status,
			formatOptionalFactTime(item.WarrantyStart),
			formatOptionalFactTime(item.WarrantyEnd),
			toolCitation,
		)
		if item.Reason != "" {
			fmt.Fprintf(&builder, "  判断依据：%s\n", item.Reason)
		}
	}
	fmt.Fprintf(
		&builder,
		"\n保修期按订单项配置，并从签收时间起算；没有签收时间时使用支付时间。%s",
		policyCitation,
	)
	if !payload.CheckedAt.IsZero() {
		fmt.Fprintf(&builder, "\n\n查询时间：%s。", formatFactTime(payload.CheckedAt))
	}
	builder.WriteString("\n如需报修，请描述故障现象；在您明确确认后可以创建售后工单。")
	return builder.String()
}

func evidenceCitationByDocument(searchData []rag.SearchResult, documentID string) string {
	for index, item := range searchData {
		if item.DocumentID == documentID {
			return evidenceCitation(index + 1)
		}
	}
	return ""
}

func elapsedCalendarDays(start, end time.Time) int {
	if end.Before(start) {
		return 0
	}
	startDate := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	end = end.In(start.Location())
	endDate := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())
	return int(endDate.Sub(startDate).Hours() / 24)
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func evidenceCitation(index int) string {
	if index <= 0 {
		return ""
	}
	return fmt.Sprintf("[E%d]", index)
}

func formatFactTime(value time.Time) string {
	if value.IsZero() {
		return "未提供"
	}
	return value.In(time.Local).Format("2006-01-02 15:04")
}

func formatOptionalFactTime(value *time.Time) string {
	if value == nil {
		return "未提供"
	}
	return formatFactTime(*value)
}

func entityStrings(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func refersToPurchase(query string) bool {
	for _, keyword := range []string{"上周买", "之前买", "我买的", "购买记录", "那个净化器"} {
		if strings.Contains(query, keyword) {
			return true
		}
	}
	return false
}

func purchaseModels(data any) []string {
	raw, _ := json.Marshal(data)
	var payload struct {
		Items []model.PurchaseRecord `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	result := make([]string, 0, len(payload.Items))
	for _, item := range payload.Items {
		if item.Model == "" {
			continue
		}
		if _, ok := seen[item.Model]; ok {
			continue
		}
		seen[item.Model] = struct{}{}
		result = append(result, item.Model)
	}
	return result
}

func accessoryModels(items []rag.SearchResult) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, item := range items {
		for _, match := range accessoryModelPattern.FindAllString(item.Title+"\n"+item.Content, -1) {
			match = strings.ToUpper(match)
			if _, ok := seen[match]; ok {
				continue
			}
			seen[match] = struct{}{}
			result = append(result, match)
		}
	}
	return result
}

func candidateModels(items []rag.SearchResult) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, 4)
	for _, item := range items {
		modelName, _ := item.Metadata["model"].(string)
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		result = append(result, modelName)
		if len(result) == 4 {
			break
		}
	}
	return result
}

func mergeSearchData(existing, added []rag.SearchResult) []rag.SearchResult {
	seen := make(map[string]struct{}, len(existing)+len(added))
	result := make([]rag.SearchResult, 0, len(existing)+len(added))
	for _, item := range append(existing, added...) {
		if _, ok := seen[item.ChunkID]; ok {
			continue
		}
		seen[item.ChunkID] = struct{}{}
		result = append(result, item)
	}
	return result
}

func compactValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToUpper(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}
