package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"CleanCaregent/internal/model"
	"CleanCaregent/internal/repository"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

type KnowledgeRepository struct {
	db *sql.DB
}

func NewKnowledgeRepository(db *sql.DB) *KnowledgeRepository {
	return &KnowledgeRepository{db: db}
}

func (r *KnowledgeRepository) CreateDocument(
	ctx context.Context,
	document model.KnowledgeDocument,
	chunks []model.KnowledgeChunk,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create knowledge document: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO kb_documents (
			doc_id, title, category, brand, doc_type, version,
			effective_time, expire_time, source, status, content_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, document.DocID, document.Title, document.Category, nullableString(document.Brand),
		document.DocType, document.Version, document.EffectiveTime, document.ExpireTime,
		document.Source, document.Status, document.ContentHash)
	if err != nil {
		var mysqlErr *mysqlDriver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return repository.ErrKnowledgeDocumentConflict
		}
		return fmt.Errorf("insert knowledge document: %w", err)
	}
	documentID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read knowledge document id: %w", err)
	}

	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO kb_chunks (
			chunk_id, document_id, section_path, content, token_count,
			intent_tags_json, metadata_json, vector_point_id, content_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare knowledge chunks: %w", err)
	}
	defer statement.Close()

	for _, chunk := range chunks {
		intentTags, err := json.Marshal(chunk.IntentTags)
		if err != nil {
			return fmt.Errorf("encode chunk intent tags: %w", err)
		}
		metadata, err := json.Marshal(chunk.Metadata)
		if err != nil {
			return fmt.Errorf("encode chunk metadata: %w", err)
		}
		if _, err := statement.ExecContext(ctx,
			chunk.ChunkID,
			documentID,
			nullableString(chunk.SectionPath),
			chunk.Content,
			chunk.TokenCount,
			intentTags,
			metadata,
			chunk.VectorPointID,
			chunk.ContentHash,
		); err != nil {
			return fmt.Errorf("insert knowledge chunk %s: %w", chunk.ChunkID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit knowledge document: %w", err)
	}
	return nil
}

func (r *KnowledgeRepository) UpdateDocumentStatus(
	ctx context.Context,
	docID string,
	version string,
	status string,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE kb_documents
		SET status = ?, updated_at = UTC_TIMESTAMP(6)
		WHERE doc_id = ? AND version = ?
	`, status, docID, version)
	if err != nil {
		return fmt.Errorf("update knowledge document status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated knowledge document rows: %w", err)
	}
	if affected == 0 {
		return repository.ErrKnowledgeDocumentNotFound
	}
	return nil
}

// ActivateDocumentVersion atomically makes one version searchable and
// supersedes every previously active version of the same logical document.
// It returns the old Qdrant point IDs so the caller can remove stale vectors
// after the database transaction commits.
func (r *KnowledgeRepository) ActivateDocumentVersion(
	ctx context.Context,
	docID string,
	version string,
) ([]string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin activate knowledge document: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	rows, err := tx.QueryContext(ctx, `
		SELECT c.vector_point_id
		FROM kb_chunks c
		JOIN kb_documents d ON d.id = c.document_id
		WHERE d.doc_id = ?
		  AND d.version <> ?
		  AND d.status = ?
		  AND c.vector_point_id IS NOT NULL
		FOR UPDATE
	`, docID, version, model.KnowledgeStatusActive)
	if err != nil {
		return nil, fmt.Errorf("list superseded vector points: %w", err)
	}
	var stalePointIDs []string
	for rows.Next() {
		var pointID string
		if err := rows.Scan(&pointID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan superseded vector point: %w", err)
		}
		if pointID != "" {
			stalePointIDs = append(stalePointIDs, pointID)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate superseded vector points: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close superseded vector points: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE kb_documents
		SET status = ?, updated_at = UTC_TIMESTAMP(6)
		WHERE doc_id = ? AND version <> ? AND status = ?
	`, model.KnowledgeStatusSuperseded, docID, version, model.KnowledgeStatusActive); err != nil {
		return nil, fmt.Errorf("supersede old knowledge versions: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE kb_documents
		SET status = ?, updated_at = UTC_TIMESTAMP(6)
		WHERE doc_id = ? AND version = ? AND status = ?
	`, model.KnowledgeStatusActive, docID, version, model.KnowledgeStatusIndexing)
	if err != nil {
		return nil, fmt.Errorf("activate new knowledge version: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read activated knowledge document rows: %w", err)
	}
	if affected == 0 {
		return nil, repository.ErrKnowledgeDocumentNotFound
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit activate knowledge document: %w", err)
	}
	return stalePointIDs, nil
}

func (r *KnowledgeRepository) KeywordSearch(
	ctx context.Context,
	request repository.KnowledgeSearchRequest,
) ([]model.KnowledgeChunk, error) {
	if request.Limit <= 0 {
		request.Limit = 20
	}
	terms := uniqueNonEmpty(request.Terms)
	if len(terms) == 0 {
		terms = []string{strings.TrimSpace(request.Query)}
	}
	if len(terms) > 32 {
		terms = terms[:32]
	}

	var conditions []string
	var args []any
	conditions = append(conditions,
		"d.status = 'active'",
		"(d.effective_time IS NULL OR d.effective_time <= ?)",
		"(d.expire_time IS NULL OR d.expire_time > ?)",
	)
	args = append(args, request.EffectiveAt, request.EffectiveAt)

	addInCondition(&conditions, &args, "d.category", request.Categories)
	addInCondition(&conditions, &args, "d.brand", request.Brands)
	addInCondition(&conditions, &args, "d.doc_type", request.DocTypes)
	if version := strings.TrimSpace(request.Version); version != "" {
		conditions = append(conditions, "d.version = ?")
		args = append(args, version)
	}

	if len(request.Models) > 0 {
		addJSONMetadataAnyCondition(&conditions, &args, "model", "models", request.Models)
	}
	addJSONMetadataAnyCondition(&conditions, &args, "product_id", "product_ids", request.ProductIDs)
	addJSONMetadataAnyCondition(&conditions, &args, "sku_id", "sku_ids", request.SKUIDs)
	addJSONArrayAnyCondition(&conditions, &args, "c.intent_tags_json", request.IntentTags)
	addJSONMetadataAnyCondition(&conditions, &args, "fault_node_id", "fault_node_ids", request.FaultNodeIDs)

	var keywordConditions []string
	for _, term := range terms {
		keywordConditions = append(keywordConditions, "(c.content LIKE ? OR d.title LIKE ?)")
		pattern := "%" + escapeLike(term) + "%"
		args = append(args, pattern, pattern)
	}
	conditions = append(conditions, "("+strings.Join(keywordConditions, " OR ")+")")
	args = append(args, min(request.Limit*5, 100))

	query := `
		SELECT
			c.chunk_id, d.doc_id, d.title, c.section_path, c.content, c.token_count,
			c.intent_tags_json, c.metadata_json, c.vector_point_id, c.content_hash
		FROM kb_chunks c
		JOIN kb_documents d ON d.id = c.document_id
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY d.updated_at DESC, c.id ASC
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("keyword search knowledge chunks: %w", err)
	}
	defer rows.Close()

	chunks, err := scanKnowledgeChunks(rows)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(chunks, func(i, j int) bool {
		return keywordScore(chunks[i], terms) > keywordScore(chunks[j], terms)
	})
	if len(chunks) > request.Limit {
		chunks = chunks[:request.Limit]
	}
	return chunks, nil
}

func (r *KnowledgeRepository) FindActiveChunks(
	ctx context.Context,
	chunkIDs []string,
	effectiveAt time.Time,
) ([]model.KnowledgeChunk, error) {
	if len(chunkIDs) == 0 {
		return []model.KnowledgeChunk{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunkIDs)), ",")
	args := make([]any, 0, len(chunkIDs)+2)
	args = append(args, effectiveAt, effectiveAt)
	for _, chunkID := range chunkIDs {
		args = append(args, chunkID)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			c.chunk_id, d.doc_id, d.title, c.section_path, c.content, c.token_count,
			c.intent_tags_json, c.metadata_json, c.vector_point_id, c.content_hash
		FROM kb_chunks c
		JOIN kb_documents d ON d.id = c.document_id
		WHERE d.status = 'active'
		  AND (d.effective_time IS NULL OR d.effective_time <= ?)
		  AND (d.expire_time IS NULL OR d.expire_time > ?)
		  AND c.chunk_id IN (`+placeholders+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("find active knowledge chunks: %w", err)
	}
	defer rows.Close()
	return scanKnowledgeChunks(rows)
}

type rowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanKnowledgeChunks(rows rowScanner) ([]model.KnowledgeChunk, error) {
	var chunks []model.KnowledgeChunk
	for rows.Next() {
		var chunk model.KnowledgeChunk
		var sectionPath sql.NullString
		var intentTagsRaw, metadataRaw []byte
		if err := rows.Scan(
			&chunk.ChunkID,
			&chunk.DocID,
			&chunk.Title,
			&sectionPath,
			&chunk.Content,
			&chunk.TokenCount,
			&intentTagsRaw,
			&metadataRaw,
			&chunk.VectorPointID,
			&chunk.ContentHash,
		); err != nil {
			return nil, fmt.Errorf("scan knowledge chunk: %w", err)
		}
		if sectionPath.Valid {
			chunk.SectionPath = sectionPath.String
		}
		if len(intentTagsRaw) > 0 {
			if err := json.Unmarshal(intentTagsRaw, &chunk.IntentTags); err != nil {
				return nil, fmt.Errorf("decode chunk intent tags: %w", err)
			}
		}
		if len(metadataRaw) > 0 {
			if err := json.Unmarshal(metadataRaw, &chunk.Metadata); err != nil {
				return nil, fmt.Errorf("decode chunk metadata: %w", err)
			}
		}
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge chunks: %w", err)
	}
	return chunks, nil
}

func addInCondition(conditions *[]string, args *[]any, column string, values []string) {
	values = uniqueNonEmpty(values)
	if len(values) == 0 {
		return
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
	*conditions = append(*conditions, column+" IN ("+placeholders+")")
	for _, value := range values {
		*args = append(*args, value)
	}
}

func addJSONMetadataAnyCondition(
	conditions *[]string,
	args *[]any,
	singularKey string,
	arrayKey string,
	values []string,
) {
	values = uniqueNonEmpty(values)
	if len(values) == 0 {
		return
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, "(JSON_UNQUOTE(JSON_EXTRACT(c.metadata_json, '$."+singularKey+"')) = ? OR JSON_CONTAINS(c.metadata_json, JSON_QUOTE(?), '$."+arrayKey+"'))")
		*args = append(*args, value, value)
	}
	*conditions = append(*conditions, "("+strings.Join(parts, " OR ")+")")
}

func addJSONArrayAnyCondition(conditions *[]string, args *[]any, column string, values []string) {
	values = uniqueNonEmpty(values)
	if len(values) == 0 {
		return
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, "JSON_CONTAINS("+column+", JSON_QUOTE(?))")
		*args = append(*args, value)
	}
	*conditions = append(*conditions, "("+strings.Join(parts, " OR ")+")")
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func keywordScore(chunk model.KnowledgeChunk, terms []string) int {
	content := strings.ToLower(chunk.Title + "\n" + chunk.Content)
	score := 0
	for _, term := range terms {
		score += strings.Count(content, strings.ToLower(term))
	}
	return score
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}
