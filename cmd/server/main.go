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
	"sort"
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
	toolmcp "CleanCaregent/internal/tool/mcp"
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
		knowledgeRetriever = retriever.NewCached(
			knowledgeRetriever,
			sharedRedis,
			cfg.RAG.RetrievalCacheTTL,
		)
		knowledgeService = service.NewKnowledgeService(
			knowledgeRepo,
			vectorStore,
			embedder,
			rag.NewSemanticProfiledStructureAwareChunker(
				cfg.RAG.MaxChunkRunes,
				cfg.RAG.ChunkOverlap,
				chunkProfiles(cfg.RAG.ChunkProfiles),
				embedder,
			),
		)
	}

	var runner agent.Runner
	switch cfg.Agent.Mode {
	case "agentic":
		if knowledgeRetriever == nil || traceStore == nil || businessRepo == nil {
			logger.Fatal("agentic dependencies are not configured")
		}
		toolClient, mcpErr := buildToolClient(context.Background(), cfg, businessRepo, logger)
		if mcpErr != nil {
			logger.Fatal("initialize mcp tool client", zap.Error(mcpErr))
		}
		toolExecutor := tool.NewExecutor(
			toolClient,
			toolLogStore,
			cfg.Tool.Timeout,
		).WithDataScope(cfg.Tool.DataScope)
		skillRegistry := skill.NewRegistry()
		skillConfig := skill.WorkflowConfig{
			DenseTopK:     cfg.RAG.DenseTopK,
			KeywordTopK:   cfg.RAG.KeywordTopK,
			RerankTopK:    cfg.RAG.RerankTopK,
			MinDenseScore: cfg.RAG.MinDenseScore,
		}
		skillValues := []skill.Skill{
			skill.NewProductComparison(knowledgeRetriever, answerGenerator, toolExecutor, skillConfig),
			skill.NewPurchaseRecommendation(knowledgeRetriever, answerGenerator, toolExecutor, skillConfig),
			skill.NewAccessoryCompatibility(knowledgeRetriever, answerGenerator, toolExecutor, skillConfig),
			skill.NewFaultDiagnosis(knowledgeRetriever, answerGenerator, toolExecutor, skillConfig, memoryStore),
			skill.NewAfterSalesJudgement(knowledgeRetriever, answerGenerator, toolExecutor, skillConfig),
		}
		if cfg.Agent.SkillConfigPath != "" {
			skillFile, openErr := os.Open(cfg.Agent.SkillConfigPath)
			if openErr != nil {
				logger.Fatal("open skill config", zap.Error(openErr))
			}
			definitions, loadErr := skill.LoadDefinitions(skillFile)
			closeErr := skillFile.Close()
			if loadErr != nil {
				logger.Fatal("load skill config", zap.Error(loadErr))
			}
			if closeErr != nil {
				logger.Fatal("close skill config", zap.Error(closeErr))
			}
			skillValues, loadErr = skill.BuildConfigured(definitions, skill.Dependencies{
				Retriever: knowledgeRetriever, Generator: answerGenerator,
				Tools: toolExecutor, DiagnosisStore: memoryStore,
			})
			if loadErr != nil {
				logger.Fatal("build configured skills", zap.Error(loadErr))
			}
		}
		for _, value := range skillValues {
			if registerErr := skillRegistry.Register(value); registerErr != nil {
				logger.Fatal("register skill", zap.String("skill", value.Name()), zap.Error(registerErr))
			}
		}
		dynamicOptions := []orchestration.Option{
			orchestration.WithKnowledgeRetriever(knowledgeRetriever),
		}
		if cfg.Prompt.EnableLLMComponents && llmClient != nil {
			dynamicOptions = append(
				dynamicOptions,
				orchestration.WithArgumentExtractor(
					orchestration.NewLLMArgumentExtractor(llmClient),
				),
			)
		}
		dynamicExecutor := orchestration.NewDynamicExecutor(
			toolExecutor,
			skillRegistry,
			dynamicOptions...,
		)
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
			agent.WithStepEmbedder(embedder),
		)
		if cfg.Prompt.EnableLLMComponents && llmClient != nil {
			intentLLMClient := llmClient.WithModel(cfg.Prompt.IntentClassifierModel)
			availableTools, listToolsErr := toolExecutor.ListAllowed(context.Background(), []string{
				"price_query",
				"inventory_check",
				"user_purchase_history",
				"order_lookup",
				"warranty_check",
				"create_after_sales_ticket",
				"return_request",
				"exchange_request",
				"refund_status",
				"repair_status",
				"handoff_to_human",
			})
			if listToolsErr != nil {
				logger.Fatal("list mcp tools", zap.Error(listToolsErr))
			}
			agentOpts = append(agentOpts,
				agent.WithLLMRouter(intent.NewHybridRouter(intentLLMClient, promptRegistry)),
				agent.WithLLMRewriter(agent.NewLLMQueryRewriter(
					llmClient.WithModel(cfg.Prompt.QueryRewriteModel),
					promptRegistry,
				)),
				agent.WithLLMPlanner(agent.NewLLMPlanner(
					llmClient.WithModel(cfg.Prompt.PlannerModel),
					promptRegistry,
					availableTools...,
				)),
				agent.WithLLMReflector(agent.NewLLMReflector(
					llmClient.WithModel(cfg.Prompt.ReflectionModel),
					promptRegistry,
				)),
				agent.WithClarifier(agent.NewClarifier(
					llmClient.WithModel(cfg.Prompt.ClarifierModel),
					promptRegistry,
				)),
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
	runner = agent.NewGuardedRunner(runner)
	serviceOptions := make([]service.ConversationOption, 0, 1)
	if memoryStore != nil {
		serviceOptions = append(serviceOptions, service.WithMemoryStore(memoryStore, func(err error) {
			logger.Warn("cache conversation message failed", zap.Error(err))
		}))
		serviceOptions = append(
			serviceOptions,
			service.WithConversationSummarizer(
				memory.NewLLMSummarizer(
					llmForModel(llmClient, cfg.Prompt.SummarizerModel),
					promptRegistry,
				),
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
				eval.NewLLMJudgeEvaluator(
					llmClient.WithModel(cfg.Prompt.EvalJudgeModel),
					promptRegistry,
				),
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
			guardedBaselineRunner := agent.NewGuardedRunner(baselineRunner)
			baselineService := service.NewConversationService(
				conversationRepo,
				guardedBaselineRunner,
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
		CircuitManager:      llm.DefaultCircuitManager,
		PromptRegistry:      promptRegistry,
		PromptEvaluator: prompt.NewLLMVersionEvaluator(
			llmForModel(llmClient, cfg.Prompt.EvalJudgeModel),
		),
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

func buildToolClient(
	ctx context.Context,
	cfg config.Config,
	businessRepo repository.BusinessRepository,
	logger *zap.Logger,
) (tool.Client, error) {
	if len(cfg.Tool.MCP.Servers) > 0 {
		clients := make([]toolmcp.NamedClient, 0, len(cfg.Tool.MCP.Servers))
		for _, serverConfig := range cfg.Tool.MCP.Servers {
			client, err := buildSingleToolClient(ctx, cfg.Tool.MCP, serverConfig, businessRepo, logger)
			if err != nil {
				return nil, fmt.Errorf("initialize mcp server %s: %w", serverConfig.Name, err)
			}
			clients = append(clients, toolmcp.NamedClient{Name: serverConfig.Name, Client: client})
		}
		aggregate, err := toolmcp.NewAggregateClient(clients)
		if err != nil {
			return nil, err
		}
		definitions, err := aggregate.ListTools(ctx)
		if err != nil {
			return nil, err
		}
		logger.Info("aggregate mcp tool servers connected",
			zap.Int("server_count", len(clients)),
			zap.Int("tool_count", len(definitions)),
		)
		return aggregate, nil
	}
	return buildSingleToolClient(ctx, cfg.Tool.MCP, config.ToolMCPServerConfig{
		Name:           "default",
		Transport:      cfg.Tool.MCP.Transport,
		Endpoint:       cfg.Tool.MCP.Endpoint,
		APIKey:         cfg.Tool.MCP.APIKey,
		Headers:        cfg.Tool.MCP.Headers,
		StdioCommand:   cfg.Tool.MCP.StdioCommand,
		StdioArgs:      cfg.Tool.MCP.StdioArgs,
		StdioEnv:       cfg.Tool.MCP.StdioEnv,
		RequestTimeout: cfg.Tool.MCP.RequestTimeout,
		ListCacheTTL:   cfg.Tool.MCP.ListCacheTTL,
		MaxRetries:     cfg.Tool.MCP.MaxRetries,
		RetryBaseDelay: cfg.Tool.MCP.RetryBaseDelay,
		RetryMaxDelay:  cfg.Tool.MCP.RetryMaxDelay,
	}, businessRepo, logger)
}

func buildSingleToolClient(
	ctx context.Context,
	base config.ToolMCPConfig,
	serverConfig config.ToolMCPServerConfig,
	businessRepo repository.BusinessRepository,
	logger *zap.Logger,
) (tool.Client, error) {
	transport := firstNonEmpty(serverConfig.Transport, base.Transport)
	switch transport {
	case "in_process":
		tools := builtin.NewBusinessTools(businessRepo)
		toolServer, err := toolmcp.NewServer(tools...)
		if err != nil {
			return nil, err
		}
		logger.Info("mcp tools configured",
			zap.String("transport", transport),
			zap.String("server", firstNonEmpty(serverConfig.Name, "default")),
			zap.Int("tool_count", len(tools)),
		)
		return toolmcp.NewInProcessClient(toolServer), nil
	case "http":
		client, err := toolmcp.NewRemoteClient(toolmcp.RemoteClientConfig{
			Endpoint:       firstNonEmpty(serverConfig.Endpoint, base.Endpoint),
			APIKey:         firstNonEmpty(serverConfig.APIKey, base.APIKey),
			Headers:        firstHeaders(serverConfig.Headers, base.Headers),
			Timeout:        firstDuration(serverConfig.RequestTimeout, base.RequestTimeout),
			ListCacheTTL:   firstDuration(serverConfig.ListCacheTTL, base.ListCacheTTL),
			MaxRetries:     firstInt(serverConfig.MaxRetries, base.MaxRetries),
			RetryBaseDelay: firstDuration(serverConfig.RetryBaseDelay, base.RetryBaseDelay),
			RetryMaxDelay:  firstDuration(serverConfig.RetryMaxDelay, base.RetryMaxDelay),
		})
		if err != nil {
			return nil, err
		}
		definitions, err := client.ListTools(ctx)
		if err != nil {
			return nil, err
		}
		logger.Info("remote mcp tool server connected",
			zap.String("transport", transport),
			zap.String("server", firstNonEmpty(serverConfig.Name, "default")),
			zap.String("endpoint", firstNonEmpty(serverConfig.Endpoint, base.Endpoint)),
			zap.Int("tool_count", len(definitions)),
		)
		startRemoteNotificationWatcher(ctx, client, logger, firstNonEmpty(serverConfig.Name, "default"))
		return client, nil
	case "stdio":
		client, err := toolmcp.NewStdioClient(toolmcp.StdioClientConfig{
			Command:        firstNonEmpty(serverConfig.StdioCommand, base.StdioCommand),
			Args:           firstStringSlice(serverConfig.StdioArgs, base.StdioArgs),
			Env:            firstStringMap(serverConfig.StdioEnv, base.StdioEnv),
			Timeout:        firstDuration(serverConfig.RequestTimeout, base.RequestTimeout),
			MaxRestarts:    firstInt(serverConfig.MaxRetries, base.MaxRetries),
			RestartBackoff: firstDuration(serverConfig.RetryBaseDelay, base.RetryBaseDelay),
		})
		if err != nil {
			return nil, err
		}
		definitions, err := client.ListTools(ctx)
		if err != nil {
			return nil, err
		}
		logger.Info("stdio mcp tool server connected",
			zap.String("transport", transport),
			zap.String("server", firstNonEmpty(serverConfig.Name, "default")),
			zap.Int("tool_count", len(definitions)),
		)
		return client, nil
	default:
		return nil, fmt.Errorf("unsupported mcp transport %q", transport)
	}
}

func startRemoteNotificationWatcher(
	ctx context.Context,
	client *toolmcp.RemoteClient,
	logger *zap.Logger,
	serverName string,
) {
	if client == nil {
		return
	}
	go func() {
		err := client.WatchNotifications(ctx, func(notification toolmcp.Notification) {
			switch notification.Method {
			case "notifications/tools/list_changed":
				client.ClearToolDefinitions()
				logger.Info("mcp tools/list cache invalidated",
					zap.String("server", serverName),
					zap.String("notification", notification.Method),
				)
			default:
				logger.Debug("mcp notification received",
					zap.String("server", serverName),
					zap.String("notification", notification.Method),
				)
			}
		})
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.Warn("mcp notification watcher stopped",
				zap.String("server", serverName),
				zap.Error(err),
			)
		}
	}()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstHeaders(values ...map[string]string) map[string]string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstStringMap(values ...map[string]string) map[string]string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func buildGenerator(
	cfg config.Config,
	prompts *prompt.Registry,
	llmClient *llm.Client,
) generator.Generator {
	if cfg.LLM.Provider == "openai_compatible" && llmClient != nil {
		primary := generator.NewOpenAIClientFromClient(
			llmClient.WithModel(cfg.Prompt.GenerationModel),
			prompts,
		)
		return generator.NewFallback(primary, generator.NewExtractive(cfg.RAG.MaxAnswerRunes))
	}
	return generator.NewExtractive(cfg.RAG.MaxAnswerRunes)
}

func llmForModel(client *llm.Client, model string) *llm.Client {
	if client == nil {
		return nil
	}
	return client.WithModel(model)
}

// buildLLMClient creates a shared LLM client when the LLM provider is configured.
// Returns nil when the provider is extractive (no LLM available).
func buildLLMClient(cfg config.Config) *llm.Client {
	if cfg.LLM.Provider != "openai_compatible" {
		return nil
	}
	if len(cfg.LLM.Providers) > 0 {
		providers := append([]config.LLMProviderConfig(nil), cfg.LLM.Providers...)
		sort.SliceStable(providers, func(i, j int) bool {
			return providers[i].Priority < providers[j].Priority
		})
		clients := make([]*llm.Client, 0, len(providers))
		for _, provider := range providers {
			timeout := provider.RequestTimeout
			if timeout <= 0 {
				timeout = cfg.LLM.RequestTimeout
			}
			maxTokens := provider.MaxTokens
			if maxTokens <= 0 {
				maxTokens = cfg.LLM.MaxTokens
			}
			temperature := provider.Temperature
			if temperature == 0 {
				temperature = cfg.LLM.Temperature
			}
			clients = append(clients, llm.NewClient(
				provider.Endpoint,
				provider.APIKey,
				provider.Model,
				maxTokens,
				temperature,
				timeout,
			).WithCircuitBreaker(cfg.LLM.FailureThreshold, cfg.LLM.OpenTimeout).
				WithFirstTokenTimeout(min(cfg.LLM.FirstTokenTimeout, timeout)))
		}
		return clients[0].WithFallbacks(clients[1:]...)
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
			MaxRunes:          value.MaxChunkRunes,
			Overlap:           value.ChunkOverlap,
			SemanticThreshold: value.SemanticThreshold,
		}
	}
	return result
}
