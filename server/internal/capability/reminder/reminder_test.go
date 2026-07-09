package reminder

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/irfanmaulana007/personal-assistant/server/internal/store"
)

var testTZ = time.FixedZone("WIB", 7*3600) // UTC+7, no DST

func testHandler(fs store.Store) (*Handler, *[]string) {
	h := &Handler{
		store:    fs,
		timezone: testTZ,
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	var sent []string
	h.SetSendFunc(func(_ context.Context, _, text string) error {
		sent = append(sent, text)
		return nil
	})
	return h, &sent
}

// fakeStore embeds the interface so only the methods exercised here need impls;
// any other call panics (nil interface), surfacing unexpected usage.
type fakeStore struct {
	store.Store
	fired []firedCall
}

type firedCall struct {
	id      int64
	at      time.Time
	disable bool
}

func (f *fakeStore) MarkReminderFired(_ context.Context, id int64, at time.Time, disable bool) error {
	f.fired = append(f.fired, firedCall{id, at, disable})
	return nil
}

func at(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, testTZ)
}

func TestEffectiveDOM(t *testing.T) {
	cases := []struct {
		dom  int
		day  time.Time
		want int
	}{
		{15, at(2026, time.March, 10, 0, 0), 15},
		{31, at(2026, time.February, 1, 0, 0), 28}, // 2026 not leap
		{31, at(2024, time.February, 1, 0, 0), 29}, // 2024 leap
		{31, at(2026, time.April, 1, 0, 0), 30},
		{31, at(2026, time.January, 1, 0, 0), 31},
	}
	for _, c := range cases {
		if got := effectiveDOM(c.dom, c.day); got != c.want {
			t.Errorf("effectiveDOM(%d, %s) = %d, want %d", c.dom, c.day.Format("2006-01"), got, c.want)
		}
	}
}

func TestDayQualifies(t *testing.T) {
	tue := at(2026, time.March, 10, 0, 0) // Tuesday
	if !dayQualifies(store.Reminder{RepeatMode: "daily"}, tue) {
		t.Error("daily should always qualify")
	}
	if !dayQualifies(store.Reminder{RepeatMode: "weekly", Weekdays: []int{2}}, tue) {
		t.Error("weekly Tue should qualify on a Tuesday")
	}
	if dayQualifies(store.Reminder{RepeatMode: "weekly", Weekdays: []int{1, 3}}, tue) {
		t.Error("weekly Mon/Wed should not qualify on a Tuesday")
	}
	if !dayQualifies(store.Reminder{RepeatMode: "monthly", DayOfMonth: 10}, tue) {
		t.Error("monthly day 10 should qualify on the 10th")
	}
	// day 31 clamps to Feb 28.
	feb28 := at(2026, time.February, 28, 0, 0)
	if !dayQualifies(store.Reminder{RepeatMode: "monthly", DayOfMonth: 31}, feb28) {
		t.Error("monthly day 31 should qualify on Feb 28 (clamped)")
	}
	if !dayQualifies(store.Reminder{RepeatMode: "once", OnceDate: "2026-03-10"}, tue) {
		t.Error("once 2026-03-10 should qualify on that day")
	}
	if dayQualifies(store.Reminder{RepeatMode: "once", OnceDate: "2026-03-11"}, tue) {
		t.Error("once for a different date should not qualify")
	}
}

func TestMostRecentSlot(t *testing.T) {
	h, _ := testHandler(nil)
	r := store.Reminder{RepeatMode: "daily", Times: []string{"08:00", "20:00"}}

	// 12:00 → most recent is today 08:00.
	got, ok := h.mostRecentSlot(r, at(2026, time.March, 10, 12, 0))
	if !ok || !got.Equal(at(2026, time.March, 10, 8, 0)) {
		t.Fatalf("expected today 08:00, got %v ok=%v", got, ok)
	}
	// 21:00 → most recent is today 20:00.
	got, ok = h.mostRecentSlot(r, at(2026, time.March, 10, 21, 0))
	if !ok || !got.Equal(at(2026, time.March, 10, 20, 0)) {
		t.Fatalf("expected today 20:00, got %v ok=%v", got, ok)
	}
	// 06:00 → most recent is yesterday 20:00 (two-day lookback).
	got, ok = h.mostRecentSlot(r, at(2026, time.March, 10, 6, 0))
	if !ok || !got.Equal(at(2026, time.March, 9, 20, 0)) {
		t.Fatalf("expected yesterday 20:00, got %v ok=%v", got, ok)
	}
}

