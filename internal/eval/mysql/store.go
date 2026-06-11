package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"CleanCaregent/internal/eval"
)

var ErrRunNotFound = errors.New("eval run not found")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) UpsertCases(ctx context.Context, version string, cases []eval.Case) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert eval cases: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO eval_cases (
			case_id, query, intent, difficulty, expected_docs_json, expected_tools_json,
			expected_tool_params_json, standard_answer, should_clarify, should_reject,
			expected_evidence_ids_json, tags_json, version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			query = VALUES(query), intent = VALUES(intent), difficulty = VALUES(difficulty),
			expected_docs_json = VALUES(expected_docs_json),
			expected_tools_json = VALUES(expected_tools_json),
			expected_tool_params_json = VALUES(expected_tool_params_json),
			standard_answer = VALUES(standard_answer),
			should_clarify = VALUES(should_clarify),
			should_reject = VALUES(should_reject),
			expected_evidence_ids_json = VALUES(expected_evidence_ids_json),
			tags_json = VALUES(tags_json), updated_at = UTC_TIMESTAMP(6)
	`)
	if err != nil {
		return fmt.Errorf("prepare eval cases: %w", err)
	}
	defer statement.Close()
	for _, item := range cases {
		expectedDocs, _ := json.Marshal(item.ExpectedDocuments)
		expectedTools, _ := json.Marshal(item.ExpectedTools)
		expectedParams, _ := json.Marshal(item.ExpectedToolParams)
		expectedEvidence, _ := json.Marshal(item.ExpectedEvidenceIDs)
		tags, _ := json.Marshal(item.Tags)
		if _, err := statement.ExecContext(ctx,
			item.CaseID, item.Query, item.Intent, item.Difficulty,
			expectedDocs, expectedTools, expectedParams, item.StandardAnswer,
			item.ShouldClarify, item.ShouldReject, expectedEvidence, tags, version,
		); err != nil {
			return fmt.Errorf("upsert eval case %s: %w", item.CaseID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit eval cases: %w", err)
	}
	return nil
}

func (s *Store) CreateRun(ctx context.Context, run eval.Run) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO eval_runs (
			run_no, dataset_version, system_version, model_config_json,
			status, started_at, created_at
		) VALUES (?, ?, ?, JSON_OBJECT(), ?, ?, UTC_TIMESTAMP(6))
	`, run.RunNo, run.DatasetVersion, run.SystemVersion, run.Status, run.StartedAt)
	if err != nil {
		return fmt.Errorf("create eval run: %w", err)
	}
	return nil
}

func (s *Store) SaveResult(
	ctx context.Context,
	runNo, datasetVersion string,
	result eval.CaseResult,
) error {
	tools, _ := json.Marshal(result.ActualTools)
	metrics, _ := json.Marshal(result.Metrics)
	execResult, err := s.db.ExecContext(ctx, `
		INSERT INTO eval_results (
			run_id, case_id, trace_id, actual_intent, actual_tools_json,
			answer, metrics_json, passed, error_type, latency_ms, token_count
		)
		SELECT r.id, c.id, ?, ?, ?, ?, ?, ?, ?, ?, ?
		FROM eval_runs r
		JOIN eval_cases c ON c.case_id = ? AND c.version = ?
		WHERE r.run_no = ?
		ON DUPLICATE KEY UPDATE
			trace_id = VALUES(trace_id), actual_intent = VALUES(actual_intent),
			actual_tools_json = VALUES(actual_tools_json), answer = VALUES(answer),
			metrics_json = VALUES(metrics_json), passed = VALUES(passed),
			error_type = VALUES(error_type), latency_ms = VALUES(latency_ms),
			token_count = VALUES(token_count)
	`, nullable(result.TraceID), result.ActualIntent, tools, nullable(result.Answer), metrics,
		result.Passed, nullable(result.ErrorType), result.LatencyMS, result.TokenCount,
		result.CaseID, datasetVersion, runNo)
	if err != nil {
		return fmt.Errorf("save eval result %s: %w", result.CaseID, err)
	}
	affected, err := execResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("read eval result rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("eval run or case not found for %s", result.CaseID)
	}
	return nil
}

func (s *Store) FinishRun(
	ctx context.Context,
	runNo, status string,
	summary map[string]any,
	finishedAt time.Time,
) error {
	raw, _ := json.Marshal(summary)
	result, err := s.db.ExecContext(ctx, `
		UPDATE eval_runs
		SET status = ?, summary_json = ?, finished_at = ?
		WHERE run_no = ?
	`, status, raw, finishedAt, runNo)
	if err != nil {
		return fmt.Errorf("finish eval run: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrRunNotFound
	}
	return nil
}

func (s *Store) GetRun(ctx context.Context, runNo string, includeFailures bool) (eval.Run, error) {
	var (
		run        eval.Run
		startedAt  sql.NullTime
		finishedAt sql.NullTime
		summaryRaw []byte
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT run_no, dataset_version, system_version, status, started_at, finished_at, summary_json
		FROM eval_runs
		WHERE run_no = ?
	`, runNo).Scan(
		&run.RunNo, &run.DatasetVersion, &run.SystemVersion, &run.Status,
		&startedAt, &finishedAt, &summaryRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return eval.Run{}, ErrRunNotFound
	}
	if err != nil {
		return eval.Run{}, fmt.Errorf("get eval run: %w", err)
	}
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		run.FinishedAt = &finishedAt.Time
	}
	if len(summaryRaw) > 0 {
		_ = json.Unmarshal(summaryRaw, &run.Summary)
	}
	if !includeFailures {
		return run, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.case_id, er.trace_id, er.actual_intent, er.actual_tools_json,
		       er.answer, er.metrics_json, er.passed, er.error_type,
		       er.latency_ms, er.token_count
		FROM eval_results er
		JOIN eval_runs r ON r.id = er.run_id
		JOIN eval_cases c ON c.id = er.case_id
		WHERE r.run_no = ? AND er.passed = FALSE
		ORDER BY er.id
	`, runNo)
	if err != nil {
		return eval.Run{}, fmt.Errorf("list eval failures: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			item       eval.CaseResult
			traceID    sql.NullString
			answer     sql.NullString
			errorType  sql.NullString
			toolsRaw   []byte
			metricsRaw []byte
		)
		if err := rows.Scan(
			&item.CaseID, &traceID, &item.ActualIntent, &toolsRaw, &answer,
			&metricsRaw, &item.Passed, &errorType, &item.LatencyMS, &item.TokenCount,
		); err != nil {
			return eval.Run{}, fmt.Errorf("scan eval failure: %w", err)
		}
		if traceID.Valid {
			item.TraceID = traceID.String
		}
		if answer.Valid {
			item.Answer = answer.String
		}
		if errorType.Valid {
			item.ErrorType = errorType.String
		}
		_ = json.Unmarshal(toolsRaw, &item.ActualTools)
		_ = json.Unmarshal(metricsRaw, &item.Metrics)
		run.Results = append(run.Results, item)
	}
	return run, rows.Err()
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
