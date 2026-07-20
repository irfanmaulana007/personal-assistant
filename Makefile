.PHONY: build build-server build-client run dev-server dev-client test lint lint-client typecheck-shared clean tidy deps \
        docker-build docker-build-backend docker-build-web

# Build both server and client
build: build-server build-client

build-server:
	$(MAKE) -C app/api build

# The web app is an npm workspace that depends on @personal-assistant/shared;
# `npm run build --workspace web` (run via app/web/Makefile) resolves it from
# the root node_modules created by `make deps`.
build-client:
	$(MAKE) -C app/web build

# Run the server
run:
	$(MAKE) -C app/api run

# Development with hot reload
dev-server:
	$(MAKE) -C app/api dev

dev-client:
	$(MAKE) -C app/web dev

test:
	$(MAKE) -C app/api test

# `make lint` lints the Go server (the release/CI Go gate). Use `make lint-client`
# for the web workspace's eslint.
lint:
	$(MAKE) -C app/api lint

lint-client:
	npm run lint --workspace web

# Type-check the shared package on its own.
typecheck-shared:
	npm run typecheck --workspace @personal-assistant/shared

# Install dependencies: one workspace install at the repo root links the shared
# package into the client, plus the root dev tooling (husky/eslint/prettier).
deps:
	npm install

tidy:
	$(MAKE) -C app/api tidy

# --- Split deployment images (built independently per service) ----------------
# Combined all-in-one image (server + bundled web) — used by docker-compose.
docker-build:
	docker build -t personal-assistant .

# Backend-only API image.
docker-build-backend:
	docker build -f deploy/backend.Dockerfile -t personal-assistant-backend .

# Web-only static image (nginx). Pass VITE_API_BASE_URL to point at the backend.
docker-build-web:
	docker build -f deploy/web.Dockerfile \
		--build-arg VITE_API_BASE_URL=$(VITE_API_BASE_URL) -t personal-assistant-web .

clean:
	$(MAKE) -C app/api clean
	$(MAKE) -C app/web clean
