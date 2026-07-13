# Deployment

## Storage

The server stores its data in **PostgreSQL** (main data) and **MongoDB**
(logs/analytics). Both are required. `docker-compose.yml` bundles `postgres:17`
and `mongo:7` alongside the assistant.

> SQLite has been fully removed. The WhatsApp (whatsmeow) session is now stored
> in PostgreSQL too (its own `whatsmeow_*` tables in the app database), so the
> server builds CGO-free and the container is stateless.

### Docker Compose

Configure secrets in `.env` (see `.env.example`), then bring up the stack:

```bash
POSTGRES_PASSWORD=...   # bundled postgres
MONGO_PASSWORD=...      # bundled mongo
ENCRYPTION_KEY=...      # openssl rand -base64 32
WEB_PASSWORD=...
OWNER_JID=...@s.whatsapp.net

docker compose up -d
```

The Postgres schema is created automatically on first boot (embedded
golang-migrate migrations); Mongo indexes are ensured on startup.

### Migrating from an older SQLite deployment

The one-time SQLite → Postgres+Mongo ETL (`migrate-db`) has been **removed** now
that all deployments have completed the migration. If you still need to import a
legacy `assistant.db`, run the `migrate-db` tool from an image built before this
change (any release up to and including `v1.0.3`), then upgrade.

The WhatsApp session does not carry over automatically: after upgrading, re-pair
your phone from the Integrations page (scan the QR once). whatsmeow then persists
the new session in Postgres.

## Local Development

### Prerequisites
- Go 1.25+
- PostgreSQL and MongoDB (e.g. via `docker compose up postgres mongo`)
- Google Cloud project with Calendar & Gmail APIs enabled

### Setup

```bash
# Clone
git clone https://github.com/irfanmaulana007/personal-assistant.git
cd personal-assistant

# Configure
cp config.example.yaml config.yaml
# Edit config.yaml with your WhatsApp JID and preferences
# Place google_credentials.json in config/

# Build & run
make build
./bin/assistant

# Or run directly
make run
```

### First Run

1. QR code appears in terminal
2. Scan with WhatsApp on your phone (Linked Devices → Link a Device)
3. Google OAuth URL appears — click to authorize Calendar & Gmail
4. Send a WhatsApp message to test: "What's on my calendar today?"

## Docker

### Dockerfile

```dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /assistant ./cmd/assistant

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /assistant /usr/local/bin/assistant

VOLUME /data
VOLUME /config

ENTRYPOINT ["assistant"]
```

### Docker Compose

```yaml
version: "3.8"

services:
  assistant:
    build: .
    restart: unless-stopped
    volumes:
      - ./data:/data
      - ./config:/config
    environment:
      - ASSISTANT_ENCRYPTION_KEY=${ASSISTANT_ENCRYPTION_KEY}
      - ASSISTANT_DATABASE_PATH=/data/assistant.db
    stdin_open: true   # needed for QR code on first run
    tty: true
```

### Running with Docker

```bash
# First run (interactive for QR code)
docker compose run --rm assistant

# After pairing, run in background
docker compose up -d

# View logs
docker compose logs -f assistant
```

## Systemd (Linux)

### Service File

```ini
# /etc/systemd/system/personal-assistant.service

[Unit]
Description=Personal Assistant
After=network.target

[Service]
Type=simple
User=assistant
WorkingDirectory=/opt/personal-assistant
ExecStart=/opt/personal-assistant/bin/assistant
Restart=on-failure
RestartSec=10

Environment=ASSISTANT_ENCRYPTION_KEY=your-key-here
Environment=ASSISTANT_DATABASE_PATH=/opt/personal-assistant/data/assistant.db

[Install]
WantedBy=multi-user.target
```

### Commands

```bash
sudo systemctl daemon-reload
sudo systemctl enable personal-assistant
sudo systemctl start personal-assistant
sudo systemctl status personal-assistant
journalctl -u personal-assistant -f
```

## Monitoring

### Health Check

The assistant exposes a simple HTTP health endpoint (optional):

```go
http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    status := map[string]string{
        "status":    "ok",
        "whatsapp":  client.IsConnected(),
        "database":  db.Ping(),
        "uptime":    time.Since(startTime).String(),
    }
    json.NewEncoder(w).Encode(status)
})
```

### Logging

Structured logs via `log/slog`:

```bash
# Filter by level
journalctl -u personal-assistant | grep "level=ERROR"

# JSON format for log aggregation
ASSISTANT_LOGGING_FORMAT=json ./bin/assistant
```

### Alerts

For critical failures (WhatsApp disconnected, database errors), consider:
- Email notification (via a separate simple script)
- Healthcheck monitoring (e.g., Uptime Kuma)
- Log-based alerting

## Backup

### Database Backup

```bash
# PostgreSQL (main data + WhatsApp session)
docker compose exec -T postgres pg_dump -U assistant assistant \
  > backups/assistant_$(date +%Y%m%d).sql

# MongoDB (logs/analytics)
docker compose exec -T mongo mongodump --archive \
  > backups/logs_$(date +%Y%m%d).archive
```

### Automated Backup (cron)

```bash
# Daily backup at 2 AM
0 2 * * * /opt/personal-assistant/scripts/backup.sh
```

### What to Back Up

- `data/assistant.db` — application data (reminders, notes, tokens)
- `data/whatsmeow.db` — WhatsApp session (avoids re-pairing)
- `config.yaml` — configuration
- `config/google_credentials.json` — Google OAuth credentials
