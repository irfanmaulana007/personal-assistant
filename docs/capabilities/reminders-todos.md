# Reminders & To-Dos

## Overview

Set, list, and manage time-based reminders. Reminders are stored in SQLite and delivered via a background scheduler.

**Storage:** SQLite `reminders` table (see [Data Model](../architecture/data-model.md))

## Commands

### Set Reminder

| Example Messages | Intent |
|-----------------|--------|
| "Remind me to call the dentist at 3pm" | Absolute time |
| "Remind me in 30 minutes to check the oven" | Relative time |
| "Remind me tomorrow at 9am to submit the report" | Date + time |
| "Remind me on Friday to buy groceries" | Date (default 9am) |

**Response:**
```
Reminder set:
  "Call the dentist"
  When: Today at 3:00 PM (in 2 hours)
```

### List Reminders

| Example Messages | Intent |
|-----------------|--------|
| "Show my reminders" | List active reminders |
| "What reminders do I have?" | List active reminders |
| "List upcoming reminders" | List active reminders |

**Response:**
```
Your active reminders:

1. Call the dentist — Today at 3:00 PM (in 2 hours)
2. Submit the report — Tomorrow at 9:00 AM
3. Buy groceries — Fri, Jul 11 at 9:00 AM

Reply "cancel 1" to remove a reminder.
```

### Cancel Reminder

| Example Messages | Intent |
|-----------------|--------|
| "Cancel reminder 1" | Cancel by number |
| "Remove the dentist reminder" | Cancel by keyword |
| "Cancel all reminders" | Cancel all (with confirmation) |

**Response:**
```
Cancelled: "Call the dentist" (was set for Today at 3:00 PM)
```

## Background Scheduler

A goroutine checks for due reminders every 30 seconds:

```go
func (s *Scheduler) Run(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            reminders, err := s.store.GetDueReminders(time.Now())
            for _, r := range reminders {
                s.transport.SendMessage(ctx, s.ownerJID, fmt.Sprintf(
                    "Reminder: %s", r.Message,
                ))
                s.store.MarkNotified(r.ID)
            }
        }
    }
}
```

## Time Parsing

### Supported Formats

| Format | Examples |
|--------|---------|
| Relative | "in 5 minutes", "in 1 hour", "in 2 hours 30 min" |
| Absolute time | "at 3pm", "at 15:00", "at 3:30 PM" |
| Date + time | "tomorrow at 9am", "on Friday at 2pm", "Jul 15 at noon" |
| Date only | "tomorrow", "on Friday", "Jul 15" (defaults to 9:00 AM) |
| Named times | "noon" (12:00), "midnight" (00:00), "morning" (9:00), "evening" (18:00) |

### Parsing Strategy (MVP)

Use regex to extract time components:

```go
var timePatterns = map[string]*regexp.Regexp{
    "relative_minutes": regexp.MustCompile(`(?i)in\s+(\d+)\s*min`),
    "relative_hours":   regexp.MustCompile(`(?i)in\s+(\d+)\s*hour`),
    "absolute_time":    regexp.MustCompile(`(?i)at\s+(\d{1,2}):?(\d{2})?\s*(am|pm)?`),
    "date_tomorrow":    regexp.MustCompile(`(?i)tomorrow`),
    "date_weekday":     regexp.MustCompile(`(?i)(monday|tuesday|wednesday|thursday|friday|saturday|sunday)`),
}
```

## Entity Extraction

| Entity | Extraction |
|--------|-----------|
| Message | Remaining text after removing time/date phrases |
| Time | Parsed from time patterns above |
| Date | Parsed from date patterns, defaults to today |

## Edge Cases

| Case | Behavior |
|------|----------|
| Time in the past | "That time has already passed. Did you mean tomorrow at 3pm?" |
| No time specified | "When should I remind you?" |
| No message specified | "What should I remind you about?" |
| Duplicate reminder | Allow it (user might want multiple) |
| Very far future (> 1 year) | Warn but allow |
| Reminder during quiet hours | Respect quiet hours config if set |

## Database Queries

```sql
-- Get due reminders
SELECT * FROM reminders
WHERE remind_at <= ? AND notified = FALSE AND cancelled = FALSE
ORDER BY remind_at;

-- Get active reminders
SELECT * FROM reminders
WHERE notified = FALSE AND cancelled = FALSE
ORDER BY remind_at;

-- Cancel reminder
UPDATE reminders SET cancelled = TRUE WHERE id = ?;
```
