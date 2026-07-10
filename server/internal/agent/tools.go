package agent

import (
	"encoding/json"

	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/llm"
)

// toolSpec maps an LLM tool to a capability action. The tool's JSON arguments
// become the ParseResult entities, so property names must match the entity keys
// each capability handler reads.
type toolSpec struct {
	name        string
	description string
	capability  intent.Capability
	action      intent.Action
	parameters  string // JSON Schema for the tool arguments
}

var toolSpecs = []toolSpec{
	{
		name:        "calendar_list",
		description: "List the user's calendar events for a given day.",
		capability:  intent.CapabilityCalendar,
		action:      intent.ActionCalendarList,
		parameters:  `{"type":"object","properties":{"date":{"type":"string","description":"Day to list, e.g. 'today', 'tomorrow', 'Friday'. Defaults to today if omitted."}}}`,
	},
	{
		name:        "calendar_create",
		description: "Create a new calendar event.",
		capability:  intent.CapabilityCalendar,
		action:      intent.ActionCalendarCreate,
		parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"Event title."},"datetime":{"type":"string","description":"Natural-language start time, e.g. 'at 3pm', 'tomorrow at 2pm'."}},"required":["title"]}`,
	},
	{
		name:        "calendar_update",
		description: "Reschedule an existing calendar event to a new time.",
		capability:  intent.CapabilityCalendar,
		action:      intent.ActionCalendarUpdate,
		parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"Title (or part of it) of the event to reschedule."},"datetime":{"type":"string","description":"New natural-language start time."}},"required":["title","datetime"]}`,
	},
	{
		name:        "calendar_delete",
		description: "Delete/cancel a calendar event.",
		capability:  intent.CapabilityCalendar,
		action:      intent.ActionCalendarDelete,
		parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"Title (or part of it) of the event to delete."}},"required":["title"]}`,
	},
	{
		name:        "email_inbox",
		description: "List the user's unread inbox emails.",
		capability:  intent.CapabilityEmail,
		action:      intent.ActionEmailInbox,
		parameters:  `{"type":"object","properties":{}}`,
	},
	{
		name:        "email_read",
		description: "Read a specific email by its number from the most recent inbox or search listing.",
		capability:  intent.CapabilityEmail,
		action:      intent.ActionEmailRead,
		parameters:  `{"type":"object","properties":{"index":{"type":"string","description":"1-based number of the email in the last listing."}},"required":["index"]}`,
	},
	{
		name:        "email_search",
		description: "Search the user's emails.",
		capability:  intent.CapabilityEmail,
		action:      intent.ActionEmailSearch,
		parameters:  `{"type":"object","properties":{"query":{"type":"string","description":"Search query."}},"required":["query"]}`,
	},
	{
		name:        "email_draft",
		description: "Create (but do NOT send) an email draft.",
		capability:  intent.CapabilityEmail,
		action:      intent.ActionEmailDraft,
		parameters:  `{"type":"object","properties":{"to":{"type":"string","description":"Recipient email address."},"body":{"type":"string","description":"Message body."}},"required":["to","body"]}`,
	},
	{
		name:        "note_save",
		description: "Save a note to the knowledge base.",
		capability:  intent.CapabilityKnowledge,
		action:      intent.ActionNoteSave,
		parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"Note title. Append #tags to the title to tag it, e.g. 'Groceries #home'."},"content":{"type":"string","description":"Note body text."}},"required":["title"]}`,
	},
	{
		name:        "note_search",
		description: "Search the user's saved notes.",
		capability:  intent.CapabilityKnowledge,
		action:      intent.ActionNoteSearch,
		parameters:  `{"type":"object","properties":{"query":{"type":"string","description":"Search query."}},"required":["query"]}`,
	},
	{
		name:        "note_list",
		description: "List saved notes, optionally filtered by a tag.",
		capability:  intent.CapabilityKnowledge,
		action:      intent.ActionNoteList,
		parameters:  `{"type":"object","properties":{"tag":{"type":"string","description":"Optional tag to filter by."}}}`,
	},
	{
		name:        "note_delete",
		description: "Delete a note by its number.",
		capability:  intent.CapabilityKnowledge,
		action:      intent.ActionNoteDelete,
		parameters:  `{"type":"object","properties":{"id":{"type":"string","description":"Note number to delete."}},"required":["id"]}`,
	},
	{
		name:        "remember",
		description: "Save a durable fact about the user to long-term memory (plans, budgets, preferences, decisions, ongoing tasks) so you can use it in later messages and future sessions.",
		capability:  intent.CapabilityMemory,
		action:      intent.ActionMemoryRemember,
		parameters:  `{"type":"object","properties":{"content":{"type":"string","description":"The fact to remember, written as a concise standalone statement (e.g. 'User's Japan trip budget is Rp35-50 million, solo, 14 days')."}},"required":["content"]}`,
	},
	{
		name:        "recall",
		description: "Search your long-term memory for what you already know about the user.",
		capability:  intent.CapabilityMemory,
		action:      intent.ActionMemoryRecall,
		parameters:  `{"type":"object","properties":{"query":{"type":"string","description":"What to look up in memory."}},"required":["query"]}`,
	},
	// Reminders are a core capability (they back the Reminders page and the
	// user's schedule/calendar), so these tools are always available.
	{
		name:        "schedule_event",
		description: "Save a ONE-TIME / dated reminder from a natural date/time — an appointment, meeting, flight, or specific-date thing (e.g. 'dentist tomorrow 3pm', 'meeting on Aug 5 at 2pm', 'besok jam 10 donor darah'). It's stored as a one-time reminder (and mirrored to Google Calendar when connected). Use reminder_schedule for anything that repeats.",
		capability:  intent.CapabilityEvent,
		action:      intent.ActionEventCreate,
		parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"What the event is."},"datetime":{"type":"string","description":"When it happens, e.g. 'tomorrow at 3pm', 'Aug 5 at 2pm', '2026-08-05 14:00'."},"duration_minutes":{"type":"integer","description":"Event length in minutes (default 60)."},"location":{"type":"string","description":"Where it is, if given."}},"required":["title","datetime"]}`,
	},
	{
		name:        "reminder_schedule",
		description: "Create a reminder — one-time or recurring. Use this for repeats ('every month on the 5th', 'every weekday at 8am', 'daily at 8pm') and for a specific-date one-off ('on Aug 5'). For a one-off given as a natural date/time ('tomorrow 3pm') you can also use schedule_event.",
		capability:  intent.CapabilityReminder,
		action:      intent.ActionReminderSchedule,
		parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"What to be reminded about."},"repeat":{"type":"string","enum":["once","daily","weekly","monthly"],"description":"How often to repeat. Use 'once' for a single specific date."},"times":{"type":"string","description":"One or more times of day in 24h HH:MM, comma-separated (e.g. '09:00' or '08:00,20:00'). Omit this if the user did not specify a time — the user's default reminder time is then used."},"day_of_month":{"type":"integer","description":"For monthly: day of the month 1-31 (e.g. 5)."},"weekdays":{"type":"string","description":"For weekly: comma-separated day names, e.g. 'Mon,Wed,Fri'."},"date":{"type":"string","description":"For once: the date as YYYY-MM-DD."}},"required":["title","repeat"]}`,
	},
	{
		name:        "reminder_list",
		description: "List the user's recurring reminders. Part of the user's schedule — pair it with list_calendar when the user asks what's on their schedule/calendar or what's coming up.",
		capability:  intent.CapabilityReminder,
		action:      intent.ActionReminderList,
		parameters:  `{"type":"object","properties":{}}`,
	},
	{
		name:        "list_calendar",
		description: "List upcoming events from the user's connected Google Calendar(s), across all their accounts and calendars. Pair it with reminder_list when the user asks what's on their schedule/calendar/agenda or what's coming up.",
		capability:  intent.CapabilityEvent,
		action:      intent.ActionEventAgenda,
		parameters:  `{"type":"object","properties":{"days":{"type":"integer","description":"How many days ahead to include (default 7)."}}}`,
	},
	{
		name:        "reminder_cancel",
		description: "Cancel a reminder by its number.",
		capability:  intent.CapabilityReminder,
		action:      intent.ActionReminderCancel,
		parameters:  `{"type":"object","properties":{"id":{"type":"string","description":"Reminder number to cancel."}},"required":["id"]}`,
	},
}

