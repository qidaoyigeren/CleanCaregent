package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func Up(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) NOT NULL PRIMARY KEY,
			checksum CHAR(64) NOT NULL,
			applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(names)

	for _, name := range names {
		content, err := migrationFiles.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		checksum := checksum(content)

		var appliedChecksum string
		err = db.QueryRowContext(ctx,
			"SELECT checksum FROM schema_migrations WHERE version = ?",
			name,
		).Scan(&appliedChecksum)
		switch {
		case err == nil:
			if appliedChecksum != checksum {
				return fmt.Errorf("migration %s checksum changed after being applied", name)
			}
			continue
		case err != sql.ErrNoRows:
			return fmt.Errorf("read migration state %s: %w", name, err)
		}

		if err := executeSQLScript(ctx, db, string(content)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := db.ExecContext(ctx,
			"INSERT INTO schema_migrations (version, checksum) VALUES (?, ?)",
			name,
			checksum,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}
	return nil
}

func executeSQLScript(ctx context.Context, db *sql.DB, script string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer conn.Close()
	for _, statement := range splitSQLStatements(script) {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("%s: %w", statementPreview(statement), err)
		}
	}
	return nil
}

func statementPreview(statement string) string {
	statement = strings.Join(strings.Fields(statement), " ")
	if len(statement) > 180 {
		return statement[:180] + "..."
	}
	return statement
}

func splitSQLStatements(script string) []string {
	var statements []string
	var builder []rune
	var quote rune
	escaped := false
	for _, current := range []rune(script) {
		if quote != 0 {
			builder = append(builder, current)
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		switch current {
		case '\'', '"', '`':
			quote = current
			builder = append(builder, current)
		case ';':
			statement := strings.TrimSpace(string(builder))
			if statement != "" {
				statements = append(statements, statement)
			}
			builder = builder[:0]
		default:
			builder = append(builder, current)
		}
	}
	statement := strings.TrimSpace(string(builder))
	if statement != "" {
		statements = append(statements, statement)
	}
	return statements
}

func checksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
