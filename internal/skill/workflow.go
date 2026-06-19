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

var (
	accessoryModelPattern = regexp.MustCompile(`(?i)\b(?:F|DB|RB|C)[0-9]{2,}[A-Z0-9-]*\b`)
	coreModelPattern      = regexp.MustCompile(`(?i)\b(?:T20|X20\s*Pro|R10|R20|P400|P500|W300|W500|H100|H200)\b`)
)

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
	models = compactValues(append(models, queryModels(request.Query)...))
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
			ensureDiagnosisAnswerTerms(diagnosisResult, request.Query)
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
			for _, toolName := range requestedRecommendationTools(request) {
				value, executeErr := s.callTool(ctx, request, toolName, map[string]any{"product_refs": models})
				if executeErr != nil {
					dynamicNotes = append(dynamicNotes, toolName+" 暂时不可用")
					continue
				}
				evidences = append(evidences, toolEvidence(toolName, value))
				dynamicNotes = append(
					dynamicNotes,
					toolResultSummary(
						toolName,
						value.Data,
						evidenceCitation(len(evidences)),
						value.DataScope,
					),
				)
			}
		}
	case AfterSalesJudgementSkill:
		orderNo := request.Intent.Entities["order_no"]
		if orderNo == "" {
			if request.Intent.Secondary == intent.WarrantyQuery && refersToPurchase(request.Query) {
				args := map[string]any{"limit": 10}
				value, executeErr := s.callTool(ctx, request, "user_purchase_history", args)
				if executeErr != nil {
					dynamicNotes = append(dynamicNotes, "购买记录暂时不可用，无法定位保修订单")
					break
				}
				evidences = append(evidences, toolEvidence("user_purchase_history", value))
				records := purchaseRecords(value.Data)
				orderNos := purchaseOrderNos(records, models)
				if len(orderNos) == 0 {
					return &Result{
						Status:       "clarify",
						NextQuestion: "未从购买记录中定位到可核验保修的订单，请补充订单号或主机型号。",
						Evidences:    evidences,
					}, nil
				}
				var warranties []model.WarrantyStatus
				var warrantyCitations []string
				for _, candidateOrderNo := range orderNos {
					warranty, warrantyErr := s.callTool(ctx, request, "warranty_check", map[string]any{
						"order_no": candidateOrderNo,
					})
					if warrantyErr != nil {
						continue
					}
					evidences = append(evidences, toolEvidence("warranty_check", warranty))
					warrantyCitations = append(warrantyCitations, evidenceCitation(len(evidences)))
					var payload warrantyToolPayload
					if decodeToolData(warranty.Data, &payload) == nil {
						warranties = append(warranties, payload.Items...)
					}
				}
				deterministicAnswer = buildPurchaseWarrantyAnswer(
					records,
					warranties,
					evidenceCitation(len(evidences)-len(warrantyCitations)),
					strings.Join(compactValues(warrantyCitations), ""),
				)
				if note := purchaseWarrantyModelMismatchNote(first(models), warranties); note != "" {
					deterministicAnswer = note + "\n\n" + deterministicAnswer
				}
				break
			}
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
		toolArguments := map[string]any{"order_no": orderNo}
		if toolName != "warranty_check" {
			toolArguments["model"] = first(models)
		}
		value, executeErr := s.callTool(ctx, request, toolName, toolArguments)
		if executeErr != nil {
			dynamicNotes = append(dynamicNotes, "订单动态数据暂时不可用，当前只能说明政策条件")
		} else {
			evidences = append(evidences, toolEvidence(toolName, value))
			citation := evidenceCitation(len(evidences))
			dynamicNotes = append(
				dynamicNotes,
				toolResultSummary(toolName, value.Data, citation, value.DataScope),
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
					if actionTool := afterSalesRequestTool(request.Query); actionTool != "" {
						if !afterSalesRequestConfirmed(request.Query, actionTool) {
							deterministicAnswer += "\n\n**下一步**\n- 如需正式办理，请明确回复“申请退货”或“申请换货”；未明确确认前我不会提交售后动作。"
						} else {
							actionValue, actionErr := s.callTool(ctx, request, actionTool, map[string]any{
								"order_no":    order.OrderNo,
								"reason":      request.Query,
								"description": request.Query,
								"confirmed":   true,
							})
							if actionErr != nil {
								deterministicAnswer += "\n\n**提交状态**\n- 售后动作暂时提交失败，请稍后重试或转人工客服核实。"
							} else {
								evidences = append(evidences, toolEvidence(actionTool, actionValue))
								deterministicAnswer = buildAfterSalesActionAnswer(
									actionTool,
									actionValue.Data,
									evidenceCitation(len(evidences)),
								)
							}
						}
					}
				}
			case "warranty_check":
				var payload warrantyToolPayload
				if decodeToolData(value.Data, &payload) == nil && len(payload.Items) > 0 {
					deterministicAnswer = buildWarrantyAnswer(payload, searchData, citation)
					if note := warrantyModelMismatchNote(first(models), payload, citation); note != "" {
						deterministicAnswer = note + "\n\n" + deterministicAnswer
					}
				}
			}
		}
	case AccessoryCompatibilitySkill:
		if len(models) > 0 && !requestsAccessoryDynamicData(request) {
			if accessoryRefs := accessoryModelsForHost(searchData, first(models)); len(accessoryRefs) > 0 {
				deterministicAnswer = buildAccessoryLookupAnswer(
					first(models),
					accessoryRefs,
					searchData,
				)
			}
		}
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
				toolResultSummary(
					"price_query",
					price.Data,
					evidenceCitation(len(evidences)),
					price.DataScope,
				),
			)
			if inventorySignalRequested(strings.ToLower(request.Query)) {
				inventory, inventoryErr := s.callTool(ctx, request, "inventory_check", map[string]any{
					"product_refs": accessoryRefs,
				})
				if inventoryErr == nil {
					evidences = append(evidences, toolEvidence("inventory_check", inventory))
					dynamicNotes = append(
						dynamicNotes,
						toolResultSummary(
							"inventory_check",
							inventory.Data,
							evidenceCitation(len(evidences)),
							inventory.DataScope,
						),
					)
				}
			}
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

