# Message Pipeline

## Overview

Every incoming message follows this pipeline:

```
Receive → Verify Owner → Parse Intent → Route → Execute → Respond
```

## Pipeline Steps

### 1. Receive Message

The transport layer (e.g., whatsmeow) receives a raw message event and converts it into the platform-agnostic `Message` struct.

```
WhatsApp Event → transport.Message{From, Text, Timestamp, Platform}
```

### 2. Verify Owner

Security check: only process messages from the configured owner JID. All other messages are silently ignored.

```go
if msg.From != config.OwnerJID {
    return // ignore
}
```

This is the primary security boundary. Since this is a personal assistant, there is no multi-user access control — just a single owner check.

### 3. Parse Intent

The intent parser analyzes the message text and produces an `Intent`:

```go
type Intent struct {
    Capability string            // e.g., "calendar", "email", "reminder"
    Action     string            // e.g., "list", "create", "delete"
    Entities   map[string]string // extracted parameters (date, subject, etc.)
    Confidence float64           // 0.0 - 1.0
    Raw        string            // original message text
}
```

**Phase 1**: Regex/keyword matching (fast, no external dependency)
**Phase 2**: LLM-based parsing via Claude API (better understanding, multi-turn context)

### 4. Route to Handler

The router iterates through registered capability handlers:

```go
for _, handler := range handlers {
    if handler.Match(msg) {
        response, err := handler.Handle(ctx, msg)
        // ...
    }
}
```

If no handler matches, return a help message listing available commands.

### 5. Execute

The matched handler:
1. Extracts parameters from the parsed intent
2. Calls the relevant integration (Google Calendar API, Gmail API, SQLite, etc.)
3. Formats the result into a human-readable response

### 6. Respond

The transport layer sends the response back to the user via the same platform.

```go
transport.SendMessage(ctx, msg.From, response)
```

## Error Handling

| Error Type | Behavior |
|-----------|----------|
| Intent not recognized | Reply with help text / available commands |
| Missing required parameter | Ask user for the missing information |
| Integration API error | Reply with friendly error message, log details |
| Timeout | Reply "Request timed out, please try again" |
| Panic recovery | Catch panics in handler, reply with generic error |

## Message Flow Diagram

```
User sends WhatsApp message
        │
        ▼
┌─────────────────┐
│ whatsmeow event │
│ handler          │
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌──────────────┐
│ Owner JID check │──No─▶│ Ignore       │
└────────┬────────┘     └──────────────┘
         │ Yes
         ▼
┌─────────────────┐
│ Intent Parser   │
│ (regex / LLM)   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌──────────────┐
│ Route to Handler│──No─▶│ Help message │
└────────┬────────┘     └──────────────┘
         │ Matched
         ▼
┌─────────────────┐
│ Execute Handler │
│ (call APIs)     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Send response   │
│ via WhatsApp    │
└─────────────────┘
```

## Logging

Every message is logged (with PII redacted in production):

```go
slog.Info("message received",
    "from", msg.From,
    "platform", msg.Platform,
    "intent", intent.Capability,
    "action", intent.Action,
)
```

Optional: persist to `message_log` table for debugging and analytics.
