# Backend (Core API) image — builds ONLY the Go server, no client assets, so the
# backend deploys independently of the web app. Serves the JSON API on :8090.
#
# Build from the repo root:  docker build -f deploy/backend.Dockerfile -t pa-backend .
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY server/ server/
# SQLite was dropped (whatsmeow's session lives in Postgres), so the build is
# CGO-free — a static binary, no C toolchain required.
RUN CGO_ENABLED=0 go build -o personal-assistant ./server/cmd/assistant

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/personal-assistant .
# Container config (paths relative to /app; secrets injected from env at runtime).
COPY server/config/config.docker.yaml ./config/config.yaml
# All persistent state lives in PostgreSQL + MongoDB, so the container is
# stateless — no data volume.
EXPOSE 8090
ENTRYPOINT ["./personal-assistant", "-config", "config/config.yaml"]
