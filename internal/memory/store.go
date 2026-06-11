package memory

import (
	"context"
	"time"

	"CleanCaregent/internal/model"
)

type DiagnosisState struct {
	ConversationID string
	ProductModel   string
	FaultNodeID    string
	Answers        map[string]string
	UpdatedAt      time.Time
}

type ConversationContext struct {
	ConversationID          string
	Summary                 string
	SummaryThroughMessageID string
	RecentMessages          []model.Message
	KnownEntities           map[string]string
	DiagnosisState          *DiagnosisState
}

type Store interface {
	LoadContext(ctx context.Context, conversationID string, recentLimit int) (*ConversationContext, error)
	AppendMessage(ctx context.Context, message model.Message) error
	SaveSummary(ctx context.Context, conversationID, summary, throughMessageID string) error
	SetEntity(ctx context.Context, conversationID, key, value string, ttl time.Duration) error
	LoadDiagnosisState(ctx context.Context, conversationID string) (*DiagnosisState, error)
	SaveDiagnosisState(ctx context.Context, state DiagnosisState) error
}
