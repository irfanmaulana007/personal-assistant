.PHONY: build run dev test lint clean tidy

BINARY=personal-assistant

# Build the server binary (output at repo root). SQLite was dropped, so the
# build no longer needs CGO — a static, CGO-free binary is produced.
build:
	CGO_ENABLED=0 go build -o ../../$(BINARY) ./cmd/assistant

# Run the server
run: build
	../../$(BINARY)

# Development with hot reload
dev:
	air

# Tests exercise the real PostgreSQL + MongoDB backends via testcontainers, so
# Docker must be running. The `integration` tag additionally enables the
# store-package integration tests.
test:
	go test -tags integration ./... -v

lint:
	golangci-lint run ./...

clean:
	rm -f ../../$(BINARY)
	rm -rf tmp

tidy:
	go mod tidy
