package store

import (
	"context"
	"testing"
)

// findSkillByKey returns the seeded skill with the given key, failing the test
// if it is missing.
func findSkillByKey(t *testing.T, s *SQLiteStore, key string) Skill {
	t.Helper()
	skills, err := s.ListSkills(context.Background())
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	for _, sk := range skills {
		if sk.Key == key {
			return sk
		}
	}
	t.Fatalf("skill %q not found", key)
	return Skill{}
}

// TestUpdateSkillPromptPersists verifies an edited prompt is stored and, most
// importantly, is NOT clobbered by a subsequent boot-time re-seed — the prompt
// is DB-owned once written so it can be managed from the UI.
func TestUpdateSkillPromptPersists(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sk := findSkillByKey(t, s, "bucket_list")
	const edited = "EDITED PROMPT — managed from the UI."

	if err := s.UpdateSkillPrompt(ctx, sk.ID, edited); err != nil {
		t.Fatalf("update skill prompt: %v", err)
	}

	got, err := s.GetSkill(ctx, sk.ID)
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	if got.Prompt != edited {
		t.Fatalf("prompt not saved: got %q, want %q", got.Prompt, edited)
	}

	// Simulate a server restart: re-seeding must leave the edited prompt intact
	// while still re-syncing code-owned metadata.
	if err := s.seedSkills(); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	got, err = s.GetSkill(ctx, sk.ID)
	if err != nil {
		t.Fatalf("get skill after re-seed: %v", err)
	}
	if got.Prompt != edited {
		t.Errorf("re-seed clobbered edited prompt: got %q, want %q", got.Prompt, edited)
	}
	if got.Name != sk.Name {
		t.Errorf("re-seed lost code-owned name: got %q, want %q", got.Name, sk.Name)
	}
}
