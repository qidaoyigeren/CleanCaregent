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
	if profile := cfg.RAG.ChunkProfiles["troubleshooting"]; profile.MaxChunkRunes != 900 || profile.ChunkOverlap != 0 {
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
