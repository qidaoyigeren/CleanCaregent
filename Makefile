.PHONY: run run-mcp-server build build-mcp-server migrate seed-all kb-validate test test-race coverage fmt vet lint tidy-check e2e-agentic-mcp e2e-agentic-mcp-eval eval-dataset eval-compare eval-regression eval-regression-report compose-up compose-down

BASE_URL ?= http://127.0.0.1:8080
PYTHON ?= python3
SYSTEM_VERSION ?= agentic-mcp-http
EVAL_MAX_CASES ?= 200
EVAL_SPLIT ?=
EVAL_OUTPUT ?= docs/eval/mcp-regression-report.md

run:
	go run ./cmd/server

run-mcp-server:
	go run ./cmd/mcp-server

build:
	go build -o bin/cleancare ./cmd/server

build-mcp-server:
	go build -o bin/cleancare-mcp-server ./cmd/mcp-server

migrate:
	go run ./cmd/migrate

seed-all:
	go run ./cmd/seed
	go run ./cmd/kb-seed

kb-validate:
	go run ./cmd/kb-validate

test:
	go test ./...

test-race:
	go test -race ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

fmt:
	gofmt -w cmd internal pkg

vet:
	go vet ./...

lint:
	golangci-lint run ./...

tidy-check:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

e2e-agentic-mcp:
	bash scripts/e2e-agentic-mcp.sh

e2e-agentic-mcp-eval:
	E2E_RUN_EVAL=true E2E_EVAL_SYSTEM_VERSION="$(SYSTEM_VERSION)" E2E_EVAL_MAX_CASES="$(EVAL_MAX_CASES)" E2E_EVAL_OUTPUT="$(EVAL_OUTPUT)" bash scripts/e2e-agentic-mcp.sh

eval-dataset:
	go run ./cmd/eval-dataset

eval-compare:
	curl --fail --silent --show-error \
		-X POST "$(BASE_URL)/api/v1/admin/eval/comparisons" \
		-H "Content-Type: application/json" \
		-d '{"dataset_version":"v2","split":"regression","max_cases":200}'

eval-regression:
	curl --fail --silent --show-error \
		-X POST "$(BASE_URL)/api/v1/admin/eval/runs" \
		-H "Content-Type: application/json" \
		-d '{"dataset_version":"v2","system_version":"regression","split":"regression","max_cases":200}'

eval-regression-report:
	$(PYTHON) scripts/eval-regression-report.py \
		--base-url "$(BASE_URL)" \
		--system-version "$(SYSTEM_VERSION)" \
		--max-cases "$(EVAL_MAX_CASES)" \
		--split "$(EVAL_SPLIT)" \
		--output "$(EVAL_OUTPUT)"

compose-up:
	docker compose --profile app up -d --build

compose-down:
	docker compose down
