package reminder

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
	"github.com/irfanmaulana007/personal-assistant/server/internal/settings"
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

func TestFireSpecific_FiresEachOffsetThenDisables(t *testing.T) {
	fs := &fakeStore{}
	h, sent := testHandler(fs)
	// Event at 2026-03-10 18:00 local; remind 1 day (1440m) and 1 hour (60m) before.
	r := store.Reminder{
		ID: 1, RepeatMode: "specific", Title: "Flight",
		EventAt: "2026-03-10T18:00", Offsets: []int{60, 1440}, Enabled: true,
	}

	// First point: event − 1 day = 2026-03-09 18:00. Tick just after it.
	h.fireSpecific(context.Background(), r, at(2026, time.March, 9, 18, 0).Add(20*time.Second))
	if len(*sent) != 1 {
		t.Fatalf("expected 1 send at the 1-day point, got %d", len(*sent))
	}
	if len(fs.fired) != 1 || fs.fired[0].disable {
		t.Fatalf("1-day point should not disable, got %+v", fs.fired)
	}
	p1 := fs.fired[0].at
	if !p1.Equal(at(2026, time.March, 9, 18, 0)) {
		t.Fatalf("first point should be event-1day, got %v", p1)
	}

	// Advance last_fired_at; next point: event − 1 hour = 2026-03-10 17:00 → disables.
	r.LastFiredAt = &p1
	h.fireSpecific(context.Background(), r, at(2026, time.March, 10, 17, 0).Add(20*time.Second))
	if len(*sent) != 2 {
		t.Fatalf("expected 2 sends after the 1-hour point, got %d", len(*sent))
	}
	if !fs.fired[1].disable {
		t.Fatalf("final (1-hour) point should disable the reminder, got %+v", fs.fired[1])
	}
}

func TestFireSpecific_GuardBlocksRefire(t *testing.T) {
	fs := &fakeStore{}
	h, sent := testHandler(fs)
	p := at(2026, time.March, 9, 18, 0)
	r := store.Reminder{
		ID: 2, RepeatMode: "specific", Title: "x",
		EventAt: "2026-03-10T18:00", Offsets: []int{1440}, Enabled: true, LastFiredAt: &p,
	}
	// The only point (event−1day = 03-09 18:00) equals last_fired_at → no refire.
	h.fireSpecific(context.Background(), r, at(2026, time.March, 9, 18, 0).Add(40*time.Second))
	if len(*sent) != 0 {
		t.Fatalf("guard should block refire of an already-fired point, got %d", len(*sent))
	}
}

