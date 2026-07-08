package store

import (
	"context"
	"time"
)

// User is an account that can sign in.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         string // "admin" or "member"
	CreatedAt    time.Time
}

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
	UserID    int64
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
	UserID           int64
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	LatencyMs        int
	ToolCalls        int
	Platform         string
	CreatedAt        time.Time
}

// UsageTotals aggregates token usage over a period.
type UsageTotals struct {
	Requests         int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// UsageDay is per-day usage for a time series.
type UsageDay struct {
	Date        string // YYYY-MM-DD (UTC)
	Requests    int
	TotalTokens int
}

// UsageModel is per-model usage for a breakdown.
type UsageModel struct {
	Model            string
	Requests         int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// UsagePlatform is per-platform usage (web, whatsapp).
type UsagePlatform struct {
	Platform    string
	Requests    int
	TotalTokens int
}

// ToolCount is how many times a tool was invoked.
type ToolCount struct {
	Tool  string
	Count int
}

// UsageStats is the combined usage report over a period.
type UsageStats struct {
	Summary      UsageTotals
	AvgLatencyMs int
	ToolCalls    int
	ByDay        []UsageDay
	ByModel      []UsageModel
	ByPlatform   []UsagePlatform
	TopTools     []ToolCount
}

// Store defines the persistence interface.
type Store interface {
	// Users
	CountUsers(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, email, passwordHash, role string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id int64) (*User, error)
	ListUsers(ctx context.Context) ([]User, error)
	UpdateUserRole(ctx context.Context, id int64, role string) error
	UpdateUserPassword(ctx context.Context, id int64, passwordHash string) error
	DeleteUser(ctx context.Context, id int64) error
	FirstAdmin(ctx context.Context) (*User, error)

	// Reminders (scoped to a user; scheduler passes the owner's id)
	CreateReminder(ctx context.Context, userID int64, message string, remindAt time.Time) (*Reminder, error)
	ListReminders(ctx context.Context, userID int64, activeOnly bool) ([]Reminder, error)
	GetDueReminders(ctx context.Context, userID int64) ([]Reminder, error)
	MarkReminderNotified(ctx context.Context, id int64) error
	CancelReminder(ctx context.Context, userID, id int64) error

	// Notes (scoped to a user)
	CreateNote(ctx context.Context, userID int64, title, content, tags string) (*Note, error)
	GetNote(ctx context.Context, userID, id int64) (*Note, error)
	UpdateNote(ctx context.Context, userID, id int64, title, content, tags string) error
	DeleteNote(ctx context.Context, userID, id int64) error
	ListNotes(ctx context.Context, userID int64, tag string) ([]Note, error)
	SearchNotes(ctx context.Context, userID int64, query string) ([]Note, error)

	// Message log (scoped to a user)
	LogMessage(ctx context.Context, log *MessageLog) error
	GetMessageHistory(ctx context.Context, userID int64, platform string, limit int) ([]MessageLog, error)

	// OAuth tokens
	SaveToken(ctx context.Context, service string, tokenData []byte) error
	GetToken(ctx context.Context, service string) ([]byte, error)

	// Settings (key/value; values may be encrypted by the caller)
	GetSetting(ctx context.Context, key string) ([]byte, error)
	SetSetting(ctx context.Context, key string, value []byte) error
	GetAllSettings(ctx context.Context) (map[string][]byte, error)

	// LLM usage
	LogUsage(ctx context.Context, usage *LLMUsage) error
	LogToolUsage(ctx context.Context, userID int64, tool, platform string) error
	UsageStatsBetween(ctx context.Context, from, to time.Time) (*UsageStats, error)

	// Lifecycle
	Close() error
}
