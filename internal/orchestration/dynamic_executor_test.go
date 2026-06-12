package orchestration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CleanCaregent/internal/agent"
	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/skill"
	"CleanCaregent/internal/tool"
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

func TestDynamicExecutorFallsBackToKnowledgeOnTimeout(t *testing.T) {
	registry := tool.NewRegistry()
	if err := registry.Register(timeoutTool{}); err != nil {
		t.Fatal(err)
	}
	executor := NewDynamicExecutor(
		tool.NewExecutor(registry, nil, 10*time.Millisecond),
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
