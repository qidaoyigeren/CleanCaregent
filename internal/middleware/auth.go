package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"CleanCaregent/internal/config"
	"CleanCaregent/pkg/response"

	"github.com/gin-gonic/gin"
)

const userIDKey = "user_id"

// Auth is kept as the user API authentication entry point for compatibility.
// New code should use JWTAuth to make the authentication scheme explicit.
func Auth(cfg config.AuthConfig) gin.HandlerFunc {
	return JWTAuth(cfg)
}

// JWTAuth authenticates user APIs with an HS256 bearer token.
func JWTAuth(cfg config.AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Set(userIDKey, cfg.DevelopmentUserID)
			c.Next()
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
		userID, err := validateJWT(token, cfg, time.Now().UTC())
		if err != nil {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "用户令牌无效或已过期")
			return
		}
		c.Set(userIDKey, userID)
		c.Next()
	}
}

// AdminAuth authenticates administrator APIs with X-Admin-API-Key.
func AdminAuth(cfg config.AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Set(userIDKey, cfg.DevelopmentUserID)
			c.Next()
			return
		}
		provided := []byte(strings.TrimSpace(c.GetHeader("X-Admin-API-Key")))
		expected := []byte(strings.TrimSpace(cfg.AdminAPIKey))
		if len(provided) == 0 || len(provided) != len(expected) ||
			subtle.ConstantTimeCompare(provided, expected) != 1 {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "管理员密钥无效")
			return
		}
		c.Set(userIDKey, "admin")
		c.Next()
	}
}

type jwtClaims struct {
	Subject   string `json:"sub"`
	Issuer    string `json:"iss"`
	ExpiresAt int64  `json:"exp"`
	NotBefore int64  `json:"nbf,omitempty"`
}

func validateJWT(token string, cfg config.AuthConfig, now time.Time) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errInvalidToken
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", errInvalidToken
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil || header.Algorithm != "HS256" {
		return "", errInvalidToken
	}
	mac := hmac.New(sha256.New, []byte(cfg.JWTSecret))
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	expectedSignature := mac.Sum(nil)
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, expectedSignature) {
		return "", errInvalidToken
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errInvalidToken
	}
	var claims jwtClaims
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		return "", errInvalidToken
	}
	leeway := int64(cfg.JWTLeeway.Seconds())
	nowUnix := now.Unix()
	if strings.TrimSpace(claims.Subject) == "" ||
		claims.ExpiresAt == 0 ||
		nowUnix > claims.ExpiresAt+leeway ||
		claims.NotBefore > 0 && nowUnix+leeway < claims.NotBefore ||
		cfg.JWTIssuer != "" && claims.Issuer != cfg.JWTIssuer {
		return "", errInvalidToken
	}
	return claims.Subject, nil
}

var errInvalidToken = &authenticationError{}

type authenticationError struct{}

func (*authenticationError) Error() string { return "鉴权令牌无效" }

// UserID returns the authenticated tenant user identifier.
func UserID(c *gin.Context) string {
	value, _ := c.Get(userIDKey)
	userID, _ := value.(string)
	return userID
}
