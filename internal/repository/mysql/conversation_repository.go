package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"CleanCaregent/internal/model"
	"CleanCaregent/internal/repository"

	mysqlDriver "github.com/go-sql-driver/mysql"
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

func (r *ConversationRepository) List(
	ctx context.Context,
	userID string,
	limit int,
) ([]model.Conversation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.conversation_no, u.user_no, c.title, c.status, c.created_at, c.last_message_at
		FROM conversations c
		JOIN users u ON u.id = c.user_id
		WHERE u.user_no = ?
		ORDER BY c.last_message_at DESC, c.id DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	conversations := make([]model.Conversation, 0, limit)
	for rows.Next() {
		var conversation model.Conversation
		if err := rows.Scan(
			&conversation.ID,
			&conversation.UserID,
			&conversation.Title,
			&conversation.Status,
			&conversation.CreatedAt,
			&conversation.LastMessageAt,
		); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conversations = append(conversations, conversation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}
	return conversations, nil
}

func (r *ConversationRepository) AppendMessage(ctx context.Context, userID string, message model.Message) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin append message: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// The user_no predicate is the tenant boundary. A conversation ID alone
	// must never authorize a cross-user message write.
	result, err := tx.ExecContext(ctx, `
		INSERT INTO messages (
			message_no, conversation_id, role, content, trace_id, client_message_id, created_at
		)
		SELECT ?, c.id, ?, ?, ?, ?, ?
		FROM conversations c
		JOIN users u ON u.id = c.user_id
		WHERE c.conversation_no = ? AND u.user_no = ?
	`, message.ID, message.Role, message.Content, nullableString(message.TraceID),
		nullableString(message.ClientMessageID), message.CreatedAt,
		message.ConversationID, userID)
	if err != nil {
		var mysqlErr *mysqlDriver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 && message.ClientMessageID != "" {
			return nil
		}
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
		SELECT message_no, conversation_no, role, content, trace_id, client_message_id, created_at
		FROM (
			SELECT m.id, m.message_no, c.conversation_no, m.role, m.content, m.trace_id, m.client_message_id, m.created_at
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
		var clientMessageID sql.NullString
		if err := rows.Scan(
			&message.ID,
			&message.ConversationID,
			&message.Role,
			&message.Content,
			&traceID,
			&clientMessageID,
			&message.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan conversation message: %w", err)
		}
		if traceID.Valid {
			message.TraceID = traceID.String
		}
		if clientMessageID.Valid {
			message.ClientMessageID = clientMessageID.String
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation messages: %w", err)
	}
	return messages, nil
}

func (r *ConversationRepository) FindMessageByClientMessageID(
	ctx context.Context,
	userID string,
	conversationID string,
	role string,
	clientMessageID string,
) (model.Message, error) {
	var message model.Message
	var traceID sql.NullString
	var storedClientMessageID sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT m.message_no, c.conversation_no, m.role, m.content, m.trace_id, m.client_message_id, m.created_at
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		JOIN users u ON u.id = c.user_id
		WHERE c.conversation_no = ? AND u.user_no = ? AND m.role = ? AND m.client_message_id = ?
		ORDER BY m.id DESC
		LIMIT 1
	`, conversationID, userID, role, clientMessageID).Scan(
		&message.ID,
		&message.ConversationID,
		&message.Role,
		&message.Content,
		&traceID,
		&storedClientMessageID,
		&message.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Message{}, repository.ErrConversationNotFound
	}
	if err != nil {
		return model.Message{}, fmt.Errorf("find message by client id: %w", err)
	}
	if traceID.Valid {
		message.TraceID = traceID.String
	}
	if storedClientMessageID.Valid {
		message.ClientMessageID = storedClientMessageID.String
	}
	return message, nil
}

func (r *ConversationRepository) StartMessageRequest(
	ctx context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
) (bool, error) {
	conversationPK, err := r.conversationPK(ctx, userID, conversationID)
	if err != nil {
		return false, err
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT IGNORE INTO conversation_message_requests (
			conversation_id, client_message_id, status, created_at, updated_at
		) VALUES (?, ?, ?, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))
	`, conversationPK, clientMessageID, repository.MessageRequestRunning)
	if err != nil {
		return false, fmt.Errorf("start message request: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read started message request rows: %w", err)
	}
	return affected > 0, nil
}

func (r *ConversationRepository) GetMessageRequest(
	ctx context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
) (repository.MessageRequest, error) {
	var request repository.MessageRequest
	var assistantID, traceID, errorMessage sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT c.conversation_no, cmr.client_message_id, cmr.status,
		       cmr.assistant_message_no, cmr.trace_id, cmr.error_message,
		       cmr.created_at, cmr.updated_at
		FROM conversation_message_requests cmr
		JOIN conversations c ON c.id = cmr.conversation_id
		JOIN users u ON u.id = c.user_id
		WHERE c.conversation_no = ? AND u.user_no = ? AND cmr.client_message_id = ?
	`, conversationID, userID, clientMessageID).Scan(
		&request.ConversationID,
		&request.ClientMessageID,
		&request.Status,
		&assistantID,
		&traceID,
		&errorMessage,
		&request.CreatedAt,
		&request.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.MessageRequest{}, repository.ErrConversationNotFound
	}
	if err != nil {
		return repository.MessageRequest{}, fmt.Errorf("get message request: %w", err)
	}
	if assistantID.Valid {
		request.AssistantID = assistantID.String
	}
	if traceID.Valid {
		request.TraceID = traceID.String
	}
	if errorMessage.Valid {
		request.ErrorMessage = errorMessage.String
	}
	return request, nil
}

func (r *ConversationRepository) CompleteMessageRequest(
	ctx context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
	assistant model.Message,
) error {
	conversationPK, err := r.conversationPK(ctx, userID, conversationID)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE conversation_message_requests
		SET status = ?, assistant_message_no = ?, trace_id = ?, error_message = NULL, updated_at = UTC_TIMESTAMP(6)
		WHERE conversation_id = ? AND client_message_id = ?
	`, repository.MessageRequestDone, assistant.ID, nullableString(assistant.TraceID), conversationPK, clientMessageID)
	if err != nil {
		return fmt.Errorf("complete message request: %w", err)
	}
	return nil
}

func (r *ConversationRepository) FailMessageRequest(
	ctx context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
	cause string,
) error {
	conversationPK, err := r.conversationPK(ctx, userID, conversationID)
	if err != nil {
		return err
	}
	if len(cause) > 1000 {
		cause = cause[:1000]
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE conversation_message_requests
		SET status = ?, error_message = ?, updated_at = UTC_TIMESTAMP(6)
		WHERE conversation_id = ? AND client_message_id = ?
	`, repository.MessageRequestFailed, cause, conversationPK, clientMessageID)
	if err != nil {
		return fmt.Errorf("fail message request: %w", err)
	}
	return nil
}

func (r *ConversationRepository) conversationPK(ctx context.Context, userID, conversationID string) (int64, error) {
	var conversationPK int64
	err := r.db.QueryRowContext(ctx, `
		SELECT c.id
		FROM conversations c
		JOIN users u ON u.id = c.user_id
		WHERE c.conversation_no = ? AND u.user_no = ?
	`, conversationID, userID).Scan(&conversationPK)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, repository.ErrConversationNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("resolve conversation: %w", err)
	}
	return conversationPK, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
