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

// Reminder represents a scheduled reminder. Newer reminders use the recurring
// model (RepeatMode + Times + the mode-specific fields); legacy one-shot
// reminders created via chat have Times empty and a populated RemindAt.
type Reminder struct {
	ID          int64
	UserID      int64
	Title       string
	RepeatMode  string   // once | daily | weekly | monthly | specific
	Times       []string // local "HH:MM", sorted ascending
	Weekdays    []int    // 0-6 (0=Sun), weekly only
	DayOfMonth  int      // 1-31, monthly only
	OnceDate    string   // local "YYYY-MM-DD", once only
	EventAt     string   // local "YYYY-MM-DDTHH:MM", the actual event (specific mode; optional otherwise)
	Offsets     []int    // minutes before EventAt to remind, ascending; specific mode only
	Enabled     bool
	LastFiredAt *time.Time // UTC instant of the most-recent slot fired; nil = never

	// Google Calendar mirror bookkeeping (managed by the reconciler).
	CalendarConn     string   // Composio connection the events live on
	CalendarEventIDs []string // one calendar event id per reminder time
	CalendarHash     string   // hash of the mirrored schedule (detects edits)

	// Legacy one-shot fields (retained for chat-created reminders).
	Message   string
	RemindAt  time.Time
	CreatedAt time.Time
	Notified  bool
	Cancelled bool
}

// ReminderInput is the create/update payload for a recurring reminder.
type ReminderInput struct {
	Title      string
	RepeatMode string
	Times      []string
	Weekdays   []int
	DayOfMonth int
	OnceDate   string
	EventAt    string
	Offsets    []int
	Enabled    bool
}

// UserPersona configures the assistant's style for a user (injected into the
// agent's system prompt). Empty/"balanced" fields use the default behavior.
type UserPersona struct {
	Tone        string // formal | balanced | casual
	Emoji       string // none | occasional | frequent
	Length      string // concise | balanced | detailed
	Personality string // balanced | professional | friendly | witty | direct | encouraging
	Name        string // what the assistant calls itself (optional)
	Custom      string // free-text extra instructions
}

