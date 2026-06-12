package api

import (
	"net/http"

	"CleanCaregent/internal/config"
	"CleanCaregent/internal/eval"
	"CleanCaregent/internal/health"
	"CleanCaregent/internal/ingest"
	"CleanCaregent/internal/middleware"
	"CleanCaregent/internal/observability"
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
	v1.Use(rateLimiter.Middleware(), middleware.Auth(deps.Config.Auth))

	conversations := NewConversationHandler(deps.ConversationService)
	v1.POST("/conversations", conversations.Create)
	v1.GET("/conversations/:conversation_id/messages", conversations.ListMessages)
	v1.POST("/conversations/:conversation_id/messages", conversations.Ask)
	v1.POST("/conversations/:conversation_id/messages:stream", conversations.Stream)

	knowledge := NewKnowledgeHandler(
		deps.KnowledgeService,
		deps.Retriever,
		deps.Config.RAG.DenseTopK,
		deps.Config.RAG.KeywordTopK,
		deps.Config.RAG.RerankTopK,
		deps.Config.RAG.MinDenseScore,
		WithKnowledgeIngestPublisher(deps.IngestPublisher),
	)
	v1.POST("/admin/kb/documents", knowledge.Ingest)
	v1.POST("/admin/kb/search", knowledge.Search)

	traces := NewTraceHandler(deps.TraceStore)
	v1.GET("/admin/traces/:trace_id", traces.Get)

	business := NewBusinessHandler(deps.BusinessService)
	v1.GET("/products", business.ListProducts)
	v1.GET("/products/:product_code", business.GetProduct)
	v1.GET("/orders/:order_no", business.GetOrder)
	v1.POST("/after-sales/tickets", business.CreateAfterSales)

	evaluations := NewEvalHandler(deps.EvalRunner, deps.EvalComparison, deps.EvalStore)
	v1.POST("/admin/eval/runs", evaluations.Run)
	v1.POST("/admin/eval/comparisons", evaluations.Compare)
	v1.GET("/admin/eval/comparisons/:comparison_id", evaluations.GetComparison)
	v1.GET("/admin/eval/runs/:run_no", evaluations.Get)

	metrics := NewMetricsHandler(deps.AgentMetrics)
	v1.GET("/admin/metrics/agent", metrics.Agent)

	return router
}
