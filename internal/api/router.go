package api

import (
	"net/http"

	"CleanCaregent/internal/config"
	"CleanCaregent/internal/eval"
	"CleanCaregent/internal/health"
	"CleanCaregent/internal/ingest"
	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/middleware"
	"CleanCaregent/internal/observability"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/service"
	"CleanCaregent/internal/trace"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Dependencies struct {
	Config              config.Config
	Logger              *zap.Logger
	ConversationService *service.ConversationService
	ReadinessService    *health.Service
	KnowledgeService    *service.KnowledgeService
	Retriever           rag.Retriever
	TraceStore          trace.Store
	BusinessService     *service.BusinessService
	RedisClient         goredis.UniversalClient
	EvalRunner          *eval.Runner
	EvalComparison      *eval.ComparisonRunner
	EvalStore           eval.Store
	AgentMetrics        *observability.AgentMetrics
	IngestPublisher     ingest.Publisher
	CircuitManager      *llm.CircuitManager
	PromptRegistry      *prompt.Registry
	PromptEvaluator     prompt.VersionEvaluator
}

func NewRouter(deps Dependencies) http.Handler {
	if deps.Config.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.OpenTelemetry(deps.Config.Tracing.ServiceName),
		middleware.Recovery(deps.Logger),
		middleware.AccessLog(deps.Logger),
	)

	healthHandler := NewHealthHandler(deps.Config, deps.ReadinessService)
	router.GET("/health/live", healthHandler.Live)
	router.GET("/health/ready", healthHandler.Ready)

	rateLimiter := middleware.NewRateLimiter(deps.Config.RateLimit)
	if deps.Config.RateLimit.Backend == "redis" && deps.RedisClient != nil {
		rateLimiter = middleware.NewRedisRateLimiter(deps.Config.RateLimit, deps.RedisClient)
	}
	v1 := router.Group("/api/v1")
	v1.Use(rateLimiter.Middleware())

	conversations := NewConversationHandler(deps.ConversationService)
	userAPI := v1.Group("")
	userAPI.Use(middleware.JWTAuth(deps.Config.Auth))
	userAPI.POST("/conversations", conversations.Create)
	userAPI.GET("/conversations/:conversation_id/messages", conversations.ListMessages)
	userAPI.POST("/conversations/:conversation_id/messages", conversations.Ask)
	userAPI.POST("/conversations/:conversation_id/messages:stream", conversations.Stream)

	knowledge := NewKnowledgeHandler(
		deps.KnowledgeService,
		deps.Retriever,
		deps.Config.RAG.DenseTopK,
		deps.Config.RAG.KeywordTopK,
		deps.Config.RAG.RerankTopK,
		deps.Config.RAG.MinDenseScore,
		WithKnowledgeIngestPublisher(deps.IngestPublisher),
	)
	adminAPI := v1.Group("/admin")
	adminAPI.Use(middleware.AdminAuth(deps.Config.Auth))
	adminAPI.POST("/kb/documents", knowledge.Ingest)
	adminAPI.POST("/kb/upload", knowledge.Upload)
	adminAPI.POST("/kb/search", knowledge.Search)

	traces := NewTraceHandler(deps.TraceStore)
	adminAPI.GET("/traces/:trace_id", traces.Get)

	business := NewBusinessHandler(deps.BusinessService)
	userAPI.GET("/products", business.ListProducts)
	userAPI.GET("/products/:product_code", business.GetProduct)
	userAPI.GET("/orders/:order_no", business.GetOrder)
	userAPI.POST("/after-sales/tickets", business.CreateAfterSales)

	evaluations := NewEvalHandler(deps.EvalRunner, deps.EvalComparison, deps.EvalStore)
	adminAPI.POST("/eval/runs", evaluations.Run)
	adminAPI.POST("/eval/comparisons", evaluations.Compare)
	adminAPI.GET("/eval/comparisons/:comparison_id", evaluations.GetComparison)
	adminAPI.GET("/eval/runs/:run_no", evaluations.Get)

	metrics := NewMetricsHandler(deps.AgentMetrics)
	adminAPI.GET("/metrics/agent", metrics.Agent)
	adminAPI.GET("/metrics/prometheus", metrics.Prometheus)

	circuits := NewCircuitHandler(deps.CircuitManager)
	adminAPI.GET("/circuit-breakers/status", circuits.Status)
	adminAPI.POST("/circuit-breakers/reset", circuits.Reset)

	prompts := NewPromptHandler(deps.PromptRegistry, deps.PromptEvaluator)
	adminAPI.GET("/prompts", prompts.List)
	adminAPI.POST("/prompts/:scenario/activate", prompts.Activate)
	adminAPI.POST("/prompts/eval", prompts.Compare)

	return router
}
