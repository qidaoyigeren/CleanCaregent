package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"CleanCaregent/internal/config"
	"CleanCaregent/internal/embedding"
	"CleanCaregent/internal/ingest"
	"CleanCaregent/internal/migrate"
	mysqlclient "CleanCaregent/internal/platform/mysql"
	"CleanCaregent/internal/productpack"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/repository"
	mysqlrepository "CleanCaregent/internal/repository/mysql"
	"CleanCaregent/internal/seed"
	"CleanCaregent/internal/service"
	"CleanCaregent/internal/vectorstore"
	qdrantstore "CleanCaregent/internal/vectorstore/qdrant"
)

func main() {
	kbPathsFlag := flag.String("kb-paths", "", "comma or semicolon separated knowledge pack files/directories")
	productPackPathsFlag := flag.String("product-packs", "", "comma or semicolon separated product pack files/directories")
	builtinFlag := flag.Bool("builtin", true, "include built-in mock knowledge documents")
	forceFlag := flag.Bool("force", false, "replace an existing doc_id+version when content changed")
	flag.Parse()

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
		local := embedder
		primary := embedding.WithCircuitBreaker(
			embedding.NewOpenAIClient(
				cfg.Embedding.Endpoint, cfg.Embedding.APIKey, cfg.Embedding.Model,
				cfg.Embedding.Dimension, cfg.Embedding.BatchSize, cfg.Embedding.RequestTimeout,
			),
			cfg.Embedding.FailureThreshold,
			cfg.Embedding.OpenTimeout,
		)
		embedder = primary
		if !strings.EqualFold(cfg.App.Env, "production") {
			embedder, err = embedding.NewFallback(primary, local)
			if err != nil {
				fail("create embedding fallback", err)
			}
		}
	}
	knowledgeRepo := mysqlrepository.NewKnowledgeRepository(db)
	knowledgeService := service.NewKnowledgeService(
		knowledgeRepo,
		vector,
		embedder,
		rag.NewSemanticProfiledStructureAwareChunker(
			cfg.RAG.MaxChunkRunes,
			cfg.RAG.ChunkOverlap,
			chunkProfiles(cfg.RAG.ChunkProfiles),
			embedder,
		),
	)
	stats := seedStats{}
	seedPaths := cfg.Knowledge.SeedPaths
	if raw := strings.TrimSpace(os.Getenv("CLEANCARE_KB_SEED_PATHS")); raw != "" {
		seedPaths = splitSeedPaths(raw)
	}
	if strings.TrimSpace(*kbPathsFlag) != "" {
		seedPaths = splitSeedPaths(*kbPathsFlag)
	}
	includeBuiltIn := cfg.Knowledge.SeedBuiltInMock
	if flagWasSet("builtin") {
		includeBuiltIn = *builtinFlag
	}
	productPackPaths := cfg.Knowledge.ProductPackPaths
	if raw := strings.TrimSpace(os.Getenv("CLEANCARE_PRODUCT_PACK_PATHS")); raw != "" {
		productPackPaths = splitSeedPaths(raw)
	}
	if strings.TrimSpace(*productPackPathsFlag) != "" {
		productPackPaths = splitSeedPaths(*productPackPathsFlag)
	}

	packs, err := productpack.Load(productPackPaths...)
	if err != nil {
		fail("load product packs", err)
	}
	if errs := productpack.Validate(packs); len(errs) > 0 {
		fail("validate product packs", errs[0])
	}
	syncResult, err := productpack.SyncBusinessData(ctx, db, packs)
	if err != nil {
		fail("sync product pack business data", err)
	}

	var documents []service.IngestDocumentRequest
	if includeBuiltIn {
		documents = append(documents, seed.DefaultKnowledgeDocuments()...)
	}
	externalDocuments, err := ingest.LoadKnowledgeDocuments(seedPaths...)
	if err != nil {
		fail("load knowledge seed paths", err)
	}
	documents = append(documents, externalDocuments...)
	packDocuments, err := productpack.KnowledgeDocuments(packs)
	if err != nil {
		fail("load product pack documents", err)
	}
	documents = append(documents, packDocuments...)
	documentSyncResult, err := productpack.SyncKnowledgeDocumentBusinessData(ctx, db, documents)
	if err != nil {
		fail("sync document business data", err)
	}
	syncResult.Products += documentSyncResult.Products
	syncResult.SKUs += documentSyncResult.SKUs
	syncResult.Inventory += documentSyncResult.Inventory

	for _, document := range documents {
		action, err := ingestSeedDocument(ctx, knowledgeService, knowledgeRepo, vector, document, *forceFlag)
		if err != nil {
			fail("ingest "+document.DocID, err)
		}
		stats.add(action)
	}
	fmt.Printf(
		"knowledge seed completed: created=%d replaced=%d skipped=%d total=%d builtin=%t external=%d pack_docs=%d kb_paths=%s product_packs=%d synced_products=%d synced_skus=%d synced_inventory=%d product_pack_paths=%s\n",
		stats.created,
		stats.replaced,
		stats.skipped,
		stats.created+stats.replaced+stats.skipped,
		includeBuiltIn,
		len(externalDocuments),
		len(packDocuments),
		strings.Join(seedPaths, ","),
		len(packs),
		syncResult.Products,
		syncResult.SKUs,
		syncResult.Inventory,
		strings.Join(productPackPaths, ","),
	)
}

