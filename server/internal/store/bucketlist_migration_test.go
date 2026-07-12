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

// Reproduces the production bug: a stale `life_goals` skill-catalog row (whose
// old prompt told the model to call the non-existent lifegoal_add tool) lingers
// alongside the new `bucket_list` row and stays enabled for a user. Verifies
// that seeding prunes the orphan row (and its per-user toggle) so only
// bucket_list remains.
func TestStaleLifeGoalsSkillIsPruned(t *testing.T) {
	path := t.TempDir() + "/stale-skill.db"
	s, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	// Simulate the orphan: a life_goals skill row that seeding never renamed
	// (the Postgres backend had no rename step), enabled for user 1.
	res, err := s.db.Exec(
		`INSERT INTO skills (key, name, description, prompt, category, default_enabled, sort_order)
		 VALUES ('life_goals', 'Life Goals', 'old', 'Use lifegoal_add to save goals.', 'Personal', 1, 1)`)
	if err != nil {
		t.Fatalf("insert stale skill: %v", err)
	}
	skillID, _ := res.LastInsertId()
	if _, err := s.db.Exec(
		`INSERT INTO user_skills (user_id, skill_id, enabled) VALUES (1, ?, 1)`, skillID); err != nil {
		t.Fatalf("enable stale skill: %v", err)
	}

	// Re-run seeding, as happens on every boot.
	if err := s.seedSkills(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// The stale life_goals row (and its toggle) must be gone; bucket_list stays.
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM skills WHERE key = 'life_goals'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("stale life_goals skill row not pruned")
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM user_skills WHERE skill_id = ?`, skillID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("stale user_skills toggle not pruned")
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM skills WHERE key = 'bucket_list'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected bucket_list skill to remain, got count %d", count)
	}

	// The user's enabled keys must no longer surface life_goals.
	keys, err := s.EnabledSkillKeys(context.Background(), 1)
	if err != nil {
		t.Fatalf("enabled keys: %v", err)
	}
	for _, k := range keys {
		if k == "life_goals" {
			t.Errorf("life_goals still reported as an enabled skill key: %v", keys)
		}
	}
}
