package intent

import (
	"regexp"
	"strings"
)

type patternRule struct {
	pattern    *regexp.Regexp
	capability Capability
	action     Action
	extractors []entityExtractor
}

type entityExtractor struct {
	name  string
	group int // regex capture group index
}

// RegexParser implements intent parsing using regex pattern matching.
type RegexParser struct {
	rules []patternRule
}

// NewRegexParser creates a new regex-based parser with predefined patterns.
func NewRegexParser() *RegexParser {
	p := &RegexParser{}
	p.registerRules()
	return p
}

func (p *RegexParser) registerRules() {
	p.rules = []patternRule{
		// --- Help ---
		{
			pattern:    re(`^(?:help|commands|what can you do|menu|\?)$`),
			capability: CapabilityHelp,
			action:     ActionHelp,
		},

		// --- Calendar ---
		{
			pattern:    re(`(?:what(?:'s| is) on |show |view |list |check )?(?:my )?(?:calendar|schedule|events?|agenda)(?:\s+(?:for\s+)?(.+))?`),
			capability: CapabilityCalendar,
			action:     ActionCalendarList,
			extractors: []entityExtractor{{name: "date", group: 1}},
		},
		{
			pattern:    re(`(?:am i|are we) (?:free|busy|available)(?:\s+(.+))?`),
			capability: CapabilityCalendar,
			action:     ActionCalendarList,
			extractors: []entityExtractor{{name: "date", group: 1}},
		},
		{
			pattern:    re(`(?:schedule|create|add|set up|book) (?:a )?(?:meeting|event|appointment|call)(?:\s+(?:with|about|for|called|titled|named))?\s+(.+?)(?:\s+(?:on|at|for|from)\s+(.+))?$`),
			capability: CapabilityCalendar,
			action:     ActionCalendarCreate,
			extractors: []entityExtractor{{name: "title", group: 1}, {name: "datetime", group: 2}},
		},
		{
			pattern:    re(`(?:move|reschedule|update|change) (?:my )?(?:meeting|event|appointment|call)\s+(.+?)(?:\s+to\s+(.+))?$`),
			capability: CapabilityCalendar,
			action:     ActionCalendarUpdate,
			extractors: []entityExtractor{{name: "title", group: 1}, {name: "datetime", group: 2}},
		},
		{
			pattern:    re(`(?:delete|remove|cancel) (?:my )?(?:meeting|event|appointment|call)\s+(.+)`),
			capability: CapabilityCalendar,
			action:     ActionCalendarDelete,
			extractors: []entityExtractor{{name: "title", group: 1}},
		},

		// --- Email ---
		{
			pattern:    re(`(?:check|show|view|list|read|get) (?:my )?(?:email|inbox|mail|messages?)(?:\s+(.+))?`),
			capability: CapabilityEmail,
			action:     ActionEmailInbox,
			extractors: []entityExtractor{{name: "filter", group: 1}},
		},
		{
			pattern:    re(`(?:read|open|show) email (?:#|number\s*)?(\d+)`),
			capability: CapabilityEmail,
			action:     ActionEmailRead,
			extractors: []entityExtractor{{name: "index", group: 1}},
		},
		{
			pattern:    re(`(?:search|find|look for) (?:email|mail|messages?)(?:\s+(?:about|from|for|with))?\s+(.+)`),
			capability: CapabilityEmail,
			action:     ActionEmailSearch,
			extractors: []entityExtractor{{name: "query", group: 1}},
		},
		{
			pattern:    re(`(?:draft|write|compose|reply to) (?:a )?(?:reply|email|response)(?:\s+to\s+(.+?))?(?:\s*:\s*(.+))?$`),
			capability: CapabilityEmail,
			action:     ActionEmailDraft,
			extractors: []entityExtractor{{name: "to", group: 1}, {name: "body", group: 2}},
		},

		// --- Reminders ---
		{
			pattern:    re(`(?:remind me|set (?:a )?reminder|reminder)\s+(?:to\s+)?(.+?)(?:\s+(?:in|at|on)\s+(.+))$`),
			capability: CapabilityReminder,
			action:     ActionReminderSet,
			extractors: []entityExtractor{{name: "message", group: 1}, {name: "time", group: 2}},
		},
		{
			pattern:    re(`(?:remind me|set (?:a )?reminder|reminder)\s+(?:to\s+)?(.+)`),
			capability: CapabilityReminder,
			action:     ActionReminderSet,
			extractors: []entityExtractor{{name: "message", group: 1}},
		},
		{
			pattern:    re(`(?:list|show|view|check|what are) (?:my )?reminders?`),
			capability: CapabilityReminder,
			action:     ActionReminderList,
		},
		{
			pattern:    re(`(?:cancel|delete|remove) reminder (?:#|number\s*)?(\d+)`),
			capability: CapabilityReminder,
			action:     ActionReminderCancel,
			extractors: []entityExtractor{{name: "id", group: 1}},
		},

		// --- Knowledge Base ---
		{
			pattern:    re(`(?:save|add|create|write) (?:a )?note(?:\s+(?:titled?|called|named))?\s+(.+?)(?:\s*:\s*(.+))?$`),
			capability: CapabilityKnowledge,
			action:     ActionNoteSave,
			extractors: []entityExtractor{{name: "title", group: 1}, {name: "content", group: 2}},
		},
		{
			pattern:    re(`(?:search|find|look for|look up) (?:my )?notes?(?:\s+(?:about|for|on|with))?\s+(.+)`),
			capability: CapabilityKnowledge,
			action:     ActionNoteSearch,
			extractors: []entityExtractor{{name: "query", group: 1}},
		},
		{
			pattern:    re(`(?:list|show|view) (?:my )?notes?(?:\s+(?:tagged?|with tag|in)\s+(.+))?`),
			capability: CapabilityKnowledge,
			action:     ActionNoteList,
			extractors: []entityExtractor{{name: "tag", group: 1}},
		},
		{
			pattern:    re(`(?:delete|remove) note (?:#|number\s*)?(\d+)`),
			capability: CapabilityKnowledge,
			action:     ActionNoteDelete,
			extractors: []entityExtractor{{name: "id", group: 1}},
		},
	}
}

func (p *RegexParser) Parse(text string) *ParseResult {
	normalized := strings.TrimSpace(strings.ToLower(text))

	for _, rule := range p.rules {
		matches := rule.pattern.FindStringSubmatch(normalized)
		if matches == nil {
			continue
		}

		entities := make(map[string]string)
		for _, ext := range rule.extractors {
			if ext.group < len(matches) && matches[ext.group] != "" {
				entities[ext.name] = strings.TrimSpace(matches[ext.group])
			}
		}

		return &ParseResult{
			Capability: rule.capability,
			Action:     rule.action,
			Entities:   entities,
			Confidence: 1.0,
			RawText:    text,
		}
	}

	return &ParseResult{
		Capability: CapabilityUnknown,
		Action:     "",
		Entities:   map[string]string{},
		Confidence: 0.0,
		RawText:    text,
	}
}

func re(pattern string) *regexp.Regexp {
	return regexp.MustCompile("(?i)" + pattern)
}
