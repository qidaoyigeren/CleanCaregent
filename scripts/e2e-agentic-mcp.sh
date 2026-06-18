#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_HOST="${E2E_API_HOST:-127.0.0.1}"
API_PORT="${E2E_API_PORT:-18080}"
MCP_HOST="${E2E_MCP_HOST:-127.0.0.1}"
MCP_PORT="${E2E_MCP_PORT:-18090}"
MCP_SECONDARY_PORT="${E2E_MCP_SECONDARY_PORT:-$((MCP_PORT + 1))}"
BASE_URL="${BASE_URL:-http://$API_HOST:$API_PORT}"
MCP_URL="${MCP_URL:-http://$MCP_HOST:$MCP_PORT/mcp}"
MCP_SECONDARY_URL="${MCP_SECONDARY_URL:-http://$MCP_HOST:$MCP_SECONDARY_PORT/mcp}"
MCP_API_KEY="${MCP_API_KEY:-e2e-mcp-key}"
LOG_DIR="${E2E_LOG_DIR:-$ROOT_DIR/.e2e}"
START_COMPOSE="${E2E_START_COMPOSE:-true}"
MCP_MODE="${E2E_MCP_MODE:-http}"
REAL_LLM="${E2E_REAL_LLM:-false}"
FRONTEND_E2E="${E2E_FRONTEND:-false}"
FRONTEND_HOST="${E2E_FRONTEND_HOST:-127.0.0.1}"
FRONTEND_PORT="${E2E_FRONTEND_PORT:-15173}"
FRONTEND_URL="${E2E_FRONTEND_URL:-http://$FRONTEND_HOST:$FRONTEND_PORT}"
LLM_API_KEY="${E2E_LLM_API_KEY:-${CLEANCARE_LLM_API_KEY:-}}"
EMBEDDING_API_KEY="${E2E_EMBEDDING_API_KEY:-${CLEANCARE_EMBEDDING_API_KEY:-}}"
RERANKER_API_KEY="${E2E_RERANKER_API_KEY:-${CLEANCARE_RERANKER_API_KEY:-}}"

while IFS='=' read -r name _; do
  if [[ "$name" == CLEANCARE_* ]]; then
    unset "$name"
  fi
done < <(env)

if [[ "$REAL_LLM" == "true" ]]; then
  if [[ -z "$LLM_API_KEY" ]]; then
    echo "E2E_REAL_LLM=true requires CLEANCARE_LLM_API_KEY or E2E_LLM_API_KEY" >&2
    exit 1
  fi
  if [[ -z "$EMBEDDING_API_KEY" ]]; then
    echo "E2E_REAL_LLM=true requires CLEANCARE_EMBEDDING_API_KEY or E2E_EMBEDDING_API_KEY for real retrieval" >&2
    exit 1
  fi
  export CLEANCARE_LLM_API_KEY="$LLM_API_KEY"
  export CLEANCARE_EMBEDDING_API_KEY="$EMBEDDING_API_KEY"
  if [[ -n "$RERANKER_API_KEY" ]]; then
    export CLEANCARE_RERANKER_API_KEY="$RERANKER_API_KEY"
  fi
fi

MYSQL_HOST="${E2E_MYSQL_HOST:-127.0.0.1}"
export CLEANCARE_MYSQL_PORT="${E2E_MYSQL_PORT:-13306}"
export CLEANCARE_REDIS_PORT="${E2E_REDIS_PORT:-16379}"
export CLEANCARE_QDRANT_PORT="${E2E_QDRANT_PORT:-16333}"
export CLEANCARE_QDRANT_GRPC_PORT="${E2E_QDRANT_GRPC_PORT:-16334}"

MYSQL_DSN="${E2E_MYSQL_DSN:-cleancare:cleancare@tcp($MYSQL_HOST:$CLEANCARE_MYSQL_PORT)/cleancare?parseTime=true&charset=utf8mb4&loc=UTC&multiStatements=true}"

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
export PYTHONUTF8=1
export PYTHONIOENCODING=utf-8
CURL_CMD=()
if [[ -n "${CURL:-}" ]]; then
  CURL_CMD=("$CURL")
elif [[ "$GO_OS" == "windows" ]] && command -v curl.exe >/dev/null 2>&1; then
  CURL_CMD=(curl.exe)
else
  CURL_CMD=(curl)
fi
NPM_CMD=()
NODE_CMD=()
if [[ "$FRONTEND_E2E" == "true" ]]; then
  if [[ -n "${NODE:-}" ]]; then
    NODE_CMD=("$NODE")
  elif command -v node >/dev/null 2>&1; then
    NODE_CMD=(node)
  elif [[ "$GO_OS" == "windows" ]] && command -v node.exe >/dev/null 2>&1; then
    NODE_CMD=(node.exe)
  else
    echo "E2E_FRONTEND=true requires node" >&2
    exit 1
  fi
  if [[ -n "${NPM:-}" ]]; then
    NPM_CMD=("$NPM")
  elif command -v npm >/dev/null 2>&1; then
    NPM_CMD=(npm)
  elif [[ "$GO_OS" == "windows" ]] && command -v npm.cmd >/dev/null 2>&1; then
    NPM_CMD=(npm.cmd)
  else
    echo "E2E_FRONTEND=true requires npm" >&2
    exit 1
  fi
