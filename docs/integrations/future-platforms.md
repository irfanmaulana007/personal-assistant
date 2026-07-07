# Future Platform Integrations

## Transport Interface

All messaging platforms implement the same `Transport` interface (defined in [Architecture Overview](../architecture/overview.md)):

```go
type Transport interface {
    Start(ctx context.Context) error
    Stop() error
    SetMessageHandler(fn func(ctx context.Context, msg *Message))
    SendMessage(ctx context.Context, recipient string, text string) error
}
```

This decoupling means adding a new platform requires only implementing these 4 methods. No changes to capability handlers or business logic.

## Planned Platforms

### Telegram Bot (Phase 3)

- Library: [telebot](https://github.com/tucnak/telebot) or [telegram-bot-api](https://github.com/go-telegram-bot-api/telegram-bot-api)
- Auth: Bot token from [@BotFather](https://t.me/BotFather)
- Advantages: rich formatting (Markdown), inline keyboards, no QR pairing
- Owner verification: Telegram user ID check

### Slack (Phase 3)

- Library: [slack-go](https://github.com/slack-go/slack)
- Auth: Slack App with Bot Token
- Use case: work-related assistant in personal Slack workspace
- Socket Mode for development, Events API for production

### Discord (Phase 3)

- Library: [discordgo](https://github.com/bwmarrin/discordgo)
- Auth: Bot token from Discord Developer Portal
- Use case: personal server with assistant bot
- Slash commands for structured interaction

### CLI (Phase 3)

- No external library needed — standard input/output
- Useful for testing and development
- Interactive REPL mode

## Multi-Transport Architecture

When multiple transports are active:

```go
func main() {
    router := capability.NewRouter(handlers...)

    // Start all configured transports
    transports := []transport.Transport{}

    if config.WhatsApp.Enabled {
        wa := whatsapp.New(config.WhatsApp)
        wa.SetMessageHandler(router.Handle)
        transports = append(transports, wa)
    }

    if config.Telegram.Enabled {
        tg := telegram.New(config.Telegram)
        tg.SetMessageHandler(router.Handle)
        transports = append(transports, tg)
    }

    // Start all transports concurrently
    for _, t := range transports {
        go t.Start(ctx)
    }
}
```

Each transport runs independently. The same router and capability handlers serve all platforms.
