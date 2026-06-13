package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"CleanCaregent/internal/config"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func Open(ctx context.Context, cfg config.MySQLConfig) (*sql.DB, error) {
	dsn, err := utcSessionDSN(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("normalize mysql dsn: %w", err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, cfg.PingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return db, nil
}

func utcSessionDSN(dsn string) (string, error) {
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	if parsed.Params == nil {
		parsed.Params = make(map[string]string)
	}
	if _, configured := parsed.Params["time_zone"]; !configured {
		parsed.Params["time_zone"] = "'+00:00'"
	}
	if parsed.Loc == nil {
		parsed.Loc = time.UTC
	}
	return parsed.FormatDSN(), nil
}
