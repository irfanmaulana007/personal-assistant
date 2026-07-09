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
}

// skillTools maps a skill key to the extra tools that skill provides. These are
// only exposed to the LLM when the user has the skill enabled.
var skillTools = map[string][]toolSpec{
	"scheduled_reminder": {
		{
			name:        "reminder_set",
			description: "Set a reminder for the user (delivered to their WhatsApp when due).",
			capability:  intent.CapabilityReminder,
			action:      intent.ActionReminderSet,
			parameters:  `{"type":"object","properties":{"message":{"type":"string","description":"What to be reminded about."},"time":{"type":"string","description":"When to remind, e.g. 'in 30 minutes', 'at 5pm', 'tomorrow at 9am', 'on the 5th at 9am'."}},"required":["message","time"]}`,
		},
		{
			name:        "reminder_list",
			description: "List the user's active reminders.",
			capability:  intent.CapabilityReminder,
			action:      intent.ActionReminderList,
			parameters:  `{"type":"object","properties":{}}`,
		},
		{
			name:        "reminder_cancel",
			description: "Cancel a reminder by its number.",
			capability:  intent.CapabilityReminder,
			action:      intent.ActionReminderCancel,
			parameters:  `{"type":"object","properties":{"id":{"type":"string","description":"Reminder number to cancel."}},"required":["id"]}`,
		},
	},
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
