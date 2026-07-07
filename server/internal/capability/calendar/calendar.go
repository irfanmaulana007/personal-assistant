package calendar

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/irfanmaulana007/personal-assistant/server/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/server/internal/integration/google"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
)

// Handler handles calendar-related commands.
type Handler struct {
	client          *google.CalendarClient
	timezone        *time.Location
	defaultDuration time.Duration
	maxResults      int
}

// New creates a new calendar handler.
func New(client *google.CalendarClient, timezone *time.Location, defaultDuration string, maxResults int) *Handler {
	dur, err := time.ParseDuration(defaultDuration)
	if err != nil {
		dur = time.Hour
	}
	return &Handler{
		client:          client,
		timezone:        timezone,
		defaultDuration: dur,
		maxResults:      maxResults,
	}
}

func (h *Handler) Name() string { return "calendar" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityCalendar
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionCalendarList:
		return h.list(ctx, result)
	case intent.ActionCalendarCreate:
		return h.create(ctx, result)
	case intent.ActionCalendarUpdate:
		return h.update(ctx, result)
	case intent.ActionCalendarDelete:
		return h.delete(ctx, result)
	default:
		return "I can view, create, update, or delete calendar events. Try: _show my calendar_", nil
	}
}

func (h *Handler) list(ctx context.Context, result *intent.ParseResult) (string, error) {
	dateStr := result.Entities["date"]
	timeMin, timeMax := capability.ParseDateRange(dateStr, h.timezone)

	events, err := h.client.ListEvents(ctx, timeMin, timeMax, h.maxResults)
	if err != nil {
		return "", fmt.Errorf("list events: %w", err)
	}

	if len(events) == 0 {
		label := "today"
		if dateStr != "" {
			label = dateStr
		}
		return fmt.Sprintf("No events scheduled for %s.", label), nil
	}

	var sb strings.Builder
	label := "Today"
	if dateStr != "" {
		label = cases.Title(language.English).String(dateStr)
	}
	sb.WriteString(fmt.Sprintf("*%s's Schedule* (%s):\n\n", label, timeMin.Format("Mon, Jan 2")))

	for i, evt := range events {
		if evt.AllDay {
			sb.WriteString(fmt.Sprintf("%d. *%s* — All day\n", i+1, evt.Title))
		} else {
			start := evt.Start.In(h.timezone).Format("3:04 PM")
			end := evt.End.In(h.timezone).Format("3:04 PM")
			sb.WriteString(fmt.Sprintf("%d. *%s* — %s to %s\n", i+1, evt.Title, start, end))
		}
		if evt.Location != "" {
			sb.WriteString(fmt.Sprintf("   📍 %s\n", evt.Location))
		}
	}

	return sb.String(), nil
}

func (h *Handler) create(ctx context.Context, result *intent.ParseResult) (string, error) {
	title := result.Entities["title"]
	if title == "" {
		return "Please specify a title for the event. Example: _schedule meeting Team Sync at 3pm_", nil
	}

	datetimeStr := result.Entities["datetime"]
	var start time.Time
	var err error

	if datetimeStr != "" {
		start, err = capability.ParseTime(datetimeStr, h.timezone)
		if err != nil {
			return fmt.Sprintf("I couldn't understand the time %q. Try something like: _at 3pm_, _tomorrow at 2pm_", datetimeStr), nil
		}
	} else {
		// Default to next hour
		now := time.Now().In(h.timezone)
		start = now.Truncate(time.Hour).Add(time.Hour)
	}

	end := start.Add(h.defaultDuration)

	evt, err := h.client.CreateEvent(ctx, title, start, end, "")
	if err != nil {
		return "", fmt.Errorf("create event: %w", err)
	}

	return fmt.Sprintf("Event created: *%s*\n%s — %s",
		evt.Title,
		evt.Start.In(h.timezone).Format("Mon, Jan 2 at 3:04 PM"),
		evt.End.In(h.timezone).Format("3:04 PM"),
	), nil
}

func (h *Handler) update(ctx context.Context, result *intent.ParseResult) (string, error) {
	title := result.Entities["title"]
	if title == "" {
		return "Please specify which event to update. Example: _reschedule meeting Team Sync to 4pm_", nil
	}

	evt, err := h.client.FindEvent(ctx, title)
	if err != nil {
		return "", fmt.Errorf("find event: %w", err)
	}
	if evt == nil {
		return fmt.Sprintf("I couldn't find an event matching %q.", title), nil
	}

	datetimeStr := result.Entities["datetime"]
	if datetimeStr == "" {
		return fmt.Sprintf("Found *%s*. When should I reschedule it to?", evt.Title), nil
	}

	newStart, err := capability.ParseTime(datetimeStr, h.timezone)
	if err != nil {
		return fmt.Sprintf("I couldn't understand the time %q.", datetimeStr), nil
	}

	duration := evt.End.Sub(evt.Start)
	newEnd := newStart.Add(duration)

	if err := h.client.UpdateEvent(ctx, evt.ID, "", newStart, newEnd); err != nil {
		return "", fmt.Errorf("update event: %w", err)
	}

	return fmt.Sprintf("Event *%s* rescheduled to %s — %s",
		evt.Title,
		newStart.In(h.timezone).Format("Mon, Jan 2 at 3:04 PM"),
		newEnd.In(h.timezone).Format("3:04 PM"),
	), nil
}

func (h *Handler) delete(ctx context.Context, result *intent.ParseResult) (string, error) {
	title := result.Entities["title"]
	if title == "" {
		return "Please specify which event to delete. Example: _cancel meeting Team Sync_", nil
	}

	evt, err := h.client.FindEvent(ctx, title)
	if err != nil {
		return "", fmt.Errorf("find event: %w", err)
	}
	if evt == nil {
		return fmt.Sprintf("I couldn't find an event matching %q.", title), nil
	}

	if err := h.client.DeleteEvent(ctx, evt.ID); err != nil {
		return "", fmt.Errorf("delete event: %w", err)
	}

	return fmt.Sprintf("Event *%s* has been deleted.", evt.Title), nil
}
