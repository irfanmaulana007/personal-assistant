package agent

import (
	"encoding/json"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/llm"
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
	// Calendar is served exclusively through the user's Composio-connected
	// Google Calendar: reads go through "list_calendar", creates through
	// "schedule_event" (stored as a reminder and mirrored to Google Calendar),
	// and deletes through "delete_calendar_event" (resolves a title/time against
	// the live calendar and removes the matching event).
	// The old native-Google "calendar_*" tools were removed: their handler is
	// gated off by default (calendar.enabled: false), so advertising them let
	// the model pick a dead tool that always replied "I don't have that
	// capability yet" even when the Composio calendar was connected.
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
	{
		name:        "delete_calendar_event",
		description: "Delete an event from the user's connected Google Calendar. Identify the event by the exact title shown in list_calendar; pass the datetime too when several events share a title so the right instance is removed (with a datetime, all duplicates at that exact time are cleared). Use this to remove wrong, stale, or duplicate calendar events. This does not cancel a recurring reminder — cancel the reminder with reminder_cancel if the event was created by one.",
		capability:  intent.CapabilityEvent,
		action:      intent.ActionEventDelete,
		parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"Exact event title as shown in list_calendar."},"datetime":{"type":"string","description":"When the event starts, e.g. 'Jul 13 at 6:00 PM', '2026-07-13 18:00'. Provide this to target a specific instance or to clear duplicates at that time."}},"required":["title"]}`,
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
	"bucket_list": {
		{
			name:        "bucketlist_add",
			description: "Add an item to the user's bucket list — something they want to do in life (e.g. 'Take a swimming course', 'Get a gym membership', 'Visit Japan'). Use this when the user says they want to do/achieve something someday.",
			capability:  intent.CapabilityBucketList,
			action:      intent.ActionBucketListAdd,
			parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"A short, punchy title for the item — phrase it like an actual title, ideally 1-3 words (e.g. 'Swimming Lessons', not 'Efficient Freestyle Swimming Lessons'; 'Visit Japan', not 'Take a Two-Week Trip to Japan')."},"description":{"type":"string","description":"A one- or two-sentence description that enriches the item based on what the user said and the title — what it involves or why it matters (e.g. for swimming: 'Learn to swim freestyle confidently through structured beginner lessons.'). Always provide this."},"note":{"type":"string","description":"Any short extra personal detail the user gave, optional (e.g. 'beginner class', 'near the office')."},"category":{"type":"string","enum":["self_improvement","learning","hiking","country","local","other"],"description":"Which bucket the item belongs to: 'self_improvement' (habits, health, personal growth), 'learning' (a skill or subject the user wants to learn, e.g. 'learn Harness Engineering', 'learn Spanish'), 'hiking' (a mountain or hiking destination), 'country' (a country to visit), 'local' (a place to visit in the user's own country/city), or 'other'. Infer it from the item; default to 'other' if unclear."}},"required":["title","description"]}`,
		},
		{
			name:        "bucketlist_list",
			description: "List the user's bucket list — the checklist of things they want to do in life, with which are done.",
			capability:  intent.CapabilityBucketList,
			action:      intent.ActionBucketListList,
			parameters:  `{"type":"object","properties":{}}`,
		},
		{
			name:        "bucketlist_check",
			description: "Mark a bucket-list item as done/achieved. Identify it by its number from the last listing or by its title.",
			capability:  intent.CapabilityBucketList,
			action:      intent.ActionBucketListCheck,
			parameters:  `{"type":"object","properties":{"item":{"type":"string","description":"The item's number from the list, or (part of) its title."}},"required":["item"]}`,
		},
		{
			name:        "bucketlist_delete",
			description: "Remove an item from the user's bucket list. Identify it by its number from the last listing or by its title.",
			capability:  intent.CapabilityBucketList,
			action:      intent.ActionBucketListDelete,
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
	"web_search": {
		{
			name:        "web_search",
			description: "Search the open web for current, real-world information the user asks about — news, sports scores and brackets, prices, weather, recent events, or any fact that may be newer than your training data. Returns the top results (title, URL, snippet) for you to summarize and cite. Use it whenever the user wants live or up-to-date information you don't already have.",
			capability:  intent.CapabilityWebSearch,
			action:      intent.ActionWebSearch,
			parameters:  `{"type":"object","properties":{"query":{"type":"string","description":"The search query, phrased as you'd type it into a search engine."},"count":{"type":"integer","description":"How many results to return (default 5, max 20)."}},"required":["query"]}`,
		},
	},
	"image_generator": {
		{
			name:        "generate_image",
			description: "Generate a brand-new image from a text description. Use this whenever the user asks you to draw, create, generate, or imagine a picture, illustration, logo, or artwork. Write a rich, detailed prompt describing subject, style, composition, colours, and mood. The image is delivered to the user automatically.",
			capability:  intent.CapabilityImageGen,
			action:      intent.ActionImageGenerate,
			parameters:  `{"type":"object","properties":{"prompt":{"type":"string","description":"A detailed description of the image to create, in English. Expand on the user's request with concrete visual detail (subject, style, composition, lighting, colours)."},"size":{"type":"string","enum":["1024x1024","1536x1024","1024x1536"],"description":"Aspect: square, landscape, or portrait. Default 1024x1024 (square)."},"quality":{"type":"string","enum":["low","medium","high"],"description":"Rendering quality. Higher costs more and is slower. Default medium."}},"required":["prompt"]}`,
		},
		{
			name:        "edit_image",
			description: "Edit or modify the image the user attached to their message, following an instruction (e.g. 'make the sky purple', 'add a party hat', 'remove the background', 'turn it into a watercolour'). Only works when the user has attached an image. The edited image is delivered to the user automatically.",
			capability:  intent.CapabilityImageGen,
			action:      intent.ActionImageEdit,
			parameters:  `{"type":"object","properties":{"prompt":{"type":"string","description":"The edit instruction describing how to change the attached image, in English."},"size":{"type":"string","enum":["1024x1024","1536x1024","1024x1536"],"description":"Output aspect. Default 1024x1024 (square)."},"quality":{"type":"string","enum":["low","medium","high"],"description":"Rendering quality. Default medium."}},"required":["prompt"]}`,
		},
	},
	"hiking_tracker": {
		{
			name:        "hike_log",
			description: "Log a hiking trip. Similar existing mountain and trail names are reused automatically to prevent typo-duplicates. Participant names are saved exactly as given (never fuzzy-matched onto a different person); a name only reuses an existing participant when it exactly matches that participant's name or a recorded nickname.",
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
		{
			name:        "hike_delete",
			description: "Delete one or more logged hikes by their number (the #id shown by hike_summary). Use this to remove duplicate or wrongly-logged hikes. Pass a single number, or several as a comma-separated list (e.g. \"12, 15, 18\") to clear multiple duplicates at once. Always run hike_summary first so you delete the right entries by their #id.",
			capability:  intent.CapabilityHiking,
			action:      intent.ActionHikeDelete,
			parameters:  `{"type":"object","properties":{"id":{"type":"string","description":"Hike number(s) to delete, from the #id shown in hike_summary. One number, or several comma-separated (e.g. '12, 15, 18')."}},"required":["id"]}`,
		},
		{
			name:        "hike_participant_list",
			description: "List the user's saved hiking participants and their nicknames/aliases. Use this before renaming or merging so you refer to the right person.",
			capability:  intent.CapabilityHiking,
			action:      intent.ActionHikeParticipants,
			parameters:  `{"type":"object","properties":{}}`,
		},
		{
			name:        "hike_participant_update",
			description: "Rename a hiking participant and/or set their nicknames. The new name is saved exactly as given — the auto-matcher will NOT remap it — so use this to fix a participant whose name was recorded wrong. 'name' identifies the current participant (by name or a known nickname); 'new_name' is the corrected full name; 'nicknames' (optional) replaces their alias list.",
			capability:  intent.CapabilityHiking,
			action:      intent.ActionHikeParticipantUpdate,
			parameters:  `{"type":"object","properties":{"name":{"type":"string","description":"Current name (or a known nickname) of the participant to update."},"new_name":{"type":"string","description":"Corrected full name (nama panjang). Omit to keep the current name and only change nicknames."},"nicknames":{"type":"string","description":"Comma-separated nicknames/aliases (panggilan) to set for this participant, replacing any existing ones."}},"required":["name"]}`,
		},
		{
			name:        "hike_participant_merge",
			description: "Merge two duplicate hiking participants (the same person recorded under two different spellings) into one record. Every hike is reattributed from the duplicate to the surviving participant, and the duplicate's name is kept as a nickname. Use this to clean up participants that were wrongly split or auto-mismatched.",
			capability:  intent.CapabilityHiking,
			action:      intent.ActionHikeParticipantMerge,
			parameters:  `{"type":"object","properties":{"from":{"type":"string","description":"Name (or nickname) of the DUPLICATE participant to merge away and remove."},"into":{"type":"string","description":"Name (or nickname) of the surviving participant to keep."}},"required":["from","into"]}`,
		},
	},
	"trello_review": {
		{
			name:        "trello_review",
			description: "List every task and bug across the user's project boards on Trello — the Task Management board (Backlog / Todo / In Progress / Done) and the Issue board (Bug / Progress / Done) — grouped by board and list. Use when the user wants to see, check, review, or summarize their tasks, backlog, or open bugs (e.g. 'what's on my board', 'list my tasks', 'any open bugs', 'review the backlog').",
			capability:  intent.CapabilityTrello,
			action:      intent.ActionTrelloReview,
			parameters:  `{"type":"object","properties":{}}`,
		},
	},
	"trello_card": {
		{
			name:        "trello_create_task",
			description: "File a NEW work task on the Trello Task Management board's Backlog list. Use this for project/development work items for the app — a new feature, an improvement, a chore, or a refactor — NOT for personal life goals (those belong on the bucket list, via bucketlist_add). Enrich the fields from what the user said and write them in English. Decide the label from the nature of the work: feature (new capability), improvement (enhance something that exists), chore (maintenance/config/docs, no behaviour change), refactor (restructure without behaviour change).",
			capability:  intent.CapabilityTrello,
			action:      intent.ActionTrelloCreateTask,
			parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"Short imperative summary of the work, in English (e.g. 'Add dark-mode toggle to settings')."},"description":{"type":"string","description":"1-3 sentences of context in English: what needs doing and why."},"acceptance_criteria":{"type":"string","description":"The concrete, verifiable conditions for the task to be done, in English — 2-5 items, ONE PER LINE (newline-separated), each a testable statement. Do not number them."},"label":{"type":"string","enum":["feature","improvement","chore","refactor"],"description":"The task type, choosing exactly one."}},"required":["title","description","acceptance_criteria","label"]}`,
		},
		{
			name:        "trello_report_bug",
			description: "File a NEW bug report on the Trello Issue board's Bug list. Use this when the user reports that something already built behaves wrong — broken, erroring, crashing, wrong result, or not doing what it should. Enrich the fields from what the user said and write them in English.",
			capability:  intent.CapabilityTrello,
			action:      intent.ActionTrelloReportBug,
			parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"Short summary of the defect, in English (e.g. 'Login button unresponsive on mobile Safari')."},"description":{"type":"string","description":"Context and, when known, the steps to reproduce, in English."},"actual_result":{"type":"string","description":"What currently happens — the wrong behaviour — in English."},"expected_result":{"type":"string","description":"What should happen instead, in English."}},"required":["title","actual_result","expected_result"]}`,
		},
		{
			name:        "trello_update_card",
			description: "Edit an EXISTING task card on the Trello Task Management board — retitle it, rewrite its description, replace its acceptance criteria, change or remove its type label, and/or move it to another list (Backlog / Todo / In Progress / Done, e.g. mark a task in progress or done). Identify the card by its current title (or a distinctive part of it, as shown by trello_review). Only pass the fields you want to change; omitted fields are left untouched. Use this when the user wants to change, edit, rename, re-scope, relabel, or move a task that already exists — NOT to create a new one (use trello_create_task) and NOT for bugs on the Issue board. Write any text in English.",
			capability:  intent.CapabilityTrello,
			action:      intent.ActionTrelloUpdateCard,
			parameters:  `{"type":"object","properties":{"card":{"type":"string","description":"Which task to update: its current title, or a distinctive part of it, exactly as it appears on the board (from trello_review). Matched on the Task Management board."},"title":{"type":"string","description":"New title for the card. Omit to keep the current title."},"description":{"type":"string","description":"New description/context for the card, in English. Replaces the current description. Do NOT put acceptance criteria here — use acceptance_criteria. Omit to keep the current description."},"acceptance_criteria":{"type":"string","description":"Replacement acceptance criteria — 2-5 testable items, ONE PER LINE (newline-separated), no numbering. Replaces both the Acceptance Criteria section and the trackable checklist. Omit to leave them unchanged."},"label":{"type":"string","enum":["feature","improvement","chore","refactor","none"],"description":"Change the task type label; use 'none' to remove all labels. Omit to leave the label unchanged."},"list":{"type":"string","enum":["Backlog","Todo","In Progress","Done"],"description":"Move the card to this list on the Task Management board. Omit to leave it where it is."}},"required":["card"]}`,
		},
	},
	"game_idea": {
		{
			name:        "trello_save_game_idea",
			description: "Save a video-game idea as a card on the user's Trello \"Games\" board (the Ideas list). Use this when the user shares a concept for a game they might build as a personal/side project — a mechanic, theme, pitch, or a vague one-liner. Do NOT use it for app work items (use trello_create_task / trello_report_bug) or personal life goals (use bucketlist_add). Enrich the idea from what the user said plus your own game-design knowledge, and ALWAYS write the card in English.",
			capability:  intent.CapabilityTrello,
			action:      intent.ActionTrelloGameIdea,
			parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"Short, catchy name for the game idea, in English (e.g. 'Time-rewind puzzle platformer')."},"concept":{"type":"string","description":"A 1-3 sentence pitch of the core idea and what makes it fun or unique, in English."},"genre":{"type":"string","description":"Genre and style, in English (e.g. 'Roguelike deck-builder', '2D metroidvania', 'Cozy farming sim')."},"core_mechanics":{"type":"string","description":"The main gameplay mechanics or core loop, in English — 2-6 items, ONE PER LINE (newline-separated). Do not number them."},"references":{"type":"string","description":"Reference games or inspirations that capture the vibe or mechanics, in English — 2-5 items, ONE PER LINE. Name the game and add a short note or URL where helpful. Fill this in from your own knowledge even if the user named none."},"notes":{"type":"string","description":"Optional extra context in English: target platform, art style, scope, monetization, or open questions."}},"required":["title","concept"]}`,
		},
	},
	"self_tuning": {
		{
			name:        "review_skill_performance",
			description: "Review recent low-quality conversations (judged at or below a quality threshold) that involved a skill, so you can improve that skill's prompt. Returns each conversation's user input, your reply, the tools you called, the judge's score and rationale, and — for every skill that appears — that skill's current prompt. Call this first, with no arguments, to get everything you need in one shot.",
			capability:  intent.CapabilitySelfTune,
			action:      intent.ActionSelfTuneReview,
			parameters:  `{"type":"object","properties":{"max_score":{"type":"number","description":"Only include conversations whose overall quality score is at or below this (1-5 scale). Default 4.0."},"hours":{"type":"integer","description":"How many hours back to look. Default 24."}}}`,
		},
		{
			name:        "update_skill_prompt",
			description: "Save an improved instruction prompt for one of the assistant's skills. The new prompt fully replaces that skill's current prompt and takes effect immediately for every future conversation, so include everything the skill needs (keep what already works; make a focused fix). It persists across restarts and can be reverted to the shipped default from the Skills page. Never target the 'self_tuning' skill.",
			capability:  intent.CapabilitySelfTune,
			action:      intent.ActionSelfTuneApply,
			parameters:  `{"type":"object","properties":{"skill":{"type":"string","description":"The stable key of the skill to update (e.g. 'web_search', 'bucket_list'), exactly as shown by review_skill_performance."},"prompt":{"type":"string","description":"The complete new instruction prompt for the skill. This replaces the whole prompt — do not send only a diff. Preserve tool names and any required output markers exactly."},"reason":{"type":"string","description":"A one-line note on what you changed and why (for the audit log)."}},"required":["skill","prompt"]}`,
		},
	},
	"auto_triage": {
		{
			name:        "triage_scan_failures",
			description: "Scan the assistant's own recent runs for things it couldn't handle automatically — agent errors and low-quality (poorly judged) replies — and return them grouped into recurring failure patterns. Each group has a stable 'signature', how many times it occurred, the skills involved, a sample input and error, and first/last-seen timestamps; the report also includes the current prompt of every skill that appears. Call this first, with no arguments, to get everything you need to file bugs and improve prompts.",
			capability:  intent.CapabilityAutoTriage,
			action:      intent.ActionAutoTriageScan,
			parameters:  `{"type":"object","properties":{"hours":{"type":"integer","description":"How many hours back to look. Default 24."}}}`,
		},
		{
			name:        "triage_file_bug",
			description: "File a bug card on the Trello Issue board's Bug list for a recurring failure pattern found by triage_scan_failures, with built-in duplicate detection: if an open card already carries the same signature (or the same title), it adds a recurrence note to that card instead of creating a duplicate. Pass the group's 'signature' verbatim so future runs can recognise it. Enrich the description with the failure context (sample input, error, occurrence count, timestamps) and write it in English.",
			capability:  intent.CapabilityAutoTriage,
			action:      intent.ActionAutoTriageFileBug,
			parameters:  `{"type":"object","properties":{"title":{"type":"string","description":"Short summary of the failure, in English (e.g. 'Web search returns nothing for sports scores')."},"signature":{"type":"string","description":"The failure pattern's stable signature, copied verbatim from the triage scan group. Used to detect duplicates."},"description":{"type":"string","description":"The bug body in English/Markdown: what fails, the sample input, the error, how many times it recurred, and the first/last-seen times. Include enough context to reproduce."},"recurrence_note":{"type":"string","description":"Optional short note to add if a card for this pattern already exists (e.g. 'Recurred 3 more times on 2026-07-19'). A sensible default is used if omitted."}},"required":["title","signature","description"]}`,
		},
		{
			name:        "triage_improve_prompt",
			description: "Save an improved instruction prompt for one of the assistant's skills whose runs keep failing in the triage scan. The new prompt fully replaces that skill's current prompt and takes effect immediately, so include everything it needs (keep what works; make a focused fix for the recurring failure, and preserve tool names and required output markers exactly). Persists across restarts and can be reverted from the Skills page. Never target the 'auto_triage' or 'self_tuning' skills.",
			capability:  intent.CapabilityAutoTriage,
			action:      intent.ActionAutoTriageImprovePrompt,
			parameters:  `{"type":"object","properties":{"skill":{"type":"string","description":"The stable key of the skill to update (e.g. 'web_search', 'trello_card'), exactly as shown in the scan's current_skill_prompts."},"prompt":{"type":"string","description":"The complete new instruction prompt for the skill. Replaces the whole prompt — do not send only a diff. Preserve tool names and any required output markers exactly."},"reason":{"type":"string","description":"A one-line note on what you changed and why (for the audit log)."}},"required":["skill","prompt"]}`,
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

// toolSkillOwner maps every skill-provided tool name back to the skill key that
// exposes it. Base tools (toolSpecs) are always-on and have no owning skill, so
// they are intentionally absent.
var toolSkillOwner = func() map[string]string {
	m := make(map[string]string)
	for key, ts := range skillTools {
		for _, t := range ts {
			m[t.name] = key
		}
	}
	return m
}()

// SkillsForTools returns the distinct skill keys whose tools appear in the given
// invoked tool names, preserving first-seen order. Tools with no owning skill
// (always-on base tools) are ignored. Used to show only the skills actually
// exercised by a conversation, rather than every enabled skill.
func SkillsForTools(toolNames []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, name := range toolNames {
		key, ok := toolSkillOwner[name]
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
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
