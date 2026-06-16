package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App       AppConfig       `mapstructure:"app"`
	Server    ServerConfig    `mapstructure:"server"`
	Log       LogConfig       `mapstructure:"log"`
	Auth      AuthConfig      `mapstructure:"auth"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Agent     AgentConfig     `mapstructure:"agent"`
	Storage   StorageConfig   `mapstructure:"storage"`
	MySQL     MySQLConfig     `mapstructure:"mysql"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Qdrant    QdrantConfig    `mapstructure:"qdrant"`
	Readiness ReadinessConfig `mapstructure:"readiness"`
	Embedding EmbeddingConfig `mapstructure:"embedding"`
	RAG       RAGConfig       `mapstructure:"rag"`
	Reranker  RerankerConfig  `mapstructure:"reranker"`
	LLM       LLMConfig       `mapstructure:"llm"`
	Tool      ToolConfig      `mapstructure:"tool"`
	Tracing   TracingConfig   `mapstructure:"tracing"`
	Prompt    PromptConfig    `mapstructure:"prompt"`
}

type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

func (c ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type LogConfig struct {
	Level       string `mapstructure:"level"`
	Development bool   `mapstructure:"development"`
}

type AuthConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	DevelopmentUserID string        `mapstructure:"development_user_id"`
	JWTSecret         string        `mapstructure:"jwt_secret"`
	JWTIssuer         string        `mapstructure:"jwt_issuer"`
	JWTLeeway         time.Duration `mapstructure:"jwt_leeway"`
	AdminAPIKey       string        `mapstructure:"admin_api_key"`
}

type RateLimitConfig struct {
	Enabled           bool    `mapstructure:"enabled"`
	Backend           string  `mapstructure:"backend"`
	RequestsPerSecond float64 `mapstructure:"requests_per_second"`
	Burst             int     `mapstructure:"burst"`
}

type AgentConfig struct {
	Mode            string        `mapstructure:"mode"`
	PlanningMode    string        `mapstructure:"planning_mode"`
	SkillConfigPath string        `mapstructure:"skill_config_path"`
	MaxSteps        int           `mapstructure:"max_steps"`
	TokenBudget     int           `mapstructure:"token_budget"`
	Timeout         time.Duration `mapstructure:"timeout"`
}

type StorageConfig struct {
	ConversationRepository string `mapstructure:"conversation_repository"`
}

type MySQLConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	PingTimeout     time.Duration `mapstructure:"ping_timeout"`
	AutoMigrate     bool          `mapstructure:"auto_migrate"`
}

type RedisConfig struct {
	Enabled              bool          `mapstructure:"enabled"`
	Address              string        `mapstructure:"address"`
	Password             string        `mapstructure:"password"`
	DB                   int           `mapstructure:"db"`
	DialTimeout          time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout          time.Duration `mapstructure:"read_timeout"`
	WriteTimeout         time.Duration `mapstructure:"write_timeout"`
	SessionTTL           time.Duration `mapstructure:"session_ttl"`
	RecentMessages       int           `mapstructure:"recent_messages"`
	IngestStreamEnabled  bool          `mapstructure:"ingest_stream_enabled"`
	IngestStream         string        `mapstructure:"ingest_stream"`
	IngestConsumerGroup  string        `mapstructure:"ingest_consumer_group"`
	IngestConsumerName   string        `mapstructure:"ingest_consumer_name"`
	IngestBlockTimeout   time.Duration `mapstructure:"ingest_block_timeout"`
	IngestClaimIdle      time.Duration `mapstructure:"ingest_claim_idle"`
	IngestBatchSize      int64         `mapstructure:"ingest_batch_size"`
	IngestMaxRetries     int           `mapstructure:"ingest_max_retries"`
	IngestDeadLetterName string        `mapstructure:"ingest_dead_letter_name"`
}

type QdrantConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	BaseURL          string        `mapstructure:"base_url"`
	APIKey           string        `mapstructure:"api_key"`
	Collection       string        `mapstructure:"collection"`
	VectorSize       int           `mapstructure:"vector_size"`
	Distance         string        `mapstructure:"distance"`
	RequestTimeout   time.Duration `mapstructure:"request_timeout"`
	EnsureCollection bool          `mapstructure:"ensure_collection"`
}

type ReadinessConfig struct {
	Timeout time.Duration `mapstructure:"timeout"`
}

