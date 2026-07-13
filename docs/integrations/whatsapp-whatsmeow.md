# WhatsApp Integration — whatsmeow

## Overview

[whatsmeow](https://github.com/tulir/whatsmeow) is a Go library that implements the WhatsApp Web API. It allows the assistant to send and receive WhatsApp messages programmatically.

## Setup

### Dependencies

```go
import (
    "go.mau.fi/whatsmeow"
    "go.mau.fi/whatsmeow/store/sqlstore"
    "go.mau.fi/whatsmeow/types/events"
    waProto "go.mau.fi/whatsmeow/binary/proto"
    waLog "go.mau.fi/whatsmeow/util/log"
)
```

### Initialization Flow

1. Create the whatsmeow store (backed by PostgreSQL) for session persistence
2. Create whatsmeow client
3. If no session exists, display QR code for pairing
4. Connect and start listening for events

```go
// 1. Session store — PostgreSQL via the pgx driver (shares the app database;
//    whatsmeow keeps its own whatsmeow_* tables).
dbLog := waLog.Stdout("Database", "WARN", true)
container, err := sqlstore.New(ctx, "pgx", postgresDSN, dbLog)
device, err := container.GetFirstDevice(ctx)

// 2. Client
clientLog := waLog.Stdout("Client", "INFO", true)
client := whatsmeow.NewClient(device, clientLog)
client.AddEventHandler(eventHandler)

// 3. Connect (QR pairing if needed)
if client.Store.ID == nil {
    qrChan, _ := client.GetQRChannel(context.Background())
    err = client.Connect()
    for evt := range qrChan {
        if evt.Event == "code" {
            // Display QR code in terminal
            qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
        }
    }
} else {
    err = client.Connect()
}
```

### QR Code Pairing

- On first run, a QR code is displayed in the terminal
- User scans the QR code with WhatsApp on their phone
- Session is persisted in PostgreSQL — subsequent runs reconnect automatically
- If session expires, a new QR code is generated

## Event Handling

### Message Types

| Type | whatsmeow Event | Support |
|------|----------------|---------|
| Text message | `*events.Message` with `msg.Message.GetConversation()` | MVP |
| Extended text | `msg.Message.GetExtendedTextMessage()` | MVP |
| Image | `msg.Message.GetImageMessage()` | Phase 2 |
| Document | `msg.Message.GetDocumentMessage()` | Phase 2 |
| Location | `msg.Message.GetLocationMessage()` | Phase 2 |

### Event Handler

```go
func eventHandler(evt interface{}) {
    switch v := evt.(type) {
    case *events.Message:
        handleMessage(v)
    case *events.Connected:
        slog.Info("connected to WhatsApp")
    case *events.Disconnected:
        slog.Warn("disconnected from WhatsApp")
    case *events.LoggedOut:
        slog.Error("logged out, need to re-pair")
    }
}

func handleMessage(msg *events.Message) {
    // Extract text from message
    text := msg.Message.GetConversation()
    if text == "" {
        ext := msg.Message.GetExtendedTextMessage()
        if ext != nil {
            text = ext.GetText()
        }
    }
    if text == "" {
        return // skip non-text messages in MVP
    }

    // Build platform-agnostic message
    message := &transport.Message{
        ID:        msg.Info.ID,
        From:      msg.Info.Sender.String(),
        Text:      text,
        Timestamp: msg.Info.Timestamp,
        Platform:  "whatsapp",
        Raw:       msg,
    }

    // Dispatch to router
    // ...
}
```

## Sending Messages

### Text Response

```go
func sendText(client *whatsmeow.Client, to types.JID, text string) error {
    _, err := client.SendMessage(context.Background(), to, &waProto.Message{
        Conversation: proto.String(text),
    })
    return err
}
```

### Formatted Text (Bold, Italic, Monospace)

WhatsApp supports basic formatting:
- `*bold*`
- `_italic_`
- `~strikethrough~`
- `` `monospace` ``
- ```` ```code block``` ````

Use these in response formatting for readability.

## Session Persistence

- whatsmeow stores its session in PostgreSQL (its own `whatsmeow_*` tables in the
  app's Postgres database, via the `pgx` driver)
- Session survives restarts — no need to re-pair unless explicitly logged out
- Enable WAL mode for better concurrent access

## Reconnection & Error Handling

```go
client.AddEventHandler(func(evt interface{}) {
    switch evt.(type) {
    case *events.Disconnected:
        // whatsmeow handles automatic reconnection
        slog.Warn("disconnected, will auto-reconnect")
    case *events.LoggedOut:
        // Session invalidated, need manual re-pairing
        slog.Error("session invalidated, restart and re-pair")
        os.Exit(1)
    case *events.StreamError:
        slog.Error("stream error", "error", evt)
    }
})
```

## Rate Limiting

WhatsApp enforces rate limits. Best practices:
- Add small delays between consecutive messages (200-500ms)
- Avoid sending more than ~50 messages per minute
- Use typing indicators (`client.SendChatPresence`) for better UX

## Security Considerations

- Only process messages from the owner's JID (configured in YAML)
- Never forward or store message content to external services (except configured integrations)
- whatsmeow session database contains encryption keys — protect it accordingly