fi

SERVER_PID=""
MCP_PID=""
MCP_SECONDARY_PID=""
FRONTEND_PID=""

stop_frontend_port_processes() {
  if [[ "$FRONTEND_E2E" != "true" ]]; then
    return 0
  fi
  if [[ "${GO_OS:-}" == "windows" ]] && command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -Command "\$ErrorActionPreference = 'SilentlyContinue'; Get-NetTCPConnection -LocalPort $FRONTEND_PORT | Select-Object -ExpandProperty OwningProcess -Unique | ForEach-Object { Stop-Process -Id \$_ -Force }" >/dev/null 2>&1 || true
  fi
}

cleanup() {
  if [[ -n "$FRONTEND_PID" ]] && kill -0 "$FRONTEND_PID" 2>/dev/null; then
    kill "$FRONTEND_PID" 2>/dev/null || true
    wait "$FRONTEND_PID" 2>/dev/null || true
  fi
  stop_frontend_port_processes
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [[ -n "$MCP_PID" ]] && kill -0 "$MCP_PID" 2>/dev/null; then
    kill "$MCP_PID" 2>/dev/null || true
    wait "$MCP_PID" 2>/dev/null || true
  fi
  if [[ -n "$MCP_SECONDARY_PID" ]] && kill -0 "$MCP_SECONDARY_PID" 2>/dev/null; then
    kill "$MCP_SECONDARY_PID" 2>/dev/null || true
    wait "$MCP_SECONDARY_PID" 2>/dev/null || true
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

wait_tcp() {
  local host="$1"
  local port="$2"
  local name="$3"
  local timeout_seconds="${4:-120}"
  local deadline=$((SECONDS + timeout_seconds))
  if [[ "${GO_OS:-}" == "windows" ]] && command -v powershell.exe >/dev/null 2>&1; then
    until powershell.exe -NoProfile -Command "if (Test-NetConnection -ComputerName '$host' -Port $port -InformationLevel Quiet) { exit 0 } else { exit 1 }" >/dev/null 2>&1; do
      if (( SECONDS >= deadline )); then
        echo "timed out waiting for $name at $host:$port; docker compose no longer starts MySQL, set E2E_MYSQL_DSN or start an external MySQL instance" >&2
        return 1
      fi
      sleep 2
    done
    return 0
  fi
  until (echo >"/dev/tcp/$host/$port") >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then
      echo "timed out waiting for $name at $host:$port; docker compose no longer starts MySQL, set E2E_MYSQL_DSN or start an external MySQL instance" >&2
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
case "$MCP_MODE" in
  http)
    MCP_CLIENT_CONFIG="$(cat <<EOF
    transport: http
    endpoint: "$MCP_URL"
    api_key: "$MCP_API_KEY"
    server_api_key: "$MCP_API_KEY"
    listen_host: "$MCP_HOST"
    listen_port: $MCP_PORT
    path: /mcp
EOF
)"
    ;;
  aggregate_http)
    MCP_CLIENT_CONFIG="$(cat <<EOF
    transport: http
    server_api_key: "$MCP_API_KEY"
    listen_host: "$MCP_HOST"
    listen_port: $MCP_PORT
    path: /mcp
    servers:
      - name: primary
        transport: http
        endpoint: "$MCP_URL"
        api_key: "$MCP_API_KEY"
      - name: secondary
        transport: http
        endpoint: "$MCP_SECONDARY_URL"
        api_key: "$MCP_API_KEY"
EOF
)"
    ;;
  *)
    echo "unsupported E2E_MCP_MODE=$MCP_MODE (expected http or aggregate_http)" >&2
    exit 1
    ;;
esac
if [[ "$REAL_LLM" == "true" ]]; then
  EMBEDDING_CONFIG="$(cat <<EOF
embedding:
  provider: openai_compatible
  endpoint: "${E2E_EMBEDDING_ENDPOINT:-https://api.siliconflow.cn/v1/embeddings}"
  api_key: ""
  model: "${E2E_EMBEDDING_MODEL:-BAAI/bge-large-zh-v1.5}"
  dimension: ${E2E_EMBEDDING_DIMENSION:-1024}
  request_timeout: "${E2E_EMBEDDING_TIMEOUT:-15s}"
  batch_size: ${E2E_EMBEDDING_BATCH_SIZE:-16}
EOF
)"
  RERANKER_CONFIG="$(cat <<EOF
