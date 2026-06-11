package inmemory

import (
	"context"
	"errors"
	"testing"
	"time"

	"CleanCaregent/internal/model"
	"CleanCaregent/internal/repository"
)

func TestConversationRepositoryEnforcesOwnership(t *testing.T) {
	repo := NewConversationRepository()
	conversation := model.Conversation{
		ID:        "cv_test",
		UserID:    "user_a",
		CreatedAt: time.Now(),
	}
	if err := repo.Create(context.Background(), conversation); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := repo.Get(context.Background(), "user_b", conversation.ID); !errors.Is(err, repository.ErrConversationForbidden) {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestConversationRepositoryListsLatestMessages(t *testing.T) {
	repo := NewConversationRepository()
	ctx := context.Background()
	conversation := model.Conversation{ID: "cv_test", UserID: "user_a", CreatedAt: time.Now()}
	if err := repo.Create(ctx, conversation); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	for i := 0; i < 3; i++ {
		message := model.Message{
			ID:             string(rune('a' + i)),
			ConversationID: conversation.ID,
			Role:           "user",
			Content:        "message",
			CreatedAt:      time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := repo.AppendMessage(ctx, "user_a", message); err != nil {
			t.Fatalf("AppendMessage() error = %v", err)
		}
	}

	messages, err := repo.ListMessages(ctx, "user_a", conversation.ID, 2)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(messages) != 2 || messages[0].ID != "b" || messages[1].ID != "c" {
		t.Fatalf("messages = %#v", messages)
	}
}
