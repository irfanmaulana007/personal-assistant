# Stage 1: Build client (npm-workspaces monorepo). The client depends on the
# `@personal-assistant/shared` workspace package, so install happens at the repo
# root and the client is built as a workspace.
FROM node:22-alpine AS client-builder
WORKDIR /app
# Workspace manifests + lockfile first, for cacheable dependency layers.
COPY package.json package-lock.json ./
COPY packages/shared/package.json ./packages/shared/package.json
COPY app/web/package.json ./app/web/package.json
# npm install (not ci) so the Linux/musl-specific optional native deps resolve
# even though the committed lockfile is generated on a different platform.
RUN npm install --no-audit --no-fund
# Sources for the shared package and the web app. The root package.json (already
# copied) is what vite.config.ts reads the app version from.
COPY packages/ ./packages/
COPY app/web/ ./app/web/
RUN npm run build --workspace web

# Stage 2: Build server. SQLite was dropped (whatsmeow's session lives in
# Postgres now), so the build is CGO-free — no C toolchain required.
FROM golang:1.25-alpine AS server-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY app/api/ app/api/
RUN CGO_ENABLED=0 go build -o personal-assistant ./app/api/cmd/assistant

# Stage 3: Final image
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=server-builder /app/personal-assistant .
COPY --from=client-builder /app/app/web/dist ./web/dist
# Container config (paths relative to /app; secrets injected from env at runtime).
COPY app/api/config/config.docker.yaml ./config/config.yaml

# All persistent state now lives in PostgreSQL + MongoDB (including the WhatsApp
# whatsmeow session), so the container itself is stateless — no data volume.

EXPOSE 8090

ENTRYPOINT ["./personal-assistant", "-config", "config/config.yaml"]
