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

		userID, err := validateJWT(bearerToken(c.GetHeader("Authorization")), cfg, time.Now().UTC())
		if err != nil {
			response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "user credentials are invalid")
			return
		}
		c.Set(userIDKey, userID)
		c.Next()
	}
}

// AdminAuth authenticates administrator APIs with an admin JWT role/scope.
// X-Admin-API-Key is kept only as a non-production compatibility fallback.
func AdminAuth(cfg config.AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Set(userIDKey, cfg.DevelopmentUserID)
			c.Next()
			return
		}

		if claims, err := validateJWTClaims(bearerToken(c.GetHeader("Authorization")), cfg, time.Now().UTC()); err == nil &&
			claimsHasAdminAccess(claims, cfg.AdminRole) {
			c.Set(userIDKey, claims.Subject)
			c.Next()
			return
		}

		provided := []byte(strings.TrimSpace(c.GetHeader("X-Admin-API-Key")))
		expected := []byte(strings.TrimSpace(cfg.AdminAPIKey))
		if len(expected) > 0 && len(provided) == len(expected) &&
			subtle.ConstantTimeCompare(provided, expected) == 1 {
			c.Set(userIDKey, "admin")
			c.Next()
			return
		}

		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "admin credentials are invalid")
	}
}

type jwtClaims struct {
	Subject   string   `json:"sub"`
	Issuer    string   `json:"iss"`
	ExpiresAt int64    `json:"exp"`
	NotBefore int64    `json:"nbf,omitempty"`
	Scope     string   `json:"scope,omitempty"`
	Roles     []string `json:"roles,omitempty"`
}

func validateJWT(token string, cfg config.AuthConfig, now time.Time) (string, error) {
	claims, err := validateJWTClaims(token, cfg, now)
	if err != nil {
		return "", err
	}
	return claims.Subject, nil
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return strings.TrimSpace(header[len("Bearer "):])
	}
	return header
}

func claimsHasAdminAccess(claims jwtClaims, adminRole string) bool {
	adminRole = strings.TrimSpace(adminRole)
	if adminRole == "" {
		adminRole = "admin"
	}
	for _, role := range claims.Roles {
		if strings.EqualFold(strings.TrimSpace(role), adminRole) {
			return true
		}
	}
	for _, scope := range strings.Fields(claims.Scope) {
		if strings.EqualFold(scope, adminRole) || strings.EqualFold(scope, "role:"+adminRole) {
			return true
		}
	}
	return false
}

func validateJWTClaims(token string, cfg config.AuthConfig, now time.Time) (jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtClaims{}, errInvalidToken
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return jwtClaims{}, errInvalidToken
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil || header.Algorithm != "HS256" {
		return jwtClaims{}, errInvalidToken
	}
	mac := hmac.New(sha256.New, []byte(cfg.JWTSecret))
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	expectedSignature := mac.Sum(nil)
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, expectedSignature) {
		return jwtClaims{}, errInvalidToken
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtClaims{}, errInvalidToken
	}
	var claims jwtClaims
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		return jwtClaims{}, errInvalidToken
	}
	leeway := int64(cfg.JWTLeeway.Seconds())
	nowUnix := now.Unix()
	if strings.TrimSpace(claims.Subject) == "" ||
		claims.ExpiresAt == 0 ||
		nowUnix > claims.ExpiresAt+leeway ||
		claims.NotBefore > 0 && nowUnix+leeway < claims.NotBefore ||
		cfg.JWTIssuer != "" && claims.Issuer != cfg.JWTIssuer {
		return jwtClaims{}, errInvalidToken
	}
	return claims, nil
}

var errInvalidToken = &authenticationError{}

type authenticationError struct{}

func (*authenticationError) Error() string { return "invalid authentication token" }

// UserID returns the authenticated tenant user identifier.
func UserID(c *gin.Context) string {
	value, _ := c.Get(userIDKey)
	userID, _ := value.(string)
	return userID
}
