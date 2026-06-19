package skill

import (
	"context"

	"CleanCaregent/internal/agent"
	"CleanCaregent/internal/intent"
)

type Request struct {
	TraceID        string
	UserID         string
	ConversationID string
	Query          string
	ContextText    string
	Intent         intent.Result
	Entities       map[string]any
}

type Result struct {
	Status        string
	AnswerDraft   string
	Evidences     []agent.Evidence
	NextQuestion  string
	NextSkill     string
	NextSkillArgs map[string]any
	Metadata      map[string]any
}

type Skill interface {
	Name() string
	CanHandle(intent intent.Type) bool
	Run(ctx context.Context, req Request) (*Result, error)
}
