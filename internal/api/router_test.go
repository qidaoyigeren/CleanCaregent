package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"CleanCaregent/internal/agent"
	"CleanCaregent/internal/config"
	"CleanCaregent/internal/repository/inmemory"
	"CleanCaregent/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestHealthAndConversationFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestRouter()

	healthResponse := httptest.NewRecorder()
	router.ServeHTTP(healthResponse, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", healthResponse.Code, healthResponse.Body.String())
	}

	createResponse := httptest.NewRecorder()
	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/conversations",
		bytes.NewBufferString(`{"title":"扫地机器人选购"}`),
	)
	createRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}

	var created struct {
		Data struct {
			ConversationID string `json:"conversation_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Data.ConversationID == "" {
		t.Fatal("conversation_id is empty")
	}

	askResponse := httptest.NewRecorder()
	askRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/conversations/"+created.Data.ConversationID+"/messages",
		bytes.NewBufferString(`{"content":"T20 吸力多大？"}`),
	)
	askRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(askResponse, askRequest)
	if askResponse.Code != http.StatusOK {
		t.Fatalf("ask status = %d, body = %s", askResponse.Code, askResponse.Body.String())
	}
	if !strings.Contains(askResponse.Body.String(), `"mode":"bootstrap"`) {
		t.Fatalf("ask body = %s", askResponse.Body.String())
	}
}

func TestConversationSSEProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestRouter()
	conversationID := createConversation(t, router)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/conversations/"+conversationID+"/messages:stream",
		bytes.NewBufferString(`{"content":"帮我比较 T20 和 X20 Pro"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("Content-Type = %q", contentType)
	}
	body := response.Body.String()
	for _, expected := range []string{"event: status", "event: delta", "event: done", `"mode":"bootstrap"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("SSE body missing %q: %s", expected, body)
		}
	}
}

func TestStreamValidatesConversationBeforeStarting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestRouter()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/conversations/missing/messages:stream",
		bytes.NewBufferString(`{"content":"test"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream started before validation")
	}
}

func newTestRouter() http.Handler {
	cfg := config.Config{
		App: config.AppConfig{Name: "clean-care-agent", Env: "test"},
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            8080,
			ReadTimeout:     time.Second,
			WriteTimeout:    time.Second,
			IdleTimeout:     time.Second,
			ShutdownTimeout: time.Second,
		},
		Log: config.LogConfig{Level: "info"},
		Auth: config.AuthConfig{
			Enabled:           false,
			DevelopmentUserID: "test-user",
		},
		RateLimit: config.RateLimitConfig{Enabled: false},
		Agent: config.AgentConfig{
			Mode:     "bootstrap",
			MaxSteps: 5,
			Timeout:  time.Second,
		},
	}
	repo := inmemory.NewConversationRepository()
	runner := agent.NewBootstrapRunner("bootstrap")
	conversationService := service.NewConversationService(repo, runner, cfg.Agent.Timeout)
	return NewRouter(Dependencies{
		Config:              cfg,
		Logger:              zap.NewNop(),
		ConversationService: conversationService,
	})
}

func createConversation(t *testing.T, router http.Handler) string {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/conversations",
		bytes.NewBufferString(`{"title":"test"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	var created struct {
		Data struct {
			ConversationID string `json:"conversation_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return created.Data.ConversationID
}