func TestBuildDigest(t *testing.T) {
	now := at(2026, time.March, 10, 7, 0) // Tuesday 07:00
	reminders := []store.Reminder{
		// Daily: 06:00 already past today (excluded), 20:00 today + tomorrow (included).
		{RepeatMode: "daily", Title: "Stretch", Times: []string{"06:00", "20:00"}, Enabled: true},
		// Weekly Wed (tomorrow) 09:00 → included; Tue is today but already past isn't set here.
		{RepeatMode: "weekly", Title: "Standup", Weekdays: []int{3}, Times: []string{"09:00"}, Enabled: true},
		// Specific event tomorrow 18:00 → included.
		{RepeatMode: "specific", Title: "Flight", EventAt: "2026-03-11T18:00", Offsets: []int{60}, Enabled: true},
		// Specific event far away → excluded.
		{RepeatMode: "specific", Title: "Faraway", EventAt: "2026-04-01T10:00", Offsets: []int{60}, Enabled: true},
	}
	msg := buildDigest(reminders, nil, now, testTZ)
	if msg == "" {
		t.Fatal("expected a non-empty digest")
	}
	for _, want := range []string{"Stretch", "Standup", "Flight", "Today 8:00 PM", "Tomorrow"} {
		if !strings.Contains(msg, want) {
			t.Errorf("digest missing %q; got:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "Faraway") {
		t.Errorf("digest should not include out-of-window reminder; got:\n%s", msg)
	}
	// Stretch's today 06:00 is before now (07:00) → the today variant is excluded
	// (tomorrow's 06:00 still legitimately appears).
	if strings.Contains(msg, "Today 6:00 AM") {
		t.Errorf("digest should exclude a past same-day slot; got:\n%s", msg)
	}
}

func TestBuildDigest_MergesCalendarEvents(t *testing.T) {
	now := at(2026, time.March, 10, 7, 0)
	cal := []calendar.Event{
		{Title: "Client call", Start: at(2026, time.March, 10, 14, 0)}, // today, in window
		{Title: "Far meeting", Start: at(2026, time.March, 20, 9, 0)},  // out of window
	}
	reminders := []store.Reminder{
		{RepeatMode: "daily", Title: "Stretch", Times: []string{"20:00"}, Enabled: true},
	}
	msg := buildDigest(reminders, cal, now, testTZ)
	if !strings.Contains(msg, "Client call") {
		t.Errorf("digest should include the in-window calendar event; got:\n%s", msg)
	}
	if !strings.Contains(msg, "Stretch") {
		t.Errorf("digest should still include reminders; got:\n%s", msg)
	}
	if strings.Contains(msg, "Far meeting") {
		t.Errorf("digest should exclude out-of-window calendar events; got:\n%s", msg)
	}
}

func TestBuildDigest_EmptyWhenNothingUpcoming(t *testing.T) {
	now := at(2026, time.March, 10, 7, 0)
	reminders := []store.Reminder{
		{RepeatMode: "weekly", Title: "Sat only", Weekdays: []int{6}, Times: []string{"09:00"}, Enabled: true},
	}
	if got := buildDigest(reminders, nil, now, testTZ); got != "" {
		t.Errorf("expected empty digest, got: %q", got)
	}
}

func TestScheduleCreatesVisibleRecurring(t *testing.T) {
	st, err := store.NewSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	h := &Handler{store: st, timezone: testTZ, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx := authctx.WithUserID(context.Background(), 1)

	msg, err := h.schedule(ctx, &intent.ParseResult{
		Action: intent.ActionReminderSchedule,
		Entities: map[string]string{
			"title": "Pay internet", "repeat": "monthly", "day_of_month": "5", "times": "09:00",
		},
	})
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if !strings.Contains(msg, "Monthly on day 5") {
		t.Errorf("unexpected confirmation: %q", msg)
	}

	// It must be a real, visible reminder (has times → not hidden by the API list).
	rs, err := st.ListReminders(ctx, 1, true)
	if err != nil || len(rs) != 1 {
		t.Fatalf("expected 1 reminder, got %d (err=%v)", len(rs), err)
	}
	r := rs[0]
	if r.RepeatMode != "monthly" || r.DayOfMonth != 5 || len(r.Times) != 1 || r.Times[0] != "09:00" {
		t.Errorf("unexpected reminder stored: %+v", r)
	}
}

func TestScheduleUsesDefaultTimeWhenOmitted(t *testing.T) {
	st, err := store.NewSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	svc := settings.New(st, nil)
	ctx := authctx.WithUserID(context.Background(), 1)
	if err := svc.SetReminderDefaultTime(ctx, "22:00"); err != nil {
		t.Fatalf("set default time: %v", err)
	}
	h := &Handler{store: st, settings: svc, timezone: testTZ, log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	// No "times" provided → the configured default (22:00) is applied.
	if _, err := h.schedule(ctx, &intent.ParseResult{
		Action:   intent.ActionReminderSchedule,
		Entities: map[string]string{"title": "Pay internet", "repeat": "monthly", "day_of_month": "5"},
	}); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	rs, _ := st.ListReminders(ctx, 1, true)
	if len(rs) != 1 || len(rs[0].Times) != 1 || rs[0].Times[0] != "22:00" {
		t.Fatalf("expected default time 22:00, got %+v", rs)
	}
}

func TestParseTimesCSV(t *testing.T) {
	got := parseTimesCSV("9:00, 8:5 ,08:05,25:00,bad")
	// zero-padded, deduped, sorted; invalid dropped.
	if len(got) != 2 || got[0] != "08:05" || got[1] != "09:00" {
		t.Fatalf("parseTimesCSV = %v", got)
	}
	if len(parseTimesCSV("")) != 0 {
		t.Error("empty should yield no times")
	}
}

func TestParseWeekdaysCSV(t *testing.T) {
	got := parseWeekdaysCSV("Mon, wednesday, jumat, 0, 0, nope")
	// Mon=1, Wed=3, Fri(jumat)=5, Sun=0; deduped + sorted.
	want := []int{0, 1, 3, 5}
	if len(got) != len(want) {
		t.Fatalf("parseWeekdaysCSV = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseWeekdaysCSV = %v, want %v", got, want)
		}
	}
}

func TestDescribeSchedule(t *testing.T) {
	cases := []struct {
		r    store.Reminder
		want string
	}{
		{store.Reminder{RepeatMode: "daily", Times: []string{"08:00", "20:00"}}, "Every day at 08:00, 20:00"},
		{store.Reminder{RepeatMode: "weekly", Weekdays: []int{1, 3}, Times: []string{"09:00"}}, "Weekly on Mon, Wed at 09:00"},
		{store.Reminder{RepeatMode: "monthly", DayOfMonth: 5, Times: []string{"07:00"}}, "Monthly on day 5 at 07:00"},
		{store.Reminder{RepeatMode: "once", OnceDate: "2026-03-10", Times: []string{"15:30"}}, "Once on Tue, Mar 10 at 15:30"},
		{store.Reminder{RepeatMode: "specific", EventAt: "2026-09-01T06:00", Offsets: []int{60}}, "Event on Tue, Sep 1 at 6:00 AM"},
		// Any reminder carrying an event time shows the event time (15:00), not
		// its earlier notification time (14:00).
		{store.Reminder{RepeatMode: "once", OnceDate: "2026-07-11", Times: []string{"14:00"}, EventAt: "2026-07-11T15:00"}, "Event on Sat, Jul 11 at 3:00 PM"},
	}
	for _, c := range cases {
		if got := describeSchedule(c.r, testTZ); got != c.want {
			t.Errorf("describeSchedule = %q, want %q", got, c.want)
		}
	}
}

func TestHumanizeLead(t *testing.T) {
	cases := map[time.Duration]string{
		24 * time.Hour:   "1 day",
		48 * time.Hour:   "2 days",
		time.Hour:        "1 hour",
		2 * time.Hour:    "2 hours",
		30 * time.Minute: "30 minutes",
		90 * time.Minute: "90 minutes",
	}
	for d, want := range cases {
		if got := humanizeLead(d); got != want {
			t.Errorf("humanizeLead(%s) = %q, want %q", d, got, want)
		}
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
