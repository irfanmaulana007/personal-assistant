package store

import (
	"context"
	"time"
)

// Reminder represents a scheduled reminder.
type Reminder struct {
	ID        int64
	Message   string
	RemindAt  time.Time
	CreatedAt time.Time
	Notified  bool
	Cancelled bool
}

// Note represents a saved note.
type Note struct {
	ID        int64
	Title     string
	Content   string
	Tags      string // comma-separated
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MessageLog records a processed message for auditing.
type MessageLog struct {
	ID        int64
	Platform  string
	Direction string // "in" or "out"
	Sender    string
	Body      string
	Intent    string
	Action    string
	CreatedAt time.Time
}

// OAuthToken stores encrypted OAuth2 tokens.
type OAuthToken struct {
	Service   string
	TokenData []byte // encrypted
	UpdatedAt time.Time
}

// LLMUsage records token usage for a single LLM turn.
type LLMUsage struct {
	ID               int64
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Platform         string
	CreatedAt        time.Time
}

// Store defines the persistence interface.
type Store interface {
	// Reminders
	CreateReminder(ctx context.Context, message string, remindAt time.Time) (*Reminder, error)
	ListReminders(ctx context.Context, activeOnly bool) ([]Reminder, error)
	GetDueReminders(ctx context.Context) ([]Reminder, error)
	MarkReminderNotified(ctx context.Context, id int64) error
	CancelReminder(ctx context.Context, id int64) error

	// Notes
	CreateNote(ctx context.Context, title, content, tags string) (*Note, error)
	GetNote(ctx context.Context, id int64) (*Note, error)
	UpdateNote(ctx context.Context, id int64, title, content, tags string) error
	DeleteNote(ctx context.Context, id int64) error
	ListNotes(ctx context.Context, tag string) ([]Note, error)
	SearchNotes(ctx context.Context, query string) ([]Note, error)

	// Message log
	LogMessage(ctx context.Context, log *MessageLog) error
	GetMessageHistory(ctx context.Context, platform string, limit int) ([]MessageLog, error)

	// OAuth tokens
	SaveToken(ctx context.Context, service string, tokenData []byte) error
	GetToken(ctx context.Context, service string) ([]byte, error)

	// Settings (key/value; values may be encrypted by the caller)
	GetSetting(ctx context.Context, key string) ([]byte, error)
	SetSetting(ctx context.Context, key string, value []byte) error
	GetAllSettings(ctx context.Context) (map[string][]byte, error)

	// LLM usage
	LogUsage(ctx context.Context, usage *LLMUsage) error

	// Lifecycle
	Close() error
}
