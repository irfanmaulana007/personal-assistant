# Data Model

## Database

SQLite is used for all local persistence. Single file, zero configuration, sufficient for single-user workloads.

Database file: `data/assistant.db`

## Tables

### reminders

Stores scheduled reminders with their trigger times.

```sql
CREATE TABLE reminders (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    message     TEXT NOT NULL,
    remind_at   DATETIME NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    notified    BOOLEAN NOT NULL DEFAULT FALSE,
    cancelled   BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_reminders_remind_at ON reminders(remind_at) WHERE notified = FALSE AND cancelled = FALSE;
```

### notes

Personal knowledge base with full-text search support.

```sql
CREATE TABLE notes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    tags        TEXT,  -- comma-separated tags
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Full-text search index
CREATE VIRTUAL TABLE notes_fts USING fts5(title, content, tags, content=notes, content_rowid=id);
```

### message_log

Optional audit log for debugging and analytics.

```sql
CREATE TABLE message_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    platform    TEXT NOT NULL,       -- "whatsapp", "telegram", etc.
    direction   TEXT NOT NULL,       -- "incoming" or "outgoing"
    sender      TEXT NOT NULL,
    body        TEXT NOT NULL,
    intent      TEXT,               -- parsed intent capability
    action      TEXT,               -- parsed intent action
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_message_log_created ON message_log(created_at);
```

### oauth_tokens

Encrypted OAuth2 token storage for Google APIs.

```sql
CREATE TABLE oauth_tokens (
    service     TEXT PRIMARY KEY,    -- "google_calendar", "gmail"
    token_data  BLOB NOT NULL,       -- encrypted JSON token
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

## Go Structs

### Reminder

```go
type Reminder struct {
    ID        int64     `db:"id"`
    Message   string    `db:"message"`
    RemindAt  time.Time `db:"remind_at"`
    CreatedAt time.Time `db:"created_at"`
    Notified  bool      `db:"notified"`
    Cancelled bool      `db:"cancelled"`
}
```

### Note

```go
type Note struct {
    ID        int64     `db:"id"`
    Title     string    `db:"title"`
    Content   string    `db:"content"`
    Tags      []string  // stored as comma-separated in DB
    CreatedAt time.Time `db:"created_at"`
    UpdatedAt time.Time `db:"updated_at"`
}
```

### MessageLog

```go
type MessageLog struct {
    ID        int64     `db:"id"`
    Platform  string    `db:"platform"`
    Direction string    `db:"direction"`
    Sender    string    `db:"sender"`
    Body      string    `db:"body"`
    Intent    string    `db:"intent"`
    Action    string    `db:"action"`
    CreatedAt time.Time `db:"created_at"`
}
```

## Intent Enum

```go
const (
    CapabilityCalendar  = "calendar"
    CapabilityEmail     = "email"
    CapabilityReminder  = "reminder"
    CapabilityKnowledge = "knowledge"
    CapabilitySearch    = "search"    // Phase 2
    CapabilityWeather   = "weather"   // Phase 2
    CapabilityFile      = "file"      // Phase 2
)

const (
    ActionList   = "list"
    ActionGet    = "get"
    ActionCreate = "create"
    ActionUpdate = "update"
    ActionDelete = "delete"
    ActionSearch = "search"
)
```

## Migration Strategy

- Migrations stored in `internal/store/migrations/` as numbered SQL files
- Applied automatically on startup via a simple migration runner
- Example: `001_initial.sql`, `002_add_notes_fts.sql`
