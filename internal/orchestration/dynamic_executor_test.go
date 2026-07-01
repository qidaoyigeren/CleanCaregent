package orchestration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CleanCaregent/internal/agent"
	"CleanCaregent/internal/compatibility"
	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/model"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/skill"
	"CleanCaregent/internal/tool"
	toolmcp "CleanCaregent/internal/tool/mcp"
)

type chainSkill struct {
	name   string
	result *skill.Result
}

func (s chainSkill) Name() string               { return s.name }
func (s chainSkill) CanHandle(intent.Type) bool { return true }
func (s chainSkill) Run(context.Context, skill.Request) (*skill.Result, error) {
	return s.result, nil
}

type recordingSkill struct {
	request skill.Request
}

func (s *recordingSkill) Name() string               { return "recording" }
func (s *recordingSkill) CanHandle(intent.Type) bool { return true }
func (s *recordingSkill) Run(_ context.Context, request skill.Request) (*skill.Result, error) {
	s.request = request
	return &skill.Result{Status: "success", AnswerDraft: "ok"}, nil
}

type timeoutTool struct{}

func (timeoutTool) Name() string        { return "price_query" }
func (timeoutTool) Description() string { return "timeout" }
func (timeoutTool) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","required":["product_refs"],"properties":{"product_refs":{"type":"array"}}}`)
}
func (timeoutTool) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	<-ctx.Done()
	return tool.Result{CallID: call.CallID}, ctx.Err()
}

type fallbackRetriever struct{}

func (fallbackRetriever) Search(context.Context, rag.SearchRequest) ([]rag.SearchResult, error) {
	return []rag.SearchResult{{
		ChunkID: "chunk_t20", DocumentID: "doc_t20", Title: "T20 商品详情",
		Content: "T20 静态商品参数。",
	}}, nil
}

