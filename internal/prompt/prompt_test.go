package prompt

import (
	"regexp"
	"strings"
	"testing"
)

func TestRegistryKeepsHistoryAndCanRollback(t *testing.T) {
	registry := NewRegistry()
	original := registry.Version(ScenarioIntent)

	registry.Set(&Template{
		Scenario: ScenarioIntent,
		Version:  "v2-test",
		System:   "system-v2",
		User:     "{query}",
	})
	if got := registry.Version(ScenarioIntent); got != "v2-test" {
		t.Fatalf("active version = %q, want v2-test", got)
	}

	if err := registry.Activate(ScenarioIntent, original); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	template, err := registry.Get(ScenarioIntent)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if template.Version != original {
		t.Fatalf("rolled back version = %q, want %q", template.Version, original)
	}
	if len(registry.Versions(ScenarioIntent)) < 2 {
		t.Fatalf("Versions() = %#v, want at least two versions", registry.Versions(ScenarioIntent))
	}
}

func TestRegistryGetReturnsCopy(t *testing.T) {
	registry := NewRegistry()
	first, err := registry.Get(ScenarioSystem)
	if err != nil {
		t.Fatal(err)
	}
	first.System = "mutated"
	second, err := registry.Get(ScenarioSystem)
	if err != nil {
		t.Fatal(err)
	}
	if second.System == "mutated" {
		t.Fatal("Get() exposed mutable registry state")
	}
}

func TestDefaultTemplatesUseV2Content(t *testing.T) {
	registry := NewRegistry()
	expectations := map[Scenario]string{
		ScenarioSystem:           "# 输出前自检清单",
		ScenarioIntent:           "# Few-Shot 示例",
		ScenarioRewrite:          "# 清洁电器领域术语归一化词典",
		ScenarioPlan:             "# 完整 ReAct 推理链示例",
		ScenarioGenerateGeneric:  "# 输出模板",
		ScenarioGenerateCompare:  "养猫家庭更推荐 X20 Pro",
		ScenarioGenerateDiagnose: "充电座的指示灯现在是什么状态",
		ScenarioGeneratePolicy:   "# 输出示例",
		ScenarioReflect:          "## 7. 安全红线",
		ScenarioClarify:          "## 示例7：指代回溯",
		ScenarioSummarize:        "# 示例",
		ScenarioEvalJudge:        "# 评估示例",
	}

	for scenario, marker := range expectations {
		template, err := registry.Get(scenario)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", scenario, err)
		}
		if template.Version != "v2" {
			t.Errorf("Get(%q).Version = %q, want v2", scenario, template.Version)
		}
		if !strings.Contains(template.System, marker) {
			t.Errorf("Get(%q).System does not contain %q", scenario, marker)
		}
	}
}

func TestDefaultTemplatesSatisfyPromptContract(t *testing.T) {
	registry := NewRegistry()
	scenarios := []Scenario{
		ScenarioSystem,
		ScenarioIntent,
		ScenarioRewrite,
		ScenarioPlan,
		ScenarioGenerateGeneric,
		ScenarioGenerateCompare,
		ScenarioGenerateDiagnose,
		ScenarioGeneratePolicy,
		ScenarioReflect,
		ScenarioClarify,
		ScenarioSummarize,
		ScenarioEvalJudge,
	}
	requiredSections := []string{
		"# 身份",
		"# 任务",
		"# 业务背景",
		"# 输出标准",
		"# 约束规则",
		"# Few-Shot 示例",
		"# 输出前自检",
	}

	for _, scenario := range scenarios {
		template := registry.MustGet(scenario)
		for _, section := range requiredSections {
			if !strings.Contains(template.System, section) {
				t.Errorf("%s missing prompt contract section %q", scenario, section)
			}
		}
		if examples := strings.Count(template.System, "## 示例"); examples < 2 {
			t.Errorf("%s has %d complete examples, want at least 2", scenario, examples)
		}
	}
}

func TestDefaultTemplateUserPlaceholdersRender(t *testing.T) {
	registry := NewRegistry()
	paramsByScenario := map[Scenario]map[string]string{
		ScenarioSystem: {},
		ScenarioIntent: {
			"summary": "summary", "recent_messages": "messages", "query": "query",
		},
		ScenarioRewrite: {
			"summary": "summary", "known_entities": "entities",
			"intent_type": "intent", "query": "query",
		},
		ScenarioPlan: {
			"max_steps": "5", "query": "query", "intent_info": "intent",
			"sub_questions": "questions", "evidence_summary": "evidence",
			"step_info": "1/5", "tool_definitions": "tools",
		},
		ScenarioGenerateGeneric: {
			"query": "query", "evidence_context": "evidence",
			"tool_results": "tools", "conversation_summary": "summary",
		},
		ScenarioGenerateCompare: {
			"models": "T20,X20 Pro", "concerns": "吸力",
			"evidence_context": "evidence", "tool_results": "tools",
		},
		ScenarioGenerateDiagnose: {
			"model": "T20", "symptom": "无法充电", "diagnosis_state": "start",
			"current_node": "charging", "evidence_context": "evidence",
		},
		ScenarioGeneratePolicy: {
			"query": "query", "order_info": "order",
			"warranty_info": "warranty", "evidence_context": "evidence",
		},
		ScenarioReflect: {
			"original_query": "query", "sub_questions": "questions",
			"draft_answer": "answer", "evidence_context": "evidence",
			"tool_calls": "tools",
		},
		ScenarioClarify: {
			"intent_type": "intent", "known_info": "known",
			"missing_info": "missing", "query": "query",
		},
		ScenarioSummarize: {
			"previous_summary": "summary", "messages": "messages",
		},
		ScenarioEvalJudge: {
			"query": "query", "standard_answer": "standard",
			"contexts": "contexts", "actual_answer": "answer",
		},
	}
	placeholderPattern := regexp.MustCompile(`\{[a-z_]+\}`)

	for scenario, params := range paramsByScenario {
		template := registry.MustGet(scenario)
		system, user := template.Render(params)
		if scenario == ScenarioPlan && strings.Contains(system, "{max_steps}") {
			t.Errorf("Render(%q) left {max_steps} in system prompt", scenario)
		}
		if placeholder := placeholderPattern.FindString(user); placeholder != "" {
			t.Errorf("Render(%q) left user placeholder %q", scenario, placeholder)
		}
	}
}