reranker:
  provider: openai_compatible
  endpoint: "${E2E_RERANKER_ENDPOINT:-https://api.siliconflow.cn/v1/rerank}"
  api_key: ""
  model: "${E2E_RERANKER_MODEL:-BAAI/bge-reranker-v2-m3}"
  request_timeout: "${E2E_RERANKER_TIMEOUT:-10s}"
EOF
)"
  LLM_CONFIG="$(cat <<EOF
llm:
  provider: openai_compatible
  endpoint: "${E2E_LLM_ENDPOINT:-https://api.deepseek.com/v1/chat/completions}"
  api_key: ""
  model: "${E2E_LLM_MODEL:-deepseek-chat}"
  request_timeout: "${E2E_LLM_TIMEOUT:-60s}"
  first_token_timeout: "${E2E_LLM_FIRST_TOKEN_TIMEOUT:-10s}"
  max_tokens: ${E2E_LLM_MAX_TOKENS:-2048}
  temperature: ${E2E_LLM_TEMPERATURE:-0.1}
EOF
)"
  PROMPT_CONFIG="$(cat <<EOF
prompt:
  enable_llm_components: true
EOF
)"
else
  EMBEDDING_CONFIG="$(cat <<EOF
embedding:
  provider: local_hash
  dimension: 1024
EOF
)"
  RERANKER_CONFIG="$(cat <<EOF
reranker:
  provider: local_lexical
EOF
)"
  LLM_CONFIG="$(cat <<EOF
llm:
  provider: extractive
EOF
)"
  PROMPT_CONFIG="$(cat <<EOF
prompt:
  enable_llm_components: false
EOF
)"
fi
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
$EMBEDDING_CONFIG
$RERANKER_CONFIG
$LLM_CONFIG
$PROMPT_CONFIG
tracing:
  enabled: false
tool:
  timeout: 3s
  data_scope: mock
  mcp:
$MCP_CLIENT_CONFIG
EOF
MCP_SECONDARY_CONFIG_FILE="$LOG_DIR/config.mcp-secondary.yaml"
MCP_SECONDARY_RUNTIME_CONFIG_FILE="$MCP_SECONDARY_CONFIG_FILE"
if [[ "$MCP_MODE" == "aggregate_http" ]]; then
  awk -v port="$MCP_SECONDARY_PORT" '
    /^[[:space:]]*listen_port:/ && !done {
      sub(/listen_port:.*/, "listen_port: " port)
      done = 1
    }
    { print }
  ' "$CONFIG_FILE" >"$MCP_SECONDARY_CONFIG_FILE"
fi
if [[ "$GO_OS" == "windows" ]] && command -v cygpath >/dev/null 2>&1; then
	case "$(uname -s 2>/dev/null || true)" in
		MINGW*|MSYS*|CYGWIN*)
			export CLEANCARE_CONFIG_FILE="$(cygpath -w "$CONFIG_FILE")"
			if [[ "$MCP_MODE" == "aggregate_http" ]]; then
				MCP_SECONDARY_RUNTIME_CONFIG_FILE="$(cygpath -w "$MCP_SECONDARY_CONFIG_FILE")"
			fi
			;;
		*)
			export CLEANCARE_CONFIG_FILE="$CONFIG_FILE"
			;;
	esac
elif [[ "$GO_OS" == "windows" ]] && command -v wslpath >/dev/null 2>&1; then
	export CLEANCARE_CONFIG_FILE="$CONFIG_FILE"
	MCP_SECONDARY_RUNTIME_CONFIG_FILE="$MCP_SECONDARY_CONFIG_FILE"
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
	case ":${WSLENV:-}:" in
		*:MCP_URL:*) ;;
		*) export WSLENV="${WSLENV:+$WSLENV:}MCP_URL" ;;
	esac
	case ":${WSLENV:-}:" in
		*:MCP_SECONDARY_URL:*) ;;
		*) export WSLENV="${WSLENV:+$WSLENV:}MCP_SECONDARY_URL" ;;
	esac
	case ":${WSLENV:-}:" in
		*:MCP_MODE:*) ;;
		*) export WSLENV="${WSLENV:+$WSLENV:}MCP_MODE" ;;
	esac
	case ":${WSLENV:-}:" in
		*:REAL_LLM:*) ;;
		*) export WSLENV="${WSLENV:+$WSLENV:}REAL_LLM" ;;
	esac
	case ":${WSLENV:-}:" in
		*:PYTHONUTF8:*) ;;
		*) export WSLENV="${WSLENV:+$WSLENV:}PYTHONUTF8" ;;
	esac
	case ":${WSLENV:-}:" in
		*:PYTHONIOENCODING:*) ;;
		*) export WSLENV="${WSLENV:+$WSLENV:}PYTHONIOENCODING" ;;
	esac
	if [[ "$REAL_LLM" == "true" ]]; then
		case ":${WSLENV:-}:" in
			*:CLEANCARE_LLM_API_KEY:*) ;;
			*) export WSLENV="${WSLENV:+$WSLENV:}CLEANCARE_LLM_API_KEY" ;;
		esac
		case ":${WSLENV:-}:" in
			*:CLEANCARE_EMBEDDING_API_KEY:*) ;;
			*) export WSLENV="${WSLENV:+$WSLENV:}CLEANCARE_EMBEDDING_API_KEY" ;;
		esac
		if [[ -n "${CLEANCARE_RERANKER_API_KEY:-}" ]]; then
			case ":${WSLENV:-}:" in
				*:CLEANCARE_RERANKER_API_KEY:*) ;;
				*) export WSLENV="${WSLENV:+$WSLENV:}CLEANCARE_RERANKER_API_KEY" ;;
			esac
		fi
	fi