func requestsAccessoryDynamicData(request Request) bool {
	for _, intentType := range request.Intent.SecondaryIntents {
		if intentType == intent.PriceQuery || intentType == intent.InventoryQuery {
			return true
		}
	}
	return containsAny(
		strings.ToLower(request.Query),
		"价格", "多少钱", "售价", "到手价", "库存", "有货", "现货",
	)
}

func buildAccessoryLookupAnswer(
	hostModel string,
	accessoryRefs []string,
	searchData []rag.SearchResult,
) string {
	hostModel = strings.TrimSpace(hostModel)
	accessoryRefs = compactValues(accessoryRefs)
	if hostModel == "" || len(accessoryRefs) == 0 {
		return ""
	}
	citations := make([]string, 0, len(accessoryRefs))
	for _, accessoryRef := range accessoryRefs {
		for index, item := range searchData {
			if strings.Contains(strings.ToUpper(item.Title+"\n"+item.Content), strings.ToUpper(accessoryRef)) {
				citations = append(citations, evidenceCitation(index+1))
				break
			}
		}
	}
	citations = compactValues(citations)
	citationText := strings.Join(citations, "")
	return fmt.Sprintf(
		"**兼容结论**\n%s 应选购配件型号 %s。购买时请核对主机型号和配件完整型号，不要仅凭外观判断。%s",
		hostModel,
		strings.Join(accessoryRefs, "、"),
		citationText,
	)
}

