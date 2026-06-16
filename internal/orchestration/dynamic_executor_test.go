package orchestration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CleanCaregent/internal/agent"
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
		"product_refs": []any{"T20", "UNKNOWN-9"},
		"order_no":     "invalid",
	})
	products, ok := result["product_refs"].([]string)
	if !ok || len(products) != 1 || products[0] != "T20" {
		t.Fatalf("result = %#v", result)
	}
	if _, exists := result["order_no"]; exists {
		t.Fatalf("unexpected order_no in %#v", result)
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
		"CC20260603001",
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