else
	export CLEANCARE_CONFIG_FILE="$CONFIG_FILE"
fi
export CLEANCARE_QDRANT_BASE_URL="$QDRANT_BASE_URL"
export BASE_URL LOG_DIR MCP_URL MCP_SECONDARY_URL MCP_MODE
{
  echo "GO_CMD=$GO_CMD"
  echo "PY_CMD=${PY_CMD[*]}"
  echo "CURL_CMD=${CURL_CMD[*]}"
  echo "CLEANCARE_CONFIG_FILE=$CLEANCARE_CONFIG_FILE"
  echo "MCP_MODE=$MCP_MODE"
  echo "REAL_LLM=$REAL_LLM"
  echo "FRONTEND_E2E=$FRONTEND_E2E"
  echo "FRONTEND_URL=$FRONTEND_URL"
  echo "LLM_API_KEY_SET=$([[ -n "${CLEANCARE_LLM_API_KEY:-}" ]] && echo true || echo false)"
  echo "EMBEDDING_API_KEY_SET=$([[ -n "${CLEANCARE_EMBEDDING_API_KEY:-}" ]] && echo true || echo false)"
  echo "MCP_SECONDARY_CONFIG_FILE=$MCP_SECONDARY_CONFIG_FILE"
  echo "WSLENV=${WSLENV:-}"
  echo "QDRANT_BASE_URL=$QDRANT_BASE_URL"
  echo "REDIS_ADDRESS=$REDIS_ADDRESS"
  echo "MYSQL_HOST=$MYSQL_HOST"
  echo "MYSQL_DSN=$MYSQL_DSN"
  "$GO_CMD" env GOOS GOARCH GOEXE GOROOT GOPATH 2>/dev/null || true
} >"$LOG_DIR/env.log"

if [[ "$START_COMPOSE" == "true" ]]; then
  if ! docker compose up -d redis qdrant >"$LOG_DIR/compose-up.log" 2>&1; then
    cat "$LOG_DIR/compose-up.log" >&2 || true
    echo "docker compose up failed; if local ports are already occupied, run with E2E_START_COMPOSE=false against existing redis/qdrant services" >&2
    exit 1
  fi
  wait_compose_health redis 120
fi

wait_tcp "$MYSQL_HOST" "$CLEANCARE_MYSQL_PORT" "mysql" 180
wait_http "$QDRANT_BASE_URL/healthz" "qdrant" 120

run_logged migrate 5 "$GO_CMD" run ./cmd/migrate
run_logged seed 3 "$GO_CMD" run ./cmd/seed
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
if [[ "$MCP_MODE" == "aggregate_http" ]]; then
  CLEANCARE_CONFIG_FILE="$MCP_SECONDARY_RUNTIME_CONFIG_FILE" "$MCP_BIN" >"$LOG_DIR/mcp-server-secondary.log" 2>&1 &
  MCP_SECONDARY_PID="$!"
  wait_http_process "http://$MCP_HOST:$MCP_SECONDARY_PORT/health/live" "secondary mcp server" "$MCP_SECONDARY_PID" "$LOG_DIR/mcp-server-secondary.log" 120
fi

"$SERVER_BIN" >"$LOG_DIR/server.log" 2>&1 &
SERVER_PID="$!"
wait_http_process "$BASE_URL/health/ready" "api server" "$SERVER_PID" "$LOG_DIR/server.log" 180
if [[ "$REAL_LLM" == "true" ]] && ! grep -Fq "llm components enabled for agentic runner" "$LOG_DIR/server.log"; then
  echo "api server did not enable real LLM components" >&2
  cat "$LOG_DIR/server.log" >&2 || true
  exit 1
fi

if [[ "$MCP_MODE" == "aggregate_http" ]]; then
  if ! grep -Fq "aggregate mcp tool servers connected" "$LOG_DIR/server.log"; then
    echo "api server did not log aggregate MCP connection" >&2
    cat "$LOG_DIR/server.log" >&2 || true
    exit 1
  fi
  if ! grep -Fq "$MCP_SECONDARY_URL" "$LOG_DIR/server.log"; then
    echo "api server did not connect to expected secondary MCP endpoint $MCP_SECONDARY_URL" >&2
    cat "$LOG_DIR/server.log" >&2 || true
    exit 1
  fi
