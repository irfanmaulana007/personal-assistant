# Stage 1: Build client
FROM node:22-alpine AS client-builder
WORKDIR /app/client
COPY client/package.json client/package-lock.json ./
RUN npm ci
COPY client/ .
RUN npm run build

# Stage 2: Build server
FROM golang:1.25-alpine AS server-builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY server/ server/
RUN CGO_ENABLED=1 go build -o personal-assistant ./server/cmd/assistant

# Stage 3: Final image
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=server-builder /app/personal-assistant .
COPY --from=client-builder /app/client/dist ./client/dist

RUN mkdir -p /app/server/data /app/server/config

VOLUME ["/app/server/data", "/app/server/config"]
EXPOSE 8080

ENTRYPOINT ["./personal-assistant", "-config", "server/config/config.yaml"]
