package store

import (
	"context"
	"testing"
)

// firstSeededSkill returns a skill known to exist in the code seed so the test
// is not coupled to a specific key ordering.
func firstSeededSkill(t *testing.T, s *SQLiteStore) Skill {
	t.Helper()
	skills, err := s.ListSkills(context.Background())
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("expected seeded skills")
	}
	return skills[0]
}

func promptOf(t *testing.T, s *SQLiteStore, id int64) string {
	t.Helper()
	sk, err := s.GetSkill(context.Background(), id)
	if err != nil || sk == nil {
		t.Fatalf("get skill %d: %v", id, err)
	}
	return sk.Prompt
}

// A custom prompt set by an admin must survive a re-seed on the next boot,
// while a never-edited skill still tracks the code default.
func TestSetSkillPromptSurvivesReseed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sk := firstSeededSkill(t, s)
	if sk.PromptUpdatedAt != nil {
		t.Fatal("freshly seeded skill should have no prompt_updated_at")
	}

	const custom = "CUSTOM PROMPT — do not clobber me."
	if err := s.SetSkillPrompt(ctx, sk.ID, custom, "admin@example.com"); err != nil {
		t.Fatalf("set skill prompt: %v", err)
	}

	// Re-run the boot seed: the customized prompt must be preserved.
	if err := s.seedSkills(); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	if got := promptOf(t, s, sk.ID); got != custom {
		t.Fatalf("custom prompt clobbered by reseed: got %q", got)
	}

	// The metadata should surface via ListUserSkills.
	us, err := s.ListUserSkills(ctx, 1)
	if err != nil {
		t.Fatalf("list user skills: %v", err)
	}
	var found *UserSkill
	for i := range us {
		if us[i].ID == sk.ID {
			found = &us[i]
			break
		}
	}
	if found == nil {
		t.Fatal("edited skill missing from ListUserSkills")
	}
	if found.PromptUpdatedAt == nil {
		t.Error("expected prompt_updated_at to be set after edit")
	}
	if found.PromptUpdatedBy != "admin@example.com" {
		t.Errorf("prompt_updated_by = %q, want admin@example.com", found.PromptUpdatedBy)
	}
}

// Resetting hands the prompt back to the seed: the default is restored and the
// customization stamp is cleared.
func TestResetSkillPromptRestoresDefault(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sk := firstSeededSkill(t, s)
	def := sk.Prompt

	if err := s.SetSkillPrompt(ctx, sk.ID, "temporary override", "admin@example.com"); err != nil {
		t.Fatalf("set skill prompt: %v", err)
	}
	// Empty updatedBy = reset; caller passes the code default explicitly.
	if err := s.SetSkillPrompt(ctx, sk.ID, DefaultSkillPrompt(sk.Key), ""); err != nil {
		t.Fatalf("reset skill prompt: %v", err)
	}

	got, err := s.GetSkill(ctx, sk.ID)
	if err != nil || got == nil {
		t.Fatalf("get skill: %v", err)
	}
	if got.Prompt != def {
		t.Errorf("prompt not restored: got %q want %q", got.Prompt, def)
	}
	if got.PromptUpdatedAt != nil {
		t.Error("prompt_updated_at should be cleared after reset")
	}
	if got.PromptUpdatedBy != "" {
		t.Errorf("prompt_updated_by should be cleared, got %q", got.PromptUpdatedBy)
	}
}
