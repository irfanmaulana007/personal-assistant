# Deployment

## Local Development

### Prerequisites
- Go 1.22+
- SQLite (included via Go library)
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
# Simple file copy (SQLite is single-file)
cp data/assistant.db backups/assistant_$(date +%Y%m%d).db

# Or use SQLite backup API
sqlite3 data/assistant.db ".backup backups/assistant_$(date +%Y%m%d).db"
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
