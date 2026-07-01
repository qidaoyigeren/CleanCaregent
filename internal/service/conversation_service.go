package service

import (
	"context"
	"errors"
	"fmt"
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
	requestPoll   time.Duration
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
		requestPoll:  100 * time.Millisecond,
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
	clientMessageID string,
	sink agent.EventSink,
) (askResult AskResult, askErr error) {
	content = strings.TrimSpace(content)
	if content == "" || len([]rune(content)) > 4000 {
		return AskResult{}, ErrInvalidMessage
	}
	clientMessageID = normalizeClientMessageID(clientMessageID)
	if _, err := s.repository.Get(ctx, userID, conversationID); err != nil {
		return AskResult{}, err
	}
	executionCtx := ctx
	if clientMessageID != "" {
		if existing, ok := s.findMessageByClientID(ctx, userID, conversationID, "assistant", clientMessageID); ok {
			return replayAskResult(existing, "idempotent_replay"), nil
		}
		started, err := s.startMessageRequest(ctx, userID, conversationID, clientMessageID)
		if err != nil {
			return AskResult{}, err
		}
		if !started {
			return s.waitMessageRequest(ctx, userID, conversationID, clientMessageID)
		}
		executionCtx = context.WithoutCancel(ctx)
		defer func() {
			if askErr != nil {
				s.failMessageRequest(executionCtx, userID, conversationID, clientMessageID, askErr)
				return
			}
			s.completeMessageRequest(executionCtx, userID, conversationID, clientMessageID, askResult.Message)
		}()
	}
	conversationContext := memory.ConversationContext{ConversationID: conversationID}
	memoryCtx, memorySpan := otel.Tracer("clean-care-agent/service").Start(executionCtx, "memory.load_context")
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
		ID:              id.New("msg"),
		ConversationID:  conversationID,
		Role:            "user",
		Content:         content,
		ClientMessageID: clientMessageID,
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.repository.AppendMessage(executionCtx, userID, userMessage); err != nil {
		return AskResult{}, err
	}
	s.cacheMessage(executionCtx, userMessage)

	traceID := id.New("tr")
	runCtx := executionCtx
	cancel := func() {}
	if s.timeout > 0 {
		runCtx, cancel = context.WithTimeout(executionCtx, s.timeout)
	}
	defer cancel()

	result, err := s.runner.Run(runCtx, agent.Request{
		TraceID:         traceID,
		UserID:          userID,
		ConversationID:  conversationID,
		ClientMessageID: clientMessageID,
		Query:           content,
		Context:         conversationContext,
	}, sink)
	if err != nil {
		return AskResult{}, err
	}

	assistantMessage := model.Message{
		ID:              id.New("msg"),
		ConversationID:  conversationID,
		Role:            "assistant",
		Content:         result.Answer,
		TraceID:         traceID,
		ClientMessageID: clientMessageID,
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.repository.AppendMessage(executionCtx, userID, assistantMessage); err != nil {
		return AskResult{}, err
	}
	s.cacheMessage(executionCtx, assistantMessage)
	s.scheduleSummary(
		executionCtx,
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

func (s *ConversationService) findMessageByClientID(
	ctx context.Context,
	userID string,
	conversationID string,
	role string,
	clientMessageID string,
) (model.Message, bool) {
	if clientMessageID == "" {
		return model.Message{}, false
	}
	repo, ok := s.repository.(repository.IdempotentConversationRepository)
	if !ok {
		return model.Message{}, false
	}
	message, err := repo.FindMessageByClientMessageID(ctx, userID, conversationID, role, clientMessageID)
	return message, err == nil
}

func (s *ConversationService) startMessageRequest(
	ctx context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
) (bool, error) {
	repo, ok := s.repository.(repository.MessageRequestRepository)
	if !ok {
		return true, nil
	}
	return repo.StartMessageRequest(ctx, userID, conversationID, clientMessageID)
}

func (s *ConversationService) waitMessageRequest(
	ctx context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
) (AskResult, error) {
	repo, ok := s.repository.(repository.MessageRequestRepository)
	if !ok {
		return AskResult{}, repository.ErrMessageRequestFailed
	}
	ticker := time.NewTicker(s.requestPoll)
	defer ticker.Stop()
	for {
		if existing, ok := s.findMessageByClientID(ctx, userID, conversationID, "assistant", clientMessageID); ok {
			return replayAskResult(existing, "idempotent_wait"), nil
		}
		request, err := repo.GetMessageRequest(ctx, userID, conversationID, clientMessageID)
		if err != nil {
			if existing, ok := s.findMessageByClientID(ctx, userID, conversationID, "assistant", clientMessageID); ok {
				return replayAskResult(existing, "idempotent_wait"), nil
			}
			return AskResult{}, err
		}
		switch request.Status {
		case repository.MessageRequestDone:
			existing, ok := s.findMessageByClientID(ctx, userID, conversationID, "assistant", clientMessageID)
			if ok {
				return replayAskResult(existing, "idempotent_wait"), nil
			}
			return AskResult{}, repository.ErrConversationNotFound
		case repository.MessageRequestFailed:
			if request.ErrorMessage != "" {
				return AskResult{}, fmt.Errorf("%w: %s", repository.ErrMessageRequestFailed, request.ErrorMessage)
			}
			return AskResult{}, repository.ErrMessageRequestFailed
		}
		select {
		case <-ctx.Done():
			return AskResult{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *ConversationService) completeMessageRequest(
	ctx context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
	assistant model.Message,
) {
	if clientMessageID == "" || assistant.ID == "" {
		return
	}
	repo, ok := s.repository.(repository.MessageRequestRepository)
	if !ok {
		return
	}
	_ = repo.CompleteMessageRequest(ctx, userID, conversationID, clientMessageID, assistant)
}

func (s *ConversationService) failMessageRequest(
	ctx context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
	cause error,
) {
	if clientMessageID == "" || cause == nil {
		return
	}
	repo, ok := s.repository.(repository.MessageRequestRepository)
	if !ok {
		return
	}
	_ = repo.FailMessageRequest(ctx, userID, conversationID, clientMessageID, cause.Error())
}

func replayAskResult(message model.Message, mode string) AskResult {
	return AskResult{
		Message: message,
		Result: agent.Result{
			Answer: message.Content,
			Mode:   mode,
		},
	}
}

func normalizeClientMessageID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return value[:128]
	}
	return value
}
