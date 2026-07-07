# Google Calendar Integration

## Overview

Integration with Google Calendar API v3 to manage events — view, create, update, and delete.

## Prerequisites

1. Create a Google Cloud project at [console.cloud.google.com](https://console.cloud.google.com)
2. Enable the Google Calendar API
3. Create OAuth2 credentials (Desktop application type)
4. Download `credentials.json` and place in the config directory

## OAuth2 Flow

### Initial Authorization

1. On first run, the assistant opens a browser URL for Google consent
2. User grants calendar access permissions
3. Authorization code is exchanged for access + refresh tokens
4. Tokens are encrypted and stored in SQLite (`oauth_tokens` table)

### Token Refresh

```go
func getCalendarService(ctx context.Context, store *store.Store) (*calendar.Service, error) {
    tokenData, err := store.GetOAuthToken("google_calendar")
    token := decrypt(tokenData)

    config := oauth2Config() // from credentials.json
    client := config.Client(ctx, token)

    // Token auto-refreshes via oauth2 package
    // Save refreshed token back to store
    newToken, err := config.TokenSource(ctx, token).Token()
    if newToken.AccessToken != token.AccessToken {
        store.SaveOAuthToken("google_calendar", encrypt(newToken))
    }

    return calendar.NewService(ctx, option.WithHTTPClient(client))
}
```

### Required Scopes

```go
var calendarScopes = []string{
    "https://www.googleapis.com/auth/calendar.readonly",
    "https://www.googleapis.com/auth/calendar.events",
}
```

## API Operations

### List Events (Today / Upcoming)

```go
func listEvents(srv *calendar.Service, timeMin, timeMax time.Time) ([]*calendar.Event, error) {
    events, err := srv.Events.List("primary").
        TimeMin(timeMin.Format(time.RFC3339)).
        TimeMax(timeMax.Format(time.RFC3339)).
        SingleEvents(true).
        OrderBy("startTime").
        MaxResults(10).
        Do()
    return events.Items, err
}
```

**Response format example:**
```
Your schedule for today (Jul 7):
- 09:00 - 10:00  Team standup
- 14:00 - 15:00  1:1 with Manager
- 16:30 - 17:00  Code review
```

### Create Event

```go
func createEvent(srv *calendar.Service, summary string, start, end time.Time) (*calendar.Event, error) {
    event := &calendar.Event{
        Summary: summary,
        Start:   &calendar.EventDateTime{DateTime: start.Format(time.RFC3339)},
        End:     &calendar.EventDateTime{DateTime: end.Format(time.RFC3339)},
    }
    return srv.Events.Insert("primary", event).Do()
}
```

### Update Event

```go
func updateEvent(srv *calendar.Service, eventID string, updates *calendar.Event) (*calendar.Event, error) {
    return srv.Events.Patch("primary", eventID, updates).Do()
}
```

### Delete Event

```go
func deleteEvent(srv *calendar.Service, eventID string) error {
    return srv.Events.Delete("primary", eventID).Do()
}
```

## Error Handling

| Error | Action |
|-------|--------|
| 401 Unauthorized | Refresh token, retry once |
| 403 Rate limit | Back off and retry with exponential delay |
| 404 Not found | Inform user the event doesn't exist |
| Network error | Inform user, suggest retrying |

## Timezone Handling

- Store user's timezone in config (e.g., `Asia/Jakarta`)
- All times displayed in user's local timezone
- All API calls use RFC3339 with timezone offset
- Natural language time parsing should account for timezone (e.g., "3pm" = 15:00 in user's TZ)
