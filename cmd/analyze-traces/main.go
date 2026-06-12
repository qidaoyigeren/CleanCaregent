package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"CleanCaregent/internal/config"
	mysqlclient "CleanCaregent/internal/platform/mysql"
	"CleanCaregent/internal/trace"
	tracemysql "CleanCaregent/internal/trace/mysql"
)

func main() {
	var (
		configPath = flag.String("config", "", "配置文件路径")
		traceID    = flag.String("trace-id", "", "单个 trace_id")
		fromRaw    = flag.String("from", "", "开始时间，RFC3339")
		toRaw      = flag.String("to", "", "结束时间，RFC3339")
		limit      = flag.Int("limit", 100, "时间段最大返回数量")
	)
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		exitError("加载配置失败", err)
	}
	if !cfg.MySQL.Enabled {
		exitError("MySQL 未启用", nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := mysqlclient.Open(ctx, cfg.MySQL)
	if err != nil {
		exitError("连接 MySQL 失败", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "关闭 MySQL 失败: %v\n", closeErr)
		}
	}()
	store := tracemysql.NewStore(db)
	if strings.TrimSpace(*traceID) != "" {
		record, err := store.Get(ctx, strings.TrimSpace(*traceID))
		if err != nil {
			exitError("查询 Trace 失败", err)
		}
		fmt.Print(formatTrace(record))
		return
	}
	from, to, err := parseRange(*fromRaw, *toRaw)
	if err != nil {
		exitError("时间范围无效", err)
	}
	records, err := store.ListByTime(ctx, from, to, *limit)
	if err != nil {
		exitError("查询时间段 Trace 失败", err)
	}
	rate, err := store.RequestionRate(ctx, from, to)
	if err != nil {
		exitError("计算重新提问率失败", err)
	}
	fmt.Printf("时间范围: %s 至 %s\nTrace 数量: %d\n30秒内重新提问率: %.2f%%\n\n",
		from.Format(time.RFC3339), to.Format(time.RFC3339), len(records), rate*100)
	for _, record := range records {
		fmt.Print(formatTrace(record))
	}
}

func parseRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	if strings.TrimSpace(fromRaw) == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("必须提供 trace-id 或 from")
	}
	from, err := time.Parse(time.RFC3339, fromRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to := time.Now().UTC()
	if strings.TrimSpace(toRaw) != "" {
		to, err = time.Parse(time.RFC3339, toRaw)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("to 必须晚于 from")
	}
	return from.UTC(), to.UTC(), nil
}

func formatTrace(record trace.AgentTraceRecord) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "=== Trace %s | intent=%s | mode=%s | status=%s | latency=%dms ===\n",
		record.TraceID, record.Intent, record.RouteMode, record.Status, record.LatencyMS)
	for index, step := range record.Steps {
		marker := "正常"
		if abnormalStep(step) {
			marker = "异常"
		}
		raw, err := json.Marshal(step.Metadata)
		if err != nil {
			raw = []byte(`{"metadata_error":"编码失败"}`)
		}
		fmt.Fprintf(&builder, "%02d. [%s] %s/%s %dms\n    Observation: %s\n",
			index+1, marker, step.Type, step.Status, step.DurationMS, raw)
	}
	for _, call := range record.ToolCalls {
		fmt.Fprintf(&builder, "    Tool: %s status=%s error=%s latency=%dms\n",
			call.ToolName, call.Status, call.ErrorCode, call.LatencyMS)
	}
	builder.WriteByte('\n')
	return builder.String()
}

func abnormalStep(step trace.Step) bool {
	if step.Status == "failed" || step.Status == "blocked" {
		return true
	}
	if value, ok := step.Metadata["redundant_information"].(bool); ok && value {
		return true
	}
	if _, ok := step.Metadata["strategy"]; ok {
		return true
	}
	return strings.Contains(strings.ToLower(step.Type), "rerun")
}

func exitError(message string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	} else {
		fmt.Fprintln(os.Stderr, message)
	}
	os.Exit(1)
}
