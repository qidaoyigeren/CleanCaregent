#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_HOST="${E2E_API_HOST:-127.0.0.1}"
API_PORT="${E2E_API_PORT:-18080}"
MCP_HOST="${E2E_MCP_HOST:-127.0.0.1}"
MCP_PORT="${E2E_MCP_PORT:-18090}"
BASE_URL="${BASE_URL:-http://$API_HOST:$API_PORT}"
MCP_URL="${MCP_URL:-http://$MCP_HOST:$MCP_PORT/mcp}"
MCP_API_KEY="${MCP_API_KEY:-e2e-mcp-key}"
LOG_DIR="${E2E_LOG_DIR:-$ROOT_DIR/.e2e}"
START_COMPOSE="${E2E_START_COMPOSE:-true}"

while IFS='=' read -r name _; do
  if [[ "$name" == CLEANCARE_* ]]; then
    unset "$name"
  fi
done < <(env)

export CLEANCARE_MYSQL_PORT="${E2E_MYSQL_PORT:-13306}"
export CLEANCARE_REDIS_PORT="${E2E_REDIS_PORT:-16379}"
export CLEANCARE_QDRANT_PORT="${E2E_QDRANT_PORT:-16333}"
export CLEANCARE_QDRANT_GRPC_PORT="${E2E_QDRANT_GRPC_PORT:-16334}"

MYSQL_DSN="${E2E_MYSQL_DSN:-cleancare:cleancare@tcp(127.0.0.1:$CLEANCARE_MYSQL_PORT)/cleancare?parseTime=true&charset=utf8mb4&loc=UTC&multiStatements=true}"

