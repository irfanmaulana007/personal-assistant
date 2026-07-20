# Stage 1: Build client (pnpm-workspace monorepo). The client depends on the
# `@personal-assistant/shared` workspace package, so install happens at the repo
# root and the client is built as a workspace.
FROM node:22-alpine AS client-builder
RUN corepack enable
WORKDIR /app
# Workspace manifests + lockfile first, for cacheable dependency layers.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/shared/package.json ./packages/shared/package.json
COPY app/web/package.json ./app/web/package.json
# pnpm's lockfile records optional native deps for every platform, so a frozen
# install resolves the Linux/musl-specific binaries even though the lockfile is
# generated on a different platform.
RUN pnpm install --frozen-lockfile
# Sources for the shared package and the web app. The root package.json (already
# copied) is what vite.config.ts reads the app version from.
COPY packages/ ./packages/
COPY app/web/ ./app/web/
RUN pnpm --filter web build

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
