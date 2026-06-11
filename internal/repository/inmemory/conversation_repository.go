package inmemory

import (
	"context"
	"sort"
	"sync"

	"CleanCaregent/internal/model"
	"CleanCaregent/internal/repository"
)

type ConversationRepository struct {
	mu            sync.RWMutex
	conversations map[string]model.Conversation
	messages      map[string][]model.Message
}

func NewConversationRepository() *ConversationRepository {
	return &ConversationRepository{
		conversations: make(map[string]model.Conversation),
		messages:      make(map[string][]model.Message),
	}
}

func (r *ConversationRepository) Create(_ context.Context, conversation model.Conversation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conversations[conversation.ID] = conversation
	return nil
}

func (r *ConversationRepository) Get(_ context.Context, userID, conversationID string) (model.Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conversation, ok := r.conversations[conversationID]
	if !ok {
		return model.Conversation{}, repository.ErrConversationNotFound
	}
	if conversation.UserID != userID {
		return model.Conversation{}, repository.ErrConversationForbidden
	}
	return conversation, nil
}

func (r *ConversationRepository) AppendMessage(_ context.Context, userID string, message model.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conversation, ok := r.conversations[message.ConversationID]
	if !ok {
		return repository.ErrConversationNotFound
	}
	if conversation.UserID != userID {
		return repository.ErrConversationForbidden
	}

	r.messages[message.ConversationID] = append(r.messages[message.ConversationID], message)
	conversation.LastMessageAt = message.CreatedAt
	r.conversations[message.ConversationID] = conversation
	return nil
}

func (r *ConversationRepository) ListMessages(_ context.Context, userID, conversationID string, limit int) ([]model.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conversation, ok := r.conversations[conversationID]
	if !ok {
		return nil, repository.ErrConversationNotFound
	}
	if conversation.UserID != userID {
		return nil, repository.ErrConversationForbidden
	}

	items := append([]model.Message(nil), r.messages[conversationID]...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items, nil
}
