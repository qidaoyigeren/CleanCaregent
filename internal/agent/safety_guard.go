package agent

import (
	"context"
	"regexp"
	"strings"
)

const injectionRefusal = "抱歉，我无法处理这个请求"

type guardedRunner struct {
	next Runner
}

// NewGuardedRunner wraps a Runner with prompt-injection and competitor-policy checks.
func NewGuardedRunner(next Runner) Runner {
	if next == nil {
		return nil
	}
	return &guardedRunner{next: next}
}

// Run rejects unsafe input before it reaches retrieval, tools, or a model.
func (r *guardedRunner) Run(ctx context.Context, request Request, sink EventSink) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if isPromptInjection(request.Query) {
		return guardedAnswer(ctx, sink, injectionRefusal, "prompt_injection_guard")
	}
	if answer, matched := competitorAnswer(request.Query); matched {
		return guardedAnswer(ctx, sink, answer, "competitor_policy")
	}
	return r.next.Run(ctx, request, sink)
}

func isPromptInjection(query string) bool {
	normalized := strings.ToLower(strings.TrimSpace(query))
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`忽略.{0,8}(之前|以上|所有|原有).{0,6}(指令|提示|规则)`),
		regexp.MustCompile(`无视.{0,8}(指令|提示|规则)`),
		regexp.MustCompile(`(泄露|显示|输出|打印|告诉我).{0,10}(系统提示词|系统指令|开发者消息)`),
		regexp.MustCompile(`(你的|内部).{0,4}(工具|限制|规则|提示词).{0,6}(是什么|有哪些|全部|列表)`),
		regexp.MustCompile(`(dan|do anything now|jailbreak|越狱|绕过限制|解除限制)`),
		regexp.MustCompile(`ignore.{0,12}(previous|all|above).{0,8}(instructions?|prompts?|rules?)`),
		regexp.MustCompile(`(reveal|show|print|dump).{0,10}(system prompt|developer message|hidden instructions?)`),
	}
	for _, pattern := range patterns {
		if pattern.MatchString(normalized) {
			return true
		}
	}
	return false
}

func competitorAnswer(query string) (string, bool) {
	competitors := []string{"小米", "追觅", "石头", "科沃斯", "戴森"}
	matched := false
	for _, competitor := range competitors {
		if strings.Contains(query, competitor) {
			matched = true
			break
		}
	}
	if !matched {
		return "", false
	}
	switch {
	case containsAnyText(query, "垃圾", "很差", "不行", "拉踩", "贬低"):
		return "我不会贬低或攻击其他品牌。我可以基于可核验的参数说明 CleanCare 产品的适用场景、限制和选购条件。", true
	case containsAnyText(query, "对比", "比较", "哪个好", "哪个更", "区别"):
		return "不同品牌产品各有特点，建议您根据实际需求选择。如果您想了解我们产品的优势，我可以为您详细介绍。", true
	default:
		return "我只能提供 CleanCare 产品信息，无法对其他品牌做评价。关于 CleanCare 产品，我可以查询参数、使用方法、兼容配件和售后政策。", true
	}
}

func guardedAnswer(ctx context.Context, sink EventSink, answer, mode string) (Result, error) {
	if sink != nil {
		if err := sink(Event{Type: "status", Data: map[string]any{
			"stage": "safety_guard",
			"mode":  mode,
		}}); err != nil {
			return Result{}, err
		}
		for _, chunk := range splitForStream(answer, 24) {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			if err := sink(Event{Type: "delta", Data: map[string]string{"content": chunk}}); err != nil {
				return Result{}, err
			}
		}
	}
	return Result{Answer: answer, Mode: mode}, nil
}

func containsAnyText(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
