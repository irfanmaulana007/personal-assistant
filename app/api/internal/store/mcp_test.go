//go:build integration

package store

import (
	"context"
	"testing"
)

func TestNotionTargetsCRUDRoundTrip(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	// Map a task and an issue database.
	task, err := s.SetNotionTarget(ctx, "task", "db-task", "Tasks", "https://notion.so/tasks")
	if err != nil {
		t.Fatalf("set task target: %v", err)
	}
	if task.ID == 0 || task.Kind != "task" || task.DatabaseID != "db-task" {
		t.Fatalf("unexpected task target: %+v", task)
	}
	if _, err := s.SetNotionTarget(ctx, "issue", "db-issue", "Issues", ""); err != nil {
		t.Fatalf("set issue target: %v", err)
	}

	list, err := s.ListNotionTargets(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list targets = %+v (err %v), want 2", list, err)
	}

	// Re-mapping the same kind upserts (new database id), not duplicates.
	task2, err := s.SetNotionTarget(ctx, "task", "db-task-2", "Tasks v2", "")
	if err != nil {
		t.Fatalf("re-map task: %v", err)
	}
	if task2.ID != task.ID || task2.DatabaseID != "db-task-2" {
		t.Fatalf("expected upsert to same row with new db id: %+v", task2)
	}
	if list, _ := s.ListNotionTargets(ctx); len(list) != 2 {
		t.Fatalf("after upsert, targets = %d, want 2", len(list))
	}

	// Delete one mapping.
	if err := s.DeleteNotionTarget(ctx, "issue"); err != nil {
		t.Fatalf("delete issue target: %v", err)
	}
	if list, _ := s.ListNotionTargets(ctx); len(list) != 1 {
		t.Fatalf("after delete, targets = %d, want 1", len(list))
	}
}
