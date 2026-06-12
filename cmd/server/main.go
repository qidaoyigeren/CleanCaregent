package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"CleanCaregent/internal/agent"
	"CleanCaregent/internal/api"
	"CleanCaregent/internal/config"
	"CleanCaregent/internal/embedding"
	"CleanCaregent/internal/eval"
	evalmysql "CleanCaregent/internal/eval/mysql"
	"CleanCaregent/internal/generator"
	"CleanCaregent/internal/health"
	"CleanCaregent/internal/ingest"
	"CleanCaregent/internal/intent"
	"CleanCaregent/internal/llm"
	"CleanCaregent/internal/logging"
	"CleanCaregent/internal/memory"
	redismemory "CleanCaregent/internal/memory/redis"
	"CleanCaregent/internal/migrate"
	"CleanCaregent/internal/observability"
	"CleanCaregent/internal/orchestration"
	mysqlclient "CleanCaregent/internal/platform/mysql"
	redisclient "CleanCaregent/internal/platform/redis"
	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/repository/inmemory"
	mysqlrepository "CleanCaregent/internal/repository/mysql"
	"CleanCaregent/internal/reranker"
	"CleanCaregent/internal/retriever"
	"CleanCaregent/internal/service"
	"CleanCaregent/internal/skill"
	"CleanCaregent/internal/tool"
	"CleanCaregent/internal/tool/builtin"
	"CleanCaregent/internal/trace"
	tracemysql "CleanCaregent/internal/trace/mysql"
	"CleanCaregent/internal/vectorstore"
	qdrantstore "CleanCaregent/internal/vectorstore/qdrant"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger, err := logging.New(cfg.Log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Sync()
	}()
	shutdownTracing, err := observability.Init(context.Background(), cfg.Tracing)
	if err != nil {
		logger.Fatal("initialize tracing", zap.Error(err))
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := shutdownTracing(shutdownCtx); shutdownErr != nil {
			logger.Warn("shutdown tracing failed", zap.Error(shutdownErr))
		}
	}()

	var closers []io.Closer
	defer func() {
		for index := len(closers) - 1; index >= 0; index-- {
			if closeErr := closers[index].Close(); closeErr != nil {
				logger.Warn("close dependency failed", zap.Error(closeErr))
			}
		}
	}()

	var checkers []health.Checker
	var conversationRepo repository.ConversationRepository = inmemory.NewConversationRepository()
	var knowledgeRepo repository.KnowledgeRepository
	var memoryStore memory.Store
	var traceStore trace.Store
	var businessRepo repository.BusinessRepository
	var toolLogStore tool.CallLogStore
	var evalStore eval.Store
	var database *sql.DB
	var vectorStore vectorstore.Store
	var sharedRedis goredis.UniversalClient

	if cfg.MySQL.Enabled {
		db, openErr := mysqlclient.Open(context.Background(), cfg.MySQL)
		if openErr != nil {
			logger.Fatal("initialize mysql", zap.Error(openErr))
		}
		database = db
		closers = append(closers, db)
		if cfg.MySQL.AutoMigrate {
			if migrateErr := migrate.Up(context.Background(), db); migrateErr != nil {
				logger.Fatal("run database migrations", zap.Error(migrateErr))
			}
		}
		checkers = append(checkers, health.FuncChecker{
			ComponentName: "mysql",
			CheckFunc:     db.PingContext,
		})
		if cfg.Storage.ConversationRepository == "mysql" {
			conversationRepo = mysqlrepository.NewConversationRepository(db)
		}
		knowledgeRepo = mysqlrepository.NewKnowledgeRepository(db)
		traceStore = tracemysql.NewStore(db)
		businessRepository := mysqlrepository.NewBusinessRepository(db)
		businessRepo = businessRepository
		toolLogStore = businessRepository
		evalStore = evalmysql.NewStore(db)
	}

	if cfg.Redis.Enabled {
		client, openErr := redisclient.Open(context.Background(), cfg.Redis)
		if openErr != nil {
			logger.Fatal("initialize redis", zap.Error(openErr))
		}
		closers = append(closers, client)
		sharedRedis = client
		memoryStore = redismemory.NewStore(client, cfg.Redis.SessionTTL, cfg.Redis.RecentMessages)
		checkers = append(checkers, health.FuncChecker{
			ComponentName: "redis",
			CheckFunc: func(ctx context.Context) error {
				return client.Ping(ctx).Err()
			},
		})
	}

	if cfg.Qdrant.Enabled {
		client := qdrantstore.NewClient(cfg.Qdrant)
		if healthErr := client.Health(context.Background()); healthErr != nil {
			logger.Fatal("initialize qdrant", zap.Error(healthErr))
		}
		if cfg.Qdrant.EnsureCollection {
			if collectionErr := client.EnsureCollection(context.Background()); collectionErr != nil {
				logger.Fatal("ensure qdrant collection", zap.Error(collectionErr))
			}
		}
		vectorStore = client
		checkers = append(checkers, client)
	}

	embedder := buildEmbedder(cfg)
	promptRegistry := prompt.NewRegistry()
	llmClient := buildLLMClient(cfg)
	answerGenerator := buildGenerator(cfg, promptRegistry, llmClient)
	var knowledgeService *service.KnowledgeService
	var knowledgeRetriever rag.Retriever
	if database != nil && knowledgeRepo != nil && vectorStore != nil {
		knowledgeRetriever = retriever.NewHybrid(
			embedder,
			vectorStore,
			knowledgeRepo,
			buildReranker(cfg),
		)
		if businessRepo != nil {
			knowledgeRetriever = retriever.NewStructuredFirst(knowledgeRetriever, businessRepo)
		}
		knowledgeService = service.NewKnowledgeService(
			knowledgeRepo,
			vectorStore,
			embedder,
			rag.NewProfiledStructureAwareChunker(
				cfg.RAG.MaxChunkRunes,
				cfg.RAG.ChunkOverlap,
				chunkProfiles(cfg.RAG.ChunkProfiles),
			),
		)
	}

	var runner agent.Runner
	switch cfg.Agent.Mode {
	case "agentic":
		if knowledgeRetriever == nil || traceStore == nil || businessRepo == nil {
			logger.Fatal("agentic dependencies are not configured")
		}
		toolRegistry := tool.NewRegistry()
		for _, value := range []tool.Tool{
			builtin.NewPriceQuery(businessRepo),
			builtin.NewInventoryCheck(businessRepo),
			builtin.NewUserPurchaseHistory(businessRepo),
			builtin.NewOrderLookup(businessRepo),
			builtin.NewWarrantyCheck(businessRepo),
			builtin.NewCreateAfterSalesTicket(businessRepo),
		} {
			if registerErr := toolRegistry.Register(value); registerErr != nil {
				logger.Fatal("register tool", zap.String("tool", value.Name()), zap.Error(registerErr))
			}
		}
		toolExecutor := tool.NewExecutor(toolRegistry, toolLogStore, cfg.Tool.Timeout)
		skillRegistry := skill.NewRegistry()
		skillConfig := skill.WorkflowConfig{
			DenseTopK:     cfg.RAG.DenseTopK,
			KeywordTopK:   cfg.RAG.KeywordTopK,
			RerankTopK:    cfg.RAG.RerankTopK,
			MinDenseScore: cfg.RAG.MinDenseScore,
		}
		for _, value := range []skill.Skill{
			skill.NewProductComparison(knowledgeRetriever, answerGenerator, toolExecutor, skillConfig),
			skill.NewPurchaseRecommendation(knowledgeRetriever, answerGenerator, toolExecutor, skillConfig),
			skill.NewAccessoryCompatibility(knowledgeRetriever, answerGenerator, toolExecutor, skillConfig),
			skill.NewFaultDiagnosis(knowledgeRetriever, answerGenerator, toolExecutor, skillConfig, memoryStore),
			skill.NewAfterSalesJudgement(knowledgeRetriever, answerGenerator, toolExecutor, skillConfig),
		} {
			if registerErr := skillRegistry.Register(value); registerErr != nil {
				logger.Fatal("register skill", zap.String("skill", value.Name()), zap.Error(registerErr))
			}
		}
		dynamicExecutor := orchestration.NewDynamicExecutor(toolExecutor, skillRegistry)
		agentConfig := agent.AgenticConfig{
			MaxSteps:            cfg.Agent.MaxSteps,
			TokenBudget:         cfg.Agent.TokenBudget,
			PlanningMode:        cfg.Agent.PlanningMode,
			DenseTopK:           cfg.RAG.DenseTopK,
			KeywordTopK:         cfg.RAG.KeywordTopK,
			RerankTopK:          cfg.RAG.RerankTopK,
			MinDenseScore:       cfg.RAG.MinDenseScore,
			EnableLLMComponents: cfg.Prompt.EnableLLMComponents,
		}
		var agentOpts []agent.AgenticRunnerOption
		agentOpts = append(
			agentOpts,
			agent.WithPromptRegistry(promptRegistry),
			agent.WithMetricsLogger(logger),
		)
		if cfg.Prompt.EnableLLMComponents && llmClient != nil {
			intentLLMClient := llmClient.WithModel(cfg.Prompt.IntentClassifierModel)
			agentOpts = append(agentOpts,
				agent.WithLLMRouter(intent.NewHybridRouter(intentLLMClient, promptRegistry)),
				agent.WithLLMRewriter(agent.NewLLMQueryRewriter(llmClient, promptRegistry)),
				agent.WithLLMPlanner(agent.NewLLMPlanner(
					llmClient,
					promptRegistry,
					toolRegistry.ListAllowed([]string{
						"price_query",
						"inventory_check",
						"user_purchase_history",
						"order_lookup",
						"warranty_check",
						"create_after_sales_ticket",
					})...,
				)),
				agent.WithLLMReflector(agent.NewLLMReflector(llmClient, promptRegistry)),
				agent.WithClarifier(agent.NewClarifier(llmClient, promptRegistry)),
			)
			logger.Info("llm components enabled for agentic runner")
		}
		runner = agent.NewAgenticRunner(
			intent.NewRuleRouter(),
			agent.NewRuleQueryRewriter(),
			agent.NewRulePlanner(),
			knowledgeRetriever,
			answerGenerator,
			traceStore,
			dynamicExecutor,
			agentConfig,
			agentOpts...,
		)
	case "naive_rag":
		if knowledgeRetriever == nil {
			logger.Fatal("naive rag dependencies are not configured")
		}
		runner = agent.NewNaiveRAGRunner(
			knowledgeRetriever,
			answerGenerator,
			agent.NaiveRAGConfig{
				DenseTopK:     cfg.RAG.DenseTopK,
				KeywordTopK:   cfg.RAG.KeywordTopK,
				RerankTopK:    cfg.RAG.RerankTopK,
				MinDenseScore: cfg.RAG.MinDenseScore,
			},
		)
	default:
		runner = agent.NewBootstrapRunner(cfg.Agent.Mode)
	}
	serviceOptions := make([]service.ConversationOption, 0, 1)
	if memoryStore != nil {
		serviceOptions = append(serviceOptions, service.WithMemoryStore(memoryStore, func(err error) {
			logger.Warn("cache conversation message failed", zap.Error(err))
		}))
		serviceOptions = append(
			serviceOptions,
			service.WithConversationSummarizer(
				memory.NewLLMSummarizer(llmClient, promptRegistry),
				5,
			),
		)
	}
	conversationService := service.NewConversationService(conversationRepo, runner, cfg.Agent.Timeout, serviceOptions...)
	var (
		ingestPublisher ingest.Publisher
		stopIngest      context.CancelFunc
	)
	if cfg.Redis.IngestStreamEnabled && sharedRedis != nil && knowledgeService != nil {
		stream := ingest.NewRedisStream(
			sharedRedis,
			knowledgeService,
			ingest.StreamConfig{
				Stream:     cfg.Redis.IngestStream,
				Group:      cfg.Redis.IngestConsumerGroup,
				Consumer:   cfg.Redis.IngestConsumerName,
				DeadLetter: cfg.Redis.IngestDeadLetterName,
				Block:      cfg.Redis.IngestBlockTimeout,
				ClaimIdle:  cfg.Redis.IngestClaimIdle,
				BatchSize:  cfg.Redis.IngestBatchSize,
				MaxRetries: cfg.Redis.IngestMaxRetries,
			},
			logger,
		)
		if groupErr := stream.EnsureGroup(context.Background()); groupErr != nil {
			logger.Fatal("initialize knowledge ingest stream", zap.Error(groupErr))
		}
		workerCtx, cancelWorker := context.WithCancel(context.Background())
		stopIngest = cancelWorker
		ingestPublisher = stream
		go func() {
			if workerErr := stream.Run(workerCtx); workerErr != nil && workerCtx.Err() == nil {
				logger.Error("knowledge ingest worker stopped", zap.Error(workerErr))
			}
		}()
		logger.Info("knowledge ingest worker started",
			zap.String("stream", cfg.Redis.IngestStream),
			zap.String("consumer", cfg.Redis.IngestConsumerName),
		)
	}
	if stopIngest != nil {
		defer stopIngest()
	}
	var evalRunner *eval.Runner
	var evalComparison *eval.ComparisonRunner
	if evalStore != nil && traceStore != nil {
		var evaluator eval.Evaluator = eval.NewRuleEvaluator()
		if llmClient != nil {
			evaluator = eval.NewCompositeEvaluator(
				evaluator,
				eval.NewLLMJudgeEvaluator(llmClient, promptRegistry),
			)
		}
		evalRunner = eval.NewRunner(
			evalStore,
			evaluator,
			conversationService,
			traceStore,
			intent.NewRuleRouter(),
		)
		if cfg.Agent.Mode == "agentic" && knowledgeRetriever != nil {
			baselineRunner := agent.NewNaiveRAGRunner(
				knowledgeRetriever,
				answerGenerator,
				agent.NaiveRAGConfig{
					DenseTopK:     cfg.RAG.DenseTopK,
					KeywordTopK:   cfg.RAG.KeywordTopK,
					RerankTopK:    cfg.RAG.RerankTopK,
					MinDenseScore: cfg.RAG.MinDenseScore,
				},
			)
			baselineService := service.NewConversationService(
				conversationRepo,
				baselineRunner,
				cfg.Agent.Timeout,
				serviceOptions...,
			)
			baselineEvalRunner := eval.NewRunner(
				evalStore,
				evaluator,
				baselineService,
				traceStore,
				intent.NewRuleRouter(),
			)
			evalComparison = eval.NewComparisonRunner(baselineEvalRunner, evalRunner)
		}
	}
	var businessService *service.BusinessService
	if businessRepo != nil {
		businessService = service.NewBusinessService(businessRepo)
	}
	readinessService := health.NewService(cfg.Readiness.Timeout, checkers...)
	router := api.NewRouter(api.Dependencies{
		Config:              cfg,
		Logger:              logger,
		ConversationService: conversationService,
		ReadinessService:    readinessService,
		KnowledgeService:    knowledgeService,
		Retriever:           knowledgeRetriever,
		TraceStore:          traceStore,
		BusinessService:     businessService,
		RedisClient:         sharedRedis,
		EvalRunner:          evalRunner,
		EvalComparison:      evalComparison,
		EvalStore:           evalStore,
		AgentMetrics:        observability.DefaultAgentMetrics,
		IngestPublisher:     ingestPublisher,
	})

	server := &http.Server{
		Addr:              cfg.Server.Address(),
		Handler:           router,
		ReadHeaderTimeout: cfg.Server.ReadTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server started",
			zap.String("address", server.Addr),
			zap.String("environment", cfg.App.Env),
			zap.String("agent_mode", cfg.Agent.Mode),
			zap.String("conversation_repository", cfg.Storage.ConversationRepository),
		)
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case serveErr := <-errCh:
		logger.Fatal("http server failed", zap.Error(serveErr))
	case <-signalCtx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
		_ = server.Close()
	}

	logger.Info("http server stopped")
}

