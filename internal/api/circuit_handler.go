package api

import (
	"net/http"

	"CleanCaregent/internal/llm"
	"CleanCaregent/pkg/response"
	"github.com/gin-gonic/gin"
)

type CircuitHandler struct {
	manager *llm.CircuitManager
}

// NewCircuitHandler creates the circuit-breaker administration handler.
func NewCircuitHandler(manager *llm.CircuitManager) *CircuitHandler {
	return &CircuitHandler{manager: manager}
}

// Status returns all registered circuit-breaker states.
func (h *CircuitHandler) Status(c *gin.Context) {
	if h.manager == nil {
		response.Error(c, http.StatusServiceUnavailable, "CIRCUIT_MANAGER_UNAVAILABLE", "熔断管理器未配置")
		return
	}
	response.OK(c, gin.H{"items": h.manager.Status()})
}

// Reset closes all registered circuit breakers.
func (h *CircuitHandler) Reset(c *gin.Context) {
	if h.manager == nil {
		response.Error(c, http.StatusServiceUnavailable, "CIRCUIT_MANAGER_UNAVAILABLE", "熔断管理器未配置")
		return
	}
	response.OK(c, gin.H{"reset_count": h.manager.ResetAll()})
}
