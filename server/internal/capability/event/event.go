// Package event handles one-time/dated events. When the user has a connected
// Google Calendar the event is added there; otherwise it falls back to a
// one-time reminder so nothing is lost.
package event

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/calendar"
	"github.com/irfanmaulana007/personal-assistant/server/internal/capability"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

// Calendar is the subset of the calendar service this handler needs (an
// interface so the logic is unit-testable without Composio).
type Calendar interface {
	HasCalendar(ctx context.Context, userID int64) bool
	CreateEvent(ctx context.Context, userID int64, ev calendar.Event) error
	ListEvents(ctx context.Context, userID int64, from, to time.Time) []calendar.Event
}

// Handler creates one-time events (calendar-first, reminder-fallback).
type Handler struct {
	calendar Calendar
	store    store.Store
	timezone *time.Location
	log      *slog.Logger
}

// New creates an event handler. cal may be nil (always falls back to a reminder).
func New(cal Calendar, s store.Store, tz *time.Location, log *slog.Logger) *Handler {
	return &Handler{calendar: cal, store: s, timezone: tz, log: log.With("component", "event")}
}

func (h *Handler) Name() string { return "event" }

func (h *Handler) Match(result *intent.ParseResult) bool {
	return result.Capability == intent.CapabilityEvent
}

func (h *Handler) Handle(ctx context.Context, result *intent.ParseResult) (string, error) {
	switch result.Action {
	case intent.ActionEventCreate:
		return h.create(ctx, result)
	case intent.ActionEventAgenda:
		return h.agenda(ctx, result)
	default:
		return "I can add a one-time event to your calendar.", nil
	}
}

// agenda lists upcoming Google Calendar events across all connected accounts and
// calendars (the calendar half of the user's schedule; reminders come from
// reminder_list).
func (h *Handler) agenda(ctx context.Context, result *intent.ParseResult) (string, error) {
	if h.calendar == nil {
		return "No Google Calendar is connected.", nil
	}
	userID := authctx.UserID(ctx)
	days := 7
	if d, err := strconv.Atoi(result.Entities["days"]); err == nil && d > 0 && d <= 60 {
		days = d
	}
	now := time.Now().In(h.timezone)
	from := now
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.timezone).AddDate(0, 0, days)

	events := h.calendar.ListEvents(ctx, userID, from, to)
	if len(events) == 0 {
		return "No calendar events in that window (or no Google Calendar connected).", nil
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Calendar* (next %d days):\n", days))
	for _, ev := range events {
		when := ev.Start.Format("Mon, Jan 2 at 3:04 PM")
		if ev.AllDay {
			when = ev.Start.Format("Mon, Jan 2") + " (all day)"
		}
		b.WriteString(fmt.Sprintf("\n• %s — %s", when, ev.Title))
	}
	return b.String(), nil
}

func (h *Handler) create(ctx context.Context, result *intent.ParseResult) (string, error) {
	title := firstNonEmpty(result.Entities["title"], result.Entities["message"])
	if title == "" {
		return "What is the event?", nil
	}
	whenStr := firstNonEmpty(result.Entities["datetime"], result.Entities["time"])
	if whenStr == "" {
		return fmt.Sprintf("When is %q? For example, _tomorrow at 3pm_ or _Aug 5 at 2pm_.", title), nil
	}
	when, err := capability.ParseTime(whenStr, h.timezone)
	if err != nil {
		return fmt.Sprintf("I couldn't understand %q as a date/time. Try _tomorrow at 3pm_ or _2026-08-05 14:00_.", whenStr), nil
	}
	local := when.In(h.timezone)

	dur := time.Hour
	if m, err := strconv.Atoi(result.Entities["duration_minutes"]); err == nil && m > 0 {
		dur = time.Duration(m) * time.Minute
	}

	userID := authctx.UserID(ctx)

	// Calendar-first.
	if h.calendar != nil && h.calendar.HasCalendar(ctx, userID) {
		ev := calendar.Event{
			Title:    title,
			Location: result.Entities["location"],
			Start:    when,
			End:      when.Add(dur),
		}
		if err := h.calendar.CreateEvent(ctx, userID, ev); err != nil {
			h.log.Warn("create calendar event", "error", err)
			return "", fmt.Errorf("add to calendar: %w", err)
		}
		return fmt.Sprintf("Added to your Google Calendar: *%s* — %s", title, local.Format("Mon, Jan 2 at 3:04 PM")), nil
	}

	// Fallback: a one-time reminder.
	in := store.ReminderInput{
		Title:      title,
		RepeatMode: "once",
		OnceDate:   local.Format("2006-01-02"),
		Times:      []string{local.Format("15:04")},
		Enabled:    true,
	}
	if _, err := h.store.CreateReminder(ctx, userID, in); err != nil {
		return "", fmt.Errorf("create fallback reminder: %w", err)
	}
	return fmt.Sprintf("You don't have Google Calendar connected, so I saved *%s* as a one-time reminder for %s. Connect Google Calendar in Integrations to put one-time events on your calendar.",
		title, local.Format("Mon, Jan 2 at 3:04 PM")), nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
