package middleware

import (
	"net/http"

	"CleanCaregent/internal/config"
	"CleanCaregent/pkg/response"

	"github.com/gin-gonic/gin"
)

const userIDKey = "user_id"

func Auth(cfg config.AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Set(userIDKey, cfg.DevelopmentUserID)
			c.Next()
			return
		}

		response.Error(c, http.StatusServiceUnavailable, "AUTH_NOT_CONFIGURED", "authentication provider is not configured")
	}
}

func UserID(c *gin.Context) string {
	value, _ := c.Get(userIDKey)
	userID, _ := value.(string)
	return userID
}