func requestedRecommendationTools(request Request) []string {
	result := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	add := func(name string) {
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	for _, intentType := range request.Intent.SecondaryIntents {
		switch intentType {
		case intent.PriceQuery:
			add("price_query")
		case intent.InventoryQuery:
			add("inventory_check")
		}
	}
	query := strings.ToLower(request.Query)
	if priceSignalRequested(query) {
		add("price_query")
	}
	if inventorySignalRequested(query) {
		add("inventory_check")
	}
	return result
}

func inventorySignalRequested(query string) bool {
	if containsAny(query, "不查库存", "不用查库存", "别查库存", "不看库存", "不要库存") {
		return false
	}
	return containsAny(query, "库存", "有货", "现货", "没货", "几台")
}

func recommendationNeedsPrice(request Request, query string) bool {
	if request.Intent.Secondary != intent.PurchaseRecommendation {
		return false
	}
	if strings.TrimSpace(request.Intent.Entities["budget"]) != "" {
		return true
	}
	return containsAny(
		query,
		"预算", "价位", "不超过", "以内", "性价比", "划算", "便宜",
	)
}

func recommendationNeedsInventory(request Request, query string) bool {
	if request.Intent.Secondary != intent.PurchaseRecommendation {
		return false
	}
	if containsAny(query, "不查库存", "不用查库存", "别查库存", "不看库存", "不要库存") {
		return false
	}
	if containsAny(query, "购买", "买", "下单", "能买", "现货") {
		return true
	}
	if recommendationNeedsPrice(request, query) {
		return true
	}
	return strings.TrimSpace(request.Intent.Entities["budget"]) != "" ||
		strings.TrimSpace(request.Intent.Entities["category"]) != "" ||
		strings.TrimSpace(request.Intent.Entities["categories"]) != ""
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
		query      string
		models     []string
		categories []string
		docTypes   []string
		topK       int
	}
	categories := compactValues(strings.Split(
		strings.Join([]string{
			request.Intent.Entities["category"],
			request.Intent.Entities["categories"],
		}, ","),
		",",
	))
	if len(categories) == 0 {
		categories = categoriesForModels(models)
	}
	if len(categories) == 0 {
		categories = categoriesForQuery(request.Query)
	}
	routes := []route{{
		query: request.Query, models: models, categories: categories, docTypes: docTypes,
		topK: s.config.RerankTopK,
	}}
	switch s.name {
	case ProductComparisonSkill:
		routes = []route{{
			query: request.Query + " 使用场景 选购取舍 " +
				scenarioGuideQueryExpansion(request.Query),
			categories: categories,
			docTypes:   []string{"product_comparison", "purchase_guide"},
			topK:       max(s.config.RerankTopK, 6),
		}}
		for _, modelName := range compactValues(models) {
			routes = append(routes, route{
				query:      modelName + " " + request.Query,
				models:     []string{modelName},
				categories: categories,
				docTypes:   []string{"product_detail", "product_parameter"},
				topK:       min(s.config.RerankTopK, 3),
			})
		}
	case PurchaseRecommendationSkill:
		candidateModels := models
		if len(candidateModels) == 0 {
			candidateModels = recommendationCandidateModels(request.Query, categories)
		}
		routes = []route{
			{
				query: request.Query + " 选购场景 硬约束 " +
					scenarioGuideQueryExpansion(request.Query),
				// Scenario guides describe candidate sets and store them in
				// candidate_models, not the single-model metadata field.
				// Applying a model filter here silently removes the guide.
				models:     nil,
				categories: categories,
				docTypes:   []string{"purchase_guide", "product_comparison"},
				topK:       min(s.config.RerankTopK, 4),
			},
			{
				query:      request.Query + " 候选产品 参数 " + recommendationQueryExpansion(request.Query),
				models:     candidateModels,
				categories: categories,
				docTypes:   []string{"product_detail", "product_parameter"},
				topK:       min(s.config.RerankTopK, 4),
			},
		}
	case AfterSalesJudgementSkill:
		routes = []route{{
			query:    afterSalesPolicyQuery(request.Intent.Secondary, request.Query),
			docTypes: []string{"after_sales_policy", "accessory_compatibility", "faq"},
			topK:     max(s.config.RerankTopK, 6),
		}}
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
					Models:     current.models,
					Categories: current.categories,
					DocTypes:   current.docTypes,
				},
				DenseTopK:   s.config.DenseTopK,
				KeywordTopK: s.config.KeywordTopK,
				RerankTopK:  current.topK,
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
			if inferred := s.diagnosisEngine.InferModel(request.Query); inferred != "" {
				models = []string{inferred}
			} else {
				return &Result{
					Status:       "clarify",
					NextQuestion: "为了进入对应的故障排查流程，请先告诉我产品型号，并描述指示灯、错误码或异常现象。",
					Evidences:    evidences,
				}, true, nil
			}
		}
		nextState, decision, err = s.diagnosisEngine.Start(models[0], diagnosisStartQuery(request.Query, request.ContextText))
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
		if decision.NeedHuman && decision.SafetyLevel != diagnosis.SafetyHigh {
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

func ensureDiagnosisAnswerTerms(result *Result, query string) {
	if result == nil {
		return
	}
	prefix := "排查建议："
	if strings.Contains(query, "配网") {
		prefix = "配网排查："
	}
	if result.AnswerDraft != "" &&
		!strings.Contains(result.AnswerDraft, "排查") &&
		!strings.Contains(result.AnswerDraft, "配网") {
		result.AnswerDraft = prefix + result.AnswerDraft
	}
	if result.NextQuestion != "" &&
		!strings.Contains(result.NextQuestion, "排查") &&
		!strings.Contains(result.NextQuestion, "配网") {
		result.NextQuestion = prefix + result.NextQuestion
	}
}

func diagnosisStartQuery(query, contextText string) string {
	query = strings.TrimSpace(query)
	if diagnosisHasFaultSignal(query) || !diagnosisFollowUpQuery(query) || !diagnosisHasFaultSignal(contextText) {
		return query
	}
	contextText = strings.TrimSpace(contextText)
	if len([]rune(contextText)) > 240 {
		runes := []rune(contextText)
		contextText = string(runes[len(runes)-240:])
	}
	return strings.TrimSpace(contextText + "\n" + query)
}

func diagnosisFollowUpQuery(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	return containsAny(query, "下一步", "怎么查", "接着", "继续", "也试了", "还是不行", "仍然", "还不行")
}

func diagnosisHasFaultSignal(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	return containsAny(
		query,
		"配网", "联网", "连接不上", "连不上", "绑不上",
		"充不进电", "无法充电", "不充电", "充不上电",
		"异响", "咔咔响", "嗡嗡响", "金属摩擦声",
		"漏水", "冒烟", "故障", "报错", "不工作",
		"出水小", "出水越来越小", "续航变短", "pm2.5一直",
	)
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
		IdempotencyKey: request.TraceID + ":" + name,
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
		return []string{"accessory_compatibility"}
	case FaultDiagnosisSkill:
		return []string{"troubleshooting", "user_manual", "faq"}
	case AfterSalesJudgementSkill:
		return []string{"after_sales_policy", "accessory_compatibility", "faq"}
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
			"data_scope":  value.DataScope,
			"finished_at": value.FinishedAt.Format(time.RFC3339Nano),
		},
	}
}

