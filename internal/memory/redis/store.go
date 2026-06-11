package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"CleanCaregent/internal/memory"
	"CleanCaregent/internal/model"

	goredis "github.com/redis/go-redis/v9"
)

type Store struct {
	client         *goredis.Client
	ttl            time.Duration
	recentMessages int
	prefix         string
}

func NewStore(client *goredis.Client, ttl time.Duration, recentMessages int) *Store {
	return &Store{
		client:         client,
		ttl:            ttl,
		recentMessages: recentMessages,
		prefix:         "cleancare:conversation:",
	}
}

func (s *Store) LoadContext(ctx context.Context, conversationID string, recentLimit int) (*memory.ConversationContext, error) {
	if recentLimit <= 0 || recentLimit > s.recentMessages {
		recentLimit = s.recentMessages
	}

	pipeline := s.client.Pipeline()
	messagesCmd := pipeline.LRange(ctx, s.messagesKey(conversationID), int64(-recentLimit), -1)
	summaryCmd := pipeline.HGet(ctx, s.summaryKey(conversationID), "content")
	summaryThroughCmd := pipeline.HGet(ctx, s.summaryKey(conversationID), "through_message_id")
	entitiesCmd := pipeline.HGetAll(ctx, s.entitiesKey(conversationID))
	diagnosisCmd := pipeline.Get(ctx, s.diagnosisKey(conversationID))
	if _, err := pipeline.Exec(ctx); err != nil && !errors.Is(err, goredis.Nil) {
		return nil, fmt.Errorf("load conversation context: %w", err)
	}

	recentMessages := make([]model.Message, 0, recentLimit)
	for _, raw := range messagesCmd.Val() {
		var message model.Message
		if err := json.Unmarshal([]byte(raw), &message); err != nil {
			return nil, fmt.Errorf("decode cached message: %w", err)
		}
		recentMessages = append(recentMessages, message)
	}

	result := &memory.ConversationContext{
		ConversationID:          conversationID,
		Summary:                 summaryCmd.Val(),
		SummaryThroughMessageID: summaryThroughCmd.Val(),
		RecentMessages:          recentMessages,
		KnownEntities:           entitiesCmd.Val(),
	}
	if raw := diagnosisCmd.Val(); raw != "" {
		var state memory.DiagnosisState
		if err := json.Unmarshal([]byte(raw), &state); err != nil {
			return nil, fmt.Errorf("decode diagnosis state: %w", err)
		}
		result.DiagnosisState = &state
	}
	return result, nil
}

func (s *Store) AppendMessage(ctx context.Context, message model.Message) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode cached message: %w", err)
	}

	key := s.messagesKey(message.ConversationID)
	pipeline := s.client.TxPipeline()
	pipeline.RPush(ctx, key, raw)
	pipeline.LTrim(ctx, key, int64(-s.recentMessages), -1)
	pipeline.Expire(ctx, key, s.ttl)
	if _, err := pipeline.Exec(ctx); err != nil {
		return fmt.Errorf("append cached message: %w", err)
	}
	return nil
}

func (s *Store) SaveSummary(
	ctx context.Context,
	conversationID string,
	summary string,
	throughMessageID string,
) error {
	key := s.summaryKey(conversationID)
	pipeline := s.client.TxPipeline()
	pipeline.HSet(ctx, key, map[string]any{
		"content":            summary,
		"through_message_id": throughMessageID,
		"updated_at":         time.Now().UTC().Format(time.RFC3339Nano),
	})
	pipeline.Expire(ctx, key, s.ttl)
	if _, err := pipeline.Exec(ctx); err != nil {
		return fmt.Errorf("save conversation summary: %w", err)
	}
	return nil
}

func (s *Store) SetEntity(
	ctx context.Context,
	conversationID string,
	key string,
	value string,
	ttl time.Duration,
) error {
	if ttl <= 0 {
		ttl = s.ttl
	}
	redisKey := s.entitiesKey(conversationID)
	pipeline := s.client.TxPipeline()
	pipeline.HSet(ctx, redisKey, key, value)
	pipeline.Expire(ctx, redisKey, ttl)
	if _, err := pipeline.Exec(ctx); err != nil {
		return fmt.Errorf("save conversation entity: %w", err)
	}
	return nil
}

func (s *Store) LoadDiagnosisState(ctx context.Context, conversationID string) (*memory.DiagnosisState, error) {
	raw, err := s.client.Get(ctx, s.diagnosisKey(conversationID)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load diagnosis state: %w", err)
	}
	var state memory.DiagnosisState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("decode diagnosis state: %w", err)
	}
	return &state, nil
}

func (s *Store) SaveDiagnosisState(ctx context.Context, state memory.DiagnosisState) error {
	state.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode diagnosis state: %w", err)
	}
	if err := s.client.Set(ctx, s.diagnosisKey(state.ConversationID), raw, s.ttl).Err(); err != nil {
		return fmt.Errorf("save diagnosis state: %w", err)
	}
	return nil
}

func (s *Store) messagesKey(conversationID string) string {
	return s.prefix + conversationID + ":messages"
}

func (s *Store) summaryKey(conversationID string) string {
	return s.prefix + conversationID + ":summary"
}

func (s *Store) entitiesKey(conversationID string) string {
	return s.prefix + conversationID + ":entities"
}

func (s *Store) diagnosisKey(conversationID string) string {
	return s.prefix + conversationID + ":diagnosis"
}
