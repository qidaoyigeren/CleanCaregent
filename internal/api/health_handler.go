package api

import (
	"net/http"
	"time"

	"CleanCaregent/internal/config"
	"CleanCaregent/internal/health"

	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	cfg       config.Config
	startedAt time.Time
	readiness *health.Service
}

func NewHealthHandler(cfg config.Config, readiness *health.Service) *HealthHandler {
	if readiness == nil {
		readiness = health.NewService(cfg.Readiness.Timeout)
	}
	return &HealthHandler{
		cfg:       cfg,
		startedAt: time.Now().UTC(),
		readiness: readiness,
	}
}

func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"service":     h.cfg.App.Name,
		"environment": h.cfg.App.Env,
		"started_at":  h.startedAt,
	})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	report := h.readiness.Check(c.Request.Context())
	statusCode := http.StatusOK
	status := "ready"
	if !report.Ready {
		statusCode = http.StatusServiceUnavailable
		status = "not_ready"
	}
	report.Components["http"] = health.ComponentStatus{Status: "ready"}
	report.Components["agent"] = health.ComponentStatus{Status: "ready", Detail: h.cfg.Agent.Mode}
	c.JSON(statusCode, gin.H{
		"status":     status,
		"components": report.Components,
	})
}
