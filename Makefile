.PHONY: build build-server build-client run dev dev-server dev-client test lint clean tidy

BINARY=personal-assistant

# Build both server and client
build: build-server build-client

build-server:
	go build -o $(BINARY) ./server/cmd/assistant

build-client:
	cd client && npm run build

# Run the server
run: build-server
	./$(BINARY)

# Development with hot reload
dev-server:
	cd server && air

dev-client:
	cd client && npm run dev

test:
	go test ./server/... -v

lint:
	golangci-lint run ./server/...

clean:
	rm -f $(BINARY)
	rm -rf server/tmp
	rm -rf client/dist
	rm -f server/data/*.db server/data/*.db-wal server/data/*.db-shm

tidy:
	go mod tidy