// Memory is a durable fact the agent has remembered about a user.
type Memory struct {
	ID        int64
	Content   string
	Kind      string
	CreatedAt time.Time
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

// LifeGoal is a single checklist item on the user's life to-do list (things
// they want to do, like "Take a swimming course"), scoped to a user.
type LifeGoal struct {
	ID          int64
	Title       string
	Description string // an enriched explanation of the goal
	Note        string // a short, optional user annotation
	Done        bool
	CreatedAt   time.Time
	DoneAt      *time.Time // set when Done, nil otherwise
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

// Mountain is a canonical hiking destination, scoped to a user.
type Mountain struct {
	ID   int64
	Name string
}

// HikeTrack is a canonical trail on a mountain, scoped to a user.
type HikeTrack struct {
	ID         int64
	MountainID int64
	Name       string
}

// Hiker is a canonical hiking participant, scoped to a user.
type Hiker struct {
	ID   int64
	Name string
}

// Hike is a logged hiking trip.
type Hike struct {
	ID          int64
	MountainID  int64
	Camped      bool
	UpTrackID   int64 // 0 = none
	DownTrackID int64 // 0 = none
	Days        int
	Nights      int
	HikedOn     time.Time
	CreatedAt   time.Time
}

// HikeDetail is a hike joined with the names it references.
type HikeDetail struct {
	Hike
	Mountain     string
	UpTrack      string
	DownTrack    string
	Participants []string
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
	LatencyMs int    `json:"latency_ms,omitempty"`
}

// LLMCall records one LLM round-trip within a trace.
type LLMCall struct {
	Step             int      `json:"step"`
	Model            string   `json:"model"`
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	TotalTokens      int      `json:"total_tokens"`
	LatencyMs        int      `json:"latency_ms"`
	FinishReason     string   `json:"finish_reason,omitempty"`
	ToolCalls        []string `json:"tool_calls,omitempty"`
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
	Steps            []LLMCall        // per-LLM-call detail; populated by GetTrace only
	Skills           []string         // skill keys active for this run
	Status           string           // "ok" or "error"
	Error            string
	CreatedAt        time.Time
	Score            *TraceScore // LLM-as-judge verdict; nil until judged
}

// TraceScore is an LLM-as-judge quality verdict for a single trace. Each
// dimension is rated 1–5; Overall is their average. A trace has at most one
// score (keyed by TraceID); re-judging overwrites it.
type TraceScore struct {
	TraceID     int64
	Accuracy    int     // did the reply correctly answer / act on the input
	Helpfulness int     // was it useful, complete, well-formed
	Safety      int     // free of harmful, wrong, or fabricated content
	Overall     float64 // average of the three dimensions
	Rationale   string  // the judge's short explanation
	JudgeModel  string  // model that produced this score
	CreatedAt   time.Time
}

// TraceFilter narrows a trace listing. Pagination is cursor-based: Cursor is
// the id of the last trace from the previous page (0 = first page); results are
// ordered by id descending, so the next page is everything with id < Cursor.
type TraceFilter struct {
	Platform string // "" = all
	From     time.Time
	To       time.Time
	Limit    int
	Cursor   int64
	// ScoreState filters by judge verdict: "" = all, "scored", "unscored", or
	// "low" (judged and overall below lowScoreThreshold).
	ScoreState string
}

// LowScoreThreshold is the overall rating below which a judged trace is
// considered "low" and surfaced by the ScoreState="low" filter.
const LowScoreThreshold = 3.0

// UsageTotals aggregates token usage over a period.
type UsageTotals struct {
	Requests         int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// UsageDay is per-day usage for a time series.
type UsageDay struct {
	Date         string // YYYY-MM-DD (UTC)
	Requests     int
	Errors       int
	TotalTokens  int
	AvgLatencyMs int
}

// DayModelUsage is per-day, per-model token usage — used to compute a per-day
// cost series (pricing is applied in the API layer).
type DayModelUsage struct {
	Date             string // YYYY-MM-DD (UTC)
	Model            string
	PromptTokens     int
	CompletionTokens int
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

// UserModelUsage is per-user, per-model usage — used to compute per-user cost
// (pricing is applied in the API layer, like DayModelUsage).
type UserModelUsage struct {
	UserID           int64
	Model            string
	Requests         int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Errors           int
}

// UsageUser is a per-user usage row for the dashboard's Users section (name and
// cost are filled in by the API layer).
type UsageUser struct {
	UserID           int64
	Name             string
	Email            string
	Requests         int
	TotalTokens      int
	Errors           int
	EstimatedCostUSD float64
}

// ModelPrice is a per-model rate (USD per 1M tokens), overriding the built-in
// pricing table for cost estimation.
type ModelPrice struct {
	Model       string
	InputPer1M  float64
	OutputPer1M float64
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
	LatencyP50   int
	LatencyP95   int
	LatencyP99   int
	ToolCalls    int
	Errors       int
	ActiveUsers  int
	ByDay        []UsageDay
	ByModel      []UsageModel
	ByPlatform   []UsagePlatform
	TopTools     []ToolCount
	ByHour       [24]int
	ByWeekday    [7]int
	ByUser       []UsageUser
}

// Translator normalizes user-supplied text into English before it is persisted.
// It is fail-soft (returns the original text on any problem) and optional — when
// none is injected the store persists text exactly as given.
type Translator interface {
	// Title normalizes a title/name to English with proper capitalization.
	Title(ctx context.Context, text string) string
	// Text normalizes free-form text (e.g. a note) to English.
	Text(ctx context.Context, text string) string
}

// Store defines the full persistence interface. It is the union of DataStore
// (operational/main data) and LogStore (append-only logs & usage analytics),
// plus GetUserActivity, which is deliberately in neither sub-interface because
// it spans both — it reads traces (a log) together with reminders and notes
// (main data). A single backend (e.g. *SQLiteStore) implements Store directly;
// a split backend composes a DataStore and a LogStore and supplies
// GetUserActivity by hand (see HybridStore).
type Store interface {
	DataStore
	LogStore

	// GetUserActivity is cross-cutting: it aggregates trace-derived usage (a
	// LogStore concern) with the user's reminders and notes (a DataStore
	// concern). Because no single sub-store owns all of its inputs, it lives
	// only here and a hybrid backend must implement it by fanning out to both.
	GetUserActivity(ctx context.Context, userID int64) (*UserActivity, error)
}

// DataStore is the operational/main-data half of the persistence interface:
// users, skills, reminders, persona, memories, contacts, life goals, hiking,
// travel, notes, OAuth tokens, settings, and model prices. In the hybrid
// backend this is served by PostgreSQL.
type DataStore interface {
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
	CreateReminder(ctx context.Context, userID int64, in ReminderInput) (*Reminder, error)
	GetReminder(ctx context.Context, userID, id int64) (*Reminder, error)
	ListReminders(ctx context.Context, userID int64, activeOnly bool) ([]Reminder, error)
	UpdateReminder(ctx context.Context, userID, id int64, in ReminderInput) error
	DeleteReminder(ctx context.Context, userID, id int64) error
	SetReminderEnabled(ctx context.Context, userID, id int64, enabled bool) error
	ListEnabledForOwner(ctx context.Context, ownerID int64) ([]Reminder, error)
	MarkReminderFired(ctx context.Context, id int64, firedAt time.Time, disable bool) error
	// Calendar-mirror bookkeeping (used by the reconciler).
	ListAllForOwner(ctx context.Context, ownerID int64) ([]Reminder, error)
	HardDeleteReminder(ctx context.Context, id int64) error
	SetReminderCalendar(ctx context.Context, id int64, conn string, eventIDs []string, hash string) error
	ClearReminderCalendar(ctx context.Context, id int64) error
	// Legacy one-shot path (chat-created reminders).
	CreateLegacyReminder(ctx context.Context, userID int64, message string, remindAt time.Time) (*Reminder, error)
	GetDueReminders(ctx context.Context, userID int64) ([]Reminder, error)
	MarkReminderNotified(ctx context.Context, id int64) error
	CancelReminder(ctx context.Context, userID, id int64) error

	// Agent persona (per-user style)
	GetUserPersona(ctx context.Context, userID int64) (UserPersona, error)
	SetUserPersona(ctx context.Context, userID int64, p UserPersona) error

	// Memories (agent long-term memory, scoped to a user; FTS-backed)
	CreateMemory(ctx context.Context, userID int64, content, kind string) (*Memory, error)
	SearchMemories(ctx context.Context, userID int64, ftsQuery string, limit int) ([]Memory, error)
	ListMemories(ctx context.Context, userID int64, limit int) ([]Memory, error)
	DeleteMemory(ctx context.Context, userID, id int64) error

	// Contacts (scoped to a user)
	CreateContact(ctx context.Context, userID int64, name, phone, email, note string) (*Contact, error)
	SearchContacts(ctx context.Context, userID int64, query string) ([]Contact, error)

	// Life goals (a simple life checklist, scoped to a user)
	CreateLifeGoal(ctx context.Context, userID int64, title, description, note string) (*LifeGoal, error)
	ListLifeGoals(ctx context.Context, userID int64) ([]LifeGoal, error)
	GetLifeGoal(ctx context.Context, userID, id int64) (*LifeGoal, error)
	UpdateLifeGoal(ctx context.Context, userID, id int64, title, description, note string) error
	SetLifeGoalDone(ctx context.Context, userID, id int64, done bool) error
	DeleteLifeGoal(ctx context.Context, userID, id int64) error

	// Hiking (scoped to a user; names are canonical for typo-free reuse)
	ListMountains(ctx context.Context, userID int64) ([]Mountain, error)
	CreateMountain(ctx context.Context, userID int64, name string) (*Mountain, error)
	ListTracks(ctx context.Context, userID, mountainID int64) ([]HikeTrack, error)
	CreateTrack(ctx context.Context, userID, mountainID int64, name string) (*HikeTrack, error)
	ListHikers(ctx context.Context, userID int64) ([]Hiker, error)
	CreateHiker(ctx context.Context, userID int64, name string) (*Hiker, error)
	CreateHike(ctx context.Context, userID int64, h *Hike) (int64, error)
	AddHikeParticipant(ctx context.Context, hikeID, hikerID int64) error
	ListHikes(ctx context.Context, userID int64, limit int) ([]HikeDetail, error)

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

	// OAuth tokens
	SaveToken(ctx context.Context, service string, tokenData []byte) error
	GetToken(ctx context.Context, service string) ([]byte, error)

	// Settings (key/value; values may be encrypted by the caller)
	GetSetting(ctx context.Context, key string) ([]byte, error)
	SetSetting(ctx context.Context, key string, value []byte) error
	GetAllSettings(ctx context.Context) (map[string][]byte, error)

	// Model prices (per-model cost overrides)
	ListModelPrices(ctx context.Context) ([]ModelPrice, error)
	UpsertModelPrice(ctx context.Context, p ModelPrice) error
	DeleteModelPrice(ctx context.Context, model string) error

	// Lifecycle
	Close() error
}

// LogStore is the append-only-logs half of the persistence interface: message
// logs, agent traces, tool-usage records, trace scores, activities, and the
// usage-analytics aggregates computed over them. These are write-mostly, have
// no foreign keys into the main data, and are read back only by user/time
// filters — in the hybrid backend this is served by MongoDB.
type LogStore interface {
	// Activities (scoped to a user)
	CreateActivity(ctx context.Context, userID int64, actType, description string, occurredAt time.Time, source string) (*Activity, error)
	ListActivitiesSince(ctx context.Context, userID int64, since time.Time) ([]Activity, error)

	// Message log (scoped to a user)
	LogMessage(ctx context.Context, log *MessageLog) error
	GetMessageHistory(ctx context.Context, userID int64, platform string, limit int) ([]MessageLog, error)

	// Traces & usage (source of truth for dashboard + logs)
	CreateTrace(ctx context.Context, t *Trace) (int64, error)
	ListTraces(ctx context.Context, f TraceFilter) ([]Trace, error)
	GetTrace(ctx context.Context, id int64) (*Trace, error)
	LogToolUsage(ctx context.Context, userID int64, tool, platform string) error

	// Trace scores (LLM-as-judge quality signal).
	SaveTraceScore(ctx context.Context, sc *TraceScore) error
	GetTraceScore(ctx context.Context, traceID int64) (*TraceScore, error)
	ListUnscoredTraces(ctx context.Context, since time.Time, limit int) ([]Trace, error)

	// Usage analytics (aggregates over traces & tool_usage)
	UsageStatsBetween(ctx context.Context, from, to time.Time, platform string) (*UsageStats, error)
	UsageByDayModel(ctx context.Context, from, to time.Time, platform string) ([]DayModelUsage, error)
	UsageByUserModel(ctx context.Context, from, to time.Time, platform string) ([]UserModelUsage, error)

	// Lifecycle
	Close() error
}
