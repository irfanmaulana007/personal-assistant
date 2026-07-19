package store

import (
	"context"
	"strings"
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
	Prompt         string // shipped default; re-seeded from code on every boot
	TunedPrompt    string // end-of-day self-tuner's override; empty ⇒ use Prompt
	Category       string
	DefaultEnabled bool
	SortOrder      int
	// PromptUpdatedAt / PromptUpdatedBy track the last admin edit of Prompt.
	// PromptUpdatedAt is nil while the prompt is still the code-owned default
	// (managed by the boot seed); once an admin customizes it, both are set and
	// the seed stops clobbering the prompt.
	PromptUpdatedAt *time.Time
	PromptUpdatedBy string
	// ProjectID scopes the skill. nil ⇒ a global (code-seeded) skill shared by
	// every project. Set ⇒ a project-owned skill: a fork a project admin created
	// from a global skill and customized, which shadows the global skill of the
	// same key for that project.
	ProjectID *int64
	// IsCore marks a global skill as "core": auto-available to every project and
	// always classified as core (never demoted to project-specific), though still
	// toggleable per project. Superadmin-managed. Always false for project forks.
	IsCore bool
}

// SkillProjectRef identifies a project that a skill maps to, for the superadmin
// skills catalog (which projects effectively enable a given skill).
type SkillProjectRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// SkillWithMapping is a skill plus the projects that effectively enable it, used
// by the platform-wide (superadmin) skills catalog to derive each skill's
// classification (core / global / project-specific) from its project mapping.
type SkillWithMapping struct {
	Skill
	Projects []SkillProjectRef
}

// IsProjectOwned reports whether the skill is a project-owned fork (as opposed
// to a shared, code-seeded global skill).
func (s Skill) IsProjectOwned() bool { return s.ProjectID != nil }

// EffectivePrompt is the prompt actually injected into the system prompt: the
// auto-tuned override when the self-tuner has set one, otherwise the shipped
// default. Clearing TunedPrompt reverts the skill to its default.
func (s Skill) EffectivePrompt() string {
	if s.TunedPrompt != "" {
		return s.TunedPrompt
	}
	return s.Prompt
}

// UserSkill is a skill together with its effective enabled state for a user (or,
// via the project-scoped methods, for a project).
type UserSkill struct {
	Skill
	Enabled bool
}

// Role constants. Global roles live on users.role; project roles live on
// project_members.role. "admin"/"member" are project-scoped; "superadmin" is the
// global god-role.
const (
	GlobalRoleSuperadmin = "superadmin"
	GlobalRoleMember     = "member"
	ProjectRoleAdmin     = "admin"
	ProjectRoleMember    = "member"
)

// Project is a workspace that scopes domain data, skills, features, and
// membership. Every user-owned record belongs to exactly one project.
type Project struct {
	ID          int64
	Name        string
	Slug        string // immutable, URL-safe; generated from Name at creation
	OwnerUserID int64
	CreatedAt   time.Time
}

// ProjectSummary is a project plus the caller's role in it and its member count,
// for the projects dashboard / switcher.
type ProjectSummary struct {
	Project
	Role        string // caller's project role ("admin"|"member"), or "superadmin"
	MemberCount int
}

// ProjectMemberDetail joins a membership with the member's identity, for the
// members list UI.
type ProjectMemberDetail struct {
	UserID    int64
	Email     string
	Name      string
	Role      string
	CreatedAt time.Time
}

// Feature is a nav/menu module. A feature owns zero or more skills; disabling a
// feature for a project disables all of its skills there.
type Feature struct {
	ID             int64
	Key            string
	Name           string
	Description    string
	SortOrder      int
	DefaultEnabled bool
}

// ProjectFeature is a feature with its effective enabled state for a project and
// the keys of the skills attached to it.
type ProjectFeature struct {
	Feature
	Enabled   bool
	SkillKeys []string
}

// WhatsAppMapping maps a WhatsApp identity (a group JID or a personal
// phone/JID) to the project and role the agent acts as for messages from it.
type WhatsAppMapping struct {
	ID        int64
	JID       string
	Kind      string // "group" | "personal"
	ProjectID int64
	Role      string // agent's role for this chat ("superadmin" allowed for personal only)
	UserID    int64  // user identity attributed to this chat (personal), 0 if none
	Label     string
	CreatedAt time.Time
}

