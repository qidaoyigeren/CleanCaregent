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