elif ! grep -Fq "remote mcp tool server connected" "$LOG_DIR/server.log"; then
  echo "api server did not log remote MCP connection" >&2
  cat "$LOG_DIR/server.log" >&2 || true
  exit 1
fi
if ! grep -Fq "$MCP_URL" "$LOG_DIR/server.log"; then
  echo "api server did not connect to expected MCP endpoint $MCP_URL" >&2
  cat "$LOG_DIR/server.log" >&2 || true
  exit 1
fi

BASE_URL="$BASE_URL" LOG_DIR="$LOG_DIR" MCP_MODE="$MCP_MODE" MCP_URL="$MCP_URL" MCP_SECONDARY_URL="$MCP_SECONDARY_URL" "${PY_CMD[@]}" - <<'PY'
import json
import os
import time
import urllib.error
import urllib.request

base_url = os.environ["BASE_URL"].rstrip("/")
log_dir = os.environ["LOG_DIR"]
mcp_url = os.environ.get("MCP_URL", "")
mcp_secondary_url = os.environ.get("MCP_SECONDARY_URL", "")
mcp_mode = os.environ.get("MCP_MODE", "http")
request_timeout = float(os.environ.get("E2E_HTTP_TIMEOUT_SECONDS", "30"))

def request(method, path, payload=None):
    data = None
    headers = {"Content-Type": "application/json"}
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(base_url + path, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=request_timeout) as resp:
            body = resp.read().decode("utf-8")
            return resp.status, json.loads(body)
    except urllib.error.HTTPError as err:
        body = err.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"{method} {path} returned {err.code}: {body}") from err

def wait_trace(trace_id, require_tool_activity):
    last_trace = None
    for _ in range(30):
        status, trace = request("GET", f"/api/v1/admin/traces/{trace_id}")
        last_trace = trace
        if status == 200 and trace.get("code") == "OK":
            record = trace["data"]
            tool_calls = record.get("tool_calls") or []
            if record.get("status") == "success" and (tool_calls or not require_tool_activity):
                return record
        time.sleep(1)
    raise RuntimeError(f"trace did not finish: {last_trace}")

def logical_name(name):
    text = str(name or "")
    return text.rsplit("/", 1)[-1]

def run_case(case):
    status, created = request("POST", "/api/v1/conversations", {"title": "e2e " + case["name"]})
    if status != 201 or created.get("code") != "OK":
        raise RuntimeError(f"{case['name']} create conversation failed: {created}")
    conversation_id = created["data"]["conversation_id"]

    status, asked = request("POST", f"/api/v1/conversations/{conversation_id}/messages", {
        "content": case["content"]
    })
    if status != 200 or asked.get("code") != "OK":
        raise RuntimeError(f"{case['name']} ask failed: {asked}")
    trace_id = asked["data"].get("trace_id")
    answer = asked["data"].get("answer", "")
    if not trace_id:
        raise RuntimeError(f"{case['name']} missing trace_id: {asked}")
    if not answer:
        raise RuntimeError(f"{case['name']} missing answer: {asked}")

    expected_tools = case.get("expected_tools", [])
    record = wait_trace(trace_id, bool(expected_tools))
    if record.get("latency_ms", 0) <= 0:
        raise RuntimeError(f"{case['name']} trace latency was not recorded: {record}")
    tool_calls = record.get("tool_calls") or []
    tool_names = [call.get("tool_name") for call in tool_calls]
    for expected in expected_tools:
        matching = [
            call for call in tool_calls
            if call.get("tool_name") == expected or logical_name(call.get("tool_name")) == expected
        ]
        if not matching:
            raise RuntimeError(f"{case['name']} missing expected tool {expected}: {tool_calls}")
        if matching[0].get("status") != "success":
            raise RuntimeError(f"{case['name']} expected tool {expected} did not succeed: {matching[0]}")
        if "latency_ms" not in matching[0]:
            raise RuntimeError(f"{case['name']} expected tool {expected} latency field was not recorded: {matching[0]}")
    if case.get("forbid_tools") and tool_calls:
        raise RuntimeError(f"{case['name']} expected no tools, got: {tool_calls}")
    if case.get("require_evidence") and not (record.get("evidence_ids") or []):
        raise RuntimeError(f"{case['name']} expected evidence ids: {record}")
    expected_intent = case.get("expected_intent")
    if expected_intent and record.get("intent") != expected_intent:
        raise RuntimeError(f"{case['name']} intent = {record.get('intent')}, want {expected_intent}")
    return {
        "name": case["name"],
        "conversation_id": conversation_id,
        "trace_id": trace_id,
        "intent": record.get("intent"),
        "answer": answer,
        "trace_status": record.get("status"),
        "trace_latency_ms": record.get("latency_ms"),
        "evidence_ids": record.get("evidence_ids") or [],
        "tool_names": tool_names,
        "tool_calls": tool_calls,
    }