func buildEmbedder(cfg config.Config) embedding.Embedder {
	if cfg.Embedding.Provider == "openai_compatible" {
		var current embedding.Embedder = embedding.WithCircuitBreaker(
			embedding.NewOpenAIClient(
				cfg.Embedding.Endpoint,
				cfg.Embedding.APIKey,
				cfg.Embedding.Model,
				cfg.Embedding.Dimension,
				cfg.Embedding.BatchSize,
				cfg.Embedding.RequestTimeout,
			),
			cfg.Embedding.FailureThreshold,
			cfg.Embedding.OpenTimeout,
		)
		fallbacks := make([]embedding.Embedder, 0, len(cfg.Embedding.Fallbacks)+1)
		for _, fallback := range cfg.Embedding.Fallbacks {
			fallbacks = append(fallbacks, embedding.WithCircuitBreaker(
				embedding.NewOpenAIClient(
					fallback.Endpoint,
					fallback.APIKey,
					fallback.Model,
					fallback.Dimension,
					fallback.BatchSize,
					fallback.RequestTimeout,
				),
				cfg.Embedding.FailureThreshold,
				cfg.Embedding.OpenTimeout,
			))
		}
		if !strings.EqualFold(cfg.App.Env, "production") {
			fallbacks = append(fallbacks, embedding.NewLocalHash(cfg.Embedding.Dimension))
		}
		for _, fallback := range fallbacks {
			chain, err := embedding.NewFallback(current, fallback)
			if err != nil {
				panic(err)
			}
			current = chain
		}
		return current
	}
	return embedding.NewLocalHash(cfg.Embedding.Dimension)
}

