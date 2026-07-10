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
func (f *fakeCalendar) ListEvents(context.Context, int64, time.Time, time.Time) []calendar.Event {
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
