package agent

import (
	"context"
	"time"

	"CleanCaregent/internal/intent"
)

type ActionType string

const (
	ActionAnswerDirect ActionType = "answer_direct"
	ActionRetrieve     ActionType = "retrieve"
	ActionCallTool     ActionType = "call_tool"
	ActionRunSkill     ActionType = "run_skill"
	ActionParallel     ActionType = "parallel"
	ActionClarify      ActionType = "clarify"
	ActionReflect      ActionType = "reflect"
	ActionFinish       ActionType = "finish"
)

type PlanRequest struct {
	TraceID          string
	UserID           string
	ConversationID   string
	Query            string
	RewrittenQueries []string
	Intent           intent.Result
	AllowedTools     []string
	MaxSteps         int
	TokenBudget      int
	Deadline         time.Time
}

type Plan struct {
	ID              string
	Mode            string
	Intent          intent.Type
	Steps           []PlanStep
	MaxSteps        int
	TokenBudget     int
	Confidence      float64
	FallbackMessage string
}

type PlanStep struct {
	StepID     string
	Action     ActionType
	SkillName  string
	ToolName   string
	Query      string
	Params     map[string]any
	ReasonCode string
	DependsOn  []string
	SubSteps   []PlanStep
}

type PlanEvaluation struct {
	Score    int
	Warnings []string
}

type NextStepValidator interface {
	ValidateNextStep(
		ctx context.Context,
		req PlanRequest,
		candidate PlanStep,
		recent []PlanStep,
	) error
}

type Planner interface {
	Plan(ctx context.Context, req PlanRequest) (*Plan, error)
}

// ReactivePlanner is implemented by planners that can choose one action at a
// time after observing retrieval and tool results. The runner falls back to
// the static Plan steps when this optional interface is unavailable.
type ReactivePlanner interface {
	Planner
	NextStep(
		ctx context.Context,
		req PlanRequest,
		currentStep int,
		evidences []Evidence,
		searchResults string,
		toolResults string,
	) (*PlanStep, error)
}

// PlanAndExecutePlanner is an optional capability for planners that create
// the complete dependency-aware plan before execution and can revise the
// remaining steps after an observation or execution failure.
type PlanAndExecutePlanner interface {
	Planner
	CompletePlan(ctx context.Context, req PlanRequest) (*Plan, error)
	RevisePlan(
		ctx context.Context,
		req PlanRequest,
		current *Plan,
		completed []PlanStep,
		evidences []Evidence,
		cause error,
	) (*Plan, error)
}
