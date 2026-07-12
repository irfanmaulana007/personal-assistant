package store

import (
	"context"
	"database/sql"
	"testing"
)

// Simulates an existing DB that still has the old life_goals table with data,
// then verifies migrate() renames it in place and adds the new columns.
func TestLegacyLifeGoalsTableMigratesToBucketList(t *testing.T) {
	path := t.TempDir() + "/legacy.db"
	// Open once, then hand-craft the legacy schema and a row.
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE life_goals (
		id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL, title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', done INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')), done_at DATETIME)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO life_goals (user_id, title) VALUES (1, 'Old Goal')`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	// Now open through the real store, which runs migrate().
	s, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	items, err := s.ListBucketItems(context.Background(), 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Old Goal" {
		t.Fatalf("legacy row not carried over: %+v", items)
	}
	if items[0].Category != CategoryOther {
		t.Errorf("expected default category 'other', got %q", items[0].Category)
	}
	if items[0].ResolutionYear != nil {
		t.Errorf("expected nil resolution year, got %v", *items[0].ResolutionYear)
	}
}
