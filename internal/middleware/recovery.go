package middleware

import (
	"net/http"

	"CleanCaregent/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.Error("panic recovered",
			zap.Any("panic", recovered),
			zap.String("request_id", response.RequestID(c)),
			zap.Stack("stack"),
		)
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	})
}
