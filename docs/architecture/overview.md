# Architecture Overview

## System Components

```
┌─────────────────────────────────────────────────────────────┐
│                      Personal Assistant                      │
│                                                              │
│  ┌───────────┐   ┌───────────┐   ┌────────────────────────┐ │
│  │ Transport  │──▶│  Intent   │──▶│   Capability Handlers  │ │
│  │  Layer     │   │  Router   │   │                        │ │
│  │           │   │           │   │  ┌──────────────────┐  │ │
│  │ WhatsApp  │   │ Regex/    │   │  │ Calendar Handler │  │ │
│  │ Telegram* │   │ LLM*      │   │  │ Email Handler    │  │ │
│  │ Slack*    │   │           │   │  │ Reminder Handler │  │ │
│  │ CLI*      │   │           │   │  │ Knowledge Handler│  │ │
│  └───────────┘   └───────────┘   │  └──────────────────┘  │ │
│                                   └───────────┬────────────┘ │
│                                               │              │
│  ┌────────────────────────┐   ┌───────────────▼────────────┐ │
│  │     Config Manager     │   │     Integration Layer      │ │
│  │                        │   │                            │ │
│  │  YAML + env vars       │   │  Google Calendar API       │ │
│  │  Owner JID             │   │  Gmail API                 │ │
│  │  OAuth2 credentials    │   │  SQLite (local store)      │ │
│  └────────────────────────┘   └────────────────────────────┘ │
│                                                              │
│  * = future phases                                           │
└─────────────────────────────────────────────────────────────┘
```

## Tech Stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| Language | Go 1.22+ | Performance, single binary, strong concurrency |
| WhatsApp | whatsmeow | Most mature Go library for WhatsApp Web API |
| Database | SQLite (via modernc.org/sqlite) | Zero-config, embedded, CGo-free option available |
| Google APIs | google.golang.org/api | Official Go client libraries |
| Config | gopkg.in/yaml.v3 | Simple, human-readable configuration |
| Logging | log/slog | Structured logging (stdlib) |

## Core Design Patterns

### Handler Interface

Every capability implements the `Handler` interface:

```go
package capability

type Handler interface {
    // Name returns the capability identifier (e.g., "calendar", "email")
    Name() string

    // Match returns true if this handler can process the message
    Match(msg *Message) bool

    // Handle processes the message and returns a text response
    Handle(ctx context.Context, msg *Message) (string, error)
}
```

The router iterates through registered handlers and dispatches to the first match. This pattern makes adding new capabilities mechanical — implement the interface, register it.

### Transport Abstraction

Transports implement a common interface to decouple platform-specific logic:

```go
package transport

type Transport interface {
    // Start begins listening for messages
    Start(ctx context.Context) error

    // Stop gracefully shuts down the transport
    Stop() error

    // SetMessageHandler registers the callback for incoming messages
    SetMessageHandler(fn func(ctx context.Context, msg *Message))

    // SendMessage sends a response back to the user
    SendMessage(ctx context.Context, recipient string, text string) error
}
```

### Message Model

A platform-agnostic message struct flows through the system:

```go
type Message struct {
    ID        string
    From      string    // sender JID / user ID
    Text      string    // message body
    Timestamp time.Time
    Platform  string    // "whatsapp", "telegram", etc.
    Raw       any       // original platform-specific event
}
```

## Key Principles

1. **Single responsibility**: Each package does one thing
2. **Dependency injection**: Handlers receive integrations via constructor, not globals
3. **Graceful degradation**: If an integration is unavailable, respond with a clear error rather than crashing
4. **Context propagation**: All operations accept `context.Context` for cancellation and timeouts
5. **No global state**: Configuration and dependencies are explicitly passed
