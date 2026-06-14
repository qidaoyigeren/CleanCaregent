package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"CleanCaregent/internal/agent"
	"CleanCaregent/internal/memory"
	"CleanCaregent/internal/model"
	"CleanCaregent/internal/platform/id"
	"CleanCaregent/internal/repository"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

var (
	ErrInvalidTitle   = errors.New("invalid conversation title")
	ErrInvalidMessage = errors.New("invalid message")
)

type ConversationService struct {
	repository    repository.ConversationRepository
	runner        agent.Runner
	timeout       time.Duration
	memory        memory.Store
	summarizer    memory.Summarizer
	summaryTurns  int
	summaryMu     sync.Mutex
	summarizing   map[string]bool
	onMemoryError func(error)
}

type AskResult struct {
	Message model.Message
	Result  agent.Result
}

type ConversationOption func(*ConversationService)

func WithMemoryStore(store memory.Store, onError func(error)) ConversationOption {
	return func(service *ConversationService) {
		service.memory = store
		service.onMemoryError = onError
	}
}

func WithConversationSummarizer(
	summarizer memory.Summarizer,
	recentTurns int,
) ConversationOption {
	return func(service *ConversationService) {
		service.summarizer = summarizer
		if recentTurns > 0 {
			service.summaryTurns = recentTurns
		}
	}
}

