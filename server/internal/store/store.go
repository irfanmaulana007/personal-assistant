package store

import (
	"context"
	"time"
)

// User is an account that can sign in.
type User struct {
	ID           int64
	Email        string
	Name         string
	PasswordHash string
	Role         string // "admin" or "member"
	CreatedAt    time.Time
}

// Skill is a master-data capability that can be enabled per user.
type Skill struct {
	ID             int64
	Key            string
	Name           string
	Description    string
	Prompt         string
	Category       string
	DefaultEnabled bool
	SortOrder      int
}

// UserSkill is a skill together with its effective enabled state for a user.
type UserSkill struct {
	Skill
	Enabled bool
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

// Contact is a saved contact, scoped to a user.
type Contact struct {
	ID        int64
	Name      string
	Phone     string
	Email     string
	Note      string
	CreatedAt time.Time
}

// Activity is a logged sport/workout session, scoped to a user.
type Activity struct {
	ID          int64
	Type        string
	Description string
	OccurredAt  time.Time
	Source      string // "chat", "reminder", "travel"
	CreatedAt   time.Time
}

// Trip is a travel trip whose expenses are tracked, scoped to a user.
type Trip struct {
	ID          int64
	Name        string
	Destination string
	Budget      float64
	Currency    string
	Active      bool
	StartedAt   time.Time
}

// TripExpense is a single expense logged against a trip.
type TripExpense struct {
	ID       int64
	TripID   int64
	Amount   float64
	Currency string
	Category string
	Note     string
	SpentAt  time.Time
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

// ToolInvocation is one tool call the agent made during a run.
type ToolInvocation struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Result    string `json:"result"`
}

// Trace is a full record of one agent run — the source of truth for both the
// dashboard aggregates and the logs viewer.
type Trace struct {
	ID               int64
	UserID           int64
	Platform         string
	Input            string
	Output           string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	LatencyMs        int
	ToolCount        int
	Tools            []ToolInvocation // populated by GetTrace only
	Status           string           // "ok" or "error"
	Error            string
	CreatedAt        time.Time
}

// TraceFilter narrows a trace listing.
type TraceFilter struct {
	Platform string // "" = all
	From     time.Time
	To       time.Time
	Limit    int
	Offset   int
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

// UserActivity summarizes a single user's own data.
type UserActivity struct {
	Runs        int
	TotalTokens int
	Reminders   int // active
	Notes       int
}

// UsageStats is the combined usage report over a period.
type UsageStats struct {
	Summary      UsageTotals
	AvgLatencyMs int
	ToolCalls    int
	Errors       int
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
	UpdateUserProfile(ctx context.Context, id int64, name, email string) error
	DeleteUser(ctx context.Context, id int64) error
	FirstAdmin(ctx context.Context) (*User, error)

	// Skills (master data + per-user enable/disable)
	ListSkills(ctx context.Context) ([]Skill, error)
	GetSkill(ctx context.Context, id int64) (*Skill, error)
	ListUserSkills(ctx context.Context, userID int64) ([]UserSkill, error)
	SetSkillEnabled(ctx context.Context, userID, skillID int64, enabled bool) error
	EnabledSkillKeys(ctx context.Context, userID int64) ([]string, error)

	// Reminders (scoped to a user; scheduler passes the owner's id)
	CreateReminder(ctx context.Context, userID int64, message string, remindAt time.Time) (*Reminder, error)
	ListReminders(ctx context.Context, userID int64, activeOnly bool) ([]Reminder, error)
	GetDueReminders(ctx context.Context, userID int64) ([]Reminder, error)
	MarkReminderNotified(ctx context.Context, id int64) error
	CancelReminder(ctx context.Context, userID, id int64) error

	// Contacts (scoped to a user)
	CreateContact(ctx context.Context, userID int64, name, phone, email, note string) (*Contact, error)
	SearchContacts(ctx context.Context, userID int64, query string) ([]Contact, error)

	// Activities (scoped to a user)
	CreateActivity(ctx context.Context, userID int64, actType, description string, occurredAt time.Time, source string) (*Activity, error)
	ListActivitiesSince(ctx context.Context, userID int64, since time.Time) ([]Activity, error)

	// Travel (scoped to a user)
	CreateTrip(ctx context.Context, userID int64, name, destination, currency string, budget float64) (*Trip, error)
	ActiveTrip(ctx context.Context, userID int64) (*Trip, error)
	FindTrip(ctx context.Context, userID int64, name string) (*Trip, error)
	AddExpense(ctx context.Context, userID, tripID int64, amount float64, currency, category, note string, spentAt time.Time) (*TripExpense, error)
	ListTripExpenses(ctx context.Context, userID, tripID int64) ([]TripExpense, error)

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

	// Traces & usage (source of truth for dashboard + logs)
	CreateTrace(ctx context.Context, t *Trace) (int64, error)
	ListTraces(ctx context.Context, f TraceFilter) ([]Trace, error)
	GetTrace(ctx context.Context, id int64) (*Trace, error)
	LogToolUsage(ctx context.Context, userID int64, tool, platform string) error
	UsageStatsBetween(ctx context.Context, from, to time.Time, platform string) (*UsageStats, error)
	GetUserActivity(ctx context.Context, userID int64) (*UserActivity, error)

	// Lifecycle
	Close() error
}
