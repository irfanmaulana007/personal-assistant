# Future Capabilities (Phase 3+)

## Smart Home Control

Integration with Home Assistant for controlling IoT devices.

| Commands | Action |
|----------|--------|
| "Turn off the living room lights" | Control lights |
| "Set thermostat to 22°C" | Adjust climate |
| "Is the front door locked?" | Query sensor state |
| "Arm the security system" | Control security |

**Integration:** Home Assistant REST API or WebSocket API

## Finance Tracking

Simple expense logging and budget queries.

| Commands | Action |
|----------|--------|
| "Log expense: lunch 45k" | Record expense |
| "How much did I spend this week?" | Spending summary |
| "Show expenses for food this month" | Category report |
| "Set budget for food: 2M/month" | Budget management |

**Storage:** SQLite tables for transactions and budgets

## Health & Fitness

Query health data and log activities.

| Commands | Action |
|----------|--------|
| "Log workout: 30 min run" | Record exercise |
| "How many workouts this week?" | Activity summary |
| "Log water: 500ml" | Hydration tracking |
| "Track weight: 75kg" | Weight logging |

**Storage:** SQLite tables for health data

## Daily Briefings

Proactive morning summary sent at a configured time.

```
Good morning! Here's your briefing for Monday, Jul 7:

Calendar:
  - 09:00 Team standup
  - 14:00 1:1 with Manager
  - 16:30 Code review

Weather:
  Jakarta: 28-34°C, Partly cloudy

Email:
  3 unread emails since last night

Reminders:
  - Submit report (due today)
```

**Implementation:** Cron-like scheduler that assembles data from all capabilities.

## Workflow Automation

Chain multiple actions into a single command.

| Commands | Action |
|----------|--------|
| "Schedule standup and remind me 5 min before" | Calendar + Reminder |
| "Check my 2pm meeting and email Alice I'll be late" | Calendar + Email |

**Implementation:** Intent parser detects compound commands and executes sequentially.

## URL Bookmarking

Save and categorize URLs.

| Commands | Action |
|----------|--------|
| "Bookmark https://example.com as reference" | Save URL |
| "Show my bookmarks tagged go" | List by tag |

## Translation

Quick text translation.

| Commands | Action |
|----------|--------|
| "Translate 'hello' to Japanese" | Translate text |
| "How do you say 'thank you' in Korean?" | Translate phrase |

**Integration:** Google Translate API or LibreTranslate (self-hosted)

## Priority & Timeline

| Capability | Phase | Priority |
|-----------|-------|----------|
| Smart home | 4 | Medium |
| Finance tracking | 4 | Medium |
| Health/fitness | 4 | Low |
| Daily briefings | 3 | High |
| Workflow automation | 4 | Medium |
| URL bookmarking | 3 | Low |
| Translation | 3 | Low |

Priorities may shift based on personal usage patterns and needs.