cases = [
    {
        "name": "price_tool",
        "content": "T20 \u73b0\u5728\u591a\u5c11\u94b1\uff1f\u8bf7\u7ed9\u51fa\u5f53\u524d\u4ef7\u683c\u548c\u4f18\u60e0\u4fe1\u606f\u3002",
        "expected_intent": "price_query",
        "expected_tools": ["price_query"],
    },
    {
        "name": "inventory_tool",
        "content": "P400\u548cP500\u54ea\u4e2a\u6709\u8d27\uff0c\u5404\u5269\u591a\u5c11\u522b\u6df7\u4e86\u3002",
        "expected_intent": "inventory_query",
        "expected_tools": ["inventory_check"],
    },
    {
        "name": "order_tool",
        "content": "\u8ba2\u5355CC20260603001\u5e2e\u6211\u6838\u5bf9\u4e0b\u5546\u54c1\u578b\u53f7\u548c\u6570\u91cf\u3002",
        "expected_intent": "order_query",
        "expected_tools": ["order_lookup"],
    },
    {
        "name": "after_sales_tool",
        "content": "\u6211\u786e\u8ba4\u63d0\u4ea4\uff0c\u7ed9CC20260603001\u5efa\u7ef4\u4fee\u5de5\u5355\uff1aP400\u5f02\u54cd\u3002",
        "expected_intent": "create_after_sales_ticket",
        "expected_tools": ["create_after_sales_ticket"],
    },
    {
        "name": "pure_kb",
        "content": "T20\u914d\u7f51\u5931\u8d25\u6309\u54ea\u51e0\u6b65\u67e5\uff1f",
        "expected_intent": "troubleshooting",
        "forbid_tools": True,
        "require_evidence": True,
    },
    {
        "name": "clarification_no_tool",
        "content": "\u90a3\u4fe9\u54ea\u4e2a\u597d\u70b9\uff0c\u4f60\u61c2\u6211\u8bf4\u7684\u5427\u3002",
        "expected_intent": "clarification",
        "forbid_tools": True,
    },
]

results = [run_case(case) for case in cases]

snapshot = {
    "mcp_transport": mcp_mode,
    "mcp_endpoint": mcp_url,
    "mcp_secondary_endpoint": mcp_secondary_url if mcp_mode == "aggregate_http" else "",
    "case_count": len(results),
    "cases": results,
}
with open(os.path.join(log_dir, "e2e-result.json"), "w", encoding="utf-8") as f:
    json.dump(snapshot, f, ensure_ascii=False, indent=2)
print(json.dumps(snapshot, ensure_ascii=False, indent=2))
PY

if [[ "$FRONTEND_E2E" == "true" ]]; then
  stop_frontend_port_processes
  export VITE_API_PROXY_TARGET="$BASE_URL"
  if [[ "$GO_OS" == "windows" ]] && command -v wslpath >/dev/null 2>&1; then
    case ":${WSLENV:-}:" in
      *:VITE_API_PROXY_TARGET:*) ;;
      *) export WSLENV="${WSLENV:+$WSLENV:}VITE_API_PROXY_TARGET" ;;
    esac
  fi
  (
    cd "$ROOT_DIR/clean-care-frontend"
    "${NPM_CMD[@]}" run dev -- --host "$FRONTEND_HOST" --port "$FRONTEND_PORT" --strictPort
  ) >"$LOG_DIR/frontend.log" 2>&1 &
  FRONTEND_PID="$!"
  wait_http_process "$FRONTEND_URL" "frontend" "$FRONTEND_PID" "$LOG_DIR/frontend.log" 120

  FRONTEND_E2E_SCRIPT="$LOG_DIR/frontend-e2e.cjs"
  cat >"$FRONTEND_E2E_SCRIPT" <<'JS'
const { chromium } = require('playwright');
const fs = require('node:fs/promises');
const path = require('node:path');

const frontendURL = process.env.FRONTEND_URL;
const logDir = process.env.FRONTEND_NODE_LOG_DIR || process.env.LOG_DIR;
const question = process.env.E2E_FRONTEND_QUESTION || 'T20 现在多少钱？请给出当前价格和优惠信息。';
const expectedTool = process.env.E2E_FRONTEND_EXPECTED_TOOL || 'price_query';

function logicalName(name) {
  const text = String(name || '');
  const slash = text.lastIndexOf('/');
  return slash >= 0 ? text.slice(slash + 1) : text;
}

