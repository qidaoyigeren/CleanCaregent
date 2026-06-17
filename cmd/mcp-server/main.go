package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"CleanCaregent/internal/config"
	"CleanCaregent/internal/logging"
	"CleanCaregent/internal/migrate"
	mysqlclient "CleanCaregent/internal/platform/mysql"
	mysqlrepository "CleanCaregent/internal/repository/mysql"
	"CleanCaregent/internal/tool/builtin"
	toolmcp "CleanCaregent/internal/tool/mcp"

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
	if !cfg.MySQL.Enabled {
		logger.Fatal("mysql must be enabled for standalone mcp server")
	}

	db, err := mysqlclient.Open(context.Background(), cfg.MySQL)
	if err != nil {
		logger.Fatal("initialize mysql", zap.Error(err))
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.Warn("close mysql failed", zap.Error(closeErr))
		}
	}()
	if cfg.MySQL.AutoMigrate {
		if migrateErr := migrate.Up(context.Background(), db); migrateErr != nil {
			logger.Fatal("run database migrations", zap.Error(migrateErr))
		}
	}

	tools := builtin.NewBusinessTools(mysqlrepository.NewBusinessRepository(db))
	toolServer, err := toolmcp.NewServer(tools...)
	if err != nil {
		logger.Fatal("initialize mcp tool server", zap.Error(err))
	}
	if cfg.Tool.MCP.ServerTransport == "stdio" {
		logger.Info("mcp stdio server started", zap.Int("tool_count", len(tools)))
		if err := toolmcp.ServeStdio(context.Background(), toolServer, os.Stdin, os.Stdout); err != nil {
			logger.Fatal("mcp stdio server failed", zap.Error(err))
		}
		return
	}

	mux := http.NewServeMux()
	mcpPath := strings.TrimSpace(cfg.Tool.MCP.Path)
	mux.Handle(mcpPath, toolmcp.NewHTTPHandler(toolServer, toolmcp.HTTPHandlerConfig{
		APIKey:               firstNonEmpty(cfg.Tool.MCP.ServerAPIKey, cfg.Tool.MCP.APIKey),
		AllowedOrigins:       cfg.Tool.MCP.AllowedOrigins,
		StreamResponses:      cfg.Tool.MCP.StreamResponses,
		RequireSession:       cfg.Tool.MCP.RequireSession,
		AuthorizationServers: cfg.Tool.MCP.AuthorizationServers,
		Scopes:               cfg.Tool.MCP.Scopes,
	}))
	protectedResourceMetadata := toolmcp.NewProtectedResourceMetadataHandler(toolmcp.ProtectedResourceMetadataConfig{
		Resource:             "http://" + cfg.Tool.MCP.Address() + mcpPath,
		ResourceName:         "CleanCare MCP Tool Server",
		AuthorizationServers: cfg.Tool.MCP.AuthorizationServers,
		Scopes:               cfg.Tool.MCP.Scopes,
	})
	mux.Handle("/.well-known/oauth-protected-resource", protectedResourceMetadata)
	mux.Handle("/.well-known/oauth-protected-resource/", protectedResourceMetadata)
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	server := &http.Server{
		Addr:              cfg.Tool.MCP.Address(),
		Handler:           mux,
		ReadHeaderTimeout: cfg.Server.ReadTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("mcp server started",
			zap.String("address", server.Addr),
			zap.String("path", mcpPath),
			zap.Int("tool_count", len(tools)),
			zap.Bool("auth_enabled", firstNonEmpty(cfg.Tool.MCP.ServerAPIKey, cfg.Tool.MCP.APIKey) != ""),
		)
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case serveErr := <-errCh:
		logger.Fatal("mcp server failed", zap.Error(serveErr))
	case <-signalCtx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
		_ = server.Close()
	}
	logger.Info("mcp server stopped")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
