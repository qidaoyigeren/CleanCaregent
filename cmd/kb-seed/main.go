package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"CleanCaregent/internal/config"
	"CleanCaregent/internal/embedding"
	"CleanCaregent/internal/migrate"
	mysqlclient "CleanCaregent/internal/platform/mysql"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/repository"
	mysqlrepository "CleanCaregent/internal/repository/mysql"
	"CleanCaregent/internal/seed"
	"CleanCaregent/internal/service"
	qdrantstore "CleanCaregent/internal/vectorstore/qdrant"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		fail("load config", err)
	}
	if !cfg.MySQL.Enabled || !cfg.Qdrant.Enabled {
		fail("validate config", fmt.Errorf("mysql and qdrant must be enabled"))
	}
	ctx := context.Background()
	db, err := mysqlclient.Open(ctx, cfg.MySQL)
	if err != nil {
		fail("open mysql", err)
	}
	defer db.Close()
	if err := migrate.Up(ctx, db); err != nil {
		fail("migrate mysql", err)
	}
	vector := qdrantstore.NewClient(cfg.Qdrant)
	if err := vector.EnsureCollection(ctx); err != nil {
		fail("ensure qdrant collection", err)
	}
	var embedder embedding.Embedder = embedding.NewLocalHash(cfg.Embedding.Dimension)
	if cfg.Embedding.Provider == "openai_compatible" {
		primary := embedding.NewOpenAIClient(
			cfg.Embedding.Endpoint, cfg.Embedding.APIKey, cfg.Embedding.Model,
			cfg.Embedding.Dimension, cfg.Embedding.BatchSize, cfg.Embedding.RequestTimeout,
		)
		embedder, err = embedding.NewFallback(primary, embedder)
		if err != nil {
			fail("create embedding fallback", err)
		}
	}
	knowledgeService := service.NewKnowledgeService(
		mysqlrepository.NewKnowledgeRepository(db),
		vector,
		embedder,
		rag.NewStructureAwareChunker(cfg.RAG.MaxChunkRunes, cfg.RAG.ChunkOverlap),
	)
	created, skipped := 0, 0
	for _, document := range seed.DefaultKnowledgeDocuments() {
		if _, err := knowledgeService.Ingest(ctx, document); err != nil {
			if errors.Is(err, repository.ErrKnowledgeDocumentConflict) {
				skipped++
				continue
			}
			fail("ingest "+document.DocID, err)
		}
		created++
	}
	fmt.Printf("knowledge seed completed: created=%d skipped=%d total=%d\n", created, skipped, created+skipped)
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
}