async function fetchTrace(page, traceId) {
  let last;
  for (let i = 0; i < 90; i += 1) {
    last = await page.evaluate(async (id) => {
      const response = await fetch(`/api/v1/admin/traces/${encodeURIComponent(id)}`);
      const body = await response.json().catch(() => ({}));
      return { status: response.status, body };
    }, traceId);
    if (last.status === 200 && last.body?.code === 'OK' && last.body?.data?.status === 'success') {
      return last.body.data;
    }
    await page.waitForTimeout(1000);
  }
  throw new Error(`trace ${traceId} did not finish successfully: ${JSON.stringify(last)}`);
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 960 } });
  const consoleMessages = [];
  page.on('console', (msg) => consoleMessages.push(`${msg.type()}: ${msg.text()}`));
  page.on('pageerror', (err) => consoleMessages.push(`pageerror: ${err.message}`));

  try {
    await page.goto(frontendURL, { waitUntil: 'domcontentloaded', timeout: 60000 });
    await page.waitForSelector('textarea.chat-input__textarea', { timeout: 30000 });
    await page.fill('textarea.chat-input__textarea', question);
    const streamResponse = page.waitForResponse(
      (response) => response.url().includes('/messages:stream') && response.status() === 200,
      { timeout: 120000 },
    );
    await page.press('textarea.chat-input__textarea', 'Enter');
    await streamResponse;
    await page.waitForFunction(() => {
      const answerBubbles = Array.from(document.querySelectorAll('.message--assistant .message__bubble'));
      const hasAnswer = answerBubbles.some((element) => (element.textContent || '').trim().length > 20);
      const traceText = document.querySelector('.pipeline-panel__trace')?.textContent?.trim();
      const stillStreaming = Boolean(document.querySelector('.status-bar__dot'));
      return hasAnswer && traceText && !stillStreaming;
    }, null, { timeout: 180000 });

    const traceId = (await page.locator('.pipeline-panel__trace').textContent()).trim();
    const trace = await fetchTrace(page, traceId);
    const answer = (await page.locator('.message--assistant .message__bubble').last().innerText()).trim();
    const toolCalls = trace.tool_calls || [];
    const toolNames = toolCalls.map((call) => call.tool_name);
    const totalTokens = (trace.input_tokens || 0) + (trace.output_tokens || 0);
    if (totalTokens <= 0) {
      throw new Error(`expected real LLM token usage, got input=${trace.input_tokens} output=${trace.output_tokens}`);
    }
    if (!toolNames.some((name) => name === expectedTool || logicalName(name) === expectedTool)) {
      throw new Error(`expected frontend trace to call ${expectedTool}, got ${JSON.stringify(toolNames)}`);
    }

    const screenshotPath = path.join(logDir, 'frontend-real-chain.png');
    await page.screenshot({ path: screenshotPath, fullPage: true });
    const result = {
      entrypoint: 'frontend',
      url: frontendURL,
      question,
      trace_id: traceId,
      intent: trace.intent,
      answer,
      input_tokens: trace.input_tokens || 0,
      output_tokens: trace.output_tokens || 0,
      total_tokens: totalTokens,
      tool_names: toolNames,
      tool_calls: toolCalls,
      evidence_ids: trace.evidence_ids || [],
      screenshot: screenshotPath,
      console_messages: consoleMessages,
    };
    await fs.writeFile(path.join(logDir, 'frontend-e2e-result.json'), JSON.stringify(result, null, 2), 'utf8');
    console.log(JSON.stringify(result, null, 2));
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
JS

  FRONTEND_PLAYWRIGHT_LOG="$LOG_DIR/frontend-e2e.log"
  FRONTEND_E2E_SCRIPT_RUNTIME="$FRONTEND_E2E_SCRIPT"
  FRONTEND_NODE_LOG_DIR="$LOG_DIR"
  PLAYWRIGHT_PREFIX="$LOG_DIR/playwright-node"
  PLAYWRIGHT_PREFIX_RUNTIME="$PLAYWRIGHT_PREFIX"
  PLAYWRIGHT_NODE_PATH="$PLAYWRIGHT_PREFIX/node_modules"
  PLAYWRIGHT_CLI_JS="$PLAYWRIGHT_PREFIX/node_modules/playwright/cli.js"
  if [[ "$GO_OS" == "windows" ]] && command -v wslpath >/dev/null 2>&1; then
    FRONTEND_E2E_SCRIPT_RUNTIME="$(wslpath -w "$FRONTEND_E2E_SCRIPT")"
    FRONTEND_NODE_LOG_DIR="$(wslpath -w "$LOG_DIR")"
    PLAYWRIGHT_PREFIX_RUNTIME="$(wslpath -w "$PLAYWRIGHT_PREFIX")"
    PLAYWRIGHT_NODE_PATH="$(wslpath -w "$PLAYWRIGHT_PREFIX/node_modules")"
    PLAYWRIGHT_CLI_JS="$(wslpath -w "$PLAYWRIGHT_CLI_JS")"
    case ":${WSLENV:-}:" in
      *:NODE_PATH:*) ;;
      *) export WSLENV="${WSLENV:+$WSLENV:}NODE_PATH" ;;
    esac
    case ":${WSLENV:-}:" in
      *:FRONTEND_URL:*) ;;
      *) export WSLENV="${WSLENV:+$WSLENV:}FRONTEND_URL" ;;
    esac
    case ":${WSLENV:-}:" in
      *:FRONTEND_NODE_LOG_DIR:*) ;;
      *) export WSLENV="${WSLENV:+$WSLENV:}FRONTEND_NODE_LOG_DIR" ;;
    esac
    case ":${WSLENV:-}:" in
      *:E2E_FRONTEND_QUESTION:*) ;;
      *) export WSLENV="${WSLENV:+$WSLENV:}E2E_FRONTEND_QUESTION" ;;
    esac
    case ":${WSLENV:-}:" in
      *:E2E_FRONTEND_EXPECTED_TOOL:*) ;;
      *) export WSLENV="${WSLENV:+$WSLENV:}E2E_FRONTEND_EXPECTED_TOOL" ;;
    esac
  fi
  if [[ ! -f "$PLAYWRIGHT_PREFIX/node_modules/playwright/package.json" ]]; then
    mkdir -p "$PLAYWRIGHT_PREFIX"
    "${NPM_CMD[@]}" install --prefix "$PLAYWRIGHT_PREFIX_RUNTIME" --no-audit --no-fund "playwright@${E2E_PLAYWRIGHT_VERSION:-latest}" >"$FRONTEND_PLAYWRIGHT_LOG" 2>&1
  fi
  export NODE_PATH="$PLAYWRIGHT_NODE_PATH"
  export FRONTEND_URL
  export FRONTEND_NODE_LOG_DIR
  export E2E_FRONTEND_QUESTION="${E2E_FRONTEND_QUESTION:-}"
  export E2E_FRONTEND_EXPECTED_TOOL="${E2E_FRONTEND_EXPECTED_TOOL:-}"
  if ! "${NODE_CMD[@]}" "$FRONTEND_E2E_SCRIPT_RUNTIME" >"$FRONTEND_PLAYWRIGHT_LOG" 2>&1; then
    if grep -Eiq "Executable doesn't exist|browser.*not.*installed|Looks like Playwright" "$FRONTEND_PLAYWRIGHT_LOG"; then
      "${NODE_CMD[@]}" "$PLAYWRIGHT_CLI_JS" install chromium >>"$FRONTEND_PLAYWRIGHT_LOG" 2>&1
      "${NODE_CMD[@]}" "$FRONTEND_E2E_SCRIPT_RUNTIME" >>"$FRONTEND_PLAYWRIGHT_LOG" 2>&1
    else
      cat "$FRONTEND_PLAYWRIGHT_LOG" >&2 || true
      exit 1
    fi
  fi
  if [[ ! -s "$LOG_DIR/frontend-e2e-result.json" ]]; then
    echo "frontend Playwright run did not produce frontend-e2e-result.json" >&2
    cat "$FRONTEND_PLAYWRIGHT_LOG" >&2 || true
    exit 1
  fi

  LOG_DIR="$LOG_DIR" REAL_LLM="$REAL_LLM" FRONTEND_URL="$FRONTEND_URL" "${PY_CMD[@]}" - <<'PY'