// skillTools maps a skill key to the extra tools that skill provides. These are
// only exposed to the LLM when the user has the skill enabled.
var skillTools = map[string][]toolSpec{
	"ask_about_contact": {
		{
			name:        "contact_add",
			description: "Save a personal contact for the user.",
			capability:  intent.CapabilityContact,
			action:      intent.ActionContactAdd,
			parameters:  `{"type":"object","properties":{"name":{"type":"string","description":"The contact's name."},"phone":{"type":"string","description":"Phone number, if given."},"email":{"type":"string","description":"Email address, if given."},"note":{"type":"string","description":"Any extra note about the contact."}},"required":["name"]}`,
		},
		{
			name:        "contact_search",
			description: "Look up the user's saved contacts. Omit query to list all.",
			capability:  intent.CapabilityContact,
			action:      intent.ActionContactSearch,
			parameters:  `{"type":"object","properties":{"query":{"type":"string","description":"Name, phone, email, or keyword to search for. Empty lists all contacts."}}}`,
		},
	},
	"life_goals": {
		{
			name:        "lifegoal_add",
			description: "Add an item to the user's life list — something they want to do in life (e.g. 'Take a swimming course', 'Get a gym membership', 'Visit Japan'). Use this when the user says they want to do/achieve something someday.",
			capability:  intent.CapabilityLifeGoal,
			action:      intent.ActionLifeGoalAdd,
			parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"A short, punchy title for the goal — phrase it like an actual title, ideally 1-3 words (e.g. 'Swimming Lessons', not 'Efficient Freestyle Swimming Lessons'; 'Visit Japan', not 'Take a Two-Week Trip to Japan')."},"description":{"type":"string","description":"A one- or two-sentence description that enriches the goal based on what the user said and the title — what it involves or why it matters (e.g. for swimming: 'Learn to swim freestyle confidently through structured beginner lessons.'). Always provide this."},"note":{"type":"string","description":"Any short extra personal detail the user gave, optional (e.g. 'beginner class', 'near the office')."}},"required":["title","description"]}`,
		},
		{
			name:        "lifegoal_list",
			description: "List the user's life list — the checklist of things they want to do in life, with which are done.",
			capability:  intent.CapabilityLifeGoal,
			action:      intent.ActionLifeGoalList,
			parameters:  `{"type":"object","properties":{}}`,
		},
		{
			name:        "lifegoal_check",
			description: "Mark a life-list item as done/achieved. Identify it by its number from the last listing or by its title.",
			capability:  intent.CapabilityLifeGoal,
			action:      intent.ActionLifeGoalCheck,
			parameters:  `{"type":"object","properties":{"item":{"type":"string","description":"The item's number from the list, or (part of) its title."}},"required":["item"]}`,
		},
		{
			name:        "lifegoal_delete",
			description: "Remove an item from the user's life list. Identify it by its number from the last listing or by its title.",
			capability:  intent.CapabilityLifeGoal,
			action:      intent.ActionLifeGoalDelete,
			parameters:  `{"type":"object","properties":{"item":{"type":"string","description":"The item's number from the list, or (part of) its title."}},"required":["item"]}`,
		},
	},
	"activity_summary": {
		{
			name:        "activity_log",
			description: "Record a sport or workout the user did.",
			capability:  intent.CapabilityActivity,
			action:      intent.ActionActivityLog,
			parameters:  `{"type":"object","properties":{"type":{"type":"string","description":"Kind of activity, e.g. running, football, gym, yoga."},"description":{"type":"string","description":"Short detail, e.g. '5k' or '1 hour'."},"when":{"type":"string","description":"When it happened, e.g. 'this morning', 'yesterday'. Defaults to now."}},"required":["type"]}`,
		},
		{
			name:        "activity_summarize",
			description: "Summarize the user's recent sport/workout activity.",
			capability:  intent.CapabilityActivity,
			action:      intent.ActionActivitySummarize,
			parameters:  `{"type":"object","properties":{"days":{"type":"integer","description":"Look-back window in days. Defaults to 7."}}}`,
		},
	},
	"travel_control": {
		{
			name:        "trip_create",
			description: "Start a new trip to track expenses against (becomes the active trip).",
			capability:  intent.CapabilityTravel,
			action:      intent.ActionTripCreate,
			parameters:  `{"type":"object","properties":{"name":{"type":"string","description":"Name for the trip."},"destination":{"type":"string","description":"Where the trip is to."},"budget":{"type":"number","description":"Optional total budget."},"currency":{"type":"string","description":"Currency code, e.g. IDR, USD."}},"required":["name"]}`,
		},
		{
			name:        "expense_add",
			description: "Record an expense against the active (or named) trip.",
			capability:  intent.CapabilityTravel,
			action:      intent.ActionExpenseAdd,
			parameters:  `{"type":"object","properties":{"amount":{"type":"number","description":"Expense amount."},"category":{"type":"string","description":"e.g. food, hotel, transport, activity."},"note":{"type":"string","description":"Optional detail."},"currency":{"type":"string","description":"Currency; defaults to the trip's currency."},"trip":{"type":"string","description":"Trip name; defaults to the active trip."}},"required":["amount"]}`,
		},
		{
			name:        "trip_summary",
			description: "Summarize a trip's spending by category vs budget.",
			capability:  intent.CapabilityTravel,
			action:      intent.ActionTripSummary,
			parameters:  `{"type":"object","properties":{"trip":{"type":"string","description":"Trip name; defaults to the active trip."}}}`,
		},
	},
	"hiking_tracker": {
		{
			name:        "hike_log",
			description: "Log a hiking trip. Similar existing mountain, trail, and participant names are reused automatically to prevent duplicates.",
			capability:  intent.CapabilityHiking,
			action:      intent.ActionHikeLog,
			parameters:  `{"type":"object","properties":{"mountain":{"type":"string","description":"Mountain / hiking destination."},"up_track":{"type":"string","description":"Trail used going up."},"down_track":{"type":"string","description":"Trail used going down."},"camped":{"type":"boolean","description":"Whether the user camped overnight."},"days":{"type":"integer","description":"Number of days spent."},"nights":{"type":"integer","description":"Number of nights spent."},"date":{"type":"string","description":"Hiking date, e.g. 'last Saturday', 'Aug 2'."},"participants":{"type":"string","description":"Comma-separated participant names."}},"required":["mountain"]}`,
		},
		{
			name:        "hike_summary",
			description: "List and summarize the user's logged hikes.",
			capability:  intent.CapabilityHiking,
			action:      intent.ActionHikeSummary,
			parameters:  `{"type":"object","properties":{"limit":{"type":"integer","description":"How many recent hikes to show. Default 10."}}}`,
		},
	},
}

// toolByName indexes every tool (base + all skill tools) so execTool can route a
// tool call to its capability action regardless of which skill exposed it.
var toolByName = func() map[string]toolSpec {
	m := make(map[string]toolSpec, len(toolSpecs))
	for _, t := range toolSpecs {
		m[t.name] = t
	}
	for _, ts := range skillTools {
		for _, t := range ts {
			m[t.name] = t
		}
	}
	return m
}()

func specToTool(t toolSpec) llm.Tool {
	return llm.Tool{
		Type:     "function",
		Function: llm.ToolFunction{Name: t.name, Description: t.description, Parameters: json.RawMessage(t.parameters)},
	}
}

// toolSchemas returns the always-on base tool definitions in the LLM wire format.
func toolSchemas() []llm.Tool {
	tools := make([]llm.Tool, len(toolSpecs))
	for i, t := range toolSpecs {
		tools[i] = specToTool(t)
	}
	return tools
}

// skillToolSchemas returns the tools provided by the given enabled skill keys.
func skillToolSchemas(keys []string) []llm.Tool {
	var out []llm.Tool
	for _, k := range keys {
		for _, t := range skillTools[k] {
			out = append(out, specToTool(t))
		}
	}
	return out
}