type EmbeddingConfig struct {
	Provider         string                    `mapstructure:"provider"`
	Endpoint         string                    `mapstructure:"endpoint"`
	APIKey           string                    `mapstructure:"api_key"`
	Model            string                    `mapstructure:"model"`
	Dimension        int                       `mapstructure:"dimension"`
	RequestTimeout   time.Duration             `mapstructure:"request_timeout"`
	BatchSize        int                       `mapstructure:"batch_size"`
	Fallbacks        []EmbeddingFallbackConfig `mapstructure:"fallbacks"`
	FailureThreshold int                       `mapstructure:"failure_threshold"`
	OpenTimeout      time.Duration             `mapstructure:"open_timeout"`
}

type EmbeddingFallbackConfig struct {
	Endpoint       string        `mapstructure:"endpoint"`
	APIKey         string        `mapstructure:"api_key"`
	Model          string        `mapstructure:"model"`
	Dimension      int           `mapstructure:"dimension"`
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
	BatchSize      int           `mapstructure:"batch_size"`
}

type RAGConfig struct {
	DenseTopK         int                           `mapstructure:"dense_top_k"`
	KeywordTopK       int                           `mapstructure:"keyword_top_k"`
	RerankTopK        int                           `mapstructure:"rerank_top_k"`
	MinDenseScore     float64                       `mapstructure:"min_dense_score"`
	MaxChunkRunes     int                           `mapstructure:"max_chunk_runes"`
	ChunkOverlap      int                           `mapstructure:"chunk_overlap"`
	ChunkProfiles     map[string]ChunkProfileConfig `mapstructure:"chunk_profiles"`
	MaxAnswerRunes    int                           `mapstructure:"max_answer_runes"`
	RetrievalCacheTTL time.Duration                 `mapstructure:"retrieval_cache_ttl"`
}

type ChunkProfileConfig struct {
	MaxChunkRunes     int     `mapstructure:"max_chunk_runes"`
	ChunkOverlap      int     `mapstructure:"chunk_overlap"`
	SemanticThreshold float64 `mapstructure:"semantic_threshold"`
}

type RerankerConfig struct {
	Provider         string                   `mapstructure:"provider"`
	Endpoint         string                   `mapstructure:"endpoint"`
	APIKey           string                   `mapstructure:"api_key"`
	Model            string                   `mapstructure:"model"`
	RequestTimeout   time.Duration            `mapstructure:"request_timeout"`
	Fallbacks        []RerankerFallbackConfig `mapstructure:"fallbacks"`
	FailureThreshold int                      `mapstructure:"failure_threshold"`
	OpenTimeout      time.Duration            `mapstructure:"open_timeout"`
}

type RerankerFallbackConfig struct {
	Endpoint       string        `mapstructure:"endpoint"`
	APIKey         string        `mapstructure:"api_key"`
	Model          string        `mapstructure:"model"`
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
}

type LLMConfig struct {
	Provider          string              `mapstructure:"provider"`
	Endpoint          string              `mapstructure:"endpoint"`
	APIKey            string              `mapstructure:"api_key"`
	Model             string              `mapstructure:"model"`
	RequestTimeout    time.Duration       `mapstructure:"request_timeout"`
	FirstTokenTimeout time.Duration       `mapstructure:"first_token_timeout"`
	MaxTokens         int                 `mapstructure:"max_tokens"`
	Temperature       float64             `mapstructure:"temperature"`
	FailureThreshold  int                 `mapstructure:"failure_threshold"`
	OpenTimeout       time.Duration       `mapstructure:"open_timeout"`
	Fallbacks         []LLMFallbackConfig `mapstructure:"fallbacks"`
	Providers         []LLMProviderConfig `mapstructure:"providers"`
}

type LLMProviderConfig struct {
	Name           string        `mapstructure:"name"`
	Endpoint       string        `mapstructure:"endpoint"`
	APIKey         string        `mapstructure:"api_key"`
	Model          string        `mapstructure:"model"`
	Priority       int           `mapstructure:"priority"`
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
	MaxTokens      int           `mapstructure:"max_tokens"`
	Temperature    float64       `mapstructure:"temperature"`
}

type LLMFallbackConfig struct {
	Endpoint       string        `mapstructure:"endpoint"`
	APIKey         string        `mapstructure:"api_key"`
	Model          string        `mapstructure:"model"`
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
	MaxTokens      int           `mapstructure:"max_tokens"`
	Temperature    float64       `mapstructure:"temperature"`
}

type ToolConfig struct {
	Timeout   time.Duration `mapstructure:"timeout"`
	DataScope string        `mapstructure:"data_scope"`
	MCP       ToolMCPConfig `mapstructure:"mcp"`
}

