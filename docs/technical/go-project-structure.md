# Go Project Structure

## Directory Layout

```
personal-assistant/
├── cmd/
│   └── assistant/
│       └── main.go              # Entry point
├── internal/
│   ├── config/
│   │   └── config.go            # YAML config loading
│   ├── transport/
│   │   ├── transport.go         # Transport interface
│   │   └── whatsapp/
│   │       └── whatsapp.go      # whatsmeow implementation
│   ├── intent/
│   │   ├── parser.go            # Intent parser interface
│   │   ├── regex.go             # Regex-based parser (MVP)
│   │   └── types.go             # Intent, Entity types
│   ├── capability/
│   │   ├── handler.go           # Handler interface + router
│   │   ├── calendar/
│   │   │   └── calendar.go      # Calendar capability
│   │   ├── email/
│   │   │   └── email.go         # Email capability
│   │   ├── reminder/
│   │   │   ├── reminder.go      # Reminder capability
│   │   │   └── scheduler.go     # Background reminder scheduler
│   │   └── knowledge/
│   │       └── knowledge.go     # Knowledge base capability
│   ├── integration/
│   │   ├── google/
│   │   │   ├── auth.go          # Google OAuth2 flow
│   │   │   ├── calendar.go      # Google Calendar API client
│   │   │   └── gmail.go         # Gmail API client
│   │   └── store/
│   │       └── sqlite.go        # SQLite operations
│   └── store/
│       ├── store.go             # Database interface
│       ├── sqlite.go            # SQLite implementation
│       └── migrations/
│           ├── 001_initial.sql
│           └── 002_notes_fts.sql
├── data/                        # Runtime data (gitignored)
│   ├── assistant.db             # Application database
│   └── whatsmeow.db            # whatsmeow session store
├── config.yaml                  # Configuration file
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
└── docs/                        # Documentation
```

## Key Interfaces

### Transport

```go
// internal/transport/transport.go
package transport

type Transport interface {
    Start(ctx context.Context) error
    Stop() error
    SetMessageHandler(fn func(ctx context.Context, msg *Message))
    SendMessage(ctx context.Context, recipient string, text string) error
}

type Message struct {
    ID        string
    From      string
    Text      string
    Timestamp time.Time
    Platform  string
    Raw       any
}
```

### Capability Handler

```go
// internal/capability/handler.go
package capability

type Handler interface {
    Name() string
    Match(msg *transport.Message) bool
    Handle(ctx context.Context, msg *transport.Message) (string, error)
}

type Router struct {
    handlers []Handler
}

func (r *Router) Route(ctx context.Context, msg *transport.Message) (string, error) {
    for _, h := range r.handlers {
        if h.Match(msg) {
            return h.Handle(ctx, msg)
        }
    }
    return helpMessage(), nil
}
```

### Intent Parser

```go
// internal/intent/parser.go
package intent

type Parser interface {
    Parse(text string) (*Intent, error)
}

type Intent struct {
    Capability string
    Action     string
    Entities   map[string]string
    Confidence float64
    Raw        string
}
```

### Store

```go
// internal/store/store.go
package store

type Store interface {
    // Reminders
    CreateReminder(r *Reminder) error
    GetDueReminders(now time.Time) ([]*Reminder, error)
    GetActiveReminders() ([]*Reminder, error)
    MarkNotified(id int64) error
    CancelReminder(id int64) error

    // Notes
    CreateNote(n *Note) error
    GetNote(id int64) (*Note, error)
    SearchNotes(query string) ([]*Note, error)
    ListNotes(limit int) ([]*Note, error)
    UpdateNote(n *Note) error
    DeleteNote(id int64) error

    // OAuth Tokens
    GetOAuthToken(service string) ([]byte, error)
    SaveOAuthToken(service string, data []byte) error

    // Message Log
    LogMessage(m *MessageLog) error
}
```

## Conventions

- **No global state** — all dependencies are passed via constructors
- **Error wrapping** — use `fmt.Errorf("operation: %w", err)` for context
- **Context everywhere** — all I/O operations accept `context.Context`
- **Logging** — use `log/slog` with structured fields
- **Testing** — table-driven tests, test files alongside source (`*_test.go`)

## Makefile Targets

```makefile
.PHONY: build run test lint

build:
	go build -o bin/assistant ./cmd/assistant

run:
	go run ./cmd/assistant

test:
	go test ./...

lint:
	golangci-lint run

migrate:
	go run ./cmd/assistant -migrate
```

## Module Name

```
module github.com/irfanmaulana007/personal-assistant
```
