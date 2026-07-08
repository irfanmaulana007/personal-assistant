.PHONY: build build-server build-client run dev-server dev-client test lint clean tidy deps

# Build both server and client
build: build-server build-client

build-server:
	$(MAKE) -C server build

build-client:
	$(MAKE) -C client build

# Run the server
run:
	$(MAKE) -C server run

# Development with hot reload
dev-server:
	$(MAKE) -C server dev

dev-client:
	$(MAKE) -C client dev

test:
	$(MAKE) -C server test

lint:
	$(MAKE) -C server lint

# Install dependencies
tidy:
	$(MAKE) -C server tidy

deps:
	$(MAKE) -C client deps

clean:
	$(MAKE) -C server clean
	$(MAKE) -C client clean