import json
import os
from pathlib import Path

log_dir = Path(os.environ["LOG_DIR"])
result_path = log_dir / "e2e-result.json"
frontend_path = log_dir / "frontend-e2e-result.json"

with result_path.open("r", encoding="utf-8") as f:
    result = json.load(f)
with frontend_path.open("r", encoding="utf-8") as f:
    frontend = json.load(f)

result["entrypoints"] = ["api", "frontend"]
result["frontend_url"] = os.environ["FRONTEND_URL"]
result["real_llm"] = os.environ.get("REAL_LLM") == "true"
result["frontend"] = frontend

with result_path.open("w", encoding="utf-8") as f:
    json.dump(result, f, ensure_ascii=False, indent=2)
print(json.dumps(result, ensure_ascii=False, indent=2))
PY
fi

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
    --poll-request-timeout "${E2E_EVAL_POLL_REQUEST_TIMEOUT:-30}" \
    --result-request-timeout "${E2E_EVAL_RESULT_REQUEST_TIMEOUT:-180}" \
    --result-retries "${E2E_EVAL_RESULT_RETRIES:-2}" \
    --baseline-label "${E2E_EVAL_BASELINE_LABEL:-2026-06-13 real-model eval_67abf7a0bdfe419fbdf92a68}" \
    --baseline-pass-rate "${E2E_EVAL_BASELINE_PASS_RATE:-0.715}" \
    --baseline-p95-latency-ms "${E2E_EVAL_BASELINE_P95_LATENCY_MS:-17077}" \
    --baseline-average-tokens "${E2E_EVAL_BASELINE_AVERAGE_TOKENS:-4007}"
fi
