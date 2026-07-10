package store

import (
	"context"
	"testing"
)

func TestLifeGoalCRUDRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const uid = 1

	created, err := s.CreateLifeGoal(ctx, uid, "Take a swimming course", "beginner class")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 || created.Title != "Take a swimming course" || created.Done {
		t.Fatalf("unexpected created goal: %+v", created)
	}

	got, err := s.GetLifeGoal(ctx, uid, created.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v (nil=%v)", err, got == nil)
	}
	if got.Note != "beginner class" || got.DoneAt != nil {
		t.Errorf("round-trip failed: %+v", got)
	}

	// Update title/note.
	if err := s.UpdateLifeGoal(ctx, uid, created.ID, "Gym membership", "near the office"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = s.GetLifeGoal(ctx, uid, created.ID)
	if got.Title != "Gym membership" || got.Note != "near the office" {
		t.Errorf("update round-trip failed: %+v", got)
	}

	// Check it off — done_at should be stamped.
	if err := s.SetLifeGoalDone(ctx, uid, created.ID, true); err != nil {
		t.Fatalf("set done: %v", err)
	}
	got, _ = s.GetLifeGoal(ctx, uid, created.ID)
	if !got.Done || got.DoneAt == nil {
		t.Errorf("expected done with a timestamp: %+v", got)
	}

	// Uncheck — done_at should clear.
	if err := s.SetLifeGoalDone(ctx, uid, created.ID, false); err != nil {
		t.Fatalf("unset done: %v", err)
	}
	got, _ = s.GetLifeGoal(ctx, uid, created.ID)
	if got.Done || got.DoneAt != nil {
		t.Errorf("expected not done with no timestamp: %+v", got)
	}

	// Delete.
	if err := s.DeleteLifeGoal(ctx, uid, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ = s.GetLifeGoal(ctx, uid, created.ID)
	if got != nil {
		t.Errorf("expected goal to be gone, got %+v", got)
	}
}

func TestLifeGoalScopedToUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	mine, err := s.CreateLifeGoal(ctx, 1, "Visit Japan", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Another user must not see or mutate it.
	if got, _ := s.GetLifeGoal(ctx, 2, mine.ID); got != nil {
		t.Errorf("user 2 should not read user 1's goal: %+v", got)
	}
	list, _ := s.ListLifeGoals(ctx, 2)
	if len(list) != 0 {
		t.Errorf("user 2 should have no goals, got %d", len(list))
	}
	// A cross-user delete is a no-op; the goal survives for its owner.
	if err := s.DeleteLifeGoal(ctx, 2, mine.ID); err != nil {
		t.Fatalf("cross-user delete: %v", err)
	}
	if got, _ := s.GetLifeGoal(ctx, 1, mine.ID); got == nil {
		t.Error("owner's goal should survive a cross-user delete")
	}
}

func TestListLifeGoalsOrdersUnfinishedFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const uid = 1

	a, _ := s.CreateLifeGoal(ctx, uid, "First", "")
	_, _ = s.CreateLifeGoal(ctx, uid, "Second", "")
	// Check off the first-created one; it should sort after the pending one.
	if err := s.SetLifeGoalDone(ctx, uid, a.ID, true); err != nil {
		t.Fatalf("set done: %v", err)
	}
	list, err := s.ListLifeGoals(ctx, uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 goals, got %d", len(list))
	}
	if list[0].Done || !list[1].Done {
		t.Errorf("unfinished goals should come first: %+v", list)
	}
}
