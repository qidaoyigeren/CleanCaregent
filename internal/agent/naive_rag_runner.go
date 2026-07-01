package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"CleanCaregent/internal/generator"
	"CleanCaregent/internal/rag"
)

type NaiveRAGConfig struct {
	DenseTopK     int
	KeywordTopK   int
	RerankTopK    int
	MinDenseScore float64
}

type NaiveRAGRunner struct {
	retriever rag.Retriever
	generator generator.Generator
	config    NaiveRAGConfig
}

var (
	productModelPattern       = regexp.MustCompile(`(?i)\b[A-Z][A-Z0-9-]*[0-9][A-Z0-9-]*(?:\s*(?:Pro|Plus|Max|Mini))?\b`)
	accessoryLikeModelPattern = regexp.MustCompile(`(?i)^(?:F|DB|RB|C)[0-9]{2,}[A-Z0-9-]*$`)
)

func NewNaiveRAGRunner(
	retriever rag.Retriever,
	generator generator.Generator,
	config NaiveRAGConfig,
) *NaiveRAGRunner {
	return &NaiveRAGRunner{
		retriever: retriever,
		generator: generator,
		config:    config,
	}
}

func (r *NaiveRAGRunner) Run(ctx context.Context, req Request, sink EventSink) (Result, error) {
	if sink != nil {
		if err := sink(Event{Type: "status", Data: map[string]any{
			"stage":    "retrieving",
			"mode":     "naive_rag",
			"trace_id": req.TraceID,
		}}); err != nil {
			return Result{}, err
		}
	}

	results, err := r.retriever.Search(ctx, rag.SearchRequest{
		Query: req.Query,
		Mode:  rag.SearchHybrid,
		Filter: rag.MetadataFilter{
			Models: extractProductModels(req.Query),
		},
		DenseTopK:   r.config.DenseTopK,
		KeywordTopK: r.config.KeywordTopK,
		RerankTopK:  r.config.RerankTopK,
		MinScore:    r.config.MinDenseScore,
		NeedRerank:  true,
	})
	if err != nil {
		return Result{}, fmt.Errorf("retrieve knowledge: %w", err)
	}

	evidences := make([]Evidence, len(results))
	for index, item := range results {
		evidences[index] = Evidence{
			ID:       fmt.Sprintf("E%d", index+1),
			Kind:     "kb_chunk",
			SourceID: item.ChunkID,
			Title:    item.Title,
			Content:  item.Content,
			Metadata: item.Metadata,
		}
		if sink != nil {
			if err := sink(Event{Type: "evidence", Data: evidences[index]}); err != nil {
				return Result{}, err
			}
		}
	}

	if sink != nil {
		if err := sink(Event{Type: "status", Data: map[string]any{
			"stage":     "generating",
			"generator": r.generator.Name(),
		}}); err != nil {
			return Result{}, err
		}
	}
	answer, err := r.generator.Generate(ctx, req.Query, results)
	if err != nil {
		return Result{}, fmt.Errorf("generate answer: %w", err)
	}
	for _, chunk := range splitForStream(answer, 24) {
		if sink != nil {
			if err := sink(Event{Type: "delta", Data: map[string]string{"content": chunk}}); err != nil {
				return Result{}, err
			}
		}
	}
	return Result{
		Answer:    answer,
		Evidences: evidences,
		Mode:      "naive_rag",
	}, nil
}

func extractProductModels(query string) []string {
	matches := productModelPattern.FindAllString(query, -1)
	seen := make(map[string]struct{}, len(matches))
	models := make([]string, 0, len(matches))
	for _, match := range matches {
		match = normalizeProductModel(match)
		if !isLikelyProductModel(match) {
			continue
		}
		key := strings.ToLower(match)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		models = append(models, match)
	}
	return models
}

func normalizeProductModel(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if strings.EqualFold(strings.ReplaceAll(value, " ", ""), "X20Pro") {
		return "X20 Pro"
	}
	return strings.ToUpper(value)
}

func isLikelyProductModel(value string) bool {
	compact := strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(value)), ""))
	if compact == "" {
		return false
	}
	if strings.HasPrefix(compact, "CC") && len(compact) >= 8 {
		return false
	}
	if strings.HasPrefix(compact, "ORDER") && len(compact) >= 10 {
		return false
	}
	if accessoryLikeModelPattern.MatchString(compact) {
		return false
	}
	switch compact {
	case "CLEAN100", "WIFI6":
		return false
	default:
		return true
	}
}
