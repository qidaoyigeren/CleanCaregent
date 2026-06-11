package repository

import (
	"context"
	"errors"

	"CleanCaregent/internal/model"
)

var (
	ErrConversationNotFound  = errors.New("conversation not found")
	ErrConversationForbidden = errors.New("conversation forbidden")
)

type ConversationRepository interface {
	Create(ctx context.Context, conversation model.Conversation) error
	Get(ctx context.Context, userID, conversationID string) (model.Conversation, error)
	AppendMessage(ctx context.Context, userID string, message model.Message) error
	ListMessages(ctx context.Context, userID, conversationID string, limit int) ([]model.Message, error)
}
