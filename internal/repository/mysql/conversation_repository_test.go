package mysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"CleanCaregent/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetConversation(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()
	repo := NewConversationRepository(db)
	now := time.Now().UTC()

	mock.ExpectQuery("SELECT c\\.conversation_no").
		WithArgs("cv_test", "user_test").
		WillReturnRows(sqlmock.NewRows([]string{
			"conversation_no", "user_no", "title", "status", "created_at", "last_message_at",
		}).AddRow("cv_test", "user_test", "title", "active", now, now))

	conversation, err := repo.Get(context.Background(), "user_test", "cv_test")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if conversation.ID != "cv_test" || conversation.UserID != "user_test" {
		t.Fatalf("conversation = %#v", conversation)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetConversationMapsMissingToDomainError(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()
	repo := NewConversationRepository(db)

	mock.ExpectQuery("SELECT c\\.conversation_no").
		WithArgs("missing", "user_test").
		WillReturnError(sql.ErrNoRows)

	_, err := repo.Get(context.Background(), "user_test", "missing")
	if !errors.Is(err, repository.ErrConversationNotFound) {
		t.Fatalf("Get() error = %v", err)
	}
}

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	return db, mock
}