func toolResultSummary(name string, data any, citation string, dataScope ...string) string {
	raw, _ := json.Marshal(data)
	label := dynamicDataLabel(first(dataScope))
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
				"%s价格（查询时间 %s）：%s %s",
				label,
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
			return label + "库存：" + strings.Join(parts, "；") + " " + citation
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
					maskPublicIdentifier(item.OrderNo),
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
				maskPublicIdentifier(order.OrderNo),
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

type warrantyToolPayload struct {
	Items     []model.WarrantyStatus `json:"items"`
	CheckedAt time.Time              `json:"checked_at"`
}

func afterSalesRequestTool(query string) string {
	lower := strings.ToLower(strings.TrimSpace(query))
	if containsAny(lower, "换货", "换一台", "申请换", "确认换", "exchange") {
		return "exchange_request"
	}
	if containsAny(lower, "退货", "我要退", "申请退", "确认退", "return") {
		return "return_request"
	}
	return ""
}

func afterSalesRequestConfirmed(query, toolName string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	if containsAny(lower, "不要", "不用", "别", "暂不", "不确认", "先别", "假装", "跳过确认", "绕过确认", "当作我已经确认", "当我已经确认") {
		return false
	}
	switch toolName {
	case "exchange_request":
		return containsAny(lower, "申请换货", "确认换货", "提交换货", "办理换货", "我要换货", "帮我换货")
	case "return_request":
		return containsAny(lower, "申请退货", "确认退货", "提交退货", "办理退货", "我要退货", "帮我退货")
	default:
		return false
	}
}

func buildAfterSalesActionAnswer(toolName string, data any, citation string) string {
	var payload model.AfterSalesActionResult
	if decodeToolData(data, &payload) != nil || payload.Ticket.TicketNo == "" {
		return "售后动作已提交，但返回数据不完整，请稍后在订单售后页核对。 " + citation
	}
	actionLabel := "售后申请"
	switch toolName {
	case "return_request":
		actionLabel = "退货申请"
	case "exchange_request":
		actionLabel = "换货申请"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "**%s已提交**\n", actionLabel)
	fmt.Fprintf(&builder, "- 订单号：%s\n", maskPublicIdentifier(payload.Ticket.OrderNo))
	fmt.Fprintf(&builder, "- 工单号：%s\n", maskPublicIdentifier(payload.Ticket.TicketNo))
	fmt.Fprintf(&builder, "- 当前状态：%s\n", payload.Ticket.Status)
	if payload.QueuePosition > 0 {
		fmt.Fprintf(&builder, "- 排队位置：%d\n", payload.QueuePosition)
	}
	if payload.SLAHours > 0 {
		fmt.Fprintf(&builder, "- 预计响应：%d 小时内\n", payload.SLAHours)
	}
	if payload.NextAction != "" {
		fmt.Fprintf(&builder, "\n**下一步**\n- %s\n", payload.NextAction)
	}
	fmt.Fprintf(&builder, "\n%s", citation)
	return strings.TrimSpace(builder.String())
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
	fmt.Fprintf(&builder, "- 订单号：%s\n", maskPublicIdentifier(order.OrderNo))
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
			maskPublicIdentifier(item.OrderNo),
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

func warrantyModelMismatchNote(
	requestedModel string,
	payload warrantyToolPayload,
	toolCitation string,
) string {
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" || len(payload.Items) == 0 {
		return ""
	}
	actualModels := make([]string, 0, len(payload.Items))
	for _, item := range payload.Items {
		if strings.EqualFold(strings.TrimSpace(item.Model), requestedModel) {
			return ""
		}
		if modelName := strings.TrimSpace(item.Model); modelName != "" {
			actualModels = append(actualModels, modelName)
		}
	}
	actualModels = compactValues(actualModels)
	if len(actualModels) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"**型号核对**\n您口述的型号是 %s，但该订单记录的商品型号是 %s；以下保修判断以订单记录为准。%s",
		requestedModel,
		strings.Join(actualModels, "、"),
		toolCitation,
	)
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

func queryModels(query string) []string {
	matches := coreModelPattern.FindAllString(query, -1)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		result = append(result, canonicalModel(match))
	}
	return result
}

