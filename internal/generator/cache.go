package generator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"CleanCaregent/internal/prompt"
	"CleanCaregent/internal/rag"
)

// Cached memoizes grounded generation results for repeated recommendation,
// comparison and usage turns that share the same evidence and tool context.
type Cached struct {
	next       Generator
	ttl        time.Duration
	maxEntries int

	mu      sync.Mutex
	entries map[string]cachedEntry
}

type cachedEntry struct {
	answer    string
	expiresAt time.Time
	lastUsed  time.Time
}

func NewCached(next Generator, ttl time.Duration, maxEntries int) Generator {
	if next == nil || ttl <= 0 || maxEntries <= 0 {
		return next
	}
	return &Cached{
		next:       next,
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    make(map[string]cachedEntry, maxEntries),
	}
}

func (g *Cached) Name() string {
	return g.next.Name() + "|cache"
}

func (g *Cached) Generate(
	ctx context.Context,
	query string,
	evidence []rag.SearchResult,
) (string, error) {
	return g.GenerateWithScenario(ctx, prompt.ScenarioGenerateGeneric, query, evidence, "", "", "")
}

func (g *Cached) GenerateWithScenario(
	ctx context.Context,
	scenario prompt.Scenario,
	query string,
	evidence []rag.SearchResult,
	toolResults string,
	conversationSummary string,
	models string,
) (string, error) {
	key := generationCacheKey(scenario, query, evidence, toolResults, conversationSummary, models)
	now := time.Now()
	if answer, ok := g.get(key, now); ok {
		return answer, nil
	}
	answer, err := g.next.GenerateWithScenario(
		ctx,
		scenario,
		query,
		evidence,
		toolResults,
		conversationSummary,
		models,
	)
	if err != nil || answer == "" {
		return answer, err
	}
	g.put(key, answer, now)
	return answer, nil
}

func (g *Cached) get(key string, now time.Time) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	entry, ok := g.entries[key]
	if !ok || now.After(entry.expiresAt) {
		if ok {
			delete(g.entries, key)
		}
		return "", false
	}
	entry.lastUsed = now
	g.entries[key] = entry
	return entry.answer, true
}

func (g *Cached) put(key, answer string, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.entries) >= g.maxEntries {
		g.evictLocked(now)
	}
	g.entries[key] = cachedEntry{
		answer:    answer,
		expiresAt: now.Add(g.ttl),
		lastUsed:  now,
	}
}

func (g *Cached) evictLocked(now time.Time) {
	var oldestKey string
	var oldest time.Time
	for key, entry := range g.entries {
		if now.After(entry.expiresAt) {
			delete(g.entries, key)
			continue
		}
		if oldestKey == "" || entry.lastUsed.Before(oldest) {
			oldestKey = key
			oldest = entry.lastUsed
		}
	}
	if len(g.entries) >= g.maxEntries && oldestKey != "" {
		delete(g.entries, oldestKey)
	}
}

func generationCacheKey(
	scenario prompt.Scenario,
	query string,
	evidence []rag.SearchResult,
	toolResults string,
	conversationSummary string,
	models string,
) string {
	payload := struct {
		Scenario            prompt.Scenario     `json:"scenario"`
		Query               string              `json:"query"`
		Evidence            []cacheEvidenceItem `json:"evidence"`
		ToolResults         string              `json:"tool_results"`
		ConversationSummary string              `json:"conversation_summary"`
		Models              string              `json:"models"`
	}{
		Scenario:            scenario,
		Query:               query,
		Evidence:            cacheEvidence(evidence),
		ToolResults:         toolResults,
		ConversationSummary: conversationSummary,
		Models:              models,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type cacheEvidenceItem struct {
	ChunkID    string         `json:"chunk_id"`
	DocumentID string         `json:"document_id"`
	Title      string         `json:"title"`
	Content    string         `json:"content"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

func cacheEvidence(evidence []rag.SearchResult) []cacheEvidenceItem {
	items := make([]cacheEvidenceItem, 0, len(evidence))
	for _, item := range evidence {
		items = append(items, cacheEvidenceItem{
			ChunkID:    item.ChunkID,
			DocumentID: item.DocumentID,
			Title:      item.Title,
			Content:    item.Content,
			Metadata:   stableMetadata(item.Metadata),
		})
	}
	return items
}

func stableMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		result[key] = metadata[key]
	}
	return result
}
