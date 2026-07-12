package store

import (
	"context"
	"testing"
)

func TestBucketItemCRUDRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const uid = 1

	created, err := s.CreateBucketItem(ctx, uid, "Swimming Lessons", "Learn to swim freestyle confidently.", "beginner class", CategorySelfImprovement, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 || created.Title != "Swimming Lessons" || created.Done {
		t.Fatalf("unexpected created item: %+v", created)
	}
	if created.Category != CategorySelfImprovement {
		t.Errorf("expected category %q, got %q", CategorySelfImprovement, created.Category)
	}

	got, err := s.GetBucketItem(ctx, uid, created.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v (nil=%v)", err, got == nil)
	}
	if got.Description != "Learn to swim freestyle confidently." || got.Note != "beginner class" || got.DoneAt != nil {
		t.Errorf("round-trip failed: %+v", got)
	}
	if got.ResolutionYear != nil {
		t.Errorf("expected no resolution year, got %v", *got.ResolutionYear)
	}

	// Update title/description/note/category.
	if err := s.UpdateBucketItem(ctx, uid, created.ID, "Gym Membership", "Build a consistent workout habit.", "near the office", CategoryOther); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = s.GetBucketItem(ctx, uid, created.ID)
	if got.Title != "Gym Membership" || got.Description != "Build a consistent workout habit." || got.Note != "near the office" || got.Category != CategoryOther {
		t.Errorf("update round-trip failed: %+v", got)
	}

	// Flag as a resolution, then clear it.
	year := 2026
	if err := s.SetBucketItemResolution(ctx, uid, created.ID, &year); err != nil {
		t.Fatalf("set resolution: %v", err)
	}
	got, _ = s.GetBucketItem(ctx, uid, created.ID)
	if got.ResolutionYear == nil || *got.ResolutionYear != 2026 {
		t.Errorf("expected resolution year 2026: %+v", got)
	}
	if err := s.SetBucketItemResolution(ctx, uid, created.ID, nil); err != nil {
		t.Fatalf("clear resolution: %v", err)
	}
	got, _ = s.GetBucketItem(ctx, uid, created.ID)
	if got.ResolutionYear != nil {
		t.Errorf("expected resolution cleared: %+v", got)
	}

	// Check it off — done_at should be stamped.
	if err := s.SetBucketItemDone(ctx, uid, created.ID, true); err != nil {
		t.Fatalf("set done: %v", err)
	}
	got, _ = s.GetBucketItem(ctx, uid, created.ID)
	if !got.Done || got.DoneAt == nil {
		t.Errorf("expected done with a timestamp: %+v", got)
	}

	// Uncheck — done_at should clear.
	if err := s.SetBucketItemDone(ctx, uid, created.ID, false); err != nil {
		t.Fatalf("unset done: %v", err)
	}
	got, _ = s.GetBucketItem(ctx, uid, created.ID)
	if got.Done || got.DoneAt != nil {
		t.Errorf("expected not done with no timestamp: %+v", got)
	}

	// Delete.
	if err := s.DeleteBucketItem(ctx, uid, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ = s.GetBucketItem(ctx, uid, created.ID)
	if got != nil {
		t.Errorf("expected item to be gone, got %+v", got)
	}
}

func TestBucketItemUnknownCategoryFallsBackToOther(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	g, err := s.CreateBucketItem(ctx, 1, "Visit Japan", "", "", "made_up_category", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if g.Category != CategoryOther {
		t.Errorf("unknown category should fall back to %q, got %q", CategoryOther, g.Category)
	}
}

func TestBucketItemScopedToUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	mine, err := s.CreateBucketItem(ctx, 1, "Visit Japan", "", "", CategoryCountry, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Another user must not see or mutate it.
	if got, _ := s.GetBucketItem(ctx, 2, mine.ID); got != nil {
		t.Errorf("user 2 should not read user 1's item: %+v", got)
	}
	list, _ := s.ListBucketItems(ctx, 2)
	if len(list) != 0 {
		t.Errorf("user 2 should have no items, got %d", len(list))
	}
	// A cross-user delete is a no-op; the item survives for its owner.
	if err := s.DeleteBucketItem(ctx, 2, mine.ID); err != nil {
		t.Fatalf("cross-user delete: %v", err)
	}
	if got, _ := s.GetBucketItem(ctx, 1, mine.ID); got == nil {
		t.Error("owner's item should survive a cross-user delete")
	}
}

func TestListBucketItemsOrdersUnfinishedFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const uid = 1

	a, _ := s.CreateBucketItem(ctx, uid, "First", "", "", CategoryOther, nil)
	_, _ = s.CreateBucketItem(ctx, uid, "Second", "", "", CategoryOther, nil)
	// Check off the first-created one; it should sort after the pending one.
	if err := s.SetBucketItemDone(ctx, uid, a.ID, true); err != nil {
		t.Fatalf("set done: %v", err)
	}
	list, err := s.ListBucketItems(ctx, uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
	if list[0].Done || !list[1].Done {
		t.Errorf("unfinished items should come first: %+v", list)
	}
}