func canonicalModel(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if strings.EqualFold(strings.ReplaceAll(value, " ", ""), "X20Pro") {
		return "X20 Pro"
	}
	return strings.ToUpper(value)
}

func categoriesForModels(models []string) []string {
	categories := make([]string, 0, len(models))
	for _, modelName := range models {
		switch canonicalModel(modelName) {
		case "T20", "X20 Pro", "R10", "R20":
			categories = append(categories, "robot_vacuum")
		case "P400", "P500":
			categories = append(categories, "air_purifier")
		case "W300", "W500":
			categories = append(categories, "water_purifier")
		case "H100", "H200":
			categories = append(categories, "humidifier")
		}
	}
	return compactValues(categories)
}

func categoriesForQuery(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	switch {
	case containsAny(query, "扫地机", "扫地机器人", "扫拖", "机器人", "地毯", "猫毛", "宠物毛", "倒尘盒", "基站"):
		return []string{"robot_vacuum"}
	case containsAny(query, "空气净化", "净化器", "cadr", "过敏", "花粉", "pm2.5"):
		return []string{"air_purifier"}
	case containsAny(query, "净水器", "通量", "出水", "用水高峰"):
		return []string{"water_purifier"}
	case containsAny(query, "加湿器", "加湿量", "湿度", "雾化"):
		return []string{"humidifier"}
	default:
		return nil
	}
}

func priceSignalRequested(query string) bool {
	if containsAny(query, "别给我报价格", "不要报价", "不查价格", "别报价格", "不用报价格") {
		return false
	}
	return containsAny(
		query,
		"价格", "多少钱", "到手价", "查价", "实时报价", "今天价格",
		"今日价", "实时价", "总价", "哪个便宜",
		"啥价", "什么价", "几钱", "领券", "优惠券", "券",
	)
}

func recommendationQueryExpansion(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	terms := make([]string, 0, 6)
	if containsAny(query, "倒尘盒", "倒垃圾", "少维护", "不想天天倒") {
		terms = append(terms, "自动集尘")
	}
	if containsAny(query, "猫毛", "宠物毛", "养猫", "养狗") {
		terms = append(terms, "养宠 防缠绕")
	}
	if strings.Contains(query, "地毯") {
		terms = append(terms, "地毯识别 自动增压 拖布抬升")
	}
	return strings.Join(terms, " ")
}

