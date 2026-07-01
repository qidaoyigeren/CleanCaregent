package inmemory

import (
	"context"
	"sort"
	"sync"
	"time"

	"CleanCaregent/internal/model"
	"CleanCaregent/internal/repository"
)

type ConversationRepository struct {
	mu            sync.RWMutex
	conversations map[string]model.Conversation
	messages      map[string][]model.Message
	requests      map[string]repository.MessageRequest
}

func NewConversationRepository() *ConversationRepository {
	return &ConversationRepository{
		conversations: make(map[string]model.Conversation),
		messages:      make(map[string][]model.Message),
		requests:      make(map[string]repository.MessageRequest),
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

func (r *ConversationRepository) List(_ context.Context, userID string, limit int) ([]model.Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]model.Conversation, 0)
	for _, conversation := range r.conversations {
		if conversation.UserID == userID {
			items = append(items, conversation)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].LastMessageAt.Equal(items[j].LastMessageAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].LastMessageAt.After(items[j].LastMessageAt)
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
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

	if message.ClientMessageID != "" {
		for _, existing := range r.messages[message.ConversationID] {
			if existing.Role == message.Role && existing.ClientMessageID == message.ClientMessageID {
				return nil
			}
		}
	}

	r.messages[message.ConversationID] = append(r.messages[message.ConversationID], message)
	conversation.LastMessageAt = message.CreatedAt
	r.conversations[message.ConversationID] = conversation
	return nil
}

func (r *ConversationRepository) FindMessageByClientMessageID(
	_ context.Context,
	userID string,
	conversationID string,
	role string,
	clientMessageID string,
) (model.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conversation, ok := r.conversations[conversationID]
	if !ok {
		return model.Message{}, repository.ErrConversationNotFound
	}
	if conversation.UserID != userID {
		return model.Message{}, repository.ErrConversationForbidden
	}
	for _, message := range r.messages[conversationID] {
		if message.Role == role && message.ClientMessageID == clientMessageID {
			return message, nil
		}
	}
	return model.Message{}, repository.ErrConversationNotFound
}

func (r *ConversationRepository) StartMessageRequest(
	_ context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	conversation, ok := r.conversations[conversationID]
	if !ok {
		return false, repository.ErrConversationNotFound
	}
	if conversation.UserID != userID {
		return false, repository.ErrConversationForbidden
	}
	key := messageRequestKey(conversationID, clientMessageID)
	if _, exists := r.requests[key]; exists {
		return false, nil
	}
	now := time.Now().UTC()
	r.requests[key] = repository.MessageRequest{
		ConversationID:  conversationID,
		ClientMessageID: clientMessageID,
		Status:          repository.MessageRequestRunning,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	return true, nil
}

func (r *ConversationRepository) GetMessageRequest(
	_ context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
) (repository.MessageRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conversation, ok := r.conversations[conversationID]
	if !ok {
		return repository.MessageRequest{}, repository.ErrConversationNotFound
	}
	if conversation.UserID != userID {
		return repository.MessageRequest{}, repository.ErrConversationForbidden
	}
	request, ok := r.requests[messageRequestKey(conversationID, clientMessageID)]
	if !ok {
		return repository.MessageRequest{}, repository.ErrConversationNotFound
	}
	return request, nil
}

func (r *ConversationRepository) CompleteMessageRequest(
	_ context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
	assistant model.Message,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conversation, ok := r.conversations[conversationID]
	if !ok {
		return repository.ErrConversationNotFound
	}
	if conversation.UserID != userID {
		return repository.ErrConversationForbidden
	}
	key := messageRequestKey(conversationID, clientMessageID)
	request := r.requests[key]
	request.ConversationID = conversationID
	request.ClientMessageID = clientMessageID
	request.Status = repository.MessageRequestDone
	request.AssistantID = assistant.ID
	request.TraceID = assistant.TraceID
	request.ErrorMessage = ""
	request.UpdatedAt = time.Now().UTC()
	if request.CreatedAt.IsZero() {
		request.CreatedAt = request.UpdatedAt
	}
	r.requests[key] = request
	return nil
}

func (r *ConversationRepository) FailMessageRequest(
	_ context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
	cause string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conversation, ok := r.conversations[conversationID]
	if !ok {
		return repository.ErrConversationNotFound
	}
	if conversation.UserID != userID {
		return repository.ErrConversationForbidden
	}
	key := messageRequestKey(conversationID, clientMessageID)
	request := r.requests[key]
	request.ConversationID = conversationID
	request.ClientMessageID = clientMessageID
	request.Status = repository.MessageRequestFailed
	request.ErrorMessage = cause
	request.UpdatedAt = time.Now().UTC()
	if request.CreatedAt.IsZero() {
		request.CreatedAt = request.UpdatedAt
	}
	r.requests[key] = request
	return nil
}

func messageRequestKey(conversationID, clientMessageID string) string {
	return conversationID + "\x00" + clientMessageID
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
