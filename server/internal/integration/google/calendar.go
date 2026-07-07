package google

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// CalendarClient wraps the Google Calendar API.
type CalendarClient struct {
	auth     *Auth
	timezone *time.Location
	log      *slog.Logger
}

// NewCalendarClient creates a new Calendar API client.
func NewCalendarClient(auth *Auth, timezone *time.Location, log *slog.Logger) *CalendarClient {
	return &CalendarClient{
		auth:     auth,
		timezone: timezone,
		log:      log.With("component", "google-calendar"),
	}
}

// CalendarEvent represents a simplified calendar event.
type CalendarEvent struct {
	ID        string
	Title     string
	Start     time.Time
	End       time.Time
	Location  string
	AllDay    bool
}

func (c *CalendarClient) service(ctx context.Context) (*calendar.Service, error) {
	ts, err := c.auth.TokenSource(ctx)
	if err != nil {
		return nil, err
	}
	return calendar.NewService(ctx, option.WithTokenSource(ts))
}

// ListEvents returns events between the given times.
func (c *CalendarClient) ListEvents(ctx context.Context, timeMin, timeMax time.Time, maxResults int) ([]CalendarEvent, error) {
	srv, err := c.service(ctx)
	if err != nil {
		return nil, fmt.Errorf("create calendar service: %w", err)
	}

	events, err := srv.Events.List("primary").
		TimeMin(timeMin.Format(time.RFC3339)).
		TimeMax(timeMax.Format(time.RFC3339)).
		MaxResults(int64(maxResults)).
		SingleEvents(true).
		OrderBy("startTime").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}

	var result []CalendarEvent
	for _, item := range events.Items {
		evt := CalendarEvent{
			ID:       item.Id,
			Title:    item.Summary,
			Location: item.Location,
		}

		if item.Start.DateTime != "" {
			evt.Start, _ = time.Parse(time.RFC3339, item.Start.DateTime)
			evt.End, _ = time.Parse(time.RFC3339, item.End.DateTime)
		} else {
			evt.Start, _ = time.Parse("2006-01-02", item.Start.Date)
			evt.End, _ = time.Parse("2006-01-02", item.End.Date)
			evt.AllDay = true
		}

		result = append(result, evt)
	}

	return result, nil
}

// CreateEvent creates a new calendar event.
func (c *CalendarClient) CreateEvent(ctx context.Context, title string, start, end time.Time, location string) (*CalendarEvent, error) {
	srv, err := c.service(ctx)
	if err != nil {
		return nil, fmt.Errorf("create calendar service: %w", err)
	}

	event := &calendar.Event{
		Summary:  title,
		Location: location,
		Start: &calendar.EventDateTime{
			DateTime: start.Format(time.RFC3339),
			TimeZone: c.timezone.String(),
		},
		End: &calendar.EventDateTime{
			DateTime: end.Format(time.RFC3339),
			TimeZone: c.timezone.String(),
		},
	}

	created, err := srv.Events.Insert("primary", event).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("create event: %w", err)
	}

	return &CalendarEvent{
		ID:    created.Id,
		Title: created.Summary,
		Start: start,
		End:   end,
	}, nil
}

// UpdateEvent updates an existing calendar event.
func (c *CalendarClient) UpdateEvent(ctx context.Context, eventID string, title string, start, end time.Time) error {
	srv, err := c.service(ctx)
	if err != nil {
		return fmt.Errorf("create calendar service: %w", err)
	}

	patch := &calendar.Event{}
	if title != "" {
		patch.Summary = title
	}

	zeroTime := time.Time{}
	if start != zeroTime {
		patch.Start = &calendar.EventDateTime{
			DateTime: start.Format(time.RFC3339),
			TimeZone: c.timezone.String(),
		}
	}
	if end != zeroTime {
		patch.End = &calendar.EventDateTime{
			DateTime: end.Format(time.RFC3339),
			TimeZone: c.timezone.String(),
		}
	}

	_, err = srv.Events.Patch("primary", eventID, patch).Context(ctx).Do()
	return err
}

// DeleteEvent deletes a calendar event.
func (c *CalendarClient) DeleteEvent(ctx context.Context, eventID string) error {
	srv, err := c.service(ctx)
	if err != nil {
		return fmt.Errorf("create calendar service: %w", err)
	}

	return srv.Events.Delete("primary", eventID).Context(ctx).Do()
}

// FindEvent searches for an event by title.
func (c *CalendarClient) FindEvent(ctx context.Context, title string) (*CalendarEvent, error) {
	srv, err := c.service(ctx)
	if err != nil {
		return nil, fmt.Errorf("create calendar service: %w", err)
	}

	now := time.Now().In(c.timezone)
	events, err := srv.Events.List("primary").
		Q(title).
		TimeMin(now.Add(-24 * time.Hour).Format(time.RFC3339)).
		TimeMax(now.Add(30 * 24 * time.Hour).Format(time.RFC3339)).
		MaxResults(1).
		SingleEvents(true).
		OrderBy("startTime").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("search events: %w", err)
	}

	if len(events.Items) == 0 {
		return nil, nil
	}

	item := events.Items[0]
	evt := &CalendarEvent{
		ID:    item.Id,
		Title: item.Summary,
	}
	if item.Start.DateTime != "" {
		evt.Start, _ = time.Parse(time.RFC3339, item.Start.DateTime)
		evt.End, _ = time.Parse(time.RFC3339, item.End.DateTime)
	}
	return evt, nil
}

// tokenSource is a helper type for creating services with token source.
type tokenSourceAdapter struct {
	ts oauth2.TokenSource
}

func (t *tokenSourceAdapter) Token() (*oauth2.Token, error) {
	return t.ts.Token()
}
