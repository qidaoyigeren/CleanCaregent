.PHONY: run migrate test test-race fmt vet eval-dataset eval-compare compose-up compose-down

BASE_URL ?= http://127.0.0.1:8080

run:
	go run ./cmd/server

migrate:
	go run ./cmd/migrate

test:
	go test ./...

test-race:
	go test -race ./...

fmt:
	gofmt -w cmd internal pkg

vet:
	go vet ./...

eval-dataset:
	go run ./cmd/eval-dataset

eval-compare:
	curl --fail --silent --show-error \
		-X POST "$(BASE_URL)/api/v1/admin/eval/comparisons" \
		-H "Content-Type: application/json" \
		-d '{"dataset_version":"v2","max_cases":100}'

compose-up:
	docker compose up -d

compose-down:
	docker compose down