func buildGenerator(
	cfg config.Config,
	prompts *prompt.Registry,
	llmClient *llm.Client,
) generator.Generator {
	if cfg.LLM.Provider == "openai_compatible" && llmClient != nil {
		primary := generator.NewOpenAIClientFromClient(llmClient, prompts)
		return generator.NewFallback(primary, generator.NewExtractive(cfg.RAG.MaxAnswerRunes))
	}
	return generator.NewExtractive(cfg.RAG.MaxAnswerRunes)
}

// buildLLMClient creates a shared LLM client when the LLM provider is configured.
// Returns nil when the provider is extractive (no LLM available).
func buildLLMClient(cfg config.Config) *llm.Client {
	if cfg.LLM.Provider != "openai_compatible" {
		return nil
	}
	primary := llm.NewClient(
		cfg.LLM.Endpoint,
		cfg.LLM.APIKey,
		cfg.LLM.Model,
		cfg.LLM.MaxTokens,
		cfg.LLM.Temperature,
		cfg.LLM.RequestTimeout,
	).WithCircuitBreaker(cfg.LLM.FailureThreshold, cfg.LLM.OpenTimeout).
		WithFirstTokenTimeout(cfg.LLM.FirstTokenTimeout)
	fallbacks := make([]*llm.Client, 0, len(cfg.LLM.Fallbacks))
	for _, fallback := range cfg.LLM.Fallbacks {
		fallbacks = append(fallbacks, llm.NewClient(
			fallback.Endpoint,
			fallback.APIKey,
			fallback.Model,
			fallback.MaxTokens,
			fallback.Temperature,
			fallback.RequestTimeout,
		).WithCircuitBreaker(cfg.LLM.FailureThreshold, cfg.LLM.OpenTimeout).
			WithFirstTokenTimeout(min(cfg.LLM.FirstTokenTimeout, fallback.RequestTimeout)))
	}
	return primary.WithFallbacks(fallbacks...)
}