mkdir -p "$LOG_DIR"
rm -f "$LOG_DIR"/*.log "$LOG_DIR"/*.json
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-cleancare_e2e}"
GO_CMD="${GO_CMD:-go}"
if ! command -v "$GO_CMD" >/dev/null 2>&1; then
  if command -v go.exe >/dev/null 2>&1; then
    GO_CMD="go.exe"
  elif command -v where.exe >/dev/null 2>&1 && command -v cygpath >/dev/null 2>&1; then
    GO_EXE="$(where.exe go 2>/dev/null | head -n 1 | tr -d '\r')"
    if [[ -n "$GO_EXE" ]]; then
      GO_CMD="$(cygpath -u "$GO_EXE")"
    fi
  fi
fi
if ! command -v "$GO_CMD" >/dev/null 2>&1 && [[ ! -x "$GO_CMD" ]]; then
  echo "go command not found; install Go or set GO_CMD=/path/to/go" >&2
  exit 1
fi
GO_OS="$("$GO_CMD" env GOOS 2>/dev/null || true)"
PY_CMD=()
if [[ -n "${PYTHON:-}" ]]; then
  PY_CMD=("$PYTHON")
elif [[ "$GO_OS" == "windows" ]] && command -v py.exe >/dev/null 2>&1; then
  PY_CMD=(py.exe -3)
elif [[ "$GO_OS" == "windows" ]] && command -v python.exe >/dev/null 2>&1; then
  PY_CMD=(python.exe)
elif command -v python3 >/dev/null 2>&1; then
  PY_CMD=(python3)
elif command -v python >/dev/null 2>&1; then
  PY_CMD=(python)
elif command -v py.exe >/dev/null 2>&1; then
  PY_CMD=(py.exe -3)
elif command -v py >/dev/null 2>&1; then
  PY_CMD=(py -3)
else
  echo "python command not found; install Python or set PYTHON=/path/to/python" >&2
  exit 1
fi
CURL_CMD=()
if [[ -n "${CURL:-}" ]]; then
  CURL_CMD=("$CURL")
elif [[ "$GO_OS" == "windows" ]] && command -v curl.exe >/dev/null 2>&1; then
  CURL_CMD=(curl.exe)
else
  CURL_CMD=(curl)
fi

SERVER_PID=""
MCP_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [[ -n "$MCP_PID" ]] && kill -0 "$MCP_PID" 2>/dev/null; then
    kill "$MCP_PID" 2>/dev/null || true
    wait "$MCP_PID" 2>/dev/null || true
  fi
  if [[ "$START_COMPOSE" == "true" ]]; then
    (cd "$ROOT_DIR" && docker compose down -v) >"$LOG_DIR/compose-down.log" 2>&1 || true
  fi
}
trap cleanup EXIT

wait_http() {
  local url="$1"
  local name="$2"
  local timeout_seconds="${3:-120}"
  local deadline=$((SECONDS + timeout_seconds))
  until "${CURL_CMD[@]}" --fail --silent --show-error "$url" >/dev/null; do
    if (( SECONDS >= deadline )); then
      echo "timed out waiting for $name at $url" >&2
      return 1
    fi
    sleep 2
  done
}

wait_http_process() {
  local url="$1"
  local name="$2"
  local pid="$3"
  local log_file="$4"
  local timeout_seconds="${5:-120}"
  local deadline=$((SECONDS + timeout_seconds))
  until "${CURL_CMD[@]}" --fail --silent --show-error "$url" >/dev/null; do
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "$name exited before becoming ready" >&2
      cat "$log_file" >&2 || true
      return 1
    fi
    if (( SECONDS >= deadline )); then
      echo "timed out waiting for $name at $url" >&2
      cat "$log_file" >&2 || true
      return 1
    fi
    sleep 2
  done
}

wait_compose_health() {
  local service="$1"
  local timeout_seconds="${2:-120}"
  local deadline=$((SECONDS + timeout_seconds))
  local container_id=""
  until [[ -n "$container_id" ]]; do
    container_id="$(docker compose ps -q "$service")"
    if (( SECONDS >= deadline )); then
      echo "timed out waiting for $service container" >&2
      return 1
    fi
    sleep 1
  done
  while true; do
    local status
    status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id")"
    if [[ "$status" == "healthy" || "$status" == "running" ]]; then
      return 0
    fi
    if (( SECONDS >= deadline )); then
      echo "timed out waiting for $service health, last status=$status" >&2
      return 1
    fi
    sleep 2
  done
}

run_logged() {
  local name="$1"
  local attempts="$2"
  shift 2
  local log_file="$LOG_DIR/$name.log"
  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if "$@" >"$log_file" 2>&1; then
      return 0
    fi
    if ((attempt == attempts)); then
      cat "$log_file" >&2 || true
      return 1
    fi
    sleep $((attempt * 3))
  done
}

cd "$ROOT_DIR"

CONFIG_FILE="${E2E_CONFIG_FILE:-$LOG_DIR/config.e2e.yaml}"
QDRANT_BASE_URL="${E2E_QDRANT_BASE_URL:-http://127.0.0.1:$CLEANCARE_QDRANT_PORT}"
REDIS_ADDRESS="${E2E_REDIS_ADDRESS:-127.0.0.1:$CLEANCARE_REDIS_PORT}"
cat >"$CONFIG_FILE" <<EOF
app:
  env: e2e
server:
  host: "$API_HOST"
  port: $API_PORT
log:
  level: info
  development: true
auth:
  enabled: false
rate_limit:
  enabled: true
  backend: redis
agent:
  mode: agentic
  planning_mode: auto
  max_steps: 5
  token_budget: 6000
  timeout: "${E2E_AGENT_TIMEOUT:-30s}"
storage:
  conversation_repository: mysql
mysql:
  enabled: true
  dsn: "$MYSQL_DSN"
  auto_migrate: false
redis:
  enabled: true
  address: "$REDIS_ADDRESS"
  ingest_stream_enabled: false
qdrant:
  enabled: true
  base_url: "$QDRANT_BASE_URL"
  ensure_collection: true
  vector_size: 1024
embedding:
  provider: local_hash
  dimension: 1024
reranker:
  provider: local_lexical
llm:
  provider: extractive
prompt:
  enable_llm_components: false
tracing:
  enabled: false
tool:
  timeout: 3s
  data_scope: mock
  mcp:
    transport: http
    endpoint: "$MCP_URL"
    api_key: "$MCP_API_KEY"
    server_api_key: "$MCP_API_KEY"
    listen_host: "$MCP_HOST"
    listen_port: $MCP_PORT
    path: /mcp
EOF
if [[ "$GO_OS" == "windows" ]] && command -v cygpath >/dev/null 2>&1; then
	case "$(uname -s 2>/dev/null || true)" in
		MINGW*|MSYS*|CYGWIN*)
			export CLEANCARE_CONFIG_FILE="$(cygpath -w "$CONFIG_FILE")"
			;;
		*)
			export CLEANCARE_CONFIG_FILE="$CONFIG_FILE"
			;;
	esac
elif [[ "$GO_OS" == "windows" ]] && command -v wslpath >/dev/null 2>&1; then
	export CLEANCARE_CONFIG_FILE="$CONFIG_FILE"
	case ":${WSLENV:-}:" in
		*:CLEANCARE_CONFIG_FILE/p:*) ;;
		*) export WSLENV="${WSLENV:+$WSLENV:}CLEANCARE_CONFIG_FILE/p" ;;
	esac
	case ":${WSLENV:-}:" in
		*:LOG_DIR/p:*) ;;
		*) export WSLENV="${WSLENV:+$WSLENV:}LOG_DIR/p" ;;
	esac
	case ":${WSLENV:-}:" in
		*:BASE_URL:*) ;;
		*) export WSLENV="${WSLENV:+$WSLENV:}BASE_URL" ;;
	esac
else
	export CLEANCARE_CONFIG_FILE="$CONFIG_FILE"
fi
export CLEANCARE_QDRANT_BASE_URL="$QDRANT_BASE_URL"
export BASE_URL LOG_DIR
{
  echo "GO_CMD=$GO_CMD"
  echo "PY_CMD=${PY_CMD[*]}"
  echo "CURL_CMD=${CURL_CMD[*]}"
  echo "CLEANCARE_CONFIG_FILE=$CLEANCARE_CONFIG_FILE"
  echo "WSLENV=${WSLENV:-}"
  echo "QDRANT_BASE_URL=$QDRANT_BASE_URL"
  echo "REDIS_ADDRESS=$REDIS_ADDRESS"
  echo "MYSQL_DSN=$MYSQL_DSN"
  "$GO_CMD" env GOOS GOARCH GOEXE GOROOT GOPATH 2>/dev/null || true
} >"$LOG_DIR/env.log"

if [[ "$START_COMPOSE" == "true" ]]; then
  if ! docker compose up -d mysql redis qdrant >"$LOG_DIR/compose-up.log" 2>&1; then
    cat "$LOG_DIR/compose-up.log" >&2 || true
    echo "docker compose up failed; if local ports are already occupied, run with E2E_START_COMPOSE=false against existing mysql/redis/qdrant services" >&2
    exit 1
  fi
  wait_compose_health mysql 180
  wait_compose_health redis 120
fi

wait_http "$QDRANT_BASE_URL/healthz" "qdrant" 120

run_logged migrate 1 "$GO_CMD" run ./cmd/migrate
run_logged seed 1 "$GO_CMD" run ./cmd/seed
run_logged kb-seed 3 "$GO_CMD" run ./cmd/kb-seed

BIN_DIR="${E2E_BIN_DIR:-.e2e/bin}"
mkdir -p "$BIN_DIR"
GO_EXE="$("$GO_CMD" env GOEXE 2>/dev/null || true)"
MCP_BIN="$BIN_DIR/cleancare-mcp-server$GO_EXE"
SERVER_BIN="$BIN_DIR/cleancare-server$GO_EXE"
run_logged build-mcp-server 1 "$GO_CMD" build -o "$MCP_BIN" ./cmd/mcp-server
run_logged build-server 1 "$GO_CMD" build -o "$SERVER_BIN" ./cmd/server

"$MCP_BIN" >"$LOG_DIR/mcp-server.log" 2>&1 &
MCP_PID="$!"
wait_http_process "http://$MCP_HOST:$MCP_PORT/health/live" "mcp server" "$MCP_PID" "$LOG_DIR/mcp-server.log" 120

"$SERVER_BIN" >"$LOG_DIR/server.log" 2>&1 &
SERVER_PID="$!"
wait_http_process "$BASE_URL/health/ready" "api server" "$SERVER_PID" "$LOG_DIR/server.log" 180

BASE_URL="$BASE_URL" LOG_DIR="$LOG_DIR" "${PY_CMD[@]}" - <<'PY'
import json
import os
import time
import urllib.error
import urllib.request

base_url = os.environ["BASE_URL"].rstrip("/")
log_dir = os.environ["LOG_DIR"]

def request(method, path, payload=None):
    data = None
    headers = {"Content-Type": "application/json"}
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(base_url + path, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = resp.read().decode("utf-8")
            return resp.status, json.loads(body)
    except urllib.error.HTTPError as err:
        body = err.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"{method} {path} returned {err.code}: {body}") from err

status, created = request("POST", "/api/v1/conversations", {"title": "e2e mcp tool trace"})
if status != 201 or created.get("code") != "OK":
    raise RuntimeError(f"create conversation failed: {created}")
conversation_id = created["data"]["conversation_id"]

status, asked = request("POST", f"/api/v1/conversations/{conversation_id}/messages", {
    "content": "T20 \u73b0\u5728\u591a\u5c11\u94b1\uff1f\u8bf7\u7ed9\u51fa\u5f53\u524d\u4ef7\u683c\u548c\u4f18\u60e0\u4fe1\u606f\u3002"
})
if status != 200 or asked.get("code") != "OK":
    raise RuntimeError(f"ask failed: {asked}")
trace_id = asked["data"].get("trace_id")
answer = asked["data"].get("answer", "")
if not trace_id:
    raise RuntimeError(f"missing trace_id in ask response: {asked}")
if not answer:
    raise RuntimeError(f"missing answer in ask response: {asked}")

last_trace = None
for _ in range(30):
    status, trace = request("GET", f"/api/v1/admin/traces/{trace_id}")
    last_trace = trace
    if status == 200 and trace.get("code") == "OK":
        record = trace["data"]
        tool_calls = record.get("tool_calls") or []
        if record.get("status") == "success" and tool_calls:
            break
    time.sleep(1)
else:
    raise RuntimeError(f"trace did not finish with tool calls: {last_trace}")

record = last_trace["data"]
tool_calls = record.get("tool_calls") or []
price_calls = [call for call in tool_calls if call.get("tool_name") == "price_query"]
if not price_calls:
    raise RuntimeError(f"price_query was not recorded in tool_calls: {tool_calls}")
if price_calls[0].get("status") != "success":
    raise RuntimeError(f"price_query did not succeed: {price_calls[0]}")
if record.get("latency_ms", 0) <= 0:
    raise RuntimeError(f"trace latency was not recorded: {record}")

snapshot = {
    "conversation_id": conversation_id,
    "trace_id": trace_id,
    "answer": answer,
    "trace_status": record.get("status"),
    "trace_latency_ms": record.get("latency_ms"),
    "tool_calls": tool_calls,
}
with open(os.path.join(log_dir, "e2e-result.json"), "w", encoding="utf-8") as f:
    json.dump(snapshot, f, ensure_ascii=False, indent=2)
print(json.dumps(snapshot, ensure_ascii=False, indent=2))
PY

if [[ "${E2E_RUN_EVAL:-false}" == "true" ]]; then
  EVAL_JSON_OUTPUT="$LOG_DIR/eval-regression-result.json"
  if [[ "$GO_OS" == "windows" ]] && command -v wslpath >/dev/null 2>&1; then
    EVAL_JSON_OUTPUT="$(wslpath -w "$EVAL_JSON_OUTPUT")"
  fi
  "${PY_CMD[@]}" scripts/eval-regression-report.py \
    --base-url "$BASE_URL" \
    --system-version "${E2E_EVAL_SYSTEM_VERSION:-agentic-mcp-http-e2e}" \
    --max-cases "${E2E_EVAL_MAX_CASES:-200}" \
    --output "${E2E_EVAL_OUTPUT:-docs/eval/mcp-regression-report.md}" \
    --json-output "$EVAL_JSON_OUTPUT" \
    --baseline-label "${E2E_EVAL_BASELINE_LABEL:-2026-06-13 real-model eval_67abf7a0bdfe419fbdf92a68}" \
    --baseline-pass-rate "${E2E_EVAL_BASELINE_PASS_RATE:-0.715}" \
    --baseline-p95-latency-ms "${E2E_EVAL_BASELINE_P95_LATENCY_MS:-17077}" \
    --baseline-average-tokens "${E2E_EVAL_BASELINE_AVERAGE_TOKENS:-4007}"
fi
