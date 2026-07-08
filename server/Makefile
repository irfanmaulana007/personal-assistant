.PHONY: build run dev test lint clean tidy

BINARY=personal-assistant
# sqlite_fts5 enables FTS5 in mattn/go-sqlite3 (required by notes_fts)
TAGS=sqlite_fts5

# Build the server binary (output at repo root)
build:
	go build -tags $(TAGS) -o ../$(BINARY) ./cmd/assistant

# Run the server
run: build
	../$(BINARY)

# Development with hot reload
dev:
	air

test:
	go test -tags $(TAGS) ./... -v

lint:
	golangci-lint run ./...

clean:
	rm -f ../$(BINARY)
	rm -rf tmp
	rm -f data/*.db data/*.db-wal data/*.db-shm

tidy:
	go mod tidy
