package api

import (
	"errors"
	"net/http"

	"CleanCaregent/internal/trace"
	tracemysql "CleanCaregent/internal/trace/mysql"
	"CleanCaregent/pkg/response"

	"github.com/gin-gonic/gin"
)

type TraceHandler struct {
	store trace.Store
}

func NewTraceHandler(store trace.Store) *TraceHandler {
	return &TraceHandler{store: store}
}

func (h *TraceHandler) Get(c *gin.Context) {
	if h.store == nil {
		response.Error(c, http.StatusServiceUnavailable, "TRACE_UNAVAILABLE", "trace store is not configured")
		return
	}
	record, err := h.store.Get(c.Request.Context(), c.Param("trace_id"))
	if err != nil {
		if errors.Is(err, tracemysql.ErrTraceNotFound) {
			response.Error(c, http.StatusNotFound, "TRACE_NOT_FOUND", "trace not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, "TRACE_QUERY_FAILED", "trace query failed")
		return
	}
	response.OK(c, record)
}
