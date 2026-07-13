# Stage 1: Build client
FROM node:22-alpine AS client-builder
WORKDIR /app/client
COPY client/package.json client/package-lock.json ./
# npm install (not ci) so the Linux/musl-specific optional native deps resolve
# even though the committed lockfile is generated on a different platform.
RUN npm install --no-audit --no-fund
COPY client/ .
RUN npm run build

# Stage 2: Build server. SQLite was dropped (whatsmeow's session lives in
# Postgres now), so the build is CGO-free — no C toolchain required.
FROM golang:1.25-alpine AS server-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY server/ server/
RUN CGO_ENABLED=0 go build -o personal-assistant ./server/cmd/assistant

# Stage 3: Final image
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=server-builder /app/personal-assistant .
COPY --from=client-builder /app/client/dist ./client/dist
# Container config (paths relative to /app; secrets injected from env at runtime).
COPY server/config/config.docker.yaml ./config/config.yaml

# All persistent state now lives in PostgreSQL + MongoDB (including the WhatsApp
# whatsmeow session), so the container itself is stateless — no data volume.

EXPOSE 8090

ENTRYPOINT ["./personal-assistant", "-config", "config/config.yaml"]