type seedAction string

const (
	seedCreated  seedAction = "created"
	seedReplaced seedAction = "replaced"
	seedSkipped  seedAction = "skipped"
)

type seedStats struct {
	created  int
	replaced int
	skipped  int
}

func (s *seedStats) add(action seedAction) {
	switch action {
	case seedCreated:
		s.created++
	case seedReplaced:
		s.replaced++
	case seedSkipped:
		s.skipped++
	}
}

func ingestSeedDocument(
	ctx context.Context,
	knowledgeService *service.KnowledgeService,
	knowledgeRepo repository.KnowledgeRepository,
	vector vectorstore.Store,
	document service.IngestDocumentRequest,
	force bool,
) (seedAction, error) {
	version := strings.TrimSpace(document.Version)
	if version == "" {
		version = "1.0"
	}
	if versionRepo, ok := knowledgeRepo.(repository.KnowledgeVersionRepository); ok {
		existing, err := versionRepo.GetDocumentVersion(ctx, document.DocID, version)
		if err == nil {
			if seedDocumentUnchanged(existing, document, version) {
				return seedSkipped, nil
			}
			if !force {
				return "", fmt.Errorf(
					"document %s version %s already exists with different content; bump version or rerun with -force",
					document.DocID,
					version,
				)
			}
			pointIDs, err := versionRepo.DeleteDocumentVersion(ctx, document.DocID, version)
			if err != nil {
				return "", err
			}
			if len(pointIDs) > 0 {
				if err := vector.Delete(ctx, pointIDs); err != nil {
					return "", fmt.Errorf("delete stale vectors for %s@%s: %w", document.DocID, version, err)
				}
			}
			if _, err := knowledgeService.Ingest(ctx, document); err != nil {
				return "", err
			}
			return seedReplaced, nil
		}
		if !errors.Is(err, repository.ErrKnowledgeDocumentNotFound) {
			return "", err
		}
	}
	if _, err := knowledgeService.Ingest(ctx, document); err != nil {
		if errors.Is(err, repository.ErrKnowledgeDocumentConflict) {
			return seedSkipped, nil
		}
		return "", err
	}
	return seedCreated, nil
}

func seedDocumentUnchanged(
	existing repository.KnowledgeDocumentVersion,
	document service.IngestDocumentRequest,
	version string,
) bool {
	hash := service.ContentHash(strings.TrimSpace(document.Content))
	if existing.ContentHash != hash {
		return false
	}
	existingMetadata := cloneSeedMetadata(existing.Metadata)
	for _, key := range []string{
		"category",
		"brand",
		"doc_type",
		"version",
		"embedding_provider",
		"fulltext_score",
	} {
		delete(existingMetadata, key)
	}
	documentMetadata := document.Metadata
	if documentMetadata == nil {
		documentMetadata = map[string]any{}
	}
	if canonicalJSON(existingMetadata) != canonicalJSON(documentMetadata) {
		return false
	}
	return metadataString(existing.Metadata, "category") == strings.TrimSpace(document.Category) &&
		metadataString(existing.Metadata, "brand") == strings.TrimSpace(document.Brand) &&
		metadataString(existing.Metadata, "doc_type") == strings.TrimSpace(document.DocType) &&
		metadataString(existing.Metadata, "version") == strings.TrimSpace(version)
}

func cloneSeedMetadata(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func canonicalJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return string(raw)
	}
	raw, err = json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(raw)
}

func metadataString(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
	os.Exit(1)
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

func splitSeedPaths(value string) []string {
	parts := strings.FieldsFunc(value, func(current rune) bool {
		return current == ',' || current == ';'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func flagWasSet(name string) bool {
	wasSet := false
	flag.Visit(func(item *flag.Flag) {
		if item.Name == name {
			wasSet = true
		}
	})
	return wasSet
}
