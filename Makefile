.PHONY: run build migrate seed-all test test-race coverage fmt vet lint tidy-check eval-dataset eval-compare eval-regression compose-up compose-down

BASE_URL ?= http://127.0.0.1:8080

run:
	go run ./cmd/server

build:
	go build -o bin/cleancare ./cmd/server

migrate:
	go run ./cmd/migrate

seed-all:
	go run ./cmd/seed
	go run ./cmd/kb-seed

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

eval-dataset:
	go run ./cmd/eval-dataset

eval-compare:
	curl --fail --silent --show-error \
		-X POST "$(BASE_URL)/api/v1/admin/eval/comparisons" \
		-H "Content-Type: application/json" \
		-d '{"dataset_version":"v2","max_cases":200}'

eval-regression:
	curl --fail --silent --show-error \
		-X POST "$(BASE_URL)/api/v1/admin/eval/runs" \
		-H "Content-Type: application/json" \
		-d '{"dataset_version":"v2","system_version":"regression","max_cases":200}'

compose-up:
	docker compose --profile app up -d --build

compose-down:
	docker compose down