// AuditEvent is a project-level action record (create, invite, skill toggle, …),
// stored append-only in the log store.
type AuditEvent struct {
	ID          int64
	ProjectID   int64
	ActorUserID int64
	ActorEmail  string
	Action      string
	Target      string
	Metadata    string
	CreatedAt   time.Time
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

// BucketItem is a single entry on the user's bucket list (things they want to
// do, like "Take a swimming course"), scoped to a user. Each item belongs to a
// category and can optionally be flagged as a resolution for a given year.
type BucketItem struct {
	ID             int64
	Title          string
	Description    string // an enriched explanation of the item
	Note           string // a short, optional user annotation
	Category       string // one of the known category keys; defaults to CategoryOther
	ResolutionYear *int   // set when flagged as that year's resolution, nil otherwise
	Done           bool
	CreatedAt      time.Time
	DoneAt         *time.Time // set when Done, nil otherwise
}

// Bucket-list category keys. Stored verbatim in the category column; the client
// maps each key to a display label. Unknown values normalize to CategoryOther.
const (
	CategorySelfImprovement = "self_improvement"
	CategoryLearning        = "learning"
	CategoryHiking          = "hiking"
	CategoryCountry         = "country"
	CategoryLocal           = "local"
	CategoryOther           = "other"
)

// bucketCategories is the set of recognized category keys.
var bucketCategories = map[string]bool{
	CategorySelfImprovement: true,
	CategoryLearning:        true,
	CategoryHiking:          true,
	CategoryCountry:         true,
	CategoryLocal:           true,
	CategoryOther:           true,
}

// NormalizeCategory lower-cases a category and falls back to CategoryOther for
// anything unrecognized, so the stored value is always a known key.
func NormalizeCategory(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	if bucketCategories[c] {
		return c
	}
	return CategoryOther
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

// ToolInvocation is one tool call the agent made during a run. Model and the
// token fields are populated only for tools that call a paid API of their own
// (today the Image Generator on gpt-image-1) and are zero/empty otherwise, so a
// run's image-generation cost can be priced separately from the LLM. They are
// embedded in the trace document/JSON, so the bson tags name the new keys
// explicitly (older tool records simply omit them).
type ToolInvocation struct {
	Name             string `json:"name"`
	Arguments        string `json:"arguments"`
	Result           string `json:"result"`
	LatencyMs        int    `json:"latency_ms,omitempty"`
	Model            string `json:"model,omitempty" bson:"model,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty" bson:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty" bson:"completion_tokens,omitempty"`
	TotalTokens      int    `json:"total_tokens,omitempty" bson:"total_tokens,omitempty"`
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
	ID        int64
	UserID    int64
	ProjectID int64 // active project the run executed in (0 = none / background)
	Platform  string
	// Source is what triggered the run: "chat" for an interactive message
	// (web/WhatsApp), or a routine key ("start_of_day" / "end_of_day") for a
	// scheduled run. Empty is normalised to "chat" on write.
	Source           string
	Input            string
	Output           string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	// Image* aggregate this run's image-generation usage (model + tokens) across
	// all its tool calls, priced separately from the LLM. Zero/empty when the run
	// generated no images.
	ImageModel            string
	ImagePromptTokens     int
	ImageCompletionTokens int
	ImageTotalTokens      int
	LatencyMs             int
	ToolCount             int
	Tools                 []ToolInvocation // populated by GetTrace only
	Steps                 []LLMCall        // per-LLM-call detail; populated by GetTrace only
	Skills                []string         // skill keys active for this run
	Status                string           // "ok" or "error"
	Error                 string
	CreatedAt             time.Time
	Score                 *TraceScore // LLM-as-judge verdict; nil until judged
}

// TraceScore is an LLM-as-judge quality verdict for a single trace. Each
// dimension is rated 1–5; Overall is their average. A trace has at most one
// score (keyed by TraceID); re-judging overwrites it.
type TraceScore struct {
	TraceID     int64
	Accuracy    int     // did the reply correctly answer / act on the input
	Helpfulness int     // did the reply actually deliver what the user asked for (low if it couldn't)
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
	Platforms []string // nil/empty = all; otherwise match any listed platform
	ProjectID int64    // 0 = all projects; otherwise restrict to this project
	From      time.Time
	To        time.Time
	Limit     int
	Cursor    int64
	// ScoreStates filters by judge verdict; nil/empty = all. Each entry is
	// "scored", "unscored", or "low" (judged and overall below
	// LowScoreThreshold). Multiple entries match traces in ANY of the states.
	ScoreStates []string
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

// ProjectModelUsage is per-project, per-model usage — used to compute per-project
// cost for the superadmin cross-project overview (pricing applied in the API
// layer, like UserModelUsage).
type ProjectModelUsage struct {
	ProjectID        int64
	Model            string
	Requests         int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
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
// (main data). The split HybridStore composes a DataStore (PostgreSQL) and a
// LogStore (MongoDB) and supplies GetUserActivity by hand.
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
// users, skills, reminders, persona, memories, contacts, bucket list, hiking,
// travel, notes, OAuth tokens, settings, and model prices. In the hybrid
// backend this is served by PostgreSQL.
type DataStore interface {
	// SetTranslator injects the optional English-normalization translator,
	// applied to reminder/bucket-list text before it is persisted. Wired after
	// construction because the translator depends on runtime settings.
	SetTranslator(t Translator)

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
	// SetSkillPrompt overwrites a skill's prompt (master data, shared by all
	// users). A non-empty updatedBy records the change as an admin customization
	// (prompt_updated_at = now, prompt_updated_by = updatedBy) which protects the
	// prompt from being clobbered by the boot seed. An empty updatedBy resets the
	// prompt to a code-owned default: prompt_updated_at is cleared so the seed
	// manages it again.
	SetSkillPrompt(ctx context.Context, skillID int64, prompt, updatedBy string) error
	EnabledSkillKeys(ctx context.Context, userID int64) ([]string, error)
	// UpdateSkillTunedPrompt sets (or, with an empty string, clears) a skill's
	// auto-tuned prompt override, keyed by the skill's stable `key`. Used by the
	// end-of-day self-tuner and the "revert to default" affordance.
	UpdateSkillTunedPrompt(ctx context.Context, key, tuned string) error
	// ListSkillsWithProjectMapping returns every skill with the projects that
	// effectively enable it, backing the superadmin skills catalog. SetSkillCore
	// marks/unmarks a global skill as core (project forks cannot be core).
	ListSkillsWithProjectMapping(ctx context.Context) ([]SkillWithMapping, error)
	SetSkillCore(ctx context.Context, skillID int64, isCore bool) error

	// Projects (the tenancy boundary)
	CreateProject(ctx context.Context, name string, ownerUserID int64) (*Project, error)
	GetProject(ctx context.Context, id int64) (*Project, error)
	ListProjects(ctx context.Context) ([]Project, error)                             // all projects (superadmin)
	ListProjectsForUser(ctx context.Context, userID int64) ([]ProjectSummary, error) // projects the user belongs to
	UpdateProjectName(ctx context.Context, id int64, name string) error
	DeleteProject(ctx context.Context, id int64) error // hard-deletes the project and every row scoped to it

	// Project membership & roles
	ListProjectMembers(ctx context.Context, projectID int64) ([]ProjectMemberDetail, error)
	GetProjectRole(ctx context.Context, projectID, userID int64) (string, error) // "" if not a member
	AddProjectMember(ctx context.Context, projectID, userID int64, role string) error
	UpdateProjectMemberRole(ctx context.Context, projectID, userID int64, role string) error
	RemoveProjectMember(ctx context.Context, projectID, userID int64) error
	CountProjectAdmins(ctx context.Context, projectID int64) (int, error)

	// Per-project skills (effective state folds in the feature-cascade gate).
	// The listing includes the project's own forks and every global skill not
	// shadowed by a fork of the same key.
	ListProjectSkills(ctx context.Context, projectID int64) ([]UserSkill, error)
	SetProjectSkillEnabled(ctx context.Context, projectID, skillID int64, enabled bool) error
	EnabledProjectSkillKeys(ctx context.Context, projectID int64) ([]string, error)
	// CreateProjectSkill forks a skill into a project (project-owned copy the
	// admin then customizes); DeleteProjectSkill removes a project's fork,
	// reverting that project to the shared global skill.
	CreateProjectSkill(ctx context.Context, projectID int64, base Skill, updatedBy string) (*Skill, error)
	DeleteProjectSkill(ctx context.Context, projectID, skillID int64) error

	// Features (catalog + per-project enable/disable; feature off ⇒ its skills off)
	ListFeatures(ctx context.Context) ([]Feature, error)
	GetFeature(ctx context.Context, id int64) (*Feature, error)
	ListProjectFeatures(ctx context.Context, projectID int64) ([]ProjectFeature, error)
	SetProjectFeatureEnabled(ctx context.Context, projectID, featureID int64, enabled bool) error

	// WhatsApp identity → project/role mappings (superadmin-managed)
	ListWhatsAppMappings(ctx context.Context) ([]WhatsAppMapping, error)
	GetWhatsAppMapping(ctx context.Context, jid string) (*WhatsAppMapping, error)
	CreateWhatsAppMapping(ctx context.Context, m WhatsAppMapping) (*WhatsAppMapping, error)
	UpdateWhatsAppMapping(ctx context.Context, id int64, m WhatsAppMapping) error
	DeleteWhatsAppMapping(ctx context.Context, id int64) error

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

	// Memories (agent long-term memory, scoped to a user; full-text backed).
	// SearchMemories takes raw query text; the backend sanitizes it for its
	// PostgreSQL tsquery search.
	CreateMemory(ctx context.Context, userID int64, content, kind string) (*Memory, error)
	GetMemory(ctx context.Context, userID, id int64) (*Memory, error)
	SearchMemories(ctx context.Context, userID int64, query string, limit int) ([]Memory, error)
	ListMemories(ctx context.Context, userID int64, limit int) ([]Memory, error)
	DeleteMemory(ctx context.Context, userID, id int64) error

	// Contacts (scoped to a user)
	CreateContact(ctx context.Context, userID int64, name, phone, email, note string) (*Contact, error)
	GetContact(ctx context.Context, userID, id int64) (*Contact, error)
	SearchContacts(ctx context.Context, userID int64, query string) ([]Contact, error)

	// Bucket list (a categorized life checklist, scoped to a user)
	CreateBucketItem(ctx context.Context, userID int64, title, description, note, category string, resolutionYear *int) (*BucketItem, error)
	ListBucketItems(ctx context.Context, userID int64) ([]BucketItem, error)
	GetBucketItem(ctx context.Context, userID, id int64) (*BucketItem, error)
	UpdateBucketItem(ctx context.Context, userID, id int64, title, description, note, category string) error
	SetBucketItemDone(ctx context.Context, userID, id int64, done bool, doneAt *time.Time) error
	SetBucketItemResolution(ctx context.Context, userID, id int64, year *int) error
	DeleteBucketItem(ctx context.Context, userID, id int64) error

	// Hiking (scoped to a user; names are canonical for typo-free reuse)
	ListMountains(ctx context.Context, userID int64) ([]Mountain, error)
	CreateMountain(ctx context.Context, userID int64, name string) (*Mountain, error)
	ListTracks(ctx context.Context, userID, mountainID int64) ([]HikeTrack, error)
	CreateTrack(ctx context.Context, userID, mountainID int64, name string) (*HikeTrack, error)
	ListHikers(ctx context.Context, userID int64) ([]Hiker, error)
	CreateHiker(ctx context.Context, userID int64, name string) (*Hiker, error)
	CreateHike(ctx context.Context, userID int64, h *Hike) (int64, error)
	GetHike(ctx context.Context, userID, id int64) (*HikeDetail, error)
	AddHikeParticipant(ctx context.Context, hikeID, hikerID int64) error
	ListHikes(ctx context.Context, userID int64, limit int) ([]HikeDetail, error)

	// Travel (scoped to a user)
	CreateTrip(ctx context.Context, userID int64, name, destination, currency string, budget float64) (*Trip, error)
	GetTrip(ctx context.Context, userID, id int64) (*Trip, error)
	ActiveTrip(ctx context.Context, userID int64) (*Trip, error)
	FindTrip(ctx context.Context, userID int64, name string) (*Trip, error)
	AddExpense(ctx context.Context, userID, tripID int64, amount float64, currency, category, note string, spentAt time.Time) (*TripExpense, error)
	GetTripExpense(ctx context.Context, userID, id int64) (*TripExpense, error)
	ListTripExpenses(ctx context.Context, userID, tripID int64) ([]TripExpense, error)

	// Notes (scoped to a user)
	CreateNote(ctx context.Context, userID int64, title, content, tags string) (*Note, error)
	GetNote(ctx context.Context, userID, id int64) (*Note, error)
	UpdateNote(ctx context.Context, userID, id int64, title, content, tags string) error
	DeleteNote(ctx context.Context, userID, id int64) error
	ListNotes(ctx context.Context, userID int64, tag string) ([]Note, error)
	// SearchNotes takes raw query text; the backend sanitizes it for its own
	// full-text search dialect.
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
	GetActivity(ctx context.Context, userID, id int64) (*Activity, error)
	ListActivitiesSince(ctx context.Context, userID int64, since time.Time) ([]Activity, error)

	// Message log (scoped to a user)
	LogMessage(ctx context.Context, log *MessageLog) error
	GetMessageHistory(ctx context.Context, userID int64, platform string, limit int) ([]MessageLog, error)

	// Audit log (project-level actions: create, invite, skill/feature toggle, …)
	RecordAudit(ctx context.Context, e *AuditEvent) error
	ListAuditEvents(ctx context.Context, projectID int64, limit int) ([]AuditEvent, error)

	// Traces & usage (source of truth for dashboard + logs)
	CreateTrace(ctx context.Context, t *Trace) (int64, error)
	ListTraces(ctx context.Context, f TraceFilter) ([]Trace, error)
	GetTrace(ctx context.Context, id int64) (*Trace, error)
	LogToolUsage(ctx context.Context, userID int64, tool, platform string) error

	// Trace scores (LLM-as-judge quality signal).
	SaveTraceScore(ctx context.Context, sc *TraceScore) error
	GetTraceScore(ctx context.Context, traceID int64) (*TraceScore, error)
	ListUnscoredTraces(ctx context.Context, since time.Time, limit int) ([]Trace, error)
	// ListLowScoreTracesWithSkills returns full-detail traces for one user in
	// [from, to) whose judge score.overall is <= maxOverall and that have at
	// least one skill attached, worst score first. Traces whose Source is in
	// excludeSources are omitted (so the self-tuner ignores its own routine
	// runs). Unlike ListTraces, Tools/Steps/Skills are populated. Powers the
	// end-of-day self-tuner's review step.
	ListLowScoreTracesWithSkills(ctx context.Context, userID int64, from, to time.Time, maxOverall float64, excludeSources []string, limit int) ([]Trace, error)
	// ListFailedTraces returns full-detail traces (Tools/Steps/Skills populated)
	// for one user in [from, to) that the assistant could not handle well —
	// either the agent run errored (status == "error") or the judge scored it
	// below LowScoreThreshold. Errors sort first, then worst score; newest is the
	// tiebreak. Traces whose Source is in excludeSources are omitted (so the
	// nightly triage never triages its own runs). Powers the auto-triage skill's
	// failure scan.
	ListFailedTraces(ctx context.Context, userID int64, from, to time.Time, excludeSources []string, limit int) ([]Trace, error)

	// Usage analytics (aggregates over traces & tool_usage)
	// platforms nil/empty = all; otherwise restricted to any listed platform.
	UsageStatsBetween(ctx context.Context, from, to time.Time, platforms []string) (*UsageStats, error)
	UsageByDayModel(ctx context.Context, from, to time.Time, platforms []string) ([]DayModelUsage, error)
	UsageByUserModel(ctx context.Context, from, to time.Time, platforms []string) ([]UserModelUsage, error)
	// UsageByProject aggregates trace usage per (project, model) over [from, to),
	// for the superadmin cross-project overview. Traces are tagged with their
	// project_id at write time.
	UsageByProject(ctx context.Context, from, to time.Time) ([]ProjectModelUsage, error)

	// Lifecycle
	Close() error
}