func TestIsDoneOnce(t *testing.T) {
	h, _ := testHandler(nil)
	r := store.Reminder{RepeatMode: "once", Times: []string{"08:00", "20:00"}}
	if h.isDone(r, at(2026, time.March, 10, 8, 0)) {
		t.Error("once should not be done after the first (08:00) slot")
	}
	if !h.isDone(r, at(2026, time.March, 10, 20, 0)) {
		t.Error("once should be done after the last (20:00) slot")
	}
	// Non-once is never auto-disabled.
	if h.isDone(store.Reminder{RepeatMode: "daily", Times: []string{"08:00"}}, at(2026, time.March, 10, 8, 0)) {
		t.Error("daily should never be done")
	}
}

func TestFireReminder_SendsOnceAndGuards(t *testing.T) {
	fs := &fakeStore{}
	h, sent := testHandler(fs)
	r := store.Reminder{ID: 1, RepeatMode: "daily", Title: "stand up", Times: []string{"08:00"}, Enabled: true}

	// Slot at 08:00, tick at 08:00:30 → fire once.
	h.fireReminder(context.Background(), r, at(2026, time.March, 10, 8, 0).Add(30*time.Second))
	if len(*sent) != 1 {
		t.Fatalf("expected 1 send, got %d", len(*sent))
	}
	if len(fs.fired) != 1 || fs.fired[0].disable {
		t.Fatalf("expected 1 fired (no disable), got %+v", fs.fired)
	}
	firedSlot := fs.fired[0].at

	// Next tick with last_fired_at set → monotonic guard blocks (no double fire).
	r.LastFiredAt = &firedSlot
	h.fireReminder(context.Background(), r, at(2026, time.March, 10, 8, 0).Add(50*time.Second))
	if len(*sent) != 1 {
		t.Fatalf("guard failed: expected still 1 send, got %d", len(*sent))
	}
}

func TestFireReminder_GraceWindowSkips(t *testing.T) {
	fs := &fakeStore{}
	h, sent := testHandler(fs)
	r := store.Reminder{ID: 2, RepeatMode: "daily", Title: "late", Times: []string{"08:00"}, Enabled: true}

	// Tick an hour late (> 15m grace) → no send, but marker advances.
	h.fireReminder(context.Background(), r, at(2026, time.March, 10, 9, 0))
	if len(*sent) != 0 {
		t.Fatalf("stale slot should not send, got %d", len(*sent))
	}
	if len(fs.fired) != 1 {
		t.Fatalf("stale slot should advance the marker, got %+v", fs.fired)
	}
}

func TestFireReminder_OnceDisables(t *testing.T) {
	fs := &fakeStore{}
	h, sent := testHandler(fs)
	r := store.Reminder{ID: 3, RepeatMode: "once", Title: "pay", OnceDate: "2026-03-10", Times: []string{"09:00"}, Enabled: true}

	h.fireReminder(context.Background(), r, at(2026, time.March, 10, 9, 0).Add(20*time.Second))
	if len(*sent) != 1 {
		t.Fatalf("expected 1 send, got %d", len(*sent))
	}
	if len(fs.fired) != 1 || !fs.fired[0].disable {
		t.Fatalf("once should disable after its last slot, got %+v", fs.fired)
	}
}

func TestFireReminder_NothingDueOnNonMatchingDay(t *testing.T) {
	fs := &fakeStore{}
	h, sent := testHandler(fs)
	// 2026-03-10 is Tuesday (2); yesterday Monday (1). A Friday-only weekly
	// reminder qualifies on neither day → nothing due.
	r := store.Reminder{ID: 4, RepeatMode: "weekly", Weekdays: []int{5}, Times: []string{"08:00"}, Enabled: true}

	h.fireReminder(context.Background(), r, at(2026, time.March, 10, 12, 0))
	if len(*sent) != 0 || len(fs.fired) != 0 {
		t.Fatalf("nothing should fire on a non-matching weekday; sent=%d fired=%d", len(*sent), len(fs.fired))
	}
}