func scenarioGuideQueryExpansion(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	terms := make([]string, 0, 8)
	if containsAny(query, "小户型", "六十平", "60平", "没地毯", "无地毯") {
		terms = append(terms, "小户型 40-80平 硬质地板 预算优先")
	}
	if containsAny(query, "两只猫", "养猫", "宠物", "猫毛") {
		terms = append(terms, "养宠 大户型 防缠绕 地毯")
	}
	if containsAny(query, "新房", "刚装完", "新装修") {
		terms = append(terms, "新装修 颗粒物 气态污染物 VOC 持续监测")
	}
	if containsAny(query, "花粉", "鼻子难受", "过敏") {
		terms = append(terms, "过敏 花粉 卧室 夜间低噪")
	}
	if containsAny(query, "大客厅", "五十多平", "50平") {
		terms = append(terms, "大客厅 45-70平 持续净化")
	}
	if containsAny(query, "租房", "租住", "安装限制", "安装改造") {
		terms = append(terms, "租住家庭 厨下空间 安装改造 进水排水电源")
	}
	if containsAny(query, "五口", "大家庭", "用水高峰", "连续接水") {
		terms = append(terms, "五口以上 大家庭 早晚高峰 连续用水")
	}
	if containsAny(query, "婴儿房", "儿童房") {
		terms = append(terms, "婴儿房 夜间安静 易清洁")
		if containsAny(query, "净化加湿", "净化和加湿", "净化、加湿") {
			terms = append(terms, "过敏 花粉 卧室 夜间低噪 空气净化")
		}
	}
	if containsAny(query, "北方", "供暖季", "少加水", "客厅") {
		terms = append(terms, "北方供暖季 客厅 长时间运行 减少加水")
	}
	return strings.Join(compactValues(terms), " ")
}

func recommendationCandidateModels(query string, categories []string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	category := first(categories)
	switch category {
	case "robot_vacuum":
		switch {
		case containsAny(query, "倒尘盒", "自动集尘", "不想天天倒", "少维护"):
			return []string{"X20 Pro", "R20"}
		case containsAny(query, "猫毛", "宠物毛", "养猫", "养狗", "地毯", "复式", "大户型", "150平"):
			return []string{"X20 Pro", "R20"}
		case containsAny(query, "手动倒尘", "2500", "两千五"):
			return []string{"T20", "R10"}
		default:
			return []string{"T20", "X20 Pro"}
		}
	case "air_purifier":
		return []string{"P400", "P500"}
	case "water_purifier":
		return []string{"W300", "W500"}
	case "humidifier":
		return []string{"H100", "H200"}
	default:
		return nil
	}
}

