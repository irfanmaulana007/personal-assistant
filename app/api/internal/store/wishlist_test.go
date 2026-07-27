//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

func TestWishItemCRUDRoundTrip(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()
	const uid = 1

	created, err := s.CreateWishItem(ctx, uid, "Rain Shower Head", 350000, "2026-09", PriorityHigh, "https://tokopedia.com/x", "white, 60cm")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 || created.Name != "Rain Shower Head" || created.Done {
		t.Fatalf("unexpected created item: %+v", created)
	}
	if created.EstimatedPrice != 350000 || created.BuyMonth != "2026-09" || created.Priority != PriorityHigh {
		t.Fatalf("unexpected created fields: %+v", created)
	}

	got, err := s.GetWishItem(ctx, uid, created.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v (nil=%v)", err, got == nil)
	}
	if got.Link != "https://tokopedia.com/x" || got.Note != "white, 60cm" || got.DoneAt != nil {
		t.Errorf("round-trip failed: %+v", got)
	}

	// Update every field.
	if err := s.UpdateWishItem(ctx, uid, created.ID, "Pegboard", 200000, "2026-10", PriorityLow, "https://ref", "for tools"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = s.GetWishItem(ctx, uid, created.ID)
	if got.Name != "Pegboard" || got.EstimatedPrice != 200000 || got.BuyMonth != "2026-10" || got.Priority != PriorityLow || got.Link != "https://ref" || got.Note != "for tools" {
		t.Errorf("update round-trip failed: %+v", got)
	}

	// Mark bought — done_at should be stamped.
	if err := s.SetWishItemDone(ctx, uid, created.ID, true, nil); err != nil {
		t.Fatalf("set done: %v", err)
	}
	got, _ = s.GetWishItem(ctx, uid, created.ID)
	if !got.Done || got.DoneAt == nil {
		t.Errorf("expected done with a timestamp: %+v", got)
	}

	// Re-check with an explicit purchase date.
	when := time.Date(2025, 3, 14, 12, 0, 0, 0, time.UTC)
	if err := s.SetWishItemDone(ctx, uid, created.ID, true, &when); err != nil {
		t.Fatalf("set done with date: %v", err)
	}
	got, _ = s.GetWishItem(ctx, uid, created.ID)
	if got.DoneAt == nil || !got.DoneAt.Equal(when) {
		t.Errorf("expected done_at %v, got %+v", when, got.DoneAt)
	}

	// Reopen — done_at should clear.
	if err := s.SetWishItemDone(ctx, uid, created.ID, false, nil); err != nil {
		t.Fatalf("unset done: %v", err)
	}
	got, _ = s.GetWishItem(ctx, uid, created.ID)
	if got.Done || got.DoneAt != nil {
		t.Errorf("expected not done with no timestamp: %+v", got)
	}

	// Delete.
	if err := s.DeleteWishItem(ctx, uid, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ = s.GetWishItem(ctx, uid, created.ID)
	if got != nil {
		t.Errorf("expected item to be gone, got %+v", got)
	}
}

func TestWishItemUnknownPriorityFallsBackToMedium(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	w, err := s.CreateWishItem(ctx, 1, "Standing Desk", 0, "", "urgent", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.Priority != PriorityMedium {
		t.Errorf("unknown priority should fall back to %q, got %q", PriorityMedium, w.Priority)
	}
}

func TestWishItemScopedToUser(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	mine, err := s.CreateWishItem(ctx, 1, "Monitor", 1500000, "2026-08", PriorityHigh, "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Another user must not see or mutate it.
	if got, _ := s.GetWishItem(ctx, 2, mine.ID); got != nil {
		t.Errorf("user 2 should not read user 1's item: %+v", got)
	}
	list, _ := s.ListWishItems(ctx, 2)
	if len(list) != 0 {
		t.Errorf("user 2 should have no items, got %d", len(list))
	}
	// A cross-user delete is a no-op; the item survives for its owner.
	if err := s.DeleteWishItem(ctx, 2, mine.ID); err != nil {
		t.Fatalf("cross-user delete: %v", err)
	}
	if got, _ := s.GetWishItem(ctx, 1, mine.ID); got == nil {
		t.Error("owner's item should survive a cross-user delete")
	}
}

func TestListWishItemsOrdering(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()
	const uid = 1

	// A dated pending item, an undated pending item, and a bought one.
	dated, _ := s.CreateWishItem(ctx, uid, "September buy", 100000, "2026-09", PriorityMedium, "", "")
	_, _ = s.CreateWishItem(ctx, uid, "No month yet", 100000, "", PriorityMedium, "", "")
	bought, _ := s.CreateWishItem(ctx, uid, "Already bought", 100000, "2026-08", PriorityMedium, "", "")
	if err := s.SetWishItemDone(ctx, uid, bought.ID, true, nil); err != nil {
		t.Fatalf("set done: %v", err)
	}

	list, err := s.ListWishItems(ctx, uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 items, got %d", len(list))
	}
	// Unfinished first; among unfinished, dated before undated; bought last.
	if list[0].ID != dated.ID {
		t.Errorf("expected dated pending item first, got %+v", list[0])
	}
	if list[0].Done || list[1].Done {
		t.Errorf("bought item should sort last: %+v", list)
	}
	if list[1].BuyMonth != "" {
		t.Errorf("undated pending item should come after dated one: %+v", list)
	}
	if !list[2].Done {
		t.Errorf("last item should be the bought one: %+v", list)
	}
}
