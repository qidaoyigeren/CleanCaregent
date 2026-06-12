package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"CleanCaregent/internal/trace"
)

var ErrTraceNotFound = errors.New("agent trace not found")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Start(ctx context.Context, value trace.AgentTrace) error {
	planJSON, err := json.Marshal(value.Plan)
	if err != nil {
		return fmt.Errorf("encode trace plan: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO agent_traces (
			trace_id, conversation_id, intent, route_mode, plan_json,
			step_summary_json, status, created_at, updated_at
		)
		SELECT ?, c.id, ?, ?, ?, JSON_ARRAY(), 'running', ?, ?
		FROM conversations c
		WHERE c.conversation_no = ?
	`, value.TraceID, value.Intent, value.RouteMode, planJSON,
		value.StartedAt, value.StartedAt, value.ConversationID)
	if err != nil {
		return fmt.Errorf("start agent trace: %w", err)
	}
	return nil
}

func (s *Store) AppendStep(ctx context.Context, step trace.Step) error {
	raw, err := json.Marshal(step)
	if err != nil {
		return fmt.Errorf("encode trace step: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE agent_traces
		SET step_summary_json = JSON_ARRAY_APPEND(
			COALESCE(step_summary_json, JSON_ARRAY()),
			'$',
			CAST(? AS JSON)
		), updated_at = UTC_TIMESTAMP(6)
		WHERE trace_id = ?
	`, raw, step.TraceID)
	if err != nil {
		return fmt.Errorf("append agent trace step: %w", err)
	}
	return nil
}

