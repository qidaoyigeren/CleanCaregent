package api

import (
	"errors"
	"net/http"
	"strconv"

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

func (h *ConversationHandler) ListMessages(c *gin.Context) {
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "limit must be a positive integer")
			return
		}
		limit = parsed
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
	if err := h.service.CheckAccess(
		c.Request.Context(),
		middleware.UserID(c),
		c.Param("conversation_id"),
	); err != nil {
		writeServiceError(c, err)
		return
	}

	stream := NewSSEWriter(c)
	result, err := h.service.Ask(
		c.Request.Context(),
		middleware.UserID(c),
		c.Param("conversation_id"),
		req.Content,
		func(event agent.Event) error {
			return stream.Send(event.Type, event.Data)
		},
	)
	if err != nil {
		_ = stream.Send("error", errorEvent(err))
		return
	}
	_ = stream.Send("done", gin.H{
		"message_id":    result.Message.ID,
		"trace_id":      result.Message.TraceID,
		"finish_reason": "stop",
		"mode":          result.Result.Mode,
	})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidTitle), errors.Is(err, service.ErrInvalidMessage):
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, repository.ErrConversationNotFound), errors.Is(err, repository.ErrConversationForbidden):
		response.Error(c, http.StatusNotFound, "CONVERSATION_NOT_FOUND", "conversation not found")
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
	default:
		return gin.H{"code": "AGENT_ERROR", "message": "agent request failed"}
	}
}