type ToolMCPConfig struct {
	Transport      string            `mapstructure:"transport"`
	Endpoint       string            `mapstructure:"endpoint"`
	APIKey         string            `mapstructure:"api_key"`
	Headers        map[string]string `mapstructure:"headers"`
	RequestTimeout time.Duration     `mapstructure:"request_timeout"`
	ListCacheTTL   time.Duration     `mapstructure:"list_cache_ttl"`
	MaxRetries     int               `mapstructure:"max_retries"`
	RetryBaseDelay time.Duration     `mapstructure:"retry_base_delay"`
	RetryMaxDelay  time.Duration     `mapstructure:"retry_max_delay"`
	ListenHost     string            `mapstructure:"listen_host"`
	ListenPort     int               `mapstructure:"listen_port"`
	Path           string            `mapstructure:"path"`
	ServerAPIKey   string            `mapstructure:"server_api_key"`
	AllowedOrigins []string          `mapstructure:"allowed_origins"`
}

func (c ToolMCPConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.ListenHost, c.ListenPort)
}

type TracingConfig struct {
	Enabled        bool    `mapstructure:"enabled"`
	ServiceName    string  `mapstructure:"service_name"`
	ServiceVersion string  `mapstructure:"service_version"`
	OTLPEndpoint   string  `mapstructure:"otlp_endpoint"`
	Insecure       bool    `mapstructure:"insecure"`
	SampleRatio    float64 `mapstructure:"sample_ratio"`
}

// PromptConfig controls prompt template versioning and LLM enrichment behaviour.
type PromptConfig struct {
	// EnableLLMComponents toggles LLM-powered intent, rewrite, plan, and reflect.
	// When false, the system uses pure rule-based components (backward compatible).
	EnableLLMComponents bool `mapstructure:"enable_llm_components"`
	// IntentClassifierModel overrides the main LLM model for intent classification
	// (empty = use llm.model).
	IntentClassifierModel string `mapstructure:"intent_classifier_model"`
	QueryRewriteModel     string `mapstructure:"query_rewrite_model"`
	PlannerModel          string `mapstructure:"planner_model"`
	ReflectionModel       string `mapstructure:"reflection_model"`
	ClarifierModel        string `mapstructure:"clarifier_model"`
	GenerationModel       string `mapstructure:"generation_model"`
	SummarizerModel       string `mapstructure:"summarizer_model"`
	EvalJudgeModel        string `mapstructure:"eval_judge_model"`
}

