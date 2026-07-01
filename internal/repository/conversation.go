package repository

import (
	"context"
	"errors"
	"time"

	"CleanCaregent/internal/model"
)

var (
	ErrConversationNotFound  = errors.New("conversation not found")
	ErrConversationForbidden = errors.New("conversation forbidden")
	ErrMessageRequestFailed  = errors.New("message request failed")
)

const (
	MessageRequestRunning = "running"
	MessageRequestDone    = "done"
	MessageRequestFailed  = "failed"
)

type ConversationRepository interface {
	Create(ctx context.Context, conversation model.Conversation) error
	Get(ctx context.Context, userID, conversationID string) (model.Conversation, error)
	List(ctx context.Context, userID string, limit int) ([]model.Conversation, error)
	AppendMessage(ctx context.Context, userID string, message model.Message) error
	ListMessages(ctx context.Context, userID, conversationID string, limit int) ([]model.Message, error)
}

type IdempotentConversationRepository interface {
	FindMessageByClientMessageID(ctx context.Context, userID, conversationID, role, clientMessageID string) (model.Message, error)
}

type MessageRequest struct {
	ConversationID  string
	ClientMessageID string
	Status          string
	AssistantID     string
	TraceID         string
	ErrorMessage    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type MessageRequestRepository interface {
	StartMessageRequest(ctx context.Context, userID, conversationID, clientMessageID string) (bool, error)
	GetMessageRequest(ctx context.Context, userID, conversationID, clientMessageID string) (MessageRequest, error)
	CompleteMessageRequest(ctx context.Context, userID, conversationID, clientMessageID string, assistant model.Message) error
	FailMessageRequest(ctx context.Context, userID, conversationID, clientMessageID string, cause string) error
}
