package event

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/irfanmaulana007/personal-assistant/app/api/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/calendar"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store"
	"github.com/irfanmaulana007/personal-assistant/app/api/internal/store/storetest"
)

type fakeCalendar struct {
	has     bool
	created []calendar.Event
	events  []calendar.Event
	deleted []string // event ids passed to DeleteEventIn
}

func (f *fakeCalendar) HasCalendar(context.Context, int64) bool { return f.has }
func (f *fakeCalendar) CreateEvent(_ context.Context, _ int64, ev calendar.Event) error {
	f.created = append(f.created, ev)
	return nil
}
func (f *fakeCalendar) ListEvents(context.Context, int64, time.Time, time.Time) []calendar.Event {
	return f.events
}
func (f *fakeCalendar) DeleteEventIn(_ context.Context, _ int64, _, _, eventID string) error {
	f.deleted = append(f.deleted, eventID)
	return nil
}

func newHandler(t *testing.T, cal Calendar) (*Handler, store.Store, context.Context) {
	t.Helper()
	st := storetest.New(t)
	tz := time.FixedZone("WIB", 7*3600)
	h := New(cal, st, tz, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return h, st, authctx.WithUserID(context.Background(), 1)
}

func createReq(title, datetime string) *intent.ParseResult {
	return &intent.ParseResult{
		Capability: intent.CapabilityEvent,
		Action:     intent.ActionEventCreate,
		Entities:   map[string]string{"title": title, "datetime": datetime},
	}
}

func TestEvent_AlwaysStoresOnceReminder(t *testing.T) {
	// A one-time event is a one-time reminder in the DB regardless of calendar
	// connection (the calendar mirror is handled by the reminder layer).
	for _, connected := range []bool{true, false} {
		h, st, ctx := newHandler(t, &fakeCalendar{has: connected})
		if _, err := h.create(ctx, createReq("Pay rent", "tomorrow at 9am")); err != nil {
			t.Fatalf("create (connected=%v): %v", connected, err)
		}
		rs, _ := st.ListReminders(ctx, 1, false)
		if len(rs) != 1 || rs[0].RepeatMode != "once" || rs[0].Title != "Pay rent" || len(rs[0].Times) != 1 {
			t.Fatalf("connected=%v: expected a one-time reminder, got %+v", connected, rs)
		}
	}
}

func TestEvent_AsksWhenMissingParts(t *testing.T) {
	h, _, ctx := newHandler(t, &fakeCalendar{})
	if msg, _ := h.create(ctx, createReq("", "tomorrow")); !strings.Contains(msg, "event") {
		t.Errorf("should ask for the event, got %q", msg)
	}
	if msg, _ := h.create(ctx, createReq("Lunch", "")); !strings.Contains(strings.ToLower(msg), "when") {
		t.Errorf("should ask when, got %q", msg)
	}
}

var testTZ = time.FixedZone("WIB", 7*3600)

func deleteReq(title, datetime string) *intent.ParseResult {
	ent := map[string]string{"title": title}
	if datetime != "" {
		ent["datetime"] = datetime
	}
	return &intent.ParseResult{Capability: intent.CapabilityEvent, Action: intent.ActionEventDelete, Entities: ent}
}

func TestEvent_DeleteSingle(t *testing.T) {
	cal := &fakeCalendar{has: true, events: []calendar.Event{
		{ID: "e1", Title: "Team Sync", Start: time.Date(2026, 8, 5, 14, 0, 0, 0, testTZ), Account: "c1", Calendar: "primary"},
	}}
	h, _, ctx := newHandler(t, cal)
	msg, err := h.delete(ctx, deleteReq("Team Sync", ""))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(cal.deleted) != 1 || cal.deleted[0] != "e1" {
		t.Fatalf("expected e1 deleted, got %v", cal.deleted)
	}
	if !strings.Contains(msg, "Deleted") {
		t.Errorf("unexpected msg %q", msg)
	}
}

func TestEvent_DeleteClearsDuplicatesAtSameTime(t *testing.T) {
	at := time.Date(2026, 7, 13, 18, 0, 0, 0, testTZ)
	cal := &fakeCalendar{has: true, events: []calendar.Event{
		{ID: "d1", Title: "Unregister Old Number", Start: at, Account: "c1", Calendar: "primary"},
		{ID: "d2", Title: "Unregister Old Number", Start: at, Account: "c1", Calendar: "primary"},
	}}
	h, _, ctx := newHandler(t, cal)
	msg, _ := h.delete(ctx, deleteReq("Unregister Old Number", ""))
	if len(cal.deleted) != 2 {
		t.Fatalf("expected both duplicates deleted, got %v", cal.deleted)
	}
	if !strings.Contains(msg, "2 duplicate") {
		t.Errorf("unexpected msg %q", msg)
	}
}

func TestEvent_DeleteAmbiguousDifferentTimesAsks(t *testing.T) {
	cal := &fakeCalendar{has: true, events: []calendar.Event{
		{ID: "a1", Title: "Standup", Start: time.Date(2026, 8, 5, 9, 0, 0, 0, testTZ), Account: "c1", Calendar: "primary"},
		{ID: "a2", Title: "Standup", Start: time.Date(2026, 8, 6, 9, 0, 0, 0, testTZ), Account: "c1", Calendar: "primary"},
	}}
	h, _, ctx := newHandler(t, cal)
	msg, _ := h.delete(ctx, deleteReq("Standup", ""))
	if len(cal.deleted) != 0 {
		t.Fatalf("should not delete anything when ambiguous, deleted %v", cal.deleted)
	}
	if !strings.Contains(strings.ToLower(msg), "which one") {
		t.Errorf("expected a disambiguation prompt, got %q", msg)
	}
}

func TestEvent_DeleteAmbiguousResolvedByDatetime(t *testing.T) {
	cal := &fakeCalendar{has: true, events: []calendar.Event{
		{ID: "a1", Title: "Standup", Start: time.Date(2026, 8, 5, 9, 0, 0, 0, testTZ), Account: "c1", Calendar: "primary"},
		{ID: "a2", Title: "Standup", Start: time.Date(2026, 8, 6, 9, 0, 0, 0, testTZ), Account: "c1", Calendar: "primary"},
	}}
	h, _, ctx := newHandler(t, cal)
	if _, err := h.delete(ctx, deleteReq("Standup", "2026-08-06 09:00")); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(cal.deleted) != 1 || cal.deleted[0] != "a2" {
		t.Fatalf("expected only a2 deleted, got %v", cal.deleted)
	}
}

func TestEvent_DeleteNoMatch(t *testing.T) {
	cal := &fakeCalendar{has: true, events: []calendar.Event{
		{ID: "e1", Title: "Team Sync", Start: time.Date(2026, 8, 5, 14, 0, 0, 0, testTZ), Account: "c1", Calendar: "primary"},
	}}
	h, _, ctx := newHandler(t, cal)
	msg, _ := h.delete(ctx, deleteReq("Nonexistent", ""))
	if len(cal.deleted) != 0 {
		t.Fatalf("nothing should be deleted, got %v", cal.deleted)
	}
	if !strings.Contains(strings.ToLower(msg), "couldn't find") {
		t.Errorf("unexpected msg %q", msg)
	}
}

func TestEvent_DeleteNoCalendar(t *testing.T) {
	h, _, ctx := newHandler(t, &fakeCalendar{has: false})
	msg, _ := h.delete(ctx, deleteReq("Team Sync", ""))
	if !strings.Contains(msg, "No Google Calendar") {
		t.Errorf("unexpected msg %q", msg)
	}
}
