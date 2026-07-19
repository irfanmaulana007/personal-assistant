# Product Requirements Document — Personal Assistant

## Vision

A personal AI assistant accessible via WhatsApp (and later other platforms) that helps manage daily tasks — calendar, email, reminders, notes, and more — through natural conversational commands.

## Target User

- **Single user** (the owner) — this is a personal tool, not a multi-tenant service
- Tech-savvy individual who wants to manage their digital life from a single chat interface
- Primary use: quick actions on-the-go via WhatsApp without switching between apps

## Goals

1. Provide a conversational interface to manage calendar, email, reminders, and notes
2. Run as a self-hosted, always-on service with minimal resource usage
3. Be extensible — easy to add new capabilities and platforms over time
4. Prioritize privacy and security — all data stays on the owner's infrastructure

## Non-Goals

- ~~Multi-user / multi-tenant support~~ — **superseded.** The app now supports
  multiple projects per user with Role-Based Access Control (superadmin / project
  admin / project member) and per-project data isolation. See
  [prd-multi-project-rbac.md](./prd-multi-project-rbac.md).
- Public-facing service or SaaS product
- Real-time voice or video processing
- Replacing full-featured apps (Gmail, Google Calendar) — this is a quick-action layer

---

## MVP Scope (Phase 1)

### Platform
- WhatsApp via [whatsmeow](https://github.com/tulir/whatsmeow) (Go library)

### Capabilities
| Capability | Operations | Priority |
|-----------|-----------|----------|
| **Calendar** | View today/upcoming, create event, update event, delete event | P0 |
| **Email** | Read inbox summary, read specific email, draft reply, search | P0 |
| **Reminders** | Set reminder (relative/absolute time), list, cancel | P0 |
| **Notes / Knowledge Base** | Save note, search notes, list by tag, delete | P1 |

### Integrations
| Service | API | Auth |
|---------|-----|------|
| WhatsApp | whatsmeow | QR code pairing |
| Google Calendar | Calendar API v3 | OAuth2 |
| Gmail | Gmail API | OAuth2 |

### Tech Stack
| Component | Technology |
|-----------|-----------|
| Language | Go |
| WhatsApp client | whatsmeow |
| Database | SQLite |
| Config | YAML + env vars |
| Intent parsing | Keyword/regex (MVP) |

---

## Phased Delivery

### Phase 1 — MVP
- WhatsApp transport (whatsmeow)
- Google Calendar integration (view, create, update, delete)
- Gmail integration (read, summarize, draft — **never auto-send**)
- Reminders with background scheduler
- Notes with full-text search
- Regex-based intent parsing
- Owner JID verification (security)

### Phase 2 — Expand Capabilities
- Web search integration
- Weather lookup
- File/document management (media download, indexing)
- LLM-based intent parsing (Claude API)
- Conversation context / multi-turn interactions

### Phase 3 — Multi-Platform
- Telegram bot transport
- Slack integration
- Discord bot
- CLI interface

### Phase 4 — Advanced
- Smart home control (Home Assistant)
- Finance tracking (expense logging, budget queries)
- Health/fitness data queries
- Proactive suggestions & daily briefings
- Workflow automation (chaining multiple actions)

---

## Architecture Overview

```
┌─────────────┐     ┌──────────────┐     ┌────────────────┐
│  WhatsApp    │────▶│   Message    │────▶│  Capability    │
│  (whatsmeow) │     │   Router     │     │  Handlers      │
└─────────────┘     └──────────────┘     └────────┬───────┘
                          │                        │
                    ┌─────▼─────┐          ┌───────▼───────┐
                    │  Intent   │          │ Integrations  │
                    │  Parser   │          │ (Google, etc) │
                    └───────────┘          └───────────────┘
                                                  │
                                           ┌──────▼──────┐
                                           │   SQLite    │
                                           │   Store     │
                                           └─────────────┘
```

Each capability implements a `Handler` interface:
```go
type Handler interface {
    Match(msg *Message) bool
    Handle(ctx context.Context, msg *Message) (string, error)
}
```

See [Architecture Overview](architecture/overview.md) for details.

---

## Safety & Security

- **Single-user only**: Only respond to the owner's WhatsApp JID
- **Email safety**: Gmail integration is draft-only by default — never auto-send
- **Token security**: OAuth2 tokens encrypted at rest
- **No external data sharing**: All processing happens locally
- **Confirmation for destructive actions**: Delete operations require explicit confirmation

---

## Success Metrics

| Metric | Target |
|--------|--------|
| Message response time | < 3 seconds for local operations |
| Uptime | 99%+ (self-hosted) |
| Supported capabilities | 4+ in MVP |
| False intent matches | < 5% of messages |

---

## Detailed Documentation

### Architecture
- [System Overview](architecture/overview.md)
- [Message Pipeline](architecture/message-pipeline.md)
- [Data Model](architecture/data-model.md)

### Integrations
- [WhatsApp (whatsmeow)](integrations/whatsapp-whatsmeow.md)
- [Google Calendar](integrations/google-calendar.md)
- [Gmail](integrations/gmail.md)
- [Future Platforms](integrations/future-platforms.md)

### Capabilities
- [Calendar Management](capabilities/calendar-management.md)
- [Email Management](capabilities/email-management.md)
- [Reminders & To-Dos](capabilities/reminders-todos.md)
- [Knowledge Base](capabilities/knowledge-base.md)
- [Web Search & Weather](capabilities/web-search-weather.md) *(Phase 2)*
- [File Management](capabilities/file-management.md) *(Phase 2)*
- [Future Capabilities](capabilities/future-capabilities.md) *(Phase 3+)*

### Technical
- [Go Project Structure](technical/go-project-structure.md)
- [NLP & Intent Parsing](technical/nlp-intent-parsing.md)
- [Authentication](technical/authentication.md)
- [Configuration](technical/configuration.md)
- [Deployment](technical/deployment.md)
