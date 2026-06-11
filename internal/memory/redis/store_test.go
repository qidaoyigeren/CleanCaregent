package redis

import (
	"context"
	"testing"
	"time"

	"CleanCaregent/internal/memory"
	"CleanCaregent/internal/model"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestStoreMaintainsRecentContext(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	defer client.Close()

	store := NewStore(client, time.Hour, 2)
	ctx := context.Background()
	for _, id := range []string{"m1", "m2", "m3"} {
		if err := store.AppendMessage(ctx, model.Message{
			ID:             id,
			ConversationID: "cv_test",
			Role:           "user",
			Content:        id,
			CreatedAt:      time.Now().UTC(),
		}); err != nil {
			t.Fatalf("AppendMessage() error = %v", err)
		}
	}
	if err := store.SaveSummary(ctx, "cv_test", "用户正在比较两款产品", "m1"); err != nil {
		t.Fatalf("SaveSummary() error = %v", err)
	}
	if err := store.SetEntity(ctx, "cv_test", "product_model", "T20", time.Hour); err != nil {
		t.Fatalf("SetEntity() error = %v", err)
	}
	if err := store.SaveDiagnosisState(ctx, memory.DiagnosisState{
		ConversationID: "cv_test",
		ProductModel:   "T20",
		FaultNodeID:    "cannot_charge.root",
		Answers:        map[string]string{"dock_light": "off"},
	}); err != nil {
		t.Fatalf("SaveDiagnosisState() error = %v", err)
	}

	conversationContext, err := store.LoadContext(ctx, "cv_test", 5)
	if err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	if len(conversationContext.RecentMessages) != 2 {
		t.Fatalf("RecentMessages length = %d", len(conversationContext.RecentMessages))
	}
	if conversationContext.RecentMessages[0].ID != "m2" || conversationContext.RecentMessages[1].ID != "m3" {
		t.Fatalf("RecentMessages = %#v", conversationContext.RecentMessages)
	}
	if conversationContext.Summary == "" || conversationContext.KnownEntities["product_model"] != "T20" {
		t.Fatalf("context = %#v", conversationContext)
	}
	if conversationContext.DiagnosisState == nil || conversationContext.DiagnosisState.FaultNodeID != "cannot_charge.root" {
		t.Fatalf("DiagnosisState = %#v", conversationContext.DiagnosisState)
	}
}
