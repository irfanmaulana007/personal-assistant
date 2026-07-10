package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// fakeTranslator stands in for the LLM-backed translator: it uppercases titles
// and suffixes notes so tests can assert the store applied normalization to the
// right fields.
type fakeTranslator struct{}

func (fakeTranslator) Title(_ context.Context, text string) string {
	return strings.ToUpper(text)
}

func (fakeTranslator) Text(_ context.Context, text string) string {
	if text == "" {
		return text
	}
	return text + " [en]"
}

func TestTranslatorNormalizesReminderTitle(t *testing.T) {
	s := newTestStore(t)
	s.SetTranslator(fakeTranslator{})
	ctx := context.Background()

	// Create: the title is normalized before it is stored (both title & message).
	r, err := s.CreateReminder(ctx, 1, ReminderInput{
		Title:      "minum obat",
		RepeatMode: "daily",
		Times:      []string{"08:00"},
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create reminder: %v", err)
	}
	if r.Title != "MINUM OBAT" {
		t.Errorf("title not normalized on create: %q", r.Title)
	}
	if r.Message != "MINUM OBAT" {
		t.Errorf("message not normalized on create: %q", r.Message)
	}

	// Update also normalizes.
	if err := s.UpdateReminder(ctx, 1, r.ID, ReminderInput{
		Title:      "olahraga pagi",
		RepeatMode: "daily",
		Times:      []string{"06:00"},
		Enabled:    true,
	}); err != nil {
		t.Fatalf("update reminder: %v", err)
	}
	got, _ := s.GetReminder(ctx, 1, r.ID)
	if got.Title != "OLAHRAGA PAGI" {
		t.Errorf("title not normalized on update: %q", got.Title)
	}

	// Legacy one-shot path normalizes too.
	lr, err := s.CreateLegacyReminder(ctx, 1, "telepon ibu", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create legacy reminder: %v", err)
	}
	if lr.Title != "TELEPON IBU" {
		t.Errorf("legacy title not normalized: %q", lr.Title)
	}
}

func TestTranslatorNormalizesLifeGoal(t *testing.T) {
	s := newTestStore(t)
	s.SetTranslator(fakeTranslator{})
	ctx := context.Background()

	g, err := s.CreateLifeGoal(ctx, 1, "belajar menyelam", "kelas pemula")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if g.Title != "BELAJAR MENYELAM" || g.Note != "kelas pemula [en]" {
		t.Errorf("life goal not normalized on create: %+v", g)
	}

	if err := s.UpdateLifeGoal(ctx, 1, g.ID, "keanggotaan gym", "dekat kantor"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := s.GetLifeGoal(ctx, 1, g.ID)
	if got.Title != "KEANGGOTAAN GYM" || got.Note != "dekat kantor [en]" {
		t.Errorf("life goal not normalized on update: %+v", got)
	}
}

func TestNoTranslatorStoresAsIs(t *testing.T) {
	s := newTestStore(t) // no translator injected
	ctx := context.Background()

	g, err := s.CreateLifeGoal(ctx, 1, "belajar menyelam", "kelas pemula")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if g.Title != "belajar menyelam" || g.Note != "kelas pemula" {
		t.Errorf("expected text stored verbatim without a translator: %+v", g)
	}
}