func afterSalesPolicyQuery(intentType intent.Type, query string) string {
	switch intentType {
	case intent.WarrantyQuery:
		return query + " 清洁电器保修政策 保修起算时间 订单保修月数 延保适用条件"
	case intent.ReturnEligibility:
		return query + " 七天无理由退货政策 质量问题换货政策 耗材退换 条件与例外"
	default:
		return query + " 售后政策 适用条件 例外"
	}
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func refersToPurchase(query string) bool {
	for _, keyword := range []string{
		"上周买", "上礼拜", "之前买", "以前买", "我买的", "买过", "购买记录",
		"上次", "那个净化器", "查订单", "订单后", "买没买", "延保", "保到哪天", "还保不保",
		"还在保", "在保", "保内",
		"买的", "买了", "啥时候买", "什么时候买", "账号里", "记录里", "最近一单", "历史记录",
	} {
		if strings.Contains(query, keyword) {
			return true
		}
	}
	return false
}

func purchaseRecords(data any) []model.PurchaseRecord {
	raw, _ := json.Marshal(data)
	var payload struct {
		Items []model.PurchaseRecord `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload.Items
}

func purchaseOrderNos(records []model.PurchaseRecord, models []string) []string {
	targets := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		if modelName = canonicalModel(modelName); modelName != "" {
			targets[modelName] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	var result []string
	var fallback []string
	for _, record := range records {
		if record.OrderNo == "" {
			continue
		}
		if _, ok := seen[record.OrderNo]; ok {
			continue
		}
		seen[record.OrderNo] = struct{}{}
		if len(fallback) < 3 {
			fallback = append(fallback, record.OrderNo)
		}
		if len(targets) > 0 {
			if _, ok := targets[canonicalModel(record.Model)]; !ok {
				continue
			}
		}
		result = append(result, record.OrderNo)
		if len(result) >= 3 {
			break
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func purchaseWarrantyModelMismatchNote(targetModel string, warranties []model.WarrantyStatus) string {
	targetModel = canonicalModel(targetModel)
	if targetModel == "" || len(warranties) == 0 {
		return ""
	}
	for _, item := range warranties {
		if canonicalModel(item.Model) == targetModel {
			return ""
		}
	}
	return fmt.Sprintf("购买记录中未找到 %s 的可核验订单；以下仅列出当前账号已定位订单的保修结果，请以订单记录为准。", targetModel)
}

func buildPurchaseWarrantyAnswer(
	records []model.PurchaseRecord,
	warranties []model.WarrantyStatus,
	purchaseCitation string,
	warrantyCitation string,
) string {
	if len(warranties) == 0 {
		return "已查询购买记录，但暂未取得可用的保修结果；请补充订单号后重试。" + purchaseCitation
	}
	recordsByOrder := map[string]model.PurchaseRecord{}
	for _, record := range records {
		if record.OrderNo != "" {
			recordsByOrder[record.OrderNo] = record
		}
	}
	var builder strings.Builder
	builder.WriteString("**保修核验结果**\n")
	for _, item := range warranties {
		status := "不在保"
		if item.InWarranty {
			status = "在保"
		}
		record := recordsByOrder[item.OrderNo]
		fmt.Fprintf(
			&builder,
			"- %s（%s，订单 %s）：%s，保修截止 %s。",
			item.Model,
			item.ProductName,
			maskPublicIdentifier(item.OrderNo),
			status,
			formatOptionalFactTime(item.WarrantyEnd),
		)
		if !record.PaidAt.IsZero() {
			fmt.Fprintf(&builder, " 购买时间 %s。", formatFactTime(record.PaidAt))
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n依据：购买记录")
	builder.WriteString(purchaseCitation)
	builder.WriteString("；保修工具")
	builder.WriteString(warrantyCitation)
	builder.WriteString("。")
	return builder.String()
}

func purchaseModels(data any) []string {
	items := purchaseRecords(data)
	seen := make(map[string]struct{})
	result := make([]string, 0, len(items))
	for _, item := range items {
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

func accessoryModelsForHost(items []rag.SearchResult, hostModel string) []string {
	hostModel = strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(hostModel)), " "))
	if hostModel == "" {
		return accessoryModels(items)
	}
	seen := make(map[string]struct{})
	var result []string
	for _, item := range items {
		text := strings.ToUpper(strings.Join(strings.Fields(item.Title+"\n"+item.Content), " "))
		if !strings.Contains(text, hostModel) {
			continue
		}
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
	add := func(modelName string) bool {
		modelName = canonicalModel(modelName)
		if modelName == "" {
			return false
		}
		if _, ok := seen[modelName]; ok {
			return false
		}
		seen[modelName] = struct{}{}
		result = append(result, modelName)
		return len(result) == 4
	}
	for _, item := range items {
		if modelName, ok := item.Metadata["model"].(string); ok && add(modelName) {
			break
		}
		for _, modelName := range stringValues(item.Metadata["models"]) {
			if add(modelName) {
				break
			}
		}
		for _, modelName := range splitModelCandidates(item.Metadata["candidate_models"]) {
			if add(modelName) {
				break
			}
		}
		for _, modelName := range coreModelPattern.FindAllString(item.Title+"\n"+item.Content, -1) {
			if add(modelName) {
				break
			}
		}
		if len(result) == 4 {
			break
		}
	}
	return result
}

func splitModelCandidates(value any) []string {
	text, _ := value.(string)
	text = strings.NewReplacer("、", ",", "，", ",", "/", ",").Replace(text)
	return compactValues(strings.Split(text, ","))
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	case string:
		return strings.Split(typed, ",")
	default:
		return nil
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