func NewConversationService(
	repository repository.ConversationRepository,
	runner agent.Runner,
	timeout time.Duration,
	options ...ConversationOption,
) *ConversationService {
	service := &ConversationService{
		repository:   repository,
		runner:       runner,
		timeout:      timeout,
		summaryTurns: 5,
		summarizing:  make(map[string]bool),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *ConversationService) Create(ctx context.Context, userID, title string) (model.Conversation, error) {
	title = strings.TrimSpace(title)
	if len([]rune(title)) > 100 {
		return model.Conversation{}, ErrInvalidTitle
	}
	if title == "" {
		title = "新会话"
	}

	now := time.Now().UTC()
	conversation := model.Conversation{
		ID:            id.New("cv"),
		UserID:        userID,
		Title:         title,
		Status:        "active",
		CreatedAt:     now,
		LastMessageAt: now,
	}
	if err := s.repository.Create(ctx, conversation); err != nil {
		return model.Conversation{}, err
	}
	return conversation, nil
}

func (s *ConversationService) List(ctx context.Context, userID string, limit int) ([]model.Conversation, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	return s.repository.List(ctx, userID, limit)
}

func (s *ConversationService) ListMessages(ctx context.Context, userID, conversationID string, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	return s.repository.ListMessages(ctx, userID, conversationID, limit)
}

func (s *ConversationService) CheckAccess(ctx context.Context, userID, conversationID string) error {
	_, err := s.repository.Get(ctx, userID, conversationID)
	return err
}

func (s *ConversationService) Ask(
	ctx context.Context,
	userID string,
	conversationID string,
	content string,
	sink agent.EventSink,
) (AskResult, error) {
	content = strings.TrimSpace(content)
	if content == "" || len([]rune(content)) > 4000 {
		return AskResult{}, ErrInvalidMessage
	}
	if _, err := s.repository.Get(ctx, userID, conversationID); err != nil {
		return AskResult{}, err
	}
	conversationContext := memory.ConversationContext{ConversationID: conversationID}
	memoryCtx, memorySpan := otel.Tracer("clean-care-agent/service").Start(ctx, "memory.load_context")
	var memoryLoadErr error
	if s.memory != nil {
		loaded, loadErr := s.memory.LoadContext(memoryCtx, conversationID, 10)
		if loadErr != nil {
			memoryLoadErr = loadErr
			if s.onMemoryError != nil {
				s.onMemoryError(loadErr)
			}
		} else if loaded != nil {
			conversationContext = *loaded
		}
	} else {
		recent, loadErr := s.repository.ListMessages(memoryCtx, userID, conversationID, 10)
		if loadErr == nil {
			conversationContext.RecentMessages = recent
		} else {
			memoryLoadErr = loadErr
		}
	}
	if memoryLoadErr != nil {
		memorySpan.RecordError(memoryLoadErr)
		memorySpan.SetStatus(codes.Error, memoryLoadErr.Error())
	}
	memorySpan.End()

	userMessage := model.Message{
		ID:             id.New("msg"),
		ConversationID: conversationID,
		Role:           "user",
		Content:        content,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.repository.AppendMessage(ctx, userID, userMessage); err != nil {
		return AskResult{}, err
	}
	s.cacheMessage(ctx, userMessage)

	traceID := id.New("tr")
	runCtx := ctx
	cancel := func() {}
	if s.timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, s.timeout)
	}
	defer cancel()

	result, err := s.runner.Run(runCtx, agent.Request{
		TraceID:        traceID,
		UserID:         userID,
		ConversationID: conversationID,
		Query:          content,
		Context:        conversationContext,
	}, sink)
	if err != nil {
		return AskResult{}, err
	}

	assistantMessage := model.Message{
		ID:             id.New("msg"),
		ConversationID: conversationID,
		Role:           "assistant",
		Content:        result.Answer,
		TraceID:        traceID,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.repository.AppendMessage(ctx, userID, assistantMessage); err != nil {
		return AskResult{}, err
	}
	s.cacheMessage(ctx, assistantMessage)
	s.scheduleSummary(
		ctx,
		userID,
		conversationID,
		conversationContext.Summary,
		conversationContext.SummaryThroughMessageID,
	)

	return AskResult{
		Message: assistantMessage,
		Result:  result,
	}, nil
}

func (s *ConversationService) scheduleSummary(
	ctx context.Context,
	userID string,
	conversationID string,
	previousSummary string,
	previousThroughMessageID string,
) {
	if s.memory == nil || s.summarizer == nil {
		return
	}
	recentMessages := max(2, s.summaryTurns*2)
	messages, err := s.repository.ListMessages(ctx, userID, conversationID, 100)
	if err != nil || len(messages) <= recentMessages {
		return
	}
	summarizeUntil := len(messages) - recentMessages
	summarizeFrom := 0
	if previousThroughMessageID != "" {
		for index, message := range messages[:summarizeUntil] {
			if message.ID == previousThroughMessageID {
				summarizeFrom = index + 1
				break
			}
		}
	}
	if summarizeFrom >= summarizeUntil {
		return
	}
	batchEnd := min(summarizeFrom+5, summarizeUntil)
	toSummarize := append([]model.Message(nil), messages[summarizeFrom:batchEnd]...)
	throughMessageID := toSummarize[len(toSummarize)-1].ID

	s.summaryMu.Lock()
	if s.summarizing[conversationID] {
		s.summaryMu.Unlock()
		return
	}
	s.summarizing[conversationID] = true
	s.summaryMu.Unlock()

	go func() {
		defer func() {
			s.summaryMu.Lock()
			delete(s.summarizing, conversationID)
			s.summaryMu.Unlock()
		}()
		summaryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		summary, summaryErr := s.summarizer.Summarize(summaryCtx, previousSummary, toSummarize)
		if summaryErr != nil {
			if s.onMemoryError != nil {
				s.onMemoryError(summaryErr)
			}
			return
		}
		if saveErr := s.memory.SaveSummary(
			summaryCtx,
			conversationID,
			summary,
			throughMessageID,
		); saveErr != nil && s.onMemoryError != nil {
			s.onMemoryError(saveErr)
		}
	}()
}

func (s *ConversationService) cacheMessage(ctx context.Context, message model.Message) {
	if s.memory == nil {
		return
	}
	if err := s.memory.AppendMessage(ctx, message); err != nil && s.onMemoryError != nil {
		s.onMemoryError(err)
	}
}
