package api

import (
	"net/http"
	"sort"
	"strings"

	"CleanCaregent/internal/eval"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/pkg/response"
	"github.com/gin-gonic/gin"
)

type PromptHandler struct {
	registry  *prompt.Registry
	evaluator prompt.VersionEvaluator
}

type comparePromptRequest struct {
	Scenario    prompt.Scenario `json:"prompt_scenario" binding:"required"`
	VersionA    string          `json:"version_a" binding:"required"`
	VersionB    string          `json:"version_b" binding:"required"`
	EvalCaseIDs []string        `json:"eval_case_ids"`
}

type activatePromptRequest struct {
	Version string `json:"version" binding:"required"`
}

// NewPromptHandler creates the Prompt version evaluation handler.
func NewPromptHandler(registry *prompt.Registry, evaluator prompt.VersionEvaluator) *PromptHandler {
	return &PromptHandler{registry: registry, evaluator: evaluator}
}

// List returns all prompt scenarios, active versions, version history, and
// active template content for the administrator UI.
func (h *PromptHandler) List(c *gin.Context) {
	if h.registry == nil {
		response.Error(c, http.StatusServiceUnavailable, "PROMPT_REGISTRY_UNAVAILABLE", "Prompt 注册表未配置")
		return
	}
	scenarios := []prompt.Scenario{
		prompt.ScenarioSystem,
		prompt.ScenarioIntent,
		prompt.ScenarioRewrite,
		prompt.ScenarioPlan,
		prompt.ScenarioGenerateGeneric,
		prompt.ScenarioGenerateCompare,
		prompt.ScenarioGenerateDiagnose,
		prompt.ScenarioGeneratePolicy,
		prompt.ScenarioReflect,
		prompt.ScenarioClarify,
		prompt.ScenarioSummarize,
		prompt.ScenarioEvalJudge,
	}
	items := make([]map[string]any, 0, len(scenarios))
	for _, scenario := range scenarios {
		template, err := h.registry.Get(scenario)
		if err != nil {
			continue
		}
		versions := h.registry.Versions(scenario)
		sort.Strings(versions)
		items = append(items, map[string]any{
			"scenario":       scenario,
			"active_version": template.Version,
			"versions":       versions,
			"system":         template.System,
			"user":           template.User,
		})
	}
	response.OK(c, map[string]any{"items": items})
}

// Activate switches a prompt scenario to a registered historical version.
func (h *PromptHandler) Activate(c *gin.Context) {
	if h.registry == nil {
		response.Error(c, http.StatusServiceUnavailable, "PROMPT_REGISTRY_UNAVAILABLE", "Prompt 注册表未配置")
		return
	}
	scenario := prompt.Scenario(strings.TrimSpace(c.Param("scenario")))
	var request activatePromptRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "Prompt 版本参数无效")
		return
	}
	if err := h.registry.Activate(scenario, strings.TrimSpace(request.Version)); err != nil {
		response.Error(c, http.StatusBadRequest, "PROMPT_ACTIVATE_FAILED", err.Error())
		return
	}
	response.OK(c, map[string]any{
		"scenario": scenario,
		"version":  h.registry.Version(scenario),
	})
}

// Compare evaluates two Prompt versions with the same cases.
func (h *PromptHandler) Compare(c *gin.Context) {
	if h.registry == nil || h.evaluator == nil {
		response.Error(c, http.StatusServiceUnavailable, "PROMPT_EVAL_UNAVAILABLE", "Prompt A/B 评估未配置")
		return
	}
	var request comparePromptRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "Prompt A/B 参数无效")
		return
	}
	cases := selectPromptEvalCases(request.EvalCaseIDs, 20)
	if len(cases) == 0 {
		response.Error(c, http.StatusBadRequest, "EVAL_CASE_NOT_FOUND", "未找到可用评估案例")
		return
	}
	result, err := h.registry.CompareVersions(
		c.Request.Context(),
		request.Scenario,
		request.VersionA,
		request.VersionB,
		cases,
		h.evaluator,
	)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "PROMPT_EVAL_FAILED", err.Error())
		return
	}
	response.OK(c, result)
}

func selectPromptEvalCases(ids []string, limit int) []prompt.EvaluationCase {
	selected := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		selected[id] = struct{}{}
	}
	result := make([]prompt.EvaluationCase, 0, limit)
	for _, evalCase := range eval.DefaultCases() {
		if len(selected) > 0 {
			if _, ok := selected[evalCase.CaseID]; !ok {
				continue
			}
		}
		result = append(result, prompt.EvaluationCase{
			ID: evalCase.CaseID, Query: evalCase.Query, Expected: evalCase.StandardAnswer,
		})
		if len(result) == limit {
			break
		}
	}
	return result
}
