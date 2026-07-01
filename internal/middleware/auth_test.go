package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"CleanCaregent/internal/config"

	"github.com/gin-gonic/gin"
)

func TestJWTAuthAcceptsValidToken(t *testing.T) {
	cfg := testAuthConfig()
	token := signTestJWT(t, cfg, jwtClaims{
		Subject: "user-1001", Issuer: cfg.JWTIssuer, ExpiresAt: time.Now().Add(time.Minute).Unix(),
	})
	recorder := serveAuthRequest(JWTAuth(cfg), map[string]string{"Authorization": "Bearer " + token})
	if recorder.Code != http.StatusOK || recorder.Body.String() != "user-1001" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestJWTAuthRejectsExpiredToken(t *testing.T) {
	cfg := testAuthConfig()
	token := signTestJWT(t, cfg, jwtClaims{
		Subject: "user-1001", Issuer: cfg.JWTIssuer, ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	})
	recorder := serveAuthRequest(JWTAuth(cfg), map[string]string{"Authorization": "Bearer " + token})
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestAdminAuthUsesSeparateAPIKey(t *testing.T) {
	cfg := testAuthConfig()
	recorder := serveAuthRequest(AdminAuth(cfg), map[string]string{"X-Admin-API-Key": cfg.AdminAPIKey})
	if recorder.Code != http.StatusOK || recorder.Body.String() != "admin" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	rejected := serveAuthRequest(AdminAuth(cfg), map[string]string{"X-Admin-API-Key": "wrong"})
	if rejected.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%q", rejected.Code, rejected.Body.String())
	}
}

func TestAdminAuthAcceptsAdminJWTAndRejectsUserJWT(t *testing.T) {
	cfg := testAuthConfig()
	adminToken := signTestJWT(t, cfg, jwtClaims{
		Subject: "admin-1001", Issuer: cfg.JWTIssuer, ExpiresAt: time.Now().Add(time.Minute).Unix(),
		Roles: []string{"admin"},
	})
	recorder := serveAuthRequest(AdminAuth(cfg), map[string]string{"Authorization": "Bearer " + adminToken})
	if recorder.Code != http.StatusOK || recorder.Body.String() != "admin-1001" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}

	userToken := signTestJWT(t, cfg, jwtClaims{
		Subject: "user-1001", Issuer: cfg.JWTIssuer, ExpiresAt: time.Now().Add(time.Minute).Unix(),
		Roles: []string{"support"},
	})
	rejected := serveAuthRequest(AdminAuth(config.AuthConfig{
		Enabled:   true,
		JWTSecret: cfg.JWTSecret,
		JWTIssuer: cfg.JWTIssuer,
		JWTLeeway: cfg.JWTLeeway,
		AdminRole: cfg.AdminRole,
	}), map[string]string{"Authorization": "Bearer " + userToken})
	if rejected.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%q", rejected.Code, rejected.Body.String())
	}
}

func testAuthConfig() config.AuthConfig {
	return config.AuthConfig{
		Enabled:     true,
		JWTSecret:   "test-secret-must-be-at-least-32-bytes",
		JWTIssuer:   "clean-care-agent",
		JWTLeeway:   time.Second,
		AdminRole:   "admin",
		AdminAPIKey: "test-admin-key",
	}
}

func signTestJWT(t *testing.T, cfg config.AuthConfig, claims jwtClaims) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(cfg.JWTSecret))
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func serveAuthRequest(handler gin.HandlerFunc, headers map[string]string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(handler)
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, UserID(c))
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
