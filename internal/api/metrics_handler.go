package api

import (
	"net/http"

	"CleanCaregent/internal/observability"
	"CleanCaregent/pkg/response"

	"github.com/gin-gonic/gin"
)

type MetricsHandler struct {
	agent *observability.AgentMetrics
}

func NewMetricsHandler(agent *observability.AgentMetrics) *MetricsHandler {
	return &MetricsHandler{agent: agent}
}

func (h *MetricsHandler) Agent(c *gin.Context) {
	if h.agent == nil {
		response.Error(c, http.StatusServiceUnavailable, "METRICS_UNAVAILABLE", "agent metrics are not configured")
		return
	}
	response.OK(c, h.agent.Snapshot())
}

// Prometheus exposes the in-process metrics in Prometheus text format.
func (h *MetricsHandler) Prometheus(c *gin.Context) {
	c.Data(
		http.StatusOK,
		"text/plain; version=0.0.4; charset=utf-8",
		[]byte(observability.DefaultPrometheusMetrics.Render()),
	)
}
