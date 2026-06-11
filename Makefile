.PHONY: run migrate test test-race fmt vet compose-up compose-down

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

compose-up:
	docker compose up -d

compose-down:
	docker compose down
