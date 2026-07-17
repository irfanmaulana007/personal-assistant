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
// interface so the agenda logic is unit-testable without Composio).
type Calendar interface {
	ListEvents(ctx context.Context, userID int64, from, to time.Time) []calendar.Event
	HasCalendar(ctx context.Context, userID int64) bool
	DeleteEventIn(ctx context.Context, userID int64, connID, calID, eventID string) error
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
	case intent.ActionEventDelete:
		return h.delete(ctx, result)
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
	if d, err := strconv.Atoi(result.Entities["days"]); err == nil && d > 0 && d <= 366 {
		days = d
	}
	now := time.Now().In(h.timezone)
	from := now
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.timezone).AddDate(0, 0, days)

	events := h.calendar.ListEvents(ctx, userID, from, to)
	if len(events) == 0 {
		// Distinguish "no calendar connected" from "connected but empty window"
		// so we never tell the user their calendar is disconnected when it isn't.
		if h.calendar.HasCalendar(ctx, userID) {
			return fmt.Sprintf("Your Google Calendar is connected, but I found no events in the next %d days.", days), nil
		}
		return "No Google Calendar is connected.", nil
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

	// A one-time event is stored as a one-time reminder (the source of truth). If
	// a Google Calendar is connected it's mirrored there automatically by the
	// reminder layer; the daily recap covers today & tomorrow.
	userID := authctx.UserID(ctx)
	in := store.ReminderInput{
		Title:      title,
		RepeatMode: "once",
		OnceDate:   local.Format("2006-01-02"),
		Times:      []string{local.Format("15:04")},
		Enabled:    true,
	}
	reminder, err := h.store.CreateReminder(ctx, userID, in)
	if err != nil {
		return "", fmt.Errorf("create reminder: %w", err)
	}

	// Read-after-write: confirm the reminder actually persisted before telling
	// the user the event was saved.
	got, err := h.store.GetReminder(ctx, userID, reminder.ID)
	if err != nil {
		return "", fmt.Errorf("verify reminder saved: %w", err)
	}
	if got == nil {
		return "", fmt.Errorf("verify reminder saved: reminder not found after create")
	}
	return fmt.Sprintf("Reminder set: *%s* — %s", title, local.Format("Mon, Jan 2 at 3:04 PM")), nil
}

// delete removes a Google Calendar event by title, optionally narrowed to a
// specific date/time. It resolves the title against the user's upcoming events
// (the model only ever sees titles + times from list_calendar, never stable
// event ids) and deletes the matching instance(s):
//
//   - no match            → says so, deletes nothing
//   - one match           → deletes it
//   - many at one time     → deletes them all (true duplicates)
//   - many at diff. times  → lists them and asks which date/time, deletes nothing
//
// A datetime narrows the candidates to that exact instant, which is how the
// model clears a specific duplicate off the calendar.
func (h *Handler) delete(ctx context.Context, result *intent.ParseResult) (string, error) {
	if h.calendar == nil {
		return "No Google Calendar is connected.", nil
	}
	title := strings.TrimSpace(firstNonEmpty(result.Entities["title"], result.Entities["message"]))
	if title == "" {
		return "Which event should I delete? Tell me its title, e.g. _delete Team Sync from my calendar_.", nil
	}
	userID := authctx.UserID(ctx)
	if !h.calendar.HasCalendar(ctx, userID) {
		return "No Google Calendar is connected.", nil
	}

	// Search a wide-ish window so we can find the event regardless of the range
	// the model last listed. Include earlier-today events too.
	now := time.Now().In(h.timezone)
	days := 90
	if d, err := strconv.Atoi(result.Entities["days"]); err == nil && d > 0 && d <= 366 {
		days = d
	}
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.timezone)
	to := from.AddDate(0, 0, days)

	// Optional datetime to pin down a specific instance.
	var when time.Time
	haveWhen := false
	if whenStr := firstNonEmpty(result.Entities["datetime"], result.Entities["time"], result.Entities["date"]); whenStr != "" {
		if t, err := capability.ParseTime(whenStr, h.timezone); err == nil {
			when = t.In(h.timezone)
			haveWhen = true
		}
	}

	// Collect events whose title matches (case-insensitive, trimmed exact).
	wantTitle := strings.ToLower(title)
	var matches []calendar.Event
	for _, ev := range h.calendar.ListEvents(ctx, userID, from, to) {
		if strings.ToLower(strings.TrimSpace(ev.Title)) != wantTitle {
			continue
		}
		if haveWhen && !sameInstant(ev, when) {
			continue
		}
		matches = append(matches, ev)
	}

	if len(matches) == 0 {
		if haveWhen {
			return fmt.Sprintf("I couldn't find %q at %s on your calendar.", title, when.Format("Mon, Jan 2 at 3:04 PM")), nil
		}
		return fmt.Sprintf("I couldn't find an event titled %q on your calendar.", title), nil
	}

	// Several matches at different times and no time given: don't guess — ask.
	if len(matches) > 1 && !haveWhen && !allSameInstant(matches) {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("There are %d events titled %q. Which one? Tell me the date/time:\n", len(matches), title))
		for _, ev := range matches {
			b.WriteString(fmt.Sprintf("\n• %s", h.whenLabel(ev)))
		}
		return b.String(), nil
	}

	deleted := 0
	for _, ev := range matches {
		if ev.ID == "" {
			continue
		}
		if err := h.calendar.DeleteEventIn(ctx, userID, ev.Account, ev.Calendar, ev.ID); err != nil {
			h.log.Warn("delete calendar event", "title", ev.Title, "id", ev.ID, "error", err)
			continue
		}
		deleted++
	}

	if deleted == 0 {
		return fmt.Sprintf("I found %q but couldn't delete it — please try again.", title), nil
	}
	if deleted == 1 {
		return fmt.Sprintf("Deleted *%s* (%s) from your calendar.", title, h.whenLabel(matches[0])), nil
	}
	return fmt.Sprintf("Deleted %d duplicate events titled *%s* from your calendar.", deleted, title), nil
}

// sameInstant reports whether an event starts at the given time — to the minute
// for timed events, to the day for all-day events.
func sameInstant(ev calendar.Event, when time.Time) bool {
	if ev.AllDay {
		y1, m1, d1 := ev.Start.Date()
		y2, m2, d2 := when.Date()
		return y1 == y2 && m1 == m2 && d1 == d2
	}
	return ev.Start.Truncate(time.Minute).Equal(when.Truncate(time.Minute))
}

// allSameInstant reports whether every event in evs starts at the same instant.
func allSameInstant(evs []calendar.Event) bool {
	for _, ev := range evs[1:] {
		if !sameInstant(ev, evs[0].Start) {
			return false
		}
	}
	return true
}

// whenLabel renders an event's start for user-facing confirmations.
func (h *Handler) whenLabel(ev calendar.Event) string {
	if ev.AllDay {
		return ev.Start.In(h.timezone).Format("Mon, Jan 2") + " (all day)"
	}
	return ev.Start.In(h.timezone).Format("Mon, Jan 2 at 3:04 PM")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
