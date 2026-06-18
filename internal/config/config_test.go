package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("CLEANCARE_SERVER_PORT", "18080")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Name != "clean-care-agent" {
		t.Fatalf("App.Name = %q", cfg.App.Name)
	}
	if cfg.Server.Port != 18080 {
		t.Fatalf("Server.Port = %d", cfg.Server.Port)
	}
	if cfg.Agent.Timeout != 20*time.Second {
		t.Fatalf("Agent.Timeout = %s", cfg.Agent.Timeout)
	}
	if cfg.Tool.MCP.Transport != "in_process" || cfg.Tool.MCP.Path != "/mcp" {
		t.Fatalf("Tool.MCP = %#v", cfg.Tool.MCP)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 70000\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load() expected validation error")
	}
}

func TestLoadUsesConfigFileEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e.yaml")
	content := []byte(`
server:
  port: 19091
mysql:
  enabled: true
  dsn: "user:pass@tcp(127.0.0.1:13306)/cleancare?parseTime=true&multiStatements=true"
storage:
  conversation_repository: mysql
qdrant:
  enabled: true
  base_url: "http://127.0.0.1:16333"
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CLEANCARE_CONFIG_FILE", path)
	t.Setenv("CLEANCARE_SERVER_PORT", "19092")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 19092 {
		t.Fatalf("Server.Port = %d", cfg.Server.Port)
	}
	if cfg.MySQL.DSN == "" || cfg.Qdrant.BaseURL != "http://127.0.0.1:16333" {
		t.Fatalf("loaded config = %#v", cfg)
	}
}

func TestValidateRequiresEnabledMySQLRepository(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.Storage.ConversationRepository = "mysql"
	cfg.MySQL.Enabled = false

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected mysql repository error")
	}
}

func TestValidateAutoMigrateRequiresMultiStatements(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.MySQL.Enabled = true
	cfg.MySQL.AutoMigrate = true
	cfg.MySQL.DSN = "user:pass@tcp(localhost:3306)/db?parseTime=true"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected multiStatements error")
	}
}

func TestValidateRejectsLocalHashInProduction(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.App.Env = "production"
	cfg.Embedding.Provider = "local_hash"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected production local_hash error")
	}
}

func TestValidateEmbeddingFallbackDimension(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.Embedding.Fallbacks = []EmbeddingFallbackConfig{{
		Endpoint:       "https://example.com/v1/embeddings",
		Model:          "fallback",
		Dimension:      cfg.Embedding.Dimension + 1,
		BatchSize:      8,
		RequestTimeout: time.Second,
	}}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected fallback dimension error")
	}
}

func TestLoadProvidesDocumentChunkProfiles(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.RAG.ChunkProfiles) != 9 {
		t.Fatalf("chunk profile count = %d", len(cfg.RAG.ChunkProfiles))
	}
	if profile := cfg.RAG.ChunkProfiles["troubleshooting"]; profile.MaxChunkRunes != 600 || profile.ChunkOverlap != 0 {
		t.Fatalf("troubleshooting profile = %#v", profile)
	}
}

func TestExampleConfigsLoad(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "configs", "config.example.yaml"),
		filepath.Join("..", "..", "configs", "config.local.example.yaml"),
	} {
		if _, err := Load(path); err != nil {
			t.Fatalf("Load(%s) error = %v", path, err)
		}
	}
}

func TestValidateRejectsMoreThanFiveAgentSteps(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.Agent.MaxSteps = 6
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected max steps error")
	}
}

func TestValidateHTTPMCPRequiresEndpoint(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.Tool.MCP.Transport = "http"
	cfg.Tool.MCP.Endpoint = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected mcp endpoint error")
	}
}

func TestValidateStdioMCPRequiresCommand(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.Tool.MCP.Transport = "stdio"
	cfg.Tool.MCP.StdioCommand = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected mcp stdio command error")
	}
	cfg.Tool.MCP.StdioCommand = "clean-care-mcp"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error = %v", err)
	}
}

func TestValidateMCPServers(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.Tool.MCP.Servers = []ToolMCPServerConfig{
		{Name: "tools", Transport: "http", Endpoint: "http://127.0.0.1:8090/mcp"},
		{Name: "stdio", Transport: "stdio", StdioCommand: "clean-care-mcp"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error = %v", err)
	}
	cfg.Tool.MCP.Servers[1].Name = "tools"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected duplicate mcp server name error")
	}
}

func TestValidateRejectsInvalidMCPRetrySettings(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.Tool.MCP.RetryBaseDelay = 2 * time.Second
	cfg.Tool.MCP.RetryMaxDelay = time.Second
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected mcp retry settings error")
	}
}

func TestLoadInheritsRerankerKeyForSameProviderHost(t *testing.T) {
	t.Setenv("CLEANCARE_EMBEDDING_API_KEY", "shared-provider-key")
	t.Setenv("CLEANCARE_RERANKER_API_KEY", "")
	path := filepath.Join(t.TempDir(), "same-host.yaml")
	content := []byte(`
embedding:
  provider: openai_compatible
  endpoint: https://api.siliconflow.cn/v1/embeddings
  model: BAAI/bge-large-zh-v1.5
reranker:
  provider: openai_compatible
  endpoint: https://api.siliconflow.cn/v1/rerank
  model: BAAI/bge-reranker-v2-m3
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Reranker.APIKey != cfg.Embedding.APIKey {
		t.Fatalf("reranker key was not inherited from the same provider host")
	}
}

func TestLoadEnvOverridesBlankAPIKeys(t *testing.T) {
	t.Setenv("CLEANCARE_LLM_API_KEY", "llm-env-key")
	t.Setenv("CLEANCARE_EMBEDDING_API_KEY", "embedding-env-key")
	path := filepath.Join(t.TempDir(), "blank-keys.yaml")
	content := []byte(`
embedding:
  provider: openai_compatible
  endpoint: https://api.siliconflow.cn/v1/embeddings
  api_key: ""
  model: BAAI/bge-large-zh-v1.5
reranker:
  provider: openai_compatible
  endpoint: https://api.siliconflow.cn/v1/rerank
  api_key: ""
  model: BAAI/bge-reranker-v2-m3
llm:
  provider: openai_compatible
  endpoint: https://api.deepseek.com/v1/chat/completions
  api_key: ""
  model: deepseek-chat
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.APIKey != "llm-env-key" {
		t.Fatalf("llm api key = %q, want env override", cfg.LLM.APIKey)
	}
	if cfg.Embedding.APIKey != "embedding-env-key" {
		t.Fatalf("embedding api key = %q, want env override", cfg.Embedding.APIKey)
	}
	if cfg.Reranker.APIKey != "embedding-env-key" {
		t.Fatalf("reranker api key = %q, want inherited embedding key", cfg.Reranker.APIKey)
	}
}

func TestLoadDoesNotShareProviderKeyAcrossHosts(t *testing.T) {
	t.Setenv("CLEANCARE_EMBEDDING_API_KEY", "embedding-only-key")
	t.Setenv("CLEANCARE_RERANKER_API_KEY", "")
	path := filepath.Join(t.TempDir(), "different-host.yaml")
	content := []byte(`
embedding:
  provider: openai_compatible
  endpoint: https://api.siliconflow.cn/v1/embeddings
  model: BAAI/bge-large-zh-v1.5
reranker:
  provider: openai_compatible
  endpoint: https://rerank.example.com/v1/rerank
  model: example-reranker
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Reranker.APIKey != "" {
		t.Fatal("reranker key must not be inherited across provider hosts")
	}
}
