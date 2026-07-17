package intent

// Capability identifies which capability should handle a message.
type Capability string

const (
	CapabilityCalendar   Capability = "calendar"
	CapabilityEvent      Capability = "event"
	CapabilityEmail      Capability = "email"
	CapabilityReminder   Capability = "reminder"
	CapabilityKnowledge  Capability = "knowledge"
	CapabilityMemory     Capability = "memory"
	CapabilityContact    Capability = "contact"
	CapabilityBucketList Capability = "bucket_list"
	CapabilityActivity   Capability = "activity"
	CapabilityTravel     Capability = "travel"
	CapabilityHiking     Capability = "hiking"
	CapabilityWebSearch  Capability = "web_search"
	CapabilityImageGen   Capability = "image_gen"
	CapabilityTrello     Capability = "trello"
	CapabilitySelfTune   Capability = "self_tune"
	CapabilityHelp       Capability = "help"
	CapabilityUnknown    Capability = "unknown"
)

// Action identifies the specific operation within a capability.
type Action string

// Calendar actions
const (
	ActionCalendarList   Action = "calendar.list"
	ActionCalendarCreate Action = "calendar.create"
	ActionCalendarUpdate Action = "calendar.update"
	ActionCalendarDelete Action = "calendar.delete"
)

// Event actions (one-time events → Google Calendar, with a reminder fallback)
const (
	ActionEventCreate Action = "event.create"
	ActionEventAgenda Action = "event.agenda"
	ActionEventDelete Action = "event.delete"
)

// Email actions
const (
	ActionEmailInbox  Action = "email.inbox"
	ActionEmailRead   Action = "email.read"
	ActionEmailSearch Action = "email.search"
	ActionEmailDraft  Action = "email.draft"
)

// Reminder actions
const (
	ActionReminderSet      Action = "reminder.set"
	ActionReminderSchedule Action = "reminder.schedule"
	ActionReminderList     Action = "reminder.list"
	ActionReminderCancel   Action = "reminder.cancel"
)

// Knowledge actions
const (
	ActionNoteSave   Action = "note.save"
	ActionNoteSearch Action = "note.search"
	ActionNoteList   Action = "note.list"
	ActionNoteDelete Action = "note.delete"
)

// Memory actions
const (
	ActionMemoryRemember Action = "memory.remember"
	ActionMemoryRecall   Action = "memory.recall"
)

// Contact actions
const (
	ActionContactAdd    Action = "contact.add"
	ActionContactSearch Action = "contact.search"
)

// Bucket list actions (a categorized life checklist)
const (
	ActionBucketListAdd    Action = "bucket_list.add"
	ActionBucketListList   Action = "bucket_list.list"
	ActionBucketListCheck  Action = "bucket_list.check"
	ActionBucketListDelete Action = "bucket_list.delete"
)

// Activity actions
const (
	ActionActivityLog       Action = "activity.log"
	ActionActivitySummarize Action = "activity.summarize"
)

// Travel actions
const (
	ActionTripCreate  Action = "trip.create"
	ActionExpenseAdd  Action = "trip.expense_add"
	ActionTripSummary Action = "trip.summary"
)

// Hiking actions
const (
	ActionHikeLog     Action = "hike.log"
	ActionHikeSummary Action = "hike.summary"
)

// Web search actions
const (
	ActionWebSearch Action = "web_search.search"
)

// Image generation actions
const (
	ActionImageGenerate Action = "image_gen.generate"
	ActionImageEdit     Action = "image_gen.edit"
)

// Trello actions (review the boards; file a task or a bug card; capture a game idea)
const (
	ActionTrelloReview     Action = "trello.review"
	ActionTrelloCreateTask Action = "trello.create_task"
	ActionTrelloReportBug  Action = "trello.report_bug"
	ActionTrelloGameIdea   Action = "trello.game_idea"
)

// Self-tuning actions (review low-quality runs and refine skill prompts)
const (
	ActionSelfTuneReview Action = "self_tune.review"
	ActionSelfTuneApply  Action = "self_tune.apply"
)

const (
	ActionHelp Action = "help"
)

// ParseResult holds the result of intent parsing.
type ParseResult struct {
	Capability Capability
	Action     Action
	Entities   map[string]string
	Confidence float64
	RawText    string
}
