# Stage 1: Build client
FROM node:22-alpine AS client-builder
WORKDIR /app/client
COPY client/package.json client/package-lock.json ./
# npm install (not ci) so the Linux/musl-specific optional native deps resolve
# even though the committed lockfile is generated on a different platform.
RUN npm install --no-audit --no-fund
COPY client/ .
RUN npm run build

# Stage 2: Build server (CGO required for go-sqlite3; sqlite_fts5 for notes search)
FROM golang:1.25-alpine AS server-builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY server/ server/
RUN CGO_ENABLED=1 go build -tags sqlite_fts5 -o personal-assistant ./server/cmd/assistant
# ETL: migrate an existing SQLite database into the hybrid Postgres+Mongo backend.
# Run in-container with e.g.:
#   docker compose run --rm --entrypoint /app/migrate-db assistant --config config/config.yaml --verify
RUN CGO_ENABLED=1 go build -tags sqlite_fts5 -o migrate-db ./server/cmd/migrate-db

# Stage 3: Final image
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=server-builder /app/personal-assistant .
COPY --from=server-builder /app/migrate-db .
COPY --from=client-builder /app/client/dist ./client/dist
# Container config (paths relative to /app; secrets injected from env at runtime).
COPY server/config/config.docker.yaml ./config/config.yaml

# Persistent data: SQLite app DB + WhatsApp (whatsmeow) session live here.
RUN mkdir -p /app/data
VOLUME ["/app/data"]

EXPOSE 8090

ENTRYPOINT ["./personal-assistant", "-config", "config/config.yaml"]
