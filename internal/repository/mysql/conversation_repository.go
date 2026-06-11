package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"CleanCaregent/internal/model"
	"CleanCaregent/internal/repository"
)

type ConversationRepository struct {
	db *sql.DB
}

func NewConversationRepository(db *sql.DB) *ConversationRepository {
	return &ConversationRepository{db: db}
}

func (r *ConversationRepository) Create(ctx context.Context, conversation model.Conversation) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create conversation: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `
		INSERT IGNORE INTO users (user_no, status, created_at, updated_at)
		VALUES (?, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))
	`, conversation.UserID); err != nil {
		return fmt.Errorf("ensure conversation user: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO conversations (
			conversation_no, user_id, title, status, last_message_at, created_at, updated_at
		)
		SELECT ?, id, ?, ?, ?, ?, ?
		FROM users
		WHERE user_no = ?
	`, conversation.ID, conversation.Title, conversation.Status, conversation.LastMessageAt,
		conversation.CreatedAt, conversation.CreatedAt, conversation.UserID); err != nil {
		return fmt.Errorf("insert conversation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create conversation: %w", err)
	}
	return nil
}

func (r *ConversationRepository) Get(ctx context.Context, userID, conversationID string) (model.Conversation, error) {
	var conversation model.Conversation
	err := r.db.QueryRowContext(ctx, `
		SELECT c.conversation_no, u.user_no, c.title, c.status, c.created_at, c.last_message_at
		FROM conversations c
		JOIN users u ON u.id = c.user_id
		WHERE c.conversation_no = ? AND u.user_no = ?
	`, conversationID, userID).Scan(
		&conversation.ID,
		&conversation.UserID,
		&conversation.Title,
		&conversation.Status,
		&conversation.CreatedAt,
		&conversation.LastMessageAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Conversation{}, repository.ErrConversationNotFound
	}
	if err != nil {
		return model.Conversation{}, fmt.Errorf("get conversation: %w", err)
	}
	return conversation, nil
}

func (r *ConversationRepository) AppendMessage(ctx context.Context, userID string, message model.Message) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin append message: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO messages (
			message_no, conversation_id, role, content, trace_id, created_at
		)
		SELECT ?, c.id, ?, ?, ?, ?
		FROM conversations c
		JOIN users u ON u.id = c.user_id
		WHERE c.conversation_no = ? AND u.user_no = ?
	`, message.ID, message.Role, message.Content, nullableString(message.TraceID), message.CreatedAt,
		message.ConversationID, userID)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read inserted message rows: %w", err)
	}
	if affected == 0 {
		return repository.ErrConversationNotFound
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations c
		JOIN users u ON u.id = c.user_id
		SET c.last_message_at = ?, c.updated_at = ?
		WHERE c.conversation_no = ? AND u.user_no = ?
	`, message.CreatedAt, message.CreatedAt, message.ConversationID, userID); err != nil {
		return fmt.Errorf("update conversation timestamp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit append message: %w", err)
	}
	return nil
}

func (r *ConversationRepository) ListMessages(
	ctx context.Context,
	userID string,
	conversationID string,
	limit int,
) ([]model.Message, error) {
	if _, err := r.Get(ctx, userID, conversationID); err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT message_no, conversation_no, role, content, trace_id, created_at
		FROM (
			SELECT m.id, m.message_no, c.conversation_no, m.role, m.content, m.trace_id, m.created_at
			FROM messages m
			JOIN conversations c ON c.id = m.conversation_id
			JOIN users u ON u.id = c.user_id
			WHERE c.conversation_no = ? AND u.user_no = ?
			ORDER BY m.id DESC
			LIMIT ?
		) recent
		ORDER BY id ASC
	`, conversationID, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list conversation messages: %w", err)
	}
	defer rows.Close()

	messages := make([]model.Message, 0, limit)
	for rows.Next() {
		var message model.Message
		var traceID sql.NullString
		if err := rows.Scan(
			&message.ID,
			&message.ConversationID,
			&message.Role,
			&message.Content,
			&traceID,
			&message.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan conversation message: %w", err)
		}
		if traceID.Valid {
			message.TraceID = traceID.String
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation messages: %w", err)
	}
	return messages, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
