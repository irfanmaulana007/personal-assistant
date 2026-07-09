package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.db.Close() })
	return s
}

func TestReminderCRUDRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const uid = 1

	in := ReminderInput{
		Title:      "Take vitamins",
		RepeatMode: "weekly",
		Times:      []string{"08:00", "20:00"},
		Weekdays:   []int{1, 3, 5},
		Enabled:    true,
	}
	created, err := s.CreateReminder(ctx, uid, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 || created.Title != in.Title {
		t.Fatalf("unexpected created reminder: %+v", created)
	}

	got, err := s.GetReminder(ctx, uid, created.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v (nil=%v)", err, got == nil)
	}
	if len(got.Times) != 2 || got.Times[0] != "08:00" || got.Times[1] != "20:00" {
		t.Errorf("times round-trip failed: %v", got.Times)
	}
	if len(got.Weekdays) != 3 || got.Weekdays[0] != 1 || got.Weekdays[2] != 5 {
		t.Errorf("weekdays round-trip failed: %v", got.Weekdays)
	}
	if !got.Enabled {
		t.Error("expected enabled")
	}

	// Update.
	in.Title = "Take supplements"
	in.RepeatMode = "monthly"
	in.DayOfMonth = 15
	in.Times = []string{"09:00"}
	if err := s.UpdateReminder(ctx, uid, created.ID, in); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = s.GetReminder(ctx, uid, created.ID)
	if got.RepeatMode != "monthly" || got.DayOfMonth != 15 || len(got.Times) != 1 {
		t.Errorf("update round-trip failed: %+v", got)
	}

	// Enable toggle.
	if err := s.SetReminderEnabled(ctx, uid, created.ID, false); err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	got, _ = s.GetReminder(ctx, uid, created.ID)
	if got.Enabled {
		t.Error("expected disabled after SetReminderEnabled(false)")
	}

	// List returns it (activeOnly=false); ListEnabledForOwner excludes disabled.
	all, _ := s.ListReminders(ctx, uid, false)
	if len(all) != 1 {
		t.Errorf("expected 1 reminder in list, got %d", len(all))
	}
	enabled, _ := s.ListEnabledForOwner(ctx, uid)
	if len(enabled) != 0 {
		t.Errorf("disabled reminder should not appear in ListEnabledForOwner, got %d", len(enabled))
	}

	// Delete.
	if err := s.DeleteReminder(ctx, uid, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ = s.GetReminder(ctx, uid, created.ID)
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestMarkReminderFiredDisables(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const uid = 1

	created, err := s.CreateReminder(ctx, uid, ReminderInput{
		Title: "One-off", RepeatMode: "once", OnceDate: "2026-03-10", Times: []string{"09:00"}, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	fired := time.Date(2026, 3, 10, 2, 0, 0, 0, time.UTC)
	if err := s.MarkReminderFired(ctx, created.ID, fired, true); err != nil {
		t.Fatalf("mark fired: %v", err)
	}
	got, _ := s.GetReminder(ctx, uid, created.ID)
	if got.Enabled {
		t.Error("expected disabled after MarkReminderFired(disable=true)")
	}
	if got.LastFiredAt == nil || !got.LastFiredAt.Equal(fired) {
		t.Errorf("last_fired_at not persisted: %v", got.LastFiredAt)
	}
}

func TestLegacyReminderUsesRemindAtBranch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const uid = 1

	past := time.Now().Add(-time.Hour)
	if _, err := s.CreateLegacyReminder(ctx, uid, "call mom", past); err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	due, err := s.GetDueReminders(ctx, uid)
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	if len(due) != 1 || len(due[0].Times) != 0 {
		t.Fatalf("expected 1 legacy due reminder with empty times, got %+v", due)
	}
}