func (s *Store) Finish(ctx context.Context, traceID string, result trace.Result) error {
	evidenceIDs, err := json.Marshal(result.EvidenceIDs)
	if err != nil {
		return fmt.Errorf("encode trace evidence ids: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE agent_traces
		SET status = ?, error_code = ?, input_tokens = ?, output_tokens = ?,
			latency_ms = ?, evidence_ids_json = ?, updated_at = ?
		WHERE trace_id = ?
	`, result.Status, nullable(result.ErrorCode), result.InputTokens, result.OutputTokens,
		result.LatencyMS, evidenceIDs, result.FinishedAt, traceID)
	if err != nil {
		return fmt.Errorf("finish agent trace: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, traceID string) (trace.AgentTraceRecord, error) {
	var (
		record       trace.AgentTraceRecord
		planRaw      []byte
		stepsRaw     []byte
		evidenceRaw  []byte
		errorCode    sql.NullString
		conversation sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT
			t.trace_id, c.conversation_no, t.intent, t.route_mode, t.plan_json,
			t.step_summary_json, t.status, t.error_code, t.input_tokens,
			t.output_tokens, t.latency_ms, t.evidence_ids_json, t.created_at, t.updated_at
		FROM agent_traces t
		LEFT JOIN conversations c ON c.id = t.conversation_id
		WHERE t.trace_id = ?
	`, traceID).Scan(
		&record.TraceID,
		&conversation,
		&record.Intent,
		&record.RouteMode,
		&planRaw,
		&stepsRaw,
		&record.Status,
		&errorCode,
		&record.InputTokens,
		&record.OutputTokens,
		&record.LatencyMS,
		&evidenceRaw,
		&record.StartedAt,
		&record.FinishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return trace.AgentTraceRecord{}, ErrTraceNotFound
	}
	if err != nil {
		return trace.AgentTraceRecord{}, fmt.Errorf("get agent trace: %w", err)
	}
	if conversation.Valid {
		record.ConversationID = conversation.String
	}
	if errorCode.Valid {
		record.ErrorCode = errorCode.String
	}
	if len(planRaw) > 0 {
		var plan any
		if err := json.Unmarshal(planRaw, &plan); err != nil {
			return trace.AgentTraceRecord{}, fmt.Errorf("decode trace plan: %w", err)
		}
		record.Plan = plan
	}
	if len(stepsRaw) > 0 {
		if err := json.Unmarshal(stepsRaw, &record.Steps); err != nil {
			return trace.AgentTraceRecord{}, fmt.Errorf("decode trace steps: %w", err)
		}
	}
	if len(evidenceRaw) > 0 {
		if err := json.Unmarshal(evidenceRaw, &record.EvidenceIDs); err != nil {
			return trace.AgentTraceRecord{}, fmt.Errorf("decode trace evidence ids: %w", err)
		}
	}
	toolCalls, err := s.listToolCalls(ctx, traceID)
	if err != nil {
		return trace.AgentTraceRecord{}, err
	}
	record.ToolCalls = toolCalls
	return record, nil
}

// ListByTime returns recent traces in the requested UTC interval.
func (s *Store) ListByTime(
	ctx context.Context,
	from time.Time,
	to time.Time,
	limit int,
) ([]trace.AgentTraceRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT trace_id
		FROM agent_traces
		WHERE created_at >= ? AND created_at < ?
		ORDER BY created_at DESC
		LIMIT ?
	`, from.UTC(), to.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("查询时间段 Trace 失败: %w", err)
	}
	defer rows.Close()
	var traceIDs []string
	for rows.Next() {
		var traceID string
		if err := rows.Scan(&traceID); err != nil {
			return nil, fmt.Errorf("读取 Trace ID 失败: %w", err)
		}
		traceIDs = append(traceIDs, traceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 Trace ID 失败: %w", err)
	}
	result := make([]trace.AgentTraceRecord, 0, len(traceIDs))
	for _, traceID := range traceIDs {
		record, err := s.Get(ctx, traceID)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

// RequestionRate calculates the share of user messages sent within 30 seconds
// after the preceding assistant response.
func (s *Store) RequestionRate(ctx context.Context, from, to time.Time) (float64, error) {
	var (
		requestions int
		followUps   int
	)
	err := s.db.QueryRowContext(ctx, `
		WITH ordered AS (
			SELECT
				role,
				created_at,
				LAG(role) OVER (PARTITION BY conversation_id ORDER BY id) AS previous_role,
				LAG(created_at) OVER (PARTITION BY conversation_id ORDER BY id) AS previous_at
			FROM messages
			WHERE created_at >= ? AND created_at < ?
		)
		SELECT
			COALESCE(SUM(
				CASE WHEN role = 'user' AND previous_role = 'assistant'
					AND TIMESTAMPDIFF(SECOND, previous_at, created_at) <= 30
				THEN 1 ELSE 0 END
			), 0),
			COALESCE(SUM(
				CASE WHEN role = 'user' AND previous_role = 'assistant'
				THEN 1 ELSE 0 END
			), 0)
		FROM ordered
	`, from.UTC(), to.UTC()).Scan(&requestions, &followUps)
	if err != nil {
		return 0, fmt.Errorf("计算用户重新提问率失败: %w", err)
	}
	if followUps == 0 {
		return 0, nil
	}
	return float64(requestions) / float64(followUps), nil
}

func (s *Store) listToolCalls(ctx context.Context, traceID string) ([]trace.ToolCall, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT call_id, tool_name, args_masked_json, result_summary_json,
		       status, error_code, latency_ms, created_at
		FROM tool_call_logs
		WHERE trace_id = ?
		ORDER BY id
	`, traceID)
	if err != nil {
		return nil, fmt.Errorf("list trace tool calls: %w", err)
	}
	defer rows.Close()
	var calls []trace.ToolCall
	for rows.Next() {
		var (
			call      trace.ToolCall
			argsRaw   []byte
			resultRaw []byte
			errorCode sql.NullString
		)
		if err := rows.Scan(
			&call.CallID, &call.ToolName, &argsRaw, &resultRaw,
			&call.Status, &errorCode, &call.LatencyMS, &call.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan trace tool call: %w", err)
		}
		if errorCode.Valid {
			call.ErrorCode = errorCode.String
		}
		if len(argsRaw) > 0 {
			_ = json.Unmarshal(argsRaw, &call.Arguments)
		}
		if len(resultRaw) > 0 {
			_ = json.Unmarshal(resultRaw, &call.ResultSummary)
		}
		calls = append(calls, call)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace tool calls: %w", err)
	}
	return calls, nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
