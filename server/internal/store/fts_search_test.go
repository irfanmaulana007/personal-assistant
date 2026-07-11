package store

import (
	"context"
	"testing"
)

func TestSQLiteFTS5Query(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple words", "project roadmap", `"project" OR "roadmap"`},
		{"drops short and dedupes", "go go to a big plan", `"big" OR "plan"`},
		{"strips punctuation", "meeting, notes! (draft)", `"meeting" OR "notes" OR "draft"`},
		{"fts5 metacharacters are neutralized", `"quote" AND drop*`, `"quote" OR "and" OR "drop"`},
		{"all noise yields empty", "a i o - , !", ""},
		{"empty yields empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sqliteFTS5Query(tt.in); got != tt.want {
				t.Fatalf("sqliteFTS5Query(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestSearchSanitizesRawText proves the store sanitizes raw query text end-to-end
// against FTS5 — including input that would break a raw MATCH — for both memories
// and notes.
func TestSearchSanitizesRawText(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u, err := s.CreateUser(ctx, "search@example.com", "hash", "member")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if _, err := s.CreateMemory(ctx, u.ID, "I love hiking mountains in summer", ""); err != nil {
		t.Fatalf("create memory: %v", err)
	}
	if _, err := s.CreateNote(ctx, u.ID, "Trip plan", "hiking gear checklist for the mountains", ""); err != nil {
		t.Fatalf("create note: %v", err)
	}

	// Raw text with punctuation and FTS5-special characters must not error and
	// must still match on the alphanumeric tokens.
	mems, err := s.SearchMemories(ctx, u.ID, `hiking, "mountains"!`, 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	if len(mems) == 0 {
		t.Fatalf("expected memory match for raw punctuated query, got none")
	}

	notes, err := s.SearchNotes(ctx, u.ID, `hiking AND mountains*`)
	if err != nil {
		t.Fatalf("search notes: %v", err)
	}
	if len(notes) == 0 {
		t.Fatalf("expected note match for raw query with FTS5 metacharacters, got none")
	}

	// An all-noise query returns no rows and no error (empty FTS query short-circuit).
	empty, err := s.SearchMemories(ctx, u.ID, "a - !", 10)
	if err != nil {
		t.Fatalf("search memories (noise): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no matches for noise query, got %d", len(empty))
	}
}
