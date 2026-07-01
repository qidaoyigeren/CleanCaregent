package api

import (
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"CleanCaregent/internal/agent"
	"CleanCaregent/internal/middleware"
	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/service"
	"CleanCaregent/pkg/response"

	"github.com/gin-gonic/gin"
)

type ConversationHandler struct {
	service *service.ConversationService
}

const streamHeartbeatInterval = 10 * time.Second

type createConversationRequest struct {
	Title string `json:"title"`
}

type askRequest struct {
	Content         string `json:"content" binding:"required"`
	ClientMessageID string `json:"client_message_id"`
}

func NewConversationHandler(service *service.ConversationService) *ConversationHandler {
	return &ConversationHandler{service: service}
}

func (h *ConversationHandler) Create(c *gin.Context) {
	var req createConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body")
		return
	}

	conversation, err := h.service.Create(c.Request.Context(), middleware.UserID(c), req.Title)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Created(c, conversation)
}

func (h *ConversationHandler) List(c *gin.Context) {
	limit, ok := parsePositiveLimit(c, 20)
	if !ok {
		return
	}
	conversations, err := h.service.List(
		c.Request.Context(),
		middleware.UserID(c),
		limit,
	)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"items": conversations})
}

func (h *ConversationHandler) ListMessages(c *gin.Context) {
	limit, ok := parsePositiveLimit(c, 20)
	if !ok {
		return
	}

	messages, err := h.service.ListMessages(
		c.Request.Context(),
		middleware.UserID(c),
		c.Param("conversation_id"),
		limit,
	)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"items": messages})
}

func parsePositiveLimit(c *gin.Context, defaultValue int) (int, bool) {
	raw := c.Query("limit")
	if raw == "" {
		return defaultValue, true
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "limit must be a positive integer")
		return 0, false
	}
	return parsed, true
}

func (h *ConversationHandler) Ask(c *gin.Context) {
	var req askRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "content is required")
		return
	}

	result, err := h.service.Ask(
		c.Request.Context(),
		middleware.UserID(c),
		c.Param("conversation_id"),
		req.Content,
		req.ClientMessageID,
		nil,
	)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{
		"message_id": result.Message.ID,
		"answer":     result.Result.Answer,
		"evidences":  result.Result.Evidences,
		"trace_id":   result.Message.TraceID,
		"mode":       result.Result.Mode,
	})
}

func (h *ConversationHandler) Stream(c *gin.Context) {
	var req askRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "content is required")
		return
	}
	requestCtx := c.Request.Context()
	userID := middleware.UserID(c)
	conversationID := c.Param("conversation_id")
	if err := h.service.CheckAccess(
		requestCtx,
		userID,
		conversationID,
	); err != nil {
		writeServiceError(c, err)
		return
	}

	stream := newSafeSSEStream(NewSSEWriter(c))
	resultCh := make(chan streamAskResult, 1)
	go func() {
		result, err := h.service.Ask(
			requestCtx,
			userID,
			conversationID,
			req.Content,
			req.ClientMessageID,
			stream.Sink,
		)
		resultCh <- streamAskResult{result: result, err: err}
	}()

	heartbeat := time.NewTicker(streamHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case outcome := <-resultCh:
			if outcome.err != nil {
				stream.Send("error", errorEvent(outcome.err))
				return
			}
			if isIdempotentReplayMode(outcome.result.Result.Mode) && outcome.result.Result.Answer != "" {
				stream.Send("delta", gin.H{"content": outcome.result.Result.Answer})
			}
			stream.Send("done", gin.H{
				"message_id":    outcome.result.Message.ID,
				"trace_id":      outcome.result.Message.TraceID,
				"finish_reason": "stop",
				"mode":          outcome.result.Result.Mode,
			})
			return
		case <-heartbeat.C:
			if !stream.Send("heartbeat", gin.H{"ts": time.Now().UTC().Format(time.RFC3339Nano)}) {
				stream.Close()
				return
			}
		case <-requestCtx.Done():
			stream.Close()
			return
		}
	}
}

type streamAskResult struct {
	result service.AskResult
	err    error
}

type safeSSEStream struct {
	writer *SSEWriter
	mu     sync.Mutex
	closed bool
}

func newSafeSSEStream(writer *SSEWriter) *safeSSEStream {
	return &safeSSEStream{writer: writer}
}

func (s *safeSSEStream) Sink(event agent.Event) error {
	s.Send(event.Type, event.Data)
	return nil
}

func (s *safeSSEStream) Send(event string, data any) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	if err := s.writer.Send(event, data); err != nil {
		s.closed = true
		return false
	}
	return true
}

func (s *safeSSEStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidTitle), errors.Is(err, service.ErrInvalidMessage):
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, repository.ErrConversationNotFound), errors.Is(err, repository.ErrConversationForbidden):
		response.Error(c, http.StatusNotFound, "CONVERSATION_NOT_FOUND", "conversation not found")
	case errors.Is(err, repository.ErrMessageRequestFailed):
		response.Error(c, http.StatusConflict, "MESSAGE_REQUEST_FAILED", "message request failed")
	case errors.Is(err, agent.ErrNotConfigured):
		response.Error(c, http.StatusServiceUnavailable, "AGENT_NOT_CONFIGURED", "agent pipeline is not configured")
	default:
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func errorEvent(err error) gin.H {
	switch {
	case errors.Is(err, agent.ErrNotConfigured):
		return gin.H{"code": "AGENT_NOT_CONFIGURED", "message": "agent pipeline is not configured"}
	case errors.Is(err, repository.ErrConversationNotFound), errors.Is(err, repository.ErrConversationForbidden):
		return gin.H{"code": "CONVERSATION_NOT_FOUND", "message": "conversation not found"}
	case errors.Is(err, repository.ErrMessageRequestFailed):
		return gin.H{"code": "MESSAGE_REQUEST_FAILED", "message": "message request failed"}
	default:
		return gin.H{"code": "AGENT_ERROR", "message": "agent request failed"}
	}
}

func isIdempotentReplayMode(mode string) bool {
	return mode == "idempotent_replay" || mode == "idempotent_wait"
}
