package event

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/authctx"
	"github.com/irfanmaulana007/personal-assistant/server/internal/calendar"
	"github.com/irfanmaulana007/personal-assistant/server/internal/intent"
	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

type fakeCalendar struct {
	has     bool
	created []calendar.Event
}

func (f *fakeCalendar) HasCalendar(context.Context, int64) bool { return f.has }
func (f *fakeCalendar) CreateEvent(_ context.Context, _ int64, ev calendar.Event) error {
	f.created = append(f.created, ev)
	return nil
}

func newHandler(t *testing.T, cal Calendar) (*Handler, store.Store, context.Context) {
	t.Helper()
	st, err := store.NewSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
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

func TestEvent_GoesToCalendarWhenConnected(t *testing.T) {
	cal := &fakeCalendar{has: true}
	h, st, ctx := newHandler(t, cal)

	msg, err := h.create(ctx, createReq("Dentist", "tomorrow at 3pm"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(cal.created) != 1 || cal.created[0].Title != "Dentist" {
		t.Fatalf("expected a calendar event, got %+v", cal.created)
	}
	if !strings.Contains(msg, "Google Calendar") {
		t.Errorf("confirmation should mention the calendar: %q", msg)
	}
	// No reminder should be created.
	rs, _ := st.ListReminders(ctx, 1, false)
	if len(rs) != 0 {
		t.Errorf("no reminder should be created when calendar is connected, got %d", len(rs))
	}
}

func TestEvent_FallsBackToReminderWhenNoCalendar(t *testing.T) {
	cal := &fakeCalendar{has: false}
	h, st, ctx := newHandler(t, cal)

	msg, err := h.create(ctx, createReq("Pay rent", "tomorrow at 9am"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(cal.created) != 0 {
		t.Errorf("should not create a calendar event when not connected")
	}
	rs, _ := st.ListReminders(ctx, 1, false)
	if len(rs) != 1 || rs[0].RepeatMode != "once" || rs[0].Title != "Pay rent" {
		t.Fatalf("expected a one-time fallback reminder, got %+v", rs)
	}
	if len(rs[0].Times) != 1 {
		t.Errorf("fallback reminder should carry a time: %+v", rs[0])
	}
	if !strings.Contains(msg, "reminder") {
		t.Errorf("confirmation should explain the fallback: %q", msg)
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
