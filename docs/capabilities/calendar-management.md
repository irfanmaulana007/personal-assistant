# Calendar Management

## Overview

Manage Google Calendar events through conversational commands — view schedule, create events, update, and delete.

**Integration:** [Google Calendar API](../integrations/google-calendar.md)

## Commands

### View Schedule

| Example Messages | Intent |
|-----------------|--------|
| "What's on my calendar today?" | List today's events |
| "Show my schedule for tomorrow" | List tomorrow's events |
| "What meetings do I have this week?" | List this week's events |
| "Am I free at 3pm?" | Check availability |

**Response format:**
```
Your schedule for today (Mon, Jul 7):

  09:00 - 10:00  Team standup
  14:00 - 15:00  1:1 with Manager
  16:30 - 17:00  Code review

You have 3 events and are free from 10:00-14:00 and 15:00-16:30.
```

If no events:
```
You have no events scheduled for today. Your calendar is clear!
```

### Create Event

| Example Messages | Intent |
|-----------------|--------|
| "Schedule a meeting tomorrow at 2pm" | Create event |
| "Add dentist appointment on Friday at 10am for 1 hour" | Create event with duration |
| "Block 3-5pm today for deep work" | Create event with time range |

**Flow:**
1. Parse: title, date, start time, duration/end time
2. Confirm with user before creating
3. Create event via API
4. Return confirmation

**Confirmation prompt:**
```
I'll create this event:
  Title: Dentist appointment
  Date: Fri, Jul 11
  Time: 10:00 - 11:00

Create this event? (yes/no)
```

### Update Event

| Example Messages | Intent |
|-----------------|--------|
| "Move my 2pm meeting to 3pm" | Reschedule |
| "Rename standup to sprint planning" | Update title |
| "Extend my 3pm meeting by 30 minutes" | Update duration |

**Flow:**
1. Find the matching event (by time and/or title)
2. If ambiguous, list matches and ask user to pick
3. Show proposed change and confirm
4. Update via API

### Delete Event

| Example Messages | Intent |
|-----------------|--------|
| "Cancel my 2pm meeting" | Delete event |
| "Remove dentist appointment" | Delete event |

**Flow:**
1. Find the matching event
2. Confirm deletion: "Delete 'Dentist appointment' on Fri Jul 11 at 10:00? (yes/no)"
3. Delete via API

## Entity Extraction

| Entity | Patterns | Examples |
|--------|----------|---------|
| Date | today, tomorrow, Monday, Jul 11, next week | "tomorrow", "on Friday", "next Monday" |
| Time | 2pm, 14:00, 2:30 PM | "at 3pm", "at 14:00" |
| Duration | 1 hour, 30 minutes, 1h30m | "for 1 hour", "for 30 min" |
| Title | remaining text after extracting date/time | "dentist", "team meeting" |

## Edge Cases

| Case | Behavior |
|------|----------|
| No date specified for create | Assume today |
| No duration specified | Default to 1 hour |
| No end time | Calculate from start + duration |
| Multiple events match | List matches, ask user to pick by number |
| Event in the past | Warn user but allow creation |
| Overlapping event | Warn about conflict, ask to proceed |
| All-day event | Support "all day" keyword |

## Regex Patterns (MVP)

```go
var calendarPatterns = []regexp.Regexp{
    regexp.MustCompile(`(?i)(what'?s|show|check).*(calendar|schedule|meeting|event)`),
    regexp.MustCompile(`(?i)(am i|are you).*(free|available|busy)`),
    regexp.MustCompile(`(?i)(schedule|create|add|book|block).*(meeting|event|appointment|call)`),
    regexp.MustCompile(`(?i)(move|reschedule|change|update).*(meeting|event|appointment)`),
    regexp.MustCompile(`(?i)(cancel|delete|remove).*(meeting|event|appointment)`),
}
```