func Load(path string) (Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetEnvPrefix("CLEANCARE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config %q: %w", path, err)
		}
	} else {
		v.SetConfigName("config.local")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		if err := v.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if !errors.As(err, &notFound) {
				return Config{}, fmt.Errorf("read local config: %w", err)
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	inheritSameProviderCredentials(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func inheritSameProviderCredentials(cfg *Config) {
	if cfg == nil ||
		cfg.Reranker.Provider != "openai_compatible" ||
		strings.TrimSpace(cfg.Reranker.APIKey) != "" ||
		cfg.Embedding.Provider != "openai_compatible" ||
		strings.TrimSpace(cfg.Embedding.APIKey) == "" ||
		!sameEndpointHost(cfg.Reranker.Endpoint, cfg.Embedding.Endpoint) {
		return
	}
	cfg.Reranker.APIKey = cfg.Embedding.APIKey
}

func sameEndpointHost(left, right string) bool {
	leftURL, leftErr := url.Parse(strings.TrimSpace(left))
	rightURL, rightErr := url.Parse(strings.TrimSpace(right))
	if leftErr != nil || rightErr != nil {
		return false
	}
	leftHost := strings.ToLower(leftURL.Hostname())
	rightHost := strings.ToLower(rightURL.Hostname())
	return leftHost != "" && leftHost == rightHost
}

func (c Config) Validate() error {
	if c.App.Name == "" {
		return errors.New("app.name is required")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return errors.New("server.port must be between 1 and 65535")
	}
	if c.Server.ReadTimeout <= 0 || c.Server.WriteTimeout <= 0 || c.Server.ShutdownTimeout <= 0 {
		return errors.New("server timeouts must be positive")
	}
	if c.RateLimit.Enabled && (c.RateLimit.RequestsPerSecond <= 0 || c.RateLimit.Burst <= 0) {
		return errors.New("rate_limit values must be positive when enabled")
	}
	if c.RateLimit.Backend != "local" && c.RateLimit.Backend != "redis" {
		return errors.New("rate_limit.backend must be local or redis")
	}
	if c.RateLimit.Backend == "redis" && !c.Redis.Enabled {
		return errors.New("redis must be enabled when rate_limit.backend=redis")
	}
	if c.Auth.Enabled {
		if len(strings.TrimSpace(c.Auth.JWTSecret)) < 32 {
			return errors.New("auth.jwt_secret must contain at least 32 characters when auth is enabled")
		}
		if strings.TrimSpace(c.Auth.AdminAPIKey) == "" {
			return errors.New("auth.admin_api_key is required when auth is enabled")
		}
		if c.Auth.JWTLeeway < 0 {
			return errors.New("auth.jwt_leeway must not be negative")
		}
	}
	if c.Agent.MaxSteps < 1 || c.Agent.MaxSteps > 5 {
		return errors.New("agent.max_steps must be between 1 and 5")
	}
	switch c.Agent.PlanningMode {
	case "react", "plan_execute", "auto":
	default:
		return errors.New("agent.planning_mode must be react, plan_execute, or auto")
	}
	if c.Agent.Timeout <= 0 {
		return errors.New("agent.timeout must be positive")
	}
	switch c.Agent.Mode {
	case "bootstrap":
	case "naive_rag", "agentic":
		if !c.MySQL.Enabled || !c.Qdrant.Enabled {
			return errors.New("mysql and qdrant must be enabled for rag agent modes")
		}
	default:
		return errors.New("agent.mode must be bootstrap, naive_rag, or agentic")
	}
	if c.Agent.TokenBudget < 500 {
		return errors.New("agent.token_budget must be at least 500")
	}
	switch c.Storage.ConversationRepository {
	case "memory":
	case "mysql":
		if !c.MySQL.Enabled {
			return errors.New("mysql must be enabled when storage.conversation_repository=mysql")
		}
	default:
		return errors.New("storage.conversation_repository must be memory or mysql")
	}
	if c.MySQL.Enabled {
		if strings.TrimSpace(c.MySQL.DSN) == "" {
			return errors.New("mysql.dsn is required when mysql is enabled")
		}
		if c.MySQL.MaxOpenConns < 1 || c.MySQL.MaxIdleConns < 0 || c.MySQL.MaxIdleConns > c.MySQL.MaxOpenConns {
			return errors.New("mysql connection pool settings are invalid")
		}
		if c.MySQL.PingTimeout <= 0 {
			return errors.New("mysql.ping_timeout must be positive")
		}
		if c.MySQL.AutoMigrate && !strings.Contains(strings.ToLower(c.MySQL.DSN), "multistatements=true") {
			return errors.New("mysql.dsn must include multiStatements=true when auto_migrate is enabled")
		}
	}
	if c.Redis.Enabled {
		if strings.TrimSpace(c.Redis.Address) == "" {
			return errors.New("redis.address is required when redis is enabled")
		}
		if c.Redis.SessionTTL <= 0 || c.Redis.RecentMessages < 2 {
			return errors.New("redis session settings are invalid")
		}
	}
	if c.Redis.IngestStreamEnabled {
		if !c.Redis.Enabled {
			return errors.New("redis must be enabled when redis.ingest_stream_enabled=true")
		}
		if strings.TrimSpace(c.Redis.IngestStream) == "" ||
			strings.TrimSpace(c.Redis.IngestConsumerGroup) == "" ||
			strings.TrimSpace(c.Redis.IngestConsumerName) == "" ||
			strings.TrimSpace(c.Redis.IngestDeadLetterName) == "" {
			return errors.New("redis ingest stream names are required")
		}
		if c.Redis.IngestBlockTimeout <= 0 ||
			c.Redis.IngestClaimIdle <= 0 ||
			c.Redis.IngestBatchSize < 1 ||
			c.Redis.IngestMaxRetries < 1 {
			return errors.New("redis ingest stream settings are invalid")
		}
	}
	if c.Qdrant.Enabled {
		if strings.TrimSpace(c.Qdrant.BaseURL) == "" || strings.TrimSpace(c.Qdrant.Collection) == "" {
			return errors.New("qdrant base_url and collection are required when qdrant is enabled")
		}
		if c.Qdrant.VectorSize < 1 || c.Qdrant.RequestTimeout <= 0 {
			return errors.New("qdrant vector_size and request_timeout must be positive")
		}
		switch strings.ToLower(c.Qdrant.Distance) {
		case "cosine", "dot", "euclid", "manhattan":
		default:
			return errors.New("qdrant.distance must be cosine, dot, euclid, or manhattan")
		}
	}
	if c.Readiness.Timeout <= 0 {
		return errors.New("readiness.timeout must be positive")
	}
	switch c.Embedding.Provider {
	case "local_hash":
		if strings.EqualFold(strings.TrimSpace(c.App.Env), "production") {
			return errors.New("embedding.provider local_hash is not allowed in production")
		}
	case "openai_compatible":
		if strings.TrimSpace(c.Embedding.Endpoint) == "" || strings.TrimSpace(c.Embedding.Model) == "" {
			return errors.New("embedding endpoint and model are required for openai_compatible provider")
		}
	default:
		return errors.New("embedding.provider must be local_hash or openai_compatible")
	}
	if c.Embedding.Dimension < 1 || c.Embedding.BatchSize < 1 || c.Embedding.RequestTimeout <= 0 {
		return errors.New("embedding dimension, batch_size and request_timeout must be positive")
	}
	if c.Embedding.FailureThreshold < 1 || c.Embedding.OpenTimeout <= 0 {
		return errors.New("embedding circuit breaker settings are invalid")
	}
	for index, fallback := range c.Embedding.Fallbacks {
		if strings.TrimSpace(fallback.Endpoint) == "" || strings.TrimSpace(fallback.Model) == "" {
			return fmt.Errorf("embedding.fallbacks[%d] endpoint and model are required", index)
		}
		if fallback.Dimension < 1 || fallback.BatchSize < 1 || fallback.RequestTimeout <= 0 {
			return fmt.Errorf("embedding.fallbacks[%d] settings are invalid", index)
		}
		if fallback.Dimension != c.Embedding.Dimension {
			return fmt.Errorf("embedding.fallbacks[%d] dimension must equal embedding.dimension", index)
		}
	}
	if c.Qdrant.Enabled && c.Embedding.Dimension != c.Qdrant.VectorSize {
		return errors.New("embedding.dimension must equal qdrant.vector_size")
	}
	if c.RAG.DenseTopK < 1 || c.RAG.KeywordTopK < 1 || c.RAG.RerankTopK < 1 {
		return errors.New("rag top_k values must be positive")
	}
	if c.RAG.RerankTopK > c.RAG.DenseTopK+c.RAG.KeywordTopK {
		return errors.New("rag.rerank_top_k is larger than all retrieval candidates")
	}
	if c.RAG.MaxChunkRunes < 100 || c.RAG.ChunkOverlap < 0 || c.RAG.ChunkOverlap >= c.RAG.MaxChunkRunes {
		return errors.New("rag chunk size settings are invalid")
	}
	for docType, profile := range c.RAG.ChunkProfiles {
		if !supportedChunkProfile(docType) {
			return fmt.Errorf("rag.chunk_profiles contains unsupported document type %q", docType)
		}
		if profile.MaxChunkRunes < 100 ||
			profile.ChunkOverlap < 0 ||
			profile.ChunkOverlap >= profile.MaxChunkRunes ||
			profile.SemanticThreshold < 0 ||
			profile.SemanticThreshold >= 1 {
			return fmt.Errorf("rag.chunk_profiles.%s settings are invalid", docType)
		}
	}
	switch c.Reranker.Provider {
	case "local_lexical":
	case "openai_compatible":
		if strings.TrimSpace(c.Reranker.Endpoint) == "" || strings.TrimSpace(c.Reranker.Model) == "" {
			return errors.New("reranker endpoint and model are required for openai_compatible provider")
		}
	default:
		return errors.New("reranker.provider must be local_lexical or openai_compatible")
	}
	if c.Reranker.RequestTimeout <= 0 {
		return errors.New("reranker.request_timeout must be positive")
	}
	if c.Reranker.FailureThreshold < 1 || c.Reranker.OpenTimeout <= 0 {
		return errors.New("reranker circuit breaker settings are invalid")
	}
	for index, fallback := range c.Reranker.Fallbacks {
		if strings.TrimSpace(fallback.Endpoint) == "" || strings.TrimSpace(fallback.Model) == "" {
			return fmt.Errorf("reranker.fallbacks[%d] endpoint and model are required", index)
		}
		if fallback.RequestTimeout <= 0 {
			return fmt.Errorf("reranker.fallbacks[%d] request_timeout must be positive", index)
		}
	}
	switch c.LLM.Provider {
	case "extractive":
	case "openai_compatible":
		if len(c.LLM.Providers) == 0 &&
			(strings.TrimSpace(c.LLM.Endpoint) == "" || strings.TrimSpace(c.LLM.Model) == "") {
			return errors.New("llm endpoint and model are required for openai_compatible provider")
		}
	default:
		return errors.New("llm.provider must be extractive or openai_compatible")
	}
	if c.LLM.RequestTimeout <= 0 || c.LLM.MaxTokens < 1 || c.LLM.Temperature < 0 || c.LLM.Temperature > 2 {
		return errors.New("llm settings are invalid")
	}
	if c.LLM.FirstTokenTimeout <= 0 || c.LLM.FirstTokenTimeout > c.LLM.RequestTimeout {
		return errors.New("llm.first_token_timeout must be positive and no greater than request_timeout")
	}
	if c.LLM.FailureThreshold < 1 || c.LLM.OpenTimeout <= 0 {
		return errors.New("llm circuit breaker settings are invalid")
	}
	for index, fallback := range c.LLM.Fallbacks {
		if strings.TrimSpace(fallback.Endpoint) == "" || strings.TrimSpace(fallback.Model) == "" {
			return fmt.Errorf("llm.fallbacks[%d] endpoint and model are required", index)
		}
		if fallback.RequestTimeout <= 0 || fallback.MaxTokens < 1 ||
			fallback.Temperature < 0 || fallback.Temperature > 2 {
			return fmt.Errorf("llm.fallbacks[%d] settings are invalid", index)
		}
	}
	seenPriorities := make(map[int]struct{}, len(c.LLM.Providers))
	for index, provider := range c.LLM.Providers {
		if strings.TrimSpace(provider.Name) == "" ||
			strings.TrimSpace(provider.Endpoint) == "" ||
			strings.TrimSpace(provider.Model) == "" {
			return fmt.Errorf("llm.providers[%d] name, endpoint and model are required", index)
		}
		if provider.Priority < 1 {
			return fmt.Errorf("llm.providers[%d] priority must be positive", index)
		}
		if _, exists := seenPriorities[provider.Priority]; exists {
			return fmt.Errorf("llm.providers[%d] priority must be unique", index)
		}
		seenPriorities[provider.Priority] = struct{}{}
	}
	if c.Tool.Timeout <= 0 {
		return errors.New("tool.timeout must be positive")
	}
	switch c.Tool.DataScope {
	case "mock", "sandbox", "external":
	default:
		return errors.New("tool.data_scope must be mock, sandbox, or external")
	}
	switch c.Tool.MCP.Transport {
	case "in_process":
	case "http":
		if strings.TrimSpace(c.Tool.MCP.Endpoint) == "" {
			return errors.New("tool.mcp.endpoint is required when tool.mcp.transport=http")
		}
		parsed, err := url.Parse(strings.TrimSpace(c.Tool.MCP.Endpoint))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("tool.mcp.endpoint must be an absolute URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("tool.mcp.endpoint scheme must be http or https")
		}
	default:
		return errors.New("tool.mcp.transport must be in_process or http")
	}
	if c.Tool.MCP.RequestTimeout <= 0 ||
		c.Tool.MCP.ListCacheTTL < 0 ||
		c.Tool.MCP.MaxRetries < 0 ||
		c.Tool.MCP.RetryBaseDelay <= 0 ||
		c.Tool.MCP.RetryMaxDelay <= 0 ||
		c.Tool.MCP.RetryBaseDelay > c.Tool.MCP.RetryMaxDelay {
		return errors.New("tool.mcp retry and timeout settings are invalid")
	}
	if c.Tool.MCP.ListenPort < 1 || c.Tool.MCP.ListenPort > 65535 {
		return errors.New("tool.mcp.listen_port must be between 1 and 65535")
	}
	if strings.TrimSpace(c.Tool.MCP.ListenHost) == "" {
		return errors.New("tool.mcp.listen_host is required")
	}
	if !strings.HasPrefix(strings.TrimSpace(c.Tool.MCP.Path), "/") {
		return errors.New("tool.mcp.path must start with /")
	}
	if strings.TrimSpace(c.Tracing.ServiceName) == "" {
		return errors.New("tracing.service_name is required")
	}
	if c.Tracing.SampleRatio < 0 || c.Tracing.SampleRatio > 1 {
		return errors.New("tracing.sample_ratio must be between 0 and 1")
	}
	return nil
}

func supportedChunkProfile(docType string) bool {
	switch docType {
	case "product_detail",
		"product_parameter",
		"product_comparison",
		"purchase_guide",
		"accessory_compatibility",
		"user_manual",
		"troubleshooting",
		"after_sales_policy",
		"faq":
		return true
	default:
		return false
	}
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "clean-care-agent")
	v.SetDefault("app.env", "local")
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "10s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "60s")
	v.SetDefault("server.shutdown_timeout", "10s")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.development", true)
	v.SetDefault("auth.enabled", false)
	v.SetDefault("auth.development_user_id", "demo-user")
	v.SetDefault("auth.jwt_secret", "")
	v.SetDefault("auth.jwt_issuer", "clean-care-agent")
	v.SetDefault("auth.jwt_leeway", "30s")
	v.SetDefault("auth.admin_api_key", "")
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.backend", "local")
	v.SetDefault("rate_limit.requests_per_second", 20)
	v.SetDefault("rate_limit.burst", 40)
	v.SetDefault("agent.mode", "bootstrap")
	v.SetDefault("agent.planning_mode", "auto")
	v.SetDefault("agent.skill_config_path", "")
	v.SetDefault("agent.max_steps", 5)
	v.SetDefault("agent.token_budget", 6000)
	v.SetDefault("agent.timeout", "20s")
	v.SetDefault("storage.conversation_repository", "memory")
	v.SetDefault("mysql.enabled", false)
	v.SetDefault("mysql.dsn", "")
	v.SetDefault("mysql.max_open_conns", 20)
	v.SetDefault("mysql.max_idle_conns", 10)
	v.SetDefault("mysql.conn_max_lifetime", "30m")
	v.SetDefault("mysql.conn_max_idle_time", "5m")
	v.SetDefault("mysql.ping_timeout", "3s")
	v.SetDefault("mysql.auto_migrate", false)
	v.SetDefault("redis.enabled", false)
	v.SetDefault("redis.address", "127.0.0.1:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.dial_timeout", "3s")
	v.SetDefault("redis.read_timeout", "2s")
	v.SetDefault("redis.write_timeout", "2s")
	v.SetDefault("redis.session_ttl", "24h")
	v.SetDefault("redis.recent_messages", 10)
	v.SetDefault("redis.ingest_stream_enabled", false)
	v.SetDefault("redis.ingest_stream", "cleancare:kb:ingest")
	v.SetDefault("redis.ingest_consumer_group", "cleancare-kb-workers")
	v.SetDefault("redis.ingest_consumer_name", "worker-1")
	v.SetDefault("redis.ingest_block_timeout", "2s")
	v.SetDefault("redis.ingest_claim_idle", "1m")
	v.SetDefault("redis.ingest_batch_size", 8)
	v.SetDefault("redis.ingest_max_retries", 3)
	v.SetDefault("redis.ingest_dead_letter_name", "cleancare:kb:ingest:dead")
	v.SetDefault("qdrant.enabled", false)
	v.SetDefault("qdrant.base_url", "http://127.0.0.1:6333")
	v.SetDefault("qdrant.api_key", "")
	v.SetDefault("qdrant.collection", "clean_care_kb")
	v.SetDefault("qdrant.vector_size", 1024)
	v.SetDefault("qdrant.distance", "cosine")
	v.SetDefault("qdrant.request_timeout", "5s")
	v.SetDefault("qdrant.ensure_collection", true)
	v.SetDefault("readiness.timeout", "2s")
	v.SetDefault("embedding.provider", "local_hash")
	v.SetDefault("embedding.endpoint", "")
	v.SetDefault("embedding.api_key", "")
	v.SetDefault("embedding.model", "local-hash-v1")
	v.SetDefault("embedding.dimension", 1024)
	v.SetDefault("embedding.request_timeout", "15s")
	v.SetDefault("embedding.batch_size", 16)
	v.SetDefault("embedding.fallbacks", []map[string]any{})
	v.SetDefault("embedding.failure_threshold", 5)
	v.SetDefault("embedding.open_timeout", "1m")
	v.SetDefault("rag.dense_top_k", 20)
	v.SetDefault("rag.keyword_top_k", 20)
	v.SetDefault("rag.rerank_top_k", 6)
	v.SetDefault("rag.min_dense_score", 0.05)
	v.SetDefault("rag.max_chunk_runes", 1200)
	v.SetDefault("rag.chunk_overlap", 120)
	v.SetDefault("rag.chunk_profiles.product_detail.max_chunk_runes", 1400)
	v.SetDefault("rag.chunk_profiles.product_detail.chunk_overlap", 120)
	v.SetDefault("rag.chunk_profiles.product_detail.semantic_threshold", 0.68)
	v.SetDefault("rag.chunk_profiles.product_parameter.max_chunk_runes", 1200)
	v.SetDefault("rag.chunk_profiles.product_parameter.chunk_overlap", 0)
	v.SetDefault("rag.chunk_profiles.product_comparison.max_chunk_runes", 2000)
	v.SetDefault("rag.chunk_profiles.product_comparison.chunk_overlap", 0)
	v.SetDefault("rag.chunk_profiles.purchase_guide.max_chunk_runes", 1400)
	v.SetDefault("rag.chunk_profiles.purchase_guide.chunk_overlap", 120)
	v.SetDefault("rag.chunk_profiles.purchase_guide.semantic_threshold", 0.72)
	v.SetDefault("rag.chunk_profiles.accessory_compatibility.max_chunk_runes", 1600)
	v.SetDefault("rag.chunk_profiles.accessory_compatibility.chunk_overlap", 0)
	v.SetDefault("rag.chunk_profiles.user_manual.max_chunk_runes", 1600)
	v.SetDefault("rag.chunk_profiles.user_manual.chunk_overlap", 80)
	v.SetDefault("rag.chunk_profiles.troubleshooting.max_chunk_runes", 600)
	v.SetDefault("rag.chunk_profiles.troubleshooting.chunk_overlap", 0)
	v.SetDefault("rag.chunk_profiles.after_sales_policy.max_chunk_runes", 800)
	v.SetDefault("rag.chunk_profiles.after_sales_policy.chunk_overlap", 0)
	v.SetDefault("rag.chunk_profiles.faq.max_chunk_runes", 400)
	v.SetDefault("rag.chunk_profiles.faq.chunk_overlap", 0)
	v.SetDefault("rag.max_answer_runes", 900)
	v.SetDefault("rag.retrieval_cache_ttl", "5m")
	v.SetDefault("reranker.provider", "local_lexical")
	v.SetDefault("reranker.endpoint", "")
	v.SetDefault("reranker.api_key", "")
	v.SetDefault("reranker.model", "")
	v.SetDefault("reranker.request_timeout", "10s")
	v.SetDefault("reranker.fallbacks", []map[string]any{})
	v.SetDefault("reranker.failure_threshold", 5)
	v.SetDefault("reranker.open_timeout", "1m")
	v.SetDefault("llm.provider", "extractive")
	v.SetDefault("llm.endpoint", "")
	v.SetDefault("llm.api_key", "")
	v.SetDefault("llm.model", "")
	v.SetDefault("llm.request_timeout", "30s")
	v.SetDefault("llm.first_token_timeout", "3s")
	v.SetDefault("llm.max_tokens", 800)
	v.SetDefault("llm.temperature", 0.1)
	v.SetDefault("llm.failure_threshold", 5)
	v.SetDefault("llm.open_timeout", "1m")
	v.SetDefault("llm.fallbacks", []map[string]any{})
	v.SetDefault("llm.providers", []map[string]any{})
	v.SetDefault("tool.timeout", "3s")
	v.SetDefault("tool.data_scope", "mock")
	v.SetDefault("tool.mcp.transport", "in_process")
	v.SetDefault("tool.mcp.endpoint", "http://127.0.0.1:8090/mcp")
	v.SetDefault("tool.mcp.api_key", "")
	v.SetDefault("tool.mcp.headers", map[string]string{})
	v.SetDefault("tool.mcp.request_timeout", "5s")
	v.SetDefault("tool.mcp.list_cache_ttl", "30s")
	v.SetDefault("tool.mcp.max_retries", 2)
	v.SetDefault("tool.mcp.retry_base_delay", "100ms")
	v.SetDefault("tool.mcp.retry_max_delay", "1s")
	v.SetDefault("tool.mcp.listen_host", "127.0.0.1")
	v.SetDefault("tool.mcp.listen_port", 8090)
	v.SetDefault("tool.mcp.path", "/mcp")
	v.SetDefault("tool.mcp.server_api_key", "")
	v.SetDefault("tool.mcp.allowed_origins", []string{})
	v.SetDefault("tracing.enabled", false)
	v.SetDefault("tracing.service_name", "clean-care-agent")
	v.SetDefault("tracing.service_version", "dev")
	v.SetDefault("tracing.otlp_endpoint", "")
	v.SetDefault("tracing.insecure", true)
	v.SetDefault("tracing.sample_ratio", 1.0)
	v.SetDefault("prompt.enable_llm_components", false)
	v.SetDefault("prompt.intent_classifier_model", "")
	v.SetDefault("prompt.query_rewrite_model", "")
	v.SetDefault("prompt.planner_model", "")
	v.SetDefault("prompt.reflection_model", "")
	v.SetDefault("prompt.clarifier_model", "")
	v.SetDefault("prompt.generation_model", "")
	v.SetDefault("prompt.summarizer_model", "")
	v.SetDefault("prompt.eval_judge_model", "")
}
