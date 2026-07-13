//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

// tunedPromptFor reads a skill's tuned_prompt override via the public store API
// (the Postgres ListSkills projects tuned_prompt into Skill.TunedPrompt).
func tunedPromptFor(t *testing.T, ctx context.Context, s Store, key string) string {
	t.Helper()
	skills, err := s.ListSkills(ctx)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	for _, sk := range skills {
		if sk.Key == key {
			return sk.TunedPrompt
		}
	}
	t.Fatalf("skill %q not found", key)
	return ""
}

// TestListLowScoreTracesWithSkills covers the self-tuner's review query: it must
// return only scored-low traces that (a) belong to the user, (b) have a
// non-empty skills array, and (c) score at or below the threshold — worst
// first — and must populate Skills/Tools for the report.
func TestListLowScoreTracesWithSkills(t *testing.T) {
	s := newTestHybrid(t)
	ctx := context.Background()
	const uid = int64(7)

	// low score + skills → should match (worst)
	idLow, _ := s.CreateTrace(ctx, &Trace{UserID: uid, Platform: "web", Input: "hi", Output: "bad", Model: "m", Status: "ok",
		Skills: []string{"web_search"}, Tools: []ToolInvocation{{Name: "web_search", Result: "nothing"}}})
	// borderline (== threshold) + skills → should match
	idEq, _ := s.CreateTrace(ctx, &Trace{UserID: uid, Platform: "web", Input: "q", Output: "meh", Model: "m", Status: "ok",
		Skills: []string{"bucket_list"}})
	// low score but NO skills → excluded
	idNoSkill, _ := s.CreateTrace(ctx, &Trace{UserID: uid, Platform: "web", Input: "x", Output: "y", Model: "m", Status: "ok"})
	// low score + skills but a DIFFERENT user → excluded
	idOther, _ := s.CreateTrace(ctx, &Trace{UserID: 99, Platform: "web", Input: "z", Output: "w", Model: "m", Status: "ok",
		Skills: []string{"web_search"}})
	// good score + skills → excluded
	idGood, _ := s.CreateTrace(ctx, &Trace{UserID: uid, Platform: "web", Input: "g", Output: "great", Model: "m", Status: "ok",
		Skills: []string{"english_tutor"}})

	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idLow, Accuracy: 1, Helpfulness: 2, Safety: 2, Overall: 1.5, Rationale: "poor", JudgeModel: "j"})
	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idEq, Accuracy: 4, Helpfulness: 4, Safety: 4, Overall: 4.0, JudgeModel: "j"})
	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idNoSkill, Accuracy: 1, Helpfulness: 1, Safety: 1, Overall: 1.0, JudgeModel: "j"})
	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idOther, Accuracy: 1, Helpfulness: 1, Safety: 1, Overall: 1.0, JudgeModel: "j"})
	_ = s.SaveTraceScore(ctx, &TraceScore{TraceID: idGood, Accuracy: 5, Helpfulness: 5, Safety: 5, Overall: 5.0, JudgeModel: "j"})
	// idNoSkill unscored-skills case already covered; leave it scored to prove the
	// skills filter (not the score) is what excludes it.

	now := time.Now()
	got, err := s.ListLowScoreTracesWithSkills(ctx, uid, now.Add(-24*time.Hour), now.Add(time.Hour), 4.0, []string{"start_of_day", "end_of_day"}, 50)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d: %+v", len(got), got)
	}
	// Worst score first.
	if got[0].ID != idLow || got[1].ID != idEq {
		t.Fatalf("expected worst-first [%d,%d], got [%d,%d]", idLow, idEq, got[0].ID, got[1].ID)
	}
	// Detail is populated for the report.
	if len(got[0].Skills) == 0 || got[0].Skills[0] != "web_search" {
		t.Fatalf("skills not populated: %+v", got[0].Skills)
	}
	if len(got[0].Tools) == 0 || got[0].Tools[0].Name != "web_search" {
		t.Fatalf("tools not populated: %+v", got[0].Tools)
	}
	_ = idGood
}

// TestUpdateSkillTunedPrompt covers the override write + EffectivePrompt fallback
// and that clearing reverts to the shipped default.
func TestUpdateSkillTunedPrompt(t *testing.T) {
	s := newTestHybrid(t)
	ctx := context.Background()

	skills, err := s.ListSkills(ctx)
	if err != nil || len(skills) == 0 {
		t.Fatalf("list skills: %v (n=%d)", err, len(skills))
	}
	target := skills[0]
	def := target.Prompt
	if def == "" {
		t.Skip("seed skill has no default prompt to compare")
	}
	if target.EffectivePrompt() != def {
		t.Fatalf("expected effective==default before tuning")
	}

	if err := s.UpdateSkillTunedPrompt(ctx, target.Key, "TUNED VERSION"); err != nil {
		t.Fatalf("update: %v", err)
	}
	tuned := tunedPromptFor(t, ctx, s, target.Key)
	if tuned != "TUNED VERSION" {
		t.Fatalf("tuned prompt not saved, got %q", tuned)
	}
	// EffectivePrompt prefers the override.
	if (Skill{Prompt: def, TunedPrompt: tuned}).EffectivePrompt() != "TUNED VERSION" {
		t.Fatalf("EffectivePrompt should prefer the override")
	}

	// Clearing reverts.
	if err := s.UpdateSkillTunedPrompt(ctx, target.Key, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	tuned = tunedPromptFor(t, ctx, s, target.Key)
	if tuned != "" {
		t.Fatalf("expected tuned cleared, got %q", tuned)
	}
	if err := s.UpdateSkillTunedPrompt(ctx, "does_not_exist", "x"); err == nil {
		t.Fatalf("expected error updating unknown skill")
	}
}