func TestDynamicExecutorChainsSkills(t *testing.T) {
	registry := skill.NewRegistry()
	if err := registry.Register(chainSkill{name: "first", result: &skill.Result{
		Status: "success", NextSkill: "second", NextSkillArgs: map[string]any{"model": "T20"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(chainSkill{name: "second", result: &skill.Result{
		Status: "success", AnswerDraft: "链式执行完成",
	}}); err != nil {
		t.Fatal(err)
	}
	executor := NewDynamicExecutor(nil, registry)
	result, err := executor.Execute(context.Background(), agent.DynamicExecutionRequest{
		Request: agent.Request{TraceID: "tr_chain"},
		Intent:  string(intent.ProductComparison),
		Step: agent.PlanStep{
			Action: agent.ActionRunSkill, SkillName: "first", Query: "比较产品",
			Params: map[string]any{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != "链式执行完成" || result.Metadata["skill_chain_length"] != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestDynamicExecutorPassesCompoundTargetIntentToSkill(t *testing.T) {
	registry := skill.NewRegistry()
	recorded := &recordingSkill{}
	if err := registry.Register(recorded); err != nil {
		t.Fatal(err)
	}
	executor := NewDynamicExecutor(nil, registry)
	_, err := executor.Execute(context.Background(), agent.DynamicExecutionRequest{
		Request:          agent.Request{TraceID: "tr_target_intent"},
		Intent:           string(intent.Troubleshooting),
		SecondaryIntents: []string{string(intent.WarrantyQuery), string(intent.OrderQuery)},
		Step: agent.PlanStep{
			Action:    agent.ActionRunSkill,
			SkillName: "recording",
			Query:     "故障并查询保修",
			Params: map[string]any{
				"target_intent": string(intent.WarrantyQuery),
				"order_no":      "CC20260603001",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorded.request.Intent.Secondary != intent.WarrantyQuery {
		t.Fatalf("secondary = %q", recorded.request.Intent.Secondary)
	}
	if len(recorded.request.Intent.SecondaryIntents) != 2 {
		t.Fatalf("secondary intents = %#v", recorded.request.Intent.SecondaryIntents)
	}
	if _, exists := recorded.request.Intent.Entities["target_intent"]; exists {
		t.Fatal("target_intent leaked into skill entities")
	}
}

func TestDynamicExecutorFallsBackToKnowledgeOnTimeout(t *testing.T) {
	server, err := toolmcp.NewServer(timeoutTool{})
	if err != nil {
		t.Fatal(err)
	}
	executor := NewDynamicExecutor(
		tool.NewExecutor(toolmcp.NewInProcessClient(server), nil, 10*time.Millisecond),
		nil,
		WithKnowledgeRetriever(fallbackRetriever{}),
	)
	result, err := executor.Execute(context.Background(), agent.DynamicExecutionRequest{
		Request: agent.Request{TraceID: "tr_timeout", Query: "T20 多少钱"},
		Intent:  string(intent.PriceQuery),
		Step: agent.PlanStep{
			Action: agent.ActionCallTool, ToolName: "price_query",
			Params: map[string]any{"product_refs": []string{"T20"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metadata["degrade_strategy"] != "tool_timeout_to_kb" || len(result.SearchData) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestNormalizeExtractedArgumentsRejectsUnknownEntities(t *testing.T) {
	result := normalizeExtractedArguments(map[string]any{
		"product_refs": []any{"T20", "SB3", "SP2-NZ", "UNKNOWN-9", "CC20260603001"},
		"order_no":     "invalid",
	})
	products, ok := result["product_refs"].([]string)
	if !ok || len(products) != 3 || products[0] != "T20" || products[1] != "SB3" || products[2] != "SP2-NZ" {
		t.Fatalf("result = %#v", result)
	}
	if _, exists := result["order_no"]; exists {
		t.Fatalf("unexpected order_no in %#v", result)
	}
}

func TestProductRefsForDynamicToolPreferAccessoryRefs(t *testing.T) {
	refs := (&DynamicExecutor{}).productRefsForDynamicTool(
		"inventory_check",
		"W300换C300步骤给我，配件现在有货不",
		nil,
		[]string{"W300", "C300"},
	)
	if len(refs) != 1 || refs[0] != "C300" {
		t.Fatalf("refs = %#v, want only C300", refs)
	}
}

func TestBuildArgumentsMapsHostAccessoryRequestThroughCompatibilityMatrix(t *testing.T) {
	executor := &DynamicExecutor{compatibility: compatibility.NewMatrix([]compatibility.Entry{{
		HostModel:      "SB3",
		AccessoryModel: "SB3-BH",
		AccessoryType:  "brush_head",
		Status:         compatibility.Compatible,
	}})}
	args := executor.buildArguments(context.Background(), agent.DynamicExecutionRequest{
		Request: agent.Request{Query: "替换刷头一起多少钱，还有货吗？"},
		Step: agent.PlanStep{
			Action:   agent.ActionCallTool,
			ToolName: "price_query",
			Params: map[string]any{
				"accessory_refs": "刷头",
				"product_refs":   []string{"SB3"},
			},
		},
	})
	refs := stringSliceArgument(args["product_refs"])
	if len(refs) != 1 || refs[0] != "SB3-BH" {
		t.Fatalf("product_refs = %#v, want SB3-BH", args["product_refs"])
	}
}

func TestBuildArgumentsKeepsHostAndInferredAccessoryForSeparateInventory(t *testing.T) {
	executor := &DynamicExecutor{compatibility: compatibility.NewMatrix([]compatibility.Entry{{
		HostModel:      "FD4",
		AccessoryModel: "FD4-PAD",
		AccessoryType:  "sleeve_pad",
		Status:         compatibility.Compatible,
	}})}
	args := executor.buildArguments(context.Background(), agent.DynamicExecutionRequest{
		Request: agent.Request{Query: "FD4 和替换掸套分别有货吗？"},
		Step: agent.PlanStep{
			Action:   agent.ActionCallTool,
			ToolName: "inventory_check",
			Params: map[string]any{
				"accessory_refs": "掸套",
				"product_refs":   []string{"FD4"},
			},
		},
	})
	refs := stringSliceArgument(args["product_refs"])
	if len(refs) != 2 || refs[0] != "FD4" || refs[1] != "FD4-PAD" {
		t.Fatalf("product_refs = %#v, want FD4 and FD4-PAD", args["product_refs"])
	}
}

func TestBuildArgumentsKeepsHyphenatedAccessoryRefs(t *testing.T) {
	executor := &DynamicExecutor{}
	args := executor.buildArguments(context.Background(), agent.DynamicExecutionRequest{
		Request: agent.Request{Query: "SB3-BH 能不能装在 SB3 上？顺便查一下刷头价格。"},
		Step: agent.PlanStep{
			Action:   agent.ActionCallTool,
			ToolName: "price_query",
			Params:   map[string]any{"models": "SB3-BH,SB3"},
		},
	})
	refs := stringSliceArgument(args["product_refs"])
	if len(refs) != 1 || refs[0] != "SB3-BH" {
		t.Fatalf("product_refs = %#v, want SB3-BH", args["product_refs"])
	}
}

func TestFormatToolAnswerMarksMockDynamicData(t *testing.T) {
	answer := formatToolAnswer("price_query", map[string]any{
		"items": []map[string]any{{
			"product_name":                "CleanCare T20",
			"sku_name":                    "标准版",
			"current_price_cents":         359900,
			"estimated_final_price_cents": 349900,
			"available_stock":             18,
		}},
	}, "mock")
	if !strings.Contains(answer, "模拟动态价格") {
		t.Fatalf("answer = %q", answer)
	}
}

func TestFormatToolAnswerDescribesCreatedTicket(t *testing.T) {
	answer := formatToolAnswer("create_after_sales_ticket", model.AfterSalesTicket{
		TicketNo: "AS_TEST_001",
		OrderNo:  "CC20260603001",
		Status:   "created",
	}, "mock")
	for _, expected := range []string{
		"模拟动态售后工单",
		"明确确认",
		"AS_TEST_001",
		"CC****3001",
		"幂等",
	} {
		if !strings.Contains(answer, expected) {
			t.Fatalf("answer = %q, missing %q", answer, expected)
		}
	}
}

func TestTicketConfirmationPresentRejectsNegation(t *testing.T) {
	for _, query := range []string{
		"我没确认创建售后工单",
		"订单没问题，但不要创建工单",
		"我未确认提交",
	} {
		if ticketConfirmationPresent(query) {
			t.Fatalf("query %q must not authorize ticket creation", query)
		}
	}
	if !ticketConfirmationPresent("我确认创建售后工单") {
		t.Fatal("explicit confirmation was not recognized")
	}
}