func buildReranker(cfg config.Config) reranker.Reranker {
	local := reranker.NewLocalLexical()
	if cfg.Reranker.Provider != "openai_compatible" {
		return local
	}
	var current reranker.Reranker = reranker.WithCircuitBreaker(
		reranker.NewOpenAIClient(
			cfg.Reranker.Endpoint,
			cfg.Reranker.APIKey,
			cfg.Reranker.Model,
			cfg.Reranker.RequestTimeout,
		),
		cfg.Reranker.FailureThreshold,
		cfg.Reranker.OpenTimeout,
	)
	for _, fallback := range cfg.Reranker.Fallbacks {
		current = reranker.NewFallback(current, reranker.WithCircuitBreaker(
			reranker.NewOpenAIClient(
				fallback.Endpoint,
				fallback.APIKey,
				fallback.Model,
				fallback.RequestTimeout,
			),
			cfg.Reranker.FailureThreshold,
			cfg.Reranker.OpenTimeout,
		))
	}
	if !strings.EqualFold(cfg.App.Env, "production") {
		current = reranker.NewFallback(current, local)
	}
	return current
}

func chunkProfiles(values map[string]config.ChunkProfileConfig) map[string]rag.ChunkProfile {
	result := make(map[string]rag.ChunkProfile, len(values))
	for docType, value := range values {
		result[docType] = rag.ChunkProfile{
			MaxRunes: value.MaxChunkRunes,
			Overlap:  value.ChunkOverlap,
		}
	}
	return result
}
